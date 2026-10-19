// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var peerFlattenedStringSink string

func messagingPeerAttrs() pcommon.Map {
	return testAttrs(map[string]string{
		keyMessagingSystem:                "kafka",
		keyMessagingDestinationName:       "orders.created",
		keyMessagingKafkaBootstrapServers: "broker-a:9092",
	})
}

func rpcPeerAttrs() pcommon.Map {
	return testAttrs(map[string]string{
		keyRPCSystemName: "grpc",
		keyRPCService:    "orders.v1.OrderService",
		keyServerAddress: "orders.internal",
	})
}

func BenchmarkPeerTags(b *testing.B) {
	tests := []struct {
		name  string
		attrs pcommon.Map
		kind  ptrace.SpanKind
	}{
		{name: "current", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "legacy_fallback", attrs: legacyFallbackPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "http", attrs: httpPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "db", attrs: dbPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "messaging", attrs: messagingPeerAttrs(), kind: ptrace.SpanKindProducer},
		{name: "rpc", attrs: rpcPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "sparse", attrs: sparsePeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "kind_gate", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindServer},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			// This is the public, source-compatible consumer shape after upgrading
			// the module. It is a conservative baseline, not a copy of v0.4's
			// internal implementation.
			b.Run("compatibility_dual_call", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					RangePeerAttributes(test.attrs, test.kind, consumePeerTag)
					peerFlattenedStringSink = GetFlattenedPeerTags(test.attrs, test.kind)
				}
			})
			b.Run("snapshot", func(b *testing.B) {
				dst := make([]byte, 0, 1024)
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					tags := GetPeerTags(test.attrs, test.kind)
					tags.Range(consumePeerTag)
					dst = tags.AppendFlattened(dst[:0])
					peerTagsFlattenedSink = dst
				}
			})
		})
	}
}

func BenchmarkSetPeerTags(b *testing.B) {
	attrs := peerSourceAttrs()
	SetPeerTags(attrs, ptrace.SpanKindClient)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		SetPeerTags(attrs, ptrace.SpanKindClient)
	}
}
