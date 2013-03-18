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
	"github.com/jmhodges/levigo"
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/coding/indexable"
	"github.com/prometheus/prometheus/model"
	dto "github.com/prometheus/prometheus/model/generated"
	"github.com/prometheus/prometheus/storage"
	"sort"
	"sync"
	"time"
)

// tieredStorage both persists samples and generates materialized views for
// queries.
type tieredStorage struct {
	appendToDiskQueue   chan model.Sample
	appendToMemoryQueue chan model.Sample
	diskFrontier        *diskFrontier
	diskStorage         *LevelDBMetricPersistence
	draining            chan bool
	flushMemoryInterval time.Duration
	memoryArena         memorySeriesStorage
	memoryTTL           time.Duration
	mutex               sync.Mutex
	viewQueue           chan viewJob
	writeMemoryInterval time.Duration
}

// viewJob encapsulates a request to extract sample values from the datastore.
type viewJob struct {
	builder ViewRequestBuilder
	output  chan View
	err     chan error
}

// Provides a unified means for batch appending values into the datastore along
// with querying for values in an efficient way.
type Storage interface {
	// Enqueues a Sample for storage.
	AppendSample(model.Sample) error
	// Enqueus a ViewRequestBuilder for materialization, subject to a timeout.
	MakeView(request ViewRequestBuilder, timeout time.Duration) (View, error)
	// Starts serving requests.
	Serve()
	// Stops the storage subsystem, flushing all pending operations.
	Drain()
	Flush()
	Close()
}

func NewTieredStorage(appendToMemoryQueueDepth, appendToDiskQueueDepth, viewQueueDepth uint, flushMemoryInterval, writeMemoryInterval, memoryTTL time.Duration, root string) Storage {
	diskStorage, err := NewLevelDBMetricPersistence(root)
	if err != nil {
		panic(err)
	}

	return &tieredStorage{
		appendToDiskQueue:   make(chan model.Sample, appendToDiskQueueDepth),
		appendToMemoryQueue: make(chan model.Sample, appendToMemoryQueueDepth),
		diskStorage:         diskStorage,
		draining:            make(chan bool),
		flushMemoryInterval: flushMemoryInterval,
		memoryArena:         NewMemorySeriesStorage(),
		memoryTTL:           memoryTTL,
		viewQueue:           make(chan viewJob, viewQueueDepth),
		writeMemoryInterval: writeMemoryInterval,
	}
}

func (t *tieredStorage) AppendSample(s model.Sample) (err error) {
	if len(t.draining) > 0 {
		return fmt.Errorf("Storage is in the process of draining.")
	}

	t.appendToMemoryQueue <- s

	return
}

func (t *tieredStorage) Drain() {
	if len(t.draining) == 0 {
		t.draining <- true
	}
}

func (t *tieredStorage) MakeView(builder ViewRequestBuilder, deadline time.Duration) (view View, err error) {
	if len(t.draining) > 0 {
		err = fmt.Errorf("Storage is in the process of draining.")
		return
	}

	result := make(chan View)
	errChan := make(chan error)
	t.viewQueue <- viewJob{
		builder: builder,
		output:  result,
		err:     errChan,
	}

	select {
	case value := <-result:
		view = value
	case err = <-errChan:
		return
	case <-time.After(deadline):
		err = fmt.Errorf("MakeView timed out after %s.", deadline)
	}

	return
}

func (t *tieredStorage) rebuildDiskFrontier() (err error) {
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: appendSample, result: success}, map[string]string{operation: rebuildDiskFrontier, result: failure})
	}()
	i, closer, err := t.diskStorage.metricSamples.GetIterator()
	if closer != nil {
		defer closer.Close()
	}
	if err != nil {
		panic(err)
	}
	t.diskFrontier, err = newDiskFrontier(i)
	if err != nil {
		panic(err)
	}
	return
}

func (t *tieredStorage) Serve() {
	var (
		flushMemoryTicker = time.Tick(t.flushMemoryInterval)
		writeMemoryTicker = time.Tick(t.writeMemoryInterval)
	)

	for {
		t.reportQueues()

		select {
		case <-writeMemoryTicker:
			t.writeMemory()
		case <-flushMemoryTicker:
			t.flushMemory()
		case viewRequest := <-t.viewQueue:
			t.renderView(viewRequest)
		case <-t.draining:
			t.flush()
			break
		}
	}
}

