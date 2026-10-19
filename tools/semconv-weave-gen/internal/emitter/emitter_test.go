// SPDX-License-Identifier: Apache-2.0

package emitter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/typedir"
)

func TestGenerateIsByteStableAndComplete(t *testing.T) {
	fixtureRoot, weaverRoot, providerRoot := testRoots()
	first, err := Generate(fixtureRoot, weaverRoot, providerRoot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(fixtureRoot, weaverRoot, providerRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := equalArtifacts(first, second); err != nil {
		t.Fatal(err)
	}
	expectedPaths := []string{goldenPath, manifestPath, supportPath, weavesPath}
	for _, path := range expectedPaths {
		if len(first[path]) == 0 {
			t.Errorf("artifact %q is missing or empty", path)
		}
	}

	manifestBytes := first[manifestPath]
	if manifestBytes[len(manifestBytes)-1] != '\n' || bytes.HasPrefix(manifestBytes, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("manifest framing is not UTF-8/no-BOM/single-LF")
	}
	var normalized any
	if err := json.Unmarshal(manifestBytes, &normalized); err != nil {
		t.Fatal(err)
	}
	canonical, err := canonicalJSON(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(manifestBytes, canonical) {
		t.Fatal("source manifest is not canonical JSON")
	}
	if bytes.Contains(manifestBytes, []byte("source_manifest_sha256")) {
		t.Fatal("source manifest contains its own digest")
	}
	if bytes.Count(manifestBytes, []byte(`"link":"pass_through"`)) != 3 {
		t.Fatal("source manifest does not classify Link pass-through for all required URLs")
	}
	if !bytes.Contains(manifestBytes, []byte(`"policy":"unversioned_union"`)) {
		t.Fatal("source manifest does not identify the unversioned fallback union")
	}
	if !bytes.Contains(manifestBytes, []byte(`"weaver_evidence":{"artifacts":[`)) ||
		!bytes.Contains(manifestBytes, []byte(`"path":"diff/v1.26.0-to-v1.41.0/diff.json"`)) ||
		!bytes.Contains(manifestBytes, []byte(`"path":"registry-model.json"`)) ||
		!bytes.Contains(manifestBytes, []byte(`"path":"provenance.json"`)) {
		t.Fatal("source manifest does not bind the complete Weaver compiler evidence")
	}
	for _, runtimeSource := range []string{
		"go.mod",
		"go.sum",
		"internal/semconvmeta/meta.go",
		"semconvweave/abi.go",
		"semconvweave/generated_contract.go",
		"semconvweave/name_projection.go",
		"semconvweave/projection.go",
		"semconvweave/runtime.go",
		"semconvweave/semantic_projection.go",
		"semconvweave/span_migration.go",
		"semconvweave/http_migration.go",
		"semconvweave/db_migration.go",
		"semconvweave/rpc_migration.go",
	} {
		if !bytes.Contains(manifestBytes, []byte(`"path":"`+runtimeSource+`"`)) {
			t.Errorf("source manifest does not bind runtime source %q", runtimeSource)
		}
	}
	weavesSum := sha256.Sum256(first[weavesPath])
	weavesDigest := hex.EncodeToString(weavesSum[:])
	if !bytes.Contains(manifestBytes, []byte(`"generated_weaves_sha256":"`+weavesDigest+`"`)) {
		t.Fatal("source manifest does not bind the emitted zz_generated_weaves.go bytes")
	}

	sum := sha256.Sum256(manifestBytes)
	digest := hex.EncodeToString(sum[:])
	if !bytes.Contains(first[supportPath], []byte(`const SourceManifestSHA256 = "`+digest+`"`)) {
		t.Fatal("support Go digest does not bind source_manifest.json")
	}

	weaves := first[weavesPath]
	for _, required := range []string{sourceURL14, sourceURL17} {
		if !bytes.Contains(weaves, []byte(required)) {
			t.Errorf("generated lookup lacks %s", required)
		}
	}
	for _, fallbackTable := range []string{
		"generatedFallbackKeyRules",
		"generatedFallbackValueRules",
		"generatedFallbackNameRules",
	} {
		if !bytes.Contains(weaves, []byte(fallbackTable)) {
			t.Errorf("generated runtime lacks %s", fallbackTable)
		}
	}
	for _, optional := range []string{
		"https://opentelemetry.io/schemas/1.6.0",
		"https://opentelemetry.io/schemas/1.8.0",
		"https://opentelemetry.io/schemas/1.40.0",
	} {
		if bytes.Contains(weaves, []byte(optional)) {
			t.Errorf("optional URL %s entered generated lookup", optional)
		}
	}
	for _, assertion := range []string{
		"generatedExpectedRuntimeABIVersion",
		"generatedRuntimeABIV1",
		"AttributeDecision{Key:",
		"func(string) WeaveID",
		"= Resolve",
	} {
		if !bytes.Contains(first[supportPath], []byte(assertion)) {
			t.Errorf("generated ABI assertion %q is absent", assertion)
		}
	}
}

func TestEmitRejectsPartialSupportAndInvalidBundle(t *testing.T) {
	fixtureRoot, weaverRoot, providerRoot := testRoots()
	bundle, err := typedir.Compile(typedir.Config{
		FixtureRoot: fixtureRoot,
		WeaverRoot:  weaverRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := collectIdentity(fixtureRoot, providerRoot)
	if err != nil {
		t.Fatal(err)
	}
	for index := range identity.policy.RequiredURLs {
		if identity.policy.RequiredURLs[index].SchemaURL == sourceURL17 {
			identity.policy.RequiredURLs[index].Domains.SpanEvents = "manual_audit_required"
		}
	}
	if _, err := Emit(bundle, identity); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("Emit(partial support) error = %v, want incomplete", err)
	}

	identity, err = collectIdentity(fixtureRoot, providerRoot)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Sources = bundle.Sources[:2]
	if _, err := Emit(bundle, identity); err == nil || !strings.Contains(err.Error(), "source count") {
		t.Fatalf("Emit(invalid bundle) error = %v, want source count", err)
	}
}

func TestWriteFreshNeverOverwritesExistingOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(output, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(output, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFresh(output, ArtifactSet{"file": []byte("new")}); err == nil {
		t.Fatal("WriteFresh accepted an existing output")
	}
	data, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "preserve" {
		t.Fatal("WriteFresh modified existing output")
	}
}

func TestStagingInsideModuleMustUseGoIgnoredDirectory(t *testing.T) {
	working := filepath.Join(string(filepath.Separator), "workspace", "module")
	if err := validateStagingLocation(filepath.Join(working, "build", "fresh"), working); err == nil {
		t.Fatal("non-ignored in-module staging path was accepted")
	}
	if err := validateStagingLocation(filepath.Join(working, "build", "_fresh"), working); err != nil {
		t.Fatalf("ignored in-module staging path was rejected: %v", err)
	}
	if err := validateStagingLocation(filepath.Join(string(filepath.Separator), "tmp", "fresh"), working); err != nil {
		t.Fatalf("out-of-module staging path was rejected: %v", err)
	}
}

func TestCheckedArtifactsMatchFreshEmission(t *testing.T) {
	fixtureRoot, weaverRoot, providerRoot := testRoots()
	artifacts, err := Generate(fixtureRoot, weaverRoot, providerRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckArtifacts(providerRoot, artifacts); err != nil {
		t.Fatal(err)
	}
}

func TestKeyRulePrioritiesUseOperationRevisionAcrossFoldedChain(t *testing.T) {
	rules := []typedir.KeyRule{
		{
			Domain: typedir.DomainSpan, Match: typedir.MatchExact,
			Source: "http.retry_count", Target: "http.request.resend_count",
			Evidence: []typedir.Evidence{
				{OperationSource: "http.retry_count", Revision: "1.15.0"},
				{OperationSource: "http.resend_count", Revision: "1.22.0"},
			},
		},
		{
			Domain: typedir.DomainSpan, Match: typedir.MatchExact,
			Source: "http.resend_count", Target: "http.request.resend_count",
			Evidence: []typedir.Evidence{{OperationSource: "http.resend_count"}},
		},
	}
	got, err := keyRulePriorities(rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 2 || got[1] != 1 {
		t.Fatalf("keyRulePriorities() = %v, want [2 1]", got)
	}
}

func TestKeyRulePrioritiesNormalizePrefixEvidenceSource(t *testing.T) {
	rules := []typedir.KeyRule{
		{
			Domain: typedir.DomainSpan, Match: typedir.MatchPrefix,
			Source: "rpc.grpc.request.metadata.", Target: "rpc.request.metadata.",
			Evidence: []typedir.Evidence{{
				OperationSource: "rpc.grpc.request.metadata.<key>", Revision: "1.20.0",
			}},
		},
	}
	got, err := keyRulePriorities(rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("keyRulePriorities() = %v, want [1]", got)
	}
}

func TestFallbackRuleUnionDeduplicatesEquivalentLegacyRules(t *testing.T) {
	key := typedir.KeyRule{
		Domain: typedir.DomainSpan, Match: typedir.MatchExact,
		Source: "legacy.key", Target: "canonical.key",
		Evidence: []typedir.Evidence{{
			StableID: "key", Authority: "test", Revision: "1.2.0",
		}},
	}
	value := typedir.ValueRule{
		Domain:    typedir.DomainSpan,
		SourceKey: "legacy.key", TargetKey: "canonical.key",
		SourceValue: "old", TargetValue: "new",
		Evidence: []typedir.Evidence{{StableID: "value", Authority: "test"}},
	}
	name := typedir.NameRule{
		Domain: typedir.DomainSpan, SourceName: "old", TargetName: "new",
		Evidence: []typedir.Evidence{{StableID: "name", Authority: "test"}},
	}
	bundle := typedir.Bundle{Sources: []typedir.SourceIR{
		{CanonicalNoOp: true},
		{KeyRules: []typedir.KeyRule{key}, ValueRules: []typedir.ValueRule{value}, NameRules: []typedir.NameRule{name}},
		{KeyRules: []typedir.KeyRule{key}, ValueRules: []typedir.ValueRule{value}, NameRules: []typedir.NameRule{name}},
	}}
	union, err := buildFallbackRuleUnion(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(union.keyRules) != 1 || len(union.valueRules) != 1 || len(union.nameRules) != 1 {
		t.Fatalf(
			"fallback union counts = keys:%d values:%d names:%d",
			len(union.keyRules), len(union.valueRules), len(union.nameRules),
		)
	}
}

func TestFallbackRuleUnionRejectsAmbiguity(t *testing.T) {
	evidence := []typedir.Evidence{{StableID: "rule", Authority: "test"}}
	tests := []struct {
		name    string
		sources []typedir.SourceIR
		want    string
	}{
		{
			name: "key target",
			sources: []typedir.SourceIR{
				{KeyRules: []typedir.KeyRule{{
					Domain: typedir.DomainSpan, Match: typedir.MatchExact,
					Source: "legacy", Target: "canonical.one", Evidence: evidence,
				}}},
				{KeyRules: []typedir.KeyRule{{
					Domain: typedir.DomainSpan, Match: typedir.MatchExact,
					Source: "legacy", Target: "canonical.two", Evidence: evidence,
				}}},
			},
			want: "fallback key ambiguity",
		},
		{
			name: "value target",
			sources: []typedir.SourceIR{
				{ValueRules: []typedir.ValueRule{{
					Domain: typedir.DomainSpan, SourceKey: "legacy", SourceValue: "old",
					TargetKey: "canonical", TargetValue: "one", Evidence: evidence,
				}}},
				{ValueRules: []typedir.ValueRule{{
					Domain: typedir.DomainSpan, SourceKey: "legacy", SourceValue: "old",
					TargetKey: "canonical", TargetValue: "two", Evidence: evidence,
				}}},
			},
			want: "fallback value ambiguity",
		},
		{
			name: "name target",
			sources: []typedir.SourceIR{
				{NameRules: []typedir.NameRule{{
					Domain: typedir.DomainSpan, SourceName: "old", TargetName: "one", Evidence: evidence,
				}}},
				{NameRules: []typedir.NameRule{{
					Domain: typedir.DomainSpan, SourceName: "old", TargetName: "two", Evidence: evidence,
				}}},
			},
			want: "fallback name ambiguity",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildFallbackRuleUnion(typedir.Bundle{Sources: test.sources})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("buildFallbackRuleUnion() error = %v, want %q", err, test.want)
			}
		})
	}
}

func testRoots() (string, string, string) {
	return filepath.Clean(filepath.Join("..", "..", "testdata")),
		filepath.Clean(filepath.Join("..", "..", "build", "weaver")),
		filepath.Clean(filepath.Join("..", "..", "..", ".."))
}
