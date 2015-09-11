// Copyright 2014 The Prometheus Authors
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

// Package local contains the local time series storage used by Prometheus.
package local

import (
	"container/list"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/log"

	"github.com/prometheus/prometheus/storage/metric"
)

const (
	evictRequestsCap = 1024
	chunkLen         = 1024

	// See waitForNextFP.
	fpMaxSweepTime    = 6 * time.Hour
	fpMaxWaitDuration = 10 * time.Second

	// See waitForNextFP.
	maxEvictInterval = time.Minute

	// If numChunskToPersist is this percentage of maxChunksToPersist, we
	// consider the storage in "graceful degradation mode", i.e. we do not
	// checkpoint anymore based on the dirty series count, and we do not
	// sync series files anymore if using the adaptive sync strategy.
	percentChunksToPersistForDegradation = 80
)

var (
	numChunksToPersistDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, "chunks_to_persist"),
		"The current number of chunks waiting for persistence.",
		nil, nil,
	)
	maxChunksToPersistDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, "max_chunks_to_persist"),
		"The maximum number of chunks that can be waiting for persistence before sample ingestion will stop.",
		nil, nil,
	)
)

type evictRequest struct {
	cd    *chunkDesc
	evict bool
}

// SyncStrategy is an enum to select a sync strategy for series files.
type SyncStrategy int

// String implements flag.Value.
func (ss SyncStrategy) String() string {
	switch ss {
	case Adaptive:
		return "adaptive"
	case Always:
		return "always"
	case Never:
		return "never"
	}
	return "<unknown>"
}

// Set implements flag.Value.
func (ss *SyncStrategy) Set(s string) error {
	switch s {
	case "adaptive":
		*ss = Adaptive
	case "always":
		*ss = Always
	case "never":
		*ss = Never
	default:
		return fmt.Errorf("invalid sync strategy: %s", s)
	}
	return nil
}

// Possible values for SyncStrategy.
const (
	_ SyncStrategy = iota
	Never
	Always
	Adaptive
)

// A syncStrategy is a function that returns whether series files should be
// synced or not. It does not need to be goroutine safe.
type syncStrategy func() bool

type memorySeriesStorage struct {
	// numChunksToPersist has to be aligned for atomic operations.
	numChunksToPersist int64 // The number of chunks waiting for persistence.
	maxChunksToPersist int   // If numChunksToPersist reaches this threshold, ingestion will stall.
	degraded           bool

	fpLocker   *fingerprintLocker
	fpToSeries *seriesMap

	options *MemorySeriesStorageOptions

	loopStopping, loopStopped  chan struct{}
	maxMemoryChunks            int
	dropAfter                  time.Duration
	checkpointInterval         time.Duration
	checkpointDirtySeriesLimit int

	persistence *persistence
	mapper      *fpMapper

	evictList                   *list.List
	evictRequests               chan evictRequest
	evictStopping, evictStopped chan struct{}

	persistErrors               prometheus.Counter
	numSeries                   prometheus.Gauge
	seriesOps                   *prometheus.CounterVec
	ingestedSamplesCount        prometheus.Counter
	outOfOrderSamplesCount      prometheus.Counter
	invalidPreloadRequestsCount prometheus.Counter
	maintainSeriesDuration      *prometheus.SummaryVec
}

// MemorySeriesStorageOptions contains options needed by
// NewMemorySeriesStorage. It is not safe to leave any of those at their zero
// values.
type MemorySeriesStorageOptions struct {
	MemoryChunks               int           // How many chunks to keep in memory.
	MaxChunksToPersist         int           // Max number of chunks waiting to be persisted.
	PersistenceStoragePath     string        // Location of persistence files.
	PersistenceRetentionPeriod time.Duration // Chunks at least that old are dropped.
	CheckpointInterval         time.Duration // How often to checkpoint the series map and head chunks.
	CheckpointDirtySeriesLimit int           // How many dirty series will trigger an early checkpoint.
	Dirty                      bool          // Force the storage to consider itself dirty on startup.
	PedanticChecks             bool          // If dirty, perform crash-recovery checks on each series file.
	SyncStrategy               SyncStrategy  // Which sync strategy to apply to series files.
}

