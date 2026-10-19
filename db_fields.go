// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// GetDBOperationName returns db.operation.name, falling back to the legacy
// db.operation attribute.
func GetDBOperationName(signalAttrs pcommon.Map) string {
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldDBOperationName)
}

// GetDBCollectionName returns db.collection.name, consolidating the legacy SQL,
// Cassandra, MongoDB, and Cosmos DB collection attributes in that order.
func GetDBCollectionName(signalAttrs pcommon.Map) string {
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldDBCollectionName)
}

// GetDBNamespace returns db.namespace, consolidating the legacy database name,
// Redis database index, and SQL Server instance attributes in that order.
func GetDBNamespace(signalAttrs pcommon.Map) string {
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldDBNamespace)
}

// GetDBQuerySummary returns the current low-cardinality db.query.summary value.
func GetDBQuerySummary(signalAttrs pcommon.Map) string {
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldDBQuerySummary)
}

// GetDBQueryText returns db.query.text, falling back to the legacy
// db.statement attribute. The value is raw and may contain sensitive data.
func GetDBQueryText(signalAttrs pcommon.Map) string {
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldDBQueryText)
}
