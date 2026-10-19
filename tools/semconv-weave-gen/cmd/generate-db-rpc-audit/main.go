package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/migrationaudit"
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/urlpolicy"
)

func main() {
	schemaPath := flag.String("schema", "testdata/schema/opentelemetry-1.41.0.yaml", "pinned Telemetry Schema")
	dbGuidePath := flag.String("db-guide", "testdata/guides/db-migration.md", "pinned DB migration guide")
	rpcGuidePath := flag.String("rpc-guide", "testdata/guides/rpc-migration.md", "pinned RPC migration guide")
	diffPath := flag.String("diff", "build/weaver/diff/v1.26.0-to-v1.41.0/diff.json", "verified Weaver Registry diff")
	aliasesPath := flag.String("aliases", "testdata/corpus/canonical-technology-aliases.json", "root alias contract corpus")
	rootSourcePath := flag.String("root-source", "../../internal/semconvmeta/meta.go", "root semantic metadata implementation")
	inventoryPath := flag.String("inventory", "testdata/audit/db-rpc-obligations.json", "generated DB/RPC inventory")
	decisionsPath := flag.String("decisions", "testdata/audit/db-rpc-decisions.json", "reviewed DB/RPC decisions")
	httpInventoryPath := flag.String("http-inventory", "testdata/audit/http-url-network-obligations.json", "HTTP inventory")
	httpDecisionsPath := flag.String("http-decisions", "testdata/audit/http-url-network-decisions.json", "HTTP decisions")
	draftPolicyPath := flag.String("draft-policy", "testdata/audit/required-source-urls-draft.json", "pre-signoff required URL policy")
	auditPath := flag.String("audit", "testdata/audit/migration-audit-v1.41.0.yaml", "combined migration audit")
	releasePolicyPath := flag.String("release-policy", "testdata/audit/required-source-urls.yaml", "release-ready required URL policy")
	inventoryOnly := flag.Bool("inventory-only", false, "generate only the DB/RPC inventory")
	flag.Parse()

	inventory, err := migrationaudit.GenerateDBRPCInventory(
		mustRead(*schemaPath),
		mustRead(*dbGuidePath),
		mustRead(*rpcGuidePath),
		mustRead(*diffPath),
		mustRead(*aliasesPath),
		mustRead(*rootSourcePath),
	)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*inventoryPath, inventory, 0o644); err != nil {
		fail(err)
	}
	if *inventoryOnly {
		return
	}
	audit, err := migrationaudit.GenerateCombinedAudit(
		mustRead(*httpInventoryPath),
		mustRead(*httpDecisionsPath),
		inventory,
		mustRead(*decisionsPath),
		mustRead(*draftPolicyPath),
	)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*auditPath, audit, 0o644); err != nil {
		fail(err)
	}
	releasePolicy, err := urlpolicy.PromoteRelease(mustRead(*draftPolicyPath))
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*releasePolicyPath, releasePolicy, 0o644); err != nil {
		fail(err)
	}
}

func mustRead(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}
	return data
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
