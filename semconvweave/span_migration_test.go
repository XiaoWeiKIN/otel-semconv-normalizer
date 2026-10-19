// SPDX-License-Identifier: Apache-2.0
package semconvweave

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestCompleteSpanMigration(t *testing.T) {
	tests := []struct {
		name        string
		attrs, want map[string]any
		options     SpanMigrationOptions
		kind        ptrace.SpanKind
		status      ptrace.StatusCode
		message     string
	}{
		{name: "HTTP sizes and carrier", attrs: map[string]any{"http.request_content_length": int64(12), "http.response_content_length": int64(24), "net.host.connection.type": "wifi", "net.host.connection.subtype": "lte", "net.host.carrier.name": "carrier", "net.host.carrier.mcc": "460", "net.host.carrier.mnc": "00", "net.host.carrier.icc": "cn"}, want: map[string]any{"http.request.body.size": int64(12), "http.response.body.size": int64(24), "network.connection.type": "wifi", "network.connection.subtype": "lte", "network.carrier.name": "carrier", "network.carrier.mcc": "460", "network.carrier.mnc": "00", "network.carrier.icc": "cn"}},
		{name: "HTTP protocol version", attrs: map[string]any{"net.protocol.name": "http", "net.protocol.version": "2.0", "http.flavor": "1.1"}, want: map[string]any{"network.protocol.name": "http", "network.protocol.version": "2"}},
		{name: "non HTTP version preserved", attrs: map[string]any{"net.protocol.name": "amqp", "net.protocol.version": "2.0"}, want: map[string]any{"network.protocol.name": "amqp", "network.protocol.version": "2.0"}},
		{name: "messaging protocol chain", attrs: map[string]any{"messaging.protocol": "amqp", "messaging.protocol_version": "1.0"}, want: map[string]any{"network.protocol.name": "amqp", "network.protocol.version": "1.0"}},
		{name: "flavor", attrs: map[string]any{"http.flavor": "3.0"}, want: map[string]any{"network.protocol.version": "3"}},
		{name: "SPDY", attrs: map[string]any{"http.flavor": "SPDY"}, want: map[string]any{"network.protocol.name": "spdy"}},
		{name: "QUIC", attrs: map[string]any{"http.flavor": "QUIC"}, want: map[string]any{"network.protocol.name": "quic"}},
		{name: "socket server client", kind: ptrace.SpanKindServer, attrs: map[string]any{"http.method": "GET", "net.sock.peer.addr": "10.0.0.1", "net.sock.peer.port": int64(54321), "net.sock.host.addr": "10.0.0.2", "net.sock.host.port": int64(80)}, want: map[string]any{"http.request.method": "GET", "network.peer.address": "10.0.0.1", "network.peer.port": int64(54321), "network.local.address": "10.0.0.2", "network.local.port": int64(80), "client.address": "10.0.0.1"}},
		{name: "forwarded client wins", kind: ptrace.SpanKindServer, attrs: map[string]any{"http.method": "GET", "net.sock.peer.addr": "proxy", "http.client_ip": "client"}, want: map[string]any{"http.request.method": "GET", "network.peer.address": "proxy", "client.address": "client"}},
		{name: "old socket aliases", attrs: map[string]any{"net.host.ip": "old", "net.sock.host.addr": "middle", "server.socket.address": "new", "net.peer.ip": "oldpeer", "net.sock.peer.addr": "peer"}, want: map[string]any{"network.local.address": "new", "network.peer.address": "peer"}},
		{name: "URL split redaction", attrs: map[string]any{"http.target": "/a%2Fb?sig=secret&x=a%20b&sig=two&Signature=s&AWSAccessKeyId=k&X-Goog-Signature=g"}, want: map[string]any{"url.path": "/a%2Fb", "url.query": "sig=REDACTED&x=a%20b&sig=REDACTED&Signature=REDACTED&AWSAccessKeyId=REDACTED&X-Goog-Signature=REDACTED"}},
		{name: "URL absolute form", attrs: map[string]any{"http.target": "https://example.com/a?q=x"}, want: map[string]any{"url.path": "/a", "url.query": "q=x"}},
		{name: "URL targets win", attrs: map[string]any{"http.target": "/old?sig=x", "url.path": "/current", "url.query": "current"}, want: map[string]any{"url.path": "/current", "url.query": "current"}},
		{name: "URL override", options: SpanMigrationOptions{SensitiveQueryParameters: []string{"custom"}}, attrs: map[string]any{"http.target": "/a?sig=x&custom=y&Custom=z"}, want: map[string]any{"url.path": "/a", "url.query": "sig=x&custom=REDACTED&Custom=z"}},
		{name: "effective HTTP authority", kind: ptrace.SpanKindServer, options: SpanMigrationOptions{HTTPServer: &HTTPServerContext{Scheme: "https", Address: "public", Port: 443}}, attrs: map[string]any{"http.scheme": "http", "net.host.name": "internal", "net.host.port": int64(8080)}, want: map[string]any{"url.scheme": "https", "server.address": "public", "server.port": int64(443)}},
		{name: "header original spelling", options: SpanMigrationOptions{HeaderNames: map[string]string{"x_request_id": "X-Request-ID"}}, attrs: map[string]any{"http.request.header.x_request_id": []any{"id"}}, want: map[string]any{"http.request.header.x-request-id": []any{"id"}}},
		{name: "DB query collection and status", options: SpanMigrationOptions{SanitizeDBQuery: func(string) (string, bool) { return "SELECT * FROM Orders WHERE id = ?", true }, DBCollection: "Orders"}, attrs: map[string]any{"db.statement": "SELECT * FROM Orders WHERE id = 1", "db.sql.table": "orders", "db.cosmosdb.status_code": int64(200)}, want: map[string]any{"db.query.text": "SELECT * FROM Orders WHERE id = ?", "db.collection.name": "Orders", "db.response.status_code": "200"}},
		{name: "DB instance namespace", options: SpanMigrationOptions{DBInstanceNamespace: "instance|db"}, attrs: map[string]any{"db.instance.id": "instance", "db.name": "db"}, want: map[string]any{"db.namespace": "instance|db"}},
		{name: "DB instance server", options: SpanMigrationOptions{DBInstanceServer: "db.example"}, attrs: map[string]any{"db.instance.id": "instance"}, want: map[string]any{"server.address": "db.example"}},
		{name: "RPC registered method", options: SpanMigrationOptions{KnownRPCMethods: []string{"pkg.Service/Get"}}, attrs: map[string]any{"rpc.system": "grpc", "rpc.service": "pkg.Service", "rpc.method": "Get", "rpc.grpc.status_code": int64(0)}, want: map[string]any{"rpc.system.name": "grpc", "rpc.method": "pkg.Service/Get", "rpc.response.status_code": "OK"}},
		{name: "RPC unknown method", attrs: map[string]any{"rpc.system": "grpc", "rpc.service": "pkg.Service", "rpc.method": "Get", "rpc.grpc.status_code": int64(5)}, want: map[string]any{"rpc.system.name": "grpc", "rpc.method": "_OTHER", "rpc.method_original": "pkg.Service/Get", "rpc.response.status_code": "NOT_FOUND"}},
		{name: "RPC canonical unchanged", attrs: map[string]any{"rpc.system.name": "grpc", "rpc.method": "pkg.Service/Get"}, want: map[string]any{"rpc.system.name": "grpc", "rpc.method": "pkg.Service/Get"}},
		{name: "RPC configured endpoint", options: SpanMigrationOptions{RPCServer: &RPCServerContext{Address: "dns:///service", Port: 443}}, attrs: map[string]any{"rpc.system.name": "grpc", "net.peer.name": "backend", "net.peer.port": int64(9090)}, want: map[string]any{"rpc.system.name": "grpc", "server.address": "dns:///service", "server.port": int64(443)}},
		{name: "JSONRPC status", status: ptrace.StatusCodeError, message: "Invalid params", attrs: map[string]any{"rpc.system": "jsonrpc", "rpc.method": "subtract", "rpc.jsonrpc.error_code": int64(-32602), "rpc.jsonrpc.error_message": "Invalid params"}, want: map[string]any{"rpc.system.name": "jsonrpc", "rpc.method": "_OTHER", "rpc.method_original": "subtract", "rpc.response.status_code": "-32602"}},
		{name: "removed attributes", attrs: map[string]any{"net.sock.family": "inet", "net.sock.peer.name": "name", "db.user": "user", "db.connection_string": "secret", "db.jdbc.driver_classname": "driver", "db.cosmosdb.operation_type": "query"}, want: map[string]any{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, w := range []WeaveID{WeaveMissing, WeaveUnknown, WeaveCanonical, Resolve("https://opentelemetry.io/schemas/1.7.0")} {
				for _, reverse := range []bool{false, true} {
					span := ptrace.NewSpan()
					span.SetKind(test.kind)
					span.Status().SetCode(test.status)
					keys := make([]string, 0, len(test.attrs))
					for key := range test.attrs {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					for i := range keys {
						if reverse {
							i = len(keys) - 1 - i
						}
						if err := span.Attributes().PutEmpty(keys[i]).FromRaw(test.attrs[keys[i]]); err != nil {
							t.Fatal(err)
						}
					}
					before := span.Attributes().AsRaw()
					got := w.ProjectSpan(span, test.options)
					if !got.Complete() || !reflect.DeepEqual(got.Attributes.AsRaw(), test.want) || got.StatusDescription != test.message {
						t.Fatalf("weave %v reverse %v got %#v status %q issues %+v; want %#v", w, reverse, got.Attributes.AsRaw(), got.StatusDescription, got.Issues, test.want)
					}
					if !reflect.DeepEqual(span.Attributes().AsRaw(), before) || span.Status().Message() != "" {
						t.Fatal("source mutated")
					}
					got.Attributes.CopyTo(span.Attributes())
					span.Status().SetMessage(got.StatusDescription)
					again := w.ProjectSpan(span, test.options)
					if !again.Complete() || !reflect.DeepEqual(again.Attributes.AsRaw(), got.Attributes.AsRaw()) || again.StatusDescription != got.StatusDescription {
						t.Fatalf("not idempotent: %+v", again)
					}
				}
			}
		})
	}
}

func TestSpanMigrationFailureBoundaries(t *testing.T) {
	for _, test := range []struct {
		key   string
		value any
		code  MigrationIssueCode
	}{
		{"http.target", "/ok?sig=%zz", MigrationInvalidValue},
		{"http.target", "/ok#fragment", MigrationInvalidValue},
		{"http.target", int64(3), MigrationInvalidValue},
		{"http.scheme", "http", MigrationMissingContext},
		{"net.host.name", "host", MigrationMissingContext},
		{"net.host.port", int64(80), MigrationMissingContext},
		{"db.statement", "select secret", MigrationMissingContext},
		{"db.sql.table", "table", MigrationMissingContext},
		{"db.instance.id", "instance", MigrationMissingContext},
		{"rpc.grpc.status_code", int64(17), MigrationInvalidValue},
		{"rpc.grpc.status_code", "0", MigrationInvalidValue},
		{"rpc.jsonrpc.error_message", "failure", MigrationMissingContext},
		{"http.request.header.x_request_id", []any{"id"}, MigrationMissingContext},
	} {
		t.Run(test.key, func(t *testing.T) {
			span := ptrace.NewSpan()
			if err := span.Attributes().PutEmpty(test.key).FromRaw(test.value); err != nil {
				t.Fatal(err)
			}
			got := WeaveMissing.ProjectSpan(span, SpanMigrationOptions{})
			if got.Complete() || len(got.Issues) != 1 || got.Issues[0].Code != test.code || !reflect.DeepEqual(got.Attributes.AsRaw(), span.Attributes().AsRaw()) {
				t.Fatalf("failure lost source or emitted partial output: %+v %#v", got, got.Attributes.AsRaw())
			}
		})
	}
	t.Run("target prevents sanitizer effects", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.Attributes().PutStr("db.statement", "secret")
		span.Attributes().PutStr("db.query.text", "safe")
		got := WeaveMissing.ProjectSpan(span, SpanMigrationOptions{SanitizeDBQuery: func(string) (string, bool) { t.Fatal("unnecessary sanitizer call"); return "", false }})
		if !got.Complete() || stringAttribute(got.Attributes, "db.query.text") != "safe" {
			t.Fatal(got)
		}
	})
}

