// Пакет render преобразует model.FileSpec в отформатированный *_test.go файл.
//
// Пайплайн:
//  1. buildTemplateData — формирует промежуточную структуру из FileSpec.
//  2. template.Execute  — выполняет встроенный Go-шаблон.
//  3. format.Source     — применяет go/format для канонического вывода.
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
		// Возвращаем неформатированный источник чтобы пользователь мог диагностировать.
		return buf.Bytes(), fmt.Errorf("render: go/format: %w\n--- неформатированный источник ---\n%s", err, buf.String())
	}
	return src, nil
}

// ── Структуры данных шаблона ──────────────────────────────────────────────────

type fileData struct {
	PackageName string
	Imports     []string
	Tests       []testData
}

type testData struct {
	TestFuncName string
	StructFields []field
	Rows         []rowData
	SetupLines   []string // строки перед вызовом (например инициализация ресивера)
	CallLine     string   // строка с вызовом функции / метода
	AssertLines  []string // утверждения после вызова
}

type field struct {
	Name    string
	TypeStr string
}

type rowData struct {
	Name   string
	Fields []fieldValue
}

type fieldValue struct {
	Name  string
	Value string
	// Comment — строка inline-комментария (без // ), или пусто.
	// render помещает его после запятой: `fieldName: value, // Comment`
	// go/format сохраняет inline-комментарии без изменений.
	Comment string
}

// ── Построение данных шаблона ─────────────────────────────────────────────────

func buildTemplateData(fs model.FileSpec) (fileData, error) {
	fd := fileData{PackageName: fs.PackageName}

	for _, ts := range fs.Tests {
		td, err := buildTestData(ts)
		if err != nil {
			return fd, err
		}
		fd.Tests = append(fd.Tests, td)
	}

	fd.Imports = planImports(fs)
	return fd, nil
}

func buildTestData(ts model.TestSpec) (testData, error) {
	fn := ts.Func
	td := testData{
		TestFuncName: "Test" + fn.Name,
	}
	if fn.IsMethod {
		td.TestFuncName = "Test" + titleCase(fn.ReceiverType) + "_" + fn.Name
	}

	// ── Поля struct ───────────────────────────────────────────────────────────
	for i, p := range fn.Params {
		name := inputFieldName(p.Name, i)
		// Analyzer уже возвращает "[]T" для вариадических параметров,
		// поэтому дополнительных преобразований не нужно.
		td.StructFields = append(td.StructFields, field{Name: name, TypeStr: p.TypeStr})
	}

	// MVP-решение: want*-поля для не-error результатов НЕ генерируются.
	// Причина: testgen не умеет надёжно вывести ожидаемые значения из сигнатуры
	// функции (только пользователь знает её бизнес-логику). Раньше мы вставляли
	// placeholder-значения (42, "test-value", []string{"test-value"}) и сравнивали
	// их через reflect.DeepEqual — но это давало заведомо падающие тесты.
	// Теперь для не-error результатов в assert-секции генерируется
	// `_ = gotN // TODO: verify ...` — пользователь сам подставляет проверку.

	if fn.HasError {
		td.StructFields = append(td.StructFields, field{Name: "wantErr", TypeStr: "bool"})
	}

	// ── Строки таблицы ────────────────────────────────────────────────────────
	for _, sc := range ts.Scenarios {
		row := rowData{Name: sc.Name}

		for i, fv := range sc.Inputs {
			fieldName := inputFieldName(fn.Params[i].Name, i)
			// Если фикстура помечена NeedsMockComment (interface-тип),
			// прикрепляем TODO-комментарий, видимый пользователю в сгенерированном файле.
			// Это место зарезервировано для будущей интеграции с mockplan:
			// когда mockplan начнёт генерировать stub-код, комментарий заменится на него.
			comment := ""
			if fv.NeedsMockComment {
				comment = "TODO: подставь mock/stub для " + fn.Params[i].TypeStr
			}
			row.Fields = append(row.Fields, fieldValue{Name: fieldName, Value: fv.Expr, Comment: comment})
		}
		// sc.Wants намеренно не записываются: want*-поля не генерируются (см. выше).
		if fn.HasError {
			errVal := "false"
			if sc.WantError {
				errVal = "true"
			}
			row.Fields = append(row.Fields, fieldValue{Name: "wantErr", Value: errVal})
		}
		td.Rows = append(td.Rows, row)
	}

	// ── Setup-строки ──────────────────────────────────────────────────────────
	if fn.IsMethod {
		rcvType := strings.TrimPrefix(fn.ReceiverType, "*")
		td.SetupLines = append(td.SetupLines,
			fmt.Sprintf("rcv := &%s{} // TODO: инициализируй ресивер правильно", rcvType),
		)
	}

	// ── Строка вызова ─────────────────────────────────────────────────────────
	td.CallLine = buildCallLine(fn)

	// ── Строки утверждений ────────────────────────────────────────────────────
	td.AssertLines = buildAssertLines(fn)

	return td, nil
}

