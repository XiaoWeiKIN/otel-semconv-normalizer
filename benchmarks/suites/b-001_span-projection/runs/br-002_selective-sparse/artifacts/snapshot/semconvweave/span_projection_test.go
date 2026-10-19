// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"reflect"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestSpanAttributeExactValueAndSafetyProjection(t *testing.T) {
	weave := Resolve("https://opentelemetry.io/schemas/1.7.0")
	tests := []struct {
		name      string
		attrs     func() pcommon.Map
		spanName  string
		sourceKey string
		source    pcommon.Value
		want      AttributeDecision
	}{
		{
			name: "approved string value rewrite",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("db.system", "mssql")
				return attrs
			},
			spanName:  "SELECT users",
			sourceKey: "db.system",
			source:    pcommon.NewValueStr("mssql"),
			want: AttributeDecision{
				Key: "db.system.name", StringValue: "microsoft.sql_server",
				Action: AttributeReplaceString,
			},
		},
		{
			name: "unknown string value preserves value under target",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("db.system", "future-db")
				return attrs
			},
			spanName:  "SELECT",
			sourceKey: "db.system",
			source:    pcommon.NewValueStr("future-db"),
			want:      AttributeDecision{Key: "db.system.name", Action: AttributeKeepValue},
		},
		{
			name: "wrong type preserves type and value under target",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutInt("db.system", 42)
				return attrs
			},
			spanName:  "SELECT",
			sourceKey: "db.system",
			source:    pcommon.NewValueInt(42),
			want:      AttributeDecision{Key: "db.system.name", Action: AttributeKeepValue},
		},
		{
			name: "canonical present empty suppresses legacy",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("db.system", "mssql")
				attrs.PutStr("db.system.name", "")
				return attrs
			},
			spanName:  "SELECT",
			sourceKey: "db.system",
			source:    pcommon.NewValueStr("mssql"),
			want:      AttributeDecision{Key: "db.system.name", Action: AttributeSuppress},
		},
		{
			name: "span exact rule",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("http.url", "https://example.test/a")
				return attrs
			},
			spanName:  "GET",
			sourceKey: "http.url",
			source:    pcommon.NewValueStr("https://example.test/a"),
			want:      AttributeDecision{Key: "url.full", Action: AttributeKeepValue},
		},
		{
			name: "unsupported split remains source",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("http.target", "/a?secret=b")
				return attrs
			},
			spanName:  "GET",
			sourceKey: "http.target",
			source:    pcommon.NewValueStr("/a?secret=b"),
			want:      AttributeDecision{Key: "http.target", Action: AttributeKeepValue},
		},
		{
			name: "sanitization dependent db statement remains source",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("db.statement", "select 1")
				return attrs
			},
			spanName:  "SELECT",
			sourceKey: "db.statement",
			source:    pcommon.NewValueStr("select 1"),
			want:      AttributeDecision{Key: "db.statement", Action: AttributeKeepValue},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attrs := test.attrs()
			before := attrs.AsRaw()
			sourceBefore := test.source.AsRaw()
			got := weave.SpanAttribute(attrs, test.spanName, test.sourceKey, test.source)
			if got != test.want {
				t.Fatalf("SpanAttribute() = %+v, want %+v", got, test.want)
			}
			if !reflect.DeepEqual(attrs.AsRaw(), before) {
				t.Fatal("SpanAttribute mutated attrs")
			}
			if !reflect.DeepEqual(test.source.AsRaw(), sourceBefore) {
				t.Fatal("SpanAttribute mutated source value")
			}
		})
	}
}

func TestSpanAttributePrefixProjectionAndWinner(t *testing.T) {
	weave := Resolve("https://opentelemetry.io/schemas/1.4.0")
	for _, reverse := range []bool{false, true} {
		attrs := pcommon.NewMap()
		if reverse {
			attrs.PutStr("rpc.grpc.request.metadata.authorization", "grpc")
			attrs.PutStr("rpc.connect_rpc.request.metadata.authorization", "connect")
		} else {
			attrs.PutStr("rpc.connect_rpc.request.metadata.authorization", "connect")
			attrs.PutStr("rpc.grpc.request.metadata.authorization", "grpc")
		}
		before := attrs.AsRaw()
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
			t.Errorf("reverse=%t connect decision = %+v", reverse, connect)
		}
		if grpc != (AttributeDecision{
			Key: "rpc.request.metadata.authorization", Action: AttributeSuppress,
		}) {
			t.Errorf("reverse=%t grpc decision = %+v", reverse, grpc)
		}
		if !reflect.DeepEqual(attrs.AsRaw(), before) {
			t.Fatalf("reverse=%t SpanAttribute mutated attrs", reverse)
		}
	}
}

func TestSpanAttributeNewerLegacyWinsRegardlessOfMapOrder(t *testing.T) {
	weave := Resolve("https://opentelemetry.io/schemas/1.7.0")
	for _, reverse := range []bool{false, true} {
		attrs := pcommon.NewMap()
		if reverse {
			attrs.PutInt("http.retry_count", 1)
			attrs.PutInt("http.resend_count", 2)
		} else {
			attrs.PutInt("http.resend_count", 2)
			attrs.PutInt("http.retry_count", 1)
		}
		resend := weave.SpanAttribute(
			attrs, "GET", "http.resend_count", pcommon.NewValueInt(2),
		)
		retry := weave.SpanAttribute(
			attrs, "GET", "http.retry_count", pcommon.NewValueInt(1),
		)
		if resend != (AttributeDecision{
			Key: "http.request.resend_count", Action: AttributeKeepValue,
		}) {
			t.Errorf("reverse=%t resend decision = %+v", reverse, resend)
		}
		if retry != (AttributeDecision{
			Key: "http.request.resend_count", Action: AttributeSuppress,
		}) {
			t.Errorf("reverse=%t retry decision = %+v", reverse, retry)
		}
	}
}

func TestSpanAttributeSourceNameAndWeaveFallbackAreTotal(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("db.system", "mssql")
	source := pcommon.NewValueStr("mssql")
	for _, spanName := range []string{"", "GET", "custom/name"} {
		got := Resolve("https://opentelemetry.io/schemas/1.7.0").SpanAttribute(
			attrs, spanName, "db.system", source,
		)
		if got.Action != AttributeReplaceString || got.Key != "db.system.name" {
			t.Errorf("span name %q decision = %+v", spanName, got)
		}
	}

	want := AttributeDecision{
		Key: "db.system.name", StringValue: "microsoft.sql_server",
		Action: AttributeReplaceString,
	}
	for _, weave := range []WeaveID{
		WeaveUnknown,
		WeaveMissing,
		WeaveCanonical,
		Resolve("https://opentelemetry.io/schemas/1.8.0"),
		5,
		65535,
	} {
		if got := weave.SpanAttribute(attrs, "SELECT", "db.system", source); got != want {
			t.Errorf("weave %d decision = %+v, want %+v", weave, got, want)
		}
	}
}
