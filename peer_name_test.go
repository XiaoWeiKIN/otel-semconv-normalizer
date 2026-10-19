// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"reflect"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestGetPeerNamePrecedence(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]string
		want  string
	}{
		{
			name:  "explicit peer.service wins across all domains",
			attrs: map[string]string{keyPeerService: "checkout", keyDBSystem: "mysql", keyDBNamespace: "orders"},
			want:  "checkout",
		},
		{
			name:  "database: db.name over hostname and system",
			attrs: map[string]string{keyDBSystem: "mysql", keyDBNamespace: "orders", keyServerAddress: "db-1"},
			want:  "orders",
		},
		{
			name:  "database: falls back to hostname then system",
			attrs: map[string]string{keyDBSystem: "mysql", keyServerAddress: "db-1"},
			want:  "db-1",
		},
		{
			name:  "database: system is the last resort",
			attrs: map[string]string{keyDBSystem: "mysql"},
			want:  "mysql",
		},
		{
			name:  "queue: destination over system",
			attrs: map[string]string{keyMessagingSystem: "kafka", keyMessagingDestinationName: "orders.created"},
			want:  "orders.created",
		},
		{
			name:  "queue: kafka bootstrap servers over system when no destination",
			attrs: map[string]string{keyMessagingSystem: "kafka", keyMessagingKafkaBootstrapServers: "b1:9092,b2:9092"},
			want:  "b1:9092,b2:9092",
		},
		{
			name:  "queue: destination still wins over kafka bootstrap servers",
			attrs: map[string]string{keyMessagingSystem: "kafka", keyMessagingDestinationName: "orders", keyMessagingKafkaBootstrapServers: "b1:9092"},
			want:  "orders",
		},
		{
			name:  "queue: rocketmq broker over system when no destination",
			attrs: map[string]string{keyMessagingSystem: "rocketmq", keyMessagingRocketMQBrokerAddress: "broker-a:10911"},
			want:  "broker-a:10911",
		},
		{
			name:  "queue: system is the last resort",
			attrs: map[string]string{keyMessagingSystem: "kafka"},
			want:  "kafka",
		},
		{
			name:  "service: rpc.service over hostname",
			attrs: map[string]string{keyRPCSystem: "grpc", keyRPCService: "orders.v1.OrderService", keyServerAddress: "svc-1"},
			want:  "orders.v1.OrderService",
		},
		{
			name:  "http-only client falls back to peer.hostname",
			attrs: map[string]string{keyServerAddress: "api.example.com"},
			want:  "api.example.com",
		},
		{
			name:  "nothing peer-ish yields empty",
			attrs: map[string]string{keyHTTPRoute: "/orders"},
			want:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := GetPeerName(testAttrs(test.attrs), ptrace.SpanKindClient); got != test.want {
				t.Fatalf("GetPeerName() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGetPeerNameDynamoDBTable(t *testing.T) {
	attrs := testAttrs(map[string]string{keyDBSystem: "dynamodb"})
	attrs.PutEmptySlice(keyAWSDynamoDBTableNames).AppendEmpty().SetStr("orders-table")

	if got := GetPeerName(attrs, ptrace.SpanKindClient); got != "orders-table" {
		t.Fatalf("GetPeerName() = %q, want %q", got, "orders-table")
	}
}

func TestGetPeerNameKindGate(t *testing.T) {
	attrs := testAttrs(map[string]string{keyDBSystem: "mysql", keyDBNamespace: "orders"})
	for _, kind := range []ptrace.SpanKind{
		ptrace.SpanKindServer,
		ptrace.SpanKindInternal,
		ptrace.SpanKindUnspecified,
	} {
		if got := GetPeerName(attrs, kind); got != "" {
			t.Errorf("GetPeerName(%s) = %q, want empty", kind, got)
		}
	}
}

func TestGetPeerNameIsReadOnly(t *testing.T) {
	attrs := testAttrs(map[string]string{keyDBSystem: "mysql", keyDBNamespace: "orders", keyServerAddress: "db-1"})
	before := attrs.AsRaw()

	GetPeerName(attrs, ptrace.SpanKindClient)

	if after := attrs.AsRaw(); !reflect.DeepEqual(after, before) {
		t.Fatalf("GetPeerName mutated attributes: before=%v, after=%v", before, after)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		values []string
		want   string
	}{
		{values: []string{"", "", "third"}, want: "third"},
		{values: []string{"first", "second"}, want: "first"},
		{values: []string{"", ""}, want: ""},
		{values: nil, want: ""},
	}
	for _, test := range tests {
		if got := firstNonEmpty(test.values...); got != test.want {
			t.Errorf("firstNonEmpty(%v) = %q, want %q", test.values, got, test.want)
		}
	}
}
