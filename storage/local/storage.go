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
	"time"

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"
	"github.com/prometheus/client_golang/prometheus"
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

	numSamples.Add(float64(len(samples)))
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
		if !unarchived {
			// This was a genuinely new series, so index the metric.
			s.persistence.indexMetric(m, fp)
		}
		series = newMemorySeries(m, !unarchived)
		s.fingerprintToSeries.put(fp, series)
		numSeries.Set(float64(s.fingerprintToSeries.length()))

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

func (s *memorySeriesStorage) preloadChunksForRange(fp clientmodel.Fingerprint, from clientmodel.Timestamp, through clientmodel.Timestamp) (chunkDescs, error) {
	stalenessDelta := 300 * time.Second // TODO: Turn into parameter.
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
		}
	}
	return series.preloadChunksForRange(from, through, s.persistence)
}

// NewIterator implements storage.
func (s *memorySeriesStorage) NewIterator(fp clientmodel.Fingerprint) SeriesIterator {
	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	series, ok := s.fingerprintToSeries.get(fp)
	if !ok {
		// TODO: Could this legitimately happen? Series just got purged?
		panic("requested iterator for non-existent series")
	}
	return series.newIterator(
		func() { s.fpLocker.Lock(fp) },
		func() { s.fpLocker.Unlock(fp) },
	)
}

func (s *memorySeriesStorage) evictMemoryChunks(ttl time.Duration) {
	defer func(begin time.Time) {
		evictionDuration.Set(float64(time.Since(begin) / time.Millisecond))
	}(time.Now())

	for m := range s.fingerprintToSeries.iter() {
		s.fpLocker.Lock(m.fp)
		if m.series.evictOlderThan(clientmodel.TimestampFromTime(time.Now()).Add(-1 * ttl)) {
			m.series.persistHeadChunk(m.fp, s.persistQueue)
			s.fingerprintToSeries.del(m.fp)
			if err := s.persistence.archiveMetric(
				m.fp, m.series.metric, m.series.firstTime(), m.series.lastTime(),
			); err != nil {
				glog.Errorf("Error archiving metric %v: %v", m.series.metric, err)
			}
		}
		s.fpLocker.Unlock(m.fp)
	}
}

func recordPersist(start time.Time, err error) {
	outcome := success
	if err != nil {
		outcome = failure
	}
	persistLatencies.WithLabelValues(outcome).Observe(float64(time.Since(start) / time.Millisecond))
}

func (s *memorySeriesStorage) handlePersistQueue() {
	for req := range s.persistQueue {
		persistQueueLength.Set(float64(len(s.persistQueue)))

		//glog.Info("Persist request: ", *req.fingerprint)
		start := time.Now()
		err := s.persistence.persistChunk(req.fingerprint, req.chunkDesc.chunk)
		recordPersist(start, err)
		if err != nil {
			glog.Error("Error persisting chunk, requeuing: ", err)
			s.persistQueue <- req
			continue
		}
		req.chunkDesc.unpin()
	}
	s.persistDone <- true
}

// Close stops serving, flushes all pending operations, and frees all
// resources. It implements Storage.
func (s *memorySeriesStorage) Close() error {
	stopped := make(chan bool)
	glog.Info("Waiting for storage to stop serving...")
	s.stopServing <- stopped
	glog.Info("Serving stopped.")
	<-stopped

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
					// TODO: Decide whether we also want to adjust the timerange index
					// entries here. Not updating them shouldn't break anything, but will
					// make some scenarios less efficient.
					s.purgeSeries(fp, ts)
				}
			}
			purgeDuration.Set(float64(time.Since(begin) / time.Millisecond))
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
			s.persistence.unindexMetric(series.metric, fp)
		}
		return
	}

	// If we arrive here, nothing was in memory, so the metric must have
	// been archived. Drop the archived metric if there are no persisted
	// chunks left.
	if !allDropped {
		return
	}
	if err := s.persistence.dropArchivedMetric(fp); err != nil {
		glog.Errorf("Error dropping archived metric for fingerprint %v: %v", fp, err)
	}
}

// Serve implements Storage.
func (s *memorySeriesStorage) Serve(started chan<- bool) {
	evictMemoryTicker := time.NewTicker(s.memoryEvictionInterval)
	defer evictMemoryTicker.Stop()

	go s.handlePersistQueue()

	stopPurge := make(chan bool)
	go s.purgePeriodically(stopPurge)

	started <- true
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

// Describe implements prometheus.Collector.
func (s *memorySeriesStorage) Describe(ch chan<- *prometheus.Desc) {
	s.persistence.Describe(ch)
}

// Collect implements prometheus.Collector.
func (s *memorySeriesStorage) Collect(ch chan<- prometheus.Metric) {
	s.persistence.Collect(ch)
}
