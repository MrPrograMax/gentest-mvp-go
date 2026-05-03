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

// ── Regression: FieldGuards из ValidateRegisterRequest ────────────────────────

func TestAnalyze_fieldGuards_structParam(t *testing.T) {
	// Минимальная версия ValidateRegisterRequest: 3 field-guard check.
	src := `package reg
import (
	"errors"
	"strings"
)
type Address struct { City string }
type RegisterRequest struct {
	Email   string
	Age     int
	Address Address
}
func ValidateRegisterRequest(req RegisterRequest) error {
	if req.Email == "" {
		return errors.New("email required")
	}
	if !strings.Contains(req.Email, "@") {
		return errors.New("invalid email")
	}
	if req.Age < 18 {
		return errors.New("underage")
	}
	if req.Address.City == "" {
		return errors.New("city required")
	}
	return nil
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
	fn := specs[0]

	// Должен быть ровно 1 параметр — req RegisterRequest — с Kind=KindStruct и StructFields.
	if len(fn.Params) != 1 {
		t.Fatalf("params count = %d, want 1", len(fn.Params))
	}
	req := fn.Params[0]
	if req.Kind != model.KindStruct {
		t.Errorf("req.Kind = %v, want KindStruct", req.Kind)
	}
	if len(req.StructFields) == 0 {
		t.Error("req.StructFields пустой — analyzer не извлёк поля RegisterRequest")
	}

	// Проверяем FieldGuards — 4 проверки в теле.
	guards := fn.Guards.FieldGuards
	if len(guards) != 4 {
		t.Fatalf("FieldGuards count = %d, want 4; guards=%+v", len(guards), guards)
	}

	checkGuard := func(idx int, paramName string, path []string, kind string) {
		t.Helper()
		g := guards[idx]
		if g.ParamName != paramName {
			t.Errorf("guards[%d].ParamName = %q, want %q", idx, g.ParamName, paramName)
		}
		if len(g.FieldPath) != len(path) {
			t.Errorf("guards[%d].FieldPath = %v, want %v", idx, g.FieldPath, path)
			return
		}
		for i, p := range path {
			if g.FieldPath[i] != p {
				t.Errorf("guards[%d].FieldPath[%d] = %q, want %q", idx, i, g.FieldPath[i], p)
			}
		}
		if string(g.Kind) != kind {
			t.Errorf("guards[%d].Kind = %q, want %q", idx, g.Kind, kind)
		}
	}

	checkGuard(0, "req", []string{"Email"}, "empty")
	checkGuard(1, "req", []string{"Email"}, "invalid")
	checkGuard(2, "req", []string{"Age"}, "less_than")
	checkGuard(3, "req", []string{"Address", "City"}, "empty")

	// Threshold для less_than.
	if guards[2].Threshold != "18" {
		t.Errorf("guards[2].Threshold = %q, want 18", guards[2].Threshold)
	}
}

func TestAnalyze_structFields_populated(t *testing.T) {
	// Проверяем что StructFields заполняются рекурсивно.
	src := `package reg
import "time"
type Address struct { City string; Street string }
type Req struct { Email string; Age int; Addr Address; CreatedAt time.Time }
func Validate(req Req) error { return nil }
`
	r := loadDir(t, src)
	specs, err := analyzer.Analyze(r)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("got %d specs, want 1", len(specs))
	}
	fields := specs[0].Params[0].StructFields
	if len(fields) == 0 {
		t.Fatal("StructFields пустой")
	}

	// Ищем поля верхнего уровня
	byName := make(map[string]model.StructField)
	for _, f := range fields {
		byName[f.Name] = f
	}
	if _, ok := byName["Email"]; !ok {
		t.Error("нет поля Email")
	}
	if _, ok := byName["Age"]; !ok {
		t.Error("нет поля Age")
	}
	addr, ok := byName["Addr"]
	if !ok {
		t.Fatal("нет поля Addr")
	}
	if addr.Kind != model.KindStruct {
		t.Errorf("Addr.Kind = %v, want KindStruct", addr.Kind)
	}
	// Рекурсивно должны быть SubFields Address.
	if len(addr.SubFields) == 0 {
		t.Error("Addr.SubFields пустой — рекурсивное извлечение не работает")
	}
}
