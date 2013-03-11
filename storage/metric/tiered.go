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

func (t *tieredStorage) renderView(viewJob viewJob) (err error) {
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: appendSample, result: success}, map[string]string{operation: renderView, result: failure})
	}()

	t.mutex.Lock()
	defer t.mutex.Unlock()

	var (
		scans = viewJob.builder.ScanJobs()
		// 	standingOperations = ops{}
		// 	lastTime           = time.Time{}
		view = newView()
	)

	// Rebuilding of the frontier should happen on a conditional basis if a
	// (fingerprint, timestamp) tuple is outside of the current frontier.
	err = t.rebuildDiskFrontier()
	if err != nil {
		panic(err)
	}

	iterator, closer, err := t.diskStorage.metricSamples.GetIterator()
	if closer != nil {
		defer closer.Close()
	}
	if err != nil {
		panic(err)
	}

	for _, scanJob := range scans {
		// XXX: Memoize the last retrieval for forward scans.
		var (
		//			standingOperations ops
		)

		//		fmt.Printf("Starting scan of %s...\n", scanJob)
		if t.diskFrontier != nil || t.diskFrontier.ContainsFingerprint(scanJob.fingerprint) {
			//			fmt.Printf("Using diskFrontier %s\n", t.diskFrontier)
			seriesFrontier, err := newSeriesFrontier(scanJob.fingerprint, *t.diskFrontier, iterator)
			//			fmt.Printf("Using seriesFrontier %s\n", seriesFrontier)
			if err != nil {
				panic(err)
			}

			if seriesFrontier != nil {
				var (
					targetKey  = &dto.SampleKey{}
					foundKey   = &dto.SampleKey{}
					foundValue *dto.SampleValueSeries
				)

				for _, operation := range scanJob.operations {
					if seriesFrontier.lastTime.Before(operation.StartsAt()) {
						//						fmt.Printf("operation %s occurs after %s; aborting...\n", operation, seriesFrontier.lastTime)
						break
					}

					scanJob.operations = scanJob.operations[1:len(scanJob.operations)]

					if operation.StartsAt().Before(seriesFrontier.firstSupertime) {
						//						fmt.Printf("operation %s occurs before %s; discarding...\n", operation, seriesFrontier.firstSupertime)
						continue
					}

					if seriesFrontier.lastSupertime.Before(operation.StartsAt()) && !seriesFrontier.lastTime.Before(operation.StartsAt()) {
						targetKey.Timestamp = indexable.EncodeTime(seriesFrontier.lastSupertime)
					} else {
						targetKey.Timestamp = indexable.EncodeTime(operation.StartsAt())
					}

					targetKey.Fingerprint = scanJob.fingerprint.ToDTO()

					rawKey, _ := coding.NewProtocolBufferEncoder(targetKey).Encode()

					// XXX: Use frontiers to manage out of range queries.
					iterator.Seek(rawKey)

					foundKey, err = extractSampleKey(iterator)
					if err != nil {
						panic(err)
					}

					var (
						fst = indexable.DecodeTime(foundKey.Timestamp)
						lst = time.Unix(*foundKey.LastTimestamp, 0)
					)

					if !((operation.StartsAt().Before(fst)) || lst.Before(operation.StartsAt())) {
						//						fmt.Printf("operation %s occurs inside of %s...\n", operation, foundKey)
						foundValue, err = extractSampleValue(iterator)
						if err != nil {
							panic(err)
						}
					} else if operation.StartsAt().Before(fst) {
						fmt.Printf("operation %s may occur in next entity; fast forwarding...\n", operation)
						panic("oops")
					} else {
						panic("illegal state")
					}

					var (
						elementCount = len(foundValue.Value)
						searcher     = func(i int) bool {
							return time.Unix(*foundValue.Value[i].Timestamp, 0).After(operation.StartsAt())
						}
						index = sort.Search(elementCount, searcher)
					)

					if index != elementCount {
						if index > 0 {
							index--
						}

						foundValue.Value = foundValue.Value[index:elementCount]
					}
					switch operation.(type) {
					case getValuesAtTimeOp:
						if len(foundValue.Value) > 0 {
							view.appendSample(scanJob.fingerprint, time.Unix(*foundValue.Value[0].Timestamp, 0), model.SampleValue(*foundValue.Value[0].Value))
						}
						if len(foundValue.Value) > 1 {
							view.appendSample(scanJob.fingerprint, time.Unix(*foundValue.Value[1].Timestamp, 0), model.SampleValue(*foundValue.Value[1].Value))
						}
					default:
						panic("unhandled")
					}
				}
			}
		}

	}

	// for {
	// 	if len(s.operations) == 0 {
	// 		if len(standingOperations) > 0 {
	// 			var (
	// 				intervals = collectIntervals(standingOperations)
	// 				ranges    = collectRanges(standingOperations)
	// 			)

	// 			if len(intervals) > 0 {
	// 			}

	// 			if len(ranges) > 0 {
	// 				if len(ranges) > 0 {

	// 				}
	// 			}
	// 			break
	// 		}
	// 	}

	// 	operation := s.operations[0]
	// 	if operation.StartsAt().Equal(lastTime) {
	// 		standingOperations = append(standingOperations, operation)
	// 	} else {
	// 		standingOperations = ops{operation}
	// 		lastTime = operation.StartsAt()
	// 	}

	// 	s.operations = s.operations[1:len(s.operations)]
	// }

	viewJob.output <- view

	return
}

