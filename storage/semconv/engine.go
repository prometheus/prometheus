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
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
)

const (
	// maxSchemaExpansion caps each bounded resolver collection.
	maxSchemaExpansion = 256

	// maxSchemaExpansionWork bounds cumulative resolver work across collections.
	maxSchemaExpansionWork = maxSchemaExpansion * maxSchemaExpansion

	// maxSchemaExpansionKeyBytes bounds cumulative synthesized deduplication keys.
	maxSchemaExpansionKeyBytes = maxRegistryDecompressedBytes
)

var (
	errSchemaExpansion       = errors.New("semconv schema expansion limit exceeded")
	errMetricNameAnchor      = errors.New("schema-aware query requires a non-empty equality matcher on __name__")
	errAmbiguousSchemaRename = errors.New("semconv schema rename is ambiguous")
	errUnsafeSchemaMatcher   = errors.New("semconv schema matcher cannot be expanded safely")
)

func schemaExpansionError(kind string) error {
	return fmt.Errorf("%w: %s would exceed %d", errSchemaExpansion, kind, maxSchemaExpansion)
}

func schemaExpansionLimitError(kind string, limit uint64) error {
	return fmt.Errorf("%w: %s would exceed %d", errSchemaExpansion, kind, limit)
}

type schemaExpansionLimits struct {
	work     uint64
	keyBytes uint64
}

func productionSchemaExpansionLimits() schemaExpansionLimits {
	return schemaExpansionLimits{
		work:     maxSchemaExpansionWork,
		keyBytes: maxSchemaExpansionKeyBytes,
	}
}

// schemaExpansionBudget is shared by one resolver invocation. It charges
// attempted work, including candidates later discarded as duplicates.
type schemaExpansionBudget struct {
	limits   schemaExpansionLimits
	work     uint64
	keyBytes uint64
}

func newSchemaExpansionBudget(limits schemaExpansionLimits) *schemaExpansionBudget {
	return &schemaExpansionBudget{limits: limits}
}

func (b *schemaExpansionBudget) reserveWork(n uint64) error {
	if b == nil || n == 0 {
		return nil
	}
	if b.work > b.limits.work || n > b.limits.work-b.work {
		return schemaExpansionLimitError("resolver work", b.limits.work)
	}
	b.work += n
	return nil
}

func (b *schemaExpansionBudget) reserveKeyBytes(n uint64) error {
	if b == nil || n == 0 {
		return nil
	}
	if b.keyBytes > b.limits.keyBytes || n > b.limits.keyBytes-b.keyBytes {
		return schemaExpansionLimitError("deduplication key bytes", b.limits.keyBytes)
	}
	b.keyBytes += n
	return nil
}

func (b *schemaExpansionBudget) remainingKeyBytes() uint64 {
	if b == nil {
		return ^uint64(0)
	}
	if b.keyBytes > b.limits.keyBytes {
		return 0
	}
	return b.limits.keyBytes - b.keyBytes
}

// registrySource provides the raw bytes of registry files addressed by their
// registry/<name> path. The embedded registry (embed.FS) satisfies it directly;
// an operator-provided registry is adapted to it via newRegistrySource.
type registrySource interface {
	ReadFile(name string) ([]byte, error)
}

type schemaEngine struct {
	registry registrySource
	limits   schemaExpansionLimits

	otelSchemaCache *staticCache[otelSchema]
	semconvCache    *staticCache[semconv]
}

func newSchemaEngine(registry registrySource) *schemaEngine {
	return &schemaEngine{
		registry:        registry,
		limits:          productionSchemaExpansionLimits(),
		otelSchemaCache: newStaticCache[otelSchema](),
		semconvCache:    newStaticCache[semconv](),
	}
}

func extractMetricName(matchers []*labels.Matcher) (string, error) {
	hasMetricMatcher := false
	for _, m := range matchers {
		if m.Name != model.MetricNameLabel {
			continue
		}
		hasMetricMatcher = true
		if m.Type == labels.MatchEqual && m.Value != "" {
			return m.Value, nil
		}
	}
	if hasMetricMatcher {
		return "", errMetricNameAnchor
	}
	return "", nil
}

// normalizeMetricMatchers evaluates every metric-name constraint against the
// exact name that anchors schema traversal. Compatible constraints are
// redundant once that equality is translated to each naming era.
func normalizeMetricMatchers(matchers []*labels.Matcher) (string, []*labels.Matcher, bool, error) {
	metricName, err := extractMetricName(matchers)
	if err != nil || metricName == "" {
		return metricName, matchers, true, err
	}

	metricMatchers := 0
	for _, matcher := range matchers {
		if matcher.Name != model.MetricNameLabel {
			continue
		}
		metricMatchers++
		if !matcher.Matches(metricName) {
			return metricName, matchers, false, nil
		}
	}
	if metricMatchers == 1 {
		return metricName, matchers, true, nil
	}

	out := make([]*labels.Matcher, 0, len(matchers)-metricMatchers+1)
	insertedMetric := false
	for _, matcher := range matchers {
		if matcher.Name != model.MetricNameLabel {
			out = append(out, matcher)
			continue
		}
		if !insertedMetric {
			out = append(out, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, metricName))
			insertedMetric = true
		}
	}
	return metricName, out, true, nil
}

// findVersionAnchorIndex returns the index of the largest version <= targetVersion.
// The versions slice must be sorted in ascending semver order.
func findVersionAnchorIndex(versions []versionRenames, targetVersion string) int {
	anchorIdx, _ := findVersionAnchorIndexWithBudget(versions, targetVersion, nil)
	return anchorIdx
}

