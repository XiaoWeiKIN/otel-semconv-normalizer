// SPDX-License-Identifier: Apache-2.0
package semconvweave

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

type projectionFixture struct {
	name    string
	span    ptrace.Span
	options SpanMigrationOptions
}

func projectionFixtures() []projectionFixture {
	var fixtures []projectionFixture
	for _, n := range []int{8, 32, 64} {
		span := ptrace.NewSpan()
		span.SetKind(ptrace.SpanKindClient)
		for i := 0; i < n; i++ {
			span.Attributes().PutStr(fmt.Sprintf("custom.attr.%02d", i), "value")
		}
		fixtures = append(fixtures, projectionFixture{name: fmt.Sprintf("Unchanged%d", n), span: span})
	}
	canonical := ptrace.NewSpan()
	canonical.SetKind(ptrace.SpanKindClient)
	for _, pair := range [][2]string{{"http.request.method", "GET"}, {"url.full", "https://example.com/a"}, {"server.address", "example.com"}, {"network.protocol.version", "2"}, {"user_agent.original", "agent"}, {"service.name", "frontend"}, {"http.route", "/a"}, {"network.transport", "tcp"}} {
		canonical.Attributes().PutStr(pair[0], pair[1])
	}
	fixtures = append(fixtures, projectionFixture{name: "CanonicalHTTP8", span: canonical})
	for _, protocol := range []string{"HTTP", "DB", "RPC", "MissingContext", "Invalid"} {
		span := ptrace.NewSpan()
		span.SetKind(ptrace.SpanKindClient)
		a := span.Attributes()
		opts := SpanMigrationOptions{}
		switch protocol {
		case "HTTP":
			a.PutStr("http.method", "GET")
			a.PutStr("http.url", "https://example.com/a?sig=secret&page=1")
			a.PutInt("http.status_code", 200)
			a.PutStr("net.peer.name", "example.com")
			a.PutInt("net.peer.port", 443)
			a.PutStr("net.protocol.name", "http")
			a.PutStr("net.protocol.version", "2.0")
			a.PutStr("net.sock.peer.addr", "10.0.0.1")
		case "DB":
			a.PutStr("db.system", "mssql")
			a.PutStr("db.name", "orders")
			a.PutStr("db.mssql.instance_name", "primary")
			a.PutStr("db.operation", "SELECT")
			a.PutStr("net.peer.name", "db.example")
			a.PutInt("net.peer.port", 1433)
			a.PutStr("db.query.text", "SELECT * FROM orders WHERE id=?")
			a.PutStr("db.collection.name", "orders")
		case "RPC":
			a.PutStr("rpc.system", "grpc")
			a.PutStr("rpc.service", "example.Service")
			a.PutStr("rpc.method", "Get")
			a.PutInt("rpc.grpc.status_code", 0)
			a.PutStr("net.peer.name", "replica")
			a.PutInt("net.peer.port", 8080)
			a.PutStr("network.peer.address", "10.0.0.1")
			a.PutStr("network.transport", "tcp")
			opts.KnownRPCMethods = []string{"example.Service/Get"}
			opts.RPCServer = &RPCServerContext{Address: "dns:///service", Port: 443}
		case "MissingContext":
			a.PutStr("db.statement", "SELECT secret")
			a.PutStr("db.sql.table", "orders")
			a.PutStr("http.scheme", "http")
			a.PutStr("net.host.name", "local")
		case "Invalid":
			a.PutInt("http.method", 3)
			a.PutStr("http.target", "/a?sig=%zz")
			a.PutInt("rpc.grpc.status_code", 17)
			a.PutStr("db.cosmosdb.status_code", "bad")
		}
		fixtures = append(fixtures, projectionFixture{name: protocol, span: span, options: opts})
	}
	return fixtures
}

func assertProjectionEquivalent(t testing.TB, span ptrace.Span, options SpanMigrationOptions, w WeaveID) {
	t.Helper()
	before := span.Attributes().AsRaw()
	baseline := w.baselineProjectSpan(span, options)
	candidate := w.ProjectSpan(span, options)
	if !reflect.DeepEqual(baseline.Attributes.AsRaw(), candidate.Attributes.AsRaw()) || baseline.StatusDescription != candidate.StatusDescription || !reflect.DeepEqual(baseline.Issues, candidate.Issues) || baseline.Complete() != candidate.Complete() {
		t.Fatalf("projection mismatch weave=%d kind=%s input=%#v\nbaseline attrs=%#v issues=%+v status=%q\ncandidate attrs=%#v issues=%+v status=%q", w, span.Kind(), before, baseline.Attributes.AsRaw(), baseline.Issues, baseline.StatusDescription, candidate.Attributes.AsRaw(), candidate.Issues, candidate.StatusDescription)
	}
	if !reflect.DeepEqual(before, span.Attributes().AsRaw()) {
		t.Fatal("source mutated")
	}
}

