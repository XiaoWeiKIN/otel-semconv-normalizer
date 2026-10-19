// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"strings"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var _ func(pcommon.Map, ptrace.SpanKind, string) string = GetResourceName

func TestGetResourceName(t *testing.T) {
	cases := []struct {
		name  string
		kind  ptrace.SpanKind
		attrs map[string]string
		want  string
	}{
		{"explicit resource.name", ptrace.SpanKindClient, map[string]string{"resource.name": "X"}, "X"},
		{"http server with route", ptrace.SpanKindServer, map[string]string{"http.request.method": "GET", "http.route": "/orders/{id}"}, "GET /orders/{id}"},
		{"http client with url template", ptrace.SpanKindClient, map[string]string{"http.method": "GET", "url.template": "/orders/{id}"}, "GET /orders/{id}"},
		{"http client ignores server route", ptrace.SpanKindClient, map[string]string{"http.request.method": "GET", "http.route": "/orders/{id}"}, "GET"},
		{"http client template precedes route", ptrace.SpanKindClient, map[string]string{"http.request.method": "GET", "url.template": "/v2/orders/{id}", "http.route": "/orders/{id}"}, "GET /v2/orders/{id}"},
		{"http client empty template ignores server route", ptrace.SpanKindClient, map[string]string{"http.request.method": "GET", "url.template": "", "http.route": "/orders/{id}"}, "GET"},
		{"http _OTHER method", ptrace.SpanKindServer, map[string]string{"http.request.method": "_OTHER"}, "HTTP"},
		{"http _OTHER method ignores original", ptrace.SpanKindServer, map[string]string{"http.request.method": "_OTHER", "http.request.method_original": "PROPFIND", "http.route": "/files/{id}"}, "HTTP /files/{id}"},
		{"http known method ignores invalid original companion", ptrace.SpanKindServer, map[string]string{"http.request.method": "GET", "http.request.method_original": "get", "http.route": "/files/{id}"}, "GET /files/{id}"},
		{"http custom method", ptrace.SpanKindServer, map[string]string{"http.request.method": "BREW", "http.route": "/coffee/{id}"}, "BREW /coffee/{id}"},
		{"messaging operation name + destination", ptrace.SpanKindConsumer, map[string]string{"messaging.operation.name": "consume", "messaging.destination.name": "orders"}, "consume orders"},
		{"messaging legacy operation + destination", ptrace.SpanKindConsumer, map[string]string{"messaging.operation": "receive", "messaging.destination": "orders"}, "receive orders"},
		{"messaging operation type is not a resource name", ptrace.SpanKindConsumer, map[string]string{"messaging.operation.type": "receive", "messaging.destination.name": "orders"}, "fallback-op"},
		{"rpc method + service", ptrace.SpanKindClient, map[string]string{"rpc.method": "Get", "rpc.service": "order.Orders"}, "Get order.Orders"},
		{"rpc fully qualified method still appends legacy service", ptrace.SpanKindClient, map[string]string{"rpc.method": "order.Orders/Get", "rpc.service": "stale.Service"}, "order.Orders/Get stale.Service"},
		{"graphql", ptrace.SpanKindServer, map[string]string{"graphql.operation.type": "query", "graphql.operation.name": "Orders"}, "query Orders"},
		{"db raw query wins over summary", ptrace.SpanKindClient, map[string]string{"db.system.name": "mysql", "db.query.summary": "SELECT orders", "db.operation.name": "select", "db.collection.name": "users", "db.query.text": "SELECT * FROM orders"}, "SELECT * FROM orders"},
		{"db current query", ptrace.SpanKindClient, map[string]string{"db.system.name": "redis", "db.query.text": "GET ?"}, "GET ?"},
		{"db legacy statement", ptrace.SpanKindClient, map[string]string{"db.system": "mysql", "db.statement": "SELECT ?"}, "SELECT ?"},
		{"db vendor system", ptrace.SpanKindClient, map[string]string{"db.type": "mysql", "db.statement": "SELECT vendor"}, "SELECT vendor"},
		{"db query without system uses fallback", ptrace.SpanKindClient, map[string]string{"db.query.text": "SELECT ?"}, "fallback-op"},
		{"db summary without query uses fallback", ptrace.SpanKindClient, map[string]string{"db.system.name": "mysql", "db.query.summary": "SELECT orders"}, "fallback-op"},
		{"db operation and collection without query use fallback", ptrace.SpanKindClient, map[string]string{"db.system.name": "mysql", "db.operation.name": "UPDATE", "db.collection.name": "orders"}, "fallback-op"},
		{"fallback", ptrace.SpanKindInternal, map[string]string{}, "fallback-op"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := GetResourceName(testAttrs(c.attrs), c.kind, "fallback-op"); got != c.want {
				t.Fatalf("GetResourceName = %q, want %q", got, c.want)
			}
		})
	}
}

