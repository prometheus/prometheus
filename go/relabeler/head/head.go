package head

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
	"runtime"
	"sync"
)

type Head struct {
	referenceCounter           *relabeler.ReferenceCounter
	generation                 uint64
	inputRelabelers            map[relabelerKey]*cppbridge.InputPerShardRelabeler
	dataStorages               []*DataStorage
	lsses                      []*cppbridge.LabelSetStorage
	stageInputRelabeling       []chan *TaskInputRelabeling
	stageAppendRelabelerSeries []chan *TaskAppendRelabelerSeries
	stageUpdateRelabelerState  []chan *TaskUpdateRelabelerState
	genericTaskCh              []chan *GenericTask
	numberOfShards             uint16
	// stat
	memoryInUse *prometheus.GaugeVec
	stopc       chan struct{}
	wg          *sync.WaitGroup
}

func New(
	generation uint64,
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
	registerer prometheus.Registerer) (*Head, error) {
	factory := util.NewUnconflictRegisterer(registerer)
	h := &Head{
		referenceCounter: &relabeler.ReferenceCounter{},
		generation:       generation,
		stopc:            make(chan struct{}),
		wg:               &sync.WaitGroup{},
		inputRelabelers: make(
			map[relabelerKey]*cppbridge.InputPerShardRelabeler,
			int(numberOfShards)*len(inputRelabelerConfigs),
		),
		// stat
		memoryInUse: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_cgo_memory_bytes",
				Help: "Current value memory in use in bytes.",
			},
			[]string{"generation", "allocator", "id"},
		),
	}

	if err := h.reconfigure(inputRelabelerConfigs, numberOfShards); err != nil {
		return nil, err
	}

	h.run()

	runtime.SetFinalizer(h, func(h *Head) {
		fmt.Println("HEAD {", generation, "} DESTROYED")
	})

	return h, nil
}

func (h *Head) ReferenceCounter() *relabeler.ReferenceCounter {
	return h.referenceCounter
}

func (h *Head) Append(ctx context.Context, incomingData *relabeler.IncomingData, metricLimits *cppbridge.MetricLimits, sourceStates *relabeler.SourceStates, staleNansTS int64, relabelerID string) ([][]*cppbridge.InnerSeries, error) {
	if sourceStates != nil {
		sourceStates.ResizeIfNeed(int(h.numberOfShards))
	}
	inputPromise := NewInputRelabelingPromise(h.numberOfShards)
	h.enqueueInputRelabeling(NewTaskInputRelabeling(
		ctx,
		inputPromise,
		relabeler.NewDestructibleIncomingData(incomingData, int(h.numberOfShards)),
		metricLimits,
		sourceStates,
		staleNansTS,
		relabelerID,
	))
	if err := inputPromise.Wait(ctx); err != nil {
		//Errorf("failed input promise: %s", err)
		// reset msr.rotateWG on error
		return nil, err
	}

	_ = h.forEachShard(func(shard relabeler.Shard) error {
		shard.DataStorage().AppendInnerSeriesSlice(inputPromise.ShardsInnerSeries(shard.ShardID()))
		return nil
	})

	return inputPromise.data, nil
}

func (h *Head) ForEachShard(fn relabeler.ShardFn) error {
	return h.forEachShard(fn)
}

func (h *Head) OnShard(shardID uint16, fn relabeler.ShardFn) error {
	return h.onShard(shardID, fn)
}

func (h *Head) NumberOfShards() uint16 {
	return h.numberOfShards
}

func (h *Head) Finalize() {
	_ = h.forEachShard(func(shard relabeler.Shard) error {
		shard.DataStorage().MergeOutOfOrderChunks()
		return nil
	})
}

func (h *Head) Reconfigure(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error {
	h.stop()
	if err := h.reconfigure(inputRelabelerConfigs, numberOfShards); err != nil {
		return err
	}
	h.run()
	return nil
}

func (h *Head) WriteMetrics() {
	_ = h.forEachShard(func(shard relabeler.Shard) error {
		h.memoryInUse.With(
			prometheus.Labels{
				"generation": fmt.Sprintf("%d", h.generation),
				"allocator":  "main_lss",
				"id":         fmt.Sprintf("%d", shard.ShardID()),
			},
		).Set(float64(shard.LSS().AllocatedMemory()))

		for relabelerID, inputRelabeler := range shard.InputRelabelers() {
			h.memoryInUse.With(
				prometheus.Labels{
					"generation": fmt.Sprintf("%d", h.generation),
					"allocator":  fmt.Sprintf("input_relabeler_%s", relabelerID),
					"id":         fmt.Sprintf("%d", shard.ShardID()),
				},
			).Set(float64(inputRelabeler.CacheAllocatedMemory()))
		}

		return nil
	})
}

func (h *Head) Rotate() error {
	return nil
}

func (h *Head) Close() error {
	h.stop()
	return nil
}

// enqueueInputRelabeling send task to shard for input relabeling.
func (h *Head) enqueueInputRelabeling(task *TaskInputRelabeling) {
	for _, s := range h.stageInputRelabeling {
		s <- task
	}
}

// enqueueHeadAppending append all series after input relabeling stage to head.
func (h *Head) forEachShard(fn relabeler.ShardFn) error {
	task := NewGenericTask(fn, make([]error, h.numberOfShards))
	task.wg.Add(int(h.numberOfShards))
	for _, shardGenericTaskCh := range h.genericTaskCh {
		shardGenericTaskCh <- task
	}
	task.wg.Wait()
	return errors.Join(task.errs...)
}

func (h *Head) onShard(shardID uint16, fn relabeler.ShardFn) error {
	task := NewGenericTask(fn, make([]error, shardID+1))
	task.wg.Add(1)
	h.genericTaskCh[shardID] <- task
	task.wg.Wait()
	return errors.Join(task.errs...)
}

func (h *Head) run() {
	var shardID uint16
	for ; shardID < h.numberOfShards; shardID++ {
		h.wg.Add(1)
		go func(shardID uint16) {
			defer h.wg.Done()
			h.shardLoop(shardID, h.stopc)
		}(shardID)
	}
}

func (h *Head) stop() {
	close(h.stopc)
	h.wg.Wait()
	h.stopc = make(chan struct{})
}
