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
	"errors"
	"slices"
	"sync"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

const (
	// Indicates that there is no index entry for an exmplar.
	noExemplar = -1
	// Estimated number of exemplars per series, for sizing the index.
	estimatedExemplarsPerSeries = 16
)

type CircularExemplarStorage struct {
	lock      sync.RWMutex
	exemplars []*circularBufferEntry
	nextIndex int
	metrics   *ExemplarMetrics

	// Map of series labels as a string to index entry, which points to the first
	// and last exemplar for the series in the exemplars circular buffer.
	index map[string]*indexEntry
}

type indexEntry struct {
	oldest       int
	newest       int
	seriesLabels labels.Labels
}

type circularBufferEntry struct {
	exemplar exemplar.Exemplar
	next     int
	ref      *indexEntry
}

type ExemplarMetrics struct {
	exemplarsAppended            prometheus.Counter
	exemplarsInStorage           prometheus.Gauge
	seriesWithExemplarsInStorage prometheus.Gauge
	lastExemplarsTs              prometheus.Gauge
	maxExemplars                 prometheus.Gauge
	outOfOrderExemplars          prometheus.Counter
}

func NewExemplarMetrics(reg prometheus.Registerer) *ExemplarMetrics {
	m := ExemplarMetrics{
		exemplarsAppended: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_exemplar_exemplars_appended_total",
			Help: "Total number of appended exemplars.",
		}),
		exemplarsInStorage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_exemplars_in_storage",
			Help: "Number of exemplars currently in circular storage.",
		}),
		seriesWithExemplarsInStorage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_series_with_exemplars_in_storage",
			Help: "Number of series with exemplars currently in circular storage.",
		}),
		lastExemplarsTs: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_last_exemplars_timestamp_seconds",
			Help: "The timestamp of the oldest exemplar stored in circular storage. Useful to check for what time" +
				"range the current exemplar buffer limit allows. This usually means the last timestamp" +
				"for all exemplars for a typical setup. This is not true though if one of the series timestamp is in future compared to rest series.",
		}),
		outOfOrderExemplars: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_exemplar_out_of_order_exemplars_total",
			Help: "Total number of out of order exemplar ingestion failed attempts.",
		}),
		maxExemplars: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_max_exemplars",
			Help: "Total number of exemplars the exemplar storage can store, resizeable.",
		}),
	}

	if reg != nil {
		reg.MustRegister(
			m.exemplarsAppended,
			m.exemplarsInStorage,
			m.seriesWithExemplarsInStorage,
			m.lastExemplarsTs,
			m.outOfOrderExemplars,
			m.maxExemplars,
		)
	}

	return &m
}

// NewCircularExemplarStorage creates an circular in memory exemplar storage.
// If we assume the average case 95 bytes per exemplar we can fit 5651272 exemplars in
// 1GB of extra memory, accounting for the fact that this is heap allocated space.
// If len <= 0, then the exemplar storage is essentially a noop storage but can later be
// resized to store exemplars.
func NewCircularExemplarStorage(length int64, m *ExemplarMetrics) (ExemplarStorage, error) {
	if length < 0 {
		length = 0
	}
	c := &CircularExemplarStorage{
		exemplars: make([]*circularBufferEntry, length),
		index:     make(map[string]*indexEntry, length/estimatedExemplarsPerSeries),
		metrics:   m,
	}

	c.metrics.maxExemplars.Set(float64(length))

	return c, nil
}

func (ce *CircularExemplarStorage) ApplyConfig(cfg *config.Config) error {
	ce.Resize(cfg.StorageConfig.ExemplarsConfig.MaxExemplars)
	return nil
}

func (ce *CircularExemplarStorage) Appender() *CircularExemplarStorage {
	return ce
}

func (ce *CircularExemplarStorage) ExemplarQuerier(_ context.Context) (storage.ExemplarQuerier, error) {
	return ce, nil
}

func (ce *CircularExemplarStorage) Querier(_ context.Context) (storage.ExemplarQuerier, error) {
	return ce, nil
}

