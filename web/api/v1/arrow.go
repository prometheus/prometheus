package v1

import (
	"fmt"
	"io"

	"github.com/apache/arrow/go/v11/arrow"
	"github.com/apache/arrow/go/v11/arrow/array"
	"github.com/apache/arrow/go/v11/arrow/ipc"
	"github.com/apache/arrow/go/v11/arrow/memory"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/pool"
)

type arrowEncoder struct {
	logger log.Logger

	w         io.Writer
	allocator memory.Allocator
}

func newArrowEncoder(logger log.Logger, w io.Writer, allocator memory.Allocator) *arrowEncoder {
	return &arrowEncoder{
		logger:    logger,
		w:         w,
		allocator: allocator,
	}
}

func (enc *arrowEncoder) EncodeMatrix(matrix promql.Matrix) error {
	longest := 0
	for _, series := range matrix {
		if len(series.Points) > longest {
			longest = len(series.Points)
		}
		if len(series.Points) > 0 && series.Points[0].H != nil {
			return fmt.Errorf("arrow not implemented for native histograms")
		}
	}

	tsBuilder := array.NewTimestampBuilder(enc.allocator, &arrow.TimestampType{Unit: arrow.Millisecond})
	tsBuilder.Reserve(longest)

	values := array.NewFloat64Builder(enc.allocator)
	values.Reserve(longest)
	for _, series := range matrix {
		err := enc.writeSeries(series, tsBuilder, values)
		if err != nil {
			level.Error(enc.logger).Log("msg", "error writing arrow response", "err", err)
			return nil
		}
	}

	return nil
}

func (enc *arrowEncoder) writeSeries(series promql.Series, timestamps *array.TimestampBuilder, values *array.Float64Builder) error {
	for _, point := range series.Points {
		timestamps.UnsafeAppend(arrow.Timestamp(point.T))
		values.UnsafeAppend(point.V)
	}

	var fields = []arrow.Field{
		{Name: "t", Type: timestamps.Type()},
		{Name: "v", Type: values.Type()},
	}
	metadata := arrow.MetadataFrom(series.Metric.Map())
	seriesSchema := arrow.NewSchema(fields, &metadata)
	writer := ipc.NewWriter(enc.w, ipc.WithAllocator(enc.allocator), ipc.WithSchema(seriesSchema))
	defer writer.Close()

	b := array.NewRecordBuilder(enc.allocator, seriesSchema)
	defer b.Release()

	ts := timestamps.NewArray()
	defer ts.Release()
	vs := values.NewArray()
	defer vs.Release()

	rec := array.NewRecord(seriesSchema, []arrow.Array{
		ts,
		vs,
	}, int64(ts.Len()))
	defer rec.Release()

	if err := writer.Write(rec); err != nil {
		return err
	}

	return nil
}

type arrowAllocator struct {
	memory.Allocator
	pool *pool.Pool
}

func newArrowAllocator() *arrowAllocator {
	base := memory.DefaultAllocator
	return &arrowAllocator{
		Allocator: memory.DefaultAllocator,
		pool: pool.New(1e3, 100e6, 3, func(size int) interface{} {
			return base.Allocate(size)
		}),
	}
}

func (a *arrowAllocator) Allocate(size int) []byte {
	b := a.pool.Get(size).([]byte)
	return b[:size]
}

func (a *arrowAllocator) Reallocate(size int, b []byte) []byte {
	if cap(b) >= size {
		return b[:size]
	}
	a.Free(b)
	newB := a.pool.Get(size).([]byte)[:size]
	copy(newB, b)
	return newB
}

func (a *arrowAllocator) Free(b []byte) {
	if b != nil {
		a.pool.Put(b)
	}
}
