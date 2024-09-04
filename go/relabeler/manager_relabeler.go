package relabeler

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"math"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	chanBufferSize        = 64
	defaultMetricDuration = 15 * time.Second
	defaultRotateDuration = 2 * time.Hour
)

// ProtobufData is an universal interface for blob protobuf data.
type ProtobufData interface {
	Bytes() []byte
	Destroy()
}

// TimeSeriesData is an universal interface for slice model.TimeSeries data.
type TimeSeriesData interface {
	TimeSeries() []model.TimeSeries
	Destroy()
}

// MetricData is an universal interface for blob protobuf or slice model.TimeSeries data.
type MetricData interface {
	Destroy()
}

// IncomingData implements.
type IncomingData struct {
	Hashdex cppbridge.ShardedData
	Data    MetricData
}

// ShardedData return hashdex.
func (i *IncomingData) ShardedData() cppbridge.ShardedData {
	return i.Hashdex
}

// Destroy increment or destroy IncomingData.
func (i *IncomingData) Destroy() {
	i.Hashdex = nil
	if i.Data != nil {
		i.Data.Destroy()
	}
}

// relabelerKey - key for relabeler.
type relabelerKey struct {
	relabelerID string
	shardID     uint16
}

// ManagerRelabeler - manager for relabeling.
type ManagerRelabeler struct {
	appendLock *sync.Mutex
	rotateLock *sync.RWMutex
	rotateWG   *sync.WaitGroup

	shards            *shards
	destinationGroups *DestinationGroups
	rotateTimer       *RotateTimer
	metricTimer       clockwork.Timer
	clock             clockwork.Clock

	stop chan struct{}
	done chan struct{}
	// stat
	memoryInUse *prometheus.GaugeVec
}

// NewManagerRelabeler - init new *ManagerRelabeler.
//
//revive:disable-next-line:function-length long but readable.
//revive:disable-next-line:cyclomatic long but understandable.
func NewManagerRelabeler(
	clock clockwork.Clock,
	registerer prometheus.Registerer,
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	destinationGroups *DestinationGroups,
	numberOfShards uint16,
) (*ManagerRelabeler, error) {
	if numberOfShards == 0 {
		numberOfShards = 1
	}

	shards, err := newshards(inputRelabelerConfigs, numberOfShards, registerer)
	if err != nil {
		return nil, err
	}

	factory := util.NewUnconflictRegisterer(registerer)
	msr := &ManagerRelabeler{
		appendLock:        new(sync.Mutex),
		rotateLock:        new(sync.RWMutex),
		rotateWG:          new(sync.WaitGroup),
		shards:            shards,
		destinationGroups: destinationGroups,
		rotateTimer:       NewRotateTimer(clock, defaultRotateDuration),
		metricTimer:       clock.NewTimer(defaultMetricDuration),
		clock:             clock,

		stop: make(chan struct{}),
		done: make(chan struct{}),

		// stat
		memoryInUse: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_relabeler_cgo_memory_bytes",
				Help: "Current value memory in use in bytes.",
			},
			[]string{"allocator", "id"},
		),
	}

	return msr, nil
}

// Append append to relabeling heshdex data.
//
//revive:disable-next-line:function-length long but readable.
func (msr *ManagerRelabeler) Append(
	ctx context.Context,
	incomingData *IncomingData,
	metricLimits *cppbridge.MetricLimits,
	relabelerID string,
) error {
	return msr.AppendWithStaleNans(
		ctx,
		incomingData,
		metricLimits,
		nil,
		0,
		relabelerID,
	)
}

// AppendWithStaleNans append to relabeling hashdex data with check state stalenans.
func (msr *ManagerRelabeler) AppendWithStaleNans(
	ctx context.Context,
	incomingData *IncomingData,
	metricLimits *cppbridge.MetricLimits,
	sourceStates *SourceStates,
	staleNansTS int64,
	relabelerID string,
) error {
	msr.appendLock.Lock()
	defer msr.appendLock.Unlock()

	select {
	case <-msr.stop:
		return ErrShutdown
	default:
	}

	// blocking from the race for rotation
	msr.rotateLock.Lock()
	msr.rotateWG.Add(len(*msr.destinationGroups))
	msr.rotateLock.Unlock()

	if sourceStates != nil {
		sourceStates.ResizeIfNeed(int(msr.shards.numberOfShards))
	}
	inputPromise := NewInputRelabelingPromise(msr.shards.numberOfShards)
	// incomingData.numberOfDestroy = int32(msr.shards.numberOfShards) * int32(len(msr.shards.inputRelabelers))
	//incomingData.numberOfDestroy = int32(msr.shards.numberOfShards)
	msr.shards.enqueueInputRelabeling(NewTaskInputRelabeling(
		ctx,
		inputPromise,
		incomingData,
		metricLimits,
		sourceStates,
		staleNansTS,
		relabelerID,
	))
	if err := inputPromise.Wait(ctx); err != nil {
		Errorf("failed input promise: %s", err)
		// reset msr.rotateWG on error
		for i := 0; i < len(*msr.destinationGroups); i++ {
			msr.rotateWG.Done()
		}
		return err
	}

	msr.shards.forEachShard(func(shard ShardInterface) {
		shard.Head().AppendInnerSeriesSlice(inputPromise.ShardsInnerSeries(shard.ShardID()))
	})

	// blocking groups from internal rotations
	_ = msr.destinationGroups.RangeGo(
		func(_ int, dg *DestinationGroup) error {
			dg.RotateLock()
			return nil
		},
	)

	outputPromise := NewOutputRelabelingPromise(msr.destinationGroups, msr.shards.numberOfShards)
	msr.shards.enqueueOutputRelabeling(ctx, outputPromise, inputPromise)
	if err := outputPromise.Wait(ctx); err != nil {
		Errorf("failed output promise: %s", err)
		// dg.RotateUnlock() on error
		_ = msr.destinationGroups.RangeGo(
			func(_ int, dg *DestinationGroup) error {
				dg.RotateUnlock()
				return nil
			},
		)

		// reset msr.rotateWG on error
		for i := 0; i < len(*msr.destinationGroups); i++ {
			msr.rotateWG.Done()
		}
		return err
	}

	wg := new(sync.WaitGroup)
	err := msr.destinationGroups.RangeGo(
		func(dgid int, dg *DestinationGroup) error {
			dg.EncodersLock()
			defer dg.EncodersUnlock()
			msr.rotateWG.Done()

			outputStateUpdates := dg.OutputStateUpdates()
			if _, err := dg.AppendOpenHead(
				ctx,
				outputPromise.OutputInnerSeries(dgid),
				outputPromise.OutputRelabeledSeries(dgid),
				outputStateUpdates,
			); err != nil {
				return err
			}

			for shardID, outputStateUpdate := range outputStateUpdates {
				wg.Add(1)
				msr.shards.enqueueOutputUpdateRelabelerState(
					ctx,
					outputStateUpdate,
					(*msr.destinationGroups)[dgid],
					wg,
					shardID,
				)
			}
			return nil
		},
	)
	// wait until it state updates
	wg.Wait()

	// unblocking groups from internal rotations
	_ = msr.destinationGroups.RangeGo(
		func(_ int, dg *DestinationGroup) error {
			dg.RotateUnlock()
			return nil
		},
	)

	return err
}

