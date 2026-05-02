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
	Name    string // идентификатор (генерируется автоматически если анонимный)
	TypeStr string // короткое представление типа (без пакета для same-pkg типов)
	// TypeStrFull — полностью квалифицированное представление типа через type-checker.
	// Для типов из того же пакета добавляет квалификатор:
	// "UserRepository" → "service.UserRepository"
	// Используется в external test package (_test) чтобы типы были в scope.
	// Пустая строка означает что type-checker не предоставил информацию.
	TypeStrFull string
	Kind        TypeKind // классификация для выбора фикстуры
	IsError     bool     // true когда тип является встроенным интерфейсом error
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
	PackageName       string
	PackagePath       string // абсолютная директория пакета
	PackageImportPath string // полный import path, например "github.com/yourorg/testgen/example/service"
	Name              string
	ReceiverName      string // например "s" — пусто для обычных функций
	ReceiverType      string // например "*Service" — пусто для обычных функций
	IsMethod          bool
	IsVariadic        bool
	Params            []ParamSpec
	Results           []ParamSpec
	HasError          bool   // true когда последний результат имеет тип error
	Guards            Guards // факты анализа тела функции

	// ReceiverFields — поля receiver-структуры (только для методов).
	// Используется mockplan для поиска интерфейс-зависимостей,
	// которые нужно подменить моками в сгенерированном тесте.
	ReceiverFields []ReceiverField

	// MockPlan — план мокирования для этой функции.
	// Заполняется mockplan.Analyze когда MockMode != none.
	// Пустой план (нет интерфейс-полей) означает что моки не нужны.
	MockPlan MockPlan
}

// ReceiverField описывает одно поле receiver-структуры.
// Например для `type Service struct { repo UserRepository }`
// будет ReceiverField{Name: "repo", TypeStr: "UserRepository", IsInterface: true}.
type ReceiverField struct {
	Name        string // имя поля как в исходном коде
	TypeStr     string // строковое представление типа
	IsInterface bool   // true если это интерфейс (кандидат на мокирование)
}

// MockSpec описывает один мок, необходимый для теста метода.
//
// Поля идентификации:
//   - FieldName — имя поля receiver-структуры ("repo")
//   - InterfaceName — короткое имя интерфейса без пакета ("UserRepository")
//   - MockType — имя генерируемого типа без пакета ("UserRepositoryMock")
//   - Constructor — конструктор без пакета ("NewUserRepositoryMock")
//
// Поля размещения (вычисляются mockplan, используются mockgen и render):
//   - SourceInterfacePath — полный путь интерфейса для minimock -i
//     ("github.com/yourorg/testgen/example/service.UserRepository")
//   - MockPackage — имя пакета моков ("mock")
//   - MockImportPath — import path пакета моков
//     ("github.com/yourorg/testgen/example/service/mock")
//   - PackageDir — абсолютный путь директории исходного пакета
//     (используется как cmd.Dir при запуске minimock)
//   - MockDir — абсолютный путь к директории моков (<PackageDir>/mock)
//   - MockFileName — имя файла в snake_case ("user_repository_mock.go")
//   - MockFilePath — абсолютный путь к файлу мока (для os.Stat/проверок)
type MockSpec struct {
	FieldName     string
	InterfaceName string
	MockType      string
	Constructor   string

	SourceInterfacePath string
	MockPackage         string
	MockImportPath      string
	PackageDir          string
	MockDir             string
	MockFileName        string
	MockFilePath        string
}

// MockPlan — список моков для одного метода.
// HasMocks() возвращает true если есть хотя бы один мок-кандидат.
type MockPlan struct {
	Mocks []MockSpec
}

// HasMocks возвращает true если план непустой.
func (p MockPlan) HasMocks() bool { return len(p.Mocks) > 0 }

// MockMode задаёт стратегию подготовки моков.
type MockMode string

const (
	MockNone     MockMode = "none"     // моки не генерируются
	MockMinimock MockMode = "minimock" // используется gojuno/minimock
)

// FixtureMode задаёт стратегию генерации тестовых фикстур.
//
// На данный момент реализован только FixtureHeuristic.
// FixtureLLM и FixtureHybrid зарезервированы архитектурно —
// они возвращают ошибку "not implemented" до подключения LLM API.
type FixtureMode string

const (
	// FixtureHeuristic — текущая реализация: детерминированные правила
	// (42 для int, "test-value" для string, new(T) для указателей и т.д.).
	FixtureHeuristic FixtureMode = "heuristic"

	// FixtureLLM — генерация фикстур через LLM API.
	// Не реализован: возвращает ошибку "llm fixture provider is not implemented".
	FixtureLLM FixtureMode = "llm"

	// FixtureHybrid — эвристика + LLM для неизвестных типов.
	// Не реализован: возвращает ошибку "hybrid fixture provider is not implemented".
	FixtureHybrid FixtureMode = "hybrid"
)

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
	PackageName       string
	PackageImportPath string // полный import path, например "github.com/yourorg/testgen/example/service"
	SourceDir         string
	Tests             []TestSpec

	// MockMode определяет, какую minimock-инфраструктуру включит render.
	// MockNone — никаких моков (поведение по умолчанию для обратной совместимости).
	// MockMinimock — external test package (_test) + auto import source+mock пакетов.
	MockMode MockMode
}
