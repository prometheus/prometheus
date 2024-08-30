package head

import (
	"context"
	"errors"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
	"sync"
	"sync/atomic"
)

type Head struct {
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
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
	registerer prometheus.Registerer) (*Head, error) {
	factory := util.NewUnconflictRegisterer(registerer)
	h := &Head{
		stopc: make(chan struct{}),
		wg:    &sync.WaitGroup{},
		inputRelabelers: make(
			map[relabelerKey]*cppbridge.InputPerShardRelabeler,
			int(numberOfShards)*len(inputRelabelerConfigs),
		),
		// stat
		memoryInUse: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_relabeler_cgo_memory_bytes",
				Help: "Current value memory in use in bytes.",
			},
			[]string{"allocator", "id"},
		),
	}

	if err := h.reconfigure(inputRelabelerConfigs, numberOfShards); err != nil {
		return nil, err
	}

	h.run()

	return h, nil
}

type destructibleIncomingData struct {
	data          *relabeler.IncomingData
	destructCount atomic.Int64
}

func newIncomingDataDestructor(data *relabeler.IncomingData, destructCount int) *destructibleIncomingData {
	d := &destructibleIncomingData{
		data: data,
	}
	d.destructCount.Store(int64(destructCount))
	return d
}

func (d *destructibleIncomingData) Data() *relabeler.IncomingData {
	return d.data
}

func (d *destructibleIncomingData) Destroy() {
	if d.destructCount.Add(-1) != 0 {
		return
	}
	d.data.Destroy()
}

func (h *Head) Append(ctx context.Context, incomingData *relabeler.IncomingData, metricLimits *cppbridge.MetricLimits, sourceStates *relabeler.SourceStates, staleNansTS int64, relabelerID string) ([][]*cppbridge.InnerSeries, error) {
	if sourceStates != nil {
		sourceStates.ResizeIfNeed(int(h.numberOfShards))
	}
	inputPromise := NewInputRelabelingPromise(h.numberOfShards)
	h.enqueueInputRelabeling(NewTaskInputRelabeling(
		ctx,
		inputPromise,
		newIncomingDataDestructor(incomingData, int(h.numberOfShards)),
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

	_ = h.forEachShard(func(shard relabeler.ShardInterface) error {
		shard.DataStorage().AppendInnerSeriesSlice(inputPromise.ShardsInnerSeries(shard.ShardID()))
		return nil
	})

	return inputPromise.data, nil
}

func (h *Head) ForEachShard(fn relabeler.ShardFnInterface) error {
	return h.forEachShard(fn)
}

func (h *Head) OnShard(shardID uint16, fn relabeler.ShardFnInterface) error {
	return h.onShard(shardID, fn)
}

func (h *Head) NumberOfShards() uint16 {
	return h.numberOfShards
}

func (h *Head) Finalize() {
	_ = h.forEachShard(func(shard relabeler.ShardInterface) error {
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
func (h *Head) forEachShard(fn relabeler.ShardFnInterface) error {
	task := NewGenericTask(fn, make([]error, h.numberOfShards))
	task.wg.Add(int(h.numberOfShards))
	for _, shardGenericTaskCh := range h.genericTaskCh {
		shardGenericTaskCh <- task
	}
	task.wg.Wait()
	return errors.Join(task.errs...)
}

func (h *Head) onShard(shardID uint16, fn relabeler.ShardFnInterface) error {
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
