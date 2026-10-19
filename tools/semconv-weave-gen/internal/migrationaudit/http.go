package migrationaudit

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/registrydiff"
)

const (
	canonicalTarget = "https://opentelemetry.io/schemas/1.41.0"
	reviewOwner     = "otel-semconv-normalizer/semconv-reviewers"
)

var (
	versionLine = regexp.MustCompile(`^  ([0-9]+\.[0-9]+\.[0-9]+):$`)
	domainLine  = regexp.MustCompile(`^    (all|resources|spans|span_events|span_names|event_names|metrics):$`)
	mappingLine = regexp.MustCompile(`^ {14}([a-z0-9_.]+): ([a-z0-9_.]+)$`)
)

type httpInventory struct {
	SchemaVersion   int              `json:"schema_version"`
	CanonicalTarget string           `json:"canonical_target"`
	Sources         inventorySources `json:"sources"`
	RequiredURLs    []string         `json:"required_source_urls"`
	Obligations     []obligation     `json:"obligations"`
}

type inventorySources struct {
	TelemetrySchemaSHA256 string `json:"telemetry_schema_sha256"`
	HTTPGuideSHA256       string `json:"http_guide_sha256"`
	WeaverDiffSHA256      string `json:"weaver_diff_sha256"`
}

type obligation struct {
	ID                  string   `json:"id"`
	Authority           string   `json:"authority"`
	Locator             string   `json:"locator"`
	Domain              string   `json:"domain"`
	Phase               string   `json:"phase"`
	Kind                string   `json:"kind"`
	SourceKey           string   `json:"source_key,omitempty"`
	TargetKeys          []string `json:"target_keys,omitempty"`
	Summary             string   `json:"summary"`
	TranslationEligible bool     `json:"translation_eligible"`
}

type httpDecisions struct {
	SchemaVersion int                       `json:"schema_version"`
	ReviewOwner   string                    `json:"review_owner"`
	WeaverDiff    []weaverDecisionSelection `json:"weaver_diff"`
	Decisions     []httpDecision            `json:"decisions"`
}

// weaverDecisionSelection keeps the human-owned decision catalog compact while
// still requiring an explicit action for every stable diff candidate. The
// generator derives only mechanical classification/reason/golden fields; it
// never chooses the action.
type weaverDecisionSelection struct {
	ObligationID string `json:"obligation_id"`
	Action       string `json:"action"`
}

type httpDecision struct {
	ObligationID   string       `json:"obligation_id"`
	Classification string       `json:"classification"`
	Action         string       `json:"action"`
	Reason         string       `json:"reason"`
	Golden         []goldenCase `json:"golden"`
}

type migrationAudit struct {
	SchemaVersion   int           `json:"schema_version"`
	Status          string        `json:"status"`
	CanonicalTarget string        `json:"canonical_target"`
	Sections        auditSections `json:"sections"`
	Signoffs        []signoff     `json:"signoffs"`
}

type auditSections struct {
	HTTPURLNetwork httpReview   `json:"http_url_network"`
	DBRPC          *dbRPCReview `json:"db_rpc,omitempty"`
}

type httpReview struct {
	Status                string          `json:"status"`
	SourceInventorySHA256 string          `json:"source_inventory_sha256"`
	Owner                 string          `json:"owner"`
	Entries               []reviewedEntry `json:"entries"`
}

type reviewedEntry struct {
	ID             string       `json:"id"`
	Authority      string       `json:"authority"`
	Locator        string       `json:"locator"`
	SourceURLs     []string     `json:"source_urls"`
	Domain         string       `json:"domain"`
	Phase          string       `json:"phase"`
	Classification string       `json:"classification"`
	Action         string       `json:"action"`
	Owner          string       `json:"owner"`
	Reason         string       `json:"reason"`
	SourceKey      string       `json:"source_key,omitempty"`
	TargetKeys     []string     `json:"target_keys,omitempty"`
	Golden         []goldenCase `json:"golden"`
}

