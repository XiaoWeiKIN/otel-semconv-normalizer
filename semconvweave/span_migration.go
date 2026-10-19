// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// SpanMigrationOptions supplies producer knowledge that cannot be recovered from
// recorded attributes. All callbacks must be deterministic and free of effects.
// Callers must not mutate options or the source span during projection.
type SpanMigrationOptions struct {
	KnownHTTPMethods []string
	// KnownRPCMethods contains fully qualified, case-sensitive registered methods.
	// Nil means no methods are known; unrecognized legacy methods become _OTHER.
	KnownRPCMethods []string
	// HTTPServer contains the effective request scheme and authority, after the
	// instrumentation has applied its trusted-proxy policy. Nil means unknown.
	HTTPServer *HTTPServerContext
	// RPCServer is the configured logical server, not the connected socket peer.
	RPCServer *RPCServerContext
	// SanitizeDBQuery must remove sensitive literals for the database dialect.
	// Returning false preserves the legacy attribute and reports missing context.
	SanitizeDBQuery func(string) (string, bool)
	// DBCollection is the application-provided single collection name. It must
	// not be inferred by parsing query text. Empty means unknown.
	DBCollection string
	// DBInstanceNamespace is the complete namespace incorporating db.instance.id
	// according to the database's convention. DBInstanceServer is an alternative
	// when the instance denotes a server; setting both is rejected.
	DBInstanceNamespace string
	DBInstanceServer    string
	// HeaderNames maps recorded HTTP header suffixes to their original wire
	// names. Required only for legacy suffixes containing ambiguous underscores.
	HeaderNames map[string]string
	// HTTPHostTarget identifies historical http.host usage: server.address,
	// client.address, or http.request.header.host. Empty means unknown.
	HTTPHostTarget string
	// SensitiveQueryParameters fully overrides the OTel v1.41.0 default list.
	// Nil selects AWSAccessKeyId, Signature, sig and X-Goog-Signature.
	SensitiveQueryParameters []string
}

// HTTPServerContext is request-derived authority, never a local socket address.
// Port zero means unknown; values outside 1..65535 are rejected.
type HTTPServerContext struct {
	Scheme, Address string
	Port            int64
}

// RPCServerContext describes the configured group of RPC server instances.
// Port zero means unknown. Address may include a resolver-specific scheme/path.
type RPCServerContext struct {
	Address string
	Port    int64
}

// MigrationIssueCode describes why a legacy attribute was not translated.
type MigrationIssueCode string

const (
	MigrationMissingContext MigrationIssueCode = "missing_context"
	MigrationInvalidValue   MigrationIssueCode = "invalid_value"
	// MigrationRemoved records intentional omission of an attribute removed by
	// the community convention. It does not make a projection incomplete.
	MigrationRemoved MigrationIssueCode = "removed"
)

// MigrationIssue identifies an input attribute, never its potentially sensitive value.
type MigrationIssue struct {
	Key    string
	Code   MigrationIssueCode
	Reason string
}

// SpanProjection owns new attributes and a projected status description. It
// never aliases or mutates the source span. Issues make incomplete migration
// observable; callers must not claim a canonical schema when Complete is false.
type SpanProjection struct {
	Attributes        pcommon.Map
	StatusDescription string
	Issues            []MigrationIssue
}

// Complete reports whether every attempted migration had sufficient valid data.
// It does not assert instrumentation conformance or validate arbitrary custom keys.
func (p SpanProjection) Complete() bool {
	for _, issue := range p.Issues {
		if issue.Code != MigrationRemoved {
			return false
		}
	}
	return true
}

