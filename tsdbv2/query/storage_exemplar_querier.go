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
	"github.com/cockroachdb/pebble"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdbv2/query/series"
	"github.com/prometheus/prometheus/tsdbv2/tenants"
)

type Exemplar struct {
	db     *pebble.DB
	source tenants.Sounce
}

var _ storage.ExemplarQuerier = (*Exemplar)(nil)

func NewExemplar(db *pebble.DB, source tenants.Sounce) *Exemplar {
	return &Exemplar{db: db, source: source}
}

func (e *Exemplar) Select(start, end int64, matchers ...[]*labels.Matcher) ([]exemplar.QueryResult, error) {
	return series.Exemplar(e.db, e.source, start, end, matchers...)
}
