package fixture_test

import (
	"strings"
	"testing"

	"github.com/yourorg/testgen/internal/fixture"
	"github.com/yourorg/testgen/internal/model"
)

// registrationFields возвращает StructField-список аналогичный RegisterRequest.
func registrationFields() []model.StructField {
	return []model.StructField{
		{Name: "Email", TypeStr: "string", Kind: model.KindString},
		{Name: "Name", TypeStr: "string", Kind: model.KindString},
		{Name: "Age", TypeStr: "int", Kind: model.KindInt},
		{Name: "Phone", TypeStr: "string", Kind: model.KindString},
		{
			Name:    "Address",
			TypeStr: "Address",
			Kind:    model.KindStruct,
			SubFields: []model.StructField{
				{Name: "City", TypeStr: "string", Kind: model.KindString},
				{Name: "Street", TypeStr: "string", Kind: model.KindString},
				{Name: "House", TypeStr: "string", Kind: model.KindString},
			},
		},
		{Name: "CreatedAt", TypeStr: "time.Time", Kind: model.KindTime},
	}
}

// ── HappyStructExpr ───────────────────────────────────────────────────────────

func TestHappyStructExpr_noFields(t *testing.T) {
	// Без полей — возвращает TypeName{}.
	got := fixture.HappyStructExpr("MyStruct", nil)
	if got != "MyStruct{}" {
		t.Errorf("HappyStructExpr(noFields) = %q, want MyStruct{}", got)
	}
}

func TestHappyStructExpr_emailSemantic(t *testing.T) {
	// Email → "user@example.com" (содержит @, не пустой).
	expr := fixture.HappyStructExpr("RegisterRequest", registrationFields())
	if !strings.Contains(expr, `"user@example.com"`) {
		t.Errorf("HappyStructExpr: Email должен быть user@example.com, expr:\n%s", expr)
	}
}

func TestHappyStructExpr_nameSemantic(t *testing.T) {
	expr := fixture.HappyStructExpr("RegisterRequest", registrationFields())
	if !strings.Contains(expr, `"Test User"`) {
		t.Errorf("HappyStructExpr: Name должен быть 'Test User', expr:\n%s", expr)
	}
}

func TestHappyStructExpr_ageSemantic(t *testing.T) {
	// Age → 25 (>= 18, проходит guard age < 18).
	expr := fixture.HappyStructExpr("RegisterRequest", registrationFields())
	if !strings.Contains(expr, "Age: 25") {
		t.Errorf("HappyStructExpr: Age должен быть 25, expr:\n%s", expr)
	}
}

func TestHappyStructExpr_citySemantic(t *testing.T) {
	expr := fixture.HappyStructExpr("RegisterRequest", registrationFields())
	if !strings.Contains(expr, `"Moscow"`) {
		t.Errorf("HappyStructExpr: City должен быть Moscow, expr:\n%s", expr)
	}
}

func TestHappyStructExpr_containsTime(t *testing.T) {
	// time.Time поле → time.Now() в выражении.
	expr := fixture.HappyStructExpr("RegisterRequest", registrationFields())
	if !strings.Contains(expr, "time.Now()") {
		t.Errorf("HappyStructExpr: CreatedAt должен использовать time.Now(), expr:\n%s", expr)
	}
}

func TestHappyStructExpr_nestedAddress(t *testing.T) {
	// Address — вложенная структура, должна быть раскрыта.
	expr := fixture.HappyStructExpr("RegisterRequest", registrationFields())
	if !strings.Contains(expr, "Address{") {
		t.Errorf("HappyStructExpr: Address должен быть composite literal, expr:\n%s", expr)
	}
}

func TestHappyStructExpr_notEmpty(t *testing.T) {
	// success-фикстура НЕ должна быть RegisterRequest{} (пустым литералом).
	expr := fixture.HappyStructExpr("RegisterRequest", registrationFields())
	if expr == "RegisterRequest{}" {
		t.Error("HappyStructExpr вернул пустой RegisterRequest{} — ожидался полный composite literal")
	}
}

// ── PatchedStructExpr ─────────────────────────────────────────────────────────

func TestPatchedStructExpr_emptyEmail(t *testing.T) {
	expr := fixture.PatchedStructExpr("RegisterRequest", registrationFields(), []string{"Email"}, `""`)
	if !strings.Contains(expr, `Email: ""`) {
		t.Errorf("PatchedStructExpr(Email=empty): ожидался Email: \"\", expr:\n%s", expr)
	}
	// Остальные поля остаются happy.
	if !strings.Contains(expr, `"Test User"`) {
		t.Errorf("PatchedStructExpr(Email=empty): Name должен остаться happy, expr:\n%s", expr)
	}
}

