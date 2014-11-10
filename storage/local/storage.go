// Copyright 2014 Prometheus Team
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
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

const (
	persistQueueCap = 1024
	chunkLen        = 1024
)

type storageState uint

const (
	storageStarting storageState = iota
	storageServing
	storageStopping
)

type persistRequest struct {
	fingerprint clientmodel.Fingerprint
	chunkDesc   *chunkDesc
}

type memorySeriesStorage struct {
	fpLocker   *fingerprintLocker
	fpToSeries *seriesMap

	loopStopping, loopStopped chan struct{}
	evictInterval, evictAfter time.Duration
	purgeInterval, purgeAfter time.Duration
	checkpointInterval        time.Duration

	persistQueue   chan persistRequest
	persistStopped chan struct{}
	persistence    *persistence

	persistLatency               prometheus.Summary
	persistErrors                *prometheus.CounterVec
	persistQueueLength           prometheus.Gauge
	numSeries                    prometheus.Gauge
	seriesOps                    *prometheus.CounterVec
	ingestedSamplesCount         prometheus.Counter
	invalidPreloadRequestsCount  prometheus.Counter
	purgeDuration, evictDuration prometheus.Gauge
}

// MemorySeriesStorageOptions contains options needed by
// NewMemorySeriesStorage. It is not safe to leave any of those at their zero
// values.
type MemorySeriesStorageOptions struct {
	MemoryEvictionInterval     time.Duration // How often to check for memory eviction.
	MemoryRetentionPeriod      time.Duration // Chunks at least that old are evicted from memory.
	PersistenceStoragePath     string        // Location of persistence files.
	PersistencePurgeInterval   time.Duration // How often to check for purging.
	PersistenceRetentionPeriod time.Duration // Chunks at least that old are purged.
	CheckpointInterval         time.Duration // How often to checkpoint the series map and head chunks.
	Dirty                      bool          // Force the storage to consider itself dirty on startup.
}

// NewMemorySeriesStorage returns a newly allocated Storage. Storage.Serve still
// has to be called to start the storage.
func NewMemorySeriesStorage(o *MemorySeriesStorageOptions) (Storage, error) {
	p, err := newPersistence(o.PersistenceStoragePath, chunkLen, o.Dirty)
	if err != nil {
		return nil, err
	}
	glog.Info("Loading series map and head chunks...")
	fpToSeries, err := p.loadSeriesMapAndHeads()
	if err != nil {
		return nil, err
	}
	glog.Infof("%d series loaded.", fpToSeries.length())
	numSeries := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "memory_series",
		Help:      "The current number of series in memory.",
	})
	numSeries.Set(float64(fpToSeries.length()))

	return &memorySeriesStorage{
		fpLocker:   newFingerprintLocker(256),
		fpToSeries: fpToSeries,

		loopStopping:       make(chan struct{}),
		loopStopped:        make(chan struct{}),
		evictInterval:      o.MemoryEvictionInterval,
		evictAfter:         o.MemoryRetentionPeriod,
		purgeInterval:      o.PersistencePurgeInterval,
		purgeAfter:         o.PersistenceRetentionPeriod,
		checkpointInterval: o.CheckpointInterval,

		persistQueue:   make(chan persistRequest, persistQueueCap),
		persistStopped: make(chan struct{}),
		persistence:    p,

		persistLatency: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "persist_latency_microseconds",
			Help:      "A summary of latencies for persisting each chunk.",
		}),
		persistErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "persist_errors_total",
				Help:      "A counter of errors persisting chunks.",
			},
			[]string{"error"},
		),
		persistQueueLength: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "persist_queue_length",
			Help:      "The current number of chunks waiting in the persist queue.",
		}),
		numSeries: numSeries,
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
		invalidPreloadRequestsCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "invalid_preload_requests_total",
			Help:      "The total number of preload requests referring to a non-existent series. This is an indication of outdated label indexes.",
		}),
		purgeDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "purge_duration_milliseconds",
			Help:      "The duration of the last storage purge iteration in milliseconds.",
		}),
		evictDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "evict_duration_milliseconds",
			Help:      "The duration of the last memory eviction iteration in milliseconds.",
		}),
	}, nil
}

