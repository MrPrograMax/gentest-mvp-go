// Файл literal.go реализует конвертацию валидированного LLM JSON-ответа
// в Go-литералы для подстановки в ScenarioSpec.Inputs.
//
// Это слой между llm.ValidateFixtureResponse (валидация структуры ответа)
// и render.RenderFile (генерация *_test.go). На текущем этапе функции
// этого файла НЕ подключены к основному generation flow в app.Run —
// они доступны как внутренний слой и покрыты тестами, но интеграция
// под --fixture=llm выполнена не будет до отдельного шага.
//
// Поддерживаемые конверсии (MVP-сet, согласованный с ParamSpec.Kind):
//
//	JSON string  → Go quoted string (через strconv.Quote).
//	JSON number  → Go int literal без кавычек.
//	JSON bool    → "true" / "false".
//	JSON string  → time.Date(...) для kind=time, парсится как RFC3339.
//	JSON object  → composite literal "Type{Field: ..., Field: ...}".
//
// Любой другой Kind (slice, map, ptr, interface, func, error, duration,
// unknown) приведёт к ошибке. Это намеренно: LLM-генератор фикстур
// должен обрабатывать только семантически осмысленные значения, а
// сложные типы остаются за heuristic-провайдером.
//
// Пакетный квалификатор у struct-литералов опционально стрипается
// (см. stripPkgs у ApplyFixtureResponseToScenarios). Это нужно когда
// тест генерируется в том же пакете, что и source: "registration.Address"
// должен превратиться в "Address{...}". Для external test package
// (когда тест в pkg_test) стрипать ничего не нужно.

package llm

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yourorg/testgen/internal/model"
)

// ── Atomic helpers (экспортированы для тестов и переиспользования) ────────────

// JSONStringToGoLiteral квотирует s как валидный Go string literal.
// Использует strconv.Quote: корректно экранирует кавычки, переводы строк,
// non-ASCII символы.
func JSONStringToGoLiteral(s string) string {
	return strconv.Quote(s)
}

// JSONNumberToIntLiteral конвертирует JSON number (декодированный
// encoding/json как float64) в строковое представление целого числа.
//
// Возвращает ошибку если значение имеет дробную часть — JSON-число
// должно соответствовать целочисленному полю Go.
func JSONNumberToIntLiteral(n float64) (string, error) {
	if n != float64(int64(n)) {
		return "", fmt.Errorf("number %v has fractional part, expected integer", n)
	}
	return strconv.FormatInt(int64(n), 10), nil
}

// JSONBoolToGoLiteral возвращает "true" или "false".
func JSONBoolToGoLiteral(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// TimeStringToGoLiteral парсит RFC3339-строку и возвращает Go-литерал вида
// "time.Date(YYYY, time.Month, D, h, m, s, ns, time.UTC)".
//
// Принимает оба варианта: RFC3339 ("2026-01-01T00:00:00Z") и RFC3339Nano
// ("2026-01-01T00:00:00.123456789Z").
//
// Время приводится к UTC: эквивалентный instant, детерминированный вывод.
// Возвращает ошибку для строки, не соответствующей RFC3339.
//
// Важно: возвращаемая строка содержит "time." — это автоматически попадает
// в render.exprImportTable и приводит к импорту "time" в сгенерированном файле.
func TimeStringToGoLiteral(s string) (string, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return "", fmt.Errorf("time string %q is not RFC3339: %w", s, err)
		}
	}
	t = t.UTC()
	return fmt.Sprintf(
		"time.Date(%d, time.%s, %d, %d, %d, %d, %d, time.UTC)",
		t.Year(),
		t.Month().String(),
		t.Day(),
		t.Hour(),
		t.Minute(),
		t.Second(),
		t.Nanosecond(),
	), nil
}

// ── Top-level конверсия ───────────────────────────────────────────────────────

// ApplyFixtureResponseToScenarios строит новый []ScenarioSpec, в котором
// поле Inputs каждого сценария заменено на Go-литералы, построенные из
// resp по схеме параметров fn.Params.
//
// Сохраняются: Name, Kind, Comment, Wants, WantError. Заменяется только Inputs.
//
// stripPkgs (variadic) — список префиксов "pkg.", которые нужно убрать у
// имён struct-типов. Передавай fn.PackageName когда тест генерируется в том
// же пакете, что и source ("registration.Address" → "Address"). Для
// external test package (pkg_test) аргумент опускается — квалификатор
// сохраняется ("registration.Address").
//
// Ошибки:
//   - missing scenario "<name>"
//   - missing param "<name>" in scenario "<sc>"
//   - missing field "<dotted>" in scenario "<sc>"
//   - field "<dotted>" expected <kind>, got <jsontype> (scenario "<sc>")
//   - field "<dotted>": time string ... is not RFC3339 (scenario "<sc>")
//   - field "<dotted>": unsupported kind ... (scenario "<sc>")
//
// Этот слой никогда не делает silent fallback на heuristic; вызывающая
// сторона решает, как реагировать на ошибку.
func ApplyFixtureResponseToScenarios(
	resp FixtureResponse,
	fn model.FunctionSpec,
	scenarios []model.ScenarioSpec,
	stripPkgs ...string,
) ([]model.ScenarioSpec, error) {
	if resp.Scenarios == nil {
		return nil, errors.New("apply fixtures: response has no scenarios")
	}

	out := make([]model.ScenarioSpec, len(scenarios))
	for i, sc := range scenarios {
		scVals, ok := resp.Scenarios[sc.Name]
		if !ok {
			return nil, fmt.Errorf("apply fixtures: missing scenario %q", sc.Name)
		}

		newInputs := make([]model.FixtureValue, len(fn.Params))
		for j, p := range fn.Params {
			paramVal, ok := scVals[p.Name]
			if !ok {
				return nil, fmt.Errorf(
					"apply fixtures: missing param %q in scenario %q",
					p.Name, sc.Name)
			}
			expr, err := paramToLiteral(p, paramVal, sc.Name, stripPkgs)
			if err != nil {
				return nil, err
			}
			newInputs[j] = model.FixtureValue{
				Expr:    expr,
				TypeStr: p.TypeStr,
			}
		}

		// Копируем сценарий по значению и заменяем только Inputs.
		// Wants/WantError/Name/Kind/Comment остаются нетронутыми.
		next := sc
		next.Inputs = newInputs
		out[i] = next
	}
	return out, nil
}

