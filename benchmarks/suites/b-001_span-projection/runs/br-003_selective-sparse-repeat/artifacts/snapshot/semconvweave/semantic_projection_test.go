// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"reflect"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func projectSemanticSpan(t *testing.T, weave WeaveID, context SpanContext, input map[string]any, reverse bool) map[string]any {
	t.Helper()
	attrs := pcommon.NewMap()
	if err := attrs.FromRaw(input); err != nil {
		t.Fatal(err)
	}
	before := attrs.AsRaw()
	keys := make([]string, 0, len(input))
	attrs.Range(func(key string, _ pcommon.Value) bool { keys = append(keys, key); return true })
	output := pcommon.NewMap()
	for index := range keys {
		if reverse {
			index = len(keys) - 1 - index
		}
		key := keys[index]
		value, _ := attrs.Get(key)
		for _, decision := range weave.SpanAttributeWithContext(attrs, context, key, value) {
			if decision.Action == AttributeSuppress {
				continue
			}
			if _, duplicate := output.Get(decision.Key); duplicate {
				t.Fatalf("duplicate output %q", decision.Key)
			}
			switch decision.Action {
			case AttributeKeepValue:
				value.CopyTo(output.PutEmpty(decision.Key))
			case AttributeReplaceString:
				output.PutStr(decision.Key, decision.StringValue)
			default:
				t.Fatalf("invalid action %d", decision.Action)
			}
		}
	}
	if !reflect.DeepEqual(before, attrs.AsRaw()) {
		t.Fatal("projection mutated input")
	}
	return output.AsRaw()
}

