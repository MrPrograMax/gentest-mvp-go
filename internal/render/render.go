// Пакет render преобразует model.FileSpec в отформатированный *_test.go файл.
//
// Пайплайн:
//  1. buildTemplateData — формирует промежуточную структуру из FileSpec.
//  2. template.Execute  — выполняет встроенный Go-шаблон.
//  3. format.Source     — применяет go/format для канонического вывода.
//
// Режимы моков (FileSpec.MockMode):
//
//	MockNone:
//	  package <name>        — обычный тест в том же пакете.
//
//	MockMinimock (при наличии MockPlan):
//	  package <name>_test   — ВНЕШНИЙ тестовый пакет.
//	  Причина: mock-пакет (service/mock) импортирует source-пакет (service),
//	  поэтому тест в package service не может импортировать service/mock —
//	  возникает import cycle. Внешний _test пакет разрывает цикл:
//	    service_test → service   (no cycle)
//	    service_test → service/mock → service  (ok, service_test ≠ service)
//
//	  Renderer добавляет:
//	    import service "pkg/path"       — для вызова service.NewService(...)
//	    import mock "pkg/path/mock"     — для *mock.UserRepositoryMock
//	  Struct fields используют TypeStrFull (qualified через go/types).
//	  Setup: rcv := service.New<Type>(m.fieldName...)
//	  Call:  rcv.Method(...) или service.Func(...)
package render

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strings"
	"text/template"

	"github.com/yourorg/testgen/internal/fixture"
	"github.com/yourorg/testgen/internal/model"
)

// RenderFile рендерит все тесты из fs в один []byte отформатированного Go-кода,
// готового к записи как *_test.go файл.
func RenderFile(fs model.FileSpec) ([]byte, error) {
	td, err := buildTemplateData(fs)
	if err != nil {
		return nil, fmt.Errorf("render: buildTemplateData: %w", err)
	}

	var buf bytes.Buffer
	if err := testFileTmpl.Execute(&buf, td); err != nil {
		return nil, fmt.Errorf("render: execute template: %w", err)
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return buf.Bytes(), fmt.Errorf("render: go/format: %w\n--- неформатированный источник ---\n%s", err, buf.String())
	}
	return src, nil
}

// ── Контекст рендеринга ────────────────────────────────────────────────────────

// renderCtx содержит параметры, влияющие на генерацию всего файла.
type renderCtx struct {
	mockMode model.MockMode

	// isExternalTest — true когда тест генерируется в package <pkg>_test.
	// Активируется при MockMode=minimock и наличии хотя бы одного MockPlan.
	isExternalTest bool

	// srcPkgName — имя source пакета ("service"). Используется как alias в импорте.
	srcPkgName string

	// srcPkgImportPath — import path source пакета.
	// Пример: "github.com/yourorg/testgen/example/service"
	srcPkgImportPath string
}

// ── Структуры данных шаблона ──────────────────────────────────────────────────

type fileData struct {
	PackageName string
	Imports     []string
	Tests       []testData

	// MinimockEnabled — true когда хотя бы один тест в файле имеет MockPlan.HasMocks()
	// и режим — minimock.
	MinimockEnabled bool
	// TestMocks — поля type testMocks struct (объединение моков всех тестов файла).
	TestMocks []testMockField
}

// testMockField — одно поле в type testMocks struct.
// FieldName = имя поля receiver-структуры ("repo").
// QualifiedType = тип с пакетным префиксом ("mock.UserRepositoryMock").
// QualifiedConstructor = конструктор с пакетным префиксом ("mock.NewUserRepositoryMock").
type testMockField struct {
	FieldName            string
	QualifiedType        string
	QualifiedConstructor string
}

type testData struct {
	TestFuncName string
	StructFields []field
	Rows         []rowData
	SetupLines   []string
	CallLine     string
	AssertLines  []string

	// HasMocks — true для метода, у которого MockPlan непустой и режим — minimock.
	HasMocks bool
	// Mocks — конкретные моки для этого теста (используются в t.Run setup).
	Mocks []testMockField
}

type field struct {
	Name    string
	TypeStr string
}

