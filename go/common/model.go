// package common contains common interfaces for encoders and decoders,
// such as Segment, Redundant, Snapshot, the Go bridged counterparts
// (like GoRedundant, etc.).
package common

import (
	"encoding/binary"
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
	Samples() uint32
	Series() uint32
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

// HashdexLimits - memory limits for Hashdex.
type HashdexLimits struct {
	MaxLabelNameLength         uint32 `validate:"required"`
	MaxLabelValueLength        uint32 `validate:"required"`
	MaxLabelNamesPerTimeseries uint32 `validate:"required"`
	MaxTimeseriesCount         uint64
	MaxPbSizeInBytes           uint64 `validate:"required"`
}

const (
	defaultMaxLabelNameLength         = 4096
	defaultMaxLabelValueLength        = 65536
	defaultMaxLabelNamesPerTimeseries = 320
	defaultMaxPbSizeInBytes           = 100 << 20
)

// DefaultHashdexLimits - Default memory limits for Hashdex.
func DefaultHashdexLimits() HashdexLimits {
	return HashdexLimits{
		MaxLabelNameLength:         defaultMaxLabelNameLength,
		MaxLabelValueLength:        defaultMaxLabelValueLength,
		MaxLabelNamesPerTimeseries: defaultMaxLabelNamesPerTimeseries,
		MaxTimeseriesCount:         0,
		MaxPbSizeInBytes:           defaultMaxPbSizeInBytes,
	}
}

// MarshalBinary - encoding to byte.
func (l *HashdexLimits) MarshalBinary() ([]byte, error) {
	//revive:disable-next-line:add-constant sum 2+3+2+4
	buf := make([]byte, 0, 11)

	buf = binary.AppendUvarint(buf, uint64(l.MaxLabelNameLength))
	buf = binary.AppendUvarint(buf, uint64(l.MaxLabelValueLength))
	buf = binary.AppendUvarint(buf, uint64(l.MaxLabelNamesPerTimeseries))
	buf = binary.AppendUvarint(buf, l.MaxTimeseriesCount)
	buf = binary.AppendUvarint(buf, l.MaxPbSizeInBytes)
	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (l *HashdexLimits) UnmarshalBinary(data []byte) error {
	var offset int

	maxLabelNameLength, n := binary.Uvarint(data[offset:])
	l.MaxLabelNameLength = uint32(maxLabelNameLength)
	offset += n

	maxLabelValueLength, n := binary.Uvarint(data[offset:])
	l.MaxLabelValueLength = uint32(maxLabelValueLength)
	offset += n

	maxLabelNamesPerTimeseries, n := binary.Uvarint(data[offset:])
	l.MaxLabelNamesPerTimeseries = uint32(maxLabelNamesPerTimeseries)
	offset += n

	maxTimeseriesCount, n := binary.Uvarint(data[offset:])
	l.MaxTimeseriesCount = maxTimeseriesCount
	offset += n

	maxPbSizeInBytes, _ := binary.Uvarint(data[offset:])
	l.MaxPbSizeInBytes = maxPbSizeInBytes

	return nil
}

// NewHashdex - init new Hashdex with limits.
func NewHashdex(protoData []byte, limits HashdexLimits) (ShardedData, error) {
	// cluster and replica - in memory GO(protoData)
	h := &Hashdex{
		hashdex: internal.CHashdexCtor(uintptr(unsafe.Pointer(&limits))),
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
