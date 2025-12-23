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

package promql

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

const targetInfo = "target_info"

// identifyingLabels are the labels we consider as identifying for info metrics.
// Currently hard coded, so we don't need knowledge of individual info metrics.
// Used by the target_info fallback path.
var identifyingLabels = []string{"instance", "job"}

// infoMode describes which enrichment paths info() should use.
type infoMode int

const (
	// infoModeNativeOnly uses only native metadata (resource attributes from TSDB).
	// Used when __name__ is absent or exactly "target_info".
	infoModeNativeOnly infoMode = iota
	// infoModeMetricJoinOnly uses only metric-join (querying actual info metrics).
	// Used when native metadata is disabled or __name__ can't match "target_info".
	infoModeMetricJoinOnly
	// infoModeHybrid combines native metadata for the target_info portion and
	// metric-join for other info metrics. Used when __name__ can match both
	// "target_info" and other names (e.g., __name__=~"target_info|build_info").
	infoModeHybrid
)

func (m infoMode) String() string {
	switch m {
	case infoModeNativeOnly:
		return "native"
	case infoModeMetricJoinOnly:
		return "metric-join"
	case infoModeHybrid:
		return "hybrid"
	default:
		return "unknown"
	}
}

// evalInfo implements the info PromQL function.
// It routes between native metadata, metric-join, or a hybrid of both
// depending on the infoResourceStrategy and the __name__ matcher.
func (ev *evaluator) evalInfo(ctx context.Context, args parser.Expressions) (parser.Value, annotations.Annotations) {
	mode := ev.classifyInfoMode(args)
	if ev.metrics != nil {
		ev.metrics.infoFunctionCalls.WithLabelValues(mode.String()).Inc()
	}
	switch mode {
	case infoModeNativeOnly:
		return ev.evalInfoNativeMetadata(ctx, args)
	case infoModeHybrid:
		return ev.evalInfoHybrid(ctx, args)
	default:
		return ev.evalInfoTargetInfo(ctx, args)
	}
}

// classifyInfoMode determines which enrichment path(s) info() should use.
func (ev *evaluator) classifyInfoMode(args parser.Expressions) infoMode {
	if ev.infoResourceStrategy == InfoResourceStrategyTargetInfo {
		return infoModeMetricJoinOnly
	}
	if len(args) <= 1 {
		return infoModeNativeOnly
	}

	hasNameMatcher := false
	matchesTargetInfo := false
	onlyTargetInfo := true

	for _, m := range args[1].(*parser.VectorSelector).LabelMatchers {
		if m.Name != model.MetricNameLabel {
			continue
		}
		hasNameMatcher = true
		if m.Matches(targetInfo) {
			matchesTargetInfo = true
		}
		if m.Type != labels.MatchEqual || m.Value != targetInfo {
			onlyTargetInfo = false
		}
	}

	if !hasNameMatcher {
		return infoModeNativeOnly
	}
	if !matchesTargetInfo {
		return infoModeMetricJoinOnly
	}
	if onlyTargetInfo {
		return infoModeNativeOnly
	}
	return infoModeHybrid
}

// ---------------------------------------------------------------------------
// Native metadata path (ResourceQuerier-based)
// ---------------------------------------------------------------------------

// evalInfoNativeMetadata enriches series with resource attributes stored in the TSDB.
// When strategy is "hybrid", it also performs target_info metric-join
// for backwards compatibility with data ingested before native metadata was enabled.
func (ev *evaluator) evalInfoNativeMetadata(ctx context.Context, args parser.Expressions) (parser.Value, annotations.Annotations) {
	val, annots := ev.eval(ctx, args[0])
	mat := val.(Matrix)
	mat = ev.applyNativeMetadata(ctx, mat, args)

	if ev.infoResourceStrategy == InfoResourceStrategyHybrid {
		// Also join with target_info metrics for backwards compatibility.
		// Native metadata labels already on the series take precedence
		// (errorOnBaseConflict=false, so conflicts are silently resolved).
		res, ws := ev.applyMetricJoin(ctx, mat, args, false, false)
		annots.Merge(ws)
		return res, annots
	}

	return mat, annots
}