type goldenCase struct {
	Name           string `json:"name"`
	InputKey       string `json:"input_key"`
	InputType      string `json:"input_type"`
	InputValue     string `json:"input_value"`
	TargetPresent  bool   `json:"target_present"`
	TargetValue    string `json:"target_value,omitempty"`
	ExpectedAction string `json:"expected_action"`
	ExpectedKey    string `json:"expected_key"`
	ExpectedType   string `json:"expected_type"`
	ExpectedValue  string `json:"expected_value"`
}

type signoff struct {
	Scope           string `json:"scope"`
	Role            string `json:"role"`
	Owner           string `json:"owner"`
	InventorySHA256 string `json:"inventory_sha256"`
	DBGuideSHA256   string `json:"db_guide_sha256"`
	RPCGuideSHA256  string `json:"rpc_guide_sha256"`
	PolicySHA256    string `json:"policy_sha256"`
	Status          string `json:"status"`
}

// GenerateHTTPInventory extracts the machine-owned Schema and Weaver
// obligations and a complete, deterministic enumeration of the trace sections
// in the pinned HTTP migration guide.
func GenerateHTTPInventory(schemaData, guideData, diffData []byte) ([]byte, error) {
	schemaObligations, err := extractHTTPSchemaObligations(schemaData)
	if err != nil {
		return nil, err
	}
	guideObligations, err := extractHTTPGuideObligations(guideData)
	if err != nil {
		return nil, err
	}
	diffObligations, err := extractHTTPDiffObligations(diffData)
	if err != nil {
		return nil, err
	}
	obligations := append(append(schemaObligations, guideObligations...), diffObligations...)
	sort.Slice(obligations, func(i, j int) bool { return obligations[i].ID < obligations[j].ID })
	for i := 1; i < len(obligations); i++ {
		if obligations[i-1].ID == obligations[i].ID {
			return nil, fmt.Errorf("duplicate HTTP obligation ID %q", obligations[i].ID)
		}
	}
	inventory := httpInventory{
		SchemaVersion:   1,
		CanonicalTarget: canonicalTarget,
		Sources: inventorySources{
			TelemetrySchemaSHA256: digest(schemaData),
			HTTPGuideSHA256:       digest(guideData),
			WeaverDiffSHA256:      digest(diffData),
		},
		RequiredURLs: []string{
			"https://opentelemetry.io/schemas/1.7.0",
			"https://opentelemetry.io/schemas/1.4.0",
		},
		Obligations: obligations,
	}
	return canonicalJSON(inventory)
}

