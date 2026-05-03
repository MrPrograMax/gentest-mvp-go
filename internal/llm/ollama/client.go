// Пакет ollama реализует HTTP-клиент для локального Ollama API.
//
// Ollama API endpoint: POST http://localhost:11434/api/generate
// Документация: https://github.com/ollama/ollama/blob/main/docs/api.md
//
// Клиент принимает llm.Request (JSON-описание функции и сценариев),
// формирует prompt и возвращает сырой текст ответа от модели.
// Генератор сам парсит ответ — модель возвращает только JSON, не Go-код.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yourorg/testgen/internal/llm"
)

const (
	// DefaultEndpoint — адрес локального Ollama по умолчанию.
	DefaultEndpoint = "http://localhost:11434"

	// DefaultTimeout — максимальное время ожидания ответа от Ollama.
	// Генерация может занять несколько минут для больших моделей.
	DefaultTimeout = 5 * time.Minute
)

// Client выполняет запросы к Ollama API.
type Client struct {
	endpoint   string
	model      string
	httpClient *http.Client
}

// New создаёт Client с заданным endpoint и именем модели.
//
// Если endpoint пустой, используется DefaultEndpoint.
// Если model пустой — Generate вернёт ошибку при вызове.
func New(endpoint, model string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    model,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// NewWithTimeout создаёт Client с кастомным таймаутом (для тестов).
func NewWithTimeout(endpoint, model string, timeout time.Duration) *Client {
	c := New(endpoint, model)
	c.httpClient.Timeout = timeout
	return c
}

// ── Ollama wire types ─────────────────────────────────────────────────────────

// generateRequest — тело POST /api/generate.
type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"` // false → единственный JSON-объект в ответе
}

// generateResponse — успешный ответ /api/generate (stream: false).
type generateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"` // текст ответа модели
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"` // ошибка на стороне Ollama
}

// ── Public API ────────────────────────────────────────────────────────────────

// Generate отправляет fixture-запрос в Ollama и возвращает сырой текст ответа.
//
// Шаги:
//  1. Проверить, что model задана.
//  2. Сформировать prompt из llm.Request.
//  3. Отправить POST /api/generate.
//  4. Распарсить ответ и вернуть Response-поле.
//
// Возвращаемый текст — ответ модели как есть (JSON fixture values).
// Парсинг и валидация JSON — ответственность вызывающего.
func (c *Client) Generate(ctx context.Context, req llm.Request) (string, error) {
	if c.model == "" {
		return "", fmt.Errorf("ollama: model is empty; specify --llm-model")
	}

	prompt := BuildPrompt(req)

	body, err := json.Marshal(generateRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
	})
	if err != nil {
		return "", fmt.Errorf("ollama: marshal request: %w", err)
	}

	url := c.endpoint + "/api/generate"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ollama unavailable at %s: %w", c.endpoint, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	var genResp generateResponse
	if err := json.Unmarshal(raw, &genResp); err != nil {
		return "", fmt.Errorf("ollama: invalid response (not JSON): %w; body: %s",
			err, truncate(string(raw), 200))
	}

	if genResp.Error != "" {
		return "", fmt.Errorf("ollama: model error: %s", genResp.Error)
	}

	return genResp.Response, nil
}

// ── Prompt builder ────────────────────────────────────────────────────────────

// BuildPrompt формирует текстовый prompt из llm.Request.
//
// Prompt содержит:
//  1. Системную инструкцию (что ожидается от модели).
//  2. JSON-описание функции и сценариев.
//
// Экспортирован для тестирования и dry-run.
func BuildPrompt(req llm.Request) string {
	payload, _ := json.MarshalIndent(req.Function, "", "  ")

	var sb strings.Builder
	sb.WriteString(req.Instructions)
	sb.WriteString("\n\n")
	sb.WriteString("Function to test:\n")
	sb.Write(payload)
	sb.WriteString("\n\nScenarios:\n")
	for _, sc := range req.Scenarios {
		sb.WriteString(fmt.Sprintf("- %s (want_error=%v): %s\n",
			sc.Name, sc.WantError, sc.Hint))
	}
	sb.WriteString("\nReturn ONLY a JSON object with fixture values.")
	return sb.String()
}

// truncate обрезает строку до maxLen символов для error-сообщений.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
