package common

import (
	"fmt"
	"math"
	"unsafe" // nolint

	"context"

	"github.com/prometheus/prometheus/pp/go/common/internal"
)

// Encoder -
type Encoder struct {
	encoder            internal.CEncoder
	lastEncodedSegment uint32
	shardID            uint16
}

// NewEncoder - init new Encoder.
func NewEncoder(shardID, numberOfShards uint16) *Encoder {
	return &Encoder{
		encoder:            internal.CEncoderCtor(shardID, numberOfShards),
		shardID:            shardID,
		lastEncodedSegment: math.MaxUint32,
	}
}

// Encode - encode income data(ShardedData) through C++ encoder to Segment.
//
//revive:disable-next-line:function-result-limit all results are essential
func (e *Encoder) Encode(ctx context.Context, shardedData ShardedData) (SegmentKey, Segment, Redundant, error) {
	hashdex, ok := shardedData.(*Hashdex)
	if !ok {
		return SegmentKey{}, nil, nil, fmt.Errorf("shardedData not casting to Hashdex")
	}

	if ctx.Err() != nil {
		return SegmentKey{}, nil, nil, ctx.Err()
	}

	// init memory in GO
	csegment := internal.NewGoSegment()
	credundant := internal.NewGoRedundant()

	// transfer go-slice in C/C++
	// e.encoder - C-Encoder
	// shardedData.hashdex - Hashdex, struct(init from GO), filling in C/C++
	// *(*C.c_slice_with_stream_buffer)(unsafe.Pointer(csegment)) - C-Segment struct(init from GO)
	// (*C.c_redundant)(unsafe.Pointer(credundant)) - C-Redundant struct(init from GO)
	internal.CEncoderEncode(e.encoder, hashdex.hashdex, csegment, credundant)
	e.lastEncodedSegment++
	segKey := SegmentKey{
		ShardID: e.shardID,
		Segment: e.lastEncodedSegment,
	}

	return segKey, csegment, credundant, nil
}

// Snapshot - get Snapshot from C-Encoder.
func (e *Encoder) Snapshot(ctx context.Context, rds []Redundant) (Snapshot, error) {
	gosnapshot := internal.NewGoSnapshot()
	crs := make([]unsafe.Pointer, len(rds))
	for i := range rds {
		crs[i] = rds[i].(*internal.GoRedundant).PointerData()
	}

	if ctx.Err() != nil {
		return gosnapshot, ctx.Err()
	}

	internal.CEncoderSnapshot(e.encoder, crs, gosnapshot)

	return gosnapshot, nil
}

// LastEncodedSegment - get last encoded segment ID.
func (e *Encoder) LastEncodedSegment() uint32 {
	return e.lastEncodedSegment
}

// Destroy - calls destructor for C-Encoder.
func (e *Encoder) Destroy() {
	internal.CEncoderDtor(e.encoder)
	e.encoder = nil
}