func TestGRPCStatusDomain(t *testing.T) {
	for code, name := range grpcStatusNames {
		span := ptrace.NewSpan()
		span.Attributes().PutInt("rpc.grpc.status_code", int64(code))
		got := WeaveMissing.ProjectSpan(span, SpanMigrationOptions{})
		if !got.Complete() || stringAttribute(got.Attributes, "rpc.response.status_code") != name {
			t.Fatal(got)
		}
	}
}

// Every non-static attribute obligation in the pinned guide inventories must
// have an explicit runtime handler or a documented no-transform disposition.
// New upstream obligations cannot silently disappear into generic pass-through.
func TestMigrationGuideAttributeCoverage(t *testing.T) {
	handled := map[string]bool{}
	for _, key := range []string{
		"http.host", "http.request_content_length", "http.response_content_length", "db.cosmosdb.operation_type", "db.cosmosdb.status_code", "db.jdbc.driver_classname", "rpc.jsonrpc.error_code", "rpc.jsonrpc.error_message", "message.compressed_size", "message.id", "message.type", "message.uncompressed_size", "rpc.message.compressed_size", "rpc.message.id", "rpc.message.type", "rpc.message.uncompressed_size",
		"http.method", "http.request.header.<key>", "http.response.header.<key>", "net.sock.family", "net.sock.peer.name", "net.peer.name", "net.peer.port", "http.target", "http.scheme", "net.host.name", "net.host.port", "http.flavor", "net.peer.ip", "net.host.ip", "net.sock.host.addr", "net.sock.host.port",
		"db.connection_string", "db.user", "db.name", "db.redis.database_index", "db.mssql.instance_name", "db.instance.id", "db.statement", "db.sql.table", "rpc.method", "rpc.service", "rpc.grpc.status_code",
		// Already-current keys or instrumentation requirements, not stored-key changes.
		"user_agent.original", "http.request.method_original", "error.type", "http.route", "network.transport", "network.type", "db.query.summary", "db.operation.batch.size", "db.response.status_code", "db.stored_procedure.name", "db.operation.parameter.<key>", "db.query.parameter.<key>", "db.response.returned_rows", "server.address", "server.port", "rpc.method_original", "client.address", "client.port",
	} {
		handled[key] = true
	}
	for _, domain := range []string{"http-url-network", "db-rpc"} {
		data, err := os.ReadFile("../tools/semconv-weave-gen/testdata/audit/" + domain + "-obligations.json")
		if err != nil {
			t.Fatal(err)
		}
		var inventory struct {
			Obligations []struct {
				ID        string `json:"id"`
				SourceKey string `json:"source_key"`
				Category  string `json:"category"`
			}
		}
		if err = json.Unmarshal(data, &inventory); err != nil {
			t.Fatal(err)
		}
		data, err = os.ReadFile("../tools/semconv-weave-gen/testdata/audit/" + domain + "-decisions.json")
		if err != nil {
			t.Fatal(err)
		}
		var decisions struct {
			WeaverDiff []struct {
				ID     string `json:"obligation_id"`
				Action string `json:"action"`
			} `json:"weaver_diff"`
			Decisions []struct {
				ID     string `json:"obligation_id"`
				Action string `json:"action"`
			}
		}
		if err = json.Unmarshal(data, &decisions); err != nil {
			t.Fatal(err)
		}
		static := map[string]bool{}
		for _, d := range decisions.WeaverDiff {
			static[d.ID] = d.Action == "translated"
		}
		for _, d := range decisions.Decisions {
			static[d.ID] = d.Action == "translated"
		}
		for _, item := range inventory.Obligations {
			if static[item.ID] || item.Category == "value" || item.Category == "event" || item.Category == "name" || item.SourceKey == "$span.name" {
				continue
			}
			if !handled[item.SourceKey] {
				t.Errorf("unclassified migration %s (%s)", item.ID, item.SourceKey)
			}
		}
	}
}

