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
	"sync"

	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
)

type CircularExemplarStorage struct {
	lock        sync.RWMutex
	index       map[string]int
	exemplars   []*circularBufferEntry
	nextIndex   int
	len         int
	secondaries []storage.ExemplarAppender
}

type circularBufferEntry struct {
	se   storageExemplar
	prev int // index of previous exemplar in circular for the same series, use -1 as a default for new entries
}

func (ce circularBufferEntry) Exemplar() exemplar.Exemplar {
	return ce.se.exemplar
}

type storageExemplar struct {
	exemplar        exemplar.Exemplar
	seriesLabels    labels.Labels
	scrapeTimestamp int64
}

// If we assume the average case 95 bytes per exemplar we can fit 5651272 exemplars in
// 1GB of extra memory, accounting for the fact that this is heap allocated space.
func NewCircularExemplarStorage(len int, secondaries ...storage.ExemplarAppender) *CircularExemplarStorage {
	return &CircularExemplarStorage{
		exemplars:   make([]*circularBufferEntry, len),
		index:       make(map[string]int),
		len:         len,
		secondaries: secondaries,
	}
}

func (ce *CircularExemplarStorage) Appender() storage.ExemplarAppender {
	return ce
}

// TODO: separate wrapper struct for queries?
func (ce *CircularExemplarStorage) Querier(ctx context.Context) (storage.ExemplarQuerier, error) {
	return ce, nil
}

func (ce *CircularExemplarStorage) SetSecondaries(secondaries ...storage.ExemplarAppender) {
	ce.secondaries = secondaries
}

// Select returns exemplars for a given set of series labels hash.
func (ce *CircularExemplarStorage) Select(start, end int64, l labels.Labels) ([]exemplar.Exemplar, error) {
	var (
		ret []exemplar.Exemplar
		e   exemplar.Exemplar
		idx int
		ok  bool
		buf []byte
	)

	ce.lock.RLock()
	defer ce.lock.RUnlock()

	if idx, ok = ce.index[l.String()]; !ok {
		return nil, nil
	}
	lastTs := ce.exemplars[idx].se.scrapeTimestamp

	for {
		// We need the labels check here in case what was the previous exemplar for the series
		// when the exemplar from the last loop iteration was written has since been overwritten
		// with an exemplar from another series.
		// todo (callum) confirm if this check is still needed now that adding an exemplar should
		// update the index and previous pointer for the series whose exemplar was overwritten.
		if idx == -1 || string(ce.exemplars[idx].se.seriesLabels.Bytes(buf)) != string(l.Bytes(buf)) {
			break
		}

		e = ce.exemplars[idx].Exemplar()
		// todo (callum) This line is needed to avoid an infinite loop, consider redesign of buffer entry struct.
		if ce.exemplars[idx].se.scrapeTimestamp > lastTs {
			break
		}

		lastTs = ce.exemplars[idx].se.scrapeTimestamp
		// Prepend since this exemplar came before the last one we appeneded chronologically.
		if e.Ts >= start && e.Ts <= end {
			ret = append([]exemplar.Exemplar{e}, ret...)
		}
		idx = ce.exemplars[idx].prev
	}
	return ret, nil
}

// Takes the circularBufferEntry that will be overwritten and updates the
// storages index for that entries labelset if necessary.
func (ce *CircularExemplarStorage) indexGcCheck(cbe *circularBufferEntry) {
	if cbe == nil {
		return
	}

	l := cbe.se.seriesLabels
	i := cbe.prev
	if cbe.prev == -1 {
		delete(ce.index, l.String())
		return
	}

	if ce.exemplars[ce.nextIndex] != nil {
		l2 := ce.exemplars[i].se.seriesLabels
		if !labels.Equal(l2, l) { // No more exemplars for series l.
			delete(ce.index, cbe.se.seriesLabels.String())
			return
		}
		// There's still at least one exemplar for the series l, so we can update the index.
		ce.index[l.String()] = i
	}
}

func (ce *CircularExemplarStorage) addExemplar(l labels.Labels, t int64, e exemplar.Exemplar) error {
	seriesLabels := l.String()
	ce.lock.RLock()
	idx, ok := ce.index[seriesLabels]
	ce.lock.RUnlock()

	ce.lock.Lock()
	defer ce.lock.Unlock()

	if !ok {
		ce.indexGcCheck(ce.exemplars[ce.nextIndex])
		// Default the prev value to -1 (which we use to detect that we've iterated through all exemplars for a series in Select)
		// since this is the first exemplar stored for this series.
		ce.exemplars[ce.nextIndex] = &circularBufferEntry{
			se: storageExemplar{
				exemplar:     e,
				seriesLabels: l,
			},
			prev: -1}
		ce.index[seriesLabels] = ce.nextIndex
		ce.nextIndex++
		if ce.nextIndex >= cap(ce.exemplars) {
			ce.nextIndex = 0
		}
		return nil
	}

	// Check for duplicate vs last stored exemplar for this series.
	if ce.exemplars[idx].Exemplar().Equals(e) {
		return storage.ErrDuplicateExemplar
	}
	if e.Ts <= ce.exemplars[idx].se.scrapeTimestamp || t <= ce.exemplars[idx].se.scrapeTimestamp {
		return storage.ErrOutOfOrderExemplar
	}
	ce.indexGcCheck(ce.exemplars[ce.nextIndex])
	ce.exemplars[ce.nextIndex] = &circularBufferEntry{
		se: storageExemplar{
			exemplar:        e,
			seriesLabels:    l,
			scrapeTimestamp: t,
		},
		prev: idx}
	ce.index[seriesLabels] = ce.nextIndex
	ce.nextIndex++
	if ce.nextIndex >= cap(ce.exemplars) {
		ce.nextIndex = 0
	}
	return nil
}

func (ce *CircularExemplarStorage) AddExemplar(l labels.Labels, t int64, e exemplar.Exemplar) error {
	if err := ce.addExemplar(l, t, e); err != nil {
		return err
	}

	for _, s := range ce.secondaries {
		if err := s.AddExemplar(l, t, e); err != nil {
			return err
		}
	}
	return nil
}

// For use in tests, clears the entire exemplar storage.
func (ce *CircularExemplarStorage) Reset() {
	ce.exemplars = make([]*circularBufferEntry, ce.len)
	ce.index = make(map[string]int)
}
