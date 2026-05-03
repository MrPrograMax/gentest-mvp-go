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

// StructField описывает одно поле структуры для генерации фикстур.
// Заполняется analyzer через go/types; используется fixture.HappyStructExpr
// для построения полного composite literal с семантическими значениями.
type StructField struct {
	Name      string        // "Email", "Age", "Address"
	TypeStr   string        // "string", "int", "Address", "time.Time"
	Kind      TypeKind      // для выбора базовой фикстуры
	SubFields []StructField // непустой для вложенных KindStruct
}

// FieldGuardKind описывает вид guard-проверки поля структуры.
type FieldGuardKind = string

const (
	// FieldGuardEmpty — поле проверяется на пустую строку: field == ""
	FieldGuardEmpty FieldGuardKind = "empty"
	// FieldGuardInvalid — поле проверяется на содержание подстроки: !strings.Contains(field, "@")
	FieldGuardInvalid FieldGuardKind = "invalid"
	// FieldGuardLessThan — поле числово меньше порога: field < N
	FieldGuardLessThan FieldGuardKind = "less_than"
	// FieldGuardNil — поле проверяется на nil: field == nil
	FieldGuardNil FieldGuardKind = "nil"
)

// FieldGuard описывает guard-проверку поля struct-параметра.
//
// Примеры:
//
//	req.Email == ""                     → Kind=empty,     FieldPath=["Email"]
//	!strings.Contains(req.Email, "@")   → Kind=invalid,   FieldPath=["Email"], Value="@"
//	req.Age < 18                        → Kind=less_than, FieldPath=["Age"],   Threshold="18"
//	req.Address.City == ""              → Kind=empty,     FieldPath=["Address","City"]
//
// Используется scenario.fieldGuardScenarios для построения отдельного
// error-сценария на каждую guard-ветку.
type FieldGuard struct {
	ParamName string         // имя параметра: "req"
	FieldPath []string       // путь к полю: ["Email"] или ["Address","City"]
	Kind      FieldGuardKind // вид проверки

	// Threshold — пороговое значение для less_than, например "18".
	Threshold string
	// Value — ожидаемая подстрока для invalid, например "@".
	Value string
}

// ParamSpec описывает один параметр функции или возвращаемое значение.
type ParamSpec struct {
	Name    string // идентификатор (генерируется автоматически если анонимный)
	TypeStr string // короткое представление типа (без пакета для same-pkg типов)

	// TypeStrFull — полностью квалифицированное представление типа через type-checker.
	// Используется в external test package (_test) чтобы типы были в scope.
	TypeStrFull string

	Kind    TypeKind // классификация для выбора фикстуры
	IsError bool     // true когда тип является встроенным интерфейсом error

	// StructFields — поля структуры для KindStruct-параметров.
	// Заполняется analyzer через go/types; используется fixture.HappyStructExpr
	// и scenario.fieldGuardScenarios для генерации осмысленных фикстур.
	StructFields []StructField
}

// Guards содержит факты статического анализа тела функции.
// Используется scenario.Generate для построения осмысленных edge-сценариев.
type Guards struct {
	// NilCheckedParams — параметры с явной проверкой x == nil / x != nil.
	NilCheckedParams map[string]bool

	// EmptyCheckedParams — параметры с проверкой x == "" / len(x) == 0.
	EmptyCheckedParams map[string]bool

	// ErrChecked — функция содержит проверку err != nil для error-переменной.
	ErrChecked bool

	// HasPanic — функция содержит вызов panic().
	HasPanic bool

	// FieldGuards — guards по конкретным полям struct-параметров.
	// Например: req.Email == "", req.Age < 18, req.Address.City == "".
	FieldGuards []FieldGuard
}

// FunctionSpec — внутреннее представление экспортируемой функции или метода.
type FunctionSpec struct {
	PackageName       string
	PackagePath       string // абсолютная директория пакета
	PackageImportPath string // полный import path
	Name              string
	ReceiverName      string
	ReceiverType      string
	IsMethod          bool
	IsVariadic        bool
	Params            []ParamSpec
	Results           []ParamSpec
	HasError          bool
	Guards            Guards

	ReceiverFields []ReceiverField
	MockPlan       MockPlan
}

// ReceiverField описывает одно поле receiver-структуры.
type ReceiverField struct {
	Name        string
	TypeStr     string
	IsInterface bool
}

// MockSpec описывает один мок, необходимый для теста метода.
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
type MockPlan struct {
	Mocks []MockSpec
}

// HasMocks возвращает true если план непустой.
func (p MockPlan) HasMocks() bool { return len(p.Mocks) > 0 }

// MockMode задаёт стратегию подготовки моков.
type MockMode string

const (
	MockNone     MockMode = "none"
	MockMinimock MockMode = "minimock"
)

// FixtureMode задаёт стратегию генерации тестовых фикстур.
type FixtureMode string

const (
	FixtureHeuristic FixtureMode = "heuristic"
	FixtureLLM       FixtureMode = "llm"
	FixtureHybrid    FixtureMode = "hybrid"
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
	Expr    string
	TypeStr string

	// NeedsMockComment — render добавит TODO-комментарий для interface-типов,
	// где Expr == "nil" является синтаксической заглушкой.
	NeedsMockComment bool
}

// ScenarioSpec представляет одну строку в table-driven тесте.
type ScenarioSpec struct {
	Name      string
	Kind      ScenarioKind
	Comment   string
	Inputs    []FixtureValue
	Wants     []FixtureValue
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
	PackageImportPath string
	SourceDir         string
	Tests             []TestSpec
	MockMode          MockMode
}
