// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestGetEntityType(t *testing.T) {
	cases := []struct {
		name  string
		kind  ptrace.SpanKind
		attrs map[string]string
		want  EntityType
	}{
		// Cache split out from DB (unlike DD's flat span.type).
		{"redis -> cache", ptrace.SpanKindClient, map[string]string{"db.system": "redis"}, EntityTypeCache},
		{"memcached new name -> cache", ptrace.SpanKindClient, map[string]string{"db.system.name": "memcached"}, EntityTypeCache},
		{"mysql -> db", ptrace.SpanKindClient, map[string]string{"db.system.name": "mysql"}, EntityTypeDB},
		{"clickhouse -> db (not collapsed like span.type)", ptrace.SpanKindClient, map[string]string{"db.system.name": "clickhouse"}, EntityTypeDB},
		{"mongodb -> db", ptrace.SpanKindClient, map[string]string{"db.system": "mongodb"}, EntityTypeDB},
		{"uppercase redis is not implicitly canonicalized", ptrace.SpanKindClient, map[string]string{"db.system": "REDIS"}, EntityTypeDB},
		{"space-padded redis is not implicitly trimmed", ptrace.SpanKindClient, map[string]string{"db.system": " redis "}, EntityTypeDB},

		{"kafka -> queue", ptrace.SpanKindProducer, map[string]string{"messaging.system": "kafka"}, EntityTypeQueue},
		{"grpc -> rpc", ptrace.SpanKindClient, map[string]string{"rpc.system": "grpc"}, EntityTypeRPC},
		{"grpc new name -> rpc", ptrace.SpanKindClient, map[string]string{"rpc.system.name": "grpc"}, EntityTypeRPC},

		// Serverless (span.type has no such value).
		{"faas trigger -> serverless", ptrace.SpanKindServer, map[string]string{"faas.trigger": "http"}, EntityTypeServerless},
		{"faas invoked -> serverless", ptrace.SpanKindClient, map[string]string{"faas.invoked_name": "fn"}, EntityTypeServerless},

		{"http server -> web", ptrace.SpanKindServer, map[string]string{"http.request.method": "GET"}, EntityTypeWeb},
		{"http client -> http", ptrace.SpanKindClient, map[string]string{"http.method": "GET"}, EntityTypeHTTP},

		{"bare server -> web", ptrace.SpanKindServer, map[string]string{}, EntityTypeWeb},
		{"bare client -> custom", ptrace.SpanKindClient, map[string]string{}, EntityTypeCustom},
		{"internal -> custom", ptrace.SpanKindInternal, map[string]string{}, EntityTypeCustom},

		// db/cache precedence beats faas/http if both somehow present.
		{"db wins over http", ptrace.SpanKindClient, map[string]string{"db.system": "redis", "http.request.method": "GET"}, EntityTypeCache},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := GetEntityType(testAttrs(c.attrs), c.kind); got != c.want {
				t.Fatalf("GetEntityType = %q, want %q", got, c.want)
			}
		})
	}
}
