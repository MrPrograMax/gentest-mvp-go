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

// ── Базовые тесты (MockNone, internal package) ────────────────────────────────

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
		Tests:       []model.TestSpec{{Func: fn, Scenarios: scenario.Generate(fn)}},
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
	if strings.Contains(out, "reflect.DeepEqual") {
		t.Error("вывод не должен содержать reflect.DeepEqual")
	}
	if !strings.Contains(out, "_ = got0") {
		t.Error("вывод должен содержать _ = got0")
	}
	if strings.Contains(out, "want0") {
		t.Error("вывод не должен содержать want0")
	}
	// Internal package — нет _test суффикса.
	if strings.Contains(out, "package calc_test") {
		t.Error("MockNone не должен генерировать external test package")
	}
	if !strings.Contains(out, "package calc") {
		t.Error("должен быть package calc")
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
		Tests:       []model.TestSpec{{Func: fn, Scenarios: scenario.Generate(fn)}},
	}
	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	if !strings.Contains(out, "wantErr") {
		t.Error("вывод должен содержать wantErr")
	}
	if !strings.Contains(out, "tt.wantErr") {
		t.Error("вывод должен содержать tt.wantErr")
	}
}

func TestRenderFile_voidFunction(t *testing.T) {
	fn := model.FunctionSpec{
		PackageName: "pkg",
		Name:        "Log",
		Guards:      emptyGuards(),
		Params:      []model.ParamSpec{{Name: "msg", TypeStr: "string", Kind: model.KindString}},
	}
	fs := model.FileSpec{
		PackageName: "pkg",
		Tests:       []model.TestSpec{{Func: fn, Scenarios: scenario.Generate(fn)}},
	}
	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	if strings.Contains(string(src), `"reflect"`) {
		t.Error("void-функция не должна импортировать reflect")
	}
}

func TestRenderFile_importsContext(t *testing.T) {
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
	if !strings.Contains(string(src), `"context"`) {
		t.Error("ожидался import context")
	}
}

func TestRenderFile_ioReaderImportsStrings(t *testing.T) {
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
	if !strings.Contains(out, `"strings"`) {
		t.Error("io.Reader fixture использует strings.NewReader → import strings")
	}
	if !strings.Contains(out, `"io"`) {
		t.Error("поле struct io.Reader требует import io")
	}
}

func TestRenderFile_ptrIntFixture(t *testing.T) {
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
		t.Error("вывод содержит &int{} — невалидный Go")
	}
	if !strings.Contains(out, "new(int)") {
		t.Error("ожидался new(int) для *int")
	}
}

// ── makeServiceMethod — helper для minimock тестов ────────────────────────────

// makeServiceMethod создаёт FunctionSpec для метода Service.GetUserName.
// TypeStrFull заполнен как от analyzer (qualified через types.TypeString).
// MockSpec содержит все поля, которые заполняет mockplan.AnalyzeReceiver.
func makeServiceMethod() model.FunctionSpec {
	return model.FunctionSpec{
		PackageName:       "service",
		PackageImportPath: "github.com/yourorg/testgen/example/service",
		PackagePath:       "/abs/example/service",
		Name:              "GetUserName",
		IsMethod:          true,
		ReceiverName:      "s",
		ReceiverType:      "*Service",
		HasError:          true,
		Guards:            emptyGuards(),
		Params: []model.ParamSpec{
			{
				Name: "ctx", TypeStr: "context.Context",
				TypeStrFull: "context.Context", // уже qualified
				Kind:        model.KindInterface,
			},
			{
				Name: "id", TypeStr: "int64",
				TypeStrFull: "int64",
				Kind:        model.KindInt,
			},
		},
		Results: []model.ParamSpec{
			{Name: "result0", TypeStr: "string", TypeStrFull: "string", Kind: model.KindString},
			{Name: "err", TypeStr: "error", TypeStrFull: "error", Kind: model.KindError, IsError: true},
		},
		ReceiverFields: []model.ReceiverField{
			{Name: "repo", TypeStr: "service.UserRepository", IsInterface: true},
		},
		MockPlan: model.MockPlan{
			Mocks: []model.MockSpec{
				{
					FieldName:           "repo",
					InterfaceName:       "UserRepository",
					MockType:            "UserRepositoryMock",
					Constructor:         "NewUserRepositoryMock",
					SourceInterfacePath: "github.com/yourorg/testgen/example/service.UserRepository",
					MockPackage:         "mock",
					MockImportPath:      "github.com/yourorg/testgen/example/service/mock",
					PackageDir:          "/abs/example/service",
					MockDir:             "/abs/example/service/mock",
					MockFileName:        "user_repository_mock.go",
					MockFilePath:        "/abs/example/service/mock/user_repository_mock.go",
				},
			},
		},
	}
}

