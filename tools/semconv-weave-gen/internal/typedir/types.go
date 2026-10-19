// SPDX-License-Identifier: Apache-2.0

// Package typedir compiles checksum-locked Telemetry Schema and reviewed
// migration evidence into the private, one-hop build IR consumed by emitters.
// It is a build-time package; no type here is part of the runtime ABI.
package typedir

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	IRSchemaVersion = 2
	CanonicalURL    = "https://opentelemetry.io/schemas/1.41.0"
)

type Domain string

const (
	DomainResource  Domain = "resource"
	DomainSpan      Domain = "span"
	DomainSpanEvent Domain = "span_event"
	DomainLink      Domain = "link"
)

type MatchKind string

const (
	MatchExact  MatchKind = "exact"
	MatchPrefix MatchKind = "prefix"
)

type Predicate struct {
	SourceSpanName  string `json:"source_span_name,omitempty"`
	SourceEventName string `json:"source_event_name,omitempty"`
}

type Evidence struct {
	StableID        string `json:"stable_id"`
	AuthorityID     string `json:"authority_stable_id,omitempty"`
	Authority       string `json:"authority"`
	Locator         string `json:"locator"`
	OperationSource string `json:"operation_source,omitempty"`
	Revision        string `json:"revision,omitempty"`
	SchemaDomain    string `json:"schema_domain,omitempty"`
	Phase           string `json:"phase"`
	Operation       int    `json:"operation_index"`
	Mapping         int    `json:"mapping_index"`
	Reconciliation  string `json:"reconciliation,omitempty"`
}

type KeyRule struct {
	Domain    Domain     `json:"domain"`
	Predicate Predicate  `json:"predicate,omitempty"`
	Match     MatchKind  `json:"match"`
	Source    string     `json:"source"`
	Target    string     `json:"target"`
	Evidence  []Evidence `json:"evidence"`
}

type ValueRule struct {
	Domain      Domain     `json:"domain"`
	Predicate   Predicate  `json:"predicate,omitempty"`
	SourceKey   string     `json:"source_key"`
	TargetKey   string     `json:"target_key"`
	SourceValue string     `json:"source_value"`
	TargetValue string     `json:"target_value"`
	Evidence    []Evidence `json:"evidence"`
}

type NameRule struct {
	Domain     Domain     `json:"domain"`
	SourceName string     `json:"source_name"`
	TargetName string     `json:"target_name"`
	Evidence   []Evidence `json:"evidence"`
}

type Disposition struct {
	Domain   Domain     `json:"domain"`
	StableID string     `json:"stable_id"`
	Action   string     `json:"action"`
	Reason   string     `json:"reason"`
	Source   string     `json:"source,omitempty"`
	Evidence []Evidence `json:"evidence,omitempty"`
}

type SourceIR struct {
	SourceURL     string        `json:"source_url"`
	SourceVersion string        `json:"source_version"`
	CanonicalNoOp bool          `json:"canonical_no_op"`
	KeyRules      []KeyRule     `json:"key_rules"`
	ValueRules    []ValueRule   `json:"value_rules"`
	NameRules     []NameRule    `json:"name_rules"`
	Dispositions  []Disposition `json:"dispositions"`
}

// WeaverArtifact is one verified member of the promoted Weaver evidence
// closure. It is carried into the private IR so downstream emitters cannot
// accidentally lose the model/diff/provenance identity used by compilation.
type WeaverArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Bundle struct {
	SchemaVersion        int              `json:"schema_version"`
	CanonicalURL         string           `json:"canonical_url"`
	InputDigest          string           `json:"input_digest"`
	WeaverEvidenceSHA256 string           `json:"weaver_evidence_sha256"`
	WeaverArtifacts      []WeaverArtifact `json:"weaver_artifacts"`
	Sources              []SourceIR       `json:"sources"`
}

