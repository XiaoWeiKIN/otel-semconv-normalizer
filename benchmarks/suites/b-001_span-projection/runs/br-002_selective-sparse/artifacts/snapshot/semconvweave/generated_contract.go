// SPDX-License-Identifier: Apache-2.0

package semconvweave

// generatedDomain is private runtime vocabulary shared with generated data.
type generatedDomain uint8

const (
	generatedDomainResource generatedDomain = iota + 1
	generatedDomainSpan
	generatedDomainSpanEvent
)

type generatedMatch uint8

const (
	generatedMatchExact generatedMatch = iota + 1
	generatedMatchPrefix
)

type generatedSource struct {
	url string
	id  WeaveID
}

type generatedKeyRule struct {
	weave           WeaveID
	domain          generatedDomain
	match           generatedMatch
	priority        uint16
	sourceSpanName  string
	sourceEventName string
	source          string
	target          string
}

type generatedValueRule struct {
	weave           WeaveID
	domain          generatedDomain
	sourceSpanName  string
	sourceEventName string
	sourceKey       string
	targetKey       string
	sourceValue     string
	targetValue     string
}

type generatedNameRule struct {
	weave      WeaveID
	domain     generatedDomain
	sourceName string
	targetName string
}

type generatedData struct {
	sources            []generatedSource
	keyRules           []generatedKeyRule
	valueRules         []generatedValueRule
	nameRules          []generatedNameRule
	fallbackKeyRules   []generatedKeyRule
	fallbackValueRules []generatedValueRule
	fallbackNameRules  []generatedNameRule
}

type generatedRuleSet struct {
	weave      WeaveID
	keyRules   []generatedKeyRule
	valueRules []generatedValueRule
	nameRules  []generatedNameRule
}

type generatedRuleRange struct {
	start int
	end   int
}

type generatedValueLookupKey struct {
	sourceKey   string
	sourceValue string
}

const generatedDomainSlots = int(generatedDomainSpanEvent) + 1
const attributePrefixSlots = 1 << 8
const attributePrefixWords = attributePrefixSlots >> 6

type generatedRuleBucket struct {
	exactKeys    map[string]generatedRuleRange
	exactTargets map[string][]int
	prefix       generatedRuleRange
	values       map[generatedValueLookupKey]generatedRuleRange
}

type generatedRuleLookup [][generatedDomainSlots]generatedRuleBucket

var (
	activeGeneratedData  generatedData
	activeRuleLookup     generatedRuleLookup
	attributePrefixIDs   [1 << 16]uint8
	attributePrefixCount uint8
	attributePrefixBits  [generatedDomainSlots][attributePrefixWords]uint64
	prefixRuleBits       [generatedDomainSlots][attributePrefixWords]uint64
	attributeLengthBits  [generatedDomainSlots][attributePrefixSlots][2]uint64
	generatedInstalled   bool
)

// installGeneratedData is called once from checked-in generated Go. Slices
// point at static arrays and are never exported or mutated after package init.
func installGeneratedData(data generatedData) {
	if generatedInstalled {
		panic("semconvweave: generated data installed more than once")
	}
	for index, source := range data.sources {
		if source.url == "" || source.id <= WeaveCanonical {
			panic("semconvweave: invalid generated source")
		}
		if index > 0 && data.sources[index-1].url >= source.url {
			panic("semconvweave: generated sources are not unique and sorted")
		}
	}
	indexAttributeRuleShapes(data.keyRules)
	indexAttributeRuleShapes(data.fallbackKeyRules)
	activeRuleLookup = buildGeneratedRuleLookup(data)
	activeGeneratedData = data
	generatedInstalled = true
}

// mayTranslateAttribute is a conservative filter derived from the installed
// rules. False means no translation can apply; true may be a false positive.
func mayTranslateAttribute(domain generatedDomain, sourceKey string) bool {
	prefixID := attributePrefixID(sourceKey)
	if !hasAttributePrefix(domain, prefixID) {
		return false
	}
	if hasPrefixRule(domain, prefixID) {
		return true
	}
	length := len(sourceKey)
	if length >= 128 {
		return true
	}
	return attributeLengthBits[domain][prefixID][length>>6]&
		(uint64(1)<<uint(length&63)) != 0
}

