// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGeneratedManifestDigestAndRuntimeTables(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve package directory")
	}
	manifest, err := os.ReadFile(filepath.Join(filepath.Dir(file), "source_manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(manifest)
	if got := hex.EncodeToString(sum[:]); got != SourceManifestSHA256 {
		t.Fatalf("source manifest SHA-256 = %s, want %s", got, SourceManifestSHA256)
	}
	if len(activeGeneratedData.sources) != 2 ||
		len(activeGeneratedData.keyRules) != 516 ||
		len(activeGeneratedData.valueRules) != 108 ||
		len(activeGeneratedData.nameRules) != 0 ||
		len(activeGeneratedData.fallbackKeyRules) != 258 ||
		len(activeGeneratedData.fallbackValueRules) != 54 ||
		len(activeGeneratedData.fallbackNameRules) != 0 {
		t.Fatalf(
			"generated table counts = sources:%d keys:%d values:%d names:%d fallback-keys:%d fallback-values:%d fallback-names:%d",
			len(activeGeneratedData.sources), len(activeGeneratedData.keyRules),
			len(activeGeneratedData.valueRules), len(activeGeneratedData.nameRules),
			len(activeGeneratedData.fallbackKeyRules), len(activeGeneratedData.fallbackValueRules),
			len(activeGeneratedData.fallbackNameRules),
		)
	}
	for _, rule := range append(activeGeneratedData.keyRules, activeGeneratedData.fallbackKeyRules...) {
		switch rule.source {
		case "db.statement", "http.method", "http.target",
			"message.id", "message.type", "message.compressed_size", "message.uncompressed_size":
			t.Fatalf("negative key entered generated table: %#v", rule)
		}
		if rule.target == "db.name" || rule.target == "net.protocol.name" ||
			rule.target == "rpc.message.id" || rule.target == "rpc.message.type" ||
			rule.target == "rpc.message.compressed_size" ||
			rule.target == "rpc.message.uncompressed_size" {
			t.Fatalf("deprecated target entered generated table: %#v", rule)
		}
	}
}

func TestGeneratedDataInstallIsSingleAssignment(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("second generated data installation did not panic")
		}
	}()
	installGeneratedData(generatedData{})
}
