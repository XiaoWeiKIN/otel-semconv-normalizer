// SPDX-License-Identifier: Apache-2.0

package semconvweave_test

import (
	"fmt"
	"reflect"
	"testing"

	otlpmap "github.com/XiaoWeiKIN/otel-semconv-normalizer"
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/semconvweave"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func emitDecision(
	dst pcommon.Map,
	sourceValue pcommon.Value,
	decision semconvweave.AttributeDecision,
) {
	switch decision.Action {
	case semconvweave.AttributeSuppress:
		return
	case semconvweave.AttributeReplaceString:
		dst.PutStr(decision.Key, decision.StringValue)
	case semconvweave.AttributeKeepValue:
		sourceValue.CopyTo(dst.PutEmpty(decision.Key))
	default:
		panic("unknown semconvweave action")
	}
}

func ExampleWeaveID_SpanAttribute() {
	source := pcommon.NewMap()
	source.PutStr("db.system", "mssql")

	weave := semconvweave.Resolve(
		"https://opentelemetry.io/schemas/1.7.0",
	)

	projected := pcommon.NewMap()
	value, _ := source.Get("db.system")
	decision := weave.SpanAttribute(
		source,
		"SELECT users",
		"db.system",
		value,
	)
	emitDecision(projected, value, decision)

	target, _ := projected.Get("db.system.name")
	fmt.Println(weave.State())
	fmt.Println(target.Str())
	// Output:
	// supported-legacy
	// microsoft.sql_server
}

func ExampleWeaveID_SpanAttributeWithContext() {
	span := ptrace.NewSpan()
	span.SetKind(ptrace.SpanKindClient)
	span.Attributes().PutStr("http.method", "PROPFIND")
	span.Attributes().PutStr("net.peer.name", "example.com")
	span.Attributes().PutInt("net.peer.port", 443)
	weave := semconvweave.Resolve("https://opentelemetry.io/schemas/1.7.0")
	context := semconvweave.SpanContext{Name: span.Name(), Kind: span.Kind()}
	projected := pcommon.NewMap()
	span.Attributes().Range(func(key string, value pcommon.Value) bool {
		for _, decision := range weave.SpanAttributeWithContext(span.Attributes(), context, key, value) {
			emitDecision(projected, value, decision)
		}
		return true
	})
	method, _ := projected.Get("http.request.method")
	original, _ := projected.Get("http.request.method_original")
	address, _ := projected.Get("server.address")
	port, _ := projected.Get("server.port")
	fmt.Println(method.Str(), original.Str(), address.Str(), port.Int())
	// Output: _OTHER PROPFIND example.com 443
}

func TestAttributeProjectionDoesNotOverrideDerivedGetters(t *testing.T) {
	source := pcommon.NewMap()
	source.PutStr("db.system.name", "")
	source.PutStr("db.system", "mysql")
	source.PutStr("db.operation.name", "")
	source.PutStr("db.operation", "select")
	source.PutStr("db.query.text", "")
	source.PutStr("db.statement", "SELECT ?")
	before := source.AsRaw()

	legacySystem, _ := source.Get("db.system")
	decision := semconvweave.Resolve(
		"https://opentelemetry.io/schemas/1.7.0",
	).SpanAttribute(source, "source-span", "db.system", legacySystem)
	if decision.Action != semconvweave.AttributeSuppress {
		t.Fatalf("legacy db.system decision = %+v, want suppress when canonical key is present", decision)
	}

	if got := otlpmap.GetOperationName(source, ptrace.SpanKindClient); got != "mysql.select" {
		t.Fatalf("GetOperationName() = %q, want raw-map non-empty fallback", got)
	}
	if got := otlpmap.GetResourceName(source, ptrace.SpanKindClient, "source-span"); got != "SELECT ?" {
		t.Fatalf("GetResourceName() = %q, want raw-map non-empty fallback", got)
	}
	if !reflect.DeepEqual(before, source.AsRaw()) {
		t.Fatal("attribute projection mutated the source map used by derived getters")
	}
}

func ExampleWeaveID_ProjectSpan() {
	span := ptrace.NewSpan()
	span.SetKind(ptrace.SpanKindServer)
	span.Attributes().PutStr("http.method", "GET")
	span.Attributes().PutStr("http.target", "/orders?sig=secret&page=2")
	span.Attributes().PutStr("net.sock.peer.addr", "10.0.0.1")
	projection := semconvweave.Resolve("").ProjectSpan(span, semconvweave.SpanMigrationOptions{})
	path, _ := projection.Attributes.Get("url.path")
	query, _ := projection.Attributes.Get("url.query")
	client, _ := projection.Attributes.Get("client.address")
	fmt.Println(projection.Complete(), path.Str(), query.Str(), client.Str())
	// Output: true /orders sig=REDACTED&page=2 10.0.0.1
}
