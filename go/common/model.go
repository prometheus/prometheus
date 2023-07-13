// package common contains common interfaces for encoders and decoders,
// such as Segment, Redundant, Snapshot, the Go bridged counterparts
// (like GoRedundant, etc.).
package common

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/common/internal"
)

// ShardedData - array of structures with (*LabelSet, timestamp, value, LSHash)
type ShardedData interface {
	Cluster() string
	Replica() string
	Destroy()
}

// Segment - encoded data segment
type Segment interface {
	Bytes() []byte
	Samples() uint32
	Series() uint32
	Earliest() int64
	Latest() int64
	Destroy()
}

// DecodedSegment - decoded to RemoteWrite protobuf segment
type DecodedSegment interface {
	Bytes() []byte
	CreatedAt() int64
	EncodedAt() int64
	Destroy()
}

// Redundant - information to create a Snapshot
type Redundant interface {
	PointerData() unsafe.Pointer
	Destroy()
}

// Snapshot - snapshot of encoder/decoder
type Snapshot interface {
	Bytes() []byte
	Destroy()
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

// NewHashdex - init new Hashdex.
func NewHashdex(protoData []byte) ShardedData {
	// cluster and replica - in memory GO(protoData)
	h := &Hashdex{
		hashdex: internal.CHashdexCtor(),
		data:    protoData,
		cluster: &internal.CSlice{},
		replica: &internal.CSlice{},
	}
	internal.CHashdexPresharding(h.hashdex, h.data, h.cluster, h.replica)
	runtime.KeepAlive(h.data)
	return h
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

// Destroy - clear memory in C/C++.
func (h *Hashdex) Destroy() {
	// so that the Garbage Collector does not clear the memory
	// associated with the slice, we hold it through KeepAlive
	runtime.KeepAlive(h.data)
	internal.CHashdexDtor(h.hashdex)
}
