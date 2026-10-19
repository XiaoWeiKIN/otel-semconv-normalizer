// SPDX-License-Identifier: Apache-2.0

package typedir

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/migrationaudit"
)

func TestCompileRealPinnedInputsIsByteStable(t *testing.T) {
	config := realCompileConfig()
	first, err := Compile(config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Compile(config)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := Encode(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := Encode(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("two real-input compilations are not byte-identical")
	}
	if len(first.Sources[1].KeyRules) == 0 || len(first.Sources[1].ValueRules) == 0 {
		t.Fatalf("historical IR lacks executable rules: %#v", first.Sources[1])
	}
	for _, source := range first.Sources {
		for _, rule := range source.KeyRules {
			if isContextualNegative(rule.Source) {
				t.Fatalf("contextual negative entered executable IR: %#v", rule)
			}
			if rule.Target == "db.name" || rule.Target == "net.protocol.name" ||
				strings.HasPrefix(rule.Target, "rpc.message.") {
				t.Fatalf("deprecated Weaver model target entered executable IR: %#v", rule)
			}
		}
	}
	if len(first.WeaverArtifacts) != 7 || first.WeaverEvidenceSHA256 == "" {
		t.Fatal("Typed IR does not bind the complete Weaver evidence closure")
	}
	if !hasKeyRule(first.Sources[1], DomainSpan, "net.host.ip", "network.local.address") {
		t.Fatal("reviewed Weaver-only rename is absent from executable IR")
	}
	for _, source := range first.Sources {
		if source.CanonicalNoOp {
			continue
		}
		if got, want := len(source.KeyRules), 258; got != want {
			t.Fatalf("%s key rule count = %d, want %d", source.SourceURL, got, want)
		}
		if got, want := len(source.ValueRules), 54; got != want {
			t.Fatalf("%s value rule count = %d, want %d", source.SourceURL, got, want)
		}
		if got := len(source.NameRules); got != 0 {
			t.Fatalf("%s name rule count = %d, want 0", source.SourceURL, got)
		}
		var passThrough, unsupported int
		for _, disposition := range source.Dispositions {
			if strings.Contains(
				disposition.Reason,
				"outside the reviewed trace-normalization projection scope",
			) {
				t.Fatalf(
					"%s still contains the retired scope omission: %#v",
					source.SourceURL, disposition,
				)
			}
			switch disposition.Action {
			case "pass_through":
				passThrough++
			case "unsupported":
				unsupported++
			}
		}
		if passThrough != 172 || unsupported != 9 {
			t.Fatalf(
				"%s dispositions = pass_through:%d unsupported:%d, want 172/9",
				source.SourceURL, passThrough, unsupported,
			)
		}
		for _, migration := range []struct {
			domain         Domain
			source, target string
		}{
			{DomainResource, "deployment.environment", "deployment.environment.name"},
			{DomainSpan, "messaging.message_id", "messaging.message.id"},
			{DomainSpanEvent, "code.filepath", "code.file.path"},
		} {
			if !hasKeyRule(source, migration.domain, migration.source, migration.target) {
				t.Fatalf(
					"%s lacks expanded migration %s:%s -> %s",
					source.SourceURL, migration.domain, migration.source, migration.target,
				)
			}
		}
	}

	golden, err := os.ReadFile(filepath.Join("testdata", "real-ir.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, golden) {
		t.Fatal("real pinned Typed IR differs from golden")
	}
}

func TestCompileFailsClosedOnBrokenWeaverEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "missing model",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "registry-model.json")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "tampered diff",
			mutate: func(t *testing.T, root string) {
				writeTestFile(t, filepath.Join(
					root, "diff", "v1.26.0-to-v1.41.0", "diff.json",
				), []byte("{}\n"))
			},
		},
		{
			name: "nonempty findings",
			mutate: func(t *testing.T, root string) {
				writeTestFile(t, filepath.Join(root, "findings-check.json"), []byte("[{}]\n"))
			},
		},
		{
			name: "tampered provenance",
			mutate: func(t *testing.T, root string) {
				writeTestFile(t, filepath.Join(root, "provenance.json"), []byte("{}\n"))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := realCompileConfig()
			config.WeaverRoot = cloneTestTree(t, config.WeaverRoot)
			test.mutate(t, config.WeaverRoot)
			if _, err := Compile(config); err == nil ||
				!strings.Contains(err.Error(), "verify Weaver evidence") {
				t.Fatalf("Compile() error = %v, want Weaver evidence rejection", err)
			}
		})
	}
}

