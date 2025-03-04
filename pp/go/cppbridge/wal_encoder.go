package cppbridge

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"runtime"

	"github.com/prometheus/prometheus/pp/go/frames"
)

// ErrMustImplementCptrable - error on sharded data must implement cptrable interface.
var ErrMustImplementCptrable = errors.New("sharded data must implement cptrable interface")

// SegmentKey is a key to store segment data in Exchange and Refill
type SegmentKey struct {
	ShardID uint16
	Segment uint32
}

// IsFirst returns true if it is a first segment in shard
func (key SegmentKey) IsFirst() bool {
	return key.Segment == 0
}

// Prev returns key to previous segment in the same shard
func (key SegmentKey) Prev() SegmentKey {
	return SegmentKey{
		ShardID: key.ShardID,
		Segment: key.Segment - 1,
	}
}

// String implements fmt.Stringer interface
func (key SegmentKey) String() string {
	return fmt.Sprintf("%d:%d", key.ShardID, key.Segment)
}

// SegmentStats - stats data for encoded segment.
type SegmentStats interface {
	// AllocatedMemory - returns size of allocated memory label set.
	AllocatedMemory() uint64
	// EarliestTimestamp - returns timestamp in ms of earliest sample in segment.
	EarliestTimestamp() int64
	// LatestTimestamp - returns timestamp in ms of latest sample in segment.
	LatestTimestamp() int64
	// RemainingTableSize - remaining table size in encoders.
	RemainingTableSize() uint32
	// Samples - returns count of samples in segment.
	Samples() uint32
	// Series - returns count of series in segment.
	Series() uint32
}

// WALEncoderStats - stats data for encoded segment.
type WALEncoderStats struct {
	earliestTimestamp int64
	latestTimestamp   int64
	allocatedMemory   uint64
	samples           uint32
	series            uint32
	remainderSize     uint32
}

var _ SegmentStats = (*WALEncoderStats)(nil)

// AllocatedMemory - returns size of allocated memory label set.
func (s WALEncoderStats) AllocatedMemory() uint64 {
	return s.allocatedMemory
}

// EarliestTimestamp - returns timestamp in ms of earliest sample in segment.
func (s WALEncoderStats) EarliestTimestamp() int64 {
	return s.earliestTimestamp
}

// LatestTimestamp - returns timestamp in ms of latest sample in segment.
func (s WALEncoderStats) LatestTimestamp() int64 {
	return s.latestTimestamp
}

// RemainingTableSize - remaining table size in encoders.
func (s WALEncoderStats) RemainingTableSize() uint32 {
	return s.remainderSize
}

// Samples - returns count of samples in segment.
func (s WALEncoderStats) Samples() uint32 {
	return s.samples
}

// Series - returns count of series in segment.
func (s WALEncoderStats) Series() uint32 {
	return s.series
}

// Segment - encoded data segment
type Segment interface {
	// WritePayload - is a payload to write in frame.
	frames.WritePayload
	// SegmentStats - stats data for encoded segment.
	SegmentStats
}

// EncodedSegment - is GO wrapper for Segment.
type EncodedSegment struct {
	buf []byte
	WALEncoderStats
}

var _ Segment = (*EncodedSegment)(nil)

// NewEncodedSegment - init new EncodedSegment.
func NewEncodedSegment(b []byte, stats WALEncoderStats) *EncodedSegment {
	s := &EncodedSegment{
		buf:             b,
		WALEncoderStats: stats,
	}
	runtime.SetFinalizer(s, func(s *EncodedSegment) {
		freeBytes(s.buf)
	})
	return s
}

// Size - returns len of bytes.
func (s *EncodedSegment) Size() int64 {
	return int64(len(s.buf))
}

// CRC32 - hash.
func (s *EncodedSegment) CRC32() uint32 {
	return crc32.ChecksumIEEE(s.buf)
}

