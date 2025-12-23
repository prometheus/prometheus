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
	"maps"

	"github.com/prometheus/otlptranslator"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

const targetInfo = "target_info"

// evalInfo implements the info PromQL function.
// It enriches series with resource attributes stored in the TSDB.
// When the second argument specifies __name__="target_info" (or is omitted),
// resource attributes are looked up and added to the series labels.
func (ev *evaluator) evalInfo(ctx context.Context, args parser.Expressions) (parser.Value, annotations.Annotations) {
	val, annots := ev.eval(ctx, args[0])
	mat := val.(Matrix)

	// Check if the querier supports ResourceQuerier first
	rq, ok := ev.querier.(storage.ResourceQuerier)
	if !ok {
		// Querier doesn't support resource lookups, return unchanged
		return mat, annots
	}

	// Build bidirectional mappings between OTel and Prometheus attribute names
	mappings := ev.buildAttrNameMappings(rq)

	// Parse the second argument to get data label matchers.
	dataLabelMatchers := map[string][]*labels.Matcher{}
	useResourceAttrs := true // Default to using resource attributes (target_info)

	if len(args) > 1 {
		labelSelector := args[1].(*parser.VectorSelector)
		for _, m := range labelSelector.LabelMatchers {
			// Translate Prometheus-compatible names back to OTel names using the mapping.
			// This correctly handles the LabelNamer transformation that was applied when
			// the API translated OTel names to Prometheus names.
			attrName := m.Name
			if mappings != nil {
				if original, ok := mappings.toOTel[m.Name]; ok {
					attrName = original
				}
			}
			dataLabelMatchers[attrName] = append(dataLabelMatchers[attrName], m)
			// Check if __name__ is specified and whether it's target_info
			if m.Name == labels.MetricName {
				// Only use resource attributes if the name matcher matches "target_info"
				if !m.Matches(targetInfo) {
					// Other __name__ values are not supported yet, skip enrichment
					useResourceAttrs = false
				}
			}
		}
	}

	// Remove __name__ from dataLabelMatchers since it's only used to select the mode
	delete(dataLabelMatchers, labels.MetricName)

	if !useResourceAttrs {
		// If not using resource attributes, return the original matrix unchanged
		return mat, annots
	}

	// Enrich series with resource attributes
	res := ev.enrichWithResourceAttrs(ctx, mat, rq, dataLabelMatchers, mappings)
	return res, annots
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
			h := sample.Metric.Hash()
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
