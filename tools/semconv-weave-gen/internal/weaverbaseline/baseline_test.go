package weaverbaseline

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestNormalizeDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "empty", want: "[]\n"},
		{name: "whitespace", input: " \n\t", want: "[]\n"},
		{name: "array", input: `[{"id":"x"}]`, want: "[{\"id\":\"x\"}]\n"},
		{name: "object", input: `{}`, wantErr: "JSON array"},
		{name: "progress", input: `[]progress`, wantErr: "trailing"},
		{name: "two values", input: `[] []`, wantErr: "trailing"},
		{name: "malformed", input: `[`, wantErr: "decode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeDiagnostics([]byte(tt.input))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("NormalizeDiagnostics() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeDiagnostics() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("NormalizeDiagnostics() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandSpecifications(t *testing.T) {
	paths := commandPaths{
		weaver:         "/stage/weaver",
		sourceRegistry: "/stage/registry/source",
		targetRegistry: "/stage/registry/target",
		policy:         "/inputs/policy",
		templates:      "/inputs/templates",
		projectConfig:  "/stage/config/.weaver.toml",
		output:         "/stage/output",
		diff:           "/stage/output/diff/v1.26.0-to-v1.41.0",
	}
	got := commandSpecifications(paths)
	wantKinds := []commandKind{commandCheck, commandGenerate, commandDiff}
	if len(got) != len(wantKinds) {
		t.Fatalf("commandSpecifications() length = %d, want %d", len(got), len(wantKinds))
	}
	for i, spec := range got {
		if spec.kind != wantKinds[i] {
			t.Fatalf("command %d kind = %q, want %q", i, spec.kind, wantKinds[i])
		}
		joined := strings.Join(spec.args, " ")
		for _, fixed := range []string{
			"--quiet",
			"--follow-symlinks=false",
			"--include-unreferenced=false",
			"--v2=false",
			"--diagnostic-format=json",
			"--diagnostic-stdout=true",
		} {
			if !strings.Contains(joined, fixed) {
				t.Errorf("%s args missing %q: %v", spec.kind, fixed, spec.args)
			}
		}
		if strings.Contains(joined, "resolve") {
			t.Errorf("%s args contain forbidden resolve: %v", spec.kind, spec.args)
		}
	}
	diffArgs := strings.Join(got[len(got)-1].args, " ")
	if !strings.Contains(diffArgs, "--baseline-registry /stage/registry/source") ||
		!strings.Contains(diffArgs, "--registry /stage/registry/target") {
		t.Fatalf("diff does not bind exact source/target registries: %s", diffArgs)
	}
	if err := validateCommand(commandSpec{kind: "resolve", args: []string{"registry", "resolve"}}); err == nil {
		t.Fatal("validateCommand(resolve) error = nil")
	}
}

func TestValidateArchivePaths(t *testing.T) {
	for _, bad := range []string{
		"../outside\n",
		"/absolute\n",
		"ok/../../outside\n",
		"ok\\windows\n",
		"\n",
	} {
		if err := validateArchivePaths([]byte(bad)); err == nil {
			t.Errorf("validateArchivePaths(%q) error = nil", bad)
		}
	}
	if err := validateArchivePaths([]byte("root/\nroot/weaver\n")); err != nil {
		t.Fatalf("validateArchivePaths(valid) error = %v", err)
	}
}

func TestSelectAsset(t *testing.T) {
	asset, err := selectAsset("darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if asset.Platform != "darwin/arm64" || !strings.HasSuffix(asset.Path, "apple-darwin.tar.xz") {
		t.Fatalf("selectAsset(darwin/arm64) = %#v", asset)
	}
	if _, err := selectAsset("windows", "amd64"); err == nil {
		t.Fatal("selectAsset(windows/amd64) error = nil")
	}
}

func TestNormalizeProcessOutput(t *testing.T) {
	if _, err := normalizeProcessOutput(nil, nil, errors.New("exit 1")); err == nil || !strings.Contains(err.Error(), "process failed") {
		t.Fatalf("process error = %v", err)
	}
	if _, err := normalizeProcessOutput(nil, []byte("progress\n"), nil); err == nil || !strings.Contains(err.Error(), "stderr") {
		t.Fatalf("stderr error = %v", err)
	}
	if _, err := normalizeProcessOutput([]byte("{}"), nil, nil); err == nil || !strings.Contains(err.Error(), "JSON array") {
		t.Fatalf("stdout framing error = %v", err)
	}
}

func TestExtractRegistryArchiveRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		fileType byte
	}{
		{name: "traversal", fileName: "../outside", fileType: tar.TypeReg},
		{name: "absolute", fileName: "/outside", fileType: tar.TypeReg},
		{name: "symlink", fileName: "root/link", fileType: tar.TypeSymlink},
		{name: "hardlink", fileName: "root/link", fileType: tar.TypeLink},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var archive bytes.Buffer
			writer := tar.NewWriter(&archive)
			if err := writer.WriteHeader(&tar.Header{
				Name:     tt.fileName,
				Typeflag: tt.fileType,
				Mode:     0o644,
			}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			err := extractRegistryArchive(tar.NewReader(bytes.NewReader(archive.Bytes())), t.TempDir())
			if err == nil {
				t.Fatal("extractRegistryArchive() error = nil")
			}
		})
	}
}

