// SPDX-License-Identifier: Apache-2.0

package typedir

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/migrationaudit"
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/registrydiff"
)

const targetRegistryURL = "semantic-conventions-v1.41.0/model"

type targetModel struct {
	attributes map[string]targetAttribute
}

type targetAttribute struct {
	name       string
	scalarType string
	enumValues map[string]struct{}
	deprecated *modelDeprecation
	locator    string
}

type modelDeprecation struct {
	Note      string `json:"note"`
	Reason    string `json:"reason"`
	RenamedTo string `json:"renamed_to,omitempty"`
}

type modelDocument struct {
	Groups      []modelGroup `json:"groups"`
	RegistryURL string       `json:"registry_url"`
}

// modelGroup lists the complete field union emitted by the pinned Weaver
// template. Fields outside the Registry attribute projection remain raw, but
// unknown fields still fail closed through strictDecodeJSON.
type modelGroup struct {
	Annotations        json.RawMessage  `json:"annotations,omitempty"`
	Attributes         []modelAttribute `json:"attributes,omitempty"`
	Body               json.RawMessage  `json:"body,omitempty"`
	Brief              json.RawMessage  `json:"brief,omitempty"`
	Deprecated         json.RawMessage  `json:"deprecated,omitempty"`
	DisplayName        json.RawMessage  `json:"display_name,omitempty"`
	EntityAssociations json.RawMessage  `json:"entity_associations,omitempty"`
	ID                 string           `json:"id"`
	Instrument         json.RawMessage  `json:"instrument,omitempty"`
	Lineage            json.RawMessage  `json:"lineage,omitempty"`
	MetricName         json.RawMessage  `json:"metric_name,omitempty"`
	Name               json.RawMessage  `json:"name,omitempty"`
	Note               json.RawMessage  `json:"note,omitempty"`
	SpanKind           json.RawMessage  `json:"span_kind,omitempty"`
	Stability          json.RawMessage  `json:"stability,omitempty"`
	Type               json.RawMessage  `json:"type,omitempty"`
	Unit               json.RawMessage  `json:"unit,omitempty"`
}

type modelAttribute struct {
	Annotations      json.RawMessage   `json:"annotations,omitempty"`
	Brief            json.RawMessage   `json:"brief,omitempty"`
	Deprecated       *modelDeprecation `json:"deprecated,omitempty"`
	Examples         json.RawMessage   `json:"examples,omitempty"`
	Name             string            `json:"name"`
	Note             json.RawMessage   `json:"note,omitempty"`
	RequirementLevel json.RawMessage   `json:"requirement_level,omitempty"`
	Role             json.RawMessage   `json:"role,omitempty"`
	SamplingRelevant json.RawMessage   `json:"sampling_relevant,omitempty"`
	Stability        json.RawMessage   `json:"stability,omitempty"`
	Tag              json.RawMessage   `json:"tag,omitempty"`
	Type             json.RawMessage   `json:"type"`
}

type enumType struct {
	Members []enumMember `json:"members"`
}

type enumMember struct {
	Annotations json.RawMessage `json:"annotations,omitempty"`
	Brief       json.RawMessage `json:"brief,omitempty"`
	Deprecated  json.RawMessage `json:"deprecated,omitempty"`
	ID          string          `json:"id"`
	Note        json.RawMessage `json:"note,omitempty"`
	Stability   json.RawMessage `json:"stability,omitempty"`
	Value       json.RawMessage `json:"value"`
}

func parseTargetModel(data []byte) (targetModel, error) {
	var document modelDocument
	if err := strictDecodeJSON(data, &document); err != nil {
		return targetModel{}, err
	}
	if document.RegistryURL != targetRegistryURL {
		return targetModel{}, fmt.Errorf(
			"registry_url = %q, want %q", document.RegistryURL, targetRegistryURL,
		)
	}
	result := targetModel{attributes: make(map[string]targetAttribute)}
	registryGroups := 0
	for groupIndex, group := range document.Groups {
		if !strings.HasPrefix(group.ID, "registry.") {
			continue
		}
		registryGroups++
		for attributeIndex, raw := range group.Attributes {
			if raw.Name == "" || len(raw.Type) == 0 {
				return targetModel{}, fmt.Errorf(
					"Registry group %q attribute[%d] lacks name/type", group.ID, attributeIndex,
				)
			}
			scalar, enumValues, err := decodeAttributeType(raw.Type)
			if err != nil {
				return targetModel{}, fmt.Errorf("Registry attribute %q type: %w", raw.Name, err)
			}
			if raw.Deprecated != nil {
				switch raw.Deprecated.Reason {
				case "renamed":
					if raw.Deprecated.RenamedTo == "" {
						return targetModel{}, fmt.Errorf("deprecated attribute %q lacks renamed_to", raw.Name)
					}
				case "obsoleted", "uncategorized":
					if raw.Deprecated.RenamedTo != "" {
						return targetModel{}, fmt.Errorf(
							"deprecated attribute %q unexpectedly has renamed_to", raw.Name,
						)
					}
				default:
					return targetModel{}, fmt.Errorf(
						"deprecated attribute %q has unknown reason %q",
						raw.Name, raw.Deprecated.Reason,
					)
				}
				if strings.TrimSpace(raw.Deprecated.Note) == "" {
					return targetModel{}, fmt.Errorf("deprecated attribute %q lacks note", raw.Name)
				}
			}
			if _, exists := result.attributes[raw.Name]; exists {
				return targetModel{}, fmt.Errorf("duplicate canonical Registry attribute %q", raw.Name)
			}
			result.attributes[raw.Name] = targetAttribute{
				name: raw.Name, scalarType: scalar, enumValues: enumValues,
				deprecated: raw.Deprecated,
				locator: fmt.Sprintf(
					"groups[%d:%s].attributes[%d:%s]",
					groupIndex, group.ID, attributeIndex, raw.Name,
				),
			}
		}
	}
	if registryGroups < 100 || len(result.attributes) < 900 {
		return targetModel{}, fmt.Errorf(
			"target Registry model is incomplete: groups=%d attributes=%d",
			registryGroups, len(result.attributes),
		)
	}
	return result, nil
}

