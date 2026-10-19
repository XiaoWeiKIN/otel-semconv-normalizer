package migrationaudit

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestDBRPCInventoryAndCombinedAudit(t *testing.T) {
	root := fixtureRoot(t)
	repoRoot := filepath.Clean(filepath.Join(root, "..", "..", ".."))
	schema := mustRead(t, filepath.Join(root, "schema", "opentelemetry-1.41.0.yaml"))
	dbGuide := mustRead(t, filepath.Join(root, "guides", "db-migration.md"))
	rpcGuide := mustRead(t, filepath.Join(root, "guides", "rpc-migration.md"))
	diff := mustRead(t, pinnedWeaverDiff(root))
	aliases := mustRead(t, filepath.Join(root, "corpus", "canonical-technology-aliases.json"))
	rootSource := mustRead(t, filepath.Join(repoRoot, "internal", "semconvmeta", "meta.go"))
	inventory := mustRead(t, filepath.Join(root, "audit", "db-rpc-obligations.json"))
	decisions := mustRead(t, filepath.Join(root, "audit", "db-rpc-decisions.json"))
	httpInventory := mustRead(t, filepath.Join(root, "audit", "http-url-network-obligations.json"))
	httpDecisions := mustRead(t, filepath.Join(root, "audit", "http-url-network-decisions.json"))
	draftPolicy := mustRead(t, filepath.Join(root, "audit", "required-source-urls-draft.json"))
	audit := mustRead(t, filepath.Join(root, "audit", "migration-audit-v1.41.0.yaml"))

	generated, err := GenerateDBRPCInventory(schema, dbGuide, rpcGuide, diff, aliases, rootSource)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, inventory) {
		t.Fatal("checked-in DB/RPC inventory differs from fresh generation")
	}
	generatedAudit, err := GenerateCombinedAudit(httpInventory, httpDecisions, inventory, decisions, draftPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generatedAudit, audit) {
		t.Fatal("checked-in combined audit differs from reviewed decisions")
	}
	if err := ValidateCombinedAudit(httpInventory, inventory, draftPolicy, audit); err != nil {
		t.Fatal(err)
	}
}

func TestDBRPCInventoryIsByteStable(t *testing.T) {
	root := fixtureRoot(t)
	repoRoot := filepath.Clean(filepath.Join(root, "..", "..", ".."))
	inputs := [][]byte{
		mustRead(t, filepath.Join(root, "schema", "opentelemetry-1.41.0.yaml")),
		mustRead(t, filepath.Join(root, "guides", "db-migration.md")),
		mustRead(t, filepath.Join(root, "guides", "rpc-migration.md")),
		mustRead(t, pinnedWeaverDiff(root)),
		mustRead(t, filepath.Join(root, "corpus", "canonical-technology-aliases.json")),
		mustRead(t, filepath.Join(repoRoot, "internal", "semconvmeta", "meta.go")),
	}
	first, err := GenerateDBRPCInventory(inputs[0], inputs[1], inputs[2], inputs[3], inputs[4], inputs[5])
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateDBRPCInventory(inputs[0], inputs[1], inputs[2], inputs[3], inputs[4], inputs[5])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("DB/RPC obligation inventory is not byte-stable")
	}
}

func TestValidateCombinedAuditRejectsUnsafeOrUnsigned(t *testing.T) {
	root := fixtureRoot(t)
	httpInventory := mustRead(t, filepath.Join(root, "audit", "http-url-network-obligations.json"))
	dbInventory := mustRead(t, filepath.Join(root, "audit", "db-rpc-obligations.json"))
	draftPolicy := mustRead(t, filepath.Join(root, "audit", "required-source-urls-draft.json"))
	audit := mustRead(t, filepath.Join(root, "audit", "migration-audit-v1.41.0.yaml"))

	tests := []struct {
		name string
		edit func([]byte) []byte
		want string
	}{
		{
			name: "db statement translated",
			edit: func(data []byte) []byte {
				var parsed migrationAudit
				if err := strictDecode(data, &parsed); err != nil {
					t.Fatal(err)
				}
				found := false
				for index := range parsed.Sections.DBRPC.Entries {
					if parsed.Sections.DBRPC.Entries[index].SourceKey == "db.statement" {
						parsed.Sections.DBRPC.Entries[index].Action = "translated"
						found = true
						break
					}
				}
				if !found {
					t.Fatal("db.statement review not found")
				}
				updated, err := canonicalJSON(parsed)
				if err != nil {
					t.Fatal(err)
				}
				return updated
			},
			want: "translated",
		},
		{
			name: "missing signoff",
			edit: func(data []byte) []byte {
				var parsed migrationAudit
				if err := strictDecode(data, &parsed); err != nil {
					t.Fatal(err)
				}
				parsed.Signoffs = parsed.Signoffs[:1]
				updated, err := canonicalJSON(parsed)
				if err != nil {
					t.Fatal(err)
				}
				return updated
			},
			want: "signoff",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCombinedAudit(httpInventory, dbInventory, draftPolicy, tt.edit(append([]byte(nil), audit...)))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateCombinedAudit() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
