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
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/storage/local/chunk"
	"github.com/prometheus/prometheus/storage/metric"
)

const (
	evictRequestsCap      = 1024
	quarantineRequestsCap = 1024

	// See waitForNextFP.
	fpMaxSweepTime    = 6 * time.Hour
	fpMaxWaitDuration = 10 * time.Second

	// See waitForNextFP.
	maxEvictInterval = time.Minute

	// Constants to control the hysteresis of entering and leaving "rushed
	// mode". In rushed mode, the dirty series count is ignored for
	// checkpointing, series are maintained as frequently as possible, and
	// series files are not synced if the adaptive sync strategy is used.
	persintenceUrgencyScoreForEnteringRushedMode = 0.8
	persintenceUrgencyScoreForLeavingRushedMode  = 0.7

	// This factor times -storage.local.memory-chunks is the number of
	// memory chunks we tolerate before throttling the storage. It is also a
	// basis for calculating the persistenceUrgencyScore.
	toleranceFactorMemChunks = 1.1
	// This factor times -storage.local.max-chunks-to-persist is the minimum
	// required number of chunks waiting for persistence before the number
	// of chunks in memory may influence the persistenceUrgencyScore. (In
	// other words: if there are no chunks to persist, it doesn't help chunk
	// eviction if we speed up persistence.)
	factorMinChunksToPersist = 0.2

	// Threshold for when to stop using LabelMatchers to retrieve and
	// intersect fingerprints. The rationale here is that looking up more
	// fingerprints has diminishing returns if we already have narrowed down
	// the possible fingerprints significantly. It is then easier to simply
	// lookup the metrics for all the fingerprints and directly compare them
	// to the matchers. Since a fingerprint lookup for an Equal matcher is
	// much less expensive, there is a lower threshold for that case.
	// TODO(beorn7): These numbers need to be tweaked, probably a bit lower.
	// 5x higher numbers have resulted in slightly worse performance in a
	// real-life production scenario.
	fpEqualMatchThreshold = 1000
	fpOtherMatchThreshold = 10000
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
	maxMemChunksDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, "max_memory_chunks"),
		"The configured maximum number of chunks that can be held in memory",
		nil, nil,
	)
)

type quarantineRequest struct {
	fp     model.Fingerprint
	metric model.Metric
	reason error
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

// A MemorySeriesStorage manages series in memory over time, while also
// interfacing with a persistence layer to make time series data persistent
// across restarts and evictable from memory.
type MemorySeriesStorage struct {
	// archiveHighWatermark and numChunksToPersist have to be aligned for atomic operations.
	archiveHighWatermark model.Time    // No archived series has samples after this time.
	numChunksToPersist   int64         // The number of chunks waiting for persistence.
	maxChunksToPersist   int           // If numChunksToPersist reaches this threshold, ingestion will be throttled.
	rushed               bool          // Whether the storage is in rushed mode.
	rushedMtx            sync.Mutex    // Protects entering and exiting rushed mode.
	throttled            chan struct{} // This chan is sent to whenever NeedsThrottling() returns true (for logging).

	fpLocker   *fingerprintLocker
	fpToSeries *seriesMap

	options *MemorySeriesStorageOptions

	loopStopping, loopStopped  chan struct{}
	logThrottlingStopped       chan struct{}
	maxMemoryChunks            int
	dropAfter                  time.Duration
	checkpointInterval         time.Duration
	checkpointDirtySeriesLimit int

	persistence *persistence
	mapper      *fpMapper

	evictList                   *list.List
	evictRequests               chan chunk.EvictRequest
	evictStopping, evictStopped chan struct{}

	quarantineRequests                    chan quarantineRequest
	quarantineStopping, quarantineStopped chan struct{}

	persistErrors                 prometheus.Counter
	queuedChunksToPersist         prometheus.Counter
	numSeries                     prometheus.Gauge
	numHeadChunks                 prometheus.Gauge
	dirtySeries                   prometheus.Gauge
	seriesOps                     *prometheus.CounterVec
	ingestedSamplesCount          prometheus.Counter
	discardedSamplesCount         *prometheus.CounterVec
	nonExistentSeriesMatchesCount prometheus.Counter
	maintainSeriesDuration        *prometheus.SummaryVec
	persistenceUrgencyScore       prometheus.Gauge
	rushedMode                    prometheus.Gauge
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
	MinShrinkRatio             float64       // Minimum ratio a series file has to shrink during truncation.
	NumMutexes                 int           // Number of mutexes used for stochastic fingerprint locking.
}

// NewMemorySeriesStorage returns a newly allocated Storage. Storage.Serve still
// has to be called to start the storage.
func NewMemorySeriesStorage(o *MemorySeriesStorageOptions) *MemorySeriesStorage {
	s := &MemorySeriesStorage{
		fpLocker: newFingerprintLocker(o.NumMutexes),

		options: o,

		loopStopping:               make(chan struct{}),
		loopStopped:                make(chan struct{}),
		logThrottlingStopped:       make(chan struct{}),
		throttled:                  make(chan struct{}, 1),
		maxMemoryChunks:            o.MemoryChunks,
		dropAfter:                  o.PersistenceRetentionPeriod,
		checkpointInterval:         o.CheckpointInterval,
		checkpointDirtySeriesLimit: o.CheckpointDirtySeriesLimit,
		archiveHighWatermark:       model.Now().Add(-headChunkTimeout),

		maxChunksToPersist: o.MaxChunksToPersist,

		evictList:     list.New(),
		evictRequests: make(chan chunk.EvictRequest, evictRequestsCap),
		evictStopping: make(chan struct{}),
		evictStopped:  make(chan struct{}),

		quarantineRequests: make(chan quarantineRequest, quarantineRequestsCap),
		quarantineStopping: make(chan struct{}),
		quarantineStopped:  make(chan struct{}),

		persistErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "persist_errors_total",
			Help:      "The total number of errors while persisting chunks.",
		}),
		queuedChunksToPersist: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queued_chunks_to_persist_total",
			Help:      "The total number of chunks queued for persistence.",
		}),
		numSeries: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "memory_series",
			Help:      "The current number of series in memory.",
		}),
		numHeadChunks: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "open_head_chunks",
			Help:      "The current number of open head chunks.",
		}),
		dirtySeries: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "memory_dirty_series",
			Help:      "The current number of series that would require a disk seek during crash recovery.",
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
		discardedSamplesCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "out_of_order_samples_total",
				Help:      "The total number of samples that were discarded because their timestamps were at or before the last received sample for a series.",
			},
			[]string{discardReasonLabel},
		),
		nonExistentSeriesMatchesCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "non_existent_series_matches_total",
			Help:      "How often a non-existent series was referred to during label matching or chunk preloading. This is an indication of outdated label indexes.",
		}),
		maintainSeriesDuration: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "maintain_series_duration_seconds",
				Help:      "The duration in seconds it took to perform maintenance on a series.",
			},
			[]string{seriesLocationLabel},
		),
		persistenceUrgencyScore: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "persistence_urgency_score",
			Help:      "A score of urgency to persist chunks, 0 is least urgent, 1 most.",
		}),
		rushedMode: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "rushed_mode",
			Help:      "1 if the storage is in rushed mode, 0 otherwise. In rushed mode, the system behaves as if the persistence_urgency_score is 1.",
		}),
	}

	// Initialize metric vectors.
	// TODO(beorn7): Rework once we have a utility function for it in client_golang.
	s.discardedSamplesCount.WithLabelValues(outOfOrderTimestamp)
	s.discardedSamplesCount.WithLabelValues(duplicateSample)
	s.maintainSeriesDuration.WithLabelValues(maintainInMemory)
	s.maintainSeriesDuration.WithLabelValues(maintainArchived)
	s.seriesOps.WithLabelValues(create)
	s.seriesOps.WithLabelValues(archive)
	s.seriesOps.WithLabelValues(unarchive)
	s.seriesOps.WithLabelValues(memoryPurge)
	s.seriesOps.WithLabelValues(archivePurge)
	s.seriesOps.WithLabelValues(requestedPurge)
	s.seriesOps.WithLabelValues(memoryMaintenance)
	s.seriesOps.WithLabelValues(archiveMaintenance)
	s.seriesOps.WithLabelValues(completedQurantine)
	s.seriesOps.WithLabelValues(droppedQuarantine)
	s.seriesOps.WithLabelValues(failedQuarantine)

	return s
}