func TestNormalizeConfigRejectsOutputOverlappingInputs(t *testing.T) {
	root := t.TempDir()
	inputs := filepath.Join(root, "build", "weaver", "inputs")
	if err := os.MkdirAll(inputs, 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		output string
	}{
		{name: "output inside inputs", output: filepath.Join(inputs, "artifacts")},
		{name: "output equals inputs", output: inputs},
		{name: "output is direct parent of inputs", output: filepath.Dir(inputs)},
		{name: "output is distant ancestor of inputs", output: root},
		{name: "unclean output ancestor of inputs", output: filepath.Join(inputs, "..", "..")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeConfig(Config{Inputs: inputs, Output: tt.output})
			if err == nil {
				t.Fatalf("normalizeConfig(output=%q) error = nil, want overlap rejection", tt.output)
			}
			if !strings.Contains(err.Error(), "must not overlap the checksum-locked inputs tree") {
				t.Fatalf("normalizeConfig(output=%q) error = %v, want inputs-tree overlap rejection", tt.output, err)
			}
		})
	}

	// A symlinked prefix must not let an output that resolves to an ancestor of
	// inputs slip past a purely lexical comparison: promoteTree would rename the
	// whole resolved tree away and then delete the backup.
	t.Run("output reaches an ancestor of inputs through a symlink", func(t *testing.T) {
		base := t.TempDir()
		link := filepath.Join(base, "link")
		if err := os.Symlink(root, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		_, err := normalizeConfig(Config{Inputs: inputs, Output: filepath.Join(link, "build")})
		if err == nil {
			t.Fatal("normalizeConfig(symlinked ancestor output) error = nil, want overlap rejection")
		}
		if !strings.Contains(err.Error(), "must not overlap the checksum-locked inputs tree") {
			t.Fatalf("normalizeConfig(symlinked ancestor output) error = %v", err)
		}
	})

	sibling := filepath.Join(root, "build", "weaver", "artifacts")
	cfg, err := normalizeConfig(Config{Inputs: inputs, Output: sibling})
	if err != nil {
		t.Fatalf("normalizeConfig(sibling output) error = %v", err)
	}
	if cfg.Output != sibling {
		t.Fatalf("normalizeConfig() output = %q, want %q", cfg.Output, sibling)
	}
}

