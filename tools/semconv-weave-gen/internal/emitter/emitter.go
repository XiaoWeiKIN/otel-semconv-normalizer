// SPDX-License-Identifier: Apache-2.0

// Package emitter turns a validated private Typed IR into the complete,
// byte-stable runtime artifact set. It does not parse Schema or audit files.
package emitter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/typedir"
)

const (
	weavesPath   = "semconvweave/zz_generated_weaves.go"
	supportPath  = "semconvweave/zz_generated_support_manifest.go"
	manifestPath = "semconvweave/source_manifest.json"
	goldenPath   = "semconvweave/testdata/generated_runtime.golden.json"
	sourceURL17  = "https://opentelemetry.io/schemas/1.7.0"
	sourceURL14  = "https://opentelemetry.io/schemas/1.4.0"
)

type ArtifactSet map[string][]byte

type sourcePolicy struct {
	SchemaURL            string       `json:"schema_url"`
	Requirement          string       `json:"requirement"`
	Reason               string       `json:"reason"`
	ClassifierOwner      string       `json:"classifier_owner"`
	CollectorOwner       string       `json:"collector_owner"`
	Evidence             []string     `json:"evidence"`
	SourceRegistryStatus string       `json:"source_registry_status"`
	Domains              domainPolicy `json:"domains"`
}

type domainPolicy struct {
	Resources  string `json:"resources"`
	Spans      string `json:"spans"`
	SpanEvents string `json:"span_events"`
	SpanNames  string `json:"span_names"`
	EventNames string `json:"event_names"`
}

type releasePolicy struct {
	SchemaVersion      int            `json:"schema_version"`
	Status             string         `json:"status"`
	CanonicalTarget    string         `json:"canonical_target"`
	CandidateMatrixSHA string         `json:"candidate_matrix_sha256"`
	RequiredURLs       []sourcePolicy `json:"required_urls"`
	OptionalURLs       []optionalURL  `json:"optional_urls"`
}

type optionalURL struct {
	SchemaURL   string `json:"schema_url"`
	Disposition string `json:"disposition"`
}

type sourceDigest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type manifestSource struct {
	URL          string         `json:"url"`
	Version      string         `json:"version"`
	WeaveID      uint16         `json:"weave_id"`
	State        string         `json:"state"`
	Link         string         `json:"link"`
	Domains      domainPolicy   `json:"domains"`
	KeyRules     map[string]int `json:"key_rules"`
	ValueRules   map[string]int `json:"value_rules"`
	NameRules    map[string]int `json:"name_rules"`
	Dispositions map[string]int `json:"dispositions"`
}

type sourceManifest struct {
	SchemaVersion       int            `json:"schema_version"`
	CanonicalURL        string         `json:"canonical_url"`
	RuntimeABIVersion   uint16         `json:"runtime_abi_version"`
	TypedIRInputDigest  string         `json:"typed_ir_input_digest"`
	WeaverEvidence      weaverEvidence `json:"weaver_evidence"`
	InputManifestSHA256 string         `json:"input_manifest_sha256"`
	InputLock           any            `json:"input_lock"`
	Inputs              []sourceDigest `json:"inputs"`
	GeneratorSources    []sourceDigest `json:"generator_sources"`
	RuntimeSources      []sourceDigest `json:"runtime_sources"`
	// GeneratedWeavesSHA256 binds the emitted zz_generated_weaves.go bytes, so
	// hand-editing the compiled weave rules cannot pass the release gates.
	GeneratedWeavesSHA256 string           `json:"generated_weaves_sha256"`
	Fallback              fallbackManifest `json:"fallback"`
	Sources               []manifestSource `json:"sources"`
}

type weaverEvidence struct {
	SHA256    string         `json:"sha256"`
	Artifacts []sourceDigest `json:"artifacts"`
}

type fallbackManifest struct {
	Policy     string         `json:"policy"`
	KeyRules   map[string]int `json:"key_rules"`
	ValueRules map[string]int `json:"value_rules"`
	NameRules  map[string]int `json:"name_rules"`
}

type runtimeGolden struct {
	SchemaVersion         int                 `json:"schema_version"`
	SourceManifestSHA256  string              `json:"source_manifest_sha256"`
	Sources               []goldenSource      `json:"sources"`
	KeyRules              []typedir.KeyRule   `json:"key_rules"`
	KeyPriorities         []goldenKeyPriority `json:"key_priorities"`
	ValueRules            []typedir.ValueRule `json:"value_rules"`
	NameRules             []typedir.NameRule  `json:"name_rules"`
	FallbackKeyRules      []typedir.KeyRule   `json:"fallback_key_rules"`
	FallbackKeyPriorities []goldenKeyPriority `json:"fallback_key_priorities"`
	FallbackValueRules    []typedir.ValueRule `json:"fallback_value_rules"`
	FallbackNameRules     []typedir.NameRule  `json:"fallback_name_rules"`
	NegativeExecutable    map[string]int      `json:"negative_executable"`
}

type goldenSource struct {
	URL     string `json:"url"`
	WeaveID uint16 `json:"weave_id"`
	State   string `json:"state"`
	Link    string `json:"link"`
}

