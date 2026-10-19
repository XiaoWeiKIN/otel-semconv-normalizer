// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"math/bits"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const peerFieldCount = int(semconvmeta.FieldCount - semconvmeta.CoreFieldCount)

type peerAttributeValues struct {
	values     [peerFieldCount]string
	priorities [peerFieldCount]uint8
	present    uint32
}

// PeerTags is an immutable value snapshot of the non-empty peer concepts
// derived from one span. Its fields are deliberately private: callers obtain a
// snapshot with GetPeerTags and consume it through Range or AppendFlattened.
// The snapshot retains selected string values, not the source pcommon.Map.
type PeerTags struct {
	attributes peerAttributeValues
}

// GetPeerTags scans signalAttrs once and returns a read-only snapshot of every
// non-empty peer concept derived for spanKind. It does not mutate signalAttrs
// and does not allocate. Only client, producer, and consumer spans yield data.
func GetPeerTags(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) PeerTags {
	if signalAttrs.Len() == 0 || !isPeerSpanKind(spanKind) {
		return PeerTags{}
	}
	return getPeerTags(signalAttrs)
}

func getPeerTags(signalAttrs pcommon.Map) PeerTags {
	var tags PeerTags
	signalAttrs.Range(func(key string, value pcommon.Value) bool {
		fieldID, priority, ok := peerFieldForSourceKey(key)
		if !ok {
			return true
		}
		tags.attributes.set(fieldID, priority, peerSourceValue(fieldID, value))
		return true
	})
	tags.attributes.canonicalizeTechnologies()
	return tags
}

// Range calls yield for every concept in the snapshot's stable semconvmeta
// field order. Returning false stops iteration. Range does not allocate.
func (tags PeerTags) Range(yield func(Concept, string) bool) {
	// Clear the lowest set bit on each iteration, preserving field order while
	// skipping absent concepts.
	for present := tags.attributes.present; present != 0; present &= present - 1 {
		offset := bits.TrailingZeros32(present)
		fieldID := semconvmeta.CoreFieldCount + semconvmeta.FieldID(offset)
		field := semconvmeta.Field(fieldID)
		value := tags.attributes.values[offset]
		if !yield(Concept(field.Concept()), value) {
			return
		}
	}
}

// RangePeerAttributes calls yield for every non-empty peer concept derived
// from signalAttrs. It scans signalAttrs once, does not mutate it, and emits
// concepts in the stable semconvmeta field order. Returning false stops
// iteration.
func RangePeerAttributes(
	signalAttrs pcommon.Map,
	spanKind ptrace.SpanKind,
	yield func(Concept, string) bool,
) {
	if signalAttrs.Len() == 0 || !isPeerSpanKind(spanKind) {
		return
	}
	getPeerTags(signalAttrs).Range(yield)
}

func (values *peerAttributeValues) set(
	fieldID semconvmeta.FieldID,
	priority uint8,
	value string,
) {
	if value == "" {
		return
	}
	offset := int(fieldID - semconvmeta.CoreFieldCount)
	bit := uint32(1) << offset
	storedPriority := priority + 1
	if values.present&bit != 0 && values.priorities[offset] <= storedPriority {
		return
	}
	values.values[offset] = value
	values.priorities[offset] = storedPriority
	values.present |= bit
}

func (values *peerAttributeValues) canonicalizeTechnologies() {
	for present := values.present; present != 0; present &= present - 1 {
		offset := bits.TrailingZeros32(present)
		fieldID := semconvmeta.CoreFieldCount + semconvmeta.FieldID(offset)
		if semconvmeta.Field(fieldID).ValuePolicy() == semconvmeta.ValueTechnology {
			values.values[offset] = semconvmeta.CanonicalizeTechnology(values.values[offset])
		}
	}
}

func peerSourceValue(fieldID semconvmeta.FieldID, value pcommon.Value) string {
	switch semconvmeta.Field(fieldID).ValueType() {
	case semconvmeta.ValueTypeString:
		return value.AsString()
	case semconvmeta.ValueTypeStringSlice:
		if value.Type() != pcommon.ValueTypeSlice {
			return ""
		}
		values := value.Slice()
		for index := 0; index < values.Len(); index++ {
			element := values.At(index)
			if element.Type() == pcommon.ValueTypeStr && element.Str() != "" {
				return element.Str()
			}
		}
	}
	return ""
}

func peerFieldForSourceKey(key string) (semconvmeta.FieldID, uint8, bool) {
	switch key {
	case keyPeerService:
		return semconvmeta.FieldPeerService, 0, true
	case keyServerAddress:
		return semconvmeta.FieldPeerHostname, 0, true
	case keyNetPeerName:
		return semconvmeta.FieldPeerHostname, 1, true
	case keyDBNamespace:
		return semconvmeta.FieldPeerDBName, 0, true
	case keyDBName:
		return semconvmeta.FieldPeerDBName, 1, true
	case keyMessagingDestinationName:
		return semconvmeta.FieldPeerMessagingDestination, 0, true
	case keyMessagingDestination:
		return semconvmeta.FieldPeerMessagingDestination, 1, true
	case keyRPCService:
		return semconvmeta.FieldPeerRPCService, 0, true
	case keyAWSDynamoDBTableNames:
		return semconvmeta.FieldPeerAWSDynamoDBTable, 0, true
	case keyAWSS3Bucket:
		return semconvmeta.FieldPeerAWSS3Bucket, 0, true
	case keyAWSKinesisStreamName:
		return semconvmeta.FieldPeerAWSKinesisStream, 0, true
	case keyDBSystemName:
		return semconvmeta.FieldPeerDBSystem, 0, true
	case keyDBSystem:
		return semconvmeta.FieldPeerDBSystem, 1, true
	case keyDBType:
		return semconvmeta.FieldPeerDBSystem, 2, true
	case keyMessagingSystem:
		return semconvmeta.FieldPeerMessagingSystem, 0, true
	case keyRPCSystemName:
		return semconvmeta.FieldPeerRPCSystem, 0, true
	case keyRPCSystem:
		return semconvmeta.FieldPeerRPCSystem, 1, true
	case keyMessagingKafkaBootstrapServers:
		return semconvmeta.FieldPeerKafkaBootstrapServers, 0, true
	case keyMessagingRocketMQBrokerAddress:
		return semconvmeta.FieldPeerRocketMQBroker, 0, true
	default:
		return 0, 0, false
	}
}
