package appender

import (
	"context"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/storage"
	"sync"
)

type QueryableAppender struct {
	lock        sync.Mutex
	openHead    relabeler.UpgradableHeadInterface
	distributor relabeler.UpgradableDistributorInterface
}

func NewQueryableAppender(head relabeler.UpgradableHeadInterface, distributor relabeler.UpgradableDistributorInterface) *QueryableAppender {
	return &QueryableAppender{
		openHead:    head,
		distributor: distributor,
	}
}

func (qa *QueryableAppender) Append(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	metricLimits *cppbridge.MetricLimits,
	relabelerID string,
) error {
	return qa.AppendWithStaleNans(
		ctx,
		incomingData,
		metricLimits,
		nil,
		0,
		relabelerID,
	)
}

func (qa *QueryableAppender) AppendWithStaleNans(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	metricLimits *cppbridge.MetricLimits,
	sourceStates *relabeler.SourceStates,
	staleNansTS int64,
	relabelerID string,
) error {
	qa.lock.Lock()
	defer qa.lock.Unlock()

	data, err := qa.openHead.Append(ctx, incomingData, metricLimits, sourceStates, staleNansTS, relabelerID)
	if err != nil {
		return err
	}

	if err = qa.distributor.Send(ctx, qa.openHead, data); err != nil {
		return err
	}

	return nil
}

func (qa *QueryableAppender) Rotate(headRotator relabeler.HeadRotator) {
	qa.lock.Lock()
	defer qa.lock.Unlock()

	qa.openHead = headRotator.Rotate(qa.openHead)
	qa.distributor.Rotate()
}

func (qa *QueryableAppender) Upgrade(headUpgrader relabeler.HeadUpgrader, distributorUpgrader relabeler.DistributorUpgrader) {
	qa.lock.Lock()
	defer qa.lock.Unlock()
	qa.openHead.Finalize()
	qa.openHead = headUpgrader.Upgrade(qa.openHead)
	qa.distributor = distributorUpgrader.Upgrade(qa.distributor)
}

func (qa *QueryableAppender) Querier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	// todo: implementation required
	panic("implement me")
}
