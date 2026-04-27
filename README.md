# gentest-mvp-go

MVP-инструмент для генерации компилируемых scaffold unit-тестов для Go-кода.

Инструмент анализирует Go-пакет, извлекает экспортируемые функции и методы, строит базовые сценарии выполнения, подбирает входные данные, генерирует `table-driven` тесты и проверяет, что результат компилируется.

---

## Что делает

- загружает Go-пакет через `go/packages`;
- анализирует функции и методы через `go/types` и AST;
- строит сценарии `success`, `error`, `edge_nil_*`, `edge_empty_*`;
- генерирует `table-driven` тесты;
- подбирает базовые фикстуры для типовых Go-типов;
- добавляет нужные импорты;
- форматирует сгенерированный код;
- валидирует компиляцию через `go test -run ^$ .`;
- поддерживает Minimock и генерирует моки в отдельную папку `mock/`.

---

## Быстрый старт

Установить зависимости и проверить проект:

```bash
go mod tidy
go test ./...
```

Сгенерировать тесты для простого demo-пакета:

```bash
go run ./cmd/testgen -validate ./example/calculator
```

Сгенерировать тесты для demo-пакета с `context.Context`, `io.Reader`, pointers и slices:

```bash
go run ./cmd/testgen -validate ./example/advanced
```

Сгенерировать тесты с Minimock:

```bash
go run ./cmd/testgen --mock=minimock -validate ./example/service
```

---

## Пример результата

Для функции:

```go
func Divide(a, b int) (int, error)
```

генератор создает table-driven scaffold с несколькими сценариями:

```go
func TestDivide(t *testing.T) {
	tests := []struct {
		name    string
		inputA  int
		inputB  int
		wantErr bool
	}{
		{
			name:    "Divide/success",
			inputA:  42,
			inputB:  42,
			wantErr: false,
		},
		{
			name:    "Divide/error",
			inputA:  42,
			inputB:  0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got0, err := Divide(tt.inputA, tt.inputB)

			if (err != nil) != tt.wantErr {
				t.Errorf("got err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			_ = got0 // TODO: verify got0 with business-specific expected value
		})
	}
}
```

Для обычных возвращаемых значений генератор не делает фейковые `expected`-assertions. Вместо этого он оставляет TODO для ручной бизнес-проверки.

---

## Minimock

В режиме:

```bash
go run ./cmd/testgen --mock=minimock -validate ./example/service
```

инструмент:

- находит интерфейсные зависимости receiver-структуры;
- создает подпапку `mock/`;
- генерирует mock-файлы через Minimock;
- генерирует тест во внешнем package `<pkg>_test`;
- подключает `minimock.Controller`;
- создает `prepare func(m *testMocks)` для ручной настройки expectations.

Пример структуры:

```text
example/service/
  service.go
  testgen_generated_test.go
  mock/
    user_repository_mock.go
```

Пример mock-инфраструктуры:

```go
type testMocks struct {
	repo *mock.UserRepositoryMock
}

mc := minimock.NewController(t)

m := &testMocks{
	repo: mock.NewUserRepositoryMock(mc),
}

rcv := service.NewService(m.repo)
```

`Expect(...).Return(...)` в текущей MVP-версии заполняется вручную:

```go
prepare: func(m *testMocks) {
	m.repo.GetByIDMock.
		Expect(context.Background(), int64(42)).
		Return(service.User{ID: 42, Name: "Alice"}, nil)
},
```

---

## CLI

```bash
go run ./cmd/testgen [flags] <package>
```

Флаги:

```text
-validate
    выполнить compile-only проверку после генерации

-v
    подробный вывод

--mock=none|minimock
    режим работы с моками
```

---

## Архитектура

Основной pipeline:

```text
loader -> analyzer -> scenario -> fixture -> mockplan/mockgen -> render -> validator
```

Ключевые пакеты:

```text
cmd/testgen          CLI-точка входа
internal/loader      загрузка Go-пакетов
internal/analyzer    анализ функций, методов и guard-фактов
internal/scenario    генерация сценариев
internal/fixture     генерация входных данных
internal/mockplan    построение плана мокирования
internal/mockgen     запуск Minimock
internal/render      генерация _test.go
internal/validator   проверка компиляции
```

---

## Ограничения MVP

- не вычисляет семантически корректные `expected`-значения;
- не генерирует business assertions автоматически;
- не заполняет Minimock expectations автоматически;
- не использует LLM;
- не использует CFG/SSA;
- генерирует scaffold, а не полностью готовые бизнес-тесты.

---

## Roadmap

Планируемые направления развития:

- LLM fixture provider;
- автоматическая генерация mock expectations;
- CFG/SSA-анализ;
- интеграционные тесты repository-layer;
- метрики качества генерации: compile rate, pass rate, TODO count, manual edit distance.

---

## Назначение проекта

Проект демонстрирует подход к автоматизации рутинной части написания unit-тестов в Go.

Инструмент не заменяет разработчика полностью, а снимает шаблонную работу: анализ сигнатур, подготовку table-driven структуры, генерацию входных данных, подключение моков и проверку компиляции. Разработчик остается ответственным за бизнес-assertions и смысловую корректность тестов.
