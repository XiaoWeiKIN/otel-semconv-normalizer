// Package semconvweave exposes the immutable runtime contract for translating
// versioned OpenTelemetry trace semantics into the provider's canonical form.
//
// The hand-written ABI deliberately compiles without generated rule data.
package semconvweave

const (
	// RuntimeABIVersion changes only when a public type, enum value, struct
	// shape, method signature, or documented decision semantic breaks.
	RuntimeABIVersion uint16 = 1
)

// WeaveID identifies a weave in the current binary. Generated legacy IDs are
// process-local implementation details and must never be persisted.
type WeaveID uint16

const (
	// WeaveUnknown is the safe zero value for missing generated knowledge.
	WeaveUnknown WeaveID = iota
	// WeaveMissing means the producer supplied no effective Schema URL.
	WeaveMissing
	// WeaveCanonical means the producer declared the exact canonical URL.
	WeaveCanonical
)

// SchemaState is the bounded, low-cardinality state exposed to callers.
type SchemaState uint8

const (
	SchemaUnknown SchemaState = iota
	SchemaMissing
	SchemaCanonical
	SchemaSupportedLegacy
)

// String returns the stable observation label for a schema state.
func (s SchemaState) String() string {
	switch s {
	case SchemaMissing:
		return "missing"
	case SchemaCanonical:
		return "canonical"
	case SchemaSupportedLegacy:
		return "supported-legacy"
	default:
		return "unknown"
	}
}

// AttributeAction tells a translation caller how to emit one source attribute.
type AttributeAction uint8

const (
	// AttributeSuppress emits nothing for the source attribute.
	AttributeSuppress AttributeAction = iota
	// AttributeKeepValue emits the original pcommon.Value under Key.
	AttributeKeepValue
	// AttributeReplaceString emits StringValue under Key.
	AttributeReplaceString
)

// AttributeDecision is the complete ABI v1 result for one source attribute.
// Field name, type, order, and semantics are compatibility-sensitive.
type AttributeDecision struct {
	Key         string
	StringValue string
	Action      AttributeAction
}
