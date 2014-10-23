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
	"fmt"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

const persistQueueCap = 1024

type storageState uint

const (
	storageStarting storageState = iota
	storageServing
	storageStopping
)

type memorySeriesStorage struct {
	fpLocker *fingerprintLocker

	persistDone chan bool
	stopServing chan chan<- bool

	fingerprintToSeries *seriesMap

	memoryEvictionInterval time.Duration
	memoryRetentionPeriod  time.Duration

	persistencePurgeInterval   time.Duration
	persistenceRetentionPeriod time.Duration

	persistQueue chan *persistRequest
	persistence  *persistence

	persistLatency                  prometheus.Summary
	persistErrors                   *prometheus.CounterVec
	persistQueueLength              prometheus.Gauge
	numSeries                       prometheus.Gauge
	seriesOps                       *prometheus.CounterVec
	ingestedSamplesCount            prometheus.Counter
	purgeDuration, evictionDuration prometheus.Gauge
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
}

// NewMemorySeriesStorage returns a newly allocated Storage. Storage.Serve still
// has to be called to start the storage.
func NewMemorySeriesStorage(o *MemorySeriesStorageOptions) (Storage, error) {
	p, err := newPersistence(o.PersistenceStoragePath, 1024)
	if err != nil {
		return nil, err
	}
	glog.Info("Loading series map and head chunks...")
	fingerprintToSeries, err := p.loadSeriesMapAndHeads()
	if err != nil {
		return nil, err
	}
	glog.Infof("%d series loaded.", fingerprintToSeries.length())
	numSeries := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "memory_series",
		Help:      "The current number of series in memory.",
	})
	numSeries.Set(float64(fingerprintToSeries.length()))

	return &memorySeriesStorage{
		fpLocker: newFingerprintLocker(100), // TODO: Tweak value.

		fingerprintToSeries: fingerprintToSeries,
		persistDone:         make(chan bool),
		stopServing:         make(chan chan<- bool),

		memoryEvictionInterval: o.MemoryEvictionInterval,
		memoryRetentionPeriod:  o.MemoryRetentionPeriod,

		persistencePurgeInterval:   o.PersistencePurgeInterval,
		persistenceRetentionPeriod: o.PersistenceRetentionPeriod,

		persistQueue: make(chan *persistRequest, persistQueueCap),
		persistence:  p,

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
		purgeDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "purge_duration_milliseconds",
			Help:      "The duration of the last storage purge iteration in milliseconds.",
		}),
		evictionDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "eviction_duration_milliseconds",
			Help:      "The duration of the last memory eviction iteration in milliseconds.",
		}),
	}, nil
}

type persistRequest struct {
	fingerprint clientmodel.Fingerprint
	chunkDesc   *chunkDesc
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
	defer s.fpLocker.Unlock(fp)

	series := s.getOrCreateSeries(fp, sample.Metric)
	series.add(fp, &metric.SamplePair{
		Value:     sample.Value,
		Timestamp: sample.Timestamp,
	}, s.persistQueue)
}

func (s *memorySeriesStorage) getOrCreateSeries(fp clientmodel.Fingerprint, m clientmodel.Metric) *memorySeries {
	series, ok := s.fingerprintToSeries.get(fp)
	if !ok {
		unarchived, err := s.persistence.unarchiveMetric(fp)
		if err != nil {
			glog.Errorf("Error unarchiving fingerprint %v: %v", fp, err)
		}
		if unarchived {
			s.seriesOps.WithLabelValues(unarchive).Inc()
		} else {
			// This was a genuinely new series, so index the metric.
			s.persistence.indexMetric(m, fp)
			s.seriesOps.WithLabelValues(create).Inc()
		}
		series = newMemorySeries(m, !unarchived)
		s.fingerprintToSeries.put(fp, series)
		s.numSeries.Inc()
	}
	return series
}

