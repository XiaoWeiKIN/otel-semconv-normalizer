// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestGetPeerTagsMatchesIndividualGetters(t *testing.T) {
	tests := []struct {
		name  string
		attrs pcommon.Map
		kind  ptrace.SpanKind
	}{
		{name: "current", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "legacy fallback", attrs: legacyFallbackPeerAttrs(), kind: ptrace.SpanKindProducer},
		{name: "kind gate", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindServer},
		{name: "empty", attrs: pcommon.NewMap(), kind: ptrace.SpanKindConsumer},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got peerProjection
			GetPeerTags(test.attrs, test.kind).Range(func(concept Concept, value string) bool {
				setPeerProjection(&got, concept, value)
				return true
			})
			want := getPeerProjection(test.attrs, test.kind)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("GetPeerTags().Range() = %#v, want %#v", got, want)
			}
		})
	}
}

func TestGetPeerTagsKindGate(t *testing.T) {
	t.Run("consumer projects all concepts", func(t *testing.T) {
		count := 0
		GetPeerTags(populatedPeerAttrs(), ptrace.SpanKindConsumer).Range(
			func(Concept, string) bool {
				count++
				return true
			},
		)
		if count != peerFieldCount {
			t.Fatalf("Consumer concept count = %d, want %d", count, peerFieldCount)
		}
	})

	for _, kind := range []ptrace.SpanKind{
		ptrace.SpanKindInternal,
		ptrace.SpanKindUnspecified,
	} {
		t.Run(kind.String(), func(t *testing.T) {
			called := false
			tags := GetPeerTags(populatedPeerAttrs(), kind)
			tags.Range(func(Concept, string) bool {
				called = true
				return true
			})
			if called {
				t.Fatalf("GetPeerTags invoked Range callback for excluded kind %s", kind)
			}
			if got := tags.AppendFlattened(nil); len(got) != 0 {
				t.Fatalf("GetPeerTags flattened excluded kind %s as %q", kind, got)
			}
		})
	}
}

func TestPeerTagsSnapshotSurvivesSourceMutation(t *testing.T) {
	attrs := testAttrs(map[string]string{
		keyServerAddress: "before.example",
		keyDBSystem:      "mssql",
	})
	tags := GetPeerTags(attrs, ptrace.SpanKindClient)
	want := "peer.db.system:microsoft.sql_server,peer.hostname:before.example"

	assertSnapshot := func(stage string) {
		t.Helper()
		gotValues := map[Concept]string{}
		tags.Range(func(concept Concept, value string) bool {
			gotValues[concept] = value
			return true
		})
		if gotValues[ConceptPeerHostname] != "before.example" ||
			gotValues[ConceptPeerDBSystem] != "microsoft.sql_server" {
			t.Fatalf("%s Range values = %v, want original canonical snapshot", stage, gotValues)
		}
		if got := string(tags.AppendFlattened(nil)); got != want {
			t.Fatalf("%s AppendFlattened() = %q, want %q", stage, got, want)
		}
	}

	attrs.PutStr(keyServerAddress, "after.example")
	attrs.PutStr(keyDBSystem, "redis")
	assertSnapshot("after overwrite")

	attrs.Clear()
	assertSnapshot("after clear")
}