func (t *tieredStorage) reportQueues() {
	queueSizes.Set(map[string]string{"queue": "append_to_disk", "facet": "occupancy"}, float64(len(t.appendToDiskQueue)))
	queueSizes.Set(map[string]string{"queue": "append_to_disk", "facet": "capacity"}, float64(cap(t.appendToDiskQueue)))

	queueSizes.Set(map[string]string{"queue": "append_to_memory", "facet": "occupancy"}, float64(len(t.appendToMemoryQueue)))
	queueSizes.Set(map[string]string{"queue": "append_to_memory", "facet": "capacity"}, float64(cap(t.appendToMemoryQueue)))

	queueSizes.Set(map[string]string{"queue": "view_generation", "facet": "occupancy"}, float64(len(t.viewQueue)))
	queueSizes.Set(map[string]string{"queue": "view_generation", "facet": "capacity"}, float64(cap(t.viewQueue)))
}

func (t *tieredStorage) writeMemory() {
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, nil, map[string]string{operation: appendSample, result: success}, map[string]string{operation: writeMemory, result: failure})
	}()

	t.mutex.Lock()
	defer t.mutex.Unlock()

	pendingLength := len(t.appendToMemoryQueue)

	for i := 0; i < pendingLength; i++ {
		t.memoryArena.AppendSample(<-t.appendToMemoryQueue)
	}
}

func (t *tieredStorage) Flush() {
	t.flush()
}

func (t *tieredStorage) Close() {
	t.Drain()
	t.diskStorage.Close()
}

// Write all pending appends.
func (t *tieredStorage) flush() (err error) {
	// Trim any old values to reduce iterative write costs.
	t.flushMemory()
	t.writeMemory()
	t.flushMemory()
	return
}

type memoryToDiskFlusher struct {
	toDiskQueue    chan model.Sample
	disk           MetricPersistence
	olderThan      time.Time
	valuesAccepted int
	valuesRejected int
}

type memoryToDiskFlusherVisitor struct {
	stream  stream
	flusher *memoryToDiskFlusher
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

	f.flusher.toDiskQueue <- model.Sample{
		Metric:    f.stream.metric,
		Timestamp: recordTime,
		Value:     recordValue,
	}

	f.stream.values.Delete(skipListTime(recordTime))

	return
}

func (f *memoryToDiskFlusher) ForStream(stream stream) (decoder storage.RecordDecoder, filter storage.RecordFilter, operator storage.RecordOperator) {
	visitor := memoryToDiskFlusherVisitor{
		stream:  stream,
		flusher: f,
	}

	//	fmt.Printf("fingerprint -> %s\n", model.NewFingerprintFromMetric(stream.metric).ToRowKey())

	return visitor, visitor, visitor
}

func (f *memoryToDiskFlusher) Flush() {
	length := len(f.toDiskQueue)
	samples := model.Samples{}
	for i := 0; i < length; i++ {
		samples = append(samples, <-f.toDiskQueue)
	}
	start := time.Now()
	f.disk.AppendSamples(samples)
	if false {
		fmt.Printf("Took %s to append...\n", time.Since(start))
	}
}

func (f memoryToDiskFlusher) Close() {
	f.Flush()
}

// Persist a whole bunch of samples to the datastore.
func (t *tieredStorage) flushMemory() {
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, nil, map[string]string{operation: appendSample, result: success}, map[string]string{operation: flushMemory, result: failure})
	}()

	t.mutex.Lock()
	defer t.mutex.Unlock()

	flusher := &memoryToDiskFlusher{
		disk:        t.diskStorage,
		olderThan:   time.Now().Add(-1 * t.memoryTTL),
		toDiskQueue: t.appendToDiskQueue,
	}
	defer flusher.Close()

	t.memoryArena.ForEachSample(flusher)

	return
}

