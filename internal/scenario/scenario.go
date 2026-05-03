// Пакет scenario строит []ScenarioSpec из FunctionSpec.
//
// Правила генерации:
//
//	success — генерируется всегда: happy-фикстуры для всех параметров.
//	          Для KindStruct-параметров с StructFields строится полный composite literal
//	          через fixture.HappyStructExpr (с семантическими хэвристиками по имени поля).
//
//	error (generic) — генерируется если функция возвращает error
//	          И у параметров нет FieldGuards.
//	          При наличии FieldGuards заменяется field-specific сценариями.
//
//	error_<kind>_<field> — по одному на каждый FieldGuard:
//	          берём happy-структуру и «ломаем» ровно одно поле (PatchedStructExpr).
//
//	edge — только по Guards-фактам на прямых параметрах (nil/empty guard).
//
// Дедупликация честная: сравниваются Inputs.Expr, Wants.Expr, WantError.
package scenario

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yourorg/testgen/internal/fixture"
	"github.com/yourorg/testgen/internal/model"
)

// Generate возвращает все ScenarioSpec для fn.
func Generate(fn model.FunctionSpec) []model.ScenarioSpec {
	var out []model.ScenarioSpec

	out = append(out, successScenario(fn))

	// Если есть FieldGuards — каждый порождает отдельный error-сценарий.
	// Они заменяют generic error, который был бы TypeName{} и не нёс бы информации.
	fieldScenarios := fieldGuardScenarios(fn)
	if len(fieldScenarios) > 0 {
		out = append(out, fieldScenarios...)
	} else if fn.HasError {
		out = append(out, errorScenario(fn))
	}

	out = append(out, edgeScenarios(fn)...)

	return out
}

// successScenario: каждый параметр получает happy-фикстуру, ошибки не ожидается.
// Для KindStruct-параметров с StructFields строим полный composite literal.
func successScenario(fn model.FunctionSpec) model.ScenarioSpec {
	return model.ScenarioSpec{
		Name:      fn.Name + "/success",
		Kind:      model.ScenarioSuccess,
		Comment:   "TODO: установи want* в ожидаемые возвращаемые значения",
		Inputs:    happyInputs(fn.Params, fn.PackageName),
		Wants:     happyWants(fn.Results),
		WantError: false,
	}
}

// errorScenario: нулевые фикстуры, ожидается ненулевой error.
func errorScenario(fn model.FunctionSpec) model.ScenarioSpec {
	return model.ScenarioSpec{
		Name:      fn.Name + "/error",
		Kind:      model.ScenarioError,
		Comment:   "TODO: подбери inputs, которые ведут к error-ветке",
		Inputs:    zeroInputs(fn.Params),
		Wants:     zeroWants(fn.Results),
		WantError: true,
	}
}

// ── Field-guard сценарии ──────────────────────────────────────────────────────

// fieldGuardScenarios строит по одному error-сценарию на каждый FieldGuard.
//
// Для каждого guard:
//  1. Находим параметр по ParamName.
//  2. Строим «сломанный» composite literal через fixture.PatchedStructExpr:
//     берём happy-структуру и заменяем одно поле на «плохое» значение.
//  3. Все остальные параметры получают happy-фикстуру.
func fieldGuardScenarios(fn model.FunctionSpec) []model.ScenarioSpec {
	if len(fn.Guards.FieldGuards) == 0 {
		return nil
	}

	var out []model.ScenarioSpec
	for _, guard := range fn.Guards.FieldGuards {
		// Ищем параметр.
		paramIdx := -1
		for i, p := range fn.Params {
			if p.Name == guard.ParamName {
				paramIdx = i
				break
			}
		}
		if paramIdx == -1 {
			continue
		}

		patchValue := fieldGuardPatchValue(guard)
		inputs := make([]model.FixtureValue, len(fn.Params))
		for i, p := range fn.Params {
			if i == paramIdx {
				var expr string
				if p.Kind == model.KindStruct && len(p.StructFields) > 0 {
					// fn.PackageName передаётся чтобы убрать квалификатор текущего пакета
					// у вложенных типов: "registration.Address" → "Address"
					expr = fixture.PatchedStructExpr(p.TypeStr, p.StructFields, guard.FieldPath, patchValue, fn.PackageName)
				} else {
					expr = patchValue
				}
				inputs[i] = model.FixtureValue{Expr: expr, TypeStr: p.TypeStr}
			} else {
				inputs[i] = fixture.Happy(p.Kind, p.TypeStr)
			}
		}

		out = append(out, model.ScenarioSpec{
			Name:      fieldGuardScenarioName(fn.Name, guard),
			Kind:      model.ScenarioError,
			Comment:   "TODO: уточни wantErr — поле " + strings.Join(guard.FieldPath, ".") + " намеренно невалидно",
			Inputs:    inputs,
			Wants:     zeroWants(fn.Results),
			WantError: true,
		})
	}
	return out
}

// fieldGuardScenarioName формирует имя сценария из имени функции и FieldGuard.
// Примеры: ValidateRegisterRequest/error_empty_email, .../error_underage
func fieldGuardScenarioName(fnName string, guard model.FieldGuard) string {
	leaf := strings.ToLower(guard.FieldPath[len(guard.FieldPath)-1])
	switch guard.Kind {
	case model.FieldGuardEmpty:
		return fnName + "/error_empty_" + leaf
	case model.FieldGuardInvalid:
		return fnName + "/error_invalid_" + leaf
	case model.FieldGuardLessThan:
		if leaf == "age" {
			return fnName + "/error_underage"
		}
		return fnName + "/error_less_than_" + leaf
	case model.FieldGuardNil:
		return fnName + "/error_nil_" + leaf
	default:
		return fnName + "/error_" + string(guard.Kind) + "_" + leaf
	}
}

