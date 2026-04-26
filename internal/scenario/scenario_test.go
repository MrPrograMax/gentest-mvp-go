package scenario_test

import (
	"testing"

	"github.com/yourorg/testgen/internal/model"
	"github.com/yourorg/testgen/internal/scenario"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// makeFunc создаёт FunctionSpec с пустыми Guards (нет Guards-фактов).
func makeFunc(name string, params []model.ParamSpec, results []model.ParamSpec, hasError bool) model.FunctionSpec {
	return model.FunctionSpec{
		PackageName: "pkg",
		Name:        name,
		Params:      params,
		Results:     results,
		HasError:    hasError,
		Guards: model.Guards{
			NilCheckedParams:   make(map[string]bool),
			EmptyCheckedParams: make(map[string]bool),
		},
	}
}

// makeFuncWithGuards создаёт FunctionSpec с заданными Guards.
func makeFuncWithGuards(name string, params []model.ParamSpec, results []model.ParamSpec, hasError bool, g model.Guards) model.FunctionSpec {
	fn := makeFunc(name, params, results, hasError)
	fn.Guards = g
	return fn
}

// countKind считает сценарии заданного вида.
func countKind(sc []model.ScenarioSpec, kind model.ScenarioKind) int {
	n := 0
	for _, s := range sc {
		if s.Kind == kind {
			n++
		}
	}
	return n
}

// ── Базовые сценарии ──────────────────────────────────────────────────────────

func TestGenerate_successAlways(t *testing.T) {
	fn := makeFunc("Add",
		[]model.ParamSpec{{Name: "a", TypeStr: "int", Kind: model.KindInt}},
		[]model.ParamSpec{{Name: "result0", TypeStr: "int", Kind: model.KindInt}},
		false,
	)
	sc := scenario.Generate(fn)
	if len(sc) == 0 {
		t.Fatal("ожидался хотя бы один сценарий")
	}
	if sc[0].Kind != model.ScenarioSuccess {
		t.Errorf("первый сценарий должен быть success, got %q", sc[0].Kind)
	}
}

func TestGenerate_errorWhenHasError(t *testing.T) {
	fn := makeFunc("Divide",
		[]model.ParamSpec{
			{Name: "a", TypeStr: "float64", Kind: model.KindInt},
			{Name: "b", TypeStr: "float64", Kind: model.KindInt},
		},
		[]model.ParamSpec{
			{Name: "result0", TypeStr: "float64", Kind: model.KindInt},
			{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true},
		},
		true,
	)
	sc := scenario.Generate(fn)
	if countKind(sc, model.ScenarioError) == 0 {
		t.Error("ожидался error-сценарий при HasError=true")
	}
}

// ── Edge: только по Guards ────────────────────────────────────────────────────

func TestGenerate_edgeOnlyWithGuards(t *testing.T) {
	// Без Guards — edge не генерируется даже для string-параметра.
	fn := makeFunc("NoGuard",
		[]model.ParamSpec{{Name: "s", TypeStr: "string", Kind: model.KindString}},
		nil, false,
	)
	sc := scenario.Generate(fn)
	if countKind(sc, model.ScenarioEdge) != 0 {
		t.Error("edge без Guards не должен генерироваться")
	}
}

// ── Нейминг edge ──────────────────────────────────────────────────────────────

func TestGenerate_edgeName_nil(t *testing.T) {
	// nil-guard → имя edge_nil_<param>.
	fn := makeFuncWithGuards("Process",
		[]model.ParamSpec{
			{Name: "p", TypeStr: "*int", Kind: model.KindPtr},
			{Name: "n", TypeStr: "int", Kind: model.KindInt}, // второй параметр — нет дедупликации с error
		},
		[]model.ParamSpec{{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true}},
		true,
		model.Guards{
			NilCheckedParams:   map[string]bool{"p": true},
			EmptyCheckedParams: make(map[string]bool),
		},
	)
	sc := scenario.Generate(fn)
	for _, s := range sc {
		if s.Kind == model.ScenarioEdge {
			want := "Process/edge_nil_p"
			if s.Name != want {
				t.Errorf("edge с nil-guard: Name = %q, want %q", s.Name, want)
			}
		}
	}
	if countKind(sc, model.ScenarioEdge) == 0 {
		t.Error("ожидался хотя бы один edge_nil сценарий")
	}
}

func TestGenerate_edgeName_empty(t *testing.T) {
	// empty-guard → имя edge_empty_<param>.
	fn := makeFuncWithGuards("Greet",
		[]model.ParamSpec{
			{Name: "name", TypeStr: "string", Kind: model.KindString},
			{Name: "lang", TypeStr: "string", Kind: model.KindString},
		},
		[]model.ParamSpec{{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true}},
		true,
		model.Guards{
			NilCheckedParams:   make(map[string]bool),
			EmptyCheckedParams: map[string]bool{"name": true},
		},
	)
	sc := scenario.Generate(fn)
	for _, s := range sc {
		if s.Kind == model.ScenarioEdge {
			want := "Greet/edge_empty_name"
			if s.Name != want {
				t.Errorf("edge с empty-guard: Name = %q, want %q", s.Name, want)
			}
		}
	}
}

// ── Дедупликация: string (single-param) ──────────────────────────────────────

func TestGenerate_edgeString_singleParam_deduplicated(t *testing.T) {
	// Single-param string + HasError:
	// error:      inputs=[""]  (Zero)
	// edge_empty: inputs=[""]  (Empty) — для string Empty="" == Zero="" → дублирует → пропускается.
	fn := makeFuncWithGuards("Greet",
		[]model.ParamSpec{
			{Name: "name", TypeStr: "string", Kind: model.KindString},
		},
		[]model.ParamSpec{
			{Name: "result0", TypeStr: "string", Kind: model.KindString},
			{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true},
		},
		true,
		model.Guards{
			NilCheckedParams:   make(map[string]bool),
			EmptyCheckedParams: map[string]bool{"name": true},
		},
	)
	sc := scenario.Generate(fn)
	// edge для "" == "" дублирует error → пропускается.
	if countKind(sc, model.ScenarioEdge) != 0 {
		t.Error("edge_empty для string совпадает с error-сценарием, должен быть дедуплицирован")
	}
}

// ── Дедупликация: ptr (single-param) ─────────────────────────────────────────

func TestGenerate_edgePtr_singleParam_deduplicated(t *testing.T) {
	// Single-param *int + HasError:
	// error:    inputs=[nil] (Zero)
	// edge_nil: inputs=[nil] (Zero) → дублирует → пропускается.
	fn := makeFuncWithGuards("Process",
		[]model.ParamSpec{
			{Name: "p", TypeStr: "*int", Kind: model.KindPtr},
		},
		[]model.ParamSpec{{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true}},
		true,
		model.Guards{
			NilCheckedParams:   map[string]bool{"p": true},
			EmptyCheckedParams: make(map[string]bool),
		},
	)
	sc := scenario.Generate(fn)
	if countKind(sc, model.ScenarioEdge) != 0 {
		t.Error("edge_nil для *T совпадает с error-сценарием, должен быть дедуплицирован")
	}
}

// ── Ключевой кейс: slice (single-param) ──────────────────────────────────────

func TestGenerate_edgeSlice_singleParam_emptyKept(t *testing.T) {
	// Single-param []string + HasError + emptyChecked:
	// error:       inputs=[nil]        (Zero) — wantErr=true
	// edge_empty:  inputs=[[]string{}] (Empty) — wantErr=true → inputs РАЗНЫЕ → НЕ дублирует!
	fn := makeFuncWithGuards("Process",
		[]model.ParamSpec{
			{Name: "items", TypeStr: "[]string", Kind: model.KindSlice},
		},
		[]model.ParamSpec{{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true}},
		true,
		model.Guards{
			NilCheckedParams:   make(map[string]bool),
			EmptyCheckedParams: map[string]bool{"items": true},
		},
	)
	sc := scenario.Generate(fn)
	if countKind(sc, model.ScenarioEdge) != 1 {
		t.Errorf("edge_empty([]string) отличается от error(nil) — должен быть сохранён, got %d edge", countKind(sc, model.ScenarioEdge))
	}
	// Проверяем, что edge использует []string{}, а не nil.
	for _, s := range sc {
		if s.Kind == model.ScenarioEdge {
			if len(s.Inputs) == 0 {
				t.Fatal("нет inputs в edge-сценарии")
			}
			if s.Inputs[0].Expr != "[]string{}" {
				t.Errorf("edge_empty input = %q, want []string{}", s.Inputs[0].Expr)
			}
			if s.Name != "Process/edge_empty_items" {
				t.Errorf("edge Name = %q, want Process/edge_empty_items", s.Name)
			}
		}
	}
}

func TestGenerate_edgeSlice_singleParam_nilSkipped(t *testing.T) {
	// Single-param []string + HasError + nilChecked:
	// error:    inputs=[nil] (Zero)
	// edge_nil: inputs=[nil] (Zero) → дублирует → пропускается.
	fn := makeFuncWithGuards("Process",
		[]model.ParamSpec{
			{Name: "items", TypeStr: "[]string", Kind: model.KindSlice},
		},
		[]model.ParamSpec{{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true}},
		true,
		model.Guards{
			NilCheckedParams:   map[string]bool{"items": true},
			EmptyCheckedParams: make(map[string]bool),
		},
	)
	sc := scenario.Generate(fn)
	if countKind(sc, model.ScenarioEdge) != 0 {
		t.Error("edge_nil([]string) совпадает с error(nil) — должен быть дедуплицирован")
	}
}

func TestGenerate_edgeSlice_singleParam_both(t *testing.T) {
	// Single-param []string + HasError + nilChecked + emptyChecked:
	// edge_nil   → nil → дублирует error → пропускается
	// edge_empty → []string{} → уникален → сохраняется
	// Итого: 1 edge.
	fn := makeFuncWithGuards("Process",
		[]model.ParamSpec{
			{Name: "items", TypeStr: "[]string", Kind: model.KindSlice},
		},
		[]model.ParamSpec{{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true}},
		true,
		model.Guards{
			NilCheckedParams:   map[string]bool{"items": true},
			EmptyCheckedParams: map[string]bool{"items": true},
		},
	)
	sc := scenario.Generate(fn)
	if countKind(sc, model.ScenarioEdge) != 1 {
		t.Errorf("ожидался 1 edge (empty), got %d", countKind(sc, model.ScenarioEdge))
	}
}

// ── Несколько параметров ──────────────────────────────────────────────────────

func TestGenerate_edgeMultipleParams(t *testing.T) {
	// Два string-параметра с emptyChecked: 2 edge_empty (inputs разные → нет дедупликации).
	fn := makeFuncWithGuards("Greet",
		[]model.ParamSpec{
			{Name: "name", TypeStr: "string", Kind: model.KindString},
			{Name: "lang", TypeStr: "string", Kind: model.KindString},
		},
		[]model.ParamSpec{{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true}},
		true,
		model.Guards{
			NilCheckedParams:   make(map[string]bool),
			EmptyCheckedParams: map[string]bool{"name": true, "lang": true},
		},
	)
	sc := scenario.Generate(fn)
	if countKind(sc, model.ScenarioEdge) != 2 {
		t.Errorf("ожидалось 2 edge-сценария, got %d", countKind(sc, model.ScenarioEdge))
	}
}

// ── Happy-фикстура для среза ──────────────────────────────────────────────────

func TestGenerate_sliceHappyNonEmpty(t *testing.T) {
	// Happy-фикстура для []string должна быть непустой.
	fn := makeFunc("Sum",
		[]model.ParamSpec{{Name: "items", TypeStr: "[]string", Kind: model.KindSlice}},
		nil, false,
	)
	sc := scenario.Generate(fn)
	if len(sc) == 0 {
		t.Fatal("ожидался хотя бы один сценарий")
	}
	expr := sc[0].Inputs[0].Expr
	if expr == "nil" || expr == "[]string{}" {
		t.Errorf("success-фикстура для []string = %q, ожидался непустой срез", expr)
	}
}