func hasAttributePrefix(domain generatedDomain, prefixID uint8) bool {
	if prefixID == 0 || int(domain) >= generatedDomainSlots || domain == 0 {
		return false
	}
	return attributePrefixBits[domain][prefixID>>6]&
		(uint64(1)<<uint(prefixID&63)) != 0
}

func mayMatchPrefixRule(domain generatedDomain, sourceKey string) bool {
	return hasPrefixRule(domain, attributePrefixID(sourceKey))
}

func hasPrefixRule(domain generatedDomain, prefixID uint8) bool {
	if prefixID == 0 || int(domain) >= generatedDomainSlots || domain == 0 {
		return false
	}
	return prefixRuleBits[domain][prefixID>>6]&
		(uint64(1)<<uint(prefixID&63)) != 0
}

// attributeKeyPrefix encodes the first two bytes without parsing a namespace.
func attributeKeyPrefix(sourceKey string) (int, bool) {
	if len(sourceKey) < 2 {
		return 0, false
	}
	return int(sourceKey[0])<<8 | int(sourceKey[1]), true
}

func attributePrefixID(sourceKey string) uint8 {
	prefix, ok := attributeKeyPrefix(sourceKey)
	if !ok {
		return 0
	}
	return attributePrefixIDs[prefix]
}

func registerAttributePrefix(prefix int) uint8 {
	if id := attributePrefixIDs[prefix]; id != 0 {
		return id
	}
	if attributePrefixCount == attributePrefixSlots-1 {
		panic("semconvweave: generated source prefix table is full")
	}
	attributePrefixCount++
	attributePrefixIDs[prefix] = attributePrefixCount
	return attributePrefixCount
}

func indexAttributeRuleShapes(rules []generatedKeyRule) {
	for index := range rules {
		rule := &rules[index]
		prefix, ok := attributeKeyPrefix(rule.source)
		if !ok || int(rule.domain) >= generatedDomainSlots || rule.domain == 0 {
			panic("semconvweave: generated key rule cannot be indexed for translation")
		}
		prefixID := registerAttributePrefix(prefix)
		attributePrefixBits[rule.domain][prefixID>>6] |=
			uint64(1) << uint(prefixID&63)
		if rule.match == generatedMatchPrefix {
			prefixRuleBits[rule.domain][prefixID>>6] |=
				uint64(1) << uint(prefixID&63)
			continue
		}
		if len(rule.source) >= 128 {
			continue
		}
		length := len(rule.source)
		attributeLengthBits[rule.domain][prefixID][length>>6] |=
			uint64(1) << uint(length&63)
	}
}

func buildGeneratedRuleLookup(data generatedData) generatedRuleLookup {
	maxWeave := WeaveUnknown
	for _, source := range data.sources {
		if source.id > maxWeave {
			maxWeave = source.id
		}
	}
	lookup := make(generatedRuleLookup, int(maxWeave)+1)
	addGeneratedKeyRules(lookup, data.keyRules)
	addGeneratedKeyRules(lookup, data.fallbackKeyRules)
	addGeneratedValueRules(lookup, data.valueRules)
	addGeneratedValueRules(lookup, data.fallbackValueRules)
	return lookup
}

func addGeneratedKeyRules(lookup generatedRuleLookup, rules []generatedKeyRule) {
	for index := 0; index < len(rules); {
		rule := rules[index]
		bucket := lookup.bucket(rule.weave, rule.domain)
		if bucket == nil {
			panic("semconvweave: generated key rule has invalid weave or domain")
		}
		end := index + 1
		for end < len(rules) &&
			rules[end].weave == rule.weave &&
			rules[end].domain == rule.domain &&
			rules[end].match == rule.match &&
			rules[end].source == rule.source {
			end++
		}
		if rule.match == generatedMatchExact {
			if bucket.exactKeys == nil {
				bucket.exactKeys = make(map[string]generatedRuleRange)
				bucket.exactTargets = make(map[string][]int)
			}
			if _, exists := bucket.exactKeys[rule.source]; exists {
				panic("semconvweave: generated exact key rules are not contiguous")
			}
			bucket.exactKeys[rule.source] = generatedRuleRange{start: index, end: end}
			for candidate := index; candidate < end; candidate++ {
				target := rules[candidate].target
				bucket.exactTargets[target] = append(
					bucket.exactTargets[target], candidate,
				)
			}
		} else {
			if bucket.prefix.start == bucket.prefix.end {
				bucket.prefix = generatedRuleRange{start: index, end: end}
			} else {
				if bucket.prefix.end != index {
					panic("semconvweave: generated Prefix key rules are not contiguous")
				}
				bucket.prefix.end = end
			}
		}
		index = end
	}
}

