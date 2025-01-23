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

	"github.com/prometheus/prometheus/pp/go/relabeler/logger"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

// RelabelerData data for relabeling - inputRelabelers per shard and state.
type RelabelerData struct {
	state           *cppbridge.State
	inputRelabelers []*cppbridge.InputPerShardRelabeler
}

// NewRelabelerData init new RelabelerData.
func NewRelabelerData(
	rCfgs []*cppbridge.RelabelConfig,
	generationHead uint64,
	numberOfShards uint16,
) (*RelabelerData, error) {
	rd := &RelabelerData{
		inputRelabelers: make([]*cppbridge.InputPerShardRelabeler, 0, numberOfShards),
	}

	if err := rd.Reconfigure(rCfgs, generationHead, numberOfShards); err != nil {
		return nil, err
	}

	return rd, nil
}

// State return State of relabeler.
func (rd *RelabelerData) State(generationHead uint64) *cppbridge.State {
	if rd.state == nil {
		rd.state = cppbridge.NewState(uint16(len(rd.inputRelabelers)))
		rd.state.Reconfigure(
			rd.generationRelabeler(),
			generationHead,
			uint16(len(rd.inputRelabelers)),
		)
	}

	return rd.state
}

// InputRelabelerByShard return InputRelabeler by shard.
func (rd *RelabelerData) InputRelabelerByShard(shardID uint16) *cppbridge.InputPerShardRelabeler {
	return rd.inputRelabelers[shardID]
}

// Reconfigure update configuration on InputRelabeler and State.
func (rd *RelabelerData) Reconfigure(
	rCfgs []*cppbridge.RelabelConfig,
	generationHead uint64,
	numberOfShards uint16,
) error {
	if err := rd.reconfigureInputRelabelers(rCfgs, numberOfShards); err != nil {
		return err
	}

	if rd.state != nil {
		rd.state.Reconfigure(rd.generationRelabeler(), generationHead, numberOfShards)
	}

	return nil
}

// generationRelabeler return current(shardID == 0) relabeler's generation.
func (rd *RelabelerData) generationRelabeler() uint64 {
	return rd.inputRelabelers[0].Generation()
}

// Reconfigure update configuration on InputRelabeler.
func (rd *RelabelerData) reconfigureInputRelabelers(
	rCfgs []*cppbridge.RelabelConfig,
	numberOfShards uint16,
) error {
	sr, err := rd.updateOrCreateStatelessRelabeler(rCfgs)
	if err != nil {
		return err
	}

	if len(rd.inputRelabelers) == int(numberOfShards) {
		return nil
	}

	if len(rd.inputRelabelers) > int(numberOfShards) {
		// cut
		rd.inputRelabelers = rd.inputRelabelers[:numberOfShards]
	}

	if len(rd.inputRelabelers) < int(numberOfShards) {
		// grow
		rd.inputRelabelers = append(
			rd.inputRelabelers,
			make([]*cppbridge.InputPerShardRelabeler, int(numberOfShards)-len(rd.inputRelabelers))...,
		)
	}

	for shardID := range rd.inputRelabelers {
		if rd.inputRelabelers[shardID] != nil {
			// TODO may be depreacate ResetTo and always recreate
			rd.inputRelabelers[shardID].ResetTo(numberOfShards)
			continue
		}

		if rd.inputRelabelers[shardID], err = cppbridge.NewInputPerShardRelabeler(
			sr,
			numberOfShards,
			uint16(shardID),
		); err != nil {
			return err
		}
	}

	return nil
}

// updateOrCreateStatelessRelabeler check inputRelabeler(shardID == 0) for key
// and update configs for StatelessRelabeler, if not exist - create new.
func (rd *RelabelerData) updateOrCreateStatelessRelabeler(
	rCfgs []*cppbridge.RelabelConfig,
) (*cppbridge.StatelessRelabeler, error) {
	if len(rd.inputRelabelers) == 0 || rd.inputRelabelers[0] == nil {
		return cppbridge.NewStatelessRelabeler(rCfgs)
	}

	sr := rd.inputRelabelers[0].StatelessRelabeler()
	if sr.EqualConfigs(rCfgs) {
		return sr, nil
	}

	if err := sr.ResetTo(rCfgs); err != nil {
		return nil, err
	}

	return sr, nil
}

type LastAppendedSegmentIDSetter interface {
	SetLastAppendedSegmentID(segmentID uint32)
}

type NoOpLastAppendedSegmentIDSetter struct{}

func (NoOpLastAppendedSegmentIDSetter) SetLastAppendedSegmentID(segmentID uint32) {}