// applyNativeMetadata enriches a matrix with resource attributes from the TSDB.
// The __name__ matcher in args is ignored; only non-__name__ matchers are used
// to filter which resource attributes to include.
func (ev *evaluator) applyNativeMetadata(ctx context.Context, mat Matrix, args parser.Expressions) Matrix {
	rq, ok := ev.querier.(storage.ResourceQuerier)
	if !ok {
		return mat
	}

	mappings := ev.buildAttrNameMappings(rq)

	dataLabelMatchers := map[string][]*labels.Matcher{}
	if len(args) > 1 {
		labelSelector := args[1].(*parser.VectorSelector)
		for _, m := range labelSelector.LabelMatchers {
			attrName := m.Name
			if mappings != nil {
				if original, ok := mappings.toOTel[m.Name]; ok {
					attrName = original
				}
			}
			dataLabelMatchers[attrName] = append(dataLabelMatchers[attrName], m)
		}
	}
	delete(dataLabelMatchers, model.MetricNameLabel)

	return ev.enrichWithResourceAttrs(ctx, mat, rq, dataLabelMatchers, mappings)
}

// attrNameMappings contains bidirectional mappings between OTel and Prometheus attribute names.
type attrNameMappings struct {
	// toOTel maps Prometheus label names to original OTel attribute names (for filtering)
	toOTel map[string]string
	// toPrometheus maps OTel attribute names to Prometheus label names (for output)
	toPrometheus map[string]string
}

// buildAttrNameMappings builds bidirectional mappings between OTel and Prometheus attribute names.
// This uses the same LabelNamer configuration as the API to ensure consistent name translation.
func (ev *evaluator) buildAttrNameMappings(rq storage.ResourceQuerier) *attrNameMappings {
	if ev.labelNamerConfig == nil {
		// No LabelNamer config, can't build mappings
		return nil
	}

	labelNamer := &otlptranslator.LabelNamer{
		UTF8Allowed:                 ev.labelNamerConfig.UTF8Allowed,
		UnderscoreLabelSanitization: ev.labelNamerConfig.UnderscoreLabelSanitization,
		PreserveMultipleUnderscores: ev.labelNamerConfig.PreserveMultipleUnderscores,
	}

	mappings := &attrNameMappings{
		toOTel:       make(map[string]string),
		toPrometheus: make(map[string]string),
	}

	// Iterate all unique attribute names and build both mappings
	err := rq.IterUniqueAttributeNames(func(originalName string) {
		// Translate the original OTel name to Prometheus format
		translatedName, err := labelNamer.Build(originalName)
		if err != nil {
			// Skip attributes that can't be translated
			return
		}
		mappings.toOTel[translatedName] = originalName
		mappings.toPrometheus[originalName] = translatedName
	})
	if err != nil {
		// On error, return nil to fall back to no translation
		return nil
	}

	return mappings
}

