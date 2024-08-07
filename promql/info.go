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
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

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
	idLblRes := map[string]string{}
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
		idLblRes[name] = sb.String()
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
	for name, re := range idLblRes {
		infoLabelMatchers = append(infoLabelMatchers, labels.MustNewMatcher(labels.MatchRegexp, name, re))
	}
	for _, ms := range ev.includeInfoMetricLabels.DataLabelMatchers {
		for _, m := range ms {
			infoLabelMatchers = append(infoLabelMatchers, m)
		}
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

func (ev *evaluator) combineWithInfoSeries(series, infoSeries []storage.Series, offset time.Duration, dataLabels []string) ([]storage.Series, annotations.Annotations) {
	// Function to compute the join signature for each series.
	buf := make([]byte, 0, 1024)
	// TODO: Fixme.
	sigf := signatureFunc(true, buf, "instance", "job")
	initSignatures := func(series labels.Labels, h *EvalSeriesHelper) {
		h.signature = sigf(series)
	}

	mat := ev.expandSeriesToMatrix(series, offset)
	infoMat := ev.expandSeriesToMatrix(infoSeries, offset)
	matrices := []Matrix{mat, infoMat}
	// Keep a copy of the original point slices so that they
	// can be returned to the pool.
	origMatrices := []Matrix{
		make(Matrix, len(mat)),
		make(Matrix, len(infoMat)),
	}
	copy(origMatrices[0], mat)
	copy(origMatrices[1], infoMat)

	numSteps := int((ev.endTimestamp-ev.startTimestamp)/ev.interval) + 1
	originalNumSamples := ev.currentSamples

	vectors := make([]Vector, 2) // Input vectors for the function.
	// Create an output vector that is as big as the input matrix with
	// the most time series.
	biggestLen := 1
	for i := range matrices {
		vectors[i] = make(Vector, 0, len(matrices[i]))
		if len(matrices[i]) > biggestLen {
			biggestLen = len(matrices[i])
		}
	}
	enh := &EvalNodeHelper{Out: make(Vector, 0, biggestLen)}
	type seriesAndTimestamp struct {
		Series
		ts int64
	}
	seriess := make(map[uint64]seriesAndTimestamp, biggestLen) // Output series by series hash.
	tempNumSamples := ev.currentSamples

	var (
		seriesHelpers [][]EvalSeriesHelper
		bufHelpers    [][]EvalSeriesHelper // Buffer updated on each step
	)

	// Run initSignatures for every single series in the matrix.
	seriesHelpers = make([][]EvalSeriesHelper, len(matrices))
	bufHelpers = make([][]EvalSeriesHelper, len(matrices))
	for i, m := range matrices {
		seriesHelpers[i] = make([]EvalSeriesHelper, len(m))
		bufHelpers[i] = make([]EvalSeriesHelper, len(m))

		for si, series := range m {
			initSignatures(series.Metric, &seriesHelpers[i][si])
		}
	}

	var warnings annotations.Annotations
	for ts := ev.startTimestamp; ts <= ev.endTimestamp; ts += ev.interval {
		if err := contextDone(ev.ctx, "expression evaluation"); err != nil {
			ev.error(err)
		}
		// Reset number of samples in memory after each timestamp.
		ev.currentSamples = tempNumSamples
		// Gather input vectors for this timestamp.
		for i, m := range matrices {
			vectors[i] = vectors[i][:0]
			bufHelpers[i] = bufHelpers[i][:0]

			for si, series := range m {
				switch {
				case len(series.Floats) > 0 && series.Floats[0].T == ts:
					vectors[i] = append(vectors[i], Sample{Metric: series.Metric, F: series.Floats[0].F, T: ts})
					// Move input vectors forward so we don't have to re-scan the same
					// past points at the next step.
					m[si].Floats = series.Floats[1:]
				case len(series.Histograms) > 0 && series.Histograms[0].T == ts:
					vectors[i] = append(vectors[i], Sample{Metric: series.Metric, H: series.Histograms[0].H, T: ts})
					m[si].Histograms = series.Histograms[1:]
				default:
					continue
				}
				bufHelpers[i] = append(bufHelpers[i], seriesHelpers[i][si])
				// Don't add histogram size here because we only
				// copy the pointer above, not the whole
				// histogram.
				ev.currentSamples++
				if ev.currentSamples > ev.maxSamples {
					ev.error(ErrTooManySamples(env))
				}
			}
			ev.samplesStats.UpdatePeak(ev.currentSamples)
		}

		vecMatching := parser.VectorMatching{
			Card: parser.CardManyToOne,
			// TODO: Fix me.
			MatchingLabels: []string{"instance", "job"},
			On:             true,
			Include:        dataLabels,
		}
		enh.Ts = ts
		result, err := ev.VectorBinop(parser.MUL, vectors[0], vectors[1], &vecMatching, false, bufHelpers[0], bufHelpers[1], enh)
		ws := handleVectorBinopError(err, nil)
		warnings.Merge(ws)
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

		// If this could be an instant query, shortcut so as not to change sort order.
		if ev.endTimestamp == ev.startTimestamp {
			if result.ContainsSameLabelset() {
				ev.errorf("vector cannot contain metrics with the same labelset")
			}
			output := make([]storage.Series, len(result))
			numSamples := 0
			for i, s := range result {
				if s.H == nil {
					numSamples++
					output[i] = NewStorageSeries(Series{Metric: s.Metric, Floats: []FPoint{{T: ts, F: s.F}}})
				} else {
					numSamples += s.H.Size()
					output[i] = NewStorageSeries(Series{Metric: s.Metric, Histograms: []HPoint{{T: ts, H: s.H}}})
				}
			}
			ev.currentSamples = originalNumSamples + numSamples
			ev.samplesStats.UpdatePeak(ev.currentSamples)
			return output, warnings
		}

		// Add samples in output vector to output series.
		for _, sample := range result {
			h := sample.Metric.Hash()
			ss, ok := seriess[h]
			if ok {
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
