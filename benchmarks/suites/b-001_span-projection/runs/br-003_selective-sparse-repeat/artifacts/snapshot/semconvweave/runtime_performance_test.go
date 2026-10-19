// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

var (
	benchmarkWeaveID           WeaveID
	benchmarkDecision          AttributeDecision
	benchmarkName              string
	benchmarkAttributeChecksum uint64
)

func TestRuntimeNoOpPathsDoNotAllocate(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("vendor.custom", "value")
	source := pcommon.NewValueStr("value")
	legacy := Resolve("https://opentelemetry.io/schemas/1.7.0")
	resourceHotAttrs := pcommon.NewMap()
	resourceHotAttrs.PutStr("service.name", "checkout")
	resourceHotValue := pcommon.NewValueStr("checkout")
	httpHotAttrs := pcommon.NewMap()
	httpHotAttrs.PutStr("http.request.method", "GET")
	httpHotValue := pcommon.NewValueStr("GET")
	dbHotAttrs := pcommon.NewMap()
	dbHotAttrs.PutStr("db.system.name", "postgresql")
	dbHotValue := pcommon.NewValueStr("postgresql")
	eventHotAttrs := pcommon.NewMap()
	eventHotAttrs.PutStr("exception.message", "timeout")
	eventHotValue := pcommon.NewValueStr("timeout")
	fallbackAttrs := pcommon.NewMap()
	fallbackAttrs.PutStr("db.system", "mssql")
	fallbackValue := pcommon.NewValueStr("mssql")
	fallbackConflictAttrs := pcommon.NewMap()
	fallbackConflictAttrs.PutStr("db.system.name", "postgresql")
	fallbackConflictAttrs.PutStr("db.system", "mssql")
	tests := []struct {
		name string
		run  func()
	}{
		{"resolve missing", func() { benchmarkWeaveID = Resolve("") }},
		{"resolve canonical", func() {
			benchmarkWeaveID = Resolve("https://opentelemetry.io/schemas/1.41.0")
		}},
		{"resolve supported", func() {
			benchmarkWeaveID = Resolve("https://opentelemetry.io/schemas/1.7.0")
		}},
		{"resolve unknown", func() {
			benchmarkWeaveID = Resolve("https://opentelemetry.io/schemas/1.8.0")
		}},
		{"resource canonical no-op", func() {
			benchmarkDecision = WeaveCanonical.ResourceAttribute(attrs, "vendor.custom", source)
		}},
		{"span canonical no-op", func() {
			benchmarkDecision = WeaveCanonical.SpanAttribute(attrs, "span", "vendor.custom", source)
		}},
		{"event canonical no-op", func() {
			benchmarkDecision = WeaveCanonical.EventAttribute(
				attrs, "span", "event", "vendor.custom", source,
			)
		}},
		{"supported no-rule value", func() {
			benchmarkDecision = legacy.SpanAttribute(
				attrs, "span", "vendor.custom", source,
			)
		}},
		{"resource service hot miss", func() {
			benchmarkDecision = legacy.ResourceAttribute(
				resourceHotAttrs, "service.name", resourceHotValue,
			)
		}},
		{"span HTTP hot miss", func() {
			benchmarkDecision = legacy.SpanAttribute(
				httpHotAttrs, "GET /orders", "http.request.method", httpHotValue,
			)
		}},
		{"span DB hot miss", func() {
			benchmarkDecision = legacy.SpanAttribute(
				dbHotAttrs, "SELECT orders", "db.system.name", dbHotValue,
			)
		}},
		{"event exception hot miss", func() {
			benchmarkDecision = legacy.EventAttribute(
				eventHotAttrs, "GET /orders", "exception",
				"exception.message", eventHotValue,
			)
		}},
		{"canonical fallback hit", func() {
			benchmarkDecision = WeaveCanonical.SpanAttribute(
				fallbackAttrs, "SELECT", "db.system", fallbackValue,
			)
		}},
		{"missing fallback hit", func() {
			benchmarkDecision = WeaveMissing.SpanAttribute(
				fallbackAttrs, "SELECT", "db.system", fallbackValue,
			)
		}},
		{"unknown fallback conflict", func() {
			benchmarkDecision = WeaveUnknown.SpanAttribute(
				fallbackConflictAttrs, "SELECT", "db.system", fallbackValue,
			)
		}},
		{"invalid fallback hit", func() {
			benchmarkDecision = WeaveID(65535).SpanAttribute(
				fallbackAttrs, "SELECT", "db.system", fallbackValue,
			)
		}},
		{"span name canonical", func() { benchmarkName = WeaveCanonical.SpanName("span") }},
		{"event name canonical", func() {
			benchmarkName = WeaveCanonical.EventName("span", "event")
		}},
		{"span name supported no-rule", func() { benchmarkName = legacy.SpanName("span") }},
		{"event name supported no-rule", func() {
			benchmarkName = legacy.EventName("span", "event")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if allocations := testing.AllocsPerRun(1000, test.run); allocations != 0 {
				t.Fatalf("allocations = %v, want 0", allocations)
			}
		})
	}
}

