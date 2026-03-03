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

package testwal

import (
	"fmt"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

// RecordsCase represents record generation option in a form of a test case.
//
// Generated Series will have refs that monotonic and deterministic, in range of [RefPadding, RefPadding+Series).
type RecordsCase struct {
	Name string

	Series                   int
	SamplesPerSeries         int
	HistogramsPerSeries      int
	FloatHistogramsPerSeries int
	ExemplarsPerSeries       int

	ExtraLabels []labels.Label

	// RefPadding represents a padding to add to Series refs.
	RefPadding int
	// LabelsFn allows injecting custom labels, by default it's a test_metric_%d with ExtraLabels.
	LabelsFn func(lb *labels.ScratchBuilder, ref int) labels.Labels
	// TsFn allows injecting custom sample timestamps. j represents the sample index within the series.
	// By default, it injects j.
	TsFn func(ref, j int) int64
	// HistogramFn source histogram for histogram and float histogram records.
	// By default, newTestHist is used (exponential bucketing)
	HistogramFn func(ref int) *histogram.Histogram
}

// Records represents batches of generated WAL records.
type Records struct {
	Series          []record.RefSeries
	Samples         []record.RefSample
	Histograms      []record.RefHistogramSample
	FloatHistograms []record.RefFloatHistogramSample
	Exemplars       []record.RefExemplar
	Metadata        []record.RefMetadata
}

func newTestHist(i int) *histogram.Histogram {
	return &histogram.Histogram{
		Schema:          2,
		ZeroThreshold:   1e-128,
		ZeroCount:       0,
		Count:           2,
		Sum:             0,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
		PositiveBuckets: []int64{int64(i) + 1},
		NegativeSpans:   []histogram.Span{{Offset: 0, Length: 1}},
		NegativeBuckets: []int64{int64(-i) - 1},
	}
}

// GenerateRecords generates batches of WAL records for a given RecordsCase.
// Batches represents set of series with the given number of counter samples, histograms, etc. per each series ref.
func GenerateRecords(c RecordsCase) (ret Records) {
	ret.Series = make([]record.RefSeries, c.Series)
	ret.Metadata = make([]record.RefMetadata, c.Series)
	ret.Samples = make([]record.RefSample, c.Series*c.SamplesPerSeries)
	ret.Histograms = make([]record.RefHistogramSample, c.Series*c.HistogramsPerSeries)
	ret.FloatHistograms = make([]record.RefFloatHistogramSample, c.Series*c.FloatHistogramsPerSeries)
	ret.Exemplars = make([]record.RefExemplar, c.Series*c.ExemplarsPerSeries)

	if c.LabelsFn == nil {
		c.LabelsFn = func(lb *labels.ScratchBuilder, i int) labels.Labels {
			// Create series with labels that contains name of series plus any extra labels supplied.
			name := fmt.Sprintf("test_metric_%d", i)
			lb.Reset()
			lb.Add(model.MetricNameLabel, name)
			for _, l := range c.ExtraLabels {
				lb.Add(l.Name, l.Value)
			}
			lb.Sort()
			return lb.Labels()
		}
	}
	if c.TsFn == nil {
		c.TsFn = func(_, j int) int64 { return int64(j) }
	}
	if c.HistogramFn == nil {
		c.HistogramFn = newTestHist
	}

	lb := labels.NewScratchBuilder(1 + len(c.ExtraLabels))
	for i := range ret.Series {
		ref := c.RefPadding + i
		ret.Series[i] = record.RefSeries{
			Ref:    chunks.HeadSeriesRef(ref),
			Labels: c.LabelsFn(&lb, ref),
		}
		ret.Metadata[i] = record.RefMetadata{
			Ref:  chunks.HeadSeriesRef(ref),
			Type: uint8(record.Counter),
			Unit: "unit text",
			Help: fmt.Sprintf("help text for %d", ref),
		}
		for j := range c.SamplesPerSeries {
			ret.Samples[i*c.SamplesPerSeries+j] = record.RefSample{
				Ref: chunks.HeadSeriesRef(ref),
				T:   c.TsFn(ref, j),
				V:   float64(ref),
			}
		}
		h := c.HistogramFn(ref)
		for j := range c.HistogramsPerSeries {
			ret.Histograms[i*c.HistogramsPerSeries+j] = record.RefHistogramSample{
				Ref: chunks.HeadSeriesRef(ref),
				T:   c.TsFn(ref, j),
				H:   h,
			}
		}
		for j := range c.FloatHistogramsPerSeries {
			ret.FloatHistograms[i*c.FloatHistogramsPerSeries+j] = record.RefFloatHistogramSample{
				Ref: chunks.HeadSeriesRef(ref),
				T:   c.TsFn(ref, j),
				FH:  h.ToFloat(nil),
			}
		}
		for j := range c.ExemplarsPerSeries {
			ret.Exemplars[i*c.ExemplarsPerSeries+j] = record.RefExemplar{
				Ref:    chunks.HeadSeriesRef(ref),
				T:      c.TsFn(ref, j),
				V:      float64(ref),
				Labels: labels.FromStrings("trace_id", fmt.Sprintf("trace-%d", ref)),
			}
		}
	}
	return ret
}