type rowData struct {
	Name        string
	Fields      []fieldValue
	PrepareBody string
}

type fieldValue struct {
	Name  string
	Value string
	// Comment — строка inline-комментария (без // ), или пусто.
	Comment string
}

// ── Построение данных шаблона ─────────────────────────────────────────────────

func buildTemplateData(fs model.FileSpec) (fileData, error) {
	// Определяем, нужен ли external test package.
	ctx := renderCtx{
		mockMode:         fs.MockMode,
		srcPkgName:       fs.PackageName,
		srcPkgImportPath: fs.PackageImportPath,
	}
	if fs.MockMode == model.MockMinimock {
		for _, ts := range fs.Tests {
			if ts.Func.MockPlan.HasMocks() {
				ctx.isExternalTest = true
				break
			}
		}
	}

	pkgName := fs.PackageName
	if ctx.isExternalTest {
		pkgName = fs.PackageName + "_test"
	}

	fd := fileData{PackageName: pkgName}
	mockSet := map[string]testMockField{}

	for _, ts := range fs.Tests {
		td, err := buildTestData(ts, ctx)
		if err != nil {
			return fd, err
		}
		fd.Tests = append(fd.Tests, td)

		for _, m := range td.Mocks {
			if _, ok := mockSet[m.FieldName]; !ok {
				mockSet[m.FieldName] = m
			}
		}
	}

	if fs.MockMode == model.MockMinimock && len(mockSet) > 0 {
		fd.MinimockEnabled = true
		for _, m := range mockSet {
			fd.TestMocks = append(fd.TestMocks, m)
		}
		sort.Slice(fd.TestMocks, func(i, j int) bool {
			return fd.TestMocks[i].FieldName < fd.TestMocks[j].FieldName
		})
	}

	fd.Imports = planImports(fs, ctx)
	return fd, nil
}

func buildTestData(ts model.TestSpec, ctx renderCtx) (testData, error) {
	fn := ts.Func
	td := testData{
		TestFuncName: "Test" + fn.Name,
	}
	if fn.IsMethod {
		td.TestFuncName = "Test" + titleCase(fn.ReceiverType) + "_" + fn.Name
	}

	// Решаем, активен ли minimock-режим для этой функции.
	td.HasMocks = ctx.mockMode == model.MockMinimock && fn.MockPlan.HasMocks()
	if td.HasMocks {
		td.Mocks = make([]testMockField, 0, len(fn.MockPlan.Mocks))
		for _, m := range fn.MockPlan.Mocks {
			pkg := m.MockPackage
			if pkg == "" {
				pkg = "mock"
			}
			td.Mocks = append(td.Mocks, testMockField{
				FieldName:            m.FieldName,
				QualifiedType:        pkg + "." + m.MockType,    // "mock.UserRepositoryMock"
				QualifiedConstructor: pkg + "." + m.Constructor, // "mock.NewUserRepositoryMock"
			})
		}
	}

	// ── Поля struct ───────────────────────────────────────────────────────────
	for i, p := range fn.Params {
		name := inputFieldName(p.Name, i)
		// В external test package используем TypeStrFull (qualified через type-checker).
		// Это позволяет правильно квалифицировать типы из source пакета:
		// "UserRepository" → "service.UserRepository"
		ts := p.TypeStr
		if ctx.isExternalTest && p.TypeStrFull != "" {
			ts = p.TypeStrFull
		}
		td.StructFields = append(td.StructFields, field{Name: name, TypeStr: ts})
	}

	if fn.HasError {
		td.StructFields = append(td.StructFields, field{Name: "wantErr", TypeStr: "bool"})
	}

	// prepare-поле — только в minimock-режиме с непустым MockPlan.
	if td.HasMocks {
		td.StructFields = append(td.StructFields, field{
			Name:    "prepare",
			TypeStr: "func(m *testMocks)",
		})
	}

	// ── Строки таблицы ────────────────────────────────────────────────────────
	for _, sc := range ts.Scenarios {
		row := rowData{Name: sc.Name}

		for i, fv := range sc.Inputs {
			fieldName := inputFieldName(fn.Params[i].Name, i)
			comment := ""
			if fv.NeedsMockComment {
				comment = "TODO: подставь mock/stub для " + fn.Params[i].TypeStr
			}
			row.Fields = append(row.Fields, fieldValue{Name: fieldName, Value: fv.Expr, Comment: comment})
		}
		if fn.HasError {
			errVal := "false"
			if sc.WantError {
				errVal = "true"
			}
			row.Fields = append(row.Fields, fieldValue{Name: "wantErr", Value: errVal})
		}
		if td.HasMocks {
			row.PrepareBody = "// TODO: configure minimock expectations for this scenario"
			row.Fields = append(row.Fields, fieldValue{
				Name:  "prepare",
				Value: fmt.Sprintf("func(m *testMocks) {\n\t\t\t\t%s\n\t\t\t}", row.PrepareBody),
			})
		}
		td.Rows = append(td.Rows, row)
	}

	// ── Setup-строки ──────────────────────────────────────────────────────────
	td.SetupLines = buildSetupLines(fn, td.HasMocks, ctx)

	// ── Строка вызова ─────────────────────────────────────────────────────────
	td.CallLine = buildCallLine(fn, ctx)

	// ── Строки утверждений ────────────────────────────────────────────────────
	td.AssertLines = buildAssertLines(fn)

	return td, nil
}

