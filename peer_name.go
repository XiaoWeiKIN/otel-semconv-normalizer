// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// GetPeerName selects a single downstream display name for an outbound span from
// its peer.* concepts, following DataDog's "Naming Precedence by Entity Type".
// It is the authoritative definition of that precedence: DataDog resolves it in
// the backend at query time, and this project recommends the same (a ClickHouse
// view over the peer.* columns). GetPeerName exists so that view — and any
// caller that materializes the name at write time — can mirror one contract-
// tested definition instead of re-deriving the order by hand.
//
// It is READ-ONLY, per-span, and deliberately NOT wired into SetPeerTags: the
// atomic peer.* columns and the flattened set are the stored facts, while the
// chosen name is a presentation projection over them (the same relationship as
// GetResourceName to raw attributes). Only client/producer/consumer spans yield
// a name; others return "".
//
// Precedence: an explicit peer.service wins across all domains. Otherwise the
// domain is decided by which peer system is present:
//   - database (peer.db.system): db.name > aws.dynamodb.table > address > db.system
//   - queue (peer.messaging.system): messaging.destination > kafka.bootstrap.servers > rocketmq.broker > aws.kinesis.stream > messaging.system
//   - service/rpc/http (neither): rpc.service > aws.s3.bucket > address
func GetPeerName(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	if !isPeerSpanKind(spanKind) {
		return ""
	}
	if service := GetPeerService(signalAttrs, spanKind); service != "" {
		return service
	}
	if dbSystem := GetPeerDBSystem(signalAttrs, spanKind); dbSystem != "" {
		return firstNonEmpty(
			GetPeerDBName(signalAttrs, spanKind),
			GetPeerAWSDynamoDBTable(signalAttrs, spanKind),
			GetPeerHostname(signalAttrs, spanKind),
			dbSystem,
		)
	}
	if messagingSystem := GetPeerMessagingSystem(signalAttrs, spanKind); messagingSystem != "" {
		return firstNonEmpty(
			GetPeerMessagingDestination(signalAttrs, spanKind),
			GetPeerKafkaBootstrapServers(signalAttrs, spanKind),
			GetPeerRocketMQBroker(signalAttrs, spanKind),
			GetPeerAWSKinesisStream(signalAttrs, spanKind),
			messagingSystem,
		)
	}
	return firstNonEmpty(
		GetPeerRPCService(signalAttrs, spanKind),
		GetPeerAWSS3Bucket(signalAttrs, spanKind),
		GetPeerHostname(signalAttrs, spanKind),
	)
}

// firstNonEmpty returns the first non-empty argument, or "" if all are empty.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