// NewMemorySeriesStorage returns a newly allocated Storage. Storage.Serve still
// has to be called to start the storage.
func NewMemorySeriesStorage(o *MemorySeriesStorageOptions) Storage {
	s := &memorySeriesStorage{
		fpLocker: newFingerprintLocker(1024),

		options: o,

		loopStopping:               make(chan struct{}),
		loopStopped:                make(chan struct{}),
		maxMemoryChunks:            o.MemoryChunks,
		dropAfter:                  o.PersistenceRetentionPeriod,
		checkpointInterval:         o.CheckpointInterval,
		checkpointDirtySeriesLimit: o.CheckpointDirtySeriesLimit,

		maxChunksToPersist: o.MaxChunksToPersist,

		evictList:     list.New(),
		evictRequests: make(chan evictRequest, evictRequestsCap),
		evictStopping: make(chan struct{}),
		evictStopped:  make(chan struct{}),

		persistErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "persist_errors_total",
			Help:      "The total number of errors while persisting chunks.",
		}),
		numSeries: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "memory_series",
			Help:      "The current number of series in memory.",
		}),
		seriesOps: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "series_ops_total",
				Help:      "The total number of series operations by their type.",
			},
			[]string{opTypeLabel},
		),
		ingestedSamplesCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "ingested_samples_total",
			Help:      "The total number of samples ingested.",
		}),
		outOfOrderSamplesCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "out_of_order_samples_total",
			Help:      "The total number of samples that were discarded because their timestamps were at or before the last received sample for a series.",
		}),
		invalidPreloadRequestsCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "invalid_preload_requests_total",
			Help:      "The total number of preload requests referring to a non-existent series. This is an indication of outdated label indexes.",
		}),
		maintainSeriesDuration: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "maintain_series_duration_milliseconds",
				Help:      "The duration (in milliseconds) it took to perform maintenance on a series.",
			},
			[]string{seriesLocationLabel},
		),
	}
	return s
}

// Start implements Storage.
func (s *memorySeriesStorage) Start() (err error) {
	var syncStrategy syncStrategy
	switch s.options.SyncStrategy {
	case Never:
		syncStrategy = func() bool { return false }
	case Always:
		syncStrategy = func() bool { return true }
	case Adaptive:
		syncStrategy = func() bool { return !s.isDegraded() }
	default:
		panic("unknown sync strategy")
	}

	var p *persistence
	p, err = newPersistence(s.options.PersistenceStoragePath, s.options.Dirty, s.options.PedanticChecks, syncStrategy)
	if err != nil {
		return err
	}
	s.persistence = p
	// Persistence must start running before loadSeriesMapAndHeads() is called.
	go s.persistence.run()

	defer func() {
		if err != nil {
			if e := p.close(); e != nil {
				log.Errorln("Error closing persistence:", e)
			}
		}
	}()

	log.Info("Loading series map and head chunks...")
	s.fpToSeries, s.numChunksToPersist, err = p.loadSeriesMapAndHeads()
	if err != nil {
		return err
	}
	log.Infof("%d series loaded.", s.fpToSeries.length())
	s.numSeries.Set(float64(s.fpToSeries.length()))

	s.mapper, err = newFPMapper(s.fpToSeries, p)
	if err != nil {
		return err
	}

	go s.handleEvictList()
	go s.loop()

	return nil
}

// Stop implements Storage.
func (s *memorySeriesStorage) Stop() error {
	log.Info("Stopping local storage...")

	log.Info("Stopping maintenance loop...")
	close(s.loopStopping)
	<-s.loopStopped

	log.Info("Stopping chunk eviction...")
	close(s.evictStopping)
	<-s.evictStopped

	// One final checkpoint of the series map and the head chunks.
	if err := s.persistence.checkpointSeriesMapAndHeads(s.fpToSeries, s.fpLocker); err != nil {
		return err
	}

	if err := s.persistence.close(); err != nil {
		return err
	}
	log.Info("Local storage stopped.")
	return nil
}

// WaitForIndexing implements Storage.
func (s *memorySeriesStorage) WaitForIndexing() {
	s.persistence.waitForIndexing()
}

// NewIterator implements Storage.
func (s *memorySeriesStorage) NewIterator(fp model.Fingerprint) SeriesIterator {
	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	series, ok := s.fpToSeries.get(fp)
	if !ok {
		// Oops, no series for fp found. That happens if, after
		// preloading is done, the whole series is identified as old
		// enough for purging and hence purged for good. As there is no
		// data left to iterate over, return an iterator that will never
		// return any values.
		return nopSeriesIterator{}
	}
	return &boundedIterator{
		it:    series.newIterator(),
		start: model.Now().Add(-s.dropAfter),
	}
}

// LastSampleForFingerprint implements Storage.
func (s *memorySeriesStorage) LastSamplePairForFingerprint(fp model.Fingerprint) *model.SamplePair {
	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	series, ok := s.fpToSeries.get(fp)
	if !ok {
		return nil
	}
	return series.head().lastSamplePair()
}

// boundedIterator wraps a SeriesIterator and does not allow fetching
// data from earlier than the configured start time.
type boundedIterator struct {
	it    SeriesIterator
	start model.Time
}

// ValueAtTime implements the SeriesIterator interface.
func (bit *boundedIterator) ValueAtTime(ts model.Time) []model.SamplePair {
	if ts < bit.start {
		return []model.SamplePair{}
	}
	return bit.it.ValueAtTime(ts)
}

// BoundaryValues implements the SeriesIterator interface.
func (bit *boundedIterator) BoundaryValues(interval metric.Interval) []model.SamplePair {
	if interval.NewestInclusive < bit.start {
		return []model.SamplePair{}
	}
	if interval.OldestInclusive < bit.start {
		interval.OldestInclusive = bit.start
	}
	return bit.it.BoundaryValues(interval)
}

