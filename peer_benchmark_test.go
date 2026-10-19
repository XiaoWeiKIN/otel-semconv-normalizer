// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var peerProjectionSink peerProjection
var peerTagsFlattenedSink []byte
var peerTagsConceptSink Concept
var peerTagsValueSink string

func consumePeerTag(concept Concept, value string) bool {
	peerTagsConceptSink = concept
	peerTagsValueSink = value
	return true
}

type peerProjection struct {
	service              string
	address              string
	dbName               string
	messagingDestination string
	rpcService           string
	dynamoDBTable        string
	s3Bucket             string
	kinesisStream        string
	dbSystem             string
	messagingSystem      string
	rpcSystem            string
	kafkaBootstrap       string
	rocketMQBroker       string
}

func populatedPeerAttrs() pcommon.Map {
	attrs := testAttrs(map[string]string{
		keyPeerService:                    "checkout-service",
		keyServerAddress:                  "api.internal",
		keyDBNamespace:                    "orders",
		keyMessagingDestinationName:       "orders.created",
		keyRPCService:                     "orders.v1.OrderService",
		keyAWSS3Bucket:                    "invoice-archive",
		keyAWSKinesisStreamName:           "order-events",
		keyDBSystemName:                   "postgresql",
		keyMessagingSystem:                "kafka",
		keyRPCSystemName:                  "grpc",
		keyMessagingKafkaBootstrapServers: "broker-a:9092",
		keyMessagingRocketMQBrokerAddress: "broker-a:10911",
	})
	attrs.PutEmptySlice(keyAWSDynamoDBTableNames).AppendEmpty().SetStr("orders")
	return attrs
}

func legacyFallbackPeerAttrs() pcommon.Map {
	attrs := testAttrs(map[string]string{
		keyPeerService:                    "checkout-service",
		keyServerAddress:                  "",
		keyNetPeerName:                    "legacy.internal",
		keyDBNamespace:                    "",
		keyDBName:                         "legacy-orders",
		keyMessagingDestinationName:       "",
		keyMessagingDestination:           "orders.created",
		keyRPCService:                     "orders.v1.OrderService",
		keyAWSS3Bucket:                    "invoice-archive",
		keyAWSKinesisStreamName:           "order-events",
		keyDBSystemName:                   "",
		keyDBSystem:                       "mssql",
		keyMessagingSystem:                "kafka",
		keyRPCSystemName:                  "",
		keyRPCSystem:                      "grpc",
		keyMessagingKafkaBootstrapServers: "broker-a:9092",
		keyMessagingRocketMQBrokerAddress: "broker-a:10911",
	})
	attrs.PutEmptySlice(keyAWSDynamoDBTableNames).AppendEmpty().SetStr("orders")
	return attrs
}

func sparsePeerAttrs() pcommon.Map {
	return testAttrs(map[string]string{
		"service.name":  "checkout",
		"vendor.region": "ap-northeast-1",
		"vendor.retry":  "false",
	})
}

func httpPeerAttrs() pcommon.Map {
	return testAttrs(map[string]string{
		keyHTTPRequestMethod: "GET",
		keyURLFull:           "https://api.internal/orders/42",
		keyServerAddress:     "api.internal",
		keyHTTPRoute:         "/orders/{id}",
	})
}

func dbPeerAttrs() pcommon.Map {
	return testAttrs(map[string]string{
		keyDBSystemName:    "postgresql",
		keyDBNamespace:     "orders",
		keyDBOperationName: "SELECT",
		keyServerAddress:   "postgres.internal",
	})
}

func getPeerProjection(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) peerProjection {
	return peerProjection{
		service:              GetPeerService(signalAttrs, spanKind),
		address:              GetPeerHostname(signalAttrs, spanKind),
		dbName:               GetPeerDBName(signalAttrs, spanKind),
		messagingDestination: GetPeerMessagingDestination(signalAttrs, spanKind),
		rpcService:           GetPeerRPCService(signalAttrs, spanKind),
		dynamoDBTable:        GetPeerAWSDynamoDBTable(signalAttrs, spanKind),
		s3Bucket:             GetPeerAWSS3Bucket(signalAttrs, spanKind),
		kinesisStream:        GetPeerAWSKinesisStream(signalAttrs, spanKind),
		dbSystem:             GetPeerDBSystem(signalAttrs, spanKind),
		messagingSystem:      GetPeerMessagingSystem(signalAttrs, spanKind),
		rpcSystem:            GetPeerRPCSystem(signalAttrs, spanKind),
		kafkaBootstrap:       GetPeerKafkaBootstrapServers(signalAttrs, spanKind),
		rocketMQBroker:       GetPeerRocketMQBroker(signalAttrs, spanKind),
	}
}

func TestPeerProjectionDoesNotAllocate(t *testing.T) {
	tests := []struct {
		name  string
		attrs pcommon.Map
		kind  ptrace.SpanKind
	}{
		{name: "current", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "legacy fallback", attrs: legacyFallbackPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "kind gate", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindServer},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(1000, func() {
				peerProjectionSink = getPeerProjection(test.attrs, test.kind)
			})
			if allocs != 0 {
				t.Fatalf("individual peer getters allocated %v times, want 0", allocs)
			}
			allocs = testing.AllocsPerRun(1000, func() {
				peerProjectionSink = rangePeerProjection(test.attrs, test.kind)
			})
			if allocs != 0 {
				t.Fatalf("RangePeerAttributes allocated %v times, want 0", allocs)
			}
		})
	}
}

func TestPeerTagsCombinedPathDoesNotAllocate(t *testing.T) {
	tests := []struct {
		name  string
		attrs pcommon.Map
		kind  ptrace.SpanKind
	}{
		{name: "current", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "legacy fallback", attrs: legacyFallbackPeerAttrs(), kind: ptrace.SpanKindProducer},
		{name: "kind gate", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindServer},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dst := make([]byte, 0, 1024)
			allocs := testing.AllocsPerRun(1000, func() {
				tags := GetPeerTags(test.attrs, test.kind)
				tags.Range(consumePeerTag)
				dst = tags.AppendFlattened(dst[:0])
				peerTagsFlattenedSink = dst
			})
			if allocs != 0 {
				t.Fatalf("GetPeerTags + Range + AppendFlattened allocated %v times, want 0", allocs)
			}
		})
	}
}

func BenchmarkPeerProjection(b *testing.B) {
	tests := []struct {
		name  string
		attrs pcommon.Map
		kind  ptrace.SpanKind
	}{
		{name: "current", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "legacy_fallback", attrs: legacyFallbackPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "http", attrs: httpPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "db", attrs: dbPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "sparse", attrs: sparsePeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "kind_gate", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindServer},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			b.Run("individual_getters", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					peerProjectionSink = getPeerProjection(test.attrs, test.kind)
				}
			})
			b.Run("range", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					peerProjectionSink = rangePeerProjection(test.attrs, test.kind)
				}
			})
		})
	}
}
