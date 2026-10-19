// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"strconv"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func (m spanMigration) projectDatabase() {
	if m.work&migrationDBRemove != 0 {
		for _, key := range []string{"db.connection_string", "db.user", "db.jdbc.driver_classname", "db.cosmosdb.operation_type"} {
			m.remove(key)
		}
	}
	if m.work&migrationDBStatus != 0 {
		v, _ := m.attrs.Get("db.cosmosdb.status_code")
		if m.has("db.response.status_code") {
			m.out.Attributes.Remove("db.cosmosdb.status_code")
		} else if v.Type() == pcommon.ValueTypeInt {
			m.stringTarget("db.cosmosdb.status_code", "db.response.status_code", strconv.FormatInt(v.Int(), 10))
		} else {
			m.issue("db.cosmosdb.status_code", MigrationInvalidValue, "Cosmos DB status code must be an integer")
		}
	}
	if m.work&migrationDBQuery != 0 {
		v, _ := m.attrs.Get("db.statement")
		switch {
		case m.has("db.query.text"):
			m.out.Attributes.Remove("db.statement")
		case v.Type() != pcommon.ValueTypeStr:
			m.issue("db.statement", MigrationInvalidValue, "database query must be a string")
		case m.options.SanitizeDBQuery == nil:
			m.issue("db.statement", MigrationMissingContext, "database-specific query sanitizer is required")
		default:
			sanitized, accepted := m.options.SanitizeDBQuery(v.Str())
			if !accepted {
				m.issue("db.statement", MigrationMissingContext, "query sanitizer could not establish safe text")
			} else {
				m.stringTarget("db.statement", "db.query.text", sanitized)
			}
		}
	}
	if m.work&migrationDBTable != 0 {
		switch {
		case m.has("db.collection.name"):
			m.out.Attributes.Remove("db.sql.table")
		case m.options.DBCollection != "":
			m.stringTarget("db.sql.table", "db.collection.name", m.options.DBCollection)
		default:
			m.issue("db.sql.table", MigrationMissingContext, "application-provided single collection name is required")
		}
	}
	if m.work&migrationDBInstance != 0 {
		namespace, server := m.options.DBInstanceNamespace, m.options.DBInstanceServer
		switch {
		case namespace != "" && server != "":
			m.issue("db.instance.id", MigrationInvalidValue, "instance cannot designate both namespace and server")
		case namespace != "":
			m.stringTarget("db.instance.id", "db.namespace", namespace)
		case server != "":
			m.stringTarget("db.instance.id", "server.address", server)
		default:
			m.issue("db.instance.id", MigrationMissingContext, "database-specific instance meaning is required")
		}
	}
	if m.work&migrationDBNamespace != 0 {
		for _, key := range []string{"db.name", "db.redis.database_index", "db.mssql.instance_name", "db.cassandra.keyspace", "db.hbase.namespace"} {
			if _, ok := m.out.Attributes.Get(key); ok {
				m.issue(key, MigrationMissingContext, "valid database system and namespace source are required")
			}
		}
	}
}
