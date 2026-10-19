// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"strings"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func compareKeyRule(
	rule *generatedKeyRule,
	weave WeaveID,
	domain generatedDomain,
	match generatedMatch,
	source string,
) int {
	if rule.weave != weave {
		if rule.weave < weave {
			return -1
		}
		return 1
	}
	if rule.domain != domain {
		if rule.domain < domain {
			return -1
		}
		return 1
	}
	if rule.match != match {
		if rule.match < match {
			return -1
		}
		return 1
	}
	return strings.Compare(rule.source, source)
}

func compareValueRule(
	rule *generatedValueRule,
	weave WeaveID,
	domain generatedDomain,
	sourceKey, sourceValue string,
) int {
	if rule.weave != weave {
		if rule.weave < weave {
			return -1
		}
		return 1
	}
	if rule.domain != domain {
		if rule.domain < domain {
			return -1
		}
		return 1
	}
	if comparison := strings.Compare(rule.sourceKey, sourceKey); comparison != 0 {
		return comparison
	}
	return strings.Compare(rule.sourceValue, sourceValue)
}

func TestGeneratedAttributeLookupTablesAreSorted(t *testing.T) {
	for _, test := range []struct {
		name  string
		rules []generatedKeyRule
	}{
		{name: "versioned keys", rules: activeGeneratedData.keyRules},
		{name: "fallback keys", rules: activeGeneratedData.fallbackKeyRules},
	} {
		t.Run(test.name, func(t *testing.T) {
			for index := range test.rules {
				if test.rules[index].priority == 0 {
					t.Fatalf("rules[%d] has invalid zero priority: %#v", index, test.rules[index])
				}
			}
			for index := 1; index < len(test.rules); index++ {
				previous, current := &test.rules[index-1], &test.rules[index]
				comparison := compareKeyRule(
					previous, current.weave, current.domain, current.match, current.source,
				)
				if comparison > 0 {
					t.Fatalf("rules[%d:%d] are not sorted: %#v then %#v",
						index-1, index, *previous, *current)
				}
			}
		})
	}

	for _, test := range []struct {
		name  string
		rules []generatedValueRule
	}{
		{name: "versioned values", rules: activeGeneratedData.valueRules},
		{name: "fallback values", rules: activeGeneratedData.fallbackValueRules},
	} {
		t.Run(test.name, func(t *testing.T) {
			for index := 1; index < len(test.rules); index++ {
				previous, current := &test.rules[index-1], &test.rules[index]
				comparison := compareValueRule(
					previous, current.weave, current.domain, current.sourceKey, current.sourceValue,
				)
				if comparison > 0 {
					t.Fatalf("rules[%d:%d] are not sorted: %#v then %#v",
						index-1, index, *previous, *current)
				}
			}
		})
	}
}

func TestGeneratedPrefixColdPathCoversInstalledRules(t *testing.T) {
	for _, test := range []struct {
		name  string
		rules []generatedKeyRule
	}{
		{name: "versioned", rules: activeGeneratedData.keyRules},
		{name: "fallback", rules: activeGeneratedData.fallbackKeyRules},
	} {
		t.Run(test.name, func(t *testing.T) {
			prefixRules := 0
			for index := range test.rules {
				rule := &test.rules[index]
				if rule.match != generatedMatchPrefix {
					continue
				}
				prefixRules++
				if !mayMatchPrefixRule(rule.domain, rule.source) {
					t.Fatalf("Prefix rule is outside generated cold path: %#v", *rule)
				}
			}
			if prefixRules == 0 {
				t.Fatal("generated table contains no Prefix rules")
			}
		})
	}
}

