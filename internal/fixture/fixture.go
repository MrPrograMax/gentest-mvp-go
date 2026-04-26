// Пакет fixture генерирует Go-выражения для тестовых фикстур.
//
// Три уровня фикстур:
//   - Happy — ненулевое валидное значение для success-сценария.
//   - Zero  — нулевое значение (nil для ссылочных типов).
//   - Empty — «пустое» значение: "" для string, []T{} для срезов/map.
//     Отличается от Zero для срезов и map: nil vs T{}.
//
// Все возвращаемые Expr — валидные Go-выражения, пригодные для вставки
// непосредственно в генерируемый исходный код.
package fixture

import (
	"fmt"
	"strings"

	"github.com/yourorg/testgen/internal/model"
)

// Happy возвращает фикстуру «happy path» для success-сценария.
func Happy(kind model.TypeKind, typeStr string) model.FixtureValue {
	switch kind {
	case model.KindString:
		return fv(`"test-value"`, typeStr)
	case model.KindInt:
		return fv("42", typeStr)
	case model.KindBool:
		return fv("true", typeStr)

	case model.KindSlice:
		// Непустой срез: один элемент с happy-значением для элементарного типа.
		elemType := strings.TrimPrefix(typeStr, "[]")
		return fv(typeStr+"{"+happyElemLit(elemType)+"}", typeStr)

	case model.KindMap:
		return fv(typeStr+"{}", typeStr)

	case model.KindPtr:
		// new(T) работает для любого типа T, включая встроенные:
		//   *int       → new(int)       ✓  (&int{} — невалидный Go)
		//   *MyStruct  → new(MyStruct)  ✓
		//   *io.Reader → new(io.Reader) ✓
		inner := strings.TrimPrefix(typeStr, "*")
		return fv("new("+inner+")", typeStr)

	case model.KindTime:
		return fv("time.Now()", typeStr)
	case model.KindDuration:
		return fv("time.Second", typeStr)

	case model.KindStruct:
		return fv(typeStr+"{}", typeStr)

	case model.KindFunc:
		// Безопасная заглушка: возвращает zero-значения вместо panic.
		// Пользователь заменяет тело на реальную логику.
		return fv(safeFuncStub(typeStr), typeStr)

	case model.KindInterface:
		// Для хорошо известных интерфейсов стандартной библиотеки генерируем
		// конкретные фикстуры вместо nil, чтобы success-сценарий не уходил в error-path.
		//
		//   context.Context → context.Background()
		//     — всегда валиден, не отменяется, не требует мока.
		//   io.Reader → strings.NewReader("test-value")
		//     — простой читаемый поток без side-эффектов.
		//
		// Для остальных интерфейсов оставляем nil + TODO-комментарий:
		// пользователь должен подставить реальный mock/stub.
		switch typeStr {
		case "context.Context":
			return fv("context.Background()", typeStr)
		case "io.Reader":
			return fv(`strings.NewReader("test-value")`, typeStr)
		default:
			return model.FixtureValue{Expr: "nil", TypeStr: typeStr, NeedsMockComment: true}
		}

	case model.KindError:
		// error в success-сценарии — просто nil, без mock-комментария.
		return fv("nil", typeStr)

	default:
		// KindUnknown — *new(T) всегда валиден и компилируется.
		return fv(zeroExpr(typeStr), typeStr)
	}
}

// Zero возвращает нулевое/nil значение для типа.
func Zero(kind model.TypeKind, typeStr string) model.FixtureValue {
	switch kind {
	case model.KindString:
		return fv(`""`, typeStr)
	case model.KindInt:
		return fv("0", typeStr)
	case model.KindBool:
		return fv("false", typeStr)
	case model.KindSlice:
		return fv("nil", typeStr)
	case model.KindMap:
		return fv("nil", typeStr)
	case model.KindPtr:
		return fv("nil", typeStr)
	case model.KindTime:
		return fv("time.Time{}", typeStr)
	case model.KindDuration:
		return fv("0", typeStr)
	case model.KindStruct:
		return fv(typeStr+"{}", typeStr)
	case model.KindFunc, model.KindInterface, model.KindError:
		return fv("nil", typeStr)
	default:
		return fv(zeroExpr(typeStr), typeStr)
	}
}

// Empty возвращает «пустое» значение, отличное от нулевого для срезов и строк.
//
// Разграничение Zero / Empty принципиально для edge-сценариев:
//   - Zero(slice)  → nil    (параметр не передан)
//   - Empty(slice) → []T{}  (параметр передан явно пустым)
//
// Это позволяет проверить граничное условие len(x)==0 без имитации nil.
func Empty(kind model.TypeKind, typeStr string) model.FixtureValue {
	switch kind {
	case model.KindString:
		return fv(`""`, typeStr)
	case model.KindSlice:
		// Пустой срез вместо nil — явная передача пустого значения.
		return fv(typeStr+"{}", typeStr)
	case model.KindMap:
		// Пустой map вместо nil — аналогично срезам: явная передача пустого.
		// Zero(map) = nil, Empty(map) = map[K]V{}.
		return fv(typeStr+"{}", typeStr)
	case model.KindPtr, model.KindInterface, model.KindFunc:
		return fv("nil", typeStr)
	default:
		return Zero(kind, typeStr)
	}
}

