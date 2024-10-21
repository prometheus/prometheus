package manager

import (
	"context"
	"errors"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

type DiscardableHead struct {
	head      relabeler.Head
	onRotate  func(id string, err error) error
	onDiscard func(id string) error
}

func NewDiscardableHead(head relabeler.Head, onDiscard func(id string) error) *DiscardableHead {
	return &DiscardableHead{
		head:      head,
		onDiscard: onDiscard,
	}
}

func (h *DiscardableHead) ID() string {
	return h.head.ID()
}

func (h *DiscardableHead) Generation() uint64 {
	return h.head.Generation()
}

func (h *DiscardableHead) ReferenceCounter() relabeler.ReferenceCounter {
	return h.head.ReferenceCounter()
}

func (h *DiscardableHead) Append(ctx context.Context, incomingData *relabeler.IncomingData, metricLimits *cppbridge.MetricLimits, sourceStates *relabeler.SourceStates, staleNansTS int64, relabelerID string) ([][]*cppbridge.InnerSeries, error) {
	return h.head.Append(ctx, incomingData, metricLimits, sourceStates, staleNansTS, relabelerID)
}

func (h *DiscardableHead) ForEachShard(fn relabeler.ShardFn) error {
	return h.head.ForEachShard(fn)
}

func (h *DiscardableHead) OnShard(shardID uint16, fn relabeler.ShardFn) error {
	return h.head.OnShard(shardID, fn)
}

func (h *DiscardableHead) NumberOfShards() uint16 {
	return h.head.NumberOfShards()
}

func (h *DiscardableHead) Finalize() {
	h.head.Finalize()
}

func (h *DiscardableHead) Reconfigure(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error {
	return h.head.Reconfigure(inputRelabelerConfigs, numberOfShards)
}

func (h *DiscardableHead) WriteMetrics() {
	h.head.WriteMetrics()
}

func (h *DiscardableHead) Status(limit int) relabeler.HeadStatus {
	return h.head.Status(limit)
}

func (h *DiscardableHead) Rotate() error {
	return h.head.Rotate()
}

func (h *DiscardableHead) Close() error {
	return h.head.Close()
}

func (h *DiscardableHead) Discard() (err error) {
	err = h.head.Discard()
	if h.onDiscard != nil {
		err = errors.Join(err, h.onDiscard(h.ID()))
		h.onDiscard = nil
	}
	return err
}
