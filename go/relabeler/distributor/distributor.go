package distributor

import (
	"context"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
)

type Distributor struct {
	destinationGroups DestinationGroups
}

func NewDistributor(destinationGroups DestinationGroups) *Distributor {
	return &Distributor{
		destinationGroups: destinationGroups,
	}
}

func (d *Distributor) Send(ctx context.Context, head relabeler.HeadInterface, shardedData [][]*cppbridge.InnerSeries) error {
	outputPromise := NewOutputRelabelingPromise(&d.destinationGroups, head.NumberOfShards())
	err := head.ForEachShard(func(shard relabeler.ShardInterface) error {
		return d.destinationGroups.RangeGo(func(destinationGroupID int, destinationGroup *DestinationGroup) error {
			outputInnerSeries := cppbridge.NewShardsInnerSeries(1 << destinationGroup.ShardsNumberPower())
			relabeledSeries := cppbridge.NewRelabeledSeries()
			if err := destinationGroup.OutputRelabeling(
				ctx,
				shard.LSS().Raw(),
				shardedData[shard.ShardID()],
				outputInnerSeries,
				relabeledSeries,
				shard.ShardID(),
			); err != nil {
				outputPromise.AddError(destinationGroupID, uint16(1<<destinationGroup.ShardsNumberPower()), err)
				return nil
			}

			for sid, innerSeries := range outputInnerSeries {
				outputPromise.AddOutputInnerSeries(destinationGroupID, uint16(sid), innerSeries)
			}
			outputPromise.AddOutputRelabeledSeries(destinationGroupID, shard.ShardID(), relabeledSeries)
			return nil
		})
	})

	if err != nil {
		return err
	}

	return d.destinationGroups.RangeGo(
		func(destinationGroupID int, destinationGroup *DestinationGroup) error {
			destinationGroup.EncodersLock()
			defer destinationGroup.EncodersUnlock()

			outputStateUpdates := destinationGroup.OutputStateUpdates()
			if _, err = destinationGroup.AppendOpenHead(
				ctx,
				outputPromise.OutputInnerSeries(destinationGroupID),
				outputPromise.OutputRelabeledSeries(destinationGroupID),
				outputStateUpdates,
			); err != nil {
				return err
			}

			for shardID, outputStateUpdate := range outputStateUpdates {
				err = head.OnShard(uint16(shardID), func(shard relabeler.ShardInterface) error {
					return d.destinationGroups[destinationGroupID].UpdateRelabelerState(
						ctx,
						shard.ShardID(),
						outputStateUpdate,
					)
				})
				if err != nil {
					return err
				}
			}
			return nil
		},
	)
}

func (d *Distributor) Rotate() {
	_ = d.destinationGroups.RangeGo(func(_ int, destinationGroup *DestinationGroup) error {
		destinationGroup.Rotate()
		return nil
	})
}

func (d *Distributor) Shutdown(ctx context.Context) error {
	return d.destinationGroups.RangeGo(func(_ int, destinationGroup *DestinationGroup) error {
		return destinationGroup.Shutdown(ctx)
	})
}
