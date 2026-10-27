// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package semconv

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"
	"go.yaml.in/yaml/v3"

	"github.com/prometheus/prometheus/model/labels"
)

const transformFileFormat = "transform/2.0"

type policyScope uint8

const (
	policyScopeDatapoint policyScope = iota
	policyScopeResource
	policyScopeSpan
	policyScopeLog
)

type policyFieldKind uint8

const (
	policyFieldAttr policyFieldKind = iota
	policyFieldMetricName
	policyFieldDatapointValue
	policyFieldOther
)

// policyField addresses either a scoped attribute key or a descriptor/record/datapoint field.
type policyField struct {
	kind  policyFieldKind
	scope policyScope
	key   string
}

func (f policyField) isAttr() bool {
	return f.kind == policyFieldAttr
}

func (f policyField) isMetricName() bool {
	return f.kind == policyFieldMetricName
}

func (f policyField) isDatapointValue() bool {
	return f.kind == policyFieldDatapointValue
}

type policyKind uint8

const (
	policyKindMetricTransform policyKind = iota
	policyKindEntityTransform
	policyKindMetricFilter
	policyKindTraceTransform
	policyKindLogTransform
)

func (k policyKind) defaultScope() policyScope {
	switch k {
	case policyKindTraceTransform:
		return policyScopeSpan
	case policyKindLogTransform:
		return policyScopeLog
	case policyKindEntityTransform:
		return policyScopeResource
	default:
		return policyScopeDatapoint
	}
}

type predicateKind uint8

const (
	predicateExists predicateKind = iota
	predicateEquals
	predicateRegex
	predicateGT
	predicateGTE
	predicateLT
	predicateLTE
)

type policyPredicate struct {
	kind   predicateKind
	equals string
	regex  *regexp.Regexp
	rawRe  string
	num    float64
}

type policyMatcher struct {
	target    policyField
	predicate policyPredicate
	negate    bool
}

type policyRename struct {
	from policyField
	to   policyField
}

type policyRenamePrefix struct {
	from    policyField
	to      string
	replace map[string]string
}

type policySplit struct {
	from      policyField
	separator string
	to        []policyField
}

type policyAdd struct {
	target    policyField
	value     *string
	from      []policyField
	separator string
	upsert    bool
}

type policyMap struct {
	target   policyField
	values   map[string]string
	unmapped string
}

type policyConvert struct {
	target     policyField
	targetType string
}

type policyOps struct {
	remove       []policyField
	rename       []policyRename
	renamePrefix []policyRenamePrefix
	split        []policySplit
	add          []policyAdd
	mapValues    []policyMap
	convert      []policyConvert
	discard      []policyField
	scale        float64
}

type sampleValueRule struct {
	hasMatchValue   bool
	matchValue      float64
	dropValues      []float64
	hasReplaceValue bool
	replaceValue    float64
	scale           float64
}

func (r *sampleValueRule) isIdentity() bool {
	if r == nil {
		return true
	}
	return !r.hasMatchValue && len(r.dropValues) == 0 && !r.hasReplaceValue && (r.scale == 0 || r.scale == 1)
}

type transformPolicy struct {
	id           string
	name         string
	description  string
	kind         policyKind
	matches      []policyMatcher
	ops          policyOps
	filterAction string
}

// usesDatapointValue reports whether the policy inspects or mutates DATAPOINT_FIELD_VALUE,
// which cannot be evaluated purely on Prometheus series labels.
func (p *transformPolicy) usesDatapointValue() bool {
	for _, m := range p.matches {
		if m.target.isDatapointValue() {
			return true
		}
	}
	for _, f := range p.ops.remove {
		if f.isDatapointValue() {
			return true
		}
	}
	for _, r := range p.ops.rename {
		if r.from.isDatapointValue() || r.to.isDatapointValue() {
			return true
		}
	}
	for _, s := range p.ops.split {
		if s.from.isDatapointValue() {
			return true
		}
		for _, t := range s.to {
			if t.isDatapointValue() {
				return true
			}
		}
	}
	for _, a := range p.ops.add {
		if a.target.isDatapointValue() {
			return true
		}
		for _, f := range a.from {
			if f.isDatapointValue() {
				return true
			}
		}
	}
	for _, m := range p.ops.mapValues {
		if m.target.isDatapointValue() {
			return true
		}
	}
	for _, d := range p.ops.discard {
		if d.isDatapointValue() {
			return true
		}
	}
	return false
}

// targetMetricName returns the metric name this policy writes via add on
// METRIC_DESCRIPTOR_FIELD_NAME, or ("", false) when the policy does not rename the metric.
func (p *transformPolicy) targetMetricName() (string, bool) {
	if p.kind != policyKindMetricTransform {
		return "", false
	}
	var out string
	var found bool
	for _, a := range p.ops.add {
		if a.target.isMetricName() && a.value != nil && *a.value != "" {
			out = *a.value
			found = true
		}
	}
	return out, found
}

// sourceMetricNames returns the literal source metric names matched by this policy's
// METRIC_DESCRIPTOR_FIELD_NAME matchers.
func (p *transformPolicy) sourceMetricNames() []string {
	var names []string
	for _, m := range p.matches {
		if !m.target.isMetricName() || m.negate {
			continue
		}
		switch m.predicate.kind {
		case predicateEquals:
			if m.predicate.equals != "" && !slices.Contains(names, m.predicate.equals) {
				names = append(names, m.predicate.equals)
			}
		case predicateRegex:
			for _, alt := range extractRegexLiteralAlternatives(m.predicate.rawRe) {
				if !slices.Contains(names, alt) {
					names = append(names, alt)
				}
			}
		}
	}
	return names
}

// matchesMetricName reports whether this policy's METRIC_DESCRIPTOR_FIELD_NAME matchers
// allow metricName.
func (p *transformPolicy) matchesMetricName(metricName string) bool {
	for _, m := range p.matches {
		if !m.target.isMetricName() {
			continue
		}
		if !m.matchesValue(metricName, metricName != "") {
			return false
		}
	}
	return true
}

// transformFile represents a parsed file_format: transform/2.0 schema file
// (see open-telemetry/weaver#1850).
type transformFile struct {
	sourceURL string
	targetURL string
	policies  []transformPolicy
}

type rawTransformEndpoint struct {
	SchemaURL    string `yaml:"schema_url"`
	RegistryPath string `yaml:"registry_path"`
}

type rawTransformFile struct {
	FileFormat string               `yaml:"file_format"`
	Source     rawTransformEndpoint `yaml:"source"`
	Target     rawTransformEndpoint `yaml:"target"`
	Gaps       yaml.Node            `yaml:"gaps"`
	Policies   []yaml.Node          `yaml:"policies"`
}

// loadTransformFile parses and validates a transform/2.0 YAML document.
func loadTransformFile(b []byte) (*transformFile, error) {
	var raw rawTransformFile
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal transform/2.0 file: %w", err)
	}
	if raw.FileFormat != transformFileFormat {
		return nil, fmt.Errorf("expected file_format %q, got %q", transformFileFormat, raw.FileFormat)
	}
	tf := &transformFile{
		sourceURL: raw.Source.SchemaURL,
		targetURL: raw.Target.SchemaURL,
		policies:  make([]transformPolicy, 0, len(raw.Policies)),
	}
	for i := range raw.Policies {
		p, err := parseTransformPolicy(&raw.Policies[i], i+1)
		if err != nil {
			return nil, err
		}
		tf.policies = append(tf.policies, p)
	}
	return tf, nil
}