// GenerateHTTPAudit joins the human-owned decision catalog to the generated
// inventory. Decisions remain a separate checked-in input; this function does
// not infer migration safety.
func GenerateHTTPAudit(inventoryData, decisionsData []byte) ([]byte, error) {
	inventory, decisions, err := decodeHTTPInputs(inventoryData, decisionsData)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]httpDecision, len(decisions.Decisions))
	for _, decision := range decisions.Decisions {
		if strings.HasPrefix(decision.ObligationID, "weaver-diff:") {
			return nil, fmt.Errorf(
				"Weaver diff decision %q must use the explicit weaver_diff catalog",
				decision.ObligationID,
			)
		}
		if _, ok := byID[decision.ObligationID]; ok {
			return nil, fmt.Errorf("duplicate HTTP decision %q", decision.ObligationID)
		}
		byID[decision.ObligationID] = decision
	}
	obligations := make(map[string]obligation, len(inventory.Obligations))
	for _, item := range inventory.Obligations {
		obligations[item.ID] = item
	}
	for _, selection := range decisions.WeaverDiff {
		item, ok := obligations[selection.ObligationID]
		if !ok || item.Authority != "weaver_diff" {
			return nil, fmt.Errorf("unknown HTTP Weaver diff decision %q", selection.ObligationID)
		}
		if _, exists := byID[selection.ObligationID]; exists {
			return nil, fmt.Errorf("duplicate HTTP decision %q", selection.ObligationID)
		}
		decision, err := deriveHTTPWeaverDecision(item, selection)
		if err != nil {
			return nil, err
		}
		byID[selection.ObligationID] = decision
	}
	entries := make([]reviewedEntry, 0, len(inventory.Obligations))
	for _, item := range inventory.Obligations {
		decision, ok := byID[item.ID]
		if !ok {
			return nil, fmt.Errorf("HTTP obligation set mismatch: no decision for %q", item.ID)
		}
		delete(byID, item.ID)
		entries = append(entries, reviewedEntry{
			ID:             item.ID,
			Authority:      item.Authority,
			Locator:        item.Locator,
			SourceURLs:     append([]string(nil), inventory.RequiredURLs...),
			Domain:         item.Domain,
			Phase:          item.Phase,
			Classification: decision.Classification,
			Action:         decision.Action,
			Owner:          decisions.ReviewOwner,
			Reason:         decision.Reason,
			SourceKey:      item.SourceKey,
			TargetKeys:     append([]string(nil), item.TargetKeys...),
			Golden:         append([]goldenCase(nil), decision.Golden...),
		})
	}
	if len(byID) != 0 {
		return nil, fmt.Errorf("HTTP obligation set mismatch: %d extra decisions", len(byID))
	}
	sum := sha256.Sum256(inventoryData)
	audit := migrationAudit{
		SchemaVersion:   1,
		Status:          "http-reviewed-db-rpc-pending",
		CanonicalTarget: canonicalTarget,
		Sections: auditSections{
			HTTPURLNetwork: httpReview{
				Status:                "reviewed",
				SourceInventorySHA256: hex.EncodeToString(sum[:]),
				Owner:                 decisions.ReviewOwner,
				Entries:               entries,
			},
		},
		Signoffs: []signoff{},
	}
	data, err := canonicalJSON(audit)
	if err != nil {
		return nil, err
	}
	if err := ValidateHTTPAudit(inventoryData, data); err != nil {
		return nil, err
	}
	return data, nil
}

func deriveHTTPWeaverDecision(
	item obligation,
	selection weaverDecisionSelection,
) (httpDecision, error) {
	classification, reason, golden, err := deriveWeaverDecision(
		item.ID, item.Kind, item.SourceKey, item.TargetKeys, item.Summary, selection.Action,
	)
	if err != nil {
		return httpDecision{}, fmt.Errorf("HTTP %w", err)
	}
	return httpDecision{
		ObligationID: item.ID, Classification: classification,
		Action: selection.Action, Reason: reason, Golden: golden,
	}, nil
}

func deriveWeaverDecision(
	id, kind, source string,
	targets []string,
	summary, action string,
) (string, string, []goldenCase, error) {
	if action != "translated" && action != "pass_through" && action != "unsupported" {
		return "", "", nil, fmt.Errorf("Weaver diff decision %q has invalid action %q", id, action)
	}
	renamed := kind == "registry_renamed"
	if action == "translated" && (!renamed || source == "" || len(targets) != 1) {
		return "", "", nil, fmt.Errorf("Weaver diff decision %q translates a non-rename", id)
	}
	if action == "translated" {
		target := targets[0]
		return "pure_rename",
			"Reviewed Weaver rename candidate and the v1.41 target model prove a deterministic one-to-one key rename: " + summary,
			[]goldenCase{
				{
					Name: "source-only-string", InputKey: source, InputType: "string",
					InputValue: "legacy-value", ExpectedAction: "translated",
					ExpectedKey: target, ExpectedType: "string", ExpectedValue: "legacy-value",
				},
				{
					Name: "target-conflict", InputKey: source, InputType: "string",
					InputValue: "legacy-value", TargetPresent: true, TargetValue: "canonical-value",
					ExpectedAction: "suppress", ExpectedKey: target,
					ExpectedType: "string", ExpectedValue: "canonical-value",
				},
				{
					Name: "wrong-type", InputKey: source, InputType: "int", InputValue: "7",
					ExpectedAction: "pass_through", ExpectedKey: source,
					ExpectedType: "int", ExpectedValue: "7",
				},
			}, nil
	}
	classification := "split_merge"
	if renamed {
		classification = "context_recompute"
	} else if kind == "registry_obsoleted" {
		classification = "removed_requirement_stability"
	}
	return classification,
		"Conservative review preserves the source because the Weaver candidate requires removal, split/merge, value conversion, runtime context, or a Migration Guide override: " + summary,
		[]goldenCase{{
			Name: "preserve-source", InputKey: source, InputType: "string",
			InputValue: "legacy-value", ExpectedAction: action, ExpectedKey: source,
			ExpectedType: "string", ExpectedValue: "legacy-value",
		}}, nil
}

