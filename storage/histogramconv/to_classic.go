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

package histogramconv

import (
	"math"
	"slices"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// toClassic converts the native histograms of ss, of the representations in
// from, to the classic histogram series with the given suffix, and returns the
// converted series whose le label matches all leMatchers. In debug mode, the
// converted series have the StoredAsLabel, set to the representation of the
// native histograms they were converted from.
//
// Native histograms with an exponential schema have no fixed bucket
// boundaries. So that the resulting classic histograms can be aggregated by
// le, across series and over time, all of them are converted with the same
// derived boundaries, see exponentialBoundaries. Hence the le set depends on
// the selected series and time range, a single low resolution histogram lowers
// the resolution of all of them, and histograms with many buckets result in
// many classic series.
//
// A converted series is marked stale at the first sample of its native
// histogram that does not result in it anymore, e.g. because the native
// histogram went stale, its bucket layout changed or it is not converted, just
// like the scrape loop marks series stale that disappear from a target.
func toClassic(ss storage.SeriesSet, suffix string, from representations, leMatchers []*labels.Matcher, debug bool) ([]*series, error) {
	nhSeries, err := readNativeHistograms(ss, from)
	if err != nil {
		return nil, err
	}
	var boundaries []float64
	if from.has(NHE) && suffix == histogram.ClassicSuffixBucket {
		boundaries = exponentialBoundaries(nhSeries)
	}

	lsetBuilder := labels.NewBuilder(labels.EmptyLabels())
	b := newClassicSeriesBuilder()
	for _, ns := range nhSeries {
		// A cache holds the label sets of the series converted from one
		// native histogram series, whatever its labels, so debug mode, which
		// adds the representation to the labels, needs one per
		// representation.
		var (
			nhcbLabels, nheLabels = ns.labels, ns.labels
			nhcbCache             = &histogram.ClassicSeriesCache{}
			nheCache              = nhcbCache
		)
		if debug {
			nhcbLabels, nheLabels = withStoredAs(ns.labels, NHCB), withStoredAs(ns.labels, NHE)
			nheCache = &histogram.ClassicSeriesCache{}
		}
		b.startSeries()
		for _, smpl := range ns.samples {
			b.startSample(smpl.t)
			if smpl.fh != nil {
				var err error
				if histogram.IsExponentialSchema(smpl.fh.Schema) {
					err = histogram.ConvertExponentialToClassic(smpl.fh, boundaries, nheLabels, lsetBuilder, suffix, nheCache, b.emit)
				} else {
					err = histogram.ConvertNHCBToClassic(smpl.fh, nhcbLabels, lsetBuilder, suffix, nhcbCache, b.emit)
				}
				if err != nil {
					return nil, err
				}
			}
			b.endSample()
		}
	}

	converted := make([]*series, 0, len(b.series))
Series:
	for _, s := range b.series {
		for _, m := range leMatchers {
			if !m.Matches(s.lset.Get(labels.BucketLabel)) {
				continue Series
			}
		}
		converted = append(converted, s)
	}
	return converted, nil
}

// nativeHistogramSeries holds the samples of a native histogram series.
type nativeHistogramSeries struct {
	labels  labels.Labels
	samples []nativeHistogramSample
}

// nativeHistogramSample is a sample of a native histogram series. fh is the
// histogram to convert, nil if the sample is not converted, e.g. a staleness
// marker.
type nativeHistogramSample struct {
	t  int64
	fh *histogram.FloatHistogram
}

// readNativeHistograms drains ss and returns its series. Only the native
// histograms of the representations in from are converted.
func readNativeHistograms(ss storage.SeriesSet, from representations) ([]nativeHistogramSeries, error) {
	var (
		nhSeries []nativeHistogramSeries
		it       chunkenc.Iterator
	)
	for ss.Next() {
		s := ss.At()
		ns := nativeHistogramSeries{labels: s.Labels()}
		it = s.Iterator(it)
		for valType := it.Next(); valType != chunkenc.ValNone; valType = it.Next() {
			smpl := nativeHistogramSample{t: it.AtT()}
			if valType == chunkenc.ValHistogram || valType == chunkenc.ValFloatHistogram {
				// This works for histograms with integer counts, too.
				if _, fh := it.AtFloatHistogram(nil); convertible(fh, from) {
					smpl.fh = fh
				}
			}
			ns.samples = append(ns.samples, smpl)
		}
		if err := it.Err(); err != nil {
			return nil, err
		}
		nhSeries = append(nhSeries, ns)
	}
	return nhSeries, ss.Err()
}

// convertible reports whether the native histogram fh, of the representations
// in from, is converted to classic histogram series.
func convertible(fh *histogram.FloatHistogram, from representations) bool {
	// Staleness markers are not converted, whatever their schema. The series
	// converted from the previous sample are marked stale instead.
	if fh == nil || value.IsStaleNaN(fh.Sum) {
		return false
	}
	return (from.has(NHCB) && histogram.IsCustomBucketsSchema(fh.Schema)) ||
		(from.has(NHE) && histogram.IsExponentialSchema(fh.Schema))
}

// exponentialBoundaries returns the le boundaries to convert the exponential
// native histograms amongst nhSeries with: the union of the boundaries of all
// of them, see histogram.AppendClassicBoundaries, reduced to the lowest schema
// amongst them. As the boundaries of a lower schema are boundaries of every
// higher schema, too, the conversion is exact for each histogram. As all of
// them are converted with the same boundaries, the resulting classic
// histograms can be aggregated by le, across series and over time.
func exponentialBoundaries(nhSeries []nativeHistogramSeries) []float64 {
	minSchema := int32(math.MaxInt32)
	for _, ns := range nhSeries {
		for _, smpl := range ns.samples {
			if smpl.fh != nil && histogram.IsExponentialSchema(smpl.fh.Schema) {
				minSchema = min(minSchema, smpl.fh.Schema)
			}
		}
	}

	var boundaries []float64
	for _, ns := range nhSeries {
		for _, smpl := range ns.samples {
			if smpl.fh == nil || !histogram.IsExponentialSchema(smpl.fh.Schema) {
				continue
			}
			fh := smpl.fh
			if fh.Schema > minSchema {
				fh = fh.CopyToSchema(minSchema)
			}
			boundaries = histogram.AppendClassicBoundaries(boundaries, fh)
		}
		// The samples of a series mostly have the same buckets, so drop the
		// duplicates after each series already.
		slices.Sort(boundaries)
		boundaries = slices.Compact(boundaries)
	}
	return boundaries
}

// classicSeriesBuilder collects the classic histogram series converted from
// native histograms, one native histogram sample after the other.
//
// A converted series is marked stale at the first sample of its native
// histogram that does not result in it anymore, e.g. because the native
// histogram went stale or its bucket layout changed, just like the scrape loop
// marks series stale that disappear from a target.
type classicSeriesBuilder struct {
	series []*series
	// byHash indexes series by label hash rather than by Labels.String().
	byHash map[uint64][]int

	// t is the timestamp of the current sample.
	t int64
	// emitted and prevEmitted are the indices of the series emitted for the
	// current and for the previous sample of the current native histogram.
	emitted, prevEmitted []int
}

func newClassicSeriesBuilder() *classicSeriesBuilder {
	return &classicSeriesBuilder{byHash: make(map[uint64][]int)}
}

// startSeries prepares for the samples of the next native histogram series.
func (b *classicSeriesBuilder) startSeries() {
	b.prevEmitted = b.prevEmitted[:0]
}

// startSample prepares for the series converted from the sample at t.
func (b *classicSeriesBuilder) startSample(t int64) {
	b.t = t
	b.emitted = b.emitted[:0]
}

// emit appends the value v at the timestamp of the current sample to the series
// with labels l. It is the emitSeriesFn of the conversion functions.
func (b *classicSeriesBuilder) emit(l labels.Labels, v float64) error {
	h := l.Hash()
	idx := -1
	for _, candidate := range b.byHash[h] {
		if labels.Equal(b.series[candidate].lset, l) {
			idx = candidate
			break
		}
	}
	if idx == -1 {
		idx = len(b.series)
		b.byHash[h] = append(b.byHash[h], idx)
		b.series = append(b.series, &series{lset: l})
	}

	b.series[idx].samples = append(b.series[idx].samples, fSample{t: b.t, f: v})
	b.emitted = append(b.emitted, idx)
	return nil
}

// endSample marks the series emitted for the previous sample, but not for the
// current one, stale.
func (b *classicSeriesBuilder) endSample() {
	for _, idx := range b.prevEmitted {
		if samples := b.series[idx].samples; samples[len(samples)-1].T() != b.t {
			b.series[idx].samples = append(samples, fSample{t: b.t, f: math.Float64frombits(value.StaleNaN)})
		}
	}
	b.emitted, b.prevEmitted = b.prevEmitted, b.emitted
}