func TestPatchedStructExpr_invalidEmail(t *testing.T) {
	expr := fixture.PatchedStructExpr("RegisterRequest", registrationFields(), []string{"Email"}, `"invalid-email"`)
	if !strings.Contains(expr, `Email: "invalid-email"`) {
		t.Errorf("PatchedStructExpr(Email=invalid): ожидался Email: \"invalid-email\", expr:\n%s", expr)
	}
}

func TestPatchedStructExpr_underage(t *testing.T) {
	expr := fixture.PatchedStructExpr("RegisterRequest", registrationFields(), []string{"Age"}, "17")
	if !strings.Contains(expr, "Age: 17") {
		t.Errorf("PatchedStructExpr(Age=17): ожидался Age: 17, expr:\n%s", expr)
	}
	// Email остаётся happy.
	if !strings.Contains(expr, `"user@example.com"`) {
		t.Errorf("PatchedStructExpr(Age=17): Email должен остаться happy, expr:\n%s", expr)
	}
}

func TestPatchedStructExpr_nestedCityEmpty(t *testing.T) {
	expr := fixture.PatchedStructExpr("RegisterRequest", registrationFields(), []string{"Address", "City"}, `""`)
	if !strings.Contains(expr, `City: ""`) {
		t.Errorf("PatchedStructExpr(Address.City=empty): ожидался City: \"\", expr:\n%s", expr)
	}
	// Address должен быть раскрыт, Street и House — happy.
	if !strings.Contains(expr, `"Tverskaya"`) {
		t.Errorf("PatchedStructExpr(Address.City=empty): Street должен остаться Tverskaya, expr:\n%s", expr)
	}
}

// ── stripPkg: qualified TypeStr из того же пакета ─────────────────────────────

// registrationFieldsQualified имитирует реальные StructFields из analyzer:
// вложенный тип Address идёт с квалификатором пакета "registration.Address",
// как возвращает types.TypeString(t, func(p *types.Package) string { return p.Name() }).
func registrationFieldsQualified() []model.StructField {
	return []model.StructField{
		{Name: "Email", TypeStr: "string", Kind: model.KindString},
		{Name: "Name", TypeStr: "string", Kind: model.KindString},
		{Name: "Age", TypeStr: "int", Kind: model.KindInt},
		{
			Name:    "Address",
			TypeStr: "registration.Address", // ← квалифицированный, как из go/types
			Kind:    model.KindStruct,
			SubFields: []model.StructField{
				{Name: "City", TypeStr: "string", Kind: model.KindString},
				{Name: "Street", TypeStr: "string", Kind: model.KindString},
			},
		},
	}
}

func TestHappyStructExpr_samePackage_noQualifier(t *testing.T) {
	// Тест в package registration: "registration.Address" должно стать "Address{...}"
	expr := fixture.HappyStructExpr("RegisterRequest", registrationFieldsQualified(), "registration")
	if strings.Contains(expr, "registration.Address") {
		t.Errorf("same-package: не должен содержать registration.Address:\n%s", expr)
	}
	if !strings.Contains(expr, "Address{") {
		t.Errorf("same-package: должен содержать Address{...}:\n%s", expr)
	}
}

func TestHappyStructExpr_externalPackage_keepsQualifier(t *testing.T) {
	// Без stripPkg (или другой пакет): квалификатор остаётся.
	expr := fixture.HappyStructExpr("RegisterRequest", registrationFieldsQualified())
	if !strings.Contains(expr, "registration.Address{") {
		t.Errorf("external-package: должен сохранить registration.Address{...}:\n%s", expr)
	}
}

func TestPatchedStructExpr_samePackage_noQualifier(t *testing.T) {
	expr := fixture.PatchedStructExpr(
		"RegisterRequest", registrationFieldsQualified(),
		[]string{"Address", "City"}, `""`,
		"registration",
	)
	if strings.Contains(expr, "registration.Address") {
		t.Errorf("same-package patched: не должен содержать registration.Address:\n%s", expr)
	}
	if !strings.Contains(expr, "Address{") {
		t.Errorf("same-package patched: должен содержать Address{...}:\n%s", expr)
	}
	if !strings.Contains(expr, `City: ""`) {
		t.Errorf("same-package patched: City должен быть пустой строкой:\n%s", expr)
	}
}