/*
func (s *memorySeriesStorage) preloadChunksAtTime(fp clientmodel.Fingerprint, ts clientmodel.Timestamp) (chunkDescs, error) {
	series, ok := s.fingerprintToSeries.get(fp)
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

	series, ok := s.fingerprintToSeries.get(fp)
	if !ok {
		has, first, last, err := s.persistence.hasArchivedMetric(fp)
		if err != nil {
			return nil, err
		}
		if !has {
			return nil, fmt.Errorf("requested preload for non-existent series %v", fp)
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

// NewIterator implements storage.
func (s *memorySeriesStorage) NewIterator(fp clientmodel.Fingerprint) SeriesIterator {
	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	series, ok := s.fingerprintToSeries.get(fp)
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

func (s *memorySeriesStorage) evictMemoryChunks(ttl time.Duration) {
	defer func(begin time.Time) {
		s.evictionDuration.Set(float64(time.Since(begin)) / float64(time.Millisecond))
	}(time.Now())

	for m := range s.fingerprintToSeries.iter() {
		s.fpLocker.Lock(m.fp)
		if m.series.evictOlderThan(
			clientmodel.TimestampFromTime(time.Now()).Add(-1*ttl),
			m.fp, s.persistQueue,
		) {
			s.fingerprintToSeries.del(m.fp)
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
	}
}

func (s *memorySeriesStorage) handlePersistQueue() {
	for req := range s.persistQueue {
		s.persistQueueLength.Set(float64(len(s.persistQueue)))
		start := time.Now()
		err := s.persistence.persistChunk(req.fingerprint, req.chunkDesc.chunk)
		s.persistLatency.Observe(float64(time.Since(start)) / float64(time.Microsecond))
		if err != nil {
			s.persistErrors.WithLabelValues(err.Error()).Inc()
			glog.Error("Error persisting chunk, requeuing: ", err)
			s.persistQueue <- req
			continue
		}
		req.chunkDesc.unpin()
		chunkOps.WithLabelValues(persistAndUnpin).Inc()
	}
	s.persistDone <- true
}

// WaitForIndexing implements Storage.
func (s *memorySeriesStorage) WaitForIndexing() {
	s.persistence.waitForIndexing()
}

// Close stops serving, flushes all pending operations, and frees all
// resources. It implements Storage.
func (s *memorySeriesStorage) Close() error {
	stopped := make(chan bool)
	glog.Info("Waiting for storage to stop serving...")
	s.stopServing <- stopped
	<-stopped
	glog.Info("Serving stopped.")

	glog.Info("Stopping persist loop...")
	close(s.persistQueue)
	<-s.persistDone
	glog.Info("Persist loop stopped.")

	glog.Info("Persisting head chunks...")
	if err := s.persistence.persistSeriesMapAndHeads(s.fingerprintToSeries); err != nil {
		return err
	}
	glog.Info("Done persisting head chunks.")

	if err := s.persistence.close(); err != nil {
		return err
	}
	return nil
}

func (s *memorySeriesStorage) purgePeriodically(stop <-chan bool) {
	purgeTicker := time.NewTicker(s.persistencePurgeInterval)
	defer purgeTicker.Stop()

	for {
		select {
		case <-stop:
			glog.Info("Purging loop stopped.")
			return
		case <-purgeTicker.C:
			glog.Info("Purging old series data...")
			ts := clientmodel.TimestampFromTime(time.Now()).Add(-1 * s.persistenceRetentionPeriod)
			begin := time.Now()

			for fp := range s.fingerprintToSeries.fpIter() {
				select {
				case <-stop:
					glog.Info("Interrupted running series purge.")
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
				case <-stop:
					glog.Info("Interrupted running series purge.")
					return
				default:
					s.purgeSeries(fp, ts)
				}
			}
			s.purgeDuration.Set(float64(time.Since(begin)) / float64(time.Millisecond))
			glog.Info("Done purging old series data.")
		}
	}
}

// purgeSeries purges chunks older than persistenceRetentionPeriod from a
// series. If the series contains no chunks after the purge, it is dropped
// entirely.
func (s *memorySeriesStorage) purgeSeries(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) {
	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	// First purge persisted chunks. We need to do that anyway.
	allDropped, err := s.persistence.dropChunks(fp, beforeTime)
	if err != nil {
		glog.Error("Error purging persisted chunks: ", err)
	}

	// Purge chunks from memory accordingly.
	if series, ok := s.fingerprintToSeries.get(fp); ok {
		if series.purgeOlderThan(beforeTime) && allDropped {
			s.fingerprintToSeries.del(fp)
			s.numSeries.Dec()
			s.seriesOps.WithLabelValues(memoryPurge).Inc()
			s.persistence.unindexMetric(series.metric, fp)
		}
		return
	}

	// If we arrive here, nothing was in memory, so the metric must have
	// been archived. Drop the archived metric if there are no persisted
	// chunks left. If we don't drop the archived metric, we should update
	// the archivedFingerprintToTimeRange index according to the remaining
	// chunks, but it's probably not worth the effort. Queries going beyond
	// the purge cut-off can be truncated in a more direct fashion.
	if allDropped {
		if err := s.persistence.dropArchivedMetric(fp); err != nil {
			glog.Errorf("Error dropping archived metric for fingerprint %v: %v", fp, err)
		} else {
			s.seriesOps.WithLabelValues(archivePurge).Inc()
		}
	}
}

// Serve implements Storage.
func (s *memorySeriesStorage) Serve(started chan struct{}) {
	evictMemoryTicker := time.NewTicker(s.memoryEvictionInterval)
	defer evictMemoryTicker.Stop()

	go s.handlePersistQueue()

	stopPurge := make(chan bool)
	go s.purgePeriodically(stopPurge)

	close(started)
	for {
		select {
		case <-evictMemoryTicker.C:
			s.evictMemoryChunks(s.memoryRetentionPeriod)
		case stopped := <-s.stopServing:
			stopPurge <- true
			stopped <- true
			return
		}
	}
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

	series, ok := s.fingerprintToSeries.get(fp)
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
	ch <- s.purgeDuration.Desc()
	ch <- s.evictionDuration.Desc()

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
	ch <- s.purgeDuration
	ch <- s.evictionDuration

	ch <- persistQueueCapGauge

	count := atomic.LoadInt64(&numMemChunks)
	ch <- prometheus.MustNewConstMetric(numMemChunksDesc, prometheus.GaugeValue, float64(count))
	count = atomic.LoadInt64(&numMemChunkDescs)
	ch <- prometheus.MustNewConstMetric(numMemChunkDescsDesc, prometheus.GaugeValue, float64(count))
}
