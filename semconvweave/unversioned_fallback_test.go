// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"reflect"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func unversionedTestWeaves() []WeaveID {
	return []WeaveID{
		WeaveUnknown,
		WeaveMissing,
		WeaveCanonical,
		Resolve("https://opentelemetry.io/schemas/1.8.0"),
		5,
		65535,
	}
}

func TestUnversionedFallbackProjectionMatrix(t *testing.T) {
	for _, weave := range unversionedTestWeaves() {
		t.Run(weave.State().String(), func(t *testing.T) {
			attrs := pcommon.NewMap()
			attrs.PutStr("db.system", "mssql")
			attrs.PutStr("message.id", "42")
			attrs.PutInt("db.cassandra.page_size", 17)
			attrs.PutStr("vendor.custom", "value")
			before := attrs.AsRaw()

			if got := weave.ResourceAttribute(
				attrs, "db.system", pcommon.NewValueStr("mssql"),
			); got != (AttributeDecision{
				Key: "db.system.name", StringValue: "microsoft.sql_server",
				Action: AttributeReplaceString,
			}) {
				t.Errorf("ResourceAttribute(%d) = %+v", weave, got)
			}
			if got := weave.SpanAttribute(
				attrs, "SELECT", "db.system", pcommon.NewValueStr("mssql"),
			); got != (AttributeDecision{
				Key: "db.system.name", StringValue: "microsoft.sql_server",
				Action: AttributeReplaceString,
			}) {
				t.Errorf("SpanAttribute value(%d) = %+v", weave, got)
			}
			if got := weave.EventAttribute(
				attrs, "rpc", "message", "message.id", pcommon.NewValueStr("42"),
			); got != (AttributeDecision{
				Key: "message.id", Action: AttributeKeepValue,
			}) {
				t.Errorf("EventAttribute(%d) = %+v", weave, got)
			}
			if got := weave.SpanAttribute(
				attrs, "SELECT", "db.cassandra.page_size", pcommon.NewValueInt(17),
			); got != (AttributeDecision{
				Key: "cassandra.page.size", Action: AttributeKeepValue,
			}) {
				t.Errorf("SpanAttribute wrong type(%d) = %+v", weave, got)
			}
			if got := weave.SpanAttribute(
				attrs, "custom", "vendor.custom", pcommon.NewValueStr("value"),
			); got != (AttributeDecision{
				Key: "vendor.custom", Action: AttributeKeepValue,
			}) {
				t.Errorf("SpanAttribute unknown(%d) = %+v", weave, got)
			}
			if !reflect.DeepEqual(before, attrs.AsRaw()) {
				t.Errorf("fallback projection %d mutated attrs", weave)
			}
		})
	}
}

func TestUnversionedFallbackWinnerIsPresenceBasedAndOrderIndependent(t *testing.T) {
	for _, weave := range unversionedTestWeaves() {
		for _, targetWrongType := range []bool{false, true} {
			for _, reverse := range []bool{false, true} {
				attrs := pcommon.NewMap()
				if reverse {
					attrs.PutStr("db.system", "mssql")
					if targetWrongType {
						attrs.PutInt("db.system.name", 7)
					} else {
						attrs.PutStr("db.system.name", "")
					}
				} else {
					if targetWrongType {
						attrs.PutInt("db.system.name", 7)
					} else {
						attrs.PutStr("db.system.name", "")
					}
					attrs.PutStr("db.system", "mssql")
				}
				if got := weave.SpanAttribute(
					attrs, "SELECT", "db.system", pcommon.NewValueStr("mssql"),
				); got != (AttributeDecision{
					Key: "db.system.name", Action: AttributeSuppress,
				}) {
					t.Errorf(
						"weave=%d wrong-type=%t reverse=%t canonical winner = %+v",
						weave, targetWrongType, reverse, got,
					)
				}
			}
		}
	}
}

func TestUnversionedFallbackLegacyPriorityAndPrefix(t *testing.T) {
	for _, weave := range unversionedTestWeaves() {
		for _, reverse := range []bool{false, true} {
			attrs := pcommon.NewMap()
			if reverse {
				attrs.PutStr("rpc.connect_rpc.request.metadata.authorization", "connect")
				attrs.PutStr("rpc.grpc.request.metadata.authorization", "grpc")
			} else {
				attrs.PutStr("rpc.grpc.request.metadata.authorization", "grpc")
				attrs.PutStr("rpc.connect_rpc.request.metadata.authorization", "connect")
			}

			connect := weave.SpanAttribute(
				attrs, "rpc", "rpc.connect_rpc.request.metadata.authorization",
				pcommon.NewValueStr("connect"),
			)
			grpc := weave.SpanAttribute(
				attrs, "rpc", "rpc.grpc.request.metadata.authorization",
				pcommon.NewValueStr("grpc"),
			)
			if connect != (AttributeDecision{
				Key: "rpc.request.metadata.authorization", Action: AttributeKeepValue,
			}) {
				t.Errorf("weave=%d reverse=%t connect prefix = %+v", weave, reverse, connect)
			}
			if grpc != (AttributeDecision{
				Key: "rpc.request.metadata.authorization", Action: AttributeSuppress,
			}) {
				t.Errorf("weave=%d reverse=%t grpc prefix = %+v", weave, reverse, grpc)
			}
		}
	}
}

func TestGeneratedRuleSelectionKeepsExactAndFallbackSeparate(t *testing.T) {
	data := generatedData{
		sources: []generatedSource{{url: "legacy", id: 3}},
		keyRules: []generatedKeyRule{{
			weave: 3, domain: generatedDomainSpan, match: generatedMatchExact,
			source: "legacy", target: "exact",
		}},
		fallbackKeyRules: []generatedKeyRule{{
			weave: WeaveUnknown, domain: generatedDomainSpan, match: generatedMatchExact,
			source: "legacy", target: "fallback",
		}},
		fallbackNameRules: []generatedNameRule{{
			weave: WeaveUnknown, domain: generatedDomainSpan,
			sourceName: "old", targetName: "new",
		}},
	}
	exact := generatedRules(data, 3)
	if exact.weave != 3 || exact.keyRules[0].target != "exact" {
		t.Fatalf("exact rule set = %+v", exact)
	}
	for _, weave := range []WeaveID{WeaveUnknown, WeaveMissing, WeaveCanonical, 65535} {
		fallback := generatedRules(data, weave)
		if fallback.weave != WeaveUnknown || fallback.keyRules[0].target != "fallback" {
			t.Fatalf("fallback rule set for %d = %+v", weave, fallback)
		}
		if got := translateNameFromRules(
			fallback.nameRules, fallback.weave, generatedDomainSpan, "old",
		); got != "new" {
			t.Fatalf("fallback SpanName(%d) = %q", weave, got)
		}
	}
}