// Start implements Storage.
func (s *memorySeriesStorage) Start() {
	go s.handlePersistQueue()
	go s.loop()
}

// Stop implements Storage.
func (s *memorySeriesStorage) Stop() error {
	glog.Info("Stopping maintenance loop...")
	close(s.loopStopping)
	<-s.loopStopped

	glog.Info("Stopping persist loop...")
	close(s.persistQueue)
	<-s.persistStopped

	// One final checkpoint of the series map and the head chunks.
	if err := s.persistence.checkpointSeriesMapAndHeads(s.fpToSeries, s.fpLocker); err != nil {
		return err
	}

	if err := s.persistence.close(); err != nil {
		return err
	}
	return nil
}

// WaitForIndexing implements Storage.
func (s *memorySeriesStorage) WaitForIndexing() {
	s.persistence.waitForIndexing()
}

// NewIterator implements storage.
func (s *memorySeriesStorage) NewIterator(fp clientmodel.Fingerprint) SeriesIterator {
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
	return series.newIterator(
		func() { s.fpLocker.Lock(fp) },
		func() { s.fpLocker.Unlock(fp) },
	)
}

// NewPreloader implements Storage.
func (s *memorySeriesStorage) NewPreloader() Preloader {
	return &memorySeriesPreloader{
		storage: s,
	}
}

// GetFingerprintsForLabelMatchers implements Storage.
func (s *memorySeriesStorage) GetFingerprintsForLabelMatchers(labelMatchers metric.LabelMatchers) clientmodel.Fingerprints {
	var result map[clientmodel.Fingerprint]struct{}
	for _, matcher := range labelMatchers {
		intersection := map[clientmodel.Fingerprint]struct{}{}
		switch matcher.Type {
		case metric.Equal:
			fps, err := s.persistence.getFingerprintsForLabelPair(
				metric.LabelPair{
					Name:  matcher.Name,
					Value: matcher.Value,
				},
			)
			if err != nil {
				glog.Error("Error getting fingerprints for label pair: ", err)
			}
			if len(fps) == 0 {
				return nil
			}
			for _, fp := range fps {
				if _, ok := result[fp]; ok || result == nil {
					intersection[fp] = struct{}{}
				}
			}
		default:
			values, err := s.persistence.getLabelValuesForLabelName(matcher.Name)
			if err != nil {
				glog.Errorf("Error getting label values for label name %q: %v", matcher.Name, err)
			}
			matches := matcher.Filter(values)
			if len(matches) == 0 {
				return nil
			}
			for _, v := range matches {
				fps, err := s.persistence.getFingerprintsForLabelPair(
					metric.LabelPair{
						Name:  matcher.Name,
						Value: v,
					},
				)
				if err != nil {
					glog.Error("Error getting fingerprints for label pair: ", err)
				}
				for _, fp := range fps {
					if _, ok := result[fp]; ok || result == nil {
						intersection[fp] = struct{}{}
					}
				}
			}
		}
		if len(intersection) == 0 {
			return nil
		}
		result = intersection
	}

	fps := make(clientmodel.Fingerprints, 0, len(result))
	for fp := range result {
		fps = append(fps, fp)
	}
	return fps
}

// GetLabelValuesForLabelName implements Storage.
func (s *memorySeriesStorage) GetLabelValuesForLabelName(labelName clientmodel.LabelName) clientmodel.LabelValues {
	lvs, err := s.persistence.getLabelValuesForLabelName(labelName)
	if err != nil {
		glog.Errorf("Error getting label values for label name %q: %v", labelName, err)
	}
	return lvs
}