func TestGetResourceNameUsesAgentSanitizedDBQuery(t *testing.T) {
	tests := []struct {
		name      string
		attrs     map[string]string
		wantQuery string
	}{
		{
			name: "current",
			attrs: map[string]string{
				"db.system.name": "postgresql",
				"db.query.text":  "SELECT * FROM current WHERE id = ?",
			},
			wantQuery: "SELECT * FROM current WHERE id = ?",
		},
		{
			name: "legacy",
			attrs: map[string]string{
				"db.system":    "mssql",
				"db.statement": "SELECT * FROM users WHERE id = ?",
			},
			wantQuery: "SELECT * FROM users WHERE id = ?",
		},
		{
			name: "vendor system",
			attrs: map[string]string{
				"db.type":      "mysql",
				"db.statement": "SELECT * FROM users WHERE id = ?",
			},
			wantQuery: "SELECT * FROM users WHERE id = ?",
		},
		{
			name: "incomplete operation pair",
			attrs: map[string]string{
				"db.system.name":    "mysql",
				"db.operation.name": "SELECT",
				"db.query.text":     "SELECT ?",
			},
			wantQuery: "SELECT ?",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attrs := testAttrs(test.attrs)
			if query := GetDBQueryText(attrs); query != test.wantQuery {
				t.Fatalf("GetDBQueryText() = %q, want %q", query, test.wantQuery)
			}
			if got := GetResourceName(attrs, ptrace.SpanKindClient, "fallback"); got != test.wantQuery {
				t.Fatalf("GetResourceName() = %q, want %q", got, test.wantQuery)
			}
			if got := GetDBQueryText(attrs); got != test.wantQuery {
				t.Fatalf("GetDBQueryText() = %q after resource lookup, want %q", got, test.wantQuery)
			}
		})
	}
}

func TestGetResourceNameDBQueryDoesNotAllocate(t *testing.T) {
	tests := []struct {
		name  string
		attrs pcommon.Map
	}{
		{
			name:  "current",
			attrs: testAttrs(map[string]string{"db.system.name": "mysql", "db.query.text": "SELECT ?"}),
		},
		{
			name:  "legacy",
			attrs: testAttrs(map[string]string{"db.system": "mysql", "db.statement": "SELECT ?"}),
		},
		{
			name:  "vendor",
			attrs: testAttrs(map[string]string{"db.type": "mysql", "db.statement": "SELECT ?"}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(1000, func() {
				getResourceNameBenchmarkResult = GetResourceName(test.attrs, ptrace.SpanKindClient, "fallback")
			})
			if allocs != 0 {
				t.Fatalf("GetResourceName() allocated %v times, want 0", allocs)
			}
		})
	}
}

func TestGetResourceNameTruncates(t *testing.T) {
	long := strings.Repeat("x", maxResourceNameLen+100)
	got := GetResourceName(testAttrs(map[string]string{"resource.name": long}), ptrace.SpanKindClient, "")
	if len(got) != maxResourceNameLen {
		t.Fatalf("expected truncation to %d, got %d", maxResourceNameLen, len(got))
	}
}

var getResourceNameBenchmarkResult string

func BenchmarkGetResourceName(b *testing.B) {
	cases := []struct {
		name     string
		kind     ptrace.SpanKind
		attrs    pcommon.Map
		fallback string
	}{
		{"http_client_template", ptrace.SpanKindClient, testAttrs(map[string]string{"http.request.method": "GET", "url.template": "/orders/{id}"}), "fallback"},
		{"db_current_query", ptrace.SpanKindClient, testAttrs(map[string]string{"db.system.name": "mysql", "db.query.text": "SELECT ?"}), "fallback"},
		{"db_legacy_statement", ptrace.SpanKindClient, testAttrs(map[string]string{"db.system": "mysql", "db.statement": "SELECT ?"}), "fallback"},
		{"full_fallback", ptrace.SpanKindInternal, pcommon.NewMap(), "fallback"},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				getResourceNameBenchmarkResult = GetResourceName(c.attrs, c.kind, c.fallback)
			}
		})
	}
}

func TestResourceAttributes(t *testing.T) {
	res := testAttrs(map[string]string{"service.name": "orders", "deployment.environment.name": "prod", "service.version": "1.2.3"})
	if got := GetServiceName(res); got != "orders" {
		t.Errorf("GetServiceName = %q", got)
	}
	if got := GetDeploymentEnvironmentName(res); got != "prod" {
		t.Errorf("GetDeploymentEnvironmentName = %q", got)
	}
	if got := GetServiceVersion(res); got != "1.2.3" {
		t.Errorf("GetServiceVersion = %q", got)
	}

	// Legacy env name + defaults.
	if got := GetDeploymentEnvironmentName(testAttrs(map[string]string{"deployment.environment": "staging"})); got != "staging" {
		t.Errorf("legacy GetDeploymentEnvironmentName = %q", got)
	}
	if got := GetDeploymentEnvironmentName(testAttrs(map[string]string{"deployment.environment.name": "prod", "deployment.environment": "staging"})); got != "prod" {
		t.Errorf("current GetDeploymentEnvironmentName = %q, want current value", got)
	}
	if got := GetDeploymentEnvironmentName(testAttrs(map[string]string{"deployment.environment.name": "", "deployment.environment": "staging"})); got != "staging" {
		t.Errorf("empty current GetDeploymentEnvironmentName = %q, want legacy fallback", got)
	}
	empty := pcommon.NewMap()
	if got := GetServiceName(empty); got != DefaultServiceName {
		t.Errorf("default GetServiceName = %q", got)
	}
	if got := GetDeploymentEnvironmentName(empty); got != DefaultDeploymentEnvironmentName {
		t.Errorf("default GetDeploymentEnvironmentName = %q", got)
	}
	if got := GetServiceVersion(empty); got != "" {
		t.Errorf("absent GetServiceVersion = %q, want empty", got)
	}
}