func makeServiceFileSpec(fn model.FunctionSpec) model.FileSpec {
	return model.FileSpec{
		PackageName:       "service",
		PackageImportPath: "github.com/yourorg/testgen/example/service",
		Tests:             []model.TestSpec{{Func: fn, Scenarios: scenario.Generate(fn)}},
		MockMode:          model.MockMinimock,
	}
}

// ── Minimock: external test package ──────────────────────────────────────────

func TestRenderFile_minimock_externalPackage(t *testing.T) {
	// При MockMode=minimock + MockPlan: пакет должен быть service_test.
	fn := makeServiceMethod()
	src, err := render.RenderFile(makeServiceFileSpec(fn))
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	if !strings.Contains(out, "package service_test") {
		t.Errorf("ожидался package service_test, вывод:\n%s", out)
	}
	// Не должно быть: package service (без _test)
	// Проверяем что первая строка с package содержит _test
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			if !strings.Contains(line, "_test") {
				t.Errorf("первая package строка должна содержать _test: %q", line)
			}
			break
		}
	}
}

func TestRenderFile_minimock_sourcePackageImport(t *testing.T) {
	// External test package должен импортировать source пакет с алиасом.
	fn := makeServiceMethod()
	src, err := render.RenderFile(makeServiceFileSpec(fn))
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	// service alias import
	if !strings.Contains(out, `service "github.com/yourorg/testgen/example/service"`) {
		t.Errorf("ожидался aliased import source пакета, вывод:\n%s", out)
	}
}

func TestRenderFile_minimock_mockPackageImport(t *testing.T) {
	// External test package должен импортировать mock пакет с алиасом.
	fn := makeServiceMethod()
	src, err := render.RenderFile(makeServiceFileSpec(fn))
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	if !strings.Contains(out, `mock "github.com/yourorg/testgen/example/service/mock"`) {
		t.Errorf("ожидался aliased import mock пакета, вывод:\n%s", out)
	}
}

func TestRenderFile_minimock_testMocksStruct(t *testing.T) {
	fn := makeServiceMethod()
	src, err := render.RenderFile(makeServiceFileSpec(fn))
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	if !strings.Contains(out, "type testMocks struct") {
		t.Error("ожидался type testMocks struct")
	}
	if !strings.Contains(out, "repo *mock.UserRepositoryMock") {
		t.Errorf("в testMocks должно быть 'repo *mock.UserRepositoryMock', вывод:\n%s", out)
	}
}

func TestRenderFile_minimock_controllerAndMocks(t *testing.T) {
	fn := makeServiceMethod()
	src, err := render.RenderFile(makeServiceFileSpec(fn))
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	if !strings.Contains(out, "minimock.NewController(t)") {
		t.Error("ожидался minimock.NewController(t)")
	}
	if !strings.Contains(out, "mock.NewUserRepositoryMock(mc)") {
		t.Errorf("ожидался mock.NewUserRepositoryMock(mc), вывод:\n%s", out)
	}
	if !strings.Contains(out, "tt.prepare(m)") {
		t.Error("ожидался tt.prepare(m)")
	}
}

func TestRenderFile_minimock_constructorSetup(t *testing.T) {
	// Receiver инициализируется через service.NewService(m.repo).
	fn := makeServiceMethod()
	src, err := render.RenderFile(makeServiceFileSpec(fn))
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	if !strings.Contains(out, "rcv := service.NewService(m.repo)") {
		t.Errorf("ожидался 'rcv := service.NewService(m.repo)', вывод:\n%s", out)
	}
	// НЕ должно быть composite literal &Service{...}
	if strings.Contains(out, "&Service{") || strings.Contains(out, "&service.Service{") {
		t.Error("в minimock режиме не должен использоваться composite literal &Service{}")
	}
}

