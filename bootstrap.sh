#!/usr/bin/env bash
# bootstrap.sh — первоначальная настройка testgen на Linux/macOS
#
# Использование:
#   chmod +x bootstrap.sh && ./bootstrap.sh
#
# Требования:
#   - Go 1.21+ установлен и в PATH
#   - Доступ к https://proxy.golang.org (для go mod tidy)

set -euo pipefail

echo "==> testgen bootstrap"

# 1. Проверяем Go
if ! command -v go &>/dev/null; then
    echo "ОШИБКА: Go не найден. Установи Go 1.21+ с https://go.dev/dl/"
    exit 1
fi
echo "✓ Go найден: $(go version)"

# 2. go mod tidy — скачивает зависимости и создаёт go.sum
echo ""
echo "==> go mod tidy (создание go.sum)..."
go mod tidy
echo "✓ go.sum создан"

# 3. gofmt — форматирование всех Go-файлов
echo ""
echo "==> gofmt -w ./..."
find . -name "*.go" -not -path "./vendor/*" -exec gofmt -w {} +
echo "✓ gofmt применён"

# 4. Проверка форматирования (должен быть пустой вывод)
echo ""
echo "==> gofmt -l (проверка)..."
UNFORMATTED=$(find . -name "*.go" -not -path "./vendor/*" | xargs gofmt -l 2>/dev/null)
if [ -n "$UNFORMATTED" ]; then
    echo "Файлы с нарушением форматирования:"
    echo "$UNFORMATTED"
else
    echo "✓ Все файлы отформатированы (пустой вывод)"
fi

# 5. go test ./...
echo ""
echo "==> go test ./..."
go test ./...
echo "✓ Все тесты прошли"

# 6. Сборка бинаря
echo ""
echo "==> go build -o testgen ./cmd/testgen"
go build -o testgen ./cmd/testgen
echo "✓ Бинарь собран: ./testgen"

echo ""
echo "==> Готово! Запусти:"
echo "  ./testgen -validate -v ./example/calculator/"
echo "  ./testgen -validate -v ./example/advanced/"
