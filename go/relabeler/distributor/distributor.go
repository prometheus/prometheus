package distributor

import (
	"context"
	"errors"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"sync"
)

type Distributor struct {
	lock              sync.Mutex
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
		return d.ParallelRange(func(destinationGroupID int, destinationGroup *relabeler.DestinationGroup) error {
			outputInnerSeries := cppbridge.NewShardsInnerSeries(1 << destinationGroup.ShardsNumberPower())
			relabeledSeries := cppbridge.NewRelabeledSeries()
			if relabelingErr := destinationGroup.OutputRelabeling(
				ctx,
				shard.LSS().Raw(),
				shardedData[shard.ShardID()],
				outputInnerSeries,
				relabeledSeries,
				shard.ShardID(),
			); relabelingErr != nil {
				outputPromise.AddError(destinationGroupID, uint16(1<<destinationGroup.ShardsNumberPower()), relabelingErr)
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

	return d.ParallelRange(
		func(destinationGroupID int, destinationGroup *relabeler.DestinationGroup) error {
			destinationGroup.EncodersLock()
			defer destinationGroup.EncodersUnlock()

			outputStateUpdates := destinationGroup.OutputStateUpdates()
			if _, appendErr := destinationGroup.AppendOpenHead(
				ctx,
				outputPromise.OutputInnerSeries(destinationGroupID),
				outputPromise.OutputRelabeledSeries(destinationGroupID),
				outputStateUpdates,
			); appendErr != nil {
				return appendErr
			}

			for shardID, outputStateUpdate := range outputStateUpdates {
				updateErr := head.OnShard(uint16(shardID), func(shard relabeler.Shard) error {
					return destinationGroup.UpdateRelabelerState(
						ctx,
						shard.ShardID(),
						outputStateUpdate,
					)
				})
				if updateErr != nil {
					return updateErr
				}
			}
			return nil
		},
	)
}

func (d *Distributor) DestinationGroups() relabeler.DestinationGroups {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.destinationGroups
}

func (d *Distributor) Rotate() error {
	return d.ParallelRange(func(_ int, destinationGroup *relabeler.DestinationGroup) error {
		destinationGroup.Rotate()
		return nil
	})
}

func (d *Distributor) SetDestinationGroups(destinationGroups relabeler.DestinationGroups) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.destinationGroups = destinationGroups
}

func (d *Distributor) Shutdown(ctx context.Context) error {
	return d.ParallelRange(func(_ int, destinationGroup *relabeler.DestinationGroup) error {
		return destinationGroup.Shutdown(ctx)
	})
}

func (d *Distributor) ParallelRange(fn func(destinationGroupID int, destinationGroup *relabeler.DestinationGroup) error) error {
	d.lock.Lock()
	defer d.lock.Unlock()
	errs := make([]error, len(d.destinationGroups))
	wg := new(sync.WaitGroup)
	wg.Add(len(d.destinationGroups))
	for destinationGroupID, destinationGroup := range d.destinationGroups {
		go func(destinationGroupID int, destinationGroup *relabeler.DestinationGroup) {
			errs[destinationGroupID] = fn(destinationGroupID, destinationGroup)
			wg.Done()
		}(destinationGroupID, destinationGroup)
	}
	wg.Wait()
	return errors.Join(errs...)
}