// buildSetupLines формирует строки, выполняемые внутри t.Run перед вызовом.
//
// Без моков, internal package:
//
//	rcv := &Service{} // TODO
//
// Без моков, external package:
//
//	rcv := &service.Service{} // TODO
//
// С моками, external package (minimock):
//
//	mc := minimock.NewController(t)
//	m := &testMocks{repo: mock.NewUserRepositoryMock(mc)}
//	if tt.prepare != nil { tt.prepare(m) }
//	rcv := service.NewService(m.repo)
func buildSetupLines(fn model.FunctionSpec, hasMocks bool, ctx renderCtx) []string {
	if !fn.IsMethod {
		return nil
	}
	rcvType := strings.TrimPrefix(fn.ReceiverType, "*") // "Service"

	if !hasMocks {
		// Без моков: простая инициализация с TODO.
		if ctx.isExternalTest {
			return []string{
				fmt.Sprintf("rcv := &%s.%s{} // TODO: инициализируй ресивер правильно", ctx.srcPkgName, rcvType),
			}
		}
		return []string{
			fmt.Sprintf("rcv := &%s{} // TODO: инициализируй ресивер правильно", rcvType),
		}
	}

	// Режим minimock: контроллер → моки → prepare → receiver через конструктор.
	lines := []string{
		"mc := minimock.NewController(t)",
		"m := &testMocks{",
	}
	for _, mock := range fn.MockPlan.Mocks {
		pkg := mock.MockPackage
		if pkg == "" {
			pkg = "mock"
		}
		qualCtor := pkg + "." + mock.Constructor
		lines = append(lines, fmt.Sprintf("\t%s: %s(mc),", mock.FieldName, qualCtor))
	}
	lines = append(lines, "}")
	lines = append(lines,
		"if tt.prepare != nil {",
		"\ttt.prepare(m)",
		"}",
	)

	// Инициализируем receiver.
	// External package: используем constructor-конвенцию service.NewService(m.repo).
	// Аргументы — поля MockPlan в порядке их появления в ReceiverFields.
	// Это правильно для стандартной DI-конвенции func NewType(dep1, dep2 ...).
	if ctx.isExternalTest {
		ctorName := "New" + rcvType // "NewService"
		var args []string
		for _, m := range fn.MockPlan.Mocks {
			args = append(args, "m."+m.FieldName)
		}
		rcvLine := fmt.Sprintf("rcv := %s.%s(%s)", ctx.srcPkgName, ctorName, strings.Join(args, ", "))
		lines = append(lines, rcvLine)
	} else {
		// Internal package: composite literal с mock-полями.
		var rcvParts []string
		for _, mock := range fn.MockPlan.Mocks {
			rcvParts = append(rcvParts, fmt.Sprintf("%s: m.%s", mock.FieldName, mock.FieldName))
		}
		rcvLine := fmt.Sprintf("rcv := &%s{%s}", rcvType, strings.Join(rcvParts, ", "))
		lines = append(lines, rcvLine)
	}

	return lines
}

