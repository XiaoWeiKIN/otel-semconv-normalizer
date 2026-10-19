package semconvweave

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHandWrittenPackageBuildsWithoutGeneratedFiles(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve package directory")
	}
	packageDir := filepath.Dir(file)
	repositoryRoot := filepath.Dir(packageDir)
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") ||
			strings.HasPrefix(entry.Name(), "zz_generated_") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(packageDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, entry.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rootGoMod, err := os.ReadFile(filepath.Join(repositoryRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	module := strings.Replace(
		string(rootGoMod),
		"module github.com/XiaoWeiKIN/otel-semconv-normalizer",
		"module semconvweave.baseline.test",
		1,
	)
	module += "\nrequire github.com/XiaoWeiKIN/otel-semconv-normalizer v0.0.0\n\n" +
		"replace github.com/XiaoWeiKIN/otel-semconv-normalizer => " +
		filepath.ToSlash(repositoryRoot) + "\n"
	if err := os.WriteFile(filepath.Join(target, "go.mod"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	rootGoSum, err := os.ReadFile(filepath.Join(repositoryRoot, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "go.sum"), rootGoSum, 0o644); err != nil {
		t.Fatal(err)
	}
	baselineTest := `package semconvweave

import "testing"

func TestGeneratedDataFreeBaseline(t *testing.T) {
	if Resolve("") != WeaveMissing || Resolve(canonicalSchemaURL) != WeaveCanonical {
		t.Fatal("baseline reserved URL behavior changed")
	}
	if Resolve("https://opentelemetry.io/schemas/1.7.0") != WeaveUnknown {
		t.Fatal("baseline inferred a generated legacy URL")
	}
	if WeaveID(3).State() != SchemaUnknown {
		t.Fatal("baseline accepted an absent generated ID")
	}
}
`
	if err := os.WriteFile(filepath.Join(target, "baseline_test.go"), []byte(baselineTest), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "test", "-mod=readonly", "./...")
	command.Dir = target
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated-data-free package failed: %v\n%s", err, output)
	}
}