func TestRenderFile_minimock_prepareTODO(t *testing.T) {
	fn := makeServiceMethod()
	src, err := render.RenderFile(makeServiceFileSpec(fn))
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	if !strings.Contains(out, "prepare: func(m *testMocks)") {
		t.Error("ожидалось поле prepare: func(m *testMocks)")
	}
	if !strings.Contains(out, "TODO: configure minimock expectations for this scenario") {
		t.Error("ожидался TODO в теле prepare")
	}
}

func TestRenderFile_minimock_noBareNames(t *testing.T) {
	// Никаких невалидных паттернов из прошлых версий.
	fn := makeServiceMethod()
	src, err := render.RenderFile(makeServiceFileSpec(fn))
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)

	// Старые невалидные паттерны.
	if strings.Contains(out, "service.UserRepository ") {
		t.Error("вывод содержит невалидное имя поля: service.UserRepository")
	}
	if strings.Contains(out, "Newservice.") {
		t.Error("вывод содержит Newservice.")
	}
	if strings.Contains(out, "m.service.") {
		t.Error("вывод содержит m.service.")
	}
	// Bare-имена без пакетного префикса
	if strings.Contains(out, "*UserRepositoryMock") {
		t.Error("вывод содержит *UserRepositoryMock без пакетного префикса")
	}
}

func TestRenderFile_mockModeNone_noMinimockArtifacts(t *testing.T) {
	fn := makeServiceMethod()
	fs := model.FileSpec{
		PackageName:       "service",
		PackageImportPath: "github.com/yourorg/testgen/example/service",
		Tests:             []model.TestSpec{{Func: fn, Scenarios: scenario.Generate(fn)}},
		MockMode:          model.MockNone,
	}
	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	if strings.Contains(out, "minimock") {
		t.Error("при MockNone не должно быть упоминаний minimock")
	}
	if strings.Contains(out, "type testMocks") {
		t.Error("при MockNone не должно быть type testMocks")
	}
	if strings.Contains(out, "service_test") {
		t.Error("при MockNone пакет не должен быть _test")
	}
}

func TestRenderFile_mockMinimock_noPlanFunction(t *testing.T) {
	// MockMode=minimock, но MockPlan пустой → нет minimock-артефактов, нет _test пакета.
	fn := model.FunctionSpec{
		PackageName:       "calc",
		PackageImportPath: "github.com/yourorg/testgen/example/calc",
		Name:              "Add",
		Guards:            emptyGuards(),
		Params: []model.ParamSpec{
			{Name: "a", TypeStr: "int", Kind: model.KindInt},
			{Name: "b", TypeStr: "int", Kind: model.KindInt},
		},
		Results: []model.ParamSpec{
			{Name: "result0", TypeStr: "int", Kind: model.KindInt},
		},
	}
	fs := model.FileSpec{
		PackageName:       "calc",
		PackageImportPath: "github.com/yourorg/testgen/example/calc",
		Tests:             []model.TestSpec{{Func: fn, Scenarios: scenario.Generate(fn)}},
		MockMode:          model.MockMinimock,
	}
	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	if strings.Contains(out, "minimock") {
		t.Error("без MockPlan minimock-артефактов быть не должно")
	}
	// Нет MockPlan → нет external test package
	if strings.Contains(out, "package calc_test") {
		t.Error("без MockPlan не должно быть external test package")
	}
}

// helpers for Go < 1.21 compat in tests
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── Regression: registration struct scenarios ─────────────────────────────────