// fieldGuardPatchValue возвращает Go-выражение «плохого» значения для FieldGuard.
func fieldGuardPatchValue(guard model.FieldGuard) string {
	switch guard.Kind {
	case model.FieldGuardEmpty:
		return `""`
	case model.FieldGuardInvalid:
		// Строка, намеренно не содержащая guard.Value (например "@")
		return `"invalid-email"`
	case model.FieldGuardLessThan:
		// threshold - 1
		if t, err := strconv.Atoi(guard.Threshold); err == nil {
			return strconv.Itoa(t - 1)
		}
		return "0"
	case model.FieldGuardNil:
		return "nil"
	default:
		return `""`
	}
}

// ── Edge сценарии (прямые параметры) ─────────────────────────────────────────

// edgeScenarios генерирует edge-сценарии только по Guards-фактам прямых параметров.
// Field-guard проверки (по полям структур) обрабатываются отдельно в fieldGuardScenarios.
func edgeScenarios(fn model.FunctionSpec) []model.ScenarioSpec {
	var out []model.ScenarioSpec

	var errSc *model.ScenarioSpec
	if fn.HasError {
		s := errorScenario(fn)
		errSc = &s
	}

	for i, p := range fn.Params {
		emptyChecked := fn.Guards.EmptyCheckedParams[p.Name]
		nilChecked := fn.Guards.NilCheckedParams[p.Name]

		if nilChecked {
			switch p.Kind {
			case model.KindPtr, model.KindInterface, model.KindFunc,
				model.KindSlice, model.KindMap:
				target := fixture.Zero(p.Kind, p.TypeStr)
				sc := buildEdgeScenario(fn, i, p.Name, "nil", target)
				if !isDuplicate(sc, errSc) && !isDuplicate(sc, lastOf(out)) {
					out = append(out, sc)
				}
			}
		}

		if emptyChecked {
			switch p.Kind {
			case model.KindString, model.KindSlice, model.KindMap:
				target := fixture.Empty(p.Kind, p.TypeStr)
				sc := buildEdgeScenario(fn, i, p.Name, "empty", target)
				if !isDuplicate(sc, errSc) && !isDuplicate(sc, lastOf(out)) {
					out = append(out, sc)
				}
			}
		}
	}

	return out
}

// buildEdgeScenario формирует один edge-сценарий.
func buildEdgeScenario(fn model.FunctionSpec, targetIdx int, paramName, guardKind string, target model.FixtureValue) model.ScenarioSpec {
	inputs := make([]model.FixtureValue, len(fn.Params))
	for j, q := range fn.Params {
		if j == targetIdx {
			inputs[j] = target
		} else {
			inputs[j] = fixture.Happy(q.Kind, q.TypeStr)
		}
	}

	var comment string
	if guardKind == "nil" {
		comment = fmt.Sprintf("TODO: проверь want* — nil %s может быть ошибкой", paramName)
	} else {
		comment = fmt.Sprintf("TODO: проверь want* — пустой %s может быть ошибкой", paramName)
	}

	return model.ScenarioSpec{
		Name:      fmt.Sprintf("%s/edge_%s_%s", fn.Name, guardKind, paramName),
		Kind:      model.ScenarioEdge,
		Comment:   comment,
		Inputs:    inputs,
		Wants:     zeroWants(fn.Results),
		WantError: fn.HasError,
	}
}

// ── Дедупликация ──────────────────────────────────────────────────────────────

func isDuplicate(candidate model.ScenarioSpec, ref *model.ScenarioSpec) bool {
	if ref == nil {
		return false
	}
	if candidate.WantError != ref.WantError {
		return false
	}
	return fixturesEqual(candidate.Inputs, ref.Inputs) &&
		fixturesEqual(candidate.Wants, ref.Wants)
}

func fixturesEqual(a, b []model.FixtureValue) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Expr != b[i].Expr {
			return false
		}
	}
	return true
}

func lastOf(ss []model.ScenarioSpec) *model.ScenarioSpec {
	if len(ss) == 0 {
		return nil
	}
	return &ss[len(ss)-1]
}

// ── Вспомогательные функции ───────────────────────────────────────────────────

// happyInputs строит happy-фикстуру для каждого параметра.
// Для KindStruct с StructFields использует fixture.HappyStructExpr.
func happyInputs(params []model.ParamSpec, packageName string) []model.FixtureValue {
	out := make([]model.FixtureValue, len(params))
	for i, p := range params {
		if p.Kind == model.KindStruct && len(p.StructFields) > 0 {
			// Передаём fn.PackageName чтобы strip убрал квалификатор
			// у вложенных типов из того же пакета:
			// "registration.Address" → "Address" (тест в package registration)
			expr := fixture.HappyStructExpr(p.TypeStr, p.StructFields, packageName)
			out[i] = model.FixtureValue{Expr: expr, TypeStr: p.TypeStr}
		} else {
			out[i] = fixture.Happy(p.Kind, p.TypeStr)
		}
	}
	return out
}

func zeroInputs(params []model.ParamSpec) []model.FixtureValue {
	out := make([]model.FixtureValue, len(params))
	for i, p := range params {
		out[i] = fixture.Zero(p.Kind, p.TypeStr)
	}
	return out
}

func happyWants(results []model.ParamSpec) []model.FixtureValue {
	var out []model.FixtureValue
	for _, r := range results {
		if r.IsError {
			continue
		}
		out = append(out, fixture.Happy(r.Kind, r.TypeStr))
	}
	return out
}

func zeroWants(results []model.ParamSpec) []model.FixtureValue {
	var out []model.FixtureValue
	for _, r := range results {
		if r.IsError {
			continue
		}
		out = append(out, fixture.Zero(r.Kind, r.TypeStr))
	}
	return out
}
