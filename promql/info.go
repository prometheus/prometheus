// Copyright 2024 The Prometheus Authors
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
	"fmt"
	"strings"
	"time"

	"github.com/grafana/regexp"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// fetchInfoSeries fetches info series given selectHints and matching identifying labels in series.
func (ev *evaluator) fetchInfoSeries(ctx context.Context, series []storage.Series, selectHints *storage.SelectHints) ([]storage.Series, annotations.Annotations, error) {
	// A map of values for all identifying labels we are interested in.
	idLblValues := map[string]map[string]struct{}{}
	for _, identifyingLabels := range ev.includeInfoMetricLabels.InfoMetrics {
		for _, l := range identifyingLabels {
			idLblValues[l] = map[string]struct{}{}
		}
		for _, s := range series {
			// Register relevant values per identifying label for this series.
			lblMap := s.Labels().Map()
			for _, l := range identifyingLabels {
				val := lblMap[l]
				if val != "" {
					idLblValues[l][val] = struct{}{}
				}
			}
		}
	}

	// Generate regexps for every interesting value per identifying label.
	var sb strings.Builder
	idLblRegexps := map[string]string{}
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
	i := 0
	sb.Reset()
	for name := range ev.includeInfoMetricLabels.InfoMetrics {
		if i > 0 {
			sb.WriteRune('|')
		}
		sb.WriteString(regexp.QuoteMeta(name))
		i++
	}
	infoLabelMatchers = append(infoLabelMatchers, labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, sb.String()))
	for name, re := range idLblRegexps {
		infoLabelMatchers = append(infoLabelMatchers, labels.MustNewMatcher(labels.MatchRegexp, name, re))
	}
	for _, ms := range ev.includeInfoMetricLabels.DataLabelMatchers {
		infoLabelMatchers = append(infoLabelMatchers, ms...)
	}

	var annots annotations.Annotations
	infoIt := ev.querier.Select(ctx, false, selectHints, infoLabelMatchers...)
	if infoIt.Err() != nil {
		annots.Merge(infoIt.Warnings())
		return nil, annots, infoIt.Err()
	}
	var infoSeries []storage.Series
	for infoIt.Next() {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}
		infoSeries = append(infoSeries, infoIt.At())
	}
	annots.Merge(infoIt.Warnings())
	return infoSeries, annots, infoIt.Err()
}

