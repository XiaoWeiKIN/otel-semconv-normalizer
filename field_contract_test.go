// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"reflect"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestCanonicalFieldGettersDoNotMutateInput(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr(keyHTTPRequestMethod, "BREW")
	attrs.PutStr(keyHTTPRequestMethodOriginal, "WIRE-BREW")
	attrs.PutStr(keyHTTPMethod, "GET")
	attrs.PutInt(keyHTTPResponseStatusCode, 207)
	attrs.PutInt(keyHTTPStatusCode, 500)
	attrs.PutStr(keyURLFull, "https://example.test/orders/42")
	attrs.PutStr(keyHTTPURL, "http://legacy.test/orders/42")
	attrs.PutStr(keyURLScheme, "https")
	attrs.PutStr(keyHTTPScheme, "http")
	attrs.PutStr(keyURLTemplate, "/orders/{id}")
	attrs.PutStr(keyHTTPRoute, "/orders/{id}")
	attrs.PutStr(keyUserAgentOriginal, "agent/current")
	attrs.PutStr(keyHTTPUserAgent, "agent/legacy")
	attrs.PutStr(keyNetworkProtocolName, "http")
	attrs.PutStr(keyNetProtocolName, "legacy-http")
	attrs.PutStr(keyNetworkTransport, "")
	attrs.PutStr(keyNetTransport, "ip_tcp")
	attrs.PutStr(keyDBOperationName, "select")
	attrs.PutStr(keyDBOperation, "legacy-select")
	attrs.PutStr(keyDBCollectionName, "orders")
	attrs.PutStr(keyDBSQLTable, "legacy-orders")
	attrs.PutStr(keyDBNamespace, "shop")
	attrs.PutStr(keyDBName, "legacy-shop")
	attrs.PutStr(keyDBQuerySummary, "SELECT orders")
	attrs.PutStr(keyDBQueryText, "SELECT * FROM orders")
	attrs.PutStr(keyDBStatement, "SELECT legacy")
	before := attrs.AsRaw()

	_ = GetHTTPRequestMethod(attrs)
	_ = GetHTTPRequestMethodOriginal(attrs)
	_, _ = GetHTTPResponseStatusCode(attrs)
	_ = GetURLFull(attrs)
	_ = GetURLScheme(attrs)
	_ = GetURLTemplate(attrs)
	_ = GetHTTPRoute(attrs)
	_ = GetUserAgentOriginal(attrs)
	_ = GetNetworkProtocolName(attrs)
	_ = GetNetworkTransport(attrs)
	_ = GetDBOperationName(attrs)
	_ = GetDBCollectionName(attrs)
	_ = GetDBNamespace(attrs)
	_ = GetDBQuerySummary(attrs)
	_ = GetDBQueryText(attrs)

	if after := attrs.AsRaw(); !reflect.DeepEqual(after, before) {
		t.Fatalf("input attributes mutated: before=%v after=%v", before, after)
	}
}

var (
	canonicalFieldStringResult string
	canonicalFieldStatusResult int64
	canonicalFieldStatusOK     bool
)

func TestCanonicalFieldGettersDoNotAllocate(t *testing.T) {
	currentURL := testAttrs(map[string]string{keyURLFull: "https://example.test/orders"})
	legacyURL := testAttrs(map[string]string{keyHTTPURL: "https://legacy.test/orders"})
	currentMethod := testAttrs(map[string]string{keyHTTPRequestMethod: "GET"})
	legacyCustomMethod := testAttrs(map[string]string{keyHTTPMethod: "PROPFIND"})
	legacyTransport := testAttrs(map[string]string{keyNetTransport: "ip_tcp"})
	legacyDBOperation := testAttrs(map[string]string{keyDBOperation: "select"})
	legacyDBCollection := testAttrs(map[string]string{keyDBCosmosDBContainer: "orders"})
	legacyDBNamespace := testAttrs(map[string]string{keyDBMSSQLInstanceName: "primary"})
	currentDBQuerySummary := testAttrs(map[string]string{keyDBQuerySummary: "SELECT orders"})
	legacyDBQueryText := testAttrs(map[string]string{keyDBStatement: "SELECT * FROM orders"})
	currentStatus := pcommon.NewMap()
	currentStatus.PutInt(keyHTTPResponseStatusCode, 201)
	legacyStatus := pcommon.NewMap()
	legacyStatus.PutInt(keyHTTPStatusCode, 503)

	tests := []struct {
		name string
		run  func()
	}{
		{
			name: "string current",
			run: func() {
				canonicalFieldStringResult = GetURLFull(currentURL)
			},
		},
		{
			name: "string legacy fallback",
			run: func() {
				canonicalFieldStringResult = GetURLFull(legacyURL)
			},
		},
		{
			name: "method current",
			run: func() {
				canonicalFieldStringResult = GetHTTPRequestMethod(currentMethod)
			},
		},
		{
			name: "method canonical",
			run: func() {
				canonicalFieldStringResult = GetHTTPRequestMethod(legacyCustomMethod)
			},
		},
		{
			name: "method original derived",
			run: func() {
				canonicalFieldStringResult = GetHTTPRequestMethodOriginal(legacyCustomMethod)
			},
		},
		{
			name: "transport canonical",
			run: func() {
				canonicalFieldStringResult = GetNetworkTransport(legacyTransport)
			},
		},
		{
			name: "db operation legacy",
			run: func() {
				canonicalFieldStringResult = GetDBOperationName(legacyDBOperation)
			},
		},
		{
			name: "db collection last legacy",
			run: func() {
				canonicalFieldStringResult = GetDBCollectionName(legacyDBCollection)
			},
		},
		{
			name: "db namespace last legacy",
			run: func() {
				canonicalFieldStringResult = GetDBNamespace(legacyDBNamespace)
			},
		},
		{
			name: "db query summary current",
			run: func() {
				canonicalFieldStringResult = GetDBQuerySummary(currentDBQuerySummary)
			},
		},
		{
			name: "db query text legacy",
			run: func() {
				canonicalFieldStringResult = GetDBQueryText(legacyDBQueryText)
			},
		},
		{
			name: "typed status current",
			run: func() {
				canonicalFieldStatusResult, canonicalFieldStatusOK = GetHTTPResponseStatusCode(currentStatus)
			},
		},
		{
			name: "typed status legacy",
			run: func() {
				canonicalFieldStatusResult, canonicalFieldStatusOK = GetHTTPResponseStatusCode(legacyStatus)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if allocs := testing.AllocsPerRun(1000, test.run); allocs != 0 {
				t.Fatalf("getter allocated %v times, want 0", allocs)
			}
		})
	}
}

