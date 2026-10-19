// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"testing"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv1_25 "go.opentelemetry.io/otel/semconv/v1.25.0"
	semconv1_41 "go.opentelemetry.io/otel/semconv/v1.41.0"
)

func TestDBFieldKeysMatchOfficialSemconv(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "db operation name", got: keyDBOperationName, want: string(semconv1_41.DBOperationNameKey)},
		{name: "legacy db operation", got: keyDBOperation, want: string(semconv1_25.DBOperationKey)},
		{name: "db collection name", got: keyDBCollectionName, want: string(semconv1_41.DBCollectionNameKey)},
		{name: "legacy db sql table", got: keyDBSQLTable, want: string(semconv1_25.DBSQLTableKey)},
		{name: "legacy db cassandra table", got: keyDBCassandraTable, want: string(semconv1_25.DBCassandraTableKey)},
		{name: "legacy db mongodb collection", got: keyDBMongoDBCollection, want: string(semconv1_25.DBMongoDBCollectionKey)},
		{name: "legacy db cosmosdb container", got: keyDBCosmosDBContainer, want: string(semconv1_25.DBCosmosDBContainerKey)},
		{name: "db namespace", got: keyDBNamespace, want: string(semconv1_41.DBNamespaceKey)},
		{name: "legacy db name", got: keyDBName, want: string(semconv1_25.DBNameKey)},
		{name: "legacy db redis database index", got: keyDBRedisDatabaseIndex, want: string(semconv1_25.DBRedisDBIndexKey)},
		{name: "legacy db mssql instance name", got: keyDBMSSQLInstanceName, want: string(semconv1_25.DBMSSQLInstanceNameKey)},
		{name: "db query summary", got: keyDBQuerySummary, want: string(semconv1_41.DBQuerySummaryKey)},
		{name: "db query text", got: keyDBQueryText, want: string(semconv1_41.DBQueryTextKey)},
		{name: "legacy db statement", got: keyDBStatement, want: string(semconv1_25.DBStatementKey)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("key = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestDBFieldRegistryShape(t *testing.T) {
	tests := []struct {
		id       semconvmeta.FieldID
		name     string
		keys     [5]string
		keyCount uint8
	}{
		{
			id:       semconvmeta.FieldDBOperationName,
			name:     keyDBOperationName,
			keys:     [5]string{keyDBOperationName, keyDBOperation},
			keyCount: 2,
		},
		{
			id:   semconvmeta.FieldDBCollectionName,
			name: keyDBCollectionName,
			keys: [5]string{
				keyDBCollectionName,
				keyDBSQLTable,
				keyDBCassandraTable,
				keyDBMongoDBCollection,
				keyDBCosmosDBContainer,
			},
			keyCount: 5,
		},
		{
			id:   semconvmeta.FieldDBNamespace,
			name: keyDBNamespace,
			keys: [5]string{
				keyDBNamespace,
				keyDBName,
				keyDBRedisDatabaseIndex,
				keyDBMSSQLInstanceName,
			},
			keyCount: 4,
		},
		{
			id:       semconvmeta.FieldDBQuerySummary,
			name:     keyDBQuerySummary,
			keys:     [5]string{keyDBQuerySummary},
			keyCount: 1,
		},
		{
			id:       semconvmeta.FieldDBQueryText,
			name:     keyDBQueryText,
			keys:     [5]string{keyDBQueryText, keyDBStatement},
			keyCount: 2,
		},
	}

	if semconvmeta.CoreFieldCount != 15 {
		t.Fatalf("CoreFieldCount = %d, want 15", semconvmeta.CoreFieldCount)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			field := semconvmeta.Field(test.id)
			if string(field.Concept()) != test.name {
				t.Errorf("concept = %q, want %q", field.Concept(), test.name)
			}
			if field.SourceKeys() != test.keys {
				t.Errorf("sourceKeys = %q, want %q", field.SourceKeys(), test.keys)
			}
			if field.SourceKeyCount() != test.keyCount {
				t.Errorf("sourceKeyCount = %d, want %d", field.SourceKeyCount(), test.keyCount)
			}
			if field.ValueType() != semconvmeta.ValueTypeString {
				t.Errorf("valueType = %d, want %d", field.ValueType(), semconvmeta.ValueTypeString)
			}
			if field.ValuePolicy() != semconvmeta.ValueIdentity {
				t.Errorf("valuePolicy = %d, want %d", field.ValuePolicy(), semconvmeta.ValueIdentity)
			}
		})
	}
}

