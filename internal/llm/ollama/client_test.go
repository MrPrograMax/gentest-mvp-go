package ollama_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/testgen/internal/llm"
	"github.com/yourorg/testgen/internal/llm/ollama"
)

// minimalRequest — минимальный llm.Request для тестов.
func minimalRequest() llm.Request {
	return llm.Request{
		Function: llm.FunctionPayload{
			Name:    "TestFunc",
			Package: "pkg",
			Params: []llm.ParamPayload{
				{Name: "x", TypeStr: "string", Kind: "string"},
			},
		},
		Scenarios: []llm.ScenarioPayload{
			{Name: "TestFunc/success", Kind: "success", WantError: false, Hint: "valid values"},
		},
		Instructions: "Return JSON only.",
	}
}

// ollamaServer создаёт httptest.Server, возвращающий заданный response JSON.
func ollamaServer(t *testing.T, statusCode int, body interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("server encode: %v", err)
		}
	}))
}

// ── Generate: happy path ──────────────────────────────────────────────────────

func TestGenerate_success(t *testing.T) {
	want := `{"scenarios":{"TestFunc/success":{"x":"hello"}}}`
	srv := ollamaServer(t, http.StatusOK, map[string]interface{}{
		"model":    "llama3",
		"response": want,
		"done":     true,
	})
	defer srv.Close()

	c := ollama.NewWithTimeout(srv.URL, "llama3", 5*time.Second)
	got, err := c.Generate(context.Background(), minimalRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != want {
		t.Errorf("response = %q, want %q", got, want)
	}
}

func TestGenerate_requestBodyContainsModel(t *testing.T) {
	// Проверяем, что клиент передаёт правильное имя модели.
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"response": `{}`,
			"done":     true,
		})
	}))
	defer srv.Close()

	c := ollama.NewWithTimeout(srv.URL, "mistral", 5*time.Second)
	_, _ = c.Generate(context.Background(), minimalRequest())

	if received["model"] != "mistral" {
		t.Errorf("request model = %v, want mistral", received["model"])
	}
	if received["stream"] != false {
		t.Errorf("request stream = %v, want false (single-response mode)", received["stream"])
	}
}

func TestGenerate_requestBodyContainsPrompt(t *testing.T) {
	// Prompt должен содержать имя функции и сценарий.
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		json.NewEncoder(w).Encode(map[string]interface{}{"response": `{}`, "done": true})
	}))
	defer srv.Close()

	c := ollama.NewWithTimeout(srv.URL, "llama3", 5*time.Second)
	_, _ = c.Generate(context.Background(), minimalRequest())

	prompt, _ := received["prompt"].(string)
	if !strings.Contains(prompt, "TestFunc") {
		t.Errorf("prompt не содержит имени функции TestFunc: %q", prompt[:min(len(prompt), 200)])
	}
	if !strings.Contains(prompt, "TestFunc/success") {
		t.Errorf("prompt не содержит имени сценария TestFunc/success")
	}
}

// ── Generate: error cases ─────────────────────────────────────────────────────

func TestGenerate_emptyModel_returnsError(t *testing.T) {
	srv := ollamaServer(t, http.StatusOK, map[string]interface{}{"response": "{}", "done": true})
	defer srv.Close()

	c := ollama.NewWithTimeout(srv.URL, "", 5*time.Second) // пустая модель
	_, err := c.Generate(context.Background(), minimalRequest())
	if err == nil {
		t.Fatal("ожидалась ошибка при пустой модели")
	}
	if !strings.Contains(err.Error(), "model is empty") {
		t.Errorf("ожидалось 'model is empty', got: %v", err)
	}
}

func TestGenerate_ollamaUnavailable_returnsError(t *testing.T) {
	// Указываем несуществующий порт.
	c := ollama.NewWithTimeout("http://127.0.0.1:19999", "llama3", 2*time.Second)
	_, err := c.Generate(context.Background(), minimalRequest())
	if err == nil {
		t.Fatal("ожидалась ошибка при недоступном Ollama")
	}
	if !strings.Contains(err.Error(), "ollama unavailable") {
		t.Errorf("ожидалось 'ollama unavailable', got: %v", err)
	}
}