func addGeneratedValueRules(lookup generatedRuleLookup, rules []generatedValueRule) {
	for index := 0; index < len(rules); {
		rule := rules[index]
		bucket := lookup.bucket(rule.weave, rule.domain)
		if bucket == nil {
			panic("semconvweave: generated value rule has invalid weave or domain")
		}
		if bucket.values == nil {
			bucket.values = make(map[generatedValueLookupKey]generatedRuleRange)
		}
		end := index + 1
		for end < len(rules) &&
			rules[end].weave == rule.weave &&
			rules[end].domain == rule.domain &&
			rules[end].sourceKey == rule.sourceKey &&
			rules[end].sourceValue == rule.sourceValue {
			end++
		}
		key := generatedValueLookupKey{
			sourceKey:   rule.sourceKey,
			sourceValue: rule.sourceValue,
		}
		if _, exists := bucket.values[key]; exists {
			panic("semconvweave: generated value rules are not contiguous")
		}
		bucket.values[key] = generatedRuleRange{start: index, end: end}
		index = end
	}
}

func (lookup generatedRuleLookup) bucket(
	weave WeaveID,
	domain generatedDomain,
) *generatedRuleBucket {
	if int(weave) >= len(lookup) || int(domain) >= generatedDomainSlots || domain == 0 {
		return nil
	}
	return &lookup[int(weave)][int(domain)]
}

func generatedExactKeyRuleRange(
	weave WeaveID,
	domain generatedDomain,
	source string,
) (int, int) {
	bucket := activeRuleLookup.bucket(weave, domain)
	if bucket == nil {
		return 0, 0
	}
	result := bucket.exactKeys[source]
	return result.start, result.end
}

func generatedPrefixKeyRuleRange(
	weave WeaveID,
	domain generatedDomain,
) (int, int) {
	bucket := activeRuleLookup.bucket(weave, domain)
	if bucket == nil {
		return 0, 0
	}
	return bucket.prefix.start, bucket.prefix.end
}

func generatedExactTargetRuleIndices(
	weave WeaveID,
	domain generatedDomain,
	target string,
) []int {
	bucket := activeRuleLookup.bucket(weave, domain)
	if bucket == nil {
		return nil
	}
	return bucket.exactTargets[target]
}

func generatedValueRuleRange(
	weave WeaveID,
	domain generatedDomain,
	sourceKey, sourceValue string,
) (int, int) {
	bucket := activeRuleLookup.bucket(weave, domain)
	if bucket == nil {
		return 0, 0
	}
	result := bucket.values[generatedValueLookupKey{
		sourceKey: sourceKey, sourceValue: sourceValue,
	}]
	return result.start, result.end
}

func resolveGenerated(schemaURL string) WeaveID {
	for _, source := range activeGeneratedData.sources {
		if source.url == schemaURL {
			return source.id
		}
	}
	return WeaveUnknown
}

func isGeneratedWeave(id WeaveID) bool {
	for _, source := range activeGeneratedData.sources {
		if source.id == id {
			return true
		}
	}
	return false
}

// generatedRules selects an exact source weave when the ID belongs to the
// current generated source set. Reserved and invalid IDs use the generated
// unversioned fallback union without changing their observable SchemaState.
func generatedRules(data generatedData, id WeaveID) generatedRuleSet {
	for _, source := range data.sources {
		if source.id == id {
			return generatedRuleSet{
				weave: id, keyRules: data.keyRules,
				valueRules: data.valueRules, nameRules: data.nameRules,
			}
		}
	}
	return generatedRuleSet{
		weave: WeaveUnknown, keyRules: data.fallbackKeyRules,
		valueRules: data.fallbackValueRules, nameRules: data.fallbackNameRules,
	}
}
