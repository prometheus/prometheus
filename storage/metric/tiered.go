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
	"os"
	"sort"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"github.com/prometheus/prometheus/utility"
)

type chunk Values

// TruncateBefore returns a subslice of the original such that extraneous
// samples in the collection that occur before the provided time are
// dropped.  The original slice is not mutated.  It works with the assumption
// that consumers of these values could want preceding values if none would
// exist prior to the defined time.
func (c chunk) TruncateBefore(t clientmodel.Timestamp) chunk {
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

// Ignore timeseries in queries that are more stale than this limit.
const stalenessLimit = time.Minute * 5

// TieredStorage both persists samples and generates materialized views for
// queries.
type TieredStorage struct {
	// mu is purely used for state transitions.
	mu sync.RWMutex

	// BUG(matt): This introduces a Law of Demeter violation.  Ugh.
	DiskStorage *LevelDBMetricPersistence

	appendToDiskQueue chan clientmodel.Samples

	memoryArena         *memorySeriesStorage
	memoryTTL           time.Duration
	flushMemoryInterval time.Duration

	ViewQueue chan viewJob

	draining chan chan<- bool

	state tieredStorageState

	memorySemaphore chan bool

	wmCache *watermarkCache

	Indexer MetricIndexer

	flushSema chan bool

	dtoSampleKeys *dtoSampleKeyList
	sampleKeys    *sampleKeyList
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
	tieredMemorySemaphores = 5
)

const watermarkCacheLimit = 1024 * 1024

// NewTieredStorage returns a TieredStorage object ready to use.
func NewTieredStorage(
	appendToDiskQueueDepth,
	viewQueueDepth uint,
	flushMemoryInterval time.Duration,
	memoryTTL time.Duration,
	rootDirectory string,
) (*TieredStorage, error) {
	if isDir, _ := utility.IsDir(rootDirectory); !isDir {
		if err := os.MkdirAll(rootDirectory, 0755); err != nil {
			return nil, fmt.Errorf("could not find or create metrics directory %s: %s", rootDirectory, err)
		}
	}

	diskStorage, err := NewLevelDBMetricPersistence(rootDirectory)
	if err != nil {
		return nil, err
	}

	wmCache := &watermarkCache{
		C: utility.NewSynchronizedCache(utility.NewLRUCache(watermarkCacheLimit)),
	}

	memOptions := MemorySeriesOptions{
		WatermarkCache: wmCache,
	}

	s := &TieredStorage{
		appendToDiskQueue:   make(chan clientmodel.Samples, appendToDiskQueueDepth),
		DiskStorage:         diskStorage,
		draining:            make(chan chan<- bool),
		flushMemoryInterval: flushMemoryInterval,
		memoryArena:         NewMemorySeriesStorage(memOptions),
		memoryTTL:           memoryTTL,
		ViewQueue:           make(chan viewJob, viewQueueDepth),

		memorySemaphore: make(chan bool, tieredMemorySemaphores),

		wmCache: wmCache,

		flushSema: make(chan bool, 1),

		dtoSampleKeys: newDtoSampleKeyList(10),
		sampleKeys:    newSampleKeyList(10),
	}

	for i := 0; i < tieredMemorySemaphores; i++ {
		s.memorySemaphore <- true
	}

	return s, nil
}

// AppendSamples enqueues Samples for storage.
func (t *TieredStorage) AppendSamples(samples clientmodel.Samples) (err error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.state != tieredStorageServing {
		return fmt.Errorf("storage is not serving")
	}

	t.memoryArena.AppendSamples(samples)
	storedSamplesCount.IncrementBy(prometheus.NilLabels, float64(len(samples)))

	return
}

// Drain stops the storage subsystem, flushing all pending operations.
func (t *TieredStorage) Drain(drained chan<- bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.drain(drained)
}

func (t *TieredStorage) drain(drained chan<- bool) {
	if t.state >= tieredStorageDraining {
		panic("Illegal State: Supplemental drain requested.")
	}

	t.state = tieredStorageDraining

	glog.Info("Triggering drain...")
	t.draining <- (drained)
}

// MakeView materializes a View according to a ViewRequestBuilder, subject to a
// timeout.
func (t *TieredStorage) MakeView(builder ViewRequestBuilder, deadline time.Duration, queryStats *stats.TimerGroup) (View, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.state != tieredStorageServing {
		return nil, fmt.Errorf("storage is not serving")
	}

	// The result channel needs a one-element buffer in case we have timed
	// out in MakeView, but the view rendering still completes afterwards
	// and writes to the channel.
	result := make(chan View, 1)
	// The abort channel needs a one-element buffer in case the view
	// rendering has already exited and doesn't consume from the channel
	// anymore.
	abortChan := make(chan bool, 1)
	errChan := make(chan error)
	queryStats.GetTimer(stats.ViewQueueTime).Start()
	t.ViewQueue <- viewJob{
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
		return nil, fmt.Errorf("fetching query data timed out after %s", deadline)
	}
}

// Serve starts serving requests.
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
			select {
			case t.flushSema <- true:
				go func() {
					t.flushMemory(t.memoryTTL)
					<-t.flushSema
				}()
			default:
				glog.Warning("Backlogging on flush...")
			}
		case viewRequest := <-t.ViewQueue:
			<-t.memorySemaphore
			viewRequest.stats.GetTimer(stats.ViewQueueTime).Stop()
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

	queueSizes.Set(map[string]string{"queue": "view_generation", "facet": "occupancy"}, float64(len(t.ViewQueue)))
	queueSizes.Set(map[string]string{"queue": "view_generation", "facet": "capacity"}, float64(cap(t.ViewQueue)))
}

