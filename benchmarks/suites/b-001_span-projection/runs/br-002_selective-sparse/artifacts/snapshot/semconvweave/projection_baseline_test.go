// SPDX-License-Identifier: Apache-2.0
// Frozen pre-optimization implementation: test-only behavioral/performance control.
package semconvweave

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

func (p SpanProjection) baselineComplete() bool {
	for _, baselineIssue := range p.Issues {
		if baselineIssue.Code != MigrationRemoved {
			return false
		}
	}
	return true
}

func (w WeaveID) baselineProjectSpan(source ptrace.Span, options SpanMigrationOptions) SpanProjection {
	attrs := source.Attributes()
	result := SpanProjection{Attributes: pcommon.NewMap(), StatusDescription: source.Status().Message()}
	context := SpanContext{Name: source.Name(), Kind: source.Kind(), KnownHTTPMethods: options.KnownHTTPMethods}
	attrs.Range(func(key string, value pcommon.Value) bool {
		for _, d := range w.baselineSpanAttributeWithContext(attrs, context, key, value) {
			switch d.Action {
			case AttributeKeepValue:
				value.CopyTo(result.Attributes.PutEmpty(d.Key))
			case AttributeReplaceString:
				result.Attributes.PutStr(d.Key, d.StringValue)
			}
		}
		return true
	})
	m := baselineSpanMigration{weave: w, source: source, attrs: attrs, out: &result, options: options}
	m.baselineProjectHTTP()
	m.baselineProjectDatabase()
	m.baselineProjectRPC()
	m.baselineReportInvalidHTTPMethod()
	return result
}

type baselineSpanMigration struct {
	weave   WeaveID
	source  ptrace.Span
	attrs   pcommon.Map
	out     *SpanProjection
	options SpanMigrationOptions
}

func (m baselineSpanMigration) baselineHas(key string) bool { _, ok := m.attrs.Get(key); return ok }

func (m baselineSpanMigration) baselineIssue(key string, code MigrationIssueCode, reason string) {
	m.out.Issues = append(m.out.Issues, MigrationIssue{Key: key, Code: code, Reason: reason})
}

func (m baselineSpanMigration) baselineRemove(key string) {
	if m.baselineHas(key) {
		m.out.Attributes.Remove(key)
		m.baselineIssue(key, MigrationRemoved, "removed by SemConv v1.41.0; no equivalent attribute")
	}
}

func (m baselineSpanMigration) baselineStringTarget(source, target, value string) {
	m.out.Attributes.Remove(source)
	if !m.baselineHas(target) {
		m.out.Attributes.PutStr(target, value)
	}
}

func (m baselineSpanMigration) baselineIntTarget(source, target string, value int64) {
	m.out.Attributes.Remove(source)
	if !m.baselineHas(target) {
		m.out.Attributes.PutInt(target, value)
	}
}

func (m baselineSpanMigration) baselineTypedRename(source, target string, typ pcommon.ValueType) {
	v, exists := m.attrs.Get(source)
	if !exists {
		return
	}
	m.out.Attributes.Remove(source)
	if m.baselineHas(target) {
		return
	}
	if v.Type() != typ {
		m.out.Attributes.Remove(target)
		v.CopyTo(m.out.Attributes.PutEmpty(source))
		m.baselineIssue(source, MigrationInvalidValue, "unexpected attribute type")
		return
	}
	v.CopyTo(m.out.Attributes.PutEmpty(target))
}

func (m baselineSpanMigration) baselineHasHTTP() bool {
	return m.baselineHas("http.method") || m.baselineHas("http.request.method") || m.baselineHas("http.flavor") || m.baselineHas("http.target")
}

func (m baselineSpanMigration) baselineRpcSystem() string {
	if m.baselineHas("rpc.system.name") {
		return baselineStringAttribute(m.attrs, "rpc.system.name")
	}
	return baselineStringAttribute(m.attrs, "rpc.system")
}

