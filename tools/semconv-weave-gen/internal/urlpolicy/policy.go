package urlpolicy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	canonicalURL = "https://opentelemetry.io/schemas/1.41.0"

	classifierOwner = "otel-semconv-normalizer/semconv-maintainers"
	collectorOwner  = "otel-semconv-normalizer/compatibility-maintainers"

	registryEvidence = "registry:v1.41.0@e018fe6f91862f5ed63c082f87697cddac596784#sha256=6e2a13d9ccac76327bbb79578eced858b14fddbc96facbf56aab16a5b19cee93"
)

var (
	versionLine = regexp.MustCompile(`^  ([0-9]+\.[0-9]+\.[0-9]+):\s*$`)
	exactURL    = regexp.MustCompile(`^https://opentelemetry\.io/schemas/([0-9]+\.[0-9]+\.[0-9]+)$`)
	hex40       = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hex64       = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type compatibilityMatrix struct {
	SchemaVersion   int         `json:"schema_version"`
	CanonicalTarget string      `json:"canonical_target"`
	Candidates      []candidate `json:"candidates"`
}

type candidate struct {
	Version              string   `json:"version"`
	SchemaURL            string   `json:"schema_url"`
	Requirement          string   `json:"requirement"`
	SourceRegistryStatus string   `json:"source_registry_status"`
	Reason               string   `json:"reason"`
	Evidence             []string `json:"evidence"`
}

type requiredPolicy struct {
	SchemaVersion      int           `json:"schema_version"`
	Status             string        `json:"status"`
	CanonicalTarget    string        `json:"canonical_target"`
	CandidateMatrixSHA string        `json:"candidate_matrix_sha256"`
	RequiredURLs       []requiredURL `json:"required_urls"`
	OptionalURLs       []optionalURL `json:"optional_urls"`
}

type requiredURL struct {
	SchemaURL            string      `json:"schema_url"`
	Requirement          string      `json:"requirement"`
	Reason               string      `json:"reason"`
	ClassifierOwner      string      `json:"classifier_owner"`
	CollectorOwner       string      `json:"collector_owner"`
	Evidence             []string    `json:"evidence"`
	SourceRegistryStatus string      `json:"source_registry_status"`
	Domains              domainState `json:"domains"`
}

type optionalURL struct {
	SchemaURL   string `json:"schema_url"`
	Disposition string `json:"disposition"`
}

type domainState struct {
	Resources  string `json:"resources"`
	Spans      string `json:"spans"`
	SpanEvents string `json:"span_events"`
	SpanNames  string `json:"span_names"`
	EventNames string `json:"event_names"`
}

type collectorCorpus struct {
	SchemaVersion int                 `json:"schema_version"`
	Repository    string              `json:"repository"`
	Commit        string              `json:"commit"`
	Files         []corpusFile        `json:"files"`
	Observations  []corpusObservation `json:"observations"`
}

type corpusFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type corpusObservation struct {
	SchemaURL       string           `json:"schema_url"`
	Reason          string           `json:"reason"`
	ClassifierOwner string           `json:"classifier_owner"`
	CollectorOwner  string           `json:"collector_owner"`
	Evidence        []corpusEvidence `json:"evidence"`
}

type corpusEvidence struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}

type schemaInventory struct {
	URL      string
	Versions []string
}

