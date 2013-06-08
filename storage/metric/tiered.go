// Copyright 2013 Prometheus Team
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

package metric

import (
	"fmt"
	"log"
	"sort"
	"time"

	dto "github.com/prometheus/prometheus/model/generated"

	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/coding/indexable"
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"sync"
)

type chunk model.Values

// TruncateBefore returns a subslice of the original such that extraneous
// samples in the collection that occur before the provided time are
// dropped.  The original slice is not mutated.  It works with the assumption
// that consumers of these values could want preceding values if none would
// exist prior to the defined time.
func (c chunk) TruncateBefore(t time.Time) chunk {
	index := sort.Search(len(c), func(i int) bool {
		timestamp := c[i].Timestamp

		return !timestamp.Before(t)
	})

	switch index {
	case 0:
		return c
	case len(c):
		return c[len(c)-1:]
	default:
		return c[index-1:]
	}
}

type tieredStorageState uint

const (
	tieredStorageStarting tieredStorageState = iota
	tieredStorageServing
	tieredStorageDraining
	tieredStorageStopping
)

const (
	// Ignore timeseries in queries that are more stale than this limit.
	stalenessLimit = time.Minute * 5
	// Size of the watermarks cache (used in determining timeseries freshness).
	wmCacheSizeBytes = 5 * 1024 * 1024
)

// TieredStorage both persists samples and generates materialized views for
// queries.
type TieredStorage struct {
	// mu is purely used for state transitions.
	mu sync.RWMutex

	// BUG(matt): This introduces a Law of Demeter violation.  Ugh.
	DiskStorage *LevelDBMetricPersistence

	appendToDiskQueue chan model.Samples

	memoryArena         *memorySeriesStorage
	memoryTTL           time.Duration
	flushMemoryInterval time.Duration

	viewQueue chan viewJob

	draining chan chan<- bool

	state tieredStorageState

	memorySemaphore chan bool
	diskSemaphore   chan bool

	wmCache *WatermarkCache
}

// viewJob encapsulates a request to extract sample values from the datastore.
type viewJob struct {
	builder ViewRequestBuilder
	output  chan View
	abort   chan bool
	err     chan error
	stats   *stats.TimerGroup
}

const (
	tieredDiskSemaphores   = 1
	tieredMemorySemaphores = 5
)

func NewTieredStorage(appendToDiskQueueDepth, viewQueueDepth uint, flushMemoryInterval, memoryTTL time.Duration, root string) (*TieredStorage, error) {
	diskStorage, err := NewLevelDBMetricPersistence(root)
	if err != nil {
		return nil, err
	}

	wmCache := NewWatermarkCache(wmCacheSizeBytes)
	memOptions := MemorySeriesOptions{WatermarkCache: wmCache}

	s := &TieredStorage{
		appendToDiskQueue:   make(chan model.Samples, appendToDiskQueueDepth),
		DiskStorage:         diskStorage,
		draining:            make(chan chan<- bool),
		flushMemoryInterval: flushMemoryInterval,
		memoryArena:         NewMemorySeriesStorage(memOptions),
		memoryTTL:           memoryTTL,
		viewQueue:           make(chan viewJob, viewQueueDepth),

		diskSemaphore:   make(chan bool, tieredDiskSemaphores),
		memorySemaphore: make(chan bool, tieredMemorySemaphores),

		wmCache: wmCache,
	}

	for i := 0; i < tieredDiskSemaphores; i++ {
		s.diskSemaphore <- true
	}
	for i := 0; i < tieredMemorySemaphores; i++ {
		s.memorySemaphore <- true
	}

	return s, nil
}

// Enqueues Samples for storage.
func (t *TieredStorage) AppendSamples(samples model.Samples) (err error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.state != tieredStorageServing {
		return fmt.Errorf("Storage is not serving.")
	}

	t.memoryArena.AppendSamples(samples)

	return
}

