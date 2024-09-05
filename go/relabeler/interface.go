package relabeler

import (
	"context"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"sync/atomic"
)

// DataStorage - data storage interface.
type DataStorage interface {
	AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries)
	Raw() *cppbridge.HeadDataStorage
}

type LSS interface {
	Raw() *cppbridge.LabelSetStorage
}

// Shard interface.
type Shard interface {
	ShardID() uint16
	DataStorage() DataStorage
	LSS() LSS
}

// ShardFn - shard function.
type ShardFn func(shard Shard) error

type Head interface {
	Append(ctx context.Context, incomingData *IncomingData, metricLimits *cppbridge.MetricLimits, sourceStates *SourceStates, staleNansTS int64, relabelerID string) ([][]*cppbridge.InnerSeries, error)
	ForEachShard(fn ShardFn) error
	OnShard(shardID uint16, fn ShardFn) error
	NumberOfShards() uint16
	Finalize()
	Reconfigure(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error
	Rotate() error
	Close() error
}

type Distributor interface {
	Send(ctx context.Context, head Head, shardedData [][]*cppbridge.InnerSeries) error
	// DestinationGroups - workaround.
	DestinationGroups() DestinationGroups
	// SetDestinationGroups - workaround.
	SetDestinationGroups(destinationGroups DestinationGroups)
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
