package analyzer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yourorg/testgen/internal/analyzer"
	"github.com/yourorg/testgen/internal/loader"
	"github.com/yourorg/testgen/internal/model"
)

// loadDir создаёт временный модуль с единственным .go файлом и загружает его.
func loadDir(t *testing.T, src string) *loader.Result {
	t.Helper()
	dir := t.TempDir()
	gomod := "module example.com/testpkg\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := loader.Load(dir)
	if err != nil {
		t.Fatalf("loader.Load: %v", err)
	}
	return r
}

func TestAnalyze_exportedOnly(t *testing.T) {
	src := `package calc
func Add(a, b int) int { return a + b }
func unexported() {}
`
	r := loadDir(t, src)
	specs, err := analyzer.Analyze(r)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("got %d specs, want 1", len(specs))
	}
	if specs[0].Name != "Add" {
		t.Errorf("Name = %q, want Add", specs[0].Name)
	}
}

func TestAnalyze_hasError(t *testing.T) {
	src := `package pkg
import "errors"
func Divide(a, b float64) (float64, error) {
	if b == 0 { return 0, errors.New("div zero") }
	return a / b, nil
}
`
	r := loadDir(t, src)
	specs, err := analyzer.Analyze(r)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("got %d specs, want 1", len(specs))
	}
	if !specs[0].HasError {
		t.Error("HasError должен быть true для функции, возвращающей error")
	}
}

func TestAnalyze_variadic(t *testing.T) {
	src := `package pkg
func Sum(nums ...int) int {
	total := 0
	for _, n := range nums { total += n }
	return total
}
`
	r := loadDir(t, src)
	specs, _ := analyzer.Analyze(r)
	if len(specs) != 1 {
		t.Fatalf("got %d specs", len(specs))
	}
	if !specs[0].IsVariadic {
		t.Error("IsVariadic должен быть true")
	}
	// Analyzer представляет variadic как []T.
	if specs[0].Params[0].TypeStr != "[]int" {
		t.Errorf("variadic TypeStr = %q, want []int", specs[0].Params[0].TypeStr)
	}
}

func TestAnalyze_funcTypeParam(t *testing.T) {
	src := `package pkg
func Apply(items []string, fn func(string) bool) []string { return nil }
`
	r := loadDir(t, src)
	specs, _ := analyzer.Analyze(r)
	if len(specs) != 1 {
		t.Fatalf("got %d specs", len(specs))
	}
	fnParam := specs[0].Params[1]
	// go/types должен дать KindFunc (а не KindInterface).
	if fnParam.Kind != model.KindFunc {
		t.Errorf("func param Kind = %v, want KindFunc", fnParam.Kind)
	}
	// TypeStr не должен быть "func(...)" — невалидный Go-тип.
	if fnParam.TypeStr == "func(...)" {
		t.Errorf("func param TypeStr невалиден: %q", fnParam.TypeStr)
	}
}

func TestAnalyze_kindClassification(t *testing.T) {
	src := `package pkg
import "time"
func Types(
	s string,
	n int,
	b bool,
	sl []string,
	m map[string]int,
	p *int,
	t time.Time,
	d time.Duration,
) {}
`
	r := loadDir(t, src)
	specs, _ := analyzer.Analyze(r)
	if len(specs) != 1 {
		t.Fatalf("got %d specs", len(specs))
	}
	params := specs[0].Params
	expected := []model.TypeKind{
		model.KindString,
		model.KindInt,
		model.KindBool,
		model.KindSlice,
		model.KindMap,
		model.KindPtr,
		model.KindTime,
		model.KindDuration,
	}
	for i, want := range expected {
		if params[i].Kind != want {
			t.Errorf("params[%d] (%s): Kind = %v, want %v", i, params[i].Name, params[i].Kind, want)
		}
	}
}

func TestAnalyze_guards_emptyCheck(t *testing.T) {
	src := `package pkg
import "errors"
func Greet(name string) (string, error) {
	if name == "" {
		return "", errors.New("name required")
	}
	return "Hello, " + name, nil
}
`
	r := loadDir(t, src)
	specs, _ := analyzer.Analyze(r)
	if len(specs) != 1 {
		t.Fatalf("got %d specs", len(specs))
	}
	g := specs[0].Guards
	if !g.EmptyCheckedParams["name"] {
		t.Error("Guards.EmptyCheckedParams[\"name\"] должен быть true")
	}
}

func TestAnalyze_guards_nilCheck(t *testing.T) {
	src := `package pkg
import "errors"
func Process(p *int) error {
	if p == nil {
		return errors.New("nil pointer")
	}
	return nil
}
`
	r := loadDir(t, src)
	specs, _ := analyzer.Analyze(r)
	if len(specs) != 1 {
		t.Fatalf("got %d specs", len(specs))
	}
	g := specs[0].Guards
	if !g.NilCheckedParams["p"] {
		t.Error("Guards.NilCheckedParams[\"p\"] должен быть true")
	}
}

func TestAnalyze_guards_errChecked(t *testing.T) {
	src := `package pkg
import "os"
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}
`
	r := loadDir(t, src)
	specs, _ := analyzer.Analyze(r)
	if len(specs) != 1 {
		t.Fatalf("got %d specs", len(specs))
	}
	if !specs[0].Guards.ErrChecked {
		t.Error("Guards.ErrChecked должен быть true")
	}
}

func TestAnalyze_guards_hasPanic(t *testing.T) {
	src := `package pkg
func MustPositive(n int) int {
	if n <= 0 {
		panic("must be positive")
	}
	return n
}
`
	r := loadDir(t, src)
	specs, _ := analyzer.Analyze(r)
	if len(specs) != 1 {
		t.Fatalf("got %d specs", len(specs))
	}
	if !specs[0].Guards.HasPanic {
		t.Error("Guards.HasPanic должен быть true")
	}
}