func TestGetPeerTagsDoesNotMutatePData(t *testing.T) {
	traces := ptrace.NewTraces()
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetKind(ptrace.SpanKindClient)
	populatedPeerAttrs().CopyTo(span.Attributes())

	marshaler := &ptrace.ProtoMarshaler{}
	before, err := marshaler.MarshalTraces(traces)
	if err != nil {
		t.Fatalf("marshal input before GetPeerTags: %v", err)
	}

	tags := GetPeerTags(span.Attributes(), span.Kind())
	tags.Range(func(Concept, string) bool { return true })
	tags.AppendFlattened(make([]byte, 0, 512))

	after, err := marshaler.MarshalTraces(traces)
	if err != nil {
		t.Fatalf("marshal input after GetPeerTags: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("GetPeerTags Range/Append mutated pdata: before=%x after=%x", before, after)
	}
}

func TestPeerTagsRangeStopsWhenYieldReturnsFalse(t *testing.T) {
	count := 0
	var gotConcept Concept
	GetPeerTags(populatedPeerAttrs(), ptrace.SpanKindClient).Range(
		func(concept Concept, _ string) bool {
			count++
			gotConcept = concept
			return false
		},
	)
	if count != 1 {
		t.Fatalf("yield count = %d, want 1", count)
	}
	if gotConcept != ConceptPeerService {
		t.Fatalf("first concept = %q, want %q", gotConcept, ConceptPeerService)
	}
}

func TestPeerSourceLookupMatchesFieldRegistry(t *testing.T) {
	sourceCount := 0
	for fieldID := semconvmeta.CoreFieldCount; fieldID < semconvmeta.FieldCount; fieldID++ {
		field := semconvmeta.Field(fieldID)
		for priority, count := uint8(0), field.SourceKeyCount(); priority < count; priority++ {
			sourceCount++
			key := field.SourceKey(priority)
			gotFieldID, gotPriority, ok := peerFieldForSourceKey(key)
			if !ok {
				t.Errorf("source %q for field %d is missing", key, fieldID)
				continue
			}
			if gotFieldID != fieldID || gotPriority != priority {
				t.Errorf(
					"source %q = field %d priority %d, want field %d priority %d",
					key,
					gotFieldID,
					gotPriority,
					fieldID,
					priority,
				)
			}
		}
	}
	if sourceCount != 19 {
		t.Fatalf("peer source count = %d, want 19", sourceCount)
	}
}

func TestRangePeerAttributesMatchesIndividualGetters(t *testing.T) {
	tests := []struct {
		name  string
		attrs pcommon.Map
		kind  ptrace.SpanKind
	}{
		{name: "current", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "legacy fallback", attrs: legacyFallbackPeerAttrs(), kind: ptrace.SpanKindProducer},
		{name: "kind gate", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindServer},
		{name: "no peer sources", attrs: testAttrs(map[string]string{
			"service.name":  "checkout",
			"vendor.region": "ap-northeast-1",
		}), kind: ptrace.SpanKindClient},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := test.attrs.AsRaw()
			got := rangePeerProjection(test.attrs, test.kind)
			want := getPeerProjection(test.attrs, test.kind)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("RangePeerAttributes() = %#v, want %#v", got, want)
			}
			if after := test.attrs.AsRaw(); !reflect.DeepEqual(after, before) {
				t.Fatalf("RangePeerAttributes mutated attributes: before=%v, after=%v", before, after)
			}
		})
	}
}

func TestRangePeerAttributesDoesNotDependOnMapOrder(t *testing.T) {
	tests := []struct {
		name string
		put  func(pcommon.Map)
	}{
		{
			name: "current then legacy",
			put: func(attrs pcommon.Map) {
				attrs.PutStr(keyServerAddress, "current.internal")
				attrs.PutStr(keyNetPeerName, "legacy.internal")
				attrs.PutStr(keyDBSystemName, "postgresql")
				attrs.PutStr(keyDBSystem, "mssql")
			},
		},
		{
			name: "legacy then current",
			put: func(attrs pcommon.Map) {
				attrs.PutStr(keyDBSystem, "mssql")
				attrs.PutStr(keyDBSystemName, "postgresql")
				attrs.PutStr(keyNetPeerName, "legacy.internal")
				attrs.PutStr(keyServerAddress, "current.internal")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			test.put(attrs)
			got := rangePeerProjection(attrs, ptrace.SpanKindClient)
			if got.address != "current.internal" {
				t.Errorf("peer hostname = %q, want current.internal", got.address)
			}
			if got.dbSystem != "postgresql" {
				t.Errorf("peer DB system = %q, want postgresql", got.dbSystem)
			}
		})
	}
}

func TestRangePeerAttributesStopsWhenYieldReturnsFalse(t *testing.T) {
	count := 0
	var gotConcept Concept
	RangePeerAttributes(
		populatedPeerAttrs(),
		ptrace.SpanKindClient,
		func(concept Concept, _ string) bool {
			count++
			gotConcept = concept
			return false
		},
	)
	if count != 1 {
		t.Fatalf("yield count = %d, want 1", count)
	}
	if gotConcept != ConceptPeerService {
		t.Fatalf("first concept = %q, want %q", gotConcept, ConceptPeerService)
	}
}

func rangePeerProjection(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) peerProjection {
	var result peerProjection
	RangePeerAttributes(signalAttrs, spanKind, func(concept Concept, value string) bool {
		setPeerProjection(&result, concept, value)
		return true
	})
	return result
}

func setPeerProjection(result *peerProjection, concept Concept, value string) {
	switch concept {
	case ConceptPeerService:
		result.service = value
	case ConceptPeerHostname:
		result.address = value
	case ConceptPeerDBName:
		result.dbName = value
	case ConceptPeerMessagingDestination:
		result.messagingDestination = value
	case ConceptPeerRPCService:
		result.rpcService = value
	case ConceptPeerAWSDynamoDBTable:
		result.dynamoDBTable = value
	case ConceptPeerAWSS3Bucket:
		result.s3Bucket = value
	case ConceptPeerAWSKinesisStream:
		result.kinesisStream = value
	case ConceptPeerDBSystem:
		result.dbSystem = value
	case ConceptPeerMessagingSystem:
		result.messagingSystem = value
	case ConceptPeerRPCSystem:
		result.rpcSystem = value
	case ConceptPeerKafkaBootstrapSrvs:
		result.kafkaBootstrap = value
	case ConceptPeerRocketMQBroker:
		result.rocketMQBroker = value
	}
}
