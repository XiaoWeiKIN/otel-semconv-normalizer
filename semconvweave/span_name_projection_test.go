// SPDX-License-Identifier: Apache-2.0

package semconvweave

import "testing"

func TestProjectNameFromRulesIsExactAndDomainScoped(t *testing.T) {
	rules := []generatedNameRule{
		{
			weave: 3, domain: generatedDomainSpan,
			sourceName: "HTTP GET", targetName: "GET",
		},
		{
			weave: 3, domain: generatedDomainSpanEvent,
			sourceName: "HTTP GET", targetName: "event.get",
		},
		{
			weave: 4, domain: generatedDomainSpan,
			sourceName: "RPC call", targetName: "rpc",
		},
	}
	tests := []struct {
		name   string
		weave  WeaveID
		domain generatedDomain
		source string
		want   string
	}{
		{"span exact", 3, generatedDomainSpan, "HTTP GET", "GET"},
		{"event domain", 3, generatedDomainSpanEvent, "HTTP GET", "event.get"},
		{"other weave", 4, generatedDomainSpan, "RPC call", "rpc"},
		{"wrong domain", 3, generatedDomainResource, "HTTP GET", "HTTP GET"},
		{"wrong weave", 4, generatedDomainSpan, "HTTP GET", "HTTP GET"},
		{"wrong source", 3, generatedDomainSpan, "http get", "http get"},
		{"unknown", WeaveUnknown, generatedDomainSpan, "HTTP GET", "HTTP GET"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := translateNameFromRules(
				rules, test.weave, test.domain, test.source,
			); got != test.want {
				t.Fatalf("translateNameFromRules() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSpanNameRealGeneratedInputsAreAuditedNoOp(t *testing.T) {
	if len(activeGeneratedData.nameRules) != 0 {
		t.Fatalf("real generated name rule count = %d, want 0", len(activeGeneratedData.nameRules))
	}
	names := []string{"", "GET", "HTTP GET", "SELECT users", "custom/name"}
	weaves := []WeaveID{
		WeaveUnknown,
		WeaveMissing,
		WeaveCanonical,
		Resolve("https://opentelemetry.io/schemas/1.7.0"),
		Resolve("https://opentelemetry.io/schemas/1.4.0"),
		5,
		65535,
	}
	for _, weave := range weaves {
		for _, name := range names {
			if got := weave.SpanName(name); got != name {
				t.Errorf("weave %d SpanName(%q) = %q", weave, name, got)
			}
		}
	}
}

func TestSpanNameNoOpDoesNotAllocate(t *testing.T) {
	for _, test := range []struct {
		name  string
		weave WeaveID
	}{
		{"canonical", WeaveCanonical},
		{"supported no rule", Resolve("https://opentelemetry.io/schemas/1.7.0")},
		{"missing", WeaveMissing},
		{"unknown", WeaveUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			allocations := testing.AllocsPerRun(1000, func() {
				if got := test.weave.SpanName("GET /users"); got != "GET /users" {
					panic(got)
				}
			})
			if allocations != 0 {
				t.Fatalf("SpanName allocations = %v, want 0", allocations)
			}
		})
	}
}
