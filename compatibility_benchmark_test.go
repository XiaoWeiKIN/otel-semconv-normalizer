// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var (
	compatibilityTechnologyResult string
	compatibilityEntityTypeResult EntityType
	compatibilityOperationResult  string
	compatibilityResourceResult   string
	compatibilityProjectionSink   compatibilityProjection
)

type compatibilityCase struct {
	name       string
	attrs      pcommon.Map
	wantTech   string
	wantEntity EntityType
}

type compatibilityProjection struct {
	technology string
	entityType EntityType
	operation  string
	resource   string
}

func classificationCompatibilityCases() []compatibilityCase {
	return []compatibilityCase{
		{name: "current_only", attrs: testAttrs(map[string]string{"db.system.name": "redis"}), wantTech: "redis", wantEntity: EntityTypeCache},
		{name: "legacy_only", attrs: testAttrs(map[string]string{"db.system": "redis"}), wantTech: "redis", wantEntity: EntityTypeCache},
		{name: "current_legacy_same", attrs: testAttrs(map[string]string{"db.system.name": "redis", "db.system": "redis"}), wantTech: "redis", wantEntity: EntityTypeCache},
		{name: "current_legacy_conflict", attrs: testAttrs(map[string]string{"db.system.name": "redis", "db.system": "mysql"}), wantTech: "redis", wantEntity: EntityTypeCache},
		{name: "vendor_fallback", attrs: testAttrs(map[string]string{"db.type": "redis"}), wantTech: "redis", wantEntity: EntityTypeCache},
		{name: "noncanonical_uppercase", attrs: testAttrs(map[string]string{"db.system": "REDIS"}), wantTech: "REDIS", wantEntity: EntityTypeDB},
		{name: "noncanonical_space_padded", attrs: testAttrs(map[string]string{"db.system": " redis "}), wantTech: " redis ", wantEntity: EntityTypeDB},
	}
}

func TestCompatibilityClassificationDoesNotAllocate(t *testing.T) {
	for _, test := range classificationCompatibilityCases() {
		t.Run(test.name, func(t *testing.T) {
			if got := GetTechnology(test.attrs); got != test.wantTech {
				t.Fatalf("GetTechnology() = %q, want %q", got, test.wantTech)
			}
			if got := GetEntityType(test.attrs, ptrace.SpanKindClient); got != test.wantEntity {
				t.Fatalf("GetEntityType() = %q, want %q", got, test.wantEntity)
			}

			technologyAllocs := testing.AllocsPerRun(1000, func() {
				compatibilityTechnologyResult = GetTechnology(test.attrs)
			})
			if technologyAllocs != 0 {
				t.Fatalf("GetTechnology() allocated %v times, want 0", technologyAllocs)
			}

			entityTypeAllocs := testing.AllocsPerRun(1000, func() {
				compatibilityEntityTypeResult = GetEntityType(test.attrs, ptrace.SpanKindClient)
			})
			if entityTypeAllocs != 0 {
				t.Fatalf("GetEntityType() allocated %v times, want 0", entityTypeAllocs)
			}
		})
	}
}

func BenchmarkGetTechnologyCompatibility(b *testing.B) {
	for _, benchmark := range classificationCompatibilityCases() {
		b.Run(benchmark.name, func(b *testing.B) {
			if got := GetTechnology(benchmark.attrs); got != benchmark.wantTech {
				b.Fatalf("GetTechnology() = %q, want %q", got, benchmark.wantTech)
			}
			b.ReportAllocs()
			for b.Loop() {
				compatibilityTechnologyResult = GetTechnology(benchmark.attrs)
			}
		})
	}
}

func BenchmarkGetEntityTypeCompatibility(b *testing.B) {
	for _, benchmark := range classificationCompatibilityCases() {
		b.Run(benchmark.name, func(b *testing.B) {
			if got := GetEntityType(benchmark.attrs, ptrace.SpanKindClient); got != benchmark.wantEntity {
				b.Fatalf("GetEntityType() = %q, want %q", got, benchmark.wantEntity)
			}
			b.ReportAllocs()
			for b.Loop() {
				compatibilityEntityTypeResult = GetEntityType(benchmark.attrs, ptrace.SpanKindClient)
			}
		})
	}
}

func BenchmarkGetOperationNameCompatibility(b *testing.B) {
	benchmarks := []struct {
		name  string
		attrs pcommon.Map
		want  string
	}{
		{name: "db_current", attrs: testAttrs(map[string]string{"db.system.name": "redis"}), want: "redis.query"},
		{name: "db_legacy", attrs: testAttrs(map[string]string{"db.system": "redis"}), want: "redis.query"},
		{name: "db_current_legacy_conflict", attrs: testAttrs(map[string]string{"db.system.name": "postgresql", "db.system": "mysql"}), want: "postgresql.query"},
		{name: "rpc_current", attrs: testAttrs(map[string]string{"rpc.system.name": "grpc"}), want: "grpc.client.request"},
		{name: "rpc_legacy", attrs: testAttrs(map[string]string{"rpc.system": "grpc"}), want: "grpc.client.request"},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			if got := GetOperationName(benchmark.attrs, ptrace.SpanKindClient); got != benchmark.want {
				b.Fatalf("GetOperationName() = %q, want %q", got, benchmark.want)
			}
			b.ReportAllocs()
			for b.Loop() {
				compatibilityOperationResult = GetOperationName(benchmark.attrs, ptrace.SpanKindClient)
			}
		})
	}
}