// buildCallLine формирует строку вызова тестируемой функции/метода.
// В external test package функции квалифицируются алиасом source пакета.
func buildCallLine(fn model.FunctionSpec, ctx renderCtx) string {
	var lhs []string
	gotIdx := 0
	for _, r := range fn.Results {
		if r.IsError {
			lhs = append(lhs, "err")
		} else {
			lhs = append(lhs, fmt.Sprintf("got%d", gotIdx))
			gotIdx++
		}
	}

	var args []string
	for i, p := range fn.Params {
		argName := fmt.Sprintf("tt.%s", inputFieldName(p.Name, i))
		if fn.IsVariadic && i == len(fn.Params)-1 {
			argName += "..."
		}
		args = append(args, argName)
	}

	var callExpr string
	if fn.IsMethod {
		// Метод: rcv.Method(...) — qualifier не нужен, rcv уже нужного типа.
		callExpr = fmt.Sprintf("rcv.%s(%s)", fn.Name, strings.Join(args, ", "))
	} else if ctx.isExternalTest {
		// Функция в external package: нужен qualifier.
		callExpr = fmt.Sprintf("%s.%s(%s)", ctx.srcPkgName, fn.Name, strings.Join(args, ", "))
	} else {
		callExpr = fmt.Sprintf("%s(%s)", fn.Name, strings.Join(args, ", "))
	}

	if len(lhs) == 0 {
		return callExpr
	}
	return strings.Join(lhs, ", ") + " := " + callExpr
}

func buildAssertLines(fn model.FunctionSpec) []string {
	var lines []string

	if fn.HasError {
		lines = append(lines,
			"if (err != nil) != tt.wantErr {",
			`t.Errorf("got err = %v, wantErr %v", err, tt.wantErr)`,
			"}",
			"if tt.wantErr {",
			"return",
			"}",
		)
	}

	gotIdx := 0
	for _, r := range fn.Results {
		if r.IsError {
			continue
		}
		lines = append(lines,
			fmt.Sprintf("_ = got%d // TODO: verify got%d with business-specific expected value", gotIdx, gotIdx),
		)
		gotIdx++
	}

	return lines
}

// ── Вспомогательные функции ───────────────────────────────────────────────────