// RangeValues implements the SeriesIterator interface.
func (bit *boundedIterator) RangeValues(interval metric.Interval) []model.SamplePair {
	if interval.NewestInclusive < bit.start {
		return []model.SamplePair{}
	}
	if interval.OldestInclusive < bit.start {
		interval.OldestInclusive = bit.start
	}
	return bit.it.RangeValues(interval)
}

// NewPreloader implements Storage.
func (s *memorySeriesStorage) NewPreloader() Preloader {
	return &memorySeriesPreloader{
		storage: s,
	}
}

// fingerprintsForLabelPairs returns the set of fingerprints that have the given labels.
// This does not work with empty label values.
func (s *memorySeriesStorage) fingerprintsForLabelPairs(pairs ...model.LabelPair) map[model.Fingerprint]struct{} {
	var result map[model.Fingerprint]struct{}
	for _, pair := range pairs {
		intersection := map[model.Fingerprint]struct{}{}
		fps, err := s.persistence.fingerprintsForLabelPair(pair)
		if err != nil {
			log.Error("Error getting fingerprints for label pair: ", err)
		}
		if len(fps) == 0 {
			return nil
		}
		for _, fp := range fps {
			if _, ok := result[fp]; ok || result == nil {
				intersection[fp] = struct{}{}
			}
		}
		if len(intersection) == 0 {
			return nil
		}
		result = intersection
	}
	return result
}

// MetricsForLabelMatchers implements Storage.
func (s *memorySeriesStorage) MetricsForLabelMatchers(matchers ...*metric.LabelMatcher) map[model.Fingerprint]metric.Metric {
	var (
		equals  []model.LabelPair
		filters []*metric.LabelMatcher
	)
	for _, lm := range matchers {
		if lm.Type == metric.Equal && lm.Value != "" {
			equals = append(equals, model.LabelPair{
				Name:  lm.Name,
				Value: lm.Value,
			})
		} else {
			filters = append(filters, lm)
		}
	}

	var resFPs map[model.Fingerprint]struct{}
	if len(equals) > 0 {
		resFPs = s.fingerprintsForLabelPairs(equals...)
	} else {
		// If we cannot make a preselection based on equality matchers, expanding the other matchers to labels
		// and intersecting their fingerprints is still likely to be the best choice.
		var remaining metric.LabelMatchers
		for _, matcher := range filters {
			// Equal matches are all empty values.
			if matcher.Match("") {
				remaining = append(remaining, matcher)
				continue
			}
			intersection := map[model.Fingerprint]struct{}{}

			matches := matcher.Filter(s.LabelValuesForLabelName(matcher.Name))
			if len(matches) == 0 {
				return nil
			}
			for _, v := range matches {
				fps := s.fingerprintsForLabelPairs(model.LabelPair{
					Name:  matcher.Name,
					Value: v,
				})
				for fp := range fps {
					if _, ok := resFPs[fp]; ok || resFPs == nil {
						intersection[fp] = struct{}{}
					}
				}
			}
			resFPs = intersection
		}
		// The intersected matchers no longer need to be compared against the actual metrics.
		filters = remaining
	}

	result := make(map[model.Fingerprint]metric.Metric, len(resFPs))
	for fp := range resFPs {
		result[fp] = s.MetricForFingerprint(fp)
	}
	for _, matcher := range filters {
		for fp, met := range result {
			if !matcher.Match(met.Metric[matcher.Name]) {
				delete(result, fp)
			}
		}
	}
	return result
}

// LabelValuesForLabelName implements Storage.
func (s *memorySeriesStorage) LabelValuesForLabelName(labelName model.LabelName) model.LabelValues {
	lvs, err := s.persistence.labelValuesForLabelName(labelName)
	if err != nil {
		log.Errorf("Error getting label values for label name %q: %v", labelName, err)
	}
	return lvs
}

// MetricForFingerprint implements Storage.
func (s *memorySeriesStorage) MetricForFingerprint(fp model.Fingerprint) metric.Metric {
	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	series, ok := s.fpToSeries.get(fp)
	if ok {
		// Wrap the returned metric in a copy-on-write (COW) metric here because
		// the caller might mutate it.
		return metric.Metric{
			Metric: series.metric,
		}
	}
	met, err := s.persistence.archivedMetric(fp)
	if err != nil {
		log.Errorf("Error retrieving archived metric for fingerprint %v: %v", fp, err)
	}

	return metric.Metric{
		Metric: met,
		Copied: false,
	}
}

// DropMetric implements Storage.
func (s *memorySeriesStorage) DropMetricsForFingerprints(fps ...model.Fingerprint) {
	for _, fp := range fps {
		s.fpLocker.Lock(fp)

		if series, ok := s.fpToSeries.get(fp); ok {
			s.fpToSeries.del(fp)
			s.numSeries.Dec()
			s.persistence.unindexMetric(fp, series.metric)
		} else if err := s.persistence.purgeArchivedMetric(fp); err != nil {
			log.Errorf("Error purging metric with fingerprint %v: %v", fp, err)
		}
		// Attempt to delete series file in any case.
		if _, err := s.persistence.deleteSeriesFile(fp); err != nil {
			log.Errorf("Error deleting series file for %v: %v", fp, err)
		}

		s.fpLocker.Unlock(fp)
	}
}

