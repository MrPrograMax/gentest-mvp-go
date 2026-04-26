package fixture_test

import (
	"strings"
	"testing"

	"github.com/yourorg/testgen/internal/fixture"
	"github.com/yourorg/testgen/internal/model"
)

// ── Happy ────────────────────────────────────────────────────────────────────

func TestHappy_nonEmpty(t *testing.T) {
	// Проверяем, что все «стандартные» типы дают непустое и не-nil выражение.
	cases := []struct {
		kind    model.TypeKind
		typeStr string
	}{
		{model.KindString, "string"},
		{model.KindInt, "int"},
		{model.KindBool, "bool"},
		{model.KindSlice, "[]string"},
		{model.KindMap, "map[string]int"},
		{model.KindTime, "time.Time"},
		{model.KindDuration, "time.Duration"},
		{model.KindStruct, "MyStruct"},
		{model.KindPtr, "*MyStruct"},
	}
	for _, c := range cases {
		fv := fixture.Happy(c.kind, c.typeStr)
		if fv.Expr == "" {
			t.Errorf("Happy(%v, %q).Expr пусто", c.kind, c.typeStr)
		}
		if fv.Expr == "nil" {
			t.Errorf("Happy(%v, %q).Expr = nil, ожидалось ненулевое", c.kind, c.typeStr)
		}
	}
}

func TestHappy_sliceNonEmpty(t *testing.T) {
	// []string должен быть []string{"test-value"}, не []string{} и не nil.
	fv := fixture.Happy(model.KindSlice, "[]string")
	if fv.Expr == "nil" || fv.Expr == "[]string{}" {
		t.Errorf("Happy(KindSlice) должен быть непустым срезом, got %q", fv.Expr)
	}
	if !strings.HasPrefix(fv.Expr, "[]string{") {
		t.Errorf("Happy(KindSlice) = %q, ожидалось []string{...}", fv.Expr)
	}
}

func TestHappy_ptrNonNil(t *testing.T) {
	// *MyStruct должен быть new(MyStruct), не nil и не &MyStruct{}.
	fv := fixture.Happy(model.KindPtr, "*MyStruct")
	if fv.Expr == "nil" {
		t.Errorf("Happy(KindPtr) = nil, ожидалось new(MyStruct)")
	}
	if fv.Expr != "new(MyStruct)" {
		t.Errorf("Happy(KindPtr, *MyStruct) = %q, want new(MyStruct)", fv.Expr)
	}
}

func TestHappy_ptrBuiltin(t *testing.T) {
	// *int → new(int): &int{} невалидный Go — int не структура.
	fv := fixture.Happy(model.KindPtr, "*int")
	if fv.Expr == "&int{}" {
		t.Error("Happy(KindPtr, *int) = &int{} — невалидный Go, ожидалось new(int)")
	}
	if fv.Expr != "new(int)" {
		t.Errorf("Happy(KindPtr, *int) = %q, want new(int)", fv.Expr)
	}
}

// ── KindFunc — безопасные заглушки ───────────────────────────────────────────

func TestHappy_funcStub_noPanic(t *testing.T) {
	// Заглушка больше не должна содержать panic — это небезопасно для тестов.
	fv := fixture.Happy(model.KindFunc, "func(string) bool")
	if strings.Contains(fv.Expr, "panic") {
		t.Errorf("Happy(KindFunc) содержит panic: %q", fv.Expr)
	}
	if fv.Expr == "nil" {
		t.Error("Happy(KindFunc) = nil, ожидалась func-заглушка")
	}
}

func TestHappy_funcStub_boolReturn(t *testing.T) {
	// func(string) bool → заглушка возвращает false.
	fv := fixture.Happy(model.KindFunc, "func(string) bool")
	if !strings.Contains(fv.Expr, "return false") {
		t.Errorf("func(string) bool stub = %q, ожидалось 'return false'", fv.Expr)
	}
}

func TestHappy_funcStub_errorReturn(t *testing.T) {
	// func(string) error → заглушка возвращает nil.
	fv := fixture.Happy(model.KindFunc, "func(string) error")
	if !strings.Contains(fv.Expr, "return nil") {
		t.Errorf("func(string) error stub = %q, ожидалось 'return nil'", fv.Expr)
	}
}

func TestHappy_funcStub_void(t *testing.T) {
	// func() → тело пустое {}, никакого return.
	fv := fixture.Happy(model.KindFunc, "func()")
	if strings.Contains(fv.Expr, "return") {
		t.Errorf("func() stub не должен содержать return: %q", fv.Expr)
	}
	if !strings.Contains(fv.Expr, "{}") {
		t.Errorf("func() stub = %q, ожидалось пустое тело {}", fv.Expr)
	}
}

func TestHappy_funcStub_multiReturn(t *testing.T) {
	// func(int, int) (int, error) → return *new(int), nil
	fv := fixture.Happy(model.KindFunc, "func(int, int) (int, error)")
	if !strings.Contains(fv.Expr, "return") {
		t.Errorf("многозначный stub должен содержать return: %q", fv.Expr)
	}
	// nil — для error
	if !strings.Contains(fv.Expr, "nil") {
		t.Errorf("многозначный stub должен содержать nil для error: %q", fv.Expr)
	}
}

func TestHappy_funcStub_validPrefix(t *testing.T) {
	// Выражение должно начинаться с typeStr — быть валидным func-литералом.
	typeStr := "func(string) bool"
	fv := fixture.Happy(model.KindFunc, typeStr)
	if !strings.HasPrefix(fv.Expr, typeStr) {
		t.Errorf("stub должен начинаться с %q, got %q", typeStr, fv.Expr)
	}
}

// ── KindInterface — mock comment ─────────────────────────────────────────────