func TestSemanticSpanProjection(t *testing.T) {
	tests := []struct {
		name        string
		context     SpanContext
		input, want map[string]any
	}{
		{name: "HTTP standard", input: map[string]any{"http.method": "GET"}, want: map[string]any{"http.request.method": "GET"}},
		{name: "HTTP QUERY", input: map[string]any{"http.method": "QUERY"}, want: map[string]any{"http.request.method": "QUERY"}},
		{name: "HTTP custom", input: map[string]any{"http.method": "PROPFIND"}, want: map[string]any{"http.request.method": "_OTHER", "http.request.method_original": "PROPFIND"}},
		{name: "HTTP case sensitive", input: map[string]any{"http.method": "GeT"}, want: map[string]any{"http.request.method": "_OTHER", "http.request.method_original": "GeT"}},
		{name: "HTTP configured methods", context: SpanContext{KnownHTTPMethods: []string{"PROPFIND"}}, input: map[string]any{"http.method": "PROPFIND"}, want: map[string]any{"http.request.method": "PROPFIND"}},
		{name: "HTTP override replaces defaults", context: SpanContext{KnownHTTPMethods: []string{"PROPFIND"}}, input: map[string]any{"http.method": "GET"}, want: map[string]any{"http.request.method": "_OTHER", "http.request.method_original": "GET"}},
		{name: "HTTP empty override", context: SpanContext{KnownHTTPMethods: []string{}}, input: map[string]any{"http.method": "GET"}, want: map[string]any{"http.request.method": "_OTHER", "http.request.method_original": "GET"}},
		{name: "HTTP current wins as pair", input: map[string]any{"http.method": "BREW", "http.request.method": "POST"}, want: map[string]any{"http.request.method": "POST"}},
		{name: "HTTP original wins", input: map[string]any{"http.method": "BREW", "http.request.method_original": "CUSTOM"}, want: map[string]any{"http.request.method": "_OTHER", "http.request.method_original": "CUSTOM"}},
		{name: "HTTP current configured preserved", input: map[string]any{"http.request.method": "PROPFIND"}, want: map[string]any{"http.request.method": "PROPFIND"}},
		{name: "HTTP wrong type", input: map[string]any{"http.method": int64(42)}, want: map[string]any{"http.method": int64(42)}},
		{name: "database name", input: map[string]any{"db.name": "MiXeD"}, want: map[string]any{"db.namespace": "MiXeD"}},
		{name: "database operation", input: map[string]any{"db.operation": "findAndModify"}, want: map[string]any{"db.operation.name": "findAndModify"}},
		{name: "MongoDB collection", input: map[string]any{"db.mongodb.collection": "Order"}, want: map[string]any{"db.collection.name": "Order"}},
		{name: "Cassandra collection", input: map[string]any{"db.cassandra.table": "Order"}, want: map[string]any{"db.collection.name": "Order"}},
		{name: "CosmosDB collection", input: map[string]any{"db.cosmosdb.container": "Order"}, want: map[string]any{"db.collection.name": "Order"}},
		{name: "database current wins", input: map[string]any{"db.name": "old", "db.namespace": "new"}, want: map[string]any{"db.namespace": "new"}},
		{name: "database empty current wins", input: map[string]any{"db.name": "old", "db.namespace": ""}, want: map[string]any{"db.namespace": ""}},
		{name: "database generated alias conflict", input: map[string]any{"db.name": "older", "db.elasticsearch.cluster.name": "newer"}, want: map[string]any{"db.namespace": "newer"}},
		{name: "Cassandra namespace", input: map[string]any{"db.system.name": "cassandra", "db.cassandra.keyspace": "Keyspace"}, want: map[string]any{"db.system.name": "cassandra", "db.namespace": "Keyspace"}},
		{name: "HBase namespace", input: map[string]any{"db.system.name": "hbase", "db.hbase.namespace": "Namespace"}, want: map[string]any{"db.system.name": "hbase", "db.namespace": "Namespace"}},
		{name: "collection current wins", input: map[string]any{"db.mongodb.collection": "old", "db.collection.name": "current"}, want: map[string]any{"db.collection.name": "current"}},
		{name: "operation current wins", input: map[string]any{"db.operation": "old", "db.operation.name": "current"}, want: map[string]any{"db.operation.name": "current"}},
		{name: "SQL Server instance", input: map[string]any{"db.system.name": "microsoft.sql_server", "db.name": "Products", "db.mssql.instance_name": "Instance"}, want: map[string]any{"db.system.name": "microsoft.sql_server", "db.namespace": "Instance|Products"}},
		{name: "SQL Server default", input: map[string]any{"db.system.name": "microsoft.sql_server", "db.name": "Products"}, want: map[string]any{"db.system.name": "microsoft.sql_server", "db.namespace": "Products"}},
		{name: "Redis zero index", input: map[string]any{"db.system.name": "redis", "db.redis.database_index": int64(0)}, want: map[string]any{"db.system.name": "redis", "db.namespace": "0"}},
		{name: "Redis index not inferred", input: map[string]any{"db.system.name": "redis", "db.name": "cache"}, want: map[string]any{"db.system.name": "redis", "db.name": "cache"}},
		{name: "Redis negative index", input: map[string]any{"db.system.name": "redis", "db.redis.database_index": int64(-1)}, want: map[string]any{"db.system.name": "redis", "db.redis.database_index": int64(-1)}},
		{name: "unknown namespace context", input: map[string]any{"db.mssql.instance_name": "Instance"}, want: map[string]any{"db.mssql.instance_name": "Instance"}},
		{name: "Kubernetes domain unchanged", input: map[string]any{"k8s.pod.name": "pod"}, want: map[string]any{"k8s.pod.name": "pod"}},
		{name: "unsanitized statement unchanged", input: map[string]any{"db.statement": "select secret"}, want: map[string]any{"db.statement": "select secret"}},
	}
	for _, weave := range []WeaveID{WeaveMissing, WeaveUnknown, WeaveCanonical, Resolve("https://opentelemetry.io/schemas/1.7.0"), Resolve("https://opentelemetry.io/schemas/1.4.0")} {
		for _, test := range tests {
			t.Run(weave.State().String()+"/"+test.name, func(t *testing.T) {
				for _, reverse := range []bool{false, true} {
					got := projectSemanticSpan(t, weave, test.context, test.input, reverse)
					if !reflect.DeepEqual(got, test.want) {
						t.Fatalf("projection = %#v, want %#v", got, test.want)
					}
					if again := projectSemanticSpan(t, weave, test.context, got, reverse); !reflect.DeepEqual(again, got) {
						t.Fatalf("projection is not idempotent: %#v -> %#v", got, again)
					}
				}
			})
		}
	}
}