func (t *tieredStorage) renderView(viewJob viewJob) {
	// Telemetry.
	var err error
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: renderView, result: success}, map[string]string{operation: renderView, result: failure})
	}()

	t.mutex.Lock()
	defer t.mutex.Unlock()

	var (
		scans = viewJob.builder.ScanJobs()
		view  = newView()
	)

	// Rebuilding of the frontier should happen on a conditional basis if a
	// (fingerprint, timestamp) tuple is outside of the current frontier.
	err = t.rebuildDiskFrontier()
	if err != nil {
		panic(err)
	}
	if t.diskFrontier == nil {
		// Storage still empty, return an empty view.
		viewJob.output <- view
		return
	}

	// Get a single iterator that will be used for all data extraction below.
	iterator, closer, err := t.diskStorage.metricSamples.GetIterator()
	if closer != nil {
		defer closer.Close()
	}
	if err != nil {
		panic(err)
	}

	for _, scanJob := range scans {
		seriesFrontier, err := newSeriesFrontier(scanJob.fingerprint, *t.diskFrontier, iterator)
		if err != nil {
			panic(err)
		}
		if seriesFrontier == nil {
			continue
		}

		standingOps := scanJob.operations
		for len(standingOps) > 0 {
			// Load data value chunk(s) around the first standing op's current time.
			highWatermark := *standingOps[0].CurrentTime()
			// XXX: For earnest performance gains analagous to the benchmarking we
			//      performed, chunk should only be reloaded if it no longer contains
			//      the values we're looking for.
			//
			//      To better understand this, look at https://github.com/prometheus/prometheus/blob/benchmark/leveldb/iterator-seek-characteristics/leveldb.go#L239 and note the behavior around retrievedValue.
			chunk := t.loadChunkAroundTime(iterator, seriesFrontier, scanJob.fingerprint, highWatermark)
			lastChunkTime := chunk[len(chunk)-1].Timestamp
			if lastChunkTime.After(highWatermark) {
				highWatermark = lastChunkTime
			}

			// For each op, extract all needed data from the current chunk.
			out := []model.SamplePair{}
			for _, op := range standingOps {
				if op.CurrentTime().After(highWatermark) {
					break
				}
				for op.CurrentTime() != nil && !op.CurrentTime().After(highWatermark) {
					out = op.ExtractSamples(chunk)
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
			// with different interval lengthsr. Their states after the cycle above
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

func (t *tieredStorage) loadChunkAroundTime(iterator *levigo.Iterator, frontier *seriesFrontier, fingerprint model.Fingerprint, ts time.Time) (chunk []model.SamplePair) {
	var (
		targetKey = &dto.SampleKey{
			Fingerprint: fingerprint.ToDTO(),
		}
		foundKey   = &dto.SampleKey{}
		foundValue *dto.SampleValueSeries
	)

	// Limit the target key to be within the series' keyspace.
	if ts.After(frontier.lastSupertime) {
		targetKey.Timestamp = indexable.EncodeTime(frontier.lastSupertime)
	} else {
		targetKey.Timestamp = indexable.EncodeTime(ts)
	}

	// Try seeking to target key.
	rawKey, _ := coding.NewProtocolBufferEncoder(targetKey).Encode()
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
	firstTime := indexable.DecodeTime(foundKey.Timestamp)
	if ts.Before(firstTime) && !frontier.firstSupertime.After(ts) {
		iterator.Prev()
		rewound = true
	}

	foundValue, err = extractSampleValues(iterator)
	if err != nil {
		panic(err)
	}

	// If we rewound, but the target time is still past the current block, return
	// the last value of the current (rewound) block and the entire next block.
	if rewound {
		foundKey, err = extractSampleKey(iterator)
		if err != nil {
			panic(err)
		}
		currentChunkLastTime := time.Unix(*foundKey.LastTimestamp, 0)

		if ts.After(currentChunkLastTime) {
			sampleCount := len(foundValue.Value)
			chunk = append(chunk, model.SamplePair{
				Timestamp: time.Unix(*foundValue.Value[sampleCount-1].Timestamp, 0),
				Value:     model.SampleValue(*foundValue.Value[sampleCount-1].Value),
			})
			// We know there's a next block since we have rewound from it.
			iterator.Next()

			foundValue, err = extractSampleValues(iterator)
			if err != nil {
				panic(err)
			}
		}
	}

	// Now append all the samples of the currently seeked block to the output.
	for _, sample := range foundValue.Value {
		chunk = append(chunk, model.SamplePair{
			Timestamp: time.Unix(*sample.Timestamp, 0),
			Value:     model.SampleValue(*sample.Value),
		})
	}

	return
}