func TestMigrationAdditionalContexts(t *testing.T) {
	tests := []struct {
		name        string
		input, want map[string]any
		options     SpanMigrationOptions
		complete    bool
	}{
		{"host server", map[string]any{"http.host": "[2001:db8::1]:443"}, map[string]any{"server.address": "2001:db8::1", "server.port": int64(443)}, SpanMigrationOptions{HTTPHostTarget: "server.address"}, true},
		{"host client", map[string]any{"http.host": "example.com"}, map[string]any{"client.address": "example.com"}, SpanMigrationOptions{HTTPHostTarget: "client.address"}, true},
		{"host header", map[string]any{"http.host": "example.com:80"}, map[string]any{"http.request.header.host": []any{"example.com:80"}}, SpanMigrationOptions{HTTPHostTarget: "http.request.header.host"}, true},
		{"host ambiguous", map[string]any{"http.host": "example.com"}, map[string]any{"http.host": "example.com"}, SpanMigrationOptions{}, false},
		{"URL credentials", map[string]any{"http.url": "https://user:password@example.com/a%2Fb?sig=secret&x=a%20b#part"}, map[string]any{"url.full": "https://example.com/a%2Fb?sig=REDACTED&x=a%20b#part"}, SpanMigrationOptions{}, true},
		{"URL invalid", map[string]any{"http.url": "/relative"}, map[string]any{"http.url": "/relative"}, SpanMigrationOptions{}, false},
		{"invalid protocol type", map[string]any{"net.protocol.version": int64(2)}, map[string]any{"net.protocol.version": int64(2)}, SpanMigrationOptions{}, false},
		{"RPC mixed statuses", map[string]any{"rpc.grpc.status_code": int64(2), "rpc.connect_rpc.error_code": "internal"}, map[string]any{"rpc.grpc.status_code": int64(2), "rpc.connect_rpc.error_code": "internal"}, SpanMigrationOptions{}, false},
		{"header mapping mismatch", map[string]any{"http.request.header.x_id": []any{"id"}}, map[string]any{"http.request.header.x_id": []any{"id"}}, SpanMigrationOptions{HeaderNames: map[string]string{"x_id": "Other-Header"}}, false},
		{"sanitizer rejects", map[string]any{"db.statement": "select secret"}, map[string]any{"db.statement": "select secret"}, SpanMigrationOptions{SanitizeDBQuery: func(string) (string, bool) { return "partial", false }}, false},
		{"instance meanings conflict", map[string]any{"db.instance.id": "x"}, map[string]any{"db.instance.id": "x"}, SpanMigrationOptions{DBInstanceNamespace: "db", DBInstanceServer: "host"}, false},
		{"RPC service conflict", map[string]any{"rpc.system": "grpc", "rpc.service": "A", "rpc.method": "B/Get"}, map[string]any{"rpc.system.name": "grpc", "rpc.service": "A", "rpc.method": "B/Get"}, SpanMigrationOptions{}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			if err := span.Attributes().FromRaw(test.input); err != nil {
				t.Fatal(err)
			}
			got := WeaveMissing.ProjectSpan(span, test.options)
			if got.Complete() != test.complete || !reflect.DeepEqual(got.Attributes.AsRaw(), test.want) {
				t.Fatalf("got %#v issues %+v; want %#v", got.Attributes.AsRaw(), got.Issues, test.want)
			}
		})
	}
}