// enrichWithResourceAttrs enriches each series in mat with resource attributes.
func (ev *evaluator) enrichWithResourceAttrs(ctx context.Context, mat Matrix, rq storage.ResourceQuerier, dataLabelMatchers map[string][]*labels.Matcher, mappings *attrNameMappings) Matrix {
	numSteps := int((ev.endTimestamp-ev.startTimestamp)/ev.interval) + 1
	originalNumSamples := ev.currentSamples

	// Keep a copy of the original point slices so they can be returned to the pool.
	origMatrix := make(Matrix, len(mat))
	copy(origMatrix, mat)

	type seriesAndTimestamp struct {
		Series
		ts int64
	}
	seriess := make(map[uint64]seriesAndTimestamp, len(mat))
	tempNumSamples := ev.currentSamples

	baseVector := make(Vector, 0, len(mat))
	enh := &EvalNodeHelper{
		Out: make(Vector, 0, len(mat)),
	}

	for ts := ev.startTimestamp; ts <= ev.endTimestamp; ts += ev.interval {
		if err := contextDone(ctx, "expression evaluation"); err != nil {
			ev.error(err)
		}

		// Reset number of samples in memory after each timestamp.
		ev.currentSamples = tempNumSamples
		// Gather input vectors for this timestamp.
		baseVector, _ = ev.gatherVector(ts, mat, baseVector, nil, nil)

		enh.Ts = ts
		result := ev.enrichVectorWithResourceAttrs(baseVector, rq, ts, enh, dataLabelMatchers, mappings)
		enh.Out = result[:0] // Reuse result vector.

		vecNumSamples := result.TotalSamples()
		ev.currentSamples += vecNumSamples
		tempNumSamples += vecNumSamples
		ev.samplesStats.UpdatePeak(ev.currentSamples)
		if ev.currentSamples > ev.maxSamples {
			ev.error(ErrTooManySamples(env))
		}

		// Add samples in result vector to output series.
		for _, sample := range result {
			h := labels.StableHash(sample.Metric)
			ss, exists := seriess[h]
			if exists {
				if ss.ts == ts {
					ev.errorf("vector cannot contain metrics with the same labelset")
				}
				ss.ts = ts
			} else {
				ss = seriesAndTimestamp{Series{Metric: sample.Metric}, ts}
			}
			addToSeries(&ss.Series, enh.Ts, sample.F, sample.H, numSteps)
			seriess[h] = ss
		}
	}

	// Reuse the original point slices.
	for _, s := range origMatrix {
		putFPointSlice(s.Floats)
		putHPointSlice(s.Histograms)
	}

	// Assemble the output matrix.
	numSamples := 0
	output := make(Matrix, 0, len(seriess))
	for _, ss := range seriess {
		numSamples += len(ss.Floats) + totalHPointSize(ss.Histograms)
		output = append(output, ss.Series)
	}
	ev.currentSamples = originalNumSamples + numSamples
	ev.samplesStats.UpdatePeak(ev.currentSamples)
	return output
}

// enrichVectorWithResourceAttrs enriches each sample in the vector with resource attributes.
func (*evaluator) enrichVectorWithResourceAttrs(base Vector, rq storage.ResourceQuerier, timestamp int64, enh *EvalNodeHelper, dataLabelMatchers map[string][]*labels.Matcher, mappings *attrNameMappings) Vector {
	if len(base) == 0 {
		return nil
	}

	for _, bs := range base {
		// Use StableHash because resource attributes are keyed by StableHash (not Hash)
		hash := labels.StableHash(bs.Metric)

		// Look up resource attributes for this series at this timestamp
		rv, found := rq.GetResourceAt(hash, timestamp)
		if !found || rv == nil {
			// No resource attributes found.
			// Check if filters reference labels that don't exist on base metric.
			// If a filter requires non-empty value for a non-existent label, skip this series.
			if hasUnmatchedFilter(bs.Metric, dataLabelMatchers) {
				continue
			}
			// Otherwise return the original sample unchanged
			enh.Out = append(enh.Out, Sample{
				Metric: bs.Metric,
				F:      bs.F,
				H:      bs.H,
			})
			continue
		}

		// Combine all resource attributes for matching
		allAttrs := make(map[string]string, len(rv.Identifying)+len(rv.Descriptive))
		maps.Copy(allAttrs, rv.Identifying)
		maps.Copy(allAttrs, rv.Descriptive)

		// If filters are specified, check that ALL matchers are satisfied
		if len(dataLabelMatchers) > 0 {
			if !allMatchersSatisfied(allAttrs, dataLabelMatchers) {
				// At least one matcher didn't match, skip this series entirely
				continue
			}
		}

		// Build the set of labels from the base metric
		baseLabels := bs.Metric.Map()
		enh.resetBuilder(bs.Metric)

		// Add resource attributes (both identifying and descriptive)
		// Skip attributes that clash with existing labels
		addAttrsToBuilder(allAttrs, baseLabels, dataLabelMatchers, mappings, enh)

		enh.Out = append(enh.Out, Sample{
			Metric: enh.lb.Labels(),
			F:      bs.F,
			H:      bs.H,
		})
	}

	return enh.Out
}

// allMatchersSatisfied checks if all matchers in dataLabelMatchers are satisfied by the attributes.
func allMatchersSatisfied(attrs map[string]string, dataLabelMatchers map[string][]*labels.Matcher) bool {
	for attrName, matchers := range dataLabelMatchers {
		value, exists := attrs[attrName]
		if !exists {
			// Attribute doesn't exist - check if matchers accept empty string
			for _, m := range matchers {
				if !m.Matches("") {
					return false
				}
			}
			continue
		}
		// Check if the value matches all matchers for this attribute
		for _, m := range matchers {
			if !m.Matches(value) {
				return false
			}
		}
	}
	return true
}