type goldenKeyPriority struct {
	URL             string            `json:"url"`
	Domain          typedir.Domain    `json:"domain"`
	Match           typedir.MatchKind `json:"match"`
	SourceSpanName  string            `json:"source_span_name,omitempty"`
	SourceEventName string            `json:"source_event_name,omitempty"`
	Source          string            `json:"source"`
	Target          string            `json:"target"`
	Priority        uint16            `json:"priority"`
}

type fallbackRuleUnion struct {
	keyRules   []typedir.KeyRule
	valueRules []typedir.ValueRule
	nameRules  []typedir.NameRule
}

// Generate re-runs the verified compiler, emits twice, and returns artifacts
// only when both path sets and bytes are identical.
func Generate(fixtureRoot, weaverRoot, providerRoot string) (ArtifactSet, error) {
	bundle, err := typedir.Compile(typedir.Config{
		FixtureRoot: fixtureRoot,
		WeaverRoot:  weaverRoot,
	})
	if err != nil {
		return nil, err
	}
	identity, err := collectIdentity(fixtureRoot, providerRoot)
	if err != nil {
		return nil, err
	}
	first, err := Emit(bundle, identity)
	if err != nil {
		return nil, err
	}
	second, err := Emit(bundle, identity)
	if err != nil {
		return nil, err
	}
	if err := equalArtifacts(first, second); err != nil {
		return nil, fmt.Errorf("repeatability gate: %w", err)
	}
	return first, nil
}

type identityInputs struct {
	inputLock        any
	inputs           []sourceDigest
	inputManifestSHA string
	generatorSources []sourceDigest
	runtimeSources   []sourceDigest
	policy           releasePolicy
}

// Emit converts a validated bundle into all runtime artifacts without I/O.
func Emit(bundle typedir.Bundle, identity identityInputs) (ArtifactSet, error) {
	if err := bundle.Validate(); err != nil {
		return nil, fmt.Errorf("validate Typed IR before emission: %w", err)
	}
	sourceIDs, err := assignSourceIDs(bundle)
	if err != nil {
		return nil, err
	}
	fallback, err := buildFallbackRuleUnion(bundle)
	if err != nil {
		return nil, err
	}
	// Weaves are emitted before the manifest so the manifest can bind the exact
	// generated bytes; the support manifest carries the manifest digest and so
	// must stay downstream of both to avoid a self-hash.
	weaves, err := emitWeaves(bundle, sourceIDs, fallback)
	if err != nil {
		return nil, err
	}
	weavesSum := sha256.Sum256(weaves)
	manifest, err := buildSourceManifest(
		bundle, sourceIDs, identity, hex.EncodeToString(weavesSum[:]), fallback,
	)
	if err != nil {
		return nil, err
	}
	manifestBytes, err := canonicalJSON(manifest)
	if err != nil {
		return nil, err
	}
	manifestSum := sha256.Sum256(manifestBytes)
	manifestDigest := hex.EncodeToString(manifestSum[:])

	support, err := emitSupportManifest(manifestDigest)
	if err != nil {
		return nil, err
	}
	golden, err := emitGolden(bundle, sourceIDs, manifestDigest, fallback)
	if err != nil {
		return nil, err
	}
	return ArtifactSet{
		weavesPath: weaves, supportPath: support, manifestPath: manifestBytes, goldenPath: golden,
	}, nil
}

func collectIdentity(fixtureRoot, providerRoot string) (identityInputs, error) {
	lockData, err := os.ReadFile(filepath.Join(fixtureRoot, "input-lock.json"))
	if err != nil {
		return identityInputs{}, err
	}
	var lock any
	if err := strictDecode(lockData, &lock); err != nil {
		return identityInputs{}, fmt.Errorf("decode input lock for manifest: %w", err)
	}
	manifestData, err := os.ReadFile(filepath.Join(fixtureRoot, "checksums.txt"))
	if err != nil {
		return identityInputs{}, err
	}
	inputs, err := parseChecksums(manifestData)
	if err != nil {
		return identityInputs{}, err
	}
	policyData, err := os.ReadFile(filepath.Join(fixtureRoot, "audit", "required-source-urls.yaml"))
	if err != nil {
		return identityInputs{}, err
	}
	var policy releasePolicy
	if err := strictDecode(policyData, &policy); err != nil {
		return identityInputs{}, fmt.Errorf("decode release policy for manifest: %w", err)
	}
	if policy.SchemaVersion != 1 || policy.Status != "release-ready" ||
		policy.CanonicalTarget != typedir.CanonicalURL {
		return identityInputs{}, errors.New("release policy identity/status changed after compiler gate")
	}
	generatorSources, err := collectGeneratorSources(providerRoot)
	if err != nil {
		return identityInputs{}, err
	}
	runtimeSources, err := digestExplicitFiles(providerRoot, []string{
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
	})
	if err != nil {
		return identityInputs{}, err
	}
	sum := sha256.Sum256(manifestData)
	return identityInputs{
		inputLock: lock, inputs: inputs, inputManifestSHA: hex.EncodeToString(sum[:]),
		generatorSources: generatorSources, runtimeSources: runtimeSources, policy: policy,
	}, nil
}

