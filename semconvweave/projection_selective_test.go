// SPDX-License-Identifier: Apache-2.0
package semconvweave

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func selectiveFixtures() []projectionFixture {
	fixtures := projectionFixtures()
	for _, n := range []int{32, 64, 128} {
		for _, changes := range []int{1, 2} {
			span := ptrace.NewSpan()
			span.SetKind(ptrace.SpanKindClient)
			for i := 0; i < n-changes; i++ {
				span.Attributes().PutStr(fmt.Sprintf("custom.attr.%03d", i), "value")
			}
			span.Attributes().PutStr("http.method", "GET")
			if changes == 2 {
				span.Attributes().PutStr("db.operation", "SELECT")
			}
			fixtures = append(fixtures, projectionFixture{name: fmt.Sprintf("Sparse%dOf%d", changes, n), span: span})
		}
		span := ptrace.NewSpan()
		span.SetKind(ptrace.SpanKindClient)
		for i := 0; i < n; i++ {
			span.Attributes().PutEmptySlice(fmt.Sprintf("http.request.header.x_%03d", i)).AppendEmpty().SetStr("value")
		}
		fixtures = append(fixtures, projectionFixture{name: fmt.Sprintf("Dense%d", n), span: span})
	}
	for _, protocol := range []string{"DB", "RPC"} {
		span := ptrace.NewSpan()
		a := span.Attributes()
		if protocol == "DB" {
			a.PutStr("db.system.name", "postgresql")
			a.PutStr("db.namespace", "orders")
			a.PutStr("db.operation.name", "SELECT")
		} else {
			a.PutStr("rpc.system.name", "grpc")
			a.PutStr("rpc.method", "Service/Get")
			a.PutStr("rpc.response.status_code", "OK")
		}
		a.PutStr("server.address", "example.com")
		a.PutInt("server.port", 443)
		a.PutStr("network.peer.address", "10.0.0.1")
		a.PutInt("network.peer.port", 443)
		a.PutStr("network.transport", "tcp")
		fixtures = append(fixtures, projectionFixture{name: "Canonical" + protocol + "8", span: span})
	}
	return fixtures
}

func assertSelectiveEquivalent(t testing.TB, span ptrace.Span, opts SpanMigrationOptions, weave WeaveID) {
	t.Helper()
	assertProjectionEquivalent(t, span, opts, weave)
	prior, candidate := weave.priorProjectSpan(span, opts), weave.ProjectSpan(span, opts)
	if !reflect.DeepEqual(prior.Attributes.AsRaw(), candidate.Attributes.AsRaw()) || !reflect.DeepEqual(prior.Issues, candidate.Issues) || prior.StatusDescription != candidate.StatusDescription || prior.Complete() != candidate.Complete() {
		t.Fatalf("BR-001 control mismatch: input=%v prior=%v candidate=%v issues=%v / %v", span.Attributes().AsRaw(), prior.Attributes.AsRaw(), candidate.Attributes.AsRaw(), prior.Issues, candidate.Issues)
	}
}

