package llm_test

import (
	"strings"
	"testing"

	"github.com/yourorg/testgen/internal/llm"
	"github.com/yourorg/testgen/internal/model"
)

// ── Builders ──────────────────────────────────────────────────────────────────

// applyTestFunctionSpec строит FunctionSpec, эквивалентный тому, что
// analyzer выдаёт для example/registration ValidateRegisterRequest.
//
// Поля сделаны минимальными — нам важны только Name, PackageName, Params
// и StructFields для корректной работы literal-конверсии.
//
// Имя с префиксом applyTest, чтобы не конфликтовать с registrationFuncSpec
// из payload_test.go (тот же пакет llm_test).
func applyTestFunctionSpec() model.FunctionSpec {
	addrFields := []model.StructField{
		{Name: "City", TypeStr: "string", Kind: model.KindString},
		{Name: "Street", TypeStr: "string", Kind: model.KindString},
		{Name: "House", TypeStr: "string", Kind: model.KindString},
	}
	return model.FunctionSpec{
		PackageName: "registration",
		Name:        "ValidateRegisterRequest",
		HasError:    true,
		Params: []model.ParamSpec{
			{
				Name:    "req",
				TypeStr: "RegisterRequest",
				Kind:    model.KindStruct,
				StructFields: []model.StructField{
					{Name: "Email", TypeStr: "string", Kind: model.KindString},
					{Name: "Name", TypeStr: "string", Kind: model.KindString},
					{Name: "Age", TypeStr: "int", Kind: model.KindInt},
					{Name: "Phone", TypeStr: "string", Kind: model.KindString},
					{
						Name:      "Address",
						TypeStr:   "Address",
						Kind:      model.KindStruct,
						SubFields: addrFields,
					},
					{Name: "CreatedAt", TypeStr: "time.Time", Kind: model.KindTime},
				},
			},
		},
		Results: []model.ParamSpec{
			{Name: "_", TypeStr: "error", Kind: model.KindError, IsError: true},
		},
	}
}

// applyTestScenarios строит 6 сценариев, как scenario.Generate
// производит для ValidateRegisterRequest. Inputs[0].Expr содержит
// маркер PLACEHOLDER, чтобы тесты могли проверить, что Apply его заменил.
//
// Шейп отличается от registrationScenarios из payload_test.go:
// здесь нужны Inputs/Wants/Comment для проверки сохранения полей.
func applyTestScenarios() []model.ScenarioSpec {
	names := []string{
		"ValidateRegisterRequest/success",
		"ValidateRegisterRequest/error_empty_email",
		"ValidateRegisterRequest/error_invalid_email",
		"ValidateRegisterRequest/error_empty_name",
		"ValidateRegisterRequest/error_underage",
		"ValidateRegisterRequest/error_empty_city",
	}
	out := make([]model.ScenarioSpec, len(names))
	for i, n := range names {
		kind := model.ScenarioError
		wantErr := true
		if i == 0 {
			kind = model.ScenarioSuccess
			wantErr = false
		}
		out[i] = model.ScenarioSpec{
			Name:    n,
			Kind:    kind,
			Comment: "preserved-comment",
			Inputs: []model.FixtureValue{
				{Expr: "PLACEHOLDER", TypeStr: "RegisterRequest"},
			},
			Wants:     []model.FixtureValue{{Expr: "PRESERVED-WANT", TypeStr: "string"}},
			WantError: wantErr,
		}
	}
	return out
}