// Flush flushes all samples to disk.
func (t *TieredStorage) Flush() {
	t.flushSema <- true
	t.flushMemory(0)
	<-t.flushSema
}

func (t *TieredStorage) flushMemory(ttl time.Duration) {
	flushOlderThan := clientmodel.Now().Add(-1 * ttl)

	glog.Info("Flushing samples to disk...")
	t.memoryArena.Flush(flushOlderThan, t.appendToDiskQueue)

	queueLength := len(t.appendToDiskQueue)
	if queueLength > 0 {
		samples := clientmodel.Samples{}
		for i := 0; i < queueLength; i++ {
			chunk := <-t.appendToDiskQueue
			samples = append(samples, chunk...)
		}

		glog.Infof("Writing %d samples...", len(samples))
		t.DiskStorage.AppendSamples(samples)
	}

	glog.Info("Done flushing.")
}

// Close stops serving, flushes all pending operations, and frees all resources.
func (t *TieredStorage) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.close()
}

func (t *TieredStorage) close() {
	if t.state == tieredStorageStopping {
		panic("Illegal State: Attempted to restop TieredStorage.")
	}

	drained := make(chan bool)
	t.drain(drained)
	<-drained

	t.memoryArena.Close()
	t.DiskStorage.Close()
	// BUG(matt): There is a probability that pending items may hang here
	// and not get flushed.
	close(t.appendToDiskQueue)
	close(t.ViewQueue)
	t.wmCache.Clear()

	t.dtoSampleKeys.Close()
	t.sampleKeys.Close()

	t.state = tieredStorageStopping
}