// GetMetricForFingerprint implements Storage.
func (s *memorySeriesStorage) GetMetricForFingerprint(fp clientmodel.Fingerprint) clientmodel.Metric {
	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	series, ok := s.fpToSeries.get(fp)
	if ok {
		// Copy required here because caller might mutate the returned
		// metric.
		m := make(clientmodel.Metric, len(series.metric))
		for ln, lv := range series.metric {
			m[ln] = lv
		}
		return m
	}
	metric, err := s.persistence.getArchivedMetric(fp)
	if err != nil {
		glog.Errorf("Error retrieving archived metric for fingerprint %v: %v", fp, err)
	}
	return metric
}

// AppendSamples implements Storage.
func (s *memorySeriesStorage) AppendSamples(samples clientmodel.Samples) {
	for _, sample := range samples {
		s.appendSample(sample)
	}

	s.ingestedSamplesCount.Add(float64(len(samples)))
}

func (s *memorySeriesStorage) appendSample(sample *clientmodel.Sample) {
	fp := sample.Metric.Fingerprint()
	s.fpLocker.Lock(fp)
	series := s.getOrCreateSeries(fp, sample.Metric)
	chunkDescsToPersist := series.add(fp, &metric.SamplePair{
		Value:     sample.Value,
		Timestamp: sample.Timestamp,
	})
	s.fpLocker.Unlock(fp)
	// Queue only outside of the locked area, processing the persistQueue
	// requires the same lock!
	for _, cd := range chunkDescsToPersist {
		s.persistQueue <- persistRequest{fp, cd}
	}
}

func (s *memorySeriesStorage) getOrCreateSeries(fp clientmodel.Fingerprint, m clientmodel.Metric) *memorySeries {
	series, ok := s.fpToSeries.get(fp)
	if !ok {
		unarchived, firstTime, err := s.persistence.unarchiveMetric(fp)
		if err != nil {
			glog.Errorf("Error unarchiving fingerprint %v: %v", fp, err)
			s.persistence.setDirty(true)
		}
		if unarchived {
			s.seriesOps.WithLabelValues(unarchive).Inc()
		} else {
			// This was a genuinely new series, so index the metric.
			s.persistence.indexMetric(fp, m)
			s.seriesOps.WithLabelValues(create).Inc()
		}
		series = newMemorySeries(m, !unarchived, firstTime)
		s.fpToSeries.put(fp, series)
		s.numSeries.Inc()
	}
	return series
}

/*
func (s *memorySeriesStorage) preloadChunksAtTime(fp clientmodel.Fingerprint, ts clientmodel.Timestamp) (chunkDescs, error) {
	series, ok := s.fpToSeries.get(fp)
	if !ok {
		panic("requested preload for non-existent series")
	}
	return series.preloadChunksAtTime(ts, s.persistence)
}
*/

func (s *memorySeriesStorage) preloadChunksForRange(
	fp clientmodel.Fingerprint,
	from clientmodel.Timestamp, through clientmodel.Timestamp,
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
			metric, err := s.persistence.getArchivedMetric(fp)
			if err != nil {
				return nil, err
			}
			series = s.getOrCreateSeries(fp, metric)
		} else {
			return nil, nil
		}
	}
	return series.preloadChunksForRange(from, through, fp, s.persistence)
}

func (s *memorySeriesStorage) handlePersistQueue() {
	for req := range s.persistQueue {
		s.persistQueueLength.Set(float64(len(s.persistQueue)))
		start := time.Now()
		s.fpLocker.Lock(req.fingerprint)
		offset, err := s.persistence.persistChunk(req.fingerprint, req.chunkDesc.chunk)
		if series, seriesInMemory := s.fpToSeries.get(req.fingerprint); err == nil && seriesInMemory && series.chunkDescsOffset == -1 {
			// This is the first chunk persisted for a newly created
			// series that had prior chunks on disk. Finally, we can
			// set the chunkDescsOffset.
			series.chunkDescsOffset = offset
		}
		s.fpLocker.Unlock(req.fingerprint)
		s.persistLatency.Observe(float64(time.Since(start)) / float64(time.Microsecond))
		if err != nil {
			s.persistErrors.WithLabelValues(err.Error()).Inc()
			glog.Error("Error persisting chunk: ", err)
			s.persistence.setDirty(true)
			continue
		}
		req.chunkDesc.unpin()
		chunkOps.WithLabelValues(persistAndUnpin).Inc()
	}
	glog.Info("Persist loop stopped.")
	close(s.persistStopped)
}

