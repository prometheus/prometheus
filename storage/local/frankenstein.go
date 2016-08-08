// Copyright 2016 The Prometheus Authors

package local

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/frankenstein/wire"
	"github.com/prometheus/prometheus/storage/metric"
)

const (
	flushCheckPeriod = 1 * time.Minute
	maxChunkAge      = 10 * time.Minute
)

// Ingestor deals with "in flight" chunks.
// Its like MemorySeriesStorage, but simpler.
type Ingestor struct {
	chunkStore ChunkStore
	stopLock   sync.RWMutex
	stopped    bool
	quit       chan struct{}
	done       chan struct{}

	fpLocker   *fingerprintLocker
	fpToSeries *seriesMap
	mapper     *fpMapper
	index      *invertedIndex

	ingestedSamples    prometheus.Counter
	discardedSamples   *prometheus.CounterVec
	storedChunks       prometheus.Counter
	chunkStoreFailures prometheus.Counter
	queries            prometheus.Counter
	queriedSamples     prometheus.Counter
}

type ChunkStore interface {
	Put([]wire.Chunk) error
	Get(from, through model.Time, matchers ...*metric.LabelMatcher) ([]wire.Chunk, error)
}

func NewIngestor(chunkStore ChunkStore) (*Ingestor, error) {
	i := &Ingestor{
		chunkStore: chunkStore,
		quit:       make(chan struct{}),
		done:       make(chan struct{}),

		fpToSeries: newSeriesMap(),
		fpLocker:   newFingerprintLocker(16),
		index:      newInvertedIndex(),

		ingestedSamples: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "ingested_samples_total",
			Help:      "The total number of samples ingested.",
		}),
		discardedSamples: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "out_of_order_samples_total",
				Help:      "The total number of samples that were discarded because their timestamps were at or before the last received sample for a series.",
			},
			[]string{discardReasonLabel},
		),
		storedChunks: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "stored_chunks_total",
			Help:      "The total number of chunks sent to the chunk store.",
		}),
		chunkStoreFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "chunk_store_failures_total",
			Help:      "The total number of errors while storing chunks to the chunk store.",
		}),
		queries: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queries_total",
			Help:      "The total number of queries the ingestor has handled.",
		}),
		queriedSamples: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queried_samples_total",
			Help:      "The total number of samples returned from queries.",
		}),
	}
	var err error
	i.mapper, err = newFPMapper(i.fpToSeries, noopPersistence{})
	if err != nil {
		return nil, err
	}

	go i.loop()
	return i, nil
}

func (*Ingestor) NeedsThrottling() bool {
	return false
}

func (i *Ingestor) Append(sample *model.Sample) error {
	for ln, lv := range sample.Metric {
		if len(lv) == 0 {
			delete(sample.Metric, ln)
		}
	}

	i.stopLock.RLock()
	defer i.stopLock.RUnlock()
	if i.stopped {
		return fmt.Errorf("ingestor stopping")
	}

	fp, series, err := i.getOrCreateSeries(sample.Metric)
	if err != nil {
		return err
	}
	defer func() {
		i.fpLocker.Unlock(fp)
	}()

	if sample.Timestamp == series.lastTime {
		// Don't report "no-op appends", i.e. where timestamp and sample
		// value are the same as for the last append, as they are a
		// common occurrence when using client-side timestamps
		// (e.g. Pushgateway or federation).
		if sample.Timestamp == series.lastTime &&
			series.lastSampleValueSet &&
			sample.Value.Equal(series.lastSampleValue) {
			return nil
		}
		i.discardedSamples.WithLabelValues(duplicateSample).Inc()
		return ErrDuplicateSampleForTimestamp // Caused by the caller.
	}
	if sample.Timestamp < series.lastTime {
		i.discardedSamples.WithLabelValues(outOfOrderTimestamp).Inc()
		return ErrOutOfOrderSample // Caused by the caller.
	}
	_, err = series.add(model.SamplePair{
		Value:     sample.Value,
		Timestamp: sample.Timestamp,
	})
	if err == nil {
		// TODO: Track append failures too (unlikely to happen).
		i.ingestedSamples.Inc()
	}
	return err
}

func (i *Ingestor) getOrCreateSeries(metric model.Metric) (model.Fingerprint, *memorySeries, error) {
	rawFP := metric.FastFingerprint()
	i.fpLocker.Lock(rawFP)
	fp := i.mapper.mapFP(rawFP, metric)
	if fp != rawFP {
		i.fpLocker.Unlock(rawFP)
		i.fpLocker.Lock(fp)
	}

	series, ok := i.fpToSeries.get(fp)
	if ok {
		return fp, series, nil
	}

	var err error
	series, err = newMemorySeries(metric, nil, time.Time{})
	if err != nil {
		// err should always be nil when chunkDescs are nil
		panic(err)
	}
	i.fpToSeries.put(fp, series)
	i.index.add(metric, fp)
	return fp, series, nil
}

