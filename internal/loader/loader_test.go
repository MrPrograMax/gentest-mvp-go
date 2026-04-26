package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yourorg/testgen/internal/loader"
)

// writeModule создаёт минимальный Go-модуль в директории,
// необходимый для работы packages.Load.
func writeModule(t *testing.T, dir string) {
	t.Helper()
	gomod := "module example.com/testpkg\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_directory(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir)

	src := `package mypkg

func Hello() string { return "hi" }
`
	if err := os.WriteFile(filepath.Join(dir, "hello.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := loader.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	pkg, ok := result.PrimaryPackage()
	if !ok {
		t.Fatal("ожидался основной пакет")
	}
	if pkg.Name != "mypkg" {
		t.Errorf("Name = %q, want mypkg", pkg.Name)
	}
}

func TestLoad_singleFile(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir)

	src := `package mypkg

func Bye() {}
`
	fp := filepath.Join(dir, "bye.go")
	if err := os.WriteFile(fp, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := loader.Load(fp)
	if err != nil {
		t.Fatalf("Load(file): %v", err)
	}
	// Директория должна совпадать с abs-путём исходной директории.
	if result.Dir != dir {
		t.Errorf("Dir = %q, want %q", result.Dir, dir)
	}
}

func TestLoad_noGoFiles(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir)
	// Нет .go файлов → ошибка.
	_, err := loader.Load(dir)
	if err == nil {
		t.Fatal("ожидалась ошибка для пустой директории")
	}
}
