package head

import (
	"context"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"sync"
)

// TaskInputRelabeling - task for stage input relabeling.
type TaskInputRelabeling struct {
	ctx          context.Context
	promise      *InputRelabelingPromise
	incomingData *relabeler.DestructibleIncomingData
	metricLimits *cppbridge.MetricLimits
	sourceStates *relabeler.SourceStates
	staleNansTS  int64
	relabelerID  string
}

// NewTaskInputRelabeling - init task for stage input relabeling.
func NewTaskInputRelabeling(
	ctx context.Context,
	promise *InputRelabelingPromise,
	incomingData *relabeler.DestructibleIncomingData,
	metricLimits *cppbridge.MetricLimits,
	sourceStates *relabeler.SourceStates,
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
	return t.incomingData.Data().ShardedData()
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

// GenericTask - generic task, will be executed on each shard.
type GenericTask struct {
	errs    []error
	shardFn relabeler.ShardFn
	wg      *sync.WaitGroup
}

func (t *GenericTask) Wait() {
	t.wg.Wait()
}

func (t *GenericTask) Errors() []error {
	return t.errs
}

func (t *GenericTask) ExecuteOnShard(shard relabeler.Shard) {
	t.errs[shard.ShardID()] = t.shardFn(shard)
}

// NewGenericTask - constructor.
func NewGenericTask(shardFn relabeler.ShardFn, numberOfShards uint16) *GenericTask {
	errs := make([]error, numberOfShards)
	wg := &sync.WaitGroup{}
	wg.Add(int(numberOfShards))
	return &GenericTask{errs: errs, shardFn: shardFn, wg: wg}
}

// NewSingleGenericTask - constructor.
func NewSingleGenericTask(shardFn relabeler.ShardFn, numberOfShards uint16) *GenericTask {
	errs := make([]error, numberOfShards)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	return &GenericTask{errs: errs, shardFn: shardFn, wg: wg}
}