func collectGeneratorSources(providerRoot string) ([]sourceDigest, error) {
	moduleRoot := filepath.Join(providerRoot, "tools", "semconv-weave-gen")
	var paths []string
	err := filepath.WalkDir(moduleRoot, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("generator source symlink %q is not allowed", current)
		}
		if entry.IsDir() {
			name := entry.Name()
			if current != moduleRoot && (name == "build" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(providerRoot, current)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if strings.HasSuffix(relative, ".go") && !strings.HasSuffix(relative, "_test.go") ||
			relative == "tools/semconv-weave-gen/go.mod" ||
			relative == "tools/semconv-weave-gen/go.sum" {
			paths = append(paths, relative)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect generator sources: %w", err)
	}
	return digestExplicitFiles(providerRoot, paths)
}

func digestExplicitFiles(root string, paths []string) ([]sourceDigest, error) {
	sort.Strings(paths)
	result := make([]sourceDigest, 0, len(paths))
	previous := ""
	for _, path := range paths {
		if path == previous {
			return nil, fmt.Errorf("duplicate source path %q", path)
		}
		previous = path
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return nil, fmt.Errorf("read source identity %q: %w", path, err)
		}
		sum := sha256.Sum256(data)
		result = append(result, sourceDigest{Path: path, SHA256: hex.EncodeToString(sum[:])})
	}
	return result, nil
}

func parseChecksums(data []byte) ([]sourceDigest, error) {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return nil, errors.New("checksum manifest is empty or lacks final LF")
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	result := make([]sourceDigest, 0, len(lines))
	previous := ""
	for index, line := range lines {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || len(parts[0]) != 64 || parts[1] == "" {
			return nil, fmt.Errorf("checksum line %d is invalid", index+1)
		}
		if parts[1] <= previous {
			return nil, errors.New("checksum paths are duplicated or unsorted")
		}
		previous = parts[1]
		result = append(result, sourceDigest{Path: parts[1], SHA256: parts[0]})
	}
	return result, nil
}

type fallbackKeyIdentity struct {
	domain    typedir.Domain
	predicate typedir.Predicate
	match     typedir.MatchKind
	source    string
}

type fallbackValueIdentity struct {
	domain      typedir.Domain
	predicate   typedir.Predicate
	sourceKey   string
	sourceValue string
}

type fallbackNameIdentity struct {
	domain     typedir.Domain
	sourceName string
}

func buildFallbackRuleUnion(bundle typedir.Bundle) (fallbackRuleUnion, error) {
	var result fallbackRuleUnion
	keyIndexes := make(map[fallbackKeyIdentity]int)
	valueIndexes := make(map[fallbackValueIdentity]int)
	nameIndexes := make(map[fallbackNameIdentity]int)
	for _, source := range bundle.Sources {
		if source.CanonicalNoOp {
			continue
		}
		for _, rule := range source.KeyRules {
			identity := fallbackKeyIdentity{
				domain: rule.Domain, predicate: rule.Predicate,
				match: rule.Match, source: rule.Source,
			}
			if index, exists := keyIndexes[identity]; exists {
				current := &result.keyRules[index]
				if current.Target != rule.Target {
					return fallbackRuleUnion{}, fmt.Errorf(
						"fallback key ambiguity for %s/%s/%q: %q and %q",
						rule.Domain, rule.Match, rule.Source, current.Target, rule.Target,
					)
				}
				evidence, err := mergeFallbackEvidence(current.Evidence, rule.Evidence)
				if err != nil {
					return fallbackRuleUnion{}, fmt.Errorf("fallback key %q: %w", rule.Source, err)
				}
				current.Evidence = evidence
				continue
			}
			keyIndexes[identity] = len(result.keyRules)
			clone := rule
			clone.Evidence = append([]typedir.Evidence(nil), rule.Evidence...)
			result.keyRules = append(result.keyRules, clone)
		}
		for _, rule := range source.ValueRules {
			identity := fallbackValueIdentity{
				domain: rule.Domain, predicate: rule.Predicate,
				sourceKey: rule.SourceKey, sourceValue: rule.SourceValue,
			}
			if index, exists := valueIndexes[identity]; exists {
				current := &result.valueRules[index]
				if current.TargetKey != rule.TargetKey || current.TargetValue != rule.TargetValue {
					return fallbackRuleUnion{}, fmt.Errorf(
						"fallback value ambiguity for %s/%q=%q: %q=%q and %q=%q",
						rule.Domain, rule.SourceKey, rule.SourceValue,
						current.TargetKey, current.TargetValue, rule.TargetKey, rule.TargetValue,
					)
				}
				evidence, err := mergeFallbackEvidence(current.Evidence, rule.Evidence)
				if err != nil {
					return fallbackRuleUnion{}, fmt.Errorf(
						"fallback value %q=%q: %w", rule.SourceKey, rule.SourceValue, err,
					)
				}
				current.Evidence = evidence
				continue
			}
			valueIndexes[identity] = len(result.valueRules)
			clone := rule
			clone.Evidence = append([]typedir.Evidence(nil), rule.Evidence...)
			result.valueRules = append(result.valueRules, clone)
		}
		for _, rule := range source.NameRules {
			identity := fallbackNameIdentity{domain: rule.Domain, sourceName: rule.SourceName}
			if index, exists := nameIndexes[identity]; exists {
				current := &result.nameRules[index]
				if current.TargetName != rule.TargetName {
					return fallbackRuleUnion{}, fmt.Errorf(
						"fallback name ambiguity for %s/%q: %q and %q",
						rule.Domain, rule.SourceName, current.TargetName, rule.TargetName,
					)
				}
				evidence, err := mergeFallbackEvidence(current.Evidence, rule.Evidence)
				if err != nil {
					return fallbackRuleUnion{}, fmt.Errorf("fallback name %q: %w", rule.SourceName, err)
				}
				current.Evidence = evidence
				continue
			}
			nameIndexes[identity] = len(result.nameRules)
			clone := rule
			clone.Evidence = append([]typedir.Evidence(nil), rule.Evidence...)
			result.nameRules = append(result.nameRules, clone)
		}
	}
	sortFallbackRules(&result)
	return result, nil
}

func mergeFallbackEvidence(
	current []typedir.Evidence,
	incoming []typedir.Evidence,
) ([]typedir.Evidence, error) {
	result := append([]typedir.Evidence(nil), current...)
	indexes := make(map[string]int, len(result))
	for index, evidence := range result {
		indexes[evidence.StableID] = index
	}
	for _, evidence := range incoming {
		if index, exists := indexes[evidence.StableID]; exists {
			if result[index] != evidence {
				return nil, fmt.Errorf("evidence %q disagrees across legacy weaves", evidence.StableID)
			}
			continue
		}
		indexes[evidence.StableID] = len(result)
		result = append(result, evidence)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StableID < result[j].StableID })
	return result, nil
}