func TestRuntimeGeneratedColdPathHotMissesKeepSource(t *testing.T) {
	legacy := Resolve("https://opentelemetry.io/schemas/1.7.0")
	for _, test := range []struct {
		name string
		key  string
		run  func(pcommon.Map, pcommon.Value) AttributeDecision
	}{
		{
			name: "Resource service", key: "service.name",
			run: func(attrs pcommon.Map, value pcommon.Value) AttributeDecision {
				return legacy.ResourceAttribute(attrs, "service.name", value)
			},
		},
		{
			name: "HTTP Span", key: "http.request.method",
			run: func(attrs pcommon.Map, value pcommon.Value) AttributeDecision {
				return legacy.SpanAttribute(attrs, "GET /orders", "http.request.method", value)
			},
		},
		{
			name: "DB Span", key: "db.system.name",
			run: func(attrs pcommon.Map, value pcommon.Value) AttributeDecision {
				return legacy.SpanAttribute(attrs, "SELECT orders", "db.system.name", value)
			},
		},
		{
			name: "Event exception", key: "exception.message",
			run: func(attrs pcommon.Map, value pcommon.Value) AttributeDecision {
				return legacy.EventAttribute(
					attrs, "GET /orders", "exception", "exception.message", value,
				)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			attrs.PutStr(test.key, "value")
			value := pcommon.NewValueStr("value")
			before := attrs.AsRaw()
			sourceBefore := value.AsRaw()

			if got := test.run(attrs, value); got != (AttributeDecision{
				Key: test.key, Action: AttributeKeepValue,
			}) {
				t.Fatalf("hot miss decision = %+v", got)
			}
			if !reflect.DeepEqual(before, attrs.AsRaw()) {
				t.Fatal("hot miss mutated attrs")
			}
			if !reflect.DeepEqual(sourceBefore, value.AsRaw()) {
				t.Fatal("hot miss mutated source value")
			}
		})
	}
}

func TestRuntimeDependencyGraphExcludesBuildOnlyPackages(t *testing.T) {
	command := exec.Command(
		"go", "list", "-deps", "-f", "{{.ImportPath}}", "./semconvweave",
	)
	command.Dir = repositoryRoot(t)
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go list runtime dependency graph: %v\n%s", err, output)
	}
	graph := string(output)
	for _, forbidden := range []string{
		"go.opentelemetry.io/otel/schema",
		"golang.org/x/perf",
		"semconv-weave-gen",
		"weaver",
	} {
		if strings.Contains(graph, forbidden) {
			t.Errorf("runtime dependency graph contains build-only package %q", forbidden)
		}
	}
}

func TestRuntimePerformanceFixturesRemainReadOnly(t *testing.T) {
	fixtures := runtimeBenchmarkFixtures()
	for _, fixture := range fixtures.attributes {
		before := fixture.attrs.AsRaw()
		sourceBefore := fixture.value.AsRaw()
		fixture.run()
		if !reflect.DeepEqual(before, fixture.attrs.AsRaw()) {
			t.Errorf("%s mutated attrs", fixture.name)
		}
		if !reflect.DeepEqual(sourceBefore, fixture.value.AsRaw()) {
			t.Errorf("%s mutated source value", fixture.name)
		}
	}
}

