package manager

import (
	"context"
	"errors"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

type DiscardableRotatableHead struct {
	head       relabeler.Head
	onRotate   func(id string, err error) error
	onDiscard  func(id string) error
	afterClose func(id string) error
}

func NewDiscardableRotatableHead(head relabeler.Head, onRotate func(id string, err error) error, onDiscard func(id string) error, afterClose func(id string) error) *DiscardableRotatableHead {
	return &DiscardableRotatableHead{
		head:       head,
		onRotate:   onRotate,
		onDiscard:  onDiscard,
		afterClose: afterClose,
	}
}

func (h *DiscardableRotatableHead) ID() string {
	return h.head.ID()
}

func (h *DiscardableRotatableHead) Generation() uint64 {
	return h.head.Generation()
}

func (h *DiscardableRotatableHead) Append(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	state *cppbridge.State,
	relabelerID string) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error) {
	return h.head.Append(ctx, incomingData, state, relabelerID)
}

func (h *DiscardableRotatableHead) ForEachShard(fn relabeler.ShardFn) error {
	return h.head.ForEachShard(fn)
}

func (h *DiscardableRotatableHead) OnShard(shardID uint16, fn relabeler.ShardFn) error {
	return h.head.OnShard(shardID, fn)
}

func (h *DiscardableRotatableHead) NumberOfShards() uint16 {
	return h.head.NumberOfShards()
}

func (h *DiscardableRotatableHead) Finalize() {
	h.head.Finalize()
}

func (h *DiscardableRotatableHead) Reconfigure(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error {
	return h.head.Reconfigure(inputRelabelerConfigs, numberOfShards)
}

func (h *DiscardableRotatableHead) WriteMetrics() {
	h.head.WriteMetrics()
}

func (h *DiscardableRotatableHead) Status(limit int) relabeler.HeadStatus {
	return h.head.Status(limit)
}

func (h *DiscardableRotatableHead) Rotate() error {
	err := h.head.Rotate()
	if h.onRotate != nil {
		err = errors.Join(err, h.onRotate(h.ID(), err))
		h.onRotate = nil
	}
	return err
}

func (h *DiscardableRotatableHead) Close() error {
	err := h.head.Close()
	if h.afterClose != nil {
		err = errors.Join(err, h.afterClose(h.ID()))
	}
	return err
}

func (h *DiscardableRotatableHead) Discard() (err error) {
	err = h.head.Discard()
	if h.onDiscard != nil {
		err = errors.Join(err, h.onDiscard(h.ID()))
		h.onDiscard = nil
	}
	return err
}
