// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestExpandedSchemaAttributeMigrations(t *testing.T) {
	weave := Resolve("https://opentelemetry.io/schemas/1.7.0")
	tests := []struct {
		name         string
		source       string
		target       string
		mayTranslate func(string) bool
		translate    func(pcommon.Map, string, pcommon.Value) AttributeDecision
	}{
		{
			name: "Resource deployment", source: "deployment.environment",
			target:       "deployment.environment.name",
			mayTranslate: MayTranslateResourceAttribute,
			translate: func(attrs pcommon.Map, key string, value pcommon.Value) AttributeDecision {
				return weave.ResourceAttribute(attrs, key, value)
			},
		},
		{
			name: "Span messaging", source: "messaging.message_id",
			target:       "messaging.message.id",
			mayTranslate: MayTranslateSpanAttribute,
			translate: func(attrs pcommon.Map, key string, value pcommon.Value) AttributeDecision {
				return weave.SpanAttribute(attrs, "", key, value)
			},
		},
		{
			name: "Event code", source: "code.filepath",
			target:       "code.file.path",
			mayTranslate: MayTranslateEventAttribute,
			translate: func(attrs pcommon.Map, key string, value pcommon.Value) AttributeDecision {
				return weave.EventAttribute(attrs, "", "", key, value)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !test.mayTranslate(test.source) {
				t.Fatalf("generated translation filter rejects %q", test.source)
			}
			attrs := pcommon.NewMap()
			attrs.PutStr(test.source, "legacy")
			value, _ := attrs.Get(test.source)
			if got := test.translate(attrs, test.source, value); got != (AttributeDecision{
				Key: test.target, Action: AttributeKeepValue,
			}) {
				t.Fatalf("projection = %+v", got)
			}
			attrs.PutStr(test.target, "canonical")
			if got := test.translate(attrs, test.source, value); got != (AttributeDecision{
				Key: test.target, Action: AttributeSuppress,
			}) {
				t.Fatalf("target collision = %+v", got)
			}
		})
	}
}

func TestAmbiguousSchemaMigrationPreservesSource(t *testing.T) {
	weave := Resolve("https://opentelemetry.io/schemas/1.7.0")
	attrs := pcommon.NewMap()
	attrs.PutStr("feature_flag.evaluation.error.message", "boom")
	value, _ := attrs.Get("feature_flag.evaluation.error.message")
	got := weave.SpanAttribute(
		attrs, "", "feature_flag.evaluation.error.message", value,
	)
	if got != (AttributeDecision{
		Key: "feature_flag.evaluation.error.message", Action: AttributeKeepValue,
	}) {
		t.Fatalf("ambiguous projection = %+v", got)
	}
}
