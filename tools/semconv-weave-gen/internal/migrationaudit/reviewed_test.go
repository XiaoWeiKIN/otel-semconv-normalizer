// SPDX-License-Identifier: Apache-2.0

package migrationaudit

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewedRulesAreDeterministicAndRejectDuplicateIDs(t *testing.T) {
	root := fixtureRoot(t)
	audit := mustRead(t, filepath.Join(root, "audit", "migration-audit-v1.41.0.yaml"))
	rules, err := ReviewedRules(audit)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 280 {
		t.Fatalf("ReviewedRules() count = %d, want 280", len(rules))
	}
	weaverRules := 0
	for index := 1; index < len(rules); index++ {
		if rules[index-1].ID >= rules[index].ID {
			t.Fatalf("rules are not uniquely sorted at %q", rules[index].ID)
		}
	}
	for _, rule := range rules {
		if rule.Authority == "weaver_diff" {
			weaverRules++
		}
	}
	if weaverRules != 81 {
		t.Fatalf("ReviewedRules() Weaver count = %d, want 81", weaverRules)
	}

	var parsed migrationAudit
	if err := strictDecode(audit, &parsed); err != nil {
		t.Fatal(err)
	}
	parsed.Sections.DBRPC.Entries[0].ID = parsed.Sections.HTTPURLNetwork.Entries[0].ID
	duplicate, err := canonicalJSON(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewedRules(duplicate); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("ReviewedRules() error = %v, want duplicate ID", err)
	}
}
