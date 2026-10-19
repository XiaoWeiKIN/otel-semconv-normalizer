package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/migrationaudit"
)

func main() {
	inventoryPath := flag.String("inventory", "testdata/audit/http-url-network-obligations.json", "HTTP obligation inventory")
	auditPath := flag.String("audit", "testdata/audit/migration-audit-v1.41.0.yaml", "combined migration audit")
	flag.Parse()

	inventory, err := os.ReadFile(*inventoryPath)
	if err != nil {
		fail(err)
	}
	audit, err := os.ReadFile(*auditPath)
	if err != nil {
		fail(err)
	}
	if err := migrationaudit.ValidateHTTPAudit(inventory, audit); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