// WriteTo - implements io.WriterTo inerface.
func (s *EncodedSegment) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(s.buf)
	return int64(n), err
}

// SourceState - pointer to source state (null on first call)
type SourceState struct {
	pointer uintptr
}

// EncodersVersion - return current encoder version.
func EncodersVersion() uint8 {
	return walEncodersVersion()
}

// WALEncoder - go wrapper for C-WALEncoder.
//
//	encoder - pointer to a C++ encoder initiated in C++ memory;
//	lastEncodedSegment - last encoded segment id;
//	shardID - current encoder shard id;
type WALEncoder struct {
	encoder            uintptr
	lastEncodedSegment uint32
	shardID            uint16
}

// NewWALEncoder - init new Encoder.
func NewWALEncoder(shardID uint16, logShards uint8) *WALEncoder {
	e := &WALEncoder{
		encoder:            walEncoderCtor(shardID, logShards),
		shardID:            shardID,
		lastEncodedSegment: math.MaxUint32,
	}
	runtime.SetFinalizer(e, func(e *WALEncoder) {
		walEncoderDtor(e.encoder)
	})
	return e
}

type cptrable interface {
	cptr() uintptr
}

// Add - add to encode incoming data(ShardedData) through C++ encoder.
func (e *WALEncoder) Add(ctx context.Context, shardedData ShardedData) (SegmentStats, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	cptrContainer, ok := shardedData.(cptrable)
	if !ok {
		return nil, ErrMustImplementCptrable
	}

	// shardedData.hashdex - Hashdex, struct(init from GO), filling in C/C++
	stats, exception := walEncoderAdd(e.encoder, cptrContainer.cptr())
	return &stats, handleException(exception)
}

// AddInnerSeries - add to encode incoming data(relabeling and cached) through C++ encoder.
func (e *WALEncoder) AddInnerSeries(ctx context.Context, innerSeries []*InnerSeries) (SegmentStats, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	stats, exception := walEncoderAddInnerSeries(e.encoder, innerSeries)
	return &stats, handleException(exception)
}

// AddRelabeledSeries - add to encode incoming data(relabeling and not cached) through C++ encoder.
func (e *WALEncoder) AddRelabeledSeries(
	ctx context.Context,
	relabeledSeries *RelabeledSeries,
	relabelerStateUpdate *RelabelerStateUpdate,
) (SegmentStats, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	stats, exception := walEncoderAddRelabeledSeries(e.encoder, relabeledSeries, relabelerStateUpdate)
	return &stats, handleException(exception)
}

// Finalize - finalize the encoded data in the C++ encoder to Segment.
func (e *WALEncoder) Finalize(ctx context.Context) (SegmentKey, Segment, error) {
	if ctx.Err() != nil {
		return SegmentKey{}, nil, ctx.Err()
	}

	// transfer go-slice in C/C++
	stats, segment, exception := walEncoderFinalize(e.encoder)
	e.lastEncodedSegment++
	segKey := SegmentKey{
		ShardID: e.shardID,
		Segment: e.lastEncodedSegment,
	}

	return segKey, NewEncodedSegment(segment, stats), handleException(exception)
}

// Encode - encode income data(ShardedData) through C++ encoder to Segment.
func (e *WALEncoder) Encode(ctx context.Context, shardedData ShardedData) (SegmentKey, Segment, error) {
	if _, err := e.Add(ctx, shardedData); err != nil {
		return SegmentKey{}, nil, err
	}

	return e.Finalize(ctx)
}

// AddWithStaleNans - add to encode incoming data(ShardedData) to current segment
// and mark as stale obsolete series through C++ encoder.
func (e *WALEncoder) AddWithStaleNans(
	ctx context.Context,
	shardedData ShardedData,
	sourceState *SourceState,
	staleTS int64,
) (SegmentStats, *SourceState, error) {
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}

	cptrContainer, ok := shardedData.(cptrable)
	if !ok {
		return nil, nil, ErrMustImplementCptrable
	}

	stats, state, exception := walEncoderAddWithStaleNans(
		e.encoder,
		cptrContainer.cptr(),
		sourceState.pointer,
		staleTS,
	)
	return &stats, &SourceState{state}, handleException(exception)
}

