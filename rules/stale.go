// Copyright 2023 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rules

import (
	"github.com/prometheus/prometheus/model/labels"
)

type StaleSeriesRepo interface {
	Clear()
	InitCurrent(length int)
	AddToCurrent(ls labels.Labels)
	ForEachStale(ruleId int, fn func(lset labels.Labels))
	MoveCurrentToPrevious(ruleId int)
	GetNumberOfRules() int
	GetAll(ruleId int) []labels.Labels
	CopyFrom(ruleId int, other StaleSeriesRepo, otherRuleId int)
}

type StaleSeriesRepoFactory func(numberOfRules int) StaleSeriesRepo

func DefaultStaleSeriesRepoFactory(numberOfRules int) StaleSeriesRepo {
	return NewDefaultStaleSeriesRepo(numberOfRules)
}

type DefaultStaleSeriesRepo struct {
	seriesReturned       map[string]bool
	seriesInPreviousEval []map[string]bool
	numberOfRules        int
}

func NewDefaultStaleSeriesRepo(numberOfRules int) *DefaultStaleSeriesRepo {
	return &DefaultStaleSeriesRepo{
		seriesInPreviousEval: make([]map[string]bool, numberOfRules),
		numberOfRules:        numberOfRules,
	}
}

func (r *DefaultStaleSeriesRepo) InitCurrent(length int) {
	r.seriesReturned = make(map[string]bool, length)
}

func (r *DefaultStaleSeriesRepo) AddToCurrent(ls labels.Labels) {
	buf := [1024]byte{}
	r.seriesReturned[string(ls.Bytes(buf[:]))] = true
}

func (r *DefaultStaleSeriesRepo) Clear() {
	r.seriesInPreviousEval = nil
	r.seriesReturned = nil
}

func (r *DefaultStaleSeriesRepo) ForEachStale(ruleId int, fn func(lset labels.Labels)) {
	for metric := range r.seriesInPreviousEval[ruleId] {
		if _, ok := r.seriesReturned[metric]; !ok {
			lset, err := labels.FromBytes([]byte(metric))
			if err == nil {
				fn(*lset)
			}
		}
	}
}

func (r *DefaultStaleSeriesRepo) GetAll(ruleId int) []labels.Labels {
	ls := make([]labels.Labels, 0, len(r.seriesInPreviousEval[ruleId]))
	for metric := range r.seriesInPreviousEval[ruleId] {
		lset, err := labels.FromBytes([]byte(metric))
		if err == nil {
			ls = append(ls, *lset)
		}
	}
	return ls
}

func (r *DefaultStaleSeriesRepo) MoveCurrentToPrevious(ruleId int) {
	r.seriesInPreviousEval[ruleId] = r.seriesReturned
	r.seriesReturned = nil
}

func (r *DefaultStaleSeriesRepo) KeyCount(ruleId int) int {
	if r.seriesInPreviousEval[ruleId] == nil {
		return 0
	}
	return len(r.seriesInPreviousEval[ruleId])
}

func (r *DefaultStaleSeriesRepo) CopyFrom(ruleId int, other StaleSeriesRepo, otherRuleId int) {
	r.seriesInPreviousEval[ruleId] = other.(*DefaultStaleSeriesRepo).seriesInPreviousEval[otherRuleId]
}

func (r *DefaultStaleSeriesRepo) GetNumberOfRules() int {
	return r.numberOfRules
}