func (i *Ingestor) Query(from, through model.Time, matchers ...*metric.LabelMatcher) (model.Matrix, error) {
	i.queries.Inc()

	fps := i.index.lookup(matchers)
	log.Infof("Query: should return %v", fps)

	// fps is sorted, lock them in order to prevent deadlocks
	queriedSamples := 0
	result := model.Matrix{}
	for _, fp := range fps {
		i.fpLocker.Lock(fp)
		series, ok := i.fpToSeries.get(fp)
		if !ok {
			i.fpLocker.Unlock(fp)
			continue
		}

		values, err := samplesForRange(series, from, through)
		i.fpLocker.Unlock(fp)
		if err != nil {
			return nil, err
		}

		result = append(result, &model.SampleStream{
			Metric: series.metric,
			Values: values,
		})
		queriedSamples += len(values)
	}

	i.queriedSamples.Add(float64(queriedSamples))

	return result, nil
}

func samplesForRange(s *memorySeries, from, through model.Time) ([]model.SamplePair, error) {
	// Find first chunk with start time after "from".
	fromIdx := sort.Search(len(s.chunkDescs), func(i int) bool {
		return s.chunkDescs[i].firstTime().After(from)
	})
	// Find first chunk with start time after "through".
	throughIdx := sort.Search(len(s.chunkDescs), func(i int) bool {
		return s.chunkDescs[i].firstTime().After(through)
	})
	if fromIdx == len(s.chunkDescs) {
		// Even the last chunk starts before "from". Find out if the
		// series ends before "from" and we don't need to do anything.
		lt, err := s.chunkDescs[len(s.chunkDescs)-1].lastTime()
		if err != nil {
			return nil, err
		}
		if lt.Before(from) {
			return nil, nil
		}
	}
	if fromIdx > 0 {
		fromIdx--
	}
	if throughIdx == len(s.chunkDescs) {
		throughIdx--
	}
	var values []model.SamplePair
	in := metric.Interval{
		OldestInclusive: from,
		NewestInclusive: through,
	}
	for idx := fromIdx; idx <= throughIdx; idx++ {
		cd := s.chunkDescs[idx]
		chValues, err := rangeValues(cd.c.newIterator(), in)
		if err != nil {
			return nil, err
		}
		values = append(values, chValues...)
	}
	return values, nil
}

func (i *Ingestor) Stop() {
	i.stopLock.Lock()
	i.stopped = true
	i.stopLock.Unlock()

	close(i.quit)
	<-i.done
}

func (i *Ingestor) loop() {
	defer i.flushAllSeries(true)
	defer close(i.done)
	tick := time.Tick(flushCheckPeriod)
	for {
		select {
		case <-tick:
			i.flushAllSeries(false)
		case <-i.quit:
			return
		}
	}
}

func (i *Ingestor) flushAllSeries(immediate bool) {
	if i.chunkStore == nil {
		return
	}
	for pair := range i.fpToSeries.iter() {
		if err := i.flushSeries(pair.fp, pair.series, immediate); err != nil {
			log.Errorf("Failed to flush chunks for series: %v", err)
		}
	}
}

func (i *Ingestor) flushSeries(fp model.Fingerprint, series *memorySeries, immediate bool) error {
	i.fpLocker.Lock(fp)

	// Decide what chunks to flush
	if immediate || time.Now().Sub(series.firstTime().Time()) > maxChunkAge {
		series.headChunkClosed = true
		series.headChunkUsedByIterator = false
		series.head().maybePopulateLastTime()
	}
	chunks := series.chunkDescs
	if !series.headChunkClosed {
		chunks = chunks[:len(chunks)-1]
	}
	if len(chunks) == 0 {
		i.fpLocker.Unlock(fp)
		return nil
	}

	// flush the chunks without locking the series
	i.fpLocker.Unlock(fp)
	log.Infof("Flushing %d chunks", len(chunks))
	if err := i.flushChunks(fp, series.metric, chunks); err != nil {
		i.chunkStoreFailures.Add(float64(len(chunks)))
		return err
	}
	i.storedChunks.Add(float64(len(chunks)))

	// now remove the chunks
	i.fpLocker.Lock(fp)
	series.chunkDescs = series.chunkDescs[len(chunks):]
	if len(series.chunkDescs) == 0 {
		i.fpToSeries.del(fp)
		i.index.delete(series.metric, fp)
	}
	i.fpLocker.Unlock(fp)
	return nil
}

