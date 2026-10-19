// SPDX-License-Identifier: Apache-2.0

package typedir

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/inputlock"
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/migrationaudit"
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/registrydiff"
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/urlpolicy"
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/weaverevidence"
	ast11 "go.opentelemetry.io/otel/schema/v1.1/ast"
)

const (
	sourceURL17 = "https://opentelemetry.io/schemas/1.7.0"
	sourceURL14 = "https://opentelemetry.io/schemas/1.4.0"
)

type compileInputs struct {
	schema        []byte
	matrix        []byte
	releasePolicy []byte
	draftPolicy   []byte
	httpInventory []byte
	dbInventory   []byte
	audit         []byte
	checksums     []byte
}

// Config makes the complete compiler boundary explicit. WeaverRoot is not
// inferred from FixtureRoot because promoted evidence is an independent,
// digest-bound input that must never be silently bypassed.
type Config struct {
	FixtureRoot string
	WeaverRoot  string
}

// Compile validates the complete locked input boundary before it parses or
// folds any operation. The returned Bundle contains no runtime dependencies.
func Compile(config Config) (Bundle, error) {
	if config.FixtureRoot == "" || config.WeaverRoot == "" {
		return Bundle{}, errors.New("fixture root and Weaver evidence root are required")
	}
	if err := inputlock.Verify(config.FixtureRoot); err != nil {
		return Bundle{}, fmt.Errorf("verify build inputs: %w", err)
	}
	weaver, err := weaverevidence.Verify(config.FixtureRoot, config.WeaverRoot)
	if err != nil {
		return Bundle{}, fmt.Errorf("verify Weaver evidence: %w", err)
	}
	model, err := parseTargetModel(weaver.Model)
	if err != nil {
		return Bundle{}, fmt.Errorf("parse Weaver target model: %w", err)
	}
	diff, err := registrydiff.Parse(weaver.Diff)
	if err != nil {
		return Bundle{}, fmt.Errorf("parse Weaver registry diff: %w", err)
	}
	inputs, err := readCompileInputs(config.FixtureRoot)
	if err != nil {
		return Bundle{}, err
	}
	if err := urlpolicy.Validate(inputs.schema, inputs.matrix, inputs.releasePolicy, true); err != nil {
		return Bundle{}, fmt.Errorf("validate release-ready URL policy: %w", err)
	}
	if err := migrationaudit.ValidateCombinedAudit(
		inputs.httpInventory, inputs.dbInventory, inputs.draftPolicy, inputs.audit,
	); err != nil {
		return Bundle{}, fmt.Errorf("validate combined migration audit: %w", err)
	}
	requiredURLs, err := urlpolicy.SortedRequiredURLs(inputs.releasePolicy)
	if err != nil {
		return Bundle{}, fmt.Errorf("read required URL set: %w", err)
	}
	expectedURLs := []string{sourceURL14, CanonicalURL, sourceURL17}
	if !equalStringSlices(requiredURLs, expectedURLs) {
		return Bundle{}, fmt.Errorf("required URL set = %q, want %q", requiredURLs, expectedURLs)
	}

	document, err := parseSchema(inputs.schema)
	if err != nil {
		return Bundle{}, err
	}
	reviewed, err := migrationaudit.ReviewedRules(inputs.audit)
	if err != nil {
		return Bundle{}, err
	}
	if err := reconcileRegistryDiff(diff, reviewed, model); err != nil {
		return Bundle{}, fmt.Errorf("reconcile Weaver registry diff: %w", err)
	}
	auditIndex, err := newSchemaAuditIndex(reviewed)
	if err != nil {
		return Bundle{}, err
	}
	overrides, err := newGuideOverrideIndex(reviewed)
	if err != nil {
		return Bundle{}, err
	}

	source17, err := compileSource(document, "1.7.0", sourceURL17, reviewed, auditIndex, overrides)
	if err != nil {
		return Bundle{}, fmt.Errorf("compile %s: %w", sourceURL17, err)
	}
	source14, err := compileSource(document, "1.4.0", sourceURL14, reviewed, auditIndex, overrides)
	if err != nil {
		return Bundle{}, fmt.Errorf("compile %s: %w", sourceURL14, err)
	}
	source17, err = reconcileSourceWithTargetModel(source17, model)
	if err != nil {
		return Bundle{}, fmt.Errorf("reconcile %s with Weaver target model: %w", sourceURL17, err)
	}
	source14, err = reconcileSourceWithTargetModel(source14, model)
	if err != nil {
		return Bundle{}, fmt.Errorf("reconcile %s with Weaver target model: %w", sourceURL14, err)
	}
	fixtureSum := sha256.Sum256(inputs.checksums)
	inputIdentity := fmt.Sprintf(
		"fixture-checksums-sha256=%s\nweaver-evidence-sha256=%s\n",
		hex.EncodeToString(fixtureSum[:]), weaver.EvidenceSHA256,
	)
	sum := sha256.Sum256([]byte(inputIdentity))
	artifacts := make([]WeaverArtifact, 0, len(weaver.Artifacts))
	for _, artifact := range weaver.Artifacts {
		artifacts = append(artifacts, WeaverArtifact{
			Path: artifact.Path, SHA256: artifact.SHA256,
		})
	}
	bundle := Bundle{
		SchemaVersion:        IRSchemaVersion,
		CanonicalURL:         CanonicalURL,
		InputDigest:          hex.EncodeToString(sum[:]),
		WeaverEvidenceSHA256: weaver.EvidenceSHA256,
		WeaverArtifacts:      artifacts,
		Sources: []SourceIR{
			{
				SourceURL: CanonicalURL, SourceVersion: "1.41.0", CanonicalNoOp: true,
				KeyRules: []KeyRule{}, ValueRules: []ValueRule{}, NameRules: []NameRule{}, Dispositions: []Disposition{},
			},
			source17,
			source14,
		},
	}
	if err := bundle.Validate(); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func readCompileInputs(root string) (compileInputs, error) {
	read := func(relative string) ([]byte, error) {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", relative, err)
		}
		return data, nil
	}
	var result compileInputs
	files := []struct {
		path   string
		target *[]byte
	}{
		{"schema/opentelemetry-1.41.0.yaml", &result.schema},
		{"audit/compatibility-matrix.json", &result.matrix},
		{"audit/required-source-urls.yaml", &result.releasePolicy},
		{"audit/required-source-urls-draft.json", &result.draftPolicy},
		{"audit/http-url-network-obligations.json", &result.httpInventory},
		{"audit/db-rpc-obligations.json", &result.dbInventory},
		{"audit/migration-audit-v1.41.0.yaml", &result.audit},
		{"checksums.txt", &result.checksums},
	}
	for _, file := range files {
		data, err := read(file.path)
		if err != nil {
			return compileInputs{}, err
		}
		*file.target = data
	}
	return result, nil
}

