// SPDX-License-Identifier: Apache-2.0

package otlpmap_test

import (
	"fmt"

	otlpmap "github.com/XiaoWeiKIN/otel-semconv-normalizer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Example maps one Redis client span (emitted with the current stable
// db.system.name) to the flat fields you would persist per span.
func Example() {
	span := pcommon.NewMap()
	span.PutStr("db.system.name", "redis")
	span.PutStr("db.query.text", "GET ?")
	span.PutStr("server.address", "cache-01")
	spanName := "GET"

	res := pcommon.NewMap()
	res.PutStr("service.name", "my-order-service")
	res.PutStr("deployment.environment.name", "prod")
	res.PutStr("service.version", "1.4.0")

	fmt.Println("technology:", otlpmap.GetTechnology(span))
	fmt.Println("entity_type:", otlpmap.GetEntityType(span, ptrace.SpanKindClient))
	fmt.Println("operation:", otlpmap.GetOperationName(span, ptrace.SpanKindClient))
	fmt.Println("resource:", otlpmap.GetResourceName(span, ptrace.SpanKindClient, spanName))
	fmt.Println("service:", otlpmap.GetServiceName(res))
	fmt.Println("env:", otlpmap.GetDeploymentEnvironmentName(res))
	fmt.Println("version:", otlpmap.GetServiceVersion(res))
	// Output:
	// technology: redis
	// entity_type: cache
	// operation: redis.query
	// resource: GET ?
	// service: my-order-service
	// env: prod
	// version: 1.4.0
}

// ExampleGetHTTPRequestMethod shows the core HTTP/URL/network getters used by
// a flat storage projection. Unknown methods keep their wire value in the
// standard method_original companion column.
func ExampleGetHTTPRequestMethod() {
	span := pcommon.NewMap()
	span.PutStr("http.request.method", "_OTHER")
	span.PutStr("http.request.method_original", "PROPFIND")
	span.PutInt("http.response.status_code", 207)
	span.PutStr("url.template", "/files/{id}")
	span.PutStr("net.transport", "ip_tcp")

	status, statusOK := otlpmap.GetHTTPResponseStatusCode(span)
	fmt.Println("method:", otlpmap.GetHTTPRequestMethod(span))
	fmt.Println("method_original:", otlpmap.GetHTTPRequestMethodOriginal(span))
	fmt.Println("status:", status, statusOK)
	fmt.Println("url_template:", otlpmap.GetURLTemplate(span))
	fmt.Println("network_transport:", otlpmap.GetNetworkTransport(span))
	// Output:
	// method: _OTHER
	// method_original: PROPFIND
	// status: 207 true
	// url_template: /files/{id}
	// network_transport: tcp
}

// ExampleGetDBOperationName shows the normalized DB core columns and the
// agent-sanitized query used as the Datadog resource.
func ExampleGetDBOperationName() {
	span := pcommon.NewMap()
	span.PutStr("db.system.name", "mysql")
	span.PutStr("db.operation.name", "select")
	span.PutStr("db.collection.name", "orders")
	span.PutStr("db.namespace", "shop")
	span.PutStr("db.query.summary", "SELECT orders")
	span.PutStr("db.query.text", "SELECT * FROM orders WHERE id = ?")
	spanName := "SELECT orders"

	fmt.Println("db_operation:", otlpmap.GetDBOperationName(span))
	fmt.Println("db_collection:", otlpmap.GetDBCollectionName(span))
	fmt.Println("db_namespace:", otlpmap.GetDBNamespace(span))
	fmt.Println("db_query_summary:", otlpmap.GetDBQuerySummary(span))
	fmt.Println("db_query_text:", otlpmap.GetDBQueryText(span))
	fmt.Println("operation:", otlpmap.GetOperationName(span, ptrace.SpanKindClient))
	fmt.Println("resource:", otlpmap.GetResourceName(span, ptrace.SpanKindClient, spanName))
	// Output:
	// db_operation: select
	// db_collection: orders
	// db_namespace: shop
	// db_query_summary: SELECT orders
	// db_query_text: SELECT * FROM orders WHERE id = ?
	// operation: mysql.select
	// resource: SELECT * FROM orders WHERE id = ?
}