func ValidateHTTPAudit(inventoryData, auditData []byte) error {
	var inventory httpInventory
	if err := strictDecode(inventoryData, &inventory); err != nil {
		return fmt.Errorf("decode HTTP inventory: %w", err)
	}
	if err := validateInventory(inventory); err != nil {
		return err
	}
	var audit migrationAudit
	if err := strictDecode(auditData, &audit); err != nil {
		return fmt.Errorf("decode migration audit: %w", err)
	}
	if audit.SchemaVersion != 1 || audit.CanonicalTarget != canonicalTarget ||
		(audit.Status != "http-reviewed-db-rpc-pending" && audit.Status != "release-reviewed") {
		return errors.New("HTTP migration audit identity/status is invalid")
	}
	if err := validateHTTPReviewOnly(inventoryData, audit.Sections.HTTPURLNetwork); err != nil {
		return err
	}
	if audit.Status == "http-reviewed-db-rpc-pending" {
		if audit.Sections.DBRPC != nil {
			return errors.New("DB/RPC audit must remain absent in Story 7.5")
		}
		if len(audit.Signoffs) != 0 {
			return errors.New("cross-domain signoffs are deferred to Story 7.6")
		}
	}
	return nil
}

func validateHTTPReviewOnly(inventoryData []byte, review httpReview) error {
	var inventory httpInventory
	if err := strictDecode(inventoryData, &inventory); err != nil {
		return fmt.Errorf("decode HTTP inventory: %w", err)
	}
	if err := validateInventory(inventory); err != nil {
		return err
	}
	sum := sha256.Sum256(inventoryData)
	if review.SourceInventorySHA256 != hex.EncodeToString(sum[:]) {
		return errors.New("HTTP source inventory digest mismatch")
	}
	if review.Status != "reviewed" || review.Owner != reviewOwner {
		return errors.New("HTTP review status/owner is invalid")
	}
	byID := make(map[string]obligation, len(inventory.Obligations))
	for _, item := range inventory.Obligations {
		byID[item.ID] = item
	}
	seen := make(map[string]struct{}, len(review.Entries))
	for _, entry := range review.Entries {
		item, ok := byID[entry.ID]
		if !ok {
			return fmt.Errorf("HTTP obligation set contains extra review %q", entry.ID)
		}
		if _, ok := seen[entry.ID]; ok {
			return fmt.Errorf("HTTP obligation set contains duplicate review %q", entry.ID)
		}
		seen[entry.ID] = struct{}{}
		if entry.Authority != item.Authority || entry.Locator != item.Locator ||
			entry.Domain != item.Domain || entry.Phase != item.Phase ||
			entry.SourceKey != item.SourceKey || !equalStrings(entry.TargetKeys, item.TargetKeys) ||
			!equalStrings(entry.SourceURLs, inventory.RequiredURLs) {
			return fmt.Errorf("HTTP review %q does not match source obligation", entry.ID)
		}
		if entry.Owner != reviewOwner || strings.TrimSpace(entry.Reason) == "" {
			return fmt.Errorf("HTTP review %q lacks owner/reason", entry.ID)
		}
		if err := validateDisposition(item, entry); err != nil {
			return err
		}
	}
	if len(seen) != len(byID) {
		return fmt.Errorf("HTTP obligation set mismatch: reviewed %d of %d", len(seen), len(byID))
	}
	return nil
}

