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

// Package histogramconv converts between histogram representations at query
// time, e.g. so that queries for classic histograms can read native histograms
// with custom buckets (NHCB).
package histogramconv

import (
	"context"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

// NHClassicCompatQuerier wraps a storage.Querier and makes native and classic
// histograms interchangeable in queries, e.g. while migrating from one to the
// other:
//
//   - A query for classic histogram series (a metric name with a _bucket,
//     _count or _sum suffix) also returns the classic series converted from the
//     native histograms of the base metric name, both native histograms with
//     custom buckets (NHCB) and with an exponential schema. See
//     NHCBAsClassicQuerier and histogram.ConvertExponentialToClassic.
//   - Any other query also returns the NHCB assembled from the classic
//     histogram series of the queried metric name, see ClassicAsNHCBQuerier.
//
// Every Select is handled by exactly one of the two conversions, and both of
// them wrap the original querier. Series converted in one direction are hence
// never converted back, which is what would happen if NHCBAsClassicQuerier
// wrapped ClassicAsNHCBQuerier: every stored classic series would be returned
// a second time, converted to NHCB and back.
//
// The limitations of both conversions apply, most notably the converted series
// are returned in addition to the stored ones, even if both exist for the same
// label set and timestamp.
type NHClassicCompatQuerier struct {
	storage.Querier

	nativeAsClassic storage.Querier
	classicAsNative storage.Querier
}

// NewNHClassicCompatQuerier returns a new querier that wraps the given querier
// and converts native histograms to classic histograms and vice versa, see
// NHClassicCompatQuerier.
func NewNHClassicCompatQuerier(q storage.Querier) storage.Querier {
	return &NHClassicCompatQuerier{
		Querier:         q,
		nativeAsClassic: &NHCBAsClassicQuerier{Querier: q, includeExponential: true},
		classicAsNative: &ClassicAsNHCBQuerier{Querier: q},
	}
}

// Select implements the storage.Querier interface.
func (q *NHClassicCompatQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	if _, suffix, _ := extractHistogramSuffix(matchers); suffix != "" {
		return q.nativeAsClassic.Select(ctx, sortSeries, hints, matchers...)
	}
	return q.classicAsNative.Select(ctx, sortSeries, hints, matchers...)
}

// NHClassicCompatStorage wraps a storage.Storage and applies the conversions of
// NHClassicCompatQuerier to its queriers.
type NHClassicCompatStorage struct {
	storage.Storage
}

// NewNHClassicCompatStorage returns a new storage that wraps the given storage
// and converts native histograms to classic histograms and vice versa in its
// queriers, see NHClassicCompatQuerier.
func NewNHClassicCompatStorage(s storage.Storage) storage.Storage {
	return &NHClassicCompatStorage{Storage: s}
}

// Querier implements the storage.Storage interface.
func (s *NHClassicCompatStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return NewNHClassicCompatQuerier(q), nil
}