func TestCompareTrees(t *testing.T) {
	left := t.TempDir()
	right := t.TempDir()
	writeTestFile(t, filepath.Join(left, "a", "artifact.json"), []byte("[]\n"))
	writeTestFile(t, filepath.Join(right, "a", "artifact.json"), []byte("[]\n"))
	if err := CompareTrees(left, right); err != nil {
		t.Fatalf("CompareTrees(equal) error = %v", err)
	}
	writeTestFile(t, filepath.Join(right, "a", "artifact.json"), []byte("[1]\n"))
	if err := CompareTrees(left, right); err == nil || !strings.Contains(err.Error(), "bytes differ") {
		t.Fatalf("CompareTrees(different) error = %v", err)
	}
}

func TestCanonicalJSON(t *testing.T) {
	value := map[string]any{
		"z": []any{"b", "a"},
		"a": map[string]any{"y": 2, "x": 1},
	}
	got, err := canonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": {\n    \"x\": 1,\n    \"y\": 2\n  },\n  \"z\": [\n    \"b\",\n    \"a\"\n  ]\n}\n"
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("canonicalJSON() = %s, want %s", got, want)
	}
	var decoded map[string]any
	if err := strictJSON(got, &decoded); err != nil {
		t.Fatalf("strictJSON(canonical) error = %v", err)
	}
	if !reflect.DeepEqual(decoded["z"], []any{"b", "a"}) {
		t.Fatalf("array order changed: %#v", decoded["z"])
	}
}

func TestSortWeaverDiffChanges(t *testing.T) {
	value := map[string]any{
		"changes": map[string]any{
			"registry_attributes": []any{
				map[string]any{"type": "renamed", "old_name": "z", "new_name": "a"},
				map[string]any{"type": "added", "name": "a"},
			},
			"spans": []any{},
		},
	}
	if err := sortWeaverDiffChanges(value); err != nil {
		t.Fatal(err)
	}
	items := value["changes"].(map[string]any)["registry_attributes"].([]any)
	if got := items[0].(map[string]any)["type"]; got != "added" {
		t.Fatalf("first sorted change type = %v, want added", got)
	}
}

func TestPinnedPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real pinned Weaver pipeline in short mode")
	}
	if _, err := selectAsset(runtime.GOOS, runtime.GOARCH); err != nil {
		t.Skip(err)
	}
	output := filepath.Join(t.TempDir(), "weaver")
	if err := Run(context.Background(), Config{
		Inputs: fixtureRoot(t),
		Output: output,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, rel := range []string{
		"findings-check.json",
		"findings-generate.json",
		"findings-diff.json",
		"registry-model.json",
		"registry-review.md",
		"diff/v1.26.0-to-v1.41.0/diff.json",
		"provenance.json",
	} {
		if info, err := os.Stat(filepath.Join(output, filepath.FromSlash(rel))); err != nil {
			t.Errorf("artifact %q: %v", rel, err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("artifact %q is not regular", rel)
		}
	}
	for _, rel := range []string{"findings-check.json", "findings-generate.json", "findings-diff.json"} {
		if got := string(readTestFile(t, filepath.Join(output, rel))); got != "[]\n" {
			t.Errorf("%s = %q, want [] LF", rel, got)
		}
	}
	model := readTestFile(t, filepath.Join(output, "registry-model.json"))
	if bytes.Contains(model, []byte(output)) || bytes.Contains(model, []byte(".weaver-baseline-stage-")) {
		t.Fatal("registry model leaks a staging path")
	}
	provenance := readTestFile(t, filepath.Join(output, "provenance.json"))
	if bytes.Contains(provenance, []byte(output)) || bytes.Contains(provenance, []byte(".weaver-baseline-stage-")) {
		t.Fatal("provenance leaks a staging path")
	}
	if !bytes.Contains(provenance, []byte(`"registry check"`)) ||
		!bytes.Contains(provenance, []byte(`"registry generate"`)) ||
		!bytes.Contains(provenance, []byte(`"registry diff"`)) {
		t.Fatal("provenance does not record all allowed Weaver commands")
	}
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "testdata"))
}