func extractHTTPSchemaObligations(data []byte) ([]obligation, error) {
	var result []obligation
	scanner := bufio.NewScanner(bytes.NewReader(data))
	version, domain := "", ""
	operationIndex, mappingIndex := -1, 0
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if match := versionLine.FindStringSubmatch(line); match != nil {
			version, domain = match[1], ""
			operationIndex = -1
			continue
		}
		if match := domainLine.FindStringSubmatch(line); match != nil {
			domain = match[1]
			operationIndex = -1
			continue
		}
		if domain == "spans" && strings.HasPrefix(line, "        - ") {
			operationIndex++
			mappingIndex = 0
			continue
		}
		if domain != "spans" {
			continue
		}
		match := mappingLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		source, target := match[1], match[2]
		currentMapping := mappingIndex
		mappingIndex++
		if !httpRelated(source) && !httpRelated(target) {
			continue
		}
		id := fmt.Sprintf("schema:%s:spans:%02d:%02d:%s", version, operationIndex, currentMapping, source)
		result = append(result, obligation{
			ID:                  id,
			Authority:           "telemetry_schema",
			Locator:             fmt.Sprintf("versions.%s.spans.changes[%d].rename_attributes[%d]@line:%d", version, operationIndex, currentMapping, lineNumber),
			Domain:              "spans",
			Phase:               "schema",
			Kind:                "rename_attribute",
			SourceKey:           source,
			TargetKeys:          []string{target},
			Summary:             source + " -> " + target,
			TranslationEligible: safeSchemaRename(source, target),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(result) < 10 {
		return nil, fmt.Errorf("HTTP Schema extraction found only %d obligations", len(result))
	}
	return result, nil
}

func extractHTTPGuideObligations(data []byte) ([]obligation, error) {
	text := string(data)
	start := strings.Index(text, "### Common attributes across HTTP client and server spans")
	end := strings.Index(text, "### HTTP client duration metric")
	if start < 0 || end <= start {
		return nil, errors.New("HTTP guide trace section boundaries not found")
	}
	var result []obligation
	section := ""
	row := 0
	scanner := bufio.NewScanner(strings.NewReader(text[start:end]))
	lineNumber := strings.Count(text[:start], "\n")
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if strings.HasPrefix(line, "### ") {
			section = guideSlug(strings.TrimPrefix(line, "### "))
			row = 0
			continue
		}
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Change ") ||
			strings.HasPrefix(line, "| ---") {
			continue
		}
		rowText := strings.TrimSpace(line)
		rowText = strings.TrimPrefix(rowText, "|")
		rowText = strings.TrimSuffix(rowText, "|")
		cells := strings.Split(rowText, "|")
		if len(cells) < 2 {
			return nil, fmt.Errorf("malformed HTTP guide table row at line %d", lineNumber)
		}
		row++
		change := strings.TrimSpace(cells[0])
		comments := strings.TrimSpace(strings.Join(cells[1:], "|"))
		source, targets := guideKeys(change)
		result = append(result, obligation{
			ID:                  fmt.Sprintf("guide:%s:%02d", section, row),
			Authority:           "http_migration_guide",
			Locator:             fmt.Sprintf("%s.table[%d]@line:%d", section, row, lineNumber),
			Domain:              "spans",
			Phase:               "post_schema",
			Kind:                guideKind(change, comments),
			SourceKey:           source,
			TargetKeys:          targets,
			Summary:             change + guideCommentSuffix(comments),
			TranslationEligible: safeGuideRename(source, targets, comments),
		})
	}
	extras := []struct {
		id      string
		needle  string
		domain  string
		kind    string
		source  string
		targets []string
		safe    bool
	}{
		{"guide:http-span-names:method-other", "The `{http.method}` portion of span names is replace by `HTTP`", "span_names", "context_recompute", "$span.name", nil, false},
		{"guide:pre-1.19:http-flavor", "`http.flavor` &rarr; `network.protocol.version`", "spans", "rename_with_value_change", "http.flavor", []string{"network.protocol.version"}, false},
		{"guide:pre-1.18:user-agent", "`http.user_agent` &rarr; `user_agent.original`", "spans", "rename_attribute", "http.user_agent", []string{"user_agent.original"}, true},
		{"guide:pre-1.17:server-name-route", "When `http.route` is available:<br> `{http.route}` &rarr;", "span_names", "context_recompute", "$span.name", nil, false},
		{"guide:pre-1.17:server-name-no-route", "When `http.route` is not available:<br> `HTTP {http.method}` &rarr;", "span_names", "context_recompute", "$span.name", nil, false},
		{"guide:pre-1.17:client-name", "- `HTTP {http.method}` &rarr; `{summary}`", "span_names", "context_recompute", "$span.name", nil, false},
	}
	for _, extra := range extras {
		index := strings.Index(text, extra.needle)
		if index < 0 {
			return nil, fmt.Errorf("HTTP guide obligation %q not found", extra.id)
		}
		result = append(result, obligation{
			ID:                  extra.id,
			Authority:           "http_migration_guide",
			Locator:             fmt.Sprintf("%s@line:%d", extra.id, strings.Count(text[:index], "\n")+1),
			Domain:              extra.domain,
			Phase:               "post_schema",
			Kind:                extra.kind,
			SourceKey:           extra.source,
			TargetKeys:          extra.targets,
			Summary:             extra.needle,
			TranslationEligible: extra.safe,
		})
	}
	if len(result) < 30 {
		return nil, fmt.Errorf("HTTP guide extraction found only %d obligations", len(result))
	}
	return result, nil
}

func extractHTTPDiffObligations(data []byte) ([]obligation, error) {
	document, err := registrydiff.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("decode Weaver Registry diff: %w", err)
	}
	var result []obligation
	for _, candidate := range document.RegistryAttributes {
		if !candidate.IsMigrationObligation() || !candidate.HTTPRelated() {
			continue
		}
		var targets []string
		if candidate.Target != "" {
			targets = []string{candidate.Target}
		}
		result = append(result, obligation{
			ID: candidate.StableID(), Authority: "weaver_diff",
			Locator: candidate.Locator(), Domain: "spans", Phase: "post_schema",
			Kind: "registry_" + candidate.Type, SourceKey: candidate.Source,
			TargetKeys: targets, Summary: candidate.Summary(),
			TranslationEligible: candidate.Type == "renamed",
		})
	}
	return result, nil
}