// CollectSource - destroy source state and mark all series as stale.
func (e *WALEncoder) CollectSource(ctx context.Context, sourceState *SourceState, staleTS int64) (SegmentStats, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	stats, exception := walEncoderCollectSource(e.encoder, sourceState.pointer, staleTS)
	return &stats, handleException(exception)
}

// LastEncodedSegment - get last encoded segment ID.
func (e *WALEncoder) LastEncodedSegment() uint32 {
	return e.lastEncodedSegment
}

// WALEncoderLightweight - go wrapper for C-WALEncoderLightweight.
//
//	encoder - pointer to a C++ encoder initiated in C++ memory;
//	lastEncodedSegment - last encoded segment id;
//	shardID - current encoder shard id;
type WALEncoderLightweight struct {
	encoder            uintptr
	lastEncodedSegment uint32
	shardID            uint16
}

// NewWALEncoderLightweight - init new WALEncoderLightweight.
func NewWALEncoderLightweight(shardID uint16, logShards uint8) *WALEncoderLightweight {
	e := &WALEncoderLightweight{
		encoder:            walEncoderLightweightCtor(shardID, logShards),
		shardID:            shardID,
		lastEncodedSegment: math.MaxUint32,
	}
	runtime.SetFinalizer(e, func(e *WALEncoderLightweight) {
		walEncoderLightweightDtor(e.encoder)
	})
	return e
}

// Add - add to encode incoming data(ShardedData) through C++ encoder.
func (e *WALEncoderLightweight) Add(ctx context.Context, shardedData ShardedData) (SegmentStats, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	cptrContainer, ok := shardedData.(cptrable)
	if !ok {
		return nil, ErrMustImplementCptrable
	}

	// shardedData.hashdex - Hashdex, struct(init from GO), filling in C/C++
	stats, exception := walEncoderLightweightAdd(e.encoder, cptrContainer.cptr())
	return &stats, handleException(exception)
}

// AddInnerSeries - add to encode incoming data(relabeling and cached) through C++ encoder.
func (e *WALEncoderLightweight) AddInnerSeries(ctx context.Context, innerSeries []*InnerSeries) (SegmentStats, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	stats, exception := walEncoderLightweightAddInnerSeries(e.encoder, innerSeries)
	return &stats, handleException(exception)
}

// AddRelabeledSeries - add to encode incoming data(relabeling and not cached) through C++ encoder.
func (e *WALEncoderLightweight) AddRelabeledSeries(
	ctx context.Context,
	relabeledSeries *RelabeledSeries,
	relabelerStateUpdate *RelabelerStateUpdate,
) (SegmentStats, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	stats, exception := walEncoderLightweightAddRelabeledSeries(e.encoder, relabeledSeries, relabelerStateUpdate)
	return &stats, handleException(exception)
}

// Finalize - finalize the encoded data in the C++ encoder to Segment.
func (e *WALEncoderLightweight) Finalize(ctx context.Context) (SegmentKey, Segment, error) {
	if ctx.Err() != nil {
		return SegmentKey{}, nil, ctx.Err()
	}

	// transfer go-slice in C/C++
	stats, segment, exception := walEncoderLightweightFinalize(e.encoder)
	e.lastEncodedSegment++
	segKey := SegmentKey{
		ShardID: e.shardID,
		Segment: e.lastEncodedSegment,
	}

	return segKey, NewEncodedSegment(segment, stats), handleException(exception)
}

// LastEncodedSegment - get last encoded segment ID.
func (e *WALEncoderLightweight) LastEncodedSegment() uint32 {
	return e.lastEncodedSegment
}