func (s scanJobs) Represent(d *LevelDBMetricPersistence, m memorySeriesStorage) (storage *memorySeriesStorage, err error) {

	if len(s) == 0 {
		return
	}

	iterator, closer, err := d.metricSamples.GetIterator()
	if err != nil {
		panic(err)
		return
	}
	defer closer.Close()

	diskFrontier, err := newDiskFrontier(iterator)
	if err != nil {
		panic(err)
		return
	}
	if diskFrontier == nil {
		panic("diskfrontier == nil")
	}

	for _, job := range s {
		if len(job.operations) == 0 {
			panic("len(job.operations) == 0 should never occur")
		}

		// Determine if the metric is in the known keyspace.  This is used as a
		// high-level heuristic before comparing the timestamps.
		var (
			fingerprint          = job.fingerprint
			absentDiskKeyspace   = fingerprint.Less(diskFrontier.firstFingerprint) || diskFrontier.lastFingerprint.Less(fingerprint)
			absentMemoryKeyspace = false
		)

		if _, ok := m.fingerprintToSeries[fingerprint]; !ok {
			absentMemoryKeyspace = true
		}

		var (
			firstSupertime time.Time
			lastSupertime  time.Time
		)

		var (
			_ = absentMemoryKeyspace
			_ = firstSupertime
			_ = lastSupertime
		)

		// If the key is present in the disk keyspace, we should find out the maximum
		// seek points ahead of time.  In the LevelDB case, this will save us from
		// having to dispose of and recreate the iterator.
		if !absentDiskKeyspace {
			seriesFrontier, err := newSeriesFrontier(fingerprint, *diskFrontier, iterator)
			if err != nil {
				panic(err)
				return nil, err
			}

			if seriesFrontier == nil {
				panic("ouch")
			}
		}
	}
	return
}

// 	var (
// 		memoryLowWaterMark  time.Time
// 		memoryHighWaterMark time.Time
// 	)

// 	if !absentMemoryKeyspace {
// 	}
// 	// if firstDiskFingerprint.Equal(job.fingerprint) {
// 	// 	for _, operation := range job.operations {
// 	// 		if o, ok := operation.(getMetricAtTimeOperation); ok {
// 	// 			if o.StartTime().Before(firstDiskSuperTime) {
// 	// 			}
// 	// 		}

// 	// 		if o, ok := operation.(GetMetricAtInterval); ok {
// 	// 		}
// 	// 	}
// 	// }
// }
// // // Compare the metrics on the basis of the keys.
// // firstSampleInRange = sort.IsSorted(model.Fingerprints{firstDiskFingerprint, s[0].fingerprint})
// // lastSampleInRange = sort.IsSorted(model.Fingerprints{s[s.Len()-1].fingerprint, lastDiskFingerprint})

// // if firstSampleInRange && firstDiskFingerprint.Equal(s[0].fingerprint) {
// // 	firstSampleInRange = !indexable.DecodeTime(firstKey.Timestamp).After(s.operations[0].StartTime())
// // }
// // if lastSampleInRange && lastDiskFingerprint.Equal(s[s.Len()-1].fingerprint) {
// // 	lastSampleInRange = !s.operations[s.Len()-1].StartTime().After(indexable.DecodeTime(lastKey.Timestamp))
// // }

// // for _, job := range s {
// // 	operations := job.operations
// // 	numberOfOperations := len(operations)
// // 	for j := 0; j < numberOfOperations; j++ {
// // 		operationTime := operations[j].StartTime()
// // 		group, skipAhead := collectOperationsForTime(operationTime, operations[j:numberOfOperations])
// // 		ranges := collectRanges(group)
// // 		intervals := collectIntervals(group)

// // 		fmt.Printf("ranges -> %s\n", ranges)
// // 		if len(ranges) > 0 {
// // 			fmt.Printf("d -> %s\n", peekForLongestRange(ranges, ranges[0].through))
// // 		}

// // 		j += skipAhead
// // 	}
// // }
