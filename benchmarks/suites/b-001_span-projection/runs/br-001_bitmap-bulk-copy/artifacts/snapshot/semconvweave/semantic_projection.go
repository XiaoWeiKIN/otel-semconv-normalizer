// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"slices"
	"strconv"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// SpanContext supplies information required by the OTel v1.41.0 migration
// guides that is absent from the single-attribute ABI.
type SpanContext struct {
	Name string
	Kind ptrace.SpanKind
	// KnownHTTPMethods replaces the default, case-sensitive OTel method set.
	// Nil selects the defaults; a non-nil empty slice marks every legacy method
	// as unknown. The caller owns the slice and must not mutate it during use.
	// Already-current http.request.method values retain the producer's policy.
	KnownHTTPMethods []string
}

// SpanAttributeWithContext returns up to two output decisions for one input.
// Iterate both slots and skip AttributeSuppress. Each KeepValue refers to the
// supplied sourceValue. Like SpanAttribute, this never mutates attrs or values.
// Pass the complete, unchanged source map for deterministic conflict handling.
// Do not gate this API with MayTranslateSpanAttribute: that filter describes
// only the older single-output API. Missing context preserves ambiguous input.
func (w WeaveID) SpanAttributeWithContext(
	attrs pcommon.Map, context SpanContext, sourceKey string, sourceValue pcommon.Value,
) [2]AttributeDecision {
	if sourceKey == "http.method" && sourceValue.Type() == pcommon.ValueTypeStr {
		if _, exists := attrs.Get("http.request.method"); exists {
			return [2]AttributeDecision{}
		}
		method := sourceValue.Str()
		known := isDefaultHTTPMethod(method)
		if context.KnownHTTPMethods != nil {
			known = slices.Contains(context.KnownHTTPMethods, method)
		}
		if known {
			return [2]AttributeDecision{{Key: "http.request.method", Action: AttributeKeepValue}}
		}
		result := [2]AttributeDecision{{Key: "http.request.method", StringValue: "_OTHER", Action: AttributeReplaceString}}
		if _, exists := attrs.Get("http.request.method_original"); !exists {
			result[1] = AttributeDecision{Key: "http.request.method_original", Action: AttributeKeepValue}
		}
		return result
	}
	if decision, ok := contextualNetworkAttribute(attrs, context.Kind, sourceKey, sourceValue); ok {
		return [2]AttributeDecision{decision}
	}
	return [2]AttributeDecision{w.SpanAttribute(attrs, context.Name, sourceKey, sourceValue)}
}

func isDefaultHTTPMethod(method string) bool {
	switch method {
	case "CONNECT", "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT", "TRACE", "QUERY":
		return true
	default:
		return false
	}
}

// semanticSpanAttribute handles migrations needing the full source map but
// only one output. Unknown HTTP methods stay lossless in ABI v1; callers must
// use SpanAttributeWithContext for the method/original pair.
func semanticSpanAttribute(attrs pcommon.Map, key string, value pcommon.Value) (AttributeDecision, bool) {
	switch key {
	case "http.method":
		if _, exists := attrs.Get("http.request.method"); exists {
			return AttributeDecision{Key: "http.request.method", Action: AttributeSuppress}, true
		}
		if value.Type() == pcommon.ValueTypeStr && isDefaultHTTPMethod(value.Str()) {
			return AttributeDecision{Key: "http.request.method", Action: AttributeKeepValue}, true
		}
		return keepSource(key), true
	case "db.name", "db.cassandra.keyspace", "db.hbase.namespace", "db.redis.database_index", "db.mssql.instance_name":
		return databaseNamespace(attrs, key, value), true
	default:
		return AttributeDecision{}, false
	}
}

func stringAttribute(attrs pcommon.Map, key string) string {
	v, ok := attrs.Get(key)
	if !ok || v.Type() != pcommon.ValueTypeStr {
		return ""
	}
	return v.Str()
}