func TestProjectionOptimizationDifferential(t *testing.T) {
	for _, fixture := range projectionFixtures() {
		t.Run(fixture.name, func(t *testing.T) { assertProjectionEquivalent(t, fixture.span, fixture.options, WeaveMissing) })
	}
	// Include every static source, plus contextual, current, malformed, prefix,
	// deleted and unknown keys. Fixed seed makes failures reproducible.
	keys := []string{"http.method", "http.request.method", "http.target", "http.url", "http.host", "http.scheme", "http.flavor", "http.request.header.x_id", "http.request.header.x-id", "db.name", "db.statement", "db.sql.table", "db.instance.id", "db.redis.database_index", "db.mssql.instance_name", "db.cassandra.keyspace", "db.hbase.namespace", "db.cosmosdb.status_code", "db.namespace", "db.system", "db.system.name", "rpc.service", "rpc.method", "rpc.method_original", "rpc.system", "rpc.system.name", "rpc.grpc.status_code", "rpc.jsonrpc.error_code", "rpc.jsonrpc.error_message", "server.address", "server.port", "net.peer.name", "net.peer.port", "server.socket.address", "server.socket.domain", "network.protocol.version", "network.peer.address", "client.address", "message.id", "rpc.message.type", "rpc.grpc.request.metadata.x-key", "url.path", "url.query", "url.full", "vendor.custom", "", "h", "http."}
	for _, rule := range activeGeneratedData.fallbackKeyRules {
		if rule.domain == generatedDomainSpan {
			key := rule.source
			if rule.match == generatedMatchPrefix {
				key += "x-key"
			}
			keys = append(keys, key)
		}
	}
	values := []any{"", "value", "GET", "PROPFIND", "2.0", "3.0", "SPDY", "QUIC", "mssql", "redis", "cassandra", "grpc", "jsonrpc", "/a?sig=secret", "/a?sig=%zz", "Service/Get", "_OTHER", int64(0), int64(5), int64(-1), int64(443), true, []any{"x"}, map[string]any{"nested": "value"}}
	r := rand.New(rand.NewSource(20260916))
	weaves := []WeaveID{WeaveMissing, WeaveUnknown, WeaveCanonical, Resolve("https://opentelemetry.io/schemas/1.4.0"), Resolve("https://opentelemetry.io/schemas/1.7.0")}
	for i := 0; i < 3000; i++ {
		span := ptrace.NewSpan()
		span.SetKind(ptrace.SpanKind(i % 6))
		span.Status().SetCode(ptrace.StatusCode(i % 3))
		if i%7 == 0 {
			span.Status().SetMessage("existing")
		}
		for j, n := 0, r.Intn(32)+1; j < n; j++ {
			key := keys[r.Intn(len(keys))]
			if err := span.Attributes().PutEmpty(key).FromRaw(values[r.Intn(len(values))]); err != nil {
				t.Fatal(err)
			}
		}
		opts := SpanMigrationOptions{}
		if i%2 == 0 {
			opts = SpanMigrationOptions{KnownHTTPMethods: []string{"GET", "PROPFIND"}, KnownRPCMethods: []string{"Service/Get"}, HTTPServer: &HTTPServerContext{Scheme: "https", Address: "public", Port: 443}, RPCServer: &RPCServerContext{Address: "dns:///service", Port: 443}, DBCollection: "orders", DBInstanceNamespace: "instance|db", HeaderNames: map[string]string{"x_id": "X-ID"}, HTTPHostTarget: "server.address", SanitizeDBQuery: func(string) (string, bool) { return "SELECT ?", true }}
		}
		assertProjectionEquivalent(t, span, opts, weaves[i%len(weaves)])
	}
}

func TestProjectionOptimizationOwnership(t *testing.T) {
	span := ptrace.NewSpan()
	span.Attributes().PutEmptyMap("custom.map").PutStr("nested", "original")
	span.Attributes().PutEmptySlice("custom.slice").AppendEmpty().SetStr("original")
	span.Attributes().PutEmptyBytes("custom.bytes").FromRaw([]byte{1, 2})
	got := WeaveCanonical.ProjectSpan(span, SpanMigrationOptions{})
	value, _ := got.Attributes.Get("custom.map")
	value.Map().PutStr("nested", "changed")
	value, _ = got.Attributes.Get("custom.slice")
	value.Slice().At(0).SetStr("changed")
	value, _ = got.Attributes.Get("custom.bytes")
	value.Bytes().AsRaw()[0] = 9
	original, _ := span.Attributes().Get("custom.map")
	if stringAttribute(original.Map(), "nested") != "original" {
		t.Fatal("map aliases source")
	}
	original, _ = span.Attributes().Get("custom.slice")
	if original.Slice().At(0).Str() != "original" {
		t.Fatal("slice aliases source")
	}
	original, _ = span.Attributes().Get("custom.bytes")
	if original.Bytes().AsRaw()[0] != 1 {
		t.Fatal("bytes alias source")
	}
}

var projectionBenchmarkSink SpanProjection

func BenchmarkProjectSpanBaseline(b *testing.B) {
	benchmarkProjectSpan(b, WeaveMissing.baselineProjectSpan)
}
func BenchmarkProjectSpanCandidate(b *testing.B) { benchmarkProjectSpan(b, WeaveMissing.ProjectSpan) }
func benchmarkProjectSpan(b *testing.B, project func(ptrace.Span, SpanMigrationOptions) SpanProjection) {
	fixtures := projectionFixtures()
	for _, fixture := range fixtures {
		b.Run(fixture.name, func(b *testing.B) {
			assertProjectionEquivalent(b, fixture.span, fixture.options, WeaveMissing)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				projectionBenchmarkSink = project(fixture.span, fixture.options)
			}
		})
	}
	b.Run("Mixed", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for _, fixture := range fixtures {
				projectionBenchmarkSink = project(fixture.span, fixture.options)
			}
		}
	})
}