func decodeHTTPInputs(inventoryData, decisionsData []byte) (httpInventory, httpDecisions, error) {
	var inventory httpInventory
	if err := strictDecode(inventoryData, &inventory); err != nil {
		return inventory, httpDecisions{}, fmt.Errorf("decode HTTP inventory: %w", err)
	}
	if err := validateInventory(inventory); err != nil {
		return inventory, httpDecisions{}, err
	}
	var decisions httpDecisions
	if err := strictDecode(decisionsData, &decisions); err != nil {
		return inventory, decisions, fmt.Errorf("decode HTTP decisions: %w", err)
	}
	if decisions.SchemaVersion != 1 || decisions.ReviewOwner != reviewOwner {
		return inventory, decisions, errors.New("HTTP decision identity/owner is invalid")
	}
	return inventory, decisions, nil
}

func validateInventory(inventory httpInventory) error {
	if inventory.SchemaVersion != 1 || inventory.CanonicalTarget != canonicalTarget {
		return errors.New("HTTP inventory identity is invalid")
	}
	expectedURLs := []string{
		"https://opentelemetry.io/schemas/1.7.0",
		"https://opentelemetry.io/schemas/1.4.0",
	}
	if !equalStrings(inventory.RequiredURLs, expectedURLs) {
		return errors.New("HTTP inventory required source URLs are invalid")
	}
	if inventory.Sources.TelemetrySchemaSHA256 == "" || inventory.Sources.HTTPGuideSHA256 == "" ||
		inventory.Sources.WeaverDiffSHA256 == "" || len(inventory.Obligations) < 40 {
		return errors.New("HTTP inventory source identity or obligation count is incomplete")
	}
	previous := ""
	for _, item := range inventory.Obligations {
		if item.ID == "" || item.Authority == "" || item.Locator == "" || item.Domain == "" ||
			item.Phase == "" || item.Kind == "" || item.Summary == "" {
			return fmt.Errorf("HTTP obligation %q lacks metadata", item.ID)
		}
		if previous != "" && item.ID <= previous {
			return errors.New("HTTP inventory obligations are duplicated or unsorted")
		}
		previous = item.ID
	}
	return nil
}