// Start implements Storage.
func (s *MemorySeriesStorage) Start() (err error) {
	var syncStrategy syncStrategy
	switch s.options.SyncStrategy {
	case Never:
		syncStrategy = func() bool { return false }
	case Always:
		syncStrategy = func() bool { return true }
	case Adaptive:
		syncStrategy = func() bool { return s.calculatePersistenceUrgencyScore() < 1 }
	default:
		panic("unknown sync strategy")
	}

	var p *persistence
	p, err = newPersistence(
		s.options.PersistenceStoragePath,
		s.options.Dirty, s.options.PedanticChecks,
		syncStrategy,
		s.options.MinShrinkRatio,
	)
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
	for fp := range s.fpToSeries.fpIter() {
		if series, ok := s.fpToSeries.get(fp); ok {
			if !series.headChunkClosed {
				s.numHeadChunks.Inc()
			}
		}
	}

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
	go s.handleQuarantine()
	go s.logThrottling()
	go s.loop()

	return nil
}

// Stop implements Storage.
func (s *MemorySeriesStorage) Stop() error {
	log.Info("Stopping local storage...")

	log.Info("Stopping maintenance loop...")
	close(s.loopStopping)
	<-s.loopStopped

	log.Info("Stopping series quarantining...")
	close(s.quarantineStopping)
	<-s.quarantineStopped

	log.Info("Stopping chunk eviction...")
	close(s.evictStopping)
	<-s.evictStopped

	// One final checkpoint of the series map and the head chunks.
	if err := s.persistence.checkpointSeriesMapAndHeads(s.fpToSeries, s.fpLocker); err != nil {
		return err
	}
	if err := s.mapper.checkpoint(); err != nil {
		return err
	}

	if err := s.persistence.close(); err != nil {
		return err
	}
	log.Info("Local storage stopped.")
	return nil
}

type memorySeriesStorageQuerier struct {
	*MemorySeriesStorage
}

func (memorySeriesStorageQuerier) Close() error {
	return nil
}

// Querier implements the storage interface.
func (s *MemorySeriesStorage) Querier() (Querier, error) {
	return memorySeriesStorageQuerier{s}, nil
}

// WaitForIndexing implements Storage.
func (s *MemorySeriesStorage) WaitForIndexing() {
	s.persistence.waitForIndexing()
}

// LastSampleForLabelMatchers implements Storage.
func (s *MemorySeriesStorage) LastSampleForLabelMatchers(_ context.Context, cutoff model.Time, matcherSets ...metric.LabelMatchers) (model.Vector, error) {
	mergedFPs := map[model.Fingerprint]struct{}{}
	for _, matchers := range matcherSets {
		fps, err := s.fpsForLabelMatchers(cutoff, model.Latest, matchers...)
		if err != nil {
			return nil, err
		}
		for fp := range fps {
			mergedFPs[fp] = struct{}{}
		}
	}

	res := make(model.Vector, 0, len(mergedFPs))
	for fp := range mergedFPs {
		s.fpLocker.Lock(fp)

		series, ok := s.fpToSeries.get(fp)
		if !ok {
			// A series could have disappeared between resolving label matchers and here.
			s.fpLocker.Unlock(fp)
			continue
		}
		sp := series.lastSamplePair()
		res = append(res, &model.Sample{
			Metric:    series.metric,
			Value:     sp.Value,
			Timestamp: sp.Timestamp,
		})
		s.fpLocker.Unlock(fp)
	}
	return res, nil
}

// boundedIterator wraps a SeriesIterator and does not allow fetching
// data from earlier than the configured start time.
type boundedIterator struct {
	it    SeriesIterator
	start model.Time
}

