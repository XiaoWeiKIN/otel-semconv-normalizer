// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"testing"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
)

func TestHTTPAndNetworkFieldRegistryShape(t *testing.T) {
	tests := []struct {
		id          semconvmeta.FieldID
		name        string
		keys        [5]string
		keyCount    uint8
		valueType   semconvmeta.ValueType
		valuePolicy semconvmeta.ValuePolicy
	}{
		{
			id:          semconvmeta.FieldHTTPRequestMethod,
			name:        keyHTTPRequestMethod,
			keys:        [5]string{keyHTTPRequestMethod, keyHTTPMethod},
			keyCount:    2,
			valueType:   semconvmeta.ValueTypeString,
			valuePolicy: semconvmeta.ValueHTTPMethod,
		},
		{
			id:          semconvmeta.FieldHTTPRequestMethodOriginal,
			name:        keyHTTPRequestMethodOriginal,
			keys:        [5]string{keyHTTPRequestMethodOriginal},
			keyCount:    1,
			valueType:   semconvmeta.ValueTypeString,
			valuePolicy: semconvmeta.ValueHTTPMethodOriginal,
		},
		{
			id:          semconvmeta.FieldHTTPResponseStatusCode,
			name:        keyHTTPResponseStatusCode,
			keys:        [5]string{keyHTTPResponseStatusCode, keyHTTPStatusCode},
			keyCount:    2,
			valueType:   semconvmeta.ValueTypeInt64,
			valuePolicy: semconvmeta.ValueIdentity,
		},
		{
			id:          semconvmeta.FieldURLFull,
			name:        keyURLFull,
			keys:        [5]string{keyURLFull, keyHTTPURL},
			keyCount:    2,
			valueType:   semconvmeta.ValueTypeString,
			valuePolicy: semconvmeta.ValueIdentity,
		},
		{
			id:          semconvmeta.FieldURLScheme,
			name:        keyURLScheme,
			keys:        [5]string{keyURLScheme, keyHTTPScheme},
			keyCount:    2,
			valueType:   semconvmeta.ValueTypeString,
			valuePolicy: semconvmeta.ValueIdentity,
		},
		{
			id:          semconvmeta.FieldURLTemplate,
			name:        keyURLTemplate,
			keys:        [5]string{keyURLTemplate},
			keyCount:    1,
			valueType:   semconvmeta.ValueTypeString,
			valuePolicy: semconvmeta.ValueIdentity,
		},
		{
			id:          semconvmeta.FieldHTTPRoute,
			name:        keyHTTPRoute,
			keys:        [5]string{keyHTTPRoute},
			keyCount:    1,
			valueType:   semconvmeta.ValueTypeString,
			valuePolicy: semconvmeta.ValueIdentity,
		},
		{
			id:          semconvmeta.FieldUserAgentOriginal,
			name:        keyUserAgentOriginal,
			keys:        [5]string{keyUserAgentOriginal, keyHTTPUserAgent},
			keyCount:    2,
			valueType:   semconvmeta.ValueTypeString,
			valuePolicy: semconvmeta.ValueIdentity,
		},
		{
			id:          semconvmeta.FieldNetworkProtocolName,
			name:        keyNetworkProtocolName,
			keys:        [5]string{keyNetworkProtocolName, keyNetProtocolName},
			keyCount:    2,
			valueType:   semconvmeta.ValueTypeString,
			valuePolicy: semconvmeta.ValueIdentity,
		},
		{
			id:          semconvmeta.FieldNetworkTransport,
			name:        keyNetworkTransport,
			keys:        [5]string{keyNetworkTransport, keyNetTransport},
			keyCount:    2,
			valueType:   semconvmeta.ValueTypeString,
			valuePolicy: semconvmeta.ValueNetworkTransport,
		},
	}

	if semconvmeta.CoreFieldCount != 15 {
		t.Fatalf("CoreFieldCount = %d, want 15", semconvmeta.CoreFieldCount)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := semconvmeta.Field(test.id)
			if string(got.Concept()) != test.name {
				t.Errorf("concept = %q, want %q", got.Concept(), test.name)
			}
			if got.SourceKeys() != test.keys {
				t.Errorf("sourceKeys = %q, want %q", got.SourceKeys(), test.keys)
			}
			if got.SourceKeyCount() != test.keyCount {
				t.Errorf("sourceKeyCount = %d, want %d", got.SourceKeyCount(), test.keyCount)
			}
			if got.ValueType() != test.valueType {
				t.Errorf("valueType = %d, want %d", got.ValueType(), test.valueType)
			}
			if got.ValuePolicy() != test.valuePolicy {
				t.Errorf("valuePolicy = %d, want %d", got.ValuePolicy(), test.valuePolicy)
			}
		})
	}
}

