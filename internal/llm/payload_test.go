package llm_test

import (
	"testing"

	"github.com/yourorg/testgen/internal/llm"
	"github.com/yourorg/testgen/internal/model"
)

// registrationFuncSpec строит FunctionSpec для ValidateRegisterRequest,
// имитируя вывод analyzer (с квалифицированным TypeStr для вложенных типов).
func registrationFuncSpec() model.FunctionSpec {
	addrFields := []model.StructField{
		{Name: "City", TypeStr: "string", Kind: model.KindString},
		{Name: "Street", TypeStr: "string", Kind: model.KindString},
		{Name: "House", TypeStr: "string", Kind: model.KindString},
	}
	reqFields := []model.StructField{
		{Name: "Email", TypeStr: "string", Kind: model.KindString},
		{Name: "Name", TypeStr: "string", Kind: model.KindString},
		{Name: "Age", TypeStr: "int", Kind: model.KindInt},
		{Name: "Phone", TypeStr: "string", Kind: model.KindString},
		{Name: "Address", TypeStr: "registration.Address", Kind: model.KindStruct, SubFields: addrFields},
		{Name: "CreatedAt", TypeStr: "time.Time", Kind: model.KindTime},
	}
	return model.FunctionSpec{
		PackageName: "registration",
		Name:        "ValidateRegisterRequest",
		HasError:    true,
		Params: []model.ParamSpec{
			{Name: "req", TypeStr: "RegisterRequest", Kind: model.KindStruct, StructFields: reqFields},
		},
		Results: []model.ParamSpec{
			{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true},
		},
		Guards: model.Guards{
			NilCheckedParams:   map[string]bool{},
			EmptyCheckedParams: map[string]bool{},
			FieldGuards: []model.FieldGuard{
				{ParamName: "req", FieldPath: []string{"Email"}, Kind: model.FieldGuardEmpty},
				{ParamName: "req", FieldPath: []string{"Email"}, Kind: model.FieldGuardInvalid},
				{ParamName: "req", FieldPath: []string{"Name"}, Kind: model.FieldGuardEmpty},
				{ParamName: "req", FieldPath: []string{"Age"}, Kind: model.FieldGuardLessThan, Threshold: "18"},
				{ParamName: "req", FieldPath: []string{"Address", "City"}, Kind: model.FieldGuardEmpty},
			},
		},
	}
}

// registrationScenarios возвращает 6 сценариев для ValidateRegisterRequest.
func registrationScenarios() []model.ScenarioSpec {
	return []model.ScenarioSpec{
		{Name: "ValidateRegisterRequest/success", Kind: model.ScenarioSuccess, WantError: false},
		{Name: "ValidateRegisterRequest/error_empty_email", Kind: model.ScenarioError, WantError: true},
		{Name: "ValidateRegisterRequest/error_invalid_email", Kind: model.ScenarioError, WantError: true},
		{Name: "ValidateRegisterRequest/error_empty_name", Kind: model.ScenarioError, WantError: true},
		{Name: "ValidateRegisterRequest/error_underage", Kind: model.ScenarioError, WantError: true},
		{Name: "ValidateRegisterRequest/error_empty_city", Kind: model.ScenarioError, WantError: true},
	}
}

// ── BuildFixtureRequest ───────────────────────────────────────────────────────

func TestBuildFixtureRequest_functionName(t *testing.T) {
	req := llm.BuildFixtureRequest(registrationFuncSpec(), registrationScenarios())
	if req.Function.Name != "ValidateRegisterRequest" {
		t.Errorf("Function.Name = %q, want ValidateRegisterRequest", req.Function.Name)
	}
	if req.Function.Package != "registration" {
		t.Errorf("Function.Package = %q, want registration", req.Function.Package)
	}
}

func TestBuildFixtureRequest_sixScenarios(t *testing.T) {
	req := llm.BuildFixtureRequest(registrationFuncSpec(), registrationScenarios())
	if len(req.Scenarios) != 6 {
		t.Fatalf("Scenarios count = %d, want 6", len(req.Scenarios))
	}
	wantNames := []string{
		"ValidateRegisterRequest/success",
		"ValidateRegisterRequest/error_empty_email",
		"ValidateRegisterRequest/error_invalid_email",
		"ValidateRegisterRequest/error_empty_name",
		"ValidateRegisterRequest/error_underage",
		"ValidateRegisterRequest/error_empty_city",
	}
	for i, want := range wantNames {
		if req.Scenarios[i].Name != want {
			t.Errorf("Scenarios[%d].Name = %q, want %q", i, req.Scenarios[i].Name, want)
		}
	}
}

func TestBuildFixtureRequest_successScenarioNotWantError(t *testing.T) {
	req := llm.BuildFixtureRequest(registrationFuncSpec(), registrationScenarios())
	if req.Scenarios[0].WantError {
		t.Error("success scenario: WantError должен быть false")
	}
	// success имеет hint с "valid values"
	if req.Scenarios[0].Hint == "" {
		t.Error("success scenario: Hint не должен быть пустым")
	}
}

