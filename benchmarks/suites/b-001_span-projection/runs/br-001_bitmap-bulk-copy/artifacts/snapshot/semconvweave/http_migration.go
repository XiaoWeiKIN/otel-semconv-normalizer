// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func (m spanMigration) projectHTTP() {
	for _, key := range []string{"net.sock.family", "net.sock.peer.name", "server.socket.domain"} {
		m.remove(key)
	}
	for _, pair := range [][2]string{{"net.sock.host.addr", "network.local.address"}, {"net.sock.host.port", "network.local.port"}, {"server.socket.address", "network.local.address"}, {"server.socket.port", "network.local.port"}} {
		typ := pcommon.ValueTypeStr
		if strings.HasSuffix(pair[1], ".port") {
			typ = pcommon.ValueTypeInt
		}
		// The newer socket alias wins even if it is ill-typed.
		if strings.HasPrefix(pair[0], "net.sock.") && m.has("server.socket."+strings.TrimPrefix(pair[1], "network.local.")) {
			m.out.Attributes.Remove(pair[0])
			continue
		}
		m.typedRename(pair[0], pair[1], typ)
	}
	// Socket attributes supersede their pre-1.13 IP aliases. Resolve this
	// explicitly before deriving HTTP client.address.
	m.typedRename("net.sock.peer.addr", "network.peer.address", pcommon.ValueTypeStr)
	m.typedRename("net.sock.peer.port", "network.peer.port", pcommon.ValueTypeInt)
	m.protocolVersion()
	if m.has("http.host") {
		m.httpHost()
	}
	if m.hasHTTP() && m.source.Kind() == ptrace.SpanKindServer && !m.has("client.address") && !m.has("http.client_ip") {
		if v, ok := m.out.Attributes.Get("network.peer.address"); ok && v.Type() == pcommon.ValueTypeStr {
			m.out.Attributes.PutStr("client.address", v.Str())
		}
	}
	if m.has("http.url") {
		m.httpURL()
	}
	if m.has("http.target") {
		m.httpTarget()
	}
	for _, pair := range [][2]string{{"http.scheme", "url.scheme"}, {"net.host.name", "server.address"}, {"net.host.port", "server.port"}} {
		key, target := pair[0], pair[1]
		if !m.has(key) {
			continue
		}
		if m.has(target) {
			m.out.Attributes.Remove(key)
			continue
		}
		ctx := m.options.HTTPServer
		if ctx == nil || m.source.Kind() != ptrace.SpanKindServer {
			m.issue(key, MigrationMissingContext, "effective HTTP server request scheme/authority is required")
			continue
		}
		switch key {
		case "http.scheme":
			if !validScheme(ctx.Scheme) {
				m.issue(key, MigrationInvalidValue, "effective URL scheme is missing or invalid")
				continue
			}
			m.stringTarget(key, target, ctx.Scheme)
		case "net.host.name":
			if ctx.Address == "" {
				m.issue(key, MigrationMissingContext, "request authority is required")
				continue
			}
			m.stringTarget(key, target, ctx.Address)
		case "net.host.port":
			if ctx.Port < 1 || ctx.Port > 65535 || ctx.Address == "" && !m.has("server.address") {
				m.issue(key, MigrationInvalidValue, "request authority port is missing or invalid")
				continue
			}
			m.intTarget(key, target, ctx.Port)
		}
	}
	for _, key := range []string{"net.peer.name", "net.peer.port"} {
		if !m.has(key) {
			continue
		}
		if _, exists := m.out.Attributes.Get(key); exists && m.rpcSystem() == "" {
			m.issue(key, MigrationMissingContext, "CLIENT kind and HTTP or database protocol are required")
		}
	}
	m.attrs.Range(func(key string, value pcommon.Value) bool {
		for _, prefix := range []string{"http.request.header.", "http.response.header."} {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			suffix := strings.TrimPrefix(key, prefix)
			if m.weave == WeaveCanonical && !m.has("http.method") && !m.has("http.target") && m.options.HeaderNames == nil {
				continue
			}
			if !strings.Contains(suffix, "_") {
				continue
			}
			wire, ok := m.options.HeaderNames[suffix]
			if !ok {
				m.issue(key, MigrationMissingContext, "original HTTP header name is required to resolve underscores")
				continue
			}
			if !validHeaderName(wire) || strings.ReplaceAll(strings.ToLower(wire), "-", "_") != suffix {
				m.issue(key, MigrationInvalidValue, "invalid original HTTP header name")
				continue
			}
			target := prefix + strings.ToLower(wire)
			if target != key {
				m.typedRename(key, target, pcommon.ValueTypeSlice)
			}
		}
		return true
	})
}

