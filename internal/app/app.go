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
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/yourorg/testgen/internal/analyzer"
	"github.com/yourorg/testgen/internal/fixture"
	"github.com/yourorg/testgen/internal/llm"
	"github.com/yourorg/testgen/internal/loader"
	"github.com/yourorg/testgen/internal/mockgen"
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

	// MockMode задаёт стратегию подготовки моков для методов структур.
	// MockNone — никаких моков (по умолчанию).
	// MockMinimock — render готовит инфраструктуру под gojuno/minimock.
	MockMode model.MockMode

	// FixtureMode задаёт стратегию генерации тестовых фикстур.
	// FixtureHeuristic — детерминированные правила (по умолчанию).
	// FixtureLLM / FixtureHybrid — не реализованы, вернут ошибку.
	FixtureMode model.FixtureMode

	// LLMProvider — идентификатор LLM-бэкенда, например "ollama".
	// Используется только при FixtureMode == FixtureLLM.
	LLMProvider string

	// LLMModel — имя модели внутри провайдера, например "llama3".
	// Используется только при FixtureMode == FixtureLLM.
	LLMModel string

	// LLMDryRun — если true и FixtureMode == FixtureLLM:
	// вместо реального вызова LLM выводит JSON-payload в stdout и завершается.
	// Позволяет инспектировать запрос до его отправки.
	LLMDryRun bool

	// Logger — получает сообщения о прогрессе и предупреждения.
	Logger *log.Logger
}

// Run выполняет полный пайплайн testgen для cfg.Target.
func Run(cfg Config) error {
	if cfg.Logger == nil {
		cfg.Logger = log.New(os.Stderr, "[testgen] ", 0)
	}
	if cfg.MockMode == "" {
		cfg.MockMode = model.MockNone
	}
	if cfg.FixtureMode == "" {
		cfg.FixtureMode = model.FixtureHeuristic
	}

	// Проверяем fixture mode сразу: fail-fast до любой дорогостоящей работы.
	// Для llm/hybrid возвращаем ошибку "not implemented" — пользователь получит
	// понятное сообщение, а не упадёт где-то в середине пайплайна.
	//
	// Исключение: --fixture=llm --llm-dry-run работает без реального провайдера —
	// он только строит JSON-payload и выводит в stdout.
	//
	// TODO: pass selected fixture.Provider into scenario/fixture planning
	// when llm/hybrid modes are implemented.
	// Сейчас NewProvider вызывается только для валидации; heuristic-поведение
	// обеспечивается пакетными функциями fixture.Happy/Zero/Empty напрямую.
	if !(cfg.FixtureMode == model.FixtureLLM && cfg.LLMDryRun) {
		if _, err := fixture.NewProvider(cfg.FixtureMode); err != nil {
			return fmt.Errorf("fixture mode %q: %w", cfg.FixtureMode, err)
		}
	}
	cfg.Logger.Printf("fixture mode: %s", cfg.FixtureMode)

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
	for i := range specs {
		fn := &specs[i]

		// a. Legacy: диагностика interface-параметров (только при -v).
		plan := mockplan.Analyze(*fn)
		if plan.HasMocks() {
			for _, e := range plan.Entries {
				cfg.Logger.Printf("  %s: параметр %q (%s) — интерфейс: %s",
					fn.Name, e.ParamName, e.TypeStr, e.Suggestion)
			}
		}

		// a'. Новый mockplan: для методов с интерфейс-полями receiver-структуры.
		// Заполняем только когда MockMode != none — иначе зашумлять FunctionSpec не нужно.
		if cfg.MockMode == model.MockMinimock {
			fn.MockPlan = mockplan.AnalyzeReceiver(*fn)
			if fn.MockPlan.HasMocks() {
				for _, m := range fn.MockPlan.Mocks {
					cfg.Logger.Printf("  %s: receiver-поле %q (%s) → mock %s",
						fn.Name, m.FieldName, m.InterfaceName, m.MockType)
				}
			}
		}

		// b. Сценарии
		scenarios := scenario.Generate(*fn)
		tests = append(tests, model.TestSpec{
			Func:      *fn,
			Scenarios: scenarios,
		})
	}

	// Определяем имя пакета для сгенерированного файла.
	pkg, ok := loaded.PrimaryPackage()
	pkgName := "main"
	if ok && pkg.Name != "" {
		pkgName = pkg.Name
	}

	// 3.5-llm. Dry-run: если --fixture=llm --llm-dry-run, выводим JSON-payload в stdout и выходим.
	// Реальный HTTP-клиент Ollama не подключён — только инспекция payload.
	if cfg.FixtureMode == model.FixtureLLM && cfg.LLMDryRun {
		return runLLMDryRun(cfg, tests)
	}

	// 3.5. Генерируем mock-файлы (только при MockMinimock).
	// Запускается ПОСЛЕ построения всех MockPlan, ДО рендеринга тестов.
	// Моки размещаются в поддиректории mock/ анализируемого пакета.
	if cfg.MockMode == model.MockMinimock {
		generated := map[string]bool{} // ключ = MockFilePath, чтобы не дублировать
		for _, ts := range tests {
			for _, m := range ts.Func.MockPlan.Mocks {
				if m.MockFilePath == "" || generated[m.MockFilePath] {
					continue
				}
				generated[m.MockFilePath] = true
				if err := mockgen.Generate(m, cfg.Logger); err != nil {
					return fmt.Errorf("mockgen: %w", err)
				}
			}
		}
	}

	// 4. Рендеринг
	fs := model.FileSpec{
		PackageName:       pkgName,
		PackageImportPath: pkg.PkgPath,
		SourceDir:         loaded.Dir,
		Tests:             tests,
		MockMode:          cfg.MockMode,
	}

	src, err := render.RenderFile(fs)
	if err != nil {
		// render может вернуть (нeформатированный src, error) — сохраняем для диагностики
		if src != nil {
			outPath := resolveOutputPath(cfg, pkgName)
			_ = os.WriteFile(outPath, src, 0o644)
		}
		return fmt.Errorf("render: %w", err)
	}

	// 5. Запись
	outPath := resolveOutputPath(cfg, pkgName)
	if err := checkOverwrite(outPath); err != nil {
		return err
	}
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

// runLLMDryRun выводит JSON-payload для каждого TestSpec в stdout и завершается.
//
// Используется с --fixture=llm --llm-dry-run:
// позволяет инспектировать, что именно будет отправлено в LLM,
// до реального HTTP-вызова.
func runLLMDryRun(cfg Config, tests []model.TestSpec) error {
	cfg.Logger.Printf("dry-run: вывод LLM payload в stdout")
	cfg.Logger.Printf("provider: %s, model: %s", cfg.LLMProvider, cfg.LLMModel)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	for _, ts := range tests {
		req := llm.BuildFixtureRequest(ts.Func, ts.Scenarios)
		if err := enc.Encode(req); err != nil {
			return fmt.Errorf("llm dry-run encode: %w", err)
		}
	}
	return nil
}