func TestCanonicalFieldRegistryInvariants(t *testing.T) {
	concepts := make(map[string]semconvmeta.FieldID, int(semconvmeta.FieldCount))
	sourceKeys := make(map[string]semconvmeta.FieldID, int(semconvmeta.FieldCount)*2)

	for id := semconvmeta.FieldID(0); id < semconvmeta.FieldCount; id++ {
		field := semconvmeta.Field(id)
		if field.Concept() == "" {
			t.Errorf("semconvmeta.Field(%d) has empty concept", id)
		}
		if previous, ok := concepts[field.Concept()]; ok {
			t.Errorf("concept %q is duplicated by fields %d and %d", field.Concept(), previous, id)
		}
		concepts[field.Concept()] = id

		if field.SourceKeyCount() == 0 || int(field.SourceKeyCount()) > len(field.SourceKeys()) {
			t.Fatalf("semconvmeta.Field(%d).SourceKeyCount() = %d, want 1..%d", id, field.SourceKeyCount(), len(field.SourceKeys()))
		}
		if id < semconvmeta.CoreFieldCount && field.SourceKeys()[0] != field.Concept() {
			t.Errorf("semconvmeta.Field(%d) first source key = %q, want concept %q", id, field.SourceKeys()[0], field.Concept())
		}
		for i := uint8(0); i < field.SourceKeyCount(); i++ {
			key := field.SourceKeys()[i]
			if key == "" {
				t.Errorf("semconvmeta.Field(%d).SourceKeys()[%d] is empty", id, i)
				continue
			}
			if previous, ok := sourceKeys[key]; ok {
				// A peer projection may intentionally reuse one core source
				// key. Duplicate sources within either registry segment remain
				// an error.
				if id < semconvmeta.CoreFieldCount || previous >= semconvmeta.CoreFieldCount {
					t.Errorf("source key %q is duplicated by fields %d and %d", key, previous, id)
				}
			}
			sourceKeys[key] = id
		}
		for i := field.SourceKeyCount(); int(i) < len(field.SourceKeys()); i++ {
			if field.SourceKeys()[i] != "" {
				t.Errorf("semconvmeta.Field(%d).SourceKeys()[%d] = %q after SourceKeyCount", id, i, field.SourceKeys()[i])
			}
		}

		switch field.ValueType() {
		case semconvmeta.ValueTypeString, semconvmeta.ValueTypeInt64, semconvmeta.ValueTypeStringSlice:
		default:
			t.Errorf("semconvmeta.Field(%d).ValueType() = %d is invalid", id, field.ValueType())
		}
		switch field.ValuePolicy() {
		case semconvmeta.ValueIdentity, semconvmeta.ValueHTTPMethod,
			semconvmeta.ValueHTTPMethodOriginal, semconvmeta.ValueNetworkTransport,
			semconvmeta.ValueTechnology:
		default:
			t.Errorf("semconvmeta.Field(%d).ValuePolicy() = %d is invalid", id, field.ValuePolicy())
		}
		switch {
		case id < semconvmeta.CoreFieldCount && field.KindPolicy() != semconvmeta.KindAny:
			t.Errorf("semconvmeta.Field(%d).KindPolicy() = %d, want any-kind", id, field.KindPolicy())
		case id >= semconvmeta.CoreFieldCount && field.KindPolicy() != semconvmeta.KindPeer:
			t.Errorf("semconvmeta.Field(%d).KindPolicy() = %d, want peer-kind", id, field.KindPolicy())
		}
	}
}