// hasUnmatchedFilter returns true if any filter references a label that doesn't exist
// on the base metric and requires a non-empty value.
// This is used when no resource attributes are found to decide if the series should be skipped.
func hasUnmatchedFilter(metric labels.Labels, dataLabelMatchers map[string][]*labels.Matcher) bool {
	metricMap := metric.Map()
	for attrName, matchers := range dataLabelMatchers {
		// Check if this attribute exists on the base metric (either by original or translated name)
		if _, exists := metricMap[attrName]; exists {
			// Label exists on base metric, filter is satisfied
			continue
		}
		// Label doesn't exist on base metric, check if matchers require non-empty value
		for _, m := range matchers {
			if !m.Matches("") {
				// This matcher requires non-empty value for a non-existent label
				return true
			}
		}
	}
	return false
}

// addAttrsToBuilder adds attributes from attrs to the label builder,
// filtering by dataLabelMatchers and skipping attributes that clash with baseLabels.
// If mappings is provided, attribute names are translated to Prometheus-compatible names.
// Note: This function assumes allMatchersSatisfied() has already verified the matchers.
func addAttrsToBuilder(
	attrs map[string]string,
	baseLabels map[string]string,
	dataLabelMatchers map[string][]*labels.Matcher,
	mappings *attrNameMappings,
	enh *EvalNodeHelper,
) {
	for name, value := range attrs {
		// Determine the output label name (translated if mappings available)
		outputName := name
		if mappings != nil {
			if translated, ok := mappings.toPrometheus[name]; ok {
				outputName = translated
			}
		}

		// Skip if this attribute already exists as a label on the base metric
		// Check both the original and translated names
		if _, exists := baseLabels[name]; exists {
			continue
		}
		if _, exists := baseLabels[outputName]; exists {
			continue
		}

		// If dataLabelMatchers is specified (non-empty), only add attributes that are in the filter
		if len(dataLabelMatchers) > 0 {
			if _, hasMatchers := dataLabelMatchers[name]; !hasMatchers {
				// This attribute name is not in the filter, skip it
				continue
			}
		}

		// Add the attribute to the label builder using the translated name
		enh.lb.Set(outputName, value)
	}
}

// ---------------------------------------------------------------------------
// Hybrid path (native metadata + metric-join)
// ---------------------------------------------------------------------------

// evalInfoHybrid combines native metadata and metric-join enrichment.
// Native metadata handles the target_info portion (resource attributes from TSDB),
// while metric-join handles other info metrics (e.g. build_info).
// Used when __name__ can match both "target_info" and other info metric names.
//
// When strategy is "hybrid", target_info is also included in
// metric-join (not excluded), and conflicts are silently resolved with native
// metadata taking precedence. This supports backwards compatibility.
func (ev *evaluator) evalInfoHybrid(ctx context.Context, args parser.Expressions) (parser.Value, annotations.Annotations) {
	val, annots := ev.eval(ctx, args[0])
	mat := val.(Matrix)

	// Step 1: Enrich with native metadata (resource attributes for the target_info portion).
	// Only pass args[0] (no label selector) so that non-__name__ data label matchers
	// don't incorrectly filter/drop series based on resource attributes.
	// Those matchers are meant for the metric-join step.
	mat = ev.applyNativeMetadata(ctx, mat, args[:1])

	// Step 2: Enrich with metric-join.
	excludeTargetInfo := ev.infoResourceStrategy == InfoResourceStrategyResourceAttributes
	errorOnBaseConflict := ev.infoResourceStrategy == InfoResourceStrategyResourceAttributes
	res, ws := ev.applyMetricJoin(ctx, mat, args, excludeTargetInfo, errorOnBaseConflict)
	annots.Merge(ws)
	return res, annots
}

// ---------------------------------------------------------------------------
// Metric-join path (used when native metadata is disabled, or for non-target_info
// info metrics in hybrid mode)
// ---------------------------------------------------------------------------

// evalInfoTargetInfo implements the info PromQL function using metric joins.
func (ev *evaluator) evalInfoTargetInfo(ctx context.Context, args parser.Expressions) (parser.Value, annotations.Annotations) {
	val, annots := ev.eval(ctx, args[0])
	mat := val.(Matrix)
	res, ws := ev.applyMetricJoin(ctx, mat, args, false, false)
	annots.Merge(ws)
	return res, annots
}

