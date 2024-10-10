package head

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

type Head struct {
	finalizer        *Finalizer
	referenceCounter relabeler.ReferenceCounter
	generation       uint64

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
	series      prometheus.Gauge
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
		finalizer:        NewFinalizer(),
		referenceCounter: relabeler.NewLoggedAtomicReferenceCounter(fmt.Sprintf("HEAD {%d}", generation)),
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
	_ = h.finalizer.Finalize(func() error {
		_ = h.forEachShard(func(shard relabeler.Shard) error {
			shard.DataStorage().MergeOutOfOrderChunks()
			return nil
		})

		// todo: wait all tasks on stop
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

	if len(result) < limit {
		limit = len(result)
	}

	return result[:limit]
}

func (h *Head) Rotate() error {
	return nil
}

func (h *Head) Close() error {
	if !h.finalizer.FinalizeCalled() {
		return errors.New("closing not finalized head")
	}
	<-h.finalizer.Done()
	var shardID uint16
	for shardID < h.numberOfShards {
		h.memoryInUse.Delete(
			prometheus.Labels{
				"generation": fmt.Sprintf("%d", h.generation),
				"allocator":  "main_lss",
				"id":         fmt.Sprintf("%d", shardID),
			},
		)
		h.memoryInUse.Delete(
			prometheus.Labels{
				"generation": fmt.Sprintf("%d", h.generation),
				"allocator":  "data_storage",
				"id":         fmt.Sprintf("%d", shardID),
			},
		)

		shardID++
	}
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
	//logWithStack("foreachshard")
	if h.finalizer.Finalized() {
		errs := make([]error, h.numberOfShards)
		wg := &sync.WaitGroup{}
		var shardID uint16
		for shardID < h.numberOfShards {
			wg.Add(1)
			s := &shard{
				id:          shardID,
				dataStorage: h.dataStorages[shardID],
				lssWrapper:  &lssWrapper{lss: h.lsses[shardID]},
			}
			go func(shardID uint16, shard *shard) {
				defer wg.Done()
				errs[shardID] = fn(shard)
			}(shardID, s)
			shardID++
		}
		wg.Wait()
		return errors.Join(errs...)
	}

	task := NewGenericTask(fn, make([]error, h.numberOfShards))
	task.wg.Add(int(h.numberOfShards))
	for _, shardGenericTaskCh := range h.genericTaskCh {
		shardGenericTaskCh <- task
	}
	task.wg.Wait()
	return errors.Join(task.errs...)
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
