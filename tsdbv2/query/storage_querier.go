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

package query

import (
	"context"

	"github.com/cockroachdb/pebble"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	ql "github.com/prometheus/prometheus/tsdbv2/query/labels"
	"github.com/prometheus/prometheus/tsdbv2/query/series"
	"github.com/prometheus/prometheus/tsdbv2/tenants"
	"github.com/prometheus/prometheus/util/annotations"
)

type Series struct {
	source     tenants.Sounce
	db         *pebble.DB
	mint, maxt int64
}

var _ storage.Querier = (*Series)(nil)

func NewSeries(db *pebble.DB, source tenants.Sounce, mint, maxt int64) *Series {
	return &Series{db: db, source: source, mint: mint, maxt: maxt}
}

func (q *Series) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	if hints == nil {
		hints = &storage.SelectHints{Start: q.mint, End: q.maxt}
	}
	return series.Select(ctx, q.db, q.source, sortSeries, hints, matchers...)
}

func (q *Series) Close() error { return nil }

func (q *Series) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return ql.LabelValues(ctx, q.db, q.source, name, hints, matchers...)
}

func (q *Series) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return ql.LabelNames(ctx, q.db, q.source, hints, matchers...)
}