// applyMetricJoin enriches a matrix by joining with info metric series.
// When excludeTargetInfo is true, target_info is excluded from the storage
// query (used in hybrid mode where native metadata handles target_info).
// When errorOnBaseConflict is true, an error is returned if a base label
// (e.g. from native metadata enrichment) conflicts with an info metric label.
func (ev *evaluator) applyMetricJoin(ctx context.Context, mat Matrix, args parser.Expressions, excludeTargetInfo, errorOnBaseConflict bool) (Matrix, annotations.Annotations) {
	// Map from data label name to matchers.
	dataLabelMatchers := map[string][]*labels.Matcher{}
	var infoNameMatchers []*labels.Matcher
	if len(args) > 1 {
		// TODO: Introduce a dedicated LabelSelector type.
		labelSelector := args[1].(*parser.VectorSelector)
		for _, m := range labelSelector.LabelMatchers {
			dataLabelMatchers[m.Name] = append(dataLabelMatchers[m.Name], m)
			if m.Name == model.MetricNameLabel {
				infoNameMatchers = append(infoNameMatchers, m)
			}
		}
	} else {
		infoNameMatchers = []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, targetInfo)}
	}

	// Don't try to enrich info series with themselves.
	ignoreSeries := map[uint64]struct{}{}
	for _, s := range mat {
		name := s.Metric.Get(model.MetricNameLabel)
		if len(infoNameMatchers) > 0 && matchersMatch(infoNameMatchers, name) {
			ignoreSeries[s.Metric.Hash()] = struct{}{}
		}
	}

	selectHints := ev.infoSelectHints(args[0])
	infoSeries, ws, err := ev.fetchInfoSeries(ctx, mat, ignoreSeries, dataLabelMatchers, selectHints, excludeTargetInfo)
	if err != nil {
		ev.error(err)
	}

	var annots annotations.Annotations
	annots.Merge(ws)
	res, ws := ev.combineWithInfoSeries(ctx, mat, infoSeries, ignoreSeries, dataLabelMatchers, errorOnBaseConflict)
	annots.Merge(ws)
	return res, annots
}

func matchersMatch(matchers []*labels.Matcher, value string) bool {
	for _, m := range matchers {
		if !m.Matches(value) {
			return false
		}
	}
	return true
}

// infoSelectHints calculates the storage.SelectHints for selecting info series, given expr (first argument to info call).
func (ev *evaluator) infoSelectHints(expr parser.Expr) storage.SelectHints {
	var nodeTimestamp *int64
	var offset int64
	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		switch n := node.(type) {
		case *parser.VectorSelector:
			if n.Timestamp != nil {
				nodeTimestamp = n.Timestamp
			}
			offset = durationMilliseconds(n.OriginalOffset)
			return errors.New("end traversal")
		default:
			return nil
		}
	})

	start := ev.startTimestamp
	end := ev.endTimestamp
	if nodeTimestamp != nil {
		// The timestamp on the selector overrides everything.
		start = *nodeTimestamp
		end = *nodeTimestamp
	}
	// Reduce the start by one fewer ms than the lookback delta
	// because wo want to exclude samples that are precisely the
	// lookback delta before the eval time.
	start -= durationMilliseconds(ev.lookbackDelta) - 1
	start -= offset
	end -= offset

	return storage.SelectHints{
		Start: start,
		End:   end,
		Step:  ev.interval,
		Func:  "info",
	}
}