// ApplyConfig update ManagerRelabeler for new config.
func (msr *ManagerRelabeler) ApplyConfig(
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	if numberOfShards == 0 {
		numberOfShards = 1
	}

	msr.shards.stopLoop()
	if err := msr.shards.reshards(inputRelabelerConfigs, numberOfShards); err != nil {
		return err
	}
	msr.shards.run()

	return nil
}

// DestinationGroups return current *DestinationGroups.
func (msr *ManagerRelabeler) DestinationGroups() *DestinationGroups {
	return msr.destinationGroups
}

// Lock manager for append and rotate for change.
func (msr *ManagerRelabeler) Lock() {
	msr.appendLock.Lock()
	msr.rotateLock.Lock()
}

// Unlock manager for append and rotate for change.
func (msr *ManagerRelabeler) Unlock() {
	msr.rotateLock.Unlock()
	msr.appendLock.Unlock()
}

// RelabelerIDIsExist check on exist relabelerID.
func (msr *ManagerRelabeler) RelabelerIDIsExist(relabelerID string) bool {
	return msr.shards.relabelerIDIsExist(relabelerID)
}

// Run main loop PerShardRelabeler's.
func (msr *ManagerRelabeler) Run(ctx context.Context) {
	msr.shards.run()
	msr.rotateLoop(ctx)
}

// Shutdown - safe shutdown ManagerShardRelabeler.
func (msr *ManagerRelabeler) Shutdown(ctx context.Context) error {
	var err error

	close(msr.stop)
	msr.shards.stopLoop()

	select {
	case <-ctx.Done():
		err = errors.Join(err, context.Cause(ctx))
	case <-msr.done:
	}

	errShutdown := msr.destinationGroups.RangeGo(
		func(_ int, dg *DestinationGroup) error {
			return dg.Shutdown(ctx)
		},
	)

	return errors.Join(err, errShutdown)
}

// rotateLoop - main loop for rotate.
func (msr *ManagerRelabeler) rotateLoop(ctx context.Context) {
	defer msr.rotateTimer.Stop()
	defer close(msr.done)
	for {
		select {
		case <-ctx.Done():
			return

		case <-msr.stop:
			return

		case <-msr.metricTimer.Chan():
			msr.rotateLock.RLock()

			msr.shards.enqueueMetricUpdate(NewTaskMetricUpdate(msr.destinationGroups))

			_ = msr.destinationGroups.RangeGo(
				func(_ int, dg *DestinationGroup) error {
					dg.ObserveEncodersMemory()
					return nil
				},
			)

			msr.metricTimer.Reset(defaultMetricDuration)
			msr.rotateLock.RUnlock()
			runtime.GC()

		case <-msr.rotateTimer.Chan():
			msr.rotateLock.Lock()
			msr.rotateWG.Wait()
			if err := msr.rotate(); err != nil {
				Errorf("failed rotate relabeler: %s", err)
				continue
			}
			msr.rotateWG = new(sync.WaitGroup)
			msr.rotateLock.Unlock()
			msr.rotateTimer.Reset()
		}
	}
}

// rotate - rotate main lss and send notifiy to all destination groups.
func (msr *ManagerRelabeler) rotate() error {
	_ = msr.destinationGroups.RangeGo(
		func(_ int, dg *DestinationGroup) error {
			dg.Rotate()
			return nil
		},
	)

	return msr.shards.rotate()
}

//
// DestinationGroups
//

// DestinationGroups wrapper for slice for convenient work.
type DestinationGroups []*DestinationGroup

// Add new DestinationGroup to DestinationGroups.
func (dgs *DestinationGroups) Add(dg *DestinationGroup) {
	(*dgs) = append((*dgs), dg)
}

// RangeGo run goroutines for each group.
func (dgs *DestinationGroups) RangeGo(fn func(destinationGroupID int, destinationGroup *DestinationGroup) error) error {
	errs := make([]error, len(*dgs))
	wg := new(sync.WaitGroup)
	wg.Add(len(*dgs))
	for destinationGroupID, destinationGroup := range *dgs {
		go func(dgid int, dg *DestinationGroup) {
			errs[dgid] = fn(dgid, dg)
			wg.Done()
		}(destinationGroupID, destinationGroup)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// RemoveByID remove DestinationGroup form DestinationGroups by id.
func (dgs *DestinationGroups) RemoveByID(ids []int) {
	for i := len(ids) - 1; i > -1; i-- {
		copy((*dgs)[ids[i]:], (*dgs)[ids[i]+1:])
	}
	(*dgs) = (*dgs)[:len(*dgs)-len(ids)]
}

//
// shards
//

type shardHead struct {
	dataStorage *cppbridge.HeadDataStorage
	encoder     *cppbridge.HeadEncoder
}

func (h *shardHead) AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries) {
	h.encoder.EncodeInnerSeriesSlice(innerSeriesSlice)
}

func (h *shardHead) ResetDataStorage() {
	h.dataStorage.Reset()
}

type shard struct {
	id   uint16
	head *shardHead
}

func (s *shard) ShardID() uint16 {
	return s.id
}

func (s *shard) Head() ShardHead {
	return s.head
}

// shards for relabelers.
type shards struct {
	stop                            chan struct{}
	donesWG                         *sync.WaitGroup
	irrwm                           *sync.RWMutex
	inputRelabelers                 map[relabelerKey]*cppbridge.InputPerShardRelabeler
	heads                           []*shardHead
	shardLsses                      []*cppbridge.LabelSetStorage
	stageMetricUpdate               []chan *TaskMetricUpdate
	stageInputRelabeling            []chan *TaskInputRelabeling
	stageAppendRelabelerSeries      []chan *TaskAppendRelabelerSeries
	stageUpdateRelabelerState       []chan *TaskUpdateRelabelerState
	genericTaskCh                   []chan *GenericTask
	stageOutputRelabeling           []chan *TaskOutputRelabeling
	stageOutputUpdateRelabelerState []chan *TaskOutputUpdateRelabelerState
	numberOfShards                  uint16
	// stat
	memoryInUse *prometheus.GaugeVec
}

// newshards init new shards.
func newshards(
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
	registerer prometheus.Registerer,
) (*shards, error) {
	factory := util.NewUnconflictRegisterer(registerer)
	s := &shards{
		stop: make(chan struct{}),
		inputRelabelers: make(
			map[relabelerKey]*cppbridge.InputPerShardRelabeler,
			int(numberOfShards)*len(inputRelabelerConfigs),
		),
		donesWG: new(sync.WaitGroup),
		irrwm:   new(sync.RWMutex),
		// stat
		memoryInUse: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_relabeler_cgo_memory_bytes",
				Help: "Current value memory in use in bytes.",
			},
			[]string{"allocator", "id"},
		),
	}

	if err := s.reshards(inputRelabelerConfigs, numberOfShards); err != nil {
		return nil, err
	}

	return s, nil
}

