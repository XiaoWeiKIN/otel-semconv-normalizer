// SPDX-License-Identifier: Apache-2.0

package typedir

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type operationKind uint8

const (
	operationKey operationKind = iota + 1
	operationValue
	operationName
)

type operation struct {
	kind        operationKind
	domain      Domain
	action      string
	source      string
	target      string
	sourceValue string
	targetValue string
	spanNames   []string
	eventNames  []string
	evidence    Evidence
	reason      string
}

type chain struct {
	domain    Domain
	predicate Predicate
	match     MatchKind
	source    string
	current   string
	visited   map[string]struct{}
	evidence  []Evidence
}

type nameChain struct {
	domain   Domain
	source   string
	current  string
	visited  map[string]struct{}
	evidence []Evidence
}

func foldOperations(operations []operation) ([]KeyRule, []ValueRule, []NameRule, []Disposition, error) {
	var (
		chains       []chain
		nameChains   []nameChain
		valueRules   []ValueRule
		dispositions []Disposition
	)
	stableIDs := make(map[string]struct{}, len(operations))
	renameTargets := make(map[string]string)

	for _, item := range operations {
		if err := validateOperation(item); err != nil {
			return nil, nil, nil, nil, err
		}
		// Schema stable IDs already carry their domain, so an `all:` replay yields
		// one ID per domain. A Migration Guide decision instead has a single ID that
		// may legitimately be replayed across domains, so uniqueness is per domain.
		scopedID := string(item.domain) + "\x00" + item.evidence.StableID
		if _, exists := stableIDs[scopedID]; exists {
			return nil, nil, nil, nil, fmt.Errorf(
				"duplicate stable ID %q in domain %q", item.evidence.StableID, item.domain,
			)
		}
		stableIDs[scopedID] = struct{}{}

		if item.action != "translated" {
			dispositions = append(dispositions, Disposition{
				Domain:   item.domain,
				StableID: item.evidence.StableID,
				Action:   item.action,
				Reason:   item.reason,
				Source:   item.source,
				Evidence: []Evidence{item.evidence},
			})
			continue
		}

		switch item.kind {
		case operationName:
			key := string(item.domain) + "\x00" + item.source
			if previous, ok := renameTargets[key]; ok && previous != item.target {
				return nil, nil, nil, nil, fmt.Errorf("name ambiguity for %q: %q and %q", item.source, previous, item.target)
			}
			renameTargets[key] = item.target
			if err := applyNameRename(&nameChains, item); err != nil {
				return nil, nil, nil, nil, err
			}
		case operationValue:
			var err error
			valueRules, err = mergeValueRule(valueRules, item)
			if err != nil {
				return nil, nil, nil, nil, err
			}
		case operationKey:
			match, source, target, err := normalizeKeyPattern(item.source, item.target)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			if isContextualNegative(source) {
				return nil, nil, nil, nil, fmt.Errorf("contextual negative %q cannot enter translated IR", source)
			}
			predicates, err := sourcePredicates(item, nameChains)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			for _, predicate := range predicates {
				key := string(item.domain) + "\x00" + predicate.SourceSpanName + "\x00" +
					predicate.SourceEventName + "\x00" + string(match) + "\x00" + source
				if previous, ok := renameTargets[key]; ok {
					if previous != target {
						return nil, nil, nil, nil, fmt.Errorf(
							"same predicate maps %q to both %q and %q", source, previous, target,
						)
					}
					if !mergeDuplicateKeyEvidence(chains, item.domain, predicate, match, source, item.evidence) {
						chains = append(chains, chain{
							domain: item.domain, predicate: predicate, match: match,
							source: source, current: target,
							visited:  map[string]struct{}{source: {}, target: {}},
							evidence: []Evidence{item.evidence},
						})
					}
					continue
				}
				renameTargets[key] = target
				if err := applyKeyRename(&chains, item.domain, predicate, match, source, target, item.evidence); err != nil {
					return nil, nil, nil, nil, err
				}
			}
		default:
			return nil, nil, nil, nil, errors.New("unknown operation kind")
		}
	}

	keyRules := make([]KeyRule, 0, len(chains))
	for _, item := range chains {
		if item.source == item.current {
			continue
		}
		keyRules = append(keyRules, KeyRule{
			Domain: item.domain, Predicate: item.predicate, Match: item.match,
			Source: item.source, Target: item.current, Evidence: item.evidence,
		})
	}
	nameRules := make([]NameRule, 0, len(nameChains))
	for _, item := range nameChains {
		if item.source == item.current {
			continue
		}
		nameRules = append(nameRules, NameRule{
			Domain: item.domain, SourceName: item.source, TargetName: item.current, Evidence: item.evidence,
		})
	}
	sortIR(keyRules, valueRules, nameRules, dispositions)
	return keyRules, valueRules, nameRules, dispositions, nil
}