type schemaAuditIndex map[string]migrationaudit.ReviewedRule

func newSchemaAuditIndex(rules []migrationaudit.ReviewedRule) (schemaAuditIndex, error) {
	result := make(schemaAuditIndex)
	for _, rule := range rules {
		if rule.Authority != "telemetry_schema" {
			continue
		}
		revision, err := schemaRevisionFromID(rule.ID)
		if err != nil {
			return nil, err
		}
		if len(rule.TargetKeys) != 1 || rule.SourceKey == "" {
			return nil, fmt.Errorf("Schema audit %q lacks exact key mapping", rule.ID)
		}
		key := schemaAuditKey(revision, rule.Domain, rule.SourceKey, rule.TargetKeys[0])
		if previous, exists := result[key]; exists {
			return nil, fmt.Errorf("Schema authority disagreement between %q and %q", previous.ID, rule.ID)
		}
		result[key] = rule
	}
	return result, nil
}

type guideOverride struct {
	id     string
	reason string
}

func newGuideOverrideIndex(rules []migrationaudit.ReviewedRule) (map[string]guideOverride, error) {
	result := make(map[string]guideOverride)
	translated := make(map[string]string)
	for _, rule := range rules {
		if rule.Authority == "telemetry_schema" || rule.Domain != "spans" ||
			rule.SourceKey == "" || len(rule.TargetKeys) != 1 || rule.SourceValue != "" {
			continue
		}
		key := rule.SourceKey + "\x00" + rule.TargetKeys[0]
		if rule.Action == "pass_through" || rule.Action == "unsupported" {
			if strings.Contains(rule.Authority, "migration_guide") {
				if previous, exists := result[key]; exists && previous.id != rule.ID {
					return nil, fmt.Errorf("multiple Guide overrides for %q", rule.SourceKey)
				}
				result[key] = guideOverride{id: rule.ID, reason: rule.Reason}
			}
			continue
		}
		if rule.Action == "translated" {
			sourceKey := rule.SourceKey
			if previous, exists := translated[sourceKey]; exists && previous != rule.TargetKeys[0] {
				return nil, fmt.Errorf("authority disagreement maps %q to %q and %q", sourceKey, previous, rule.TargetKeys[0])
			}
			translated[sourceKey] = rule.TargetKeys[0]
		}
	}
	return result, nil
}