// Select returns exemplars for a given set of label matchers.
func (ce *CircularExemplarStorage) Select(start, end int64, matchers ...[]*labels.Matcher) ([]exemplar.QueryResult, error) {
	ret := make([]exemplar.QueryResult, 0)

	if len(ce.exemplars) == 0 {
		return ret, nil
	}

	ce.lock.RLock()
	defer ce.lock.RUnlock()

	// Loop through each index entry, which will point us to first/last exemplar for each series.
	for _, idx := range ce.index {
		var se exemplar.QueryResult
		e := ce.exemplars[idx.oldest]
		if e.exemplar.Ts > end || ce.exemplars[idx.newest].exemplar.Ts < start {
			continue
		}
		if !matchesSomeMatcherSet(idx.seriesLabels, matchers) {
			continue
		}
		se.SeriesLabels = idx.seriesLabels

		// Loop through all exemplars in the circular buffer for the current series.
		for e.exemplar.Ts <= end {
			if e.exemplar.Ts >= start {
				se.Exemplars = append(se.Exemplars, e.exemplar)
			}
			if e.next == noExemplar {
				break
			}
			e = ce.exemplars[e.next]
		}
		if len(se.Exemplars) > 0 {
			ret = append(ret, se)
		}
	}

	slices.SortFunc(ret, func(a, b exemplar.QueryResult) int {
		return labels.Compare(a.SeriesLabels, b.SeriesLabels)
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

func (ce *CircularExemplarStorage) ValidateExemplar(l labels.Labels, e exemplar.Exemplar) error {
	var buf [1024]byte
	seriesLabels := l.Bytes(buf[:])

	// TODO(bwplotka): This lock can lock all scrapers, there might high contention on this on scale.
	// Optimize by moving the lock to be per series (& benchmark it).
	ce.lock.RLock()
	defer ce.lock.RUnlock()
	return ce.validateExemplar(seriesLabels, e, false)
}

// Not thread safe. The appended parameters tells us whether this is an external validation, or internal
// as a result of an AddExemplar call, in which case we should update any relevant metrics.
func (ce *CircularExemplarStorage) validateExemplar(key []byte, e exemplar.Exemplar, appended bool) error {
	if len(ce.exemplars) == 0 {
		return storage.ErrExemplarsDisabled
	}

	// Exemplar label length does not include chars involved in text rendering such as quotes
	// equals sign, or commas. See definition of const ExemplarMaxLabelLength.
	labelSetLen := 0
	if err := e.Labels.Validate(func(l labels.Label) error {
		labelSetLen += utf8.RuneCountInString(l.Name)
		labelSetLen += utf8.RuneCountInString(l.Value)

		if labelSetLen > exemplar.ExemplarMaxLabelSetLength {
			return storage.ErrExemplarLabelLength
		}
		return nil
	}); err != nil {
		return err
	}

	idx, ok := ce.index[string(key)]
	if !ok {
		return nil
	}

	// Check for duplicate vs last stored exemplar for this series.
	// NB these are expected, and appending them is a no-op.
	// For floats and classic histograms, there is only 1 exemplar per series,
	// so this is sufficient. For native histograms with multiple exemplars per series,
	// we have another check below.
	newestExemplar := ce.exemplars[idx.newest].exemplar
	if newestExemplar.Equals(e) {
		return storage.ErrDuplicateExemplar
	}

	// Since during the scrape the exemplars are sorted first by timestamp, then value, then labels,
	// if any of these conditions are true, we know that the exemplar is either a duplicate
	// of a previous one (but not the most recent one as that is checked above) or out of order.
	// We now allow exemplars with duplicate timestamps as long as they have different values and/or labels
	// since that can happen for different buckets of a native histogram.
	// We do not distinguish between duplicates and out of order as iterating through the exemplars
	// to check for that would be expensive (versus just comparing with the most recent one) especially
	// since this is run under a lock, and not worth it as we just need to return an error so we do not
	// append the exemplar.
	if e.Ts < newestExemplar.Ts ||
		(e.Ts == newestExemplar.Ts && e.Value < newestExemplar.Value) ||
		(e.Ts == newestExemplar.Ts && e.Value == newestExemplar.Value && e.Labels.Hash() < newestExemplar.Labels.Hash()) {
		if appended {
			ce.metrics.outOfOrderExemplars.Inc()
		}
		return storage.ErrOutOfOrderExemplar
	}
	return nil
}

// Resize changes the size of exemplar buffer by allocating a new buffer and migrating data to it.
// Exemplars are kept when possible. Shrinking will discard oldest data (in order of ingest) as needed.
func (ce *CircularExemplarStorage) Resize(l int64) int {
	// Accept negative values as just 0 size.
	if l <= 0 {
		l = 0
	}

	if l == int64(len(ce.exemplars)) {
		return 0
	}

	ce.lock.Lock()
	defer ce.lock.Unlock()

	oldBuffer := ce.exemplars
	oldNextIndex := int64(ce.nextIndex)

	ce.exemplars = make([]*circularBufferEntry, l)
	ce.index = make(map[string]*indexEntry, l/estimatedExemplarsPerSeries)
	ce.nextIndex = 0

	// Replay as many entries as needed, starting with oldest first.
	count := int64(len(oldBuffer))
	if l < count {
		count = l
	}

	migrated := 0

	if l > 0 && len(oldBuffer) > 0 {
		// Rewind previous next index by count with wrap-around.
		// This math is essentially looking at nextIndex, where we would write the next exemplar to,
		// and find the index in the old exemplar buffer that we should start migrating exemplars from.
		// This way we don't migrate exemplars that would just be overwritten when migrating later exemplars.
		startIndex := (oldNextIndex - count + int64(len(oldBuffer))) % int64(len(oldBuffer))

		for i := int64(0); i < count; i++ {
			idx := (startIndex + i) % int64(len(oldBuffer))
			if entry := oldBuffer[idx]; entry != nil {
				ce.migrate(entry)
				migrated++
			}
		}
	}

	ce.computeMetrics()
	ce.metrics.maxExemplars.Set(float64(l))

	return migrated
}

// migrate is like AddExemplar but reuses existing structs. Expected to be called in batch and requires
// external lock and does not compute metrics.
func (ce *CircularExemplarStorage) migrate(entry *circularBufferEntry) {
	var buf [1024]byte
	seriesLabels := entry.ref.seriesLabels.Bytes(buf[:])

	idx, ok := ce.index[string(seriesLabels)]
	if !ok {
		idx = entry.ref
		idx.oldest = ce.nextIndex
		ce.index[string(seriesLabels)] = idx
	} else {
		entry.ref = idx
		ce.exemplars[idx.newest].next = ce.nextIndex
	}
	idx.newest = ce.nextIndex

	entry.next = noExemplar
	ce.exemplars[ce.nextIndex] = entry

	ce.nextIndex = (ce.nextIndex + 1) % len(ce.exemplars)
}

func (ce *CircularExemplarStorage) AddExemplar(l labels.Labels, e exemplar.Exemplar) error {
	if len(ce.exemplars) == 0 {
		return storage.ErrExemplarsDisabled
	}

	var buf [1024]byte
	seriesLabels := l.Bytes(buf[:])

	// TODO(bwplotka): This lock can lock all scrapers, there might high contention on this on scale.
	// Optimize by moving the lock to be per series (& benchmark it).
	ce.lock.Lock()
	defer ce.lock.Unlock()

	err := ce.validateExemplar(seriesLabels, e, true)
	if err != nil {
		if errors.Is(err, storage.ErrDuplicateExemplar) {
			// Duplicate exemplar, noop.
			return nil
		}
		return err
	}

	_, ok := ce.index[string(seriesLabels)]
	if !ok {
		ce.index[string(seriesLabels)] = &indexEntry{oldest: ce.nextIndex, seriesLabels: l}
	} else {
		ce.exemplars[ce.index[string(seriesLabels)].newest].next = ce.nextIndex
	}

	if prev := ce.exemplars[ce.nextIndex]; prev == nil {
		ce.exemplars[ce.nextIndex] = &circularBufferEntry{}
	} else {
		// There exists an exemplar already on this ce.nextIndex entry,
		// drop it, to make place for others.
		var buf [1024]byte
		prevLabels := prev.ref.seriesLabels.Bytes(buf[:])
		if prev.next == noExemplar {
			// Last item for this series, remove index entry.
			delete(ce.index, string(prevLabels))
		} else {
			ce.index[string(prevLabels)].oldest = prev.next
		}
	}

	// Default the next value to -1 (which we use to detect that we've iterated through all exemplars for a series in Select)
	// since this is the first exemplar stored for this series.
	ce.exemplars[ce.nextIndex].next = noExemplar
	ce.exemplars[ce.nextIndex].exemplar = e
	ce.exemplars[ce.nextIndex].ref = ce.index[string(seriesLabels)]
	ce.index[string(seriesLabels)].newest = ce.nextIndex

	ce.nextIndex = (ce.nextIndex + 1) % len(ce.exemplars)

	ce.metrics.exemplarsAppended.Inc()
	ce.computeMetrics()
	return nil
}

func (ce *CircularExemplarStorage) computeMetrics() {
	ce.metrics.seriesWithExemplarsInStorage.Set(float64(len(ce.index)))

	if len(ce.exemplars) == 0 {
		ce.metrics.exemplarsInStorage.Set(float64(0))
		ce.metrics.lastExemplarsTs.Set(float64(0))
		return
	}

	if next := ce.exemplars[ce.nextIndex]; next != nil {
		ce.metrics.exemplarsInStorage.Set(float64(len(ce.exemplars)))
		ce.metrics.lastExemplarsTs.Set(float64(next.exemplar.Ts) / 1000)
		return
	}

	// We did not yet fill the buffer.
	ce.metrics.exemplarsInStorage.Set(float64(ce.nextIndex))
	if ce.exemplars[0] != nil {
		ce.metrics.lastExemplarsTs.Set(float64(ce.exemplars[0].exemplar.Ts) / 1000)
	}
}

// IterateExemplars iterates through all the exemplars from oldest to newest appended and calls
// the given function on all of them till the end (or) till the first function call that returns an error.
func (ce *CircularExemplarStorage) IterateExemplars(f func(seriesLabels labels.Labels, e exemplar.Exemplar) error) error {
	ce.lock.RLock()
	defer ce.lock.RUnlock()

	idx := ce.nextIndex
	l := len(ce.exemplars)
	for i := 0; i < l; i, idx = i+1, (idx+1)%l {
		if ce.exemplars[idx] == nil {
			continue
		}
		err := f(ce.exemplars[idx].ref.seriesLabels, ce.exemplars[idx].exemplar)
		if err != nil {
			return err
		}
	}
	return nil
}
