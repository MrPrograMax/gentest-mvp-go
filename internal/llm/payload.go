// Пакет llm содержит инфраструктуру для LLM-провайдера фикстур.
//
// На текущем этапе реализован только dry-run:
// BuildFixtureRequest строит JSON-payload, который будет отправлен в LLM,
// а реальный HTTP-клиент ещё не подключён.
//
// Контракт с LLM:
// Генератор отправляет в LLM описание функции + её сценарии и ожидает назад
// JSON с конкретными Go-значениями для каждого поля каждого сценария.
// LLM не пишет Go-код — только JSON. Генератор сам валидирует и конвертирует.
package llm

import "github.com/yourorg/testgen/internal/model"

// ── Request ───────────────────────────────────────────────────────────────────

// Request — полный JSON-payload для LLM.
// Отправляется как тело запроса к модели.
type Request struct {
	// Function — описание тестируемой функции.
	Function FunctionPayload `json:"function"`

	// Scenarios — список сценариев, для которых нужны фикстуры.
	// LLM должна вернуть значения полей для каждого сценария.
	Scenarios []ScenarioPayload `json:"scenarios"`

	// Instructions — системная инструкция для модели (что от неё ожидается).
	Instructions string `json:"instructions"`
}

// FunctionPayload — описание функции для LLM.
type FunctionPayload struct {
	// Name — имя функции, например "ValidateRegisterRequest".
	Name string `json:"name"`

	// Package — имя пакета, например "registration".
	Package string `json:"package"`

	// Params — список параметров функции.
	Params []ParamPayload `json:"params"`

	// FieldGuards — guard-проверки по полям struct-параметров.
	// Помогают LLM понять, какие значения намеренно невалидны для каждого сценария.
	FieldGuards []FieldGuardPayload `json:"field_guards,omitempty"`
}

// ParamPayload — описание одного параметра функции.
type ParamPayload struct {
	// Name — имя параметра, например "req".
	Name string `json:"name"`

	// TypeStr — строковый тип параметра, например "RegisterRequest".
	TypeStr string `json:"type"`

	// Kind — классификация типа: "string", "int", "struct" и т.д.
	Kind string `json:"kind"`

	// StructFields — поля структуры (заполняется только для kind=struct).
	StructFields []StructFieldPayload `json:"struct_fields,omitempty"`
}

// StructFieldPayload — описание одного поля структуры.
// Рекурсивно вложен для nested struct (например Address внутри RegisterRequest).
type StructFieldPayload struct {
	// Name — имя поля, например "Email".
	Name string `json:"name"`

	// TypeStr — тип поля, например "string", "int", "time.Time", "Address".
	TypeStr string `json:"type"`

	// Kind — классификация типа: "string", "int", "struct", "time" и т.д.
	Kind string `json:"kind"`

	// Fields — вложенные поля (только для kind=struct).
	Fields []StructFieldPayload `json:"fields,omitempty"`
}

// FieldGuardPayload — guard-проверка по полю структуры.
type FieldGuardPayload struct {
	// Param — имя параметра, которому принадлежит поле, например "req".
	Param string `json:"param"`

	// FieldPath — путь к полю: ["Email"] или ["Address", "City"].
	FieldPath []string `json:"field_path"`

	// Kind — вид проверки: "empty", "invalid", "less_than", "nil".
	Kind string `json:"kind"`

	// Threshold — пороговое значение для "less_than", например "18".
	Threshold string `json:"threshold,omitempty"`
}

// ScenarioPayload — описание одного тестового сценария.
type ScenarioPayload struct {
	// Name — имя сценария, например "ValidateRegisterRequest/error_empty_email".
	Name string `json:"name"`

	// Kind — тип сценария: "success", "error", "edge".
	Kind string `json:"kind"`

	// WantError — ожидается ли ошибка от функции.
	WantError bool `json:"want_error"`

	// Hint — подсказка для LLM о том, что нужно сделать с данными.
	// Например: "set Email to empty string" или "use valid values for all fields".
	Hint string `json:"hint"`
}

// ── Builder ───────────────────────────────────────────────────────────────────

