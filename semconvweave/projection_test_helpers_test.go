// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// These helpers deliberately interpret arbitrary hand-written rules for unit
// tests. Production only consumes validated generated data through the
// immutable lookup in projection.go.
func translateAttributeFromData(
	data generatedData,
	weave WeaveID,
	domain generatedDomain,
	attrs pcommon.Map,
	sourceSpanName, sourceEventName, sourceKey string,
	sourceValue pcommon.Value,
) AttributeDecision {
	return translateAttributeFromRules(
		data.keyRules, data.valueRules, weave, domain, attrs,
		sourceSpanName, sourceEventName, sourceKey, sourceValue,
	)
}

func translateAttributeFromRules(
	keyRules []generatedKeyRule,
	valueRules []generatedValueRule,
	weave WeaveID,
	domain generatedDomain,
	attrs pcommon.Map,
	sourceSpanName, sourceEventName, sourceKey string,
	sourceValue pcommon.Value,
) AttributeDecision {
	rule := findTestKeyRule(
		keyRules, weave, domain,
		sourceSpanName, sourceEventName, sourceKey,
	)
	if rule == nil {
		return keepSource(sourceKey)
	}

	targetKey := rule.target
	if rule.match == generatedMatchPrefix {
		targetKey += strings.TrimPrefix(sourceKey, rule.source)
	}
	if _, present := attrs.Get(targetKey); present {
		return AttributeDecision{Key: targetKey, Action: AttributeSuppress}
	}
	if higherPriorityTestSourcePresent(
		keyRules, weave, domain, attrs,
		sourceSpanName, sourceEventName, targetKey, rule.priority,
	) {
		return AttributeDecision{Key: targetKey, Action: AttributeSuppress}
	}
	if sourceValue.Type() == pcommon.ValueTypeStr {
		if valueRule := findTestValueRule(
			valueRules, weave, domain,
			sourceSpanName, sourceEventName,
			sourceKey, targetKey, sourceValue.Str(),
		); valueRule != nil {
			return AttributeDecision{
				Key:         targetKey,
				StringValue: valueRule.targetValue,
				Action:      AttributeReplaceString,
			}
		}
	}
	return AttributeDecision{Key: targetKey, Action: AttributeKeepValue}
}

func findTestKeyRule(
	rules []generatedKeyRule,
	weave WeaveID,
	domain generatedDomain,
	sourceSpanName, sourceEventName, sourceKey string,
) *generatedKeyRule {
	for index := range rules {
		rule := &rules[index]
		if rule.weave == weave &&
			rule.domain == domain &&
			rule.match == generatedMatchExact &&
			rule.source == sourceKey &&
			predicateMatches(rule, sourceSpanName, sourceEventName) {
			return rule
		}
	}
	for index := range rules {
		rule := &rules[index]
		if rule.weave == weave &&
			rule.domain == domain &&
			rule.match == generatedMatchPrefix &&
			strings.HasPrefix(sourceKey, rule.source) &&
			predicateMatches(rule, sourceSpanName, sourceEventName) {
			return rule
		}
	}
	return nil
}

func higherPriorityTestSourcePresent(
	rules []generatedKeyRule,
	weave WeaveID,
	domain generatedDomain,
	attrs pcommon.Map,
	sourceSpanName, sourceEventName, targetKey string,
	priority uint16,
) bool {
	if priority <= 1 {
		return false
	}
	for index := range rules {
		candidate := &rules[index]
		if candidate.weave != weave ||
			candidate.domain != domain ||
			candidate.priority >= priority ||
			!predicateMatches(candidate, sourceSpanName, sourceEventName) {
			continue
		}
		if candidate.match == generatedMatchPrefix {
			if !strings.HasPrefix(targetKey, candidate.target) {
				continue
			}
			suffix := strings.TrimPrefix(targetKey, candidate.target)
			if _, present := attrs.Get(candidate.source + suffix); present {
				return true
			}
			continue
		}
		if candidate.target == targetKey {
			if _, present := attrs.Get(candidate.source); present {
				return true
			}
		}
	}
	return false
}

func findTestValueRule(
	rules []generatedValueRule,
	weave WeaveID,
	domain generatedDomain,
	sourceSpanName, sourceEventName, sourceKey, targetKey, sourceValue string,
) *generatedValueRule {
	for index := range rules {
		rule := &rules[index]
		if rule.weave == weave &&
			rule.domain == domain &&
			rule.sourceKey == sourceKey &&
			rule.targetKey == targetKey &&
			rule.sourceValue == sourceValue &&
			(rule.sourceSpanName == "" || rule.sourceSpanName == sourceSpanName) &&
			(rule.sourceEventName == "" || rule.sourceEventName == sourceEventName) {
			return rule
		}
	}
	return nil
}
