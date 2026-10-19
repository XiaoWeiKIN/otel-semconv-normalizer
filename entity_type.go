// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv1_41 "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// EntityType is the coarse role used for grouping and technology icons. It is
// the "which role" axis, orthogonal to GetTechnology's "which system" axis.
//
// The value is derived directly from semantic attributes rather than from
// GetTechnology, so the two outputs remain independent:
//
//	db.system=clickhouse ─┬─(role) → EntityType "db"          (icon, GROUP BY)
//	                      └─(raw)  → GetTechnology "clickhouse"  (drill-down)
//
// In DataDog terms EntityType mirrors the catalog entity type (datastore /
// queue / ...), which DD also derives straight from db.system / peer.*, rather
// than the legacy span.type→service-type path.
type EntityType string

const (
	EntityTypeWeb        EntityType = "web"        // inbound HTTP handler
	EntityTypeHTTP       EntityType = "http"       // outbound HTTP call
	EntityTypeDB         EntityType = "db"         // relational / document / wide-column store
	EntityTypeCache      EntityType = "cache"      // redis / memcached / valkey
	EntityTypeRPC        EntityType = "rpc"        // gRPC / dubbo / connect
	EntityTypeQueue      EntityType = "queue"      // kafka / rabbitmq / sqs ...
	EntityTypeServerless EntityType = "serverless" // FaaS / lambda
	EntityTypeCustom     EntityType = "custom"     // unknown
)

// GetEntityType derives the coarse entity type from a span's kind and
// semantic attributes. Decision order mirrors GetTechnology (DB/Cache → Queue →
// RPC → Serverless → HTTP → kind fallback), but it splits Cache out from
// DB and adds a Serverless type. Unlike GetTechnology it always resolves, even for
// spans with no backing system (bare server → web, everything else → custom).
//
// To match DD's catalog exactly (redis as datastore, no separate cache), fold
// EntityTypeCache into EntityTypeDB downstream.
func GetEntityType(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) EntityType {
	if db := GetOTelAttrVal(signalAttrs, keyDBSystemName, keyDBSystem, keyDBType); db != "" {
		if isCacheSystem(db) {
			return EntityTypeCache
		}
		return EntityTypeDB
	}
	if GetOTelAttrVal(signalAttrs, keyMessagingSystem) != "" {
		return EntityTypeQueue
	}
	if GetOTelAttrVal(signalAttrs, keyRPCSystemName, keyRPCSystem) != "" {
		return EntityTypeRPC
	}
	if GetOTelAttrVal(signalAttrs, keyFaaSTrigger) != "" || GetOTelAttrVal(signalAttrs, keyFaaSInvokedName) != "" {
		return EntityTypeServerless
	}
	if hasHTTPAttributes(signalAttrs) {
		if spanKind == ptrace.SpanKindServer || spanKind == ptrace.SpanKindConsumer {
			return EntityTypeWeb
		}
		return EntityTypeHTTP
	}
	if spanKind == ptrace.SpanKindServer {
		return EntityTypeWeb
	}
	return EntityTypeCustom
}

// cacheSystemSet contains db.system(.name) values rendered as a cache rather
// than a database. This is an explicit product choice; fold them into
// EntityTypeDB if datastore semantics are preferred.
var cacheSystemSet = map[string]struct{}{
	semconv1_41.DBSystemNameRedis.Value.AsString():     {},
	semconv1_41.DBSystemNameMemcached.Value.AsString(): {},
	"valkey": {}, // Supported by the project; not yet enumerated by SemConv v1.41.
}

func isCacheSystem(system string) bool {
	// SemConv enum values are canonical lowercase strings. Attribute adapters own
	// validation or cleanup; this hot-path predicate deliberately performs an
	// exact lookup without trimming or case folding.
	_, ok := cacheSystemSet[system]
	return ok
}