// ProjectSpan projects recorded span attributes to SemConv v1.41.0, including
// split/merge/type changes and producer-dependent migrations. Current targets
// win by presence; legacy aliases are resolved against the unchanged source.
// Removed attributes are omitted and reported. Invalid or ambiguous legacy
// attributes are retained with an issue, never silently labelled canonical.
// Resource relocation, instrumentation of missing data, names, events and
// metrics are outside this attribute API.
func (w WeaveID) ProjectSpan(source ptrace.Span, options SpanMigrationOptions) SpanProjection {
	attrs := source.Attributes()
	result := SpanProjection{Attributes: pcommon.NewMap(), StatusDescription: source.Status().Message()}
	context := SpanContext{Name: source.Name(), Kind: source.Kind(), KnownHTTPMethods: options.KnownHTTPMethods}
	var work migrationWork
	var candidates [2]string
	candidateCount := 0
	var rpcLegacy, rpcCurrent string
	rpcCurrentPresent := false
	attrs.Range(func(key string, value pcommon.Value) bool {
		work |= migrationWorkForKey(key)
		if mayProjectContextAttribute(key) {
			if candidateCount < len(candidates) {
				candidates[candidateCount] = key
			}
			candidateCount++
		}
		switch key {
		case "rpc.system":
			if value.Type() == pcommon.ValueTypeStr {
				rpcLegacy = value.Str()
			}
		case "rpc.system.name":
			rpcCurrentPresent = true
			if value.Type() == pcommon.ValueTypeStr {
				rpcCurrent = value.Str()
			}
		}
		return true
	})
	switch {
	case candidateCount == 0:
		attrs.CopyTo(result.Attributes)
	case attrs.Len() >= 32 && candidateCount <= len(candidates):
		w.projectSparseAttributeMap(attrs, result.Attributes, context, candidates[:candidateCount])
	default:
		w.projectAttributeMap(attrs, result.Attributes, context)
	}
	rpc := rpcLegacy
	if rpcCurrentPresent {
		rpc = rpcCurrent
	}
	m := spanMigration{weave: w, source: source, attrs: attrs, out: &result, options: options, work: work, rpc: rpc}
	if work&migrationHTTP != 0 {
		m.projectHTTP()
	}
	if work&migrationDB != 0 {
		m.projectDatabase()
	}
	if work&migrationRPC != 0 || work&migrationPeer != 0 && rpc != "" {
		m.projectRPC()
	}
	if work&migrationMethod != 0 {
		m.reportInvalidHTTPMethod()
	}
	return result
}

type spanMigration struct {
	weave   WeaveID
	source  ptrace.Span
	attrs   pcommon.Map
	out     *SpanProjection
	options SpanMigrationOptions
	work    migrationWork
	rpc     string
}

func (m spanMigration) has(key string) bool { _, ok := m.attrs.Get(key); return ok }
func (m spanMigration) issue(key string, code MigrationIssueCode, reason string) {
	m.out.Issues = append(m.out.Issues, MigrationIssue{Key: key, Code: code, Reason: reason})
}
func (m spanMigration) remove(key string) {
	if m.has(key) {
		m.out.Attributes.Remove(key)
		m.issue(key, MigrationRemoved, "removed by SemConv v1.41.0; no equivalent attribute")
	}
}
func (m spanMigration) stringTarget(source, target, value string) {
	m.out.Attributes.Remove(source)
	if !m.has(target) {
		m.out.Attributes.PutStr(target, value)
	}
}
func (m spanMigration) intTarget(source, target string, value int64) {
	m.out.Attributes.Remove(source)
	if !m.has(target) {
		m.out.Attributes.PutInt(target, value)
	}
}
func (m spanMigration) typedRename(source, target string, typ pcommon.ValueType) {
	v, exists := m.attrs.Get(source)
	if !exists {
		return
	}
	m.out.Attributes.Remove(source)
	if m.has(target) {
		return
	}
	if v.Type() != typ {
		m.out.Attributes.Remove(target)
		v.CopyTo(m.out.Attributes.PutEmpty(source))
		m.issue(source, MigrationInvalidValue, "unexpected attribute type")
		return
	}
	v.CopyTo(m.out.Attributes.PutEmpty(target))
}
func (m spanMigration) hasHTTP() bool     { return m.work&migrationHTTPIdentity != 0 }
func (m spanMigration) rpcSystem() string { return m.rpc }

// Work bits describe contextual operations, separately from the generated-rule
// bitmap. They are collected from the immutable source, including ill-typed
// values; an absent bit skips only work with no possible observable effect.
type migrationWork uint32

const (
	migrationSocket migrationWork = 1 << iota
	migrationProtocol
	migrationHost
	migrationHTTPIdentity
	migrationURL
	migrationTarget
	migrationServer
	migrationPeer
	migrationHeaders
	migrationMethod
	migrationDBRemove
	migrationDBStatus
	migrationDBQuery
	migrationDBTable
	migrationDBInstance
	migrationDBNamespace
	migrationRPCStatus
	migrationRPCError
	migrationRPCLegacy
	migrationRPCRemove
)

const (
	migrationHTTP = migrationSocket | migrationProtocol | migrationHost | migrationHTTPIdentity | migrationURL | migrationTarget | migrationServer | migrationPeer | migrationHeaders
	migrationDB   = migrationDBRemove | migrationDBStatus | migrationDBQuery | migrationDBTable | migrationDBInstance | migrationDBNamespace
	migrationRPC  = migrationRPCStatus | migrationRPCError | migrationRPCLegacy | migrationRPCRemove
)

