# testgen — генератор unit-тестов для Go

`testgen` анализирует экспортируемый Go-пакет и генерирует table-driven `*_test.go`
со сценариями success / error / edge.

Сгенерированный файл компилируется сразу. Для не-error результатов генерируется
`_ = gotN // TODO: verify gotN with business-specific expected value` —
пользователь сам добавляет конкретные проверки, зная бизнес-логику функции.

---

## Быстрый старт

```bash
# 1. Загрузить зависимости
go mod download

# 2. Собрать
go build -o testgen ./cmd/testgen          # Linux/macOS
go build -o testgen.exe ./cmd/testgen      # Windows

# 3. Сгенерировать тесты для примеров
./testgen -v ./example/calculator/
./testgen -v ./example/advanced/

# 4. Проверить компиляцию сгенерированного файла
./testgen -validate -v ./example/advanced/

# Makefile:
make build     # go build
make test      # go test ./...
make validate  # generate + compile check
```

---

## Структура проекта

```
testgen/
├── Makefile
├── go.mod
├── go.sum
├── README.md
├── cmd/testgen/main.go             ← CLI точка входа
├── internal/
│   ├── model/model.go              ← общие типы данных
│   ├── loader/loader.go            ← загрузчик: golang.org/x/tools/go/packages
│   ├── analyzer/analyzer.go        ← go/types + AST → []FunctionSpec + Guards
│   ├── fixture/fixture.go          ← фикстуры: Happy / Zero / Empty
│   ├── scenario/scenario.go        ← ScenarioSpec по Guards-фактам
│   ├── mockplan/mockplan.go        ← MockPlan по полям receiver-структуры
│   ├── render/render.go            ← text/template + go/format → []byte
│   ├── validator/validator.go      ← go test -run ^$ .
│   └── app/app.go                  ← оркестратор пайплайна
├── templates/test.go.tmpl          ← справочная копия шаблона
└── example/
    ├── calculator/calculator.go    ← простые функции (без зависимостей)
    ├── advanced/advanced.go        ← context.Context, io.Reader, *int, []string
    └── service/service.go          ← Service + UserRepository (для --mock=minimock)
```

---

## Флаги CLI

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `-o <file>` | автоматически | Путь для записи `*_test.go` |
| `-validate` | false | Компиляционная проверка через `go test -run ^$ .` |
| `-v` | false | Подробное логирование |
| `--mock=MODE` | `none` | Стратегия моков: `none` \| `minimock` |

---

## Что генерируется

Для каждой экспортируемой функции или метода генерируется до трёх сценариев:

| Сценарий | Когда | Фикстура |
|----------|-------|----------|
| `success` | всегда | happy-значения для всех параметров |
| `error` | функция возвращает `error` | нулевые значения, `wantErr: true` |
| `edge_nil_<p>` | Guards: `p == nil` найдено в теле | `nil` для p |
| `edge_empty_<p>` | Guards: `p == ""` / `len(p) == 0` найдено в теле | `""` / `[]T{}` / `map[K]V{}` |

### Assertion-стратегия

- **error** — проверяется реально: `if (err != nil) != tt.wantErr { t.Errorf(...) }`.
- **не-error результаты** — placeholder не используется. Вместо `reflect.DeepEqual`
  генерируется `_ = got0 // TODO: verify got0 with business-specific expected value`.
  Пользователь сам добавляет нужную проверку.

### Дедупликация edge-сценариев

Сравниваются `Inputs.Expr`, `Wants.Expr`, `WantError`. Если edge полностью
совпадает с error — пропускается. Ключевой кейс для `[]string` с одним параметром:

| Сценарий | items | Пропуск? |
|----------|-------|----------|
| error | `nil` (Zero) | нет |
| edge_nil | `nil` (Zero) | **да** — дубликат error |
| edge_empty | `[]string{}` (Empty) | **нет** — уникален |

---

## Фикстуры

| TypeKind | `Happy` | `Zero` |
|----------|---------|--------|
| string | `"test-value"` | `""` |
| int | `42` | `0` |
| bool | `true` | `false` |
| `[]T` | `[]T{"test-value"}` | `nil` |
| `map[K]V` | `map[K]V{}` | `nil` |
| `*T` | `new(T)` | `nil` |
| `time.Time` | `time.Now()` | `time.Time{}` |
| `time.Duration` | `time.Second` | `0` |
| `context.Context` | `context.Background()` | `nil` |
| `io.Reader` | `strings.NewReader("test-value")` | `nil` |
| func | safe stub (zero-returns) | `nil` |
| interface | `nil` + TODO comment | `nil` |

