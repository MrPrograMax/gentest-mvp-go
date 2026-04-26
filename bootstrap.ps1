# bootstrap.ps1 — первоначальная настройка testgen на Windows
#
# Использование:
#   .\bootstrap.ps1
#
# Требования:
#   - Go 1.21+ установлен и в PATH
#   - Доступ к https://proxy.golang.org (для go mod tidy)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Write-Host "==> testgen bootstrap" -ForegroundColor Cyan

# 1. Проверяем Go
try {
    $goVersion = (go version 2>&1)
    Write-Host "✓ Go найден: $goVersion" -ForegroundColor Green
} catch {
    Write-Host "ОШИБКА: Go не найден. Установи Go 1.21+ с https://go.dev/dl/" -ForegroundColor Red
    exit 1
}

# 2. go mod tidy — скачивает зависимости и создаёт go.sum
Write-Host "`n==> go mod tidy (создание go.sum)..."
go mod tidy
if ($LASTEXITCODE -ne 0) {
    Write-Host "ОШИБКА: go mod tidy завершился с кодом $LASTEXITCODE" -ForegroundColor Red
    exit 1
}
Write-Host "✓ go.sum создан" -ForegroundColor Green

# 3. gofmt — форматирование всех Go-файлов
Write-Host "`n==> gofmt -w ..."
$goFiles = Get-ChildItem -Recurse -Filter "*.go" | Select-Object -ExpandProperty FullName
foreach ($f in $goFiles) {
    gofmt -w $f
}
Write-Host "✓ gofmt применён" -ForegroundColor Green

# 4. Проверка форматирования (должен быть пустой вывод)
Write-Host "`n==> gofmt -l (проверка)..."
$unformatted = $goFiles | ForEach-Object { gofmt -l $_ } | Where-Object { $_ -ne "" }
if ($unformatted) {
    Write-Host "Файлы с нарушением форматирования:" -ForegroundColor Yellow
    $unformatted | ForEach-Object { Write-Host "  $_" }
} else {
    Write-Host "✓ Все файлы отформатированы" -ForegroundColor Green
}

# 5. go test ./...
Write-Host "`n==> go test ./..."
go test ./...
if ($LASTEXITCODE -ne 0) {
    Write-Host "ОШИБКА: тесты не прошли" -ForegroundColor Red
    exit 1
}
Write-Host "✓ Все тесты прошли" -ForegroundColor Green

# 6. Сборка бинаря
Write-Host "`n==> go build -o testgen.exe ./cmd/testgen"
go build -o testgen.exe ./cmd/testgen
if ($LASTEXITCODE -ne 0) {
    Write-Host "ОШИБКА: сборка завершилась ошибкой" -ForegroundColor Red
    exit 1
}
Write-Host "✓ Бинарь собран: testgen.exe" -ForegroundColor Green

Write-Host "`n==> Готово! Запусти:" -ForegroundColor Cyan
Write-Host "  .\testgen.exe -validate -v .\example\calculator\"
Write-Host "  .\testgen.exe -validate -v .\example\advanced\"
