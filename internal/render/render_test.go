package render_test

import (
	"strings"
	"testing"

	"github.com/yourorg/testgen/internal/model"
	"github.com/yourorg/testgen/internal/render"
	"github.com/yourorg/testgen/internal/scenario"
)

// emptyGuards возвращает Guards с инициализированными пустыми картами.
func emptyGuards() model.Guards {
	return model.Guards{
		NilCheckedParams:   make(map[string]bool),
		EmptyCheckedParams: make(map[string]bool),
	}
}

func TestRenderFile_basicOutput(t *testing.T) {
	fn := model.FunctionSpec{
		PackageName: "calc",
		Name:        "Add",
		Guards:      emptyGuards(),
		Params: []model.ParamSpec{
			{Name: "a", TypeStr: "int", Kind: model.KindInt},
			{Name: "b", TypeStr: "int", Kind: model.KindInt},
		},
		Results: []model.ParamSpec{
			{Name: "result0", TypeStr: "int", Kind: model.KindInt},
		},
	}

	fs := model.FileSpec{
		PackageName: "calc",
		Tests: []model.TestSpec{
			{Func: fn, Scenarios: scenario.Generate(fn)},
		},
	}

	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}

	out := string(src)
	if !strings.Contains(out, "func TestAdd(t *testing.T)") {
		t.Error("вывод не содержит функции TestAdd")
	}
	if !strings.Contains(out, `"testing"`) {
		t.Error("вывод не содержит импорт testing")
	}
	// MVP-поведение: НЕТ reflect.DeepEqual — мы не сравниваем got с placeholder-want.
	if strings.Contains(out, "reflect.DeepEqual") {
		t.Error("вывод НЕ должен содержать reflect.DeepEqual (placeholder-сравнения убраны)")
	}
	if strings.Contains(out, `"reflect"`) {
		t.Error("вывод НЕ должен импортировать reflect")
	}
	// Вместо assert — `_ = got0 // TODO`.
	if !strings.Contains(out, "_ = got0") {
		t.Error("вывод должен содержать `_ = got0` чтобы переменная считалась использованной")
	}
	if !strings.Contains(out, "TODO: verify got0") {
		t.Error("вывод должен содержать TODO-комментарий о проверке got0")
	}
	// want0-поле не должно появляться в struct.
	if strings.Contains(out, "want0") {
		t.Error("вывод не должен содержать want0 — placeholder-поля убраны")
	}
}

func TestRenderFile_withError(t *testing.T) {
	fn := model.FunctionSpec{
		PackageName: "pkg",
		Name:        "Divide",
		HasError:    true,
		Guards:      emptyGuards(),
		Params: []model.ParamSpec{
			{Name: "a", TypeStr: "float64", Kind: model.KindInt},
			{Name: "b", TypeStr: "float64", Kind: model.KindInt},
		},
		Results: []model.ParamSpec{
			{Name: "result0", TypeStr: "float64", Kind: model.KindInt},
			{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true},
		},
	}

	fs := model.FileSpec{
		PackageName: "pkg",
		Tests: []model.TestSpec{
			{Func: fn, Scenarios: scenario.Generate(fn)},
		},
	}

	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}

	out := string(src)
	if !strings.Contains(out, "wantErr") {
		t.Error("вывод не содержит поле wantErr")
	}
	if !strings.Contains(out, "tt.wantErr") {
		t.Error("вывод не содержит проверку tt.wantErr")
	}
}

func TestRenderFile_voidFunction(t *testing.T) {
	fn := model.FunctionSpec{
		PackageName: "pkg",
		Name:        "Log",
		Guards:      emptyGuards(),
		Params:      []model.ParamSpec{{Name: "msg", TypeStr: "string", Kind: model.KindString}},
		Results:     nil,
	}

	fs := model.FileSpec{
		PackageName: "pkg",
		Tests: []model.TestSpec{
			{Func: fn, Scenarios: scenario.Generate(fn)},
		},
	}

	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}

	// reflect не используется ни для каких функций (placeholder-сравнения убраны).
	if strings.Contains(string(src), `"reflect"`) {
		t.Error("сгенерированный файл не должен импортировать reflect (placeholder-сравнения убраны в MVP)")
	}
}

func TestRenderFile_importsContext(t *testing.T) {
	// Параметр context.Context → import "context" обязателен в test-файле.
	fn := model.FunctionSpec{
		PackageName: "pkg",
		Name:        "Run",
		HasError:    true,
		Guards:      emptyGuards(),
		Params: []model.ParamSpec{
			{Name: "ctx", TypeStr: "context.Context", Kind: model.KindInterface},
		},
		Results: []model.ParamSpec{
			{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true},
		},
	}

	fs := model.FileSpec{
		PackageName: "pkg",
		Tests:       []model.TestSpec{{Func: fn, Scenarios: scenario.Generate(fn)}},
	}

	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	if !strings.Contains(out, `"context"`) {
		t.Error("сгенерированный файл должен содержать import \"context\"")
	}
	// success-сценарий должен использовать context.Background(), а не nil.
	if !strings.Contains(out, "context.Background()") {
		t.Error("success-сценарий должен использовать context.Background() для context.Context")
	}
}

func TestRenderFile_ioReaderImportsStrings(t *testing.T) {
	// io.Reader → fixture strings.NewReader(...) → import "strings" обязателен.
	fn := model.FunctionSpec{
		PackageName: "pkg",
		Name:        "Read",
		HasError:    true,
		Guards:      emptyGuards(),
		Params: []model.ParamSpec{
			{Name: "r", TypeStr: "io.Reader", Kind: model.KindInterface},
		},
		Results: []model.ParamSpec{
			{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true},
		},
	}

	fs := model.FileSpec{
		PackageName: "pkg",
		Tests:       []model.TestSpec{{Func: fn, Scenarios: scenario.Generate(fn)}},
	}

	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	// fixture использует strings.NewReader → нужен import "strings"
	if !strings.Contains(out, `"strings"`) {
		t.Error("io.Reader fixture использует strings.NewReader → должен быть import \"strings\"")
	}
	// import "io" нужен для поля struct типа io.Reader
	if !strings.Contains(out, `"io"`) {
		t.Error("поле struct io.Reader требует import \"io\"")
	}
	// success-сценарий НЕ должен использовать nil для io.Reader
	if strings.Contains(out, "inputR: nil") {
		t.Error("success-сценарий не должен использовать nil для io.Reader")
	}
}

func TestRenderFile_ptrIntFixture(t *testing.T) {
	// *int должен генерировать new(int), а не &int{} (невалидный Go).
	fn := model.FunctionSpec{
		PackageName: "pkg",
		Name:        "Scale",
		HasError:    true,
		Guards:      emptyGuards(),
		Params: []model.ParamSpec{
			{Name: "p", TypeStr: "*int", Kind: model.KindPtr},
		},
		Results: []model.ParamSpec{
			{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true},
		},
	}

	fs := model.FileSpec{
		PackageName: "pkg",
		Tests:       []model.TestSpec{{Func: fn, Scenarios: scenario.Generate(fn)}},
	}

	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	if strings.Contains(out, "&int{}") {
		t.Error("сгенерированный файл содержит &int{} — невалидный Go")
	}
	if !strings.Contains(out, "new(int)") {
		t.Error("сгенерированный файл должен содержать new(int) для *int")
	}
}
