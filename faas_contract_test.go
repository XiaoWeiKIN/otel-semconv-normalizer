// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"reflect"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/attribute"
	semconv1_41 "go.opentelemetry.io/otel/semconv/v1.41.0"
)

func TestFaaSTechnologyDecisionContract(t *testing.T) {
	tests := []struct {
		name           string
		kind           ptrace.SpanKind
		attrs          []attribute.KeyValue
		wantTechnology string
		wantEntityType EntityType
	}{
		{
			name: "outgoing invoked name only identifies serverless",
			kind: ptrace.SpanKindClient,
			attrs: []attribute.KeyValue{
				semconv1_41.FaaSInvokedName("checkout"),
			},
			wantTechnology: TechnologyUnknown,
			wantEntityType: EntityTypeServerless,
		},
		{
			name: "outgoing provider alone is not a FaaS marker",
			kind: ptrace.SpanKindClient,
			attrs: []attribute.KeyValue{
				semconv1_41.FaaSInvokedProviderAWS,
			},
			wantTechnology: TechnologyUnknown,
			wantEntityType: EntityTypeCustom,
		},
		{
			name: "outgoing provider and invoked name",
			kind: ptrace.SpanKindClient,
			attrs: []attribute.KeyValue{
				semconv1_41.FaaSInvokedProviderAWS,
				semconv1_41.FaaSInvokedName("checkout"),
			},
			wantTechnology: TechnologyUnknown,
			wantEntityType: EntityTypeServerless,
		},
		{
			name:           "incoming trigger only",
			kind:           ptrace.SpanKindServer,
			attrs:          []attribute.KeyValue{semconv1_41.FaaSTriggerHTTP},
			wantTechnology: TechnologyUnknown,
			wantEntityType: EntityTypeServerless,
		},
		{
			name: "DB takes both columns before FaaS",
			kind: ptrace.SpanKindServer,
			attrs: []attribute.KeyValue{
				semconv1_41.FaaSTriggerHTTP,
				semconv1_41.DBSystemNameMySQL,
			},
			wantTechnology: semconv1_41.DBSystemNameMySQL.Value.AsString(),
			wantEntityType: EntityTypeDB,
		},
		{
			name: "messaging takes both columns before FaaS",
			kind: ptrace.SpanKindConsumer,
			attrs: []attribute.KeyValue{
				semconv1_41.FaaSTriggerPubSub,
				semconv1_41.MessagingSystemKafka,
			},
			wantTechnology: semconv1_41.MessagingSystemKafka.Value.AsString(),
			wantEntityType: EntityTypeQueue,
		},
		{
			name: "RPC takes both columns before FaaS",
			kind: ptrace.SpanKindServer,
			attrs: []attribute.KeyValue{
				semconv1_41.FaaSTriggerHTTP,
				semconv1_41.RPCSystemNameGRPC,
			},
			wantTechnology: semconv1_41.RPCSystemNameGRPC.Value.AsString(),
			wantEntityType: EntityTypeRPC,
		},
		{
			name: "FaaS role takes precedence over HTTP role",
			kind: ptrace.SpanKindServer,
			attrs: []attribute.KeyValue{
				semconv1_41.FaaSTriggerHTTP,
				semconv1_41.HTTPRequestMethodGet,
			},
			wantTechnology: TechnologyHTTP,
			wantEntityType: EntityTypeServerless,
		},
		{
			name: "empty trigger is not a FaaS marker",
			kind: ptrace.SpanKindClient,
			attrs: []attribute.KeyValue{
				semconv1_41.FaaSTriggerKey.String(""),
			},
			wantTechnology: TechnologyUnknown,
			wantEntityType: EntityTypeCustom,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			attrs.EnsureCapacity(len(test.attrs))
			for _, attr := range test.attrs {
				attrs.PutStr(string(attr.Key), attr.Value.AsString())
			}
			before := attrs.AsRaw()

			if got := GetTechnology(attrs); got != test.wantTechnology {
				t.Errorf("GetTechnology() = %q, want %q under DB > messaging > RPC > HTTP precedence", got, test.wantTechnology)
			}
			if got := GetEntityType(attrs, test.kind); got != test.wantEntityType {
				t.Errorf("GetEntityType() = %q, want %q under DB > messaging > RPC > FaaS > HTTP precedence", got, test.wantEntityType)
			}
			if after := attrs.AsRaw(); !reflect.DeepEqual(after, before) {
				t.Errorf("classification mutated attributes: before=%v, after=%v", before, after)
			}
		})
	}
}
