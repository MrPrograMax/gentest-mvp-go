// Пакет model определяет общие структуры данных testgen.
// Все остальные пакеты общаются через эти типы.
package model

// TypeKind классифицирует Go-тип для выбора фикстур и сценариев.
type TypeKind int

const (
	KindUnknown   TypeKind = iota
	KindString             // string
	KindInt                // int, int8…int64, uint…uint64, float32, float64, byte, rune
	KindBool               // bool
	KindError              // встроенный интерфейс error
	KindSlice              // []T
	KindMap                // map[K]V
	KindPtr                // *T
	KindInterface          // interface{} / any / именованный интерфейс
	KindStruct             // именованная структура (не time.Time)
	KindTime               // time.Time
	KindDuration           // time.Duration
	KindFunc               // func(...) — тип-функция, передаваемый как аргумент
)

// ParamSpec описывает один параметр функции или возвращаемое значение.
type ParamSpec struct {
	Name    string   // идентификатор (генерируется автоматически если анонимный)
	TypeStr string   // строковое представление типа, например "[]string", "*MyType"
	Kind    TypeKind // классификация для выбора фикстуры
	IsError bool     // true когда тип является встроенным интерфейсом error
}

// Guards содержит факты статического анализа тела функции.
// Используется scenario.Generate для построения осмысленных edge-сценариев:
// edge создаётся только там, где в реальном коде есть соответствующая проверка.
type Guards struct {
	// NilCheckedParams — имена параметров, для которых тело функции
	// содержит явную проверку на nil (x == nil или x != nil).
	NilCheckedParams map[string]bool

	// EmptyCheckedParams — имена параметров с проверкой на пустоту:
	// x == "", len(x) == 0 и аналогичные паттерны.
	EmptyCheckedParams map[string]bool

	// ErrChecked — функция содержит хотя бы одну проверку вида err != nil
	// для локальной переменной типа error.
	ErrChecked bool

	// HasPanic — функция содержит вызов panic().
	HasPanic bool
}

// FunctionSpec — внутреннее представление экспортируемой функции или метода.
type FunctionSpec struct {
	PackageName  string
	PackagePath  string // директория, содержащая пакет
	Name         string
	ReceiverName string // например "s" — пусто для обычных функций
	ReceiverType string // например "*Service" — пусто для обычных функций
	IsMethod     bool
	IsVariadic   bool
	Params       []ParamSpec
	Results      []ParamSpec
	HasError     bool   // true когда последний результат имеет тип error
	Guards       Guards // факты анализа тела функции
}

// ScenarioKind обозначает назначение тестового сценария.
type ScenarioKind string

const (
	ScenarioSuccess ScenarioKind = "success"
	ScenarioError   ScenarioKind = "error"
	ScenarioEdge    ScenarioKind = "edge"
)

// FixtureValue — Go-выражение, используемое как тестовая фикстура.
type FixtureValue struct {
	Expr    string // Go-литерал или выражение, например "hello", 42, nil
	TypeStr string // соответствующая строка типа

	// NeedsMockComment — если true, render добавит TODO-комментарий в
	// сгенерированный тест рядом с этим полем. Используется для interface-типов,
	// где Expr == "nil" является лишь синтаксически корректной заглушкой,
	// но семантически пользователь обязан подставить реальный mock/stub.
	// Поле подготовлено для будущей интеграции с mockplan.
	NeedsMockComment bool
}

// ScenarioSpec представляет одну строку в table-driven тесте.
type ScenarioSpec struct {
	Name      string
	Kind      ScenarioKind
	Comment   string
	Inputs    []FixtureValue // по одному на каждый FunctionSpec.Params
	Wants     []FixtureValue // по одному на каждый не-error результат
	WantError bool
}

// TestSpec связывает FunctionSpec со сгенерированными ScenarioSpec.
type TestSpec struct {
	Func      FunctionSpec
	Scenarios []ScenarioSpec
}

// FileSpec — верхнеуровневый объект для рендеринга *_test.go файла.
type FileSpec struct {
	PackageName string
	SourceDir   string
	Tests       []TestSpec
}
