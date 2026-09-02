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
	"slices"
	"strconv"
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

// metricOwnershipTracker follows metric identities through schema-version
// boundaries. Identity unions model explicit convergence, while reused records
// a disconnected identity claiming a previously occupied surface name.
type metricOwnershipTracker struct {
	budget *schemaExpansionBudget

	parent     []int
	size       []uint32
	occupied   map[string]int
	firstOwner map[string]int
	reused     map[string]struct{}
}

func newMetricOwnershipTracker(budget *schemaExpansionBudget) *metricOwnershipTracker {
	return &metricOwnershipTracker{
		budget:     budget,
		occupied:   map[string]int{},
		firstOwner: map[string]int{},
		reused:     map[string]struct{}{},
	}
}

func (t *metricOwnershipTracker) newIdentity() (int, error) {
	if err := t.budget.reserveWork(1); err != nil {
		return 0, err
	}
	id := len(t.parent)
	t.parent = append(t.parent, id)
	t.size = append(t.size, 1)
	return id, nil
}

func (t *metricOwnershipTracker) root(id int) (int, error) {
	for t.parent[id] != id {
		if err := t.budget.reserveWork(1); err != nil {
			return 0, err
		}
		id = t.parent[id]
	}
	return id, nil
}

func (t *metricOwnershipTracker) merge(a, b int) (int, error) {
	a, err := t.root(a)
	if err != nil {
		return 0, err
	}
	b, err = t.root(b)
	if err != nil {
		return 0, err
	}
	if a == b {
		return a, nil
	}
	if t.size[a] < t.size[b] || (t.size[a] == t.size[b] && b < a) {
		a, b = b, a
	}
	t.parent[b] = a
	t.size[a] += t.size[b]
	return a, nil
}

func (t *metricOwnershipTracker) recordBoundary(name string, identity int) error {
	identity, err := t.root(identity)
	if err != nil {
		return err
	}
	first, exists := t.firstOwner[name]
	if !exists {
		t.firstOwner[name] = identity
		return nil
	}
	first, err = t.root(first)
	if err != nil {
		return err
	}
	if first != identity {
		t.reused[name] = struct{}{}
	}
	return nil
}

func sortedMapKeysWithBudget[V any](m map[string]V, budget *schemaExpansionBudget) ([]string, error) {
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

// reusedMetricNamesWithBudget reports surface names that distinct metric
// identities occupy at schema-version boundaries. Changes within one revision
// remain ordered, but each individual rename map is applied atomically.
func reusedMetricNamesWithBudget(revisions []schemaRevision, budget *schemaExpansionBudget) (map[string]struct{}, error) {
	tracker := newMetricOwnershipTracker(budget)
	for _, revision := range revisions {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		targets, err := metricTargetsByFirstChangeWithBudget(revision, budget)
		if err != nil {
			return nil, err
		}

		seeded := map[string]int{}
		for changeIndex, change := range revision.changes {
			if change.metricRenames == nil {
				continue
			}
			sources, err := sortedMapKeysWithBudget(change.metricRenames.forward, budget)
			if err != nil {
				return nil, err
			}
			for _, source := range sources {
				if _, exists := tracker.occupied[source]; exists {
					continue
				}
				if producedByEarlierChange(targets, changeIndex, source) {
					continue
				}
				// Schemas may repeat a retired spelling as a compatibility input in a
				// later convergence. Reuse its known identity rather than inventing one.
				identity, known := tracker.firstOwner[source]
				if known {
					identity, err = tracker.root(identity)
					if err != nil {
						return nil, err
					}
				} else {
					identity, err = tracker.newIdentity()
					if err != nil {
						return nil, err
					}
				}
				tracker.occupied[source] = identity
				seeded[source] = identity
			}
		}
		seededNames, err := sortedMapKeysWithBudget(seeded, budget)
		if err != nil {
			return nil, err
		}
		for _, name := range seededNames {
			if err := tracker.recordBoundary(name, seeded[name]); err != nil {
				return nil, err
			}
		}

		touched := map[string]struct{}{}
		for _, change := range revision.changes {
			if change.metricRenames == nil {
				continue
			}
			if err := tracker.applyRenameMap(change.metricRenames, touched); err != nil {
				return nil, err
			}
		}
		touchedNames, err := sortedMapKeysWithBudget(touched, budget)
		if err != nil {
			return nil, err
		}
		for _, name := range touchedNames {
			identity, exists := tracker.occupied[name]
			if !exists {
				continue
			}
			if err := tracker.recordBoundary(name, identity); err != nil {
				return nil, err
			}
		}
	}
	return tracker.reused, nil
}

func metricTargetsByFirstChangeWithBudget(revision schemaRevision, budget *schemaExpansionBudget) (map[string]int, error) {
	targets := map[string]int{}
	for i, change := range revision.changes {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		if change.metricRenames == nil {
			continue
		}
		names, err := sortedMapKeysWithBudget(change.metricRenames.reverse, budget)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			if _, exists := targets[name]; !exists {
				targets[name] = i
			}
		}
	}
	return targets, nil
}

func schemaHasAttributeRenamesWithBudget(revisions []schemaRevision, budget *schemaExpansionBudget) (bool, error) {
	for _, revision := range revisions {
		for _, change := range revision.changes {
			if err := budget.reserveWork(1); err != nil {
				return false, err
			}
			if change.attributeRenames != nil {
				return true, nil
			}
		}
	}
	return false, nil
}

func (t *metricOwnershipTracker) applyRenameMap(renames *directedRenames, touched map[string]struct{}) error {
	sources, err := sortedMapKeysWithBudget(renames.forward, t.budget)
	if err != nil {
		return err
	}
	incoming := map[string][]int{}
	stationaryTargets := map[string]int{}
	for _, source := range sources {
		target := renames.forward[source]
		touched[source] = struct{}{}
		touched[target] = struct{}{}
		identity, exists := t.occupied[source]
		if exists {
			incoming[target] = append(incoming[target], identity)
		}
		if _, targetMoves := renames.forward[target]; targetMoves {
			continue
		}
		if identity, occupied := t.occupied[target]; occupied {
			stationaryTargets[target] = identity
		}
	}
	for _, source := range sources {
		delete(t.occupied, source)
	}
	targetNames, err := sortedMapKeysWithBudget(incoming, t.budget)
	if err != nil {
		return err
	}
	for _, target := range targetNames {
		identities := incoming[target]
		identity := identities[0]
		for _, other := range identities[1:] {
			identity, err = t.merge(identity, other)
			if err != nil {
				return err
			}
		}
		if stationary, exists := stationaryTargets[target]; exists {
			identity, err = t.merge(identity, stationary)
			if err != nil {
				return err
			}
		}
		t.occupied[target] = identity
	}
	return nil
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

	otelSchemaCache   *staticCache[otelSchema]
	semconvCache      *staticCache[semconv]
	schemaSafetyCache *staticCache[schemaSafetyAnalysis]
}