func (m spanMigration) protocolVersion() {
	// Newer aliases win by presence, even when ill-typed.
	keys := []string{"network.protocol.version", "net.protocol.version", "net.app.protocol.version", "messaging.protocol_version", "http.flavor"}
	winner := ""
	for _, key := range keys {
		if m.has(key) {
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
		m.issue(winner, MigrationInvalidValue, "protocol version must be a string")
		return
	}
	value := v.Str()
	if winner == "http.flavor" && (value == "SPDY" || value == "QUIC") {
		// These legacy enum values carry a name but no version. Do not invent one.
		if !m.has("network.protocol.name") && !m.has("net.protocol.name") && !m.has("net.app.protocol.name") && !m.has("messaging.protocol") {
			m.out.Attributes.PutStr("network.protocol.name", strings.ToLower(value))
		}
		m.out.Attributes.Remove(winner)
		return
	}
	protocol := stringAttribute(m.out.Attributes, "network.protocol.name")
	if protocol == "http" || protocol == "" && (m.hasHTTP() || winner == "http.flavor") {
		if value == "2.0" {
			value = "2"
		}
		if value == "3.0" {
			value = "3"
		}
	}
	m.stringTarget(winner, "network.protocol.version", value)
}

func (m spanMigration) httpTarget() {
	const key = "http.target"
	if m.has("url.path") && m.has("url.query") {
		m.out.Attributes.Remove(key)
		return
	}
	v, _ := m.attrs.Get(key)
	if v.Type() != pcommon.ValueTypeStr {
		m.issue(key, MigrationInvalidValue, "HTTP request target must be a string")
		return
	}
	raw := v.Str()
	// origin-form and absolute-form are URL-bearing targets. Authority-form
	// (CONNECT) has no path/query; asterisk-form has the literal path '*'.
	if raw == "*" {
		m.stringTarget(key, "url.path", "*")
		return
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || strings.Contains(raw, "#") || (u.IsAbs() && u.Host == "") {
		m.issue(key, MigrationInvalidValue, "invalid URL-bearing HTTP request target")
		return
	}
	query, ok := redactQuery(u.RawQuery, m.options.SensitiveQueryParameters)
	if !ok {
		m.issue(key, MigrationInvalidValue, "invalid query escaping")
		return
	}
	m.out.Attributes.Remove(key)
	if !m.has("url.path") {
		m.out.Attributes.PutStr("url.path", u.EscapedPath())
	}
	if u.RawQuery != "" && !m.has("url.query") {
		m.out.Attributes.PutStr("url.query", query)
	}
}

func redactQuery(raw string, sensitive []string) (string, bool) {
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
func validScheme(s string) bool {
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
func validHeaderName(s string) bool {
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

func (m spanMigration) httpHost() {
	const key = "http.host"
	target := m.options.HTTPHostTarget
	if target != "server.address" && target != "client.address" && target != "http.request.header.host" {
		m.issue(key, MigrationMissingContext, "historical HTTP host usage is required")
		return
	}
	value, _ := m.attrs.Get(key)
	if value.Type() != pcommon.ValueTypeStr {
		m.issue(key, MigrationInvalidValue, "HTTP host must be a string")
		return
	}
	if target == "http.request.header.host" {
		m.out.Attributes.Remove(key)
		if !m.has(target) {
			m.out.Attributes.PutEmptySlice(target).AppendEmpty().SetStr(value.Str())
		}
		return
	}
	authority, err := url.Parse("//" + value.Str())
	if err != nil || authority.Hostname() == "" || authority.User != nil || authority.Path != "" || authority.RawQuery != "" || authority.Fragment != "" {
		m.issue(key, MigrationInvalidValue, "invalid HTTP host authority")
		return
	}
	var port int64
	if authority.Port() != "" {
		port, err = strconv.ParseInt(authority.Port(), 10, 64)
		if err != nil || port < 1 || port > 65535 {
			m.issue(key, MigrationInvalidValue, "invalid HTTP host port")
			return
		}
	}
	m.stringTarget(key, target, authority.Hostname())
	portKey := strings.TrimSuffix(target, "address") + "port"
	if port != 0 && !m.has(portKey) {
		m.out.Attributes.PutInt(portKey, port)
	}
}

func (m spanMigration) reportInvalidHTTPMethod() {
	if v, ok := m.attrs.Get("http.method"); ok && !m.has("http.request.method") && v.Type() != pcommon.ValueTypeStr {
		m.issue("http.method", MigrationInvalidValue, "HTTP method must be a string")
	}
}

func (m spanMigration) httpURL() {
	const key = "http.url"
	if m.has("url.full") {
		m.out.Attributes.Remove(key)
		return
	}
	value, _ := m.attrs.Get(key)
	// The static rename may have emitted a value already. Restore the source
	// before parsing so failures cannot expose a partially canonicalized URL.
	m.out.Attributes.Remove("url.full")
	value.CopyTo(m.out.Attributes.PutEmpty(key))
	if value.Type() != pcommon.ValueTypeStr {
		m.issue(key, MigrationInvalidValue, "HTTP URL must be a string")
		return
	}
	u, err := url.Parse(value.Str())
	if err != nil || !u.IsAbs() || u.Host == "" {
		m.issue(key, MigrationInvalidValue, "HTTP URL must be absolute")
		return
	}
	query, ok := redactQuery(u.RawQuery, m.options.SensitiveQueryParameters)
	if !ok {
		m.issue(key, MigrationInvalidValue, "invalid query escaping")
		return
	}
	u.User = nil
	u.RawQuery = query
	m.stringTarget(key, "url.full", u.String())
}
