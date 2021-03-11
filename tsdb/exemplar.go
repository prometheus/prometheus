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

package tsdb

import (
	"context"
	"sort"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
)

type CircularExemplarStorage struct {
	outOfOrderExemplars prometheus.Counter

	lock      sync.RWMutex
	exemplars []*circularBufferEntry
	nextIndex int

	// Map of series labels as a string to index entry, which points to the first
	// and last exemplar for the series in the exemplars circular buffer.
	index map[string]*indexEntry
}

type indexEntry struct {
	first int
	last  int
}

type circularBufferEntry struct {
	exemplar     exemplar.Exemplar
	seriesLabels labels.Labels
	next         int
}

// If we assume the average case 95 bytes per exemplar we can fit 5651272 exemplars in
// 1GB of extra memory, accounting for the fact that this is heap allocated space.
// If len < 1, then the exemplar storage is disabled.
func NewCircularExemplarStorage(len int, reg prometheus.Registerer) (ExemplarStorage, error) {
	if len < 1 {
		return &noopExemplarStorage{}, nil
	}
	c := &CircularExemplarStorage{
		exemplars: make([]*circularBufferEntry, len),
		index:     make(map[string]*indexEntry),
		outOfOrderExemplars: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_exemplar_out_of_order_exemplars_total",
			Help: "Total number of out of order exemplar ingestion failed attempts",
		}),
	}

	if reg != nil {
		reg.MustRegister(c.outOfOrderExemplars)
	}

	return c, nil
}

func (ce *CircularExemplarStorage) Appender() *CircularExemplarStorage {
	return ce
}

func (ce *CircularExemplarStorage) ExemplarQuerier(_ context.Context) (storage.ExemplarQuerier, error) {
	return ce, nil
}

func (ce *CircularExemplarStorage) Querier(ctx context.Context) (storage.ExemplarQuerier, error) {
	return ce, nil
}

// Select returns exemplars for a given set of label matchers.
func (ce *CircularExemplarStorage) Select(start, end int64, matchers ...[]*labels.Matcher) ([]exemplar.QueryResult, error) {
	ret := make([]exemplar.QueryResult, 0)

	ce.lock.RLock()
	defer ce.lock.RUnlock()

	// Loop through each index entry, which will point us to first/last exemplar for each series.
	for _, idx := range ce.index {
		var se exemplar.QueryResult
		e := ce.exemplars[idx.first]
		if !matchesSomeMatcherSet(e.seriesLabels, matchers) {
			continue
		}
		se.SeriesLabels = e.seriesLabels

		// Loop through all exemplars in the circular buffer for the current series.
		for e.exemplar.Ts <= end {
			if e.exemplar.Ts >= start {
				se.Exemplars = append(se.Exemplars, e.exemplar)
			}
			if e.next == -1 {
				break
			}
			e = ce.exemplars[e.next]
		}
		if len(se.Exemplars) > 0 {
			ret = append(ret, se)
		}
	}

	sort.Slice(ret, func(i, j int) bool {
		return labels.Compare(ret[i].SeriesLabels, ret[j].SeriesLabels) < 0
	})

	return ret, nil
}

func matchesSomeMatcherSet(lbls labels.Labels, matchers [][]*labels.Matcher) bool {
Outer:
	for _, ms := range matchers {
		for _, m := range ms {
			if !m.Matches(lbls.Get(m.Name)) {
				continue Outer
			}
		}
		return true
	}
	return false
}

// indexGc takes the circularBufferEntry that will be overwritten and updates the
// storages index for that entries labelset if necessary.
func (ce *CircularExemplarStorage) indexGc(cbe *circularBufferEntry) {
	if cbe == nil {
		return
	}

	l := cbe.seriesLabels.String()
	i := cbe.next
	if i == -1 {
		delete(ce.index, l)
		return
	}

	ce.index[l] = &indexEntry{i, ce.index[l].last}
}

func (ce *CircularExemplarStorage) AddExemplar(l labels.Labels, e exemplar.Exemplar) error {
	seriesLabels := l.String()
	ce.lock.Lock()
	defer ce.lock.Unlock()

	idx, ok := ce.index[seriesLabels]
	if !ok {
		ce.indexGc(ce.exemplars[ce.nextIndex])
		// Default the next value to -1 (which we use to detect that we've iterated through all exemplars for a series in Select)
		// since this is the first exemplar stored for this series.
		ce.exemplars[ce.nextIndex] = &circularBufferEntry{
			exemplar:     e,
			seriesLabels: l,
			next:         -1}
		ce.index[seriesLabels] = &indexEntry{ce.nextIndex, ce.nextIndex}
		ce.nextIndex = (ce.nextIndex + 1) % len(ce.exemplars)
		return nil
	}

	// Check for duplicate vs last stored exemplar for this series.
	// NB these are expected, add appending them is a no-op.
	if ce.exemplars[idx.last].exemplar.Equals(e) {
		return nil
	}

	if e.Ts <= ce.exemplars[idx.last].exemplar.Ts {
		ce.outOfOrderExemplars.Inc()
		return storage.ErrOutOfOrderExemplar
	}
	ce.indexGc(ce.exemplars[ce.nextIndex])
	ce.exemplars[ce.nextIndex] = &circularBufferEntry{
		exemplar:     e,
		seriesLabels: l,
		next:         -1,
	}

	ce.exemplars[ce.index[seriesLabels].last].next = ce.nextIndex
	ce.index[seriesLabels].last = ce.nextIndex
	ce.nextIndex = (ce.nextIndex + 1) % len(ce.exemplars)
	return nil
}

type noopExemplarStorage struct{}

func (noopExemplarStorage) AddExemplar(l labels.Labels, e exemplar.Exemplar) error {
	return nil
}

func (noopExemplarStorage) ExemplarQuerier(context.Context) (storage.ExemplarQuerier, error) {
	return &noopExemplarQuerier{}, nil
}

type noopExemplarQuerier struct{}

func (noopExemplarQuerier) Select(_, _ int64, _ ...[]*labels.Matcher) ([]exemplar.QueryResult, error) {
	return nil, nil
}