// enqueueInputRelabeling send task to shard for input relabeling.
func (s *shards) enqueueInputRelabeling(task *TaskInputRelabeling) {
	for _, s := range s.stageInputRelabeling {
		s <- task
	}
}

// enqueueMetricUpdate send task to shard for update metrics.
func (s *shards) enqueueMetricUpdate(task *TaskMetricUpdate) {
	for _, s := range s.stageMetricUpdate {
		s <- task
	}
}

// enqueueHeadAppending append all series after input relabeling stage to head.
func (s *shards) forEachShard(fn ShardFnInterface) {
	task := NewGenericTask(fn)
	task.wg.Add(int(s.numberOfShards))
	for _, shardGenericTaskCh := range s.genericTaskCh {
		shardGenericTaskCh <- task
	}
	task.wg.Wait()
}

// enqueueOutputRelabeling send task to shard for output relabeling.
func (s *shards) enqueueOutputRelabeling(
	ctx context.Context,
	outputPromise *OutputRelabelingPromise,
	inputPromise *InputRelabelingPromise,
) {
	for i, s := range s.stageOutputRelabeling {
		s <- NewTaskOutputRelabeling(ctx, outputPromise, inputPromise.ShardsInnerSeries(uint16(i)))
	}
}

// enqueueOutputUpdateRelabelerState send task to shard for UpdateRelabelerState.
func (s *shards) enqueueOutputUpdateRelabelerState(
	ctx context.Context,
	outputStateUpdate []*cppbridge.RelabelerStateUpdate,
	destinationGroup *DestinationGroup,
	wg *sync.WaitGroup,
	shardID int,
) {
	s.stageOutputUpdateRelabelerState[shardID] <- NewTaskOutputUpdateRelabelerState(
		ctx,
		outputStateUpdate,
		destinationGroup,
		wg,
	)
}

// reshards changes the number of shards to the required amount.
func (s *shards) reshards(
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	s.stop = make(chan struct{})

	s.reconfiguringStages(numberOfShards)

	s.reconfiguringShardLsses(numberOfShards)

	if err := s.reconfiguringInputRelabeler(inputRelabelerConfigs, numberOfShards); err != nil {
		return err
	}

	s.reconfigureHeads(numberOfShards)

	s.numberOfShards = numberOfShards

	return nil
}

// reconfiguringStages reconfiguring stages for all shards.
func (s *shards) reconfiguringStages(numberOfShards uint16) {
	if s.numberOfShards == numberOfShards {
		return
	}

	s.stageMetricUpdate = make([]chan *TaskMetricUpdate, numberOfShards)
	s.stageInputRelabeling = make([]chan *TaskInputRelabeling, numberOfShards)
	s.stageAppendRelabelerSeries = make([]chan *TaskAppendRelabelerSeries, numberOfShards)
	s.stageUpdateRelabelerState = make([]chan *TaskUpdateRelabelerState, numberOfShards)
	s.genericTaskCh = make([]chan *GenericTask, numberOfShards)
	s.stageOutputRelabeling = make([]chan *TaskOutputRelabeling, numberOfShards)
	s.stageOutputUpdateRelabelerState = make([]chan *TaskOutputUpdateRelabelerState, numberOfShards)

	var shardID uint16
	for ; shardID < numberOfShards; shardID++ {
		s.stageMetricUpdate[shardID] = make(chan *TaskMetricUpdate, chanBufferSize)
		s.stageInputRelabeling[shardID] = make(chan *TaskInputRelabeling, chanBufferSize)
		s.stageAppendRelabelerSeries[shardID] = make(chan *TaskAppendRelabelerSeries, chanBufferSize)
		s.stageUpdateRelabelerState[shardID] = make(chan *TaskUpdateRelabelerState, chanBufferSize)
		s.genericTaskCh[shardID] = make(chan *GenericTask, chanBufferSize)
		s.stageOutputRelabeling[shardID] = make(chan *TaskOutputRelabeling, chanBufferSize)
		s.stageOutputUpdateRelabelerState[shardID] = make(chan *TaskOutputUpdateRelabelerState, chanBufferSize)
	}
}

// relabelerIDIsExist check on exist relabelerID.
func (s *shards) relabelerIDIsExist(relabelerID string) bool {
	s.irrwm.RLock()
	_, ok := s.inputRelabelers[relabelerKey{relabelerID, 0}]
	s.irrwm.RUnlock()
	return ok
}

// reconfiguringShardLsses reconfiguring lss for all shards.
func (s *shards) reconfiguringShardLsses(numberOfShards uint16) {
	if s.numberOfShards == numberOfShards {
		return
	}

	if len(s.shardLsses) > int(numberOfShards) {
		for shardID := range s.shardLsses {
			if shardID >= int(numberOfShards) {
				// clear unnecessary
				s.shardLsses[shardID] = nil
				s.memoryInUse.Delete(prometheus.Labels{"allocator": "main_lss", "id": fmt.Sprintf("%d", shardID)})
				continue
			}
			s.shardLsses[shardID].Reset()
		}
		// cut
		s.shardLsses = s.shardLsses[:numberOfShards]
		return
	}

	// resize
	s.shardLsses = append(
		s.shardLsses,
		make([]*cppbridge.LabelSetStorage, int(numberOfShards)-len(s.shardLsses))...,
	)
	for shardID := 0; shardID < int(numberOfShards); shardID++ {
		if s.shardLsses[shardID] != nil {
			s.shardLsses[shardID].Reset()
			continue
		}

		// create if not exist
		s.shardLsses[shardID] = cppbridge.NewQueryableLssStorage()
	}
}