func migrationWorkForKey(key string) migrationWork {
	switch key {
	case "net.sock.family", "net.sock.peer.name", "server.socket.domain", "net.sock.host.addr", "net.sock.host.port", "server.socket.address", "server.socket.port", "net.sock.peer.addr", "net.sock.peer.port":
		return migrationSocket
	case "net.protocol.version", "net.app.protocol.version", "messaging.protocol_version":
		return migrationProtocol
	case "http.flavor":
		return migrationProtocol | migrationHTTPIdentity
	case "http.method":
		return migrationMethod | migrationHTTPIdentity
	case "http.request.method":
		return migrationHTTPIdentity
	case "http.host":
		return migrationHost
	case "http.url":
		return migrationURL
	case "http.target":
		return migrationTarget | migrationHTTPIdentity
	case "http.scheme", "net.host.name", "net.host.port":
		return migrationServer
	case "net.peer.name", "net.peer.port":
		return migrationPeer
	case "db.connection_string", "db.user", "db.jdbc.driver_classname", "db.cosmosdb.operation_type":
		return migrationDBRemove
	case "db.cosmosdb.status_code":
		return migrationDBStatus
	case "db.statement":
		return migrationDBQuery
	case "db.sql.table":
		return migrationDBTable
	case "db.instance.id":
		return migrationDBInstance
	case "db.name", "db.redis.database_index", "db.mssql.instance_name", "db.cassandra.keyspace", "db.hbase.namespace":
		return migrationDBNamespace
	case "rpc.grpc.status_code", "rpc.jsonrpc.error_code":
		return migrationRPCStatus | migrationRPCLegacy
	case "rpc.jsonrpc.error_message":
		return migrationRPCError
	case "rpc.service", "rpc.system":
		return migrationRPCLegacy
	case "message.compressed_size", "message.id", "message.type", "message.uncompressed_size", "rpc.message.compressed_size", "rpc.message.id", "rpc.message.type", "rpc.message.uncompressed_size":
		return migrationRPCRemove
	}
	if strings.HasPrefix(key, "http.request.header.") || strings.HasPrefix(key, "http.response.header.") {
		return migrationHeaders
	}
	return 0
}

// The bitmap belongs to the installed rules, while contextual peer projection
// is an additional rule of the multi-output API. Neither may hide the other.
func mayProjectContextAttribute(key string) bool {
	return MayTranslateSpanAttribute(key) || key == "net.peer.name" || key == "net.peer.port"
}

func (w WeaveID) projectAttributeMap(attrs, output pcommon.Map, context SpanContext) {
	// One reservation instead of growing for every batch of new keys. Unlike
	// copy-then-rewrite, this only materializes the final value for renamed keys.
	output.EnsureCapacity(attrs.Len())
	attrs.Range(func(key string, value pcommon.Value) bool {
		if !mayProjectContextAttribute(key) {
			value.CopyTo(output.PutEmpty(key))
			return true
		}
		for _, decision := range w.SpanAttributeWithContext(attrs, context, key, value) {
			switch decision.Action {
			case AttributeKeepValue:
				value.CopyTo(output.PutEmpty(decision.Key))
			case AttributeReplaceString:
				output.PutStr(decision.Key, decision.StringValue)
			}
		}
		return true
	})
}

// Sparse projection avoids destination searches for every unchanged attribute.
// The copy owns its values. Move only those copied values, reacquiring handles
// after insertion; pdata invalidates handles when the map's shape changes.
func (w WeaveID) projectSparseAttributeMap(attrs, output pcommon.Map, context SpanContext, keys []string) {
	var plans [2][2]AttributeDecision
	capacity, size := attrs.Len(), attrs.Len()
	for i, key := range keys {
		value, _ := attrs.Get(key)
		plans[i] = w.SpanAttributeWithContext(attrs, context, key, value)
		keep := false
		for _, decision := range plans[i] {
			if decision.Action == AttributeSuppress {
				continue
			}
			if decision.Key == key {
				keep = true
			} else {
				size++
			}
		}
		capacity = max(capacity, size)
		if !keep {
			size--
		}
	}
	output.EnsureCapacity(capacity)
	attrs.CopyTo(output)
	for i, key := range keys {
		decisions := plans[i]
		for _, decision := range decisions {
			switch decision.Action {
			case AttributeKeepValue:
				if decision.Key == key {
					continue
				}
				output.PutEmpty(decision.Key)
				src, _ := output.Get(key)
				dst, _ := output.Get(decision.Key)
				// Two KeepValue outputs need distinct owning values.
				if decisions[0].Action == AttributeKeepValue && decisions[1].Action == AttributeKeepValue {
					value, _ := attrs.Get(key)
					value.CopyTo(dst)
				} else {
					src.MoveTo(dst)
				}
			case AttributeReplaceString:
				output.PutStr(decision.Key, decision.StringValue)
			}
		}
		keep := false
		for _, decision := range decisions {
			keep = keep || decision.Action != AttributeSuppress && decision.Key == key
		}
		if !keep {
			output.Remove(key)
		}
	}
}
