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
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/storage"
	"sync"
	"time"
)

// tieredStorage both persists samples and generates materialized views for
// queries.
type tieredStorage struct {
	appendToDiskQueue   chan model.Sample
	appendToMemoryQueue chan model.Sample
	diskStorage         *LevelDBMetricPersistence
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

type Storage interface {
	AppendSample(model.Sample)
	MakeView(ViewRequestBuilder, time.Duration) (View, error)
	Serve()
	Expose()
}

func NewTieredStorage(appendToMemoryQueueDepth, appendToDiskQueueDepth, viewQueueDepth uint, flushMemoryInterval, writeMemoryInterval, memoryTTL time.Duration) Storage {
	diskStorage, err := NewLevelDBMetricPersistence("/tmp/metrics-foof")
	if err != nil {
		panic(err)
	}

	return &tieredStorage{
		appendToDiskQueue:   make(chan model.Sample, appendToDiskQueueDepth),
		appendToMemoryQueue: make(chan model.Sample, appendToMemoryQueueDepth),
		diskStorage:         diskStorage,
		flushMemoryInterval: flushMemoryInterval,
		memoryArena:         NewMemorySeriesStorage(),
		memoryTTL:           memoryTTL,
		viewQueue:           make(chan viewJob, viewQueueDepth),
		writeMemoryInterval: writeMemoryInterval,
	}
}

func (t *tieredStorage) AppendSample(s model.Sample) {
	t.appendToMemoryQueue <- s
}

func (t *tieredStorage) MakeView(builder ViewRequestBuilder, deadline time.Duration) (view View, err error) {
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

func (t *tieredStorage) Expose() {
	ticker := time.Tick(5 * time.Second)
	f := model.NewFingerprintFromRowKey("05232115763668508641-g-97-d")
	for {
		<-ticker

		var (
			first  = time.Now()
			second = first.Add(1 * time.Minute)
			third  = first.Add(2 * time.Minute)
		)

		vrb := NewViewRequestBuilder()
		fmt.Printf("vrb -> %s\n", vrb)
		vrb.GetMetricRange(f, first, second)
		vrb.GetMetricRange(f, first, third)
		js := vrb.ScanJobs()
		consume(js[0])
		// fmt.Printf("js -> %s\n", js)
		// js.Represent(t.diskStorage, t.memoryArena)
		// 		i, c, _ := t.diskStorage.metricSamples.GetIterator()
		// 		start := time.Now()
		// 		f, _ := newDiskFrontier(i)
		// 		fmt.Printf("df -> %s\n", time.Since(start))
		// 		fmt.Printf("df -- -> %s\n", f)
		// 		start = time.Now()
		// //		sf, _ := newSeriesFrontier(model.NewFingerprintFromRowKey("05232115763668508641-g-97-d"), *f, i)
		// //		sf, _ := newSeriesFrontier(model.NewFingerprintFromRowKey("16879485108969112708-g-184-s"), *f, i)
		// 		sf, _ := newSeriesFrontier(model.NewFingerprintFromRowKey("08437776163162606855-g-169-s"), *f, i)
		// 		fmt.Printf("sf -> %s\n", time.Since(start))
		// 		fmt.Printf("sf -- -> %s\n", sf)
		// 		c.Close()
	}
}

func (t *tieredStorage) Serve() {
	var (
		flushMemoryTicker = time.Tick(t.flushMemoryInterval)
		writeMemoryTicker = time.Tick(t.writeMemoryInterval)
	)
	for {
		select {
		case <-writeMemoryTicker:
			t.writeMemory()
		case <-flushMemoryTicker:
			t.flushMemory()
		case viewRequest := <-t.viewQueue:
			t.renderView(viewRequest)
		}
	}
}

func (t *tieredStorage) writeMemory() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	pendingLength := len(t.appendToMemoryQueue)

	for i := 0; i < pendingLength; i++ {
		t.memoryArena.AppendSample(<-t.appendToMemoryQueue)
	}
}

// Write all pending appends.
func (t *tieredStorage) flush() (err error) {
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

	fmt.Printf("fingerprint -> %s\n", model.NewFingerprintFromMetric(stream.metric).ToRowKey())

	return visitor, visitor, visitor
}

func (f *memoryToDiskFlusher) Flush() {
	length := len(f.toDiskQueue)
	samples := model.Samples{}
	for i := 0; i < length; i++ {
		samples = append(samples, <-f.toDiskQueue)
	}
	fmt.Printf("%d samples to write\n", length)
	f.disk.AppendSamples(samples)
}

func (f memoryToDiskFlusher) Close() {
	fmt.Println("memory flusher close")
	f.Flush()
}

// Persist a whole bunch of samples to the datastore.
func (t *tieredStorage) flushMemory() {

	t.mutex.Lock()
	defer t.mutex.Unlock()

	flusher := &memoryToDiskFlusher{
		disk:        t.diskStorage,
		olderThan:   time.Now().Add(-1 * t.memoryTTL),
		toDiskQueue: t.appendToDiskQueue,
	}
	defer flusher.Close()

	v := time.Now()
	t.memoryArena.ForEachSample(flusher)
	fmt.Printf("Done flushing memory in %s", time.Since(v))

	return
}

func (t *tieredStorage) renderView(viewJob viewJob) (err error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	return
}


func consume(s scanJob) {
	var (
		standingOperations = ops{}
		lastTime           = time.Time{}
	)

	for {
		if len(s.operations) == 0 {
			if len(standingOperations) > 0 {
				var (
					intervals = collectIntervals(standingOperations)
					ranges    = collectRanges(standingOperations)
				)

				if len(intervals) > 0 {
				}

				if len(ranges) > 0 {
					if len(ranges) > 0 {

					}
				}
				break
			}
		}

		operation := s.operations[0]
		if operation.StartsAt().Equal(lastTime) {
			standingOperations = append(standingOperations, operation)
		} else {
			standingOperations = ops{operation}
			lastTime = operation.StartsAt()
		}

		s.operations = s.operations[1:len(s.operations)]
	}
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
