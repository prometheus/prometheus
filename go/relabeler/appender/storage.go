package appender

import (
	"fmt"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/block"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/storage"
	"strings"
	"sync"
	"time"
)

const (
	PersistedHeadValue = -(1 << 30)
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
	heads       []relabeler.Head

	signal chan struct{}
	closer *util.Closer

	headPersistenceDuration *prometheus.GaugeVec
	querierMetrics          *querier.Metrics
}

// NewQueryableStorage - QueryableStorage constructor.
func NewQueryableStorage(blockWriter BlockWriter, registerer prometheus.Registerer, querierMetrics *querier.Metrics) *QueryableStorage {
	factory := util.NewUnconflictRegisterer(registerer)
	qs := &QueryableStorage{
		blockWriter: blockWriter,
		signal:      make(chan struct{}),
		closer:      util.NewCloser(),
		headPersistenceDuration: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_persistence_duration_duration",
				Help: "Block write duration in milliseconds.",
			},
			[]string{"generation"},
		),
		querierMetrics: querierMetrics,
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
	var heads []relabeler.Head
	for _, head := range qs.heads {
		if head.ReferenceCounter().Value() >= 0 {
			heads = append(heads, head)
		}
	}
	qs.mtx.Unlock()

	var headList []string
	for _, head := range heads {
		headList = append(headList, fmt.Sprintf("%d", head.Generation()))
	}

	fmt.Println("QUERYABLE STORAGE: write: selected {", strings.Join(headList, ", "), "} heads")
	for _, head := range heads {
		start := time.Now()
		err := head.ForEachShard(func(shard relabeler.Shard) error {
			return qs.blockWriter.Write(relabeler.NewBlock(shard.LSS().Raw(), shard.DataStorage().Raw()))
		})
		if err != nil {
			// todo: log
			fmt.Println("QUERYABLE STORAGE: failed to write head: ", err.Error())
			continue
		}
		qs.headPersistenceDuration.With(prometheus.Labels{
			"generation": fmt.Sprintf("%d", head.Generation()),
		}).Set(float64(time.Since(start).Milliseconds()))
		head.ReferenceCounter().Add(PersistedHeadValue)
		fmt.Println("QUERYABLE STORAGE: head {", head.Generation(), "} persisted, duration: ", time.Since(start).Nanoseconds(), "ms")
	}

	qs.shrink()
}

// Add - Storage interface implementation.
func (qs *QueryableStorage) Add(head relabeler.Head) {
	qs.mtx.Lock()
	qs.heads = append(qs.heads, head)
	fmt.Println("QUERYABLE STORAGE: head {", head.Generation(), "} added")
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
	var heads []relabeler.Head
	for _, head := range qs.heads {
		if head.ReferenceCounter().Add(1) < 0 {
			head.ReferenceCounter().Add(-1)
			continue
		}
		heads = append(heads, head)
	}
	qs.mtx.Unlock()

	for _, head := range heads {
		head.WriteMetrics()
		head.ReferenceCounter().Add(-1)
	}

	qs.shrink()
}

// Querier - storage.Queryable interface implementation.
func (qs *QueryableStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	qs.mtx.Lock()

	var heads []relabeler.Head
	for _, head := range qs.heads {
		if head.ReferenceCounter().Add(1) < 0 {
			head.ReferenceCounter().Add(-1)
			continue
		}
		heads = append(heads, head)
	}
	qs.mtx.Unlock()

	var queriers []storage.Querier
	for _, head := range heads {
		h := head
		queriers = append(
			queriers,
			querier.NewQuerier(
				h,
				querier.NoOpShardedDeduplicatorFactory(),
				mint,
				maxt,
				func() error {
					h.ReferenceCounter().Add(-1)
					return nil
				},
				qs.querierMetrics,
			),
		)
	}

	q := querier.NewMultiQuerier(
		queriers,
		func() error {
			qs.shrink()
			return nil
		},
	)

	return q, nil
}

func (qs *QueryableStorage) shrink() {
	qs.mtx.Lock()
	defer qs.mtx.Unlock()

	var heads []relabeler.Head
	for _, head := range qs.heads {
		fmt.Println("QUERYABLE STORAGE: SHRINK: HEAD {", head.Generation(), "}: persisted: ", head.ReferenceCounter().Value() < 0, ", ref count: ", refCount(head.ReferenceCounter().Value()))
		if head.ReferenceCounter().Value() == PersistedHeadValue {
			_ = head.Close()
			fmt.Println("QUERYABLE STORAGE: head {", head.Generation(), "} persisted and closed")
			continue
		}
		heads = append(heads, head)
	}
	qs.heads = heads
}

func refCount(value int64) int64 {
	if value < 0 {
		return value - PersistedHeadValue
	}
	return value
}
