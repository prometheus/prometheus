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
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

const (
	evictRequestsCap = 1024
	chunkLen         = 1024

	// See waitForNextFP.
	fpMaxWaitDuration = 10 * time.Second
	fpMinWaitDuration = 20 * time.Millisecond // A small multiple of disk seek time.
	fpMaxSweepTime    = 6 * time.Hour

	maxEvictInterval = time.Minute
	headChunkTimeout = time.Hour // Close head chunk if not touched for that long.

	appendWorkers  = 8 // Should be enough to not make appending a bottleneck.
	appendQueueCap = 2 * appendWorkers
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

	loopStopping, loopStopped  chan struct{}
	maxMemoryChunks            int
	dropAfter                  time.Duration
	checkpointInterval         time.Duration
	checkpointDirtySeriesLimit int
	chunkType                  byte

	appendQueue         chan *clientmodel.Sample
	appendLastTimestamp clientmodel.Timestamp // The timestamp of the last sample sent to the append queue.
	appendWaitGroup     sync.WaitGroup        // To wait for all appended samples to be processed.

	persistQueue    chan persistRequest
	persistQueueCap int // Not actually the cap of above channel. See handlePersistQueue.
	persistStopped  chan struct{}

	persistence *persistence

	countPersistedHeadChunks chan struct{}

	evictList                   *list.List
	evictRequests               chan evictRequest
	evictStopping, evictStopped chan struct{}

	persistLatency              prometheus.Summary
	persistErrors               prometheus.Counter
	persistQueueCapacity        prometheus.Metric
	persistQueueLength          prometheus.Gauge
	numSeries                   prometheus.Gauge
	seriesOps                   *prometheus.CounterVec
	ingestedSamplesCount        prometheus.Counter
	invalidPreloadRequestsCount prometheus.Counter
}

// MemorySeriesStorageOptions contains options needed by
// NewMemorySeriesStorage. It is not safe to leave any of those at their zero
// values.
type MemorySeriesStorageOptions struct {
	MemoryChunks               int           // How many chunks to keep in memory.
	PersistenceStoragePath     string        // Location of persistence files.
	PersistenceRetentionPeriod time.Duration // Chunks at least that old are dropped.
	PersistenceQueueCapacity   int           // Capacity of queue for chunks to be persisted.
	CheckpointInterval         time.Duration // How often to checkpoint the series map and head chunks.
	CheckpointDirtySeriesLimit int           // How many dirty series will trigger an early checkpoint.
	ChunkType                  byte          // Chunk type for newly created chunks.
	Dirty                      bool          // Force the storage to consider itself dirty on startup.
}

// NewMemorySeriesStorage returns a newly allocated Storage. Storage.Serve still
// has to be called to start the storage.
func NewMemorySeriesStorage(o *MemorySeriesStorageOptions) (Storage, error) {
	if o.ChunkType > 1 {
		return nil, fmt.Errorf("unsupported chunk type %d", o.ChunkType)
	}
	p, err := newPersistence(o.PersistenceStoragePath, o.ChunkType, o.Dirty)
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

	s := &memorySeriesStorage{
		fpLocker:   newFingerprintLocker(1024),
		fpToSeries: fpToSeries,

		loopStopping:               make(chan struct{}),
		loopStopped:                make(chan struct{}),
		maxMemoryChunks:            o.MemoryChunks,
		dropAfter:                  o.PersistenceRetentionPeriod,
		checkpointInterval:         o.CheckpointInterval,
		checkpointDirtySeriesLimit: o.CheckpointDirtySeriesLimit,
		chunkType:                  o.ChunkType,

		appendLastTimestamp: clientmodel.Earliest,
		appendQueue:         make(chan *clientmodel.Sample, appendQueueCap),

		// The actual buffering happens within handlePersistQueue, so
		// cap of persistQueue just has to be enough to not block while
		// handlePersistQueue is writing to disk (20ms or so).
		persistQueue:    make(chan persistRequest, 1024),
		persistQueueCap: o.PersistenceQueueCapacity,
		persistStopped:  make(chan struct{}),
		persistence:     p,

		countPersistedHeadChunks: make(chan struct{}, 100),

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
		persistErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "persist_errors_total",
			Help:      "The total number of errors while persisting chunks.",
		}),
		persistQueueCapacity: prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, "persist_queue_capacity"),
				"The total capacity of the persist queue.",
				nil, nil,
			),
			prometheus.GaugeValue, float64(o.PersistenceQueueCapacity),
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
	}

	for i := 0; i < appendWorkers; i++ {
		go func() {
			for sample := range s.appendQueue {
				s.appendSample(sample)
				s.appendWaitGroup.Done()
			}
		}()
	}

	return s, nil
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

	glog.Info("Draining append queue...")
	close(s.appendQueue)
	s.appendWaitGroup.Wait()
	glog.Info("Append queue drained.")

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
	// First let all goroutines appending samples stop.
	s.appendWaitGroup.Wait()
	// Only then wait for the persistence to index them.
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
func (s *memorySeriesStorage) GetMetricForFingerprint(fp clientmodel.Fingerprint) clientmodel.COWMetric {
	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	series, ok := s.fpToSeries.get(fp)
	if ok {
		// Wrap the returned metric in a copy-on-write (COW) metric here because
		// the caller might mutate it.
		return clientmodel.COWMetric{
			Metric: series.metric,
		}
	}
	metric, err := s.persistence.getArchivedMetric(fp)
	if err != nil {
		glog.Errorf("Error retrieving archived metric for fingerprint %v: %v", fp, err)
	}
	return clientmodel.COWMetric{
		Metric: metric,
	}
}