// Stops the storage subsystem, flushing all pending operations.
func (t *TieredStorage) Drain(drained chan<- bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.drain(drained)
}

func (t *TieredStorage) drain(drained chan<- bool) {
	if t.state >= tieredStorageDraining {
		panic("Illegal State: Supplemental drain requested.")
	}

	log.Println("Triggering drain...")
	t.draining <- (drained)
}

// Enqueues a ViewRequestBuilder for materialization, subject to a timeout.
func (t *TieredStorage) MakeView(builder ViewRequestBuilder, deadline time.Duration, queryStats *stats.TimerGroup) (View, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.state != tieredStorageServing {
		return nil, fmt.Errorf("Storage is not serving")
	}

	// The result channel needs a one-element buffer in case we have timed out in
	// MakeView, but the view rendering still completes afterwards and writes to
	// the channel.
	result := make(chan View, 1)
	// The abort channel needs a one-element buffer in case the view rendering
	// has already exited and doesn't consume from the channel anymore.
	abortChan := make(chan bool, 1)
	errChan := make(chan error)
	queryStats.GetTimer(stats.ViewQueueTime).Start()
	t.viewQueue <- viewJob{
		builder: builder,
		output:  result,
		abort:   abortChan,
		err:     errChan,
		stats:   queryStats,
	}

	select {
	case view := <-result:
		return view, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(deadline):
		abortChan <- true
		return nil, fmt.Errorf("MakeView timed out after %s.", deadline)
	}
}

// Starts serving requests.
func (t *TieredStorage) Serve(started chan<- bool) {
	t.mu.Lock()
	if t.state != tieredStorageStarting {
		panic("Illegal State: Attempted to restart TieredStorage.")
	}

	t.state = tieredStorageServing
	t.mu.Unlock()

	flushMemoryTicker := time.NewTicker(t.flushMemoryInterval)
	defer flushMemoryTicker.Stop()
	queueReportTicker := time.NewTicker(time.Second)
	defer queueReportTicker.Stop()

	go func() {
		for _ = range queueReportTicker.C {
			t.reportQueues()
		}
	}()

	started <- true

	for {
		select {
		case <-flushMemoryTicker.C:
			t.flushMemory(t.memoryTTL)
		case viewRequest := <-t.viewQueue:
			viewRequest.stats.GetTimer(stats.ViewQueueTime).Stop()
			<-t.memorySemaphore
			go t.renderView(viewRequest)
		case drainingDone := <-t.draining:
			t.Flush()
			drainingDone <- true
			return
		}
	}
}

func (t *TieredStorage) reportQueues() {
	queueSizes.Set(map[string]string{"queue": "append_to_disk", "facet": "occupancy"}, float64(len(t.appendToDiskQueue)))
	queueSizes.Set(map[string]string{"queue": "append_to_disk", "facet": "capacity"}, float64(cap(t.appendToDiskQueue)))

	queueSizes.Set(map[string]string{"queue": "view_generation", "facet": "occupancy"}, float64(len(t.viewQueue)))
	queueSizes.Set(map[string]string{"queue": "view_generation", "facet": "capacity"}, float64(cap(t.viewQueue)))
}

func (t *TieredStorage) Flush() {
	t.flushMemory(0)
}

func (t *TieredStorage) flushMemory(ttl time.Duration) {
	t.memoryArena.RLock()
	defer t.memoryArena.RUnlock()

	cutOff := time.Now().Add(-1 * ttl)

	log.Println("Flushing...")

	for _, stream := range t.memoryArena.fingerprintToSeries {
		finder := func(i int) bool {
			return stream.values[i].Timestamp.After(cutOff)
		}

		stream.Lock()

		i := sort.Search(len(stream.values), finder)
		toArchive := stream.values[:i]
		toKeep := stream.values[i:]
		queued := make(model.Samples, 0, len(toArchive))

		for _, value := range toArchive {
			queued = append(queued, model.Sample{
				Metric:    stream.metric,
				Timestamp: value.Timestamp,
				Value:     value.Value,
			})
		}

		t.appendToDiskQueue <- queued

		stream.values = toKeep
		stream.Unlock()
	}

	queueLength := len(t.appendToDiskQueue)
	if queueLength > 0 {
		log.Printf("Writing %d samples ...", queueLength)
		samples := model.Samples{}
		for i := 0; i < queueLength; i++ {
			chunk := <-t.appendToDiskQueue
			samples = append(samples, chunk...)
		}

		t.DiskStorage.AppendSamples(samples)
	}

	log.Println("Done flushing...")
}