// applyTestResponseJSON возвращает валидный 6-сценариевый JSON-ответ,
// соответствующий applyTestFunctionSpec и applyTestScenarios.
func applyTestResponseJSON() string {
	return `{
  "scenarios": {
    "ValidateRegisterRequest/success": {
      "req": {
        "Email": "user@example.com",
        "Name": "Test User",
        "Age": 25,
        "Phone": "+79991234567",
        "Address": {"City": "Moscow", "Street": "Tverskaya", "House": "1"},
        "CreatedAt": "2026-01-01T00:00:00Z"
      }
    },
    "ValidateRegisterRequest/error_empty_email": {
      "req": {
        "Email": "",
        "Name": "Test User",
        "Age": 25,
        "Phone": "+79991234567",
        "Address": {"City": "Moscow", "Street": "Tverskaya", "House": "1"},
        "CreatedAt": "2026-01-01T00:00:00Z"
      }
    },
    "ValidateRegisterRequest/error_invalid_email": {
      "req": {
        "Email": "invalid-email",
        "Name": "Test User",
        "Age": 25,
        "Phone": "+79991234567",
        "Address": {"City": "Moscow", "Street": "Tverskaya", "House": "1"},
        "CreatedAt": "2026-01-01T00:00:00Z"
      }
    },
    "ValidateRegisterRequest/error_empty_name": {
      "req": {
        "Email": "user@example.com",
        "Name": "",
        "Age": 25,
        "Phone": "+79991234567",
        "Address": {"City": "Moscow", "Street": "Tverskaya", "House": "1"},
        "CreatedAt": "2026-01-01T00:00:00Z"
      }
    },
    "ValidateRegisterRequest/error_underage": {
      "req": {
        "Email": "user@example.com",
        "Name": "Test User",
        "Age": 17,
        "Phone": "+79991234567",
        "Address": {"City": "Moscow", "Street": "Tverskaya", "House": "1"},
        "CreatedAt": "2026-01-01T00:00:00Z"
      }
    },
    "ValidateRegisterRequest/error_empty_city": {
      "req": {
        "Email": "user@example.com",
        "Name": "Test User",
        "Age": 25,
        "Phone": "+79991234567",
        "Address": {"City": "", "Street": "Tverskaya", "House": "1"},
        "CreatedAt": "2026-01-01T00:00:00Z"
      }
    }
  }
}`
}

// ── Atomic helper tests ───────────────────────────────────────────────────────