func (s *memorySeriesStorage) loop() {
	evictTicker := time.NewTicker(s.evictInterval)
	purgeTicker := time.NewTicker(s.purgeInterval)
	checkpointTicker := time.NewTicker(s.checkpointInterval)
	defer func() {
		evictTicker.Stop()
		purgeTicker.Stop()
		checkpointTicker.Stop()
		glog.Info("Maintenance loop stopped.")
		close(s.loopStopped)
	}()

	for {
		select {
		case <-s.loopStopping:
			return
		case <-checkpointTicker.C:
			s.persistence.checkpointSeriesMapAndHeads(s.fpToSeries, s.fpLocker)
		case <-evictTicker.C:
			// TODO: Change this to be based on number of chunks in memory.
			glog.Info("Evicting chunks...")
			begin := time.Now()

			for m := range s.fpToSeries.iter() {
				select {
				case <-s.loopStopping:
					glog.Info("Interrupted evicting chunks.")
					return
				default:
					// Keep going.
				}
				s.fpLocker.Lock(m.fp)
				allEvicted, headChunkToPersist := m.series.evictOlderThan(
					clientmodel.TimestampFromTime(time.Now()).Add(-1 * s.evictAfter),
				)
				if allEvicted {
					s.fpToSeries.del(m.fp)
					s.numSeries.Dec()
					if err := s.persistence.archiveMetric(
						m.fp, m.series.metric, m.series.firstTime(), m.series.lastTime(),
					); err != nil {
						glog.Errorf("Error archiving metric %v: %v", m.series.metric, err)
					} else {
						s.seriesOps.WithLabelValues(archive).Inc()
					}
				}
				s.fpLocker.Unlock(m.fp)
				// Queue outside of lock!
				if headChunkToPersist != nil {
					s.persistQueue <- persistRequest{m.fp, headChunkToPersist}
				}
			}

			duration := time.Since(begin)
			s.evictDuration.Set(float64(duration) / float64(time.Millisecond))
			glog.Infof("Done evicting chunks in %v.", duration)
		case <-purgeTicker.C:
			glog.Info("Purging old series data...")
			ts := clientmodel.TimestampFromTime(time.Now()).Add(-1 * s.purgeAfter)
			begin := time.Now()

			for fp := range s.fpToSeries.fpIter() {
				select {
				case <-s.loopStopping:
					glog.Info("Interrupted purging series.")
					return
				default:
					s.purgeSeries(fp, ts)
				}
			}

			persistedFPs, err := s.persistence.getFingerprintsModifiedBefore(ts)
			if err != nil {
				glog.Error("Failed to lookup persisted fingerprint ranges: ", err)
				break
			}
			for _, fp := range persistedFPs {
				select {
				case <-s.loopStopping:
					glog.Info("Interrupted purging series.")
					return
				default:
					s.purgeSeries(fp, ts)
				}
			}
			duration := time.Since(begin)
			s.purgeDuration.Set(float64(duration) / float64(time.Millisecond))
			glog.Infof("Done purging old series data in %v.", duration)
		}
	}
}