func compileSource(
	document *ast11.Schema,
	sourceVersion, sourceURL string,
	reviewed []migrationaudit.ReviewedRule,
	audits schemaAuditIndex,
	overrides map[string]guideOverride,
) (SourceIR, error) {
	facts, err := schemaFacts(document, sourceVersion)
	if err != nil {
		return SourceIR{}, err
	}
	sourceParsed, _ := parseSemanticVersion(sourceVersion)
	matchedSchemaAudits := make(map[string]struct{})
	operations := make([]operation, 0, len(facts)+len(reviewed))

	for _, fact := range facts {
		instanceID := schemaFactStableID(sourceVersion, fact)
		action := "translated"
		reason := "Official Telemetry Schema defines a deterministic attribute rename."
		if fact.kind == operationName {
			action = "pass_through"
			reason = "Signal name normalization is outside the attribute-only runtime contract."
		}
		item := operation{
			kind: fact.kind, domain: fact.domain, action: action,
			source: fact.source, target: fact.target,
			spanNames: fact.spanNames, eventNames: fact.eventNames,
			reason: reason,
			evidence: Evidence{
				StableID: instanceID, Authority: "telemetry_schema",
				Locator: fmt.Sprintf(
					"versions.%s.%s.changes[%d].mapping[%d]",
					fact.revision, fact.yamlDomain, fact.operation, fact.mapping,
				),
				OperationSource: fact.source,
				Revision:        fact.revision, SchemaDomain: fact.yamlDomain, Phase: fact.phase,
				Operation: fact.operation, Mapping: fact.mapping,
			},
		}
		key := schemaAuditKey(fact.revision, fact.yamlDomain, fact.source, fact.target)
		if reviewedRule, ok := audits[key]; ok {
			item.action = reviewedRule.Action
			item.reason = reviewedRule.Reason
			item.evidence.AuthorityID = reviewedRule.ID
			item.evidence.Authority = reviewedRule.Authority
			item.evidence.Locator = reviewedRule.Locator
			matchedSchemaAudits[reviewedRule.ID] = struct{}{}
			if override, exists := overrides[fact.source+"\x00"+fact.target]; exists && fact.domain == DomainSpan {
				item.action = "pass_through"
				item.reason = "Migration Guide context overrides the otherwise pure Schema rename: " + override.reason
				item.evidence.Reconciliation = override.id
			}
		}
		operations = append(operations, item)
	}
	classifyAmbiguousAutomaticSchemaRenames(operations)
	for _, rule := range reviewed {
		if rule.Authority != "telemetry_schema" {
			continue
		}
		revisionText, err := schemaRevisionFromID(rule.ID)
		if err != nil {
			return SourceIR{}, err
		}
		revision, _ := parseSemanticVersion(revisionText)
		if compareVersion(revision, sourceParsed) <= 0 {
			continue
		}
		if _, ok := matchedSchemaAudits[rule.ID]; !ok {
			return SourceIR{}, fmt.Errorf("required Schema audit %q is absent from official parser replay", rule.ID)
		}
	}

	// A Schema `all:` rename lands one key in every domain, but a Migration Guide
	// value rename is authored against a single domain. Replaying the value rename
	// into every domain that renames its key keeps the two halves aligned;
	// otherwise the key is rewritten on resources and span events while its value
	// keeps a legacy, non-canonical enum member.
	keyRenameDomains := map[string]map[Domain]struct{}{}
	for _, item := range operations {
		if item.kind != operationKey || item.source == "" {
			continue
		}
		if keyRenameDomains[item.source] == nil {
			keyRenameDomains[item.source] = map[Domain]struct{}{}
		}
		keyRenameDomains[item.source][item.domain] = struct{}{}
	}

	for _, rule := range reviewed {
		if rule.Authority == "telemetry_schema" || !containsString(rule.SourceURLs, sourceURL) {
			continue
		}
		item, err := auditOperation(rule)
		if err != nil {
			return SourceIR{}, err
		}
		operations = append(operations, item)
		if item.kind != operationValue {
			continue
		}
		for _, domain := range replayDomains(keyRenameDomains[item.source], item.domain) {
			replay := item
			replay.domain = domain
			// StableID identifies this executable operation instance. The signed
			// audit rule remains available through AuthorityID, while the suffix
			// prevents one reviewed value mapping replayed into multiple runtime
			// domains from colliding in foldOperations.
			replay.evidence.AuthorityID = item.evidence.StableID
			replay.evidence.StableID = fmt.Sprintf("%s#domain=%s", item.evidence.StableID, domain)
			operations = append(operations, replay)
		}
	}

	keyRules, valueRules, nameRules, dispositions, err := foldOperations(operations)
	if err != nil {
		return SourceIR{}, err
	}
	dispositions = append(dispositions, Disposition{
		Domain: DomainLink, StableID: "domain:link:pass-through",
		Action: "pass_through", Reason: "Link normalization is explicitly deferred by the initial runtime ABI.",
		Evidence: []Evidence{{
			StableID: "domain:link:pass-through", Authority: "architecture",
			Locator: "Story 7.8 AC8", Phase: "domain_disposition", Operation: -1, Mapping: -1,
		}},
	})
	sortIR(keyRules, valueRules, nameRules, dispositions)
	result := SourceIR{
		SourceURL: sourceURL, SourceVersion: sourceVersion,
		KeyRules: keyRules, ValueRules: valueRules, NameRules: nameRules, Dispositions: dispositions,
	}
	if err := validateSchemaFactCoverage(result, facts, sourceVersion); err != nil {
		return SourceIR{}, err
	}
	return result, nil
}