func validateDisposition(item obligation, entry reviewedEntry) error {
	allowedClassification := map[string]struct{}{
		"pure_rename":                   {},
		"split_merge":                   {},
		"context_recompute":             {},
		"removed_requirement_stability": {},
		"dual_emit":                     {},
	}
	if _, ok := allowedClassification[entry.Classification]; !ok {
		return fmt.Errorf("HTTP review %q has invalid classification %q", entry.ID, entry.Classification)
	}
	allowedAction := map[string]struct{}{"translated": {}, "pass_through": {}, "unsupported": {}}
	if _, ok := allowedAction[entry.Action]; !ok {
		return fmt.Errorf("HTTP review %q has invalid action %q", entry.ID, entry.Action)
	}
	if entry.Action == "translated" {
		if entry.Classification != "pure_rename" || !item.TranslationEligible ||
			item.SourceKey == "" || len(item.TargetKeys) != 1 {
			return fmt.Errorf("HTTP review %q translated action is not a proven pure rename", entry.ID)
		}
	} else if entry.Classification == "pure_rename" && item.TranslationEligible {
		// A safe rename may deliberately remain pass-through, but the reason must
		// make that conservative exception explicit.
		if !strings.Contains(strings.ToLower(entry.Reason), "conservative") {
			return fmt.Errorf("HTTP review %q safe rename pass-through lacks conservative reason", entry.ID)
		}
	}
	if len(entry.Golden) == 0 {
		return fmt.Errorf("HTTP review %q has empty golden", entry.ID)
	}
	hasTranslated, hasSuppressed, hasWrongType := false, false, false
	for _, golden := range entry.Golden {
		if golden.Name == "" || golden.InputKey == "" || golden.ExpectedAction == "" ||
			golden.InputType == "" || golden.ExpectedKey == "" || golden.ExpectedType == "" {
			return fmt.Errorf("HTTP review %q has incomplete golden", entry.ID)
		}
		if entry.Action == "translated" {
			switch golden.ExpectedAction {
			case "translated":
				hasTranslated = true
				if golden.ExpectedKey != item.TargetKeys[0] {
					return fmt.Errorf("HTTP review %q golden target disagrees with pure rename", entry.ID)
				}
			case "suppress":
				hasSuppressed = true
				if !golden.TargetPresent || golden.ExpectedKey != item.TargetKeys[0] {
					return fmt.Errorf("HTTP review %q suppress golden lacks canonical target", entry.ID)
				}
			case "pass_through":
				hasWrongType = true
				if golden.InputType == "string" || golden.ExpectedKey != golden.InputKey ||
					golden.ExpectedType != golden.InputType {
					return fmt.Errorf("HTTP review %q wrong-type golden is invalid", entry.ID)
				}
			default:
				return fmt.Errorf("HTTP review %q translated golden has invalid action", entry.ID)
			}
		} else if golden.ExpectedAction != entry.Action || golden.ExpectedKey != golden.InputKey ||
			golden.ExpectedType != golden.InputType {
			return fmt.Errorf("HTTP review %q pass-through golden must preserve source", entry.ID)
		}
	}
	if entry.Action == "translated" && (!hasTranslated || !hasSuppressed || !hasWrongType) {
		return fmt.Errorf("HTTP review %q lacks translated/conflict/wrong-type golden coverage", entry.ID)
	}
	return nil
}