func parseTransformPolicy(node *yaml.Node, index int) (transformPolicy, error) {
	fields, _, err := decodeEffectiveYAMLMapping(node)
	if err != nil {
		return transformPolicy{}, fmt.Errorf("policy %d: %w", index, err)
	}
	idNode, hasID := fields["id"]
	id := fmt.Sprintf("<policy %d>", index)
	if hasID {
		id, err = decodeYAMLScalar(idNode)
		if err != nil {
			return transformPolicy{}, fmt.Errorf("policy %d id: %w", index, err)
		}
	}
	var name, desc string
	if n, ok := fields["name"]; ok {
		name, _ = decodeYAMLScalar(n)
	}
	if d, ok := fields["description"]; ok {
		desc, _ = decodeYAMLScalar(d)
	}

	type kindEntry struct {
		key  string
		kind policyKind
	}
	knownKinds := []kindEntry{
		{key: "metric_transform", kind: policyKindMetricTransform},
		{key: "entity_transform", kind: policyKindEntityTransform},
		{key: "metric_filter", kind: policyKindMetricFilter},
		{key: "trace_transform", kind: policyKindTraceTransform},
		{key: "log_transform", kind: policyKindLogTransform},
	}

	var (
		matchedKind policyKind
		bodyNode    yaml.Node
		foundKind   bool
	)
	for _, ke := range knownKinds {
		if b, ok := fields[ke.key]; ok {
			if foundKind {
				return transformPolicy{}, fmt.Errorf("policy %q declares multiple transform blocks", id)
			}
			matchedKind = ke.kind
			bodyNode = b
			foundKind = true
		}
	}
	if !foundKind {
		return transformPolicy{}, fmt.Errorf("policy %q has no supported transform block", id)
	}

	bodyFields, _, err := decodeEffectiveYAMLMapping(&bodyNode)
	if err != nil {
		return transformPolicy{}, fmt.Errorf("policy %q body: %w", id, err)
	}

	var matches []policyMatcher
	if matchesNode, ok := bodyFields["matches"]; ok {
		if matchesNode.Kind != yaml.SequenceNode {
			return transformPolicy{}, fmt.Errorf("policy %q matches must be a sequence", id)
		}
		matches = make([]policyMatcher, 0, len(matchesNode.Content))
		for _, mNode := range matchesNode.Content {
			m, err := parsePolicyMatcher(mNode)
			if err != nil {
				return transformPolicy{}, fmt.Errorf("policy %q matcher: %w", id, err)
			}
			matches = append(matches, m)
		}
	}

	var filterAction string
	if matchedKind == policyKindMetricFilter {
		if actNode, ok := bodyFields["action"]; ok {
			filterAction, err = decodeYAMLScalar(actNode)
			if err != nil {
				return transformPolicy{}, fmt.Errorf("policy %q action: %w", id, err)
			}
		}
	}

	ops, err := parsePolicyOps(bodyFields, matchedKind.defaultScope())
	if err != nil {
		return transformPolicy{}, fmt.Errorf("policy %q: %w", id, err)
	}

	return transformPolicy{
		id:           id,
		name:         name,
		description:  desc,
		kind:         matchedKind,
		matches:      matches,
		ops:          ops,
		filterAction: filterAction,
	}, nil
}

func parsePolicyField(node *yaml.Node) (policyField, error) {
	fields, keys, err := decodeEffectiveYAMLMapping(node)
	if err != nil {
		return policyField{}, fmt.Errorf("field is not a mapping: %w", err)
	}
	if len(keys) != 1 {
		return policyField{}, fmt.Errorf("field mapping must have exactly one key, found %v", keys)
	}
	k := keys[0]
	val := fields[k]

	decodeAttrKey := func(scope policyScope) (policyField, error) {
		if val.Kind != yaml.SequenceNode || len(val.Content) == 0 {
			return policyField{}, fmt.Errorf("attribute path sequence expected for %s", k)
		}
		attrKey, err := decodeYAMLScalar(*val.Content[0])
		if err != nil || attrKey == "" {
			return policyField{}, fmt.Errorf("invalid attribute key for %s", k)
		}
		return policyField{kind: policyFieldAttr, scope: scope, key: attrKey}, nil
	}

	switch k {
	case "datapoint_attribute":
		return decodeAttrKey(policyScopeDatapoint)
	case "resource_attribute":
		return decodeAttrKey(policyScopeResource)
	case "span_attribute":
		return decodeAttrKey(policyScopeSpan)
	case "log_attribute":
		return decodeAttrKey(policyScopeLog)
	case "descriptor_field", "record_field", "datapoint_field":
		fieldName, err := decodeYAMLScalar(val)
		if err != nil || fieldName == "" {
			return policyField{}, fmt.Errorf("invalid field name for %s", k)
		}
		switch fieldName {
		case "METRIC_DESCRIPTOR_FIELD_NAME":
			return policyField{kind: policyFieldMetricName, key: fieldName}, nil
		case "DATAPOINT_FIELD_VALUE":
			return policyField{kind: policyFieldDatapointValue, key: fieldName}, nil
		default:
			return policyField{kind: policyFieldOther, key: fieldName}, nil
		}
	default:
		return policyField{}, fmt.Errorf("unknown field kind %q", k)
	}
}

func parsePolicyMatcher(node *yaml.Node) (policyMatcher, error) {
	fields, _, err := decodeEffectiveYAMLMapping(node)
	if err != nil {
		return policyMatcher{}, err
	}
	targetNode, ok := fields["target"]
	if !ok {
		return policyMatcher{}, fmt.Errorf("matcher missing target")
	}
	target, err := parsePolicyField(&targetNode)
	if err != nil {
		return policyMatcher{}, err
	}
	predNode, ok := fields["predicate"]
	if !ok {
		return policyMatcher{}, fmt.Errorf("matcher missing predicate")
	}
	predFields, predKeys, err := decodeEffectiveYAMLMapping(&predNode)
	if err != nil || len(predKeys) != 1 {
		return policyMatcher{}, fmt.Errorf("predicate must be a single-key mapping")
	}
	pk := predKeys[0]
	pv := predFields[pk]

	var pred policyPredicate
	switch pk {
	case "exists":
		pred = policyPredicate{kind: predicateExists}
	case "equals":
		s, err := decodeYAMLScalar(pv)
		if err != nil {
			return policyMatcher{}, fmt.Errorf("equals predicate: %w", err)
		}
		pred = policyPredicate{kind: predicateEquals, equals: s}
	case "regex":
		s, err := decodeYAMLScalar(pv)
		if err != nil {
			return policyMatcher{}, fmt.Errorf("regex predicate: %w", err)
		}
		re, err := regexp.Compile(s)
		if err != nil {
			return policyMatcher{}, fmt.Errorf("compile regex %q: %w", s, err)
		}
		pred = policyPredicate{kind: predicateRegex, regex: re, rawRe: s}
	case "gt", "gte", "lt", "lte":
		s, err := decodeYAMLScalar(pv)
		if err != nil {
			return policyMatcher{}, fmt.Errorf("%s predicate: %w", pk, err)
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return policyMatcher{}, fmt.Errorf("%s predicate float %q: %w", pk, s, err)
		}
		var kind predicateKind
		switch pk {
		case "gt":
			kind = predicateGT
		case "gte":
			kind = predicateGTE
		case "lt":
			kind = predicateLT
		case "lte":
			kind = predicateLTE
		}
		pred = policyPredicate{kind: kind, num: n}
	default:
		return policyMatcher{}, fmt.Errorf("unknown predicate %q", pk)
	}

	var negate bool
	if negNode, ok := fields["negate"]; ok {
		if err := negNode.Decode(&negate); err != nil {
			return policyMatcher{}, fmt.Errorf("negate: %w", err)
		}
	}
	return policyMatcher{
		target:    target,
		predicate: pred,
		negate:    negate,
	}, nil
}

func deriveAttrField(from policyField, key string, defaultScope policyScope) policyField {
	scope := defaultScope
	if from.isAttr() {
		scope = from.scope
	}
	return policyField{kind: policyFieldAttr, scope: scope, key: key}
}