func buildCallLine(fn model.FunctionSpec) string {
	// Формируем список переменных результата: got0, got1, ..., err
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

	// Формируем список аргументов
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
		callExpr = fmt.Sprintf("rcv.%s(%s)", fn.Name, strings.Join(args, ", "))
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

	// Проверка ошибки — реальная: генератор знает что значит wantErr=true/false.
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

	// Проверка не-error результатов — НЕ автоматическая.
	// Раньше здесь был reflect.DeepEqual(got, want), но want — placeholder,
	// который генератор не может надёжно вывести → тесты заведомо падали.
	// Сейчас: для каждого got* выдаём `_ = gotN` (чтобы переменная считалась
	// использованной) и TODO-комментарий — пользователь сам впишет проверку,
	// когда определит ожидаемое поведение функции.
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

// titleCase приводит первый символ к верхнему регистру и убирает ведущий *.
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
	// Первая буква в верхнем регистре: "a" → "inputA"
	return "input" + strings.ToUpper(paramName[:1]) + paramName[1:]
}

// ── Import planner ────────────────────────────────────────────────────────────

// typeImportTable — таблица «подстрока TypeStr → путь импорта».
// Порядок важен: более специфичные записи должны идти раньше общих.
// Расширяй список при добавлении новых пакетов.
var typeImportTable = []struct {
	prefix string // подстрока, которую ищем в TypeStr
	path   string // путь импорта в кавычках
}{
	// Стандартная библиотека, обычно встречающаяся в сигнатурах функций:
	{"context.", `"context"`},
	{"io.", `"io"`},
	{"sql.", `"database/sql"`},
	{"http.", `"net/http"`},
	{"time.", `"time"`},
}

// exprImportTable — таблица «подстрока Expr → путь импорта».
// Нужна отдельно от typeImportTable: Expr и TypeStr могут требовать разных пакетов.
// Пример: TypeStr="io.Reader" → import "io", Expr="strings.NewReader(...)" → import "strings".
var exprImportTable = []struct {
	prefix string
	path   string
}{
	{"strings.", `"strings"`},
	{"context.", `"context"`},
	{"time.", `"time"`},
}

// planImports возвращает отсортированный список строк импортов для сгенерированного файла.
//
// Алгоритм:
//  1. Всегда добавляет "testing".
//  2. Сканирует TypeStr параметров и результатов через typeImportTable.
//  3. Сканирует Expr фикстур через exprImportTable (например strings.NewReader → "strings").
//  4. Проверяет фикстуры на time-выражения через NeedsTimeImport.
//
// reflect не нужен: assert не использует reflect.DeepEqual в MVP
// (см. комментарий в buildAssertLines).
//
// Итоговый список дедуплицируется и сортируется по алфавиту.
func planImports(fs model.FileSpec) []string {
	needed := map[string]bool{
		`"testing"`: true,
	}

	for _, ts := range fs.Tests {
		// Сканируем TypeStr параметров и результатов.
		for _, p := range ts.Func.Params {
			for _, imp := range importsForType(p.TypeStr) {
				needed[imp] = true
			}
		}
		for _, r := range ts.Func.Results {
			for _, imp := range importsForType(r.TypeStr) {
				needed[imp] = true
			}
		}

		// Сканируем Expr всех фикстур: они могут ссылаться на пакеты,
		// отличные от тех что видны в TypeStr.
		// Пример: TypeStr="io.Reader" → "io", Expr="strings.NewReader(...)" → "strings".
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
			// Фикстуры: time нужен если используются time-выражения.
			if fixture.NeedsTimeImport(sc.Inputs...) || fixture.NeedsTimeImport(sc.Wants...) {
				needed[`"time"`] = true
			}
		}
	}

	imports := make([]string, 0, len(needed))
	for imp := range needed {
		imports = append(imports, imp)
	}
	sort.Strings(imports)
	return imports
}

// importsForType возвращает список импортов, необходимых для typeStr.
// Проверяет наличие подстроки-префикса через typeImportTable.
// Использует strings.Contains, чтобы корректно обрабатывать составные типы:
//
//	*context.Context        → "context"
//	[]io.Reader             → "io"
//	map[string]sql.NullStr  → "database/sql"
//	func(context.Context)   → "context"
func importsForType(typeStr string) []string {
	var result []string
	for _, entry := range typeImportTable {
		if strings.Contains(typeStr, entry.prefix) {
			result = append(result, entry.path)
		}
	}
	return result
}

// importsForExpr возвращает список импортов, необходимых для Go-выражения expr.
// Используется для сканирования fixture.Expr, чтобы выявить пакеты,
// явно используемые в значениях (например strings.NewReader, context.Background).
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
{{range .Tests}}
// {{.TestFuncName}} сгенерирован testgen.
// TODO: дополни проверки got* в соответствии с бизнес-логикой
// (см. строки '_ = gotN' ниже — там стоят TODO-комментарии).
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