func findVersionAnchorIndexWithBudget(versions []versionRenames, targetVersion string, budget *schemaExpansionBudget) (int, error) {
	target := strings.TrimPrefix(targetVersion, "v")
	anchorIdx := 0
	for i, v := range versions {
		if err := budget.reserveWork(1); err != nil {
			return 0, err
		}
		if compareSemver(v.version, target) > 0 {
			break
		}
		anchorIdx = i
	}
	return anchorIdx, nil
}

// generateMatcherVariants generates matcher sets for schema version renames,
// anchored at the specified version.
// For each version, applies both metric and attribute renames together.
// Walks backward through versions <= version to find older name variants,
// and forward through versions > version to find newer name variants.
func generateMatcherVariants(version string, schema *otelSchema, matchers []*labels.Matcher) ([][]*labels.Matcher, error) {
	return generateMatcherVariantsWithBudget(version, schema, matchers, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
}

func generateMatcherVariantsWithBudget(version string, schema *otelSchema, matchers []*labels.Matcher, budget *schemaExpansionBudget) ([][]*labels.Matcher, error) {
	if len(schema.versionRenames) == 0 {
		return [][]*labels.Matcher{matchers}, nil
	}

	key, err := matcherKeyWithBudget(matchers, budget)
	if err != nil {
		return nil, err
	}
	variants := [][]*labels.Matcher{matchers}
	seen := map[string]struct{}{key: {}}
	anchorIdx, err := findVersionAnchorIndexWithBudget(schema.versionRenames, version, budget)
	if err != nil {
		return nil, err
	}

	// Backward for older names.
	variants, err = walkVersionsWithBudget(schema.versionRenames[:anchorIdx+1], matchers, seen, variants, true, budget)
	if err != nil {
		return nil, err
	}

	// Forward for newer names.
	variants, err = walkVersionsWithBudget(schema.versionRenames[anchorIdx+1:], matchers, seen, variants, false, budget)
	if err != nil {
		return nil, err
	}

	return variants, nil
}

// walkVersions walks through versions applying renames, chaining results until no new variants.
// If reverse is false, walks oldest→newest; if true, walks newest→oldest.
func walkVersionsWithBudget(
	versions []versionRenames,
	matchers []*labels.Matcher,
	seen map[string]struct{},
	result [][]*labels.Matcher,
	reverse bool,
	budget *schemaExpansionBudget,
) ([][]*labels.Matcher, error) {
	current := matchers
	for {
		found := false
		var versionsIter iter.Seq2[int, versionRenames]
		if reverse {
			versionsIter = slices.Backward(versions)
		} else {
			versionsIter = slices.All(versions)
		}

		for _, v := range versionsIter {
			if err := budget.reserveWork(1); err != nil {
				return nil, err
			}
			transformed := applyVersionRenames(current, v)
			if transformed == nil {
				continue
			}

			key, err := matcherKeyWithBudget(transformed, budget)
			if err != nil {
				return nil, err
			}
			if _, exists := seen[key]; exists {
				continue
			}
			if len(result) >= maxSchemaExpansion {
				return nil, schemaExpansionError("matcher variants")
			}

			seen[key] = struct{}{}
			result = append(result, transformed)
			current = transformed
			found = true
			break
		}
		if !found {
			break
		}
	}
	return result, nil
}

// buildAttributeRenameMap returns a map from each historical or forward
// attribute alias to its name at anchorVersion, for the attributes in
// canonicalAttrs (the metric's attributes declared by the anchor semconv
// version). It is anchored and walked exactly like generateMatcherVariants
// (backward over versions <= anchor, forward over versions > anchor), so every
// alias a returned series can carry maps back to the queried version's name.
// Identity entries (alias == canonical) are omitted; it returns nil when the
// schema renames none of the attributes.
func buildAttributeRenameMap(anchorVersion string, schema *otelSchema, canonicalAttrs []string) (map[string]string, error) {
	return buildAttributeRenameMapWithBudget(anchorVersion, schema, canonicalAttrs, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
}

func buildAttributeRenameMapWithBudget(anchorVersion string, schema *otelSchema, canonicalAttrs []string, budget *schemaExpansionBudget) (map[string]string, error) {
	if len(schema.versionRenames) == 0 || len(canonicalAttrs) == 0 {
		return nil, nil
	}
	if len(canonicalAttrs) > maxSchemaExpansion {
		return nil, schemaExpansionError("canonical attributes")
	}
	anchorIdx, err := findVersionAnchorIndexWithBudget(schema.versionRenames, anchorVersion, budget)
	if err != nil {
		return nil, err
	}
	backward := schema.versionRenames[:anchorIdx+1]
	forward := schema.versionRenames[anchorIdx+1:]

	out := map[string]string{}
	for _, canon := range canonicalAttrs {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		if err := walkAttributeRenamesWithBudget(backward, canon, true, out, budget); err != nil {
			return nil, err
		}
		if err := walkAttributeRenamesWithBudget(forward, canon, false, out, budget); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// walkAttributeRenames threads canon through the versions' attribute renames,
// recording each distinct produced alias → canon in out. With reverse=true it
// walks newest→oldest, otherwise oldest→newest, chaining via a per-canon seen
// set — mirroring walkVersions so the attribute walk stays consistent with the
// matcher fan-out.
func walkAttributeRenamesWithBudget(versions []versionRenames, canon string, reverse bool, out map[string]string, budget *schemaExpansionBudget) error {
	current := canon
	seen := map[string]struct{}{canon: {}}
	for {
		found := false
		var versionsIter iter.Seq2[int, versionRenames]
		if reverse {
			versionsIter = slices.Backward(versions)
		} else {
			versionsIter = slices.All(versions)
		}

		for _, v := range versionsIter {
			if err := budget.reserveWork(1); err != nil {
				return err
			}
			next, ok := v.attributes[current]
			if !ok {
				continue
			}
			if _, exists := seen[next]; exists {
				continue
			}
			if len(seen) >= maxSchemaExpansion {
				return schemaExpansionError("attribute rename states")
			}
			if _, exists := out[next]; !exists && len(out) >= maxSchemaExpansion {
				return schemaExpansionError("attribute rename mappings")
			}
			seen[next] = struct{}{}
			out[next] = canon
			current = next
			found = true
			break
		}
		if !found {
			break
		}
	}
	return nil
}

type orderedTraversalDirection uint8

const (
	orderedBackward orderedTraversalDirection = iota
	orderedForward
)

func (d orderedTraversalDirection) String() string {
	if d == orderedForward {
		return "forward"
	}
	return "backward"
}

type orderedRenameState struct {
	matchers         []*labels.Matcher
	metric           string
	translatedLabels map[string]string
	// Predecessor choices span revisions so unsupported convergence fails closed.
	pendingMetricPredecessors    map[string]string
	pendingAttributePredecessors map[string]map[int]string
}

type orderedVariantAccumulator struct {
	anchorMetric       string
	variants           []matcherVariant
	byMatchers         map[string]int
	lineageMetricNames map[string]struct{}
	attributeSlots     int
	budget             *schemaExpansionBudget
}

func newOrderedVariantAccumulator(anchorMetric string, budget *schemaExpansionBudget) *orderedVariantAccumulator {
	return &orderedVariantAccumulator{
		anchorMetric:       anchorMetric,
		byMatchers:         map[string]int{},
		lineageMetricNames: map[string]struct{}{anchorMetric: {}},
		budget:             budget,
	}
}

func (a *orderedVariantAccumulator) observeMetric(metric string) error {
	if _, exists := a.lineageMetricNames[metric]; exists {
		return nil
	}
	if len(a.lineageMetricNames) >= maxSchemaExpansion {
		return schemaExpansionError("metric lineage names")
	}
	if err := a.budget.reserveWork(1); err != nil {
		return err
	}
	a.lineageMetricNames[metric] = struct{}{}
	return nil
}

func (a *orderedVariantAccumulator) add(state orderedRenameState) error {
	key, err := matcherKeyWithBudget(state.matchers, a.budget)
	if err != nil {
		return err
	}
	idx, exists := a.byMatchers[key]
	if !exists {
		if len(a.variants) >= maxSchemaExpansion {
			return schemaExpansionError("matcher variants")
		}
		translated, err := cloneTranslatedLabelsWithBudget(state.translatedLabels, a.budget)
		if err != nil {
			return err
		}
		if len(translated) > maxSchemaExpansion-a.attributeSlots {
			return schemaExpansionError("attribute mappings")
		}
		a.attributeSlots += len(translated)
		a.byMatchers[key] = len(a.variants)
		a.variants = append(a.variants, matcherVariant{
			matchers: state.matchers,
			mapping:  buildLabelMapping(a.anchorMetric, translated),
		})
		return nil
	}

	mapping := a.variants[idx].mapping
	keys, err := sortedRenameKeysWithBudget(state.translatedLabels, a.budget, "attribute mappings")
	if err != nil {
		return err
	}
	for _, alias := range keys {
		canonical := state.translatedLabels[alias]
		if existing, ok := mapping.translatedLabels[alias]; ok {
			if existing != canonical {
				return ambiguousAttributeRenameError(alias, existing, canonical)
			}
			continue
		}
		if a.attributeSlots >= maxSchemaExpansion {
			return schemaExpansionError("attribute mappings")
		}
		if mapping.translatedLabels == nil {
			mapping.translatedLabels = map[string]string{}
		}
		mapping.translatedLabels[alias] = canonical
		a.attributeSlots++
	}
	return nil
}

func ambiguousAttributeRenameError(alias, first, second string) error {
	return fmt.Errorf("%w: attribute %q resolves to both %q and %q", errAmbiguousSchemaRename, alias, first, second)
}

func sortedRenameKeysWithBudget[V any](m map[string]V, budget *schemaExpansionBudget, kind string) ([]string, error) {
	if len(m) == 0 {
		return nil, nil
	}
	if len(m) > maxSchemaExpansion {
		return nil, schemaExpansionError(kind)
	}
	if err := budget.reserveWork(uint64(len(m))); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys, nil
}

func cloneTranslatedLabelsWithBudget(in map[string]string, budget *schemaExpansionBudget) (map[string]string, error) {
	keys, err := sortedRenameKeysWithBudget(in, budget, "attribute mappings")
	if err != nil || len(keys) == 0 {
		return nil, err
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = in[key]
	}
	return out, nil
}

func addTranslatedLabel(out map[string]string, alias, canonical string) error {
	if existing, ok := out[alias]; ok {
		if existing != canonical {
			return ambiguousAttributeRenameError(alias, existing, canonical)
		}
		return nil
	}
	if alias == canonical {
		return nil
	}
	if len(out) >= maxSchemaExpansion {
		return schemaExpansionError("attribute mappings")
	}
	out[alias] = canonical
	return nil
}

func canonicalLabelName(mapping map[string]string, name string) string {
	if canonical, ok := mapping[name]; ok {
		return canonical
	}
	return name
}

func orderedRevisionPartitionWithBudget(revisions []versionRenames, version string, budget *schemaExpansionBudget) (int, error) {
	target := strings.TrimPrefix(version, "v")
	for i, revision := range revisions {
		if err := budget.reserveWork(1); err != nil {
			return 0, err
		}
		if compareSemver(revision.version, target) > 0 {
			return i, nil
		}
	}
	return len(revisions), nil
}

// Ordered matcher generation resolves the scoped subset of schema
// transformations supported by this fan-out layer. Transformations that would
// require branching fail closed; the later lineage resolver adds that support.
func generateOrderedMatcherVariantsWithBudget(version string, schema *otelSchema, matchers []*labels.Matcher, metricName string, budget *schemaExpansionBudget) ([]matcherVariant, error) {
	anchor := orderedRenameState{matchers: matchers, metric: metricName}
	acc := newOrderedVariantAccumulator(metricName, budget)
	if err := acc.add(anchor); err != nil {
		return nil, err
	}
	if len(schema.versionRenames) == 0 {
		return acc.variants, nil
	}

	partition, err := orderedRevisionPartitionWithBudget(schema.versionRenames, version, budget)
	if err != nil {
		return nil, err
	}
	if err := walkOrderedRenamesWithBudget(schema.versionRenames[:partition], anchor, orderedBackward, acc, budget); err != nil {
		return nil, err
	}
	if err := walkOrderedRenamesWithBudget(schema.versionRenames[partition:], anchor, orderedForward, acc, budget); err != nil {
		return nil, err
	}
	if err := validateOrderedMetricConvergenceWithBudget(schema.versionRenames, acc.lineageMetricNames, budget); err != nil {
		return nil, err
	}
	return expandOrderedMatcherVariantsWithBudget(acc.variants, matchers, metricName, budget)
}

func walkOrderedRenamesWithBudget(revisions []versionRenames, anchor orderedRenameState, direction orderedTraversalDirection, acc *orderedVariantAccumulator, budget *schemaExpansionBudget) error {
	state := anchor
	for step := range revisions {
		if err := budget.reserveWork(1); err != nil {
			return err
		}
		revisionIndex := step
		if direction == orderedBackward {
			revisionIndex = len(revisions) - 1 - step
		}
		revision := revisions[revisionIndex]
		for changeStep := range revision.changes {
			if err := budget.reserveWork(1); err != nil {
				return err
			}
			changeIndex := changeStep
			if direction == orderedBackward {
				changeIndex = len(revision.changes) - 1 - changeStep
			}
			change := revision.changes[changeIndex]
			var err error
			switch {
			case change.metrics != nil:
				state, err = applyOrderedMetricRenamesWithBudget(state, change.metrics, direction, revision.version, budget)
				if err == nil {
					err = acc.observeMetric(state.metric)
				}
			case change.attributes != nil:
				state, err = applyOrderedAttributeRenamesWithBudget(state, change.attributes, direction, revision.version, budget)
			}
			if err != nil {
				return err
			}
		}
		if err := acc.add(state); err != nil {
			return err
		}
	}
	return nil
}

func (r *directedRenames) targets(name string, direction orderedTraversalDirection) (targets []string, renamed, wrongSide bool) {
	if direction == orderedForward {
		if target, ok := r.forward[name]; ok {
			return []string{target}, true, false
		}
		_, wrongSide = r.reverse[name]
		return nil, false, wrongSide
	}
	if targets, ok := r.reverse[name]; ok {
		return targets, true, false
	}
	_, wrongSide = r.forward[name]
	return nil, false, wrongSide
}

func applyOrderedMetricRenamesWithBudget(state orderedRenameState, renames *directedRenames, direction orderedTraversalDirection, revision string, budget *schemaExpansionBudget) (orderedRenameState, error) {
	if err := budget.reserveWork(1); err != nil {
		return orderedRenameState{}, err
	}
	targets, renamed, wrongSide := renames.targets(state.metric, direction)
	if !renamed {
		if wrongSide {
			if repeatedOrderedMetricEdge(state, renames, direction) {
				return state, nil
			}
			return orderedRenameState{}, fmt.Errorf("%w: metric %q is on the wrong side of a %s traversal at schema version %s", errAmbiguousSchemaRename, state.metric, direction, revision)
		}
		return state, nil
	}
	if len(targets) != 1 {
		return orderedRenameState{}, fmt.Errorf("%w: metric %q has %d predecessors at schema version %s", errAmbiguousSchemaRename, state.metric, len(targets), revision)
	}
	target := targets[0]
	predecessor, destination := state.metric, target
	if direction == orderedBackward {
		predecessor, destination = target, state.metric
	}
	if err := rememberOrderedMetricPredecessorWithBudget(&state, destination, predecessor, budget); err != nil {
		return orderedRenameState{}, err
	}
	matchers := slices.Clone(state.matchers)
	for i, matcher := range matchers {
		if err := budget.reserveWork(1); err != nil {
			return orderedRenameState{}, err
		}
		if matcher.Name == model.MetricNameLabel && matcher.Type == labels.MatchEqual && matcher.Value == state.metric {
			matchers[i] = labels.MustNewMatcher(matcher.Type, matcher.Name, target)
		}
	}
	state.matchers = matchers
	state.metric = target
	return state, nil
}

func repeatedOrderedMetricEdge(state orderedRenameState, renames *directedRenames, direction orderedTraversalDirection) bool {
	if direction == orderedBackward {
		target, ok := renames.forward[state.metric]
		return ok && state.pendingMetricPredecessors[target] == state.metric
	}
	predecessor := state.pendingMetricPredecessors[state.metric]
	return predecessor != "" && slices.Contains(renames.reverse[state.metric], predecessor)
}

func rememberOrderedMetricPredecessorWithBudget(state *orderedRenameState, target, predecessor string, budget *schemaExpansionBudget) error {
	if err := budget.reserveWork(1); err != nil {
		return err
	}
	if state.pendingMetricPredecessors == nil {
		state.pendingMetricPredecessors = map[string]string{}
	}
	if existing, ok := state.pendingMetricPredecessors[target]; ok && existing != predecessor {
		return fmt.Errorf("%w: metric %q has conflicting ordered predecessors", errAmbiguousSchemaRename, target)
	}
	state.pendingMetricPredecessors[target] = predecessor
	return nil
}

func validateOrderedMetricConvergenceWithBudget(revisions []versionRenames, lineageMetricNames map[string]struct{}, budget *schemaExpansionBudget) error {
	predecessors := map[string]map[string]struct{}{}
	predecessorEntries := 0
	for _, revision := range revisions {
		for _, change := range revision.changes {
			if change.metrics == nil {
				continue
			}
			sources, err := sortedRenameKeysWithBudget(change.metrics.forward, budget, "metric rename mappings")
			if err != nil {
				return err
			}
			for _, source := range sources {
				target := change.metrics.forward[source]
				if _, selected := lineageMetricNames[target]; !selected {
					continue
				}
				byTarget := predecessors[target]
				if byTarget == nil {
					byTarget = map[string]struct{}{}
					predecessors[target] = byTarget
				}
				if _, exists := byTarget[source]; exists {
					continue
				}
				if predecessorEntries >= maxSchemaExpansion {
					return schemaExpansionError("metric predecessor mappings")
				}
				byTarget[source] = struct{}{}
				predecessorEntries++
				if len(byTarget) > 1 {
					return fmt.Errorf("%w: metric %q has multiple ordered predecessors at schema version %s", errAmbiguousSchemaRename, target, revision.version)
				}
			}
		}
	}
	return nil
}

type orderedMatcherGroup struct {
	canonicalName string
	indexes       []int
	aliases       []string
}

func expandOrderedMatcherVariantsWithBudget(observed []matcherVariant, anchorMatchers []*labels.Matcher, anchorMetric string, budget *schemaExpansionBudget) ([]matcherVariant, error) {
	metricNames := make([]string, 0, len(observed))
	seenMetrics := map[string]struct{}{}
	translatedLabels := map[string]string{}
	for _, variant := range observed {
		metric, err := extractMetricName(variant.matchers)
		if err != nil {
			return nil, err
		}
		if _, exists := seenMetrics[metric]; !exists {
			if len(metricNames) >= maxSchemaExpansion {
				return nil, schemaExpansionError("physical metric names")
			}
			seenMetrics[metric] = struct{}{}
			metricNames = append(metricNames, metric)
		}
		if variant.mapping == nil {
			continue
		}
		aliases, err := sortedRenameKeysWithBudget(variant.mapping.translatedLabels, budget, "attribute mappings")
		if err != nil {
			return nil, err
		}
		for _, alias := range aliases {
			if err := addTranslatedLabel(translatedLabels, alias, variant.mapping.translatedLabels[alias]); err != nil {
				return nil, err
			}
		}
	}
	if len(translatedLabels) == 0 {
		return observed, nil
	}
	if len(anchorMatchers) > maxSchemaExpansion {
		return nil, schemaExpansionError("canonical matchers")
	}

	canonicalMatchers := slices.Clone(anchorMatchers)
	groupsByName := map[string]int{}
	groups := make([]orderedMatcherGroup, 0, len(anchorMatchers))
	for i, matcher := range anchorMatchers {
		if matcher.Name == model.MetricNameLabel {
			continue
		}
		canonical := canonicalLabelName(translatedLabels, matcher.Name)
		if canonical != matcher.Name {
			canonicalMatchers[i] = labels.MustNewMatcher(matcher.Type, canonical, matcher.Value)
		}
		groupIndex, exists := groupsByName[canonical]
		if !exists {
			if len(groups) >= maxSchemaExpansion {
				return nil, schemaExpansionError("attribute matcher groups")
			}
			groupIndex = len(groups)
			groupsByName[canonical] = groupIndex
			groups = append(groups, orderedMatcherGroup{canonicalName: canonical})
		}
		if len(groups[groupIndex].indexes) >= maxSchemaExpansion {
			return nil, schemaExpansionError("attribute matcher group")
		}
		groups[groupIndex].indexes = append(groups[groupIndex].indexes, i)
	}

	for groupIndex := range groups {
		group := &groups[groupIndex]
		seenAliases := map[string]struct{}{group.canonicalName: {}}
		group.aliases = append(group.aliases, group.canonicalName)
		for _, variant := range observed {
			for _, index := range group.indexes {
				alias := variant.matchers[index].Name
				if _, exists := seenAliases[alias]; exists {
					continue
				}
				if len(group.aliases) >= maxSchemaExpansion {
					return nil, schemaExpansionError("attribute matcher aliases")
				}
				seenAliases[alias] = struct{}{}
				group.aliases = append(group.aliases, alias)
			}
		}
		if len(group.aliases) == 1 {
			continue
		}
		slices.Sort(group.aliases[1:])
		matchesEmpty := true
		for _, index := range group.indexes {
			if !canonicalMatchers[index].Matches("") {
				matchesEmpty = false
				break
			}
		}
		if matchesEmpty {
			return nil, fmt.Errorf("%w: renamed attribute %q matcher conjunction matches an absent label", errUnsafeSchemaMatcher, group.canonicalName)
		}
	}

	matcherSets := [][]*labels.Matcher{canonicalMatchers}
	for _, group := range groups {
		if len(group.aliases) == 1 {
			continue
		}
		if len(matcherSets) > maxSchemaExpansion/len(group.aliases) {
			return nil, schemaExpansionError("matcher variants")
		}
		next := make([][]*labels.Matcher, 0, len(matcherSets)*len(group.aliases))
		for _, matcherSet := range matcherSets {
			for _, alias := range group.aliases {
				if err := budget.reserveWork(uint64(len(group.indexes))); err != nil {
					return nil, err
				}
				candidate := slices.Clone(matcherSet)
				for _, index := range group.indexes {
					matcher := candidate[index]
					candidate[index] = labels.MustNewMatcher(matcher.Type, alias, matcher.Value)
				}
				next = append(next, candidate)
			}
		}
		matcherSets = next
	}

	if len(metricNames) > maxSchemaExpansion/len(matcherSets) {
		return nil, schemaExpansionError("matcher variants")
	}
	mapping := buildLabelMapping(anchorMetric, translatedLabels)
	variants := make([]matcherVariant, 0, len(metricNames)*len(matcherSets))
	seenMatchers := map[string]struct{}{}
	for _, metric := range metricNames {
		for _, matcherSet := range matcherSets {
			if err := budget.reserveWork(uint64(len(matcherSet))); err != nil {
				return nil, err
			}
			candidate := slices.Clone(matcherSet)
			for i, matcher := range candidate {
				if matcher.Name == model.MetricNameLabel {
					candidate[i] = labels.MustNewMatcher(matcher.Type, matcher.Name, metric)
				}
			}
			key, err := matcherKeyWithBudget(candidate, budget)
			if err != nil {
				return nil, err
			}
			if _, exists := seenMatchers[key]; exists {
				continue
			}
			if len(variants) >= maxSchemaExpansion {
				return nil, schemaExpansionError("matcher variants")
			}
			seenMatchers[key] = struct{}{}
			variants = append(variants, matcherVariant{
				matchers:          candidate,
				mapping:           mapping,
				canonicalMatchers: canonicalMatchers,
			})
		}
	}
	return variants, nil
}

func applyOrderedAttributeRenamesWithBudget(state orderedRenameState, step *attributeRenameStep, direction orderedTraversalDirection, revision string, budget *schemaExpansionBudget) (orderedRenameState, error) {
	if !step.appliesTo(state.metric) {
		return state, nil
	}
	before := state.translatedLabels
	out, err := cloneTranslatedLabelsWithBudget(before, budget)
	if err != nil {
		return orderedRenameState{}, err
	}
	if out == nil {
		out = map[string]string{}
	}

	if direction == orderedForward {
		sources, err := sortedRenameKeysWithBudget(step.renames.forward, budget, "attribute rename mappings")
		if err != nil {
			return orderedRenameState{}, err
		}
		for _, source := range sources {
			if err := addTranslatedLabel(out, step.renames.forward[source], canonicalLabelName(before, source)); err != nil {
				return orderedRenameState{}, err
			}
		}
	} else {
		targets, err := sortedRenameKeysWithBudget(step.renames.reverse, budget, "attribute rename mappings")
		if err != nil {
			return orderedRenameState{}, err
		}
		for _, target := range targets {
			if err := rejectOrderedAttributeConvergenceWithBudget(state, target, step.renames.reverse[target], revision, budget); err != nil {
				return orderedRenameState{}, err
			}
			canonical := canonicalLabelName(before, target)
			for _, source := range step.renames.reverse[target] {
				if err := budget.reserveWork(1); err != nil {
					return orderedRenameState{}, err
				}
				if err := addTranslatedLabel(out, source, canonical); err != nil {
					return orderedRenameState{}, err
				}
			}
		}
	}

	matchers := state.matchers
	matchersCloned := false
	for i, matcher := range state.matchers {
		if err := budget.reserveWork(1); err != nil {
			return orderedRenameState{}, err
		}
		if matcher.Name == model.MetricNameLabel {
			continue
		}
		targets, renamed, _ := step.renames.targets(matcher.Name, direction)
		if !renamed {
			continue
		}
		if len(targets) != 1 {
			return orderedRenameState{}, fmt.Errorf("%w: attribute matcher %q has %d predecessors", errAmbiguousSchemaRename, matcher.Name, len(targets))
		}
		if direction == orderedBackward {
			if err := rememberOrderedAttributePredecessorWithBudget(&state, matcher.Name, targets[0], i, budget); err != nil {
				return orderedRenameState{}, err
			}
		}
		if !matchersCloned {
			matchers = slices.Clone(state.matchers)
			matchersCloned = true
		}
		matchers[i] = labels.MustNewMatcher(matcher.Type, targets[0], matcher.Value)
	}
	state.matchers = matchers
	state.translatedLabels = out
	return state, nil
}

func rejectOrderedAttributeConvergenceWithBudget(state orderedRenameState, target string, sources []string, revision string, budget *schemaExpansionBudget) error {
	predecessors := state.pendingAttributePredecessors[target]
	if len(predecessors) == 0 {
		return nil
	}
	if err := budget.reserveWork(uint64(len(predecessors))); err != nil {
		return err
	}
	if len(sources) == 1 {
		samePredecessor := true
		for _, predecessor := range predecessors {
			if predecessor != sources[0] {
				samePredecessor = false
				break
			}
		}
		if samePredecessor {
			return nil
		}
	}
	return fmt.Errorf("%w: attribute matcher %q has multiple ordered predecessors at schema version %s", errAmbiguousSchemaRename, target, revision)
}

func rememberOrderedAttributePredecessorWithBudget(state *orderedRenameState, target, predecessor string, index int, budget *schemaExpansionBudget) error {
	if err := budget.reserveWork(1); err != nil {
		return err
	}
	if state.pendingAttributePredecessors == nil {
		state.pendingAttributePredecessors = map[string]map[int]string{}
	}
	predecessors := state.pendingAttributePredecessors[target]
	if predecessors == nil {
		predecessors = map[int]string{}
		state.pendingAttributePredecessors[target] = predecessors
	}
	if existing, ok := predecessors[index]; ok && existing != predecessor {
		return fmt.Errorf("%w: attribute matcher %q has conflicting ordered predecessors", errAmbiguousSchemaRename, target)
	}
	predecessors[index] = predecessor
	return nil
}

// matcherKey generates a string key for a matcher set to detect duplicates.
func matcherKey(matchers []*labels.Matcher) string {
	key, _ := matcherKeyWithBudget(matchers, nil)
	return key
}

func matcherKeyWithBudget(matchers []*labels.Matcher, budget *schemaExpansionBudget) (string, error) {
	if err := budget.reserveWork(1); err != nil {
		return "", err
	}
	remaining := budget.remainingKeyBytes()
	var size uint64
	addSize := func(n uint64) error {
		if n > remaining-size {
			limit := remaining
			if budget != nil {
				limit = budget.limits.keyBytes
			}
			return schemaExpansionLimitError("deduplication key bytes", limit)
		}
		size += n
		return nil
	}
	for i, matcher := range matchers {
		if i > 0 {
			if err := addSize(1); err != nil {
				return "", err
			}
		}
		if err := addSize(uint64(len(matcher.Name))); err != nil {
			return "", err
		}
		if err := addSize(1); err != nil {
			return "", err
		}
		if err := addSize(uint64(len(matcher.Value))); err != nil {
			return "", err
		}
	}
	if err := budget.reserveKeyBytes(size); err != nil {
		return "", err
	}

	var b strings.Builder
	b.Grow(int(size))
	for i, m := range matchers {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(m.Name)
		b.WriteByte('=')
		b.WriteString(m.Value)
	}
	return b.String(), nil
}

// applyVersionRenames applies a version's metric and attribute renames to matchers.
// Returns nil if no renames apply. Uses lazy allocation to avoid allocating when no changes are made.
func applyVersionRenames(matchers []*labels.Matcher, renames versionRenames) []*labels.Matcher {
	var result []*labels.Matcher
	for i, m := range matchers {
		var newMatcher *labels.Matcher
		if m.Name == model.MetricNameLabel {
			if variant, ok := renames.metrics[m.Value]; ok {
				newMatcher = labels.MustNewMatcher(m.Type, m.Name, variant)
			}
		} else if variant, ok := renames.attributes[m.Name]; ok {
			newMatcher = labels.MustNewMatcher(m.Type, variant, m.Value)
		}
		if newMatcher != nil {
			if result == nil {
				// Lazy allocate and copy preceding unchanged matchers.
				result = make([]*labels.Matcher, len(matchers))
				copy(result[:i], matchers[:i])
			}
			result[i] = newMatcher
		} else if result != nil {
			result[i] = m
		}
	}

	return result
}

type matcherVariant struct {
	matchers []*labels.Matcher
	mapping  *labelMapping
	// canonicalMatchers revalidates transformed series after alias fan-out.
	canonicalMatchers []*labels.Matcher
}

type queryContext struct {
	warnings []string
}

// getSemconv returns the semconv parsed from url, fetching it via the
// embedded registry on a cache miss.
func (e *schemaEngine) getSemconv(url string) (semconv, error) {
	if sc, ok := e.semconvCache.get(url); ok {
		return sc, nil
	}
	sc, err := e.fetchSemconv(url)
	if err != nil {
		return semconv{}, err
	}
	e.semconvCache.set(url, sc)
	return sc, nil
}

// getOTelSchema returns the OTel schema parsed from url, fetching it via the
// embedded registry on a cache miss.
func (e *schemaEngine) getOTelSchema(url string) (otelSchema, error) {
	if s, ok := e.otelSchemaCache.get(url); ok {
		return s, nil
	}
	s, err := e.fetchOTelSchema(url)
	if err != nil {
		return otelSchema{}, err
	}
	e.otelSchemaCache.set(url, s)
	return s, nil
}

// findMatcherVariants returns all variants to match for a single schematized
// metric selection. semconvURL points to a semantic conventions file and is
// always required. In production schemaURL (an OTel schema file with versioned
// renames) is also always set, because classifyMatchers only triggers fan-out
// when both are present; the empty-schemaURL path exists only for the direct
// unit test. It returns one variant per schema-version rename of the metric,
// plus a label mapping for transforming results back to the requested version.
// The returned matchers do not include the reserved schema matchers. It returns
// an error if semconvURL or a non-empty equality __name__ matcher is not provided.
func (e *schemaEngine) findMatcherVariants(semconvURL, schemaURL string, originalMatchers []*labels.Matcher) ([]matcherVariant, queryContext, error) {
	if semconvURL == "" {
		return nil, queryContext{}, errors.New("semconvURL is required")
	}

	// Filter out the wrapper's reserved matchers.
	matchers := stripReservedLabels(originalMatchers)

	metricName, normalizedMatchers, satisfiable, err := normalizeMetricMatchers(matchers)
	if err != nil {
		return nil, queryContext{}, err
	}
	if metricName == "" {
		return nil, queryContext{}, errMetricNameAnchor
	}
	if !satisfiable {
		return []matcherVariant{{matchers: matchers}}, queryContext{}, nil
	}
	matchers = normalizedMatchers

	// Fetch semantic conventions for the anchor version (also validates the URL).
	sc, err := e.getSemconv(semconvURL)
	if err != nil {
		return nil, queryContext{}, err
	}

	// Generate schema-version rename variants. In production schemaURL is always
	// set (classifyMatchers gates fan-out on it); the empty case is reached only
	// by direct unit tests and falls through to the unmodified matchers.
	variants := []matcherVariant{{matchers: matchers, mapping: buildLabelMapping(metricName, nil)}}
	if schemaURL != "" {
		schema, err := e.getOTelSchema(schemaURL)
		if err != nil {
			return nil, queryContext{}, err
		}
		budget := newSchemaExpansionBudget(e.limits)
		variants, err = generateOrderedMatcherVariantsWithBudget(sc.version, &schema, matchers, metricName, budget)
		if err != nil {
			return nil, queryContext{}, err
		}
	}
	return variants, queryContext{}, nil
}

// labelMapping rewrites a returned series' names to the queried semantic-
// conventions version: translatedMetric is the queried (anchor) metric name
// that every variant collapses to, and translatedLabels maps each historical
// attribute alias back to its anchor-version name.
type labelMapping struct {
	translatedLabels map[string]string // historical attribute alias → anchor name, e.g. "user" -> "tenant"
	translatedMetric string
}

// buildLabelMapping creates the mapping used to rewrite result labels back to
// the requested semantic-conventions version: the result metric name maps to
// the queried (anchor) name, and translatedLabels maps each historical
// attribute alias back to its anchor-version name (nil/empty when no attribute
// was renamed).
func buildLabelMapping(metricName string, translatedLabels map[string]string) *labelMapping {
	return &labelMapping{translatedMetric: metricName, translatedLabels: translatedLabels}
}

// aliasesOf returns name together with every historical alias that maps to it,
// i.e. the set of label names a returned series may carry for the canonical
// name. It is the inverse of translatedLabels and is used to fan LabelValues
// out across a renamed attribute's historical names. The metric name has no
// attribute aliases, so it is returned unchanged.
func (m *labelMapping) aliasesOf(name string) []string {
	aliases := []string{name}
	for alias, canonical := range m.translatedLabels {
		if canonical == name {
			aliases = append(aliases, alias)
		}
	}
	slices.Sort(aliases[1:])
	return aliases
}

// transformOTelSchemaLabels transforms series labels to the current semantic
// conventions version using the label mapping.
func transformOTelSchemaLabels(originalLabels labels.Labels, mapping *labelMapping) (labels.Labels, error) {
	if !labelMappingChangesLabels(originalLabels, mapping) {
		return originalLabels, nil
	}
	return transformChangedOTelSchemaLabels(originalLabels, mapping)
}

func transformChangedOTelSchemaLabels(originalLabels labels.Labels, mapping *labelMapping) (labels.Labels, error) {
	type mappedLabel struct {
		source string
		value  string
	}

	builder := labels.NewScratchBuilder(originalLabels.Len())
	mapped := make(map[string]mappedLabel, originalLabels.Len())
	var transformErr error
	originalLabels.Range(func(l labels.Label) {
		if transformErr != nil {
			return
		}

		name := l.Name
		value := l.Value
		switch l.Name {
		case semconvURLLabel, schemaURLLabel:
			return
		case model.MetricNameLabel:
			value = mapping.translatedMetric
		default:
			if canonical, ok := mapping.translatedLabels[l.Name]; ok {
				name = canonical
			}
		}

		if existing, ok := mapped[name]; ok {
			if existing.value != value {
				transformErr = fmt.Errorf("semconv label transformation maps %q and %q to %q with conflicting values", existing.source, l.Name, name)
			}
			return
		}
		mapped[name] = mappedLabel{source: l.Name, value: value}
		builder.Add(name, value)
	})
	if transformErr != nil {
		return labels.EmptyLabels(), transformErr
	}
	builder.Sort()
	return builder.Labels(), nil
}

func labelMappingChangesLabels(lbls labels.Labels, mapping *labelMapping) bool {
	changed := false
	lbls.Range(func(label labels.Label) {
		if changed {
			return
		}
		if isReservedLabel(label.Name) {
			changed = true
			return
		}
		if mapping == nil {
			return
		}
		if label.Name == model.MetricNameLabel {
			changed = label.Value != mapping.translatedMetric
			return
		}
		_, changed = mapping.translatedLabels[label.Name]
	})
	return changed
}