func parsePolicyOps(bodyFields map[string]yaml.Node, defaultScope policyScope) (policyOps, error) {
	var ops policyOps

	if remNode, ok := bodyFields["remove"]; ok {
		if remNode.Kind != yaml.SequenceNode {
			return policyOps{}, fmt.Errorf("remove must be a sequence")
		}
		for _, item := range remNode.Content {
			m, _, err := decodeEffectiveYAMLMapping(item)
			if err != nil {
				return policyOps{}, fmt.Errorf("remove item: %w", err)
			}
			tNode, ok := m["target"]
			if !ok {
				return policyOps{}, fmt.Errorf("remove missing target")
			}
			f, err := parsePolicyField(&tNode)
			if err != nil {
				return policyOps{}, fmt.Errorf("remove target: %w", err)
			}
			ops.remove = append(ops.remove, f)
		}
	}

	if discNode, ok := bodyFields["discard"]; ok {
		if discNode.Kind != yaml.SequenceNode {
			return policyOps{}, fmt.Errorf("discard must be a sequence")
		}
		for _, item := range discNode.Content {
			m, _, err := decodeEffectiveYAMLMapping(item)
			if err != nil {
				return policyOps{}, fmt.Errorf("discard item: %w", err)
			}
			tNode, ok := m["target"]
			if !ok {
				return policyOps{}, fmt.Errorf("discard missing target")
			}
			f, err := parsePolicyField(&tNode)
			if err != nil {
				return policyOps{}, fmt.Errorf("discard target: %w", err)
			}
			ops.discard = append(ops.discard, f)
		}
	}

	if renNode, ok := bodyFields["rename"]; ok {
		if renNode.Kind != yaml.SequenceNode {
			return policyOps{}, fmt.Errorf("rename must be a sequence")
		}
		for _, item := range renNode.Content {
			m, _, err := decodeEffectiveYAMLMapping(item)
			if err != nil {
				return policyOps{}, fmt.Errorf("rename item: %w", err)
			}
			fNode, ok := m["from"]
			if !ok {
				return policyOps{}, fmt.Errorf("rename missing from")
			}
			fromField, err := parsePolicyField(&fNode)
			if err != nil {
				return policyOps{}, fmt.Errorf("rename from: %w", err)
			}
			var toField policyField
			if toTargetNode, ok := m["to_target"]; ok {
				toField, err = parsePolicyField(&toTargetNode)
				if err != nil {
					return policyOps{}, fmt.Errorf("rename to_target: %w", err)
				}
			} else if toNode, ok := m["to"]; ok {
				toKey, err := decodeYAMLScalar(toNode)
				if err != nil || toKey == "" {
					return policyOps{}, fmt.Errorf("rename to: invalid key")
				}
				toField = deriveAttrField(fromField, toKey, defaultScope)
			} else {
				return policyOps{}, fmt.Errorf("rename missing to or to_target")
			}
			ops.rename = append(ops.rename, policyRename{from: fromField, to: toField})
		}
	}

	if rpNode, ok := bodyFields["rename_prefix"]; ok {
		if rpNode.Kind != yaml.SequenceNode {
			return policyOps{}, fmt.Errorf("rename_prefix must be a sequence")
		}
		for _, item := range rpNode.Content {
			m, _, err := decodeEffectiveYAMLMapping(item)
			if err != nil {
				return policyOps{}, fmt.Errorf("rename_prefix item: %w", err)
			}
			fNode, ok := m["from"]
			if !ok {
				return policyOps{}, fmt.Errorf("rename_prefix missing from")
			}
			fromField, err := parsePolicyField(&fNode)
			if err != nil {
				return policyOps{}, fmt.Errorf("rename_prefix from: %w", err)
			}
			toNode, ok := m["to"]
			if !ok {
				return policyOps{}, fmt.Errorf("rename_prefix missing to")
			}
			toPrefix, err := decodeYAMLScalar(toNode)
			if err != nil {
				return policyOps{}, fmt.Errorf("rename_prefix to: %w", err)
			}
			var replace map[string]string
			if repNode, ok := m["replace"]; ok {
				replace, err = decodeStringMap(&repNode)
				if err != nil {
					return policyOps{}, fmt.Errorf("rename_prefix replace: %w", err)
				}
			}
			ops.renamePrefix = append(ops.renamePrefix, policyRenamePrefix{
				from:    fromField,
				to:      toPrefix,
				replace: replace,
			})
		}
	}

	if splitNode, ok := bodyFields["split"]; ok {
		if splitNode.Kind != yaml.SequenceNode {
			return policyOps{}, fmt.Errorf("split must be a sequence")
		}
		for _, item := range splitNode.Content {
			m, _, err := decodeEffectiveYAMLMapping(item)
			if err != nil {
				return policyOps{}, fmt.Errorf("split item: %w", err)
			}
			fNode, ok := m["from"]
			if !ok {
				return policyOps{}, fmt.Errorf("split missing from")
			}
			fromField, err := parsePolicyField(&fNode)
			if err != nil {
				return policyOps{}, fmt.Errorf("split from: %w", err)
			}
			var sep string
			if sepNode, ok := m["separator"]; ok {
				sep, err = decodeYAMLScalar(sepNode)
				if err != nil {
					return policyOps{}, fmt.Errorf("split separator: %w", err)
				}
			}
			toNode, ok := m["to"]
			if !ok || toNode.Kind != yaml.SequenceNode {
				return policyOps{}, fmt.Errorf("split to must be a sequence")
			}
			toFields := make([]policyField, 0, len(toNode.Content))
			for _, elem := range toNode.Content {
				k, err := decodeYAMLScalar(*elem)
				if err != nil || k == "" {
					return policyOps{}, fmt.Errorf("split to element must be a non-empty string")
				}
				toFields = append(toFields, deriveAttrField(fromField, k, defaultScope))
			}
			ops.split = append(ops.split, policySplit{
				from:      fromField,
				separator: sep,
				to:        toFields,
			})
		}
	}

	if addNode, ok := bodyFields["add"]; ok {
		if addNode.Kind != yaml.SequenceNode {
			return policyOps{}, fmt.Errorf("add must be a sequence")
		}
		for _, item := range addNode.Content {
			m, _, err := decodeEffectiveYAMLMapping(item)
			if err != nil {
				return policyOps{}, fmt.Errorf("add item: %w", err)
			}
			tNode, ok := m["target"]
			if !ok {
				return policyOps{}, fmt.Errorf("add missing target")
			}
			targetField, err := parsePolicyField(&tNode)
			if err != nil {
				return policyOps{}, fmt.Errorf("add target: %w", err)
			}
			var valPtr *string
			if vNode, ok := m["value"]; ok {
				vStr, err := decodeYAMLScalar(vNode)
				if err != nil {
					return policyOps{}, fmt.Errorf("add value: %w", err)
				}
				valPtr = &vStr
			}
			var fromFields []policyField
			if fNode, ok := m["from"]; ok {
				if fNode.Kind == yaml.SequenceNode {
					for _, elem := range fNode.Content {
						ff, err := parsePolicyField(elem)
						if err != nil {
							return policyOps{}, fmt.Errorf("add from element: %w", err)
						}
						fromFields = append(fromFields, ff)
					}
				} else {
					ff, err := parsePolicyField(&fNode)
					if err != nil {
						return policyOps{}, fmt.Errorf("add from: %w", err)
					}
					fromFields = append(fromFields, ff)
				}
			}
			var sep string
			if sepNode, ok := m["separator"]; ok {
				sep, err = decodeYAMLScalar(sepNode)
				if err != nil {
					return policyOps{}, fmt.Errorf("add separator: %w", err)
				}
			}
			var upsert bool
			if upNode, ok := m["upsert"]; ok {
				if err := upNode.Decode(&upsert); err != nil {
					return policyOps{}, fmt.Errorf("add upsert: %w", err)
				}
			}
			ops.add = append(ops.add, policyAdd{
				target:    targetField,
				value:     valPtr,
				from:      fromFields,
				separator: sep,
				upsert:    upsert,
			})
		}
	}

	if mapNode, ok := bodyFields["map"]; ok {
		if mapNode.Kind != yaml.SequenceNode {
			return policyOps{}, fmt.Errorf("map must be a sequence")
		}
		for _, item := range mapNode.Content {
			m, _, err := decodeEffectiveYAMLMapping(item)
			if err != nil {
				return policyOps{}, fmt.Errorf("map item: %w", err)
			}
			tNode, ok := m["target"]
			if !ok {
				return policyOps{}, fmt.Errorf("map missing target")
			}
			targetField, err := parsePolicyField(&tNode)
			if err != nil {
				return policyOps{}, fmt.Errorf("map target: %w", err)
			}
			vNode, ok := m["values"]
			if !ok {
				return policyOps{}, fmt.Errorf("map missing values")
			}
			values, err := decodeStringMap(&vNode)
			if err != nil {
				return policyOps{}, fmt.Errorf("map values: %w", err)
			}
			unmapped := "keep"
			if uNode, ok := m["unmapped"]; ok {
				unmapped, err = decodeYAMLScalar(uNode)
				if err != nil {
					return policyOps{}, fmt.Errorf("map unmapped: %w", err)
				}
			}
			ops.mapValues = append(ops.mapValues, policyMap{
				target:   targetField,
				values:   values,
				unmapped: unmapped,
			})
		}
	}

	if convNode, ok := bodyFields["convert"]; ok {
		if convNode.Kind != yaml.SequenceNode {
			return policyOps{}, fmt.Errorf("convert must be a sequence")
		}
		for _, item := range convNode.Content {
			m, _, err := decodeEffectiveYAMLMapping(item)
			if err != nil {
				return policyOps{}, fmt.Errorf("convert item: %w", err)
			}
			tNode, ok := m["target"]
			if !ok {
				return policyOps{}, fmt.Errorf("convert missing target")
			}
			targetField, err := parsePolicyField(&tNode)
			if err != nil {
				return policyOps{}, fmt.Errorf("convert target: %w", err)
			}
			var targetType string
			if tyNode, ok := m["type"]; ok {
				targetType, err = decodeYAMLScalar(tyNode)
				if err != nil {
					return policyOps{}, fmt.Errorf("convert type: %w", err)
				}
			}
			ops.convert = append(ops.convert, policyConvert{
				target:     targetField,
				targetType: targetType,
			})
		}
	}

	if scaleNode, ok := bodyFields["scale"]; ok {
		s, err := decodeYAMLScalar(scaleNode)
		if err != nil {
			return policyOps{}, fmt.Errorf("scale: %w", err)
		}
		scaleVal, err := strconv.ParseFloat(s, 64)
		if err != nil || scaleVal == 0 {
			return policyOps{}, fmt.Errorf("scale must be a non-zero float, got %q", s)
		}
		ops.scale = scaleVal
	}

	return ops, nil
}

