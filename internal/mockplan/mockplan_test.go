package mockplan_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourorg/testgen/internal/mockplan"
	"github.com/yourorg/testgen/internal/model"
)

// ── ToSnakeCase ───────────────────────────────────────────────────────────────

func TestToSnakeCase_simple(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"UserRepository", "user_repository"},
		{"HTTPClient", "http_client"},
		{"BKITransport", "bki_transport"},
		{"Service", "service"},
		{"MyInterface", "my_interface"},
		{"XMLParser", "xml_parser"},
		{"PDFConverter", "pdf_converter"},
	}
	for _, c := range cases {
		got := mockplan.ToSnakeCase(c.input)
		if got != c.want {
			t.Errorf("ToSnakeCase(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestToSnakeCase_noDotsOrSpaces(t *testing.T) {
	// Результат никогда не содержит точек и пробелов.
	inputs := []string{"UserRepository", "HTTPClient", "BKITransport", "XMLParser"}
	for _, s := range inputs {
		got := mockplan.ToSnakeCase(s)
		if strings.ContainsAny(got, ". ") {
			t.Errorf("ToSnakeCase(%q) = %q содержит точку или пробел", s, got)
		}
	}
}

// ── makeMethod helper ─────────────────────────────────────────────────────────

func makeMethod(name string, fields []model.ReceiverField) model.FunctionSpec {
	return model.FunctionSpec{
		Name:              name,
		IsMethod:          true,
		ReceiverType:      "*Service",
		PackageImportPath: "github.com/yourorg/testgen/example/service",
		PackagePath:       "/abs/example/service",
		ReceiverFields:    fields,
	}
}

// ── AnalyzeReceiver ───────────────────────────────────────────────────────────

func TestAnalyzeReceiver_emptyForNonMethod(t *testing.T) {
	fn := model.FunctionSpec{Name: "Helper", IsMethod: false}
	plan := mockplan.AnalyzeReceiver(fn)
	if plan.HasMocks() {
		t.Error("AnalyzeReceiver не должен возвращать моки для функции без receiver")
	}
}

func TestAnalyzeReceiver_emptyWhenNoInterfaceFields(t *testing.T) {
	fn := makeMethod("Total", []model.ReceiverField{
		{Name: "value", TypeStr: "int", IsInterface: false},
	})
	plan := mockplan.AnalyzeReceiver(fn)
	if plan.HasMocks() {
		t.Errorf("ожидался пустой план, got %d mocks", len(plan.Mocks))
	}
}

func TestAnalyzeReceiver_basicFields(t *testing.T) {
	// Базовые поля MockSpec для repo UserRepository.
	fn := makeMethod("GetUserName", []model.ReceiverField{
		{Name: "repo", TypeStr: "UserRepository", IsInterface: true},
	})
	plan := mockplan.AnalyzeReceiver(fn)
	if len(plan.Mocks) != 1 {
		t.Fatalf("ожидался 1 мок, got %d", len(plan.Mocks))
	}
	m := plan.Mocks[0]

	if m.FieldName != "repo" {
		t.Errorf("FieldName = %q, want repo", m.FieldName)
	}
	if m.InterfaceName != "UserRepository" {
		t.Errorf("InterfaceName = %q, want UserRepository", m.InterfaceName)
	}
	if m.MockType != "UserRepositoryMock" {
		t.Errorf("MockType = %q, want UserRepositoryMock", m.MockType)
	}
	if m.Constructor != "NewUserRepositoryMock" {
		t.Errorf("Constructor = %q, want NewUserRepositoryMock", m.Constructor)
	}
}

func TestAnalyzeReceiver_pathFields(t *testing.T) {
	// Проверяем поля, используемые mockgen и render.
	fn := makeMethod("GetUserName", []model.ReceiverField{
		{Name: "repo", TypeStr: "UserRepository", IsInterface: true},
	})
	plan := mockplan.AnalyzeReceiver(fn)
	m := plan.Mocks[0]

	if m.MockPackage != "mock" {
		t.Errorf("MockPackage = %q, want mock", m.MockPackage)
	}
	// PackageDir — абсолютный путь пакета (используется как cmd.Dir для minimock).
	if m.PackageDir != "/abs/example/service" {
		t.Errorf("PackageDir = %q, want /abs/example/service", m.PackageDir)
	}
	wantImport := "github.com/yourorg/testgen/example/service/mock"
	if m.MockImportPath != wantImport {
		t.Errorf("MockImportPath = %q, want %q", m.MockImportPath, wantImport)
	}
	wantSrc := "github.com/yourorg/testgen/example/service.UserRepository"
	if m.SourceInterfacePath != wantSrc {
		t.Errorf("SourceInterfacePath = %q, want %q", m.SourceInterfacePath, wantSrc)
	}
	if m.MockFileName != "user_repository_mock.go" {
		t.Errorf("MockFileName = %q, want user_repository_mock.go", m.MockFileName)
	}
	wantMockFilePathSuffix := filepath.Join("mock", "user_repository_mock.go")
	if !strings.HasSuffix(m.MockFilePath, wantMockFilePathSuffix) {
		t.Fatalf("MockFilePath = %q, should end with %q", m.MockFilePath, wantMockFilePathSuffix)
	}

	wantMockDirSuffix := string(filepath.Separator) + "mock"
	if !strings.HasSuffix(m.MockDir, wantMockDirSuffix) {
		t.Fatalf("MockDir = %q, should end with %q", m.MockDir, wantMockDirSuffix)
	}
}

func TestAnalyzeReceiver_qualifiedTypeName(t *testing.T) {
	// analyzer возвращает TypeStr с квалификатором "service.UserRepository".
	// AnalyzeReceiver должен срезать его.
	fn := makeMethod("GetUserName", []model.ReceiverField{
		{Name: "repo", TypeStr: "service.UserRepository", IsInterface: true},
	})
	plan := mockplan.AnalyzeReceiver(fn)
	m := plan.Mocks[0]

	if m.InterfaceName != "UserRepository" {
		t.Errorf("InterfaceName = %q, want UserRepository", m.InterfaceName)
	}
	if m.MockType != "UserRepositoryMock" {
		t.Errorf("MockType = %q, want UserRepositoryMock", m.MockType)
	}
	if m.MockFileName != "user_repository_mock.go" {
		t.Errorf("MockFileName = %q, want user_repository_mock.go", m.MockFileName)
	}
	// Ни одно поле идентификации не должно содержать точку.
	for _, s := range []string{m.FieldName, m.InterfaceName, m.MockType, m.Constructor} {
		if strings.Contains(s, ".") {
			t.Errorf("MockSpec поле содержит точку: %q", s)
		}
	}
}

func TestAnalyzeReceiver_mixedFields(t *testing.T) {
	fn := makeMethod("Process", []model.ReceiverField{
		{Name: "logger", TypeStr: "Logger", IsInterface: true},
		{Name: "counter", TypeStr: "int", IsInterface: false},
		{Name: "store", TypeStr: "KeyValueStore", IsInterface: true},
	})
	plan := mockplan.AnalyzeReceiver(fn)
	if len(plan.Mocks) != 2 {
		t.Fatalf("ожидалось 2 мока, got %d", len(plan.Mocks))
	}
}

// ── Legacy Analyze ────────────────────────────────────────────────────────────

func TestAnalyze_legacyInterfaceParam(t *testing.T) {
	fn := model.FunctionSpec{
		Name: "Run",
		Params: []model.ParamSpec{
			{Name: "ctx", TypeStr: "context.Context", Kind: model.KindInterface},
		},
	}
	plan := mockplan.Analyze(fn)
	if !plan.HasMocks() {
		t.Error("Analyze должен находить interface-параметры")
	}
}