func schemaFactStableID(sourceVersion string, fact schemaFact) string {
	return fmt.Sprintf(
		"schema-ir:%s:%s:%s:%s:%02d:%02d:%s",
		sourceVersion, fact.domain, fact.revision, fact.yamlDomain,
		fact.operation, fact.mapping, fact.source,
	)
}

func validateSchemaFactCoverage(
	source SourceIR,
	facts []schemaFact,
	sourceVersion string,
) error {
	covered := make(map[string]struct{}, len(facts))
	addEvidence := func(evidence []Evidence) {
		for _, item := range evidence {
			covered[item.StableID] = struct{}{}
		}
	}
	for _, rule := range source.KeyRules {
		addEvidence(rule.Evidence)
	}
	for _, rule := range source.ValueRules {
		addEvidence(rule.Evidence)
	}
	for _, rule := range source.NameRules {
		addEvidence(rule.Evidence)
	}
	for _, disposition := range source.Dispositions {
		addEvidence(disposition.Evidence)
	}
	for _, fact := range facts {
		id := schemaFactStableID(sourceVersion, fact)
		if _, ok := covered[id]; !ok {
			return fmt.Errorf("official Schema operation %q has no runtime rule or explicit disposition", id)
		}
	}
	return nil
}

// classifyAmbiguousAutomaticSchemaRenames keeps automatic Schema replay
// deterministic when upstream revisions reuse one source key for different
// targets. A signed audit may select one target; otherwise every conflicting
// automatic rename is recorded as unsupported and the source is preserved.
func classifyAmbiguousAutomaticSchemaRenames(operations []operation) {
	groups := make(map[string][]int)
	for index := range operations {
		item := &operations[index]
		if item.kind != operationKey || item.action != "translated" ||
			item.evidence.Authority != "telemetry_schema" {
			continue
		}
		key := string(item.domain) + "\x00" + item.source + "\x00" +
			strings.Join(item.spanNames, "\x00") + "\x01" +
			strings.Join(item.eventNames, "\x00")
		groups[key] = append(groups[key], index)
	}
	for _, indices := range groups {
		targets := make(map[string]struct{})
		signed := make(map[string]struct{})
		for _, index := range indices {
			item := &operations[index]
			targets[item.target] = struct{}{}
			if item.evidence.AuthorityID != "" {
				signed[item.target] = struct{}{}
			}
		}
		if len(targets) <= 1 {
			continue
		}
		for _, index := range indices {
			item := &operations[index]
			if len(signed) == 1 {
				if _, selected := signed[item.target]; selected {
					continue
				}
			} else if item.evidence.AuthorityID != "" {
				continue
			}
			item.action = "unsupported"
			item.reason = "Conflicting official Schema revisions map the same source to different targets without one signed selection."
		}
	}
}