func (m baselineSpanMigration) baselineProjectHTTP() {
	for _, key := range []string{"net.sock.family", "net.sock.peer.name", "server.socket.domain"} {
		m.baselineRemove(key)
	}
	for _, pair := range [][2]string{{"net.sock.host.addr", "network.local.address"}, {"net.sock.host.port", "network.local.port"}, {"server.socket.address", "network.local.address"}, {"server.socket.port", "network.local.port"}} {
		typ := pcommon.ValueTypeStr
		if strings.HasSuffix(pair[1], ".port") {
			typ = pcommon.ValueTypeInt
		}
		// The newer socket alias wins even if it is ill-typed.
		if strings.HasPrefix(pair[0], "net.sock.") && m.baselineHas("server.socket."+strings.TrimPrefix(pair[1], "network.local.")) {
			m.out.Attributes.Remove(pair[0])
			continue
		}
		m.baselineTypedRename(pair[0], pair[1], typ)
	}
	// Socket attributes supersede their pre-1.13 IP aliases. Resolve this
	// explicitly before deriving HTTP client.address.
	m.baselineTypedRename("net.sock.peer.addr", "network.peer.address", pcommon.ValueTypeStr)
	m.baselineTypedRename("net.sock.peer.port", "network.peer.port", pcommon.ValueTypeInt)
	m.baselineProtocolVersion()
	if m.baselineHas("http.host") {
		m.baselineHttpHost()
	}
	if m.baselineHasHTTP() && m.source.Kind() == ptrace.SpanKindServer && !m.baselineHas("client.address") && !m.baselineHas("http.client_ip") {
		if v, ok := m.out.Attributes.Get("network.peer.address"); ok && v.Type() == pcommon.ValueTypeStr {
			m.out.Attributes.PutStr("client.address", v.Str())
		}
	}
	if m.baselineHas("http.url") {
		m.baselineHttpURL()
	}
	if m.baselineHas("http.target") {
		m.baselineHttpTarget()
	}
	for _, pair := range [][2]string{{"http.scheme", "url.scheme"}, {"net.host.name", "server.address"}, {"net.host.port", "server.port"}} {
		key, target := pair[0], pair[1]
		if !m.baselineHas(key) {
			continue
		}
		if m.baselineHas(target) {
			m.out.Attributes.Remove(key)
			continue
		}
		ctx := m.options.HTTPServer
		if ctx == nil || m.source.Kind() != ptrace.SpanKindServer {
			m.baselineIssue(key, MigrationMissingContext, "effective HTTP server request scheme/authority is required")
			continue
		}
		switch key {
		case "http.scheme":
			if !baselineValidScheme(ctx.Scheme) {
				m.baselineIssue(key, MigrationInvalidValue, "effective URL scheme is missing or invalid")
				continue
			}
			m.baselineStringTarget(key, target, ctx.Scheme)
		case "net.host.name":
			if ctx.Address == "" {
				m.baselineIssue(key, MigrationMissingContext, "request authority is required")
				continue
			}
			m.baselineStringTarget(key, target, ctx.Address)
		case "net.host.port":
			if ctx.Port < 1 || ctx.Port > 65535 || ctx.Address == "" && !m.baselineHas("server.address") {
				m.baselineIssue(key, MigrationInvalidValue, "request authority port is missing or invalid")
				continue
			}
			m.baselineIntTarget(key, target, ctx.Port)
		}
	}
	for _, key := range []string{"net.peer.name", "net.peer.port"} {
		if !m.baselineHas(key) {
			continue
		}
		if _, exists := m.out.Attributes.Get(key); exists && m.baselineRpcSystem() == "" {
			m.baselineIssue(key, MigrationMissingContext, "CLIENT kind and HTTP or database protocol are required")
		}
	}
	m.attrs.Range(func(key string, value pcommon.Value) bool {
		for _, prefix := range []string{"http.request.header.", "http.response.header."} {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			suffix := strings.TrimPrefix(key, prefix)
			if m.weave == WeaveCanonical && !m.baselineHas("http.method") && !m.baselineHas("http.target") && m.options.HeaderNames == nil {
				continue
			}
			if !strings.Contains(suffix, "_") {
				continue
			}
			wire, ok := m.options.HeaderNames[suffix]
			if !ok {
				m.baselineIssue(key, MigrationMissingContext, "original HTTP header name is required to resolve underscores")
				continue
			}
			if !baselineValidHeaderName(wire) || strings.ReplaceAll(strings.ToLower(wire), "-", "_") != suffix {
				m.baselineIssue(key, MigrationInvalidValue, "invalid original HTTP header name")
				continue
			}
			target := prefix + strings.ToLower(wire)
			if target != key {
				m.baselineTypedRename(key, target, pcommon.ValueTypeSlice)
			}
		}
		return true
	})
}

