package llm_test

import (
	"strings"
	"testing"

	"github.com/yourorg/testgen/internal/llm"
)

// validRegistrationResponse — полный валидный JSON-ответ для ValidateRegisterRequest.
const validRegistrationResponse = `{
  "scenarios": {
    "ValidateRegisterRequest/success": {
      "req": {
        "Email": "user@example.com",
        "Name": "Test User",
        "Age": 25,
        "Phone": "+79991234567",
        "Address": {
          "City": "Moscow",
          "Street": "Tverskaya",
          "House": "1"
        },
        "CreatedAt": "2024-01-01T00:00:00Z"
      }
    },
    "ValidateRegisterRequest/error_empty_email": {
      "req": {
        "Email": "",
        "Name": "Test User",
        "Age": 25,
        "Phone": "+79991234567",
        "Address": {
          "City": "Moscow",
          "Street": "Tverskaya",
          "House": "1"
        },
        "CreatedAt": "2024-01-01T00:00:00Z"
      }
    }
  }
}`

// minimalRequest — минимальный Request для тестов валидации.
func minimalRegistrationRequest() llm.Request {
	addrFields := []llm.StructFieldPayload{
		{Name: "City", TypeStr: "string", Kind: "string"},
		{Name: "Street", TypeStr: "string", Kind: "string"},
		{Name: "House", TypeStr: "string", Kind: "string"},
	}
	return llm.Request{
		Function: llm.FunctionPayload{
			Name:    "ValidateRegisterRequest",
			Package: "registration",
			Params: []llm.ParamPayload{
				{
					Name:    "req",
					TypeStr: "RegisterRequest",
					Kind:    "struct",
					StructFields: []llm.StructFieldPayload{
						{Name: "Email", TypeStr: "string", Kind: "string"},
						{Name: "Name", TypeStr: "string", Kind: "string"},
						{Name: "Age", TypeStr: "int", Kind: "int"},
						{Name: "Phone", TypeStr: "string", Kind: "string"},
						{Name: "Address", TypeStr: "Address", Kind: "struct", Fields: addrFields},
						{Name: "CreatedAt", TypeStr: "time.Time", Kind: "time"},
					},
				},
			},
		},
		Scenarios: []llm.ScenarioPayload{
			{Name: "ValidateRegisterRequest/success", Kind: "success", WantError: false},
			{Name: "ValidateRegisterRequest/error_empty_email", Kind: "error", WantError: true},
		},
	}
}

// ── ParseFixtureResponse ──────────────────────────────────────────────────────

func TestParseFixtureResponse_valid(t *testing.T) {
	resp, err := llm.ParseFixtureResponse(validRegistrationResponse)
	if err != nil {
		t.Fatalf("ParseFixtureResponse: %v", err)
	}
	if len(resp.Scenarios) != 2 {
		t.Errorf("Scenarios count = %d, want 2", len(resp.Scenarios))
	}
}