// ValueAtOrBeforeTime implements the SeriesIterator interface.
func (bit *boundedIterator) ValueAtOrBeforeTime(ts model.Time) model.SamplePair {
	if ts < bit.start {
		return model.ZeroSamplePair
	}
	return bit.it.ValueAtOrBeforeTime(ts)
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

// Metric implements SeriesIterator.
func (bit *boundedIterator) Metric() metric.Metric {
	return bit.it.Metric()
}

// Close implements SeriesIterator.
func (bit *boundedIterator) Close() {
	bit.it.Close()
}

// QueryRange implements Storage.
func (s *MemorySeriesStorage) QueryRange(_ context.Context, from, through model.Time, matchers ...*metric.LabelMatcher) ([]SeriesIterator, error) {
	if through.Before(from) {
		// In that case, nothing will match.
		return nil, nil
	}
	fpSeriesPairs, err := s.seriesForLabelMatchers(from, through, matchers...)
	if err != nil {
		return nil, err
	}
	iterators := make([]SeriesIterator, 0, len(fpSeriesPairs))
	for _, pair := range fpSeriesPairs {
		it := s.preloadChunksForRange(pair, from, through)
		iterators = append(iterators, it)
	}
	return iterators, nil
}

// QueryInstant implements Storage.
func (s *MemorySeriesStorage) QueryInstant(_ context.Context, ts model.Time, stalenessDelta time.Duration, matchers ...*metric.LabelMatcher) ([]SeriesIterator, error) {
	if stalenessDelta < 0 {
		panic("negative staleness delta")
	}
	from := ts.Add(-stalenessDelta)
	through := ts

	fpSeriesPairs, err := s.seriesForLabelMatchers(from, through, matchers...)
	if err != nil {
		return nil, err
	}
	iterators := make([]SeriesIterator, 0, len(fpSeriesPairs))
	for _, pair := range fpSeriesPairs {
		it := s.preloadChunksForInstant(pair, from, through)
		iterators = append(iterators, it)
	}
	return iterators, nil
}

// fingerprintsForLabelPair returns the fingerprints with the given
// LabelPair. If intersectWith is non-nil, the method will only return
// fingerprints that are also contained in intersectsWith. If mergeWith is
// non-nil, the found fingerprints are added to the given map. The returned map
// is the same as the given one.
func (s *MemorySeriesStorage) fingerprintsForLabelPair(
	pair model.LabelPair,
	mergeWith map[model.Fingerprint]struct{},
	intersectWith map[model.Fingerprint]struct{},
) map[model.Fingerprint]struct{} {
	if mergeWith == nil {
		mergeWith = map[model.Fingerprint]struct{}{}
	}
	for _, fp := range s.persistence.fingerprintsForLabelPair(pair) {
		if intersectWith == nil {
			mergeWith[fp] = struct{}{}
			continue
		}
		if _, ok := intersectWith[fp]; ok {
			mergeWith[fp] = struct{}{}
		}
	}
	return mergeWith
}

// MetricsForLabelMatchers implements Storage.
func (s *MemorySeriesStorage) MetricsForLabelMatchers(
	_ context.Context,
	from, through model.Time,
	matcherSets ...metric.LabelMatchers,
) ([]metric.Metric, error) {
	fpToMetric := map[model.Fingerprint]metric.Metric{}
	for _, matchers := range matcherSets {
		metrics, err := s.metricsForLabelMatchers(from, through, matchers...)
		if err != nil {
			return nil, err
		}
		for fp, m := range metrics {
			fpToMetric[fp] = m
		}
	}

	metrics := make([]metric.Metric, 0, len(fpToMetric))
	for _, m := range fpToMetric {
		metrics = append(metrics, m)
	}
	return metrics, nil
}

// candidateFPsForLabelMatchers returns candidate FPs for given matchers and remaining matchers to be checked.
func (s *MemorySeriesStorage) candidateFPsForLabelMatchers(
	matchers ...*metric.LabelMatcher,
) (map[model.Fingerprint]struct{}, []*metric.LabelMatcher, error) {
	sort.Sort(metric.LabelMatchers(matchers))

	if len(matchers) == 0 || matchers[0].MatchesEmptyString() {
		// No matchers at all or even the best matcher matches the empty string.
		return nil, nil, nil
	}

	var (
		matcherIdx   int
		candidateFPs map[model.Fingerprint]struct{}
	)

	// Equal matchers.
	for ; matcherIdx < len(matchers) && (candidateFPs == nil || len(candidateFPs) > fpEqualMatchThreshold); matcherIdx++ {
		m := matchers[matcherIdx]
		if m.Type != metric.Equal || m.MatchesEmptyString() {
			break
		}
		candidateFPs = s.fingerprintsForLabelPair(
			model.LabelPair{
				Name:  m.Name,
				Value: m.Value,
			},
			nil,
			candidateFPs,
		)
		if len(candidateFPs) == 0 {
			return nil, nil, nil
		}
	}

	// Other matchers.
	for ; matcherIdx < len(matchers) && (candidateFPs == nil || len(candidateFPs) > fpOtherMatchThreshold); matcherIdx++ {
		m := matchers[matcherIdx]
		if m.MatchesEmptyString() {
			break
		}

		lvs, err := s.LabelValuesForLabelName(context.TODO(), m.Name)
		if err != nil {
			return nil, nil, err
		}
		lvs = m.Filter(lvs)
		if len(lvs) == 0 {
			return nil, nil, nil
		}
		fps := map[model.Fingerprint]struct{}{}
		for _, lv := range lvs {
			s.fingerprintsForLabelPair(
				model.LabelPair{
					Name:  m.Name,
					Value: lv,
				},
				fps,
				candidateFPs,
			)
		}
		candidateFPs = fps
		if len(candidateFPs) == 0 {
			return nil, nil, nil
		}
	}
	return candidateFPs, matchers[matcherIdx:], nil
}

func (s *MemorySeriesStorage) seriesForLabelMatchers(
	from, through model.Time,
	matchers ...*metric.LabelMatcher,
) ([]fingerprintSeriesPair, error) {
	candidateFPs, matchersToCheck, err := s.candidateFPsForLabelMatchers(matchers...)
	if err != nil {
		return nil, err
	}

	result := []fingerprintSeriesPair{}
FPLoop:
	for fp := range candidateFPs {
		s.fpLocker.Lock(fp)
		series := s.seriesForRange(fp, from, through)
		s.fpLocker.Unlock(fp)

		if series == nil {
			continue FPLoop
		}

		for _, m := range matchersToCheck {
			if !m.Match(series.metric[m.Name]) {
				continue FPLoop
			}
		}
		result = append(result, fingerprintSeriesPair{fp, series})
	}
	return result, nil
}

func (s *MemorySeriesStorage) fpsForLabelMatchers(
	from, through model.Time,
	matchers ...*metric.LabelMatcher,
) (map[model.Fingerprint]struct{}, error) {
	candidateFPs, matchersToCheck, err := s.candidateFPsForLabelMatchers(matchers...)
	if err != nil {
		return nil, err
	}

FPLoop:
	for fp := range candidateFPs {
		s.fpLocker.Lock(fp)
		met, _, ok := s.metricForRange(fp, from, through)
		s.fpLocker.Unlock(fp)

		if !ok {
			delete(candidateFPs, fp)
			continue FPLoop
		}

		for _, m := range matchersToCheck {
			if !m.Match(met[m.Name]) {
				delete(candidateFPs, fp)
				continue FPLoop
			}
		}
	}
	return candidateFPs, nil
}

func (s *MemorySeriesStorage) metricsForLabelMatchers(
	from, through model.Time,
	matchers ...*metric.LabelMatcher,
) (map[model.Fingerprint]metric.Metric, error) {

	candidateFPs, matchersToCheck, err := s.candidateFPsForLabelMatchers(matchers...)
	if err != nil {
		return nil, err
	}

	result := map[model.Fingerprint]metric.Metric{}
FPLoop:
	for fp := range candidateFPs {
		s.fpLocker.Lock(fp)
		met, _, ok := s.metricForRange(fp, from, through)
		s.fpLocker.Unlock(fp)

		if !ok {
			continue FPLoop
		}

		for _, m := range matchersToCheck {
			if !m.Match(met[m.Name]) {
				continue FPLoop
			}
		}
		result[fp] = metric.Metric{Metric: met}
	}
	return result, nil
}

// metricForRange returns the metric for the given fingerprint if the
// corresponding time series has samples between 'from' and 'through', together
// with a pointer to the series if it is in memory already. For a series that
// does not have samples between 'from' and 'through', the returned bool is
// false. For an archived series that does contain samples between 'from' and
// 'through', it returns (metric, nil, true).
//
// The caller must have locked the fp.
func (s *MemorySeriesStorage) metricForRange(
	fp model.Fingerprint,
	from, through model.Time,
) (model.Metric, *memorySeries, bool) {
	series, ok := s.fpToSeries.get(fp)
	if ok {
		if series.lastTime.Before(from) || series.firstTime().After(through) {
			return nil, nil, false
		}
		return series.metric, series, true
	}
	// From here on, we are only concerned with archived metrics.
	// If the high watermark of archived series is before 'from', we are done.
	watermark := model.Time(atomic.LoadInt64((*int64)(&s.archiveHighWatermark)))
	if watermark < from {
		return nil, nil, false
	}
	if from.After(model.Earliest) || through.Before(model.Latest) {
		// The range lookup is relatively cheap, so let's do it first if
		// we have a chance the archived metric is not in the range.
		has, first, last := s.persistence.hasArchivedMetric(fp)
		if !has {
			s.nonExistentSeriesMatchesCount.Inc()
			return nil, nil, false
		}
		if first.After(through) || last.Before(from) {
			return nil, nil, false
		}
	}

	metric, err := s.persistence.archivedMetric(fp)
	if err != nil {
		// archivedMetric has already flagged the storage as dirty in this case.
		return nil, nil, false
	}
	return metric, nil, true
}

// LabelValuesForLabelName implements Storage.
func (s *MemorySeriesStorage) LabelValuesForLabelName(_ context.Context, labelName model.LabelName) (model.LabelValues, error) {
	return s.persistence.labelValuesForLabelName(labelName)
}

// DropMetricsForLabelMatchers implements Storage.
func (s *MemorySeriesStorage) DropMetricsForLabelMatchers(_ context.Context, matchers ...*metric.LabelMatcher) (int, error) {
	fps, err := s.fpsForLabelMatchers(model.Earliest, model.Latest, matchers...)
	if err != nil {
		return 0, err
	}
	for fp := range fps {
		s.purgeSeries(fp, nil, nil)
	}
	return len(fps), nil
}

var (
	// ErrOutOfOrderSample is returned if a sample has a timestamp before the latest
	// timestamp in the series it is appended to.
	ErrOutOfOrderSample = fmt.Errorf("sample timestamp out of order")
	// ErrDuplicateSampleForTimestamp is returned if a sample has the same
	// timestamp as the latest sample in the series it is appended to but a
	// different value. (Appending an identical sample is a no-op and does
	// not cause an error.)
	ErrDuplicateSampleForTimestamp = fmt.Errorf("sample with repeated timestamp but different value")
)

// Append implements Storage.
func (s *MemorySeriesStorage) Append(sample *model.Sample) error {
	for ln, lv := range sample.Metric {
		if len(lv) == 0 {
			delete(sample.Metric, ln)
		}
	}
	rawFP := sample.Metric.FastFingerprint()
	s.fpLocker.Lock(rawFP)
	fp := s.mapper.mapFP(rawFP, sample.Metric)
	defer func() {
		s.fpLocker.Unlock(fp)
	}() // Func wrapper because fp might change below.
	if fp != rawFP {
		// Switch locks.
		s.fpLocker.Unlock(rawFP)
		s.fpLocker.Lock(fp)
	}
	series, err := s.getOrCreateSeries(fp, sample.Metric)
	if err != nil {
		return err // getOrCreateSeries took care of quarantining already.
	}

	if sample.Timestamp == series.lastTime {
		// Don't report "no-op appends", i.e. where timestamp and sample
		// value are the same as for the last append, as they are a
		// common occurrence when using client-side timestamps
		// (e.g. Pushgateway or federation).
		if sample.Timestamp == series.lastTime &&
			series.lastSampleValueSet &&
			sample.Value.Equal(series.lastSampleValue) {
			return nil
		}
		s.discardedSamplesCount.WithLabelValues(duplicateSample).Inc()
		return ErrDuplicateSampleForTimestamp // Caused by the caller.
	}
	if sample.Timestamp < series.lastTime {
		s.discardedSamplesCount.WithLabelValues(outOfOrderTimestamp).Inc()
		return ErrOutOfOrderSample // Caused by the caller.
	}
	completedChunksCount, err := series.add(model.SamplePair{
		Value:     sample.Value,
		Timestamp: sample.Timestamp,
	})
	if err != nil {
		s.quarantineSeries(fp, sample.Metric, err)
		return err
	}
	s.ingestedSamplesCount.Inc()
	s.incNumChunksToPersist(completedChunksCount)

	return nil
}

// NeedsThrottling implements Storage.
func (s *MemorySeriesStorage) NeedsThrottling() bool {
	if s.getNumChunksToPersist() > s.maxChunksToPersist ||
		float64(atomic.LoadInt64(&chunk.NumMemChunks)) > float64(s.maxMemoryChunks)*toleranceFactorMemChunks {
		select {
		case s.throttled <- struct{}{}:
		default: // Do nothing, signal already pending.
		}
		return true
	}
	return false
}

// logThrottling handles logging of throttled events and has to be started as a
// goroutine. It stops once s.loopStopping is closed.
//
// Logging strategy: Whenever Throttle() is called and returns true, an signal
// is sent to s.throttled. If that happens for the first time, an Error is
// logged that the storage is now throttled. As long as signals continues to be
// sent via s.throttled at least once per minute, nothing else is logged. Once
// no signal has arrived for a minute, an Info is logged that the storage is not
// throttled anymore. This resets things to the initial state, i.e. once a
// signal arrives again, the Error will be logged again.
func (s *MemorySeriesStorage) logThrottling() {
	timer := time.NewTimer(time.Minute)
	timer.Stop()

	// Signal exit of the goroutine. Currently only needed by test code.
	defer close(s.logThrottlingStopped)

	for {
		select {
		case <-s.throttled:
			if !timer.Reset(time.Minute) {
				log.
					With("chunksToPersist", s.getNumChunksToPersist()).
					With("maxChunksToPersist", s.maxChunksToPersist).
					With("memoryChunks", atomic.LoadInt64(&chunk.NumMemChunks)).
					With("maxToleratedMemChunks", int(float64(s.maxMemoryChunks)*toleranceFactorMemChunks)).
					Error("Storage needs throttling. Scrapes and rule evaluations will be skipped.")
			}
		case <-timer.C:
			log.
				With("chunksToPersist", s.getNumChunksToPersist()).
				With("maxChunksToPersist", s.maxChunksToPersist).
				With("memoryChunks", atomic.LoadInt64(&chunk.NumMemChunks)).
				With("maxToleratedMemChunks", int(float64(s.maxMemoryChunks)*toleranceFactorMemChunks)).
				Info("Storage does not need throttling anymore.")
		case <-s.loopStopping:
			return
		}
	}
}

func (s *MemorySeriesStorage) getOrCreateSeries(fp model.Fingerprint, m model.Metric) (*memorySeries, error) {
	series, ok := s.fpToSeries.get(fp)
	if !ok {
		var cds []*chunk.Desc
		var modTime time.Time
		unarchived, err := s.persistence.unarchiveMetric(fp)
		if err != nil {
			log.Errorf("Error unarchiving fingerprint %v (metric %v): %v", fp, m, err)
			return nil, err
		}
		if unarchived {
			s.seriesOps.WithLabelValues(unarchive).Inc()
			// We have to load chunk.Descs anyway to do anything with
			// the series, so let's do it right now so that we don't
			// end up with a series without any chunk.Descs for a
			// while (which is confusing as it makes the series
			// appear as archived or purged).
			cds, err = s.loadChunkDescs(fp, 0)
			if err == nil && len(cds) == 0 {
				err = fmt.Errorf("unarchived fingerprint %v (metric %v) has no chunks on disk", fp, m)
			}
			if err != nil {
				s.quarantineSeries(fp, m, err)
				return nil, err
			}
			modTime = s.persistence.seriesFileModTime(fp)
		} else {
			// This was a genuinely new series, so index the metric.
			s.persistence.indexMetric(fp, m)
			s.seriesOps.WithLabelValues(create).Inc()
		}
		series, err = newMemorySeries(m, cds, modTime)
		if err != nil {
			s.quarantineSeries(fp, m, err)
			return nil, err
		}
		s.fpToSeries.put(fp, series)
		s.numSeries.Inc()
		if !series.headChunkClosed {
			s.numHeadChunks.Inc()
		}
	}
	return series, nil
}

// seriesForRange is a helper method for seriesForLabelMatchers.
//
// The caller must have locked the fp.
func (s *MemorySeriesStorage) seriesForRange(
	fp model.Fingerprint,
	from model.Time, through model.Time,
) *memorySeries {
	metric, series, ok := s.metricForRange(fp, from, through)
	if !ok {
		return nil
	}
	if series == nil {
		series, _ = s.getOrCreateSeries(fp, metric)
		// getOrCreateSeries took care of quarantining already, so ignore the error.
	}
	return series
}

func (s *MemorySeriesStorage) preloadChunksForRange(
	pair fingerprintSeriesPair,
	from model.Time, through model.Time,
) SeriesIterator {
	fp, series := pair.fp, pair.series
	if series == nil {
		return nopIter
	}

	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	iter, err := series.preloadChunksForRange(fp, from, through, s)
	if err != nil {
		s.quarantineSeries(fp, series.metric, err)
		return nopIter
	}
	return iter
}

func (s *MemorySeriesStorage) preloadChunksForInstant(
	pair fingerprintSeriesPair,
	from model.Time, through model.Time,
) SeriesIterator {
	fp, series := pair.fp, pair.series
	if series == nil {
		return nopIter
	}

	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	iter, err := series.preloadChunksForInstant(fp, from, through, s)
	if err != nil {
		s.quarantineSeries(fp, series.metric, err)
		return nopIter
	}
	return iter
}

func (s *MemorySeriesStorage) handleEvictList() {
	ticker := time.NewTicker(maxEvictInterval)
	count := 0

	for {
		// To batch up evictions a bit, this tries evictions at least
		// once per evict interval, but earlier if the number of evict
		// requests with evict==true that have happened since the last
		// evict run is more than maxMemoryChunks/1000.
		select {
		case req := <-s.evictRequests:
			if req.Evict {
				req.Desc.EvictListElement = s.evictList.PushBack(req.Desc)
				count++
				if count > s.maxMemoryChunks/1000 {
					s.maybeEvict()
					count = 0
				}
			} else {
				if req.Desc.EvictListElement != nil {
					s.evictList.Remove(req.Desc.EvictListElement)
					req.Desc.EvictListElement = nil
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
func (s *MemorySeriesStorage) maybeEvict() {
	numChunksToEvict := int(atomic.LoadInt64(&chunk.NumMemChunks)) - s.maxMemoryChunks
	if numChunksToEvict <= 0 {
		return
	}
	chunkDescsToEvict := make([]*chunk.Desc, numChunksToEvict)
	for i := range chunkDescsToEvict {
		e := s.evictList.Front()
		if e == nil {
			break
		}
		cd := e.Value.(*chunk.Desc)
		cd.EvictListElement = nil
		chunkDescsToEvict[i] = cd
		s.evictList.Remove(e)
	}
	// Do the actual eviction in a goroutine as we might otherwise deadlock,
	// in the following way: A chunk was Unpinned completely and therefore
	// scheduled for eviction. At the time we actually try to evict it,
	// another goroutine is pinning the chunk. The pinning goroutine has
	// currently locked the chunk and tries to send the evict request (to
	// remove the chunk from the evict list) to the evictRequests
	// channel. The send blocks because evictRequests is full. However, the
	// goroutine that is supposed to empty the channel is waiting for the
	// Chunk.Desc lock to try to evict the chunk.
	go func() {
		for _, cd := range chunkDescsToEvict {
			if cd == nil {
				break
			}
			cd.MaybeEvict()
			// We don't care if the eviction succeeds. If the chunk
			// was pinned in the meantime, it will be added to the
			// evict list once it gets Unpinned again.
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
func (s *MemorySeriesStorage) waitForNextFP(numberOfFPs int, maxWaitDurationFactor float64) bool {
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
func (s *MemorySeriesStorage) cycleThroughMemoryFingerprints() chan model.Fingerprint {
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
				// Reduce the wait time according to the urgency score.
				s.waitForNextFP(s.fpToSeries.length(), 1-s.calculatePersistenceUrgencyScore())
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
func (s *MemorySeriesStorage) cycleThroughArchivedFingerprints() chan model.Fingerprint {
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

func (s *MemorySeriesStorage) loop() {
	checkpointTimer := time.NewTimer(s.checkpointInterval)

	var dirtySeriesCount int64

	defer func() {
		checkpointTimer.Stop()
		log.Info("Maintenance loop stopped.")
		close(s.loopStopped)
	}()

	memoryFingerprints := s.cycleThroughMemoryFingerprints()
	archivedFingerprints := s.cycleThroughArchivedFingerprints()

	// Checkpoints can happen concurrently with maintenance so even with heavy
	// checkpointing there will still be sufficient progress on maintenance.
	checkpointLoopStopped := make(chan struct{})
	go func() {
		for {
			select {
			case <-s.loopStopping:
				checkpointLoopStopped <- struct{}{}
				return
			case <-checkpointTimer.C:
				// We clear this before the checkpoint so that dirtySeriesCount
				// is an upper bound.
				atomic.StoreInt64(&dirtySeriesCount, 0)
				s.dirtySeries.Set(0)
				err := s.persistence.checkpointSeriesMapAndHeads(s.fpToSeries, s.fpLocker)
				if err != nil {
					log.Errorln("Error while checkpointing:", err)
				}
				// If a checkpoint takes longer than checkpointInterval, unluckily timed
				// combination with the Reset(0) call below can lead to a case where a
				// time is lurking in C leading to repeated checkpointing without break.
				select {
				case <-checkpointTimer.C: // Get rid of the lurking time.
				default:
				}
				checkpointTimer.Reset(s.checkpointInterval)
			}
		}
	}()

loop:
	for {
		select {
		case <-s.loopStopping:
			break loop
		case fp := <-memoryFingerprints:
			if s.maintainMemorySeries(fp, model.Now().Add(-s.dropAfter)) {
				dirty := atomic.AddInt64(&dirtySeriesCount, 1)
				s.dirtySeries.Set(float64(dirty))
				// Check if we have enough "dirty" series so that we need an early checkpoint.
				// However, if we are already behind persisting chunks, creating a checkpoint
				// would be counterproductive, as it would slow down chunk persisting even more,
				// while in a situation like that, where we are clearly lacking speed of disk
				// maintenance, the best we can do for crash recovery is to persist chunks as
				// quickly as possible. So only checkpoint if the urgency score is < 1.
				if dirty >= int64(s.checkpointDirtySeriesLimit) &&
					s.calculatePersistenceUrgencyScore() < 1 {
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
	<-checkpointLoopStopped
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
// Finally, it evicts chunk.Descs if there are too many.
func (s *MemorySeriesStorage) maintainMemorySeries(
	fp model.Fingerprint, beforeTime model.Time,
) (becameDirty bool) {
	defer func(begin time.Time) {
		s.maintainSeriesDuration.WithLabelValues(maintainInMemory).Observe(
			time.Since(begin).Seconds(),
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

	closed, err := series.maybeCloseHeadChunk()
	if err != nil {
		s.quarantineSeries(fp, series.metric, err)
		s.persistErrors.Inc()
	}
	if closed {
		s.incNumChunksToPersist(1)
		s.numHeadChunks.Dec()
	}

	seriesWasDirty := series.dirty

	if s.writeMemorySeries(fp, series, beforeTime) {
		// Series is gone now, we are done.
		return false
	}

	iOldestNotEvicted := -1
	for i, cd := range series.chunkDescs {
		if !cd.IsEvicted() {
			iOldestNotEvicted = i
			break
		}
	}

	// Archive if all chunks are evicted. Also make sure the last sample has
	// an age of at least headChunkTimeout (which is very likely anyway).
	if iOldestNotEvicted == -1 && model.Now().Sub(series.lastTime) > headChunkTimeout {
		s.fpToSeries.del(fp)
		s.numSeries.Dec()
		s.persistence.archiveMetric(fp, series.metric, series.firstTime(), series.lastTime)
		s.seriesOps.WithLabelValues(archive).Inc()
		oldWatermark := atomic.LoadInt64((*int64)(&s.archiveHighWatermark))
		if oldWatermark < int64(series.lastTime) {
			if !atomic.CompareAndSwapInt64(
				(*int64)(&s.archiveHighWatermark),
				oldWatermark, int64(series.lastTime),
			) {
				panic("s.archiveHighWatermark modified outside of maintainMemorySeries")
			}
		}
		return
	}
	// If we are here, the series is not archived, so check for Chunk.Desc
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
// If a persist error is encountered, the series is queued for quarantine. In
// that case, the method returns true, too, because the series should not be
// processed anymore (even if it will only be gone for real once quarantining
// has been completed).
//
// The caller must have locked the fp.
func (s *MemorySeriesStorage) writeMemorySeries(
	fp model.Fingerprint, series *memorySeries, beforeTime model.Time,
) bool {
	var (
		persistErr error
		cds        = series.chunksToPersist()
	)

	defer func() {
		if persistErr != nil {
			s.quarantineSeries(fp, series.metric, persistErr)
			s.persistErrors.Inc()
		}
		// The following is done even in case of an error to ensure
		// correct counter bookkeeping and to not pin chunks in memory
		// that belong to a series that is scheduled for quarantine
		// anyway.
		for _, cd := range cds {
			cd.Unpin(s.evictRequests)
		}
		s.incNumChunksToPersist(-len(cds))
		chunk.Ops.WithLabelValues(chunk.PersistAndUnpin).Add(float64(len(cds)))
		series.modTime = s.persistence.seriesFileModTime(fp)
	}()

	// Get the actual chunks from underneath the chunk.Descs.
	// No lock required as chunks still to persist cannot be evicted.
	chunks := make([]chunk.Chunk, len(cds))
	for i, cd := range cds {
		chunks[i] = cd.C
	}

	if !series.firstTime().Before(beforeTime) {
		// Oldest sample not old enough, just append chunks, if any.
		if len(cds) == 0 {
			return false
		}
		var offset int
		offset, persistErr = s.persistence.persistChunks(fp, chunks)
		if persistErr != nil {
			return true
		}
		if series.chunkDescsOffset == -1 {
			// This is the first chunk persisted for a newly created
			// series that had prior chunks on disk. Finally, we can
			// set the chunkDescsOffset.
			series.chunkDescsOffset = offset
		}
		return false
	}

	newFirstTime, offset, numDroppedFromPersistence, allDroppedFromPersistence, persistErr :=
		s.persistence.dropAndPersistChunks(fp, beforeTime, chunks)
	if persistErr != nil {
		return true
	}
	if persistErr = series.dropChunks(beforeTime); persistErr != nil {
		return true
	}
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
			persistErr = errors.New("dropped more chunks from persistence than from memory")
			series.chunkDescsOffset = 0
			return true
		}
	}
	return false
}

// maintainArchivedSeries drops chunks older than beforeTime from an archived
// series. If the series contains no chunks after that, it is purged entirely.
func (s *MemorySeriesStorage) maintainArchivedSeries(fp model.Fingerprint, beforeTime model.Time) {
	defer func(begin time.Time) {
		s.maintainSeriesDuration.WithLabelValues(maintainArchived).Observe(
			time.Since(begin).Seconds(),
		)
	}(time.Now())

	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	has, firstTime, lastTime := s.persistence.hasArchivedMetric(fp)
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
		s.persistence.purgeArchivedMetric(fp) // Ignoring error. Nothing we can do.
		s.seriesOps.WithLabelValues(archivePurge).Inc()
		return
	}
	if err := s.persistence.updateArchivedTimeRange(fp, newFirstTime, lastTime); err != nil {
		log.Errorf("Error updating archived time range for fingerprint %v: %s", fp, err)
	}
}

// See persistence.loadChunks for detailed explanation.
func (s *MemorySeriesStorage) loadChunks(fp model.Fingerprint, indexes []int, indexOffset int) ([]chunk.Chunk, error) {
	return s.persistence.loadChunks(fp, indexes, indexOffset)
}

// See persistence.loadChunkDescs for detailed explanation.
func (s *MemorySeriesStorage) loadChunkDescs(fp model.Fingerprint, offsetFromEnd int) ([]*chunk.Desc, error) {
	return s.persistence.loadChunkDescs(fp, offsetFromEnd)
}

// getNumChunksToPersist returns numChunksToPersist in a goroutine-safe way.
func (s *MemorySeriesStorage) getNumChunksToPersist() int {
	return int(atomic.LoadInt64(&s.numChunksToPersist))
}

// incNumChunksToPersist increments numChunksToPersist in a goroutine-safe way. Use a
// negative 'by' to decrement.
func (s *MemorySeriesStorage) incNumChunksToPersist(by int) {
	atomic.AddInt64(&s.numChunksToPersist, int64(by))
	if by > 0 {
		s.queuedChunksToPersist.Add(float64(by))
	}
}

// calculatePersistenceUrgencyScore calculates and returns an urgency score for
// the speed of persisting chunks. The score is between 0 and 1, where 0 means
// no urgency at all and 1 means highest urgency.
//
// The score is the maximum of the two following sub-scores:
//
// (1) The first sub-score is the number of chunks waiting for persistence
// divided by the maximum number of chunks allowed to be waiting for
// persistence.
//
// (2) If there are more chunks in memory than allowed AND there are more chunks
// waiting for persistence than factorMinChunksToPersist times
// -storage.local.max-chunks-to-persist, then the second sub-score is the
// fraction the number of memory chunks has reached between
// -storage.local.memory-chunks and toleranceFactorForMemChunks times
// -storage.local.memory-chunks.
//
// Should the score ever hit persintenceUrgencyScoreForEnteringRushedMode, the
// storage locks into "rushed mode", in which the returned score is always
// bumped up to 1 until the non-bumped score is below
// persintenceUrgencyScoreForLeavingRushedMode.
//
// This method is not goroutine-safe, but it is only ever called by the single
// goroutine that is in charge of series maintenance. According to the returned
// score, series maintenance should be sped up. If a score of 1 is returned,
// checkpointing based on dirty-series count should be disabled, and series
// files should not by synced anymore provided the user has specified the
// adaptive sync strategy.
func (s *MemorySeriesStorage) calculatePersistenceUrgencyScore() float64 {
	s.rushedMtx.Lock()
	defer s.rushedMtx.Unlock()

	var (
		chunksToPersist    = float64(s.getNumChunksToPersist())
		maxChunksToPersist = float64(s.maxChunksToPersist)
		memChunks          = float64(atomic.LoadInt64(&chunk.NumMemChunks))
		maxMemChunks       = float64(s.maxMemoryChunks)
	)
	score := chunksToPersist / maxChunksToPersist
	if chunksToPersist > maxChunksToPersist*factorMinChunksToPersist {
		score = math.Max(
			score,
			(memChunks/maxMemChunks-1)/(toleranceFactorMemChunks-1),
		)
	}
	if score > 1 {
		score = 1
	}
	s.persistenceUrgencyScore.Set(score)

	if s.rushed {
		// We are already in rushed mode. If the score is still above
		// persintenceUrgencyScoreForLeavingRushedMode, return 1 and
		// leave things as they are.
		if score > persintenceUrgencyScoreForLeavingRushedMode {
			return 1
		}
		// We are out of rushed mode!
		s.rushed = false
		s.rushedMode.Set(0)
		log.
			With("urgencyScore", score).
			With("chunksToPersist", int(chunksToPersist)).
			With("maxChunksToPersist", int(maxChunksToPersist)).
			With("memoryChunks", int(memChunks)).
			With("maxMemoryChunks", int(maxMemChunks)).
			Info("Storage has left rushed mode.")
		return score
	}
	if score > persintenceUrgencyScoreForEnteringRushedMode {
		// Enter rushed mode.
		s.rushed = true
		s.rushedMode.Set(1)
		log.
			With("urgencyScore", score).
			With("chunksToPersist", int(chunksToPersist)).
			With("maxChunksToPersist", int(maxChunksToPersist)).
			With("memoryChunks", int(memChunks)).
			With("maxMemoryChunks", int(maxMemChunks)).
			Warn("Storage has entered rushed mode.")
		return 1
	}
	return score
}

// quarantineSeries registers the provided fingerprint for quarantining. It
// always returns immediately. Quarantine requests are processed
// asynchronously. If there are too many requests queued, they are simply
// dropped.
//
// Quarantining means that the series file is moved to the orphaned directory,
// and all its traces are removed from indices. Call this method if an
// unrecoverable error is detected while dealing with a series, and pass in the
// encountered error. It will be saved as a hint in the orphaned directory.
func (s *MemorySeriesStorage) quarantineSeries(fp model.Fingerprint, metric model.Metric, err error) {
	req := quarantineRequest{fp: fp, metric: metric, reason: err}
	select {
	case s.quarantineRequests <- req:
		// Request submitted.
	default:
		log.
			With("fingerprint", fp).
			With("metric", metric).
			With("reason", err).
			Warn("Quarantine queue full. Dropped quarantine request.")
		s.seriesOps.WithLabelValues(droppedQuarantine).Inc()
	}
}

func (s *MemorySeriesStorage) handleQuarantine() {
	for {
		select {
		case req := <-s.quarantineRequests:
			s.purgeSeries(req.fp, req.metric, req.reason)
			log.
				With("fingerprint", req.fp).
				With("metric", req.metric).
				With("reason", req.reason).
				Warn("Series quarantined.")
		case <-s.quarantineStopping:
			log.Info("Series quarantining stopped.")
			close(s.quarantineStopped)
			return
		}
	}

}

// purgeSeries removes all traces of a series. If a non-nil quarantine reason is
// provided, the series file will not be deleted completely, but moved to the
// orphaned directory with the reason and the metric in a hint file. The
// provided metric might be nil if unknown.
func (s *MemorySeriesStorage) purgeSeries(fp model.Fingerprint, m model.Metric, quarantineReason error) {
	s.fpLocker.Lock(fp)

	var (
		series *memorySeries
		ok     bool
	)

	if series, ok = s.fpToSeries.get(fp); ok {
		s.fpToSeries.del(fp)
		s.numSeries.Dec()
		m = series.metric

		// Adjust s.numChunksToPersist and chunk.NumMemChunks down by
		// the number of chunks in this series that are not
		// persisted yet. Persisted chunks will be deducted from
		// chunk.NumMemChunks upon eviction.
		numChunksNotYetPersisted := len(series.chunkDescs) - series.persistWatermark
		atomic.AddInt64(&chunk.NumMemChunks, int64(-numChunksNotYetPersisted))
		if !series.headChunkClosed {
			// Head chunk wasn't counted as waiting for persistence yet.
			// (But it was counted as a chunk in memory.)
			numChunksNotYetPersisted--
		}
		s.incNumChunksToPersist(-numChunksNotYetPersisted)

	} else {
		s.persistence.purgeArchivedMetric(fp) // Ignoring error. There is nothing we can do.
	}
	if m != nil {
		// If we know a metric now, unindex it in any case.
		// purgeArchivedMetric might have done so already, but we cannot
		// be sure. Unindexing in idempotent, though.
		s.persistence.unindexMetric(fp, m)
	}
	// Attempt to delete/quarantine the series file in any case.
	if quarantineReason == nil {
		// No reason stated, simply delete the file.
		if _, err := s.persistence.deleteSeriesFile(fp); err != nil {
			log.
				With("fingerprint", fp).
				With("metric", m).
				With("error", err).
				Error("Error deleting series file.")
		}
		s.seriesOps.WithLabelValues(requestedPurge).Inc()
	} else {
		if err := s.persistence.quarantineSeriesFile(fp, quarantineReason, m); err == nil {
			s.seriesOps.WithLabelValues(completedQurantine).Inc()
		} else {
			s.seriesOps.WithLabelValues(failedQuarantine).Inc()
			log.
				With("fingerprint", fp).
				With("metric", m).
				With("reason", quarantineReason).
				With("error", err).
				Error("Error quarantining series file.")
		}
	}

	s.fpLocker.Unlock(fp)
}

// Describe implements prometheus.Collector.
func (s *MemorySeriesStorage) Describe(ch chan<- *prometheus.Desc) {
	s.persistence.Describe(ch)
	s.mapper.Describe(ch)

	ch <- s.persistErrors.Desc()
	ch <- s.queuedChunksToPersist.Desc()
	ch <- maxChunksToPersistDesc
	ch <- numChunksToPersistDesc
	ch <- s.numSeries.Desc()
	ch <- s.numHeadChunks.Desc()
	ch <- s.dirtySeries.Desc()
	s.seriesOps.Describe(ch)
	ch <- s.ingestedSamplesCount.Desc()
	s.discardedSamplesCount.Describe(ch)
	ch <- s.nonExistentSeriesMatchesCount.Desc()
	ch <- chunk.NumMemChunksDesc
	s.maintainSeriesDuration.Describe(ch)
	ch <- s.persistenceUrgencyScore.Desc()
	ch <- s.rushedMode.Desc()
}

// Collect implements prometheus.Collector.
func (s *MemorySeriesStorage) Collect(ch chan<- prometheus.Metric) {
	s.persistence.Collect(ch)
	s.mapper.Collect(ch)

	ch <- s.persistErrors
	ch <- s.queuedChunksToPersist
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
	ch <- s.numHeadChunks
	ch <- s.dirtySeries
	s.seriesOps.Collect(ch)
	ch <- s.ingestedSamplesCount
	s.discardedSamplesCount.Collect(ch)
	ch <- s.nonExistentSeriesMatchesCount
	ch <- prometheus.MustNewConstMetric(
		maxMemChunksDesc,
		prometheus.GaugeValue,
		float64(s.maxMemoryChunks),
	)
	ch <- prometheus.MustNewConstMetric(
		chunk.NumMemChunksDesc,
		prometheus.GaugeValue,
		float64(atomic.LoadInt64(&chunk.NumMemChunks)),
	)
	s.maintainSeriesDuration.Collect(ch)
	ch <- s.persistenceUrgencyScore
	ch <- s.rushedMode
}