func (t *TieredStorage) seriesTooOld(f *clientmodel.Fingerprint, i clientmodel.Timestamp) (bool, error) {
	// BUG(julius): Make this configurable by query layer.
	i = i.Add(-stalenessLimit)

	wm, cacheHit, _ := t.wmCache.Get(f)
	if !cacheHit {
		if t.memoryArena.HasFingerprint(f) {
			samples := t.memoryArena.CloneSamples(f)
			if len(samples) > 0 {
				newest := samples[len(samples)-1].Timestamp
				t.wmCache.Put(f, &watermarks{High: newest})

				return newest.Before(i), nil
			}
		}

		highTime, diskHit, err := t.DiskStorage.MetricHighWatermarks.Get(f)
		if err != nil {
			return false, err
		}

		if diskHit {
			t.wmCache.Put(f, &watermarks{High: highTime})

			return highTime.Before(i), nil
		}

		t.wmCache.Put(f, &watermarks{})
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

		recordOutcome(
			duration,
			err,
			map[string]string{operation: renderView, result: success},
			map[string]string{operation: renderView, result: failure},
		)
	}()

	view := newView()

	var iterator leveldb.Iterator
	diskPresent := true

	firstBlock, _ := t.sampleKeys.Get()
	defer t.sampleKeys.Give(firstBlock)

	lastBlock, _ := t.sampleKeys.Get()
	defer t.sampleKeys.Give(lastBlock)

	sampleKeyDto, _ := t.dtoSampleKeys.Get()
	defer t.dtoSampleKeys.Give(sampleKeyDto)

	extractionTimer := viewJob.stats.GetTimer(stats.ViewDataExtractionTime).Start()
	for viewJob.builder.HasOp() {
		op := viewJob.builder.PopOp()
		defer giveBackOp(op)

		fp := op.Fingerprint()
		old, err := t.seriesTooOld(fp, op.CurrentTime())
		if err != nil {
			glog.Errorf("Error getting watermark from cache for %s: %s", fp, err)
			continue
		}
		if old {
			continue
		}

		memValues := t.memoryArena.CloneSamples(fp)

		for !op.Consumed() {
			// Abort the view rendering if the caller (MakeView) has timed out.
			if len(viewJob.abort) > 0 {
				return
			}

			// Load data value chunk(s) around the current time.
			targetTime := op.CurrentTime()

			currentChunk := chunk{}
			// If we aimed before the oldest value in memory, load more data from disk.
			if (len(memValues) == 0 || memValues.FirstTimeAfter(targetTime)) && diskPresent {
				if iterator == nil {
					// Get a single iterator that will be used for all data extraction
					// below.
					iterator, _ = t.DiskStorage.MetricSamples.NewIterator(true)
					defer iterator.Close()
					if diskPresent = iterator.SeekToLast(); diskPresent {
						if err := iterator.Key(sampleKeyDto); err != nil {
							panic(err)
						}

						lastBlock.Load(sampleKeyDto)

						if !iterator.SeekToFirst() {
							diskPresent = false
						} else {
							if err := iterator.Key(sampleKeyDto); err != nil {
								panic(err)
							}

							firstBlock.Load(sampleKeyDto)
						}
					}
				}

				if diskPresent {
					diskTimer := viewJob.stats.GetTimer(stats.ViewDiskExtractionTime).Start()
					diskValues, expired := t.loadChunkAroundTime(
						iterator,
						fp,
						targetTime,
						firstBlock,
						lastBlock,
					)
					if expired {
						diskPresent = false
					}
					diskTimer.Stop()

					// If we aimed past the newest value on disk,
					// combine it with the next value from memory.
					if len(diskValues) == 0 {
						currentChunk = chunk(memValues)
					} else {
						if len(memValues) > 0 && diskValues.LastTimeBefore(targetTime) {
							latestDiskValue := diskValues[len(diskValues)-1:]
							currentChunk = append(chunk(latestDiskValue), chunk(memValues)...)
						} else {
							currentChunk = chunk(diskValues)
						}
					}
				} else {
					currentChunk = chunk(memValues)
				}
			} else {
				currentChunk = chunk(memValues)
			}

			// There's no data at all for this fingerprint, so stop processing.
			if len(currentChunk) == 0 {
				break
			}

			currentChunk = currentChunk.TruncateBefore(targetTime)

			lastChunkTime := currentChunk[len(currentChunk)-1].Timestamp
			if lastChunkTime.After(targetTime) {
				targetTime = lastChunkTime
			}

			if op.CurrentTime().After(targetTime) {
				break
			}

			// Extract all needed data from the current chunk and append the
			// extracted samples to the materialized view.
			for !op.Consumed() && !op.CurrentTime().After(targetTime) {
				view.appendSamples(fp, op.ExtractSamples(Values(currentChunk)))
			}
		}
	}
	extractionTimer.Stop()

	viewJob.output <- view
	return
}