func validateOperation(item operation) error {
	if item.evidence.StableID == "" || item.evidence.Authority == "" || item.domain == "" ||
		(item.action != "translated" && item.action != "pass_through" && item.action != "unsupported") {
		return fmt.Errorf("unclassified or incomplete required operation %q", item.evidence.StableID)
	}
	switch item.evidence.Phase {
	case "schema_all", "schema_signal", "schema_name", "post_schema", "classifier_fallback", "domain_disposition":
	default:
		return fmt.Errorf("unknown phase %q in %q", item.evidence.Phase, item.evidence.StableID)
	}
	if item.action == "translated" {
		if item.source == "" || item.target == "" {
			return fmt.Errorf("translated operation %q lacks source or target", item.evidence.StableID)
		}
		if item.kind == operationValue && (item.sourceValue == "" || item.targetValue == "") {
			return fmt.Errorf("translated value operation %q lacks value mapping", item.evidence.StableID)
		}
	}
	if item.domain != DomainResource && item.domain != DomainSpan &&
		item.domain != DomainSpanEvent && item.domain != DomainLink {
		return fmt.Errorf("unknown domain %q in %q", item.domain, item.evidence.StableID)
	}
	return nil
}

func applyNameRename(chains *[]nameChain, item operation) error {
	updated := false
	for index := range *chains {
		current := &(*chains)[index]
		if current.domain != item.domain || current.current != item.source {
			continue
		}
		if _, cycle := current.visited[item.target]; cycle {
			return fmt.Errorf("name cycle %q -> %q", item.source, item.target)
		}
		current.current = item.target
		current.visited[item.target] = struct{}{}
		current.evidence = append(current.evidence, item.evidence)
		updated = true
	}
	if updated {
		return nil
	}
	if item.source == item.target {
		return fmt.Errorf("name self-cycle %q", item.source)
	}
	*chains = append(*chains, nameChain{
		domain: item.domain, source: item.source, current: item.target,
		visited:  map[string]struct{}{item.source: {}, item.target: {}},
		evidence: []Evidence{item.evidence},
	})
	return nil
}

