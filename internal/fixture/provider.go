// Файл provider.go определяет интерфейс fixture.Provider и его реализации.
//
// Архитектура:
//
//	Provider            — интерфейс, описывающий контракт поставщика фикстур.
//	HeuristicProvider   — текущая реализация: детерминированные правила.
//	(LLMProvider)       — будущая реализация через LLM API (не реализовано).
//	(HybridProvider)    — будущая реализация: эвристика + LLM (не реализовано).
//
// Пакетные функции Happy/Zero/Empty делегируют DefaultProvider(),
// поэтому весь существующий код (scenario, render) работает без изменений.
package fixture

import (
	"errors"

	"github.com/yourorg/testgen/internal/model"
)

// Provider — контракт поставщика тестовых фикстур.
//
// Метод Happy возвращает «осмысленное» непустое значение для success-сценария.
// Метод Zero возвращает нулевое/nil значение.
// Метод Empty возвращает явно пустое значение ([] T{}, "", map[K]V{}).
// Разграничение Zero/Empty принципиально для edge-сценариев:
// Zero(slice)=nil проверяет nil-guard, Empty(slice)=[]T{} — empty-guard.
type Provider interface {
	Happy(kind model.TypeKind, typeStr string) model.FixtureValue
	Zero(kind model.TypeKind, typeStr string) model.FixtureValue
	Empty(kind model.TypeKind, typeStr string) model.FixtureValue
}

// ── HeuristicProvider ─────────────────────────────────────────────────────────

// HeuristicProvider реализует Provider через детерминированные правила:
//   - int → 42, string → "test-value", bool → true
//   - *T → new(T), []T → []T{"test-value"}, context.Context → context.Background()
//   - func(...) → безопасная заглушка с zero-returns
//
// Это единственная полностью реализованная стратегия на текущий момент.
type HeuristicProvider struct{}

// NewHeuristicProvider создаёт HeuristicProvider.
func NewHeuristicProvider() *HeuristicProvider { return &HeuristicProvider{} }

// Happy делегирует в пакетную функцию Happy (детерминированные эвристики).
func (h *HeuristicProvider) Happy(kind model.TypeKind, typeStr string) model.FixtureValue {
	return Happy(kind, typeStr)
}

// Zero делегирует в пакетную функцию Zero.
func (h *HeuristicProvider) Zero(kind model.TypeKind, typeStr string) model.FixtureValue {
	return Zero(kind, typeStr)
}

// Empty делегирует в пакетную функцию Empty.
func (h *HeuristicProvider) Empty(kind model.TypeKind, typeStr string) model.FixtureValue {
	return Empty(kind, typeStr)
}

// ── Заглушки для будущих провайдеров ─────────────────────────────────────────

// errNotImplemented возвращает стандартную ошибку для не реализованных провайдеров.
// Вынесена в переменную чтобы тесты могли сравнивать через errors.Is.
var (
	ErrLLMNotImplemented    = errors.New("llm fixture provider is not implemented")
	ErrHybridNotImplemented = errors.New("hybrid fixture provider is not implemented")
)

// ── Фабрика ───────────────────────────────────────────────────────────────────

// NewProvider создаёт Provider для заданного FixtureMode.
//
// Возвращает ошибку для ещё не реализованных режимов (llm, hybrid).
// Это позволяет fail-fast в app.Run до начала анализа пакета.
func NewProvider(mode model.FixtureMode) (Provider, error) {
	switch mode {
	case model.FixtureHeuristic, "":
		return NewHeuristicProvider(), nil
	case model.FixtureLLM:
		return nil, ErrLLMNotImplemented
	case model.FixtureHybrid:
		return nil, ErrHybridNotImplemented
	default:
		return nil, errors.New("неизвестный fixture mode: " + string(mode))
	}
}

// DefaultProvider возвращает HeuristicProvider — провайдер по умолчанию.
// Используется пакетными функциями Happy/Zero/Empty для обратной совместимости.
func DefaultProvider() Provider {
	return NewHeuristicProvider()
}
