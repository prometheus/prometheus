package head

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strconv"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

type Head struct {
	id               string
	finalizer        *Finalizer
	referenceCounter relabeler.ReferenceCounter
	generation       uint64

	inputRelabelers            map[relabelerKey]*cppbridge.InputPerShardRelabeler
	dataStorages               []*DataStorage
	wals                       []*ShardWal
	lsses                      []*cppbridge.LabelSetStorage
	stageInputRelabeling       []chan *TaskInputRelabeling
	stageAppendRelabelerSeries []chan *TaskAppendRelabelerSeries
	stageUpdateRelabelerState  []chan *TaskUpdateRelabelerState
	genericTaskCh              []chan *GenericTask
	numberOfShards             uint16
	// stat
	memoryInUse *prometheus.GaugeVec
	series      prometheus.Gauge
	stopc       chan struct{}
	wg          *sync.WaitGroup
}

func New(
	id string,
	generation uint64,
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	lsses []*cppbridge.LabelSetStorage,
	wals []*ShardWal,
	dataStorages []*DataStorage,
	numberOfShards uint16,
	registerer prometheus.Registerer) (*Head, error) {

	stageInputRelabeling := make([]chan *TaskInputRelabeling, numberOfShards)
	stageAppendRelabelerSeries := make([]chan *TaskAppendRelabelerSeries, numberOfShards)
	stageUpdateRelabelerState := make([]chan *TaskUpdateRelabelerState, numberOfShards)
	genericTaskCh := make([]chan *GenericTask, numberOfShards)

	var shardID uint16
	for ; shardID < numberOfShards; shardID++ {
		stageInputRelabeling[shardID] = make(chan *TaskInputRelabeling, chanBufferSize)
		stageAppendRelabelerSeries[shardID] = make(chan *TaskAppendRelabelerSeries, chanBufferSize)
		stageUpdateRelabelerState[shardID] = make(chan *TaskUpdateRelabelerState, chanBufferSize)
		genericTaskCh[shardID] = make(chan *GenericTask, chanBufferSize)
	}

	factory := util.NewUnconflictRegisterer(registerer)
	h := &Head{
		id:                         id,
		finalizer:                  NewFinalizer(),
		referenceCounter:           relabeler.NewLoggedAtomicReferenceCounter(fmt.Sprintf("HEAD {%d}", generation)),
		generation:                 generation,
		lsses:                      lsses,
		wals:                       wals,
		dataStorages:               dataStorages,
		stageInputRelabeling:       stageInputRelabeling,
		stageAppendRelabelerSeries: stageAppendRelabelerSeries,
		stageUpdateRelabelerState:  stageUpdateRelabelerState,
		genericTaskCh:              genericTaskCh,
		stopc:                      make(chan struct{}),
		wg:                         &sync.WaitGroup{},
		inputRelabelers: make(
			map[relabelerKey]*cppbridge.InputPerShardRelabeler,
			int(numberOfShards)*len(inputRelabelerConfigs),
		),
		numberOfShards: numberOfShards,
		// stat
		memoryInUse: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_cgo_memory_bytes",
				Help: "Current value memory in use in bytes.",
			},
			[]string{"generation", "allocator", "id"},
		),
		series: factory.NewGauge(prometheus.GaugeOpts{
			Name: "prompp_head_series",
			Help: "Total number of series in the heads block.",
		}),
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

func (h *Head) ID() string {
	return h.id
}

func (h *Head) Generation() uint64 {
	return h.generation
}

func (h *Head) ReferenceCounter() relabeler.ReferenceCounter {
	return h.referenceCounter
}