func sourcePredicates(item operation, names []nameChain) ([]Predicate, error) {
	if len(item.spanNames) == 0 && len(item.eventNames) == 0 {
		return []Predicate{{}}, nil
	}
	if len(item.spanNames) != 0 && len(item.eventNames) != 0 {
		return nil, fmt.Errorf("operation %q requires unsupported span+event cross-product context", item.evidence.StableID)
	}
	var result []Predicate
	if len(item.spanNames) != 0 {
		for _, currentName := range item.spanNames {
			matched := false
			for _, chain := range names {
				if chain.domain == DomainSpan && chain.current == currentName {
					result = append(result, Predicate{SourceSpanName: chain.source})
					matched = true
				}
			}
			if !matched {
				result = append(result, Predicate{SourceSpanName: currentName})
			}
		}
	} else {
		for _, currentName := range item.eventNames {
			matched := false
			for _, chain := range names {
				if chain.domain == DomainSpanEvent && chain.current == currentName {
					result = append(result, Predicate{SourceEventName: chain.source})
					matched = true
				}
			}
			if !matched {
				return nil, fmt.Errorf(
					"operation %q event condition %q has no source-name context",
					item.evidence.StableID, currentName,
				)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SourceSpanName != result[j].SourceSpanName {
			return result[i].SourceSpanName < result[j].SourceSpanName
		}
		return result[i].SourceEventName < result[j].SourceEventName
	})
	return result, nil
}

func applyKeyRename(chains *[]chain, domain Domain, predicate Predicate, match MatchKind, source, target string, evidence Evidence) error {
	var candidates []int
	for index := range *chains {
		item := &(*chains)[index]
		if item.domain == domain && item.match == match && item.current == source &&
			(item.predicate == predicate || predicate == (Predicate{})) {
			candidates = append(candidates, index)
		}
	}
	if len(candidates) == 0 && predicate != (Predicate{}) {
		for index := range *chains {
			item := &(*chains)[index]
			if item.domain == domain && item.match == match && item.current == source &&
				item.predicate == (Predicate{}) {
				clone := cloneChain(*item)
				clone.predicate = predicate
				*chains = append(*chains, clone)
				candidates = append(candidates, len(*chains)-1)
			}
		}
	}
	if len(candidates) == 0 {
		if source == target {
			return fmt.Errorf("key self-cycle %q", source)
		}
		*chains = append(*chains, chain{
			domain: domain, predicate: predicate, match: match, source: source, current: target,
			visited: map[string]struct{}{source: {}, target: {}}, evidence: []Evidence{evidence},
		})
		return nil
	}
	for _, index := range candidates {
		item := &(*chains)[index]
		if _, cycle := item.visited[target]; cycle {
			return fmt.Errorf("key cycle %q -> %q in source chain %q", source, target, item.source)
		}
		item.current = target
		item.visited[target] = struct{}{}
		item.evidence = append(item.evidence, evidence)
	}
	return nil
}

func cloneChain(source chain) chain {
	result := source
	result.visited = make(map[string]struct{}, len(source.visited))
	for key := range source.visited {
		result.visited[key] = struct{}{}
	}
	result.evidence = append([]Evidence(nil), source.evidence...)
	return result
}

func mergeDuplicateKeyEvidence(chains []chain, domain Domain, predicate Predicate, match MatchKind, source string, evidence Evidence) bool {
	for index := range chains {
		item := &chains[index]
		if item.domain == domain && item.predicate == predicate && item.match == match && item.source == source {
			item.evidence = append(item.evidence, evidence)
			return true
		}
	}
	return false
}

func mergeValueRule(rules []ValueRule, item operation) ([]ValueRule, error) {
	for index := range rules {
		rule := &rules[index]
		if rule.Domain != item.domain || rule.SourceKey != item.source || rule.SourceValue != item.sourceValue {
			continue
		}
		if rule.TargetKey != item.target || rule.TargetValue != item.targetValue {
			return nil, fmt.Errorf(
				"value authority disagreement for %s=%q: %s=%q and %s=%q",
				item.source, item.sourceValue, rule.TargetKey, rule.TargetValue, item.target, item.targetValue,
			)
		}
		rule.Evidence = append(rule.Evidence, item.evidence)
		return rules, nil
	}
	return append(rules, ValueRule{
		Domain: item.domain, SourceKey: item.source, TargetKey: item.target, SourceValue: item.sourceValue,
		TargetValue: item.targetValue, Evidence: []Evidence{item.evidence},
	}), nil
}

func normalizeKeyPattern(source, target string) (MatchKind, string, string, error) {
	sourceWildcard := strings.HasSuffix(source, "<key>")
	targetWildcard := strings.HasSuffix(target, "<key>")
	if sourceWildcard != targetWildcard {
		return "", "", "", fmt.Errorf("wildcard shape differs for %q -> %q", source, target)
	}
	if !sourceWildcard {
		if strings.Contains(source, "<") || strings.Contains(target, "<") {
			return "", "", "", fmt.Errorf("unsupported contextual key %q -> %q", source, target)
		}
		return MatchExact, source, target, nil
	}
	return MatchPrefix, strings.TrimSuffix(source, "<key>"), strings.TrimSuffix(target, "<key>"), nil
}

func isContextualNegative(source string) bool {
	switch source {
	case "db.statement", "http.target", "http.method":
		return true
	default:
		return false
	}
}

func sortIR(keys []KeyRule, values []ValueRule, names []NameRule, dispositions []Disposition) {
	sort.Slice(keys, func(i, j int) bool {
		left, right := keys[i], keys[j]
		return string(left.Domain)+"\x00"+string(left.Match)+"\x00"+left.Source+"\x00"+
			left.Predicate.SourceSpanName+"\x00"+left.Predicate.SourceEventName <
			string(right.Domain)+"\x00"+string(right.Match)+"\x00"+right.Source+"\x00"+
				right.Predicate.SourceSpanName+"\x00"+right.Predicate.SourceEventName
	})
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i], values[j]
		return string(left.Domain)+"\x00"+left.SourceKey+"\x00"+left.SourceValue <
			string(right.Domain)+"\x00"+right.SourceKey+"\x00"+right.SourceValue
	})
	sort.Slice(names, func(i, j int) bool {
		return string(names[i].Domain)+"\x00"+names[i].SourceName <
			string(names[j].Domain)+"\x00"+names[j].SourceName
	})
	sort.Slice(dispositions, func(i, j int) bool {
		return string(dispositions[i].Domain)+"\x00"+dispositions[i].StableID <
			string(dispositions[j].Domain)+"\x00"+dispositions[j].StableID
	})
	for index := range values {
		sort.Slice(values[index].Evidence, func(i, j int) bool {
			return values[index].Evidence[i].StableID < values[index].Evidence[j].StableID
		})
	}
}