func (t *TieredStorage) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state == tieredStorageStopping {
		panic("Illegal State: Attempted to restop TieredStorage.")
	}

	drained := make(chan bool)
	t.drain(drained)
	<-drained

	t.memoryArena.Close()
	t.DiskStorage.Close()
	// BUG(matt): There is a probability that pending items may hang here and not
	//            get flushed.
	close(t.appendToDiskQueue)
	close(t.viewQueue)
	t.wmCache.Clear()

	t.state = tieredStorageStopping
}

func (t *TieredStorage) seriesTooOld(f *model.Fingerprint, i time.Time) (bool, error) {
	// BUG(julius): Make this configurable by query layer.
	i = i.Add(-stalenessLimit)

	wm, ok := t.wmCache.Get(f)
	if !ok {
		value := &dto.MetricHighWatermark{}
		present, err := t.DiskStorage.MetricHighWatermarks.Get(f.ToDTO(), value)
		if err != nil {
			return false, err
		}
		if present {
			wmTime := time.Unix(*value.Timestamp, 0).UTC()
			t.wmCache.Set(f, &Watermarks{High: wmTime})
			return wmTime.Before(i), nil
		}
		return true, nil
	}
	return wm.High.Before(i), nil
}

func (t *TieredStorage) renderView(viewJob viewJob) {
	// Telemetry.
	var err error
	begin := time.Now()
	defer func() {
		t.memorySemaphore <- true

		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: renderView, result: success}, map[string]string{operation: renderView, result: failure})
	}()

	scanJobsTimer := viewJob.stats.GetTimer(stats.ViewScanJobsTime).Start()
	scans := viewJob.builder.ScanJobs()
	scanJobsTimer.Stop()
	view := newView()

	var iterator leveldb.Iterator = nil
	var diskFrontier *diskFrontier = nil
	var diskPresent = true

	extractionTimer := viewJob.stats.GetTimer(stats.ViewDataExtractionTime).Start()
	for _, scanJob := range scans {
		old, err := t.seriesTooOld(scanJob.fingerprint, *scanJob.operations[0].CurrentTime())
		if err != nil {
			log.Printf("Error getting watermark from cache for %s: %s", scanJob.fingerprint, err)
			continue
		}
		if old {
			continue
		}

		var seriesFrontier *seriesFrontier = nil
		var seriesPresent = true

		standingOps := scanJob.operations
		memValues := t.memoryArena.CloneSamples(scanJob.fingerprint)

		for len(standingOps) > 0 {
			// Abort the view rendering if the caller (MakeView) has timed out.
			if len(viewJob.abort) > 0 {
				return
			}

			// Load data value chunk(s) around the first standing op's current time.
			targetTime := *standingOps[0].CurrentTime()

			currentChunk := chunk{}
			// If we aimed before the oldest value in memory, load more data from disk.
			if (len(memValues) == 0 || memValues.FirstTimeAfter(targetTime)) && diskPresent && seriesPresent {
				diskPrepareTimer := viewJob.stats.GetTimer(stats.ViewDiskPreparationTime).Start()
				// Conditionalize disk access.
				if diskFrontier == nil && diskPresent {
					if iterator == nil {
						<-t.diskSemaphore
						defer func() {
							t.diskSemaphore <- true
						}()

						// Get a single iterator that will be used for all data extraction
						// below.
						iterator = t.DiskStorage.MetricSamples.NewIterator(true)
						defer iterator.Close()
					}

					diskFrontier, diskPresent, err = newDiskFrontier(iterator)
					if err != nil {
						panic(err)
					}
					if !diskPresent {
						seriesPresent = false
					}
				}

				if seriesFrontier == nil && diskPresent {
					seriesFrontier, seriesPresent, err = newSeriesFrontier(scanJob.fingerprint, diskFrontier, iterator)
					if err != nil {
						panic(err)
					}
				}
				diskPrepareTimer.Stop()

				if diskPresent && seriesPresent {
					diskTimer := viewJob.stats.GetTimer(stats.ViewDiskExtractionTime).Start()
					diskValues := t.loadChunkAroundTime(iterator, seriesFrontier, scanJob.fingerprint, targetTime)
					diskTimer.Stop()

					// If we aimed past the newest value on disk, combine it with the next value from memory.
					if len(memValues) > 0 && diskValues.LastTimeBefore(targetTime) {
						latestDiskValue := diskValues[len(diskValues)-1:]
						currentChunk = append(chunk(latestDiskValue), chunk(memValues)...)
					} else {
						currentChunk = chunk(diskValues)
					}
				} else {
					currentChunk = chunk(memValues)
				}
			} else {
				currentChunk = chunk(memValues)
			}

			// There's no data at all for this fingerprint, so stop processing ops for it.
			if len(currentChunk) == 0 {
				break
			}

			currentChunk = currentChunk.TruncateBefore(targetTime)

			lastChunkTime := currentChunk[len(currentChunk)-1].Timestamp
			if lastChunkTime.After(targetTime) {
				targetTime = lastChunkTime
			}

			// For each op, extract all needed data from the current chunk.
			out := model.Values{}
			for _, op := range standingOps {
				if op.CurrentTime().After(targetTime) {
					break
				}

				currentChunk = currentChunk.TruncateBefore(*(op.CurrentTime()))

				for op.CurrentTime() != nil && !op.CurrentTime().After(targetTime) {
					out = op.ExtractSamples(model.Values(currentChunk))

					// Append the extracted samples to the materialized view.
					view.appendSamples(scanJob.fingerprint, out)
				}
			}

			// Throw away standing ops which are finished.
			filteredOps := ops{}
			for _, op := range standingOps {
				if op.CurrentTime() != nil {
					filteredOps = append(filteredOps, op)
				}
			}
			standingOps = filteredOps

			// Sort ops by start time again, since they might be slightly off now.
			// For example, consider a current chunk of values and two interval ops
			// with different interval lengths. Their states after the cycle above
			// could be:
			//
			// (C = current op time)
			//
			// Chunk: [ X X X X X ]
			// Op 1:      [ X   X   C  .  .  . ]
			// Op 2:        [ X X C . . .]
			//
			// Op 2 now has an earlier current time than Op 1.
			sort.Sort(startsAtSort{standingOps})
		}
	}
	extractionTimer.Stop()

	viewJob.output <- view
	return
}

