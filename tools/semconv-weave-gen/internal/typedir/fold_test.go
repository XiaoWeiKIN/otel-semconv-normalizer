// SPDX-License-Identifier: Apache-2.0

package typedir

import (
	"strings"
	"testing"
)

func TestFoldOperationsMultiHopAndEvidenceOrder(t *testing.T) {
	operations := []operation{
		keyOperation("schema:a", "old.key", "middle.key"),
		keyOperation("schema:b", "middle.key", "canonical.key"),
	}
	keys, _, _, _, err := foldOperations(operations)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Source != "old.key" || keys[0].Target != "canonical.key" {
		t.Fatalf("folded keys = %#v", keys)
	}
	if len(keys[0].Evidence) != 2 ||
		keys[0].Evidence[0].StableID != "schema:a" ||
		keys[0].Evidence[1].StableID != "schema:b" {
		t.Fatalf("evidence chain = %#v", keys[0].Evidence)
	}
}

func TestFoldOperationsSimulatesEventNameBeforeCondition(t *testing.T) {
	name := operation{
		kind: operationName, domain: DomainSpanEvent, action: "translated",
		source: "legacy.event", target: "canonical.event",
		evidence: testEvidence("name:rename"),
	}
	attribute := keyOperation("event:attribute", "legacy.key", "canonical.key")
	attribute.domain = DomainSpanEvent
	attribute.eventNames = []string{"canonical.event"}

	keys, _, names, _, err := foldOperations([]operation{name, attribute})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0].SourceName != "legacy.event" || names[0].TargetName != "canonical.event" {
		t.Fatalf("name rules = %#v", names)
	}
	if len(keys) != 1 || keys[0].Predicate.SourceEventName != "legacy.event" {
		t.Fatalf("event predicate was not rebound to source name: %#v", keys)
	}
}

func TestFoldOperationsFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		operations []operation
		want       string
	}{
		{
			name: "cycle",
			operations: []operation{
				keyOperation("a", "a", "b"),
				keyOperation("b", "b", "a"),
			},
			want: "cycle",
		},
		{
			name: "ambiguity",
			operations: []operation{
				keyOperation("a", "a", "b"),
				keyOperation("b", "a", "c"),
			},
			want: "both",
		},
		{
			name: "unclassified",
			operations: []operation{{
				kind: operationKey, domain: DomainSpan, source: "a", target: "b",
				evidence: testEvidence("missing-action"),
			}},
			want: "unclassified",
		},
		{
			name:       "contextual negative",
			operations: []operation{keyOperation("negative", "http.method", "http.request.method")},
			want:       "contextual negative",
		},
		{
			name: "unknown domain",
			operations: []operation{{
				kind: operationKey, domain: "metric", action: "translated", source: "a", target: "b",
				evidence: testEvidence("metric"),
			}},
			want: "unknown domain",
		},
		{
			name: "unknown phase",
			operations: []operation{{
				kind: operationKey, domain: DomainSpan, action: "translated", source: "a", target: "b",
				evidence: Evidence{
					StableID: "phase", Authority: "test", Locator: "synthetic",
					Phase: "runtime_guess", Operation: 0, Mapping: 0,
				},
			}},
			want: "unknown phase",
		},
		{
			name: "missing event context",
			operations: []operation{{
				kind: operationKey, domain: DomainSpanEvent, action: "translated",
				source: "a", target: "b", eventNames: []string{"renamed.event"},
				evidence: testEvidence("event"),
			}},
			want: "no source-name context",
		},
		{
			name: "value authority disagreement",
			operations: []operation{
				valueOperation("v1", "db.system", "db.system.name", "legacy", "one"),
				valueOperation("v2", "db.system", "db.system.name", "legacy", "two"),
			},
			want: "authority disagreement",
		},
		{
			name: "wildcard mismatch",
			operations: []operation{keyOperation(
				"wildcard", "rpc.request.metadata.<key>", "rpc.request.metadata",
			)},
			want: "wildcard shape",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, _, err := foldOperations(test.operations)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("foldOperations() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func keyOperation(id, source, target string) operation {
	return operation{
		kind: operationKey, domain: DomainSpan, action: "translated",
		source: source, target: target, evidence: testEvidence(id),
	}
}

func valueOperation(id, sourceKey, targetKey, sourceValue, targetValue string) operation {
	return operation{
		kind: operationValue, domain: DomainSpan, action: "translated",
		source: sourceKey, target: targetKey, sourceValue: sourceValue, targetValue: targetValue,
		evidence: testEvidence(id),
	}
}

func testEvidence(id string) Evidence {
	return Evidence{
		StableID: id, Authority: "test", Locator: "synthetic",
		Phase: "schema_signal", Operation: 0, Mapping: 0,
	}
}