func BenchmarkCanonicalFieldGetters(b *testing.B) {
	currentURL := testAttrs(map[string]string{keyURLFull: "https://example.test/orders"})
	legacyURL := testAttrs(map[string]string{keyHTTPURL: "https://legacy.test/orders"})
	currentMethod := testAttrs(map[string]string{keyHTTPRequestMethod: "GET"})
	legacyCustomMethod := testAttrs(map[string]string{keyHTTPMethod: "PROPFIND"})
	legacyTransport := testAttrs(map[string]string{keyNetTransport: "ip_tcp"})
	legacyDBOperation := testAttrs(map[string]string{keyDBOperation: "select"})
	legacyDBCollection := testAttrs(map[string]string{keyDBCosmosDBContainer: "orders"})
	legacyDBNamespace := testAttrs(map[string]string{keyDBMSSQLInstanceName: "primary"})
	currentDBQuerySummary := testAttrs(map[string]string{keyDBQuerySummary: "SELECT orders"})
	legacyDBQueryText := testAttrs(map[string]string{keyDBStatement: "SELECT * FROM orders"})
	currentStatus := pcommon.NewMap()
	currentStatus.PutInt(keyHTTPResponseStatusCode, 201)
	legacyStatus := pcommon.NewMap()
	legacyStatus.PutInt(keyHTTPStatusCode, 503)

	benchmarks := []struct {
		name string
		run  func()
	}{
		{name: "string_current", run: func() { canonicalFieldStringResult = GetURLFull(currentURL) }},
		{name: "string_legacy", run: func() { canonicalFieldStringResult = GetURLFull(legacyURL) }},
		{name: "method_current", run: func() { canonicalFieldStringResult = GetHTTPRequestMethod(currentMethod) }},
		{name: "method_canonical", run: func() { canonicalFieldStringResult = GetHTTPRequestMethod(legacyCustomMethod) }},
		{name: "method_original", run: func() { canonicalFieldStringResult = GetHTTPRequestMethodOriginal(legacyCustomMethod) }},
		{name: "transport_canonical", run: func() { canonicalFieldStringResult = GetNetworkTransport(legacyTransport) }},
		{name: "db_operation_legacy", run: func() { canonicalFieldStringResult = GetDBOperationName(legacyDBOperation) }},
		{name: "db_collection_last_legacy", run: func() { canonicalFieldStringResult = GetDBCollectionName(legacyDBCollection) }},
		{name: "db_namespace_last_legacy", run: func() { canonicalFieldStringResult = GetDBNamespace(legacyDBNamespace) }},
		{name: "db_query_summary_current", run: func() { canonicalFieldStringResult = GetDBQuerySummary(currentDBQuerySummary) }},
		{name: "db_query_text_legacy", run: func() { canonicalFieldStringResult = GetDBQueryText(legacyDBQueryText) }},
		{name: "status_current", run: func() {
			canonicalFieldStatusResult, canonicalFieldStatusOK = GetHTTPResponseStatusCode(currentStatus)
		}},
		{name: "status_legacy", run: func() {
			canonicalFieldStatusResult, canonicalFieldStatusOK = GetHTTPResponseStatusCode(legacyStatus)
		}},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmark.run()
			}
		})
	}
}