func TestBuildFixtureRequest_errorScenariosWantError(t *testing.T) {
	req := llm.BuildFixtureRequest(registrationFuncSpec(), registrationScenarios())
	for _, sc := range req.Scenarios[1:] {
		if !sc.WantError {
			t.Errorf("scenario %q: WantError должен быть true", sc.Name)
		}
	}
}

func TestBuildFixtureRequest_fieldGuards(t *testing.T) {
	req := llm.BuildFixtureRequest(registrationFuncSpec(), registrationScenarios())
	guards := req.Function.FieldGuards
	if len(guards) != 5 {
		t.Fatalf("FieldGuards count = %d, want 5; guards=%+v", len(guards), guards)
	}

	// Проверяем конкретные guards
	checkGuard := func(idx int, param string, path []string, kind string) {
		t.Helper()
		g := guards[idx]
		if g.Param != param {
			t.Errorf("guards[%d].Param = %q, want %q", idx, g.Param, param)
		}
		if len(g.FieldPath) != len(path) {
			t.Errorf("guards[%d].FieldPath = %v, want %v", idx, g.FieldPath, path)
			return
		}
		for i, p := range path {
			if g.FieldPath[i] != p {
				t.Errorf("guards[%d].FieldPath[%d] = %q, want %q", idx, i, g.FieldPath[i], p)
			}
		}
		if g.Kind != kind {
			t.Errorf("guards[%d].Kind = %q, want %q", idx, g.Kind, kind)
		}
	}

	checkGuard(0, "req", []string{"Email"}, "empty")
	checkGuard(1, "req", []string{"Email"}, "invalid")
	checkGuard(2, "req", []string{"Name"}, "empty")
	checkGuard(3, "req", []string{"Age"}, "less_than")
	checkGuard(4, "req", []string{"Address", "City"}, "empty")

	// Threshold для less_than
	if guards[3].Threshold != "18" {
		t.Errorf("guards[3].Threshold = %q, want 18", guards[3].Threshold)
	}
}

func TestBuildFixtureRequest_paramWithStructFields(t *testing.T) {
	req := llm.BuildFixtureRequest(registrationFuncSpec(), registrationScenarios())
	if len(req.Function.Params) != 1 {
		t.Fatalf("Params count = %d, want 1", len(req.Function.Params))
	}
	param := req.Function.Params[0]
	if param.Name != "req" {
		t.Errorf("param.Name = %q, want req", param.Name)
	}
	if param.Kind != "struct" {
		t.Errorf("param.Kind = %q, want struct", param.Kind)
	}
	if len(param.StructFields) == 0 {
		t.Fatal("param.StructFields пустой — поля RegisterRequest не переданы в payload")
	}
}

func TestBuildFixtureRequest_nestedAddressFields(t *testing.T) {
	req := llm.BuildFixtureRequest(registrationFuncSpec(), registrationScenarios())
	param := req.Function.Params[0]

	// Ищем поле Address среди полей RegisterRequest
	var addrField *llm.StructFieldPayload
	for i := range param.StructFields {
		if param.StructFields[i].Name == "Address" {
			addrField = &param.StructFields[i]
			break
		}
	}
	if addrField == nil {
		t.Fatal("поле Address не найдено в payload.StructFields")
	}
	if addrField.Kind != "struct" {
		t.Errorf("Address.Kind = %q, want struct", addrField.Kind)
	}
	if len(addrField.Fields) == 0 {
		t.Fatal("Address.Fields пустой — вложенные поля City/Street/House не переданы")
	}

	// Проверяем наличие City
	cityFound := false
	for _, f := range addrField.Fields {
		if f.Name == "City" {
			cityFound = true
			break
		}
	}
	if !cityFound {
		t.Error("City не найден в Address.Fields")
	}
}

func TestBuildFixtureRequest_topLevelFields(t *testing.T) {
	req := llm.BuildFixtureRequest(registrationFuncSpec(), registrationScenarios())
	param := req.Function.Params[0]

	byName := make(map[string]llm.StructFieldPayload)
	for _, f := range param.StructFields {
		byName[f.Name] = f
	}

	for _, want := range []string{"Email", "Name", "Age", "Phone", "Address", "CreatedAt"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("поле %q отсутствует в payload.StructFields", want)
		}
	}

	// Проверяем kinds
	if byName["Email"].Kind != "string" {
		t.Errorf("Email.Kind = %q, want string", byName["Email"].Kind)
	}
	if byName["Age"].Kind != "int" {
		t.Errorf("Age.Kind = %q, want int", byName["Age"].Kind)
	}
	if byName["CreatedAt"].Kind != "time" {
		t.Errorf("CreatedAt.Kind = %q, want time", byName["CreatedAt"].Kind)
	}
}

func TestBuildFixtureRequest_instructionsNotEmpty(t *testing.T) {
	req := llm.BuildFixtureRequest(registrationFuncSpec(), registrationScenarios())
	if req.Instructions == "" {
		t.Error("Instructions не должны быть пустыми")
	}
}
