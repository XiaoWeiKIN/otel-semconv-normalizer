package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/migrationaudit"
)

func main() {
	httpInventoryPath := flag.String("http-inventory", "testdata/audit/http-url-network-obligations.json", "HTTP inventory")
	dbInventoryPath := flag.String("db-inventory", "testdata/audit/db-rpc-obligations.json", "DB/RPC inventory")
	draftPolicyPath := flag.String("draft-policy", "testdata/audit/required-source-urls-draft.json", "pre-signoff policy")
	auditPath := flag.String("audit", "testdata/audit/migration-audit-v1.41.0.yaml", "combined migration audit")
	flag.Parse()

	if err := migrationaudit.ValidateCombinedAudit(
		mustRead(*httpInventoryPath),
		mustRead(*dbInventoryPath),
		mustRead(*draftPolicyPath),
		mustRead(*auditPath),
	); err != nil {
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