func (h *Head) Append(ctx context.Context, incomingData *relabeler.IncomingData, metricLimits *cppbridge.MetricLimits, sourceStates *relabeler.SourceStates, staleNansTS int64, relabelerID string) ([][]*cppbridge.InnerSeries, error) {
	if h.finalizer.FinalizeCalled() {
		return nil, errors.New("appending to a finalized head")
	}

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

	err := h.forEachShard(func(shard relabeler.Shard) error {
		return shard.Wal().Write(inputPromise.ShardsInnerSeries(shard.ShardID()))
	})

	if err != nil {
		return nil, fmt.Errorf("failed to write wal: %w", err)
	}

	_ = h.forEachShard(func(shard relabeler.Shard) error {
		shard.DataStorage().AppendInnerSeriesSlice(inputPromise.ShardsInnerSeries(shard.ShardID()))
		return nil
	})

	return inputPromise.data, nil
}

func (h *Head) ForEachShard(fn relabeler.ShardFn) error {
	if h.finalizer.FinalizeCalled() {
		<-h.finalizer.Done()
	}

	return h.forEachShard(fn)
}

func (h *Head) OnShard(shardID uint16, fn relabeler.ShardFn) error {
	if h.finalizer.FinalizeCalled() {
		<-h.finalizer.Done()
	}

	return h.onShard(shardID, fn)
}

func (h *Head) NumberOfShards() uint16 {
	return h.numberOfShards
}

func (h *Head) Finalize() {
	// todo: wait all tasks on stop
	_ = h.finalizer.Finalize(func() error {
		_ = h.forEachShard(func(shard relabeler.Shard) error {
			shard.DataStorage().MergeOutOfOrderChunks()
			return nil
		})

		h.stop()
		for key := range h.inputRelabelers {
			h.memoryInUse.Delete(
				prometheus.Labels{
					"generation": fmt.Sprintf("%d", h.generation),
					"allocator":  fmt.Sprintf("input_relabeler_%s", key.relabelerID),
					"id":         fmt.Sprintf("%d", key.shardID),
				},
			)
		}
		h.inputRelabelers = nil
		return nil
	})
}

