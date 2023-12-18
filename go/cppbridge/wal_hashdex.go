package cppbridge

import (
	"encoding/binary"
	"runtime"
	"strings"
)

// ShardedData - array of structures with (*LabelSet, timestamp, value, LSHash)
type ShardedData interface {
	Cluster() string
	Replica() string
}

// WALHashdex - Presharding data, GO wrapper for WALHashdex, init from GO and filling from C/C++.
type WALHashdex struct {
	hashdex uintptr
	data    []byte
	cluster string
	replica string
}

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

// NewWALHashdex - init new Hashdex with limits.
func NewWALHashdex(protoData []byte, limits HashdexLimits) (ShardedData, error) {
	// cluster and replica - in memory GO(protoData)
	h := &WALHashdex{
		hashdex: walHashdexCtor(limits),
		data:    protoData,
	}
	runtime.SetFinalizer(h, func(h *WALHashdex) {
		runtime.KeepAlive(h.data)
		walHashdexDtor(h.hashdex)
	})
	var exception []byte
	h.cluster, h.replica, exception = walHashdexPresharding(h.hashdex, protoData)
	return h, handleException(exception)
}

// Cluster - get Cluster name.
func (h *WALHashdex) Cluster() string {
	return strings.Clone(h.cluster)
}

// Replica - get Replica name.
func (h *WALHashdex) Replica() string {
	return strings.Clone(h.replica)
}