func sortFallbackRules(union *fallbackRuleUnion) {
	sort.Slice(union.keyRules, func(i, j int) bool {
		left, right := union.keyRules[i], union.keyRules[j]
		return string(left.Domain)+"\x00"+string(left.Match)+"\x00"+left.Source+"\x00"+
			left.Predicate.SourceSpanName+"\x00"+left.Predicate.SourceEventName <
			string(right.Domain)+"\x00"+string(right.Match)+"\x00"+right.Source+"\x00"+
				right.Predicate.SourceSpanName+"\x00"+right.Predicate.SourceEventName
	})
	sort.Slice(union.valueRules, func(i, j int) bool {
		left, right := union.valueRules[i], union.valueRules[j]
		return string(left.Domain)+"\x00"+left.SourceKey+"\x00"+left.SourceValue+"\x00"+
			left.Predicate.SourceSpanName+"\x00"+left.Predicate.SourceEventName <
			string(right.Domain)+"\x00"+right.SourceKey+"\x00"+right.SourceValue+"\x00"+
				right.Predicate.SourceSpanName+"\x00"+right.Predicate.SourceEventName
	})
	sort.Slice(union.nameRules, func(i, j int) bool {
		return string(union.nameRules[i].Domain)+"\x00"+union.nameRules[i].SourceName <
			string(union.nameRules[j].Domain)+"\x00"+union.nameRules[j].SourceName
	})
}

func assignSourceIDs(bundle typedir.Bundle) (map[string]uint16, error) {
	result := map[string]uint16{typedir.CanonicalURL: 2}
	next := uint16(3)
	for _, source := range bundle.Sources {
		if source.CanonicalNoOp {
			continue
		}
		if source.SourceURL == "" {
			return nil, errors.New("historical source lacks URL")
		}
		result[source.SourceURL] = next
		next++
	}
	if len(result) != 3 {
		return nil, fmt.Errorf("generated source ID count = %d, want 3", len(result))
	}
	return result, nil
}