// purgeSeries purges chunks older than beforeTime from a series. If the series
// contains no chunks after the purge, it is dropped entirely.
func (s *memorySeriesStorage) purgeSeries(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) {
	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	if series, ok := s.fpToSeries.get(fp); ok {
		// Deal with series in memory.
		if !series.firstTime().Before(beforeTime) {
			// Oldest sample not old enough.
			return
		}
		newFirstTime, numDropped, allDropped, err := s.persistence.dropChunks(fp, beforeTime)
		if err != nil {
			glog.Error("Error purging persisted chunks: ", err)
		}
		numPurged, allPurged := series.purgeOlderThan(beforeTime)
		if allPurged && allDropped {
			s.fpToSeries.del(fp)
			s.numSeries.Dec()
			s.seriesOps.WithLabelValues(memoryPurge).Inc()
			s.persistence.unindexMetric(fp, series.metric)
		} else if series.chunkDescsOffset != -1 {
			series.savedFirstTime = newFirstTime
			series.chunkDescsOffset += numPurged - numDropped
			if series.chunkDescsOffset < 0 {
				panic("dropped more chunks from persistence than from memory")
			}
		}
		return
	}

	// Deal with archived series.
	has, firstTime, lastTime, err := s.persistence.hasArchivedMetric(fp)
	if err != nil {
		glog.Error("Error looking up archived time range: ", err)
		return
	}
	if !has || !firstTime.Before(beforeTime) {
		// Oldest sample not old enough, or metric purged or unarchived in the meantime.
		return
	}

	newFirstTime, _, allDropped, err := s.persistence.dropChunks(fp, beforeTime)
	if err != nil {
		glog.Error("Error purging persisted chunks: ", err)
	}
	if allDropped {
		if err := s.persistence.dropArchivedMetric(fp); err != nil {
			glog.Errorf("Error dropping archived metric for fingerprint %v: %v", fp, err)
			return
		}
		s.seriesOps.WithLabelValues(archivePurge).Inc()
		return
	}
	s.persistence.updateArchivedTimeRange(fp, newFirstTime, lastTime)
}

// To expose persistQueueCap as metric:
var (
	persistQueueCapDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, "persist_queue_capacity"),
		"The total capacity of the persist queue.",
		nil, nil,
	)
	persistQueueCapGauge = prometheus.MustNewConstMetric(
		persistQueueCapDesc, prometheus.GaugeValue, persistQueueCap,
	)
)

// Describe implements prometheus.Collector.
func (s *memorySeriesStorage) Describe(ch chan<- *prometheus.Desc) {
	s.persistence.Describe(ch)

	ch <- s.persistLatency.Desc()
	s.persistErrors.Describe(ch)
	ch <- s.persistQueueLength.Desc()
	ch <- s.numSeries.Desc()
	s.seriesOps.Describe(ch)
	ch <- s.ingestedSamplesCount.Desc()
	ch <- s.invalidPreloadRequestsCount.Desc()
	ch <- s.purgeDuration.Desc()
	ch <- s.evictDuration.Desc()

	ch <- persistQueueCapDesc

	ch <- numMemChunksDesc
	ch <- numMemChunkDescsDesc
}

// Collect implements prometheus.Collector.
func (s *memorySeriesStorage) Collect(ch chan<- prometheus.Metric) {
	s.persistence.Collect(ch)

	ch <- s.persistLatency
	s.persistErrors.Collect(ch)
	ch <- s.persistQueueLength
	ch <- s.numSeries
	s.seriesOps.Collect(ch)
	ch <- s.ingestedSamplesCount
	ch <- s.invalidPreloadRequestsCount
	ch <- s.purgeDuration
	ch <- s.evictDuration

	ch <- persistQueueCapGauge

	count := atomic.LoadInt64(&numMemChunks)
	ch <- prometheus.MustNewConstMetric(numMemChunksDesc, prometheus.GaugeValue, float64(count))
	count = atomic.LoadInt64(&numMemChunkDescs)
	ch <- prometheus.MustNewConstMetric(numMemChunkDescsDesc, prometheus.GaugeValue, float64(count))
}
