package migrationaudit

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHTTPInventoryAndAudit(t *testing.T) {
	root := fixtureRoot(t)
	schema := mustRead(t, filepath.Join(root, "schema", "opentelemetry-1.41.0.yaml"))
	guide := mustRead(t, filepath.Join(root, "guides", "http-migration.md"))
	diff := mustRead(t, pinnedWeaverDiff(root))
	inventory := mustRead(t, filepath.Join(root, "audit", "http-url-network-obligations.json"))
	decisions := mustRead(t, filepath.Join(root, "audit", "http-url-network-decisions.json"))
	audit := mustRead(t, filepath.Join(root, "audit", "migration-audit-v1.41.0.yaml"))

	generated, err := GenerateHTTPInventory(schema, guide, diff)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, inventory) {
		t.Fatal("checked-in HTTP obligation inventory differs from fresh generation")
	}
	generatedAudit, err := GenerateHTTPAudit(inventory, decisions)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateHTTPAudit(inventory, generatedAudit); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHTTPAudit(inventory, audit); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPInventoryIsByteStable(t *testing.T) {
	root := fixtureRoot(t)
	schema := mustRead(t, filepath.Join(root, "schema", "opentelemetry-1.41.0.yaml"))
	guide := mustRead(t, filepath.Join(root, "guides", "http-migration.md"))
	diff := mustRead(t, pinnedWeaverDiff(root))

	first, err := GenerateHTTPInventory(schema, guide, diff)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateHTTPInventory(schema, guide, diff)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("HTTP obligation inventory is not byte-stable")
	}
}

func TestValidateHTTPAuditRejectsInvalidReview(t *testing.T) {
	root := fixtureRoot(t)
	inventory := mustRead(t, filepath.Join(root, "audit", "http-url-network-obligations.json"))
	audit := mustRead(t, filepath.Join(root, "audit", "migration-audit-v1.41.0.yaml"))

	tests := []struct {
		name string
		edit func([]byte) []byte
		want string
	}{
		{
			name: "unknown field",
			edit: func(data []byte) []byte {
				return bytes.Replace(data, []byte(`"schema_version": 1`), []byte(`"schema_version": 1, "surprise": true`), 1)
			},
			want: "unknown field",
		},
		{
			name: "missing obligation",
			edit: func(data []byte) []byte {
				start := bytes.Index(data, []byte(`    {`))
				end := bytes.Index(data[start:], []byte("    },\n"))
				return append(append([]byte(nil), data[:start]...), data[start+end+7:]...)
			},
			want: "obligation set",
		},
		{
			name: "contextual translated",
			edit: func(data []byte) []byte {
				return bytes.Replace(data, []byte(`"action": "pass_through"`), []byte(`"action": "translated"`), 1)
			},
			want: "translated",
		},
		{
			name: "empty golden",
			edit: func(data []byte) []byte {
				var parsed migrationAudit
				if err := strictDecode(data, &parsed); err != nil {
					t.Fatal(err)
				}
				parsed.Sections.HTTPURLNetwork.Entries[0].Golden = nil
				updated, err := canonicalJSON(parsed)
				if err != nil {
					t.Fatal(err)
				}
				return updated
			},
			want: "empty golden",
		},
		{
			name: "inventory digest drift",
			edit: func(data []byte) []byte {
				return bytes.Replace(data, []byte(`"source_inventory_sha256": "`), []byte(`"source_inventory_sha256": "0`), 1)
			},
			want: "digest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHTTPAudit(inventory, tt.edit(append([]byte(nil), audit...)))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateHTTPAudit() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "testdata"))
}

func pinnedWeaverDiff(fixtureRoot string) string {
	return filepath.Clean(filepath.Join(
		fixtureRoot, "..", "build", "weaver", "diff",
		"v1.26.0-to-v1.41.0", "diff.json",
	))
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