func decodeYAMLScalar(node yaml.Node) (string, error) {
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		node = *node.Alias
	}
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("expected scalar YAML node, got kind %d", node.Kind)
	}
	return node.Value, nil
}

func decodeStringMap(node *yaml.Node) (map[string]string, error) {
	fields, _, err := decodeEffectiveYAMLMapping(node)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(fields))
	for k, vNode := range fields {
		v, err := decodeYAMLScalar(vNode)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// extractRegexLiteralAlternatives extracts literal strings from simple anchored regexes
// such as `^http\.server\.(duration|request\.size)$` or `^(a|b)$` when used on metric names.
func extractRegexLiteralAlternatives(expr string) []string {
	s := strings.TrimPrefix(expr, "^")
	s = strings.TrimSuffix(s, "$")
	openIdx := strings.IndexByte(s, '(')
	closeIdx := strings.LastIndexByte(s, ')')
	var rawParts []string
	if openIdx >= 0 && closeIdx > openIdx && !strings.ContainsAny(s[:openIdx], "()|") && !strings.ContainsAny(s[closeIdx+1:], "()|") {
		prefix := s[:openIdx]
		suffix := s[closeIdx+1:]
		for _, alt := range strings.Split(s[openIdx+1:closeIdx], "|") {
			rawParts = append(rawParts, prefix+alt+suffix)
		}
	} else if !strings.ContainsAny(s, "()") {
		rawParts = strings.Split(s, "|")
	}
	var out []string
	for _, p := range rawParts {
		unescaped := strings.ReplaceAll(p, `\.`, ".")
		if strings.ContainsAny(unescaped, `*+?[]{}^\$`) {
			continue
		}
		if unescaped != "" {
			out = append(out, unescaped)
		}
	}
	return out
}

func (m policyMatcher) matchesLabels(lbls labels.Labels) bool {
	switch m.target.kind {
	case policyFieldMetricName:
		val := lbls.Get(model.MetricNameLabel)
		return m.matchesValue(val, val != "")
	case policyFieldAttr:
		if m.target.scope != policyScopeDatapoint && m.target.scope != policyScopeResource {
			return false
		}
		val := lbls.Get(m.target.key)
		return m.matchesValue(val, val != "")
	default:
		return false
	}
}

func (m policyMatcher) matchesValue(val string, exists bool) bool {
	var hit bool
	switch m.predicate.kind {
	case predicateExists:
		hit = exists
	case predicateEquals:
		hit = exists && val == m.predicate.equals
	case predicateRegex:
		hit = exists && m.predicate.regex.MatchString(val)
	case predicateGT, predicateGTE, predicateLT, predicateLTE:
		if exists {
			if x, err := strconv.ParseFloat(val, 64); err == nil {
				switch m.predicate.kind {
				case predicateGT:
					hit = x > m.predicate.num
				case predicateGTE:
					hit = x >= m.predicate.num
				case predicateLT:
					hit = x < m.predicate.num
				case predicateLTE:
					hit = x <= m.predicate.num
				}
			}
		}
	}
	if m.negate {
		return !hit
	}
	return hit
}

// policySeriesTransformer executes a transform/2.0 policy set against incoming series labels.
type policySeriesTransformer struct {
	entityPolicies []transformPolicy
	metricPolicies []transformPolicy
	histSuffix     string
	presetRule     *sampleValueRule
}

func (t *policySeriesTransformer) transformLabels(originalLabels labels.Labels) (labels.Labels, error) {
	lbls, _, err := t.transformSeries(originalLabels)
	return lbls, err
}

func (t *policySeriesTransformer) transformSeries(originalLabels labels.Labels) (labels.Labels, *sampleValueRule, error) {
	matchLabels := originalLabels
	rawMetricName := originalLabels.Get(model.MetricNameLabel)
	if t.histSuffix != "" && strings.HasSuffix(rawMetricName, t.histSuffix) {
		b := labels.NewBuilder(originalLabels)
		b.Set(model.MetricNameLabel, strings.TrimSuffix(rawMetricName, t.histSuffix))
		matchLabels = b.Labels()
	}

	var matchedEntities []*transformPolicy
	for i := range t.entityPolicies {
		p := &t.entityPolicies[i]
		if policyMatchesSeries(p, matchLabels) {
			matchedEntities = append(matchedEntities, p)
		}
	}

	var (
		matchedMetrics []*transformPolicy
		dropVals       []float64
		matchVal       float64
		hasMatchVal    bool
		needsAttrVal   bool
		scaleVal       float64
	)
	for i := range t.metricPolicies {
		p := &t.metricPolicies[i]
		if !policyMatchesSeries(p, matchLabels) {
			continue
		}
		if p.kind == policyKindMetricFilter && p.filterAction == "ACTION_DROP" {
			if dv, ok := extractDatapointEqualsMatcher(p); ok {
				dropVals = append(dropVals, dv)
				continue
			}
			return labels.EmptyLabels(), nil, nil
		}
		if mv, ok := extractDatapointEqualsMatcher(p); ok {
			hasMatchVal = true
			matchVal = mv
		}
		for _, r := range p.ops.rename {
			if r.from.isAttr() && r.to.isDatapointValue() {
				needsAttrVal = true
			}
		}
		if p.ops.scale != 0 && t.histSuffix != "_count" {
			scaleVal = p.ops.scale
		}
		matchedMetrics = append(matchedMetrics, p)
	}

	metricName := matchLabels.Get(model.MetricNameLabel)
	attrs := make(map[string]string, originalLabels.Len())
	hasReserved := false
	originalLabels.Range(func(l labels.Label) {
		if isReservedLabel(l.Name) {
			hasReserved = true
			return
		}
		if l.Name == model.MetricNameLabel {
			return
		}
		attrs[l.Name] = l.Value
	})

	var rule *sampleValueRule
	if t.presetRule != nil {
		rCopy := *t.presetRule
		rule = &rCopy
	}
	if len(dropVals) > 0 || hasMatchVal || scaleVal != 0 {
		if rule == nil {
			rule = &sampleValueRule{}
		}
		rule.dropValues = append(rule.dropValues, dropVals...)
		if hasMatchVal && !rule.hasMatchValue {
			rule.hasMatchValue = true
			rule.matchValue = matchVal
		}
		if scaleVal != 0 && rule.scale == 0 {
			rule.scale = scaleVal
		}
	}

	if len(matchedEntities) == 0 && len(matchedMetrics) == 0 {
		if t.histSuffix != "" && metricName != "" {
			metricName += t.histSuffix
		}
		if !hasReserved && metricName == rawMetricName {
			return originalLabels, rule, nil
		}
		return buildSeriesLabels(metricName, attrs), rule, nil
	}

	var dpValueStr string
	if len(matchedEntities) > 0 {
		applyPolicyBatch(&metricName, attrs, &dpValueStr, matchedEntities)
	}
	if len(matchedMetrics) > 0 {
		applyPolicyBatch(&metricName, attrs, &dpValueStr, matchedMetrics)
	}

	if needsAttrVal {
		repVal, err := strconv.ParseFloat(dpValueStr, 64)
		if err != nil {
			return labels.EmptyLabels(), nil, nil
		}
		if rule == nil {
			rule = &sampleValueRule{}
		}
		rule.hasReplaceValue = true
		rule.replaceValue = repVal
	} else if dpValueStr != "" && (rule == nil || !rule.hasReplaceValue) {
		if repVal, err := strconv.ParseFloat(dpValueStr, 64); err == nil {
			if rule == nil {
				rule = &sampleValueRule{}
			}
			rule.hasReplaceValue = true
			rule.replaceValue = repVal
		}
	}

	if t.histSuffix != "" && metricName != "" {
		metricName += t.histSuffix
	}
	if rule != nil && rule.isIdentity() {
		rule = nil
	}
	return buildSeriesLabels(metricName, attrs), rule, nil
}

func extractDatapointEqualsMatcher(p *transformPolicy) (float64, bool) {
	for _, m := range p.matches {
		if m.target.isDatapointValue() && !m.negate && m.predicate.kind == predicateEquals {
			if v, err := strconv.ParseFloat(m.predicate.equals, 64); err == nil {
				return v, true
			}
		}
	}
	return 0, false
}

// mapLabelValue applies any value mapping for canonicalAttr when answering LabelValues queries.
func (t *policySeriesTransformer) mapLabelValue(canonicalAttr, val string) (string, bool) {
	for _, p := range t.metricPolicies {
		for _, m := range p.ops.mapValues {
			if m.target.isAttr() && m.target.key == canonicalAttr {
				if mapped, ok := m.values[val]; ok {
					return mapped, true
				}
				if m.unmapped == "remove" {
					return "", false
				}
			}
		}
	}
	return val, true
}

func policyMatchesSeries(p *transformPolicy, lbls labels.Labels) bool {
	for _, m := range p.matches {
		if m.target.isDatapointValue() {
			continue
		}
		if !m.matchesLabels(lbls) {
			return false
		}
	}
	return true
}

// applyPolicyBatch executes all matching policies in the fixed 7-stage pipeline defined
// by OTEP #4738 and open-telemetry/weaver#1850:
// remove -> rename / rename_prefix -> split -> add -> map -> convert -> discard.
func applyPolicyBatch(metricName *string, attrs map[string]string, dpValueStr *string, policies []*transformPolicy) {
	// Stage 1: remove.
	for _, p := range policies {
		for _, f := range p.ops.remove {
			if f.isAttr() {
				delete(attrs, f.key)
			}
		}
	}

	// Stage 2: rename and rename_prefix.
	for _, p := range policies {
		for _, r := range p.ops.rename {
			if !r.from.isAttr() {
				continue
			}
			val, ok := attrs[r.from.key]
			if !ok {
				continue
			}
			delete(attrs, r.from.key)
			if r.to.isMetricName() {
				if val != "" {
					*metricName = val
				}
			} else if r.to.isDatapointValue() {
				if dpValueStr != nil {
					*dpValueStr = val
				}
			} else if r.to.isAttr() {
				if _, exists := attrs[r.to.key]; !exists && val != "" {
					attrs[r.to.key] = val
				}
			}
		}
		for _, rp := range p.ops.renamePrefix {
			if !rp.from.isAttr() {
				continue
			}
			prefix := rp.from.key
			keys := slices.Sorted(maps.Keys(attrs))
			for _, k := range keys {
				kWithDot := k + "."
				if !strings.HasPrefix(kWithDot, prefix) {
					continue
				}
				suffix := strings.TrimSuffix(kWithDot[len(prefix):], ".")
				if len(rp.replace) > 0 {
					for _, oldSub := range slices.Sorted(maps.Keys(rp.replace)) {
						suffix = strings.ReplaceAll(suffix, oldSub, rp.replace[oldSub])
					}
				}
				newKey := strings.TrimSuffix(rp.to+suffix, ".")
				if newKey == k || newKey == "" {
					continue
				}
				val := attrs[k]
				delete(attrs, k)
				if _, exists := attrs[newKey]; !exists && val != "" {
					attrs[newKey] = val
				}
			}
		}
	}

	// Stage 3: split.
	for _, p := range policies {
		for _, s := range p.ops.split {
			if !s.from.isAttr() || len(s.to) == 0 {
				continue
			}
			val, ok := attrs[s.from.key]
			if !ok {
				continue
			}
			parts := strings.SplitN(val, s.separator, len(s.to))
			for i, target := range s.to {
				if !target.isAttr() {
					continue
				}
				if i < len(parts) && parts[i] != "" {
					if target.key == s.from.key {
						attrs[target.key] = parts[i]
					} else if _, exists := attrs[target.key]; !exists {
						attrs[target.key] = parts[i]
					}
				} else if target.key == s.from.key {
					delete(attrs, s.from.key)
				}
			}
		}
	}

	// Stage 4: add.
	for _, p := range policies {
		for _, a := range p.ops.add {
			var (
				valToWrite string
				hasVal     bool
			)
			if len(a.from) > 0 {
				parts := make([]string, 0, len(a.from))
				allPresent := true
				for _, ff := range a.from {
					if ff.isMetricName() && *metricName != "" {
						parts = append(parts, *metricName)
					} else if ff.isAttr() {
						if v, ok := attrs[ff.key]; ok && v != "" {
							parts = append(parts, v)
						} else {
							allPresent = false
						}
					} else {
						allPresent = false
					}
				}
				// When joining multiple fields with a separator (e.g. rpc.service + rpc.method),
				// require all source fields to be present so target-shaped attributes are not corrupted.
				if len(a.from) == 1 && len(parts) == 1 {
					valToWrite = parts[0]
					hasVal = true
				} else if len(a.from) > 1 && allPresent {
					valToWrite = strings.Join(parts, a.separator)
					hasVal = valToWrite != ""
				}
			} else if a.value != nil {
				valToWrite = *a.value
				hasVal = valToWrite != ""
			}
			if !hasVal {
				continue
			}

			if a.target.isMetricName() {
				if a.upsert || *metricName == "" {
					*metricName = valToWrite
				}
			} else if a.target.isDatapointValue() {
				if dpValueStr != nil && (a.upsert || *dpValueStr == "") {
					*dpValueStr = valToWrite
				}
			} else if a.target.isAttr() {
				if _, exists := attrs[a.target.key]; !exists || a.upsert {
					attrs[a.target.key] = valToWrite
				}
			}
		}
	}

	// Stage 5: map.
	for _, p := range policies {
		for _, m := range p.ops.mapValues {
			if m.target.isDatapointValue() && dpValueStr != nil && *dpValueStr != "" {
				if mapped, found := m.values[*dpValueStr]; found {
					*dpValueStr = mapped
				} else if m.unmapped == "remove" {
					*dpValueStr = ""
				}
				continue
			}
			if !m.target.isAttr() {
				continue
			}
			val, ok := attrs[m.target.key]
			if !ok {
				continue
			}
			if mapped, found := m.values[val]; found {
				if mapped == "" {
					delete(attrs, m.target.key)
				} else {
					attrs[m.target.key] = mapped
				}
			} else if m.unmapped == "remove" {
				delete(attrs, m.target.key)
			}
		}
	}

	// Stage 6: convert.
	// Prometheus stores all label values as strings; no conversion is required for string/int targets.

	// Stage 7: discard.
	for _, p := range policies {
		for _, f := range p.ops.discard {
			if f.isAttr() {
				delete(attrs, f.key)
			}
		}
	}
}

func buildSeriesLabels(metricName string, attrs map[string]string) labels.Labels {
	builder := labels.NewScratchBuilder(len(attrs) + 1)
	if metricName != "" {
		builder.Add(model.MetricNameLabel, metricName)
	}
	for k, v := range attrs {
		if v != "" {
			builder.Add(k, v)
		}
	}
	builder.Sort()
	return builder.Labels()
}

func splitClassicHistogramSuffix(metricName string) (string, string) {
	for _, suf := range []string{"_bucket", "_sum", "_count"} {
		if strings.HasSuffix(metricName, suf) && len(metricName) > len(suf) {
			return strings.TrimSuffix(metricName, suf), suf
		}
	}
	return metricName, ""
}

type expandedValuePolicy struct {
	policy transformPolicy
	rule   sampleValueRule
}

// expandValueToAttributePolicy checks whether p renames DATAPOINT_FIELD_VALUE to an
// attribute and maps numeric values to string labels (such as k8s.pod.phase 1..5 ->
// k8s.pod.status.phase="Pending".."Unknown"). When supported, it expands into one
// synthetic policy + sampleValueRule per mapped value that satisfies canonicalMatchers.
func expandValueToAttributePolicy(p transformPolicy, canonicalMatchers []*labels.Matcher) ([]expandedValuePolicy, bool) {
	var targetAttr string
	for _, r := range p.ops.rename {
		if r.from.isDatapointValue() && r.to.isAttr() {
			targetAttr = r.to.key
			break
		}
	}
	if targetAttr == "" {
		return nil, false
	}

	var valueMap map[string]string
	for _, m := range p.ops.mapValues {
		if m.target.isAttr() && m.target.key == targetAttr {
			valueMap = m.values
			break
		}
	}
	if len(valueMap) == 0 {
		return nil, false
	}

	var (
		hasRepVal bool
		repVal    float64
	)
	for _, a := range p.ops.add {
		if a.target.isDatapointValue() && a.value != nil {
			if v, err := strconv.ParseFloat(*a.value, 64); err == nil {
				hasRepVal = true
				repVal = v
			}
		}
	}

	keys := slices.Sorted(maps.Keys(valueMap))
	var out []expandedValuePolicy
	for _, k := range keys {
		srcVal, err := strconv.ParseFloat(k, 64)
		if err != nil {
			continue
		}
		mappedVal := valueMap[k]
		if !attrValueMatchesCanonical(targetAttr, mappedVal, canonicalMatchers) {
			continue
		}

		synth := p
		synth.ops.rename = nil
		for _, r := range p.ops.rename {
			if !r.from.isDatapointValue() {
				synth.ops.rename = append(synth.ops.rename, r)
			}
		}
		synth.ops.mapValues = nil
		for _, m := range p.ops.mapValues {
			if !(m.target.isAttr() && m.target.key == targetAttr) {
				synth.ops.mapValues = append(synth.ops.mapValues, m)
			}
		}
		synth.ops.add = slices.Clone(p.ops.add)
		valCopy := mappedVal
		synth.ops.add = append(synth.ops.add, policyAdd{
			target: policyField{kind: policyFieldAttr, scope: policyScopeDatapoint, key: targetAttr},
			value:  &valCopy,
			upsert: true,
		})

		out = append(out, expandedValuePolicy{
			policy: synth,
			rule: sampleValueRule{
				hasMatchValue:   true,
				matchValue:      srcVal,
				hasReplaceValue: hasRepVal,
				replaceValue:    repVal,
				scale:           p.ops.scale,
			},
		})
	}
	return out, true
}

func attrValueMatchesCanonical(attrName, attrVal string, matchers []*labels.Matcher) bool {
	for _, m := range matchers {
		if m.Name == attrName && !m.Matches(attrVal) {
			return false
		}
	}
	return true
}

// isSupportedAttrToValuePolicy reports whether a policy using DATAPOINT_FIELD_VALUE
// renames a source attribute to DATAPOINT_FIELD_VALUE and maps its values to numbers
// (such as k8s.pod.status.phase="Running" -> k8s.pod.phase=2), or is a DATAPOINT_FIELD_VALUE
// drop filter.
func isSupportedAttrToValuePolicy(p *transformPolicy) bool {
	if p.kind == policyKindMetricFilter && p.filterAction == "ACTION_DROP" {
		_, ok := extractDatapointEqualsMatcher(p)
		return ok
	}
	hasAttrToValRename := false
	for _, r := range p.ops.rename {
		if r.from.isAttr() && r.to.isDatapointValue() {
			hasAttrToValRename = true
		} else if r.from.isDatapointValue() || r.to.isDatapointValue() {
			return false
		}
	}
	if !hasAttrToValRename {
		return false
	}
	for _, m := range p.ops.mapValues {
		if m.target.isDatapointValue() && len(m.values) > 0 {
			return true
		}
	}
	return false
}

// generateTransformPolicyVariantsMulti resolves storage selector variants across one or
// more transform/2.0 policy files that share the requested target schema_url.
func generateTransformPolicyVariantsMulti(schemas []*otelSchema, matchers []*labels.Matcher, budget *schemaExpansionBudget) ([]matcherVariant, []string, error) {
	if len(schemas) == 1 {
		return generateTransformPolicyVariants(schemas[0], matchers, budget)
	}
	var (
		allVariants   []matcherVariant
		allWarnings   []string
		seenWarnings  = map[string]struct{}{}
		addedTargetPT bool
	)
	targetMetric, err := extractMetricName(matchers)
	if err != nil || targetMetric == "" {
		return nil, nil, errMetricNameAnchor
	}
	for _, s := range schemas {
		vs, ws, err := generateTransformPolicyVariants(s, matchers, budget)
		if err != nil {
			return nil, nil, err
		}
		for _, w := range ws {
			if _, ok := seenWarnings[w]; !ok {
				seenWarnings[w] = struct{}{}
				allWarnings = append(allWarnings, w)
			}
		}
		for _, v := range vs {
			mName, _ := extractMetricName(v.matchers)
			if mName == targetMetric && identityMatcherVariant(v) {
				if addedTargetPT {
					continue
				}
				addedTargetPT = true
			}
			allVariants = append(allVariants, v)
		}
	}
	return allVariants, allWarnings, nil
}

// generateTransformPolicyVariants resolves storage selector variants and series transformers
// for a query against a transform/2.0 policy file.
func generateTransformPolicyVariants(schema *otelSchema, matchers []*labels.Matcher, budget *schemaExpansionBudget) ([]matcherVariant, []string, error) {
	tf := schema.transform
	if tf == nil {
		return nil, nil, fmt.Errorf("missing transform/2.0 definition")
	}

	targetMetric, err := extractMetricName(matchers)
	if err != nil || targetMetric == "" {
		return nil, nil, errMetricNameAnchor
	}

	baseTarget := targetMetric
	var histSuffix string
	if !tfHasPoliciesForMetric(tf, targetMetric) {
		if base, suf := splitClassicHistogramSuffix(targetMetric); suf != "" && tfHasPoliciesForMetric(tf, base) {
			baseTarget = base
			histSuffix = suf
		}
	}

	var (
		entityPolicies []transformPolicy
		warnings       []string
		seenWarnings   = map[string]struct{}{}
	)
	addWarning := func(msg string) {
		if _, exists := seenWarnings[msg]; !exists {
			seenWarnings[msg] = struct{}{}
			warnings = append(warnings, msg)
		}
	}

	for _, p := range tf.policies {
		if err := budget.reserveWork(1); err != nil {
			return nil, nil, err
		}
		if p.kind == policyKindEntityTransform {
			if !p.usesDatapointValue() {
				entityPolicies = append(entityPolicies, p)
			}
		}
	}

	// Collect candidate base metric names to query in storage: always start with baseTarget
	// followed by any source metric names that policies rename into baseTarget.
	candidateMetrics := []string{baseTarget}
	seenMetrics := map[string]struct{}{baseTarget: {}}

	for _, p := range tf.policies {
		if err := budget.reserveWork(1); err != nil {
			return nil, nil, err
		}
		if p.kind != policyKindMetricTransform {
			continue
		}
		renamedTo, isRename := p.targetMetricName()
		if !isRename || renamedTo != baseTarget {
			continue
		}
		if p.usesDatapointValue() {
			if _, ok := expandValueToAttributePolicy(p, matchers); !ok && !isSupportedAttrToValuePolicy(&p) {
				addWarning(fmt.Sprintf(
					"policy %q uses datapoint_field DATAPOINT_FIELD_VALUE, which cannot be evaluated on Prometheus series labels; skipping policy",
					p.id,
				))
				continue
			}
		}
		for _, srcMetric := range p.sourceMetricNames() {
			if _, exists := seenMetrics[srcMetric]; !exists {
				if len(candidateMetrics) >= maxSchemaExpansion {
					return nil, nil, schemaExpansionError("transform policy metric candidates")
				}
				seenMetrics[srcMetric] = struct{}{}
				candidateMetrics = append(candidateMetrics, srcMetric)
			}
		}
	}

	var variants []matcherVariant
	for _, candMetric := range candidateMetrics {
		if err := budget.reserveWork(1); err != nil {
			return nil, nil, err
		}

		var (
			candMetricPolicies  []transformPolicy
			renamingPolicies    []transformPolicy
			valToAttrExpansions []expandedValuePolicy
			hasValToAttrPolicy  bool
			dropVals            []float64
		)
		for _, p := range tf.policies {
			if p.kind != policyKindMetricTransform && p.kind != policyKindMetricFilter {
				continue
			}
			if !p.matchesMetricName(candMetric) {
				continue
			}
			if p.usesDatapointValue() {
				if expanded, ok := expandValueToAttributePolicy(p, matchers); ok {
					renamedTo, isRen := p.targetMetricName()
					if (!isRen && candMetric == baseTarget) || (isRen && renamedTo == baseTarget) {
						hasValToAttrPolicy = true
						valToAttrExpansions = append(valToAttrExpansions, expanded...)
					}
					continue
				}
				if isSupportedAttrToValuePolicy(&p) {
					if p.kind == policyKindMetricFilter {
						if dv, ok := extractDatapointEqualsMatcher(&p); ok {
							dropVals = append(dropVals, dv)
						}
					}
					candMetricPolicies = append(candMetricPolicies, p)
					if renamedTo, ok := p.targetMetricName(); ok && renamedTo == baseTarget {
						renamingPolicies = append(renamingPolicies, p)
					}
					continue
				}
				if candMetric == baseTarget {
					addWarning(fmt.Sprintf(
						"policy %q uses datapoint_field DATAPOINT_FIELD_VALUE, which cannot be evaluated on Prometheus series labels; skipping policy",
						p.id,
					))
				}
				continue
			}
			candMetricPolicies = append(candMetricPolicies, p)
			if renamedTo, ok := p.targetMetricName(); ok && renamedTo == baseTarget {
				renamingPolicies = append(renamingPolicies, p)
			}
		}

		// If this candidate metric has value-to-attribute expansions (e.g. k8s.pod.phase 1..5 ->
		// k8s.pod.status.phase="Pending".."Unknown"), emit one variant per matching state value.
		if hasValToAttrPolicy {
			if candMetric == baseTarget {
				// Emit the target passthrough variant for series that already carry the target attribute.
				variants = append(variants, matcherVariant{
					matchers:          matchers,
					mapping:           buildLabelMapping(targetMetric, nil),
					canonicalMatchers: matchers,
				})
			}
			for _, exp := range valToAttrExpansions {
				expRule := exp.rule
				expRule.dropValues = append(expRule.dropValues, dropVals...)
				expPolicies := append(slices.Clone(candMetricPolicies), exp.policy)
				expRenaming := []transformPolicy{exp.policy}
				v, ok := buildCandidatePolicyVariant(candMetric, baseTarget, targetMetric, histSuffix, entityPolicies, expPolicies, expRenaming, &expRule, matchers)
				if ok {
					variants = append(variants, v)
				}
			}
			continue
		}

		if candMetric != baseTarget && len(renamingPolicies) == 0 {
			continue
		}

		// If no policies apply to targetMetric and there are no source metric renames,
		// return a single passthrough variant so the caller skips transformation overhead.
		if candMetric == baseTarget && len(candMetricPolicies) == 0 && len(entityPolicies) == 0 && len(candidateMetrics) == 1 {
			return []matcherVariant{{matchers: matchers}}, warnings, nil
		}

		v, ok := buildCandidatePolicyVariant(candMetric, baseTarget, targetMetric, histSuffix, entityPolicies, candMetricPolicies, renamingPolicies, nil, matchers)
		if ok {
			variants = append(variants, v)
		}
	}

	return variants, warnings, nil
}

func tfHasPoliciesForMetric(tf *transformFile, metricName string) bool {
	for _, p := range tf.policies {
		if p.kind != policyKindMetricTransform && p.kind != policyKindMetricFilter {
			continue
		}
		if p.matchesMetricName(metricName) {
			return true
		}
		if renamedTo, ok := p.targetMetricName(); ok && renamedTo == metricName {
			return true
		}
	}
	return false
}

func buildCandidatePolicyVariant(
	candMetric, baseTarget, targetMetric, histSuffix string,
	entityPolicies, candMetricPolicies, renamingPolicies []transformPolicy,
	presetRule *sampleValueRule,
	matchers []*labels.Matcher,
) (matcherVariant, bool) {
	allCandPolicies := make([]transformPolicy, 0, len(entityPolicies)+len(candMetricPolicies))
	allCandPolicies = append(allCandPolicies, entityPolicies...)
	allCandPolicies = append(allCandPolicies, candMetricPolicies...)

	translatedLabels := collectPolicyAttributeAliases(allCandPolicies)
	touchedTargetAttrs, touchedPrefixes := collectTouchedTargetAttributes(allCandPolicies)

	storageMatchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, candMetric+histSuffix),
	}

	// Push down source attribute constraints when a source metric is only renamed
	// to baseTarget under specific attribute predicates (such as K8s ResourceQuota splits).
	if (candMetric != baseTarget || presetRule != nil) && len(renamingPolicies) > 0 {
		storageMatchers = append(storageMatchers, deriveSourcePushdownMatchers(renamingPolicies)...)
	}

	// Push down user query matchers on untouched attributes, and invert equality matchers
	// on unambiguously transformed attributes when candMetric is a single-policy source rename.
	for _, m := range matchers {
		if m.Name == model.MetricNameLabel {
			continue
		}
		if !isAttrTouched(m.Name, touchedTargetAttrs, touchedPrefixes) {
			storageMatchers = append(storageMatchers, m)
			continue
		}
		if candMetric != baseTarget && len(renamingPolicies) == 1 {
			inverted, satisfiable := invertPolicyMatcherForVariant(m, renamingPolicies[0], allCandPolicies)
			if !satisfiable {
				return matcherVariant{}, false
			}
			storageMatchers = append(storageMatchers, inverted...)
		}
	}

	mapping := &labelMapping{
		translatedMetric: targetMetric,
		translatedLabels: translatedLabels,
		policyTransform: &policySeriesTransformer{
			entityPolicies: entityPolicies,
			metricPolicies: candMetricPolicies,
			histSuffix:     histSuffix,
			presetRule:     presetRule,
		},
	}
	return matcherVariant{
		matchers:          storageMatchers,
		mapping:           mapping,
		canonicalMatchers: matchers,
	}, true
}