// fetchInfoSeries fetches info series given matching identifying labels in mat.
// Series in ignoreSeries are not fetched.
// When excludeTargetInfo is true, target_info is excluded from the storage query.
// dataLabelMatchers may be mutated.
func (ev *evaluator) fetchInfoSeries(ctx context.Context, mat Matrix, ignoreSeries map[uint64]struct{}, dataLabelMatchers map[string][]*labels.Matcher, selectHints storage.SelectHints, excludeTargetInfo bool) (Matrix, annotations.Annotations, error) {
	removeNameFromDataLabelMatchers := func() {
		for name, ms := range dataLabelMatchers {
			ms = slices.DeleteFunc(ms, func(m *labels.Matcher) bool {
				return m.Name == model.MetricNameLabel
			})
			if len(ms) > 0 {
				dataLabelMatchers[name] = ms
			} else {
				delete(dataLabelMatchers, name)
			}
		}
	}

	// A map of values for all identifying labels we are interested in.
	idLblValues := map[string]map[string]struct{}{}
	for _, s := range mat {
		if _, exists := ignoreSeries[s.Metric.Hash()]; exists {
			continue
		}

		// Register relevant values per identifying label for this series.
		for _, l := range identifyingLabels {
			val := s.Metric.Get(l)
			if val == "" {
				continue
			}

			if idLblValues[l] == nil {
				idLblValues[l] = map[string]struct{}{}
			}
			idLblValues[l][val] = struct{}{}
		}
	}
	if len(idLblValues) == 0 {
		// Even when returning early, we need to remove __name__ from dataLabelMatchers
		// since it's not a data label selector (it's used to select which info metrics
		// to consider). Without this, combineWithInfoVector would incorrectly exclude
		// series when only __name__ is specified in the selector.
		removeNameFromDataLabelMatchers()
		return nil, nil, nil
	}

	// Generate regexps for every interesting value per identifying label.
	var sb strings.Builder
	idLblRegexps := make(map[string]string, len(idLblValues))
	for name, vals := range idLblValues {
		sb.Reset()
		i := 0
		for v := range vals {
			if i > 0 {
				sb.WriteRune('|')
			}
			sb.WriteString(regexp.QuoteMeta(v))
			i++
		}
		idLblRegexps[name] = sb.String()
	}

	var infoLabelMatchers []*labels.Matcher
	for name, re := range idLblRegexps {
		infoLabelMatchers = append(infoLabelMatchers, labels.MustNewMatcher(labels.MatchRegexp, name, re))
	}
	hasNameMatcher := false
	for _, ms := range dataLabelMatchers {
		for _, m := range ms {
			if m.Name == model.MetricNameLabel {
				hasNameMatcher = true
			}
			infoLabelMatchers = append(infoLabelMatchers, m)
		}
	}
	removeNameFromDataLabelMatchers()
	if !hasNameMatcher {
		// Default to using the target_info metric.
		infoLabelMatchers = append([]*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, targetInfo)}, infoLabelMatchers...)
	}
	if excludeTargetInfo {
		// In hybrid mode, native metadata handles target_info.
		// Exclude it from the metric-join storage query.
		infoLabelMatchers = append(infoLabelMatchers, labels.MustNewMatcher(labels.MatchNotEqual, model.MetricNameLabel, targetInfo))
	}

	infoIt := ev.querier.Select(ctx, false, &selectHints, infoLabelMatchers...)
	infoSeries, ws, err := expandSeriesSet(ctx, infoIt)
	if err != nil {
		return nil, ws, err
	}

	infoMat := ev.evalSeries(ctx, infoSeries, 0, true)
	return infoMat, ws, nil
}

