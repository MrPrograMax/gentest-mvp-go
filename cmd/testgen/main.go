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
//	-o string     записать вывод в этот файл (по умолчанию выводится из <путь>)
//	-validate     скомпилировать вывод через `go test -run ^$ .` после записи
//	-v            подробное логирование
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/yourorg/testgen/internal/app"
)

func main() {
	var (
		outputFile    = flag.String("o", "", "путь к выходному файлу (по умолчанию выводится из target)")
		runValidation = flag.Bool("validate", false, "запустить `go test -run ^$ .` для проверки компиляции")
		verbose       = flag.Bool("v", false, "подробное логирование")
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
		Logger:        logger,
	}

	if err := app.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "testgen: %v\n", err)
		os.Exit(1)
	}
}