// Append implements Storage.
func (s *memorySeriesStorage) Append(sample *model.Sample) {
	for ln, lv := range sample.Metric {
		if len(lv) == 0 {
			delete(sample.Metric, ln)
		}
	}
	if s.getNumChunksToPersist() >= s.maxChunksToPersist {
		log.Warnf(
			"%d chunks waiting for persistence, sample ingestion suspended.",
			s.getNumChunksToPersist(),
		)
		for s.getNumChunksToPersist() >= s.maxChunksToPersist {
			time.Sleep(time.Second)
		}
		log.Warn("Sample ingestion resumed.")
	}
	rawFP := sample.Metric.FastFingerprint()
	s.fpLocker.Lock(rawFP)
	fp, err := s.mapper.mapFP(rawFP, sample.Metric)
	if err != nil {
		log.Errorf("Error while mapping fingerprint %v: %v", rawFP, err)
		s.persistence.setDirty(true)
	}
	if fp != rawFP {
		// Switch locks.
		s.fpLocker.Unlock(rawFP)
		s.fpLocker.Lock(fp)
	}
	series := s.getOrCreateSeries(fp, sample.Metric)

	if sample.Timestamp <= series.lastTime {
		// Don't log and track equal timestamps, as they are a common occurrence
		// when using client-side timestamps (e.g. Pushgateway or federation).
		// It would be even better to also compare the sample values here, but
		// we don't have efficient access to a series's last value.
		if sample.Timestamp != series.lastTime {
			log.Warnf("Ignoring sample with out-of-order timestamp for fingerprint %v (%v): %v is not after %v", fp, series.metric, sample.Timestamp, series.lastTime)
			s.outOfOrderSamplesCount.Inc()
		}
		s.fpLocker.Unlock(fp)
		return
	}
	completedChunksCount := series.add(&model.SamplePair{
		Value:     sample.Value,
		Timestamp: sample.Timestamp,
	})
	s.fpLocker.Unlock(fp)
	s.ingestedSamplesCount.Inc()
	s.incNumChunksToPersist(completedChunksCount)
}

func (s *memorySeriesStorage) getOrCreateSeries(fp model.Fingerprint, m model.Metric) *memorySeries {
	series, ok := s.fpToSeries.get(fp)
	if !ok {
		var cds []*chunkDesc
		var modTime time.Time
		unarchived, err := s.persistence.unarchiveMetric(fp)
		if err != nil {
			log.Errorf("Error unarchiving fingerprint %v (metric %v): %v", fp, m, err)
		}
		if unarchived {
			s.seriesOps.WithLabelValues(unarchive).Inc()
			// We have to load chunkDescs anyway to do anything with
			// the series, so let's do it right now so that we don't
			// end up with a series without any chunkDescs for a
			// while (which is confusing as it makes the series
			// appear as archived or purged).
			cds, err = s.loadChunkDescs(fp, 0)
			if err != nil {
				log.Errorf("Error loading chunk descs for fingerprint %v (metric %v): %v", fp, m, err)
			}
			modTime = s.persistence.seriesFileModTime(fp)
		} else {
			// This was a genuinely new series, so index the metric.
			s.persistence.indexMetric(fp, m)
			s.seriesOps.WithLabelValues(create).Inc()
		}
		series = newMemorySeries(m, cds, modTime)
		s.fpToSeries.put(fp, series)
		s.numSeries.Inc()
	}
	return series
}

func (s *memorySeriesStorage) preloadChunksForRange(
	fp model.Fingerprint,
	from model.Time, through model.Time,
	stalenessDelta time.Duration,
) ([]*chunkDesc, error) {
	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	series, ok := s.fpToSeries.get(fp)
	if !ok {
		has, first, last, err := s.persistence.hasArchivedMetric(fp)
		if err != nil {
			return nil, err
		}
		if !has {
			s.invalidPreloadRequestsCount.Inc()
			return nil, nil
		}
		if from.Add(-stalenessDelta).Before(last) && through.Add(stalenessDelta).After(first) {
			metric, err := s.persistence.archivedMetric(fp)
			if err != nil {
				return nil, err
			}
			series = s.getOrCreateSeries(fp, metric)
		} else {
			return nil, nil
		}
	}
	return series.preloadChunksForRange(from, through, fp, s)
}

