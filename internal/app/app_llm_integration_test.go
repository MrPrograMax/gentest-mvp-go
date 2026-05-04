// Интеграционные тесты для --fixture=llm: запускают app.Run на example/registration
// против fake Ollama-сервера (httptest), и проверяют, что значения LLM
// добираются до сгенерированного *_test.go.
//
// Тесты не вызывают реальный Ollama — endpoint конфигурируется через
// cfg.LLMEndpoint и указывает на локальный httptest.Server.
package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yourorg/testgen/internal/app"
	"github.com/yourorg/testgen/internal/model"
)

// llmFixtureRegistrationJSON — валидный 6-сценариевый JSON-ответ для
// example/registration ValidateRegisterRequest. Использует характерные
// "LLM-marker" значения, по которым тесты ищут литералы в выходном файле.
const llmFixtureRegistrationJSON = `{
  "scenarios": {
    "ValidateRegisterRequest/success": {
      "req": {
        "Email": "llm-marker@test.example",
        "Name": "LLM User",
        "Age": 99,
        "Phone": "+79990000001",
        "Address": {"City": "LLMCity", "Street": "LLMStreet", "House": "42"},
        "CreatedAt": "2027-03-15T12:34:56Z"
      }
    },
    "ValidateRegisterRequest/error_empty_email": {
      "req": {
        "Email": "",
        "Name": "LLM User",
        "Age": 99,
        "Phone": "+79990000001",
        "Address": {"City": "LLMCity", "Street": "LLMStreet", "House": "42"},
        "CreatedAt": "2027-03-15T12:34:56Z"
      }
    },
    "ValidateRegisterRequest/error_invalid_email": {
      "req": {
        "Email": "not-an-email",
        "Name": "LLM User",
        "Age": 99,
        "Phone": "+79990000001",
        "Address": {"City": "LLMCity", "Street": "LLMStreet", "House": "42"},
        "CreatedAt": "2027-03-15T12:34:56Z"
      }
    },
    "ValidateRegisterRequest/error_empty_name": {
      "req": {
        "Email": "llm-marker@test.example",
        "Name": "",
        "Age": 99,
        "Phone": "+79990000001",
        "Address": {"City": "LLMCity", "Street": "LLMStreet", "House": "42"},
        "CreatedAt": "2027-03-15T12:34:56Z"
      }
    },
    "ValidateRegisterRequest/error_underage": {
      "req": {
        "Email": "llm-marker@test.example",
        "Name": "LLM User",
        "Age": 7,
        "Phone": "+79990000001",
        "Address": {"City": "LLMCity", "Street": "LLMStreet", "House": "42"},
        "CreatedAt": "2027-03-15T12:34:56Z"
      }
    },
    "ValidateRegisterRequest/error_empty_city": {
      "req": {
        "Email": "llm-marker@test.example",
        "Name": "LLM User",
        "Age": 99,
        "Phone": "+79990000001",
        "Address": {"City": "", "Street": "LLMStreet", "House": "42"},
        "CreatedAt": "2027-03-15T12:34:56Z"
      }
    }
  }
}`

// fakeOllamaServer возвращает httptest.Server, эмулирующий POST /api/generate.
// На каждый запрос инкрементирует *calls (atomic) и отдаёт responseBody
// упакованным в Ollama-обёртку {"response": "..."}.
//
// Если responseBody пустой — отдаёт HTTP 500 (для тестов на ошибки сервера).
func fakeOllamaServer(t *testing.T, calls *int32, responseBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(calls, 1)
		if responseBody == "" {
			http.Error(w, "test: no body configured", http.StatusInternalServerError)
			return
		}
		env := map[string]any{
			"model":    "test",
			"response": responseBody,
			"done":     true,
		}
		body, _ := json.Marshal(env)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
}

// readGeneratedFile читает содержимое сгенерированного файла или валит тест.
func readGeneratedFile(t *testing.T, outPath string) string {
	t.Helper()
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("не удалось прочитать сгенерированный файл %s: %v", outPath, err)
	}
	return string(b)
}

// llmConfig возвращает Config для --fixture=llm с заданным endpoint и
// outPath во временной директории.
func llmConfig(t *testing.T, endpoint string) (app.Config, string) {
	t.Helper()
	outPath := filepath.Join(t.TempDir(), "registration_test.go")
	return app.Config{
		Target:      "../../example/registration",
		OutputFile:  outPath,
		FixtureMode: model.FixtureLLM,
		LLMProvider: "ollama",
		LLMModel:    "test",
		LLMEndpoint: endpoint,
		Logger:      silentLogger(),
	}, outPath
}

// ── Happy path: LLM-значения доходят до сгенерированного файла ────────────────

func TestRun_llm_writesFileWithLLMValues(t *testing.T) {
	var calls int32
	srv := fakeOllamaServer(t, &calls, llmFixtureRegistrationJSON)
	defer srv.Close()

	cfg, outPath := llmConfig(t, srv.URL)
	if err := app.Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("ожидался 1 HTTP-вызов сервера, получено %d", got)
	}

	src := readGeneratedFile(t, outPath)

	// Ключевые маркеры из LLM-фикстуры.
	wantContains := []string{
		// success-сценарий получил LLM email.
		`Email: "llm-marker@test.example"`,
		// nested struct из LLM ответа.
		`City: "LLMCity"`,
		// time.Time → time.Date(...) литерал из RFC3339.
		`time.Date(2027, time.March, 15, 12, 34, 56, 0, time.UTC)`,
		// import "time" подтянут import-планировщиком.
		`"time"`,
		// underage сценарий получил Age: 7 из LLM (не heuristic 17).
		`Age: 7`,
	}
	for _, w := range wantContains {
		if !strings.Contains(src, w) {
			t.Errorf("сгенерированный файл не содержит %q\n--- файл ---\n%s", w, src)
		}
	}
}

