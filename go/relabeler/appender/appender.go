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
	head        relabeler.Head
	distributor relabeler.Distributor
}

func NewQueryableAppender(head relabeler.Head, distributor relabeler.Distributor) *QueryableAppender {
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

func (qa *QueryableAppender) WriteMetrics() {
	qa.lock.Lock()
	defer qa.lock.Unlock()

	qa.head.WriteMetrics()
	qa.distributor.WriteMetrics(qa.head)
}

func (qa *QueryableAppender) Rotate() error {
	qa.lock.Lock()
	defer qa.lock.Unlock()

	if err := qa.head.Rotate(); err != nil {
		return fmt.Errorf("failed to rotate head: %w", err)
	}

	if err := qa.distributor.Rotate(); err != nil {
		return fmt.Errorf("failed to rotate distributor: %w", err)
	}

	return nil
}

func (qa *QueryableAppender) Reconfigure(headConfigurator relabeler.HeadConfigurator, distributorConfigurator relabeler.DistributorConfigurator) error {
	qa.lock.Lock()
	defer qa.lock.Unlock()

	if err := headConfigurator.Configure(qa.head); err != nil {
		return fmt.Errorf("failed to reconfigure head: %w", err)
	}

	if err := distributorConfigurator.Configure(qa.distributor); err != nil {
		return fmt.Errorf("failed to upgrade distributor: %w", err)
	}

	return nil
}

func (qa *QueryableAppender) Querier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	// todo: implementation required
	panic("implement me")
}
