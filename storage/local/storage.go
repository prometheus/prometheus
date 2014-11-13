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
	"container/list"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

const (
	persistQueueCap  = 1024
	evictRequestsCap = 1024
	chunkLen         = 1024

	// See waitForNextFP.
	fpMaxWaitDuration = 10 * time.Second
	fpMinWaitDuration = 5 * time.Millisecond // ~ hard disk seek time.
	fpMaxSweepTime    = 6 * time.Hour

	maxEvictInterval = time.Minute
	headChunkTimeout = time.Hour // Close head chunk if not touched for that long.
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

type evictRequest struct {
	cd    *chunkDesc
	evict bool
}

type memorySeriesStorage struct {
	fpLocker   *fingerprintLocker
	fpToSeries *seriesMap

	loopStopping, loopStopped chan struct{}
	maxMemoryChunks           int
	purgeAfter                time.Duration
	checkpointInterval        time.Duration

	persistQueue   chan persistRequest
	persistStopped chan struct{}
	persistence    *persistence

	evictList                   *list.List
	evictRequests               chan evictRequest
	evictStopping, evictStopped chan struct{}

	persistLatency              prometheus.Summary
	persistErrors               *prometheus.CounterVec
	persistQueueLength          prometheus.Gauge
	numSeries                   prometheus.Gauge
	seriesOps                   *prometheus.CounterVec
	ingestedSamplesCount        prometheus.Counter
	invalidPreloadRequestsCount prometheus.Counter
	purgeDuration               prometheus.Gauge
}

// MemorySeriesStorageOptions contains options needed by
// NewMemorySeriesStorage. It is not safe to leave any of those at their zero
// values.
type MemorySeriesStorageOptions struct {
	MemoryChunks               int           // How many chunks to keep in memory.
	PersistenceStoragePath     string        // Location of persistence files.
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
		maxMemoryChunks:    o.MemoryChunks,
		purgeAfter:         o.PersistenceRetentionPeriod,
		checkpointInterval: o.CheckpointInterval,

		persistQueue:   make(chan persistRequest, persistQueueCap),
		persistStopped: make(chan struct{}),
		persistence:    p,

		evictList:     list.New(),
		evictRequests: make(chan evictRequest, evictRequestsCap),
		evictStopping: make(chan struct{}),
		evictStopped:  make(chan struct{}),

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
	}, nil
}

// Start implements Storage.
func (s *memorySeriesStorage) Start() {
	go s.handleEvictList()
	go s.handlePersistQueue()
	go s.loop()
}

// Stop implements Storage.
func (s *memorySeriesStorage) Stop() error {
	glog.Info("Stopping local storage...")

	glog.Info("Stopping maintenance loop...")
	close(s.loopStopping)
	<-s.loopStopped

	glog.Info("Stopping persist queue...")
	close(s.persistQueue)
	<-s.persistStopped

	glog.Info("Stopping chunk eviction...")
	close(s.evictStopping)
	<-s.evictStopped

	// One final checkpoint of the series map and the head chunks.
	if err := s.persistence.checkpointSeriesMapAndHeads(s.fpToSeries, s.fpLocker); err != nil {
		return err
	}

	if err := s.persistence.close(); err != nil {
		return err
	}
	glog.Info("Local storage stopped.")
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
	return series.preloadChunksForRange(from, through, fp, s)
}

func (s *memorySeriesStorage) handleEvictList() {
	ticker := time.NewTicker(maxEvictInterval)
	count := 0
loop:
	for {
		// To batch up evictions a bit, this tries evictions at least
		// once per evict interval, but earlier if the number of evict
		// requests with evict==true that has happened since the last
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
			break loop
		}
	}
	ticker.Stop()
	glog.Info("Chunk eviction stopped.")
	close(s.evictStopped)
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
	// goroutine that is supposed to empty the channel is wating for the
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
		req.chunkDesc.unpin(s.evictRequests)
		chunkOps.WithLabelValues(persistAndUnpin).Inc()
	}
	glog.Info("Persist queue drained and stopped.")
	close(s.persistStopped)
}

// waitForNextFP waits an estimated duration, after which we want to process
// another fingerprint so that we will process all fingerprints in a tenth of
// s.purgeAfter assuming that the system is doing nothing else, e.g. if we want
// to purge after 40h, we want to cycle through all fingerprints within
// 4h. However, the maximum sweep time is capped at fpMaxSweepTime. Furthermore,
// this method will always wait for at least fpMinWaitDuration and never longer
// than fpMaxWaitDuration. If s.loopStopped is closed, it will return false
// immediately. The estimation is based on the total number of fingerprints as
// passed in.
func (s *memorySeriesStorage) waitForNextFP(numberOfFPs int) bool {
	d := fpMaxWaitDuration
	if numberOfFPs != 0 {
		sweepTime := s.purgeAfter / 10
		if sweepTime > fpMaxSweepTime {
			sweepTime = fpMaxSweepTime
		}
		d = sweepTime / time.Duration(numberOfFPs)
		if d < fpMinWaitDuration {
			d = fpMinWaitDuration
		}
		if d > fpMaxWaitDuration {
			d = fpMaxWaitDuration
		}
	}
	t := time.NewTimer(d)
	select {
	case <-t.C:
		return true
	case <-s.loopStopping:
		return false
	}
}