// LLM-режим не должен оставлять в файле heuristic-маркеры.
// time.Now() — heuristic для time.Time; "user@example.com" — heuristic для Email.
func TestRun_llm_doesNotEmitHeuristicValues(t *testing.T) {
	var calls int32
	srv := fakeOllamaServer(t, &calls, llmFixtureRegistrationJSON)
	defer srv.Close()

	cfg, outPath := llmConfig(t, srv.URL)
	if err := app.Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	src := readGeneratedFile(t, outPath)

	if strings.Contains(src, "time.Now()") {
		t.Errorf("LLM-сгенерированный файл не должен содержать time.Now()\n--- файл ---\n%s", src)
	}
	if strings.Contains(src, `"user@example.com"`) {
		t.Errorf("LLM-сгенерированный файл не должен содержать heuristic email\n--- файл ---\n%s", src)
	}
}

// ── dry-run не вызывает сервер и не пишет файл ────────────────────────────────

func TestRun_llmDryRun_doesNotCallServer(t *testing.T) {
	var calls int32
	srv := fakeOllamaServer(t, &calls, "should-not-be-used")
	defer srv.Close()

	outPath := filepath.Join(t.TempDir(), "registration_test.go")
	cfg := app.Config{
		Target:      "../../example/registration",
		OutputFile:  outPath,
		FixtureMode: model.FixtureLLM,
		LLMDryRun:   true,
		LLMProvider: "ollama",
		LLMModel:    "test",
		LLMEndpoint: srv.URL,
		Logger:      silentLogger(),
	}
	// dry-run печатает payload в os.Stdout — для теста это не критично,
	// stdout от тестов идёт в test output и не ломает их.
	if err := app.Run(cfg); err != nil {
		t.Fatalf("Run dry-run: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("dry-run не должен вызывать сервер, было %d вызовов", got)
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("dry-run не должен создавать output файл, но %s существует (или ошибка stat: %v)",
			outPath, err)
	}
}

// ── Ошибочные ответы LLM: fail-fast, файл не пишется ─────────────────────────

func TestRun_llm_invalidJSON_returnsError(t *testing.T) {
	var calls int32
	srv := fakeOllamaServer(t, &calls, "this is not JSON at all")
	defer srv.Close()

	cfg, outPath := llmConfig(t, srv.URL)
	err := app.Run(cfg)
	if err == nil {
		t.Fatal("ожидалась ошибка для невалидного JSON-ответа")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("ожидалось 'invalid JSON' в ошибке, got: %v", err)
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("при ошибке файл не должен быть создан: %s", outPath)
	}
}

func TestRun_llm_missingScenario_returnsValidationError(t *testing.T) {
	// JSON содержит только success — в request 6 сценариев.
	incomplete := `{
		"scenarios": {
			"ValidateRegisterRequest/success": {
				"req": {
					"Email": "x@y.com", "Name": "A", "Age": 25, "Phone": "1",
					"Address": {"City": "M", "Street": "S", "House": "1"},
					"CreatedAt": "2026-01-01T00:00:00Z"
				}
			}
		}
	}`
	var calls int32
	srv := fakeOllamaServer(t, &calls, incomplete)
	defer srv.Close()

	cfg, outPath := llmConfig(t, srv.URL)
	err := app.Run(cfg)
	if err == nil {
		t.Fatal("ожидалась ошибка для неполного JSON-ответа")
	}
	if !strings.Contains(err.Error(), "missing scenario") {
		t.Errorf("ожидалось 'missing scenario' в ошибке, got: %v", err)
	}
	// Никакого silent fallback — файл не пишется.
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("при ошибке валидации файл не должен быть создан: %s", outPath)
	}
}

// ── heuristic mode не изменился ───────────────────────────────────────────────

func TestRun_heuristic_unchanged_emitsTimeNow(t *testing.T) {
	// Heuristic mode остаётся прежним: для time.Time-поля emit time.Now(),
	// для Email — "user@example.com". Это контрольный тест, что
	// рефактор --fixture=llm не задел heuristic ветку.
	outPath := filepath.Join(t.TempDir(), "registration_test.go")
	cfg := app.Config{
		Target:      "../../example/registration",
		OutputFile:  outPath,
		FixtureMode: model.FixtureHeuristic,
		Logger:      silentLogger(),
	}
	if err := app.Run(cfg); err != nil {
		t.Fatalf("Run heuristic: %v", err)
	}
	src := readGeneratedFile(t, outPath)

	if !strings.Contains(src, "time.Now()") {
		t.Errorf("heuristic mode должен emit time.Now() для CreatedAt\n--- файл ---\n%s", src)
	}
	if !strings.Contains(src, `"user@example.com"`) {
		t.Errorf("heuristic mode должен использовать \"user@example.com\" как Email\n--- файл ---\n%s", src)
	}
	// LLM-маркеры в heuristic-файле не должны появляться.
	if strings.Contains(src, "llm-marker") {
		t.Errorf("heuristic-сгенерированный файл не должен содержать LLM-маркеры")
	}
}