func BenchmarkRuntimeResolve(b *testing.B) {
	for _, test := range []struct {
		name string
		url  string
	}{
		{"missing", ""},
		{"canonical", "https://opentelemetry.io/schemas/1.41.0"},
		{"supported_legacy", "https://opentelemetry.io/schemas/1.7.0"},
		{"unknown", "https://opentelemetry.io/schemas/1.8.0"},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkWeaveID = Resolve(test.url)
			}
		})
	}
}

func BenchmarkRuntimeAttribute(b *testing.B) {
	fixtures := runtimeBenchmarkFixtures()
	for _, fixture := range fixtures.attributes {
		b.Run(fixture.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkDecision = fixture.run()
			}
		})
	}
	b.Run("event_heavy_8", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var checksum uint64
			for _, fixture := range fixtures.eventHeavy {
				decision := fixture.run()
				checksum += uint64(decision.Action) + uint64(len(decision.Key))
			}
			benchmarkAttributeChecksum = checksum
		}
	})
}

func BenchmarkRuntimeName(b *testing.B) {
	legacy := Resolve("https://opentelemetry.io/schemas/1.7.0")
	for _, test := range []struct {
		name string
		run  func() string
	}{
		{"span_canonical_noop", func() string { return WeaveCanonical.SpanName("GET") }},
		{"span_supported_no_rule", func() string { return legacy.SpanName("GET") }},
		{"event_canonical_noop", func() string {
			return WeaveCanonical.EventName("GET", "exception")
		}},
		{"event_supported_no_rule", func() string {
			return legacy.EventName("GET", "exception")
		}},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkName = test.run()
			}
		})
	}
}

type runtimeAttributeFixture struct {
	name  string
	attrs pcommon.Map
	value pcommon.Value
	run   func() AttributeDecision
}

type benchmarkFixtures struct {
	attributes []runtimeAttributeFixture
	eventHeavy []runtimeAttributeFixture
}