// invertPolicyMatcherForVariant inverts a canonical attribute matcher m into source
// storageMatchers when a single renaming policy unambiguously produces m.Name.
// It returns satisfiable=false when m contradicts a constant attribute added by the policy.
func invertPolicyMatcherForVariant(m *labels.Matcher, renPolicy transformPolicy, allPolicies []transformPolicy) ([]*labels.Matcher, bool) {
	// If any policy other than renPolicy writes to m.Name, skip pushdown and rely on canonicalMatchers.
	for _, p := range allPolicies {
		if p.id == renPolicy.id {
			continue
		}
		if policyWritesAttr(&p, m.Name) {
			return nil, true
		}
	}

	// 1. Check if m.Name is set by a constant add (upsert: true, no from).
	for _, a := range renPolicy.ops.add {
		if a.target.isAttr() && a.target.key == m.Name && a.value != nil && len(a.from) == 0 && a.upsert {
			return nil, m.Matches(*a.value)
		}
	}

	if m.Type != labels.MatchEqual || m.Value == "" {
		return nil, true
	}

	// 2. Check if m.Name is produced by a separator-join add (e.g. rpc.service + "/" + rpc.method -> rpc.method).
	for _, a := range renPolicy.ops.add {
		if a.target.isAttr() && a.target.key == m.Name && len(a.from) == 2 && a.separator != "" && a.from[0].isAttr() && a.from[1].isAttr() {
			if strings.Count(m.Value, a.separator) == 1 {
				parts := strings.SplitN(m.Value, a.separator, 2)
				k0, k1 := a.from[0].key, a.from[1].key
				if k0 != m.Name && k1 != m.Name {
					return []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, k0, parts[0]),
						labels.MustNewMatcher(labels.MatchEqual, k1, parts[1]),
					}, true
				}
				if k1 == m.Name {
					re := "^(" + regexp.QuoteMeta(parts[1]) + "|" + regexp.QuoteMeta(m.Value) + ")$"
					return []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchRegexp, k1, re),
					}, true
				}
			}
			return nil, true
		}
	}

	// 3. Check if m.Name is produced by a split operation (e.g. resource="requests.hugepages-2Mi" -> k8s.hugepage.size="2Mi").
	for _, s := range renPolicy.ops.split {
		if !s.from.isAttr() || s.separator == "" || len(s.to) != 2 {
			continue
		}
		if s.to[0].isAttr() && s.to[0].key == m.Name {
			re := "^" + regexp.QuoteMeta(m.Value) + "(" + regexp.QuoteMeta(s.separator) + ".*)?$"
			return []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, s.from.key, re),
			}, true
		}
		if s.to[1].isAttr() && s.to[1].key == m.Name {
			re := "^.*" + regexp.QuoteMeta(s.separator) + regexp.QuoteMeta(m.Value) + "$"
			return []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, s.from.key, re),
			}, true
		}
	}

	// 4. Check if m.Name is produced by a 1:1 rename (+ optional value map).
	var srcAttrs []string
	for _, r := range renPolicy.ops.rename {
		if r.from.isAttr() && r.to.isAttr() && r.to.key == m.Name {
			srcAttrs = append(srcAttrs, r.from.key)
		}
	}
	if len(srcAttrs) != 1 {
		return nil, true
	}
	srcAttr := srcAttrs[0]

	allowedVals := []string{m.Value}
	for _, mv := range renPolicy.ops.mapValues {
		if mv.target.isAttr() && mv.target.key == m.Name {
			var preMap []string
			for _, k := range slices.Sorted(maps.Keys(mv.values)) {
				if mv.values[k] == m.Value {
					preMap = append(preMap, k)
				}
			}
			if mv.unmapped != "remove" {
				if _, overwritten := mv.values[m.Value]; !overwritten && !slices.Contains(preMap, m.Value) {
					preMap = append(preMap, m.Value)
				}
			}
			if len(preMap) == 0 {
				return nil, false
			}
			allowedVals = preMap
		}
	}

	if len(allowedVals) == 1 {
		return []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, srcAttr, allowedVals[0]),
		}, true
	}
	quoted := make([]string, len(allowedVals))
	for i, v := range allowedVals {
		quoted[i] = regexp.QuoteMeta(v)
	}
	return []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchRegexp, srcAttr, "^("+strings.Join(quoted, "|")+")$"),
	}, true
}