func (s *memorySeriesStorage) loop() {
	checkpointTicker := time.NewTicker(s.checkpointInterval)

	defer func() {
		checkpointTicker.Stop()
		glog.Info("Maintenance loop stopped.")
		close(s.loopStopped)
	}()

	memoryFingerprints := make(chan clientmodel.Fingerprint)
	go func() {
		var fpIter <-chan clientmodel.Fingerprint

		defer func() {
			if fpIter != nil {
				for _ = range fpIter {
					// Consume the iterator.
				}
			}
			close(memoryFingerprints)
		}()

		for {
			// Initial wait, also important if there are no FPs yet.
			if !s.waitForNextFP(s.fpToSeries.length()) {
				return
			}
			begun := time.Now()
			fpIter = s.fpToSeries.fpIter()
			for fp := range fpIter {
				select {
				case memoryFingerprints <- fp:
				case <-s.loopStopping:
					return
				}
				s.waitForNextFP(s.fpToSeries.length())
			}
			glog.Infof("Completed maintenance sweep through in-memory fingerprints in %v.", time.Since(begun))
		}
	}()

	archivedFingerprints := make(chan clientmodel.Fingerprint)
	go func() {
		defer close(archivedFingerprints)

		for {
			archivedFPs, err := s.persistence.getFingerprintsModifiedBefore(
				clientmodel.TimestampFromTime(time.Now()).Add(-1 * s.purgeAfter),
			)
			if err != nil {
				glog.Error("Failed to lookup archived fingerprint ranges: ", err)
				s.waitForNextFP(0)
				continue
			}
			// Initial wait, also important if there are no FPs yet.
			if !s.waitForNextFP(len(archivedFPs)) {
				return
			}
			begun := time.Now()
			for _, fp := range archivedFPs {
				select {
				case archivedFingerprints <- fp:
				case <-s.loopStopping:
					return
				}
				s.waitForNextFP(len(archivedFPs))
			}
			glog.Infof("Completed maintenance sweep through archived fingerprints in %v.", time.Since(begun))
		}
	}()

loop:
	for {
		select {
		case <-s.loopStopping:
			break loop
		case <-checkpointTicker.C:
			s.persistence.checkpointSeriesMapAndHeads(s.fpToSeries, s.fpLocker)
		case fp := <-memoryFingerprints:
			s.purgeSeries(fp, clientmodel.TimestampFromTime(time.Now()).Add(-1*s.purgeAfter))
			s.maintainSeries(fp)
			s.seriesOps.WithLabelValues(memoryMaintenance).Inc()
		case fp := <-archivedFingerprints:
			s.purgeSeries(fp, clientmodel.TimestampFromTime(time.Now()).Add(-1*s.purgeAfter))
			s.seriesOps.WithLabelValues(archiveMaintenance).Inc()
		}
	}
	// Wait until both channels are closed.
	for channelStillOpen := true; channelStillOpen; _, channelStillOpen = <-memoryFingerprints {
	}
	for channelStillOpen := true; channelStillOpen; _, channelStillOpen = <-archivedFingerprints {
	}
}

// maintainSeries closes the head chunk if not touched in a while. It archives a
// series if all chunks are evicted. It evicts chunkDescs if there are too many.
func (s *memorySeriesStorage) maintainSeries(fp clientmodel.Fingerprint) {
	var headChunkToPersist *chunkDesc
	s.fpLocker.Lock(fp)
	defer func() {
		s.fpLocker.Unlock(fp)
		// Queue outside of lock!
		if headChunkToPersist != nil {
			s.persistQueue <- persistRequest{fp, headChunkToPersist}
		}
	}()

	series, ok := s.fpToSeries.get(fp)
	if !ok {
		return
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
			fp, series.metric, series.firstTime(), series.lastTime(),
		); err != nil {
			glog.Errorf("Error archiving metric %v: %v", series.metric, err)
		} else {
			s.seriesOps.WithLabelValues(archive).Inc()
		}
		return
	}
	// If we are here, the series is not archived, so check for chunkDesc
	// eviction next and then if the head chunk needs to be persisted.
	series.evictChunkDescs(iOldestNotEvicted)
	if !series.headChunkPersisted && time.Now().Sub(series.head().firstTime().Time()) > headChunkTimeout {
		series.headChunkPersisted = true
		// Since we cannot modify the head chunk from now on, we
		// don't need to bother with cloning anymore.
		series.headChunkUsedByIterator = false
		headChunkToPersist = series.head()
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
	glog.Infoln("DEBUG:", newFirstTime, allDropped)
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

	ch <- persistQueueCapGauge

	count := atomic.LoadInt64(&numMemChunks)
	ch <- prometheus.MustNewConstMetric(numMemChunksDesc, prometheus.GaugeValue, float64(count))
	count = atomic.LoadInt64(&numMemChunkDescs)
	ch <- prometheus.MustNewConstMetric(numMemChunkDescsDesc, prometheus.GaugeValue, float64(count))
}
