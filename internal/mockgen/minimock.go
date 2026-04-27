// Пакет mockgen запускает minimock для генерации mock-файлов.
//
// Моки создаются в поддиректории mock/ внутри анализируемого пакета:
//
//	example/service/mock/user_repository_mock.go  (package mock)
//
// Инструмент запускается через "go run" с фиксированной версией,
// поэтому глобальная установка minimock не требуется.
//
// Кроссплатформенность (Windows/Linux/macOS):
// Чтобы избежать передачи абсолютных Windows-путей вида "C:\..." в аргумент -o
// (что приводит к ошибке вида "open ./C:\..."), команда запускается с:
//
//	cmd.Dir = spec.PackageDir   — рабочая директория = директория пакета
//	-o mock/<filename>          — относительный путь в формате forward-slash
//
// Таким образом minimock сам разрешает путь относительно PackageDir,
// и никаких "./C:\" не возникает ни на одной платформе.
package mockgen

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/yourorg/testgen/internal/model"
)

// minimockVersion — версия minimock, используемая для генерации.
// Фиксированная, чтобы вывод был воспроизводимым.
const minimockVersion = "v3.4.7"

// minimockModule — полный import path CLI-команды minimock.
const minimockModule = "github.com/gojuno/minimock/v3/cmd/minimock@" + minimockVersion

// Generate создаёт mock-файл для одного MockSpec.
//
// Алгоритм (кроссплатформенный):
//  1. os.MkdirAll(spec.MockDir) — создаём директорию для моков.
//  2. cmd.Dir = spec.PackageDir — рабочая директория = директория пакета.
//  3. -o = "mock/<filename>"   — ОТНОСИТЕЛЬНЫЙ путь (forward-slash, без "./").
//
// Пример на Windows при PackageDir = "C:\repo\example\service":
//
//	cmd.Dir  = C:\repo\example\service
//	-o       = mock/user_repository_mock.go
//	(minimock создаёт C:\repo\example\service\mock\user_repository_mock.go)
func Generate(m model.MockSpec, logger *log.Logger) error {
	if logger == nil {
		logger = log.New(os.Stderr, "[mockgen] ", 0)
	}

	// 1. Создаём директорию для моков (абсолютный путь, кроссплатформенный).
	if err := os.MkdirAll(m.MockDir, 0o755); err != nil {
		return fmt.Errorf("mockgen: создание директории %q: %w", m.MockDir, err)
	}

	// Относительный путь для флага -o: "mock/user_repository_mock.go".
	// filepath.ToSlash гарантирует forward-slash на всех платформах,
	// что важно для флагов Go-инструментов.
	relOutput := filepath.ToSlash(filepath.Join("mock", m.MockFileName))

	logger.Printf("генерируем мок %s → %s (dir: %s)", m.SourceInterfacePath, relOutput, m.PackageDir)

	// 2. Запускаем minimock через go run.
	// Флаги:
	//   -i  полный путь интерфейса: pkg.Interface
	//   -o  ОТНОСИТЕЛЬНЫЙ путь к файлу (без абсолютного префикса)
	//   -n  имя генерируемого типа (без пакетного префикса)
	//   -p  имя пакета (mock)
	//   -g  добавить //go:generate комментарий
	args := []string{
		"run", minimockModule,
		"-i", m.SourceInterfacePath,
		"-o", relOutput,
		"-n", m.MockType,
		"-p", m.MockPackage,
		"-g",
	}

	cmd := exec.Command("go", args...)
	// cmd.Dir = директория пакета → minimock разрешает -o относительно неё.
	// На Windows это предотвращает ошибку вида "open ./C:\...: синтаксис пути".
	cmd.Dir = m.PackageDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mockgen: minimock для %s: %w\n%s",
			m.InterfaceName, err, stderr.String())
	}

	logger.Printf("сгенерирован %s", m.MockFilePath)
	return nil
}

// OutputArg возвращает аргумент -o, который будет передан в minimock.
// Вынесен в отдельную функцию для unit-тестирования.
// Всегда возвращает относительный путь с forward-slash: "mock/<filename>".
func OutputArg(m model.MockSpec) string {
	return filepath.ToSlash(filepath.Join("mock", m.MockFileName))
}