func (s *memorySeriesStorage) handleEvictList() {
	ticker := time.NewTicker(maxEvictInterval)
	count := 0

	for {
		// To batch up evictions a bit, this tries evictions at least
		// once per evict interval, but earlier if the number of evict
		// requests with evict==true that have happened since the last
		// evict run is more than maxMemoryChunks/1000.
		select {
		case req := <-s.evictRequests:
			if req.evict {
				req.cd.evictListElement = s.evictList.PushBack(req.cd)
				count++
				if count > s.maxMemoryChunks/1000 {
					s.maybeEvict()
					count = 0
				}
			} else {
				if req.cd.evictListElement != nil {
					s.evictList.Remove(req.cd.evictListElement)
					req.cd.evictListElement = nil
				}
			}
		case <-ticker.C:
			if s.evictList.Len() > 0 {
				s.maybeEvict()
			}
		case <-s.evictStopping:
			// Drain evictRequests forever in a goroutine to not let
			// requesters hang.
			go func() {
				for {
					<-s.evictRequests
				}
			}()
			ticker.Stop()
			log.Info("Chunk eviction stopped.")
			close(s.evictStopped)
			return
		}
	}
}

// maybeEvict is a local helper method. Must only be called by handleEvictList.
func (s *memorySeriesStorage) maybeEvict() {
	numChunksToEvict := int(atomic.LoadInt64(&numMemChunks)) - s.maxMemoryChunks
	if numChunksToEvict <= 0 {
		return
	}
	chunkDescsToEvict := make([]*chunkDesc, numChunksToEvict)
	for i := range chunkDescsToEvict {
		e := s.evictList.Front()
		if e == nil {
			break
		}
		cd := e.Value.(*chunkDesc)
		cd.evictListElement = nil
		chunkDescsToEvict[i] = cd
		s.evictList.Remove(e)
	}
	// Do the actual eviction in a goroutine as we might otherwise deadlock,
	// in the following way: A chunk was unpinned completely and therefore
	// scheduled for eviction. At the time we actually try to evict it,
	// another goroutine is pinning the chunk. The pinning goroutine has
	// currently locked the chunk and tries to send the evict request (to
	// remove the chunk from the evict list) to the evictRequests
	// channel. The send blocks because evictRequests is full. However, the
	// goroutine that is supposed to empty the channel is waiting for the
	// chunkDesc lock to try to evict the chunk.
	go func() {
		for _, cd := range chunkDescsToEvict {
			if cd == nil {
				break
			}
			cd.maybeEvict()
			// We don't care if the eviction succeeds. If the chunk
			// was pinned in the meantime, it will be added to the
			// evict list once it gets unpinned again.
		}
	}()
}

// waitForNextFP waits an estimated duration, after which we want to process
// another fingerprint so that we will process all fingerprints in a tenth of
// s.dropAfter assuming that the system is doing nothing else, e.g. if we want
// to drop chunks after 40h, we want to cycle through all fingerprints within
// 4h.  The estimation is based on the total number of fingerprints as passed
// in. However, the maximum sweep time is capped at fpMaxSweepTime. Also, the
// method will never wait for longer than fpMaxWaitDuration.
//
// The maxWaitDurationFactor can be used to reduce the waiting time if a faster
// processing is required (for example because unpersisted chunks pile up too
// much).
//
// Normally, the method returns true once the wait duration has passed. However,
// if s.loopStopped is closed, it will return false immediately.
func (s *memorySeriesStorage) waitForNextFP(numberOfFPs int, maxWaitDurationFactor float64) bool {
	d := fpMaxWaitDuration
	if numberOfFPs != 0 {
		sweepTime := s.dropAfter / 10
		if sweepTime > fpMaxSweepTime {
			sweepTime = fpMaxSweepTime
		}
		calculatedWait := time.Duration(float64(sweepTime) / float64(numberOfFPs) * maxWaitDurationFactor)
		if calculatedWait < d {
			d = calculatedWait
		}
	}
	if d == 0 {
		return true
	}
	t := time.NewTimer(d)
	select {
	case <-t.C:
		return true
	case <-s.loopStopping:
		return false
	}
}

// cycleThroughMemoryFingerprints returns a channel that emits fingerprints for
// series in memory in a throttled fashion. It continues to cycle through all
// fingerprints in memory until s.loopStopping is closed.
func (s *memorySeriesStorage) cycleThroughMemoryFingerprints() chan model.Fingerprint {
	memoryFingerprints := make(chan model.Fingerprint)
	go func() {
		var fpIter <-chan model.Fingerprint

		defer func() {
			if fpIter != nil {
				for range fpIter {
					// Consume the iterator.
				}
			}
			close(memoryFingerprints)
		}()

		for {
			// Initial wait, also important if there are no FPs yet.
			if !s.waitForNextFP(s.fpToSeries.length(), 1) {
				return
			}
			begin := time.Now()
			fpIter = s.fpToSeries.fpIter()
			count := 0
			for fp := range fpIter {
				select {
				case memoryFingerprints <- fp:
				case <-s.loopStopping:
					return
				}
				// Reduce the wait time by the backlog score.
				s.waitForNextFP(s.fpToSeries.length(), s.persistenceBacklogScore())
				count++
			}
			if count > 0 {
				log.Infof(
					"Completed maintenance sweep through %d in-memory fingerprints in %v.",
					count, time.Since(begin),
				)
			}
		}
	}()

	return memoryFingerprints
}

