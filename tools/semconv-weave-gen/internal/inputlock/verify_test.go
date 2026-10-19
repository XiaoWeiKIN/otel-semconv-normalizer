package inputlock

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVerifyPinnedInputs(t *testing.T) {
	if err := Verify(fixtureRoot(t)); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*testing.T, string)
		wantErr string
	}{
		{
			name: "tampered bytes",
			mutate: func(t *testing.T, root string) {
				replaceFile(t, filepath.Join(root, "audit/required-source-urls.yaml"), []byte("tampered\n"))
			},
			wantErr: "checksum mismatch",
		},
		{
			name: "missing file",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "guides/rpc-migration.md")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "missing fixture",
		},
		{
			name: "extra file",
			mutate: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "extra.txt"), []byte("extra\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "unlisted fixture",
		},
		{
			name: "duplicate manifest path",
			mutate: func(t *testing.T, root string) {
				manifest := filepath.Join(root, "checksums.txt")
				data := readFile(t, manifest)
				first := strings.SplitN(string(data), "\n", 2)[0]
				replaceFile(t, manifest, append(data, []byte(first+"\n")...))
			},
			wantErr: "duplicate checksum path",
		},
		{
			name: "unsorted manifest",
			mutate: func(t *testing.T, root string) {
				manifest := filepath.Join(root, "checksums.txt")
				lines := readLines(t, manifest)
				lines[0], lines[1] = lines[1], lines[0]
				replaceFile(t, manifest, []byte(strings.Join(lines, "\n")+"\n"))
			},
			wantErr: "not sorted",
		},
		{
			name: "path traversal",
			mutate: func(t *testing.T, root string) {
				manifest := filepath.Join(root, "checksums.txt")
				lines := readLines(t, manifest)
				parts := strings.SplitN(lines[0], "  ", 2)
				lines[0] = parts[0] + "  ../outside"
				replaceFile(t, manifest, []byte(strings.Join(lines, "\n")+"\n"))
			},
			wantErr: "unsafe checksum path",
		},
		{
			name: "symlink",
			mutate: func(t *testing.T, root string) {
				if err := os.Symlink("guides/db-migration.md", filepath.Join(root, "linked-guide.md")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "symlink",
		},
		{
			name: "weaver identity mismatch",
			mutate: func(t *testing.T, root string) {
				lockPath := filepath.Join(root, "input-lock.json")
				data := strings.Replace(
					string(readFile(t, lockPath)),
					`"release": "v0.25.0"`,
					`"release": "latest"`,
					1,
				)
				replaceFile(t, lockPath, []byte(data))
				updateManifestChecksum(t, root, "input-lock.json")
			},
			wantErr: "weaver release",
		},
		{
			name: "schema identity mismatch",
			mutate: func(t *testing.T, root string) {
				lockPath := filepath.Join(root, "input-lock.json")
				data := strings.Replace(
					string(readFile(t, lockPath)),
					"d73002442a640ce74a185359843b9046884d16eec198c3576d2dcc779deaf740",
					strings.Repeat("0", 64),
					1,
				)
				replaceFile(t, lockPath, []byte(data))
				updateManifestChecksum(t, root, "input-lock.json")
			},
			wantErr: "schema sha256",
		},
		{
			name: "unknown lock field",
			mutate: func(t *testing.T, root string) {
				lockPath := filepath.Join(root, "input-lock.json")
				data := strings.Replace(string(readFile(t, lockPath)), "{\n", "{\n  \"unexpected\": true,\n", 1)
				replaceFile(t, lockPath, []byte(data))
				updateManifestChecksum(t, root, "input-lock.json")
			},
			wantErr: "unknown field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := cloneFixture(t)
			tt.mutate(t, root)
			err := Verify(root)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Verify() error = %v, want containing %q", err, tt.wantErr)
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

func cloneFixture(t *testing.T) string {
	t.Helper()
	src := fixtureRoot(t)
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected source symlink %q", rel)
		}
		if err := os.Link(path, target); err == nil {
			return nil
		}
		return copyFile(path, target)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func replaceFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

func updateManifestChecksum(t *testing.T, root, rel string) {
	t.Helper()
	sum := sha256.Sum256(readFile(t, filepath.Join(root, filepath.FromSlash(rel))))
	manifest := filepath.Join(root, "checksums.txt")
	lines := readLines(t, manifest)
	found := false
	for i, line := range lines {
		if strings.HasSuffix(line, "  "+rel) {
			lines[i] = fmt.Sprintf("%x  %s", sum, rel)
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("manifest path %q not found", rel)
	}
	replaceFile(t, manifest, []byte(strings.Join(lines, "\n")+"\n"))
}
