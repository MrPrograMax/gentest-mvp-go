// Тесты для app.Run, сфокусированные на валидации FixtureMode.
// Не тестируют полный пайплайн (для этого нужен реальный Go-пакет),
// а проверяют fail-fast поведение до загрузки исходников.
package app_test

import (
	"errors"
	"io"
	"log"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourorg/testgen/internal/app"
	"github.com/yourorg/testgen/internal/fixture"
	"github.com/yourorg/testgen/internal/model"
)

// silentLogger отбрасывает все логи — нужен чтобы тесты не засоряли stdout.
func silentLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

// baseConfig возвращает минимальную конфигурацию с корректным target.
// Target указывает на example/registration — реальный пакет в репозитории.
func baseConfig(fixtureMode model.FixtureMode) app.Config {
	return app.Config{
		Target:      "../../example/registration",
		FixtureMode: fixtureMode,
		Logger:      silentLogger(),
	}
}

// ── fixture.NewProvider ────────────────────────────────────────────────────────

func TestNewProvider_heuristic(t *testing.T) {
	// FixtureHeuristic — единственный реализованный режим.
	p, err := fixture.NewProvider(model.FixtureHeuristic)
	if err != nil {
		t.Fatalf("NewProvider(heuristic) вернул ошибку: %v", err)
	}
	if p == nil {
		t.Fatal("NewProvider(heuristic) вернул nil провайдер")
	}
}

func TestNewProvider_emptyIsHeuristic(t *testing.T) {
	// Пустая строка — синоним heuristic (поведение по умолчанию).
	p, err := fixture.NewProvider("")
	if err != nil {
		t.Fatalf("NewProvider(\"\") вернул ошибку: %v", err)
	}
	if p == nil {
		t.Fatal("NewProvider(\"\") вернул nil провайдер")
	}
}

func TestNewProvider_llm_notImplemented(t *testing.T) {
	// llm возвращает конкретную ошибку ErrLLMNotImplemented.
	_, err := fixture.NewProvider(model.FixtureLLM)
	if err == nil {
		t.Fatal("NewProvider(llm) должен вернуть ошибку")
	}
	if !errors.Is(err, fixture.ErrLLMNotImplemented) {
		t.Errorf("NewProvider(llm): got %v, want errors.Is(ErrLLMNotImplemented)", err)
	}
	if err.Error() != "llm fixture provider is not implemented" {
		t.Errorf("неожиданный текст ошибки: %q", err.Error())
	}
}

func TestNewProvider_hybrid_notImplemented(t *testing.T) {
	// hybrid возвращает конкретную ошибку ErrHybridNotImplemented.
	_, err := fixture.NewProvider(model.FixtureHybrid)
	if err == nil {
		t.Fatal("NewProvider(hybrid) должен вернуть ошибку")
	}
	if !errors.Is(err, fixture.ErrHybridNotImplemented) {
		t.Errorf("NewProvider(hybrid): got %v, want errors.Is(ErrHybridNotImplemented)", err)
	}
	if err.Error() != "hybrid fixture provider is not implemented" {
		t.Errorf("неожиданный текст ошибки: %q", err.Error())
	}
}

func TestNewProvider_unknown(t *testing.T) {
	// Неизвестный режим — ошибка с полезным сообщением.
	_, err := fixture.NewProvider("gpt4")
	if err == nil {
		t.Fatal("NewProvider(unknown) должен вернуть ошибку")
	}
}

// ── app.Run fail-fast ─────────────────────────────────────────────────────────

func TestRun_defaultFixtureModeIsHeuristic(t *testing.T) {
	// Если FixtureMode не задан, Run не должен падать из-за fixture validation.
	//
	// OutputFile направлен во временную директорию t.TempDir() — это гарантирует,
	// что go test никогда не создаёт и не изменяет файлы в example/registration.
	// Директория автоматически удаляется по завершении теста.
	cfg := app.Config{
		Target:     "../../example/registration",
		OutputFile: filepath.Join(t.TempDir(), "registration_test.go"),
		Logger:     silentLogger(),
		// FixtureMode намеренно не задан — должен применяться heuristic по умолчанию
	}
	err := app.Run(cfg)
	// Ошибка может быть из-за чего угодно (go/packages требует реального Go в PATH),
	// но НЕ должна быть связана с fixture mode.
	if err != nil && containsFixtureError(err) {
		t.Errorf("FixtureMode по умолчанию не должен вызывать ошибку fixture mode: %v", err)
	}
}

func TestRun_llm_failsFast(t *testing.T) {
	// llm возвращает ошибку ДО загрузки пакета.
	cfg := baseConfig(model.FixtureLLM)
	err := app.Run(cfg)
	if err == nil {
		t.Fatal("Run с fixture=llm должен вернуть ошибку")
	}
	if !errors.Is(err, fixture.ErrLLMNotImplemented) {
		t.Errorf("Run(llm): ожидался errors.Is(ErrLLMNotImplemented), got: %v", err)
	}
}

func TestRun_hybrid_failsFast(t *testing.T) {
	// hybrid возвращает ошибку ДО загрузки пакета.
	cfg := baseConfig(model.FixtureHybrid)
	err := app.Run(cfg)
	if err == nil {
		t.Fatal("Run с fixture=hybrid должен вернуть ошибку")
	}
	if !errors.Is(err, fixture.ErrHybridNotImplemented) {
		t.Errorf("Run(hybrid): ожидался errors.Is(ErrHybridNotImplemented), got: %v", err)
	}
}

// containsFixtureError проверяет, связана ли ошибка с fixture mode.
func containsFixtureError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "fixture mode") ||
		errors.Is(err, fixture.ErrLLMNotImplemented) ||
		errors.Is(err, fixture.ErrHybridNotImplemented)
}