func TestSelectiveProjectionDifferential(t *testing.T) {
	weaves := []WeaveID{WeaveMissing, WeaveUnknown, WeaveCanonical, Resolve("https://opentelemetry.io/schemas/1.4.0"), Resolve("https://opentelemetry.io/schemas/1.7.0")}
	for _, fixture := range selectiveFixtures() {
		for _, weave := range weaves {
			assertSelectiveEquivalent(t, fixture.span, fixture.options, weave)
		}
	}
	// Every installed source, its canonical target, and every contextual source
	// participates. Padding crosses sparse admission boundaries; random insertion
	// order and current/legacy collisions test conflict handling.
	keys := map[string]bool{}
	for _, rule := range activeGeneratedData.fallbackKeyRules {
		if rule.domain != generatedDomainSpan {
			continue
		}
		source, target := rule.source, rule.target
		if rule.match == generatedMatchPrefix {
			source += "x_id"
			target += "x_id"
		}
		keys[source], keys[target] = true, true
	}
	for _, key := range []string{"http.method", "http.target", "http.host", "http.url", "http.flavor", "http.scheme", "net.host.name", "net.host.port", "net.peer.name", "net.peer.port", "db.statement", "db.sql.table", "db.instance.id", "db.name", "db.redis.database_index", "db.mssql.instance_name", "db.cassandra.keyspace", "db.hbase.namespace", "db.cosmosdb.status_code", "rpc.service", "rpc.system", "rpc.system.name", "rpc.grpc.status_code", "rpc.jsonrpc.error_code", "rpc.jsonrpc.error_message", "rpc.connect_rpc.error_code", "server.socket.domain", "message.id", "rpc.message.id"} {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	values := []any{"value", "", "GET", "PROPFIND", "grpc", "jsonrpc", "mssql", "redis", "2.0", "SPDY", "Service/Get", "https://example.com/a?sig=secret", int64(0), int64(-1), int64(200), false, []any{"a"}, map[string]any{"nested": "value"}, []byte{1, 2}}
	r := rand.New(rand.NewSource(20260917))
	for i := 0; i < 6000; i++ {
		span := ptrace.NewSpan()
		span.SetKind(ptrace.SpanKind(i % 6))
		span.Status().SetCode(ptrace.StatusCode(i % 3))
		attrs := span.Attributes()
		count := 1 + r.Intn(4)
		selected := []string{ordered[i%len(ordered)]}
		for j := 1; j < count; j++ {
			selected = append(selected, ordered[r.Intn(len(ordered))])
		}
		padding := []int{0, 29, 30, 31, 32, 63, 127}[i%7]
		for j := 0; j < padding; j++ {
			selected = append(selected, fmt.Sprintf("custom.%03d", j))
		}
		r.Shuffle(len(selected), func(i, j int) { selected[i], selected[j] = selected[j], selected[i] })
		for _, key := range selected {
			if err := attrs.PutEmpty(key).FromRaw(values[r.Intn(len(values))]); err != nil {
				t.Fatal(err)
			}
		}
		opts := SpanMigrationOptions{}
		if i%2 == 0 {
			opts = SpanMigrationOptions{KnownHTTPMethods: []string{"GET", "PROPFIND"}, KnownRPCMethods: []string{"Service/Get"}, HTTPServer: &HTTPServerContext{Scheme: "https", Address: "public", Port: 443}, RPCServer: &RPCServerContext{Address: "dns:///service", Port: 443}, HeaderNames: map[string]string{"x_id": "X-ID"}, HTTPHostTarget: "server.address", DBCollection: "orders", DBInstanceNamespace: "orders", SanitizeDBQuery: func(string) (string, bool) { return "SELECT ?", true }}
		}
		assertSelectiveEquivalent(t, span, opts, weaves[i%len(weaves)])
	}
}

func TestSparseProjectionOwnership(t *testing.T) {
	for _, key := range []string{"db.operation", "http.method", "custom.value"} {
		for _, raw := range []any{map[string]any{"nested": "original"}, []any{"original"}, []byte{1, 2}} {
			span := ptrace.NewSpan()
			for i := 0; i < 32; i++ {
				span.Attributes().PutStr(fmt.Sprintf("custom.%02d", i), "value")
			}
			if err := span.Attributes().PutEmpty(key).FromRaw(raw); err != nil {
				t.Fatal(err)
			}
			before := span.Attributes().AsRaw()
			got := WeaveMissing.ProjectSpan(span, SpanMigrationOptions{})
			got.Attributes.Range(func(_ string, v pcommon.Value) bool {
				switch v.Type() {
				case pcommon.ValueTypeMap:
					v.Map().PutStr("nested", "changed")
				case pcommon.ValueTypeSlice:
					v.Slice().At(0).SetStr("changed")
				case pcommon.ValueTypeBytes:
					v.Bytes().AsRaw()[0] = 9
				}
				return true
			})
			if !reflect.DeepEqual(before, span.Attributes().AsRaw()) {
				t.Fatal("sparse output aliases input")
			}
		}
	}
}

func BenchmarkSelectiveProjectionPrior(b *testing.B) {
	benchmarkSelectiveProjection(b, WeaveMissing.priorProjectSpan)
}
func BenchmarkSelectiveProjectionCandidate(b *testing.B) {
	benchmarkSelectiveProjection(b, WeaveMissing.ProjectSpan)
}
func benchmarkSelectiveProjection(b *testing.B, project func(ptrace.Span, SpanMigrationOptions) SpanProjection) {
	fixtures := selectiveFixtures()
	for _, fixture := range fixtures {
		b.Run(fixture.name, func(b *testing.B) {
			assertSelectiveEquivalent(b, fixture.span, fixture.options, WeaveMissing)
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

func TestSparseProjectionConflictsAndExpansion(t *testing.T) {
	cases := []map[string]any{
		{"http.method": "PROPFIND", "db.operation": "SELECT"},
		{"http.method": "PROPFIND", "http.request.method_original": "existing"},
		{"http.method": "GET", "http.request.method": int64(7)},
		{"db.name": "orders", "db.elasticsearch.cluster.name": "cluster"},
		{"db.statement": "SELECT secret", "db.query.text": "SELECT ?"},
		{"net.protocol.version": "2.0", "http.flavor": "3.0"},
		{"net.protocol.version": "2.0", "network.protocol.version": int64(3)},
		{"rpc.system": "grpc", "rpc.system.name": int64(3), "net.peer.name": "peer"},
		{"net.peer.name": "peer", "http.server_name": "server"},
		{"http.request.header.x_id": []any{"old"}, "http.request.header.x-id": []any{"current"}},
	}
	for _, input := range cases {
		for _, n := range []int{31, 32, 33, 64, 128} {
			for _, reverse := range []bool{false, true} {
				keys := make([]string, 0, len(input))
				for key := range input {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				if reverse {
					sort.Sort(sort.Reverse(sort.StringSlice(keys)))
				}
				span := ptrace.NewSpan()
				span.SetKind(ptrace.SpanKindClient)
				for i := 0; i < n-len(input); i++ {
					span.Attributes().PutStr(fmt.Sprintf("custom.%03d", i), "value")
				}
				for _, key := range keys {
					if err := span.Attributes().PutEmpty(key).FromRaw(input[key]); err != nil {
						t.Fatal(err)
					}
				}
				for _, w := range []WeaveID{WeaveMissing, WeaveCanonical, Resolve("https://opentelemetry.io/schemas/1.4.0")} {
					assertSelectiveEquivalent(t, span, SpanMigrationOptions{HeaderNames: map[string]string{"x_id": "X-ID"}}, w)
				}
			}
		}
	}
}
