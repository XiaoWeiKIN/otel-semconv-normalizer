package semconvweave

import "go.opentelemetry.io/collector/pdata/pcommon"

const canonicalSchemaURL = "https://opentelemetry.io/schemas/1.41.0"

// MayTranslateResourceAttribute reports whether sourceKey may have a generated
// Resource schema translation. A false result guarantees that callers can keep
// the source attribute; a true result may be a conservative false positive.
func MayTranslateResourceAttribute(sourceKey string) bool {
	return mayTranslateAttribute(generatedDomainResource, sourceKey)
}

// MayTranslateSpanAttribute is the Span counterpart of
// MayTranslateResourceAttribute.
func MayTranslateSpanAttribute(sourceKey string) bool {
	switch sourceKey {
	case "http.method", "db.name", "db.cassandra.keyspace", "db.hbase.namespace", "db.redis.database_index", "db.mssql.instance_name":
		return true
	}
	return mayTranslateAttribute(generatedDomainSpan, sourceKey)
}

// MayTranslateEventAttribute is the Span Event counterpart of
// MayTranslateResourceAttribute.
func MayTranslateEventAttribute(sourceKey string) bool {
	return mayTranslateAttribute(generatedDomainSpanEvent, sourceKey)
}

// Resolve maps an exact Schema URL to its immutable generated weave.
func Resolve(schemaURL string) WeaveID {
	switch schemaURL {
	case "":
		return WeaveMissing
	case canonicalSchemaURL:
		return WeaveCanonical
	default:
		return resolveGenerated(schemaURL)
	}
}

// State returns a bounded state for every possible WeaveID.
func (w WeaveID) State() SchemaState {
	switch w {
	case WeaveMissing:
		return SchemaMissing
	case WeaveCanonical:
		return SchemaCanonical
	default:
		if isGeneratedWeave(w) {
			return SchemaSupportedLegacy
		}
		return SchemaUnknown
	}
}

// ResourceAttribute returns a read-only schema translation decision.
func (w WeaveID) ResourceAttribute(
	attrs pcommon.Map,
	sourceKey string,
	sourceValue pcommon.Value,
) AttributeDecision {
	return translateAttribute(
		w, generatedDomainResource, attrs, "", "", sourceKey, sourceValue,
	)
}

// SpanAttribute returns a read-only schema translation decision for a Span.
func (w WeaveID) SpanAttribute(
	attrs pcommon.Map,
	sourceSpanName string,
	sourceKey string,
	sourceValue pcommon.Value,
) AttributeDecision {
	if decision, ok := semanticSpanAttribute(attrs, sourceKey, sourceValue); ok {
		return decision
	}
	return translateAttribute(
		w, generatedDomainSpan, attrs, sourceSpanName, "", sourceKey, sourceValue,
	)
}

// EventAttribute returns a read-only schema translation decision for a Span Event.
func (w WeaveID) EventAttribute(
	attrs pcommon.Map,
	sourceSpanName string,
	sourceEventName string,
	sourceKey string,
	sourceValue pcommon.Value,
) AttributeDecision {
	return translateAttribute(
		w, generatedDomainSpanEvent, attrs,
		sourceSpanName, sourceEventName, sourceKey, sourceValue,
	)
}

// SpanName applies an approved static Span name rule when one exists.
func (w WeaveID) SpanName(source string) string {
	return translateName(w, generatedDomainSpan, source)
}

// EventName applies an approved static Event name rule when one exists.
func (w WeaveID) EventName(_ string, sourceEventName string) string {
	return translateName(w, generatedDomainSpanEvent, sourceEventName)
}

func keepSource(sourceKey string) AttributeDecision {
	return AttributeDecision{
		Key:    sourceKey,
		Action: AttributeKeepValue,
	}
}
