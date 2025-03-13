package scrape

import (
	"errors"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"

	pp_model "github.com/prometheus/prometheus/pp/go/model"
)

// buildersPool pool for reuse labels builders.
type buildersPool struct {
	pool *sync.Pool
}

// newBuildersPool init new *buildersPool.
func newBuildersPool() *buildersPool {
	return &buildersPool{
		pool: &sync.Pool{
			New: func() any {
				return pp_model.NewLabelSetSimpleBuilder()
			},
		},
	}
}

// get take from pool labels builder.
func (p *buildersPool) get() *pp_model.LabelSetSimpleBuilder {
	return p.pool.Get().(*pp_model.LabelSetSimpleBuilder)
}

// put return to pool labels builder.
func (p *buildersPool) put(builder *pp_model.LabelSetSimpleBuilder) {
	p.pool.Put(builder)
}

// batchesPool *sync.Pool for batches TimeSeries.
type batchesPool struct {
	pool *sync.Pool
}

// newbatchesPool init *batchesPool.
func newbatchesPool() *batchesPool {
	return &batchesPool{
		pool: &sync.Pool{
			New: func() any {
				return newBatchTimeSeries()
			},
		},
	}
}

// get return batch with TimeSeries from pool.
func (p *batchesPool) get() *BatchTimeSeries {
	batch := p.pool.Get().(*BatchTimeSeries)
	batch.setDestroyFunc(func() {
		p.put(batch)
		batch.setDestroyFunc(nil)
	})
	batch.reset()
	return batch
}

// put to poll batch with TimeSeries.
func (p *batchesPool) put(batch *BatchTimeSeries) {
	p.pool.Put(batch)
}

//
// Batch
//

var errSampleLimit = errors.New("sample limit exceeded")

const maxAheadTime = 10 * time.Minute

type Batch interface {
	// Add add to batch timeseries, timestamp and value.
	Add(builder *pp_model.LabelSetSimpleBuilder, timestamp uint64, value float64) error
	// Destroy destroy batch with destroyFunc(return to pool).
	Destroy()
	// IsEmpty check batch is empty.
	IsEmpty() bool
	// Len return current length data.
	Len() int
	// TimeSeries return batched slice TimeSeries.
	TimeSeries() []pp_model.TimeSeries
}

// BatchWithLimit wrap batch timeseries into limits.
func BatchWithLimit(batch Batch) Batch {
	batch = &timeLimitBatch{
		Batch:   batch,
		maxTime: uint64(timestamp.FromTime(time.Now().Add(maxAheadTime))),
	}

	return batch
}

// BatchTimeSeries batch time series accumulated from source.
type BatchTimeSeries struct {
	data        []pp_model.TimeSeries
	destroyFunc func()
}

var _ Batch = (*BatchTimeSeries)(nil)

// newBatchTimeSeries init new *BatchTimeSeries.
func newBatchTimeSeries() *BatchTimeSeries {
	return &BatchTimeSeries{}
}

// Add add to batch timeseries, timestamp and value.
func (batch *BatchTimeSeries) Add(builder *pp_model.LabelSetSimpleBuilder, timestamp uint64, val float64) error {
	batch.data = append(
		batch.data,
		pp_model.TimeSeries{
			LabelSet:  builder.Build(),
			Timestamp: timestamp,
			Value:     val,
		},
	)

	return nil
}

// Destroy destroy batch with destroyFunc(return to pool).
func (batch *BatchTimeSeries) Destroy() {
	if batch.destroyFunc == nil {
		return
	}
	batch.destroyFunc()
}

// IsEmpty check batch is empty.
func (batch *BatchTimeSeries) IsEmpty() bool {
	return len(batch.data) == 0
}

// Len return current length data.
func (batch *BatchTimeSeries) Len() int {
	return len(batch.data)
}

// TimeSeries return batched slice TimeSeries.
func (batch *BatchTimeSeries) TimeSeries() []pp_model.TimeSeries {
	return batch.data
}

// reset clear batch.
func (batch *BatchTimeSeries) reset() {
	batch.data = batch.data[:0]
}

// setDestroyFunc set destroy function.
func (batch *BatchTimeSeries) setDestroyFunc(destroyFunc func()) {
	batch.destroyFunc = destroyFunc
}

// samplesLimitBatch limits the number of total appended samples in a batch.
type samplesLimitBatch struct {
	Batch

	limit int
	i     int
}

var _ Batch = (*samplesLimitBatch)(nil)

// Add add to batch timeseries, timestamp and value.
func (b *samplesLimitBatch) Add(builder *pp_model.LabelSetSimpleBuilder, timestamp uint64, val float64) error {
	if !value.IsStaleNaN(val) {
		b.i++
		if b.i > b.limit {
			return errSampleLimit
		}
	}

	return b.Batch.Add(builder, timestamp, val)
}

// timeLimitBatch limits time on sample.
type timeLimitBatch struct {
	Batch

	maxTime uint64
}

var _ Batch = (*timeLimitBatch)(nil)

// Add add to batch timeseries, timestamp and value.
func (b *timeLimitBatch) Add(builder *pp_model.LabelSetSimpleBuilder, timestamp uint64, val float64) error {
	if timestamp > b.maxTime {
		return storage.ErrOutOfBounds
	}

	return b.Batch.Add(builder, timestamp, val)
}
