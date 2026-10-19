// SPDX-License-Identifier: Apache-2.0

package semconvweave

func translateName(weave WeaveID, domain generatedDomain, source string) string {
	rules := generatedRules(activeGeneratedData, weave)
	return translateNameFromRules(rules.nameRules, rules.weave, domain, source)
}

func translateNameFromRules(
	rules []generatedNameRule,
	weave WeaveID,
	domain generatedDomain,
	source string,
) string {
	for _, rule := range rules {
		if rule.weave == weave && rule.domain == domain && rule.sourceName == source {
			return rule.targetName
		}
	}
	return source
}