func TestHappy_interfaceMockComment(t *testing.T) {
	// interface{} в success-сценарии: Expr = nil, NeedsMockComment = true.
	fv := fixture.Happy(model.KindInterface, "interface{}")
	if fv.Expr != "nil" {
		t.Errorf("Happy(KindInterface).Expr = %q, want nil", fv.Expr)
	}
	if !fv.NeedsMockComment {
		t.Error("Happy(KindInterface).NeedsMockComment должен быть true")
	}
}

func TestHappy_namedInterfaceMockComment(t *testing.T) {
	// Произвольный именованный интерфейс (не context.Context, не io.Reader)
	// получает nil + NeedsMockComment.
	fv := fixture.Happy(model.KindInterface, "MyCustomInterface")
	if !fv.NeedsMockComment {
		t.Errorf("Happy(KindInterface, MyCustomInterface).NeedsMockComment должен быть true")
	}
	if fv.Expr != "nil" {
		t.Errorf("Happy(KindInterface, MyCustomInterface).Expr = %q, want nil", fv.Expr)
	}
}

// ── Конкретные фикстуры для хорошо известных интерфейсов ─────────────────────

func TestHappy_contextContext(t *testing.T) {
	// context.Context → context.Background() (не nil, не требует мока).
	fv := fixture.Happy(model.KindInterface, "context.Context")
	if fv.Expr != "context.Background()" {
		t.Errorf("Happy(context.Context) = %q, want context.Background()", fv.Expr)
	}
	// context.Background() — конкретное значение, не нужен TODO-комментарий.
	if fv.NeedsMockComment {
		t.Error("Happy(context.Context).NeedsMockComment должен быть false")
	}
}

func TestHappy_ioReader(t *testing.T) {
	// io.Reader → strings.NewReader("test-value") (читаемый поток без мока).
	fv := fixture.Happy(model.KindInterface, "io.Reader")
	want := `strings.NewReader("test-value")`
	if fv.Expr != want {
		t.Errorf("Happy(io.Reader) = %q, want %q", fv.Expr, want)
	}
	// Конкретное значение — мок не нужен.
	if fv.NeedsMockComment {
		t.Error("Happy(io.Reader).NeedsMockComment должен быть false")
	}
}

func TestHappy_errorNoMockComment(t *testing.T) {
	// error в success-сценарии — nil без mock-комментария.
	fv := fixture.Happy(model.KindError, "error")
	if fv.Expr != "nil" {
		t.Errorf("Happy(KindError).Expr = %q, want nil", fv.Expr)
	}
	if fv.NeedsMockComment {
		t.Error("Happy(KindError).NeedsMockComment должен быть false")
	}
}

// ── Zero ─────────────────────────────────────────────────────────────────────

func TestZero_values(t *testing.T) {
	cases := []struct {
		kind    model.TypeKind
		typeStr string
		want    string
	}{
		{model.KindString, "string", `""`},
		{model.KindInt, "int", "0"},
		{model.KindBool, "bool", "false"},
		{model.KindSlice, "[]string", "nil"},     // Zero для среза — nil
		{model.KindMap, "map[string]int", "nil"}, // Zero для map — nil
		{model.KindPtr, "*int", "nil"},
		{model.KindFunc, "func()", "nil"},
		{model.KindInterface, "interface{}", "nil"},
	}
	for _, c := range cases {
		fv := fixture.Zero(c.kind, c.typeStr)
		if fv.Expr != c.want {
			t.Errorf("Zero(%v, %q) = %q, want %q", c.kind, c.typeStr, fv.Expr, c.want)
		}
	}
}

// ── Empty ─────────────────────────────────────────────────────────────────────

func TestEmpty_sliceVsZero(t *testing.T) {
	// Empty для среза — явно пустой []T{}, а не nil.
	zero := fixture.Zero(model.KindSlice, "[]string")
	empty := fixture.Empty(model.KindSlice, "[]string")
	if zero.Expr != "nil" {
		t.Errorf("Zero(slice) = %q, want nil", zero.Expr)
	}
	if empty.Expr != "[]string{}" {
		t.Errorf("Empty(slice) = %q, want []string{}", empty.Expr)
	}
	if zero.Expr == empty.Expr {
		t.Error("Zero и Empty для среза не должны совпадать")
	}
}

func TestEmpty_mapVsZero(t *testing.T) {
	// Empty для map — явно пустой map[K]V{}, а не nil.
	zero := fixture.Zero(model.KindMap, "map[string]int")
	empty := fixture.Empty(model.KindMap, "map[string]int")
	if zero.Expr != "nil" {
		t.Errorf("Zero(map) = %q, want nil", zero.Expr)
	}
	if empty.Expr != "map[string]int{}" {
		t.Errorf("Empty(map) = %q, want map[string]int{}", empty.Expr)
	}
	if zero.Expr == empty.Expr {
		t.Error("Zero и Empty для map не должны совпадать")
	}
}

func TestEmpty_string(t *testing.T) {
	fv := fixture.Empty(model.KindString, "string")
	if fv.Expr != `""` {
		t.Errorf("Empty(string) = %q, want %q", fv.Expr, `""`)
	}
}

// ── NeedsTimeImport ───────────────────────────────────────────────────────────

func TestNeedsTimeImport(t *testing.T) {
	timeFixtures := []model.FixtureValue{
		{Expr: "time.Now()", TypeStr: "time.Time"},
	}
	if !fixture.NeedsTimeImport(timeFixtures...) {
		t.Error("NeedsTimeImport: ожидался true для time.Now()")
	}

	noTime := []model.FixtureValue{
		{Expr: `"hello"`, TypeStr: "string"},
	}
	if fixture.NeedsTimeImport(noTime...) {
		t.Error("NeedsTimeImport: ожидался false для строки")
	}
}