func TestGenerate_httpErrorStatus_returnsError(t *testing.T) {
	srv := ollamaServer(t, http.StatusInternalServerError, map[string]interface{}{
		"error": "model not found",
	})
	defer srv.Close()

	c := ollama.NewWithTimeout(srv.URL, "unknown-model", 5*time.Second)
	_, err := c.Generate(context.Background(), minimalRequest())
	if err == nil {
		t.Fatal("ожидалась ошибка при HTTP 500")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("ожидалось 'HTTP 500', got: %v", err)
	}
}

func TestGenerate_ollamaModelError_returnsError(t *testing.T) {
	// Ollama вернул HTTP 200, но с полем error.
	srv := ollamaServer(t, http.StatusOK, map[string]interface{}{
		"error": "model 'nonexistent' not found",
		"done":  false,
	})
	defer srv.Close()

	c := ollama.NewWithTimeout(srv.URL, "nonexistent", 5*time.Second)
	_, err := c.Generate(context.Background(), minimalRequest())
	if err == nil {
		t.Fatal("ожидалась ошибка при model error в ответе")
	}
	if !strings.Contains(err.Error(), "model error") {
		t.Errorf("ожидалось 'model error', got: %v", err)
	}
}

func TestGenerate_invalidJSON_returnsError(t *testing.T) {
	// Сервер возвращает не-JSON ответ.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json at all"))
	}))
	defer srv.Close()

	c := ollama.NewWithTimeout(srv.URL, "llama3", 5*time.Second)
	_, err := c.Generate(context.Background(), minimalRequest())
	if err == nil {
		t.Fatal("ожидалась ошибка при невалидном JSON")
	}
	if !strings.Contains(err.Error(), "invalid response") {
		t.Errorf("ожидалось 'invalid response', got: %v", err)
	}
}

func TestGenerate_contextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := ollama.NewWithTimeout("http://127.0.0.1:1", "llama3", 10*time.Second)

	_, err := c.Generate(ctx, minimalRequest())
	if err == nil {
		t.Fatal("ожидалась ошибка при отменённом контексте")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ожидалась context.Canceled, got %v", err)
	}
}

// ── BuildPrompt ───────────────────────────────────────────────────────────────

func TestBuildPrompt_containsFunctionName(t *testing.T) {
	req := minimalRequest()
	prompt := ollama.BuildPrompt(req)
	if !strings.Contains(prompt, "TestFunc") {
		t.Errorf("prompt не содержит TestFunc: %q", prompt[:min(len(prompt), 300)])
	}
}

func TestBuildPrompt_containsInstructions(t *testing.T) {
	req := minimalRequest()
	prompt := ollama.BuildPrompt(req)
	if !strings.Contains(prompt, req.Instructions) {
		t.Errorf("prompt не содержит instructions")
	}
}

func TestBuildPrompt_containsScenarioNames(t *testing.T) {
	req := minimalRequest()
	prompt := ollama.BuildPrompt(req)
	if !strings.Contains(prompt, "TestFunc/success") {
		t.Errorf("prompt не содержит имени сценария")
	}
}

func TestBuildPrompt_endsWithJsonInstruction(t *testing.T) {
	req := minimalRequest()
	prompt := ollama.BuildPrompt(req)
	if !strings.Contains(prompt, "JSON") {
		t.Errorf("prompt не содержит упоминания JSON")
	}
}

// ── New ───────────────────────────────────────────────────────────────────────

func TestNew_defaultEndpoint(t *testing.T) {
	// Без endpoint → используется DefaultEndpoint.
	// Проверяем косвенно: клиент создаётся без паники.
	c := ollama.New("", "llama3")
	if c == nil {
		t.Error("New(\"\", ...) вернул nil")
	}
}

// min — вспомогательная функция для совместимости с Go < 1.21.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
