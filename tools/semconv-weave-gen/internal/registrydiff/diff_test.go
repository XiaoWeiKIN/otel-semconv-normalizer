// SPDX-License-Identifier: Apache-2.0

package registrydiff

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseRealPinnedDiff(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Clean(filepath.Join(
		filepath.Dir(file), "..", "..", "build", "weaver", "diff", Interval, "diff.json",
	))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(document.RegistryAttributes); got != 567 {
		t.Fatalf("Registry attribute candidate count = %d, want 567", got)
	}
	relevant := 0
	for _, candidate := range document.RegistryAttributes {
		if candidate.IsMigrationObligation() &&
			(candidate.HTTPRelated() || candidate.DBRPCRelated()) {
			relevant++
		}
	}
	if relevant != 81 {
		t.Fatalf("relevant migration candidate count = %d, want 81", relevant)
	}
}

func TestParseRejectsUnknownShape(t *testing.T) {
	data := []byte(`{
	  "baseline":{"semconv_version":""},
	  "changes":{
	    "entities":[],
	    "events":[],
	    "metrics":[],
	    "registry_attributes":[{"name":"http.old","type":"mystery"}],
	    "spans":[]
	  },
	  "head":{"semconv_version":""}
	}`)
	if _, err := Parse(data); err == nil || !strings.Contains(err.Error(), "unknown type") {
		t.Fatalf("Parse() error = %v, want unknown type", err)
	}
}
