package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/urlpolicy"
)

func main() {
	schemaPath := flag.String("schema", "testdata/schema/opentelemetry-1.41.0.yaml", "pinned Telemetry Schema")
	matrixPath := flag.String("matrix", "testdata/audit/compatibility-matrix.json", "compatibility matrix")
	policyPath := flag.String("policy", "testdata/audit/required-source-urls.yaml", "required URL policy")
	releaseReady := flag.Bool("release-ready", false, "require every required URL to have all five trace domains audited")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "validate-url-policy accepts flags only")
		os.Exit(2)
	}
	schema, err := os.ReadFile(*schemaPath)
	if err != nil {
		fail(err)
	}
	matrix, err := os.ReadFile(*matrixPath)
	if err != nil {
		fail(err)
	}
	policy, err := os.ReadFile(*policyPath)
	if err != nil {
		fail(err)
	}
	if err := urlpolicy.Validate(schema, matrix, policy, *releaseReady); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
