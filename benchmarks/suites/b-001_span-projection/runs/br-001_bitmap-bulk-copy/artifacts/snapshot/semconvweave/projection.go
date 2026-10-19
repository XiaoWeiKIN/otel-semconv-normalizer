// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func translateAttribute(
	weave WeaveID,
	domain generatedDomain,
	attrs pcommon.Map,
	sourceSpanName, sourceEventName, sourceKey string,
	sourceValue pcommon.Value,
) AttributeDecision {
	if !mayTranslateAttribute(domain, sourceKey) {
		return keepSource(sourceKey)
	}
	return decideAttributeTranslation(
		weave, domain, attrs,
		sourceSpanName, sourceEventName, sourceKey, sourceValue,
	)
}

func decideAttributeTranslation(
	weave WeaveID,
	domain generatedDomain,
	attrs pcommon.Map,
	sourceSpanName, sourceEventName, sourceKey string,
	sourceValue pcommon.Value,
) AttributeDecision {
	rules := generatedRules(activeGeneratedData, weave)
	rule := findGeneratedKeyRule(
		rules.keyRules, rules.weave, domain,
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
	if rule.priority > 1 && higherPrioritySourcePresent(
		rules.keyRules, rules.weave, domain, attrs,
		sourceSpanName, sourceEventName, targetKey, rule.priority,
	) {
		return AttributeDecision{Key: targetKey, Action: AttributeSuppress}
	}
	if sourceValue.Type() == pcommon.ValueTypeStr {
		if valueRule := findGeneratedValueRule(
			rules.valueRules, rules.weave, domain,
			sourceSpanName, sourceEventName,
			sourceKey, targetKey, sourceValue.Str(),
		); valueRule != nil {
			return AttributeDecision{
				Key: targetKey, StringValue: valueRule.targetValue, Action: AttributeReplaceString,
			}
		}
	}
	return AttributeDecision{Key: targetKey, Action: AttributeKeepValue}
}

func findGeneratedKeyRule(
	rules []generatedKeyRule,
	weave WeaveID,
	domain generatedDomain,
	sourceSpanName, sourceEventName, sourceKey string,
) *generatedKeyRule {
	start, end := generatedExactKeyRuleRange(weave, domain, sourceKey)
	for index := start; index < end; index++ {
		rule := &rules[index]
		if predicateMatches(rule, sourceSpanName, sourceEventName) {
			return rule
		}
	}

	// Prefix rules only serve dynamic RPC metadata on Span attributes.
	if !mayMatchPrefixRule(domain, sourceKey) {
		return nil
	}

	start, end = generatedPrefixKeyRuleRange(weave, domain)
	for index := start; index < end; index++ {
		rule := &rules[index]
		if strings.HasPrefix(sourceKey, rule.source) &&
			predicateMatches(rule, sourceSpanName, sourceEventName) {
			return rule
		}
	}
	return nil
}

func predicateMatches(rule *generatedKeyRule, sourceSpanName, sourceEventName string) bool {
	return (rule.sourceSpanName == "" || rule.sourceSpanName == sourceSpanName) &&
		(rule.sourceEventName == "" || rule.sourceEventName == sourceEventName)
}

func higherPrioritySourcePresent(
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
	for _, index := range generatedExactTargetRuleIndices(weave, domain, targetKey) {
		candidate := &rules[index]
		if candidate.priority >= priority ||
			!predicateMatches(candidate, sourceSpanName, sourceEventName) {
			continue
		}
		if _, present := attrs.Get(candidate.source); present {
			return true
		}
	}
	start, end := generatedPrefixKeyRuleRange(weave, domain)
	for index := start; index < end; index++ {
		candidate := &rules[index]
		if candidate.priority >= priority ||
			!predicateMatches(candidate, sourceSpanName, sourceEventName) ||
			!strings.HasPrefix(targetKey, candidate.target) {
			continue
		}
		suffix := strings.TrimPrefix(targetKey, candidate.target)
		if _, present := attrs.Get(candidate.source + suffix); present {
			return true
		}
	}
	return false
}

func findGeneratedValueRule(
	rules []generatedValueRule,
	weave WeaveID,
	domain generatedDomain,
	sourceSpanName, sourceEventName, sourceKey, targetKey, sourceValue string,
) *generatedValueRule {
	start, end := generatedValueRuleRange(weave, domain, sourceKey, sourceValue)
	for index := start; index < end; index++ {
		rule := &rules[index]
		if rule.targetKey == targetKey &&
			(rule.sourceSpanName == "" || rule.sourceSpanName == sourceSpanName) &&
			(rule.sourceEventName == "" || rule.sourceEventName == sourceEventName) {
			return rule
		}
	}
	return nil
}
