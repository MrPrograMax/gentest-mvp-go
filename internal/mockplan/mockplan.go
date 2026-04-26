// Пакет mockplan находит параметры с типами-интерфейсами и func-типами,
// и предлагает пользователю подсказку о необходимости заглушки.
//
// MVP: только диагностика — код мока не генерируется.
// KindFunc-параметры получают автоматическую заглушку через fixture.Happy,
// но их всё равно логируем чтобы пользователь знал.
package mockplan

import (
	"github.com/yourorg/testgen/internal/model"
)

// MockEntry описывает один параметр-интерфейс или func-тип.
type MockEntry struct {
	ParamName  string
	TypeStr    string
	Suggestion string // подсказка, выводимая в stderr при -v
}

// Plan — результат анализа одного FunctionSpec.
type Plan struct {
	FuncName string
	Entries  []MockEntry
}

// HasMocks сообщает, найдены ли параметры, требующие заглушек.
func (p Plan) HasMocks() bool { return len(p.Entries) > 0 }

// Analyze возвращает Plan для fn.
// Если interface- и func-параметров нет, Entries пуст.
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
			// fixture.Happy уже генерирует заглушку-panic, но сообщаем пользователю.
			plan.Entries = append(plan.Entries, MockEntry{
				ParamName:  p.Name,
				TypeStr:    p.TypeStr,
				Suggestion: "сгенерирована заглушка panic(\"testgen stub\") — замени на реальную логику",
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