// cycleThroughArchivedFingerprints returns a channel that emits fingerprints
// for archived series in a throttled fashion. It continues to cycle through all
// archived fingerprints until s.loopStopping is closed.
func (s *memorySeriesStorage) cycleThroughArchivedFingerprints() chan model.Fingerprint {
	archivedFingerprints := make(chan model.Fingerprint)
	go func() {
		defer close(archivedFingerprints)

		for {
			archivedFPs, err := s.persistence.fingerprintsModifiedBefore(
				model.Now().Add(-s.dropAfter),
			)
			if err != nil {
				log.Error("Failed to lookup archived fingerprint ranges: ", err)
				s.waitForNextFP(0, 1)
				continue
			}
			// Initial wait, also important if there are no FPs yet.
			if !s.waitForNextFP(len(archivedFPs), 1) {
				return
			}
			begin := time.Now()
			for _, fp := range archivedFPs {
				select {
				case archivedFingerprints <- fp:
				case <-s.loopStopping:
					return
				}
				// Never speed up maintenance of archived FPs.
				s.waitForNextFP(len(archivedFPs), 1)
			}
			if len(archivedFPs) > 0 {
				log.Infof(
					"Completed maintenance sweep through %d archived fingerprints in %v.",
					len(archivedFPs), time.Since(begin),
				)
			}
		}
	}()
	return archivedFingerprints
}

func (s *memorySeriesStorage) loop() {
	checkpointTimer := time.NewTimer(s.checkpointInterval)

	dirtySeriesCount := 0

	defer func() {
		checkpointTimer.Stop()
		log.Info("Maintenance loop stopped.")
		close(s.loopStopped)
	}()

	memoryFingerprints := s.cycleThroughMemoryFingerprints()
	archivedFingerprints := s.cycleThroughArchivedFingerprints()

loop:
	for {
		select {
		case <-s.loopStopping:
			break loop
		case <-checkpointTimer.C:
			err := s.persistence.checkpointSeriesMapAndHeads(s.fpToSeries, s.fpLocker)
			if err != nil {
				log.Errorln("Error while checkpointing:", err)
			} else {
				dirtySeriesCount = 0
			}
			checkpointTimer.Reset(s.checkpointInterval)
		case fp := <-memoryFingerprints:
			if s.maintainMemorySeries(fp, model.Now().Add(-s.dropAfter)) {
				dirtySeriesCount++
				// Check if we have enough "dirty" series so that we need an early checkpoint.
				// However, if we are already behind persisting chunks, creating a checkpoint
				// would be counterproductive, as it would slow down chunk persisting even more,
				// while in a situation like that, where we are clearly lacking speed of disk
				// maintenance, the best we can do for crash recovery is to persist chunks as
				// quickly as possible. So only checkpoint if the storage is not in "graceful
				// degradation mode".
				if dirtySeriesCount >= s.checkpointDirtySeriesLimit && !s.isDegraded() {
					checkpointTimer.Reset(0)
				}
			}
		case fp := <-archivedFingerprints:
			s.maintainArchivedSeries(fp, model.Now().Add(-s.dropAfter))
		}
	}
	// Wait until both channels are closed.
	for range memoryFingerprints {
	}
	for range archivedFingerprints {
	}
}

// maintainMemorySeries maintains a series that is in memory (i.e. not
// archived). It returns true if the method has changed from clean to dirty
// (i.e. it is inconsistent with the latest checkpoint now so that in case of a
// crash a recovery operation that requires a disk seek needed to be applied).
//
// The method first closes the head chunk if it was not touched for the duration
// of headChunkTimeout.
//
// Then it determines the chunks that need to be purged and the chunks that need
// to be persisted. Depending on the result, it does the following:
//
// - If all chunks of a series need to be purged, the whole series is deleted
// for good and the method returns false. (Detecting non-existence of a series
// file does not require a disk seek.)
//
// - If any chunks need to be purged (but not all of them), it purges those
// chunks from memory and rewrites the series file on disk, leaving out the
// purged chunks and appending all chunks not yet persisted (with the exception
// of a still open head chunk).
//
// - If no chunks on disk need to be purged, but chunks need to be persisted,
// those chunks are simply appended to the existing series file (or the file is
// created if it does not exist yet).
//
// - If no chunks need to be purged and no chunks need to be persisted, nothing
// happens in this step.
//
// Next, the method checks if all chunks in the series are evicted. In that
// case, it archives the series and returns true.
//
// Finally, it evicts chunkDescs if there are too many.
func (s *memorySeriesStorage) maintainMemorySeries(
	fp model.Fingerprint, beforeTime model.Time,
) (becameDirty bool) {
	defer func(begin time.Time) {
		s.maintainSeriesDuration.WithLabelValues(maintainInMemory).Observe(
			float64(time.Since(begin)) / float64(time.Millisecond),
		)
	}(time.Now())

	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	series, ok := s.fpToSeries.get(fp)
	if !ok {
		// Series is actually not in memory, perhaps archived or dropped in the meantime.
		return false
	}

	defer s.seriesOps.WithLabelValues(memoryMaintenance).Inc()

	if series.maybeCloseHeadChunk() {
		s.incNumChunksToPersist(1)
	}

	seriesWasDirty := series.dirty

	if s.writeMemorySeries(fp, series, beforeTime) {
		// Series is gone now, we are done.
		return false
	}

	iOldestNotEvicted := -1
	for i, cd := range series.chunkDescs {
		if !cd.isEvicted() {
			iOldestNotEvicted = i
			break
		}
	}

	// Archive if all chunks are evicted.
	if iOldestNotEvicted == -1 {
		s.fpToSeries.del(fp)
		s.numSeries.Dec()
		if err := s.persistence.archiveMetric(
			fp, series.metric, series.firstTime(), series.lastTime,
		); err != nil {
			log.Errorf("Error archiving metric %v: %v", series.metric, err)
			return
		}
		s.seriesOps.WithLabelValues(archive).Inc()
		return
	}
	// If we are here, the series is not archived, so check for chunkDesc
	// eviction next.
	series.evictChunkDescs(iOldestNotEvicted)

	return series.dirty && !seriesWasDirty
}