func policyWritesAttr(p *transformPolicy, attr string) bool {
	for _, r := range p.ops.rename {
		if r.to.isAttr() && r.to.key == attr {
			return true
		}
	}
	for _, s := range p.ops.split {
		for _, t := range s.to {
			if t.isAttr() && t.key == attr {
				return true
			}
		}
	}
	for _, a := range p.ops.add {
		if a.target.isAttr() && a.target.key == attr {
			return true
		}
	}
	for _, m := range p.ops.mapValues {
		if m.target.isAttr() && m.target.key == attr {
			return true
		}
	}
	return false
}

func collectPolicyAttributeAliases(policies []transformPolicy) map[string]string {
	var out map[string]string
	for _, p := range policies {
		for _, r := range p.ops.rename {
			if r.from.isAttr() && r.to.isAttr() && r.from.key != r.to.key {
				if out == nil {
					out = map[string]string{}
				}
				out[r.from.key] = r.to.key
			}
		}
	}
	return out
}

func collectTouchedTargetAttributes(policies []transformPolicy) (map[string]struct{}, []string) {
	touched := map[string]struct{}{}
	var prefixes []string
	for _, p := range policies {
		for _, f := range p.ops.remove {
			if f.isAttr() {
				touched[f.key] = struct{}{}
			}
		}
		for _, r := range p.ops.rename {
			if r.from.isAttr() {
				touched[r.from.key] = struct{}{}
			}
			if r.to.isAttr() {
				touched[r.to.key] = struct{}{}
			}
		}
		for _, rp := range p.ops.renamePrefix {
			if rp.from.isAttr() {
				prefixes = append(prefixes, rp.from.key)
			}
			if rp.to != "" {
				prefixes = append(prefixes, rp.to)
			}
		}
		for _, s := range p.ops.split {
			if s.from.isAttr() {
				touched[s.from.key] = struct{}{}
			}
			for _, t := range s.to {
				if t.isAttr() {
					touched[t.key] = struct{}{}
				}
			}
		}
		for _, a := range p.ops.add {
			if a.target.isAttr() {
				touched[a.target.key] = struct{}{}
			}
		}
		for _, m := range p.ops.mapValues {
			if m.target.isAttr() {
				touched[m.target.key] = struct{}{}
			}
		}
		for _, d := range p.ops.discard {
			if d.isAttr() {
				touched[d.key] = struct{}{}
			}
		}
	}
	return touched, prefixes
}

func isAttrTouched(attr string, touched map[string]struct{}, prefixes []string) bool {
	if _, ok := touched[attr]; ok {
		return true
	}
	attrWithDot := attr + "."
	for _, p := range prefixes {
		if strings.HasPrefix(attrWithDot, p) || strings.HasPrefix(attr, p) {
			return true
		}
	}
	return false
}

// deriveSourcePushdownMatchers extracts positive equality attribute matchers from a single
// renaming policy so storage only scans matching attribute values. Regex predicates are
// evaluated by policyMatchesSeries because OTel policy regexes are not implicitly full-string
// anchored like Prometheus MatchRegexp matchers.
func deriveSourcePushdownMatchers(renamingPolicies []transformPolicy) []*labels.Matcher {
	if len(renamingPolicies) == 1 {
		var out []*labels.Matcher
		for _, m := range renamingPolicies[0].matches {
			if !m.target.isAttr() || m.negate {
				continue
			}
			if m.predicate.kind == predicateEquals {
				out = append(out, labels.MustNewMatcher(labels.MatchEqual, m.target.key, m.predicate.equals))
			}
		}
		return out
	}
	return nil
}