// instructions — системная инструкция, отправляемая вместе с каждым запросом.
// LLM должна вернуть JSON с конкретными Go-значениями, не Go-код.
const instructions = `You are a Go test data generator. 
Return ONLY a JSON object with fixture values for each scenario.
Do NOT generate Go code. Only JSON.
For each scenario, provide concrete values for each parameter field.
Values must satisfy the scenario semantics (e.g. for error_empty_email, Email must be "").
Response format:
{
  "scenarios": {
    "<scenario_name>": {
      "<param_name>": {
        "<field_name>": <value>
      }
    }
  }
}`

// BuildFixtureRequest строит Request для отправки в LLM.
//
// Включает:
//   - описание функции и её параметров (с рекурсивными полями struct)
//   - field guards (какие проверки делает функция в теле)
//   - список сценариев с hints для каждого
//
// Используется в dry-run режиме (вывод в stdout) и
// будет использоваться HTTP-клиентом при реальном вызове LLM.
func BuildFixtureRequest(fn model.FunctionSpec, scenarios []model.ScenarioSpec) Request {
	return Request{
		Function:     buildFunctionPayload(fn),
		Scenarios:    buildScenarioPayloads(fn.Name, scenarios),
		Instructions: instructions,
	}
}

func buildFunctionPayload(fn model.FunctionSpec) FunctionPayload {
	fp := FunctionPayload{
		Name:    fn.Name,
		Package: fn.PackageName,
	}

	for _, p := range fn.Params {
		fp.Params = append(fp.Params, buildParamPayload(p))
	}

	for _, g := range fn.Guards.FieldGuards {
		fp.FieldGuards = append(fp.FieldGuards, buildFieldGuardPayload(g))
	}

	return fp
}

func buildParamPayload(p model.ParamSpec) ParamPayload {
	pp := ParamPayload{
		Name:    p.Name,
		TypeStr: p.TypeStr,
		Kind:    kindName(p.Kind),
	}

	if p.Kind == model.KindStruct {
		for _, sf := range p.StructFields {
			pp.StructFields = append(pp.StructFields, buildStructFieldPayload(sf))
		}
	}

	return pp
}

func buildStructFieldPayload(sf model.StructField) StructFieldPayload {
	sp := StructFieldPayload{
		Name:    sf.Name,
		TypeStr: sf.TypeStr,
		Kind:    kindName(sf.Kind),
	}

	if sf.Kind == model.KindStruct {
		for _, sub := range sf.SubFields {
			sp.Fields = append(sp.Fields, buildStructFieldPayload(sub))
		}
	}

	return sp
}

func buildFieldGuardPayload(g model.FieldGuard) FieldGuardPayload {
	return FieldGuardPayload{
		Param:     g.ParamName,
		FieldPath: g.FieldPath,
		Kind:      string(g.Kind),
		Threshold: g.Threshold,
	}
}

func buildScenarioPayloads(fnName string, scenarios []model.ScenarioSpec) []ScenarioPayload {
	out := make([]ScenarioPayload, 0, len(scenarios))
	for _, sc := range scenarios {
		out = append(out, ScenarioPayload{
			Name:      sc.Name,
			Kind:      string(sc.Kind),
			WantError: sc.WantError,
			Hint:      scenarioHint(fnName, sc),
		})
	}
	return out
}

// scenarioHint формирует короткую подсказку для LLM о смысле сценария.
func scenarioHint(fnName string, sc model.ScenarioSpec) string {
	switch sc.Kind {
	case model.ScenarioSuccess:
		return "use valid values for all fields so the function returns no error"
	case model.ScenarioError:
		return "use zero/invalid values so the function returns an error"
	case model.ScenarioEdge:
		return "use boundary values; function is expected to return an error"
	default:
		return ""
	}
}

// kindName возвращает строковое имя TypeKind для JSON-payload.
func kindName(k model.TypeKind) string {
	switch k {
	case model.KindString:
		return "string"
	case model.KindInt:
		return "int"
	case model.KindBool:
		return "bool"
	case model.KindStruct:
		return "struct"
	case model.KindTime:
		return "time"
	case model.KindDuration:
		return "duration"
	case model.KindSlice:
		return "slice"
	case model.KindMap:
		return "map"
	case model.KindPtr:
		return "ptr"
	case model.KindInterface:
		return "interface"
	case model.KindFunc:
		return "func"
	case model.KindError:
		return "error"
	default:
		return "unknown"
	}
}