func newSchemaEngine(registry registrySource) *schemaEngine {
	return &schemaEngine{
		registry:          registry,
		limits:            productionSchemaExpansionLimits(),
		otelSchemaCache:   newStaticCache[otelSchema](),
		semconvCache:      newStaticCache[semconv](),
		schemaSafetyCache: newStaticCache[schemaSafetyAnalysis](),
	}
}

type schemaSafetyAnalysis struct {
	reused              map[string]struct{}
	hasAttributeRenames bool
	work                uint64
	err                 error
}

// schemaSafetyAnalysis returns immutable lifecycle metadata for one cached
// schema. The work remains charged to every query even when the scan is cached.
func (e *schemaEngine) schemaSafetyAnalysis(url string, schema *otelSchema) schemaSafetyAnalysis {
	if analysis, ok := e.schemaSafetyCache.get(url); ok {
		return analysis
	}
	budget := newSchemaExpansionBudget(productionSchemaExpansionLimits())
	reused, err := reusedMetricNamesWithBudget(schema.revisions, budget)
	hasAttributeRenames := false
	if err == nil {
		hasAttributeRenames, err = schemaHasAttributeRenamesWithBudget(schema.revisions, budget)
	}
	analysis := schemaSafetyAnalysis{
		reused:              reused,
		hasAttributeRenames: hasAttributeRenames,
		work:                budget.work,
		err:                 err,
	}
	e.schemaSafetyCache.set(url, analysis)
	return analysis
}

// renameValidator carries traversal diagnostics. Semconv-backed consistency
// checks are layered onto it separately from the ordered lineage walk.
type renameValidator struct {
	anchorDeclared bool
	warnings       []string
	seen           map[string]struct{}
}

func newRenameValidator(anchor semconv, anchorName string) *renameValidator {
	_, declared := anchor.metrics[anchorName]
	return &renameValidator{anchorDeclared: declared, seen: map[string]struct{}{}}
}

func (rv *renameValidator) warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if _, dup := rv.seen[msg]; dup {
		return
	}
	rv.seen[msg] = struct{}{}
	rv.warnings = append(rv.warnings, msg)
}

var errMetricNameAnchor = errors.New("schema-aware query requires a non-empty equality matcher on __name__")

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

type traversalDirection uint8

const (
	traverseBackward traversalDirection = iota
	traverseForward
)

func (d traversalDirection) String() string {
	if d == traverseForward {
		return "forward"
	}
	return "backward"
}