func (s *shards) reconfigureHeads(numberOfShards uint16) {
	if s.numberOfShards == numberOfShards {
		return
	}

	if len(s.heads) > int(numberOfShards) {
		for shardID := range s.heads {
			if shardID >= int(numberOfShards) {
				s.heads[shardID] = nil
			}
		}
		s.heads = s.heads[:numberOfShards]
		return
	}

	s.heads = append(
		s.heads,
		make([]*shardHead, int(numberOfShards)-len(s.heads))...,
	)

	for shardID := 0; shardID < int(numberOfShards); shardID++ {
		if s.heads[shardID] != nil {
			continue
		}

		dataStorage := cppbridge.NewHeadDataStorage()
		s.heads[shardID] = &shardHead{
			dataStorage: dataStorage,
			encoder:     cppbridge.NewHeadEncoderWithDataStorage(dataStorage),
		}
	}

	return
}

// reconfiguringInputRelabeler reconfiguring input relabelers for all shards.
func (s *shards) reconfiguringInputRelabeler(
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	updated := make(map[relabelerKey]struct{})
	s.irrwm.Lock()
	defer s.irrwm.Unlock()
	for _, cfgs := range inputRelabelerConfigs {
		key := relabelerKey{cfgs.GetName(), 0}
		statelessRelabeler, err := s.updateOrCreateStatelessRelabeler(key, cfgs.GetConfigs())
		if err != nil {
			return err
		}

		for ; key.shardID < numberOfShards; key.shardID++ {
			updated[key] = struct{}{}
			if inputRelabeler, ok := s.inputRelabelers[key]; ok {
				if numberOfShards != inputRelabeler.NumberOfShards() ||
					s.shardLsses[key.shardID].Generation() != inputRelabeler.Generation() {
					inputRelabeler.ResetTo(s.shardLsses[key.shardID].Generation(), numberOfShards)
				}
				continue
			}

			if s.inputRelabelers[key], err = cppbridge.NewInputPerShardRelabeler(
				statelessRelabeler,
				s.shardLsses[key.shardID].Generation(),
				numberOfShards,
				key.shardID,
			); err != nil {
				return err
			}
		}
	}

	for key := range s.inputRelabelers {
		if _, ok := updated[key]; !ok {
			// clear unnecessary
			s.memoryInUse.Delete(
				prometheus.Labels{
					"allocator": fmt.Sprintf("input_relabeler_%s", key.relabelerID),
					"id":        fmt.Sprintf("%d", key.shardID),
				},
			)
			delete(s.inputRelabelers, key)
		}
	}

	return nil
}

// updateOrCreateStatelessRelabeler check inputRelabeler(shardID == 0) for key
// and update configs for StatelessRelabeler, if not exist - create new.
func (s *shards) updateOrCreateStatelessRelabeler(
	key relabelerKey,
	rCfgs []*cppbridge.RelabelConfig,
) (*cppbridge.StatelessRelabeler, error) {
	inputRelabeler, ok := s.inputRelabelers[key]
	if !ok {
		statelessRelabeler, err := cppbridge.NewStatelessRelabeler(rCfgs)
		if err != nil {
			return nil, err
		}
		return statelessRelabeler, nil
	}

	sr := inputRelabeler.StatelessRelabeler()
	if sr.EqualConfigs(rCfgs) {
		return sr, nil
	}

	if err := sr.ResetTo(rCfgs); err != nil {
		return nil, err
	}

	return sr, nil
}

// rotate main lss and input relabelers.
func (s *shards) rotate() error {
	var shardID uint16
	for ; shardID < s.numberOfShards; shardID++ {
		s.shardLsses[shardID].Reset()
		s.heads[shardID].ResetDataStorage()
	}

	for rkey, inputRelabeler := range s.inputRelabelers {
		inputRelabeler.ResetTo(s.shardLsses[rkey.shardID].Generation(), s.numberOfShards)
	}

	return nil
}

// run shards loop for relableling.
func (s *shards) run() {
	var shardID uint16
	for ; shardID < s.numberOfShards; shardID++ {
		s.donesWG.Add(1)
		go s.shardLoop(shardID)
	}
}

