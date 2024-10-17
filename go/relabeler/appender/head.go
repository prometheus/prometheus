package appender

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

// Storage - head storage.
type Storage interface {
	Add(head relabeler.Head)
}

// HeadBuilder - head builder.
type HeadBuilder interface {
	Build() (relabeler.Head, error)
}

// RotatableHead - head wrapper, allows rotations.
type RotatableHead struct {
	head    relabeler.Head
	storage Storage
	builder HeadBuilder
}

// Generation - relabeler.Head interface implementation.
func (h *RotatableHead) Generation() uint64 {
	return h.head.Generation()
}

// ReferenceCounter - relabeler.Head interface implementation.
func (h *RotatableHead) ReferenceCounter() relabeler.ReferenceCounter {
	return h.head.ReferenceCounter()
}

// Append - relabeler.Head interface implementation.
func (h *RotatableHead) Append(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	options cppbridge.RelabelerOptions,
	sourceStates *relabeler.SourceStates,
	staleNansTS int64,
	relabelerID string,
) ([][]*cppbridge.InnerSeries, error) {
	return h.head.Append(ctx, incomingData, options, sourceStates, staleNansTS, relabelerID)
}

// ForEachShard - relabeler.Head interface implementation.
func (h *RotatableHead) ForEachShard(fn relabeler.ShardFn) error {
	return h.head.ForEachShard(fn)
}

// OnShard - relabeler.Head interface implementation.
func (h *RotatableHead) OnShard(shardID uint16, fn relabeler.ShardFn) error {
	return h.head.OnShard(shardID, fn)
}

// NumberOfShards - relabeler.Head interface implementation.
func (h *RotatableHead) NumberOfShards() uint16 {
	return h.head.NumberOfShards()
}

// Finalize - relabeler.Head interface implementation.
func (*RotatableHead) Finalize() {}

// Reconfigure - relabeler.Head interface implementation.
func (h *RotatableHead) Reconfigure(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error {
	return h.head.Reconfigure(inputRelabelerConfigs, numberOfShards)
}

// WriteMetrics - relabeler.Head interface implementation.
func (h *RotatableHead) WriteMetrics() {
	h.head.WriteMetrics()
}

// Status return head stats.
func (h *RotatableHead) Status(limit int) relabeler.HeadStatus {
	return h.head.Status(limit)
}

// Close - relabeler.Head interface implementation.
func (h *RotatableHead) Close() error {
	return h.head.Close()
}

// NewRotatableHead - RotatableHead constructor.
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

// Rotate - relabeler.Head interface implementation.
func (h *RotatableHead) Rotate() error {
	newHead, err := h.builder.Build()
	if err != nil {
		return err
	}

	h.head.Finalize()
	h.storage.Add(h.head)
	h.head = newHead

	return nil
}