func httpRelated(key string) bool {
	for _, prefix := range []string{"http.", "url.", "net.", "network.", "client.", "server."} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func safeSchemaRename(source, target string) bool {
	if supplementalHTTPRename(source, target) {
		return true
	}
	safe := map[string]string{
		"http.retry_count":      "http.resend_count",
		"http.resend_count":     "http.request.resend_count",
		"http.status_code":      "http.response.status_code",
		"http.url":              "url.full",
		"http.user_agent":       "user_agent.original",
		"net.app.protocol.name": "net.protocol.name",
		"net.protocol.name":     "network.protocol.name",
	}
	return safe[source] == target
}

func safeGuideRename(source string, targets []string, comments string) bool {
	if len(targets) == 1 && supplementalHTTPRename(source, targets[0]) {
		return true
	}
	if len(targets) != 1 {
		return false
	}
	if strings.TrimSpace(comments) != "" {
		return false
	}
	safe := map[string]string{
		"http.status_code":  "http.response.status_code",
		"http.url":          "url.full",
		"http.resend_count": "http.request.resend_count",
	}
	return safe[source] == targets[0]
}

func guideKeys(change string) (string, []string) {
	keys := regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(change, -1)
	if len(keys) == 0 {
		return "", nil
	}
	source := keys[0][1]
	if strings.HasPrefix(strings.ToLower(change), "new:") || !strings.Contains(change, "rarr") {
		return source, nil
	}
	targets := make([]string, 0, len(keys)-1)
	for _, key := range keys[1:] {
		targets = append(targets, key[1])
	}
	return source, targets
}

func guideKind(change, comments string) string {
	lower := strings.ToLower(change + " " + comments)
	switch {
	case strings.HasPrefix(strings.ToLower(change), "new:"):
		return "added"
	case strings.Contains(lower, "removed"):
		return "removed"
	case strings.Contains(change, " and ") && strings.Contains(change, "rarr"):
		return "split"
	case strings.Contains(change, "rarr"):
		return "rename_or_recompute"
	default:
		return "requirement_or_normalization_change"
	}
}

func guideSlug(heading string) string {
	slug := strings.ToLower(heading)
	replacer := strings.NewReplacer(" ", "-", "/", "-", "`", "", ".", "", "(", "", ")", "")
	return replacer.Replace(slug)
}

func guideCommentSuffix(comments string) string {
	if comments == "" {
		return ""
	}
	return " | " + comments
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

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// These comments change recording requirements, not the meaning of an already
// recorded value. Version value changes are handled by the contextual runtime.
func supplementalHTTPRename(source, target string) bool {
	mappings := map[string]string{
		"http.request_content_length":  "http.request.body.size",
		"http.response_content_length": "http.response.body.size",
		"net.protocol.name":            "network.protocol.name",
		"net.protocol.version":         "network.protocol.version",
		"net.app.protocol.name":        "net.protocol.name",
		"net.app.protocol.version":     "net.protocol.version",
		"messaging.protocol":           "net.app.protocol.name",
		"messaging.protocol_version":   "net.app.protocol.version",
		"net.sock.peer.addr":           "network.peer.address",
		"net.sock.peer.port":           "network.peer.port",
		"net.sock.host.addr":           "network.local.address",
		"net.sock.host.port":           "network.local.port",
		"http.client_ip":               "client.address",
		"net.host.connection.type":     "network.connection.type",
		"net.host.connection.subtype":  "network.connection.subtype",
		"net.host.carrier.name":        "network.carrier.name",
		"net.host.carrier.mcc":         "network.carrier.mcc",
		"net.host.carrier.mnc":         "network.carrier.mnc",
		"net.host.carrier.icc":         "network.carrier.icc",
	}
	return mappings[source] == target
}
