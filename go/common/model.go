// package common contains common interfaces for encoders and decoders,
// such as Segment, Redundant, Snapshot, the Go bridged counterparts
// (like GoRedundant, etc.).
package common

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/prometheus/pp/go/common/internal"
	"github.com/prometheus/prometheus/pp/go/frames"
)

// ShardedData - array of structures with (*LabelSet, timestamp, value, LSHash)
type ShardedData interface {
	Cluster() string
	Replica() string
}

// Segment - encoded data segment
type Segment interface {
	frames.WritePayload
	Samples() uint32
	Series() uint32
	Earliest() int64
	Latest() int64
	RemainingTableSize() uint32
}

// DecodedSegment - decoded to RemoteWrite protobuf segment
type DecodedSegment interface {
	frames.WritePayload
	CreatedAt() int64
	EncodedAt() int64
	UnmarshalTo(proto.Unmarshaler) error
}

// Redundant - information to create a Snapshot
type Redundant interface {
	PointerData() unsafe.Pointer
}

// Snapshot - snapshot of encoder/decoder
type Snapshot interface {
	frames.WritePayload
}

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

// Hashdex - Presharding data, GO wrapper for Hashdex, init from GO and filling from C/C++.
type Hashdex struct {
	hashdex internal.CHashdex
	data    []byte
	cluster *internal.CSlice
	replica *internal.CSlice
}

//
// Hashdex API.
//

type HashdexMemoryLimits struct {
	MaxLabelNameLength         uint32
	MaxLabelValueLength        uint32
	MaxLabelNamesPerTimeseries uint32
	MaxTimeseriesCount         uint64
	MaxPbSizeInBytes           uint64
}

// Default memory limits for Hashdex.
func HashdexDefaultLimits() HashdexMemoryLimits {
	return HashdexMemoryLimits{
		MaxLabelNameLength:         4096,
		MaxLabelValueLength:        65536,
		MaxLabelNamesPerTimeseries: 320,
		MaxTimeseriesCount:         0,
		MaxPbSizeInBytes:           100 << 20,
	}
}

// NewHashdex - init new Hashdex.
func NewHashdex(protoData []byte) (ShardedData, error) {
	args := HashdexDefaultLimits()
	return NewHashdexWithLimits(protoData, &args)
}

func NewHashdexWithLimits(protoData []byte, limits *HashdexMemoryLimits) (ShardedData, error) {
	// cluster and replica - in memory GO(protoData)
	h := &Hashdex{
		hashdex: internal.CHashdexCtor(uintptr(unsafe.Pointer(limits))),
		data:    protoData,
		cluster: &internal.CSlice{},
		replica: &internal.CSlice{},
	}
	runtime.SetFinalizer(h, func(h *Hashdex) {
		runtime.KeepAlive(h.data)
		internal.CHashdexDtor(h.hashdex)
	})
	cerr := internal.NewGoErrorInfo()
	internal.CHashdexPresharding(h.hashdex, h.data, h.cluster, h.replica, cerr)
	runtime.KeepAlive(limits)
	return h, cerr.GetError()
}

// Cluster - get Cluster name.
func (h *Hashdex) Cluster() string {
	data := *(*[]byte)((unsafe.Pointer(h.cluster)))
	copyData := make([]byte, len(data))
	copy(copyData, data)
	return string(copyData)
}

// Replica - get Replica name.
func (h *Hashdex) Replica() string {
	data := *(*[]byte)((unsafe.Pointer(h.replica)))
	copyData := make([]byte, len(data))
	copy(copyData, data)
	return string(copyData)
}
