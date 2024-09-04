package appender

import (
	"context"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

type Storage interface {
	Add(head relabeler.Head)
}

type HeadBuilder interface {
	Build() (relabeler.Head, error)
}

type RotatableHead struct {
	head    relabeler.Head
	storage Storage
	builder HeadBuilder
}

func (h *RotatableHead) Append(ctx context.Context, incomingData *relabeler.IncomingData, metricLimits *cppbridge.MetricLimits, sourceStates *relabeler.SourceStates, staleNansTS int64, relabelerID string) ([][]*cppbridge.InnerSeries, error) {
	return h.head.Append(ctx, incomingData, metricLimits, sourceStates, staleNansTS, relabelerID)
}

func (h *RotatableHead) ForEachShard(fn relabeler.ShardFn) error {
	return h.head.ForEachShard(fn)
}

func (h *RotatableHead) OnShard(shardID uint16, fn relabeler.ShardFn) error {
	return h.head.OnShard(shardID, fn)
}

func (h *RotatableHead) NumberOfShards() uint16 {
	return h.head.NumberOfShards()
}

func (h *RotatableHead) Finalize() {}

func (h *RotatableHead) Reconfigure(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error {
	return h.head.Reconfigure(inputRelabelerConfigs, numberOfShards)
}

func (h *RotatableHead) Close() error {
	return h.head.Close()
}

func NewRotatableHead(storage Storage, builder HeadBuilder) (*RotatableHead, error) {
	hd, err := builder.Build()
	if err != nil {
		return nil, err
	}

	return &RotatableHead{
		head:    hd,
		storage: storage,
		builder: builder,
	}, nil
}

func (r *RotatableHead) Rotate() error {
	newHead, err := r.builder.Build()
	if err != nil {
		return err
	}

	r.head.Finalize()
	r.storage.Add(r.head)
	r.head = newHead

	return nil
}
