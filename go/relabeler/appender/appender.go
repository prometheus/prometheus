package appender

import (
	"context"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/storage"
	"sync"
)

type QueryableAppender struct {
	lock        sync.Mutex
	head        relabeler.UpgradableHeadInterface
	distributor relabeler.UpgradableDistributorInterface
}

func NewQueryableAppender(head relabeler.UpgradableHeadInterface, distributor relabeler.UpgradableDistributorInterface) *QueryableAppender {
	return &QueryableAppender{
		head:        head,
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

	data, err := qa.head.Append(ctx, incomingData, metricLimits, sourceStates, staleNansTS, relabelerID)
	if err != nil {
		return err
	}

	if err = qa.distributor.Send(ctx, qa.head, data); err != nil {
		return err
	}

	return nil
}

func (qa *QueryableAppender) Rotate(headRotator relabeler.HeadRotator) error {
	qa.lock.Lock()
	defer qa.lock.Unlock()

	qa.head.Finalize()
	rotatedHead, err := headRotator.Rotate(qa.head)
	if err != nil {
		return err
	}

	qa.head = rotatedHead
	qa.distributor.Rotate()

	return nil
}

func (qa *QueryableAppender) Upgrade(headUpgrader relabeler.HeadUpgrader, distributorUpgrader relabeler.DistributorUpgrader) error {
	qa.lock.Lock()
	defer qa.lock.Unlock()

	upgradedHead, err := headUpgrader.Upgrade(qa.head)
	if err != nil {
		return fmt.Errorf("failed to upgrade head: %w", err)
	}

	upgradedDistributor, err := distributorUpgrader.Upgrade(qa.distributor)
	if err != nil {
		return fmt.Errorf("failed to upgrade distributor: %w", err)
	}

	qa.head = upgradedHead
	qa.distributor = upgradedDistributor

	return nil
}

func (qa *QueryableAppender) Querier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	// todo: implementation required
	panic("implement me")
}