// revisionPartitionWithBudget returns the number of revisions whose version is at or
// before targetVersion. Returning a count, rather than an anchor index, lets an
// anchor predating every revision correctly have an empty backward range.
func revisionPartitionWithBudget(revisions []schemaRevision, targetVersion string, budget *schemaExpansionBudget) (int, error) {
	target := strings.TrimPrefix(targetVersion, "v")
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

// traversalPartitionWithBudget returns the revision boundary from which both
// directions should be walked. When an anchor is the revision that retires an
// undeclared historical name, that revision belongs to the forward walk. A later
// anchor must stop at the retirement because the surface name may have been reused.
func traversalPartitionWithBudget(revisions []schemaRevision, targetVersion, metricName string, anchorDeclared bool, budget *schemaExpansionBudget) (int, error) {
	partition, err := revisionPartitionWithBudget(revisions, targetVersion, budget)
	if err != nil {
		return 0, err
	}
	if anchorDeclared {
		return partition, nil
	}

	anchorVersion := strings.TrimPrefix(targetVersion, "v")
	for i := partition - 1; i >= 0; i-- {
		if err := budget.reserveWork(1); err != nil {
			return 0, err
		}
		before, after, mentioned, err := revisions[i].metricBoundaryWithBudget(metricName, budget)
		if err != nil {
			return 0, err
		}
		if !mentioned || (!before && !after) {
			continue
		}
		if before && !after && compareSemver(revisions[i].version, anchorVersion) == 0 {
			return i, nil
		}
		break
	}
	return partition, nil
}

type attributeOrigin uint8

const (
	attributeOriginResolved attributeOrigin = iota
	attributeOriginPending
)

type attributeLineage map[string]map[string]attributeOrigin

func newAttributeLineageWithBudget(canonical []string, budget *schemaExpansionBudget) (attributeLineage, error) {
	if len(canonical) == 0 {
		return nil, nil
	}
	lineage := make(attributeLineage, min(len(canonical), maxSchemaExpansion))
	for _, name := range canonical {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		if _, exists := lineage[name]; exists {
			continue
		}
		if len(lineage) >= maxSchemaExpansion {
			return nil, schemaExpansionError("attribute lineage")
		}
		lineage[name] = map[string]attributeOrigin{name: attributeOriginResolved}
	}
	return lineage, nil
}

type lineageState struct {
	matchers []*labels.Matcher
	metric   string
	attrs    attributeLineage

	// metricProduced is set while walking a revision forward when an earlier
	// transformation produced metric. metricOriginPending marks a temporary
	// identity branch while walking a converging revision backward.
	metricProduced      bool
	metricOriginPending bool

	// pendingAttributeMatchers identifies matcher positions retained as
	// temporary identity branches while walking a converging revision backward.
	pendingAttributeMatchers map[int]struct{}
}

type matcherVariant struct {
	matchers          []*labels.Matcher
	mapping           *labelMapping
	canonicalMatchers []*labels.Matcher
}

type attributeAlias struct {
	alias     string
	canonical string
}

type variantAccumulator struct {
	anchorMetric string
	variants     []matcherVariant
	byMatchers   map[string]int

	attributeIdentities map[string]map[string]struct{}
	attributeAliases    map[string]map[string]string
	attributeParents    map[string]map[string]string
	attributeSizes      map[string]map[string]uint32
	trackAttrIdentities bool
	ambiguousMetrics    map[string]struct{}
	conflicts           []map[string]struct{}
	validator           *renameValidator

	budget         *schemaExpansionBudget
	attributeSlots int
}

func newVariantAccumulatorWithBudget(anchorMetric string, canonicalAttrs []string, rv *renameValidator, trackAttrIdentities bool, budget *schemaExpansionBudget) (*variantAccumulator, error) {
	acc := &variantAccumulator{
		anchorMetric:        anchorMetric,
		byMatchers:          map[string]int{},
		trackAttrIdentities: trackAttrIdentities,
		validator:           rv,
		budget:              budget,
	}
	if !trackAttrIdentities {
		return acc, nil
	}
	canonical := make(map[string]struct{}, min(len(canonicalAttrs), maxSchemaExpansion))
	for _, name := range canonicalAttrs {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		canonical[name] = struct{}{}
	}
	acc.attributeIdentities = map[string]map[string]struct{}{anchorMetric: canonical}
	acc.attributeAliases = map[string]map[string]string{}
	acc.attributeParents = map[string]map[string]string{anchorMetric: {}}
	acc.attributeSizes = map[string]map[string]uint32{anchorMetric: {}}
	for name := range canonical {
		acc.attributeParents[anchorMetric][name] = name
		acc.attributeSizes[anchorMetric][name] = 1
	}
	return acc, nil
}

// add folds states with identical underlying matcher queries together. Their
// compatible label aliases can be normalised by one query. A physical metric
// or attribute name owned by distinct identities makes the whole query unsafe.
func (a *variantAccumulator) add(state lineageState) error {
	if _, ambiguous := a.ambiguousMetrics[state.metric]; ambiguous {
		return fmt.Errorf("%w: metric name %q belongs to distinct schema lineages", errAmbiguousSchemaRename, state.metric)
	}
	key, err := matcherKeyWithBudget(state.matchers, a.budget)
	if err != nil {
		return err
	}
	idx, exists := a.byMatchers[key]
	if !exists {
		if len(a.variants) >= maxSchemaExpansion {
			return schemaExpansionError("matcher variants")
		}
		idx = len(a.variants)
		a.byMatchers[key] = idx
		a.variants = append(a.variants, matcherVariant{
			matchers: state.matchers,
			mapping:  buildLabelMapping(a.anchorMetric, nil),
		})
		a.conflicts = append(a.conflicts, map[string]struct{}{})
	}
	return a.mergeAttributeAliases(idx, state.metric, state.attrs)
}

func (a *variantAccumulator) mergeAttributeAliases(idx int, metric string, lineage attributeLineage) error {
	aliases, err := attributeAliasesWithBudget(lineage, a.budget)
	if err != nil {
		return err
	}

	if a.trackAttrIdentities {
		owners, err := resolvedAttributeOwnersWithBudget(lineage, a.budget)
		if err != nil {
			return err
		}

		knownIdentities := a.attributeIdentities[metric]
		knownAliases := a.attributeAliases[metric]
		stagedIdentities := make(map[string]struct{}, len(owners))
		currentOwners := make(map[string][]string, len(owners))
		for _, owner := range owners {
			if err := a.ensureAttributeIdentity(metric, owner.canonical); err != nil {
				return err
			}
			stagedIdentities[owner.canonical] = struct{}{}
			currentOwners[owner.current] = append(currentOwners[owner.current], owner.canonical)
		}

		// Co-occupancy at one schema boundary is an explicit convergence, not name
		// reuse. Merge those identities before comparing the boundary with history.
		currentNames, err := sortedMapKeysWithBudget(currentOwners, a.budget)
		if err != nil {
			return err
		}
		for _, current := range currentNames {
			identities := currentOwners[current]
			for _, identity := range identities[1:] {
				if err := a.mergeAttributeIdentities(metric, identities[0], identity); err != nil {
					return err
				}
			}
		}

		stagedNames, err := sortedMapKeysWithBudget(stagedIdentities, a.budget)
		if err != nil {
			return err
		}
		for _, identity := range stagedNames {
			if canonical, exists := knownAliases[identity]; exists {
				same, err := a.sameAttributeIdentity(metric, identity, canonical)
				if err != nil {
					return err
				}
				if same {
					continue
				}
				return ambiguousAttributeRenameError(metric, identity, identity, canonical)
			}
		}
		for _, pair := range aliases {
			if err := a.budget.reserveWork(1); err != nil {
				return err
			}
			if _, identity := knownIdentities[pair.alias]; identity {
				same, err := a.sameAttributeIdentity(metric, pair.alias, pair.canonical)
				if err != nil {
					return err
				}
				if !same {
					return ambiguousAttributeRenameError(metric, pair.alias, pair.alias, pair.canonical)
				}
			}
			if _, identity := stagedIdentities[pair.alias]; identity {
				same, err := a.sameAttributeIdentity(metric, pair.alias, pair.canonical)
				if err != nil {
					return err
				}
				if !same {
					return ambiguousAttributeRenameError(metric, pair.alias, pair.alias, pair.canonical)
				}
			}
		}

		if knownIdentities == nil {
			knownIdentities = map[string]struct{}{}
			a.attributeIdentities[metric] = knownIdentities
		}
		for identity := range stagedIdentities {
			knownIdentities[identity] = struct{}{}
		}
		if knownAliases == nil {
			knownAliases = map[string]string{}
			a.attributeAliases[metric] = knownAliases
		}
		for _, pair := range aliases {
			if _, exists := knownAliases[pair.alias]; !exists {
				knownAliases[pair.alias] = pair.canonical
			}
		}
	}

	mapping := a.variants[idx].mapping
	if len(aliases) > 0 && mapping.translatedLabels == nil {
		mapping.translatedLabels = map[string]string{}
	}
	for _, pair := range aliases {
		if err := a.budget.reserveWork(1); err != nil {
			return err
		}
		if _, conflicted := a.conflicts[idx][pair.alias]; conflicted {
			continue
		}
		if existing, exists := mapping.translatedLabels[pair.alias]; exists && existing != pair.canonical {
			if err := a.markAttributeConflict(idx, pair.alias, existing, pair.canonical); err != nil {
				return err
			}
			continue
		}
		if _, exists := mapping.translatedLabels[pair.alias]; !exists {
			if err := a.reserveAttributeSlot(); err != nil {
				return err
			}
		}
		mapping.translatedLabels[pair.alias] = pair.canonical
	}
	if len(mapping.translatedLabels) == 0 {
		mapping.translatedLabels = nil
	}
	return nil
}

func (a *variantAccumulator) mergeExistingAttributeAliases(matchers []*labels.Matcher, metric string, lineage attributeLineage) error {
	key, err := matcherKeyWithBudget(matchers, a.budget)
	if err != nil {
		return err
	}
	idx, exists := a.byMatchers[key]
	if !exists {
		return nil
	}
	return a.mergeAttributeAliases(idx, metric, lineage)
}

func ambiguousAttributeRenameError(metric, alias, first, second string) error {
	return fmt.Errorf("%w: attribute name %q resolves to both %q and %q for metric %q", errAmbiguousSchemaRename, alias, first, second, metric)
}

func (a *variantAccumulator) ensureAttributeIdentity(metric, identity string) error {
	parents := a.attributeParents[metric]
	if parents == nil {
		parents = map[string]string{}
		a.attributeParents[metric] = parents
		a.attributeSizes[metric] = map[string]uint32{}
	}
	if _, exists := parents[identity]; exists {
		return nil
	}
	if err := a.budget.reserveWork(1); err != nil {
		return err
	}
	parents[identity] = identity
	a.attributeSizes[metric][identity] = 1
	return nil
}

func (a *variantAccumulator) attributeIdentityRoot(metric, identity string) (string, error) {
	if err := a.ensureAttributeIdentity(metric, identity); err != nil {
		return "", err
	}
	parents := a.attributeParents[metric]
	for parents[identity] != identity {
		if err := a.budget.reserveWork(1); err != nil {
			return "", err
		}
		identity = parents[identity]
	}
	return identity, nil
}

func (a *variantAccumulator) mergeAttributeIdentities(metric, first, second string) error {
	first, err := a.attributeIdentityRoot(metric, first)
	if err != nil {
		return err
	}
	second, err = a.attributeIdentityRoot(metric, second)
	if err != nil {
		return err
	}
	if first == second {
		return nil
	}
	sizes := a.attributeSizes[metric]
	if sizes[first] < sizes[second] || (sizes[first] == sizes[second] && second < first) {
		first, second = second, first
	}
	a.attributeParents[metric][second] = first
	sizes[first] += sizes[second]
	return nil
}

func (a *variantAccumulator) sameAttributeIdentity(metric, first, second string) (bool, error) {
	first, err := a.attributeIdentityRoot(metric, first)
	if err != nil {
		return false, err
	}
	second, err = a.attributeIdentityRoot(metric, second)
	if err != nil {
		return false, err
	}
	return first == second, nil
}

func (a *variantAccumulator) reserveAttributeSlot() error {
	if a.attributeSlots >= maxSchemaExpansion {
		return schemaExpansionError("attribute mappings")
	}
	a.attributeSlots++
	return nil
}

func (a *variantAccumulator) markAttributeConflict(idx int, alias, first, second string) error {
	if _, mapped := a.variants[idx].mapping.translatedLabels[alias]; !mapped {
		if err := a.reserveAttributeSlot(); err != nil {
			return err
		}
	}
	delete(a.variants[idx].mapping.translatedLabels, alias)
	a.conflicts[idx][alias] = struct{}{}
	if a.validator != nil {
		a.validator.warn("attribute name %q resolves to both %q and %q for metric %q; leaving it unmodified rather than merging distinct labels",
			alias, first, second, a.anchorMetric)
	}
	return nil
}

type attributeOwner struct {
	canonical string
	current   string
}

func resolvedAttributeOwnersWithBudget(lineage attributeLineage, budget *schemaExpansionBudget) ([]attributeOwner, error) {
	canonicalNames, err := sortedMapKeysWithBudget(lineage, budget)
	if err != nil {
		return nil, err
	}
	resolved := make([]attributeOwner, 0, len(canonicalNames))
	for _, canonical := range canonicalNames {
		currentNames, err := sortedMapKeysWithBudget(lineage[canonical], budget)
		if err != nil {
			return nil, err
		}
		for _, current := range currentNames {
			if lineage[canonical][current] == attributeOriginResolved {
				resolved = append(resolved, attributeOwner{canonical: canonical, current: current})
			}
		}
	}
	return resolved, nil
}

func attributeAliasesWithBudget(lineage attributeLineage, budget *schemaExpansionBudget) ([]attributeAlias, error) {
	var aliases []attributeAlias
	for canonical, currentNames := range lineage {
		for current, origin := range currentNames {
			if err := budget.reserveWork(1); err != nil {
				return nil, err
			}
			if origin == attributeOriginResolved && current != canonical {
				aliases = append(aliases, attributeAlias{alias: current, canonical: canonical})
			}
		}
	}
	slices.SortFunc(aliases, func(a, b attributeAlias) int {
		if c := strings.Compare(a.alias, b.alias); c != 0 {
			return c
		}
		return strings.Compare(a.canonical, b.canonical)
	})
	return aliases, nil
}

// generateMatcherVariantsWithBudget resolves ordered schema transformations in both
// directions from version. Metric renames may branch; each returned matcher
// set carries only the attribute aliases valid for its accepted lineages.
func generateMatcherVariantsWithBudget(version string, schema *otelSchema, matchers []*labels.Matcher, canonicalAttrs []string, rv *renameValidator, budget *schemaExpansionBudget) ([]matcherVariant, error) {
	return generateMatcherVariantsWithAnalysis(version, schema, matchers, canonicalAttrs, rv, nil, budget)
}

func generateMatcherVariantsWithAnalysis(version string, schema *otelSchema, matchers []*labels.Matcher, canonicalAttrs []string, rv *renameValidator, analysis *schemaSafetyAnalysis, budget *schemaExpansionBudget) ([]matcherVariant, error) {
	metricName, normalizedMatchers, satisfiable, err := normalizeMetricMatchers(matchers)
	if err != nil {
		return nil, err
	}
	if metricName == "" {
		return nil, errMetricNameAnchor
	}
	if !satisfiable {
		return []matcherVariant{{matchers: matchers}}, nil
	}
	matchers = normalizedMatchers

	var ambiguousMetrics map[string]struct{}
	trackAttrIdentities := false
	if analysis == nil {
		ambiguousMetrics, err = reusedMetricNamesWithBudget(schema.revisions, budget)
		if err != nil {
			return nil, err
		}
		trackAttrIdentities, err = schemaHasAttributeRenamesWithBudget(schema.revisions, budget)
		if err != nil {
			return nil, err
		}
	} else {
		ambiguousMetrics = analysis.reused
		trackAttrIdentities = analysis.hasAttributeRenames
	}

	attrs, err := newAttributeLineageWithBudget(canonicalAttrs, budget)
	if err != nil {
		return nil, err
	}

	anchor := lineageState{
		matchers: matchers,
		metric:   metricName,
		attrs:    attrs,
	}
	acc, err := newVariantAccumulatorWithBudget(metricName, canonicalAttrs, rv, trackAttrIdentities, budget)
	if err != nil {
		return nil, err
	}
	acc.ambiguousMetrics = ambiguousMetrics
	if err := acc.add(anchor); err != nil {
		return nil, err
	}

	anchorDeclared := rv == nil || rv.anchorDeclared
	partition, err := traversalPartitionWithBudget(schema.revisions, version, metricName, anchorDeclared, budget)
	if err != nil {
		return nil, err
	}
	backwardEnd, forwardStart := partition, partition
	if err := walkSchemaRevisions(schema.revisions[:backwardEnd], anchor, traverseBackward, rv, acc, budget); err != nil {
		return nil, err
	}
	if err := walkSchemaRevisions(schema.revisions[forwardStart:], anchor, traverseForward, rv, acc, budget); err != nil {
		return nil, err
	}
	return finalizeMatcherVariants(acc.variants, matchers, budget)
}

type revisionTargets struct {
	metric    map[string]int
	attribute map[string]int
}

func targetsByFirstChange(revision schemaRevision, budget *schemaExpansionBudget) (revisionTargets, error) {
	targets := revisionTargets{
		metric:    map[string]int{},
		attribute: map[string]int{},
	}
	for i, change := range revision.changes {
		if err := budget.reserveWork(1); err != nil {
			return revisionTargets{}, err
		}
		var targetMap map[string]int
		var renames *directedRenames
		switch {
		case change.metricRenames != nil:
			targetMap = targets.metric
			renames = change.metricRenames
		case change.attributeRenames != nil:
			targetMap = targets.attribute
			renames = change.attributeRenames.renames
		}
		if renames == nil {
			continue
		}
		for target := range renames.reverse {
			if err := budget.reserveWork(1); err != nil {
				return revisionTargets{}, err
			}
			if _, exists := targetMap[target]; !exists {
				targetMap[target] = i
			}
		}
	}
	return targets, nil
}

func producedByEarlierChange(targets map[string]int, changeIndex int, name string) bool {
	first, exists := targets[name]
	return exists && first < changeIndex
}

type lineageStateSet struct {
	states []lineageState
	seen   map[string]struct{}
	budget *schemaExpansionBudget
}

func newLineageStateSet(capacity int, budget *schemaExpansionBudget) *lineageStateSet {
	if capacity > maxSchemaExpansion {
		capacity = maxSchemaExpansion
	}
	return &lineageStateSet{
		states: make([]lineageState, 0, capacity),
		seen:   make(map[string]struct{}, capacity),
		budget: budget,
	}
}

func (s *lineageStateSet) add(state lineageState) error {
	key, err := lineageStateKeyWithBudget(state, s.budget)
	if err != nil {
		return err
	}
	if _, exists := s.seen[key]; exists {
		return nil
	}
	if len(s.states) >= maxSchemaExpansion {
		return schemaExpansionError("lineage states")
	}
	s.seen[key] = struct{}{}
	s.states = append(s.states, state)
	return nil
}

func walkSchemaRevisions(revisions []schemaRevision, anchor lineageState, direction traversalDirection, rv *renameValidator, acc *variantAccumulator, budget *schemaExpansionBudget) error {
	states := []lineageState{anchor}
	for step := 0; step < len(revisions) && len(states) > 0; step++ {
		if err := budget.reserveWork(1); err != nil {
			return err
		}
		revisionIndex := step
		if direction == traverseBackward {
			revisionIndex = len(revisions) - 1 - step
		}
		revision := revisions[revisionIndex]
		targets, err := targetsByFirstChange(revision, budget)
		if err != nil {
			return err
		}
		for i := range states {
			if err := budget.reserveWork(1); err != nil {
				return err
			}
			states[i].metricProduced = false
			states[i].metricOriginPending = false
			states[i].pendingAttributeMatchers = nil
		}

		for changeStep := 0; changeStep < len(revision.changes) && len(states) > 0; changeStep++ {
			if err := budget.reserveWork(1); err != nil {
				return err
			}
			changeIndex := changeStep
			if direction == traverseBackward {
				changeIndex = len(revision.changes) - 1 - changeStep
			}
			change := revision.changes[changeIndex]
			var err error
			switch {
			case change.metricRenames != nil:
				states, err = applyMetricRenames(states, change.metricRenames, direction, revision.version, changeIndex, targets.metric, rv, budget)
			case change.attributeRenames != nil:
				states, err = applyAttributeRenames(states, change.attributeRenames, direction, changeIndex, targets.attribute, acc, budget)
			}
			if err != nil {
				return err
			}
		}

		validated := newLineageStateSet(len(states), budget)
		for _, state := range states {
			if err := budget.reserveWork(1); err != nil {
				return err
			}
			if state.metricOriginPending || len(state.pendingAttributeMatchers) > 0 {
				continue
			}
			state.attrs, err = prunePendingAttributeOrigins(state.attrs, budget)
			if err != nil {
				return err
			}
			state.metricProduced = false
			state.metricOriginPending = false
			state.pendingAttributeMatchers = nil
			if err := validated.add(state); err != nil {
				return err
			}
		}
		states = validated.states
		for _, state := range states {
			if err := acc.add(state); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *directedRenames) targets(name string, direction traversalDirection) (targets []string, renamed, wrongSide bool) {
	if direction == traverseForward {
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

func applyMetricRenames(states []lineageState, renames *directedRenames, direction traversalDirection, revision string, changeIndex int, earlierTargets map[string]int, rv *renameValidator, budget *schemaExpansionBudget) ([]lineageState, error) {
	out := newLineageStateSet(len(states), budget)
	for _, state := range states {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		targets, renamed, wrongSide := renames.targets(state.metric, direction)
		if !renamed {
			if wrongSide {
				if direction == traverseForward && state.metricProduced {
					if err := out.add(state); err != nil {
						return nil, err
					}
					continue
				}
				if rv != nil {
					rv.warn("schema version %s encounters metric %q only on the wrong side of a rename while traversing %s; stopping at this metric lifecycle boundary",
						revision, state.metric, direction)
				}
				continue
			}
			if err := out.add(state); err != nil {
				return nil, err
			}
			continue
		}

		for _, target := range targets {
			if err := budget.reserveWork(1); err != nil {
				return nil, err
			}
			next := state
			next.metric = target
			var err error
			next.matchers, err = replaceMetricMatchersWithBudget(state.matchers, state.metric, target, budget)
			if err != nil {
				return nil, err
			}
			next.metricOriginPending = false
			if direction == traverseForward {
				next.metricProduced = true
			}
			if err := out.add(next); err != nil {
				return nil, err
			}
		}

		_, isSource := renames.forward[state.metric]
		if direction == traverseBackward && !isSource && producedByEarlierChange(earlierTargets, changeIndex, state.metric) {
			next := state
			next.metricOriginPending = true
			if err := out.add(next); err != nil {
				return nil, err
			}
		}
	}
	return out.states, nil
}

func replaceMetricMatchersWithBudget(matchers []*labels.Matcher, currentMetric, targetMetric string, budget *schemaExpansionBudget) ([]*labels.Matcher, error) {
	if err := budget.reserveWork(uint64(len(matchers))); err != nil {
		return nil, err
	}
	out := slices.Clone(matchers)
	for i, matcher := range out {
		if matcher.Name == model.MetricNameLabel && matcher.Type == labels.MatchEqual && matcher.Value == currentMetric {
			out[i] = labels.MustNewMatcher(matcher.Type, matcher.Name, targetMetric)
		}
	}
	return out, nil
}

func applyAttributeRenames(states []lineageState, step *attributeRenameStep, direction traversalDirection, changeIndex int, earlierTargets map[string]int, acc *variantAccumulator, budget *schemaExpansionBudget) ([]lineageState, error) {
	out := newLineageStateSet(len(states), budget)
	for _, state := range states {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		if !step.appliesTo(state.metric) {
			if err := out.add(state); err != nil {
				return nil, err
			}
			continue
		}

		lineage, err := seedAttributeLineage(state.attrs, step.renames, direction, budget)
		if err != nil {
			return nil, err
		}
		lineage, err = transformAttributeLineage(lineage, step.renames, direction, changeIndex, earlierTargets, budget)
		if err != nil {
			return nil, err
		}
		if err := acc.mergeExistingAttributeAliases(state.matchers, state.metric, lineage); err != nil {
			return nil, err
		}
		state.attrs = lineage
		matcherStates, err := transformAttributeMatcherStatesWithBudget(state, step.renames, direction, changeIndex, earlierTargets, budget)
		if err != nil {
			return nil, err
		}
		for _, next := range matcherStates {
			if err := out.add(next); err != nil {
				return nil, err
			}
		}
	}
	return out.states, nil
}

func seedAttributeLineage(lineage attributeLineage, renames *directedRenames, direction traversalDirection, budget *schemaExpansionBudget) (attributeLineage, error) {
	out := make(attributeLineage, len(lineage))
	entries := 0
	seenCurrent := map[string]struct{}{}
	canonicalNames := make([]string, 0, len(lineage))
	for canonical := range lineage {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		canonicalNames = append(canonicalNames, canonical)
	}
	slices.Sort(canonicalNames)
	for _, canonical := range canonicalNames {
		currentNames := make([]string, 0, len(lineage[canonical]))
		for current := range lineage[canonical] {
			if err := budget.reserveWork(1); err != nil {
				return nil, err
			}
			currentNames = append(currentNames, current)
		}
		slices.Sort(currentNames)
		for _, current := range currentNames {
			if err := addAttributeOrigin(out, canonical, current, lineage[canonical][current], &entries, budget); err != nil {
				return nil, err
			}
			seenCurrent[current] = struct{}{}
		}
	}

	var currentSide map[string][]string
	if direction == traverseForward {
		currentSide = make(map[string][]string, len(renames.forward))
		for source := range renames.forward {
			currentSide[source] = nil
		}
	} else {
		currentSide = renames.reverse
	}
	names := make([]string, 0, len(currentSide))
	for name := range currentSide {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if _, exists := seenCurrent[name]; exists {
			continue
		}
		if err := addAttributeOrigin(out, name, name, attributeOriginResolved, &entries, budget); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func transformAttributeLineage(lineage attributeLineage, renames *directedRenames, direction traversalDirection, changeIndex int, earlierTargets map[string]int, budget *schemaExpansionBudget) (attributeLineage, error) {
	if len(lineage) == 0 {
		return nil, nil
	}
	out := make(attributeLineage, len(lineage))
	entries := 0
	canonicalNames := make([]string, 0, len(lineage))
	for canonical := range lineage {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		canonicalNames = append(canonicalNames, canonical)
	}
	slices.Sort(canonicalNames)
	for _, canonical := range canonicalNames {
		currentNames := make([]string, 0, len(lineage[canonical]))
		for current := range lineage[canonical] {
			if err := budget.reserveWork(1); err != nil {
				return nil, err
			}
			currentNames = append(currentNames, current)
		}
		slices.Sort(currentNames)
		for _, current := range currentNames {
			origin := lineage[canonical][current]
			targets, renamed, _ := renames.targets(current, direction)
			if !renamed {
				if err := addAttributeOrigin(out, canonical, current, origin, &entries, budget); err != nil {
					return nil, err
				}
				continue
			}
			for _, target := range targets {
				if err := addAttributeOrigin(out, canonical, target, attributeOriginResolved, &entries, budget); err != nil {
					return nil, err
				}
			}

			_, isSource := renames.forward[current]
			if direction == traverseBackward && !isSource && producedByEarlierChange(earlierTargets, changeIndex, current) {
				if err := addAttributeOrigin(out, canonical, current, attributeOriginPending, &entries, budget); err != nil {
					return nil, err
				}
			}
		}
	}
	return out, nil
}

func addAttributeOrigin(lineage attributeLineage, canonical, current string, origin attributeOrigin, entries *int, budget *schemaExpansionBudget) error {
	if err := budget.reserveWork(1); err != nil {
		return err
	}
	currentNames := lineage[canonical]
	if currentNames == nil {
		currentNames = map[string]attributeOrigin{}
		lineage[canonical] = currentNames
	}
	if existing, exists := currentNames[current]; exists {
		if origin < existing {
			currentNames[current] = origin
		}
		return nil
	}
	if *entries >= maxSchemaExpansion {
		return schemaExpansionError("attribute lineage")
	}
	currentNames[current] = origin
	(*entries)++
	return nil
}

type attributeMatcherGroup struct {
	name    string
	indexes []int
}

func attributeMatcherGroups(matchers []*labels.Matcher, budget *schemaExpansionBudget) ([]attributeMatcherGroup, error) {
	var groups []attributeMatcherGroup
	byName := map[string]int{}
	for i, matcher := range matchers {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		if matcher.Name == model.MetricNameLabel {
			continue
		}
		idx, exists := byName[matcher.Name]
		if !exists {
			idx = len(groups)
			byName[matcher.Name] = idx
			groups = append(groups, attributeMatcherGroup{name: matcher.Name})
		}
		groups[idx].indexes = append(groups[idx].indexes, i)
	}
	return groups, nil
}

func finalizeMatcherVariants(variants []matcherVariant, canonicalMatchers []*labels.Matcher, budget *schemaExpansionBudget) ([]matcherVariant, error) {
	renamedCanonical := map[string]struct{}{}
	for _, variant := range variants {
		if variant.mapping == nil {
			continue
		}
		for alias, canonical := range variant.mapping.translatedLabels {
			if err := budget.reserveWork(1); err != nil {
				return nil, err
			}
			if alias != canonical {
				renamedCanonical[canonical] = struct{}{}
			}
		}
	}
	if len(renamedCanonical) == 0 {
		return variants, nil
	}

	groups, err := attributeMatcherGroups(canonicalMatchers, budget)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if _, renamed := renamedCanonical[group.name]; !renamed {
			continue
		}
		matchesEmpty := true
		for _, index := range group.indexes {
			if err := budget.reserveWork(1); err != nil {
				return nil, err
			}
			if !canonicalMatchers[index].Matches("") {
				matchesEmpty = false
				break
			}
		}
		if matchesEmpty {
			return nil, fmt.Errorf("%w: renamed attribute %q matcher conjunction matches an absent label", errUnsafeSchemaMatcher, group.name)
		}
	}

	for i := range variants {
		if variants[i].mapping != nil && len(variants[i].mapping.translatedLabels) > 0 {
			variants[i].canonicalMatchers = canonicalMatchers
		}
	}
	return variants, nil
}

func transformAttributeMatcherStatesWithBudget(state lineageState, renames *directedRenames, direction traversalDirection, changeIndex int, earlierTargets map[string]int, budget *schemaExpansionBudget) ([]lineageState, error) {
	states := []lineageState{state}
	groups, err := attributeMatcherGroups(state.matchers, budget)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		targets, renamed, _ := renames.targets(group.name, direction)
		if !renamed {
			continue
		}

		nextStates := newLineageStateSet(len(states), budget)
		for _, current := range states {
			if err := budget.reserveWork(1); err != nil {
				return nil, err
			}
			for _, target := range targets {
				if err := budget.reserveWork(1); err != nil {
					return nil, err
				}
				next := current
				next.matchers = slices.Clone(current.matchers)
				for _, i := range group.indexes {
					if err := budget.reserveWork(1); err != nil {
						return nil, err
					}
					matcher := current.matchers[i]
					next.matchers[i] = labels.MustNewMatcher(matcher.Type, target, matcher.Value)
				}
				next.pendingAttributeMatchers, err = updatePendingAttributeMatchers(current.pendingAttributeMatchers, group.indexes, false, budget)
				if err != nil {
					return nil, err
				}
				if err := nextStates.add(next); err != nil {
					return nil, err
				}
			}

			_, isSource := renames.forward[group.name]
			if direction == traverseBackward && !isSource && producedByEarlierChange(earlierTargets, changeIndex, group.name) {
				next := current
				next.pendingAttributeMatchers, err = updatePendingAttributeMatchers(current.pendingAttributeMatchers, group.indexes, true, budget)
				if err != nil {
					return nil, err
				}
				if err := nextStates.add(next); err != nil {
					return nil, err
				}
			}
		}
		states = nextStates.states
	}
	return states, nil
}

func updatePendingAttributeMatchers(pending map[int]struct{}, indexes []int, add bool, budget *schemaExpansionBudget) (map[int]struct{}, error) {
	updated := make(map[int]struct{}, len(pending)+len(indexes))
	for i := range pending {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		updated[i] = struct{}{}
	}
	for _, i := range indexes {
		if err := budget.reserveWork(1); err != nil {
			return nil, err
		}
		if add {
			updated[i] = struct{}{}
		} else {
			delete(updated, i)
		}
	}
	if len(updated) == 0 {
		return nil, nil
	}
	return updated, nil
}

func prunePendingAttributeOrigins(lineage attributeLineage, budget *schemaExpansionBudget) (attributeLineage, error) {
	if len(lineage) == 0 {
		return nil, nil
	}
	out := make(attributeLineage, len(lineage))
	for canonical, currentNames := range lineage {
		for current, origin := range currentNames {
			if err := budget.reserveWork(1); err != nil {
				return nil, err
			}
			if origin != attributeOriginResolved {
				continue
			}
			if out[canonical] == nil {
				out[canonical] = map[string]attributeOrigin{}
			}
			out[canonical][current] = origin
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func deduplicateLineageStatesWithBudget(states []lineageState, budget *schemaExpansionBudget) ([]lineageState, error) {
	out := newLineageStateSet(len(states), budget)
	for _, state := range states {
		if err := out.add(state); err != nil {
			return nil, err
		}
	}
	return out.states, nil
}

func lineageStateKeyWithBudget(state lineageState, budget *schemaExpansionBudget) (string, error) {
	remaining := budget.remainingKeyBytes()
	var size uint64
	addSize := func(n uint64) error {
		if n > remaining-size {
			return schemaExpansionLimitError("deduplication key bytes", budget.limits.keyBytes)
		}
		size += n
		return nil
	}
	addPart := func(value string) error {
		return addSize(keyPartSize(uint64(len(value))))
	}
	addInt := func(value int) error {
		return addSize(keyIntSize(value))
	}

	if err := budget.reserveWork(1); err != nil {
		return "", err
	}
	if err := addInt(len(state.matchers)); err != nil {
		return "", err
	}
	for _, matcher := range state.matchers {
		if err := budget.reserveWork(1); err != nil {
			return "", err
		}
		if err := addSize(1); err != nil {
			return "", err
		}
		if err := addPart(matcher.Name); err != nil {
			return "", err
		}
		if err := addPart(matcher.Value); err != nil {
			return "", err
		}
	}
	if err := addPart(state.metric); err != nil {
		return "", err
	}
	if err := addSize(2); err != nil {
		return "", err
	}
	if err := addInt(len(state.pendingAttributeMatchers)); err != nil {
		return "", err
	}
	for i := range state.pendingAttributeMatchers {
		if err := budget.reserveWork(1); err != nil {
			return "", err
		}
		if err := addInt(i); err != nil {
			return "", err
		}
	}
	if err := addInt(len(state.attrs)); err != nil {
		return "", err
	}
	for canonical, currentNames := range state.attrs {
		if err := budget.reserveWork(1); err != nil {
			return "", err
		}
		if err := addPart(canonical); err != nil {
			return "", err
		}
		if err := addInt(len(currentNames)); err != nil {
			return "", err
		}
		for current := range currentNames {
			if err := budget.reserveWork(1); err != nil {
				return "", err
			}
			if err := addPart(current); err != nil {
				return "", err
			}
			if err := addSize(1); err != nil {
				return "", err
			}
		}
	}
	if err := budget.reserveKeyBytes(size); err != nil {
		return "", err
	}

	var b strings.Builder
	b.Grow(int(size))
	writeKeyInt(&b, len(state.matchers))
	for _, matcher := range state.matchers {
		b.WriteByte(byte(matcher.Type))
		writeKeyPart(&b, matcher.Name)
		writeKeyPart(&b, matcher.Value)
	}
	writeKeyPart(&b, state.metric)
	if state.metricProduced {
		b.WriteByte(1)
	} else {
		b.WriteByte(0)
	}
	if state.metricOriginPending {
		b.WriteByte(1)
	} else {
		b.WriteByte(0)
	}
	pendingMatcherIndexes := make([]int, 0, len(state.pendingAttributeMatchers))
	for i := range state.pendingAttributeMatchers {
		pendingMatcherIndexes = append(pendingMatcherIndexes, i)
	}
	slices.Sort(pendingMatcherIndexes)
	writeKeyInt(&b, len(pendingMatcherIndexes))
	for _, i := range pendingMatcherIndexes {
		writeKeyInt(&b, i)
	}
	canonicalNames := make([]string, 0, len(state.attrs))
	for canonical := range state.attrs {
		canonicalNames = append(canonicalNames, canonical)
	}
	slices.Sort(canonicalNames)
	writeKeyInt(&b, len(canonicalNames))
	for _, canonical := range canonicalNames {
		writeKeyPart(&b, canonical)
		currentNames := make([]string, 0, len(state.attrs[canonical]))
		for current := range state.attrs[canonical] {
			currentNames = append(currentNames, current)
		}
		slices.Sort(currentNames)
		writeKeyInt(&b, len(currentNames))
		for _, current := range currentNames {
			writeKeyPart(&b, current)
			b.WriteByte(byte(state.attrs[canonical][current]))
		}
	}
	return b.String(), nil
}

func matcherKeyWithBudget(matchers []*labels.Matcher, budget *schemaExpansionBudget) (string, error) {
	if err := budget.reserveWork(1); err != nil {
		return "", err
	}
	remaining := budget.remainingKeyBytes()
	size := keyIntSize(len(matchers))
	if size > remaining {
		return "", schemaExpansionLimitError("deduplication key bytes", budget.limits.keyBytes)
	}
	for _, matcher := range matchers {
		if err := budget.reserveWork(1); err != nil {
			return "", err
		}
		parts := 1 + keyPartSize(uint64(len(matcher.Name))) + keyPartSize(uint64(len(matcher.Value)))
		if parts > remaining-size {
			return "", schemaExpansionLimitError("deduplication key bytes", budget.limits.keyBytes)
		}
		size += parts
	}
	if err := budget.reserveKeyBytes(size); err != nil {
		return "", err
	}

	var b strings.Builder
	b.Grow(int(size))
	writeKeyInt(&b, len(matchers))
	for _, matcher := range matchers {
		b.WriteByte(byte(matcher.Type))
		writeKeyPart(&b, matcher.Name)
		writeKeyPart(&b, matcher.Value)
	}
	return b.String(), nil
}

func keyPartSize(valueLen uint64) uint64 {
	return decimalDigits(valueLen) + 1 + valueLen
}

func keyIntSize(value int) uint64 {
	digits := decimalDigits(uint64(value))
	return keyPartSize(digits)
}

func decimalDigits(value uint64) uint64 {
	digits := uint64(1)
	for value >= 10 {
		value /= 10
		digits++
	}
	return digits
}

func writeKeyPart(b *strings.Builder, value string) {
	b.WriteString(strconv.Itoa(len(value)))
	b.WriteByte(':')
	b.WriteString(value)
}

func writeKeyInt(b *strings.Builder, value int) {
	writeKeyPart(b, strconv.Itoa(value))
}

type queryContext struct {
	// warnings holds resolution problems that make the answer incomplete or
	// potentially ambiguous. They are surfaced through Warnings().
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
// Each resolved lineage carries its own label mapping because scoped attribute
// changes can differ between branches.
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
	allVariants := []matcherVariant{{
		matchers: matchers,
		mapping:  buildLabelMapping(metricName, nil),
	}}
	if schemaURL != "" {
		schema, err := e.getOTelSchema(schemaURL)
		if err != nil {
			return nil, queryContext{}, err
		}
		budget := newSchemaExpansionBudget(e.limits)
		analysis := e.schemaSafetyAnalysis(schemaURL, &schema)
		if analysis.err != nil {
			return nil, queryContext{}, analysis.err
		}
		if err := budget.reserveWork(analysis.work); err != nil {
			return nil, queryContext{}, err
		}
		rv := newRenameValidator(sc, metricName)
		allVariants, err = generateMatcherVariantsWithAnalysis(sc.version, &schema, matchers, sc.attributesOf(metricName), rv, &analysis, budget)
		if err != nil {
			return nil, queryContext{}, err
		}
		return allVariants, queryContext{warnings: rv.warnings}, nil
	}

	return allVariants, queryContext{}, nil
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