func (ev *evaluator) combineWithInfoSeries(series, infoSeries []storage.Series, offset time.Duration, selectHints *storage.SelectHints) ([]storage.Series, annotations.Annotations) {
	lb := labels.NewScratchBuilder(3)
	buf := make([]byte, 0, 1024)
	sigfs := make(map[string]func(labels.Labels) string, len(ev.includeInfoMetricLabels.InfoMetrics))
	for name, identifyingLabels := range ev.includeInfoMetricLabels.InfoMetrics {
		sigfs[name] = func(lset labels.Labels) string {
			lb.Reset()
			lb.Add(labels.MetricName, name)
			lset.MatchLabels(true, identifyingLabels...).Range(func(l labels.Label) {
				lb.Add(l.Name, l.Value)
			})
			lb.Sort()
			return string(lb.Labels().Bytes(buf))
		}
	}

	mat := ev.expandSeriesToMatrix(series, offset, selectHints.Start, selectHints.End, ev.interval)
	infoMat := ev.expandSeriesToMatrix(infoSeries, offset, selectHints.Start, selectHints.End, ev.interval)

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
		Out:          make(Vector, 0, biggestLen),
		labelBuilder: &lb,
	}
	type seriesAndTimestamp struct {
		Series
		ts int64
	}
	seriess := make(map[uint64]seriesAndTimestamp, biggestLen) // Output series by series hash.
	tempNumSamples := ev.currentSamples

	// For every base series, compute signature per info metric.
	baseSigs := make([]map[string]string, 0, len(mat))
	for _, s := range mat {
		sigs := make(map[string]string, len(ev.includeInfoMetricLabels.InfoMetrics))
		for infoName := range ev.includeInfoMetricLabels.InfoMetrics {
			sigs[infoName] = sigfs[infoName](s.Metric)
		}
		baseSigs = append(baseSigs, sigs)
	}

	infoSigs := make([]string, 0, len(infoMat))
	for _, s := range infoMat {
		name := s.Metric.Map()[labels.MetricName]
		infoSigs = append(infoSigs, sigfs[name](s.Metric))
	}

	var warnings annotations.Annotations
	for ts := selectHints.Start; ts <= selectHints.End; ts += ev.interval {
		if err := contextDone(ev.ctx, "expression evaluation"); err != nil {
			ev.error(err)
		}
		// Reset number of samples in memory after each timestamp.
		ev.currentSamples = tempNumSamples
		// Gather input vectors for this timestamp.
		baseVector = ev.gatherVector(ts, mat, baseVector)
		infoVector = ev.gatherVector(ts, infoMat, infoVector)

		enh.Ts = ts
		result, err := ev.infoBinop(baseVector, infoVector, baseSigs, infoSigs, enh)
		if err != nil {
			warnings.Add(err)
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

		// Add samples in output vector to output series.
		for _, sample := range result {
			h := sample.Metric.Hash()
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
	output := make([]storage.Series, 0, len(seriess))
	for _, ss := range seriess {
		numSamples += len(ss.Floats) + totalHPointSize(ss.Histograms)
		output = append(output, NewStorageSeries(ss.Series))
	}
	ev.currentSamples = originalNumSamples + numSamples
	ev.samplesStats.UpdatePeak(ev.currentSamples)
	return output, warnings
}

func (ev *evaluator) gatherVector(ts int64, input Matrix, output Vector) Vector {
	output = output[:0]
	for i, series := range input {
		switch {
		case len(series.Floats) > 0 && series.Floats[0].T == ts:
			output = append(output, Sample{Metric: series.Metric, F: series.Floats[0].F, T: ts})
			// Move input vectors forward so we don't have to re-scan the same
			// past points at the next step.
			input[i].Floats = series.Floats[1:]
		case len(series.Histograms) > 0 && series.Histograms[0].T == ts:
			output = append(output, Sample{Metric: series.Metric, H: series.Histograms[0].H, T: ts})
			input[i].Histograms = series.Histograms[1:]
		default:
			continue
		}

		// Don't add histogram size here because we only
		// copy the pointer above, not the whole
		// histogram.
		ev.currentSamples++
		if ev.currentSamples > ev.maxSamples {
			ev.error(ErrTooManySamples(env))
		}
	}
	ev.samplesStats.UpdatePeak(ev.currentSamples)

	return output
}

// infoBinop evaluates a binary operation between base and info Vectors.
func (ev *evaluator) infoBinop(base, info Vector, baseSigs []map[string]string, infoSigs []string, enh *EvalNodeHelper) (Vector, error) {
	if len(base) == 0 || len(info) == 0 {
		return nil, nil // Short-circuit: nothing is going to match.
	}

	// All samples from the info Vectors hashed by the matching label/values.
	if enh.infoSamplesBySig == nil {
		enh.infoSamplesBySig = make(map[string]Sample, len(enh.Out))
	} else {
		clear(enh.infoSamplesBySig)
	}
	infoSamplesBySig := enh.infoSamplesBySig

	for i, s := range info {
		sig := infoSigs[i]
		if _, exists := infoSamplesBySig[sig]; exists {
			// TODO: Let the newest sample win.
			name := s.Metric.Map()[labels.MetricName]
			ev.errorf("found duplicate series for info metric %s", name)
		}
		infoSamplesBySig[sig] = s
	}

	dataLabelMatchers := ev.includeInfoMetricLabels.DataLabelMatchers
	lb := enh.labelBuilder
	for i, bs := range base {
		lb.Reset()
		seen := bs.Metric.Map()

		// For every info metric name, try to find an info series with the same signature.
		seenInfoMetrics := map[string]struct{}{}
		for infoName, sig := range baseSigs[i] {
			is, exists := infoSamplesBySig[sig]
			if !exists {
				continue
			}

			if _, exists := seenInfoMetrics[infoName]; exists {
				continue
			}

			var err error
			is.Metric.Range(func(l labels.Label) {
				if err != nil {
					return
				}
				if _, exists := dataLabelMatchers[l.Name]; len(dataLabelMatchers) > 0 && !exists {
					// Not among the specified data label matchers.
					return
				}

				if v, exists := seen[l.Name]; exists && v != l.Value {
					err = fmt.Errorf("conflicting label: %s", l.Name)
					return
				}

				lb.Add(l.Name, l.Value)
				seen[l.Name] = l.Value
			})
			if err != nil {
				return nil, err
			}
			seenInfoMetrics[infoName] = struct{}{}
		}
		lb.Sort()
		infoLbls := lb.Labels()

		lb.Reset()
		bs.Metric.Range(func(l labels.Label) {
			lb.Add(l.Name, l.Value)
		})
		infoLbls.Range(func(l labels.Label) {
			lb.Add(l.Name, l.Value)
		})

		enh.Out = append(enh.Out, Sample{
			Metric: lb.Labels(),
			F:      bs.F,
			H:      bs.H,
		})
	}
	return enh.Out, nil
}