// shardLoop run relabeling on the shard.
//
//revive:disable-next-line:function-length long but readable.
//revive:disable-next-line:cognitive-complexity long but understandable.
//revive:disable-next-line:cyclomatic long but understandable.
func (s *shards) shardLoop(shardID uint16) {
	for {
		select {
		case <-s.stop:
			s.donesWG.Done()
			return
		case task := <-s.stageMetricUpdate[shardID]:
			s.memoryInUse.With(
				prometheus.Labels{"allocator": "main_lss", "id": fmt.Sprintf("%d", shardID)},
			).Set(float64(s.shardLsses[shardID].AllocatedMemory()))

			for k, r := range s.inputRelabelers {
				if k.shardID != shardID {
					continue
				}

				s.memoryInUse.With(
					prometheus.Labels{
						"allocator": fmt.Sprintf("input_relabeler_%s", k.relabelerID),
						"id":        fmt.Sprintf("%d", shardID),
					},
				).Set(float64(r.CacheAllocatedMemory()))
			}

			task.updateOutputRelabersMetrics(shardID)

		case task := <-s.stageInputRelabeling[shardID]:
			shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
			shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(s.numberOfShards)

			var err error
			if task.WithStaleNans() {
				err = s.inputRelabelers[relabelerKey{task.RelabelerID(), shardID}].InputRelabelingWithStalenans(
					task.Ctx(),
					s.shardLsses[shardID],
					task.MetricLimits(),
					task.SourceStateByShard(shardID),
					task.StaleNansTS(),
					task.ShardedData(),
					shardsInnerSeries,
					shardsRelabeledSeries,
				)

			} else {
				err = s.inputRelabelers[relabelerKey{task.RelabelerID(), shardID}].InputRelabeling(
					task.Ctx(),
					s.shardLsses[shardID],
					task.MetricLimits(),
					task.ShardedData(),
					shardsInnerSeries,
					shardsRelabeledSeries,
				)
			}

			task.IncomingDataDestroy()
			if err != nil {
				task.AddError(shardID, fmt.Errorf("failed input relabeling shard %d: %w", shardID, err))
				continue
			}

			for sid, relabeledSeries := range shardsRelabeledSeries {
				if relabeledSeries.Size() == 0 {
					task.AddResult(uint16(sid), nil)
					continue
				}

				s.stageAppendRelabelerSeries[sid] <- NewTaskAppendRelabelerSeries(
					task.Ctx(),
					relabeledSeries,
					task.Promise(),
					task.RelabelerID(),
					shardID,
				)
			}

			for sid, innerSeries := range shardsInnerSeries {
				task.AddResult(uint16(sid), innerSeries)
			}

		case task := <-s.stageAppendRelabelerSeries[shardID]:
			relabelerStateUpdate := cppbridge.NewRelabelerStateUpdate()
			innerSeries := cppbridge.NewInnerSeries()

			if err := s.inputRelabelers[relabelerKey{task.RelabelerID(), shardID}].AppendRelabelerSeries(
				task.Ctx(),
				s.shardLsses[shardID],
				relabelerStateUpdate,
				innerSeries,
				task.RelabeledSeries(),
			); err != nil {
				task.AddError(shardID, fmt.Errorf("failed input append relabeler series shard %d: %w", shardID, err))
				continue
			}

			s.stageUpdateRelabelerState[task.SourceShardID()] <- NewTaskUpdateRelabelerState(
				task.Ctx(),
				relabelerStateUpdate,
				task.RelabelerID(),
				shardID,
			)

			task.AddResult(shardID, innerSeries)
		case task := <-s.stageUpdateRelabelerState[shardID]:
			if err := s.inputRelabelers[relabelerKey{task.RelabelerID(), shardID}].UpdateRelabelerState(
				task.Ctx(),
				task.RelabelerStateUpdate(),
				task.RelabeledShardID(),
			); err != nil {
				Errorf("failed input update relabeler state %d: %s", shardID, err)
				continue
			}
		case task := <-s.genericTaskCh[shardID]:
			task.shardFn(&shard{id: shardID, head: s.heads[shardID]})
			task.wg.Done()
		case task := <-s.stageOutputRelabeling[shardID]:
			_ = task.DestinationGroups().RangeGo(
				func(dgid int, dg *DestinationGroup) error {
					outputInnerSeries := cppbridge.NewShardsInnerSeries(1 << dg.ShardsNumberPower())
					relabeledSeries := cppbridge.NewRelabeledSeries()
					if err := dg.OutputRelabeling(
						task.Ctx(),
						s.shardLsses[shardID],
						task.OutputData(),
						outputInnerSeries,
						relabeledSeries,
						shardID,
					); err != nil {
						task.AddError(dgid, uint16(1<<dg.ShardsNumberPower()), err)
						return nil
					}

					for sid, innerSeries := range outputInnerSeries {
						task.AddOutputInnerSeries(dgid, uint16(sid), innerSeries)
					}
					task.AddOutputRelabeledSeries(dgid, shardID, relabeledSeries)
					return nil
				},
			)

		case task := <-s.stageOutputUpdateRelabelerState[shardID]:
			if err := task.DestinationGroup().UpdateRelabelerState(
				task.Ctx(),
				shardID,
				task.OutputData(),
			); err != nil {
				Errorf("failed output update relabeler state %d: %s", shardID, err)
			}
			task.WaitGroupDone()
		}
	}
}

// stopLoop sends a stop signal to loop shards and waits.
func (s *shards) stopLoop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}

	s.donesWG.Wait()
}

//
// TaskMetricUpdate
//

// TaskMetricUpdate - task for stage update metrics.
type TaskMetricUpdate struct {
	destinationGroups *DestinationGroups
}

// NewTaskMetricUpdate init task for stage update metrics.
func NewTaskMetricUpdate(
	destinationGroups *DestinationGroups,
) *TaskMetricUpdate {
	return &TaskMetricUpdate{
		destinationGroups: destinationGroups,
	}
}

// updateOutputRelabersMetrics update output relabers metrics.
func (task *TaskMetricUpdate) updateOutputRelabersMetrics(shardID uint16) {
	_ = task.destinationGroups.RangeGo(
		func(_ int, dg *DestinationGroup) error {
			dg.ObserveCacheAllocatedMemory(shardID)
			return nil
		},
	)
}

//
// TaskInputRelabeling
//

// TaskInputRelabeling - task for stage input relabeling.
type TaskInputRelabeling struct {
	ctx          context.Context
	promise      *InputRelabelingPromise
	incomingData *IncomingData
	metricLimits *cppbridge.MetricLimits
	sourceStates *SourceStates
	staleNansTS  int64
	relabelerID  string
}

// NewTaskInputRelabeling - init task for stage input relabeling.
func NewTaskInputRelabeling(
	ctx context.Context,
	promise *InputRelabelingPromise,
	incomingData *IncomingData,
	metricLimits *cppbridge.MetricLimits,
	sourceStates *SourceStates,
	staleNansTS int64,
	relabelerID string,
) *TaskInputRelabeling {
	return &TaskInputRelabeling{
		ctx:          ctx,
		promise:      promise,
		incomingData: incomingData,
		metricLimits: metricLimits,
		sourceStates: sourceStates,
		staleNansTS:  staleNansTS,
		relabelerID:  relabelerID,
	}
}

// AddError - add to promise error.
func (t *TaskInputRelabeling) AddError(shardID uint16, err error) {
	t.promise.AddError(shardID, err)
}

// AddResult - add to promise result.
func (t *TaskInputRelabeling) AddResult(shardID uint16, innerSeries *cppbridge.InnerSeries) {
	t.promise.AddResult(shardID, innerSeries)
}

// Ctx - return task context.
func (t *TaskInputRelabeling) Ctx() context.Context {
	return t.ctx
}

// RelabelerID - return RelabelerID.
func (t *TaskInputRelabeling) RelabelerID() string {
	return t.relabelerID
}

// MetricLimits return *cppbridge.MetricLimits.
func (t *TaskInputRelabeling) MetricLimits() *cppbridge.MetricLimits {
	return t.metricLimits
}

// ShardedData - return ShardedData.
func (t *TaskInputRelabeling) ShardedData() cppbridge.ShardedData {
	return t.incomingData.ShardedData()
}

// SourceStateByShard return state for stalenans for shard.
func (t *TaskInputRelabeling) SourceStateByShard(shardID uint16) *cppbridge.SourceStaleNansState {
	return t.sourceStates.GetByShard(shardID)
}

