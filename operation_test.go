// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestGetOperationName(t *testing.T) {
	cases := []struct {
		name  string
		kind  ptrace.SpanKind
		attrs map[string]string
		want  string
	}{
		{"explicit wins", ptrace.SpanKindClient, map[string]string{"operation.name": "custom.op", "db.system": "mysql"}, "custom.op"},
		{"http server", ptrace.SpanKindServer, map[string]string{"http.request.method": "GET"}, "http.server.request"},
		{"http client legacy name", ptrace.SpanKindClient, map[string]string{"http.method": "GET"}, "http.client.request"},
		{"http server from status", ptrace.SpanKindServer, map[string]string{"http.response.status_code": "200"}, "http.server.request"},
		{"http client from legacy status", ptrace.SpanKindClient, map[string]string{"http.status_code": "404"}, "http.client.request"},
		{"http current empty falls back to legacy status", ptrace.SpanKindServer, map[string]string{"http.response.status_code": "", "http.status_code": "200"}, "http.server.request"},
		{"empty http status is not a marker", ptrace.SpanKindServer, map[string]string{"http.response.status_code": "", "http.status_code": ""}, "server.request"},
		{"db client new name", ptrace.SpanKindClient, map[string]string{"db.system.name": "redis"}, "redis.query"},
		{"db client current operation", ptrace.SpanKindClient, map[string]string{"db.system.name": "mysql", "db.operation.name": "select"}, "mysql.select"},
		{"db client legacy operation", ptrace.SpanKindClient, map[string]string{"db.system": "mysql", "db.operation": "insert"}, "mysql.insert"},
		{"db client current operation wins", ptrace.SpanKindClient, map[string]string{"db.system.name": "mysql", "db.operation.name": "select", "db.operation": "insert"}, "mysql.select"},
		{"db client empty current operation falls back", ptrace.SpanKindClient, map[string]string{"db.system.name": "mysql", "db.operation.name": "", "db.operation": "update"}, "mysql.update"},
		{"db client legacy alias normalized", ptrace.SpanKindClient, map[string]string{"db.system": "mssql"}, "microsoft.sql_server.query"},
		{"db client vendor fallback", ptrace.SpanKindClient, map[string]string{"db.type": "mysql"}, "mysql.query"},
		{"db server is not a query", ptrace.SpanKindServer, map[string]string{"db.system": "redis"}, "server.request"},
		{"messaging new op name", ptrace.SpanKindConsumer, map[string]string{"messaging.system": "kafka", "messaging.operation.type": "receive"}, "kafka.receive"},
		{"messaging producer kind fallback", ptrace.SpanKindProducer, map[string]string{"messaging.system": "kafka"}, "kafka.send"},
		{"messaging consumer kind fallback", ptrace.SpanKindConsumer, map[string]string{"messaging.system": "kafka"}, "kafka.process"},
		{"messaging client kind fallback", ptrace.SpanKindClient, map[string]string{"messaging.system": "kafka"}, "kafka.client"},
		{"messaging server kind fallback", ptrace.SpanKindServer, map[string]string{"messaging.system": "kafka"}, "kafka.server"},
		{"messaging internal keeps kind fallback", ptrace.SpanKindInternal, map[string]string{"messaging.system": "kafka"}, "internal"},
		{"messaging fallback precedes rpc", ptrace.SpanKindClient, map[string]string{"messaging.system": "kafka", "rpc.system": "grpc"}, "kafka.client"},
		{"grpc client", ptrace.SpanKindClient, map[string]string{"rpc.system": "grpc"}, "grpc.client.request"},
		{"grpc client new name", ptrace.SpanKindClient, map[string]string{"rpc.system.name": "grpc"}, "grpc.client.request"},
		{"dubbo legacy alias normalized", ptrace.SpanKindClient, map[string]string{"rpc.system": "apache_dubbo"}, "dubbo.client.request"},
		{"connect rpc legacy alias normalized", ptrace.SpanKindServer, map[string]string{"rpc.system": "connect_rpc"}, "connectrpc.server.request"},
		{"aws-api client with service", ptrace.SpanKindClient, map[string]string{"rpc.system": "aws-api", "rpc.service": "S3"}, "aws.S3.request"},
		{"faas server", ptrace.SpanKindServer, map[string]string{"faas.trigger": "timer"}, "timer.invoke"},
		{"graphql", ptrace.SpanKindServer, map[string]string{"graphql.operation.type": "query"}, "graphql.server.request"},
		{"generic protocol server", ptrace.SpanKindServer, map[string]string{"network.protocol.name": "amqp"}, "amqp.server.request"},
		{"generic protocol legacy name", ptrace.SpanKindClient, map[string]string{"net.protocol.name": "amqp"}, "amqp.client.request"},
		{"generic protocol current wins", ptrace.SpanKindServer, map[string]string{"network.protocol.name": "http2", "net.protocol.name": "http"}, "http2.server.request"},
		{"generic protocol current empty falls back", ptrace.SpanKindServer, map[string]string{"network.protocol.name": "", "net.protocol.name": "http"}, "http.server.request"},
		{"bare server", ptrace.SpanKindServer, map[string]string{}, "server.request"},
		{"bare client", ptrace.SpanKindClient, map[string]string{}, "client.request"},
		{"bare producer", ptrace.SpanKindProducer, map[string]string{}, "producer"},
		{"bare consumer", ptrace.SpanKindConsumer, map[string]string{}, "consumer"},
		{"internal", ptrace.SpanKindInternal, map[string]string{}, "internal"},
		{"unspecified defaults to internal", ptrace.SpanKindUnspecified, map[string]string{}, "internal"},
		{"unknown kind keeps unspecified fallback", ptrace.SpanKind(99), map[string]string{}, "unspecified"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := GetOperationName(testAttrs(c.attrs), c.kind); got != c.want {
				t.Fatalf("GetOperationName = %q, want %q", got, c.want)
			}
		})
	}
}

func TestGetOperationNameUsesEveryCanonicalAlias(t *testing.T) {
	for _, test := range officialCanonicalAliasTests() {
		legacy := test.legacy.Value.AsString()
		want := test.canonical.Value.AsString() + test.suffix
		for _, key := range []string{string(test.legacy.Key), test.currentKey} {
			name := key + "=" + legacy
			t.Run(name, func(t *testing.T) {
				attrs := testAttrs(map[string]string{key: legacy})
				if got := GetOperationName(attrs, ptrace.SpanKindClient); got != want {
					t.Fatalf("GetOperationName() = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestGetOperationNameTypedHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		key  string
		kind ptrace.SpanKind
		want string
	}{
		{name: "current", key: keyHTTPResponseStatusCode, kind: ptrace.SpanKindServer, want: "http.server.request"},
		{name: "legacy", key: keyHTTPStatusCode, kind: ptrace.SpanKindClient, want: "http.client.request"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			attrs.PutInt(test.key, 204)
			if got := GetOperationName(attrs, test.kind); got != test.want {
				t.Fatalf("GetOperationName() = %q, want %q", got, test.want)
			}
		})
	}
}