func TestAttributeTranslationFilterCoversInstalledRules(t *testing.T) {
	for _, test := range []struct {
		name  string
		rules []generatedKeyRule
	}{
		{name: "versioned", rules: activeGeneratedData.keyRules},
		{name: "fallback", rules: activeGeneratedData.fallbackKeyRules},
	} {
		t.Run(test.name, func(t *testing.T) {
			for index := range test.rules {
				rule := &test.rules[index]
				if !mayTranslateAttribute(rule.domain, rule.source) {
					t.Fatalf("translation filter rejects installed rule: %#v", *rule)
				}
				var mayTranslate bool
				switch rule.domain {
				case generatedDomainResource:
					mayTranslate = MayTranslateResourceAttribute(rule.source)
				case generatedDomainSpan:
					mayTranslate = MayTranslateSpanAttribute(rule.source)
				case generatedDomainSpanEvent:
					mayTranslate = MayTranslateEventAttribute(rule.source)
				}
				if !mayTranslate {
					t.Fatalf("public translation filter rejects installed rule: %#v", *rule)
				}
			}
		})
	}

	for _, test := range []struct {
		domain generatedDomain
		source string
	}{
		{domain: generatedDomainResource, source: "service.name"},
		{domain: generatedDomainSpan, source: "vendor.custom"},
		{domain: generatedDomainSpanEvent, source: "exception.message"},
	} {
		if mayTranslateAttribute(test.domain, test.source) {
			t.Fatalf("translation filter accepted %d/%q", test.domain, test.source)
		}
	}
}

func TestAttributeTranslationFilterRejectsImpossibleKeys(t *testing.T) {
	for _, test := range []struct {
		name string
		got  bool
	}{
		{name: "Resource unknown", got: MayTranslateResourceAttribute("zz.resource")},
		{name: "Span unknown", got: MayTranslateSpanAttribute("zz.span")},
		{name: "Event unknown", got: MayTranslateEventAttribute("zz.event")},
		{name: "Resource short", got: MayTranslateResourceAttribute("z")},
		{name: "Span empty", got: MayTranslateSpanAttribute("")},
		{name: "Span installed prefix wrong length", got: MayTranslateSpanAttribute("http.x")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.got {
				t.Fatal("translation filter accepted a key outside the generated rules")
			}
		})
	}
}

func TestGeneratedPrefixRoutingIsDerivedFromInstalledRules(t *testing.T) {
	prefixRules := 0
	for _, rules := range [][]generatedKeyRule{
		activeGeneratedData.keyRules,
		activeGeneratedData.fallbackKeyRules,
	} {
		for index := range rules {
			rule := &rules[index]
			if rule.match != generatedMatchPrefix {
				continue
			}
			prefixRules++
			if !mayMatchPrefixRule(rule.domain, rule.source+"dynamic") {
				t.Fatalf("generated Prefix route rejects installed rule: %#v", *rule)
			}
		}
	}
	if prefixRules == 0 {
		t.Fatal("generated table contains no Prefix rules")
	}
	for _, domain := range []generatedDomain{
		generatedDomainResource,
		generatedDomainSpan,
		generatedDomainSpanEvent,
	} {
		if mayMatchPrefixRule(domain, "zz.dynamic") {
			t.Fatalf("domain %d accepted an uninstalled dynamic prefix", domain)
		}
	}
}

