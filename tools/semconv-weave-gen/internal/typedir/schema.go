// SPDX-License-Identifier: Apache-2.0

package typedir

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	ast10 "go.opentelemetry.io/otel/schema/v1.0/ast"
	schema11 "go.opentelemetry.io/otel/schema/v1.1"
	ast11 "go.opentelemetry.io/otel/schema/v1.1/ast"
	types11 "go.opentelemetry.io/otel/schema/v1.1/types"
)

type schemaFact struct {
	kind       operationKind
	domain     Domain
	source     string
	target     string
	spanNames  []string
	eventNames []string
	revision   string
	yamlDomain string
	phase      string
	operation  int
	mapping    int
}

type semanticVersion struct {
	major int
	minor int
	patch int
	text  string
}

func parseSchema(data []byte) (*ast11.Schema, error) {
	document, err := schema11.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("strict parse Telemetry Schema: %w", err)
	}
	if document.SchemaURL != CanonicalURL || document.FileFormat != "1.1.0" {
		return nil, fmt.Errorf("Telemetry Schema identity = %q/%q, want 1.1.0/%s", document.FileFormat, document.SchemaURL, CanonicalURL)
	}
	return document, nil
}

func schemaFacts(document *ast11.Schema, sourceVersion string) ([]schemaFact, error) {
	source, err := parseSemanticVersion(sourceVersion)
	if err != nil {
		return nil, err
	}
	revisions := make([]semanticVersion, 0, len(document.Versions))
	for raw := range document.Versions {
		version, err := parseSemanticVersion(string(raw))
		if err != nil {
			return nil, fmt.Errorf("Schema revision %q: %w", raw, err)
		}
		if compareVersion(version, source) > 0 {
			revisions = append(revisions, version)
		}
	}
	sort.Slice(revisions, func(i, j int) bool { return compareVersion(revisions[i], revisions[j]) < 0 })

	var result []schemaFact
	for _, revision := range revisions {
		definition := document.Versions[types11.TelemetryVersion(revision.text)]
		// Domain order is an invariant: Resource, Span, Span Event. Within each
		// domain the common `all` phase precedes the signal-specific phase.
		result = append(result, attributeFacts(revision.text, "all", "schema_all", DomainResource, definition.All)...)
		result = append(result, attributeFacts(revision.text, "resources", "schema_signal", DomainResource, definition.Resources)...)
		result = append(result, attributeFacts(revision.text, "all", "schema_all", DomainSpan, definition.All)...)
		result = append(result, spanFacts(revision.text, definition.Spans)...)
		result = append(result, attributeFacts(revision.text, "all", "schema_all", DomainSpanEvent, definition.All)...)
		result = append(result, spanEventFacts(revision.text, definition.SpanEvents)...)
	}
	return result, nil
}

func attributeFacts(revision, yamlDomain, phase string, domain Domain, attributes ast10.Attributes) []schemaFact {
	var result []schemaFact
	for operationIndex, change := range attributes.Changes {
		if change.RenameAttributes == nil {
			continue
		}
		result = append(result, mapFacts(
			revision, yamlDomain, phase, domain, operationIndex,
			change.RenameAttributes.AttributeMap, nil, nil,
		)...)
	}
	return result
}

func spanFacts(revision string, spans ast10.Spans) []schemaFact {
	var result []schemaFact
	for operationIndex, change := range spans.Changes {
		if change.RenameAttributes == nil {
			continue
		}
		spanNames := make([]string, len(change.RenameAttributes.ApplyToSpans))
		for index, name := range change.RenameAttributes.ApplyToSpans {
			spanNames[index] = string(name)
		}
		sort.Strings(spanNames)
		result = append(result, mapFacts(
			revision, "spans", "schema_signal", DomainSpan, operationIndex,
			change.RenameAttributes.AttributeMap, spanNames, nil,
		)...)
	}
	return result
}

func spanEventFacts(revision string, events ast10.SpanEvents) []schemaFact {
	var result []schemaFact
	for operationIndex, change := range events.Changes {
		if change.RenameEvents != nil {
			keys := sortedMapKeys(change.RenameEvents.EventNameMap)
			for mappingIndex, source := range keys {
				result = append(result, schemaFact{
					kind: operationName, domain: DomainSpanEvent,
					source: source, target: change.RenameEvents.EventNameMap[source],
					revision: revision, yamlDomain: "span_events", phase: "schema_name",
					operation: operationIndex, mapping: mappingIndex,
				})
			}
		}
		if change.RenameAttributes == nil {
			continue
		}
		spanNames := make([]string, len(change.RenameAttributes.ApplyToSpans))
		for index, name := range change.RenameAttributes.ApplyToSpans {
			spanNames[index] = string(name)
		}
		eventNames := make([]string, len(change.RenameAttributes.ApplyToEvents))
		for index, name := range change.RenameAttributes.ApplyToEvents {
			eventNames[index] = string(name)
		}
		sort.Strings(spanNames)
		sort.Strings(eventNames)
		result = append(result, mapFacts(
			revision, "span_events", "schema_signal", DomainSpanEvent, operationIndex,
			change.RenameAttributes.AttributeMap, spanNames, eventNames,
		)...)
	}
	return result
}

func mapFacts(
	revision, yamlDomain, phase string,
	domain Domain,
	operationIndex int,
	attributeMap ast10.AttributeMap,
	spanNames, eventNames []string,
) []schemaFact {
	keys := sortedMapKeys(attributeMap)
	result := make([]schemaFact, 0, len(keys))
	for mappingIndex, source := range keys {
		result = append(result, schemaFact{
			kind: operationKey, domain: domain, source: source, target: attributeMap[source],
			spanNames: append([]string(nil), spanNames...), eventNames: append([]string(nil), eventNames...),
			revision: revision, yamlDomain: yamlDomain, phase: phase,
			operation: operationIndex, mapping: mappingIndex,
		})
	}
	return result
}

func sortedMapKeys[T ~map[string]string](values T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parseSemanticVersion(raw string) (semanticVersion, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return semanticVersion{}, fmt.Errorf("invalid semantic version %q", raw)
	}
	values := [3]int{}
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return semanticVersion{}, fmt.Errorf("invalid semantic version %q", raw)
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return semanticVersion{}, fmt.Errorf("invalid semantic version %q", raw)
		}
		values[index] = value
	}
	return semanticVersion{major: values[0], minor: values[1], patch: values[2], text: raw}, nil
}

func compareVersion(left, right semanticVersion) int {
	if left.major != right.major {
		return left.major - right.major
	}
	if left.minor != right.minor {
		return left.minor - right.minor
	}
	return left.patch - right.patch
}