// ── Внутренние помощники ──────────────────────────────────────────────────────

// paramToLiteral строит литерал для одного параметра функции.
// Для KindStruct рекурсивно собирает composite literal через structLiteral.
// Для примитивов делегирует в primitiveLiteral.
func paramToLiteral(p model.ParamSpec, val any, scName string, stripPkgs []string) (string, error) {
	if p.Kind == model.KindStruct {
		obj, ok := val.(map[string]any)
		if !ok {
			return "", fmt.Errorf(
				"apply fixtures: param %q in scenario %q expected object, got %s",
				p.Name, scName, jsonTypeName(val))
		}
		return structLiteral(p.TypeStr, p.StructFields, obj, p.Name, scName, stripPkgs)
	}
	return primitiveLiteral(p.Kind, val, p.Name, scName)
}

// structLiteral собирает composite literal "TypeName{F1: v1, F2: v2, ...}".
// path — точечный путь "req" или "req.Address", используется в сообщениях
// об ошибках.
//
// Для пустого fields возвращает "TypeName{}" — go/format корректно
// нормализует. Это фоллбэк на случай, если StructFields не были заполнены
// analyzer-ом (опционально); MVP покрывает только заполненный случай.
func structLiteral(
	typeStr string,
	fields []model.StructField,
	obj map[string]any,
	path, scName string,
	stripPkgs []string,
) (string, error) {
	typeName := stripPkgPrefix(typeStr, stripPkgs)
	if len(fields) == 0 {
		return typeName + "{}", nil
	}

	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		fval, ok := obj[f.Name]
		if !ok {
			return "", fmt.Errorf(
				"apply fixtures: missing field %q in scenario %q",
				path+"."+f.Name, scName)
		}

		var lit string
		if f.Kind == model.KindStruct {
			nested, ok := fval.(map[string]any)
			if !ok {
				return "", fmt.Errorf(
					"apply fixtures: field %q expected object, got %s (scenario %q)",
					path+"."+f.Name, jsonTypeName(fval), scName)
			}
			sub, err := structLiteral(
				f.TypeStr, f.SubFields, nested,
				path+"."+f.Name, scName, stripPkgs,
			)
			if err != nil {
				return "", err
			}
			lit = sub
		} else {
			l, err := primitiveLiteral(f.Kind, fval, path+"."+f.Name, scName)
			if err != nil {
				return "", err
			}
			lit = l
		}
		parts = append(parts, f.Name+": "+lit)
	}
	return typeName + "{" + strings.Join(parts, ", ") + "}", nil
}

// primitiveLiteral конвертирует одно JSON-значение в Go-литерал по Kind.
// path и scName — для сообщений об ошибках.
func primitiveLiteral(kind model.TypeKind, val any, path, scName string) (string, error) {
	switch kind {
	case model.KindString:
		s, ok := val.(string)
		if !ok {
			return "", fmt.Errorf(
				"apply fixtures: field %q expected string, got %s (scenario %q)",
				path, jsonTypeName(val), scName)
		}
		return JSONStringToGoLiteral(s), nil

	case model.KindInt:
		n, ok := val.(float64)
		if !ok {
			return "", fmt.Errorf(
				"apply fixtures: field %q expected number, got %s (scenario %q)",
				path, jsonTypeName(val), scName)
		}
		lit, err := JSONNumberToIntLiteral(n)
		if err != nil {
			return "", fmt.Errorf(
				"apply fixtures: field %q: %w (scenario %q)", path, err, scName)
		}
		return lit, nil

	case model.KindBool:
		b, ok := val.(bool)
		if !ok {
			return "", fmt.Errorf(
				"apply fixtures: field %q expected bool, got %s (scenario %q)",
				path, jsonTypeName(val), scName)
		}
		return JSONBoolToGoLiteral(b), nil

	case model.KindTime:
		s, ok := val.(string)
		if !ok {
			return "", fmt.Errorf(
				"apply fixtures: field %q expected time string, got %s (scenario %q)",
				path, jsonTypeName(val), scName)
		}
		lit, err := TimeStringToGoLiteral(s)
		if err != nil {
			return "", fmt.Errorf(
				"apply fixtures: field %q: %w (scenario %q)", path, err, scName)
		}
		return lit, nil

	default:
		return "", fmt.Errorf(
			"apply fixtures: field %q: unsupported kind for literal conversion (scenario %q)",
			path, scName)
	}
}

// stripPkgPrefix убирает первый совпавший префикс "pkg." из typeStr.
// Зеркалит fixture.stripPkgPrefix (там unexported), чтобы не вводить
// циклическую зависимость llm → fixture.
func stripPkgPrefix(typeStr string, stripPkgs []string) string {
	for _, pkg := range stripPkgs {
		if pkg == "" {
			continue
		}
		prefix := pkg + "."
		if strings.HasPrefix(typeStr, prefix) {
			return typeStr[len(prefix):]
		}
	}
	return typeStr
}
