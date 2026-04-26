// Пакет advanced демонстрирует генерацию тестов для функций с
// внешними типами: context.Context, io.Reader, указатели на встроенные типы.
package advanced

import (
	"context"
	"errors"
	"io"
)

// Process читает данные из r, используя ctx для отмены, умножает результат
// на *p и фильтрует items. Возвращает ошибку при невалидных входных данных.
//
// Намеренно содержит nil-guard для ctx, r и p — analyzer должен
// определить Guards.NilCheckedParams для каждого из них.
func Process(ctx context.Context, r io.Reader, p *int, items []string) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	if r == nil {
		return errors.New("nil reader")
	}
	if p == nil {
		return errors.New("nil multiplier")
	}
	if len(items) == 0 {
		return errors.New("empty items")
	}

	// Проверяем отмену контекста.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Имитируем чтение (в реальном коде здесь была бы обработка r и items).
	_ = *p
	return nil
}

// Validate проверяет непустую строку. Демонстрирует simple string guard.
func Validate(s string) error {
	if s == "" {
		return errors.New("пустая строка")
	}
	return nil
}
