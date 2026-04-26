# testgen — генератор unit-тестов для Go

`testgen` анализирует экспортируемый Go-пакет и генерирует table-driven `*_test.go` файлы
со сценариями success / error / edge. Сгенерированный файл компилируется сразу;
пользователь заполняет только `want*`-поля ожидаемыми значениями.

---

## Быстрый старт

```bash
# 1. Загрузить модули (go.sum включён, но модули нужно скачать)
go mod download

# 2. Собрать
Для Linux/MacOS
go build -o testgen ./cmd/testgen

Для Windows
go build -o testgen.exe ./cmd/testgen

# 3. Сгенерировать тесты для примеров
./testgen -v ./example/calculator/
./testgen -v ./example/advanced/

# 4. Скомпилировать и проверить результат
./testgen -validate -v ./example/advanced/

# Или через Makefile:
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
├── README.md
├── cmd/testgen/main.go             ← CLI точка входа
├── internal/
│   ├── model/model.go              ← общие типы данных
│   ├── loader/loader.go            ← загрузчик на golang.org/x/tools/go/packages
│   ├── analyzer/analyzer.go        ← AST + go/types → []FunctionSpec + Guards
│   ├── fixture/fixture.go          ← фикстуры: Happy / Zero / Empty
│   ├── scenario/scenario.go        ← генерация ScenarioSpec по Guards
│   ├── mockplan/mockplan.go        ← диагностика interface-параметров
│   ├── render/render.go            ← text/template + go/format → []byte
│   ├── validator/validator.go      ← go test -run ^$ компиляционная проверка
│   └── app/app.go                  ← оркестратор пайплайна
├── templates/test.go.tmpl          ← справочная копия шаблона
└── example/
    ├── calculator/calculator.go    ← простой пример
    └── advanced/advanced.go        ← context.Context, io.Reader, *int, []string
```

---

## Флаги CLI

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `-o <file>` | автоматически | Путь для записи `*_test.go` |
| `-validate` | false | Скомпилировать вывод через `go test -run ^$ .` |
| `-v` | false | Подробное логирование |

---

## Что генерируется

Для каждой экспортируемой функции/метода генерируются сценарии:

| Сценарий | Когда | Фикстура |
|----------|-------|----------|
| `success` | всегда | happy-значения для всех параметров |
| `error` | функция возвращает `error` | нулевые значения, `wantErr: true` |
| `edge_nil_<p>` | Guards: `p == nil` в теле | `nil` для p |
| `edge_empty_<p>` | Guards: `p == ""` / `len(p) == 0` | `""` / `[]T{}` / `map[K]V{}` для p |

### Дедупликация edge-сценариев

Если содержимое `edge` полностью совпадает с `error` (те же Expr и WantError) — `edge` пропускается.
Ключевой кейс: для `[]string` с `emptyChecked`:
- `error`: `items=nil` (Zero)
- `edge_nil`: `items=nil` → дубликат → **пропускается**
- `edge_empty`: `items=[]string{}` → уникален → **сохраняется**

---

## Фикстуры (fixture package)

| TypeKind | `Happy` | `Zero` | `Empty` |
|----------|---------|--------|---------|
| string | `"test-value"` | `""` | `""` |
| int | `42` | `0` | — |
| bool | `true` | `false` | — |
| `[]T` | `[]T{"test-value"}` | `nil` | `[]T{}` |
| `map[K]V` | `map[K]V{}` | `nil` | `map[K]V{}` |
| `*T` | `new(T)` | `nil` | `nil` |
| `time.Time` | `time.Now()` | `time.Time{}` | — |
| `time.Duration` | `time.Second` | `0` | — |
| `context.Context` | `context.Background()` | `nil` | — |
| `io.Reader` | `strings.NewReader("test-value")` | `nil` | — |
| func | safe stub (zero returns) | `nil` | — |
| interface | `nil` + TODO comment | `nil` | — |

`new(T)` работает для любого T включая встроенные типы (`*int → new(int)`).

---

## Import planner

render автоматически определяет необходимые импорты:
- Сканирует `TypeStr` параметров и результатов (context., io., time., sql., http.)
- Сканирует `Expr` фикстур (strings.NewReader → "strings", context.Background → "context")
- Всегда добавляет "testing"; "reflect" — если есть не-error результаты

---

## Архитектура (pipeline)

```
Target path
    │
    ▼
loader.Load          golang.org/x/tools/go/packages → AST + TypesInfo
    │
    ▼
analyzer.Analyze     go/types классификация + AST-обход тела → FunctionSpec + Guards
    │
    ▼
scenario.Generate    Guards → осмысленные ScenarioSpec (success/error/edge)
    │
    ▼
fixture.Happy/Zero   TypeStr → конкретные Go-выражения
    │
    ▼
render.RenderFile    text/template + planImports + go/format → []byte
    │
    ▼
os.WriteFile         *_test.go
    │
    ▼ (если -validate)
validator.Validate   go test -run ^$ .
```

---

## Ограничения MVP

- **Interface-параметры** (кроме context.Context и io.Reader) генерируют `nil` с TODO-комментарием.
- **Метод-ресиверы** инициализируются как `&ReceiverType{}` — нужно заполнить вручную.
- **`want*`-поля** — всегда placeholder; пользователь должен задать ожидаемые значения.
- **Вложенные пакеты**: import planner использует `strings.Contains`, что может давать ложные срабатывания для нестандартных имён типов.