`new(T)` работает для любого T, включая встроенные: `*int → new(int)`.

---

## Поддержка Minimock

`--mock=minimock` **автоматически генерирует** mock-файлы через `go run` и размещает
их в поддиректории `mock/` анализируемого пакета:

```
example/service/mock/user_repository_mock.go   (package mock)
```

### Почему external test package

При `--mock=minimock` тест генерируется в **external test package** (`package service_test`),
а не в `package service`. Это решает import cycle:

```
package service       # исходный код
package service/mock  # моки — импортируют service (для типов UserRepository, User)
package service_test  # тест — может импортировать и service, и service/mock без цикла
```

Если бы тест был в `package service`, возник бы цикл:
```
service → service/mock → service   ← ОШИБКА
```

### Что генерируется

**`example/service/mock/user_repository_mock.go`** (package mock, сгенерирован minimock):
```go
package mock
type UserRepositoryMock struct { ... }
func NewUserRepositoryMock(t minimock.Tester) *UserRepositoryMock { ... }
```

**`example/service/testgen_generated_test.go`** (package service_test):
```go
package service_test

import (
    "context"
    "github.com/gojuno/minimock/v3"
    mock    "github.com/yourorg/testgen/example/service/mock"
    service "github.com/yourorg/testgen/example/service"
    "testing"
)

type testMocks struct {
    repo *mock.UserRepositoryMock
}

func TestService_GetUserName(t *testing.T) {
    tests := []struct {
        name      string
        inputCtx  context.Context
        inputId   int64
        wantErr   bool
        prepare   func(m *testMocks)
    }{
        {
            name: "GetUserName/success",
            inputCtx: context.Background(),
            inputId:  42,
            wantErr:  false,
            prepare: func(m *testMocks) {
                // TODO: configure minimock expectations for this scenario
            },
        },
        // ... error и edge сценарии
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mc := minimock.NewController(t)
            m := &testMocks{
                repo: mock.NewUserRepositoryMock(mc),
            }
            if tt.prepare != nil {
                tt.prepare(m)
            }
            rcv := service.NewService(m.repo)   // ← constructor из source пакета
            got0, err := rcv.GetUserName(tt.inputCtx, tt.inputId)
            ...
        })
    }
}
```

### Запуск

```bash
# Одна команда: генерирует моки, scaffold, проверяет компиляцию
go run ./cmd/testgen --mock=minimock -validate ./example/service

# Затем — заполни prepare-ожидания и запусти тесты
go test ./example/service/...
```

### Заполнение prepare

```go
prepare: func(m *testMocks) {
    m.repo.GetByIDMock.
        Expect(context.Background(), 42).
        Return(service.User{ID: 42, Name: "Alice"}, nil)
},
```

---

## Архитектура (pipeline)

```
Target path
    │
    ▼
loader.Load         golang.org/x/tools/go/packages → AST + TypesInfo
    │
    ▼
analyzer.Analyze    go/types + AST-тело → []FunctionSpec + Guards + ReceiverFields
    │
    ▼
mockplan.Analyze    ReceiverFields → MockPlan (при --mock=minimock)
    │                 (FieldName, InterfaceName, MockImportPath, MockFilePath, ...)
    ▼
mockgen.Generate    go run minimock@v3.4.7 → <pkg>/mock/<name>_mock.go
    │
    ▼
scenario.Generate   Guards → ScenarioSpec (success / error / edge_nil / edge_empty)
    │
    ▼
render.RenderFile   text/template + planImports + go/format → []byte
    │               (mock.MockType, mock.Constructor, import mock-пакета)
    ▼
os.WriteFile        *_test.go
    │
    ▼ (если -validate)
validator.Validate  go test -run ^$ .
```

---

## Ограничения MVP

- **Interface-параметры** (кроме `context.Context` и `io.Reader`) → `nil` с TODO.
- `--mock=minimock` автоматически генерирует mock-файлы в поддиректорию `mock/`.
- `.Expect(...).Return(...)` в `prepare` заполняются разработчиком вручную.
- `-validate` проверяет компиляцию сгенерированного scaffold, но не запускает сами тесты.
- **Import planner** использует `strings.Contains` — возможны ложные срабатывания для нестандартных имён.
