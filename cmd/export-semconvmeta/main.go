// SPDX-License-Identifier: Apache-2.0

// Command export-semconvmeta materializes the build-neutral semantic metadata
// as a checksum-lockable JSON handoff for the independent generator module.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
)

const (
	repository = "github.com/XiaoWeiKIN/otel-semconv-normalizer"
	owner      = "otel-semconv-normalizer/semconv-maintainers"
)

type corpus struct {
	SchemaVersion int                           `json:"schema_version"`
	Repository    string                        `json:"repository"`
	Commit        string                        `json:"commit"`
	SourcePath    string                        `json:"source_path"`
	SourceSHA256  string                        `json:"source_sha256"`
	Owner         string                        `json:"owner"`
	Aliases       []semconvmeta.TechnologyAlias `json:"aliases"`
}

func main() {
	sourcePath := flag.String("source", "internal/semconvmeta/meta.go", "semantic metadata source")
	outputPath := flag.String("output", "tools/semconv-weave-gen/testdata/corpus/canonical-technology-aliases.json", "generated corpus")
	commit := flag.String("commit", "", "review baseline commit")
	flag.Parse()

	if *commit == "" {
		fail(fmt.Errorf("-commit is required"))
	}
	source, err := os.ReadFile(*sourcePath)
	if err != nil {
		fail(err)
	}
	sum := sha256.Sum256(source)
	payload := corpus{
		SchemaVersion: 2,
		Repository:    repository,
		Commit:        *commit,
		SourcePath:    *sourcePath,
		SourceSHA256:  hex.EncodeToString(sum[:]),
		Owner:         owner,
		Aliases:       semconvmeta.TechnologyAliases(),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fail(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(*outputPath, data, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