// AppendSamples implements Storage.
func (s *memorySeriesStorage) AppendSamples(samples clientmodel.Samples) {
	for _, sample := range samples {
		if sample.Timestamp != s.appendLastTimestamp {
			// Timestamp has changed. We have to wait for processing
			// of all appended samples before proceeding. Otherwise,
			// we might violate the storage contract that each
			// sample appended to a given series has to have a
			// timestamp greater or equal to the previous sample
			// appended to that series.
			s.appendWaitGroup.Wait()
			s.appendLastTimestamp = sample.Timestamp
		}
		s.appendWaitGroup.Add(1)
		s.appendQueue <- sample
	}
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
	s.ingestedSamplesCount.Inc()

	if len(chunkDescsToPersist) == 0 {
		return
	}
	// Queue only outside of the locked area, processing the persistQueue
	// requires the same lock!
	for _, cd := range chunkDescsToPersist {
		s.persistQueue <- persistRequest{fp, cd}
	}
	// Count that a head chunk was persisted, but only best effort, i.e. we
	// don't want to block here.
	select {
	case s.countPersistedHeadChunks <- struct{}{}: // Counted.
	default: // Meh...
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
		series = newMemorySeries(m, !unarchived, firstTime, s.chunkType)
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
			glog.Info("Chunk eviction stopped.")
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

func (s *memorySeriesStorage) handlePersistQueue() {
	chunkMaps := chunkMaps{}
	chunkCount := 0

	persistMostConsecutiveChunks := func() {
		fp, cds := chunkMaps.pop()
		if err := s.persistChunks(fp, cds); err != nil {
			// Need to put chunks back for retry.
			for _, cd := range cds {
				chunkMaps.add(fp, cd)
			}
			return
		}
		chunkCount -= len(cds)
		s.persistQueueLength.Set(float64(chunkCount))
	}

loop:
	for {
		if chunkCount >= s.persistQueueCap && chunkCount > 0 {
			glog.Warningf("%d chunks queued for persistence. Ingestion pipeline will backlog.", chunkCount)
			persistMostConsecutiveChunks()
		}
		select {
		case req, ok := <-s.persistQueue:
			if !ok {
				break loop
			}
			chunkMaps.add(req.fingerprint, req.chunkDesc)
			chunkCount++
		default:
			if chunkCount > 0 {
				persistMostConsecutiveChunks()
				continue loop
			}
			// If we are here, there is nothing to do right now. So
			// just wait for a persist request to come in.
			req, ok := <-s.persistQueue
			if !ok {
				break loop
			}
			chunkMaps.add(req.fingerprint, req.chunkDesc)
			chunkCount++
		}
		s.persistQueueLength.Set(float64(chunkCount))
	}

	// Drain all requests.
	for _, m := range chunkMaps {
		for fp, cds := range m {
			if s.persistChunks(fp, cds) == nil {
				chunkCount -= len(cds)
				if (chunkCount+len(cds))/1000 > chunkCount/1000 {
					glog.Infof(
						"Still draining persist queue, %d chunks left to persist...",
						chunkCount,
					)
				}
				s.persistQueueLength.Set(float64(chunkCount))
			}
		}
	}

	glog.Info("Persist queue drained and stopped.")
	close(s.persistStopped)
}

func (s *memorySeriesStorage) persistChunks(fp clientmodel.Fingerprint, cds []*chunkDesc) error {
	start := time.Now()
	chunks := make([]chunk, len(cds))
	for i, cd := range cds {
		chunks[i] = cd.chunk
	}
	s.fpLocker.Lock(fp)
	offset, err := s.persistence.persistChunks(fp, chunks)
	if series, seriesInMemory := s.fpToSeries.get(fp); err == nil && seriesInMemory && series.chunkDescsOffset == -1 {
		// This is the first chunk persisted for a newly created
		// series that had prior chunks on disk. Finally, we can
		// set the chunkDescsOffset.
		series.chunkDescsOffset = offset
	}
	s.fpLocker.Unlock(fp)
	s.persistLatency.Observe(float64(time.Since(start)) / float64(time.Microsecond))
	if err != nil {
		s.persistErrors.Inc()
		glog.Error("Error persisting chunks: ", err)
		s.persistence.setDirty(true)
		return err
	}
	for _, cd := range cds {
		cd.unpin(s.evictRequests)
	}
	chunkOps.WithLabelValues(persistAndUnpin).Add(float64(len(cds)))
	return nil
}

// waitForNextFP waits an estimated duration, after which we want to process
// another fingerprint so that we will process all fingerprints in a tenth of
// s.dropAfter assuming that the system is doing nothing else, e.g. if we want
// to drop chunks after 40h, we want to cycle through all fingerprints within
// 4h. However, the maximum sweep time is capped at fpMaxSweepTime. Furthermore,
// this method will always wait for at least fpMinWaitDuration and never longer
// than fpMaxWaitDuration. If s.loopStopped is closed, it will return false
// immediately. The estimation is based on the total number of fingerprints as
// passed in.
func (s *memorySeriesStorage) waitForNextFP(numberOfFPs int) bool {
	d := fpMaxWaitDuration
	if numberOfFPs != 0 {
		sweepTime := s.dropAfter / 10
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

// cycleThroughMemoryFingerprints returns a channel that emits fingerprints for
// series in memory in a throttled fashion. It continues to cycle through all
// fingerprints in memory until s.loopStopping is closed.
func (s *memorySeriesStorage) cycleThroughMemoryFingerprints() chan clientmodel.Fingerprint {
	memoryFingerprints := make(chan clientmodel.Fingerprint)
	go func() {
		var fpIter <-chan clientmodel.Fingerprint

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
			if !s.waitForNextFP(s.fpToSeries.length()) {
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
				s.waitForNextFP(s.fpToSeries.length())
				count++
			}
			if count > 0 {
				glog.Infof(
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
func (s *memorySeriesStorage) cycleThroughArchivedFingerprints() chan clientmodel.Fingerprint {
	archivedFingerprints := make(chan clientmodel.Fingerprint)
	go func() {
		defer close(archivedFingerprints)

		for {
			archivedFPs, err := s.persistence.getFingerprintsModifiedBefore(
				clientmodel.TimestampFromTime(time.Now()).Add(-s.dropAfter),
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
			begin := time.Now()
			for _, fp := range archivedFPs {
				select {
				case archivedFingerprints <- fp:
				case <-s.loopStopping:
					return
				}
				s.waitForNextFP(len(archivedFPs))
			}
			if len(archivedFPs) > 0 {
				glog.Infof(
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

	// We take the number of head chunks persisted since the last checkpoint
	// as an approximation for the number of series that are "dirty",
	// i.e. whose head chunk is different from the one in the most recent
	// checkpoint or for which the fact that the head chunk has been
	// persisted is not reflected in the most recent checkpoint. This count
	// could overestimate the number of dirty series, but it's good enough
	// as a heuristic.
	headChunksPersistedSinceLastCheckpoint := 0

	defer func() {
		checkpointTimer.Stop()
		glog.Info("Maintenance loop stopped.")
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
			s.persistence.checkpointSeriesMapAndHeads(s.fpToSeries, s.fpLocker)
			headChunksPersistedSinceLastCheckpoint = 0
			checkpointTimer.Reset(s.checkpointInterval)
		case fp := <-memoryFingerprints:
			s.maintainMemorySeries(fp, clientmodel.TimestampFromTime(time.Now()).Add(-s.dropAfter))
		case fp := <-archivedFingerprints:
			s.maintainArchivedSeries(fp, clientmodel.TimestampFromTime(time.Now()).Add(-s.dropAfter))
		case <-s.countPersistedHeadChunks:
			headChunksPersistedSinceLastCheckpoint++
			// Check if we have enough "dirty" series so that we need an early checkpoint.
			// As described above, we take the headChunksPersistedSinceLastCheckpoint as a
			// heuristic for "dirty" series. However, if we are already backlogging
			// chunks to be persisted, creating a checkpoint would be counterproductive,
			// as it would slow down chunk persisting even more, while in a situation like
			// that, the best we can do for crash recovery is to work through the persist
			// queue as quickly as possible. So only checkpoint if s.persistQueue is
			// at most 20% full.
			if headChunksPersistedSinceLastCheckpoint >= s.checkpointDirtySeriesLimit &&
				len(s.persistQueue) < cap(s.persistQueue)/5 {
				checkpointTimer.Reset(0)
			}
		}
	}
	// Wait until both channels are closed.
	for range memoryFingerprints {
	}
	for range archivedFingerprints {
	}
}

// maintainMemorySeries first purges the series from old chunks. If the series
// still exists after that, it proceeds with the following steps: It closes the
// head chunk if it was not touched in a while. It archives a series if all
// chunks are evicted. It evicts chunkDescs if there are too many.
func (s *memorySeriesStorage) maintainMemorySeries(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) {
	var headChunkToPersist *chunkDesc
	s.fpLocker.Lock(fp)
	defer func() {
		s.fpLocker.Unlock(fp)
		// Queue outside of lock!
		if headChunkToPersist != nil {
			s.persistQueue <- persistRequest{fp, headChunkToPersist}
			// Count that a head chunk was persisted, but only best effort, i.e. we
			// don't want to block here.
			select {
			case s.countPersistedHeadChunks <- struct{}{}: // Counted.
			default: // Meh...
			}
		}
	}()

	series, ok := s.fpToSeries.get(fp)
	if !ok {
		// Series is actually not in memory, perhaps archived or dropped in the meantime.
		return
	}

	defer s.seriesOps.WithLabelValues(memoryMaintenance).Inc()

	if s.purgeMemorySeries(fp, series, beforeTime) {
		// Series is gone now, we are done.
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
		// Make sure we have a head chunk descriptor (a freshly
		// unarchived series has none).
		if len(series.chunkDescs) == 0 {
			cds, err := s.loadChunkDescs(fp, clientmodel.Latest)
			if err != nil {
				glog.Errorf(
					"Could not load chunk descriptors prior to archiving metric %v, metric will not be archived: %v",
					series.metric, err,
				)
				return
			}
			series.chunkDescs = cds
		}
		if err := s.persistence.archiveMetric(
			fp, series.metric, series.firstTime(), series.head().lastTime(),
		); err != nil {
			glog.Errorf("Error archiving metric %v: %v", series.metric, err)
			return
		}
		s.seriesOps.WithLabelValues(archive).Inc()
		return
	}
	// If we are here, the series is not archived, so check for chunkDesc
	// eviction next and then if the head chunk needs to be persisted.
	series.evictChunkDescs(iOldestNotEvicted)
	if !series.headChunkPersisted && time.Now().Sub(series.head().lastTime().Time()) > headChunkTimeout {
		series.headChunkPersisted = true
		// Since we cannot modify the head chunk from now on, we
		// don't need to bother with cloning anymore.
		series.headChunkUsedByIterator = false
		headChunkToPersist = series.head()
	}
}

// purgeMemorySeries drops chunks older than beforeTime from the provided memory
// series. The caller must have locked fp. If the series contains no chunks
// after dropping old chunks, it is purged entirely. In that case, the method
// returns true.
func (s *memorySeriesStorage) purgeMemorySeries(fp clientmodel.Fingerprint, series *memorySeries, beforeTime clientmodel.Timestamp) bool {
	if !series.firstTime().Before(beforeTime) {
		// Oldest sample not old enough.
		return false
	}
	newFirstTime, numDroppedFromPersistence, allDroppedFromPersistence, err := s.persistence.dropChunks(fp, beforeTime)
	if err != nil {
		glog.Error("Error dropping persisted chunks: ", err)
	}
	numDroppedFromMemory, allDroppedFromMemory := series.dropChunks(beforeTime)
	if allDroppedFromPersistence && allDroppedFromMemory {
		s.fpToSeries.del(fp)
		s.numSeries.Dec()
		s.seriesOps.WithLabelValues(memoryPurge).Inc()
		s.persistence.unindexMetric(fp, series.metric)
		return true
	}
	if series.chunkDescsOffset != -1 {
		series.savedFirstTime = newFirstTime
		series.chunkDescsOffset += numDroppedFromMemory - numDroppedFromPersistence
		if series.chunkDescsOffset < 0 {
			panic("dropped more chunks from persistence than from memory")
		}
	}
	return false
}

// maintainArchivedSeries drops chunks older than beforeTime from an archived
// series. If the series contains no chunks after that, it is purged entirely.
func (s *memorySeriesStorage) maintainArchivedSeries(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) {
	s.fpLocker.Lock(fp)
	defer s.fpLocker.Unlock(fp)

	has, firstTime, lastTime, err := s.persistence.hasArchivedMetric(fp)
	if err != nil {
		glog.Error("Error looking up archived time range: ", err)
		return
	}
	if !has || !firstTime.Before(beforeTime) {
		// Oldest sample not old enough, or metric purged or unarchived in the meantime.
		return
	}

	defer s.seriesOps.WithLabelValues(archiveMaintenance).Inc()

	newFirstTime, _, allDropped, err := s.persistence.dropChunks(fp, beforeTime)
	if err != nil {
		glog.Error("Error dropping persisted chunks: ", err)
	}
	if allDropped {
		if err := s.persistence.purgeArchivedMetric(fp); err != nil {
			glog.Errorf("Error purging archived metric for fingerprint %v: %v", fp, err)
			return
		}
		s.seriesOps.WithLabelValues(archivePurge).Inc()
		return
	}
	s.persistence.updateArchivedTimeRange(fp, newFirstTime, lastTime)
}

// See persistence.loadChunks for detailed explanation.
func (s *memorySeriesStorage) loadChunks(fp clientmodel.Fingerprint, indexes []int, indexOffset int) ([]chunk, error) {
	return s.persistence.loadChunks(fp, indexes, indexOffset)
}

// See persistence.loadChunkDescs for detailed explanation.
func (s *memorySeriesStorage) loadChunkDescs(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) ([]*chunkDesc, error) {
	return s.persistence.loadChunkDescs(fp, beforeTime)
}

// Describe implements prometheus.Collector.
func (s *memorySeriesStorage) Describe(ch chan<- *prometheus.Desc) {
	s.persistence.Describe(ch)

	ch <- s.persistLatency.Desc()
	ch <- s.persistErrors.Desc()
	ch <- s.persistQueueCapacity.Desc()
	ch <- s.persistQueueLength.Desc()
	ch <- s.numSeries.Desc()
	s.seriesOps.Describe(ch)
	ch <- s.ingestedSamplesCount.Desc()
	ch <- s.invalidPreloadRequestsCount.Desc()

	ch <- numMemChunksDesc
}

// Collect implements prometheus.Collector.
func (s *memorySeriesStorage) Collect(ch chan<- prometheus.Metric) {
	s.persistence.Collect(ch)

	ch <- s.persistLatency
	ch <- s.persistErrors
	ch <- s.persistQueueCapacity
	ch <- s.persistQueueLength
	ch <- s.numSeries
	s.seriesOps.Collect(ch)
	ch <- s.ingestedSamplesCount
	ch <- s.invalidPreloadRequestsCount

	ch <- prometheus.MustNewConstMetric(
		numMemChunksDesc,
		prometheus.GaugeValue,
		float64(atomic.LoadInt64(&numMemChunks)))
}

// chunkMaps is a slice of maps with chunkDescs to be persisted.
// Each chunk map contains n consecutive chunks to persist, where
// n is the index+1.
type chunkMaps []map[clientmodel.Fingerprint][]*chunkDesc

// add adds a chunk to chunkMaps.
func (cm *chunkMaps) add(fp clientmodel.Fingerprint, cd *chunkDesc) {
	// Runtime of this method is linear with the number of
	// chunkMaps. However, we expect only ever very few maps.
	numMaps := len(*cm)
	for i, m := range *cm {
		if cds, ok := m[fp]; ok {
			// Found our fp! Add cd and level up.
			cds = append(cds, cd)
			delete(m, fp)
			if i == numMaps-1 {
				*cm = append(*cm, map[clientmodel.Fingerprint][]*chunkDesc{})
			}
			(*cm)[i+1][fp] = cds
			return
		}
	}
	// Our fp isn't contained in cm yet. Add it to the first map (and add a
	// first map if there is none).
	if numMaps == 0 {
		*cm = chunkMaps{map[clientmodel.Fingerprint][]*chunkDesc{}}
	}
	(*cm)[0][fp] = []*chunkDesc{cd}
}

// pop retrieves and removes a fingerprint with all its chunks. It chooses one
// of the fingerprints with the most chunks. It panics if cm has no entries.
func (cm *chunkMaps) pop() (clientmodel.Fingerprint, []*chunkDesc) {
	m := (*cm)[len(*cm)-1]
	for fp, cds := range m {
		delete(m, fp)
		// Prune empty maps from top level.
		for len(m) == 0 {
			*cm = (*cm)[:len(*cm)-1]
			if len(*cm) == 0 {
				break
			}
			m = (*cm)[len(*cm)-1]
		}
		return fp, cds
	}
	panic("popped from empty chunkMaps")
}
