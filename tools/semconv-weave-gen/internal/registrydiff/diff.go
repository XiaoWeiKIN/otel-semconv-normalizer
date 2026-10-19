// SPDX-License-Identifier: Apache-2.0

// Package registrydiff provides the single typed interpretation of the pinned
// Weaver Registry diff used by audit generation and Typed IR compilation.
package registrydiff

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const Interval = "v1.26.0-to-v1.41.0"

// Candidate is one Registry attribute change. Index is its stable position in
// Weaver's canonical, byte-stable registry_attributes array.
type Candidate struct {
	Index  int
	Type   string
	Source string
	Target string
	Note   string
}

// StableID binds the diff interval, category, stable array index and source
// identity. It is shared by inventory generation and compiler reconciliation.
func (candidate Candidate) StableID() string {
	return fmt.Sprintf(
		"weaver-diff:%s:registry_attributes:%04d:%s",
		Interval, candidate.Index, candidate.Source,
	)
}

func (candidate Candidate) Locator() string {
	return fmt.Sprintf("changes.registry_attributes[%d]", candidate.Index)
}

func (candidate Candidate) Summary() string {
	summary := candidate.Type + " " + candidate.Source
	if candidate.Target != "" {
		summary += " -> " + candidate.Target
	}
	if note := strings.TrimSpace(candidate.Note); note != "" {
		summary += " | " + note
	}
	return summary
}

func (candidate Candidate) IsMigrationObligation() bool {
	return candidate.Type != "added"
}

func (candidate Candidate) HTTPRelated() bool {
	return related(candidate.Source, []string{
		"http.", "url.", "net.", "network.", "client.", "server.", "user_agent.",
	})
}

func (candidate Candidate) DBRPCRelated() bool {
	return related(candidate.Source, []string{
		"db.", "rpc.", "cassandra.", "azure.cosmosdb.", "azure.client.",
		"elasticsearch.", "jsonrpc.", "message.",
	})
}

type Document struct {
	RegistryAttributes []Candidate
}

type rawDocument struct {
	Baseline versionIdentity `json:"baseline"`
	Changes  rawChanges      `json:"changes"`
	Head     versionIdentity `json:"head"`
}

type versionIdentity struct {
	SemconvVersion string `json:"semconv_version"`
}

type rawChanges struct {
	Entities           []rawChange `json:"entities"`
	Events             []rawChange `json:"events"`
	Metrics            []rawChange `json:"metrics"`
	RegistryAttributes []rawChange `json:"registry_attributes"`
	Spans              []rawChange `json:"spans"`
}

type rawChange struct {
	Name    string `json:"name,omitempty"`
	NewName string `json:"new_name,omitempty"`
	Note    string `json:"note,omitempty"`
	OldName string `json:"old_name,omitempty"`
	Type    string `json:"type"`
}

// Parse strictly decodes the complete Weaver diff shape, then returns the
// Registry attribute candidates relevant to normalization compilation.
func Parse(data []byte) (Document, error) {
	var raw rawDocument
	if err := strictDecode(data, &raw); err != nil {
		return Document{}, fmt.Errorf("decode Registry diff: %w", err)
	}
	if len(raw.Changes.RegistryAttributes) == 0 {
		return Document{}, errors.New("Registry diff has no attribute changes")
	}
	for category, changes := range map[string][]rawChange{
		"entities":            raw.Changes.Entities,
		"events":              raw.Changes.Events,
		"metrics":             raw.Changes.Metrics,
		"registry_attributes": raw.Changes.RegistryAttributes,
		"spans":               raw.Changes.Spans,
	} {
		for index, change := range changes {
			if err := validateRawChange(category, index, change); err != nil {
				return Document{}, err
			}
		}
	}
	result := Document{
		RegistryAttributes: make([]Candidate, 0, len(raw.Changes.RegistryAttributes)),
	}
	seen := make(map[string]struct{}, len(raw.Changes.RegistryAttributes))
	for index, change := range raw.Changes.RegistryAttributes {
		source := change.Name
		if change.Type == "renamed" {
			source = change.OldName
		}
		candidate := Candidate{
			Index: index, Type: change.Type, Source: source,
			Target: change.NewName, Note: change.Note,
		}
		id := candidate.StableID()
		if _, exists := seen[id]; exists {
			return Document{}, fmt.Errorf("duplicate Registry diff candidate %q", id)
		}
		seen[id] = struct{}{}
		result.RegistryAttributes = append(result.RegistryAttributes, candidate)
	}
	return result, nil
}

func validateRawChange(category string, index int, change rawChange) error {
	switch change.Type {
	case "added", "removed":
		if change.Name == "" || change.OldName != "" || change.NewName != "" {
			return fmt.Errorf("%s change[%d] has invalid %s identity", category, index, change.Type)
		}
	case "obsoleted", "uncategorized":
		if change.Name == "" || change.OldName != "" || change.NewName != "" ||
			strings.TrimSpace(change.Note) == "" {
			return fmt.Errorf("%s change[%d] has invalid %s identity", category, index, change.Type)
		}
	case "renamed":
		if change.Name != "" || change.OldName == "" || change.NewName == "" ||
			change.OldName == change.NewName || strings.TrimSpace(change.Note) == "" {
			return fmt.Errorf("%s change[%d] has invalid renamed identity", category, index)
		}
	default:
		return fmt.Errorf("%s change[%d] has unknown type %q", category, index, change.Type)
	}
	return nil
}

func related(key string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func strictDecode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}
