package migrationaudit

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/registrydiff"
)

const (
	classifierSignoffOwner = "otel-semconv-normalizer/semconv-maintainers"
	semanticSignoffOwner   = "otel-semconv-normalizer/semantic-reviewers"
)

type dbRPCInventory struct {
	SchemaVersion   int               `json:"schema_version"`
	CanonicalTarget string            `json:"canonical_target"`
	Sources         dbRPCSources      `json:"sources"`
	RequiredURLs    []string          `json:"required_source_urls"`
	Obligations     []dbRPCObligation `json:"obligations"`
}

type dbRPCSources struct {
	TelemetrySchemaSHA256 string `json:"telemetry_schema_sha256"`
	DBGuideSHA256         string `json:"db_guide_sha256"`
	RPCGuideSHA256        string `json:"rpc_guide_sha256"`
	WeaverDiffSHA256      string `json:"weaver_diff_sha256"`
	AliasCorpusSHA256     string `json:"alias_corpus_sha256"`
	RootSourceSHA256      string `json:"root_source_sha256"`
}

type dbRPCObligation struct {
	ID                  string   `json:"id"`
	Authority           string   `json:"authority"`
	Locator             string   `json:"locator"`
	Domain              string   `json:"domain"`
	Phase               string   `json:"phase"`
	Category            string   `json:"category"`
	Kind                string   `json:"kind"`
	SourceKey           string   `json:"source_key,omitempty"`
	SourceValue         string   `json:"source_value,omitempty"`
	TargetKeys          []string `json:"target_keys,omitempty"`
	TargetValue         string   `json:"target_value,omitempty"`
	Summary             string   `json:"summary"`
	TranslationEligible bool     `json:"translation_eligible"`
}

type aliasCorpus struct {
	SchemaVersion int         `json:"schema_version"`
	Repository    string      `json:"repository"`
	Commit        string      `json:"commit"`
	SourcePath    string      `json:"source_path"`
	SourceSHA256  string      `json:"source_sha256"`
	Owner         string      `json:"owner"`
	Aliases       []aliasPair `json:"aliases"`
}

type aliasPair struct {
	Domain            string `json:"domain"`
	Source            string `json:"source"`
	Target            string `json:"target"`
	ProjectionAllowed bool   `json:"projection_allowed"`
	Owner             string `json:"owner"`
	Rationale         string `json:"rationale"`
}

type dbRPCDecisions struct {
	SchemaVersion int                       `json:"schema_version"`
	ReviewOwner   string                    `json:"review_owner"`
	WeaverDiff    []weaverDecisionSelection `json:"weaver_diff"`
	Decisions     []dbRPCDecision           `json:"decisions"`
}

type dbRPCDecision struct {
	ObligationID   string       `json:"obligation_id"`
	Classification string       `json:"classification"`
	Action         string       `json:"action"`
	Reason         string       `json:"reason"`
	Golden         []goldenCase `json:"golden"`
}

type dbRPCReview struct {
	Status                string               `json:"status"`
	SourceInventorySHA256 string               `json:"source_inventory_sha256"`
	Owner                 string               `json:"owner"`
	Entries               []dbRPCReviewedEntry `json:"entries"`
}

type dbRPCReviewedEntry struct {
	ID             string       `json:"id"`
	Authority      string       `json:"authority"`
	Locator        string       `json:"locator"`
	SourceURLs     []string     `json:"source_urls"`
	Domain         string       `json:"domain"`
	Phase          string       `json:"phase"`
	Category       string       `json:"category"`
	Classification string       `json:"classification"`
	Action         string       `json:"action"`
	Owner          string       `json:"owner"`
	Reason         string       `json:"reason"`
	SourceKey      string       `json:"source_key,omitempty"`
	SourceValue    string       `json:"source_value,omitempty"`
	TargetKeys     []string     `json:"target_keys,omitempty"`
	TargetValue    string       `json:"target_value,omitempty"`
	Golden         []goldenCase `json:"golden"`
}