func (m baselineSpanMigration) baselineProtocolVersion() {
	// Newer aliases win by presence, even when ill-typed.
	keys := []string{"network.protocol.version", "net.protocol.version", "net.app.protocol.version", "messaging.protocol_version", "http.flavor"}
	winner := ""
	for _, key := range keys {
		if m.baselineHas(key) {
			winner = key
			break
		}
	}
	if winner == "" || winner == keys[0] {
		m.out.Attributes.Remove("http.flavor")
		return
	}
	for _, key := range keys[1:] {
		if key != winner {
			m.out.Attributes.Remove(key)
		}
	}
	v, _ := m.attrs.Get(winner)
	if v.Type() != pcommon.ValueTypeStr {
		m.out.Attributes.Remove("network.protocol.version")
		v.CopyTo(m.out.Attributes.PutEmpty(winner))
		m.baselineIssue(winner, MigrationInvalidValue, "protocol version must be a string")
		return
	}
	value := v.Str()
	if winner == "http.flavor" && (value == "SPDY" || value == "QUIC") {
		// These legacy enum values carry a name but no version. Do not invent one.
		if !m.baselineHas("network.protocol.name") && !m.baselineHas("net.protocol.name") && !m.baselineHas("net.app.protocol.name") && !m.baselineHas("messaging.protocol") {
			m.out.Attributes.PutStr("network.protocol.name", strings.ToLower(value))
		}
		m.out.Attributes.Remove(winner)
		return
	}
	protocol := baselineStringAttribute(m.out.Attributes, "network.protocol.name")
	if protocol == "http" || protocol == "" && (m.baselineHasHTTP() || winner == "http.flavor") {
		if value == "2.0" {
			value = "2"
		}
		if value == "3.0" {
			value = "3"
		}
	}
	m.baselineStringTarget(winner, "network.protocol.version", value)
}

func (m baselineSpanMigration) baselineHttpTarget() {
	const key = "http.target"
	if m.baselineHas("url.path") && m.baselineHas("url.query") {
		m.out.Attributes.Remove(key)
		return
	}
	v, _ := m.attrs.Get(key)
	if v.Type() != pcommon.ValueTypeStr {
		m.baselineIssue(key, MigrationInvalidValue, "HTTP request target must be a string")
		return
	}
	raw := v.Str()
	// origin-form and absolute-form are URL-bearing targets. Authority-form
	// (CONNECT) baselineHas no path/query; asterisk-form baselineHas the literal path '*'.
	if raw == "*" {
		m.baselineStringTarget(key, "url.path", "*")
		return
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || strings.Contains(raw, "#") || (u.IsAbs() && u.Host == "") {
		m.baselineIssue(key, MigrationInvalidValue, "invalid URL-bearing HTTP request target")
		return
	}
	query, ok := baselineRedactQuery(u.RawQuery, m.options.SensitiveQueryParameters)
	if !ok {
		m.baselineIssue(key, MigrationInvalidValue, "invalid query escaping")
		return
	}
	m.out.Attributes.Remove(key)
	if !m.baselineHas("url.path") {
		m.out.Attributes.PutStr("url.path", u.EscapedPath())
	}
	if u.RawQuery != "" && !m.baselineHas("url.query") {
		m.out.Attributes.PutStr("url.query", query)
	}
}

func baselineRedactQuery(raw string, sensitive []string) (string, bool) {
	if sensitive == nil {
		sensitive = []string{"AWSAccessKeyId", "Signature", "sig", "X-Goog-Signature"}
	}
	if raw == "" {
		return "", true
	}
	parts := strings.Split(raw, "&")
	for i, part := range parts {
		key, value, hasValue := strings.Cut(part, "=")
		decoded, err := url.QueryUnescape(key)
		if err != nil {
			return "", false
		}
		if _, err = url.QueryUnescape(value); err != nil {
			return "", false
		}
		if hasValue && slices.Contains(sensitive, decoded) {
			parts[i] = key + "=REDACTED"
		}
	}
	return strings.Join(parts, "&"), true
}

func baselineValidScheme(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.') {
			continue
		}
		return false
	}
	return true
}

func baselineValidHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c) {
			continue
		}
		return false
	}
	return true
}

func (m baselineSpanMigration) baselineHttpHost() {
	const key = "http.host"
	target := m.options.HTTPHostTarget
	if target != "server.address" && target != "client.address" && target != "http.request.header.host" {
		m.baselineIssue(key, MigrationMissingContext, "historical HTTP host usage is required")
		return
	}
	value, _ := m.attrs.Get(key)
	if value.Type() != pcommon.ValueTypeStr {
		m.baselineIssue(key, MigrationInvalidValue, "HTTP host must be a string")
		return
	}
	if target == "http.request.header.host" {
		m.out.Attributes.Remove(key)
		if !m.baselineHas(target) {
			m.out.Attributes.PutEmptySlice(target).AppendEmpty().SetStr(value.Str())
		}
		return
	}
	authority, err := url.Parse("//" + value.Str())
	if err != nil || authority.Hostname() == "" || authority.User != nil || authority.Path != "" || authority.RawQuery != "" || authority.Fragment != "" {
		m.baselineIssue(key, MigrationInvalidValue, "invalid HTTP host authority")
		return
	}
	var port int64
	if authority.Port() != "" {
		port, err = strconv.ParseInt(authority.Port(), 10, 64)
		if err != nil || port < 1 || port > 65535 {
			m.baselineIssue(key, MigrationInvalidValue, "invalid HTTP host port")
			return
		}
	}
	m.baselineStringTarget(key, target, authority.Hostname())
	portKey := strings.TrimSuffix(target, "address") + "port"
	if port != 0 && !m.baselineHas(portKey) {
		m.out.Attributes.PutInt(portKey, port)
	}
}

func (m baselineSpanMigration) baselineReportInvalidHTTPMethod() {
	if v, ok := m.attrs.Get("http.method"); ok && !m.baselineHas("http.request.method") && v.Type() != pcommon.ValueTypeStr {
		m.baselineIssue("http.method", MigrationInvalidValue, "HTTP method must be a string")
	}
}

func (m baselineSpanMigration) baselineHttpURL() {
	const key = "http.url"
	if m.baselineHas("url.full") {
		m.out.Attributes.Remove(key)
		return
	}
	value, _ := m.attrs.Get(key)
	// The static rename may have emitted a value already. Restore the source
	// before parsing so failures cannot expose a partially canonicalized URL.
	m.out.Attributes.Remove("url.full")
	value.CopyTo(m.out.Attributes.PutEmpty(key))
	if value.Type() != pcommon.ValueTypeStr {
		m.baselineIssue(key, MigrationInvalidValue, "HTTP URL must be a string")
		return
	}
	u, err := url.Parse(value.Str())
	if err != nil || !u.IsAbs() || u.Host == "" {
		m.baselineIssue(key, MigrationInvalidValue, "HTTP URL must be absolute")
		return
	}
	query, ok := baselineRedactQuery(u.RawQuery, m.options.SensitiveQueryParameters)
	if !ok {
		m.baselineIssue(key, MigrationInvalidValue, "invalid query escaping")
		return
	}
	u.User = nil
	u.RawQuery = query
	m.baselineStringTarget(key, "url.full", u.String())
}

