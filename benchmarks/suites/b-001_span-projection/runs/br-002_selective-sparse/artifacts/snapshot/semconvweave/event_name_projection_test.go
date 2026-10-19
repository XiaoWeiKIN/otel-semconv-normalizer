// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestEventNameRealGeneratedInputsAreAuditedNoOp(t *testing.T) {
	if len(activeGeneratedData.nameRules) != 0 {
		t.Fatalf("real generated name rule count = %d, want 0", len(activeGeneratedData.nameRules))
	}
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
		for _, spanName := range []string{"", "GET", "checkout"} {
			for _, eventName := range []string{"", "exception", "custom.event"} {
				if got := weave.EventName(spanName, eventName); got != eventName {
					t.Errorf(
						"weave %d EventName(%q, %q) = %q",
						weave, spanName, eventName, got,
					)
				}
			}
		}
	}
}

func TestEventNameAndAttributeUseSameWeave(t *testing.T) {
	weave := Resolve("https://opentelemetry.io/schemas/1.7.0")
	attrs := pcommon.NewMap()
	attrs.PutStr("message.id", "42")
	decision := weave.EventAttribute(
		attrs, "rpc", "message", "message.id", pcommon.NewValueStr("42"),
	)
	if decision != (AttributeDecision{
		Key: "message.id", Action: AttributeKeepValue,
	}) {
		t.Fatalf("EventAttribute() = %+v", decision)
	}
	if got := weave.EventName("rpc", "message"); got != "message" {
		t.Fatalf("EventName() = %q, want source no-op", got)
	}

	unknown := Resolve("https://opentelemetry.io/schemas/1.8.0")
	if got := unknown.EventAttribute(
		attrs, "rpc", "message", "message.id", pcommon.NewValueStr("42"),
	); got != (AttributeDecision{Key: "message.id", Action: AttributeKeepValue}) {
		t.Fatalf("unknown EventAttribute() = %+v", got)
	}
	if got := unknown.EventName("rpc", "message"); got != "message" {
		t.Fatalf("unknown EventName() = %q", got)
	}
}

func TestEventNameNoOpDoesNotAllocate(t *testing.T) {
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
				if got := test.weave.EventName("GET", "exception"); got != "exception" {
					panic(got)
				}
			})
			if allocations != 0 {
				t.Fatalf("EventName allocations = %v, want 0", allocations)
			}
		})
	}
}