// WithStaleNans check task for stalenans states.
func (t *TaskInputRelabeling) WithStaleNans() bool {
	return t.sourceStates != nil
}

// StaleNansTS return timestamp for stalenans.
func (t *TaskInputRelabeling) StaleNansTS() int64 {
	return t.staleNansTS
}

// IncomingDataDestroy increment or destroy IncomingData.
func (t *TaskInputRelabeling) IncomingDataDestroy() {
	t.incomingData.Destroy()
}

// Promise - return *IncomingRelabelingPromise.
func (t *TaskInputRelabeling) Promise() *InputRelabelingPromise {
	return t.promise
}

//
// TaskAppendRelabelerSeries
//

// TaskAppendRelabelerSeries - task for stage add to the lss required shard.
type TaskAppendRelabelerSeries struct {
	ctx             context.Context
	relabeledSeries *cppbridge.RelabeledSeries
	promise         *InputRelabelingPromise
	relabelerID     string
	sourceShardID   uint16
}

// NewTaskAppendRelabelerSeries - init task stage for append relabeler series.
func NewTaskAppendRelabelerSeries(
	ctx context.Context,
	relabeledSeries *cppbridge.RelabeledSeries,
	promise *InputRelabelingPromise,
	relabelerID string,
	sourceShardID uint16,
) *TaskAppendRelabelerSeries {
	return &TaskAppendRelabelerSeries{
		ctx:             ctx,
		relabeledSeries: relabeledSeries,
		promise:         promise,
		relabelerID:     relabelerID,
		sourceShardID:   sourceShardID,
	}
}

// AddError - add to promise error.
func (t *TaskAppendRelabelerSeries) AddError(shardID uint16, err error) {
	t.promise.AddError(shardID, err)
}

// AddResult - add to promise result.
func (t *TaskAppendRelabelerSeries) AddResult(shardID uint16, innerSeries *cppbridge.InnerSeries) {
	t.promise.AddResult(shardID, innerSeries)
}

// Ctx - return task context.
func (t *TaskAppendRelabelerSeries) Ctx() context.Context {
	return t.ctx
}

// Promise - return IncomingRelabelingPromise.
func (t *TaskAppendRelabelerSeries) Promise() *InputRelabelingPromise {
	return t.promise
}

// RelabeledSeries - return *RelabeledSeries.
func (t *TaskAppendRelabelerSeries) RelabeledSeries() *cppbridge.RelabeledSeries {
	return t.relabeledSeries
}

// RelabelerID - return RelabelerID.
func (t *TaskAppendRelabelerSeries) RelabelerID() string {
	return t.relabelerID
}

// SourceShardID - return source shardID.
func (t *TaskAppendRelabelerSeries) SourceShardID() uint16 {
	return t.sourceShardID
}

//
// TaskUpdateRelabelerState
//

// TaskUpdateRelabelerState - task for stage updates the cache in the source shard with the relabeled shard.
type TaskUpdateRelabelerState struct {
	ctx                  context.Context
	relabelerStateUpdate *cppbridge.RelabelerStateUpdate
	relabelerID          string
	relabeledShardID     uint16
}

// NewTaskUpdateRelabelerState - init task for stage updates the cache in the source shard with the relabeled shard.
func NewTaskUpdateRelabelerState(
	ctx context.Context,
	relabelerStateUpdate *cppbridge.RelabelerStateUpdate,
	relabelerID string,
	relabeledShardID uint16,
) *TaskUpdateRelabelerState {
	return &TaskUpdateRelabelerState{
		ctx:                  ctx,
		relabelerStateUpdate: relabelerStateUpdate,
		relabelerID:          relabelerID,
		relabeledShardID:     relabeledShardID,
	}
}

// Ctx - return task context.
func (t *TaskUpdateRelabelerState) Ctx() context.Context {
	return t.ctx
}

// RelabeledShardID - return relabeled shardID.
func (t *TaskUpdateRelabelerState) RelabeledShardID() uint16 {
	return t.relabeledShardID
}

// RelabelerID - return RelabelerID.
func (t *TaskUpdateRelabelerState) RelabelerID() string {
	return t.relabelerID
}

// RelabelerStateUpdate - return *RelabelerStateUpdate.
func (t *TaskUpdateRelabelerState) RelabelerStateUpdate() *cppbridge.RelabelerStateUpdate {
	return t.relabelerStateUpdate
}

//
// InputRelabelingPromise
//

// InputRelabelingPromise - promise for processing incoming data.
type InputRelabelingPromise struct {
	mx        *sync.Mutex
	done      chan struct{}
	data      [][]*cppbridge.InnerSeries
	errors    []error
	shardDone uint16
}

// NewInputRelabelingPromise - init new *InputRelabelingPromise.
func NewInputRelabelingPromise(numberOfShards uint16) *InputRelabelingPromise {
	// id slice - shard id
	data := make([][]*cppbridge.InnerSeries, numberOfShards)
	for i := range data {
		// amount of data = x2 numberOfShards
		data[i] = make([]*cppbridge.InnerSeries, 0, 2*numberOfShards)
	}
	return &InputRelabelingPromise{
		data:      data,
		errors:    make([]error, numberOfShards),
		shardDone: numberOfShards,
		done:      make(chan struct{}),
		mx:        new(sync.Mutex),
	}
}

// AddError - add to promise error.
func (p *InputRelabelingPromise) AddError(shardID uint16, err error) {
	// error on shard
	p.mx.Lock()
	p.errors[shardID] = err
	p.shardDone--

	if p.shardDone == 0 {
		close(p.done)
	}
	p.mx.Unlock()
}

// AddResult - add to promise result.
func (p *InputRelabelingPromise) AddResult(shardID uint16, innerSeries *cppbridge.InnerSeries) {
	p.mx.Lock()
	if innerSeries != nil && innerSeries.Size() == 0 {
		innerSeries = nil
	}
	p.data[shardID] = append(p.data[shardID], innerSeries)
	if cap(p.data[shardID]) == len(p.data[shardID]) {
		p.shardDone--
	}
	if p.shardDone == 0 {
		close(p.done)
	}
	p.mx.Unlock()
}

// ShardsInnerSeries - return slice with the results of relabeling per shards.
func (p *InputRelabelingPromise) ShardsInnerSeries(shardID uint16) []*cppbridge.InnerSeries {
	return p.data[shardID]
}

// Wait - wait until all results are received.
func (p *InputRelabelingPromise) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-p.done:
		return errors.Join(p.errors...)
	}
}