func Generate(schema, corpusData []byte) ([]byte, []byte, error) {
	inventory, err := parseSchemaInventory(schema)
	if err != nil {
		return nil, nil, err
	}
	corpus, err := parseCorpus(corpusData, inventory)
	if err != nil {
		return nil, nil, err
	}
	observed := make(map[string]corpusObservation, len(corpus.Observations))
	for _, observation := range corpus.Observations {
		observed[observation.SchemaURL] = observation
	}

	schemaSum := sha256.Sum256(schema)
	matrix := compatibilityMatrix{
		SchemaVersion:   1,
		CanonicalTarget: canonicalURL,
		Candidates:      make([]candidate, 0, len(inventory.Versions)),
	}
	for _, version := range inventory.Versions {
		url := schemaURL(version)
		item := candidate{
			Version:              version,
			SchemaURL:            url,
			Requirement:          "optional",
			SourceRegistryStatus: "manual_audit_required",
			Reason:               "Official Schema candidate is not present in the pinned Collector versioned corpus.",
			Evidence:             []string{"schema:opentelemetry-1.41.0#versions/" + version},
		}
		if url == canonicalURL {
			item.Requirement = "required"
			item.SourceRegistryStatus = "parsed"
			item.Reason = "Canonical target is always required by the provider contract."
			item.Evidence = []string{
				"schema:opentelemetry-1.41.0#sha256=" + hex.EncodeToString(schemaSum[:]),
				registryEvidence,
			}
		} else if observation, ok := observed[url]; ok {
			item.Requirement = "required"
			item.Reason = observation.Reason
			item.Evidence = corpusEvidenceStrings(corpus, observation)
		}
		matrix.Candidates = append(matrix.Candidates, item)
	}
	matrixBytes, err := canonicalJSON(matrix)
	if err != nil {
		return nil, nil, err
	}
	matrixSum := sha256.Sum256(matrixBytes)

	policy := requiredPolicy{
		SchemaVersion:      1,
		Status:             "audit-in-progress",
		CanonicalTarget:    canonicalURL,
		CandidateMatrixSHA: hex.EncodeToString(matrixSum[:]),
	}
	for _, item := range matrix.Candidates {
		if item.Requirement == "required" {
			domains := manualDomains()
			if item.SchemaURL == canonicalURL {
				domains = canonicalDomains()
			}
			policy.RequiredURLs = append(policy.RequiredURLs, requiredURL{
				SchemaURL:            item.SchemaURL,
				Requirement:          item.Requirement,
				Reason:               item.Reason,
				ClassifierOwner:      classifierOwner,
				CollectorOwner:       collectorOwner,
				Evidence:             append([]string(nil), item.Evidence...),
				SourceRegistryStatus: item.SourceRegistryStatus,
				Domains:              domains,
			})
			continue
		}
		policy.OptionalURLs = append(policy.OptionalURLs, optionalURL{
			SchemaURL:   item.SchemaURL,
			Disposition: "unknown",
		})
	}
	policyBytes, err := canonicalJSON(policy)
	if err != nil {
		return nil, nil, err
	}
	if err := Validate(schema, matrixBytes, policyBytes, false); err != nil {
		return nil, nil, fmt.Errorf("validate generated URL policy: %w", err)
	}
	return matrixBytes, policyBytes, nil
}

func Validate(schema, matrixData, policyData []byte, releaseReady bool) error {
	inventory, err := parseSchemaInventory(schema)
	if err != nil {
		return err
	}
	var matrix compatibilityMatrix
	if err := strictDecode(matrixData, &matrix); err != nil {
		return fmt.Errorf("decode compatibility matrix: %w", err)
	}
	if err := validateMatrix(inventory, matrix); err != nil {
		return err
	}
	var policy requiredPolicy
	if err := strictDecode(policyData, &policy); err != nil {
		return fmt.Errorf("decode required URL policy: %w", err)
	}
	return validatePolicy(matrixData, matrix, policy, releaseReady)
}

func parseSchemaInventory(data []byte) (schemaInventory, error) {
	if len(data) == 0 {
		return schemaInventory{}, errors.New("Schema input is empty")
	}
	var inventory schemaInventory
	seen := make(map[string]struct{})
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "schema_url:") {
			if inventory.URL != "" {
				return schemaInventory{}, errors.New("Schema contains duplicate schema_url")
			}
			inventory.URL = strings.TrimSpace(strings.TrimPrefix(line, "schema_url:"))
			continue
		}
		match := versionLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		version := match[1]
		if _, ok := seen[version]; ok {
			return schemaInventory{}, fmt.Errorf("Schema contains duplicate version %q", version)
		}
		seen[version] = struct{}{}
		inventory.Versions = append(inventory.Versions, version)
	}
	if inventory.URL != canonicalURL {
		return schemaInventory{}, fmt.Errorf("Schema URL = %q, want %q", inventory.URL, canonicalURL)
	}
	if len(inventory.Versions) != 39 {
		return schemaInventory{}, fmt.Errorf("Schema candidate count = %d, want 39", len(inventory.Versions))
	}
	if inventory.Versions[0] != "1.41.0" || inventory.Versions[len(inventory.Versions)-1] != "1.4.0" {
		return schemaInventory{}, errors.New("Schema candidate boundaries do not match 1.41.0 through 1.4.0")
	}
	return inventory, nil
}

