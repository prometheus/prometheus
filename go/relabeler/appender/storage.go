package appender

import (
	"context"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/block"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
	"sync"
	"sync/atomic"
)

// BlockWriter writes block on disk.
type BlockWriter interface {
	Write(block block.Block) error
}

// QueryableStorage hold reference to finalized heads and writes blocks from them. Also allows query not yet not
// persisted heads.
type QueryableStorage struct {
	blockWriter BlockWriter
	mtx         sync.Mutex
	heads       []*headWrapper

	signal chan struct{}
	closer *util.Closer
}

// NewQueryableStorage - QueryableStorage constructor.
func NewQueryableStorage(blockWriter BlockWriter) *QueryableStorage {
	qs := &QueryableStorage{
		blockWriter: blockWriter,
		signal:      make(chan struct{}),
		closer:      util.NewCloser(),
	}
	go qs.loop()

	return qs
}

func (qs *QueryableStorage) loop() {
	defer qs.closer.Done()

	var writeFinished chan struct{}
	writeRequested := false
	closed := false

	for {
		if closed && writeFinished == nil {
			fmt.Println("QUERYABLE STORAGE: done")
			return
		}

		if !closed && writeRequested && writeFinished == nil {
			writeRequested = false
			writeFinished = make(chan struct{}, 1)
			go func() {
				qs.write()
				writeFinished <- struct{}{}
			}()
		}

		select {
		case <-qs.signal:
			writeRequested = true
		case <-writeFinished:
			writeFinished = nil
		case <-qs.closer.Signal():
			closed = true
		}
	}
}

func (qs *QueryableStorage) write() {
	qs.mtx.Lock()
	var heads []*headWrapper
	for _, head := range qs.heads {
		if head.persisted.Load() == false {
			heads = append(heads, head)
		}
	}
	qs.mtx.Unlock()

	fmt.Println("QUERYABLE STORAGE: write: selected ", len(heads), " heads")
	for _, head := range heads {
		err := head.head.ForEachShard(func(shard relabeler.Shard) error {
			return qs.blockWriter.Write(relabeler.NewBlock(shard.LSS().Raw(), shard.DataStorage().Raw()))
		})
		if err != nil {
			// todo: log
			fmt.Println("QUERYABLE STORAGE: failed to write head: ", err.Error())
			continue
		}
		head.persisted.Store(true)
	}

	qs.shrink()
}

// Add - Storage interface implementation.
func (qs *QueryableStorage) Add(head relabeler.Head) {
	qs.mtx.Lock()
	qs.heads = append(qs.heads, &headWrapper{head: head})
	qs.mtx.Unlock()

	select {
	case qs.signal <- struct{}{}:
	case <-qs.closer.Signal():
	}
}

func (qs *QueryableStorage) Close() error {
	return qs.closer.Close()
}

// WriteMetrics - MetricWriterTarget interface implementation.
func (qs *QueryableStorage) WriteMetrics() {
	qs.mtx.Lock()
	var heads []*headWrapper
	for _, head := range qs.heads {
		heads = append(heads, head)
	}
	defer qs.mtx.Unlock()

	for _, head := range heads {
		head.head.WriteMetrics()
	}
}

// Querier - storage.Queryable interface implementation.
func (qs *QueryableStorage) Querier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	qs.mtx.Lock()
	defer qs.mtx.Unlock()

	q := &storageQuerier{
		mint: mint,
		maxt: maxt,
		closer: func() error {
			return nil
		},
	}
	for _, head := range qs.heads {
		if head.persisted.Load() {
			continue
		}
		q.heads = append(q.heads, head)
	}

	return q, nil
}

func (qs *QueryableStorage) shrink() {
	qs.mtx.Lock()
	defer qs.mtx.Unlock()

	var heads []*headWrapper
	for _, head := range qs.heads {
		if head.persisted.Load() {
			_ = head.head.Close()
			fmt.Println("head persisted and closed")
			continue
		}
		heads = append(heads, head)
	}
	qs.heads = heads
}

type headWrapper struct {
	head      relabeler.Head
	persisted atomic.Bool
}

type storageQuerier struct {
	mint   int64
	maxt   int64
	heads  []*headWrapper
	closer func() error
}

// LabelValues - storage.Querier interface implementation.
func (q *storageQuerier) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	//TODO implement me
	panic("implement me")
}

// LabelNames - storage.Querier interface implementation.
func (q *storageQuerier) LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	//TODO implement me
	panic("implement me")
}

// Close - closes querier.
func (q *storageQuerier) Close() error {
	if q.closer != nil {
		return q.closer()
	}

	return nil
}

// Select - storage.Querier interface implementation.
func (q *storageQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	//TODO implement me
	panic("implement me")
}
