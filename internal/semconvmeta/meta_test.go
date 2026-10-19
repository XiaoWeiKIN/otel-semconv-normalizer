// SPDX-License-Identifier: Apache-2.0

package semconvmeta

import (
	"reflect"
	"testing"
	"testing/quick"
)

func TestFieldDescriptorsAreComplete(t *testing.T) {
	seen := make(map[string]struct{}, FieldCount)
	for id := FieldID(0); id < FieldCount; id++ {
		field := Field(id)
		if field.Concept() == "" || field.SourceKeyCount() == 0 || int(field.SourceKeyCount()) > len(field.SourceKeys()) {
			t.Fatalf("Field(%d) is incomplete: %#v", id, field)
		}
		if _, exists := seen[field.Concept()]; exists {
			t.Fatalf("duplicate concept %q", field.Concept())
		}
		seen[field.Concept()] = struct{}{}
	}
	if got := Field(FieldCount); got.Concept() != "" {
		t.Fatalf("invalid Field ID returned %#v", got)
	}
}

// Story 7.16 requires the descriptor table to stay immutable at runtime: a
// caller that mutates what Field returns must not be able to corrupt the
// shared static table for every other goroutine.
func TestFieldDoesNotExposeMutableTableState(t *testing.T) {
	const id = FieldPeerDBSystem
	original := Field(id)
	originalKeys := original.SourceKeys()

	escapedKeys := Field(id).SourceKeys()
	escapedKeys[0] = "mutated"

	after := Field(id)
	if after.Concept() != original.Concept() {
		t.Fatalf("Concept = %q after caller mutation, want %q", after.Concept(), original.Concept())
	}
	if after.SourceKeys() != originalKeys {
		t.Fatalf("SourceKeys = %v after caller mutation, want %v", after.SourceKeys(), originalKeys)
	}

	viewType := reflect.TypeOf(FieldDescriptor{})
	for i := 0; i < viewType.NumField(); i++ {
		if viewType.Field(i).IsExported() {
			t.Fatalf("FieldDescriptor exposes mutable field %q", viewType.Field(i).Name)
		}
	}
}

func TestFieldIsZeroAllocation(t *testing.T) {
	var sink FieldDescriptor
	allocations := testing.AllocsPerRun(1000, func() {
		for id := FieldID(0); id < FieldCount; id++ {
			sink = Field(id)
		}
	})
	if allocations != 0 {
		t.Fatalf("Field() allocations = %v, want 0", allocations)
	}
	if sink.Concept() == "" {
		t.Fatal("sink was not populated")
	}
}

func TestTechnologyAliasesAreSortedAndCanonicalized(t *testing.T) {
	aliases := TechnologyAliases()
	if len(aliases) != 19 {
		t.Fatalf("TechnologyAliases() count = %d, want 19", len(aliases))
	}
	for i, alias := range aliases {
		if alias.Source == "" || alias.Target == "" || alias.Owner == "" || alias.Rationale == "" {
			t.Fatalf("alias %d is incomplete: %#v", i, alias)
		}
		if i > 0 && aliases[i-1].Source >= alias.Source {
			t.Fatalf("aliases are not unique and sorted at %q", alias.Source)
		}
		if got := CanonicalizeTechnology(alias.Source); got != alias.Target {
			t.Fatalf("CanonicalizeTechnology(%q) = %q, want %q", alias.Source, got, alias.Target)
		}
	}
	aliases[0].Target = "mutated"
	if got := CanonicalizeTechnology(aliases[0].Source); got == "mutated" {
		t.Fatal("TechnologyAliases returned mutable package storage")
	}
}

func TestCanonicalizeTechnologyPreservesUnknown(t *testing.T) {
	if err := quick.Check(func(value string) bool {
		for _, alias := range technologyAliases {
			if value == alias.Source {
				return true
			}
		}
		return CanonicalizeTechnology(value) == value
	}, nil); err != nil {
		t.Fatal(err)
	}
}
