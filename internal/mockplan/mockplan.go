// Пакет mockplan строит план мокирования для метода с receiver-структурой.
//
// Основная задача: найти поля receiver-структуры с интерфейсным типом
// и собрать MockSpec со всеми путями, необходимыми mockgen и render.
//
// Структура вывода для поля repo UserRepository:
//
//	MockSpec{
//	    FieldName:           "repo",
//	    InterfaceName:       "UserRepository",
//	    MockType:            "UserRepositoryMock",
//	    Constructor:         "NewUserRepositoryMock",
//	    SourceInterfacePath: "<pkg>.UserRepository",
//	    MockPackage:         "mock",
//	    MockImportPath:      "<pkg>/mock",
//	    MockDir:             "<pkgDir>/mock",
//	    MockFileName:        "user_repository_mock.go",
//	    MockFilePath:        "<pkgDir>/mock/user_repository_mock.go",
//	}
//
// Для обратной совместимости здесь же оставлена диагностика interface-параметров
// функций (старый API), используемая app.go при выводе предупреждений в -v режиме.
package mockplan

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/yourorg/testgen/internal/model"
)

// ToSnakeCase конвертирует CamelCase имя в snake_case.
// Правила вставки _:
//  1. lowercase → Uppercase: "userRepo" → "user_repo"
//  2. Uppercase run → Uppercase+lowercase: "HTTPClient" → "http_client"
//
// Примеры:
//
//	UserRepository → user_repository
//	HTTPClient     → http_client
//	BKITransport   → bki_transport
func ToSnakeCase(s string) string {
	runes := []rune(s)
	var out []rune
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
				// Вставляем _, если предыдущий символ — строчная буква,
				// или если это конец серии заглавных перед строчной (HTTP→http, BKI→bki).
				if unicode.IsLower(prev) || (unicode.IsUpper(prev) && nextIsLower) {
					out = append(out, '_')
				}
			}
			out = append(out, unicode.ToLower(r))
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}

// AnalyzeReceiver строит MockPlan по полям receiver-структуры fn.
// Возвращает пустой план, если fn не является методом или у receiver-структуры
// нет полей-интерфейсов.
//
// Важно: analyzer.receiverFields возвращает TypeStr с квалификатором пакета
// (например "service.UserRepository"), потому что types.TypeString всегда
// использует короткое имя пакета. Квалификатор срезается — нужно только
// короткое имя (часть после последней точки).
func AnalyzeReceiver(fn model.FunctionSpec) model.MockPlan {
	if !fn.IsMethod {
		return model.MockPlan{}
	}

	var mocks []model.MockSpec
	for _, f := range fn.ReceiverFields {
		if !f.IsInterface {
			continue
		}

		// shortName: "service.UserRepository" → "UserRepository"
		shortName := f.TypeStr
		if idx := strings.LastIndex(f.TypeStr, "."); idx >= 0 {
			shortName = f.TypeStr[idx+1:]
		}

		mockFileName := ToSnakeCase(shortName) + "_mock.go"
		mockDir := filepath.Join(fn.PackagePath, "mock")

		mocks = append(mocks, model.MockSpec{
			FieldName:     f.Name,                     // "repo"
			InterfaceName: shortName,                  // "UserRepository"
			MockType:      shortName + "Mock",         // "UserRepositoryMock"
			Constructor:   "New" + shortName + "Mock", // "NewUserRepositoryMock"

			SourceInterfacePath: fn.PackageImportPath + "." + shortName,
			MockPackage:         "mock",
			MockImportPath:      fn.PackageImportPath + "/mock",
			PackageDir:          fn.PackagePath, // абсолютный путь пакета
			MockDir:             mockDir,
			MockFileName:        mockFileName,
			MockFilePath:        filepath.Join(mockDir, mockFileName), // для os.Stat/проверок
		})
	}

	return model.MockPlan{Mocks: mocks}
}

// ── Legacy: диагностика interface/func-параметров функций ─────────────────────

// MockEntry описывает один параметр-интерфейс или func-тип.
// Используется только для вывода предупреждений в stderr при -v режиме.
type MockEntry struct {
	ParamName  string
	TypeStr    string
	Suggestion string
}

// Plan — результат старого анализа FunctionSpec.Params.
// Не путать с model.MockPlan (новый план, для receiver-полей).
type Plan struct {
	FuncName string
	Entries  []MockEntry
}

// HasMocks сообщает, найдены ли параметры, требующие заглушек.
func (p Plan) HasMocks() bool { return len(p.Entries) > 0 }

// Analyze возвращает Plan для fn — диагностику interface/func-параметров.
// Сохранён ради обратной совместимости с app.go.
func Analyze(fn model.FunctionSpec) Plan {
	plan := Plan{FuncName: fn.Name}

	for _, p := range fn.Params {
		switch p.Kind {
		case model.KindInterface, model.KindError:
			plan.Entries = append(plan.Entries, MockEntry{
				ParamName:  p.Name,
				TypeStr:    p.TypeStr,
				Suggestion: interfaceSuggestion(p),
			})
		case model.KindFunc:
			plan.Entries = append(plan.Entries, MockEntry{
				ParamName:  p.Name,
				TypeStr:    p.TypeStr,
				Suggestion: "сгенерирована безопасная заглушка — при необходимости замени на реальную логику",
			})
		}
	}

	return plan
}

func interfaceSuggestion(p model.ParamSpec) string {
	switch {
	case p.IsError:
		return "передай errors.New(\"...\") или nil"
	case p.TypeStr == "interface{}" || p.TypeStr == "any":
		return "передай любое конкретное значение, подходящее для теста"
	default:
		return "реализуй локальный stub или используй mock-библиотеку"
	}
}
