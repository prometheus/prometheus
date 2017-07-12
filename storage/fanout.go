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
// limitations under the License.package remote

package storage

import (
	"container/heap"
	"strings"

	"github.com/prometheus/prometheus/pkg/labels"
)

type fanout struct {
	storages []Storage
}

// NewFanout returns a new fan-out Storage, which proxies reads and writes
// through to multiple underlying storages.
func NewFanout(storages ...Storage) Storage {
	return &fanout{
		storages: storages,
	}
}

func (f *fanout) Querier(mint, maxt int64) (Querier, error) {
	queriers := mergeQuerier{
		queriers: make([]Querier, 0, len(f.storages)),
	}
	for _, storage := range f.storages {
		querier, err := storage.Querier(mint, maxt)
		if err != nil {
			queriers.Close()
			return nil, err
		}
		queriers.queriers = append(queriers.queriers, querier)
	}
	return &queriers, nil
}

func (f *fanout) Appender() (Appender, error) {
	appenders := make([]Appender, 0, len(f.storages))
	for _, storage := range f.storages {
		appender, err := storage.Appender()
		if err != nil {
			return nil, err
		}
		appenders = append(appenders, appender)
	}
	return &fanoutAppender{
		appenders: appenders,
	}, nil
}

// Close closes the storage and all its underlying resources.
func (f *fanout) Close() error {
	// TODO return multiple errors?
	var lastErr error
	for _, storage := range f.storages {
		if err := storage.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// fanoutAppender implements Appender.
type fanoutAppender struct {
	appenders []Appender
}

func (f *fanoutAppender) Add(l labels.Labels, t int64, v float64) (string, error) {
	for _, appender := range f.appenders {
		if _, err := appender.Add(l, t, v); err != nil {
			return "", err
		}
	}
	return "", nil
}

func (f *fanoutAppender) AddFast(l labels.Labels, ref string, t int64, v float64) error {
	for _, appender := range f.appenders {
		if err := appender.AddFast(l, ref, t, v); err != nil {
			return err
		}
	}
	return nil
}

func (f *fanoutAppender) Commit() error {
	for _, appender := range f.appenders {
		if err := appender.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (f *fanoutAppender) Rollback() error {
	for _, appender := range f.appenders {
		if err := appender.Rollback(); err != nil {
			return err
		}
	}
	return nil
}

// mergeQuerier implements Querier.
type mergeQuerier struct {
	queriers []Querier
}

func NewMergeQuerier(queriers []Querier) Querier {
	return &mergeQuerier{
		queriers: queriers,
	}
}

// Select returns a set of series that matches the given label matchers.
func (q *mergeQuerier) Select(matchers ...*labels.Matcher) SeriesSet {
	seriesSets := make([]SeriesSet, 0, len(q.queriers))
	for _, querier := range q.queriers {
		seriesSets = append(seriesSets, querier.Select(matchers...))
	}
	return newMergeSeriesSet(seriesSets)
}

// LabelValues returns all potential values for a label name.
func (q *mergeQuerier) LabelValues(name string) ([]string, error) {
	var results [][]string
	for _, querier := range q.queriers {
		values, err := querier.LabelValues(name)
		if err != nil {
			return nil, err
		}
		results = append(results, values)
	}
	return mergeStringSlices(results), nil
}

func mergeStringSlices(ss [][]string) []string {
	switch len(ss) {
	case 0:
		return nil
	case 1:
		return ss[0]
	case 2:
		return mergeTwoStringSlices(ss[0], ss[1])
	default:
		halfway := len(ss) / 2
		return mergeTwoStringSlices(
			mergeStringSlices(ss[:halfway]),
			mergeStringSlices(ss[halfway:]),
		)
	}
}

func mergeTwoStringSlices(a, b []string) []string {
	i, j := 0, 0
	result := make([]string, 0, len(a)+len(b))
	for i < len(a) && j < len(b) {
		switch strings.Compare(a[i], b[j]) {
		case 0:
			result = append(result, a[i])
			i++
			j++
		case 1:
			result = append(result, a[i])
			i++
		case -1:
			result = append(result, b[j])
			j++
		}
	}
	copy(result, a[i:])
	copy(result, b[j:])
	return result
}

// Close releases the resources of the Querier.
func (q *mergeQuerier) Close() error {
	// TODO return multiple errors?
	var lastErr error
	for _, querier := range q.queriers {
		if err := querier.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// mergeSeriesSet implements SeriesSet
type mergeSeriesSet struct {
	currentLabels labels.Labels
	currentSets   []SeriesSet
	sets          seriesSetHeap
}

func newMergeSeriesSet(sets []SeriesSet) SeriesSet {
	// Sets need to be pre-advanced, so we can introspect the label of the
	// series under the cursor.
	var h seriesSetHeap
	for _, set := range sets {
		if set.Next() {
			heap.Push(&h, set)
		}
	}
	return &mergeSeriesSet{
		sets: h,
	}
}

func (c *mergeSeriesSet) Next() bool {
	// Firstly advance all the current series sets.  If any of them have run out
	// we can drop them, otherwise they should be inserted back into the heap.
	for _, set := range c.currentSets {
		if set.Next() {
			heap.Push(&c.sets, set)
		}
	}
	if len(c.sets) == 0 {
		return false
	}

	// Now, pop items of the heap that have equal label sets.
	c.currentSets = nil
	c.currentLabels = c.sets[0].At().Labels()
	for len(c.sets) > 0 && labels.Equal(c.currentLabels, c.sets[0].At().Labels()) {
		set := heap.Pop(&c.sets).(SeriesSet)
		c.currentSets = append(c.currentSets, set)
	}
	return true
}

func (c *mergeSeriesSet) At() Series {
	series := []Series{}
	for _, seriesSet := range c.currentSets {
		series = append(series, seriesSet.At())
	}
	return &mergeSeries{
		labels: c.currentLabels,
		series: series,
	}
}

func (c *mergeSeriesSet) Err() error {
	for _, set := range c.sets {
		if err := set.Err(); err != nil {
			return err
		}
	}
	return nil
}

type seriesSetHeap []SeriesSet

func (h seriesSetHeap) Len() int      { return len(h) }
func (h seriesSetHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h seriesSetHeap) Less(i, j int) bool {
	a, b := h[i].At().Labels(), h[j].At().Labels()
	return labels.Compare(a, b) < 0
}

func (h *seriesSetHeap) Push(x interface{}) {
	*h = append(*h, x.(SeriesSet))
}

func (h *seriesSetHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type mergeSeries struct {
	labels labels.Labels
	series []Series
}

func (m *mergeSeries) Labels() labels.Labels {
	return m.labels
}

func (m *mergeSeries) Iterator() SeriesIterator {
	iterators := make([]SeriesIterator, 0, len(m.series))
	for _, s := range m.series {
		iterators = append(iterators, s.Iterator())
	}
	return &mergeIterator{
		iterators: iterators,
	}
}

type mergeIterator struct {
	iterators []SeriesIterator
	h         seriesIteratorHeap
}

func newMergeIterator(iterators []SeriesIterator) SeriesIterator {
	return &mergeIterator{
		iterators: iterators,
		h:         nil,
	}
}

func (c *mergeIterator) Seek(t int64) bool {
	c.h = seriesIteratorHeap{}
	for _, iter := range c.iterators {
		if iter.Seek(t) {
			heap.Push(&c.h, iter)
		}
	}
	return len(c.h) > 0
}

func (c *mergeIterator) At() (t int64, v float64) {
	// TODO do I need to dedupe or just merge?
	return c.h[0].At()
}

func (c *mergeIterator) Next() bool {
	// Detect the case where Next is called before At
	if c.h == nil {
		panic("Next() called before Seek()")
	}

	if len(c.h) == 0 {
		return false
	}
	iter := heap.Pop(&c.h).(SeriesIterator)
	if iter.Next() {
		heap.Push(&c.h, iter)
	}
	return len(c.h) > 0
}

func (c *mergeIterator) Err() error {
	for _, iter := range c.iterators {
		if err := iter.Err(); err != nil {
			return err
		}
	}
	return nil
}

type seriesIteratorHeap []SeriesIterator

func (h seriesIteratorHeap) Len() int      { return len(h) }
func (h seriesIteratorHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h seriesIteratorHeap) Less(i, j int) bool {
	at, _ := h[i].At()
	bt, _ := h[j].At()
	return at < bt
}

func (h *seriesIteratorHeap) Push(x interface{}) {
	*h = append(*h, x.(SeriesIterator))
}

func (h *seriesIteratorHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