func (t *TieredStorage) loadChunkAroundTime(
	iterator leveldb.Iterator,
	fingerprint *clientmodel.Fingerprint,
	ts clientmodel.Timestamp,
	firstBlock,
	lastBlock *SampleKey,
) (chunk Values, expired bool) {
	if fingerprint.Less(firstBlock.Fingerprint) {
		return nil, false
	}
	if lastBlock.Fingerprint.Less(fingerprint) {
		return nil, true
	}

	seekingKey, _ := t.sampleKeys.Get()
	defer t.sampleKeys.Give(seekingKey)

	seekingKey.Fingerprint = fingerprint

	if fingerprint.Equal(firstBlock.Fingerprint) && ts.Before(firstBlock.FirstTimestamp) {
		seekingKey.FirstTimestamp = firstBlock.FirstTimestamp
	} else if fingerprint.Equal(lastBlock.Fingerprint) && ts.After(lastBlock.FirstTimestamp) {
		seekingKey.FirstTimestamp = lastBlock.FirstTimestamp
	} else {
		seekingKey.FirstTimestamp = ts
	}

	dto, _ := t.dtoSampleKeys.Get()
	defer t.dtoSampleKeys.Give(dto)

	seekingKey.Dump(dto)
	if !iterator.Seek(dto) {
		return chunk, true
	}

	var foundValues Values

	if err := iterator.Key(dto); err != nil {
		panic(err)
	}
	seekingKey.Load(dto)

	if seekingKey.Fingerprint.Equal(fingerprint) {
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
		if !seekingKey.MayContain(ts) {
			postValues := unmarshalValues(iterator.RawValue())
			if !seekingKey.Equal(firstBlock) {
				if !iterator.Previous() {
					panic("This should never return false.")
				}

				if err := iterator.Key(dto); err != nil {
					panic(err)
				}
				seekingKey.Load(dto)

				if !seekingKey.Fingerprint.Equal(fingerprint) {
					return postValues, false
				}

				foundValues = unmarshalValues(iterator.RawValue())
				foundValues = append(foundValues, postValues...)
				return foundValues, false
			}
		}

		foundValues = unmarshalValues(iterator.RawValue())
		return foundValues, false
	}

	if fingerprint.Less(seekingKey.Fingerprint) {
		if !seekingKey.Equal(firstBlock) {
			if !iterator.Previous() {
				panic("This should never return false.")
			}

			if err := iterator.Key(dto); err != nil {
				panic(err)
			}
			seekingKey.Load(dto)

			if !seekingKey.Fingerprint.Equal(fingerprint) {
				return nil, false
			}

			foundValues = unmarshalValues(iterator.RawValue())
			return foundValues, false
		}
	}

	panic("illegal state: violated sort invariant")
}

// GetAllValuesForLabel gets all label values that are associated with the
// provided label name.
func (t *TieredStorage) GetAllValuesForLabel(labelName clientmodel.LabelName) (clientmodel.LabelValues, error) {
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

	valueSet := map[clientmodel.LabelValue]bool{}
	values := clientmodel.LabelValues{}
	for _, value := range append(diskValues, memoryValues...) {
		if !valueSet[value] {
			values = append(values, value)
			valueSet[value] = true
		}
	}

	return values, nil
}

// GetFingerprintsForLabelSet gets all of the metric fingerprints that are
// associated with the provided label set.
func (t *TieredStorage) GetFingerprintsForLabelSet(labelSet clientmodel.LabelSet) (clientmodel.Fingerprints, error) {
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
	fingerprintSet := map[clientmodel.Fingerprint]bool{}
	for _, fingerprint := range append(memFingerprints, diskFingerprints...) {
		fingerprintSet[*fingerprint] = true
	}
	fingerprints := clientmodel.Fingerprints{}
	for fingerprint := range fingerprintSet {
		fpCopy := fingerprint
		fingerprints = append(fingerprints, &fpCopy)
	}

	return fingerprints, nil
}

// GetMetricForFingerprint gets the metric associated with the provided
// fingerprint.
func (t *TieredStorage) GetMetricForFingerprint(f *clientmodel.Fingerprint) (clientmodel.Metric, error) {
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
		t.memoryArena.CreateEmptySeries(m)
	}
	return m, err
}