func buildSourceManifest(
	bundle typedir.Bundle,
	sourceIDs map[string]uint16,
	identity identityInputs,
	weavesDigest string,
	fallback fallbackRuleUnion,
) (sourceManifest, error) {
	policyByURL := make(map[string]domainPolicy, len(identity.policy.RequiredURLs))
	for _, source := range identity.policy.RequiredURLs {
		if _, exists := policyByURL[source.SchemaURL]; exists {
			return sourceManifest{}, fmt.Errorf("duplicate policy URL %q", source.SchemaURL)
		}
		if err := validateAtomicDomains(source.SchemaURL, source.Domains); err != nil {
			return sourceManifest{}, err
		}
		policyByURL[source.SchemaURL] = source.Domains
	}
	if len(policyByURL) != len(bundle.Sources) {
		return sourceManifest{}, errors.New("required URL policy and Typed IR source set differ")
	}
	sources := make([]manifestSource, 0, len(bundle.Sources))
	for _, source := range bundle.Sources {
		domains, ok := policyByURL[source.SourceURL]
		if !ok {
			return sourceManifest{}, fmt.Errorf("Typed IR URL %q is absent from required policy", source.SourceURL)
		}
		state := "supported-legacy"
		if source.CanonicalNoOp {
			state = "canonical"
		}
		sources = append(sources, manifestSource{
			URL: source.SourceURL, Version: source.SourceVersion,
			WeaveID: sourceIDs[source.SourceURL], State: state, Link: "pass_through", Domains: domains,
			KeyRules:     countKeyRules(source.KeyRules),
			ValueRules:   countValueRules(source.ValueRules),
			NameRules:    countNameRules(source.NameRules),
			Dispositions: countDispositions(source.Dispositions),
		})
	}
	weaverArtifacts := make([]sourceDigest, 0, len(bundle.WeaverArtifacts))
	for _, artifact := range bundle.WeaverArtifacts {
		weaverArtifacts = append(weaverArtifacts, sourceDigest{
			Path: artifact.Path, SHA256: artifact.SHA256,
		})
	}
	return sourceManifest{
		SchemaVersion: 2, CanonicalURL: typedir.CanonicalURL, RuntimeABIVersion: 1,
		TypedIRInputDigest: bundle.InputDigest, InputManifestSHA256: identity.inputManifestSHA,
		WeaverEvidence: weaverEvidence{
			SHA256: bundle.WeaverEvidenceSHA256, Artifacts: weaverArtifacts,
		},
		InputLock: identity.inputLock, Inputs: identity.inputs,
		GeneratorSources: identity.generatorSources, RuntimeSources: identity.runtimeSources,
		GeneratedWeavesSHA256: weavesDigest,
		Fallback: fallbackManifest{
			Policy:     "unversioned_union",
			KeyRules:   countKeyRules(fallback.keyRules),
			ValueRules: countValueRules(fallback.valueRules),
			NameRules:  countNameRules(fallback.nameRules),
		},
		Sources: sources,
	}, nil
}

func validateAtomicDomains(url string, domains domainPolicy) error {
	values := []string{domains.Resources, domains.Spans, domains.SpanEvents, domains.SpanNames, domains.EventNames}
	for _, value := range values {
		if url == typedir.CanonicalURL {
			if value != "canonical_noop" {
				return fmt.Errorf("canonical URL domain is %q, want canonical_noop", value)
			}
			continue
		}
		if value != "translated" && value != "pass_through" {
			return fmt.Errorf("required URL %q has incomplete domain disposition %q", url, value)
		}
	}
	return nil
}

func countKeyRules(rules []typedir.KeyRule) map[string]int {
	result := zeroDomainCounts()
	for _, rule := range rules {
		result[string(rule.Domain)]++
	}
	return result
}

func countValueRules(rules []typedir.ValueRule) map[string]int {
	result := zeroDomainCounts()
	for _, rule := range rules {
		result[string(rule.Domain)]++
	}
	return result
}

func countNameRules(rules []typedir.NameRule) map[string]int {
	result := zeroDomainCounts()
	for _, rule := range rules {
		result[string(rule.Domain)]++
	}
	return result
}

func countDispositions(items []typedir.Disposition) map[string]int {
	result := map[string]int{"link": 0, "resource": 0, "span": 0, "span_event": 0}
	for _, item := range items {
		result[string(item.Domain)]++
	}
	return result
}

func zeroDomainCounts() map[string]int {
	return map[string]int{"resource": 0, "span": 0, "span_event": 0}
}