func TestGeneratedAttributeLookupPreservesRangeSemantics(t *testing.T) {
	keyRules := []generatedKeyRule{
		{
			weave: 3, domain: generatedDomainResource, match: generatedMatchExact,
			priority: 1, source: "legacy.first", target: "canonical.first",
		},
		{
			weave: 3, domain: generatedDomainSpan, match: generatedMatchExact,
			priority: 1, sourceSpanName: "aaa", source: "legacy.exact", target: "wrong",
		},
		{
			weave: 3, domain: generatedDomainSpan, match: generatedMatchExact,
			priority: 1, sourceSpanName: "checkout", source: "legacy.exact", target: "canonical.exact",
		},
		{
			weave: 3, domain: generatedDomainSpan, match: generatedMatchPrefix,
			priority: 1, source: "legacy.connect.", target: "canonical.metadata.",
		},
		{
			weave: 3, domain: generatedDomainSpan, match: generatedMatchPrefix,
			priority: 1, sourceSpanName: "checkout",
			source: "legacy.grpc.", target: "canonical.metadata.",
		},
		{
			weave: 3, domain: generatedDomainSpanEvent, match: generatedMatchExact,
			priority: 1, source: "legacy.last", target: "canonical.last",
		},
		{
			weave: 4, domain: generatedDomainResource, match: generatedMatchExact,
			priority: 1, source: "legacy.other", target: "canonical.other",
		},
	}
	valueRules := []generatedValueRule{
		{
			weave: 3, domain: generatedDomainResource,
			sourceKey: "legacy.first", targetKey: "canonical.first",
			sourceValue: "old", targetValue: "new",
		},
		{
			weave: 3, domain: generatedDomainSpan, sourceSpanName: "aaa",
			sourceKey: "legacy.exact", targetKey: "canonical.exact",
			sourceValue: "old", targetValue: "wrong",
		},
		{
			weave: 3, domain: generatedDomainSpan, sourceSpanName: "checkout",
			sourceKey: "legacy.exact", targetKey: "canonical.exact",
			sourceValue: "old", targetValue: "new",
		},
		{
			weave: 3, domain: generatedDomainSpanEvent,
			sourceKey: "legacy.last", targetKey: "canonical.last",
			sourceValue: "old", targetValue: "new",
		},
	}

	attrs := pcommon.NewMap()
	attrs.PutStr("legacy.exact", "old")
	exact := translateAttributeFromRules(
		keyRules, valueRules, 3, generatedDomainSpan, attrs,
		"checkout", "", "legacy.exact", pcommon.NewValueStr("old"),
	)
	if exact != (AttributeDecision{
		Key: "canonical.exact", StringValue: "new", Action: AttributeReplaceString,
	}) {
		t.Fatalf("exact/value decision = %+v", exact)
	}

	attrs.PutStr("legacy.grpc.authorization", "secret")
	prefix := translateAttributeFromRules(
		keyRules, valueRules, 3, generatedDomainSpan, attrs,
		"checkout", "", "legacy.grpc.authorization", pcommon.NewValueStr("secret"),
	)
	if prefix != (AttributeDecision{
		Key: "canonical.metadata.authorization", Action: AttributeKeepValue,
	}) {
		t.Fatalf("prefix decision = %+v", prefix)
	}

	miss := translateAttributeFromRules(
		keyRules, valueRules, 3, generatedDomainSpan, attrs,
		"checkout", "", "vendor.custom", pcommon.NewValueStr("value"),
	)
	if miss != (AttributeDecision{Key: "vendor.custom", Action: AttributeKeepValue}) {
		t.Fatalf("miss decision = %+v", miss)
	}
}

func TestGenericAttributeLookupAllowsArbitraryPrefix(t *testing.T) {
	rules := []generatedKeyRule{{
		weave: 3, domain: generatedDomainResource, match: generatedMatchPrefix,
		priority: 1, source: "vendor.dynamic.", target: "canonical.dynamic.",
	}}
	attrs := pcommon.NewMap()
	attrs.PutStr("vendor.dynamic.custom", "value")

	got := translateAttributeFromRules(
		rules, nil, 3, generatedDomainResource, attrs,
		"", "", "vendor.dynamic.custom", pcommon.NewValueStr("value"),
	)
	if got != (AttributeDecision{
		Key: "canonical.dynamic.custom", Action: AttributeKeepValue,
	}) {
		t.Fatalf("generic Prefix decision = %+v", got)
	}
}

func TestHigherPrioritySourceSearchSpansExactAndPrefixRules(t *testing.T) {
	rules := []generatedKeyRule{
		{
			weave: 3, domain: generatedDomainSpan, match: generatedMatchExact,
			priority: 2, source: "legacy.exact", target: "canonical.metadata.",
		},
		{
			weave: 3, domain: generatedDomainSpan, match: generatedMatchPrefix,
			priority: 1, source: "legacy.preferred.", target: "canonical.metadata.",
		},
	}
	attrs := pcommon.NewMap()
	attrs.PutStr("legacy.exact", "loser")
	attrs.PutStr("legacy.preferred.", "winner")

	got := translateAttributeFromRules(
		rules, nil, 3, generatedDomainSpan, attrs,
		"", "", "legacy.exact", pcommon.NewValueStr("loser"),
	)
	if got != (AttributeDecision{Key: "canonical.metadata.", Action: AttributeSuppress}) {
		t.Fatalf("cross-match priority decision = %+v", got)
	}
}

func TestHigherPrioritySourceSearchSkipsMinimumPriority(t *testing.T) {
	rules := []generatedKeyRule{{
		weave: 3, domain: generatedDomainSpan, match: generatedMatchExact,
		priority: 0, source: "invalid.higher", target: "canonical",
	}}
	attrs := pcommon.NewMap()
	attrs.PutStr("invalid.higher", "value")
	if higherPriorityTestSourcePresent(
		rules, 3, generatedDomainSpan, attrs, "", "", "canonical", 1,
	) {
		t.Fatal("minimum priority performed an unnecessary conflict search")
	}
}
