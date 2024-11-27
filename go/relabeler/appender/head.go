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
	BuildWithConfig(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) (relabeler.Head, error)
}

// RotatableHead - head wrapper, allows rotations.
type RotatableHead struct {
	head    relabeler.Head
	storage Storage
	builder HeadBuilder
}

// ID - relabeler.Head interface implementation.
func (h *RotatableHead) ID() string {
	return h.head.ID()
}

// Generation - relabeler.Head interface implementation.
func (h *RotatableHead) Generation() uint64 {
	return h.head.Generation()
}

// Append - relabeler.Head interface implementation.
func (h *RotatableHead) Append(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	state *cppbridge.State,
	relabelerID string,
) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error) {
	return h.head.Append(ctx, incomingData, state, relabelerID)
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
	if h.head.NumberOfShards() != numberOfShards {
		return h.RotateWithConfig(inputRelabelerConfigs, numberOfShards)
	}
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
func NewRotatableHead(head relabeler.Head, storage Storage, builder HeadBuilder) *RotatableHead {
	return &RotatableHead{
		head:    head,
		storage: storage,
		builder: builder,
	}
}

// Rotate - relabeler.Head interface implementation.
func (h *RotatableHead) Rotate() error {
	newHead, err := h.builder.Build()
	if err != nil {
		return err
	}

	h.head.Finalize()
	_ = h.head.Rotate()
	h.storage.Add(h.head)
	h.head = newHead
	return nil
}

func (h *RotatableHead) RotateWithConfig(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error {
	newHead, err := h.builder.BuildWithConfig(inputRelabelerConfigs, numberOfShards)
	if err != nil {
		return err
	}

	h.head.Finalize()
	_ = h.head.Rotate()
	h.storage.Add(h.head)
	h.head = newHead

	return nil
}

func (h *RotatableHead) Discard() error {
	return h.head.Discard()
}

type HeapProfileWriter interface {
	WriteHeapProfile() error
}

type HeapProfileWritableHead struct {
	head              relabeler.Head
	heapProfileWriter HeapProfileWriter
}

func (h *HeapProfileWritableHead) ID() string {
	return h.head.ID()
}

func (h *HeapProfileWritableHead) Generation() uint64 {
	return h.head.Generation()
}

func (h *HeapProfileWritableHead) Append(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	state *cppbridge.State,
	relabelerID string,
) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error) {
	return h.head.Append(ctx, incomingData, state, relabelerID)
}

func (h *HeapProfileWritableHead) ForEachShard(fn relabeler.ShardFn) error {
	return h.head.ForEachShard(fn)
}

func (h *HeapProfileWritableHead) OnShard(shardID uint16, fn relabeler.ShardFn) error {
	return h.head.OnShard(shardID, fn)
}

func (h *HeapProfileWritableHead) NumberOfShards() uint16 {
	return h.head.NumberOfShards()
}

func (h *HeapProfileWritableHead) Finalize() {
	h.head.Finalize()
}

func (h *HeapProfileWritableHead) Reconfigure(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error {
	return h.head.Reconfigure(inputRelabelerConfigs, numberOfShards)
}

func (h *HeapProfileWritableHead) WriteMetrics() {
	h.head.WriteMetrics()
}

func (h *HeapProfileWritableHead) Status(limit int) relabeler.HeadStatus {
	return h.head.Status(limit)
}

func (h *HeapProfileWritableHead) Rotate() error {
	if err := h.head.Rotate(); err != nil {
		return err
	}

	return h.heapProfileWriter.WriteHeapProfile()
}

func (h *HeapProfileWritableHead) Close() error {
	return h.head.Close()
}

func (h *HeapProfileWritableHead) Discard() error {
	return h.head.Discard()
}

func NewHeapProfileWritableHead(head relabeler.Head, heapProfileWriter HeapProfileWriter) *HeapProfileWritableHead {
	return &HeapProfileWritableHead{head: head, heapProfileWriter: heapProfileWriter}
}
