# testgen Makefile
# Использование:
#   make download  — скачать зависимости (go.sum включён в репозиторий)
#   make test      — запустить все тесты
#   make build     — собрать бинарь
#   make generate  — сгенерировать тесты для example-пакетов
#   make validate  — сгенерировать + проверить компиляцию
#   make all       — download + test + build

BINARY := testgen
EXAMPLES := ./example/calculator ./example/advanced

.PHONY: all download test build generate validate clean

all: download test build

# download: загрузить зависимости согласно go.sum (не изменяет go.sum)
download:
	go mod download

test:
	go test ./...

build:
	go build -o $(BINARY) ./cmd/testgen

# generate: запускает testgen для всех example-пакетов
generate: build
	@for pkg in $(EXAMPLES); do \
		echo "==> $$pkg"; \
		./$(BINARY) -v $$pkg; \
	done

# validate: generate + компиляционная проверка сгенерированных файлов
validate: build
	@for pkg in $(EXAMPLES); do \
		echo "==> $$pkg"; \
		./$(BINARY) -validate -v $$pkg; \
	done

clean:
	rm -f $(BINARY)
	find ./example -name "testgen_generated_test.go" -delete