func TestContextualNetworkProjection(t *testing.T) {
	for _, kind := range []ptrace.SpanKind{ptrace.SpanKindUnspecified, ptrace.SpanKindClient, ptrace.SpanKindServer, ptrace.SpanKindProducer, ptrace.SpanKindConsumer, ptrace.SpanKindInternal} {
		input := map[string]any{"http.request.method": "GET", "net.peer.name": "origin", "net.peer.port": int64(443), "network.peer.address": "10.0.0.1"}
		want := input
		if kind == ptrace.SpanKindClient {
			want = map[string]any{"http.request.method": "GET", "server.address": "origin", "server.port": int64(443), "network.peer.address": "10.0.0.1"}
		}
		if got := projectSemanticSpan(t, WeaveMissing, SpanContext{Kind: kind}, input, false); !reflect.DeepEqual(got, want) {
			t.Fatalf("kind %s = %#v, want %#v", kind, got, want)
		}
	}
	for _, input := range []map[string]any{
		{"net.peer.name": "unknown", "net.peer.port": int64(80)},
		{"rpc.system.name": "grpc", "net.peer.name": "unknown", "net.peer.port": int64(80)},
		{"http.request.method": "GET", "net.peer.name": int64(42), "net.peer.port": "443"},
	} {
		if got := projectSemanticSpan(t, WeaveMissing, SpanContext{Kind: ptrace.SpanKindClient}, input, false); !reflect.DeepEqual(got, input) {
			t.Fatalf("ambiguous/invalid input = %#v", got)
		}
	}
	input := map[string]any{"http.request.method": "GET", "http.server_name": "virtual", "net.peer.name": "peer", "net.peer.port": int64(80), "server.port": int64(443)}
	want := map[string]any{"http.request.method": "GET", "server.address": "peer", "server.port": int64(443)}
	for _, reverse := range []bool{false, true} {
		if got := projectSemanticSpan(t, WeaveMissing, SpanContext{Kind: ptrace.SpanKindClient}, input, reverse); !reflect.DeepEqual(got, want) {
			t.Fatalf("network target conflict = %#v, want %#v", got, want)
		}
	}
}

var semanticDecisionsSink [2]AttributeDecision

func TestSingleOutputHTTPCompatibility(t *testing.T) {
	attrs := pcommon.NewMap()
	for _, method := range []string{"GET", "PROPFIND"} {
		attrs.PutStr("http.method", method)
		value, _ := attrs.Get("http.method")
		got := WeaveMissing.SpanAttribute(attrs, "", "http.method", value)
		key := "http.method"
		if method == "GET" {
			key = "http.request.method"
		}
		if got != (AttributeDecision{Key: key, Action: AttributeKeepValue}) || !MayTranslateSpanAttribute("http.method") {
			t.Fatalf("single output %s = %+v", method, got)
		}
	}
}

func TestSemanticProjectionDoesNotAllocate(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "PROPFIND")
	value, _ := attrs.Get("http.method")
	if got := testing.AllocsPerRun(100, func() {
		semanticDecisionsSink = WeaveMissing.SpanAttributeWithContext(attrs, SpanContext{}, "http.method", value)
	}); got != 0 {
		t.Fatalf("HTTP projection allocations = %v", got)
	}
}

func BenchmarkSemanticSpanProjection(b *testing.B) {
	for _, item := range []struct{ key, value string }{
		{"vendor.custom", "value"}, {"http.method", "GET"}, {"http.method", "PROPFIND"}, {"db.name", "Orders"},
	} {
		b.Run(item.key+"/"+item.value, func(b *testing.B) {
			attrs := pcommon.NewMap()
			attrs.PutStr(item.key, item.value)
			value, _ := attrs.Get(item.key)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				semanticDecisionsSink = WeaveMissing.SpanAttributeWithContext(attrs, SpanContext{}, item.key, value)
			}
		})
	}
}