func decodeAttributeType(data []byte) (string, map[string]struct{}, error) {
	var scalar string
	if err := json.Unmarshal(data, &scalar); err == nil {
		if scalar == "" {
			return "", nil, errors.New("empty scalar type")
		}
		return scalar, nil, nil
	}
	var enum enumType
	if err := strictDecodeJSON(data, &enum); err != nil {
		return "", nil, fmt.Errorf("decode enum type: %w", err)
	}
	if len(enum.Members) == 0 {
		return "", nil, errors.New("enum has no members")
	}
	values := make(map[string]struct{}, len(enum.Members))
	for index, member := range enum.Members {
		if member.ID == "" || len(member.Value) == 0 {
			return "", nil, fmt.Errorf("enum member[%d] lacks id/value", index)
		}
		var value string
		if err := json.Unmarshal(member.Value, &value); err != nil {
			// Integer-valued enums are valid model facts, but cannot be the
			// target of the string-only value projection rules compiled here.
			continue
		}
		// The Registry may retain multiple symbolic member IDs for the same
		// wire value during enum evolution. Runtime validation is value-based,
		// so membership is intentionally a set.
		values[value] = struct{}{}
	}
	return "enum", values, nil
}

func reconcileRegistryDiff(
	diff registrydiff.Document,
	reviewed []migrationaudit.ReviewedRule,
	model targetModel,
) error {
	reviews := make(map[string]migrationaudit.ReviewedRule)
	for _, rule := range reviewed {
		if rule.Authority != "weaver_diff" {
			continue
		}
		if _, exists := reviews[rule.ID]; exists {
			return fmt.Errorf("duplicate Weaver diff review %q", rule.ID)
		}
		reviews[rule.ID] = rule
	}

	required := 0
	for _, candidate := range diff.RegistryAttributes {
		if !candidate.IsMigrationObligation() {
			continue
		}
		source, ok := model.attributes[candidate.Source]
		if !ok || source.deprecated == nil {
			return fmt.Errorf(
				"candidate %q source is absent or not deprecated in target model",
				candidate.StableID(),
			)
		}
		if source.deprecated.Reason != candidate.Type {
			return fmt.Errorf(
				"candidate %q type %q disagrees with target model reason %q",
				candidate.StableID(), candidate.Type, source.deprecated.Reason,
			)
		}
		if candidate.Type == "renamed" {
			if source.deprecated.RenamedTo != candidate.Target {
				return fmt.Errorf(
					"candidate %q target %q disagrees with model renamed_to %q",
					candidate.StableID(), candidate.Target, source.deprecated.RenamedTo,
				)
			}
			target, exists := model.attributes[candidate.Target]
			if !exists || target.deprecated != nil {
				return fmt.Errorf(
					"candidate %q does not land on an active target attribute",
					candidate.StableID(),
				)
			}
		}

		if !candidate.HTTPRelated() && !candidate.DBRPCRelated() {
			continue
		}
		required++
		rule, exists := reviews[candidate.StableID()]
		if !exists {
			return fmt.Errorf("relevant candidate %q has no signed disposition", candidate.StableID())
		}
		delete(reviews, candidate.StableID())
		if rule.SourceKey != candidate.Source || rule.Locator != candidate.Locator() ||
			rule.Domain != "spans" || rule.Phase != "post_schema" ||
			len(rule.SourceURLs) != 2 {
			return fmt.Errorf("signed disposition %q does not match diff candidate", rule.ID)
		}
		switch candidate.Type {
		case "renamed":
			if len(rule.TargetKeys) != 1 || rule.TargetKeys[0] != candidate.Target {
				return fmt.Errorf("signed disposition %q has wrong rename target", rule.ID)
			}
			if rule.Action == "translated" && rule.Classification != "pure_rename" {
				return fmt.Errorf("signed disposition %q translates without pure_rename", rule.ID)
			}
		default:
			if len(rule.TargetKeys) != 0 || rule.Action == "translated" {
				return fmt.Errorf("signed disposition %q translates a non-rename candidate", rule.ID)
			}
		}
	}
	if required == 0 {
		return errors.New("Registry diff produced no relevant signed obligations")
	}
	if len(reviews) != 0 {
		extra := make([]string, 0, len(reviews))
		for id := range reviews {
			extra = append(extra, id)
		}
		sort.Strings(extra)
		return fmt.Errorf("signed audit contains stale Weaver diff dispositions: %q", extra)
	}
	return nil
}

