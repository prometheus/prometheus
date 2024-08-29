package appender

import (
	"context"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/block"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"sync"
	"sync/atomic"
)

type BlockWriter interface {
	Write(block block.Block) error
}

type QueryableStorage struct {
	blockWriter BlockWriter
	mtx         sync.Mutex
	heads       []*headWrapper

	signal chan struct{}
	close  chan struct{}
	done   chan struct{}
}

func NewQueryableStorage(blockWriter BlockWriter) *QueryableStorage {
	qs := &QueryableStorage{
		blockWriter: blockWriter,
	}
	go qs.writeLoop()
	return qs
}

func (qs *QueryableStorage) writeLoop() {
	defer close(qs.done)

	var writeFinished chan struct{}
	writeRequsted := false
	closed := false

	for {
		if closed && writeFinished == nil {
			return
		}

		if writeRequsted && writeFinished == nil {
			writeRequsted = false
			writeFinished = make(chan struct{}, 1)
			go func() {
				qs.write()
				writeFinished <- struct{}{}
			}()
		}

		select {
		case <-qs.signal:
			writeRequsted = true
		case <-writeFinished:
			writeFinished = nil
		case <-qs.close:
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

	var err error
	for _, head := range heads {
		err = nil
		err = head.head.ForEachShard(func(shard relabeler.ShardInterface) error {
			return qs.blockWriter.Write(relabeler.NewBlock(shard.LSS().Raw(), shard.DataStorage().Raw()))
		})
		if err != nil {
			// todo: log
			continue
		}
		head.persisted.Store(true)
	}

	qs.shrink()
}

func (qs *QueryableStorage) Add(head relabeler.UpgradableHeadInterface) {
	qs.mtx.Lock()
	defer qs.mtx.Unlock()
	qs.heads = append(qs.heads, &headWrapper{head: head})
}

func (qs *QueryableStorage) Querier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	qs.mtx.Lock()
	defer qs.mtx.Unlock()

	q := &storageQuerier{
		mint: mint,
		maxt: maxt,
		closer: func() error {
			qs.shrink()
			return nil
		},
	}
	for _, head := range qs.heads {
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
			continue
		}
		heads = append(heads, head)
	}
}

type headWrapper struct {
	head      relabeler.UpgradableHeadInterface
	persisted atomic.Bool
}

type storageQuerier struct {
	mint   int64
	maxt   int64
	heads  []*headWrapper
	closer func() error
}

func (q *storageQuerier) LabelValues(name string, matchers ...*labels.Matcher) ([]string, storage.Warnings, error) {
	//TODO implement me
	panic("implement me")
}

func (q *storageQuerier) LabelNames(matchers ...*labels.Matcher) ([]string, storage.Warnings, error) {
	//TODO implement me
	panic("implement me")
}

func (q *storageQuerier) Close() error {
	if q.closer != nil {
		return q.closer()
	}

	return nil
}

func (q *storageQuerier) Select(sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	//TODO implement me
	panic("implement me")
}