func (m baselineSpanMigration) baselineProjectDatabase() {
	for _, key := range []string{"db.connection_string", "db.user", "db.jdbc.driver_classname", "db.cosmosdb.operation_type"} {
		m.baselineRemove(key)
	}
	if v, ok := m.attrs.Get("db.cosmosdb.status_code"); ok {
		if m.baselineHas("db.response.status_code") {
			m.out.Attributes.Remove("db.cosmosdb.status_code")
		} else if v.Type() == pcommon.ValueTypeInt {
			m.baselineStringTarget("db.cosmosdb.status_code", "db.response.status_code", strconv.FormatInt(v.Int(), 10))
		} else {
			m.baselineIssue("db.cosmosdb.status_code", MigrationInvalidValue, "Cosmos DB status code must be an integer")
		}
	}
	if v, ok := m.attrs.Get("db.statement"); ok {
		switch {
		case m.baselineHas("db.query.text"):
			m.out.Attributes.Remove("db.statement")
		case v.Type() != pcommon.ValueTypeStr:
			m.baselineIssue("db.statement", MigrationInvalidValue, "database query must be a string")
		case m.options.SanitizeDBQuery == nil:
			m.baselineIssue("db.statement", MigrationMissingContext, "database-specific query sanitizer is required")
		default:
			sanitized, accepted := m.options.SanitizeDBQuery(v.Str())
			if !accepted {
				m.baselineIssue("db.statement", MigrationMissingContext, "query sanitizer could not establish safe text")
			} else {
				m.baselineStringTarget("db.statement", "db.query.text", sanitized)
			}
		}
	}
	if m.baselineHas("db.sql.table") {
		switch {
		case m.baselineHas("db.collection.name"):
			m.out.Attributes.Remove("db.sql.table")
		case m.options.DBCollection != "":
			m.baselineStringTarget("db.sql.table", "db.collection.name", m.options.DBCollection)
		default:
			m.baselineIssue("db.sql.table", MigrationMissingContext, "application-provided single collection name is required")
		}
	}
	if m.baselineHas("db.instance.id") {
		namespace, server := m.options.DBInstanceNamespace, m.options.DBInstanceServer
		switch {
		case namespace != "" && server != "":
			m.baselineIssue("db.instance.id", MigrationInvalidValue, "instance cannot designate both namespace and server")
		case namespace != "":
			m.baselineStringTarget("db.instance.id", "db.namespace", namespace)
		case server != "":
			m.baselineStringTarget("db.instance.id", "server.address", server)
		default:
			m.baselineIssue("db.instance.id", MigrationMissingContext, "database-specific instance meaning is required")
		}
	}
	for _, key := range []string{"db.name", "db.redis.database_index", "db.mssql.instance_name", "db.cassandra.keyspace", "db.hbase.namespace"} {
		if _, ok := m.out.Attributes.Get(key); ok {
			m.baselineIssue(key, MigrationMissingContext, "valid database system and namespace source are required")
		}
	}
}

var baselineGrpcStatusNames = [...]string{"OK", "CANCELLED", "UNKNOWN", "INVALID_ARGUMENT", "DEADLINE_EXCEEDED", "NOT_FOUND", "ALREADY_EXISTS", "PERMISSION_DENIED", "RESOURCE_EXHAUSTED", "FAILED_PRECONDITION", "ABORTED", "OUT_OF_RANGE", "UNIMPLEMENTED", "INTERNAL", "UNAVAILABLE", "DATA_LOSS", "UNAUTHENTICATED"}

