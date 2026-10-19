// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"reflect"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestResourceAttributeSupportedLegacyProjection(t *testing.T) {
	weave := Resolve("https://opentelemetry.io/schemas/1.7.0")
	tests := []struct {
		name      string
		attrs     func() pcommon.Map
		sourceKey string
		source    pcommon.Value
		want      AttributeDecision
	}{
		{
			name: "legacy only",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("db.system", "mysql")
				return attrs
			},
			sourceKey: "db.system",
			source:    pcommon.NewValueStr("mysql"),
			want:      AttributeDecision{Key: "db.system.name", Action: AttributeKeepValue},
		},
		{
			name: "canonical only",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("db.system.name", "mysql")
				return attrs
			},
			sourceKey: "db.system.name",
			source:    pcommon.NewValueStr("mysql"),
			want:      AttributeDecision{Key: "db.system.name", Action: AttributeKeepValue},
		},
		{
			name: "both same",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("db.system", "mysql")
				attrs.PutStr("db.system.name", "mysql")
				return attrs
			},
			sourceKey: "db.system",
			source:    pcommon.NewValueStr("mysql"),
			want:      AttributeDecision{Key: "db.system.name", Action: AttributeSuppress},
		},
		{
			name: "both conflict",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("db.system.name", "postgresql")
				attrs.PutStr("db.system", "mysql")
				return attrs
			},
			sourceKey: "db.system",
			source:    pcommon.NewValueStr("mysql"),
			want:      AttributeDecision{Key: "db.system.name", Action: AttributeSuppress},
		},
		{
			name: "canonical present empty",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("db.system", "mysql")
				attrs.PutStr("db.system.name", "")
				return attrs
			},
			sourceKey: "db.system",
			source:    pcommon.NewValueStr("mysql"),
			want:      AttributeDecision{Key: "db.system.name", Action: AttributeSuppress},
		},
		{
			name: "wrong type preserved under target",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutInt("db.system", 42)
				return attrs
			},
			sourceKey: "db.system",
			source:    pcommon.NewValueInt(42),
			want:      AttributeDecision{Key: "db.system.name", Action: AttributeKeepValue},
		},
		{
			name: "resource applies value normalization for all-domain key rename",
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
			name: "span-only key is pass through",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("http.url", "https://example.test")
				return attrs
			},
			sourceKey: "http.url",
			source:    pcommon.NewValueStr("https://example.test"),
			want:      AttributeDecision{Key: "http.url", Action: AttributeKeepValue},
		},
		{
			name: "unknown key",
			attrs: func() pcommon.Map {
				attrs := pcommon.NewMap()
				attrs.PutStr("vendor.custom", "value")
				return attrs
			},
			sourceKey: "vendor.custom",
			source:    pcommon.NewValueStr("value"),
			want:      AttributeDecision{Key: "vendor.custom", Action: AttributeKeepValue},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attrs := test.attrs()
			before := attrs.AsRaw()
			got := weave.ResourceAttribute(attrs, test.sourceKey, test.source)
			if got != test.want {
				t.Fatalf("ResourceAttribute() = %+v, want %+v", got, test.want)
			}
			if !reflect.DeepEqual(attrs.AsRaw(), before) {
				t.Fatal("ResourceAttribute mutated attrs")
			}
			if test.source.Type() == pcommon.ValueTypeInt && test.source.Int() != 42 {
				t.Fatal("ResourceAttribute mutated source value")
			}
		})
	}
}

func TestResourceAttributeLegacyWinnerIgnoresMapInsertionOrder(t *testing.T) {
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
		connect := weave.ResourceAttribute(
			attrs, "rpc.connect_rpc.request.metadata", pcommon.NewValueStr("connect"),
		)
		grpc := weave.ResourceAttribute(
			attrs, "rpc.grpc.request.metadata", pcommon.NewValueStr("grpc"),
		)
		if connect != (AttributeDecision{Key: "rpc.request.metadata", Action: AttributeKeepValue}) {
			t.Errorf("reverse=%t connect decision = %+v", reverse, connect)
		}
		if grpc != (AttributeDecision{Key: "rpc.request.metadata", Action: AttributeSuppress}) {
			t.Errorf("reverse=%t grpc decision = %+v", reverse, grpc)
		}
	}
}

func TestResourceAttributeUnversionedWeavesUseFallback(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("db.system", "mysql")
	source := pcommon.NewValueStr("mysql")
	want := AttributeDecision{Key: "db.system.name", Action: AttributeKeepValue}
	for _, weave := range []WeaveID{
		WeaveUnknown,
		WeaveMissing,
		WeaveCanonical,
		Resolve("https://opentelemetry.io/schemas/1.8.0"),
		5,
		65535,
	} {
		if got := weave.ResourceAttribute(attrs, "db.system", source); got != want {
			t.Errorf("weave %d decision = %+v, want %+v", weave, got, want)
		}
	}
}