func parseCorpus(data []byte, inventory schemaInventory) (collectorCorpus, error) {
	var corpus collectorCorpus
	if err := strictDecode(data, &corpus); err != nil {
		return corpus, fmt.Errorf("decode Collector corpus: %w", err)
	}
	if corpus.SchemaVersion != 1 {
		return corpus, fmt.Errorf("Collector corpus schema_version = %d, want 1", corpus.SchemaVersion)
	}
	if corpus.Repository == "" || !hex40.MatchString(corpus.Commit) {
		return corpus, errors.New("Collector corpus repository/commit identity is invalid")
	}
	files := make(map[string]string, len(corpus.Files))
	lastPath := ""
	for _, file := range corpus.Files {
		if file.Path == "" || !hex64.MatchString(file.SHA256) {
			return corpus, fmt.Errorf("Collector corpus file identity is invalid for %q", file.Path)
		}
		if file.Path <= lastPath {
			return corpus, errors.New("Collector corpus files must be unique and sorted")
		}
		lastPath = file.Path
		files[file.Path] = file.SHA256
	}
	versions := make(map[string]struct{}, len(inventory.Versions))
	for _, version := range inventory.Versions {
		versions[schemaURL(version)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(corpus.Observations))
	for _, observation := range corpus.Observations {
		if _, ok := versions[observation.SchemaURL]; !ok || observation.SchemaURL == canonicalURL {
			return corpus, fmt.Errorf("Collector corpus URL %q is not a non-canonical candidate", observation.SchemaURL)
		}
		if _, ok := seen[observation.SchemaURL]; ok {
			return corpus, fmt.Errorf("Collector corpus URL %q is duplicated", observation.SchemaURL)
		}
		seen[observation.SchemaURL] = struct{}{}
		if observation.Reason == "" || observation.ClassifierOwner == "" || observation.CollectorOwner == "" {
			return corpus, fmt.Errorf("Collector corpus URL %q lacks reason or owner", observation.SchemaURL)
		}
		if observation.ClassifierOwner != classifierOwner || observation.CollectorOwner != collectorOwner {
			return corpus, fmt.Errorf("Collector corpus URL %q has unexpected owner", observation.SchemaURL)
		}
		if len(observation.Evidence) == 0 {
			return corpus, fmt.Errorf("Collector corpus URL %q lacks evidence", observation.SchemaURL)
		}
		for _, evidence := range observation.Evidence {
			if _, ok := files[evidence.Path]; !ok || evidence.Line <= 0 {
				return corpus, fmt.Errorf("Collector corpus URL %q has invalid evidence", observation.SchemaURL)
			}
		}
	}
	return corpus, nil
}

func validateMatrix(inventory schemaInventory, matrix compatibilityMatrix) error {
	if matrix.SchemaVersion != 1 || matrix.CanonicalTarget != canonicalURL {
		return errors.New("compatibility matrix identity is invalid")
	}
	if len(matrix.Candidates) != len(inventory.Versions) {
		return fmt.Errorf("compatibility matrix candidate count = %d, want %d", len(matrix.Candidates), len(inventory.Versions))
	}
	seen := make(map[string]struct{}, len(matrix.Candidates))
	for index, item := range matrix.Candidates {
		version := inventory.Versions[index]
		if item.Version != version || item.SchemaURL != schemaURL(version) {
			return fmt.Errorf("compatibility matrix candidate %d does not match Schema version %q", index, version)
		}
		if _, ok := seen[item.SchemaURL]; ok {
			return fmt.Errorf("compatibility matrix URL %q is duplicated", item.SchemaURL)
		}
		seen[item.SchemaURL] = struct{}{}
		if item.Requirement != "required" && item.Requirement != "optional" {
			return fmt.Errorf("compatibility matrix URL %q has invalid requirement", item.SchemaURL)
		}
		if item.SourceRegistryStatus != "parsed" && item.SourceRegistryStatus != "manual_audit_required" {
			return fmt.Errorf("compatibility matrix URL %q has invalid source Registry status", item.SchemaURL)
		}
		if item.Reason == "" || len(item.Evidence) == 0 {
			return fmt.Errorf("compatibility matrix URL %q lacks reason/evidence", item.SchemaURL)
		}
		if item.SchemaURL == canonicalURL {
			if item.Requirement != "required" || item.SourceRegistryStatus != "parsed" {
				return errors.New("canonical URL must be required and parsed")
			}
		} else if item.SourceRegistryStatus != "manual_audit_required" {
			return fmt.Errorf("historical URL %q must remain manual_audit_required without a pinned source Registry", item.SchemaURL)
		}
	}
	return nil
}

func validatePolicy(matrixData []byte, matrix compatibilityMatrix, policy requiredPolicy, releaseReady bool) error {
	if policy.SchemaVersion != 1 || policy.CanonicalTarget != canonicalURL {
		return errors.New("required URL policy identity is invalid")
	}
	matrixSum := sha256.Sum256(matrixData)
	if policy.CandidateMatrixSHA != hex.EncodeToString(matrixSum[:]) {
		return errors.New("required URL policy candidate matrix digest mismatch")
	}
	if policy.Status != "audit-in-progress" && policy.Status != "release-ready" {
		return fmt.Errorf("required URL policy status %q is invalid", policy.Status)
	}
	required := make(map[string]requiredURL, len(policy.RequiredURLs))
	for _, item := range policy.RequiredURLs {
		if _, ok := required[item.SchemaURL]; ok {
			return fmt.Errorf("required URL %q is duplicated", item.SchemaURL)
		}
		required[item.SchemaURL] = item
		if item.SchemaURL == canonicalURL && item.Requirement != "required" {
			return errors.New("canonical URL requirement must be required")
		}
		if item.Requirement != "required" || item.Reason == "" ||
			item.ClassifierOwner == "" || item.CollectorOwner == "" || len(item.Evidence) == 0 {
			return fmt.Errorf("required URL %q lacks requirement, reason, owner, or evidence", item.SchemaURL)
		}
		if item.ClassifierOwner != classifierOwner || item.CollectorOwner != collectorOwner {
			return fmt.Errorf("required URL %q has unexpected owner", item.SchemaURL)
		}
		if item.SourceRegistryStatus != "parsed" && item.SourceRegistryStatus != "manual_audit_required" {
			return fmt.Errorf("required URL %q has invalid source Registry status", item.SchemaURL)
		}
		if err := validateDomains(item.SchemaURL, item.Domains, releaseReady); err != nil {
			return err
		}
	}
	optional := make(map[string]optionalURL, len(policy.OptionalURLs))
	for _, item := range policy.OptionalURLs {
		if _, ok := optional[item.SchemaURL]; ok {
			return fmt.Errorf("optional URL %q is duplicated", item.SchemaURL)
		}
		optional[item.SchemaURL] = item
		if item.Disposition != "unknown" {
			return fmt.Errorf("optional disposition for %q = %q, want unknown", item.SchemaURL, item.Disposition)
		}
	}
	for _, item := range matrix.Candidates {
		requiredItem, isRequired := required[item.SchemaURL]
		_, isOptional := optional[item.SchemaURL]
		if isRequired == isOptional {
			return fmt.Errorf("URL %q must appear exactly once in required or optional policy", item.SchemaURL)
		}
		if item.Requirement == "required" {
			if !isRequired {
				if item.SchemaURL == canonicalURL {
					return errors.New("canonical URL must appear in required policy")
				}
				return fmt.Errorf("matrix-required URL %q is not required by policy", item.SchemaURL)
			}
			if requiredItem.SourceRegistryStatus != item.SourceRegistryStatus {
				return fmt.Errorf("required URL %q source Registry status disagrees with matrix", item.SchemaURL)
			}
		} else if !isOptional {
			return fmt.Errorf("matrix-optional URL %q is not optional/unknown", item.SchemaURL)
		}
	}
	if _, ok := required[canonicalURL]; !ok {
		return errors.New("canonical URL is not required")
	}
	if releaseReady && policy.Status != "release-ready" {
		return errors.New("release-ready validation requires policy status release-ready")
	}
	return nil
}

func validateDomains(url string, domains domainState, releaseReady bool) error {
	values := []struct {
		name  string
		value string
	}{
		{name: "resources", value: domains.Resources},
		{name: "spans", value: domains.Spans},
		{name: "span_events", value: domains.SpanEvents},
		{name: "span_names", value: domains.SpanNames},
		{name: "event_names", value: domains.EventNames},
	}
	if url == canonicalURL {
		for _, domain := range values {
			if domain.value != "canonical_noop" {
				return fmt.Errorf("canonical domain %s = %q, want canonical_noop", domain.name, domain.value)
			}
		}
		return nil
	}
	allowed := map[string]struct{}{
		"manual_audit_required": {},
		"translated":            {},
		"pass_through":          {},
		"unsupported":           {},
	}
	for _, domain := range values {
		if _, ok := allowed[domain.value]; !ok {
			return fmt.Errorf("required URL %q domain %s has invalid disposition %q", url, domain.name, domain.value)
		}
		if releaseReady && domain.value == "manual_audit_required" {
			return fmt.Errorf("required URL %q domain %s is manual_audit_required", url, domain.name)
		}
	}
	return nil
}

func corpusEvidenceStrings(corpus collectorCorpus, observation corpusObservation) []string {
	fileSums := make(map[string]string, len(corpus.Files))
	for _, file := range corpus.Files {
		fileSums[file.Path] = file.SHA256
	}
	evidence := make([]string, 0, len(observation.Evidence))
	for _, item := range observation.Evidence {
		evidence = append(evidence,
			corpus.Repository+"@"+corpus.Commit+":"+item.Path+":"+strconv.Itoa(item.Line)+"#sha256="+fileSums[item.Path],
		)
	}
	return evidence
}

func schemaURL(version string) string {
	return "https://opentelemetry.io/schemas/" + version
}

func manualDomains() domainState {
	return domainState{
		Resources:  "manual_audit_required",
		Spans:      "manual_audit_required",
		SpanEvents: "manual_audit_required",
		SpanNames:  "manual_audit_required",
		EventNames: "manual_audit_required",
	}
}

func canonicalDomains() domainState {
	return domainState{
		Resources:  "canonical_noop",
		Spans:      "canonical_noop",
		SpanEvents: "canonical_noop",
		SpanNames:  "canonical_noop",
		EventNames: "canonical_noop",
	}
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
		return fmt.Errorf("trailing JSON content: %w", err)
	}
	return nil
}

func canonicalJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func SortedRequiredURLs(policyData []byte) ([]string, error) {
	var policy requiredPolicy
	if err := strictDecode(policyData, &policy); err != nil {
		return nil, err
	}
	urls := make([]string, 0, len(policy.RequiredURLs))
	for _, item := range policy.RequiredURLs {
		urls = append(urls, item.SchemaURL)
	}
	sort.Strings(urls)
	return urls, nil
}

// PromoteRelease converts the checksum-locked audit draft into the atomic
// full-domain policy accepted by the release-ready validator. Callers must
// validate the signed combined migration audit before invoking this function.
func PromoteRelease(policyData []byte) ([]byte, error) {
	var policy requiredPolicy
	if err := strictDecode(policyData, &policy); err != nil {
		return nil, fmt.Errorf("decode draft required URL policy: %w", err)
	}
	if policy.SchemaVersion != 1 || policy.CanonicalTarget != canonicalURL ||
		policy.Status != "audit-in-progress" || len(policy.RequiredURLs) != 3 {
		return nil, errors.New("draft required URL policy identity/status is invalid")
	}
	expected := []string{
		canonicalURL,
		"https://opentelemetry.io/schemas/1.7.0",
		"https://opentelemetry.io/schemas/1.4.0",
	}
	for index := range expected {
		if policy.RequiredURLs[index].SchemaURL != expected[index] {
			return nil, errors.New("draft required URL set/order is invalid")
		}
	}
	policy.Status = "release-ready"
	for index := range policy.RequiredURLs {
		item := &policy.RequiredURLs[index]
		if item.SchemaURL == canonicalURL {
			continue
		}
		item.Domains = domainState{
			Resources:  "translated",
			Spans:      "translated",
			SpanEvents: "translated",
			SpanNames:  "pass_through",
			EventNames: "pass_through",
		}
	}
	return canonicalJSON(policy)
}
