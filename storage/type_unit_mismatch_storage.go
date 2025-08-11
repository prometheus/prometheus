// Copyright 2025 The Prometheus Authors
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
	"fmt"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/annotations"
)

// TypeAndUnitMismatchStorage wraps given storage and tracks type and unit
// labels across series. It will produce an info annotation if there
// is a mismatch between type or unit in series with the same name.
func TypeAndUnitMismatchStorage(s Storage) Storage {
	return &typeAndUnitMismatchStorage{
		Storage: s,
	}
}

type typeAndUnitMismatchStorage struct {
	Storage
}

type typeAndUnitMismatchQuerier struct {
	Querier
}

func (s *typeAndUnitMismatchStorage) Querier(mint, maxt int64) (Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &typeAndUnitMismatchQuerier{
		Querier: q,
	}, nil
}

func (q *typeAndUnitMismatchQuerier) Select(ctx context.Context, sort bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	ss := q.Querier.Select(ctx, sort, hints, matchers...)
	return TypeAndUnitMismatchSeriesSet(ss)
}

type typeAndUnitInfo struct {
	typ  string
	unit string
}

func IsTypeEmpty(typ string) bool {
	return typ == "" || typ == "unknown"
}

type typeAndUnitMismatchSeriesSet struct {
	SeriesSet

	prev        *typeAndUnitInfo
	annotations annotations.Annotations
}

func TypeAndUnitMismatchSeriesSet(s SeriesSet) SeriesSet {
	return &typeAndUnitMismatchSeriesSet{
		SeriesSet: s,
	}
}

func (s *typeAndUnitMismatchSeriesSet) At() Series {
	return s.SeriesSet.At()
}

func (s *typeAndUnitMismatchSeriesSet) Next() bool {
	if !s.SeriesSet.Next() {
		return false
	}

	series := s.SeriesSet.At()
	metric := series.Labels().Get(labels.MetricName)
	typ := series.Labels().Get("__type__")
	unit := series.Labels().Get("__unit__")

	if s.prev == nil {
		s.prev = &typeAndUnitInfo{
			typ:  typ,
			unit: unit,
		}
	} else {
		if s.prev.typ != typ {
			if IsTypeEmpty(s.prev.typ) && !IsTypeEmpty(typ) {
				s.prev.typ = typ
			} else if !IsTypeEmpty(s.prev.typ) && !IsTypeEmpty(typ) {
				s.annotations.Add(fmt.Errorf("%w for metric %q", annotations.MismatchedTypeInfo, metric))
			}
		}
		if s.prev.unit != unit {
			s.annotations.Add(fmt.Errorf("%w for metric %q", annotations.MismatchedUnitInfo, metric))
		}
	}

	return true
}

func (s *typeAndUnitMismatchSeriesSet) Warnings() annotations.Annotations {
	got := s.SeriesSet.Warnings()
	if got == nil {
		got = make(annotations.Annotations)
	}
	got.Merge(s.annotations)
	return got
}