type Head struct {
	id         string
	finalizer  *Finalizer
	generation uint64

	lastAppendedSegmentID       *uint32
	lastAppendedSegmentIDSetter LastAppendedSegmentIDSetter

	relabelersData             map[string]*RelabelerData
	dataStorages               []*DataStorage
	wals                       []*ShardWal
	lsses                      []*LSS
	stageInputRelabeling       []chan *TaskInputRelabeling
	stageAppendRelabelerSeries []chan *TaskAppendRelabelerSeries
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
	lsses []*LSS,
	wals []*ShardWal,
	dataStorages []*DataStorage,
	numberOfShards uint16,
	lastAppendedSegmentID *uint32,
	lastAppendedSegmentIDSetter LastAppendedSegmentIDSetter,
	registerer prometheus.Registerer) (*Head, error) {

	stageInputRelabeling := make([]chan *TaskInputRelabeling, numberOfShards)
	stageAppendRelabelerSeries := make([]chan *TaskAppendRelabelerSeries, numberOfShards)
	genericTaskCh := make([]chan *GenericTask, numberOfShards)

	var shardID uint16
	for ; shardID < numberOfShards; shardID++ {
		stageInputRelabeling[shardID] = make(chan *TaskInputRelabeling, chanBufferSize)
		stageAppendRelabelerSeries[shardID] = make(chan *TaskAppendRelabelerSeries, chanBufferSize)
		genericTaskCh[shardID] = make(chan *GenericTask, chanBufferSize)
	}

	factory := util.NewUnconflictRegisterer(registerer)
	h := &Head{
		id:                          id,
		finalizer:                   NewFinalizer(),
		generation:                  generation,
		lsses:                       lsses,
		wals:                        wals,
		dataStorages:                dataStorages,
		lastAppendedSegmentID:       lastAppendedSegmentID,
		lastAppendedSegmentIDSetter: lastAppendedSegmentIDSetter,
		stageInputRelabeling:        stageInputRelabeling,
		stageAppendRelabelerSeries:  stageAppendRelabelerSeries,
		genericTaskCh:               genericTaskCh,
		stopc:                       make(chan struct{}),
		wg:                          &sync.WaitGroup{},
		relabelersData:              make(map[string]*RelabelerData, len(inputRelabelerConfigs)),
		numberOfShards:              numberOfShards,
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
		logger.Debugf("head {%d} destroyed", generation)
	})

	return h, nil
}

func (h *Head) ID() string {
	return h.id
}

func (h *Head) Generation() uint64 {
	return h.generation
}

// Append incoming data to head.
func (h *Head) Append(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	state *cppbridge.State,
	relabelerID string,
) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error) {
	if h.finalizer.FinalizeCalled() {
		return nil, cppbridge.RelabelerStats{}, errors.New("appending to a finalized head")
	}

	rd, ok := h.relabelersData[relabelerID]
	if !ok {
		return nil, cppbridge.RelabelerStats{}, fmt.Errorf("relabeler ID not exist: %s", relabelerID)
	}

	if state != nil {
		state.Reconfigure(rd.generationRelabeler(), h.generation, h.numberOfShards)
	}

	if state == nil {
		state = rd.State(h.generation)
	}

	inputPromise := NewInputRelabelingPromise(h.numberOfShards)
	h.enqueueInputRelabeling(NewTaskInputRelabeling(
		ctx,
		inputPromise,
		relabeler.NewDestructibleIncomingData(incomingData, int(h.numberOfShards)),
		rd,
		state,
	))
	if err := inputPromise.Wait(ctx); err != nil {
		// reset msr.rotateWG on error
		return nil, cppbridge.RelabelerStats{}, fmt.Errorf("failed input promise: %s", err)
	}

	inputPromise.UpdateRelabeler()

	err := h.forEachShard(func(shard relabeler.Shard) error {
		return shard.Wal().Write(inputPromise.ShardsInnerSeries(shard.ShardID()))
	})

	if err != nil {
		return nil, cppbridge.RelabelerStats{}, fmt.Errorf("failed to write wal: %w", err)
	}

	_ = h.forEachShard(func(shard relabeler.Shard) error {
		shard.DataStorage().AppendInnerSeriesSlice(inputPromise.ShardsInnerSeries(shard.ShardID()))
		return nil
	})

	if h.lastAppendedSegmentID == nil {
		var segmentID uint32 = 0
		h.lastAppendedSegmentID = &segmentID
	} else {
		*h.lastAppendedSegmentID++
	}
	h.lastAppendedSegmentIDSetter.SetLastAppendedSegmentID(*h.lastAppendedSegmentID)

	return inputPromise.data, inputPromise.Stats(), nil
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
		for relabelerID := range h.relabelersData {
			// clear unnecessary
			h.memoryInUse.DeletePartialMatch(prometheus.Labels{
				"generation": fmt.Sprintf("%d", h.generation),
				"allocator":  fmt.Sprintf("input_relabeler_%s", relabelerID),
			})
		}
		h.relabelersData = nil
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

		return nil
	})
}

func (h *Head) Status(limit int) relabeler.HeadStatus {
	shardStatuses := make([]*cppbridge.HeadStatus, h.NumberOfShards())
	_ = h.ForEachShard(func(shard relabeler.Shard) error {
		shardStatuses[shard.ShardID()] = cppbridge.GetHeadStatus(
			shard.LSS().Raw().Pointer(),
			shard.DataStorage().Raw().Pointer(),
			limit,
		)
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
				lss:         h.lsses[shardID],
				dataStorage: h.dataStorages[shardID],
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
			lss:         h.lsses[shardID],
			dataStorage: h.dataStorages[shardID],
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
