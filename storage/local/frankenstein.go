// Copyright 2016 The Prometheus Authors

package local

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/frankenstein/wire"
	"github.com/prometheus/prometheus/storage/metric"
)

const (
	flushCheckPeriod = 5 * time.Second
	maxChunkAge      = 30 * time.Second
)

// Ingestor deals with "in flight" chunks.
// Its like MemorySeriesStorage, but simpler.
type Ingestor struct {
	chunkStore ChunkStore
	quit       chan struct{}
	done       chan struct{}

	fpLocker   *fingerprintLocker
	fpToSeries *seriesMap
	mapper     *fpMapper

	ingestedSamples    prometheus.Counter
	discardedSamples   *prometheus.CounterVec
	storedChunks       prometheus.Counter
	chunkStoreFailures prometheus.Counter
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
	return fp, series, nil
}

func (i *Ingestor) Query(from, to model.Time, matchers ...*metric.LabelMatcher) (model.Matrix, error) {
	return model.Matrix{}, nil
}

func (i *Ingestor) Stop() {
	close(i.quit)
	<-i.done
}

func (i *Ingestor) loop() {
	defer close(i.done)
	tick := time.Tick(flushCheckPeriod)
	for {
		select {
		case <-tick:
			for pair := range i.fpToSeries.iter() {
				if err := i.flushSeries(pair.fp, pair.series); err != nil {
					log.Errorf("Failed to flush chunks for series: %v", err)
				}
			}
		case <-i.quit:
			return
		}
	}
}

func (i *Ingestor) flushSeries(fp model.Fingerprint, series *memorySeries) error {
	i.fpLocker.Lock(fp)

	// Decide what chunks to flush
	if time.Now().Sub(series.firstTime().Time()) > maxChunkAge {
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
	if err := i.flushChunks(fp, series.metric, chunks); err != nil {
		i.chunkStoreFailures.Add(float64(len(chunks)))
		return err
	}
	i.storedChunks.Add(float64(len(chunks)))

	// now remove the chunks
	// TODO deal with iterator reading from them
	i.fpLocker.Lock(fp)
	series.chunkDescs = series.chunkDescs[len(chunks):]
	if len(series.chunkDescs) == 0 {
		i.fpToSeries.del(fp)
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
}

// Collect implements prometheus.Collector.
func (i *Ingestor) Collect(ch chan<- prometheus.Metric) {
	i.mapper.Collect(ch)

	ch <- i.ingestedSamples
	i.discardedSamples.Collect(ch)
	ch <- i.storedChunks
	ch <- i.chunkStoreFailures
}
