package distributor

import (
	"context"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
)

type Distributor struct {
	destinationGroups relabeler.DestinationGroups
}

func NewDistributor(destinationGroups relabeler.DestinationGroups) *Distributor {
	return &Distributor{
		destinationGroups: destinationGroups,
	}
}

func (d *Distributor) Send(ctx context.Context, head relabeler.Head, shardedData [][]*cppbridge.InnerSeries) error {
	outputPromise := NewOutputRelabelingPromise(&d.destinationGroups, head.NumberOfShards())
	err := head.ForEachShard(func(shard relabeler.Shard) error {
		return d.destinationGroups.RangeGo(func(destinationGroupID int, destinationGroup *relabeler.DestinationGroup) error {
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
		func(destinationGroupID int, destinationGroup *relabeler.DestinationGroup) error {
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
				err = head.OnShard(uint16(shardID), func(shard relabeler.Shard) error {
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

func (d *Distributor) DestinationGroups() relabeler.DestinationGroups {
	return d.destinationGroups
}

func (d *Distributor) Rotate() error {
	return d.destinationGroups.RangeGo(func(_ int, destinationGroup *relabeler.DestinationGroup) error {
		destinationGroup.Rotate()
		return nil
	})
}

func (d *Distributor) SetDestinationGroups(destinationGroups relabeler.DestinationGroups) {
	d.destinationGroups = destinationGroups
}

func (d *Distributor) Shutdown(ctx context.Context) error {
	return d.destinationGroups.RangeGo(func(_ int, destinationGroup *relabeler.DestinationGroup) error {
		return destinationGroup.Shutdown(ctx)
	})
}
