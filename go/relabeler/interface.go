package relabeler

import (
	"context"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"sync/atomic"
)

// DataStorage - data storage interface.
type DataStorage interface {
	AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries)
	Raw() *cppbridge.HeadDataStorage
	MergeOutOfOrderChunks()
	Query(query cppbridge.HeadDataStorageQuery) *cppbridge.HeadDataStorageSerializedChunks
	AllocatedMemory() uint64
}

type LSS interface {
	Raw() *cppbridge.LabelSetStorage
	AllocatedMemory() uint64
	QueryLabelValues(label_name string, matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelValuesResult
	QueryLabelNames(matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelNamesResult
	Query(matchers []model.LabelMatcher) *cppbridge.LSSQueryResult
	GetLabelSets(labelSetIDs []uint32) *cppbridge.LabelSetStorageGetLabelSetsResult
}

type InputRelabeler interface {
	CacheAllocatedMemory() uint64
}

// Shard interface.
type Shard interface {
	ShardID() uint16
	DataStorage() DataStorage
	LSS() LSS
	InputRelabelers() map[string]InputRelabeler
}

// ShardFn - shard function.
type ShardFn func(shard Shard) error

type Head interface {
	Generation() uint64
	ReferenceCounter() ReferenceCounter
	Append(ctx context.Context, incomingData *IncomingData, metricLimits *cppbridge.MetricLimits, sourceStates *SourceStates, staleNansTS int64, relabelerID string) ([][]*cppbridge.InnerSeries, error)
	ForEachShard(fn ShardFn) error
	OnShard(shardID uint16, fn ShardFn) error
	NumberOfShards() uint16
	Finalize()
	Reconfigure(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error
	WriteMetrics()
	Rotate() error
	Close() error
}

type Distributor interface {
	Send(ctx context.Context, head Head, shardedData [][]*cppbridge.InnerSeries) error
	// DestinationGroups - workaround.
	DestinationGroups() DestinationGroups
	// SetDestinationGroups - workaround.
	SetDestinationGroups(destinationGroups DestinationGroups)
	WriteMetrics(head Head)
	Rotate() error
}

type HeadConfigurator interface {
	Configure(head Head) error
}

type DistributorConfigurator interface {
	Configure(distributor Distributor) error
}

type DestructibleIncomingData struct {
	data          *IncomingData
	destructCount atomic.Int64
}

func NewDestructibleIncomingData(data *IncomingData, destructCount int) *DestructibleIncomingData {
	d := &DestructibleIncomingData{
		data: data,
	}
	d.destructCount.Store(int64(destructCount))
	return d
}

func (d *DestructibleIncomingData) Data() *IncomingData {
	return d.data
}

func (d *DestructibleIncomingData) Destroy() {
	if d.destructCount.Add(-1) != 0 {
		return
	}
	d.data.Destroy()
}

type ReferenceCounter interface {
	Add(delta int64) int64
	Value() int64
}

type AtomicReferenceCounter struct {
	value atomic.Int64
}

func (rc *AtomicReferenceCounter) Add(delta int64) int64 {
	return rc.value.Add(delta)
}

func (rc *AtomicReferenceCounter) Value() int64 {
	value := rc.value.Load()
	return value
}

type LoggedAtomicReferenceCounter struct {
	prefix string
	value  atomic.Int64
}

func NewLoggedAtomicReferenceCounter(prefix string) *LoggedAtomicReferenceCounter {
	return &LoggedAtomicReferenceCounter{
		prefix: prefix,
	}
}

func (rc *LoggedAtomicReferenceCounter) Add(delta int64) int64 {
	newValue := rc.value.Add(delta)
	fmt.Println(rc.prefix, ": Reference Counter: add: ", delta, ", value: ", newValue)
	return newValue
}

func (rc *LoggedAtomicReferenceCounter) Value() int64 {
	value := rc.value.Load()
	fmt.Println(rc.prefix, ": Reference Counter: value: ", value)
	return value
}