func TestGetDBFieldsCurrentNames(t *testing.T) {
	attrs := testAttrs(map[string]string{
		keyDBOperationName:  "SELECT",
		keyDBCollectionName: "orders",
		keyDBNamespace:      "shop",
		keyDBQuerySummary:   "SELECT orders",
		keyDBQueryText:      "SELECT * FROM orders WHERE id = 42",
	})

	if got := GetDBOperationName(attrs); got != "SELECT" {
		t.Errorf("GetDBOperationName() = %q, want SELECT", got)
	}
	if got := GetDBCollectionName(attrs); got != "orders" {
		t.Errorf("GetDBCollectionName() = %q, want orders", got)
	}
	if got := GetDBNamespace(attrs); got != "shop" {
		t.Errorf("GetDBNamespace() = %q, want shop", got)
	}
	if got := GetDBQuerySummary(attrs); got != "SELECT orders" {
		t.Errorf("GetDBQuerySummary() = %q, want SELECT orders", got)
	}
	if got := GetDBQueryText(attrs); got != "SELECT * FROM orders WHERE id = 42" {
		t.Errorf("GetDBQueryText() = %q", got)
	}
}

func TestGetDBFieldCompatibility(t *testing.T) {
	tests := []struct {
		name   string
		getter func(pcommon.Map) string
		attrs  map[string]string
		want   string
	}{
		{
			name:   "operation current wins",
			getter: GetDBOperationName,
			attrs:  map[string]string{keyDBOperationName: "select", keyDBOperation: "insert"},
			want:   "select",
		},
		{
			name:   "operation empty current falls back",
			getter: GetDBOperationName,
			attrs:  map[string]string{keyDBOperationName: "", keyDBOperation: "insert"},
			want:   "insert",
		},
		{
			name:   "collection current wins",
			getter: GetDBCollectionName,
			attrs:  map[string]string{keyDBCollectionName: "current", keyDBSQLTable: "legacy"},
			want:   "current",
		},
		{
			name:   "collection empty current falls back",
			getter: GetDBCollectionName,
			attrs:  map[string]string{keyDBCollectionName: "", keyDBSQLTable: "orders"},
			want:   "orders",
		},
		{
			name:   "collection sql table",
			getter: GetDBCollectionName,
			attrs:  map[string]string{keyDBSQLTable: "orders"},
			want:   "orders",
		},
		{
			name:   "collection cassandra table",
			getter: GetDBCollectionName,
			attrs:  map[string]string{keyDBCassandraTable: "events"},
			want:   "events",
		},
		{
			name:   "collection mongodb collection",
			getter: GetDBCollectionName,
			attrs:  map[string]string{keyDBMongoDBCollection: "users"},
			want:   "users",
		},
		{
			name:   "collection cosmosdb container",
			getter: GetDBCollectionName,
			attrs:  map[string]string{keyDBCosmosDBContainer: "items"},
			want:   "items",
		},
		{
			name:   "collection consolidation order",
			getter: GetDBCollectionName,
			attrs:  map[string]string{keyDBSQLTable: "sql", keyDBCassandraTable: "cassandra"},
			want:   "sql",
		},
		{
			name:   "namespace current wins",
			getter: GetDBNamespace,
			attrs:  map[string]string{keyDBNamespace: "current", keyDBName: "legacy"},
			want:   "current",
		},
		{
			name:   "namespace empty current falls back",
			getter: GetDBNamespace,
			attrs:  map[string]string{keyDBNamespace: "", keyDBName: "shop"},
			want:   "shop",
		},
		{
			name:   "namespace database name",
			getter: GetDBNamespace,
			attrs:  map[string]string{keyDBName: "shop"},
			want:   "shop",
		},
		{
			name:   "namespace mssql instance",
			getter: GetDBNamespace,
			attrs:  map[string]string{keyDBMSSQLInstanceName: "primary"},
			want:   "primary",
		},
		{
			name:   "query summary current",
			getter: GetDBQuerySummary,
			attrs:  map[string]string{keyDBQuerySummary: "SELECT orders"},
			want:   "SELECT orders",
		},
		{
			name:   "query text current wins",
			getter: GetDBQueryText,
			attrs:  map[string]string{keyDBQueryText: "current", keyDBStatement: "legacy"},
			want:   "current",
		},
		{
			name:   "query text empty current falls back",
			getter: GetDBQueryText,
			attrs:  map[string]string{keyDBQueryText: "", keyDBStatement: "legacy"},
			want:   "legacy",
		},
		{
			name:   "query text legacy",
			getter: GetDBQueryText,
			attrs:  map[string]string{keyDBStatement: "SELECT 1"},
			want:   "SELECT 1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.getter(testAttrs(test.attrs)); got != test.want {
				t.Fatalf("getter() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGetDBNamespaceLegacyRedisIndex(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutInt(keyDBRedisDatabaseIndex, 3)
	if got := GetDBNamespace(attrs); got != "3" {
		t.Fatalf("GetDBNamespace() = %q, want 3", got)
	}
}

func TestDBNamesUseAgentSanitizedQueryText(t *testing.T) {
	attrs := testAttrs(map[string]string{
		keyDBSystemName:     "mysql",
		keyDBOperationName:  "select",
		keyDBCollectionName: "orders",
		keyDBQuerySummary:   "SELECT orders",
		keyDBQueryText:      "SELECT * FROM orders WHERE email = ?",
	})

	if got := GetOperationName(attrs, ptrace.SpanKindClient); got != "mysql.select" {
		t.Fatalf("GetOperationName() = %q, want mysql.select", got)
	}
	if got := GetResourceName(attrs, ptrace.SpanKindClient, "fallback"); got != "SELECT * FROM orders WHERE email = ?" {
		t.Fatalf("GetResourceName() = %q, want sanitized query", got)
	}
	if got := GetDBQueryText(attrs); got != "SELECT * FROM orders WHERE email = ?" {
		t.Fatalf("GetDBQueryText() = %q", got)
	}
}

func TestRemovedDBAttributesAreNeverConsumed(t *testing.T) {
	attrs := testAttrs(map[string]string{
		"db.connection_string": "postgresql://alice:secret@example.test/shop",
		"db.user":              "alice",
	})

	getters := []struct {
		name string
		get  func(pcommon.Map) string
	}{
		{name: "operation", get: GetDBOperationName},
		{name: "collection", get: GetDBCollectionName},
		{name: "namespace", get: GetDBNamespace},
		{name: "query summary", get: GetDBQuerySummary},
		{name: "query text", get: GetDBQueryText},
	}
	for _, getter := range getters {
		t.Run(getter.name, func(t *testing.T) {
			if got := getter.get(attrs); got != "" {
				t.Fatalf("getter() = %q, want empty", got)
			}
		})
	}

	if got := GetOperationName(attrs, ptrace.SpanKindClient); got != "client.request" {
		t.Fatalf("GetOperationName() = %q, want client.request", got)
	}
	if got := GetResourceName(attrs, ptrace.SpanKindClient, "fallback"); got != "fallback" {
		t.Fatalf("GetResourceName() = %q, want fallback", got)
	}

	for id := semconvmeta.FieldID(0); id < semconvmeta.FieldCount; id++ {
		field := semconvmeta.Field(id)
		for i := uint8(0); i < field.SourceKeyCount(); i++ {
			switch field.SourceKeys()[i] {
			case "db.connection_string", "db.user":
				t.Fatalf("semconvmeta.Field(%d) consumes removed attribute %q", id, field.SourceKeys()[i])
			}
		}
	}
}
