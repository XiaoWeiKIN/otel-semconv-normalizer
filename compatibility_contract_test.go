// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"reflect"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestCompatibilityProjectionContract(t *testing.T) {
	tests := []struct {
		name       string
		attrs      map[string]string
		kind       ptrace.SpanKind
		fallback   string
		technology string
		entityType EntityType
		operation  string
		resource   string
	}{
		{
			name:       "new only",
			attrs:      map[string]string{"db.system.name": "mysql", "db.query.text": "SELECT current"},
			kind:       ptrace.SpanKindClient,
			fallback:   "fallback",
			technology: "mysql",
			entityType: EntityTypeDB,
			operation:  "mysql.query",
			resource:   "SELECT current",
		},
		{
			name:       "legacy only",
			attrs:      map[string]string{"db.system": "mssql", "db.statement": "SELECT legacy"},
			kind:       ptrace.SpanKindClient,
			fallback:   "fallback",
			technology: "microsoft.sql_server",
			entityType: EntityTypeDB,
			operation:  "microsoft.sql_server.query",
			resource:   "SELECT legacy",
		},
		{
			name:       "both same",
			attrs:      map[string]string{"db.system.name": "mysql", "db.system": "mysql", "db.query.text": "SELECT current", "db.statement": "SELECT legacy"},
			kind:       ptrace.SpanKindClient,
			fallback:   "fallback",
			technology: "mysql",
			entityType: EntityTypeDB,
			operation:  "mysql.query",
			resource:   "SELECT current",
		},
		{
			name:       "both conflict current wins",
			attrs:      map[string]string{"db.system.name": "postgresql", "db.system": "mysql", "db.query.text": "SELECT current", "db.statement": "SELECT legacy"},
			kind:       ptrace.SpanKindClient,
			fallback:   "fallback",
			technology: "postgresql",
			entityType: EntityTypeDB,
			operation:  "postgresql.query",
			resource:   "SELECT current",
		},
		{
			name:       "present empty falls back",
			attrs:      map[string]string{"db.system.name": "", "db.system": "mssql", "db.query.text": "", "db.statement": "SELECT legacy"},
			kind:       ptrace.SpanKindClient,
			fallback:   "fallback",
			technology: "microsoft.sql_server",
			entityType: EntityTypeDB,
			operation:  "microsoft.sql_server.query",
			resource:   "SELECT legacy",
		},
		{
			name:       "unknown value passes through",
			attrs:      map[string]string{"db.system.name": "future_db", "db.query.text": "SELECT future"},
			kind:       ptrace.SpanKindClient,
			fallback:   "fallback",
			technology: "future_db",
			entityType: EntityTypeDB,
			operation:  "future_db.query",
			resource:   "SELECT future",
		},
		{
			name:       "vendor fallback is full library compatibility",
			attrs:      map[string]string{"db.type": "mysql", "db.statement": "SELECT vendor"},
			kind:       ptrace.SpanKindClient,
			fallback:   "fallback",
			technology: "mysql",
			entityType: EntityTypeDB,
			operation:  "mysql.query",
			resource:   "SELECT vendor",
		},
		{
			name:       "rpc new only",
			attrs:      map[string]string{"rpc.system.name": "dubbo", "rpc.method": "orders.Orders/Get"},
			kind:       ptrace.SpanKindClient,
			fallback:   "fallback",
			technology: "dubbo",
			entityType: EntityTypeRPC,
			operation:  "dubbo.client.request",
			resource:   "orders.Orders/Get",
		},
		{
			name:       "rpc legacy only",
			attrs:      map[string]string{"rpc.system": "apache_dubbo", "rpc.service": "orders.Orders", "rpc.method": "Get"},
			kind:       ptrace.SpanKindClient,
			fallback:   "fallback",
			technology: "dubbo",
			entityType: EntityTypeRPC,
			operation:  "dubbo.client.request",
			resource:   "Get orders.Orders",
		},
		{
			name:       "legacy network protocol",
			attrs:      map[string]string{"net.protocol.name": "amqp"},
			kind:       ptrace.SpanKindServer,
			fallback:   "fallback",
			technology: "unknown",
			entityType: EntityTypeWeb,
			operation:  "amqp.server.request",
			resource:   "fallback",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attrs := testAttrs(test.attrs)
			before := attrs.AsRaw()

			if got := GetTechnology(attrs); got != test.technology {
				t.Errorf("GetTechnology() = %q, want %q", got, test.technology)
			}
			if got := GetEntityType(attrs, test.kind); got != test.entityType {
				t.Errorf("GetEntityType() = %q, want %q", got, test.entityType)
			}
			if got := GetOperationName(attrs, test.kind); got != test.operation {
				t.Errorf("GetOperationName() = %q, want %q", got, test.operation)
			}
			if got := GetResourceName(attrs, test.kind, test.fallback); got != test.resource {
				t.Errorf("GetResourceName() = %q, want %q", got, test.resource)
			}

			if after := attrs.AsRaw(); !reflect.DeepEqual(after, before) {
				t.Fatalf("input attributes mutated: before=%v after=%v", before, after)
			}
		})
	}
}
