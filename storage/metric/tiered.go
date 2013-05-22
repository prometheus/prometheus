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

	memoryArena         *memorySeriesStorage
	memoryTTL           time.Duration
	flushMemoryInterval time.Duration

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

	t.memoryArena.AppendSamples(samples)

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
func (t *TieredStorage) MakeView(builder ViewRequestBuilder, deadline time.Duration) (View, error) {
	if len(t.draining) > 0 {
		return nil, fmt.Errorf("Storage is in the process of draining.")
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
	case view := <-result:
		return view, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(deadline):
		abortChan <- true
		return nil, fmt.Errorf("MakeView timed out after %s.", deadline)
	}
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
			t.flushMemory(t.memoryTTL)
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
	t.flushMemory(0)
}

func (t *TieredStorage) flushMemory(ttl time.Duration) {
	t.memoryArena.RLock()
	defer t.memoryArena.RUnlock()

	cutOff := time.Now().Add(-1 * ttl)

	log.Println("Flushing...")

	for _, stream := range t.memoryArena.fingerprintToSeries {
		finder := func(i int) bool {
			return !cutOff.After(stream.values[i].Timestamp)
		}

		stream.Lock()

		i := sort.Search(len(stream.values), finder)
		toArchive := stream.values[i:]
		toKeep := stream.values[:i]
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
	log.Println("Closing tiered storage...")
	t.Drain()
	t.DiskStorage.Close()
	t.memoryArena.Close()

	close(t.appendToDiskQueue)
	close(t.viewQueue)
	log.Println("Done.")
}

func (t *TieredStorage) renderView(viewJob viewJob) {
	// Telemetry.
	var err error
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: renderView, result: success}, map[string]string{operation: renderView, result: failure})
	}()

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
			if (len(memValues) == 0 || memValues.FirstTimeAfter(targetTime)) && seriesFrontier != nil {
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
		fingerprintSet[*fingerprint] = true
	}
	for fingerprint := range fingerprintSet {
		fpCopy := fingerprint
		fingerprints = append(fingerprints, &fpCopy)
	}

	return
}

// Get the metric associated with the provided fingerprint.
func (t *TieredStorage) GetMetricForFingerprint(f *model.Fingerprint) (m model.Metric, err error) {
	m, err = t.memoryArena.GetMetricForFingerprint(f)
	if err != nil {
		return
	}
	if m == nil {
		m, err = t.DiskStorage.GetMetricForFingerprint(f)
	}
	return
}