func (i *Ingestor) flushChunks(fp model.Fingerprint, metric model.Metric, chunks []*chunkDesc) error {
	wireChunks := make([]wire.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		buf := make([]byte, chunkLen)
		if err := chunk.c.marshalToBuf(buf); err != nil {
			return err
		}

		wireChunks = append(wireChunks, wire.Chunk{
			ID:      fmt.Sprintf("%d:%d:%d", fp, chunk.chunkFirstTime, chunk.chunkLastTime),
			From:    chunk.chunkFirstTime,
			Through: chunk.chunkLastTime,
			Metric:  metric,
			Data:    buf,
		})
	}
	return i.chunkStore.Put(wireChunks)
}

// Describe implements prometheus.Collector.
func (i *Ingestor) Describe(ch chan<- *prometheus.Desc) {
	i.mapper.Describe(ch)

	ch <- i.ingestedSamples.Desc()
	i.discardedSamples.Describe(ch)
	ch <- i.storedChunks.Desc()
	ch <- i.chunkStoreFailures.Desc()
	ch <- i.queries.Desc()
	ch <- i.queriedSamples.Desc()
}

// Collect implements prometheus.Collector.
func (i *Ingestor) Collect(ch chan<- prometheus.Metric) {
	i.mapper.Collect(ch)

	ch <- i.ingestedSamples
	i.discardedSamples.Collect(ch)
	ch <- i.storedChunks
	ch <- i.chunkStoreFailures
	ch <- i.queries
	ch <- i.queriedSamples
}

type invertedIndex struct {
	mtx sync.RWMutex
	idx map[model.LabelName]map[model.LabelValue][]model.Fingerprint // entries are sorted in fp order?
}

func newInvertedIndex() *invertedIndex {
	return &invertedIndex{
		idx: map[model.LabelName]map[model.LabelValue][]model.Fingerprint{},
	}
}

func (i *invertedIndex) add(metric model.Metric, fp model.Fingerprint) {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	for name, value := range metric {
		values, ok := i.idx[name]
		if !ok {
			values = map[model.LabelValue][]model.Fingerprint{}
		}
		fingerprints := values[value]
		j := sort.Search(len(fingerprints), func(i int) bool {
			return fingerprints[i] >= fp
		})
		fingerprints = append(fingerprints, 0)
		copy(fingerprints[j+1:], fingerprints[j:])
		fingerprints[j] = fp
		values[value] = fingerprints
		i.idx[name] = values
	}
}

func (i *invertedIndex) lookup(matchers []*metric.LabelMatcher) []model.Fingerprint {
	if len(matchers) == 0 {
		return nil
	}
	i.mtx.RLock()
	defer i.mtx.RUnlock()

	// intersection is initially nil, which is a special case.
	var intersection []model.Fingerprint
	for _, matcher := range matchers {
		values, ok := i.idx[matcher.Name]
		if !ok {
			return nil
		}
		var toIntersect []model.Fingerprint
		for value, fps := range values {
			if matcher.Match(value) {
				toIntersect = merge(toIntersect, fps)
			}
		}
		intersection = intersect(intersection, toIntersect)
		if len(intersection) == 0 {
			return nil
		}
	}

	return intersection
}

func (i *invertedIndex) delete(metric model.Metric, fp model.Fingerprint) {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	for name, value := range metric {
		values, ok := i.idx[name]
		if !ok {
			continue
		}
		fingerprints, ok := values[value]
		if !ok {
			continue
		}

		j := sort.Search(len(fingerprints), func(i int) bool {
			return fingerprints[i] >= fp
		})
		fingerprints = fingerprints[:j+copy(fingerprints[j:], fingerprints[j+1:])]

		if len(fingerprints) == 0 {
			delete(values, value)
		} else {
			values[value] = fingerprints
		}

		if len(values) == 0 {
			delete(i.idx, name)
		} else {
			i.idx[name] = values
		}
	}
}

// intersect two sorted lists of fingerprints.  Assumes there are no duplicate
// fingerprints within the input lists.
func intersect(a, b []model.Fingerprint) []model.Fingerprint {
	if a == nil {
		return b
	}
	result := []model.Fingerprint{}
	for i, j := 0, 0; i < len(a) && j < len(b); {
		if a[i] == b[j] {
			result = append(result, a[i])
		}
		if a[i] < b[j] {
			i++
		} else {
			j++
		}
	}
	return result
}

// merge two sorted lists of fingerprints.  Assumes there are no duplicate
// fingerprints between or within the input lists.
func merge(a, b []model.Fingerprint) []model.Fingerprint {
	result := make([]model.Fingerprint, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] < b[j] {
			result = append(result, a[i])
			i++
		} else {
			result = append(result, b[j])
			j++
		}
	}
	for ; i < len(a); i++ {
		result = append(result, a[i])
	}
	for ; j < len(b); j++ {
		result = append(result, b[j])
	}
	return result
}
