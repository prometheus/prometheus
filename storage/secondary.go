// Copyright 2017 The Prometheus Authors
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
	"sync"

	"github.com/prometheus/prometheus/pkg/labels"
)

// secondaryQuerier is a wrapper that allows a querier to be treated in a best effort manner.
// This means that an error on any method returned by Querier except Close will be returned as a warning,
// and the result will be empty.
//
// Additionally, Querier ensures that if ANY SeriesSet returned by this querier's Select failed on an initial Next,
// All other SeriesSet will be return no response as well. This ensures consistent partial response strategy, where you
// have either full results or none from each secondary Querier.
// NOTE: This works well only for implementations that only fail during first Next() (e.g fetch from network). If implementation fails
// during further iterations, set will panic. If Select is invoked after first Next of any returned SeriesSet, querier will panic.
//
// Not go-routine safe.
// NOTE: Prometheus treats all remote storages as secondary / best effort.
type secondaryQuerier struct {
	genericQuerier

	once      sync.Once
	done      bool
	asyncSets []genericSeriesSet
}

func newSecondaryQuerierFrom(q Querier) genericQuerier {
	return &secondaryQuerier{genericQuerier: newGenericQuerierFrom(q)}
}

func newSecondaryQuerierFromChunk(cq ChunkQuerier) genericQuerier {
	return &secondaryQuerier{genericQuerier: newGenericQuerierFromChunk(cq)}
}

func (s *secondaryQuerier) LabelValues(name string) ([]string, Warnings, error) {
	vals, w, err := s.genericQuerier.LabelValues(name)
	if err != nil {
		return nil, append([]error{err}, w...), nil
	}
	return vals, w, nil
}

func (s *secondaryQuerier) LabelNames() ([]string, Warnings, error) {
	names, w, err := s.genericQuerier.LabelNames()
	if err != nil {
		return nil, append([]error{err}, w...), nil
	}
	return names, w, nil
}

func (s *secondaryQuerier) createFn(asyncSet genericSeriesSet) func() (genericSeriesSet, bool) {
	s.asyncSets = append(s.asyncSets, asyncSet)
	curr := len(s.asyncSets) - 1
	return func() (genericSeriesSet, bool) {
		s.once.Do(func() {
			// At first create invocation we iterate over all sets and ensure its Next() returns some value without
			// errors. This is to ensure we support consistent partial failures.
			for i, set := range s.asyncSets {
				if set.Next() {
					continue
				}
				ws := set.Warnings()
				// Failed set.
				if err := set.Err(); err != nil {
					ws = append([]error{err}, ws...)
					// Promote the warnings to the current one.
					s.asyncSets[curr] = warningsOnlySeriesSet(ws)
					// One of the sets failed, ensure rest of the sets returns nothing. (All or nothing logic).
					for i := range s.asyncSets {
						if curr != i {
							s.asyncSets[i] = noopGenericSeriesSet{}
						}
					}
					break
				}
				// Exhausted set.
				s.asyncSets[i] = warningsOnlySeriesSet(ws)
			}

			s.done = true
		})

		switch s.asyncSets[curr].(type) {
		case warningsOnlySeriesSet, noopGenericSeriesSet:
			return s.asyncSets[curr], false
		default:
			return s.asyncSets[curr], true
		}
	}
}

func (s *secondaryQuerier) Select(sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) genericSeriesSet {
	if s.done {
		panic("secondaryQuerier: Select invoked after first Next of any returned SeriesSet was done")
	}
	return &lazySeriesSet{create: s.createFn(s.genericQuerier.Select(sortSeries, hints, matchers...))}
}
