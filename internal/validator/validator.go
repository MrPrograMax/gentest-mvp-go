// Пакет validator выполняет лёгкую проверку компиляции сгенерированного
// *_test.go файла командой `go test -run ^$ .`
//
// Паттерн ^$ не совпадает ни с одним тестом, поэтому команда только
// компилирует пакет и test-файлы, не запуская сами тесты.
// Ненулевой код выхода означает синтаксическую или типовую ошибку.
//
// Используется `.` вместо `./...` — проверяем только директорию
// сгенерированного файла, без рекурсивного обхода поддиректорий.
package validator

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Validate компилирует пакет, содержащий filePath.
// Возвращает ошибку если пакет не компилируется.
func Validate(filePath string) error {
	dir := filepath.Dir(filePath)

	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("validator: директория %q не найдена: %w", dir, err)
	}

	// Используем "." вместо "./..." — только текущая директория.
	cmd := exec.Command("go", "test", "-run", "^$", ".")
	cmd.Dir = dir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("validator: компиляция не прошла:\n%s", stderr.String())
	}

	return nil
}