// writeMemorySeries (re-)writes a memory series file. While doing so, it drops
// chunks older than beforeTime from both the series file (if it exists) as well
// as from memory. The provided chunksToPersist are appended to the newly
// written series file. If no chunks need to be purged, but chunksToPersist is
// not empty, those chunks are simply appended to the series file. If the series
// contains no chunks after dropping old chunks, it is purged entirely. In that
// case, the method returns true.
//
// The caller must have locked the fp.
func (s *memorySeriesStorage) writeMemorySeries(
	fp model.Fingerprint, series *memorySeries, beforeTime model.Time,
) bool {
	cds := series.chunksToPersist()
	defer func() {
		for _, cd := range cds {
			cd.unpin(s.evictRequests)
		}
		s.incNumChunksToPersist(-len(cds))
		chunkOps.WithLabelValues(persistAndUnpin).Add(float64(len(cds)))
		series.modTime = s.persistence.seriesFileModTime(fp)
	}()

	// Get the actual chunks from underneath the chunkDescs.
	// No lock required as chunks still to persist cannot be evicted.
	chunks := make([]chunk, len(cds))
	for i, cd := range cds {
		chunks[i] = cd.c
	}

	if !series.firstTime().Before(beforeTime) {
		// Oldest sample not old enough, just append chunks, if any.
		if len(cds) == 0 {
			return false
		}
		offset, err := s.persistence.persistChunks(fp, chunks)
		if err != nil {
			s.persistErrors.Inc()
			return false
		}
		if series.chunkDescsOffset == -1 {
			// This is the first chunk persisted for a newly created
			// series that had prior chunks on disk. Finally, we can
			// set the chunkDescsOffset.
			series.chunkDescsOffset = offset
		}
		return false
	}

	newFirstTime, offset, numDroppedFromPersistence, allDroppedFromPersistence, err :=
		s.persistence.dropAndPersistChunks(fp, beforeTime, chunks)
	if err != nil {
		s.persistErrors.Inc()
		return false
	}
	series.dropChunks(beforeTime)
	if len(series.chunkDescs) == 0 && allDroppedFromPersistence {
		// All chunks dropped from both memory and persistence. Delete the series for good.
		s.fpToSeries.del(fp)
		s.numSeries.Dec()
		s.seriesOps.WithLabelValues(memoryPurge).Inc()
		s.persistence.unindexMetric(fp, series.metric)
		return true
	}
	series.savedFirstTime = newFirstTime
	if series.chunkDescsOffset == -1 {
		series.chunkDescsOffset = offset
	} else {
		series.chunkDescsOffset -= numDroppedFromPersistence
		if series.chunkDescsOffset < 0 {
			log.Errorf("Dropped more chunks from persistence than from memory for fingerprint %v, series %v.", fp, series)
			s.persistence.setDirty(true)
			series.chunkDescsOffset = -1 // Makes sure it will be looked at during crash recovery.
		}
	}
	return false
}

// maintainArchivedSeries drops chunks older than beforeTime from an archived
// series. If the series contains no chunks after that, it is purged entirely.
func (s *memorySeriesStorage) maintainArchivedSeries(fp model.Fingerprint, beforeTime model.Time) {
	defer func(begin time.Time) {
		s.maintainSeriesDuration.WithLabelValues(maintainArchived).Observe(
			float64(time.Since(begin)) / float64(time.Millisecond),
		)
	}(time.Now())

	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	has, firstTime, lastTime, err := s.persistence.hasArchivedMetric(fp)
	if err != nil {
		log.Error("Error looking up archived time range: ", err)
		return
	}
	if !has || !firstTime.Before(beforeTime) {
		// Oldest sample not old enough, or metric purged or unarchived in the meantime.
		return
	}

	defer s.seriesOps.WithLabelValues(archiveMaintenance).Inc()

	newFirstTime, _, _, allDropped, err := s.persistence.dropAndPersistChunks(fp, beforeTime, nil)
	if err != nil {
		log.Error("Error dropping persisted chunks: ", err)
	}
	if allDropped {
		if err := s.persistence.purgeArchivedMetric(fp); err != nil {
			log.Errorf("Error purging archived metric for fingerprint %v: %v", fp, err)
			return
		}
		s.seriesOps.WithLabelValues(archivePurge).Inc()
		return
	}
	s.persistence.updateArchivedTimeRange(fp, newFirstTime, lastTime)
}

