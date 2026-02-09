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

package storage

import (
	"context"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

// NHCBAsClassicQuerier wraps a Querier and converts NHCB (Native Histogram Custom Buckets)
// queries to classic histogram format when classic series don't exist.
type NHCBAsClassicQuerier struct {
	Querier
}

// NewNHCBAsClassicQuerier returns a new querier that wraps the given querier
// and converts NHCB to classic histogram format for queries.
func NewNHCBAsClassicQuerier(q Querier) Querier {
	return &NHCBAsClassicQuerier{Querier: q}
}

// NHCBAsClassicStorage wraps a Storage and applies NHCB-to-classic conversion
// to queriers when enabled.
type NHCBAsClassicStorage struct {
	Storage
}

// NewNHCBAsClassicStorage returns a new storage that wraps the given storage
// and applies NHCB-to-classic conversion to queriers.
func NewNHCBAsClassicStorage(s Storage) Storage {
	return &NHCBAsClassicStorage{Storage: s}
}

// Querier implements the Storage interface.
func (s *NHCBAsClassicStorage) Querier(mint, maxt int64) (Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return NewNHCBAsClassicQuerier(q), nil
}

// Select implements the Querier interface.
func (q *NHCBAsClassicQuerier) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	metricName, suffix, baseMatchers := extractHistogramSuffix(matchers)
	if suffix == "" {
		// Not a classic histogram query, pass through
		return q.Querier.Select(ctx, sortSeries, hints, matchers...)
	}

	classicSet := q.Querier.Select(ctx, sortSeries, hints, matchers...)
	if classicSet.Err() != nil {
		return classicSet
	}

	var classicSeries []Series
	for classicSet.Next() {
		classicSeries = append(classicSeries, classicSet.At())
	}

	if err := classicSet.Err(); err != nil {
		return ErrSeriesSet(err)
	}

	if len(classicSeries) > 0 {
		return &bufferedSeriesSet{series: classicSeries}
	}

	baseMatchers = append(baseMatchers, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, metricName))
	nhcbSet := q.Querier.Select(ctx, sortSeries, hints, baseMatchers...)
	if nhcbSet.Err() != nil {
		return nhcbSet
	}

	return &nhcbToClassicSeriesSet{
		nhcbSet:    nhcbSet,
		suffix:     suffix,
		metricName: metricName,
	}
}

// bufferedSeriesSet wraps a buffered list of series.
type bufferedSeriesSet struct {
	series []Series
	idx    int
}

func (b *bufferedSeriesSet) Next() bool {
	if b.idx < len(b.series) {
		b.idx++
		return true
	}
	return false
}

func (b *bufferedSeriesSet) At() Series {
	if b.idx == 0 || b.idx > len(b.series) {
		return nil
	}
	return b.series[b.idx-1]
}

func (*bufferedSeriesSet) Err() error {
	return nil
}

func (*bufferedSeriesSet) Warnings() annotations.Annotations {
	return nil
}

// extractHistogramSuffix extracts the metric name, suffix (_bucket, _count, _sum), and base matchers.
// Returns empty suffix if not a classic histogram query.
func extractHistogramSuffix(matchers []*labels.Matcher) (string, string, []*labels.Matcher) {
	var metricName string
	baseMatchers := make([]*labels.Matcher, 0, len(matchers))

	for _, m := range matchers {
		if m.Name == model.MetricNameLabel {
			metricName = m.Value
		} else {
			baseMatchers = append(baseMatchers, m)
		}
	}

	if metricName == "" {
		return "", "", matchers
	}

	var suffix string

	switch {
	case strings.HasSuffix(metricName, "_bucket"):
		suffix = "_bucket"
	case strings.HasSuffix(metricName, "_count"):
		suffix = "_count"
	case strings.HasSuffix(metricName, "_sum"):
		suffix = "_sum"
	default:
		return "", "", matchers
	}

	baseName := metricName[:len(metricName)-len(suffix)]
	return baseName, suffix, baseMatchers
}

// nhcbToClassicSeriesSet converts NHCB series to classic histogram series format.
type nhcbToClassicSeriesSet struct {
	nhcbSet    SeriesSet
	suffix     string
	metricName string
	series     []Series
	idx        int
	err        error
}

func (s *nhcbToClassicSeriesSet) Next() bool {
	if s.err != nil {
		return false
	}

	// convert all NHCB series on first Next() call
	// It is easier to implement like this as a single NHCB represents multiple series when converted
	// to a classic Histogram.
	if s.series == nil {
		s.series = make([]Series, 0)
		lsetBuilder := labels.NewBuilder(labels.EmptyLabels())

		convertedSeriesMap := make(map[string]*convertedSeriesData)

		for s.nhcbSet.Next() {
			nhcbSeries := s.nhcbSet.At()
			if nhcbSeries == nil {
				continue
			}

			// Check if this is an NHCB histogram series
			nhcbLabels := nhcbSeries.Labels()
			it := nhcbSeries.Iterator(nil)
			if it == nil {
				continue
			}

			for {
				valType := it.Next()
				if valType == chunkenc.ValNone {
					break
				}

				var h *histogram.Histogram
				var fh *histogram.FloatHistogram
				var t int64

				switch valType {
				case chunkenc.ValHistogram:
					t, h = it.AtHistogram(nil)
					if h == nil || !histogram.IsCustomBucketsSchema(h.Schema) {
						continue
					}
				case chunkenc.ValFloatHistogram:
					t, fh = it.AtFloatHistogram(nil)
					if fh == nil || !histogram.IsCustomBucketsSchema(fh.Schema) {
						continue
					}
				default:
					// Not a histogram, skip
					continue
				}

				var nhcb any
				if h != nil {
					nhcb = h
				} else {
					nhcb = fh
				}

				// We could try to find a way to cache the names and do only once the string concatenation
				// also this convert to all parts of the Histogram (buckets sum, count) while we only need one
				err := histogram.ConvertNHCBToClassic(nhcb, nhcbLabels, lsetBuilder, func(l labels.Labels, value float64) error {
					// keep only series matching the requested suffix
					name := l.Get(model.MetricNameLabel)
					if !strings.HasSuffix(name, s.suffix) {
						return nil
					}

					key := l.String()
					if _, exists := convertedSeriesMap[key]; !exists {
						convertedSeriesMap[key] = &convertedSeriesData{
							labels:  l,
							samples: make([]chunks.Sample, 0),
						}
					}

					convertedSeriesMap[key].samples = append(convertedSeriesMap[key].samples, fSample{
						t: t,
						f: value,
					})
					return nil
				})
				if err != nil {
					s.err = err
					return false
				}
			}

			if err := it.Err(); err != nil {
				s.err = err
				return false
			}
		}

		if err := s.nhcbSet.Err(); err != nil {
			s.err = err
			return false
		}

		for _, data := range convertedSeriesMap {
			s.series = append(s.series, NewListSeries(data.labels, data.samples))
		}
	}

	if s.idx < len(s.series) {
		s.idx++
		return true
	}

	return false
}

func (s *nhcbToClassicSeriesSet) At() Series {
	if s.idx == 0 || s.idx > len(s.series) {
		return nil
	}
	return s.series[s.idx-1]
}

func (s *nhcbToClassicSeriesSet) Err() error {
	return s.err
}

func (s *nhcbToClassicSeriesSet) Warnings() annotations.Annotations {
	return s.nhcbSet.Warnings()
}

type convertedSeriesData struct {
	labels  labels.Labels
	samples []chunks.Sample
}