func TestRequirementRemovalDoesNotDeleteGeneralAttributes(t *testing.T) {
	span := ptrace.NewSpan()
	input := map[string]any{"rpc.system.name": "grpc", "client.address": "client", "client.port": int64(1234), "network.transport": "tcp", "network.type": "ipv4", "k8s.pod.name": "pod"}
	if err := span.Attributes().FromRaw(input); err != nil {
		t.Fatal(err)
	}
	got := WeaveCanonical.ProjectSpan(span, SpanMigrationOptions{})
	if !got.Complete() || !reflect.DeepEqual(got.Attributes.AsRaw(), input) {
		t.Fatalf("removed a valid general attribute: %#v", got.Attributes.AsRaw())
	}
	span.Attributes().PutInt("message.id", 1)
	span.Attributes().PutStr("rpc.message.type", "SENT")
	got = WeaveCanonical.ProjectSpan(span, SpanMigrationOptions{})
	if !got.Complete() || len(got.Issues) != 2 || !reflect.DeepEqual(got.Attributes.AsRaw(), input) {
		t.Fatalf("obsolete message attributes were not accounted for: %+v", got)
	}
}

func TestLegacyRPCSameKeyEndpoint(t *testing.T) {
	span := ptrace.NewSpan()
	span.Attributes().PutStr("rpc.system", "grpc")
	span.Attributes().PutStr("server.address", "replica")
	span.Attributes().PutInt("server.port", 8080)
	without := WeaveMissing.ProjectSpan(span, SpanMigrationOptions{})
	if without.Complete() || len(without.Issues) != 2 {
		t.Fatalf("legacy endpoint incorrectly claimed complete: %+v", without)
	}
	with := WeaveMissing.ProjectSpan(span, SpanMigrationOptions{RPCServer: &RPCServerContext{Address: "dns:///logical", Port: 443}})
	port, _ := with.Attributes.Get("server.port")
	if !with.Complete() || stringAttribute(with.Attributes, "server.address") != "dns:///logical" || port.Int() != 443 {
		t.Fatal(with)
	}
	span.Attributes().PutStr("rpc.system.name", "grpc")
	span.Attributes().PutStr("rpc.service", "Service")
	span.Attributes().PutStr("rpc.method", "Service/Get")
	dual := WeaveMissing.ProjectSpan(span, SpanMigrationOptions{})
	if !dual.Complete() || stringAttribute(dual.Attributes, "server.address") != "replica" || stringAttribute(dual.Attributes, "rpc.method") != "Service/Get" {
		t.Fatal(dual)
	}
}
