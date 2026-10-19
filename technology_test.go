// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"testing"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/otel/attribute"
	semconv1_27 "go.opentelemetry.io/otel/semconv/v1.27.0"
	semconv1_41 "go.opentelemetry.io/otel/semconv/v1.41.0"
)

type canonicalAliasTest struct {
	legacy     attribute.KeyValue
	canonical  attribute.KeyValue
	currentKey string
	suffix     string
}

func officialCanonicalAliasTests() []canonicalAliasTest {
	return []canonicalAliasTest{
		{semconv1_27.DBSystemMSSQL, semconv1_41.DBSystemNameMicrosoftSQLServer, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemDynamoDB, semconv1_41.DBSystemNameAWSDynamoDB, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemCosmosDB, semconv1_41.DBSystemNameAzureCosmosDB, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemRedshift, semconv1_41.DBSystemNameAWSRedshift, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemSpanner, semconv1_41.DBSystemNameGCPSpanner, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemOracle, semconv1_41.DBSystemNameOracleDB, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemHanaDB, semconv1_41.DBSystemNameSAPHANA, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemMaxDB, semconv1_41.DBSystemNameSAPMaxDB, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemAdabas, semconv1_41.DBSystemNameSoftwareagAdabas, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemIngres, semconv1_41.DBSystemNameActianIngres, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemInformix, semconv1_41.DBSystemNameIBMInformix, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemNetezza, semconv1_41.DBSystemNameIBMNetezza, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemFirebird, semconv1_41.DBSystemNameFirebirdSQL, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemIntersystemsCache, semconv1_41.DBSystemNameIntersystemsCache, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemCache, semconv1_41.DBSystemNameIntersystemsCache, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemDB2, semconv1_41.DBSystemNameIBMDB2, keyDBSystemName, ".query"},
		{semconv1_27.DBSystemH2, semconv1_41.DBSystemNameH2database, keyDBSystemName, ".query"},
		{semconv1_27.RPCSystemApacheDubbo, semconv1_41.RPCSystemNameDubbo, keyRPCSystemName, ".client.request"},
		{semconv1_27.RPCSystemConnectRPC, semconv1_41.RPCSystemNameConnectrpc, keyRPCSystemName, ".client.request"},
	}
}

func TestGetTechnology(t *testing.T) {
	cases := []struct {
		name  string
		attrs map[string]string
		want  string
	}{
		// DB / messaging / rpc → raw system value, no semantic collapse, both
		// attribute generations.
		{"mysql", map[string]string{"db.system": "mysql"}, "mysql"},
		{"clickhouse (not collapsed to sql)", map[string]string{"db.system.name": "clickhouse"}, "clickhouse"},
		{"postgres via new name", map[string]string{"db.system.name": "postgresql"}, "postgresql"},
		{"redis", map[string]string{"db.system": "redis"}, "redis"},
		{"dd legacy db.type", map[string]string{"db.type": "mysql"}, "mysql"},
		{"kafka (not flattened to queue)", map[string]string{"messaging.system": "kafka"}, "kafka"},
		{"rabbitmq", map[string]string{"messaging.system": "rabbitmq"}, "rabbitmq"},
		{"grpc (not flattened to rpc)", map[string]string{"rpc.system": "grpc"}, "grpc"},
		{"grpc via new name", map[string]string{"rpc.system.name": "grpc"}, "grpc"},

		// Spelling normalization across semconv generations → one value per
		// technology (distinct technologies are still never merged).
		{"mssql old spelling normalized", map[string]string{"db.system": "mssql"}, "microsoft.sql_server"},
		{"sql server new spelling unchanged", map[string]string{"db.system.name": "microsoft.sql_server"}, "microsoft.sql_server"},
		{"dynamodb old spelling normalized", map[string]string{"db.system": "dynamodb"}, "aws.dynamodb"},
		{"cosmosdb old spelling normalized", map[string]string{"db.system": "cosmosdb"}, "azure.cosmosdb"},
		{"oracle old spelling normalized", map[string]string{"db.system": "oracle"}, "oracle.db"},
		{"intersystems cache legacy alias normalized", map[string]string{"db.system": "cache"}, "intersystems.cache"},
		{"db2 old spelling normalized", map[string]string{"db.system": "db2"}, "ibm.db2"},
		{"h2 old spelling normalized", map[string]string{"db.system": "h2"}, "h2database"},
		{"dubbo old spelling normalized", map[string]string{"rpc.system": "apache_dubbo"}, "dubbo"},
		{"connect rpc old spelling normalized", map[string]string{"rpc.system": "connect_rpc"}, "connectrpc"},

		// HTTP → protocol "http". Direction is deliberately not an input; role
		// (web vs http) is GetEntityType's job, not this column's.
		{"http current marker", map[string]string{"http.request.method": "GET"}, "http"},
		{"http legacy marker", map[string]string{"http.method": "GET"}, "http"},
		{"url.full only", map[string]string{"url.full": "http://x"}, "http"},

		// No backing technology → unknown (never a role value).
		{"no backing technology", map[string]string{}, "unknown"},
		{"empty system", map[string]string{"db.system.name": ""}, "unknown"},
		{"unknown vendor preserved", map[string]string{"db.system.name": "future_db"}, "future_db"},

		// Precedence: a typed system beats an HTTP marker.
		{"db beats http", map[string]string{"db.system": "redis", "http.request.method": "GET"}, "redis"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := GetTechnology(testAttrs(c.attrs)); got != c.want {
				t.Fatalf("GetTechnology = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCanonicalAliasesMatchOfficialSemconv(t *testing.T) {
	for _, test := range officialCanonicalAliasTests() {
		legacy := test.legacy.Value.AsString()
		want := test.canonical.Value.AsString()
		if got := semconvmeta.CanonicalizeTechnology(legacy); got != want {
			t.Errorf("semconvmeta.CanonicalizeTechnology(%q) = %q, want %q", legacy, got, want)
		}
	}
}
