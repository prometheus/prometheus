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
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/coding/indexable"
	"github.com/prometheus/prometheus/model"
	dto "github.com/prometheus/prometheus/model/generated"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"log"
	"sort"
	"sync"
	"time"
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

// TieredStorage both persists samples and generates materialized views for
// queries.
type TieredStorage struct {
	// BUG(matt): This introduces a Law of Demeter violation.  Ugh.
	DiskStorage *LevelDBMetricPersistence

	appendToDiskQueue chan model.Samples

	diskFrontier *diskFrontier

	memoryArena         memorySeriesStorage
	memoryTTL           time.Duration
	flushMemoryInterval time.Duration

	// This mutex manages any concurrent reads/writes of the memoryArena.
	memoryMutex sync.RWMutex
	// This mutex blocks only deletions from the memoryArena. It is held for a
	// potentially long time for an entire renderView() duration, since we depend
	// on no samples being removed from memory after grabbing a LevelDB snapshot.
	memoryDeleteMutex sync.RWMutex

	viewQueue chan viewJob

	draining chan chan bool

	mutex sync.Mutex
}

// viewJob encapsulates a request to extract sample values from the datastore.
type viewJob struct {
	builder ViewRequestBuilder
	output  chan View
	abort   chan bool
	err     chan error
}

func NewTieredStorage(appendToDiskQueueDepth, viewQueueDepth uint, flushMemoryInterval, memoryTTL time.Duration, root string) (storage *TieredStorage, err error) {
	diskStorage, err := NewLevelDBMetricPersistence(root)
	if err != nil {
		return
	}

	storage = &TieredStorage{
		appendToDiskQueue:   make(chan model.Samples, appendToDiskQueueDepth),
		DiskStorage:         diskStorage,
		draining:            make(chan chan bool),
		flushMemoryInterval: flushMemoryInterval,
		memoryArena:         NewMemorySeriesStorage(),
		memoryTTL:           memoryTTL,
		viewQueue:           make(chan viewJob, viewQueueDepth),
	}
	return
}

// Enqueues Samples for storage.
func (t *TieredStorage) AppendSamples(samples model.Samples) (err error) {
	if len(t.draining) > 0 {
		return fmt.Errorf("Storage is in the process of draining.")
	}

	t.memoryMutex.Lock()
	t.memoryArena.AppendSamples(samples)
	t.memoryMutex.Unlock()

	return
}

// Stops the storage subsystem, flushing all pending operations.
func (t *TieredStorage) Drain() {
	log.Println("Starting drain...")
	drainingDone := make(chan bool)
	if len(t.draining) == 0 {
		t.draining <- drainingDone
	}
	<-drainingDone
	log.Println("Done.")
}

// Enqueues a ViewRequestBuilder for materialization, subject to a timeout.
func (t *TieredStorage) MakeView(builder ViewRequestBuilder, deadline time.Duration) (view View, err error) {
	if len(t.draining) > 0 {
		err = fmt.Errorf("Storage is in the process of draining.")
		return
	}

	// The result channel needs a one-element buffer in case we have timed out in
	// MakeView, but the view rendering still completes afterwards and writes to
	// the channel.
	result := make(chan View, 1)
	// The abort channel needs a one-element buffer in case the view rendering
	// has already exited and doesn't consume from the channel anymore.
	abortChan := make(chan bool, 1)
	errChan := make(chan error)
	t.viewQueue <- viewJob{
		builder: builder,
		output:  result,
		abort:   abortChan,
		err:     errChan,
	}

	select {
	case value := <-result:
		view = value
	case err = <-errChan:
		return
	case <-time.After(deadline):
		abortChan <- true
		err = fmt.Errorf("MakeView timed out after %s.", deadline)
	}

	return
}

func (t *TieredStorage) rebuildDiskFrontier(i leveldb.Iterator) (err error) {
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: appendSample, result: success}, map[string]string{operation: rebuildDiskFrontier, result: failure})
	}()

	t.diskFrontier, err = newDiskFrontier(i)
	if err != nil {
		return
	}
	return
}