// GenerateDBRPCInventory extracts DB/RPC obligations and binds the existing
// root alias contract without importing the runtime module.
func GenerateDBRPCInventory(schemaData, dbGuideData, rpcGuideData, diffData, aliasData, rootSource []byte) ([]byte, error) {
	var corpus aliasCorpus
	if err := strictDecode(aliasData, &corpus); err != nil {
		return nil, fmt.Errorf("decode technology alias corpus: %w", err)
	}
	if err := validateAliasCorpus(corpus, rootSource); err != nil {
		return nil, err
	}
	schemaItems, err := extractDBRPCSchemaObligations(schemaData)
	if err != nil {
		return nil, err
	}
	dbItems, err := extractDBGuideObligations(dbGuideData, corpus)
	if err != nil {
		return nil, err
	}
	rpcItems, err := extractRPCGuideObligations(rpcGuideData, corpus)
	if err != nil {
		return nil, err
	}
	if err := validateAliasGuideCoverage(dbItems, rpcItems, corpus); err != nil {
		return nil, err
	}
	diffItems, err := extractDBRPCDiffObligations(diffData)
	if err != nil {
		return nil, err
	}
	aliasItems := extractAliasObligations(corpus)
	items := append(append(append(append(schemaItems, dbItems...), rpcItems...), diffItems...), aliasItems...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	for index := 1; index < len(items); index++ {
		if items[index-1].ID == items[index].ID {
			return nil, fmt.Errorf("duplicate DB/RPC obligation ID %q", items[index].ID)
		}
	}
	inventory := dbRPCInventory{
		SchemaVersion:   1,
		CanonicalTarget: canonicalTarget,
		Sources: dbRPCSources{
			TelemetrySchemaSHA256: digest(schemaData),
			DBGuideSHA256:         digest(dbGuideData),
			RPCGuideSHA256:        digest(rpcGuideData),
			WeaverDiffSHA256:      digest(diffData),
			AliasCorpusSHA256:     digest(aliasData),
			RootSourceSHA256:      digest(rootSource),
		},
		RequiredURLs: historicalRequiredURLs(),
		Obligations:  items,
	}
	return canonicalJSON(inventory)
}

// GenerateCombinedAudit joins both human decision catalogs and emits the
// signed, full-domain audit. It never derives a disposition from upstream text.
func GenerateCombinedAudit(httpInventoryData, httpDecisionsData, dbInventoryData, dbDecisionsData, draftPolicy []byte) ([]byte, error) {
	pendingHTTP, err := GenerateHTTPAudit(httpInventoryData, httpDecisionsData)
	if err != nil {
		return nil, err
	}
	var httpAudit migrationAudit
	if err := strictDecode(pendingHTTP, &httpAudit); err != nil {
		return nil, err
	}
	var inventory dbRPCInventory
	if err := strictDecode(dbInventoryData, &inventory); err != nil {
		return nil, fmt.Errorf("decode DB/RPC inventory: %w", err)
	}
	if err := validateDBRPCInventory(inventory); err != nil {
		return nil, err
	}
	var decisions dbRPCDecisions
	if err := strictDecode(dbDecisionsData, &decisions); err != nil {
		return nil, fmt.Errorf("decode DB/RPC decisions: %w", err)
	}
	if decisions.SchemaVersion != 1 || decisions.ReviewOwner != reviewOwner {
		return nil, errors.New("DB/RPC decision identity/owner is invalid")
	}
	entries, err := joinDBRPCDecisions(inventory, decisions)
	if err != nil {
		return nil, err
	}
	inventorySum := digest(dbInventoryData)
	policySum := digest(draftPolicy)
	audit := migrationAudit{
		SchemaVersion:   1,
		Status:          "release-reviewed",
		CanonicalTarget: canonicalTarget,
		Sections: auditSections{
			HTTPURLNetwork: httpAudit.Sections.HTTPURLNetwork,
			DBRPC: &dbRPCReview{
				Status:                "reviewed",
				SourceInventorySHA256: inventorySum,
				Owner:                 reviewOwner,
				Entries:               entries,
			},
		},
		Signoffs: []signoff{
			newSignoff("classifier_maintainer", classifierSignoffOwner, inventory, inventorySum, policySum),
			newSignoff("semantic_reviewer", semanticSignoffOwner, inventory, inventorySum, policySum),
		},
	}
	data, err := canonicalJSON(audit)
	if err != nil {
		return nil, err
	}
	if err := ValidateCombinedAudit(httpInventoryData, dbInventoryData, draftPolicy, data); err != nil {
		return nil, err
	}
	return data, nil
}

func ValidateCombinedAudit(httpInventoryData, dbInventoryData, draftPolicy, auditData []byte) error {
	var audit migrationAudit
	if err := strictDecode(auditData, &audit); err != nil {
		return fmt.Errorf("decode combined migration audit: %w", err)
	}
	if audit.SchemaVersion != 1 || audit.CanonicalTarget != canonicalTarget || audit.Status != "release-reviewed" {
		return errors.New("combined migration audit identity/status is invalid")
	}
	if err := validateHTTPReviewOnly(httpInventoryData, audit.Sections.HTTPURLNetwork); err != nil {
		return err
	}
	if audit.Sections.DBRPC == nil {
		return errors.New("DB/RPC audit section is missing")
	}
	var inventory dbRPCInventory
	if err := strictDecode(dbInventoryData, &inventory); err != nil {
		return fmt.Errorf("decode DB/RPC inventory: %w", err)
	}
	if err := validateDBRPCInventory(inventory); err != nil {
		return err
	}
	if err := validateDBRPCReview(inventory, dbInventoryData, *audit.Sections.DBRPC); err != nil {
		return err
	}
	if err := validateSignoffs(audit.Signoffs, inventory, dbInventoryData, draftPolicy); err != nil {
		return err
	}
	return nil
}

func extractDBRPCSchemaObligations(data []byte) ([]dbRPCObligation, error) {
	var result []dbRPCObligation
	scanner := bufio.NewScanner(bytes.NewReader(data))
	version, domain := "", ""
	operationIndex, mappingIndex, lineNumber := -1, 0, 0
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
		if domain != "metrics" && strings.HasPrefix(line, "        - ") {
			operationIndex++
			mappingIndex = 0
			continue
		}
		if domain == "" || domain == "metrics" {
			continue
		}
		match := mappingLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		source, target := match[1], match[2]
		currentMapping := mappingIndex
		mappingIndex++
		if !dbRPCRelated(source) && !dbRPCRelated(target) {
			continue
		}
		result = append(result, dbRPCObligation{
			ID:                  fmt.Sprintf("schema:%s:%s:%02d:%02d:%s", version, domain, operationIndex, currentMapping, source),
			Authority:           "telemetry_schema",
			Locator:             fmt.Sprintf("versions.%s.%s.changes[%d].rename_attributes[%d]@line:%d", version, domain, operationIndex, currentMapping, lineNumber),
			Domain:              domain,
			Phase:               "schema",
			Category:            schemaCategory(domain),
			Kind:                "rename_attribute",
			SourceKey:           source,
			TargetKeys:          []string{target},
			Summary:             source + " -> " + target,
			TranslationEligible: safeDBRPCSchemaRename(source, target),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(result) < 25 {
		return nil, fmt.Errorf("DB/RPC Schema extraction found only %d obligations", len(result))
	}
	return result, nil
}

func extractDBGuideObligations(data []byte, corpus aliasCorpus) ([]dbRPCObligation, error) {
	text := string(data)
	var result []dbRPCObligation
	attrs, err := extractGuideTable(text,
		"### Database client span attributes",
		"### Database client span names",
		"db-guide:span-attributes",
		"spans",
		"key",
		func(source string, targets []string, comments string) bool {
			if len(targets) != 1 {
				return false
			}
			switch source {
			case "db.system":
				return targets[0] == "db.system.name"
			case "db.operation":
				return targets[0] == "db.operation.name"
			case "db.mongodb.collection", "db.cassandra.table", "db.cosmosdb.container":
				// The guide requires preserving case, not recomputing the value.
				return targets[0] == "db.collection.name"
			default:
				return false
			}
		},
	)
	if err != nil {
		return nil, err
	}
	result = append(result, attrs...)
	nameNeedle := "The recommended span name has changed."
	nameAt := strings.Index(text, nameNeedle)
	if nameAt < 0 {
		return nil, errors.New("DB span-name obligation not found")
	}
	result = append(result, dbRPCObligation{
		ID:        "db-guide:span-name:01",
		Authority: "db_migration_guide",
		Locator:   fmt.Sprintf("database-client-span-names@line:%d", strings.Count(text[:nameAt], "\n")+1),
		Domain:    "span_names",
		Phase:     "post_schema",
		Category:  "name",
		Kind:      "context_recompute",
		SourceKey: "$span.name",
		Summary:   nameNeedle,
	})
	values, err := extractDBValueTable(text, corpus)
	if err != nil {
		return nil, err
	}
	result = append(result, values...)
	return result, nil
}

func extractRPCGuideObligations(data []byte, corpus aliasCorpus) ([]dbRPCObligation, error) {
	text := string(data)
	spanItems, err := extractGuideTable(text,
		"### RPC span attributes",
		"### `rpc.system.name` values",
		"rpc-guide:span-attributes",
		"spans",
		"key",
		safeRPCGuideKey,
	)
	if err != nil {
		return nil, err
	}
	valueItems, err := extractRPCValueTable(text, corpus)
	if err != nil {
		return nil, err
	}
	result := append(spanItems, valueItems...)
	exceptionNeedle := "`rpc.client.call.exception` and `rpc.server.call.exception`"
	at := strings.Index(text, exceptionNeedle)
	if at < 0 {
		return nil, errors.New("RPC exception event obligation not found")
	}
	result = append(result, dbRPCObligation{
		ID:        "rpc-guide:event-names:exception",
		Authority: "rpc_migration_guide",
		Locator:   fmt.Sprintf("rpc-exception-events@line:%d", strings.Count(text[:at], "\n")+1),
		Domain:    "event_names",
		Phase:     "post_schema",
		Category:  "name",
		Kind:      "added",
		SourceKey: "$event.name",
		Summary:   exceptionNeedle,
	})
	deprecated, err := extractGuideTable(text,
		"### Deprecated events",
		"### Deprecated metrics",
		"rpc-guide:deprecated-events",
		"span_events",
		"event",
		func(string, []string, string) bool { return false },
	)
	if err != nil {
		return nil, err
	}
	for index := range deprecated {
		deprecated[index].Kind = "removed"
		if strings.Contains(deprecated[index].SourceKey, "event") {
			deprecated[index].Domain = "event_names"
			deprecated[index].Category = "name"
			deprecated[index].SourceKey = "$event.name"
		}
	}
	result = append(result, deprecated...)
	return result, nil
}

func extractGuideTable(text, startMarker, endMarker, idPrefix, domain, category string, safe func(string, []string, string) bool) ([]dbRPCObligation, error) {
	start := strings.Index(text, startMarker)
	end := strings.Index(text, endMarker)
	if start < 0 || end <= start {
		return nil, fmt.Errorf("guide table boundaries %q..%q not found", startMarker, endMarker)
	}
	baseLine := strings.Count(text[:start], "\n")
	scanner := bufio.NewScanner(strings.NewReader(text[start:end]))
	row := 0
	var result []dbRPCObligation
	for scanner.Scan() {
		baseLine++
		line := scanner.Text()
		if !strings.HasPrefix(strings.TrimSpace(line), "|") || isMarkdownSeparator(line) {
			continue
		}
		cells := splitMarkdownRow(line)
		if len(cells) < 2 {
			continue
		}
		firstCell := strings.ToLower(strings.TrimSpace(cells[0]))
		if firstCell == "change" || firstCell == "attribute change" ||
			firstCell == "deprecated event/attribute" {
			continue
		}
		row++
		change := strings.TrimSpace(cells[0])
		comments := strings.TrimSpace(cells[len(cells)-1])
		source, targets := guideKeysWithComments(change, comments)
		result = append(result, dbRPCObligation{
			ID:                  fmt.Sprintf("%s:%02d", idPrefix, row),
			Authority:           strings.ReplaceAll(strings.Split(idPrefix, ":")[0], "-", "_") + "_migration_guide",
			Locator:             fmt.Sprintf("%s.table[%d]@line:%d", idPrefix, row, baseLine),
			Domain:              domain,
			Phase:               "post_schema",
			Category:            category,
			Kind:                guideKind(change, comments),
			SourceKey:           normalizeGuideSource(source),
			TargetKeys:          targets,
			Summary:             change + guideCommentSuffix(comments),
			TranslationEligible: safe(source, targets, comments),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("guide table %q is empty", idPrefix)
	}
	return result, nil
}

func extractDBValueTable(text string, corpus aliasCorpus) ([]dbRPCObligation, error) {
	start := strings.Index(text, "### Database system name")
	end := strings.Index(text, "### Database client operation duration metric")
	if start < 0 || end <= start {
		return nil, errors.New("DB value table boundaries not found")
	}
	aliases := aliasLookup(corpus, "db")
	return extractValueRows(text, start, end, "db-guide:system-values", "db", aliases)
}

func extractRPCValueTable(text string, corpus aliasCorpus) ([]dbRPCObligation, error) {
	start := strings.Index(text, "### `rpc.system.name` values")
	end := strings.Index(text, "### RPC server call duration metric")
	if start < 0 || end <= start {
		return nil, errors.New("RPC value table boundaries not found")
	}
	aliases := aliasLookup(corpus, "rpc")
	return extractValueRows(text, start, end, "rpc-guide:system-values", "rpc", aliases)
}

func extractValueRows(text string, start, end int, idPrefix, systemDomain string, aliases map[string]string) ([]dbRPCObligation, error) {
	lineNumber := strings.Count(text[:start], "\n")
	scanner := bufio.NewScanner(strings.NewReader(text[start:end]))
	row := 0
	var result []dbRPCObligation
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if !strings.HasPrefix(strings.TrimSpace(line), "|") || isMarkdownSeparator(line) ||
			strings.Contains(line, "Old `db.system` value") || strings.Contains(line, "| Change") {
			continue
		}
		keys := regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(line, -1)
		if len(keys) == 0 {
			continue
		}
		row++
		source := keys[0][1]
		target := ""
		if len(keys) > 1 {
			target = keys[len(keys)-1][1]
		}
		removed := strings.Contains(line, "Removed")
		eligible := target != "" && !removed && aliases[source] == target
		result = append(result, dbRPCObligation{
			ID:                  fmt.Sprintf("%s:%02d:%s", idPrefix, row, source),
			Authority:           systemDomain + "_migration_guide",
			Locator:             fmt.Sprintf("%s.table[%d]@line:%d", idPrefix, row, lineNumber),
			Domain:              "spans",
			Phase:               "post_schema",
			Category:            "value",
			Kind:                valueKind(removed, target),
			SourceKey:           systemDomain + ".system",
			SourceValue:         source,
			TargetKeys:          []string{systemDomain + ".system.name"},
			TargetValue:         target,
			Summary:             strings.TrimSpace(line),
			TranslationEligible: eligible,
		})
	}
	return result, nil
}

func extractAliasObligations(corpus aliasCorpus) []dbRPCObligation {
	items := make([]dbRPCObligation, 0, len(corpus.Aliases))
	for _, alias := range corpus.Aliases {
		items = append(items, dbRPCObligation{
			ID:                  fmt.Sprintf("root-alias:%s:%s", alias.Domain, alias.Source),
			Authority:           "classifier_alias_contract",
			Locator:             fmt.Sprintf("%s#technologyAliases[%s/%s]", corpus.SourcePath, alias.Domain, alias.Source),
			Domain:              "spans",
			Phase:               "classifier_fallback",
			Category:            "value",
			Kind:                "value_map",
			SourceKey:           alias.Domain + ".system",
			SourceValue:         alias.Source,
			TargetKeys:          []string{alias.Domain + ".system.name"},
			TargetValue:         alias.Target,
			Summary:             alias.Source + " -> " + alias.Target,
			TranslationEligible: alias.ProjectionAllowed,
		})
	}
	return items
}

func extractDBRPCDiffObligations(data []byte) ([]dbRPCObligation, error) {
	document, err := registrydiff.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("decode Weaver Registry diff: %w", err)
	}
	var result []dbRPCObligation
	for _, candidate := range document.RegistryAttributes {
		if !candidate.IsMigrationObligation() || !candidate.DBRPCRelated() {
			continue
		}
		var targets []string
		if candidate.Target != "" {
			targets = []string{candidate.Target}
		}
		result = append(result, dbRPCObligation{
			ID: candidate.StableID(), Authority: "weaver_diff",
			Locator: candidate.Locator(), Domain: "spans", Phase: "post_schema",
			Category: "key", Kind: "registry_" + candidate.Type,
			SourceKey: candidate.Source, TargetKeys: targets,
			Summary:             candidate.Summary(),
			TranslationEligible: candidate.Type == "renamed",
		})
	}
	return result, nil
}

func joinDBRPCDecisions(inventory dbRPCInventory, decisions dbRPCDecisions) ([]dbRPCReviewedEntry, error) {
	byID := make(map[string]dbRPCDecision, len(decisions.Decisions))
	for _, decision := range decisions.Decisions {
		if strings.HasPrefix(decision.ObligationID, "weaver-diff:") {
			return nil, fmt.Errorf(
				"Weaver diff decision %q must use the explicit weaver_diff catalog",
				decision.ObligationID,
			)
		}
		if _, ok := byID[decision.ObligationID]; ok {
			return nil, fmt.Errorf("duplicate DB/RPC decision %q", decision.ObligationID)
		}
		byID[decision.ObligationID] = decision
	}
	obligations := make(map[string]dbRPCObligation, len(inventory.Obligations))
	for _, item := range inventory.Obligations {
		obligations[item.ID] = item
	}
	for _, selection := range decisions.WeaverDiff {
		item, ok := obligations[selection.ObligationID]
		if !ok || item.Authority != "weaver_diff" {
			return nil, fmt.Errorf("unknown DB/RPC Weaver diff decision %q", selection.ObligationID)
		}
		if _, exists := byID[selection.ObligationID]; exists {
			return nil, fmt.Errorf("duplicate DB/RPC decision %q", selection.ObligationID)
		}
		classification, reason, golden, err := deriveWeaverDecision(
			item.ID, item.Kind, item.SourceKey, item.TargetKeys, item.Summary, selection.Action,
		)
		if err != nil {
			return nil, fmt.Errorf("DB/RPC %w", err)
		}
		byID[selection.ObligationID] = dbRPCDecision{
			ObligationID: item.ID, Classification: classification,
			Action: selection.Action, Reason: reason, Golden: golden,
		}
	}
	entries := make([]dbRPCReviewedEntry, 0, len(inventory.Obligations))
	for _, item := range inventory.Obligations {
		decision, ok := byID[item.ID]
		if !ok {
			return nil, fmt.Errorf("DB/RPC obligation set mismatch: no decision for %q", item.ID)
		}
		delete(byID, item.ID)
		entries = append(entries, dbRPCReviewedEntry{
			ID:             item.ID,
			Authority:      item.Authority,
			Locator:        item.Locator,
			SourceURLs:     append([]string(nil), inventory.RequiredURLs...),
			Domain:         item.Domain,
			Phase:          item.Phase,
			Category:       item.Category,
			Classification: decision.Classification,
			Action:         decision.Action,
			Owner:          decisions.ReviewOwner,
			Reason:         decision.Reason,
			SourceKey:      item.SourceKey,
			SourceValue:    item.SourceValue,
			TargetKeys:     append([]string(nil), item.TargetKeys...),
			TargetValue:    item.TargetValue,
			Golden:         append([]goldenCase(nil), decision.Golden...),
		})
	}
	if len(byID) != 0 {
		return nil, fmt.Errorf("DB/RPC obligation set mismatch: %d extra decisions", len(byID))
	}
	return entries, nil
}

func validateDBRPCInventory(inventory dbRPCInventory) error {
	if inventory.SchemaVersion != 1 || inventory.CanonicalTarget != canonicalTarget ||
		!equalStrings(inventory.RequiredURLs, historicalRequiredURLs()) {
		return errors.New("DB/RPC inventory identity is invalid")
	}
	if inventory.Sources.TelemetrySchemaSHA256 == "" || inventory.Sources.DBGuideSHA256 == "" ||
		inventory.Sources.RPCGuideSHA256 == "" || inventory.Sources.WeaverDiffSHA256 == "" ||
		inventory.Sources.AliasCorpusSHA256 == "" || inventory.Sources.RootSourceSHA256 == "" ||
		len(inventory.Obligations) < 80 {
		return errors.New("DB/RPC inventory source identity or obligation count is incomplete")
	}
	previous := ""
	for _, item := range inventory.Obligations {
		if item.ID == "" || item.Authority == "" || item.Locator == "" || item.Domain == "" ||
			item.Phase == "" || item.Category == "" || item.Kind == "" || item.Summary == "" {
			return fmt.Errorf("DB/RPC obligation %q lacks metadata", item.ID)
		}
		if previous != "" && item.ID <= previous {
			return errors.New("DB/RPC inventory obligations are duplicated or unsorted")
		}
		previous = item.ID
	}
	return nil
}

func validateDBRPCReview(inventory dbRPCInventory, inventoryData []byte, review dbRPCReview) error {
	if review.Status != "reviewed" || review.Owner != reviewOwner ||
		review.SourceInventorySHA256 != digest(inventoryData) {
		return errors.New("DB/RPC review identity/digest is invalid")
	}
	byID := make(map[string]dbRPCObligation, len(inventory.Obligations))
	for _, item := range inventory.Obligations {
		byID[item.ID] = item
	}
	seen := make(map[string]struct{}, len(review.Entries))
	for _, entry := range review.Entries {
		item, ok := byID[entry.ID]
		if !ok {
			return fmt.Errorf("DB/RPC obligation set contains extra review %q", entry.ID)
		}
		if _, ok := seen[entry.ID]; ok {
			return fmt.Errorf("DB/RPC obligation set contains duplicate review %q", entry.ID)
		}
		seen[entry.ID] = struct{}{}
		if entry.Authority != item.Authority || entry.Locator != item.Locator ||
			entry.Domain != item.Domain || entry.Phase != item.Phase || entry.Category != item.Category ||
			entry.SourceKey != item.SourceKey || entry.SourceValue != item.SourceValue ||
			!equalStrings(entry.TargetKeys, item.TargetKeys) || entry.TargetValue != item.TargetValue ||
			!equalStrings(entry.SourceURLs, inventory.RequiredURLs) {
			return fmt.Errorf("DB/RPC review %q does not match source obligation", entry.ID)
		}
		if entry.Owner != reviewOwner || strings.TrimSpace(entry.Reason) == "" {
			return fmt.Errorf("DB/RPC review %q lacks owner/reason", entry.ID)
		}
		if err := validateDBRPCDisposition(item, entry); err != nil {
			return err
		}
	}
	if len(seen) != len(byID) {
		return fmt.Errorf("DB/RPC obligation set mismatch: reviewed %d of %d", len(seen), len(byID))
	}
	return nil
}

func validateDBRPCDisposition(item dbRPCObligation, entry dbRPCReviewedEntry) error {
	allowedClassification := map[string]struct{}{
		"pure_rename":                   {},
		"deterministic_value_map":       {},
		"split_merge":                   {},
		"context_recompute":             {},
		"removed_requirement_stability": {},
		"dual_emit":                     {},
	}
	if _, ok := allowedClassification[entry.Classification]; !ok {
		return fmt.Errorf("DB/RPC review %q has invalid classification", entry.ID)
	}
	if entry.Action != "translated" && entry.Action != "pass_through" && entry.Action != "unsupported" {
		return fmt.Errorf("DB/RPC review %q has invalid action", entry.ID)
	}
	if entry.SourceKey == "db.statement" && entry.Action == "translated" {
		return fmt.Errorf("DB/RPC review %q translated db.statement without sanitization proof", entry.ID)
	}
	if entry.Action == "translated" {
		expectedClass := "pure_rename"
		hasTarget := len(item.TargetKeys) == 1
		if item.Category == "value" {
			expectedClass = "deterministic_value_map"
			hasTarget = item.TargetValue != ""
		}
		if !item.TranslationEligible || entry.Classification != expectedClass || !hasTarget {
			return fmt.Errorf("DB/RPC review %q translated action is not proven", entry.ID)
		}
	}
	if len(entry.Golden) == 0 {
		return fmt.Errorf("DB/RPC review %q has empty golden", entry.ID)
	}
	hasTranslated, hasSuppressed, hasWrongType, hasUnknownValue := false, false, false, item.Category != "value"
	for _, golden := range entry.Golden {
		if golden.Name == "" || golden.InputKey == "" || golden.InputType == "" ||
			golden.ExpectedAction == "" || golden.ExpectedKey == "" || golden.ExpectedType == "" {
			return fmt.Errorf("DB/RPC review %q has incomplete golden", entry.ID)
		}
		if entry.Action != "translated" {
			if golden.ExpectedAction != entry.Action || golden.ExpectedKey != golden.InputKey ||
				golden.ExpectedType != golden.InputType {
				return fmt.Errorf("DB/RPC review %q pass-through golden must preserve source", entry.ID)
			}
			continue
		}
		targetKey := item.SourceKey
		if len(item.TargetKeys) == 1 {
			targetKey = item.TargetKeys[0]
		}
		switch golden.ExpectedAction {
		case "translated":
			hasTranslated = true
			if golden.ExpectedKey != targetKey {
				return fmt.Errorf("DB/RPC review %q translated golden target mismatch", entry.ID)
			}
		case "suppress":
			hasSuppressed = true
			if !golden.TargetPresent {
				return fmt.Errorf("DB/RPC review %q suppress golden lacks target", entry.ID)
			}
		case "pass_through":
			if golden.InputType != "string" {
				hasWrongType = true
			}
			if item.Category == "value" && golden.InputType == "string" && golden.InputValue == "unregistered.custom" {
				hasUnknownValue = true
			}
		default:
			return fmt.Errorf("DB/RPC review %q has invalid translated golden action", entry.ID)
		}
	}
	if entry.Action == "translated" && (!hasTranslated || !hasSuppressed || !hasWrongType || !hasUnknownValue) {
		return fmt.Errorf("DB/RPC review %q lacks translated/conflict/wrong-type/unknown-value golden", entry.ID)
	}
	return nil
}

func validateAliasCorpus(corpus aliasCorpus, rootSource []byte) error {
	if corpus.SchemaVersion != 2 || corpus.Repository != "github.com/XiaoWeiKIN/otel-semconv-normalizer" ||
		corpus.Commit == "" || corpus.SourcePath != "internal/semconvmeta/meta.go" ||
		corpus.SourceSHA256 != digest(rootSource) || corpus.Owner != classifierSignoffOwner ||
		len(corpus.Aliases) != 19 {
		return errors.New("technology alias corpus identity is invalid")
	}
	seen := make(map[string]struct{}, len(corpus.Aliases))
	sourceText := string(rootSource)
	for _, alias := range corpus.Aliases {
		key := alias.Domain + "\x00" + alias.Source
		if alias.Source == "" || alias.Target == "" || alias.Owner != classifierSignoffOwner ||
			strings.TrimSpace(alias.Rationale) == "" || (alias.Domain != "db" && alias.Domain != "rpc") {
			return errors.New("technology alias corpus contains invalid pair")
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("technology alias %q is duplicated", alias.Source)
		}
		seen[key] = struct{}{}
		if !strings.Contains(sourceText, `Source: "`+alias.Source+`"`) {
			return fmt.Errorf("technology alias source %q is absent from root contract", alias.Source)
		}
		if alias.Source == "cache" && alias.ProjectionAllowed {
			return errors.New("removed DB cache alias cannot be projection-eligible")
		}
	}
	return nil
}

func validateAliasGuideCoverage(dbItems, rpcItems []dbRPCObligation, corpus aliasCorpus) error {
	guidePairs := make(map[string]string)
	for _, item := range append(append([]dbRPCObligation(nil), dbItems...), rpcItems...) {
		if item.Category == "value" && item.SourceValue != "" {
			domain := strings.TrimSuffix(item.SourceKey, ".system")
			guidePairs[domain+"\x00"+item.SourceValue] = item.TargetValue
		}
	}
	for _, alias := range corpus.Aliases {
		target, ok := guidePairs[alias.Domain+"\x00"+alias.Source]
		if !ok {
			return fmt.Errorf("classifier alias %s/%s is absent from migration Guide value inventory", alias.Domain, alias.Source)
		}
		if alias.Source == "cache" {
			if target != "" {
				return errors.New("removed DB cache alias unexpectedly has a Guide target")
			}
			continue
		}
		if target != alias.Target {
			return fmt.Errorf("classifier alias %s/%s target %q disagrees with Guide %q", alias.Domain, alias.Source, alias.Target, target)
		}
	}
	return nil
}

func newSignoff(role, owner string, inventory dbRPCInventory, inventorySum, policySum string) signoff {
	return signoff{
		Scope:           "db-rpc-and-required-url-domains",
		Role:            role,
		Owner:           owner,
		InventorySHA256: inventorySum,
		DBGuideSHA256:   inventory.Sources.DBGuideSHA256,
		RPCGuideSHA256:  inventory.Sources.RPCGuideSHA256,
		PolicySHA256:    policySum,
		Status:          "approved",
	}
}

func validateSignoffs(signoffs []signoff, inventory dbRPCInventory, inventoryData, draftPolicy []byte) error {
	if len(signoffs) != 2 {
		return errors.New("exactly two DB/RPC signoffs are required")
	}
	expected := []signoff{
		newSignoff("classifier_maintainer", classifierSignoffOwner, inventory, digest(inventoryData), digest(draftPolicy)),
		newSignoff("semantic_reviewer", semanticSignoffOwner, inventory, digest(inventoryData), digest(draftPolicy)),
	}
	for index := range expected {
		if signoffs[index] != expected[index] {
			return fmt.Errorf("DB/RPC signoff %d is invalid", index)
		}
	}
	return nil
}

func historicalRequiredURLs() []string {
	return []string{
		"https://opentelemetry.io/schemas/1.7.0",
		"https://opentelemetry.io/schemas/1.4.0",
	}
}

func dbRPCRelated(key string) bool {
	for _, prefix := range []string{"db.", "rpc.", "cassandra.", "azure.cosmosdb.", "azure.client.", "elasticsearch.", "jsonrpc.", "message."} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func schemaCategory(domain string) string {
	if domain == "span_names" || domain == "event_names" {
		return "name"
	}
	if domain == "span_events" {
		return "event"
	}
	return "key"
}

func safeDBRPCSchemaRename(source, target string) bool {
	if source == "db.statement" || source == "db.name" || source == "db.sql.table" ||
		source == "rpc.grpc.status_code" {
		return false
	}
	if source == "db.system" && target == "db.system.name" {
		return true
	}
	if source == "rpc.system" && target == "rpc.system.name" {
		return true
	}
	if strings.HasPrefix(source, "db.") || strings.HasPrefix(source, "rpc.") ||
		strings.HasPrefix(source, "message.") || strings.HasPrefix(source, "jsonrpc.") {
		return true
	}
	return false
}

func safeRPCGuideKey(source string, targets []string, comments string) bool {
	if len(targets) != 1 {
		return false
	}
	switch source {
	case "rpc.system":
		return targets[0] == "rpc.system.name"
	case "rpc.connect_rpc.error_code":
		return targets[0] == "rpc.response.status_code" && comments == ""
	case "rpc.grpc.request.metadata.<key>",
		"rpc.grpc.response.metadata.<key>",
		"rpc.connect_rpc.request.metadata.<key>",
		"rpc.connect_rpc.response.metadata.<key>":
		return true
	default:
		return false
	}
}

func guideKeysWithComments(change, comments string) (string, []string) {
	changeKeys := regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(change, -1)
	if len(changeKeys) == 0 {
		return "", nil
	}
	source := changeKeys[0][1]
	var targets []string
	if strings.Contains(change, "rarr") {
		for _, key := range changeKeys[1:] {
			targets = append(targets, key[1])
		}
	}
	if len(targets) == 0 && strings.Contains(strings.ToLower(comments), "replaced by") {
		commentKeys := regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(comments, -1)
		for _, key := range commentKeys {
			targets = append(targets, key[1])
		}
	}
	return source, targets
}

func normalizeGuideSource(source string) string {
	return strings.TrimSuffix(source, " event")
}

func splitMarkdownRow(line string) []string {
	row := strings.TrimSpace(line)
	row = strings.TrimPrefix(row, "|")
	row = strings.TrimSuffix(row, "|")
	parts := strings.Split(row, "|")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func isMarkdownSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") {
		return false
	}
	trimmed = strings.NewReplacer("|", "", "-", "", ":", "", " ", "").Replace(trimmed)
	return trimmed == ""
}

func aliasLookup(corpus aliasCorpus, domain string) map[string]string {
	result := make(map[string]string)
	for _, alias := range corpus.Aliases {
		if alias.Domain == domain {
			result[alias.Source] = alias.Target
		}
	}
	return result
}

func valueKind(removed bool, target string) string {
	if removed {
		return "removed"
	}
	if target == "" {
		return "added_or_unchanged"
	}
	return "value_map"
}