func TestJSONStringToGoLiteral(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello", `"hello"`},
		{"", `""`},
		{`a"b`, `"a\"b"`},
		{"line\nbreak", `"line\nbreak"`},
		{"user@example.com", `"user@example.com"`},
	}
	for _, c := range cases {
		if got := llm.JSONStringToGoLiteral(c.in); got != c.want {
			t.Errorf("JSONStringToGoLiteral(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestJSONNumberToIntLiteral_integer(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{25, "25"},
		{0, "0"},
		{-3, "-3"},
		{1000000, "1000000"},
	}
	for _, c := range cases {
		got, err := llm.JSONNumberToIntLiteral(c.in)
		if err != nil {
			t.Errorf("JSONNumberToIntLiteral(%v) returned error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("JSONNumberToIntLiteral(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestJSONNumberToIntLiteral_fractionalRejected(t *testing.T) {
	if _, err := llm.JSONNumberToIntLiteral(2.5); err == nil {
		t.Errorf("expected error for fractional value 2.5")
	}
}

func TestJSONBoolToGoLiteral(t *testing.T) {
	if got := llm.JSONBoolToGoLiteral(true); got != "true" {
		t.Errorf("true → %q, want \"true\"", got)
	}
	if got := llm.JSONBoolToGoLiteral(false); got != "false" {
		t.Errorf("false → %q, want \"false\"", got)
	}
}

func TestTimeStringToGoLiteral_RFC3339(t *testing.T) {
	got, err := llm.TimeStringToGoLiteral("2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestTimeStringToGoLiteral_RFC3339Nano(t *testing.T) {
	got, err := llm.TimeStringToGoLiteral("2026-06-15T10:30:45.123456789Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "time.Date(2026, time.June, 15, 10, 30, 45, 123456789, time.UTC)"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestTimeStringToGoLiteral_offsetConvertedToUTC(t *testing.T) {
	// 2026-01-01T03:00:00+03:00 — это тот же instant что 2026-01-01T00:00:00Z.
	got, err := llm.TimeStringToGoLiteral("2026-01-01T03:00:00+03:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestTimeStringToGoLiteral_invalid(t *testing.T) {
	_, err := llm.TimeStringToGoLiteral("not a date")
	if err == nil {
		t.Fatal("expected error for non-RFC3339 string")
	}
	if !strings.Contains(err.Error(), "RFC3339") {
		t.Errorf("expected error to mention 'RFC3339', got: %v", err)
	}
}

// ── ApplyFixtureResponseToScenarios — happy-path ──────────────────────────────

func TestApply_replacesAllSixRegistrationScenarios(t *testing.T) {
	resp, err := llm.ParseFixtureResponse(applyTestResponseJSON())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fn := applyTestFunctionSpec()
	scenarios := applyTestScenarios()

	out, err := llm.ApplyFixtureResponseToScenarios(resp, fn, scenarios, "registration")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(out) != 6 {
		t.Fatalf("expected 6 scenarios, got %d", len(out))
	}

	// Имена сценариев сохраняются.
	for i := range scenarios {
		if out[i].Name != scenarios[i].Name {
			t.Errorf("scenario[%d].Name = %q, want %q",
				i, out[i].Name, scenarios[i].Name)
		}
	}

	// wantErr сохраняется.
	for i := range scenarios {
		if out[i].WantError != scenarios[i].WantError {
			t.Errorf("scenario[%d].WantError = %v, want %v",
				i, out[i].WantError, scenarios[i].WantError)
		}
	}

	// Inputs действительно заменены — PLACEHOLDER исчезает.
	for i := range out {
		if len(out[i].Inputs) == 0 {
			t.Fatalf("scenario[%d] Inputs пустой", i)
		}
		if strings.Contains(out[i].Inputs[0].Expr, "PLACEHOLDER") {
			t.Errorf("scenario[%d] Expr всё ещё содержит PLACEHOLDER:\n%s",
				i, out[i].Inputs[0].Expr)
		}
	}

	// Comment, Kind и Wants сохраняются.
	for i := range out {
		if out[i].Comment != "preserved-comment" {
			t.Errorf("scenario[%d].Comment = %q, want preserved-comment",
				i, out[i].Comment)
		}
		if out[i].Kind != scenarios[i].Kind {
			t.Errorf("scenario[%d].Kind = %v, want %v",
				i, out[i].Kind, scenarios[i].Kind)
		}
		if len(out[i].Wants) != 1 || out[i].Wants[0].Expr != "PRESERVED-WANT" {
			t.Errorf("scenario[%d].Wants не сохранён: %+v", i, out[i].Wants)
		}
	}
}

func TestApply_doesNotMutateInputScenarios(t *testing.T) {
	resp, _ := llm.ParseFixtureResponse(applyTestResponseJSON())
	fn := applyTestFunctionSpec()
	scenarios := applyTestScenarios()

	_, err := llm.ApplyFixtureResponseToScenarios(resp, fn, scenarios, "registration")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Исходный slice остался с PLACEHOLDER.
	if scenarios[0].Inputs[0].Expr != "PLACEHOLDER" {
		t.Errorf("исходный scenarios[0] был мутирован: %s",
			scenarios[0].Inputs[0].Expr)
	}
}

func TestApply_buildsExpectedRegistrationLiteral(t *testing.T) {
	resp, _ := llm.ParseFixtureResponse(applyTestResponseJSON())
	fn := applyTestFunctionSpec()
	scenarios := applyTestScenarios()[:1] // только success

	out, err := llm.ApplyFixtureResponseToScenarios(resp, fn, scenarios, "registration")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	expr := out[0].Inputs[0].Expr

	// Проверяем по фрагментам — go/format нормализовал бы пробелы,
	// но мы здесь не проходим через format.Source, поэтому
	// substring-проверки достаточно для доказательства смыслового совпадения.
	wantContains := []string{
		`RegisterRequest{`,
		`Email: "user@example.com"`,
		`Name: "Test User"`,
		`Age: 25`,
		`Phone: "+79991234567"`,
		`Address: Address{`,
		`City: "Moscow"`,
		`Street: "Tverskaya"`,
		`House: "1"`,
		`CreatedAt: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)`,
	}
	for _, s := range wantContains {
		if !strings.Contains(expr, s) {
			t.Errorf("literal не содержит %q\nfull literal: %s", s, expr)
		}
	}
}

// ── Package-qualifier behaviour ───────────────────────────────────────────────

func TestApply_samePackageStripsPrefix(t *testing.T) {
	resp, _ := llm.ParseFixtureResponse(applyTestResponseJSON())
	fn := applyTestFunctionSpec()
	// Эмулируем ситуацию когда analyzer положил квалификатор в TypeStr
	// (это бывает в TypeStrFull для external-test, но здесь намеренно
	// форсим квалификатор и в TypeStr, чтобы проверить strip).
	fn.Params[0].TypeStr = "registration.RegisterRequest"
	fn.Params[0].StructFields[4].TypeStr = "registration.Address"

	scenarios := applyTestScenarios()[:1]
	out, err := llm.ApplyFixtureResponseToScenarios(resp, fn, scenarios, "registration")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	expr := out[0].Inputs[0].Expr

	if strings.Contains(expr, "registration.Address") {
		t.Errorf("same-package literal не должен содержать registration.Address;\nfull: %s", expr)
	}
	if strings.Contains(expr, "registration.RegisterRequest") {
		t.Errorf("same-package literal не должен содержать registration.RegisterRequest;\nfull: %s", expr)
	}
	if !strings.Contains(expr, "Address{") {
		t.Errorf("ожидался Address{ во вложенной структуре;\nfull: %s", expr)
	}
	if !strings.Contains(expr, "RegisterRequest{") {
		t.Errorf("ожидался RegisterRequest{ верхнего уровня;\nfull: %s", expr)
	}
}

func TestApply_externalPackageKeepsPrefix(t *testing.T) {
	resp, _ := llm.ParseFixtureResponse(applyTestResponseJSON())
	fn := applyTestFunctionSpec()
	// External package: типы квалифицированные.
	fn.Params[0].TypeStr = "registration.RegisterRequest"
	fn.Params[0].StructFields[4].TypeStr = "registration.Address"

	scenarios := applyTestScenarios()[:1]
	// stripPkgs не передаётся — квалификаторы должны остаться.
	out, err := llm.ApplyFixtureResponseToScenarios(resp, fn, scenarios)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	expr := out[0].Inputs[0].Expr

	if !strings.Contains(expr, "registration.RegisterRequest{") {
		t.Errorf("ожидался registration.RegisterRequest{;\nfull: %s", expr)
	}
	if !strings.Contains(expr, "registration.Address{") {
		t.Errorf("ожидался registration.Address{;\nfull: %s", expr)
	}
}

// ── Отдельные case-проверки конверсий внутри struct ───────────────────────────

func TestApply_emptyStringFieldProducesEmptyQuotedLiteral(t *testing.T) {
	resp, _ := llm.ParseFixtureResponse(applyTestResponseJSON())
	fn := applyTestFunctionSpec()
	// error_empty_email содержит Email: ""
	scenarios := []model.ScenarioSpec{{
		Name:      "ValidateRegisterRequest/error_empty_email",
		Kind:      model.ScenarioError,
		Inputs:    []model.FixtureValue{{Expr: "PLACEHOLDER", TypeStr: "RegisterRequest"}},
		WantError: true,
	}}
	out, err := llm.ApplyFixtureResponseToScenarios(resp, fn, scenarios, "registration")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	expr := out[0].Inputs[0].Expr
	if !strings.Contains(expr, `Email: ""`) {
		t.Errorf("ожидался Email: \"\"; got: %s", expr)
	}
}

func TestApply_underageAgeProducesIntLiteral(t *testing.T) {
	resp, _ := llm.ParseFixtureResponse(applyTestResponseJSON())
	fn := applyTestFunctionSpec()
	scenarios := []model.ScenarioSpec{{
		Name:      "ValidateRegisterRequest/error_underage",
		Kind:      model.ScenarioError,
		Inputs:    []model.FixtureValue{{Expr: "PLACEHOLDER", TypeStr: "RegisterRequest"}},
		WantError: true,
	}}
	out, err := llm.ApplyFixtureResponseToScenarios(resp, fn, scenarios, "registration")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	expr := out[0].Inputs[0].Expr
	if !strings.Contains(expr, `Age: 17`) {
		t.Errorf("ожидался Age: 17 без кавычек; got: %s", expr)
	}
}

// ── Error-path тесты ──────────────────────────────────────────────────────────

func TestApply_missingScenario(t *testing.T) {
	raw := `{"scenarios": {
		"ValidateRegisterRequest/success": {
			"req": {
				"Email": "x@y.com", "Name": "A", "Age": 25, "Phone": "1",
				"Address": {"City": "M", "Street": "S", "House": "1"},
				"CreatedAt": "2026-01-01T00:00:00Z"
			}
		}
	}}`
	resp, _ := llm.ParseFixtureResponse(raw)
	fn := applyTestFunctionSpec()
	scenarios := applyTestScenarios() // 6 шт., в response только 1

	_, err := llm.ApplyFixtureResponseToScenarios(resp, fn, scenarios, "registration")
	if err == nil {
		t.Fatal("ожидалась ошибка для отсутствующего сценария")
	}
	if !strings.Contains(err.Error(), "missing scenario") {
		t.Errorf("ожидалось 'missing scenario' в ошибке, got: %v", err)
	}
}

func TestApply_missingParam(t *testing.T) {
	raw := `{"scenarios": {"ValidateRegisterRequest/success": {}}}`
	resp, _ := llm.ParseFixtureResponse(raw)
	fn := applyTestFunctionSpec()
	scenarios := applyTestScenarios()[:1]

	_, err := llm.ApplyFixtureResponseToScenarios(resp, fn, scenarios, "registration")
	if err == nil {
		t.Fatal("ожидалась ошибка для отсутствующего параметра")
	}
	if !strings.Contains(err.Error(), "missing param") {
		t.Errorf("ожидалось 'missing param' в ошибке, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"req"`) {
		t.Errorf("ожидалось упоминание имени параметра 'req', got: %v", err)
	}
}

func TestApply_missingNestedField(t *testing.T) {
	// В Address отсутствует City.
	raw := `{"scenarios": {"ValidateRegisterRequest/success": {
		"req": {
			"Email": "x@y.com", "Name": "A", "Age": 25, "Phone": "1",
			"Address": {"Street": "S", "House": "1"},
			"CreatedAt": "2026-01-01T00:00:00Z"
		}
	}}}`
	resp, _ := llm.ParseFixtureResponse(raw)
	fn := applyTestFunctionSpec()
	scenarios := applyTestScenarios()[:1]

	_, err := llm.ApplyFixtureResponseToScenarios(resp, fn, scenarios, "registration")
	if err == nil {
		t.Fatal("ожидалась ошибка для отсутствующего nested-поля")
	}
	if !strings.Contains(err.Error(), "City") {
		t.Errorf("ожидалось упоминание поля 'City', got: %v", err)
	}
}

func TestApply_typeMismatch_intExpectedGotString(t *testing.T) {
	raw := `{"scenarios": {"ValidateRegisterRequest/success": {
		"req": {
			"Email": "x@y.com", "Name": "A", "Age": "twenty-five", "Phone": "1",
			"Address": {"City": "M", "Street": "S", "House": "1"},
			"CreatedAt": "2026-01-01T00:00:00Z"
		}
	}}}`
	resp, _ := llm.ParseFixtureResponse(raw)
	fn := applyTestFunctionSpec()
	scenarios := applyTestScenarios()[:1]

	_, err := llm.ApplyFixtureResponseToScenarios(resp, fn, scenarios, "registration")
	if err == nil {
		t.Fatal("ожидалась ошибка для несоответствия типа")
	}
	if !strings.Contains(err.Error(), "expected number") {
		t.Errorf("ожидалось 'expected number' в ошибке, got: %v", err)
	}
}

func TestApply_invalidTimeString(t *testing.T) {
	raw := `{"scenarios": {"ValidateRegisterRequest/success": {
		"req": {
			"Email": "x@y.com", "Name": "A", "Age": 25, "Phone": "1",
			"Address": {"City": "M", "Street": "S", "House": "1"},
			"CreatedAt": "not-a-date"
		}
	}}}`
	resp, _ := llm.ParseFixtureResponse(raw)
	fn := applyTestFunctionSpec()
	scenarios := applyTestScenarios()[:1]

	_, err := llm.ApplyFixtureResponseToScenarios(resp, fn, scenarios, "registration")
	if err == nil {
		t.Fatal("ожидалась ошибка для невалидной time-строки")
	}
	if !strings.Contains(err.Error(), "RFC3339") {
		t.Errorf("ожидалось 'RFC3339' в ошибке, got: %v", err)
	}
}