func BenchmarkGetResourceNameCompatibility(b *testing.B) {
	benchmarks := []struct {
		name     string
		kind     ptrace.SpanKind
		attrs    pcommon.Map
		fallback string
		want     string
	}{
		{name: "db_current", kind: ptrace.SpanKindClient, attrs: testAttrs(map[string]string{"db.system.name": "redis", "db.query.text": "GET ?"}), fallback: "fallback", want: "GET ?"},
		{name: "db_legacy", kind: ptrace.SpanKindClient, attrs: testAttrs(map[string]string{"db.system": "mysql", "db.statement": "SELECT ?"}), fallback: "fallback", want: "SELECT ?"},
		{name: "http_client_no_template", kind: ptrace.SpanKindClient, attrs: testAttrs(map[string]string{"http.request.method": "GET", "url.template": "", "http.route": "/orders/{id}"}), fallback: "fallback", want: "GET"},
		{name: "full_fallback", kind: ptrace.SpanKindInternal, attrs: pcommon.NewMap(), fallback: "fallback", want: "fallback"},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			if got := GetResourceName(benchmark.attrs, benchmark.kind, benchmark.fallback); got != benchmark.want {
				b.Fatalf("GetResourceName() = %q, want %q", got, benchmark.want)
			}
			b.ReportAllocs()
			for b.Loop() {
				compatibilityResourceResult = GetResourceName(benchmark.attrs, benchmark.kind, benchmark.fallback)
			}
		})
	}
}

func BenchmarkCompatibilityProjection(b *testing.B) {
	benchmarks := []struct {
		name     string
		attrs    pcommon.Map
		kind     ptrace.SpanKind
		fallback string
		want     compatibilityProjection
	}{
		{name: "current_only", attrs: testAttrs(map[string]string{"db.system.name": "redis", "db.query.text": "GET ?"}), kind: ptrace.SpanKindClient, fallback: "fallback", want: compatibilityProjection{technology: "redis", entityType: EntityTypeCache, operation: "redis.query", resource: "GET ?"}},
		{name: "legacy_only", attrs: testAttrs(map[string]string{"db.system": "mssql", "db.statement": "SELECT ?"}), kind: ptrace.SpanKindClient, fallback: "fallback", want: compatibilityProjection{technology: "microsoft.sql_server", entityType: EntityTypeDB, operation: "microsoft.sql_server.query", resource: "SELECT ?"}},
		{name: "current_legacy_conflict", attrs: testAttrs(map[string]string{"db.system.name": "postgresql", "db.system": "mysql", "db.query.text": "SELECT ?", "db.statement": "SELECT legacy"}), kind: ptrace.SpanKindClient, fallback: "fallback", want: compatibilityProjection{technology: "postgresql", entityType: EntityTypeDB, operation: "postgresql.query", resource: "SELECT ?"}},
		{name: "vendor_fallback", attrs: testAttrs(map[string]string{"db.type": "mysql", "db.statement": "SELECT ?"}), kind: ptrace.SpanKindClient, fallback: "fallback", want: compatibilityProjection{technology: "mysql", entityType: EntityTypeDB, operation: "mysql.query", resource: "SELECT ?"}},
		{name: "http_client_no_template", attrs: testAttrs(map[string]string{"http.request.method": "GET", "url.template": "", "http.route": "/orders/{id}"}), kind: ptrace.SpanKindClient, fallback: "fallback", want: compatibilityProjection{technology: "http", entityType: EntityTypeHTTP, operation: "http.client.request", resource: "GET"}},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			got := compatibilityProjection{
				technology: GetTechnology(benchmark.attrs),
				entityType: GetEntityType(benchmark.attrs, benchmark.kind),
				operation:  GetOperationName(benchmark.attrs, benchmark.kind),
				resource:   GetResourceName(benchmark.attrs, benchmark.kind, benchmark.fallback),
			}
			if got != benchmark.want {
				b.Fatalf("compatibility projection = %+v, want %+v", got, benchmark.want)
			}
			b.ReportAllocs()
			for b.Loop() {
				compatibilityProjectionSink = compatibilityProjection{
					technology: GetTechnology(benchmark.attrs),
					entityType: GetEntityType(benchmark.attrs, benchmark.kind),
					operation:  GetOperationName(benchmark.attrs, benchmark.kind),
					resource:   GetResourceName(benchmark.attrs, benchmark.kind, benchmark.fallback),
				}
			}
		})
	}
}

func BenchmarkCoreFieldConsumers(b *testing.B) {
	attrs := testAttrs(map[string]string{
		keyHTTPRequestMethod: "GET",
		keyHTTPRoute:         "/orders/{id}",
		keyURLFull:           "https://example.test/orders/42",
	})

	b.Run("technology_http", func(b *testing.B) {
		if got := GetTechnology(attrs); got != TechnologyHTTP {
			b.Fatalf("GetTechnology() = %q, want %q", got, TechnologyHTTP)
		}
		b.ReportAllocs()
		for b.Loop() {
			compatibilityTechnologyResult = GetTechnology(attrs)
		}
	})

	b.Run("operation_http_server", func(b *testing.B) {
		const want = "http.server.request"
		if got := GetOperationName(attrs, ptrace.SpanKindServer); got != want {
			b.Fatalf("GetOperationName() = %q, want %q", got, want)
		}
		b.ReportAllocs()
		for b.Loop() {
			compatibilityOperationResult = GetOperationName(attrs, ptrace.SpanKindServer)
		}
	})

	b.Run("resource_http_server", func(b *testing.B) {
		const want = "GET /orders/{id}"
		if got := GetResourceName(attrs, ptrace.SpanKindServer, "fallback"); got != want {
			b.Fatalf("GetResourceName() = %q, want %q", got, want)
		}
		b.ReportAllocs()
		for b.Loop() {
			compatibilityResourceResult = GetResourceName(attrs, ptrace.SpanKindServer, "fallback")
		}
	})
}
