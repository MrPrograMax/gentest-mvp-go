// Пакет loader загружает Go-пакет вместе с типовой информацией,
// используя golang.org/x/tools/go/packages.
//
// В отличие от go/parser, packages.Load запускает полный type-checker,
// что позволяет analyzer использовать go/types вместо строковой классификации.
package loader

import (
	"fmt"
	"go/token"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// loadMode — набор флагов, необходимых для анализа типов.
// NeedSyntax даёт AST, NeedTypes + NeedTypesInfo — type-checker.
const loadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedSyntax |
	packages.NeedTypes |
	packages.NeedTypesInfo

// Result хранит загруженные пакеты и метаданные.
type Result struct {
	Fset *token.FileSet
	// Pkgs — основные пакеты без _test-суффикса.
	Pkgs []*packages.Package
	// Dir — директория, из которой выполнялась загрузка.
	Dir string
}

// PrimaryPackage возвращает первый основной пакет (обычно он единственный).
func (r *Result) PrimaryPackage() (*packages.Package, bool) {
	if len(r.Pkgs) == 0 {
		return nil, false
	}
	return r.Pkgs[0], true
}

// Load загружает Go-пакет по указанному пути.
//
// path может быть:
//   - директорией:      "./mypkg"  "/home/user/mypkg"
//   - одиночным .go файлом: "./mypkg/foo.go"
//     (в этом случае загружается вся директория)
func Load(path string) (*Result, error) {
	dir := path
	if strings.HasSuffix(path, ".go") {
		dir = filepath.Dir(path)
		if dir == "" {
			dir = "."
		}
	}

	// Преобразуем в абсолютный путь, чтобы go/packages мог найти go.mod.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("loader: абсолютный путь для %q: %w", dir, err)
	}

	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: loadMode,
		Fset: fset,
		Dir:  absDir,
		// Тесты не загружаем — нам нужен только продакшн-код.
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("loader: packages.Load %q: %w", absDir, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("loader: Go-файлы не найдены в %q", absDir)
	}

	hasGoFiles := false

	for _, pkg := range pkgs {
		if len(pkg.GoFiles) > 0 || len(pkg.CompiledGoFiles) > 0 {
			hasGoFiles = true
			break
		}
	}

	if !hasGoFiles {
		return nil, fmt.Errorf("no Go files found in %s", path)
	}

	// Собираем ошибки загрузки (синтаксические / типовые).
	// Не фатальны — анализ продолжается с тем, что удалось загрузить.
	var loadErrs []string
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			loadErrs = append(loadErrs, e.Error())
		}
	}

	// Фильтруем _test-пакеты.
	var main []*packages.Package
	for _, pkg := range pkgs {
		if !strings.HasSuffix(pkg.Name, "_test") {
			main = append(main, pkg)
		}
	}
	if len(main) == 0 {
		return nil, fmt.Errorf("loader: основной пакет не найден в %q (только _test?)", absDir)
	}

	if len(loadErrs) > 0 {
		// Возвращаем ошибки как предупреждения через отдельную структуру,
		// но не блокируем работу — частичная типовая информация лучше, чем ничего.
		_ = loadErrs // TODO: передавать предупреждения через Result
	}

	return &Result{Fset: fset, Pkgs: main, Dir: absDir}, nil
}