func (m baselineSpanMigration) baselineProjectRPC() {
	system := m.baselineRpcSystem()
	for _, key := range []string{"rpc.grpc.status_code", "rpc.jsonrpc.error_code"} {
		v, ok := m.attrs.Get(key)
		if !ok {
			continue
		}
		if m.baselineHas("rpc.response.status_code") {
			m.out.Attributes.Remove(key)
			continue
		}
		if (m.baselineHas("rpc.grpc.status_code") && m.baselineHas("rpc.jsonrpc.error_code")) || m.baselineHas("rpc.connect_rpc.error_code") {
			m.out.Attributes.Remove("rpc.response.status_code")
			for _, sourceKey := range []string{"rpc.grpc.status_code", "rpc.jsonrpc.error_code", "rpc.connect_rpc.error_code"} {
				if sourceValue, exists := m.attrs.Get(sourceKey); exists {
					sourceValue.CopyTo(m.out.Attributes.PutEmpty(sourceKey))
				}
			}
			m.baselineIssue(key, MigrationInvalidValue, "conflicting RPC status code sources")
			continue
		}
		if v.Type() != pcommon.ValueTypeInt {
			m.baselineIssue(key, MigrationInvalidValue, "RPC response status code must be an integer")
			continue
		}
		var code string
		if key == "rpc.grpc.status_code" {
			if v.Int() < 0 || v.Int() >= int64(len(baselineGrpcStatusNames)) {
				m.baselineIssue(key, MigrationInvalidValue, "gRPC status code is outside 0..16")
				continue
			}
			if system != "" && system != "grpc" {
				m.baselineIssue(key, MigrationInvalidValue, "gRPC status code conflicts with RPC system")
				continue
			}
			code = baselineGrpcStatusNames[v.Int()]
		} else {
			if system != "" && system != "jsonrpc" {
				m.baselineIssue(key, MigrationInvalidValue, "JSON-RPC error code conflicts with RPC system")
				continue
			}
			code = strconv.FormatInt(v.Int(), 10)
		}
		m.baselineStringTarget(key, "rpc.response.status_code", code)
	}
	if v, ok := m.attrs.Get("rpc.jsonrpc.error_message"); ok {
		if system != "" && system != "jsonrpc" {
			m.baselineIssue("rpc.jsonrpc.error_message", MigrationInvalidValue, "JSON-RPC error message conflicts with RPC system")
		} else if m.source.Status().Code() != ptrace.StatusCodeError {
			m.baselineIssue("rpc.jsonrpc.error_message", MigrationMissingContext, "ERROR span status is required for status description")
		} else if v.Type() != pcommon.ValueTypeStr {
			m.baselineIssue("rpc.jsonrpc.error_message", MigrationInvalidValue, "JSON-RPC error message must be a string")
		} else {
			if m.out.StatusDescription == "" {
				m.out.StatusDescription = v.Str()
			}
			m.out.Attributes.Remove("rpc.jsonrpc.error_message")
		}
	}
	// Same-key changes require evidence of legacy RPC semantics. Canonical
	// rpc.system.name alone is never proof that a method is still unqualified.
	legacy := m.baselineHas("rpc.service") || m.baselineHas("rpc.system") || m.baselineHas("rpc.grpc.status_code") || m.baselineHas("rpc.jsonrpc.error_code")
	if legacy {
		m.baselineRpcMethod(system)
	}
	for _, key := range []string{"message.compressed_size", "message.id", "message.type", "message.uncompressed_size", "rpc.message.compressed_size", "rpc.message.id", "rpc.message.type", "rpc.message.uncompressed_size"} {
		m.baselineRemove(key)
	}
	if system != "" {
		// rpc.system.name marks a dual-emitted current endpoint. A genuinely
		// legacy RPC endpoint baselineHas the same key but changed meaning in v1.41.0.
		if legacy && !m.baselineHas("rpc.system.name") {
			m.baselineRpcExistingEndpoint()
		}
		// Network and client attributes remain valid general attributes; removal from the RPC
		// requirement table does not make them globally invalid.
		for _, key := range []string{"net.peer.name", "net.peer.port"} {
			if !m.baselineHas(key) {
				continue
			}
			target := "server.address"
			if key == "net.peer.port" {
				target = "server.port"
			}
			if m.baselineHas(target) {
				m.out.Attributes.Remove(key)
				continue
			}
			ctx := m.options.RPCServer
			if ctx == nil || ctx.Address == "" {
				m.baselineIssue(key, MigrationMissingContext, "configured logical RPC server is required")
				continue
			}
			if key == "net.peer.name" {
				m.baselineStringTarget(key, target, ctx.Address)
			} else if ctx.Port > 0 && ctx.Port <= 65535 {
				m.baselineIntTarget(key, target, ctx.Port)
			} else {
				m.baselineIssue(key, MigrationInvalidValue, "configured RPC server port is missing or invalid")
			}
		}
	}
}

