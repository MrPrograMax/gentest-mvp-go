package mockgen_test

import (
	"strings"
	"testing"

	"github.com/yourorg/testgen/internal/mockgen"
	"github.com/yourorg/testgen/internal/model"
)

// makeSpec создаёт MockSpec с заданным PackageDir для тестов OutputArg.
func makeSpec(packageDir, mockFileName string) model.MockSpec {
	return model.MockSpec{
		InterfaceName: "UserRepository",
		MockType:      "UserRepositoryMock",
		MockPackage:   "mock",
		MockFileName:  mockFileName,
		PackageDir:    packageDir,
		MockDir:       packageDir + "/mock",
	}
}

// TestOutputArg_relativeNoAbsolutePrefix проверяет, что OutputArg
// никогда не возвращает абсолютный путь — ни на Linux ни на Windows.
func TestOutputArg_relativeNoAbsolutePrefix(t *testing.T) {
	cases := []struct {
		name       string
		packageDir string
		fileName   string
	}{
		{
			name:       "linux absolute path",
			packageDir: "/home/user/project/example/service",
			fileName:   "user_repository_mock.go",
		},
		{
			name:       "windows absolute path C:",
			packageDir: `C:\Users\MrPrograMax\Desktop\gentest-mvp-go\example\service`,
			fileName:   "user_repository_mock.go",
		},
		{
			name:       "windows absolute path D:",
			packageDir: `D:\Projects\myapp\pkg\svc`,
			fileName:   "http_client_mock.go",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec := makeSpec(c.packageDir, c.fileName)
			got := mockgen.OutputArg(spec)

			// Результат не должен начинаться с "./C:" или вообще с "/"
			if strings.HasPrefix(got, "/") {
				t.Errorf("OutputArg(%q) = %q: не должен начинаться с /", c.packageDir, got)
			}
			// Не должен содержать двоеточие (признак Windows-абсолютного пути)
			if strings.Contains(got, ":") {
				t.Errorf("OutputArg(%q) = %q: не должен содержать ':' (абсолютный Windows-путь)", c.packageDir, got)
			}
			// Не должен начинаться с "./" (которое Windows не понимает для абсолютных путей)
			if strings.HasPrefix(got, "./") {
				t.Errorf("OutputArg(%q) = %q: не должен начинаться с ./", c.packageDir, got)
			}
			// Должен начинаться с "mock/"
			if !strings.HasPrefix(got, "mock/") {
				t.Errorf("OutputArg(%q) = %q: должен начинаться с mock/", c.packageDir, got)
			}
			// Должен заканчиваться на имя файла
			if !strings.HasSuffix(got, c.fileName) {
				t.Errorf("OutputArg(%q) = %q: должен заканчиваться на %q", c.packageDir, got, c.fileName)
			}
		})
	}
}

func TestOutputArg_forwardSlash(t *testing.T) {
	// Путь всегда в forward-slash формате (и на Windows).
	spec := makeSpec(`C:\repo\service`, "user_repository_mock.go")
	got := mockgen.OutputArg(spec)
	if strings.Contains(got, `\`) {
		t.Errorf("OutputArg = %q: не должен содержать обратные слэши", got)
	}
	if got != "mock/user_repository_mock.go" {
		t.Errorf("OutputArg = %q, want mock/user_repository_mock.go", got)
	}
}

func TestOutputArg_differentFiles(t *testing.T) {
	cases := []struct {
		fileName string
		want     string
	}{
		{"user_repository_mock.go", "mock/user_repository_mock.go"},
		{"http_client_mock.go", "mock/http_client_mock.go"},
		{"bki_transport_mock.go", "mock/bki_transport_mock.go"},
	}
	for _, c := range cases {
		spec := makeSpec("/abs/pkg", c.fileName)
		got := mockgen.OutputArg(spec)
		if got != c.want {
			t.Errorf("OutputArg(fileName=%q) = %q, want %q", c.fileName, got, c.want)
		}
	}
}
