# Project build-only guard for the canonical OpenTelemetry Registry.
# Migration rename/fallback/winner semantics intentionally do not belong in Rego.
package before_resolution

import rego.v1

deny contains violation if {
	group := input.groups[_]
	startswith(group.id, "registry.")
	group.type != "attribute_group"
	violation := {
		"id": "otelmap.registry_group_type",
		"type": "semconv_group",
		"category": "otelmap.canonical",
		"group": group.id,
	}
}
