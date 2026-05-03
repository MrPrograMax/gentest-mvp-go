package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ── Response model ────────────────────────────────────────────────────────────

// FixtureResponse — разобранный JSON-ответ от LLM.
//
// Ожидаемая структура ответа:
//
//	{
//	  "scenarios": {
//	    "ValidateRegisterRequest/success": {
//	      "req": {
//	        "Email": "user@example.com",
//	        "Age": 25,
//	        "Address": { "City": "Moscow", "Street": "Tverskaya" }
//	      }
//	    },
//	    "ValidateRegisterRequest/error_empty_email": {
//	      "req": { "Email": "", ... }
//	    }
//	  }
//	}
//
// Значение параметра — map[string]any: поддерживает и плоские значения
// (string, number, bool) и вложенные struct (map[string]any).
// Дополнительные ключи в ответе разрешены и игнорируются.
type FixtureResponse struct {
	// Scenarios: scenario_name → (param_name → field_values).
	Scenarios map[string]map[string]any `json:"scenarios"`
}

// ── Parser ────────────────────────────────────────────────────────────────────

// ParseFixtureResponse разбирает сырой JSON-ответ от LLM в FixtureResponse.
//
// LLM может обернуть JSON в markdown-блок (```json ... ```).
// ParseFixtureResponse автоматически извлекает JSON из таких блоков.
//
// Возвращает ошибку если:
//   - ответ не является валидным JSON
//   - верхнеуровневый ключ "scenarios" отсутствует
func ParseFixtureResponse(raw string) (FixtureResponse, error) {
	raw = strings.TrimSpace(raw)
	raw = stripMarkdownFence(raw)

	var resp FixtureResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return FixtureResponse{}, fmt.Errorf("parse llm response: invalid JSON: %w", err)
	}

	if resp.Scenarios == nil {
		return FixtureResponse{}, fmt.Errorf("parse llm response: missing top-level key \"scenarios\"")
	}

	return resp, nil
}

// stripMarkdownFence убирает markdown-обёртку ```json ... ``` или ``` ... ```,
// которую некоторые модели добавляют вокруг JSON-блока.
func stripMarkdownFence(s string) string {
	// Ищем открывающий ``` (с опциональным "json")
	for _, prefix := range []string{"```json", "```"} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimPrefix(s, prefix)
			// Убираем закрывающий ```
			if idx := strings.LastIndex(s, "```"); idx >= 0 {
				s = s[:idx]
			}
			return strings.TrimSpace(s)
		}
	}
	return s
}

// ── Validator ─────────────────────────────────────────────────────────────────

// ValidateFixtureResponse проверяет, что ответ LLM содержит все необходимые
// данные согласно исходному запросу req.
//
// Проверяет:
//   - для каждого ScenarioPayload из req есть ключ в resp.Scenarios
//   - для каждого параметра функции есть запись в сценарии
//   - для struct-параметров рекурсивно проверяются поля (включая nested structs)
//   - базовые типы: string→JSON string, int/int64→JSON number, bool→JSON bool,
//     time.Time→JSON string (ISO format от LLM)
//
// Лишние поля и ключи разрешены.
func ValidateFixtureResponse(resp FixtureResponse, req Request) error {
	var errs []string

	for _, sc := range req.Scenarios {
		scVals, ok := resp.Scenarios[sc.Name]
		if !ok {
			errs = append(errs, fmt.Sprintf("missing scenario %q", sc.Name))
			continue
		}

		for _, param := range req.Function.Params {
			paramVal, ok := scVals[param.Name]
			if !ok {
				errs = append(errs,
					fmt.Sprintf("missing param %q in scenario %q", param.Name, sc.Name))
				continue
			}

			if verr := validateParamValue(param, paramVal, sc.Name); verr != "" {
				errs = append(errs, verr)
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.New("llm response validation failed:\n  " + strings.Join(errs, "\n  "))
}

// validateParamValue проверяет значение параметра из ответа LLM.
// Для struct-параметров рекурсивно проверяет поля.
// Возвращает пустую строку если всё OK, иначе — текст ошибки.
func validateParamValue(param ParamPayload, val any, scName string) string {
	switch param.Kind {
	case "struct":
		obj, ok := toStringMap(val)
		if !ok {
			return fmt.Sprintf(
				"param %q in scenario %q expected object, got %T",
				param.Name, scName, val)
		}
		return validateStructFields(param.Name, param.StructFields, obj, scName)

	default:
		return validatePrimitive(
			fmt.Sprintf("param %q", param.Name), param.Kind, val, scName)
	}
}

// validateStructFields рекурсивно проверяет поля struct-объекта.
// path — человекочитаемый путь для сообщений об ошибках (param.name или field).
func validateStructFields(
	path string,
	fields []StructFieldPayload,
	obj map[string]any,
	scName string,
) string {
	var errs []string

	for _, f := range fields {
		fval, ok := obj[f.Name]
		if !ok {
			errs = append(errs,
				fmt.Sprintf("missing field %q in %s in scenario %q", f.Name, path, scName))
			continue
		}

		if f.Kind == "struct" {
			nested, ok := toStringMap(fval)
			if !ok {
				errs = append(errs, fmt.Sprintf(
					"field %q in %s expected object, got %T (scenario %q)",
					f.Name, path, fval, scName))
				continue
			}
			if verr := validateStructFields(path+"."+f.Name, f.Fields, nested, scName); verr != "" {
				errs = append(errs, verr)
			}
		} else {
			fieldPath := path + "." + f.Name
			if verr := validatePrimitive(fieldPath, f.Kind, fval, scName); verr != "" {
				errs = append(errs, verr)
			}
		}
	}

	return strings.Join(errs, "\n  ")
}

// validatePrimitive проверяет, что JSON-значение соответствует ожидаемому kind.
//
// Таблица соответствий:
//
//	string    → JSON string
//	int       → JSON number (float64 в Go)
//	int64     → JSON number
//	bool      → JSON bool
//	time      → JSON string (LLM возвращает ISO-строку)
//	duration  → JSON string или number
//	error     → любое (обычно null или string)
//	interface → любое
//	unknown   → пропускается
//
// path и scName используются только для сообщений об ошибках.
func validatePrimitive(path, kind string, val any, scName string) string {
	switch kind {
	case "string", "time", "duration":
		if _, ok := val.(string); !ok {
			return fmt.Sprintf(
				"field %s expected string (kind=%s), got %s (scenario %q)",
				path, kind, jsonTypeName(val), scName)
		}

	case "int", "int64":
		// JSON numbers декодируются как float64 при unmarshal в any.
		if _, ok := val.(float64); !ok {
			return fmt.Sprintf(
				"field %s expected number, got %s (scenario %q)",
				path, jsonTypeName(val), scName)
		}

	case "bool":
		if _, ok := val.(bool); !ok {
			return fmt.Sprintf(
				"field %s expected bool, got %s (scenario %q)",
				path, jsonTypeName(val), scName)
		}

		// ptr, slice, map, func, interface, error, unknown — принимаем любое значение,
		// включая null. Генератор сам решит что с ними делать.
	}

	return ""
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// toStringMap приводит any к map[string]any.
// Возвращает (nil, false) если значение не является объектом.
func toStringMap(val any) (map[string]any, bool) {
	m, ok := val.(map[string]any)
	return m, ok
}

// jsonTypeName возвращает человекочитаемое имя JSON-типа для сообщений об ошибках.
func jsonTypeName(val any) string {
	if val == nil {
		return "null"
	}
	switch val.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return fmt.Sprintf("%T", val)
	}
}
