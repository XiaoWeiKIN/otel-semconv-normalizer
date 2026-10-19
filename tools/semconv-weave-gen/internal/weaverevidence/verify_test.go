package weaverevidence

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVerifyPinnedEvidence(t *testing.T) {
	bundle, err := Verify(fixtureRoot(t), evidenceRoot(t))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if len(bundle.Model) == 0 || len(bundle.Diff) == 0 || !validDigest(bundle.EvidenceSHA256) {
		t.Fatalf("Verify() returned incomplete bundle: %#v", bundle)
	}
}

func TestVerifyRejectsBrokenEvidenceClosure(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*testing.T, string)
		wantErr string
	}{
		{
			name: "missing model",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "registry-model.json")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "file set",
		},
		{
			name: "tampered diff",
			mutate: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, filepath.FromSlash(diffPath)), []byte("{}\n"))
			},
			wantErr: "artifact",
		},
		{
			name: "nonempty findings",
			mutate: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "findings-check.json"), []byte("[{\"error\":\"x\"}]\n"))
			},
			wantErr: "artifact",
		},
		{
			name: "tampered provenance",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, "provenance.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data = []byte(strings.Replace(string(data), `"v1.26.0"`, `"latest"`, 1))
				writeFile(t, path, data)
			},
			wantErr: "source Registry identity",
		},
		{
			name: "extra file",
			mutate: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "extra.json"), []byte("{}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "file set",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := cloneTree(t, evidenceRoot(t))
			test.mutate(t, root)
			_, err := Verify(fixtureRoot(t), root)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Verify() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "testdata"))
}

func evidenceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "build", "weaver"))
}

func cloneTree(t *testing.T, source string) string {
	t.Helper()
	target := t.TempDir()
	err := filepath.WalkDir(source, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dst := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dst, 0o700)
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