// combineWithInfoSeries combines mat with select data labels from infoMat.
// When errorOnBaseConflict is true, an error is returned if a label already
// on the base metric conflicts with an info metric label (used in hybrid mode
// to detect conflicts between native metadata and metric-join labels).
func (ev *evaluator) combineWithInfoSeries(ctx context.Context, mat, infoMat Matrix, ignoreSeries map[uint64]struct{}, dataLabelMatchers map[string][]*labels.Matcher, errorOnBaseConflict bool) (Matrix, annotations.Annotations) {
	buf := make([]byte, 0, 1024)
	lb := labels.NewScratchBuilder(0)
	sigFunction := func(name string) func(labels.Labels) string {
		return func(lset labels.Labels) string {
			lb.Reset()
			lb.Add(model.MetricNameLabel, name)
			lset.MatchLabels(true, identifyingLabels...).Range(func(l labels.Label) {
				lb.Add(l.Name, l.Value)
			})
			lb.Sort()
			return string(lb.Labels().Bytes(buf))
		}
	}

	infoMetrics := map[string]struct{}{}
	for _, is := range infoMat {
		lblMap := is.Metric.Map()
		infoMetrics[lblMap[model.MetricNameLabel]] = struct{}{}
	}
	sigfs := make(map[string]func(labels.Labels) string, len(infoMetrics))
	for name := range infoMetrics {
		sigfs[name] = sigFunction(name)
	}

	// Keep a copy of the original point slices so they can be returned to the pool.
	origMatrices := []Matrix{
		make(Matrix, len(mat)),
		make(Matrix, len(infoMat)),
	}
	copy(origMatrices[0], mat)
	copy(origMatrices[1], infoMat)

	numSteps := int((ev.endTimestamp-ev.startTimestamp)/ev.interval) + 1
	originalNumSamples := ev.currentSamples

	// Create an output vector that is as big as the input matrix with
	// the most time series.
	biggestLen := max(len(mat), len(infoMat))
	baseVector := make(Vector, 0, len(mat))
	infoVector := make(Vector, 0, len(infoMat))
	enh := &EvalNodeHelper{
		Out: make(Vector, 0, biggestLen),
	}
	type seriesAndTimestamp struct {
		Series
		ts int64
	}
	seriess := make(map[uint64]seriesAndTimestamp, biggestLen) // Output series by series hash.
	tempNumSamples := ev.currentSamples

	// For every base series, compute signature per info metric.
	baseSigs := make(map[uint64]map[string]string, len(mat))
	for _, s := range mat {
		sigs := make(map[string]string, len(infoMetrics))
		for infoName := range infoMetrics {
			sigs[infoName] = sigfs[infoName](s.Metric)
		}
		baseSigs[s.Metric.Hash()] = sigs
	}

	infoSigs := make(map[uint64]string, len(infoMat))
	for _, s := range infoMat {
		name := s.Metric.Map()[model.MetricNameLabel]
		infoSigs[s.Metric.Hash()] = sigfs[name](s.Metric)
	}

	for ts := ev.startTimestamp; ts <= ev.endTimestamp; ts += ev.interval {
		if err := contextDone(ctx, "expression evaluation"); err != nil {
			ev.error(err)
		}

		// Reset number of samples in memory after each timestamp.
		ev.currentSamples = tempNumSamples
		// Gather input vectors for this timestamp.
		baseVector, _ = ev.gatherVector(ts, mat, baseVector, nil, nil)
		infoVector, _ = ev.gatherVector(ts, infoMat, infoVector, nil, nil)

		enh.Ts = ts
		result, err := ev.combineWithInfoVector(baseVector, infoVector, ignoreSeries, baseSigs, infoSigs, enh, dataLabelMatchers, errorOnBaseConflict)
		if err != nil {
			ev.error(err)
		}
		enh.Out = result[:0] // Reuse result vector.

		vecNumSamples := result.TotalSamples()
		ev.currentSamples += vecNumSamples
		// When we reset currentSamples to tempNumSamples during the next iteration of the loop it also
		// needs to include the samples from the result here, as they're still in memory.
		tempNumSamples += vecNumSamples
		ev.samplesStats.UpdatePeak(ev.currentSamples)
		if ev.currentSamples > ev.maxSamples {
			ev.error(ErrTooManySamples(env))
		}

		// Add samples in result vector to output series.
		for _, sample := range result {
			h := labels.StableHash(sample.Metric)
			ss, exists := seriess[h]
			if exists {
				if ss.ts == ts { // If we've seen this output series before at this timestamp, it's a duplicate.
					ev.errorf("vector cannot contain metrics with the same labelset")
				}
				ss.ts = ts
			} else {
				ss = seriesAndTimestamp{Series{Metric: sample.Metric}, ts}
			}
			addToSeries(&ss.Series, enh.Ts, sample.F, sample.H, numSteps)
			seriess[h] = ss
		}
	}

	// Reuse the original point slices.
	for _, m := range origMatrices {
		for _, s := range m {
			putFPointSlice(s.Floats)
			putHPointSlice(s.Histograms)
		}
	}
	// Assemble the output matrix. By the time we get here we know we don't have too many samples.
	numSamples := 0
	output := make(Matrix, 0, len(seriess))
	for _, ss := range seriess {
		numSamples += len(ss.Floats) + totalHPointSize(ss.Histograms)
		output = append(output, ss.Series)
	}
	ev.currentSamples = originalNumSamples + numSamples
	ev.samplesStats.UpdatePeak(ev.currentSamples)
	return output, nil
}