func reconcileSourceWithTargetModel(source SourceIR, model targetModel) (SourceIR, error) {
	keyRules := make([]KeyRule, 0, len(source.KeyRules))
	dispositions := append([]Disposition(nil), source.Dispositions...)
	for _, rule := range source.KeyRules {
		attributeName := rule.Target
		if rule.Match == MatchPrefix {
			if !strings.HasSuffix(rule.Target, ".") {
				return SourceIR{}, fmt.Errorf("prefix target %q lacks trailing dot", rule.Target)
			}
			attributeName = strings.TrimSuffix(rule.Target, ".")
		}
		attribute, ok := model.attributes[attributeName]
		if !ok {
			dispositions = append(dispositions, modelMissingDisposition(
				rule.Domain, rule.Source, rule.Target, rule.Evidence,
			))
			continue
		}
		if rule.Match == MatchPrefix && attribute.scalarType != "template[string[]]" {
			return SourceIR{}, fmt.Errorf(
				"prefix target %q has model type %q, want template[string[]]",
				attributeName, attribute.scalarType,
			)
		}
		if attribute.deprecated != nil {
			dispositions = append(dispositions, modelBlockedDisposition(
				rule.Domain, rule.Source, rule.Target, attribute, rule.Evidence,
			))
			continue
		}
		keyRules = append(keyRules, rule)
	}

	valueRules := make([]ValueRule, 0, len(source.ValueRules))
	for _, rule := range source.ValueRules {
		attribute, ok := model.attributes[rule.TargetKey]
		if !ok {
			dispositions = append(dispositions, modelMissingDisposition(
				rule.Domain, rule.SourceKey, rule.TargetKey, rule.Evidence,
			))
			continue
		}
		if attribute.deprecated != nil {
			dispositions = append(dispositions, modelBlockedDisposition(
				rule.Domain, rule.SourceKey, rule.TargetKey, attribute, rule.Evidence,
			))
			continue
		}
		if attribute.scalarType != "enum" {
			return SourceIR{}, fmt.Errorf(
				"value target key %q has model type %q, want enum",
				rule.TargetKey, attribute.scalarType,
			)
		}
		if _, ok := attribute.enumValues[rule.TargetValue]; !ok {
			return SourceIR{}, fmt.Errorf(
				"value target %s=%q is absent from target Registry enum",
				rule.TargetKey, rule.TargetValue,
			)
		}
		valueRules = append(valueRules, rule)
	}

	source.KeyRules = keyRules
	source.ValueRules = valueRules
	source.Dispositions = dispositions
	sortIR(source.KeyRules, source.ValueRules, source.NameRules, source.Dispositions)
	return source, nil
}

func modelMissingDisposition(
	domain Domain,
	sourceKey, targetKey string,
	original []Evidence,
) Disposition {
	id := fmt.Sprintf("weaver-model:%s:%s:%s", domain, sourceKey, targetKey)
	evidence := append([]Evidence(nil), original...)
	evidence = append(evidence, Evidence{
		StableID: id, Authority: "weaver_target_model",
		Locator:         "registry-model.json#attributes",
		OperationSource: sourceKey, Phase: "target_model_reconciliation",
		Operation: -1, Mapping: -1,
	})
	return Disposition{
		Domain: domain, StableID: id, Action: "unsupported", Source: sourceKey,
		Reason: fmt.Sprintf(
			"Weaver target model does not contain target %q; preserve the original source instead of emitting a non-canonical key.",
			targetKey,
		),
		Evidence: evidence,
	}
}

func modelBlockedDisposition(
	domain Domain,
	sourceKey, targetKey string,
	attribute targetAttribute,
	original []Evidence,
) Disposition {
	id := fmt.Sprintf("weaver-model:%s:%s:%s", domain, sourceKey, targetKey)
	evidence := append([]Evidence(nil), original...)
	evidence = append(evidence, Evidence{
		StableID: id, Authority: "weaver_target_model", Locator: attribute.locator,
		OperationSource: sourceKey, Phase: "target_model_reconciliation",
		Operation: -1, Mapping: -1,
	})
	return Disposition{
		Domain: domain, StableID: id, Action: "pass_through", Source: sourceKey,
		Reason: fmt.Sprintf(
			"Weaver target model marks intermediate target %q deprecated (%s); preserve the original source instead of emitting a non-canonical key.",
			targetKey, attribute.deprecated.Reason,
		),
		Evidence: evidence,
	}
}

func strictDecodeJSON(data []byte, target any) error {
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
		return fmt.Errorf("trailing JSON content: %w", err)
	}
	return nil
}