// Starts serving requests.
func (t *TieredStorage) Serve() {
	flushMemoryTicker := time.NewTicker(t.flushMemoryInterval)
	defer flushMemoryTicker.Stop()
	queueReportTicker := time.NewTicker(time.Second)
	defer queueReportTicker.Stop()

	go func() {
		for _ = range queueReportTicker.C {
			t.reportQueues()
		}
	}()

	for {
		select {
		case <-flushMemoryTicker.C:
			t.flushMemory()
		case viewRequest := <-t.viewQueue:
			t.renderView(viewRequest)
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
	t.flushMemory()
}

func (t *TieredStorage) Close() {
	log.Println("Closing tiered storage...")
	t.Drain()
	t.DiskStorage.Close()
	t.memoryArena.Close()

	close(t.appendToDiskQueue)
	close(t.viewQueue)
	log.Println("Done.")
}

type memoryToDiskFlusher struct {
	toDiskQueue       chan model.Samples
	disk              MetricPersistence
	olderThan         time.Time
	valuesAccepted    int
	valuesRejected    int
	memoryDeleteMutex *sync.RWMutex
}

type memoryToDiskFlusherVisitor struct {
	stream            stream
	flusher           *memoryToDiskFlusher
	memoryDeleteMutex *sync.RWMutex
}

func (f memoryToDiskFlusherVisitor) DecodeKey(in interface{}) (out interface{}, err error) {
	out = time.Time(in.(skipListTime))
	return
}

func (f memoryToDiskFlusherVisitor) DecodeValue(in interface{}) (out interface{}, err error) {
	out = in.(value).get()
	return
}

func (f memoryToDiskFlusherVisitor) Filter(key, value interface{}) (filterResult storage.FilterResult) {
	var (
		recordTime = key.(time.Time)
	)

	if recordTime.Before(f.flusher.olderThan) {
		f.flusher.valuesAccepted++

		return storage.ACCEPT
	}
	f.flusher.valuesRejected++
	return storage.STOP
}

func (f memoryToDiskFlusherVisitor) Operate(key, value interface{}) (err *storage.OperatorError) {
	var (
		recordTime  = key.(time.Time)
		recordValue = value.(model.SampleValue)
	)

	if len(f.flusher.toDiskQueue) == cap(f.flusher.toDiskQueue) {
		f.flusher.Flush()
	}

	f.flusher.toDiskQueue <- model.Samples{
		model.Sample{
			Metric:    f.stream.metric,
			Timestamp: recordTime,
			Value:     recordValue,
		},
	}

	f.memoryDeleteMutex.Lock()
	f.stream.values.Delete(skipListTime(recordTime))
	f.memoryDeleteMutex.Unlock()

	return
}

func (f *memoryToDiskFlusher) ForStream(stream stream) (decoder storage.RecordDecoder, filter storage.RecordFilter, operator storage.RecordOperator) {
	visitor := memoryToDiskFlusherVisitor{
		stream:            stream,
		flusher:           f,
		memoryDeleteMutex: f.memoryDeleteMutex,
	}

	return visitor, visitor, visitor
}

func (f *memoryToDiskFlusher) Flush() {
	length := len(f.toDiskQueue)
	samples := model.Samples{}
	for i := 0; i < length; i++ {
		samples = append(samples, <-f.toDiskQueue...)
	}
	f.disk.AppendSamples(samples)
}

func (f memoryToDiskFlusher) Close() {
	f.Flush()
}

// Persist a whole bunch of samples from memory to the datastore.
func (t *TieredStorage) flushMemory() {
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, nil, map[string]string{operation: appendSample, result: success}, map[string]string{operation: flushMemory, result: failure})
	}()

	t.memoryMutex.RLock()
	defer t.memoryMutex.RUnlock()

	flusher := &memoryToDiskFlusher{
		disk:              t.DiskStorage,
		olderThan:         time.Now().Add(-1 * t.memoryTTL),
		toDiskQueue:       t.appendToDiskQueue,
		memoryDeleteMutex: &t.memoryDeleteMutex,
	}
	defer flusher.Close()

	t.memoryArena.ForEachSample(flusher)

	return
}