// combineWithInfoVector combines base and info Vectors.
// Base series in ignoreSeries are not combined.
// When errorOnBaseConflict is true, an error is returned if a label already
// on the base metric has a different value than the corresponding info metric label.
func (ev *evaluator) combineWithInfoVector(base, info Vector, ignoreSeries map[uint64]struct{}, baseSigs map[uint64]map[string]string, infoSigs map[uint64]string, enh *EvalNodeHelper, dataLabelMatchers map[string][]*labels.Matcher, errorOnBaseConflict bool) (Vector, error) {
	if len(base) == 0 {
		return nil, nil // Short-circuit: nothing is going to match.
	}

	// All samples from the info Vector hashed by the matching label/values.
	if enh.rightStrSigs == nil {
		enh.rightStrSigs = make(map[string]Sample, len(enh.Out))
	} else {
		clear(enh.rightStrSigs)
	}

	for _, s := range info {
		if s.H != nil {
			ev.error(errors.New("info sample should be float"))
		}
		// We encode original info sample timestamps via the float value.
		origT := int64(s.F)

		sig := infoSigs[s.Metric.Hash()]
		if existing, exists := enh.rightStrSigs[sig]; exists {
			// We encode original info sample timestamps via the float value.
			existingOrigT := int64(existing.F)
			switch {
			case existingOrigT > origT:
				// Keep the other info sample, since it's newer.
			case existingOrigT < origT:
				// Keep this info sample, since it's newer.
				enh.rightStrSigs[sig] = s
			default:
				// The two info samples have the same timestamp - conflict.
				ev.errorf("found duplicate series for info metric: existing %s @ %d, new %s @ %d",
					existing.Metric.String(), existingOrigT, s.Metric.String(), origT)
			}
		} else {
			enh.rightStrSigs[sig] = s
		}
	}

	for _, bs := range base {
		hash := bs.Metric.Hash()

		if _, exists := ignoreSeries[hash]; exists {
			// This series should not be enriched with info metric data labels.
			enh.Out = append(enh.Out, Sample{
				Metric: bs.Metric,
				F:      bs.F,
				H:      bs.H,
			})
			continue
		}

		baseLabels := bs.Metric.Map()
		enh.resetBuilder(labels.Labels{})

		// For every info metric name, try to find an info series with the same signature.
		matched := false
		for _, sig := range baseSigs[hash] {
			is, exists := enh.rightStrSigs[sig]
			if !exists {
				continue
			}

			err := is.Metric.Validate(func(l labels.Label) error {
				if l.Name == model.MetricNameLabel {
					return nil
				}
				if _, exists := dataLabelMatchers[l.Name]; len(dataLabelMatchers) > 0 && !exists {
					// Not among the specified data label matchers.
					return nil
				}

				if v := enh.lb.Get(l.Name); v != "" && v != l.Value {
					return fmt.Errorf("conflicting label: %s", l.Name)
				}
				if v, exists := baseLabels[l.Name]; exists {
					if errorOnBaseConflict && v != l.Value {
						return fmt.Errorf("conflicting label %s: enriched metric value %q, info metric value %q", l.Name, v, l.Value)
					}
					// Skip labels already on the base metric.
					return nil
				}

				enh.lb.Set(l.Name, l.Value)
				return nil
			})
			if err != nil {
				return nil, err
			}
			matched = true
		}

		infoLbls := enh.lb.Labels()
		if !matched {
			// No info series matched this base series. If there's at least one data
			// label matcher not matching the empty string, we have to ignore this
			// series as there are no matching info series.
			allMatchersMatchEmpty := true
			for _, ms := range dataLabelMatchers {
				for _, m := range ms {
					if !m.Matches("") {
						allMatchersMatchEmpty = false
						break
					}
				}
			}
			if !allMatchersMatchEmpty {
				continue
			}
		}

		enh.resetBuilder(bs.Metric)
		infoLbls.Range(func(l labels.Label) {
			enh.lb.Set(l.Name, l.Value)
		})

		enh.Out = append(enh.Out, Sample{
			Metric: enh.lb.Labels(),
			F:      bs.F,
			H:      bs.H,
		})
	}
	return enh.Out, nil
}