func (h *Head) Reconfigure(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error {
	if h.finalizer.FinalizeCalled() {
		return errors.New("reconfiguring finalized head")
	}

	h.stop()
	if err := h.reconfigure(inputRelabelerConfigs, numberOfShards); err != nil {
		return err
	}
	h.run()
	return nil
}

func (h *Head) WriteMetrics() {
	h.series.Set(float64(h.Status(1).HeadStats.NumSeries))

	_ = h.ForEachShard(func(shard relabeler.Shard) error {
		h.memoryInUse.With(
			prometheus.Labels{
				"generation": fmt.Sprintf("%d", h.generation),
				"allocator":  "main_lss",
				"id":         fmt.Sprintf("%d", shard.ShardID()),
			},
		).Set(float64(shard.LSS().AllocatedMemory()))

		h.memoryInUse.With(
			prometheus.Labels{
				"generation": fmt.Sprintf("%d", h.generation),
				"allocator":  "data_storage",
				"id":         fmt.Sprintf("%d", shard.ShardID()),
			},
		).Set(float64(shard.DataStorage().AllocatedMemory()))

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

func (h *Head) Status(limit int) relabeler.HeadStatus {
	shardStatuses := make([]*cppbridge.HeadStatus, h.NumberOfShards())
	_ = h.ForEachShard(func(shard relabeler.Shard) error {
		shardStatuses[shard.ShardID()] = cppbridge.GetHeadStatus(shard.LSS().Raw().Pointer(), shard.DataStorage().Raw().Pointer(), limit)
		return nil
	})

	headStatus := relabeler.HeadStatus{
		HeadStats: relabeler.HeadStats{
			MinTime: math.MaxInt64,
			MaxTime: math.MinInt64,
		},
	}

	seriesStats := make(map[string]uint64)
	labelsStats := make(map[string]uint64)
	memoryStats := make(map[string]uint64)
	countStats := make(map[string]uint64)

	for _, shardStatus := range shardStatuses {
		if headStatus.HeadStats.MaxTime < shardStatus.TimeInterval.Max {
			headStatus.HeadStats.MaxTime = shardStatus.TimeInterval.Max
		}
		if headStatus.HeadStats.MinTime > shardStatus.TimeInterval.Min {
			headStatus.HeadStats.MinTime = shardStatus.TimeInterval.Min
		}

		headStatus.HeadStats.NumSeries += uint64(shardStatus.NumSeries)
		headStatus.HeadStats.ChunkCount += int64(shardStatus.ChunkCount)
		headStatus.HeadStats.NumLabelPairs += int(shardStatus.NumLabelPairs)

		for _, stat := range shardStatus.SeriesCountByMetricName {
			seriesStats[stat.Name] += uint64(stat.Count)
		}
		for _, stat := range shardStatus.LabelValueCountByLabelName {
			labelsStats[stat.Name] += uint64(stat.Count)
		}
		for _, stat := range shardStatus.MemoryInBytesByLabelName {
			memoryStats[stat.Name] += uint64(stat.Size)
		}
		for _, stat := range shardStatus.SeriesCountByLabelValuePair {
			countStats[stat.Name+"="+stat.Value] += uint64(stat.Count)
		}
	}

	headStatus.SeriesCountByMetricName = getSortedStats(seriesStats, limit)
	headStatus.LabelValueCountByLabelName = getSortedStats(labelsStats, limit)
	headStatus.MemoryInBytesByLabelName = getSortedStats(memoryStats, limit)
	headStatus.SeriesCountByLabelValuePair = getSortedStats(countStats, limit)

	return headStatus
}

func getSortedStats(stats map[string]uint64, limit int) []relabeler.HeadStat {
	result := make([]relabeler.HeadStat, 0, len(stats))
	for k, v := range stats {
		result = append(result, relabeler.HeadStat{
			Name:  k,
			Value: v,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Value > result[j].Value
	})

	if len(result) > limit {
		return result[:limit]
	}

	return result
}

func (h *Head) Rotate() error {
	return nil
}

func (h *Head) Close() error {
	if !h.finalizer.FinalizeCalled() {
		return errors.New("closing not finalized head")
	}
	<-h.finalizer.Done()
	h.memoryInUse.DeletePartialMatch(prometheus.Labels{"generation": strconv.FormatUint(h.generation, 10)})
	var err error
	for _, wal := range h.wals {
		err = errors.Join(err, wal.Close())
	}
	return err
}

// enqueueInputRelabeling send task to shard for input relabeling.
func (h *Head) enqueueInputRelabeling(task *TaskInputRelabeling) {
	for _, s := range h.stageInputRelabeling {
		s <- task
	}
}

func (h *Head) Discard() error {
	return nil
}

// enqueueHeadAppending append all series after input relabeling stage to head.
func (h *Head) forEachShard(fn relabeler.ShardFn) error {
	task := NewGenericTask(fn, h.numberOfShards)
	if h.finalizer.Finalized() {
		for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
			s := &shard{
				id:          shardID,
				dataStorage: h.dataStorages[shardID],
				lssWrapper:  &lssWrapper{lss: h.lsses[shardID]},
			}
			go func(shard *shard) {
				task.ExecuteOnShard(shard)
			}(s)
		}
		task.Wait()
		return errors.Join(task.Errors()...)
	}

	for _, shardGenericTaskCh := range h.genericTaskCh {
		shardGenericTaskCh <- task
	}
	task.Wait()
	return errors.Join(task.Errors()...)
}

func (h *Head) onShard(shardID uint16, fn relabeler.ShardFn) error {
	if h.finalizer.FinalizeCalled() {
		<-h.finalizer.Done()

		s := &shard{
			id:          shardID,
			dataStorage: h.dataStorages[shardID],
			lssWrapper:  &lssWrapper{lss: h.lsses[shardID]},
		}

		return fn(s)
	}

	task := NewSingleGenericTask(fn, h.numberOfShards)
	h.genericTaskCh[shardID] <- task
	task.Wait()
	return errors.Join(task.Errors()...)
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
