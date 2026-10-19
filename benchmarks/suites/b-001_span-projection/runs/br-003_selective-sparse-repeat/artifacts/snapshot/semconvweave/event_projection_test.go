// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"reflect"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestEventAttributeRealGeneratedProjectionIsReadOnly(t *testing.T) {
	weave := Resolve("https://opentelemetry.io/schemas/1.7.0")
	tests := []struct {
		name      string
		attrs     func() pcommon.Map
		sourceKey string
		source    pcommon.Value
		want      AttributeDecision
	}{
		{
			name: "target-model-obsoleted attribute stays source",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("message.id", "42")
				return attrs
			},
			sourceKey: "message.id",
			source:    pcommon.NewValueStr("42"),
			want:      AttributeDecision{Key: "message.id", Action: AttributeKeepValue},
		},
		{
			name: "value normalization follows all-domain key rename",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("db.system", "mssql")
				return attrs
			},
			sourceKey: "db.system",
			source:    pcommon.NewValueStr("mssql"),
			want: AttributeDecision{
				Key: "db.system.name", StringValue: "microsoft.sql_server",
				Action: AttributeReplaceString,
			},
		},
		{
			name: "deprecated intermediate target does not suppress source",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("message.id", "42")
				attrs.PutInt("rpc.message.id", 7)
				return attrs
			},
			sourceKey: "message.id",
			source:    pcommon.NewValueStr("42"),
			want:      AttributeDecision{Key: "message.id", Action: AttributeKeepValue},
		},
		{
			name: "unsupported split remains source",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("http.target", "/a?b=c")
				return attrs
			},
			sourceKey: "http.target",
			source:    pcommon.NewValueStr("/a?b=c"),
			want:      AttributeDecision{Key: "http.target", Action: AttributeKeepValue},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attrs := test.attrs()
			before := attrs.AsRaw()
			sourceBefore := test.source.AsRaw()
			got := weave.EventAttribute(
				attrs, "source span", "source.event", test.sourceKey, test.source,
			)
			if got != test.want {
				t.Fatalf("EventAttribute() = %+v, want %+v", got, test.want)
			}
			if !reflect.DeepEqual(attrs.AsRaw(), before) {
				t.Fatal("EventAttribute mutated attrs")
			}
			if !reflect.DeepEqual(test.source.AsRaw(), sourceBefore) {
				t.Fatal("EventAttribute mutated source value")
			}
		})
	}
}

func TestEventAttributeConditionalRuleBindsBothSourceNames(t *testing.T) {
	data := generatedData{
		keyRules: []generatedKeyRule{{
			weave: 3, domain: generatedDomainSpanEvent, match: generatedMatchExact,
			priority: 1, sourceSpanName: "checkout", sourceEventName: "exception",
			source: "legacy.error", target: "error.type",
		}},
		valueRules: []generatedValueRule{{
			weave: 3, domain: generatedDomainSpanEvent,
			sourceSpanName: "checkout", sourceEventName: "exception",
			sourceKey: "legacy.error", targetKey: "error.type",
			sourceValue: "E_TIMEOUT", targetValue: "timeout",
		}},
	}
	attrs := pcommon.NewMap()
	attrs.PutStr("legacy.error", "E_TIMEOUT")
	source := pcommon.NewValueStr("E_TIMEOUT")
	tests := []struct {
		name      string
		weave     WeaveID
		domain    generatedDomain
		spanName  string
		eventName string
		want      AttributeDecision
	}{
		{
			name: "both names match", weave: 3, domain: generatedDomainSpanEvent,
			spanName: "checkout", eventName: "exception",
			want: AttributeDecision{
				Key: "error.type", StringValue: "timeout", Action: AttributeReplaceString,
			},
		},
		{
			name: "span name mismatch", weave: 3, domain: generatedDomainSpanEvent,
			spanName: "payment", eventName: "exception",
			want: AttributeDecision{Key: "legacy.error", Action: AttributeKeepValue},
		},
		{
			name: "event name mismatch", weave: 3, domain: generatedDomainSpanEvent,
			spanName: "checkout", eventName: "log",
			want: AttributeDecision{Key: "legacy.error", Action: AttributeKeepValue},
		},
		{
			name: "domain mismatch", weave: 3, domain: generatedDomainSpan,
			spanName: "checkout", eventName: "exception",
			want: AttributeDecision{Key: "legacy.error", Action: AttributeKeepValue},
		},
		{
			name: "weave mismatch", weave: 4, domain: generatedDomainSpanEvent,
			spanName: "checkout", eventName: "exception",
			want: AttributeDecision{Key: "legacy.error", Action: AttributeKeepValue},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := translateAttributeFromData(
				data, test.weave, test.domain, attrs,
				test.spanName, test.eventName, "legacy.error", source,
			)
			if got != test.want {
				t.Fatalf("translateAttributeFromData() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestEventAttributeWinnerIgnoresMapInsertionOrder(t *testing.T) {
	weave := Resolve("https://opentelemetry.io/schemas/1.4.0")
	for _, reverse := range []bool{false, true} {
		attrs := pcommon.NewMap()
		if reverse {
			attrs.PutStr("rpc.grpc.request.metadata", "grpc")
			attrs.PutStr("rpc.connect_rpc.request.metadata", "connect")
		} else {
			attrs.PutStr("rpc.connect_rpc.request.metadata", "connect")
			attrs.PutStr("rpc.grpc.request.metadata", "grpc")
		}
		connect := weave.EventAttribute(
			attrs, "rpc", "message", "rpc.connect_rpc.request.metadata",
			pcommon.NewValueStr("connect"),
		)
		grpc := weave.EventAttribute(
			attrs, "rpc", "message", "rpc.grpc.request.metadata",
			pcommon.NewValueStr("grpc"),
		)
		if connect != (AttributeDecision{
			Key: "rpc.request.metadata", Action: AttributeKeepValue,
		}) {
			t.Errorf("reverse=%t connect decision = %+v", reverse, connect)
		}
		if grpc != (AttributeDecision{
			Key: "rpc.request.metadata", Action: AttributeSuppress,
		}) {
			t.Errorf("reverse=%t grpc decision = %+v", reverse, grpc)
		}
	}
}

func TestEventAttributeUnversionedWeavesUseFallback(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("message.id", "42")
	source := pcommon.NewValueStr("42")
	want := AttributeDecision{Key: "message.id", Action: AttributeKeepValue}
	for _, weave := range []WeaveID{
		WeaveUnknown,
		WeaveMissing,
		WeaveCanonical,
		Resolve("https://opentelemetry.io/schemas/1.8.0"),
		5,
		65535,
	} {
		if got := weave.EventAttribute(
			attrs, "span", "event", "message.id", source,
		); got != want {
			t.Errorf("weave %d decision = %+v, want %+v", weave, got, want)
		}
	}
}