// A Migration Guide value rename must reach every domain in which the Schema
// renames the key it qualifies. Otherwise the key is rewritten on resources and
// span events while its value keeps a legacy, non-canonical enum member.
func TestGuideValueRulesCoverEveryDomainOfTheirKeyRename(t *testing.T) {
	bundle, err := Compile(realCompileConfig())
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, source := range bundle.Sources {
		keyDomains := map[string]map[Domain]struct{}{}
		for _, rule := range source.KeyRules {
			if rule.Match != MatchExact {
				continue
			}
			if keyDomains[rule.Source] == nil {
				keyDomains[rule.Source] = map[Domain]struct{}{}
			}
			keyDomains[rule.Source][rule.Domain] = struct{}{}
		}
		valueDomains := map[string]map[Domain]struct{}{}
		for _, rule := range source.ValueRules {
			if valueDomains[rule.SourceKey] == nil {
				valueDomains[rule.SourceKey] = map[Domain]struct{}{}
			}
			valueDomains[rule.SourceKey][rule.Domain] = struct{}{}
		}
		for key, wantDomains := range valueDomains {
			renamed, ok := keyDomains[key]
			if !ok {
				t.Errorf(
					"%s: value rule source %q has no key rename and cannot enter the runtime projection path",
					source.SourceURL, key,
				)
				continue
			}
			checked++
			for domain := range renamed {
				if _, covered := wantDomains[domain]; !covered {
					t.Errorf(
						"%s: key %q is renamed in domain %q but has no value rule there (value rule domains: %v)",
						source.SourceURL, key, domain, wantDomains,
					)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no value rule shares a source key with a key rename; the invariant was not exercised")
	}
}

func realCompileConfig() Config {
	return Config{
		FixtureRoot: filepath.Clean(filepath.Join("..", "..", "testdata")),
		WeaverRoot:  filepath.Clean(filepath.Join("..", "..", "build", "weaver")),
	}
}

func hasKeyRule(source SourceIR, domain Domain, legacy, canonical string) bool {
	for _, rule := range source.KeyRules {
		if rule.Domain == domain && rule.Source == legacy && rule.Target == canonical {
			return true
		}
	}
	return false
}

func TestValidateSchemaFactCoverageRejectsMissingOperation(t *testing.T) {
	fact := schemaFact{
		kind: operationKey, domain: DomainSpan,
		source: "messaging.old", target: "messaging.new",
		revision: "1.20.0", yamlDomain: "all", phase: "schema_all",
		operation: 1, mapping: 2,
	}
	if err := validateSchemaFactCoverage(SourceIR{}, []schemaFact{fact}, "1.7.0"); err == nil ||
		!strings.Contains(err.Error(), "has no runtime rule or explicit disposition") {
		t.Fatalf("validateSchemaFactCoverage() error = %v", err)
	}
	source := SourceIR{KeyRules: []KeyRule{{
		Domain: DomainSpan, Match: MatchExact,
		Source: fact.source, Target: fact.target,
		Evidence: []Evidence{{StableID: schemaFactStableID("1.7.0", fact)}},
	}}}
	if err := validateSchemaFactCoverage(source, []schemaFact{fact}, "1.7.0"); err != nil {
		t.Fatalf("validateSchemaFactCoverage() = %v", err)
	}
}

func cloneTestTree(t *testing.T, source string) string {
	t.Helper()
	target := t.TempDir()
	err := filepath.WalkDir(source, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaReplayOrderAndStrictParser(t *testing.T) {
	valid := []byte(`
file_format: 1.1.0
schema_url: https://opentelemetry.io/schemas/1.41.0
versions:
  1.6.0:
    all:
      changes:
        - rename_attributes:
            attribute_map:
              common.old: common.new
    spans:
      changes:
        - rename_attributes:
            attribute_map:
              span.old: span.new
  1.5.0:
    resources:
      changes:
        - rename_attributes:
            attribute_map:
              resource.old: resource.new
`)
	document, err := parseSchema(valid)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := schemaFacts(document, "1.4.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 5 {
		t.Fatalf("schemaFacts() count = %d, want 5", len(facts))
	}
	got := []string{
		facts[0].revision + "/" + string(facts[0].domain) + "/" + facts[0].yamlDomain,
		facts[1].revision + "/" + string(facts[1].domain) + "/" + facts[1].yamlDomain,
		facts[2].revision + "/" + string(facts[2].domain) + "/" + facts[2].yamlDomain,
		facts[3].revision + "/" + string(facts[3].domain) + "/" + facts[3].yamlDomain,
		facts[4].revision + "/" + string(facts[4].domain) + "/" + facts[4].yamlDomain,
	}
	want := []string{
		"1.5.0/resource/resources",
		"1.6.0/resource/all",
		"1.6.0/span/all",
		"1.6.0/span/spans",
		"1.6.0/span_event/all",
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("replay order[%d] = %q, want %q (all=%q)", index, got[index], want[index], got)
		}
	}

	unknown := append([]byte(nil), valid...)
	unknown = bytes.Replace(unknown, []byte("versions:"), []byte("unknown_field: true\nversions:"), 1)
	if _, err := parseSchema(unknown); err == nil {
		t.Fatal("strict parser accepted an unknown field")
	}
}

func TestAuthorityReconciliationRejectsTwoTranslatedTargets(t *testing.T) {
	rules := []migrationaudit.ReviewedRule{
		{
			ID: "guide:a", Authority: "db_migration_guide", Domain: "spans",
			Action: "translated", SourceKey: "legacy.key", TargetKeys: []string{"target.one"},
		},
		{
			ID: "guide:b", Authority: "rpc_migration_guide", Domain: "spans",
			Action: "translated", SourceKey: "legacy.key", TargetKeys: []string{"target.two"},
		},
	}
	if _, err := newGuideOverrideIndex(rules); err == nil || !strings.Contains(err.Error(), "authority disagreement") {
		t.Fatalf("newGuideOverrideIndex() error = %v, want authority disagreement", err)
	}
}
