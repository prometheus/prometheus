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

// AnnotatingStorage wraps given storage and tracks type and unit
// labels across series. It will produce an info annotation if there
// is a mismatch between type or unit in series with the same name.
func AnnotatingStorage(s Storage) Storage {
	return &annotatingStorage{
		Storage: s,
	}
}

type annotatingStorage struct {
	Storage
}

type annotatingQuerier struct {
	Querier
}

func (s *annotatingStorage) Querier(mint, maxt int64) (Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &annotatingQuerier{
		Querier: q,
	}, nil
}

func (q *annotatingQuerier) Select(ctx context.Context, sort bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	ss := q.Querier.Select(ctx, sort, hints, matchers...)
	return AnnotatingSeriesSet(ss)
}

type annotatingSeriesSet struct {
	SeriesSet

	seen map[string]struct {
		Type string
		Unit string
	}
	annotations annotations.Annotations
}

func AnnotatingSeriesSet(s SeriesSet) SeriesSet {
	return &annotatingSeriesSet{
		SeriesSet: s,
		seen: make(map[string]struct {
			Type string
			Unit string
		}),
	}
}

func (s *annotatingSeriesSet) At() Series {
	return s.SeriesSet.At()
}

func (s *annotatingSeriesSet) Next() bool {
	if !s.SeriesSet.Next() {
		return false
	}

	series := s.SeriesSet.At()
	metric := series.Labels().Get(labels.MetricName)
	mType := series.Labels().Get("__type__")
	mUnit := series.Labels().Get("__unit__")

	if prev, ok := s.seen[metric]; ok {
		if prev.Type != mType || prev.Unit != mUnit {
			if prev.Type == "unknown" || mType == "unknown" {
				if prev.Type == "unknown" && mType != "unknown" {
					s.seen[metric] = struct {
						Type string
						Unit string
					}{Type: mType, Unit: mUnit}
				}
			} else {
				s.annotations.Add(fmt.Errorf("%w %q", annotations.MismatchedTypeOrUnitInfo, metric))
			}
		}
	} else {
		s.seen[metric] = struct {
			Type string
			Unit string
		}{Type: mType, Unit: mUnit}
	}
	return true
}

func (s *annotatingSeriesSet) Warnings() annotations.Annotations {
	got := s.SeriesSet.Warnings()
	if got == nil {
		got = make(annotations.Annotations)
	}
	got.Merge(s.annotations)
	return got
}