// See persistence.loadChunks for detailed explanation.
func (s *memorySeriesStorage) loadChunks(fp model.Fingerprint, indexes []int, indexOffset int) ([]chunk, error) {
	return s.persistence.loadChunks(fp, indexes, indexOffset)
}

// See persistence.loadChunkDescs for detailed explanation.
func (s *memorySeriesStorage) loadChunkDescs(fp model.Fingerprint, offsetFromEnd int) ([]*chunkDesc, error) {
	return s.persistence.loadChunkDescs(fp, offsetFromEnd)
}

// getNumChunksToPersist returns numChunksToPersist in a goroutine-safe way.
func (s *memorySeriesStorage) getNumChunksToPersist() int {
	return int(atomic.LoadInt64(&s.numChunksToPersist))
}

// incNumChunksToPersist increments numChunksToPersist in a goroutine-safe way. Use a
// negative 'by' to decrement.
func (s *memorySeriesStorage) incNumChunksToPersist(by int) {
	atomic.AddInt64(&s.numChunksToPersist, int64(by))
}

// isDegraded returns whether the storage is in "graceful degradation mode",
// which is the case if the number of chunks waiting for persistence has reached
// a percentage of maxChunksToPersist that exceeds
// percentChunksToPersistForDegradation. The method is not goroutine safe (but
// only ever called from the goroutine dealing with series maintenance).
// Changes of degradation mode are logged.
func (s *memorySeriesStorage) isDegraded() bool {
	nowDegraded := s.getNumChunksToPersist() > s.maxChunksToPersist*percentChunksToPersistForDegradation/100
	if s.degraded && !nowDegraded {
		log.Warn("Storage has left graceful degradation mode. Things are back to normal.")
	} else if !s.degraded && nowDegraded {
		log.Warnf(
			"%d chunks waiting for persistence (%d%% of the allowed maximum %d). Storage is now in graceful degradation mode. Series files are not synced anymore if following the adaptive strategy. Checkpoints are not performed more often than every %v. Series maintenance happens as frequently as possible.",
			s.getNumChunksToPersist(),
			s.getNumChunksToPersist()*100/s.maxChunksToPersist,
			s.maxChunksToPersist,
			s.checkpointInterval)
	}
	s.degraded = nowDegraded
	return s.degraded
}

// persistenceBacklogScore works similar to isDegraded, but returns a score
// about how close we are to degradation. This score is 1.0 if no chunks are
// waiting for persistence and 0.0 if we are at or above the degradation
// threshold.
func (s *memorySeriesStorage) persistenceBacklogScore() float64 {
	score := 1 - float64(s.getNumChunksToPersist())/float64(s.maxChunksToPersist*percentChunksToPersistForDegradation/100)
	if score < 0 {
		return 0
	}
	return score
}

// Describe implements prometheus.Collector.
func (s *memorySeriesStorage) Describe(ch chan<- *prometheus.Desc) {
	s.persistence.Describe(ch)
	s.mapper.Describe(ch)

	ch <- s.persistErrors.Desc()
	ch <- maxChunksToPersistDesc
	ch <- numChunksToPersistDesc
	ch <- s.numSeries.Desc()
	s.seriesOps.Describe(ch)
	ch <- s.ingestedSamplesCount.Desc()
	ch <- s.outOfOrderSamplesCount.Desc()
	ch <- s.invalidPreloadRequestsCount.Desc()
	ch <- numMemChunksDesc
	s.maintainSeriesDuration.Describe(ch)
}

// Collect implements prometheus.Collector.
func (s *memorySeriesStorage) Collect(ch chan<- prometheus.Metric) {
	s.persistence.Collect(ch)
	s.mapper.Collect(ch)

	ch <- s.persistErrors
	ch <- prometheus.MustNewConstMetric(
		maxChunksToPersistDesc,
		prometheus.GaugeValue,
		float64(s.maxChunksToPersist),
	)
	ch <- prometheus.MustNewConstMetric(
		numChunksToPersistDesc,
		prometheus.GaugeValue,
		float64(s.getNumChunksToPersist()),
	)
	ch <- s.numSeries
	s.seriesOps.Collect(ch)
	ch <- s.ingestedSamplesCount
	ch <- s.outOfOrderSamplesCount
	ch <- s.invalidPreloadRequestsCount
	ch <- prometheus.MustNewConstMetric(
		numMemChunksDesc,
		prometheus.GaugeValue,
		float64(atomic.LoadInt64(&numMemChunks)),
	)
	s.maintainSeriesDuration.Collect(ch)
}