// ShardHead - head appender interface.
type ShardHead interface {
	AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries)
}

// ShardInterface interface.
type ShardInterface interface {
	ShardID() uint16
	Head() ShardHead
}

// ShardFnInterface - shard function.
type ShardFnInterface func(shard ShardInterface)

// GenericTask - generic task, will be executed on each shard.
type GenericTask struct {
	shardFn ShardFnInterface
	wg      *sync.WaitGroup
}

// NewGenericTask - constructor.
func NewGenericTask(shardFn ShardFnInterface) *GenericTask {
	return &GenericTask{shardFn: shardFn, wg: &sync.WaitGroup{}}
}

//
// TaskOutputRelabeling
//

// TaskOutputRelabeling - task for stage output relabeling.
type TaskOutputRelabeling struct {
	ctx        context.Context
	promise    *OutputRelabelingPromise
	outputData []*cppbridge.InnerSeries
}

// NewTaskOutputRelabeling - init task for stage output relabeling.
func NewTaskOutputRelabeling(
	ctx context.Context,
	promise *OutputRelabelingPromise,
	outputData []*cppbridge.InnerSeries,
) *TaskOutputRelabeling {
	return &TaskOutputRelabeling{
		ctx:        ctx,
		promise:    promise,
		outputData: outputData,
	}
}

// AddError - add to promise error.
func (t *TaskOutputRelabeling) AddError(groupID int, dgShards uint16, err error) {
	t.promise.AddError(groupID, dgShards, err)
}

// AddOutputInnerSeries - add to promise inner series.
func (t *TaskOutputRelabeling) AddOutputInnerSeries(
	groupID int,
	shardID uint16,
	innerSeries *cppbridge.InnerSeries,
) {
	t.promise.AddOutputInnerSeries(groupID, shardID, innerSeries)
}

// AddOutputRelabeledSeries - add to promise relabeled series.
func (t *TaskOutputRelabeling) AddOutputRelabeledSeries(
	groupID int,
	shardID uint16,
	relabeledSeries *cppbridge.RelabeledSeries,
) {
	t.promise.AddOutputRelabeledSeries(groupID, shardID, relabeledSeries)
}

// Ctx - return task context.
func (t *TaskOutputRelabeling) Ctx() context.Context {
	return t.ctx
}

// DestinationGroups return pointer to DestinationGroups.
func (t *TaskOutputRelabeling) DestinationGroups() *DestinationGroups {
	return t.promise.DestinationGroups()
}

// OutputData - return OutputData.
func (t *TaskOutputRelabeling) OutputData() []*cppbridge.InnerSeries {
	return t.outputData
}

// Promise - return *OutputRelabelingPromise.
func (t *TaskOutputRelabeling) Promise() *OutputRelabelingPromise {
	return t.promise
}

//
// OutputRelabelingPromise
//

// OutputRelabelingPromise - promise for processing output data.
type OutputRelabelingPromise struct {
	destinationGroups       *DestinationGroups
	mx                      *sync.Mutex
	done                    chan struct{}
	dgOutputInnerSeries     [][][]*cppbridge.InnerSeries   // [groupID[encoderShardID[mainShardID]]]
	dgOutputRelabeledSeries [][]*cppbridge.RelabeledSeries // [groupID[mainShardID]]
	errors                  []error                        // [groupID]
	shardDone               uint16
}

// NewOutputRelabelingPromise - init new *OutputRelabelingPromise.
func NewOutputRelabelingPromise(destinationGroups *DestinationGroups, numberOfShards uint16) *OutputRelabelingPromise {
	groups := len(*destinationGroups)
	p := &OutputRelabelingPromise{
		destinationGroups:       destinationGroups,
		mx:                      new(sync.Mutex),
		done:                    make(chan struct{}),
		dgOutputInnerSeries:     make([][][]*cppbridge.InnerSeries, groups),
		dgOutputRelabeledSeries: make([][]*cppbridge.RelabeledSeries, groups),
		errors:                  make([]error, groups),
		shardDone:               numberOfShards * uint16(groups),
	}

	for i := range p.dgOutputInnerSeries {
		// each destination group can have a different number of shards
		length := 1 << (*destinationGroups)[i].ShardsNumberPower()
		p.dgOutputInnerSeries[i] = make([][]*cppbridge.InnerSeries, length)
		p.shardDone += uint16(length)
		for j := range p.dgOutputInnerSeries[i] {
			p.dgOutputInnerSeries[i][j] = make([]*cppbridge.InnerSeries, 0, numberOfShards)
		}
	}

	for i := range p.dgOutputRelabeledSeries {
		p.dgOutputRelabeledSeries[i] = make([]*cppbridge.RelabeledSeries, numberOfShards)
	}

	return p
}

// AddError - add to promise error.
func (p *OutputRelabelingPromise) AddError(groupID int, dgShards uint16, err error) {
	p.mx.Lock()
	p.errors[groupID] = err
	p.shardDone--

	// if an error occurs in a group, there will be no processed data
	var shardID uint16
	for ; shardID < dgShards; shardID++ {
		p.dgOutputInnerSeries[groupID][shardID] = append(p.dgOutputInnerSeries[groupID][shardID], nil)
		if cap(p.dgOutputInnerSeries[groupID][shardID]) == len(p.dgOutputInnerSeries[groupID][shardID]) {
			p.shardDone--
		}

		if p.shardDone == 0 {
			close(p.done)
		}
	}
	p.mx.Unlock()
}

// AddOutputInnerSeries - add to promise inner series.
func (p *OutputRelabelingPromise) AddOutputInnerSeries(
	groupID int,
	shardID uint16,
	innerSeries *cppbridge.InnerSeries,
) {
	p.mx.Lock()
	if innerSeries.Size() == 0 {
		innerSeries = nil
	}

	p.dgOutputInnerSeries[groupID][shardID] = append(p.dgOutputInnerSeries[groupID][shardID], innerSeries)
	if cap(p.dgOutputInnerSeries[groupID][shardID]) == len(p.dgOutputInnerSeries[groupID][shardID]) {
		p.shardDone--
	}

	if p.shardDone == 0 {
		close(p.done)
	}
	p.mx.Unlock()
}

// AddOutputRelabeledSeries - add to promise relabeled series.
func (p *OutputRelabelingPromise) AddOutputRelabeledSeries(
	groupID int,
	shardID uint16,
	relabeledSeries *cppbridge.RelabeledSeries,
) {
	p.mx.Lock()
	if relabeledSeries.Size() == 0 {
		relabeledSeries = nil
	}

	p.dgOutputRelabeledSeries[groupID][shardID] = relabeledSeries
	p.shardDone--

	if p.shardDone == 0 {
		close(p.done)
	}
	p.mx.Unlock()
}

