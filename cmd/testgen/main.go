// Команда testgen генерирует table-driven unit-тесты для экспортируемых Go-функций.
//
// Использование:
//
//	testgen [флаги] <путь>
//
// <путь> — директория пакета или одиночный .go файл.
//
// Флаги:
//
//	-o string         записать вывод в этот файл (по умолчанию выводится из <путь>)
//	-validate         скомпилировать вывод через `go test -run ^$ .` после записи
//	-v                подробное логирование
//	--mock=MODE       стратегия моков: none|minimock (по умолчанию none)
//	--fixture=MODE    стратегия фикстур: heuristic|llm|hybrid (по умолчанию heuristic)
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/yourorg/testgen/internal/app"
	"github.com/yourorg/testgen/internal/model"
)

func main() {
	var (
		outputFile    = flag.String("o", "", "путь к выходному файлу (по умолчанию выводится из target)")
		runValidation = flag.Bool("validate", false, "запустить `go test -run ^$ .` для проверки компиляции")
		verbose       = flag.Bool("v", false, "подробное логирование")
		mockMode      = flag.String("mock", "none", "стратегия моков: none|minimock")
		fixtureMode   = flag.String("fixture", "heuristic", "стратегия фикстур: heuristic|llm|hybrid")
	)
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Использование: testgen [флаги] <директория-пакета|файл.go>")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	// Валидируем --mock.
	var mode model.MockMode
	switch *mockMode {
	case "none", "":
		mode = model.MockNone
	case "minimock":
		mode = model.MockMinimock
	default:
		fmt.Fprintf(os.Stderr, "testgen: неизвестное значение --mock=%q (допустимо: none|minimock)\n", *mockMode)
		os.Exit(1)
	}

	// Валидируем --fixture.
	// Неизвестные значения отклоняются здесь; llm/hybrid отклоняются в app.Run.
	var fMode model.FixtureMode
	switch *fixtureMode {
	case "heuristic", "":
		fMode = model.FixtureHeuristic
	case "llm":
		fMode = model.FixtureLLM
	case "hybrid":
		fMode = model.FixtureHybrid
	default:
		fmt.Fprintf(os.Stderr, "testgen: неизвестное значение --fixture=%q (допустимо: heuristic|llm|hybrid)\n", *fixtureMode)
		os.Exit(1)
	}

	target := flag.Arg(0)

	var logger *log.Logger
	if *verbose {
		logger = log.New(os.Stderr, "[testgen] ", 0)
	} else {
		logger = log.New(io.Discard, "", 0)
	}

	cfg := app.Config{
		Target:        target,
		OutputFile:    *outputFile,
		RunValidation: *runValidation,
		MockMode:      mode,
		FixtureMode:   fMode,
		Logger:        logger,
	}

	if err := app.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "testgen: %v\n", err)
		os.Exit(1)
	}
}