func titleCase(s string) string {
	s = strings.TrimPrefix(s, "*")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func inputFieldName(paramName string, idx int) string {
	if paramName == "" || paramName == "_" {
		return fmt.Sprintf("input%d", idx)
	}
	return "input" + strings.ToUpper(paramName[:1]) + paramName[1:]
}

// ── Import planner ────────────────────────────────────────────────────────────

var typeImportTable = []struct {
	prefix string
	path   string
}{
	{"context.", `"context"`},
	{"io.", `"io"`},
	{"sql.", `"database/sql"`},
	{"http.", `"net/http"`},
	{"time.", `"time"`},
}

var exprImportTable = []struct {
	prefix string
	path   string
}{
	{"strings.", `"strings"`},
	{"context.", `"context"`},
	{"time.", `"time"`},
}

// planImports возвращает отсортированный список строк импортов.
//
// В режиме MockMinimock + external test package:
//   - добавляется import "github.com/gojuno/minimock/v3"
//   - добавляется import с алиасом source пакета: service "pkg/path"
//   - добавляется import с алиасом mock пакета: mock "pkg/path/mock"
//
// Поля TypeStr сканируются в TypeStrFull-форме (qualified), чтобы добраться
// до нужных пакетов, даже когда TypeStr содержит только короткое имя.
func planImports(fs model.FileSpec, ctx renderCtx) []string {
	needed := map[string]bool{
		`"testing"`: true,
	}

	hasMinimock := false

	for _, ts := range fs.Tests {
		// Сканируем TypeStr (и TypeStrFull) параметров и результатов.
		for _, p := range ts.Func.Params {
			for _, imp := range importsForType(p.TypeStr) {
				needed[imp] = true
			}
			// TypeStrFull может содержать квалифицированные типы из внешних пакетов.
			if p.TypeStrFull != "" && p.TypeStrFull != p.TypeStr {
				for _, imp := range importsForType(p.TypeStrFull) {
					needed[imp] = true
				}
			}
		}
		for _, r := range ts.Func.Results {
			for _, imp := range importsForType(r.TypeStr) {
				needed[imp] = true
			}
		}

		for _, sc := range ts.Scenarios {
			for _, fv := range sc.Inputs {
				for _, imp := range importsForExpr(fv.Expr) {
					needed[imp] = true
				}
			}
			for _, fv := range sc.Wants {
				for _, imp := range importsForExpr(fv.Expr) {
					needed[imp] = true
				}
			}
			if fixture.NeedsTimeImport(sc.Inputs...) || fixture.NeedsTimeImport(sc.Wants...) {
				needed[`"time"`] = true
			}
		}

		if fs.MockMode == model.MockMinimock && ts.Func.MockPlan.HasMocks() {
			hasMinimock = true
			for _, m := range ts.Func.MockPlan.Mocks {
				if m.MockImportPath != "" {
					if ctx.isExternalTest {
						// Используем явный alias совпадающий с именем пакета (mock).
						// Это нужно чтобы не конфликтовать с другими imports.
						needed[m.MockPackage+` "`+m.MockImportPath+`"`] = true
					} else {
						needed[`"`+m.MockImportPath+`"`] = true
					}
				}
			}
		}
	}

	if hasMinimock {
		needed[`"github.com/gojuno/minimock/v3"`] = true
		// В external test package добавляем import source пакета с алиасом.
		if ctx.isExternalTest && ctx.srcPkgImportPath != "" {
			needed[ctx.srcPkgName+` "`+ctx.srcPkgImportPath+`"`] = true
		}
	}

	imports := make([]string, 0, len(needed))
	for imp := range needed {
		imports = append(imports, imp)
	}
	sort.Strings(imports)
	return imports
}

func importsForType(typeStr string) []string {
	var result []string
	for _, entry := range typeImportTable {
		if strings.Contains(typeStr, entry.prefix) {
			result = append(result, entry.path)
		}
	}
	return result
}

func importsForExpr(expr string) []string {
	var result []string
	for _, entry := range exprImportTable {
		if strings.Contains(expr, entry.prefix) {
			result = append(result, entry.path)
		}
	}
	return result
}

// ── Шаблон ────────────────────────────────────────────────────────────────────

const testFileTemplate = `// Code generated by testgen. DO NOT EDIT.
package {{.PackageName}}

import (
{{- range .Imports}}
	{{.}}
{{- end}}
)
{{if .MinimockEnabled}}
// testMocks объединяет все мок-объекты, используемые в тестах файла.
// Каждое поле инициализируется через minimock.NewController в t.Run.
type testMocks struct {
{{- range .TestMocks}}
	{{.FieldName}} *{{.QualifiedType}}
{{- end}}
}
{{end}}
{{range .Tests}}
// {{.TestFuncName}} сгенерирован testgen.
// TODO: дополни проверки got* в соответствии с бизнес-логикой
// (см. строки '_ = gotN' ниже — там стоят TODO-комментарии).
{{- if .HasMocks}}
// Для каждой строки таблицы реализуй prepare(m *testMocks):
// настрой ожидания минимок-моков, например m.<fieldName>.<Method>Mock.Expect(...).Return(...).
{{- end}}
func {{.TestFuncName}}(t *testing.T) {
	tests := []struct {
		name string
{{- range .StructFields}}
		{{.Name}} {{.TypeStr}}
{{- end}}
	}{
{{- range .Rows}}
		{
			name:    {{printf "%q" .Name}},
{{- range .Fields}}
			{{.Name}}: {{.Value}},{{if .Comment}} // {{.Comment}}{{end}}
{{- end}}
		},
{{- end}}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
{{- range .SetupLines}}
			{{.}}
{{- end}}
			{{.CallLine}}
{{- range .AssertLines}}
			{{.}}
{{- end}}
		})
	}
}
{{end}}`

var testFileTmpl = template.Must(template.New("testfile").Parse(testFileTemplate))