func emitWeaves(
	bundle typedir.Bundle,
	sourceIDs map[string]uint16,
	fallback fallbackRuleUnion,
) ([]byte, error) {
	var output strings.Builder
	output.WriteString("// Code generated by semconv-weave-gen; DO NOT EDIT.\n")
	output.WriteString("// SPDX-License-Identifier: Apache-2.0\n\n")
	output.WriteString("package semconvweave\n\n")
	output.WriteString("const (\n")
	for _, source := range bundle.Sources {
		if source.CanonicalNoOp {
			continue
		}
		fmt.Fprintf(&output, "\t%s WeaveID = %d\n", generatedIDName(source.SourceVersion), sourceIDs[source.SourceURL])
	}
	output.WriteString(")\n\n")

	sortedSources := append([]typedir.SourceIR(nil), bundle.Sources...)
	sort.Slice(sortedSources, func(i, j int) bool { return sortedSources[i].SourceURL < sortedSources[j].SourceURL })
	output.WriteString("var generatedSources = [...]generatedSource{\n")
	for _, source := range sortedSources {
		if source.CanonicalNoOp {
			continue
		}
		fmt.Fprintf(&output, "\t{url: %s, id: %s},\n", strconv.Quote(source.SourceURL), generatedIDName(source.SourceVersion))
	}
	output.WriteString("}\n\n")

	fallbackPriorities, err := keyRulePriorities(fallback.keyRules)
	if err != nil {
		return nil, err
	}
	output.WriteString("var generatedFallbackKeyRules = [...]generatedKeyRule{\n")
	for index, rule := range fallback.keyRules {
		fmt.Fprintf(
			&output,
			"\t{weave: WeaveUnknown, domain: %s, match: %s, priority: %d, sourceSpanName: %s, sourceEventName: %s, source: %s, target: %s},\n",
			generatedDomainName(rule.Domain), generatedMatchName(rule.Match), fallbackPriorities[index],
			strconv.Quote(rule.Predicate.SourceSpanName), strconv.Quote(rule.Predicate.SourceEventName),
			strconv.Quote(rule.Source), strconv.Quote(rule.Target),
		)
	}
	output.WriteString("}\n\n")

	output.WriteString("var generatedFallbackValueRules = [...]generatedValueRule{\n")
	for _, rule := range fallback.valueRules {
		fmt.Fprintf(
			&output,
			"\t{weave: WeaveUnknown, domain: %s, sourceSpanName: %s, sourceEventName: %s, sourceKey: %s, targetKey: %s, sourceValue: %s, targetValue: %s},\n",
			generatedDomainName(rule.Domain),
			strconv.Quote(rule.Predicate.SourceSpanName), strconv.Quote(rule.Predicate.SourceEventName),
			strconv.Quote(rule.SourceKey), strconv.Quote(rule.TargetKey),
			strconv.Quote(rule.SourceValue), strconv.Quote(rule.TargetValue),
		)
	}
	output.WriteString("}\n\n")

	output.WriteString("var generatedFallbackNameRules = [...]generatedNameRule{\n")
	for _, rule := range fallback.nameRules {
		fmt.Fprintf(
			&output,
			"\t{weave: WeaveUnknown, domain: %s, sourceName: %s, targetName: %s},\n",
			generatedDomainName(rule.Domain),
			strconv.Quote(rule.SourceName), strconv.Quote(rule.TargetName),
		)
	}
	output.WriteString("}\n\n")

	output.WriteString("var generatedKeyRules = [...]generatedKeyRule{\n")
	for _, source := range bundle.Sources {
		if source.CanonicalNoOp {
			continue
		}
		priorities, err := keyRulePriorities(source.KeyRules)
		if err != nil {
			return nil, err
		}
		for index, rule := range source.KeyRules {
			fmt.Fprintf(
				&output,
				"\t{weave: %s, domain: %s, match: %s, priority: %d, sourceSpanName: %s, sourceEventName: %s, source: %s, target: %s},\n",
				generatedIDName(source.SourceVersion), generatedDomainName(rule.Domain), generatedMatchName(rule.Match),
				priorities[index],
				strconv.Quote(rule.Predicate.SourceSpanName), strconv.Quote(rule.Predicate.SourceEventName),
				strconv.Quote(rule.Source), strconv.Quote(rule.Target),
			)
		}
	}
	output.WriteString("}\n\n")

	output.WriteString("var generatedValueRules = [...]generatedValueRule{\n")
	for _, source := range bundle.Sources {
		if source.CanonicalNoOp {
			continue
		}
		for _, rule := range source.ValueRules {
			fmt.Fprintf(
				&output,
				"\t{weave: %s, domain: %s, sourceSpanName: %s, sourceEventName: %s, sourceKey: %s, targetKey: %s, sourceValue: %s, targetValue: %s},\n",
				generatedIDName(source.SourceVersion), generatedDomainName(rule.Domain),
				strconv.Quote(rule.Predicate.SourceSpanName), strconv.Quote(rule.Predicate.SourceEventName),
				strconv.Quote(rule.SourceKey), strconv.Quote(rule.TargetKey),
				strconv.Quote(rule.SourceValue), strconv.Quote(rule.TargetValue),
			)
		}
	}
	output.WriteString("}\n\n")

	output.WriteString("var generatedNameRules = [...]generatedNameRule{\n")
	for _, source := range bundle.Sources {
		if source.CanonicalNoOp {
			continue
		}
		for _, rule := range source.NameRules {
			fmt.Fprintf(
				&output,
				"\t{weave: %s, domain: %s, sourceName: %s, targetName: %s},\n",
				generatedIDName(source.SourceVersion), generatedDomainName(rule.Domain),
				strconv.Quote(rule.SourceName), strconv.Quote(rule.TargetName),
			)
		}
	}
	output.WriteString("}\n\n")
	output.WriteString("func init() {\n")
	output.WriteString("\tinstallGeneratedData(generatedData{\n")
	output.WriteString("\t\tsources: generatedSources[:],\n")
	output.WriteString("\t\tkeyRules: generatedKeyRules[:],\n")
	output.WriteString("\t\tvalueRules: generatedValueRules[:],\n")
	output.WriteString("\t\tnameRules: generatedNameRules[:],\n")
	output.WriteString("\t\tfallbackKeyRules: generatedFallbackKeyRules[:],\n")
	output.WriteString("\t\tfallbackValueRules: generatedFallbackValueRules[:],\n")
	output.WriteString("\t\tfallbackNameRules: generatedFallbackNameRules[:],\n")
	output.WriteString("\t})\n")
	output.WriteString("}\n")
	return format.Source([]byte(output.String()))
}

