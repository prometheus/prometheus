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
func NewQueryableStorage(
	blockWriter BlockWriter,
	registerer prometheus.Registerer,
	querierMetrics *querier.Metrics,
	heads ...relabeler.Head,
) *QueryableStorage {
	return NewQueryableStorageWithWriteNotifier(blockWriter, registerer, querierMetrics, noOpWriteNotifier{}, heads...)
}

// NewQueryableStorageWithWriteNotifier - QueryableStorage constructor.
func NewQueryableStorageWithWriteNotifier(
	blockWriter BlockWriter,
	registerer prometheus.Registerer,
	querierMetrics *querier.Metrics,
	writeNotifier WriteNotifier,
	heads ...relabeler.Head,
) *QueryableStorage {
	factory := util.NewUnconflictRegisterer(registerer)
	qs := &QueryableStorage{
		blockWriter:   blockWriter,
		writeNotifier: writeNotifier,
		heads:         heads,
		signal:        make(chan struct{}, 1),
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

	return qs
}

// Run loop for converting heads.
func (qs *QueryableStorage) Run() {
	go qs.loop()
}

func (qs *QueryableStorage) loop() {
	defer qs.closer.Done()

	timer := time.NewTimer(0)
	// skip 0 start
	<-timer.C

	for {
		if !qs.write() {
			if !timer.Stop() {
				// in the new version of go cleaning of C is not required
				select {
				case <-timer.C:
				default:
				}
			}
			// try write after 5 minute
			timer.Reset(5 * time.Minute)
		}

		select {
		case <-qs.signal:
		case <-timer.C:
		case <-qs.closer.Signal():
			logger.Infof("QUERYABLE STORAGE: done")
			return
		}
	}
}

func (qs *QueryableStorage) write() bool {
	qs.mtx.Lock()
	lenHeads := len(qs.heads)
	if lenHeads == 0 {
		// quick exit
		qs.mtx.Unlock()
		return true
	}
	heads := make([]relabeler.Head, lenHeads)
	copy(heads, qs.heads)
	qs.mtx.Unlock()

	successful := true
	shouldNotify := false
	persisted := make([]string, 0, lenHeads)
	for _, head := range heads {
		start := time.Now()
		err := head.ForEachShard(func(shard relabeler.Shard) error {
			return qs.blockWriter.Write(relabeler.NewBlock(shard.LSS().Raw(), shard.DataStorage().Raw()))
		})
		if err != nil {
			logger.Errorf("QUERYABLE STORAGE: failed to write head %s: %s", head.String(), err.Error())
			successful = false
			continue
		}
		qs.headPersistenceDuration.With(prometheus.Labels{
			"generation": fmt.Sprintf("%d", head.Generation()),
		}).Set(float64(time.Since(start).Milliseconds()))
		persisted = append(persisted, head.ID())
		shouldNotify = true
		logger.Infof("QUERYABLE STORAGE: head %s persisted, duration: %v", head.String(), time.Since(start))
	}

	if shouldNotify {
		qs.writeNotifier.NotifyWritten()
	}

	qs.shrink(persisted...)
	return successful
}

// Add - Storage interface implementation.
func (qs *QueryableStorage) Add(head relabeler.Head) {
	qs.mtx.Lock()
	qs.heads = append(qs.heads, head)
	logger.Infof("QUERYABLE STORAGE: head %s added", head.String())
	qs.mtx.Unlock()

	select {
	case qs.signal <- struct{}{}:
	case <-qs.closer.Signal():
	default:
	}
}

func (qs *QueryableStorage) Close() error {
	return qs.closer.Close()
}

// WriteMetrics - MetricWriterTarget interface implementation.
func (qs *QueryableStorage) WriteMetrics() {
	qs.mtx.Lock()
	heads := make([]relabeler.Head, len(qs.heads))
	copy(heads, qs.heads)
	qs.mtx.Unlock()

	for _, head := range heads {
		head.WriteMetrics()
	}

	qs.shrink()
}

// Querier - storage.Queryable interface implementation.
func (qs *QueryableStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	qs.mtx.Lock()
	heads := make([]relabeler.Head, len(qs.heads))
	copy(heads, qs.heads)
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
			logger.Infof("QUERYABLE STORAGE: head %s persisted, closed and discarded", head.String())
			continue
		}
		heads = append(heads, head)
	}
	qs.heads = heads
}

type noOpWriteNotifier struct {
}

func (noOpWriteNotifier) NotifyWritten() {}