func makeRegistrationFuncSpec() model.FunctionSpec {
	addrFields := []model.StructField{
		{Name: "City", TypeStr: "string", Kind: model.KindString},
		{Name: "Street", TypeStr: "string", Kind: model.KindString},
		{Name: "House", TypeStr: "string", Kind: model.KindString},
	}
	reqFields := []model.StructField{
		{Name: "Email", TypeStr: "string", Kind: model.KindString},
		{Name: "Name", TypeStr: "string", Kind: model.KindString},
		{Name: "Age", TypeStr: "int", Kind: model.KindInt},
		{Name: "Phone", TypeStr: "string", Kind: model.KindString},
		// TypeStr квалифицирован — как из analyzer. render передаёт PackageName чтобы убрать его.
		{Name: "Address", TypeStr: "registration.Address", Kind: model.KindStruct, SubFields: addrFields},
		{Name: "CreatedAt", TypeStr: "time.Time", Kind: model.KindTime},
	}
	return model.FunctionSpec{
		PackageName: "registration",
		Name:        "ValidateRegisterRequest",
		HasError:    true,
		Guards: model.Guards{
			NilCheckedParams:   map[string]bool{},
			EmptyCheckedParams: map[string]bool{},
			FieldGuards: []model.FieldGuard{
				{ParamName: "req", FieldPath: []string{"Email"}, Kind: model.FieldGuardEmpty},
				{ParamName: "req", FieldPath: []string{"Email"}, Kind: model.FieldGuardInvalid},
				{ParamName: "req", FieldPath: []string{"Name"}, Kind: model.FieldGuardEmpty},
				{ParamName: "req", FieldPath: []string{"Age"}, Kind: model.FieldGuardLessThan, Threshold: "18"},
				{ParamName: "req", FieldPath: []string{"Address", "City"}, Kind: model.FieldGuardEmpty},
			},
		},
		Params: []model.ParamSpec{
			{Name: "req", TypeStr: "RegisterRequest", Kind: model.KindStruct, StructFields: reqFields},
		},
		Results: []model.ParamSpec{
			{Name: "err", TypeStr: "error", Kind: model.KindError, IsError: true},
		},
	}
}

func TestRenderFile_registration_sixScenarios(t *testing.T) {
	fn := makeRegistrationFuncSpec()
	fs := model.FileSpec{
		PackageName: "registration",
		Tests:       []model.TestSpec{{Func: fn, Scenarios: scenario.Generate(fn)}},
	}
	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)

	wantScenarios := []string{
		"ValidateRegisterRequest/success",
		"ValidateRegisterRequest/error_empty_email",
		"ValidateRegisterRequest/error_invalid_email",
		"ValidateRegisterRequest/error_empty_name",
		"ValidateRegisterRequest/error_underage",
		"ValidateRegisterRequest/error_empty_city",
	}
	for _, name := range wantScenarios {
		if !strings.Contains(out, name) {
			t.Errorf("вывод не содержит сценарий %q", name)
		}
	}
}

func TestRenderFile_registration_noEmptyStruct(t *testing.T) {
	fn := makeRegistrationFuncSpec()
	fs := model.FileSpec{
		PackageName: "registration",
		Tests:       []model.TestSpec{{Func: fn, Scenarios: scenario.Generate(fn)}},
	}
	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	out := string(src)
	// Success-сценарий не должен использовать RegisterRequest{} без полей.
	if strings.Contains(out, "inputReq: RegisterRequest{}") {
		t.Error("success сценарий не должен использовать RegisterRequest{}")
	}
	if !strings.Contains(out, `"user@example.com"`) {
		t.Error("success сценарий должен содержать user@example.com")
	}
	// Тест в package registration — квалификатор "registration.Address" невалиден.
	if strings.Contains(out, "registration.Address") {
		t.Error("same-package тест не должен содержать 'registration.Address'")
	}
	if !strings.Contains(out, "Address{") {
		t.Error("должен содержать 'Address{' (без квалификатора)")
	}
}

func TestRenderFile_registration_importTime(t *testing.T) {
	fn := makeRegistrationFuncSpec()
	fs := model.FileSpec{
		PackageName: "registration",
		Tests:       []model.TestSpec{{Func: fn, Scenarios: scenario.Generate(fn)}},
	}
	src, err := render.RenderFile(fs)
	if err != nil {
		t.Fatalf("RenderFile: %v\n%s", err, src)
	}
	// RegisterRequest содержит time.Now() → import "time" обязателен
	if !strings.Contains(string(src), `"time"`) {
		t.Error("сгенерированный файл должен содержать import \"time\"")
	}
}
