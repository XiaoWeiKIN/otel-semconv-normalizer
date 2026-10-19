// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// TechnologyHTTP is the value GetTechnology returns for a span that carries an HTTP
// marker but no db / messaging / rpc system. It is the protocol, not a role:
// inbound vs outbound is GetEntityType's job (web vs http).
const TechnologyHTTP = "http"

// TechnologyUnknown is the fallback when no backing technology can be identified.
const TechnologyUnknown = "unknown"

// GetTechnology returns the concrete backing system of a span as a single raw
// value — the "which technology" axis, orthogonal to EntityType's "which
// role" axis. It is lossless: mysql and clickhouse stay distinct, kafka is not
// flattened to a queue, grpc is not flattened to rpc.
//
//	db.system[.name]  → its value, spelling-normalized (mysql, clickhouse, ...)
//	messaging.system  → its value (kafka, rabbitmq, sqs, ...)
//	rpc.system[.name] → its value, spelling-normalized (grpc, dubbo, ...)
//	any HTTP marker   → "http"   (inbound vs outbound is GetEntityType's job)
//	otherwise         → "unknown" (internal / unclassifiable: no technology)
//
// Values are normalized across semconv generations so one technology maps to
// one value: a SQL Server span reads "microsoft.sql_server" whether the emitter
// tagged db.system=mssql (pre-1.30) or db.system.name=microsoft.sql_server. This
// is spelling canonicalization only — distinct technologies are never merged
// (contrast DataDog's span.type, which collapses every SQL engine to "sql").
//
// The coarse EntityType (web/db/cache/queue/...) is derived separately; the two
// are independent columns. GetTechnology deliberately does NOT emit a role value,
// so it never leaks "web"/"custom" into the technology axis.
func GetTechnology(signalAttrs pcommon.Map) string {
	if db := GetOTelAttrVal(signalAttrs, keyDBSystemName, keyDBSystem, keyDBType); db != "" {
		return semconvmeta.CanonicalizeTechnology(db)
	}
	if messagingSystem := GetOTelAttrVal(signalAttrs, keyMessagingSystem); messagingSystem != "" {
		return semconvmeta.CanonicalizeTechnology(messagingSystem)
	}
	if rpcSystem := GetOTelAttrVal(signalAttrs, keyRPCSystemName, keyRPCSystem); rpcSystem != "" {
		return semconvmeta.CanonicalizeTechnology(rpcSystem)
	}
	if hasHTTPAttributes(signalAttrs) {
		return TechnologyHTTP
	}
	return TechnologyUnknown
}

func hasHTTPAttributes(signalAttrs pcommon.Map) bool {
	return getHTTPRequestMethodRaw(signalAttrs) != "" ||
		GetURLFull(signalAttrs) != "" ||
		GetURLScheme(signalAttrs) != ""
}
