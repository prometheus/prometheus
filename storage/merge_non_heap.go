// Copyright 2020 The Prometheus Authors
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
	"github.com/prometheus/prometheus/pkg/labels"
)

// genericMergeSeriesSet implements genericSeriesSet.
type genericMergeSeriesSet struct {
	all  []genericSeriesSet
	buf  []genericSeriesSet // A buffer for keeping the order of genericSeriesSet slice during forwarding the genericSeriesSet.
	ids  []int              // The indices of chosen genericSeriesSet for the current run.
	done bool
	err  error
	cur  Labels
	ws   Warnings

	mergeFunc genericSeriesMergeFunc
}

// newGenericMergeSeriesSet returns a new genericSeriesSet that merges (and deduplicates)
// series returned by the series sets when iterating.
// Each series set must return its series in labels order, otherwise
// merged series set will be incorrect.
// Overlapped situations are merged using provided mergeFunc.
func newGenericMergeSeriesSet(sets []genericSeriesSet, mergeFunc genericSeriesMergeFunc) genericSeriesSet {
	if len(sets) == 1 {
		return sets[0]
	}

	s := &genericMergeSeriesSet{all: sets, mergeFunc: mergeFunc}
	// Initialize first elements of all sets as Next() needs one element look-ahead.
	s.nextAll()
	if len(s.all) == 0 {
		s.done = true
	}

	return s
}

func (s *genericMergeSeriesSet) At() Labels {
	return s.cur
}

func (s *genericMergeSeriesSet) Err() error {
	return s.err
}

func (s *genericMergeSeriesSet) Warnings() Warnings {
	var ws Warnings
	if len(s.ws) > 0 {
		ws = append(make(Warnings, 0, len(s.ws)), s.ws...)
	}
	for _, set := range s.all {
		ws = append(ws, set.Warnings()...)
	}
	return ws
}

// nextAll is to call Next() for all genericSeriesSet.
// Because the order of the genericSeriesSet slice will affect the results,
// we need to use an buffer slice to hold the order.
func (s *genericMergeSeriesSet) nextAll() {
	s.buf = s.buf[:0]
	for _, ss := range s.all {
		if ss.Next() {
			s.buf = append(s.buf, ss)
			continue
		}
		s.ws = append(s.ws, ss.Warnings()...)
		if ss.Err() != nil {
			s.done = true
			s.err = ss.Err()
			break
		}
	}
	s.all, s.buf = s.buf, s.all
}

// nextWithID is to call Next() for the genericSeriesSet with the indices of s.ids.
// Because the order of the genericSeriesSet slice will affect the results,
// we need to use an buffer slice to hold the order.
func (s *genericMergeSeriesSet) nextWithID() {
	if len(s.ids) == 0 {
		return
	}

	s.buf = s.buf[:0]
	i1 := 0
	i2 := 0
	for i1 < len(s.all) {
		if i2 < len(s.ids) && i1 == s.ids[i2] {
			if !s.all[s.ids[i2]].Next() {
				s.ws = append(s.ws, s.all[s.ids[i2]].Warnings()...)
				if s.all[s.ids[i2]].Err() != nil {
					s.done = true
					s.err = s.all[s.ids[i2]].Err()
					break
				}
				i2++
				i1++
				continue
			}
			i2++
		}
		s.buf = append(s.buf, s.all[i1])
		i1++
	}
	s.all, s.buf = s.buf, s.all
}

func (s *genericMergeSeriesSet) Next() bool {
	if s.done {
		return false
	}

	s.nextWithID()
	if s.done {
		return false
	}
	s.ids = s.ids[:0]
	if len(s.all) == 0 {
		s.done = true
		return false
	}

	// Here we are looking for a set of series sets with the lowest labels,
	// and we will cache their indexes in s.ids.
	s.ids = append(s.ids, 0)
	for i := 1; i < len(s.all); i++ {
		cmp := labels.Compare(s.all[s.ids[0]].At().Labels(), s.all[i].At().Labels())
		if cmp > 0 {
			s.ids = s.ids[:1]
			s.ids[0] = i
		} else if cmp == 0 {
			s.ids = append(s.ids, i)
		}
	}

	if len(s.ids) > 1 {
		overlapping := make([]Labels, len(s.ids))
		for i, idx := range s.ids {
			overlapping[i] = s.all[idx].At()
		}
		s.cur = s.mergeFunc(overlapping...)
		return true
	}

	s.cur = s.all[s.ids[0]].At()
	return true
}
