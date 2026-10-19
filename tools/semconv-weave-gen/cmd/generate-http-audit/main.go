package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/migrationaudit"
)

func main() {
	schemaPath := flag.String("schema", "testdata/schema/opentelemetry-1.41.0.yaml", "pinned Telemetry Schema")
	guidePath := flag.String("guide", "testdata/guides/http-migration.md", "pinned HTTP migration guide")
	diffPath := flag.String("diff", "build/weaver/diff/v1.26.0-to-v1.41.0/diff.json", "verified Weaver Registry diff")
	inventoryPath := flag.String("inventory", "testdata/audit/http-url-network-obligations.json", "generated obligation inventory")
	decisionsPath := flag.String("decisions", "testdata/audit/http-url-network-decisions.json", "reviewed decision catalog")
	auditPath := flag.String("audit", "testdata/audit/migration-audit-v1.41.0.yaml", "generated combined audit")
	inventoryOnly := flag.Bool("inventory-only", false, "generate only the obligation inventory")
	flag.Parse()

	schema := mustRead(*schemaPath)
	guide := mustRead(*guidePath)
	diff := mustRead(*diffPath)
	inventory, err := migrationaudit.GenerateHTTPInventory(schema, guide, diff)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*inventoryPath, inventory, 0o644); err != nil {
		fail(err)
	}
	if *inventoryOnly {
		return
	}
	decisions := mustRead(*decisionsPath)
	audit, err := migrationaudit.GenerateHTTPAudit(inventory, decisions)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*auditPath, audit, 0o644); err != nil {
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
