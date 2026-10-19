package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/urlpolicy"
)

func main() {
	schemaPath := flag.String("schema", "testdata/schema/opentelemetry-1.41.0.yaml", "pinned Telemetry Schema")
	corpusPath := flag.String("corpus", "testdata/corpus/collector-schema-urls.json", "pinned Collector Schema URL corpus")
	matrixPath := flag.String("matrix", "testdata/audit/compatibility-matrix.json", "compatibility matrix output")
	policyPath := flag.String("policy", "testdata/audit/required-source-urls-draft.json", "audit-draft required URL policy output")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "generate-url-policy accepts flags only")
		os.Exit(2)
	}
	schema, err := os.ReadFile(*schemaPath)
	if err != nil {
		fail(err)
	}
	corpus, err := os.ReadFile(*corpusPath)
	if err != nil {
		fail(err)
	}
	matrix, policy, err := urlpolicy.Generate(schema, corpus)
	if err != nil {
		fail(err)
	}
	if err := writeFile(*matrixPath, matrix); err != nil {
		fail(err)
	}
	if err := writeFile(*policyPath, policy); err != nil {
		fail(err)
	}
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
