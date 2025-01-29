package relabeler

import (
	"context"
	"sync/atomic"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
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
	Query(matchers []model.LabelMatcher, querySource uint32) *cppbridge.LSSQueryResult
	GetLabelSets(labelSetIDs []uint32) *cppbridge.LabelSetStorageGetLabelSetsResult
}

type Wal interface {
	Write(innerSeriesSlice []*cppbridge.InnerSeries) error
	// DO NOT USE in public interfaces like ForEachShard
	Uncommited() bool
	Commit() error
}

type InputRelabeler interface {
	CacheAllocatedMemory() uint64
}

// Shard interface.
type Shard interface {
	ShardID() uint16
	DataStorage() DataStorage
	LSS() LSS
	Wal() Wal
}

// ShardFn - shard function.
type ShardFn func(shard Shard) error

type Head interface {
	ID() string
	Generation() uint64
	Append(
		ctx context.Context,
		incomingData *IncomingData,
		state *cppbridge.State,
		relabelerID string,
		commitToWal bool,
	) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error)
	CommitToWal() error
	ForEachShard(fn ShardFn) error
	OnShard(shardID uint16, fn ShardFn) error
	NumberOfShards() uint16
	Finalize() error
	Reconfigure(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error
	WriteMetrics()
	Status(limit int) HeadStatus
	Rotate() error
	Close() error
	Discard() error
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

// HeadStatus holds information about all shards.
type HeadStatus struct {
	HeadStats                   HeadStats  `json:"headStats"`
	SeriesCountByMetricName     []HeadStat `json:"seriesCountByMetricName"`
	LabelValueCountByLabelName  []HeadStat `json:"labelValueCountByLabelName"`
	MemoryInBytesByLabelName    []HeadStat `json:"memoryInBytesByLabelName"`
	SeriesCountByLabelValuePair []HeadStat `json:"seriesCountByLabelValuePair"`
}

// HeadStat holds the information about individual cardinality.
type HeadStat struct {
	Name  string `json:"name"`
	Value uint64 `json:"value"`
}

// HeadStats has information about the head.
type HeadStats struct {
	NumSeries             uint64 `json:"numSeries"`
	NumLabelPairs         int    `json:"numLabelPairs"`
	ChunkCount            int64  `json:"chunkCount"`
	MinTime               int64  `json:"minTime"`
	MaxTime               int64  `json:"maxTime"`
	RuleQueriedSeries     int64  `json:"-"`
	FederateQueriedSeries int64  `json:"-"`
	OtherQueriedSeries    int64  `json:"-"`
}