func (m baselineSpanMigration) baselineRpcMethod(system string) {
	method, ok := m.attrs.Get("rpc.method")
	if !ok {
		if m.baselineHas("rpc.service") {
			m.baselineIssue("rpc.service", MigrationMissingContext, "RPC method is required to merge service")
		}
		return
	}
	if method.Type() != pcommon.ValueTypeStr || method.Str() == "" {
		m.baselineIssue("rpc.method", MigrationInvalidValue, "RPC method must be a nonempty string")
		return
	}
	if method.Str() == "_OTHER" {
		m.out.Attributes.Remove("rpc.service")
		return
	}
	original := method.Str()
	if m.baselineHas("rpc.system.name") && strings.Contains(original, "/") {
		// The already-qualified current method wins over a dual-emitted service.
		m.out.Attributes.Remove("rpc.service")
		return
	}
	service := baselineStringAttribute(m.attrs, "rpc.service")
	if m.baselineHas("rpc.service") && service == "" {
		m.baselineIssue("rpc.service", MigrationInvalidValue, "RPC service must be a nonempty string")
		return
	}
	if system != "jsonrpc" {
		if service != "" && !strings.Contains(original, "/") {
			original = service + "/" + original
		}
		if !strings.Contains(original, "/") {
			m.baselineIssue("rpc.method", MigrationMissingContext, "fully qualified RPC service/method is required")
			return
		}
	}
	if system != "jsonrpc" && service != "" && !strings.HasPrefix(original, service+"/") {
		m.baselineIssue("rpc.method", MigrationInvalidValue, "RPC service conflicts with qualified method")
		return
	}
	m.out.Attributes.Remove("rpc.service")
	if slices.Contains(m.options.KnownRPCMethods, original) {
		m.out.Attributes.PutStr("rpc.method", original)
		return
	}
	m.out.Attributes.PutStr("rpc.method", "_OTHER")
	if !m.baselineHas("rpc.method_original") {
		m.out.Attributes.PutStr("rpc.method_original", original)
	}
}

func (m baselineSpanMigration) baselineRpcExistingEndpoint() {
	for _, key := range []string{"server.address", "server.port"} {
		if !m.baselineHas(key) {
			continue
		}
		ctx := m.options.RPCServer
		if ctx == nil || ctx.Address == "" {
			m.baselineIssue(key, MigrationMissingContext, "legacy RPC endpoint requires configured logical server")
			continue
		}
		if key == "server.address" {
			m.out.Attributes.PutStr(key, ctx.Address)
			continue
		}
		if ctx.Port < 1 || ctx.Port > 65535 {
			m.baselineIssue(key, MigrationInvalidValue, "configured RPC server port is missing or invalid")
			continue
		}
		m.out.Attributes.PutInt(key, ctx.Port)
	}
}