func emitSupportManifest(digest string) ([]byte, error) {
	source := fmt.Sprintf(`// Code generated by semconv-weave-gen; DO NOT EDIT.
// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// SourceManifestSHA256 identifies the checked-in canonical source manifest.
const SourceManifestSHA256 = %q

const generatedExpectedRuntimeABIVersion uint16 = 1

var (
	_ [int(RuntimeABIVersion)-int(generatedExpectedRuntimeABIVersion)]struct{}
	_ [int(generatedExpectedRuntimeABIVersion)-int(RuntimeABIVersion)]struct{}

	_ [int(WeaveUnknown)-0]struct{}
	_ [0-int(WeaveUnknown)]struct{}
	_ [int(WeaveMissing)-1]struct{}
	_ [1-int(WeaveMissing)]struct{}
	_ [int(WeaveCanonical)-2]struct{}
	_ [2-int(WeaveCanonical)]struct{}

	_ [int(SchemaUnknown)-0]struct{}
	_ [0-int(SchemaUnknown)]struct{}
	_ [int(SchemaMissing)-1]struct{}
	_ [1-int(SchemaMissing)]struct{}
	_ [int(SchemaCanonical)-2]struct{}
	_ [2-int(SchemaCanonical)]struct{}
	_ [int(SchemaSupportedLegacy)-3]struct{}
	_ [3-int(SchemaSupportedLegacy)]struct{}

	_ [int(AttributeSuppress)-0]struct{}
	_ [0-int(AttributeSuppress)]struct{}
	_ [int(AttributeKeepValue)-1]struct{}
	_ [1-int(AttributeKeepValue)]struct{}
	_ [int(AttributeReplaceString)-2]struct{}
	_ [2-int(AttributeReplaceString)]struct{}

	_ = AttributeDecision{Key: "", StringValue: "", Action: AttributeKeepValue}
)

type generatedRuntimeABIV1 interface {
	State() SchemaState
	ResourceAttribute(pcommon.Map, string, pcommon.Value) AttributeDecision
	SpanAttribute(pcommon.Map, string, string, pcommon.Value) AttributeDecision
	EventAttribute(pcommon.Map, string, string, string, pcommon.Value) AttributeDecision
	SpanName(string) string
	EventName(string, string) string
}

var (
	_ generatedRuntimeABIV1 = WeaveID(0)
	_ func(string) WeaveID = Resolve
)
`, digest)
	return format.Source([]byte(source))
}

func emitGolden(
	bundle typedir.Bundle,
	sourceIDs map[string]uint16,
	manifestDigest string,
	fallback fallbackRuleUnion,
) ([]byte, error) {
	golden := runtimeGolden{
		SchemaVersion: 1, SourceManifestSHA256: manifestDigest,
		FallbackKeyRules:   fallback.keyRules,
		FallbackValueRules: fallback.valueRules,
		FallbackNameRules:  fallback.nameRules,
		NegativeExecutable: map[string]int{"db.statement": 0, "http.method": 0, "http.target": 0},
	}
	fallbackPriorities, err := keyRulePriorities(fallback.keyRules)
	if err != nil {
		return nil, err
	}
	for index, rule := range fallback.keyRules {
		golden.FallbackKeyPriorities = append(
			golden.FallbackKeyPriorities,
			goldenKeyPriority{
				URL: "unversioned-fallback", Domain: rule.Domain, Match: rule.Match,
				SourceSpanName:  rule.Predicate.SourceSpanName,
				SourceEventName: rule.Predicate.SourceEventName,
				Source:          rule.Source, Target: rule.Target, Priority: fallbackPriorities[index],
			},
		)
	}
	for _, source := range bundle.Sources {
		state := "supported-legacy"
		if source.CanonicalNoOp {
			state = "canonical"
		}
		golden.Sources = append(golden.Sources, goldenSource{
			URL: source.SourceURL, WeaveID: sourceIDs[source.SourceURL], State: state, Link: "pass_through",
		})
		priorities, err := keyRulePriorities(source.KeyRules)
		if err != nil {
			return nil, err
		}
		for index, rule := range source.KeyRules {
			golden.KeyPriorities = append(golden.KeyPriorities, goldenKeyPriority{
				URL: source.SourceURL, Domain: rule.Domain, Match: rule.Match,
				SourceSpanName:  rule.Predicate.SourceSpanName,
				SourceEventName: rule.Predicate.SourceEventName,
				Source:          rule.Source, Target: rule.Target, Priority: priorities[index],
			})
		}
		golden.KeyRules = append(golden.KeyRules, source.KeyRules...)
		golden.ValueRules = append(golden.ValueRules, source.ValueRules...)
		golden.NameRules = append(golden.NameRules, source.NameRules...)
	}
	for _, rule := range append(golden.KeyRules, golden.FallbackKeyRules...) {
		if _, negative := golden.NegativeExecutable[rule.Source]; negative {
			golden.NegativeExecutable[rule.Source]++
		}
	}
	for key, count := range golden.NegativeExecutable {
		if count != 0 {
			return nil, fmt.Errorf("negative key %q entered runtime golden", key)
		}
	}
	return canonicalJSON(golden)
}