// NeedsTimeImport сообщает, требует ли хотя бы одна фикстура импорта "time".
func NeedsTimeImport(fvs ...model.FixtureValue) bool {
	for _, f := range fvs {
		if f.TypeStr == "time.Time" || f.TypeStr == "time.Duration" {
			return true
		}
		if f.Expr == "time.Now()" || f.Expr == "time.Second" || f.Expr == "time.Time{}" {
			return true
		}
	}
	return false
}

// ── Вспомогательные функции ───────────────────────────────────────────────────

func fv(expr, typeStr string) model.FixtureValue {
	return model.FixtureValue{Expr: expr, TypeStr: typeStr}
}

// zeroExpr возвращает *new(T) — универсальное нулевое выражение для любого
// именованного типа T, даже когда его вид неизвестен.
func zeroExpr(typeStr string) string {
	if typeStr == "" {
		return "nil"
	}
	return fmt.Sprintf("*new(%s)", typeStr)
}

// happyElemLit возвращает один «осмысленный» элемент для срезов в happy-фикстуре.
// Например: []string{"test-value"}, []int{42}, []MyStruct{MyStruct{}}.
func happyElemLit(elemType string) string {
	switch elemType {
	case "string":
		return `"test-value"`
	case "bool":
		return "true"
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "byte", "rune", "uintptr":
		return "42"
	default:
		if strings.HasPrefix(elemType, "*") {
			// Срез указателей: один nil-элемент лучше, чем невалидный литерал.
			return "nil"
		}
		// Структура или именованный тип — нулевое значение как составной литерал.
		return elemType + "{}"
	}
}

// ── Безопасная func-заглушка ─────────────────────────────────────────────────
//
// safeFuncStub генерирует анонимную функцию, которая возвращает zero-значения
// вместо panic. Это позволяет тесту компилироваться и не падать при случайном
// вызове в non-error сценарии.
//
// Примеры:
//
//	func(string) bool          → func(string) bool { return false }
//	func(string) error         → func(string) error { return nil }
//	func(int, int) (int, error)→ func(int, int) (int, error) { return *new(int), nil }
//	func()                     → func() {}
func safeFuncStub(typeStr string) string {
	results := parseFuncResults(typeStr)
	return typeStr + " " + buildStubBody(results)
}

// buildStubBody формирует тело функции-заглушки по списку типов результатов.
func buildStubBody(results []string) string {
	if len(results) == 0 {
		return "{}"
	}
	zeros := make([]string, len(results))
	for i, r := range results {
		zeros[i] = zeroLitForType(r)
	}
	return "{ return " + strings.Join(zeros, ", ") + " }"
}

// zeroLitForType возвращает Go-выражение нулевого значения для строки типа.
// Используется только внутри func-заглушек; не зависит от TypeKind.
func zeroLitForType(t string) string {
	t = strings.TrimSpace(t)
	switch t {
	case "bool":
		return "false"
	case "error":
		return "nil"
	case "string":
		return `""`
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "byte", "rune", "uintptr":
		return "0"
	}
	// Ссылочные и составные типы — nil.
	if strings.HasPrefix(t, "*") ||
		strings.HasPrefix(t, "[]") ||
		strings.HasPrefix(t, "map[") ||
		strings.HasPrefix(t, "func(") ||
		strings.HasPrefix(t, "chan ") ||
		t == "interface{}" || t == "any" {
		return "nil"
	}
	// Именованный тип (структура, псевдоним) — универсальный zero.
	return "*new(" + t + ")"
}

// parseFuncResults извлекает список строк типов результатов из typeStr вида
// "func(PARAMS)" или "func(PARAMS) T" или "func(PARAMS) (T1, T2)".
//
// Алгоритм: пропускаем параметры считая скобочную глубину, берём остаток.
func parseFuncResults(typeStr string) []string {
	const prefix = "func("
	if !strings.HasPrefix(typeStr, prefix) {
		return nil
	}

	// Пропускаем "func(" и ищем закрывающую скобку params.
	rest := typeStr[len(prefix):]
	depth := 1
	i := 0
	for i < len(rest) && depth > 0 {
		switch rest[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		i++
	}
	// rest[i:] — всё после закрывающей скобки params, например " bool" или " (int, error)".
	after := strings.TrimSpace(rest[i:])

	if after == "" {
		// Нет возвращаемых значений.
		return nil
	}

	if strings.HasPrefix(after, "(") && strings.HasSuffix(after, ")") {
		// Несколько результатов в скобках: (T1, T2, ...).
		inner := after[1 : len(after)-1]
		return splitBalanced(inner, ',')
	}

	// Один результат без скобок.
	return []string{after}
}

// splitBalanced разбивает строку s по разделителю sep,
// игнорируя вхождения sep внутри сбалансированных скобок ( ) [ ].
// Используется для разбора списка типов в сигнатуре функции.
func splitBalanced(s string, sep rune) []string {
	var parts []string
	var cur strings.Builder
	depth := 0
	for _, ch := range s {
		switch ch {
		case '(', '[':
			depth++
			cur.WriteRune(ch)
		case ')', ']':
			depth--
			cur.WriteRune(ch)
		case sep:
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(cur.String()))
				cur.Reset()
			} else {
				cur.WriteRune(ch)
			}
		default:
			cur.WriteRune(ch)
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		parts = append(parts, s)
	}
	return parts
}
