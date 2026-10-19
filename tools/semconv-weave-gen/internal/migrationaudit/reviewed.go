// SPDX-License-Identifier: Apache-2.0

package migrationaudit

import (
	"errors"
	"fmt"
	"sort"
)

// ReviewedRule is the normalized, read-only handoff from the signed migration
// audit to later build stages. Callers must validate the audit and its bound
// inventories before consuming these records.
type ReviewedRule struct {
	ID             string
	Authority      string
	Locator        string
	SourceURLs     []string
	Domain         string
	Phase          string
	Category       string
	Classification string
	Action         string
	Reason         string
	SourceKey      string
	SourceValue    string
	TargetKeys     []string
	TargetValue    string
}

// ReviewedRules decodes the signed audit into a deterministic typed view. It
// deliberately does not expose the JSON AST or let downstream stages infer a
// missing decision.
func ReviewedRules(auditData []byte) ([]ReviewedRule, error) {
	var audit migrationAudit
	if err := strictDecode(auditData, &audit); err != nil {
		return nil, fmt.Errorf("decode combined migration audit: %w", err)
	}
	if audit.SchemaVersion != 1 || audit.Status != "release-reviewed" ||
		audit.CanonicalTarget != canonicalTarget || audit.Sections.DBRPC == nil {
		return nil, errors.New("combined migration audit identity/status is invalid")
	}

	result := make([]ReviewedRule, 0, len(audit.Sections.HTTPURLNetwork.Entries)+len(audit.Sections.DBRPC.Entries))
	for _, entry := range audit.Sections.HTTPURLNetwork.Entries {
		result = append(result, ReviewedRule{
			ID:             entry.ID,
			Authority:      entry.Authority,
			Locator:        entry.Locator,
			SourceURLs:     append([]string(nil), entry.SourceURLs...),
			Domain:         entry.Domain,
			Phase:          entry.Phase,
			Category:       "key",
			Classification: entry.Classification,
			Action:         entry.Action,
			Reason:         entry.Reason,
			SourceKey:      entry.SourceKey,
			TargetKeys:     append([]string(nil), entry.TargetKeys...),
		})
	}
	for _, entry := range audit.Sections.DBRPC.Entries {
		result = append(result, ReviewedRule{
			ID:             entry.ID,
			Authority:      entry.Authority,
			Locator:        entry.Locator,
			SourceURLs:     append([]string(nil), entry.SourceURLs...),
			Domain:         entry.Domain,
			Phase:          entry.Phase,
			Category:       entry.Category,
			Classification: entry.Classification,
			Action:         entry.Action,
			Reason:         entry.Reason,
			SourceKey:      entry.SourceKey,
			SourceValue:    entry.SourceValue,
			TargetKeys:     append([]string(nil), entry.TargetKeys...),
			TargetValue:    entry.TargetValue,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	for index, item := range result {
		if item.ID == "" || item.Authority == "" || item.Action == "" {
			return nil, fmt.Errorf("reviewed rule %d is incomplete", index)
		}
		if index > 0 && result[index-1].ID == item.ID {
			return nil, fmt.Errorf("duplicate reviewed stable ID %q", item.ID)
		}
	}
	return result, nil
}