func runtimeBenchmarkFixtures() benchmarkFixtures {
	legacy := Resolve("https://opentelemetry.io/schemas/1.7.0")
	makeFixture := func(
		name string,
		attrs pcommon.Map,
		value pcommon.Value,
		run func() AttributeDecision,
	) runtimeAttributeFixture {
		return runtimeAttributeFixture{name: name, attrs: attrs, value: value, run: run}
	}

	canonicalAttrs := pcommon.NewMap()
	canonicalAttrs.PutStr("vendor.custom", "value")
	canonicalValue := pcommon.NewValueStr("value")

	resourceHotAttrs := pcommon.NewMap()
	resourceHotAttrs.PutStr("service.name", "checkout")
	resourceHotValue := pcommon.NewValueStr("checkout")

	httpHotAttrs := pcommon.NewMap()
	httpHotAttrs.PutStr("http.request.method", "GET")
	httpHotValue := pcommon.NewValueStr("GET")

	dbHotAttrs := pcommon.NewMap()
	dbHotAttrs.PutStr("db.system.name", "postgresql")
	dbHotValue := pcommon.NewValueStr("postgresql")

	eventHotAttrs := pcommon.NewMap()
	eventHotAttrs.PutStr("exception.message", "timeout")
	eventHotValue := pcommon.NewValueStr("timeout")

	exactAttrs := pcommon.NewMap()
	exactAttrs.PutStr("http.url", "https://example.test")
	exactValue := pcommon.NewValueStr("https://example.test")

	valueAttrs := pcommon.NewMap()
	valueAttrs.PutStr("db.system", "mssql")
	valueValue := pcommon.NewValueStr("mssql")

	prefixAttrs := pcommon.NewMap()
	prefixAttrs.PutStr("rpc.grpc.request.metadata.authorization", "secret")
	prefixValue := pcommon.NewValueStr("secret")

	conflictAttrs := pcommon.NewMap()
	conflictAttrs.PutStr("db.system.name", "postgresql")
	conflictAttrs.PutStr("db.system", "mssql")
	conflictValue := pcommon.NewValueStr("mssql")

	fixtures := benchmarkFixtures{
		attributes: []runtimeAttributeFixture{
			makeFixture("resource_hot_miss", resourceHotAttrs, resourceHotValue, func() AttributeDecision {
				return legacy.ResourceAttribute(
					resourceHotAttrs, "service.name", resourceHotValue,
				)
			}),
			makeFixture("span_http_hot_miss", httpHotAttrs, httpHotValue, func() AttributeDecision {
				return legacy.SpanAttribute(
					httpHotAttrs, "GET /orders", "http.request.method", httpHotValue,
				)
			}),
			makeFixture("span_db_hot_miss", dbHotAttrs, dbHotValue, func() AttributeDecision {
				return legacy.SpanAttribute(
					dbHotAttrs, "SELECT orders", "db.system.name", dbHotValue,
				)
			}),
			makeFixture("event_hot_miss", eventHotAttrs, eventHotValue, func() AttributeDecision {
				return legacy.EventAttribute(
					eventHotAttrs, "GET /orders", "exception",
					"exception.message", eventHotValue,
				)
			}),
			makeFixture("span_canonical_noop", canonicalAttrs, canonicalValue, func() AttributeDecision {
				return WeaveCanonical.SpanAttribute(
					canonicalAttrs, "GET", "vendor.custom", canonicalValue,
				)
			}),
			makeFixture("span_fallback_miss", canonicalAttrs, canonicalValue, func() AttributeDecision {
				return WeaveUnknown.SpanAttribute(
					canonicalAttrs, "GET", "vendor.custom", canonicalValue,
				)
			}),
			makeFixture("span_fallback_value", valueAttrs, valueValue, func() AttributeDecision {
				return WeaveMissing.SpanAttribute(valueAttrs, "SELECT", "db.system", valueValue)
			}),
			makeFixture("span_fallback_conflict", conflictAttrs, conflictValue, func() AttributeDecision {
				return WeaveCanonical.SpanAttribute(
					conflictAttrs, "SELECT", "db.system", conflictValue,
				)
			}),
			makeFixture("span_legacy_exact", exactAttrs, exactValue, func() AttributeDecision {
				return legacy.SpanAttribute(exactAttrs, "GET", "http.url", exactValue)
			}),
			makeFixture("span_legacy_value", valueAttrs, valueValue, func() AttributeDecision {
				return legacy.SpanAttribute(valueAttrs, "SELECT", "db.system", valueValue)
			}),
			makeFixture("span_legacy_prefix", prefixAttrs, prefixValue, func() AttributeDecision {
				return legacy.SpanAttribute(
					prefixAttrs, "rpc", "rpc.grpc.request.metadata.authorization", prefixValue,
				)
			}),
			makeFixture("span_conflict", conflictAttrs, conflictValue, func() AttributeDecision {
				return legacy.SpanAttribute(conflictAttrs, "SELECT", "db.system", conflictValue)
			}),
		},
	}
	eventKeys := [...]string{
		"message.id",
		"message.type",
		"message.compressed_size",
		"message.uncompressed_size",
		"rpc.system",
		"rpc.jsonrpc.version",
		"rpc.jsonrpc.request_id",
		"db.system",
	}
	for _, key := range eventKeys {
		attrs := pcommon.NewMap()
		value := pcommon.NewValueStr("value")
		attrs.PutStr(key, "value")
		keyCopy := key
		attrsCopy := attrs
		valueCopy := value
		fixtures.eventHeavy = append(fixtures.eventHeavy, makeFixture(
			"event_"+key, attrsCopy, valueCopy, func() AttributeDecision {
				return legacy.EventAttribute(
					attrsCopy, "span", "event", keyCopy, valueCopy,
				)
			},
		))
	}
	return fixtures
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	command := exec.Command("go", "env", "GOMOD")
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(strings.TrimSpace(string(output)))
}