func (w WeaveID) baselineSpanAttributeWithContext(
	attrs pcommon.Map, context SpanContext, sourceKey string, sourceValue pcommon.Value,
) [2]AttributeDecision {
	if sourceKey == "http.method" && sourceValue.Type() == pcommon.ValueTypeStr {
		if _, exists := attrs.Get("http.request.method"); exists {
			return [2]AttributeDecision{}
		}
		method := sourceValue.Str()
		known := baselineIsDefaultHTTPMethod(method)
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
	if decision, ok := baselineContextualNetworkAttribute(attrs, context.Kind, sourceKey, sourceValue); ok {
		return [2]AttributeDecision{decision}
	}
	return [2]AttributeDecision{w.baselineSpanAttribute(attrs, context.Name, sourceKey, sourceValue)}
}

func baselineIsDefaultHTTPMethod(method string) bool {
	switch method {
	case "CONNECT", "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT", "TRACE", "QUERY":
		return true
	default:
		return false
	}
}

func baselineSemanticSpanAttribute(attrs pcommon.Map, key string, value pcommon.Value) (AttributeDecision, bool) {
	switch key {
	case "http.method":
		if _, exists := attrs.Get("http.request.method"); exists {
			return AttributeDecision{Key: "http.request.method", Action: AttributeSuppress}, true
		}
		if value.Type() == pcommon.ValueTypeStr && baselineIsDefaultHTTPMethod(value.Str()) {
			return AttributeDecision{Key: "http.request.method", Action: AttributeKeepValue}, true
		}
		return keepSource(key), true
	case "db.name", "db.cassandra.keyspace", "db.hbase.namespace", "db.redis.database_index", "db.mssql.instance_name":
		return baselineDatabaseNamespace(attrs, key, value), true
	default:
		return AttributeDecision{}, false
	}
}

func baselineStringAttribute(attrs pcommon.Map, key string) string {
	v, ok := attrs.Get(key)
	if !ok || v.Type() != pcommon.ValueTypeStr {
		return ""
	}
	return v.Str()
}

func baselineDatabaseNamespace(attrs pcommon.Map, key string, value pcommon.Value) AttributeDecision {
	const target = "db.namespace"
	if _, exists := attrs.Get(target); exists {
		return AttributeDecision{Key: target, Action: AttributeSuppress}
	}
	// The newer Elasticsearch alias is already handled by the generated table.
	if _, exists := attrs.Get("db.elasticsearch.cluster.name"); exists {
		return AttributeDecision{Key: target, Action: AttributeSuppress}
	}
	system := baselineStringAttribute(attrs, "db.system.name")
	if _, exists := attrs.Get("db.system.name"); !exists {
		system = baselineStringAttribute(attrs, "db.system")
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
		name := baselineStringAttribute(attrs, "db.name")
		instance := baselineStringAttribute(attrs, "db.mssql.instance_name")
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
	if key != "db.name" && baselineStringAttribute(attrs, "db.name") != "" {
		return AttributeDecision{Key: target, Action: AttributeSuppress}
	}
	return AttributeDecision{Key: target, Action: AttributeKeepValue}
}

func baselineContextualNetworkAttribute(attrs pcommon.Map, kind ptrace.SpanKind, key string, value pcommon.Value) (AttributeDecision, bool) {
	if key != "net.peer.name" && key != "net.peer.port" && key != "http.server_name" {
		return AttributeDecision{}, false
	}
	// Peer means server only on supported client operations. In particular,
	// HTTP server peer attributes describe the client, and socket peer addresses
	// can describe a proxy rather than the logical server.
	if kind != ptrace.SpanKindClient || !baselineHasClientProtocol(attrs) {
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

func baselineHasClientProtocol(attrs pcommon.Map) bool {
	for _, key := range [...]string{"http.request.method", "http.method", "db.system.name", "db.system"} {
		if baselineStringAttribute(attrs, key) != "" {
			return true
		}
	}
	return false
}

func (w WeaveID) baselineSpanAttribute(attrs pcommon.Map, name, key string, value pcommon.Value) AttributeDecision {
	if decision, ok := baselineSemanticSpanAttribute(attrs, key, value); ok {
		return decision
	}
	return translateAttribute(w, generatedDomainSpan, attrs, name, "", key, value)
}
