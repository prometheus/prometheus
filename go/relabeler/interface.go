package relabeler

import (
	"context"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// DataStorage - data storage interface.
type DataStorage interface {
	AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries)
	Raw() *cppbridge.HeadDataStorage
}

type LSS interface {
	Raw() *cppbridge.LabelSetStorage
}

// ShardInterface interface.
type ShardInterface interface {
	ShardID() uint16
	DataStorage() DataStorage
	LSS() LSS
}

// ShardFnInterface - shard function.
type ShardFnInterface func(shard ShardInterface) error

type HeadInterface interface {
	Append(ctx context.Context, incomingData *IncomingData, metricLimits *cppbridge.MetricLimits, sourceStates *SourceStates, staleNansTS int64, relabelerID string) ([][]*cppbridge.InnerSeries, error)
	ForEachShard(fn ShardFnInterface) error
	OnShard(shardID uint16, fn ShardFnInterface) error
	NumberOfShards() uint16
	Finalize()
	Close() error
}

type UpgradableHeadInterface interface {
	HeadInterface
}

type DistributorInterface interface {
	Send(ctx context.Context, head HeadInterface, shardedData [][]*cppbridge.InnerSeries) error
	Rotate()
}

type UpgradableDistributorInterface interface {
	DistributorInterface
}

type HeadRotator interface {
	Rotate(head UpgradableHeadInterface) UpgradableHeadInterface
}

type HeadUpgrader interface {
	Upgrade(head UpgradableHeadInterface) UpgradableHeadInterface
}

type DistributorUpgrader interface {
	Upgrade(distributor UpgradableDistributorInterface) UpgradableDistributorInterface
}