func (bundle Bundle) Validate() error {
	if bundle.SchemaVersion != IRSchemaVersion || bundle.CanonicalURL != CanonicalURL ||
		len(bundle.InputDigest) != 64 || len(bundle.WeaverEvidenceSHA256) != 64 {
		return errors.New("typed IR identity is invalid")
	}
	if _, err := hex.DecodeString(bundle.InputDigest); err != nil ||
		bundle.InputDigest != strings.ToLower(bundle.InputDigest) {
		return errors.New("typed IR input digest is not lowercase SHA-256")
	}
	if _, err := hex.DecodeString(bundle.WeaverEvidenceSHA256); err != nil ||
		bundle.WeaverEvidenceSHA256 != strings.ToLower(bundle.WeaverEvidenceSHA256) {
		return errors.New("typed IR Weaver evidence digest is not lowercase SHA-256")
	}
	if len(bundle.WeaverArtifacts) != 7 {
		return fmt.Errorf("typed IR Weaver artifact count = %d, want 7", len(bundle.WeaverArtifacts))
	}
	previousArtifact := ""
	for _, artifact := range bundle.WeaverArtifacts {
		if artifact.Path == "" || artifact.Path <= previousArtifact || len(artifact.SHA256) != 64 {
			return errors.New("typed IR Weaver artifacts are incomplete, duplicated, or unsorted")
		}
		if _, err := hex.DecodeString(artifact.SHA256); err != nil ||
			artifact.SHA256 != strings.ToLower(artifact.SHA256) {
			return fmt.Errorf("typed IR Weaver artifact %q has invalid SHA-256", artifact.Path)
		}
		previousArtifact = artifact.Path
	}
	if len(bundle.Sources) != 3 {
		return fmt.Errorf("typed IR source count = %d, want 3", len(bundle.Sources))
	}
	seen := make(map[string]struct{}, len(bundle.Sources))
	expectedURLs := []string{
		CanonicalURL,
		"https://opentelemetry.io/schemas/1.7.0",
		"https://opentelemetry.io/schemas/1.4.0",
	}
	for sourceIndex, source := range bundle.Sources {
		if source.SourceURL == "" || source.SourceVersion == "" {
			return errors.New("typed IR source identity is incomplete")
		}
		if source.SourceURL != expectedURLs[sourceIndex] {
			return fmt.Errorf("typed IR source order[%d] = %q, want %q", sourceIndex, source.SourceURL, expectedURLs[sourceIndex])
		}
		if _, ok := seen[source.SourceURL]; ok {
			return fmt.Errorf("duplicate source URL %q", source.SourceURL)
		}
		seen[source.SourceURL] = struct{}{}
		if source.CanonicalNoOp {
			if source.SourceURL != CanonicalURL || len(source.KeyRules) != 0 ||
				len(source.ValueRules) != 0 || len(source.NameRules) != 0 {
				return errors.New("canonical source is not a no-op")
			}
			continue
		}
		if source.SourceURL == CanonicalURL {
			return errors.New("canonical source lacks no-op marker")
		}
		linkCount := 0
		for _, disposition := range source.Dispositions {
			if disposition.Domain == DomainLink && disposition.Action == "pass_through" {
				linkCount++
			}
		}
		if linkCount != 1 {
			return fmt.Errorf("source %q Link pass-through count = %d, want 1", source.SourceURL, linkCount)
		}
		for _, rule := range source.KeyRules {
			if !validRuntimeDomain(rule.Domain) || (rule.Match != MatchExact && rule.Match != MatchPrefix) ||
				rule.Source == "" || rule.Target == "" || len(rule.Evidence) == 0 {
				return fmt.Errorf("source %q has invalid key rule %#v", source.SourceURL, rule)
			}
		}
		for _, rule := range source.ValueRules {
			if !validRuntimeDomain(rule.Domain) || rule.SourceKey == "" || rule.TargetKey == "" ||
				rule.SourceValue == "" || rule.TargetValue == "" || len(rule.Evidence) == 0 {
				return fmt.Errorf("source %q has invalid value rule %#v", source.SourceURL, rule)
			}
		}
		for _, rule := range source.NameRules {
			if (rule.Domain != DomainSpan && rule.Domain != DomainSpanEvent) ||
				rule.SourceName == "" || rule.TargetName == "" || len(rule.Evidence) == 0 {
				return fmt.Errorf("source %q has invalid name rule %#v", source.SourceURL, rule)
			}
		}
	}
	return nil
}

func validRuntimeDomain(domain Domain) bool {
	return domain == DomainResource || domain == DomainSpan || domain == DomainSpanEvent
}

// Encode emits the canonical byte representation consumed by later stories.
// Struct field order and pre-sorted slices make json.MarshalIndent stable.
func Encode(bundle Bundle) ([]byte, error) {
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