// DestinationGroups return pointer to DestinationGroups.
func (p *OutputRelabelingPromise) DestinationGroups() *DestinationGroups {
	return p.destinationGroups
}

// OutputInnerSeries - return OutputInnerSeries(series with ls id) on shards.
func (p *OutputRelabelingPromise) OutputInnerSeries(groupID int) [][]*cppbridge.InnerSeries {
	return p.dgOutputInnerSeries[groupID]
}

// OutputRelabeledSeries - return OutputRelabeledSeries(series with label set) on shards.
func (p *OutputRelabelingPromise) OutputRelabeledSeries(groupID int) []*cppbridge.RelabeledSeries {
	return p.dgOutputRelabeledSeries[groupID]
}

// Wait - wait until all results are received.
func (p *OutputRelabelingPromise) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-p.done:
		return errors.Join(p.errors...)
	}
}

//
// TaskOutputUpdateRelabelerState
//

// TaskOutputUpdateRelabelerState - task for stage updates the cache
// in the source shard with the encoders relabeled shard.
type TaskOutputUpdateRelabelerState struct {
	ctx              context.Context
	destinationGroup *DestinationGroup
	wg               *sync.WaitGroup
	outputData       []*cppbridge.RelabelerStateUpdate
}

// NewTaskOutputUpdateRelabelerState - init task for stage updates state output relabeling.
func NewTaskOutputUpdateRelabelerState(
	ctx context.Context,
	encodersStateUpdates []*cppbridge.RelabelerStateUpdate,
	destinationGroup *DestinationGroup,
	wg *sync.WaitGroup,
) *TaskOutputUpdateRelabelerState {
	return &TaskOutputUpdateRelabelerState{
		ctx:              ctx,
		outputData:       encodersStateUpdates,
		destinationGroup: destinationGroup,
		wg:               wg,
	}
}

// Ctx - return task context.
func (t *TaskOutputUpdateRelabelerState) Ctx() context.Context {
	return t.ctx
}

// DestinationGroup return pointer to destination group.
func (t *TaskOutputUpdateRelabelerState) DestinationGroup() *DestinationGroup {
	return t.destinationGroup
}

// WaitGroupDone - decrements the WaitGroup counter by one.
func (t *TaskOutputUpdateRelabelerState) WaitGroupDone() {
	t.wg.Done()
}

// OutputData - return OutputData.
func (t *TaskOutputUpdateRelabelerState) OutputData() []*cppbridge.RelabelerStateUpdate {
	return t.outputData
}

// RotateTimer - custom timer with reset the timer for the delay time.
type RotateTimer struct {
	clock            clockwork.Clock
	timer            clockwork.Timer
	rotateAt         time.Time
	mx               *sync.Mutex
	durationBlock    int64
	rndDurationBlock int64
}

// NewRotateTimer - init new RotateTimer. The duration durationBlock and delayAfterNotify must be greater than zero;
// if not, Ticker will panic. Stop the ticker to release associated resources.
func NewRotateTimer(clock clockwork.Clock, desiredBlockFormationDuration time.Duration) *RotateTimer {
	bd := desiredBlockFormationDuration.Milliseconds()
	//nolint:gosec // there is no need for cryptographic strength here
	rnd := rand.New(rand.NewSource(clock.Now().UnixNano()))
	rt := &RotateTimer{
		clock:            clock,
		durationBlock:    bd,
		rndDurationBlock: rnd.Int63n(bd),
		mx:               new(sync.Mutex),
	}

	rt.rotateAt = rt.RotateAtNext()
	rt.timer = clock.NewTimer(rt.rotateAt.Sub(rt.clock.Now()))

	return rt
}

// Chan - return chan with ticker time.
func (rt *RotateTimer) Chan() <-chan time.Time {
	return rt.timer.Chan()
}

// Reset - changes the timer to expire after duration Block and clearing channels.
func (rt *RotateTimer) Reset() {
	rt.mx.Lock()
	rt.rotateAt = rt.RotateAtNext()
	if !rt.timer.Stop() {
		select {
		case <-rt.timer.Chan():
		default:
		}
	}
	rt.timer.Reset(rt.rotateAt.Sub(rt.clock.Now()))
	rt.mx.Unlock()
}

// RotateAtNext - calculated next rotate time.
func (rt *RotateTimer) RotateAtNext() time.Time {
	now := rt.clock.Now().UnixMilli()
	k := now % rt.durationBlock
	startBlock := math.Floor(float64(now)/float64(rt.durationBlock)) * float64(rt.durationBlock)

	if rt.rndDurationBlock > k {
		return time.UnixMilli(int64(startBlock) + rt.rndDurationBlock)
	}

	return time.UnixMilli(int64(startBlock) + rt.durationBlock + rt.rndDurationBlock)
}

// Stop - prevents the Timer from firing.
// Stop does not close the channel, to prevent a read from the channel succeeding incorrectly.
func (rt *RotateTimer) Stop() {
	if !rt.timer.Stop() {
		<-rt.timer.Chan()
	}
}

// SourceStates state for stalenans for all shards.
type SourceStates struct {
	states []*cppbridge.SourceStaleNansState
}

// NewSourceStates init new SourceStates with 1 mandatory shard.
func NewSourceStates() *SourceStates {
	return &SourceStates{
		states: []*cppbridge.SourceStaleNansState{cppbridge.NewSourceStaleNansState()},
	}
}

// GetByShard return SourceStaleNansState for shard.
func (s *SourceStates) GetByShard(shardID uint16) *cppbridge.SourceStaleNansState {
	return s.states[shardID]
}

// ResizeIfNeed resize states according to the number of shards.
func (s *SourceStates) ResizeIfNeed(numberOfShards int) {
	if len(s.states) == numberOfShards {
		return
	}

	if len(s.states) > numberOfShards {
		// cut
		s.states = s.states[:numberOfShards]
		return
	}

	// grow
	s.states = append(
		s.states,
		make([]*cppbridge.SourceStaleNansState, numberOfShards-len(s.states))...,
	)
	for shardID := 0; shardID < numberOfShards; shardID++ {
		if s.states[shardID] != nil {
			continue
		}

		// create if not exist
		s.states[shardID] = cppbridge.NewSourceStaleNansState()
	}
}
