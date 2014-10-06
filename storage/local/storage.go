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
	"sync"
	"time"

	"github.com/golang/glog"

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
	mtx sync.RWMutex

	state       storageState
	persistDone chan bool
	stopServing chan chan<- bool

	fingerprintToSeries SeriesMap

	memoryEvictionInterval time.Duration
	memoryRetentionPeriod  time.Duration

	persistencePurgeInterval   time.Duration
	persistenceRetentionPeriod time.Duration

	persistQueue chan *persistRequest
	persistence  Persistence
}

// MemorySeriesStorageOptions contains options needed by
// NewMemorySeriesStorage. It is not safe to leave any of those at their zero
// values.
type MemorySeriesStorageOptions struct {
	Persistence                Persistence   // Used to persist storage content across restarts.
	MemoryEvictionInterval     time.Duration // How often to check for memory eviction.
	MemoryRetentionPeriod      time.Duration // Chunks at least that old are evicted from memory.
	PersistencePurgeInterval   time.Duration // How often to check for purging.
	PersistenceRetentionPeriod time.Duration // Chunks at least that old are purged.
}

// NewMemorySeriesStorage returns a newly allocated Storage. Storage.Serve still
// has to be called to start the storage.
func NewMemorySeriesStorage(o *MemorySeriesStorageOptions) (Storage, error) {
	glog.Info("Loading series map and head chunks...")
	fingerprintToSeries, err := o.Persistence.LoadSeriesMapAndHeads()
	if err != nil {
		return nil, err
	}
	numSeries.Set(float64(len(fingerprintToSeries)))

	return &memorySeriesStorage{
		fingerprintToSeries: fingerprintToSeries,
		persistDone:         make(chan bool),
		stopServing:         make(chan chan<- bool),

		memoryEvictionInterval: o.MemoryEvictionInterval,
		memoryRetentionPeriod:  o.MemoryRetentionPeriod,

		persistencePurgeInterval:   o.PersistencePurgeInterval,
		persistenceRetentionPeriod: o.PersistenceRetentionPeriod,

		persistQueue: make(chan *persistRequest, persistQueueCap),
		persistence:  o.Persistence,
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
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.state != storageServing {
		panic("storage is not serving")
	}

	fp := sample.Metric.Fingerprint()
	series := s.getOrCreateSeries(fp, sample.Metric)
	series.add(fp, &metric.SamplePair{
		Value:     sample.Value,
		Timestamp: sample.Timestamp,
	}, s.persistQueue)
}

func (s *memorySeriesStorage) getOrCreateSeries(fp clientmodel.Fingerprint, m clientmodel.Metric) *memorySeries {
	series, ok := s.fingerprintToSeries[fp]

	if !ok {
		series = newMemorySeries(m)
		s.fingerprintToSeries[fp] = series
		numSeries.Set(float64(len(s.fingerprintToSeries)))

		unarchived, err := s.persistence.UnarchiveMetric(fp)
		if err != nil {
			glog.Errorf("Error unarchiving fingerprint %v: %v", fp, err)
		}

		if unarchived {
			// The series existed before, had been archived at some
			// point, and has now been unarchived, i.e. it has
			// chunks on disk. Set chunkDescsLoaded accordingly so
			// that they will be looked at later. Also, an
			// unarchived series comes with a persisted head chunk.
			series.chunkDescsLoaded = false
			series.headChunkPersisted = true
		} else {
			// This was a genuinely new series, so index the metric.
			s.persistence.IndexMetric(m, fp)
		}
	}
	return series
}

/*
func (s *memorySeriesStorage) preloadChunksAtTime(fp clientmodel.Fingerprint, ts clientmodel.Timestamp) (chunkDescs, error) {
	series, ok := s.fingerprintToSeries[fp]
	if !ok {
		panic("requested preload for non-existent series")
	}
	return series.preloadChunksAtTime(ts, s.persistence)
}
*/

func (s *memorySeriesStorage) preloadChunksForRange(fp clientmodel.Fingerprint, from clientmodel.Timestamp, through clientmodel.Timestamp) (chunkDescs, error) {
	stalenessDelta := 300 * time.Second // TODO: Turn into parameter.

	s.mtx.RLock()
	series, ok := s.fingerprintToSeries[fp]
	s.mtx.RUnlock()

	if !ok {
		has, first, last, err := s.persistence.HasArchivedMetric(fp)
		if err != nil {
			return nil, err
		}
		if !has {
			return nil, fmt.Errorf("requested preload for non-existent series %v", fp)
		}
		if from.Add(-stalenessDelta).Before(last) && through.Add(stalenessDelta).After(first) {
			metric, err := s.persistence.GetArchivedMetric(fp)
			if err != nil {
				return nil, err
			}
			s.mtx.Lock()
			series = s.getOrCreateSeries(fp, metric)
			defer s.mtx.Unlock() // Ugh.
		}
	}
	return series.preloadChunksForRange(from, through, s.persistence)
}

// NewIterator implements storage.
func (s *memorySeriesStorage) NewIterator(fp clientmodel.Fingerprint) SeriesIterator {
	s.mtx.RLock()
	series, ok := s.fingerprintToSeries[fp]
	s.mtx.RUnlock()

	if !ok {
		panic("requested iterator for non-existent series")
	}
	return series.newIterator()
}

func (s *memorySeriesStorage) evictMemoryChunks(ttl time.Duration) {
	fspsToArchive := []FingerprintSeriesPair{}

	defer func(begin time.Time) {
		evictionDuration.Set(float64(time.Since(begin) / time.Millisecond))
	}(time.Now())

	s.mtx.RLock()
	for fp, series := range s.fingerprintToSeries {
		if series.evictOlderThan(clientmodel.TimestampFromTime(time.Now()).Add(-1 * ttl)) {
			fspsToArchive = append(fspsToArchive, FingerprintSeriesPair{
				Fingerprint: fp,
				Series:      series,
			})
		}
		series.persistHeadChunk(fp, s.persistQueue)
	}
	s.mtx.RUnlock()

	if len(fspsToArchive) == 0 {
		return
	}

	// If we are here, we have metrics to archive. For that, we need the write lock.
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, fsp := range fspsToArchive {
		// TODO: Need series lock (or later FP lock)?
		if !fsp.Series.headChunkPersisted {
			// Oops. The series has received new samples all of a
			// sudden, giving it a new head chunk. Leave it alone.
			return
		}
		if err := s.persistence.ArchiveMetric(
			fsp.Fingerprint, fsp.Series.metric, fsp.Series.firstTime(), fsp.Series.lastTime(),
		); err != nil {
			glog.Errorf("Error archiving metric %v: %v", fsp.Series.metric, err)
		}
		delete(s.fingerprintToSeries, fsp.Fingerprint)
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
		// TODO: Make this thread-safe?
		persistQueueLength.Set(float64(len(s.persistQueue)))

		//glog.Info("Persist request: ", *req.fingerprint)
		start := time.Now()
		err := s.persistence.PersistChunk(req.fingerprint, req.chunkDesc.chunk)
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
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.state == storageStopping {
		panic("Illegal State: Attempted to restop memorySeriesStorage.")
	}

	stopped := make(chan bool)
	glog.Info("Waiting for storage to stop serving...")
	s.stopServing <- (stopped)
	glog.Info("Serving stopped.")
	<-stopped

	glog.Info("Stopping persist loop...")
	close(s.persistQueue)
	<-s.persistDone
	glog.Info("Persist loop stopped.")

	glog.Info("Persisting head chunks...")
	if err := s.persistence.PersistSeriesMapAndHeads(s.fingerprintToSeries); err != nil {
		return err
	}
	glog.Info("Done persisting head chunks.")

	s.fingerprintToSeries = nil
	if err := s.persistence.Close(); err != nil {
		return err
	}

	s.state = storageStopping
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
			s.mtx.RLock()

			begin := time.Now()

			fps := make([]clientmodel.Fingerprint, 0, len(s.fingerprintToSeries))
			for fp := range s.fingerprintToSeries {
				fps = append(fps, fp)
			}
			s.mtx.RUnlock()

			ts := clientmodel.TimestampFromTime(time.Now()).Add(-1 * s.persistenceRetentionPeriod)

			// TODO: If we decide not to remove entries from the timerange disk index
			// upon unarchival, we could remove the memory copy above and only use
			// the fingerprints from the disk index.
			persistedFPs, err := s.persistence.GetFingerprintsModifiedBefore(ts)
			if err != nil {
				glog.Error("Failed to lookup persisted fingerprint ranges: ", err)
				break
			}
			fps = append(fps, persistedFPs...)

			for _, fp := range fps {
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
	s.mtx.Lock()
	// TODO: This is a lock FAR to coarse! However, we cannot lock using the
	// memorySeries since we might have none (for series that are on disk
	// only). And we really don't want to un-archive a series from disk
	// while we are at the same time purging it. A locking per fingerprint
	// would be nice. Or something... Have to think about it... Careful,
	// more race conditions lurk below. Also unsolved: If there are chunks
	// in the persist queue. persistence.DropChunks and
	// persistence.PersistChunck needs to be locked on fp level, or
	// something. And even then, what happens if everything is dropped, but
	// there are still chunks hung in the persist queue? They would later
	// re-create a file for a series that doesn't exist anymore...  But
	// there is the ref count, which is one higher if you have not yet
	// persisted the chunk.
	defer s.mtx.Unlock()

	// First purge persisted chunks. We need to do that anyway.
	allDropped, err := s.persistence.DropChunks(fp, beforeTime)
	if err != nil {
		glog.Error("Error purging persisted chunks: ", err)
	}

	// Purge chunks from memory accordingly.
	if series, ok := s.fingerprintToSeries[fp]; ok {
		if series.purgeOlderThan(beforeTime) && allDropped {
			delete(s.fingerprintToSeries, fp)
			s.persistence.UnindexMetric(series.metric, fp)
		}
		return
	}

	// If we arrive here, nothing was in memory, so the metric must have
	// been archived. Drop the archived metric if there are no persisted
	// chunks left.
	if !allDropped {
		return
	}
	if err := s.persistence.DropArchivedMetric(fp); err != nil {
		glog.Errorf("Error dropping archived metric for fingerprint %v: %v", fp, err)
	}
}

// Serve implements Storage.
func (s *memorySeriesStorage) Serve(started chan<- bool) {
	s.mtx.Lock()
	if s.state != storageStarting {
		panic("Illegal State: Attempted to restart memorySeriesStorage.")
	}
	s.state = storageServing
	s.mtx.Unlock()

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
	// TODO: Is this lock needed?
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	var result map[clientmodel.Fingerprint]struct{}
	for _, matcher := range labelMatchers {
		intersection := map[clientmodel.Fingerprint]struct{}{}
		switch matcher.Type {
		case metric.Equal:
			fps, err := s.persistence.GetFingerprintsForLabelPair(
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
			values, err := s.persistence.GetLabelValuesForLabelName(matcher.Name)
			if err != nil {
				glog.Errorf("Error getting label values for label name %q: %v", matcher.Name, err)
			}
			matches := matcher.Filter(values)
			if len(matches) == 0 {
				return nil
			}
			for _, v := range matches {
				fps, err := s.persistence.GetFingerprintsForLabelPair(
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
	// TODO: Is this lock needed?
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	lvs, err := s.persistence.GetLabelValuesForLabelName(labelName)
	if err != nil {
		glog.Errorf("Error getting label values for label name %q: %v", labelName, err)
	}
	return lvs
}

// GetMetricForFingerprint implements Storage.
func (s *memorySeriesStorage) GetMetricForFingerprint(fp clientmodel.Fingerprint) clientmodel.Metric {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	series, ok := s.fingerprintToSeries[fp]
	if ok {
		// Copy required here because caller might mutate the returned
		// metric.
		m := make(clientmodel.Metric, len(series.metric))
		for ln, lv := range series.metric {
			m[ln] = lv
		}
		return m
	}
	metric, err := s.persistence.GetArchivedMetric(fp)
	if err != nil {
		glog.Errorf("Error retrieving archived metric for fingerprint %v: %v", fp, err)
	}
	return metric
}