func (t *TieredStorage) loadChunkAroundTime(iterator leveldb.Iterator, frontier *seriesFrontier, fingerprint *model.Fingerprint, ts time.Time) (chunk model.Values) {
	var (
		targetKey = &dto.SampleKey{
			Fingerprint: fingerprint.ToDTO(),
		}
		foundKey    model.SampleKey
		foundValues model.Values
	)

	// Limit the target key to be within the series' keyspace.
	if ts.After(frontier.lastSupertime) {
		targetKey.Timestamp = indexable.EncodeTime(frontier.lastSupertime)
	} else {
		targetKey.Timestamp = indexable.EncodeTime(ts)
	}

	// Try seeking to target key.
	rawKey := coding.NewPBEncoder(targetKey).MustEncode()
	iterator.Seek(rawKey)

	foundKey, err := extractSampleKey(iterator)
	if err != nil {
		panic(err)
	}

	// Figure out if we need to rewind by one block.
	// Imagine the following supertime blocks with time ranges:
	//
	// Block 1: ft 1000 - lt 1009 <data>
	// Block 1: ft 1010 - lt 1019 <data>
	//
	// If we are aiming to find time 1005, we would first seek to the block with
	// supertime 1010, then need to rewind by one block by virtue of LevelDB
	// iterator seek behavior.
	//
	// Only do the rewind if there is another chunk before this one.
	rewound := false
	firstTime := foundKey.FirstTimestamp
	if ts.Before(firstTime) && !frontier.firstSupertime.After(ts) {
		iterator.Previous()
		rewound = true
	}

	foundValues, err = extractSampleValues(iterator)
	if err != nil {
		return
	}

	// If we rewound, but the target time is still past the current block, return
	// the last value of the current (rewound) block and the entire next block.
	if rewound {
		foundKey, err = extractSampleKey(iterator)
		if err != nil {
			return
		}
		currentChunkLastTime := foundKey.LastTimestamp

		if ts.After(currentChunkLastTime) {
			sampleCount := len(foundValues)
			chunk = append(chunk, foundValues[sampleCount-1])
			// We know there's a next block since we have rewound from it.
			iterator.Next()

			foundValues, err = extractSampleValues(iterator)
			if err != nil {
				return
			}
		}
	}

	// Now append all the samples of the currently seeked block to the output.
	chunk = append(chunk, foundValues...)

	return
}