func auditOperation(rule migrationaudit.ReviewedRule) (operation, error) {
	domain, err := auditDomain(rule.Domain)
	if err != nil {
		return operation{}, fmt.Errorf("audit %q: %w", rule.ID, err)
	}
	item := operation{
		kind: operationKey, domain: domain, action: rule.Action,
		source: rule.SourceKey, reason: rule.Reason,
		evidence: Evidence{
			StableID: rule.ID, Authority: rule.Authority, Locator: rule.Locator,
			OperationSource: rule.SourceKey,
			Phase:           rule.Phase, Operation: -1, Mapping: -1,
		},
	}
	if len(rule.TargetKeys) == 1 {
		item.target = rule.TargetKeys[0]
	}
	switch rule.Category {
	case "value":
		item.kind = operationValue
		item.sourceValue = rule.SourceValue
		item.targetValue = rule.TargetValue
	case "name":
		item.kind = operationName
	case "key", "event", "":
		item.kind = operationKey
	default:
		return operation{}, fmt.Errorf("audit %q has unknown category %q", rule.ID, rule.Category)
	}
	return item, nil
}

// replayDomains returns the domains, other than authored, in which a value
// rename must be repeated. The result is sorted so emission stays byte-stable.
func replayDomains(renamed map[Domain]struct{}, authored Domain) []Domain {
	if len(renamed) == 0 {
		return nil
	}
	result := make([]Domain, 0, len(renamed))
	for domain := range renamed {
		if domain != authored {
			result = append(result, domain)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func auditDomain(raw string) (Domain, error) {
	switch raw {
	case "resources":
		return DomainResource, nil
	case "spans", "span_names":
		return DomainSpan, nil
	case "span_events", "event_names":
		return DomainSpanEvent, nil
	default:
		return "", fmt.Errorf("unknown audit domain %q", raw)
	}
}

func schemaRevisionFromID(id string) (string, error) {
	parts := strings.Split(id, ":")
	if len(parts) < 5 || parts[0] != "schema" {
		return "", fmt.Errorf("invalid Schema audit stable ID %q", id)
	}
	if _, err := parseSemanticVersion(parts[1]); err != nil {
		return "", fmt.Errorf("invalid Schema audit stable ID %q: %w", id, err)
	}
	return parts[1], nil
}

func schemaAuditKey(revision, domain, source, target string) string {
	return revision + "\x00" + domain + "\x00" + source + "\x00" + target
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
