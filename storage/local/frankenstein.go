package local

import (
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/frankenstein/wire"
	"github.com/prometheus/prometheus/storage/metric"
)

const (
	flushCheckPeriod = 1 * time.Minute
)

// Ingestor deals with "in flight" chunks.
// Its like memseriesstorage, but simplier.
type Ingestor struct {
	chunkStore ChunkStore
	quit       chan struct{}
	done       chan struct{}

	fpLocker   *fingerprintLocker
	fpToSeries *seriesMap
	mapper     *fpMapper
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
	}
	go i.loop()
	return i, nil
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
		return ErrDuplicateSampleForTimestamp // Caused by the caller.
	}
	if sample.Timestamp < series.lastTime {
		return ErrOutOfOrderSample // Caused by the caller.
	}
	_, err = series.add(model.SamplePair{
		Value:     sample.Value,
		Timestamp: sample.Timestamp,
	})
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
	if err := i.flushChunks(series.metric, chunks); err != nil {
		return err
	}

	// now remove the chunks
	// TODO deal with iterator reading from them
	i.fpLocker.Lock(fp)
	series.chunkDescs = series.chunkDescs[len(chunks):]
	i.fpLocker.Unlock(fp)
	return nil
}

func (i *Ingestor) flushChunks(metric model.Metric, chunks []*chunkDesc) error {
	wireChunks := make([]wire.Chunk, len(chunks))
	for _, chunk := range chunks {
		buf := make([]byte, chunkLen)
		if err := chunk.c.marshalToBuf(buf); err != nil {
			return err
		}
		wireChunks = append(wireChunks, wire.Chunk{
			ID:      "",
			From:    chunk.chunkFirstTime,
			Through: chunk.chunkLastTime,
			Metric:  metric,
			Data:    buf,
		})
	}
	return i.chunkStore.Put(wireChunks)
}