// Get all label values that are associated with the provided label name.
func (t *TieredStorage) GetAllValuesForLabel(labelName model.LabelName) (model.LabelValues, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.state != tieredStorageServing {
		panic("Illegal State: Attempted to query non-running TieredStorage.")
	}

	diskValues, err := t.DiskStorage.GetAllValuesForLabel(labelName)
	if err != nil {
		return nil, err
	}
	memoryValues, err := t.memoryArena.GetAllValuesForLabel(labelName)
	if err != nil {
		return nil, err
	}

	valueSet := map[model.LabelValue]bool{}
	values := model.LabelValues{}
	for _, value := range append(diskValues, memoryValues...) {
		if !valueSet[value] {
			values = append(values, value)
			valueSet[value] = true
		}
	}

	return values, nil
}

// Get all of the metric fingerprints that are associated with the provided
// label set.
func (t *TieredStorage) GetFingerprintsForLabelSet(labelSet model.LabelSet) (model.Fingerprints, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.state != tieredStorageServing {
		panic("Illegal State: Attempted to query non-running TieredStorage.")
	}

	memFingerprints, err := t.memoryArena.GetFingerprintsForLabelSet(labelSet)
	if err != nil {
		return nil, err
	}
	diskFingerprints, err := t.DiskStorage.GetFingerprintsForLabelSet(labelSet)
	if err != nil {
		return nil, err
	}
	fingerprintSet := map[model.Fingerprint]bool{}
	for _, fingerprint := range append(memFingerprints, diskFingerprints...) {
		fingerprintSet[*fingerprint] = true
	}
	fingerprints := model.Fingerprints{}
	for fingerprint := range fingerprintSet {
		fpCopy := fingerprint
		fingerprints = append(fingerprints, &fpCopy)
	}

	return fingerprints, nil
}

// Get the metric associated with the provided fingerprint.
func (t *TieredStorage) GetMetricForFingerprint(f *model.Fingerprint) (model.Metric, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.state != tieredStorageServing {
		panic("Illegal State: Attempted to query non-running TieredStorage.")
	}

	m, err := t.memoryArena.GetMetricForFingerprint(f)
	if err != nil {
		return nil, err
	}
	if m == nil {
		m, err = t.DiskStorage.GetMetricForFingerprint(f)
	}
	return m, err
}