func databaseNamespace(attrs pcommon.Map, key string, value pcommon.Value) AttributeDecision {
	const target = "db.namespace"
	if _, exists := attrs.Get(target); exists {
		return AttributeDecision{Key: target, Action: AttributeSuppress}
	}
	// The newer Elasticsearch alias is already handled by the generated table.
	if _, exists := attrs.Get("db.elasticsearch.cluster.name"); exists {
		return AttributeDecision{Key: target, Action: AttributeSuppress}
	}
	system := stringAttribute(attrs, "db.system.name")
	if _, exists := attrs.Get("db.system.name"); !exists {
		system = stringAttribute(attrs, "db.system")
	}
	if key == "db.redis.database_index" {
		if system != "redis" || value.Type() != pcommon.ValueTypeInt || value.Int() < 0 {
			return keepSource(key)
		}
		return AttributeDecision{Key: target, StringValue: strconv.FormatInt(value.Int(), 10), Action: AttributeReplaceString}
	}
	if value.Type() != pcommon.ValueTypeStr || value.Str() == "" {
		return keepSource(key)
	}
	if system == "redis" {
		// The Redis namespace is the database index, not a display name.
		return keepSource(key)
	}
	if system == "mssql" || system == "microsoft.sql_server" {
		name := stringAttribute(attrs, "db.name")
		instance := stringAttribute(attrs, "db.mssql.instance_name")
		if key == "db.mssql.instance_name" {
			if name != "" {
				return AttributeDecision{Key: target, Action: AttributeSuppress}
			}
			return AttributeDecision{Key: target, Action: AttributeKeepValue}
		}
		if key == "db.name" && instance != "" {
			return AttributeDecision{Key: target, StringValue: instance + "|" + value.Str(), Action: AttributeReplaceString}
		}
	}
	if key == "db.mssql.instance_name" {
		return keepSource(key)
	}
	if key == "db.cassandra.keyspace" && system != "cassandra" || key == "db.hbase.namespace" && system != "hbase" {
		return keepSource(key)
	}
	if key != "db.name" && stringAttribute(attrs, "db.name") != "" {
		return AttributeDecision{Key: target, Action: AttributeSuppress}
	}
	return AttributeDecision{Key: target, Action: AttributeKeepValue}
}

func contextualNetworkAttribute(attrs pcommon.Map, kind ptrace.SpanKind, key string, value pcommon.Value) (AttributeDecision, bool) {
	if key != "net.peer.name" && key != "net.peer.port" && key != "http.server_name" {
		return AttributeDecision{}, false
	}
	// Peer means server only on supported client operations. In particular,
	// HTTP server peer attributes describe the client, and socket peer addresses
	// can describe a proxy rather than the logical server.
	if kind != ptrace.SpanKindClient || !hasClientProtocol(attrs) {
		if key == "http.server_name" {
			return AttributeDecision{}, false
		}
		return keepSource(key), true
	}
	if key == "http.server_name" {
		// net.peer.name supersedes this older HTTP alias. Let the existing
		// generated rule run when no usable peer name is available.
		peer, exists := attrs.Get("net.peer.name")
		if exists && peer.Type() == pcommon.ValueTypeStr {
			return AttributeDecision{Key: "server.address", Action: AttributeSuppress}, true
		}
		return AttributeDecision{}, false
	}
	target, expected := "server.address", pcommon.ValueTypeStr
	if key == "net.peer.port" {
		target, expected = "server.port", pcommon.ValueTypeInt
	}
	if _, exists := attrs.Get(target); exists {
		return AttributeDecision{Key: target, Action: AttributeSuppress}, true
	}
	if value.Type() != expected {
		return keepSource(key), true
	}
	return AttributeDecision{Key: target, Action: AttributeKeepValue}, true
}

func hasClientProtocol(attrs pcommon.Map) bool {
	for _, key := range [...]string{"http.request.method", "http.method", "db.system.name", "db.system"} {
		if stringAttribute(attrs, key) != "" {
			return true
		}
	}
	return false
}