func TestParseFixtureResponse_invalidJSON(t *testing.T) {
	_, err := llm.ParseFixtureResponse("not json at all")
	if err == nil {
		t.Fatal("ожидалась ошибка для невалидного JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("ожидалось 'invalid JSON', got: %v", err)
	}
}

func TestParseFixtureResponse_missingScenariosKey(t *testing.T) {
	_, err := llm.ParseFixtureResponse(`{"something": {}}`)
	if err == nil {
		t.Fatal("ожидалась ошибка при отсутствии ключа scenarios")
	}
	if !strings.Contains(err.Error(), "scenarios") {
		t.Errorf("ожидалось упоминание 'scenarios', got: %v", err)
	}
}

func TestParseFixtureResponse_stripsMarkdownFence(t *testing.T) {
	raw := "```json\n" + validRegistrationResponse + "\n```"
	resp, err := llm.ParseFixtureResponse(raw)
	if err != nil {
		t.Fatalf("ParseFixtureResponse с markdown fence: %v", err)
	}
	if len(resp.Scenarios) == 0 {
		t.Error("Scenarios не должен быть пустым после strip markdown")
	}
}

func TestParseFixtureResponse_stripsMarkdownFenceNoLang(t *testing.T) {
	raw := "```\n" + validRegistrationResponse + "\n```"
	resp, err := llm.ParseFixtureResponse(raw)
	if err != nil {
		t.Fatalf("ParseFixtureResponse с ``` fence: %v", err)
	}
	if len(resp.Scenarios) == 0 {
		t.Error("Scenarios не должен быть пустым")
	}
}

// ── ValidateFixtureResponse ───────────────────────────────────────────────────

func TestValidateFixtureResponse_valid(t *testing.T) {
	resp, _ := llm.ParseFixtureResponse(validRegistrationResponse)
	req := minimalRegistrationRequest()
	if err := llm.ValidateFixtureResponse(resp, req); err != nil {
		t.Errorf("валидный ответ не должен вызывать ошибку: %v", err)
	}
}

func TestValidateFixtureResponse_missingScenario(t *testing.T) {
	// Ответ содержит только success, а в request ещё и error_empty_email.
	raw := `{"scenarios": {"ValidateRegisterRequest/success": {"req": {"Email": "x@x.com", "Name": "A", "Age": 25, "Phone": "1", "Address": {"City": "M", "Street": "S", "House": "1"}, "CreatedAt": "2024-01-01"}}}}`
	resp, _ := llm.ParseFixtureResponse(raw)

	req := minimalRegistrationRequest()
	err := llm.ValidateFixtureResponse(resp, req)
	if err == nil {
		t.Fatal("ожидалась ошибка при отсутствующем сценарии")
	}
	if !strings.Contains(err.Error(), "missing scenario") {
		t.Errorf("ожидалось 'missing scenario', got: %v", err)
	}
	if !strings.Contains(err.Error(), "error_empty_email") {
		t.Errorf("ожидалось упоминание 'error_empty_email', got: %v", err)
	}
}

func TestValidateFixtureResponse_missingParam(t *testing.T) {
	// Сценарий присутствует, но в нём нет параметра "req".
	raw := `{"scenarios": {
		"ValidateRegisterRequest/success": {},
		"ValidateRegisterRequest/error_empty_email": {}
	}}`
	resp, _ := llm.ParseFixtureResponse(raw)

	req := minimalRegistrationRequest()
	err := llm.ValidateFixtureResponse(resp, req)
	if err == nil {
		t.Fatal("ожидалась ошибка при отсутствующем параметре")
	}
	if !strings.Contains(err.Error(), "missing param") {
		t.Errorf("ожидалось 'missing param', got: %v", err)
	}
	if !strings.Contains(err.Error(), `"req"`) {
		t.Errorf("ожидалось упоминание param 'req', got: %v", err)
	}
}

func TestValidateFixtureResponse_missingField(t *testing.T) {
	// Параметр req есть, но в нём нет поля Email.
	raw := `{"scenarios": {
		"ValidateRegisterRequest/success": {
			"req": {"Name": "A", "Age": 25, "Phone": "1", "Address": {"City": "M", "Street": "S", "House": "1"}, "CreatedAt": "2024-01-01"}
		},
		"ValidateRegisterRequest/error_empty_email": {
			"req": {"Name": "A", "Age": 25, "Phone": "1", "Address": {"City": "M", "Street": "S", "House": "1"}, "CreatedAt": "2024-01-01"}
		}
	}}`
	resp, _ := llm.ParseFixtureResponse(raw)

	req := minimalRegistrationRequest()
	err := llm.ValidateFixtureResponse(resp, req)
	if err == nil {
		t.Fatal("ожидалась ошибка при отсутствующем поле")
	}
	if !strings.Contains(err.Error(), "missing field") {
		t.Errorf("ожидалось 'missing field', got: %v", err)
	}
	if !strings.Contains(err.Error(), `"Email"`) {
		t.Errorf("ожидалось упоминание поля 'Email', got: %v", err)
	}
}

func TestValidateFixtureResponse_wrongPrimitiveType_ageAsString(t *testing.T) {
	// Age должен быть number, а не string.
	raw := `{"scenarios": {
		"ValidateRegisterRequest/success": {
			"req": {"Email": "x@x.com", "Name": "A", "Age": "25", "Phone": "1", "Address": {"City": "M", "Street": "S", "House": "1"}, "CreatedAt": "2024-01-01"}
		},
		"ValidateRegisterRequest/error_empty_email": {
			"req": {"Email": "", "Name": "A", "Age": "25", "Phone": "1", "Address": {"City": "M", "Street": "S", "House": "1"}, "CreatedAt": "2024-01-01"}
		}
	}}`
	resp, _ := llm.ParseFixtureResponse(raw)

	req := minimalRegistrationRequest()
	err := llm.ValidateFixtureResponse(resp, req)
	if err == nil {
		t.Fatal("ожидалась ошибка при неправильном типе поля Age")
	}
	if !strings.Contains(err.Error(), "expected number") {
		t.Errorf("ожидалось 'expected number', got: %v", err)
	}
}

func TestValidateFixtureResponse_nestedAddressCity(t *testing.T) {
	// Вложенное поле Address.City отсутствует.
	raw := `{"scenarios": {
		"ValidateRegisterRequest/success": {
			"req": {"Email": "x@x.com", "Name": "A", "Age": 25, "Phone": "1", "Address": {"Street": "S", "House": "1"}, "CreatedAt": "2024-01-01"}
		},
		"ValidateRegisterRequest/error_empty_email": {
			"req": {"Email": "", "Name": "A", "Age": 25, "Phone": "1", "Address": {"Street": "S", "House": "1"}, "CreatedAt": "2024-01-01"}
		}
	}}`
	resp, _ := llm.ParseFixtureResponse(raw)

	req := minimalRegistrationRequest()
	err := llm.ValidateFixtureResponse(resp, req)
	if err == nil {
		t.Fatal("ожидалась ошибка при отсутствующем вложенном поле City")
	}
	if !strings.Contains(err.Error(), "missing field") {
		t.Errorf("ожидалось 'missing field', got: %v", err)
	}
	if !strings.Contains(err.Error(), `"City"`) {
		t.Errorf("ожидалось упоминание поля 'City', got: %v", err)
	}
}

func TestValidateFixtureResponse_timeAcceptsString(t *testing.T) {
	// CreatedAt (kind=time) должен принимать JSON string.
	resp, _ := llm.ParseFixtureResponse(validRegistrationResponse)
	req := minimalRegistrationRequest()
	// validRegistrationResponse уже содержит CreatedAt как строку — должно пройти.
	if err := llm.ValidateFixtureResponse(resp, req); err != nil {
		t.Errorf("time.Time как string должна быть валидной: %v", err)
	}
}

func TestValidateFixtureResponse_extraFieldsAllowed(t *testing.T) {
	// Лишние поля в ответе должны быть разрешены.
	raw := `{"scenarios": {
		"ValidateRegisterRequest/success": {
			"req": {
				"Email": "x@x.com", "Name": "A", "Age": 25, "Phone": "1",
				"Address": {"City": "M", "Street": "S", "House": "1"},
				"CreatedAt": "2024-01-01",
				"ExtraUnknownField": "ignored"
			},
			"extra_param": "ignored"
		},
		"ValidateRegisterRequest/error_empty_email": {
			"req": {
				"Email": "", "Name": "A", "Age": 25, "Phone": "1",
				"Address": {"City": "M", "Street": "S", "House": "1"},
				"CreatedAt": "2024-01-01"
			}
		}
	}, "extra_top_level": "ignored"}`
	resp, _ := llm.ParseFixtureResponse(raw)

	req := minimalRegistrationRequest()
	if err := llm.ValidateFixtureResponse(resp, req); err != nil {
		t.Errorf("лишние поля должны быть разрешены: %v", err)
	}
}
