// Пакет app связывает все подпакеты testgen в единый вызов Run.
//
// Пайплайн:
//  1. loader.Load      — загружает Go-пакет с типовой информацией.
//  2. analyzer.Analyze — извлекает []FunctionSpec (с Guards).
//  3. Для каждой функции:
//     a. mockplan.Analyze   — логирует interface-параметры (справочно).
//     b. scenario.Generate  — строит []ScenarioSpec по Guards-фактам.
//  4. render.RenderFile  — выполняет template + go/format → []byte.
//  5. os.WriteFile       — записывает файл.
//  6. validator.Validate — опциональная проверка компиляции.
package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/yourorg/testgen/internal/analyzer"
	"github.com/yourorg/testgen/internal/loader"
	"github.com/yourorg/testgen/internal/mockplan"
	"github.com/yourorg/testgen/internal/model"
	"github.com/yourorg/testgen/internal/render"
	"github.com/yourorg/testgen/internal/scenario"
	"github.com/yourorg/testgen/internal/validator"
)

// Config — параметры одного вызова testgen.
type Config struct {
	// Target — путь к директории пакета или одному .go файлу.
	Target string

	// OutputFile — путь для записи сгенерированного *_test.go.
	// Если пусто, путь выводится автоматически из Target.
	OutputFile string

	// RunValidation — запускать ли `go test -run ^$ .` после записи.
	RunValidation bool

	// Logger — получает сообщения о прогрессе и предупреждения.
	Logger *log.Logger
}

// Run выполняет полный пайплайн testgen для cfg.Target.
func Run(cfg Config) error {
	if cfg.Logger == nil {
		cfg.Logger = log.New(os.Stderr, "[testgen] ", 0)
	}

	// 1. Загрузка
	cfg.Logger.Printf("загружаем %s", cfg.Target)
	loaded, err := loader.Load(cfg.Target)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}

	// 2. Анализ
	specs, err := analyzer.Analyze(loaded)
	if err != nil {
		return fmt.Errorf("analyze: %w", err)
	}
	cfg.Logger.Printf("найдено %d экспортируемых функций", len(specs))
	if len(specs) == 0 {
		return fmt.Errorf("экспортируемые функции не найдены в %s", cfg.Target)
	}

	// 3. Формируем TestSpec
	var tests []model.TestSpec
	for _, fn := range specs {
		// a. Mock plan (только диагностика — мок-код не генерируется)
		plan := mockplan.Analyze(fn)
		if plan.HasMocks() {
			for _, e := range plan.Entries {
				cfg.Logger.Printf("  %s: параметр %q (%s) — интерфейс: %s",
					fn.Name, e.ParamName, e.TypeStr, e.Suggestion)
			}
		}

		// b. Сценарии
		scenarios := scenario.Generate(fn)
		tests = append(tests, model.TestSpec{
			Func:      fn,
			Scenarios: scenarios,
		})
	}

	// Определяем имя пакета для сгенерированного файла.
	// PrimaryPackage теперь возвращает (*packages.Package, bool).
	pkg, ok := loaded.PrimaryPackage()
	pkgName := "main"
	if ok && pkg.Name != "" {
		pkgName = pkg.Name
	}

	// 4. Рендеринг
	fs := model.FileSpec{
		PackageName: pkgName,
		SourceDir:   loaded.Dir,
		Tests:       tests,
	}

	src, err := render.RenderFile(fs)
	if err != nil {
		// render может вернуть (нeформатированный src, error) — сохраняем для диагностики
		if src != nil {
			_ = os.WriteFile(outputPath(cfg), src, 0o644)
		}
		return fmt.Errorf("render: %w", err)
	}

	// 5. Запись
	outPath := outputPath(cfg)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(outPath, src, 0o644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	cfg.Logger.Printf("записан %s", outPath)

	// 6. Валидация
	if cfg.RunValidation {
		cfg.Logger.Printf("проверяем компиляцию…")
		if err := validator.Validate(outPath); err != nil {
			return err
		}
		cfg.Logger.Printf("компиляция прошла успешно")
	}

	return nil
}

// outputPath выводит путь выходного файла из cfg.
func outputPath(cfg Config) string {
	if cfg.OutputFile != "" {
		return cfg.OutputFile
	}

	info, err := os.Stat(cfg.Target)
	if err == nil && !info.IsDir() {
		// Одиночный файл: заменяем .go на _test.go
		base := strings.TrimSuffix(cfg.Target, ".go")
		return base + "_test.go"
	}

	// Директория: кладём сгенерированный файл внутрь.
	dir := cfg.Target
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "testgen_generated_test.go")
}
