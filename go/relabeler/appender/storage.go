package appender

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/block"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/storage"
)

const (
	PersistedHeadValue = -(1 << 30)
)

type WriteNotifier interface {
	NotifyWritten()
}

// BlockWriter writes block on disk.
type BlockWriter interface {
	Write(block block.Block) error
}

// QueryableStorage hold reference to finalized heads and writes blocks from them. Also allows query not yet not
// persisted heads.
type QueryableStorage struct {
	blockWriter   BlockWriter
	writeNotifier WriteNotifier
	mtx           sync.Mutex
	heads         []relabeler.Head

	signal chan struct{}
	closer *util.Closer

	headPersistenceDuration *prometheus.GaugeVec
	querierMetrics          *querier.Metrics
}

// NewQueryableStorage - QueryableStorage constructor.
func NewQueryableStorage(blockWriter BlockWriter, registerer prometheus.Registerer, querierMetrics *querier.Metrics, heads ...relabeler.Head) *QueryableStorage {
	return NewQueryableStorageWithWriteNotifier(blockWriter, registerer, querierMetrics, noOpWriteNotifier{}, heads...)
}

// NewQueryableStorageWithWriteNotifier - QueryableStorage constructor.
func NewQueryableStorageWithWriteNotifier(blockWriter BlockWriter, registerer prometheus.Registerer, querierMetrics *querier.Metrics, writeNotifier WriteNotifier, heads ...relabeler.Head) *QueryableStorage {
	factory := util.NewUnconflictRegisterer(registerer)
	qs := &QueryableStorage{
		blockWriter:   blockWriter,
		writeNotifier: writeNotifier,
		heads:         heads,
		signal:        make(chan struct{}),
		closer:        util.NewCloser(),
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
	writeRequested := len(qs.heads) > 0
	closed := false

	for {
		if closed && writeFinished == nil {
			logger.Infof("QUERYABLE STORAGE: done")
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
	for i := range qs.heads {
		heads = append(heads, qs.heads[i])
	}
	qs.mtx.Unlock()

	shouldNotify := false
	var persisted []string
	for _, head := range heads {
		start := time.Now()
		err := head.ForEachShard(func(shard relabeler.Shard) error {
			return qs.blockWriter.Write(relabeler.NewBlock(shard.LSS().Raw(), shard.DataStorage().Raw()))
		})
		if err != nil {
			// todo: log
			logger.Errorf("QUERYABLE STORAGE: failed to write head: %s", err.Error())
			continue
		}
		qs.headPersistenceDuration.With(prometheus.Labels{
			"generation": fmt.Sprintf("%d", head.Generation()),
		}).Set(float64(time.Since(start).Milliseconds()))
		persisted = append(persisted, head.ID())
		shouldNotify = true
		logger.Infof("QUERYABLE STORAGE: head { %d } persisted, duration: %v", head.Generation(), time.Since(start))
	}

	if shouldNotify {
		qs.writeNotifier.NotifyWritten()
	}

	qs.shrink(persisted...)
}

// Add - Storage interface implementation.
func (qs *QueryableStorage) Add(head relabeler.Head) {
	qs.mtx.Lock()
	qs.heads = append(qs.heads, head)
	logger.Infof("QUERYABLE STORAGE: head { %d } added", head.Generation())
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
		heads = append(heads, head)
	}
	qs.mtx.Unlock()

	for _, head := range heads {
		head.WriteMetrics()
	}

	qs.shrink()
}

// Querier - storage.Queryable interface implementation.
func (qs *QueryableStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	qs.mtx.Lock()
	var heads []relabeler.Head
	for _, head := range qs.heads {
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
				nil,
				qs.querierMetrics,
			),
		)
	}

	q := querier.NewMultiQuerier(
		queriers,
		nil,
	)

	return q, nil
}

func (qs *QueryableStorage) shrink(persisted ...string) {
	qs.mtx.Lock()
	defer qs.mtx.Unlock()

	persistedMap := make(map[string]struct{})
	for _, headID := range persisted {
		persistedMap[headID] = struct{}{}
	}

	var heads []relabeler.Head
	for _, head := range qs.heads {
		if _, ok := persistedMap[head.ID()]; ok {
			_ = head.Close()
			_ = head.Discard()
			logger.Infof("QUERYABLE STORAGE: head { %d } persisted, closed and discarded", head.Generation())
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

type noOpWriteNotifier struct {
}

func (noOpWriteNotifier) NotifyWritten() {}
