// Пакет scenario строит []ScenarioSpec из FunctionSpec.
//
// Правила генерации:
//
//	success — генерируется всегда: happy-фикстуры для всех параметров.
//
//	error — генерируется если функция возвращает error:
//	        нулевые фикстуры, wantErr = true.
//
//	edge — генерируется только на основе реальных Guards-фактов из тела функции.
//	       Для каждого параметра возможны два независимых edge:
//	         edge_nil_<param>   — nil-guard: fixture.Zero (nil для ptr/slice/map/interface)
//	         edge_empty_<param> — empty-guard: fixture.Empty ([]T{} / "" / map[K]V{})
//	       Ключевое отличие для []string с единственным параметром:
//	         error:       inputs=[nil]        (Zero)
//	         edge_nil:    inputs=[nil]        (Zero) → дублирует error → пропускается
//	         edge_empty:  inputs=[[]string{}] (Empty) → уникален → сохраняется
//
// Дедупликация честная: сравниваются Inputs.Expr, Wants.Expr, WantError.
// Только при полном совпадении содержимого кандидат пропускается.
package scenario

import (
	"fmt"

	"github.com/yourorg/testgen/internal/fixture"
	"github.com/yourorg/testgen/internal/model"
)

// Generate возвращает все ScenarioSpec для fn.
func Generate(fn model.FunctionSpec) []model.ScenarioSpec {
	var out []model.ScenarioSpec

	out = append(out, successScenario(fn))

	if fn.HasError {
		out = append(out, errorScenario(fn))
	}

	out = append(out, edgeScenarios(fn)...)

	return out
}

// successScenario: каждый параметр получает happy-фикстуру, ошибки не ожидается.
func successScenario(fn model.FunctionSpec) model.ScenarioSpec {
	return model.ScenarioSpec{
		Name:      fn.Name + "/success",
		Kind:      model.ScenarioSuccess,
		Comment:   "TODO: установи want* в ожидаемые возвращаемые значения",
		Inputs:    happyInputs(fn.Params),
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

// edgeScenarios генерирует edge-сценарии только по Guards-фактам.
//
// Для каждого параметра возможны два независимых edge:
//   - edge_nil_<param>   — nil-guard: fixture.Zero (nil)
//   - edge_empty_<param> — empty-guard: fixture.Empty ([]T{} / "" / map[K]V{})
//
// Каждый кандидат проверяется через isDuplicate против error-сценария
// и предыдущего добавленного edge. При совпадении содержимого — пропускается.
func edgeScenarios(fn model.FunctionSpec) []model.ScenarioSpec {
	var out []model.ScenarioSpec

	// Предвычисляем error-сценарий один раз для сравнения при дедупликации.
	var errSc *model.ScenarioSpec
	if fn.HasError {
		s := errorScenario(fn)
		errSc = &s
	}

	for i, p := range fn.Params {
		emptyChecked := fn.Guards.EmptyCheckedParams[p.Name]
		nilChecked := fn.Guards.NilCheckedParams[p.Name]

		// edge_nil: nil-guard → fixture.Zero (nil) для целевого параметра.
		// Имеет смысл только для типов, которые Go допускает как nil.
		if nilChecked {
			switch p.Kind {
			case model.KindPtr, model.KindInterface, model.KindFunc,
				model.KindSlice, model.KindMap:
				target := fixture.Zero(p.Kind, p.TypeStr) // nil
				sc := buildEdgeScenario(fn, i, p.Name, "nil", target)
				if !isDuplicate(sc, errSc) && !isDuplicate(sc, lastOf(out)) {
					out = append(out, sc)
				}
			}
		}

		// edge_empty: empty-guard → fixture.Empty для целевого параметра.
		// Для среза/map это []T{} / map[K]V{} — отличается от nil в error-сценарии.
		if emptyChecked {
			switch p.Kind {
			case model.KindString, model.KindSlice, model.KindMap:
				target := fixture.Empty(p.Kind, p.TypeStr) // "" / []T{} / map[K]V{}
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
//   - targetIdx — индекс параметра, получающего edge-фикстуру
//   - paramName — имя параметра (для имени сценария и комментария)
//   - guardKind — "nil" или "empty"
//   - target    — фикстура для целевого параметра (Zero или Empty)
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

// isDuplicate проверяет, совпадает ли candidate по содержимому с ref.
// Сравниваются Inputs.Expr, Wants.Expr, WantError — именно то,
// что определяет строку таблицы в сгенерированном тесте.
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

// fixturesEqual сравнивает два среза фикстур по полю Expr.
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

// lastOf возвращает указатель на последний элемент среза, или nil если пуст.
// Нужен чтобы не добавлять два идентичных edge для одного параметра подряд.
func lastOf(ss []model.ScenarioSpec) *model.ScenarioSpec {
	if len(ss) == 0 {
		return nil
	}
	return &ss[len(ss)-1]
}

// ── Вспомогательные функции ───────────────────────────────────────────────────

func happyInputs(params []model.ParamSpec) []model.FixtureValue {
	out := make([]model.FixtureValue, len(params))
	for i, p := range params {
		out[i] = fixture.Happy(p.Kind, p.TypeStr)
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

// happyWants — happy-фикстуры для каждого не-error результата.
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

// zeroWants — нулевые фикстуры для каждого не-error результата.
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