func keyRulePriorities(rules []typedir.KeyRule) ([]uint16, error) {
	type candidate struct {
		index   int
		version int
		source  string
	}
	groupIndexes := make(map[string][]int)
	for index, rule := range rules {
		group := string(rule.Domain) + "\x00" + string(rule.Match) + "\x00" +
			rule.Predicate.SourceSpanName + "\x00" + rule.Predicate.SourceEventName + "\x00" + rule.Target
		groupIndexes[group] = append(groupIndexes[group], index)
	}
	result := make([]uint16, len(rules))
	for _, indexes := range groupIndexes {
		candidates := make([]candidate, 0, len(indexes))
		for _, index := range indexes {
			version, err := sourceRevision(rules[index].Source, rules, indexes)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, candidate{
				index: index, version: version, source: rules[index].Source,
			})
		}
		sort.Slice(candidates, func(i, j int) bool {
			// Schema 1.13 replaced net.{host,peer}.ip with socket addresses.
			// Guide rules have no revision field, so lexical tie-breaking alone
			// incorrectly lets the older IP alias win over its replacement.
			newerI := isSocketAddressAlias(candidates[i].source)
			newerJ := isSocketAddressAlias(candidates[j].source)
			if newerI != newerJ {
				return newerI
			}
			if candidates[i].version != candidates[j].version {
				return candidates[i].version > candidates[j].version
			}
			return candidates[i].source < candidates[j].source
		})
		for index, candidate := range candidates {
			if index >= int(^uint16(0)) {
				return nil, errors.New("legacy priority exceeds uint16")
			}
			result[candidate.index] = uint16(index + 1)
		}
	}
	return result, nil
}

func sourceRevision(source string, rules []typedir.KeyRule, indexes []int) (int, error) {
	maxVersion := 0
	for _, index := range indexes {
		for _, evidence := range rules[index].Evidence {
			if !operationSourceMatches(source, evidence.OperationSource) || evidence.Revision == "" {
				continue
			}
			version, err := packedVersion(evidence.Revision, source)
			if err != nil {
				return 0, err
			}
			if version > maxVersion {
				maxVersion = version
			}
		}
	}
	return maxVersion, nil
}

func operationSourceMatches(source, operationSource string) bool {
	if source == operationSource {
		return true
	}
	return strings.HasSuffix(operationSource, "<key>") &&
		source == strings.TrimSuffix(operationSource, "<key>")
}

func packedVersion(revision, source string) (int, error) {
	parts := strings.Split(revision, ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("rule %q has invalid evidence revision %q", source, revision)
	}
	values := [3]int{}
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 || value > 999 {
			return 0, fmt.Errorf("rule %q has invalid evidence revision %q", source, revision)
		}
		values[index] = value
	}
	return values[0]*1_000_000 + values[1]*1_000 + values[2], nil
}

func generatedIDName(version string) string {
	return "weaveLegacy" + strings.ReplaceAll(version, ".", "_")
}

func generatedDomainName(domain typedir.Domain) string {
	switch domain {
	case typedir.DomainResource:
		return "generatedDomainResource"
	case typedir.DomainSpan:
		return "generatedDomainSpan"
	case typedir.DomainSpanEvent:
		return "generatedDomainSpanEvent"
	default:
		panic("validated Typed IR contains unknown domain")
	}
}

func generatedMatchName(match typedir.MatchKind) string {
	if match == typedir.MatchPrefix {
		return "generatedMatchPrefix"
	}
	return "generatedMatchExact"
}

func canonicalJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	data, err = json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func strictDecode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func equalArtifacts(left, right ArtifactSet) error {
	if len(left) != len(right) {
		return fmt.Errorf("artifact count differs: %d vs %d", len(left), len(right))
	}
	paths := make([]string, 0, len(left))
	for path := range left {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		rightData, ok := right[path]
		if !ok {
			return fmt.Errorf("artifact %q is missing from second emission", path)
		}
		if !bytes.Equal(left[path], rightData) {
			return fmt.Errorf("artifact %q differs", path)
		}
	}
	return nil
}

// WriteFresh writes a verified artifact set into a previously absent staging
// directory. Checked-in promotion remains a separate Git change-set action.
func WriteFresh(root string, artifacts ArtifactSet) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := validateStagingLocation(root, workingDirectory); err != nil {
		return err
	}
	if _, err := os.Lstat(root); err == nil {
		return fmt.Errorf("staging output %q already exists", root)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		target := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, artifacts[path], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func validateStagingLocation(root, workingDirectory string) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absoluteWorking, err := filepath.Abs(workingDirectory)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(absoluteWorking, absoluteRoot)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil
	}
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		if strings.HasPrefix(segment, "_") || strings.HasPrefix(segment, ".") {
			return nil
		}
	}
	return fmt.Errorf(
		"staging output %q is inside the Go module but has no '_' or '.'-prefixed ignored directory",
		root,
	)
}

// CheckArtifacts verifies that providerRoot contains exactly the generated
// bytes for every artifact path.
func CheckArtifacts(providerRoot string, artifacts ArtifactSet) error {
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		actual, err := os.ReadFile(filepath.Join(providerRoot, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("read checked artifact %q: %w", path, err)
		}
		if !bytes.Equal(actual, artifacts[path]) {
			return fmt.Errorf("checked artifact %q differs from fresh emission", path)
		}
	}
	return nil
}

func isSocketAddressAlias(source string) bool {
	return source == "net.sock.peer.addr" || source == "net.sock.host.addr"
}