func (t *TieredStorage) renderView(viewJob viewJob) {
	// Telemetry.
	var err error
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: renderView, result: success}, map[string]string{operation: renderView, result: failure})
	}()

	// No samples may be deleted from memory while rendering a view.
	t.memoryDeleteMutex.RLock()
	defer t.memoryDeleteMutex.RUnlock()

	scans := viewJob.builder.ScanJobs()
	view := newView()
	// Get a single iterator that will be used for all data extraction below.
	iterator := t.DiskStorage.MetricSamples.NewIterator(true)
	defer iterator.Close()

	// Rebuilding of the frontier should happen on a conditional basis if a
	// (fingerprint, timestamp) tuple is outside of the current frontier.
	err = t.rebuildDiskFrontier(iterator)
	if err != nil {
		panic(err)
	}

	for _, scanJob := range scans {
		var seriesFrontier *seriesFrontier = nil
		if t.diskFrontier != nil {
			seriesFrontier, err = newSeriesFrontier(scanJob.fingerprint, *t.diskFrontier, iterator)
			if err != nil {
				panic(err)
			}
		}

		standingOps := scanJob.operations
		for len(standingOps) > 0 {
			// Abort the view rendering if the caller (MakeView) has timed out.
			if len(viewJob.abort) > 0 {
				return
			}

			// Load data value chunk(s) around the first standing op's current time.
			targetTime := *standingOps[0].CurrentTime()

			currentChunk := chunk{}
			t.memoryMutex.RLock()
			memValues := t.memoryArena.GetValueAtTime(scanJob.fingerprint, targetTime)
			t.memoryMutex.RUnlock()
			// If we aimed before the oldest value in memory, load more data from disk.
			if (len(memValues) == 0 || memValues.FirstTimeAfter(targetTime)) && seriesFrontier != nil {
				// XXX: For earnest performance gains analagous to the benchmarking we
				//      performed, chunk should only be reloaded if it no longer contains
				//      the values we're looking for.
				//
				//      To better understand this, look at https://github.com/prometheus/prometheus/blob/benchmark/leveldb/iterator-seek-characteristics/leveldb.go#L239 and note the behavior around retrievedValue.
				diskValues := t.loadChunkAroundTime(iterator, seriesFrontier, scanJob.fingerprint, targetTime)

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
				}
			}

			// Append the extracted samples to the materialized view.
			for _, sample := range out {
				view.appendSample(scanJob.fingerprint, sample.Timestamp, sample.Value)
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

	viewJob.output <- view
	return
}

func (t *TieredStorage) loadChunkAroundTime(iterator leveldb.Iterator, frontier *seriesFrontier, fingerprint model.Fingerprint, ts time.Time) (chunk model.Values) {
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
	rawKey, _ := coding.NewProtocolBuffer(targetKey).Encode()
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
func (t *TieredStorage) GetAllValuesForLabel(labelName model.LabelName) (values model.LabelValues, err error) {
	diskValues, err := t.DiskStorage.GetAllValuesForLabel(labelName)
	if err != nil {
		return
	}
	memoryValues, err := t.memoryArena.GetAllValuesForLabel(labelName)
	if err != nil {
		return
	}

	valueSet := map[model.LabelValue]bool{}
	for _, value := range append(diskValues, memoryValues...) {
		if !valueSet[value] {
			values = append(values, value)
			valueSet[value] = true
		}
	}

	return
}

// Get all of the metric fingerprints that are associated with the provided
// label set.
func (t *TieredStorage) GetFingerprintsForLabelSet(labelSet model.LabelSet) (fingerprints model.Fingerprints, err error) {
	memFingerprints, err := t.memoryArena.GetFingerprintsForLabelSet(labelSet)
	if err != nil {
		return
	}
	diskFingerprints, err := t.DiskStorage.GetFingerprintsForLabelSet(labelSet)
	if err != nil {
		return
	}
	fingerprintSet := map[model.Fingerprint]bool{}
	for _, fingerprint := range append(memFingerprints, diskFingerprints...) {
		fingerprintSet[fingerprint] = true
	}
	for fingerprint := range fingerprintSet {
		fingerprints = append(fingerprints, fingerprint)
	}

	return
}

// Get the metric associated with the provided fingerprint.
func (t *TieredStorage) GetMetricForFingerprint(f model.Fingerprint) (m *model.Metric, err error) {
	m, err = t.memoryArena.GetMetricForFingerprint(f)
	if err != nil {
		return
	}
	if m == nil {
		m, err = t.DiskStorage.GetMetricForFingerprint(f)
	}
	return
}
