package cppbridge

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/model"
)

// ShardedData - array of structures with (*LabelSet, timestamp, value, LSHash)
type ShardedData interface {
	Cluster() string
	Replica() string

	RangeMetadata(f func(metadata WALScraperHashdexMetadata) bool)
}

// WALProtobufHashdex - Presharding data, GO wrapper for WALProtobufHashdex, init from GO and filling from C/C++.
type WALProtobufHashdex struct {
	hashdex uintptr
	cluster string
	replica string
}

// WALHashdexLimits - memory limits for Hashdex.
type WALHashdexLimits struct {
	MaxLabelNameLength         uint32 `validate:"required"`
	MaxLabelValueLength        uint32 `validate:"required"`
	MaxLabelNamesPerTimeseries uint32 `validate:"required"`
	MaxTimeseriesCount         uint64
}

const (
	defaultMaxLabelNameLength         = 4096
	defaultMaxLabelValueLength        = 65536
	defaultMaxLabelNamesPerTimeseries = 320
)

// DefaultWALHashdexLimits - Default memory limits for Hashdex.
func DefaultWALHashdexLimits() WALHashdexLimits {
	return WALHashdexLimits{
		MaxLabelNameLength:         defaultMaxLabelNameLength,
		MaxLabelValueLength:        defaultMaxLabelValueLength,
		MaxLabelNamesPerTimeseries: defaultMaxLabelNamesPerTimeseries,
		MaxTimeseriesCount:         0,
	}
}

// MarshalBinary - encoding to byte.
func (l *WALHashdexLimits) MarshalBinary() ([]byte, error) {
	//revive:disable-next-line:add-constant sum 2+3+2+4
	buf := make([]byte, 0, 11)

	buf = binary.AppendUvarint(buf, uint64(l.MaxLabelNameLength))
	buf = binary.AppendUvarint(buf, uint64(l.MaxLabelValueLength))
	buf = binary.AppendUvarint(buf, uint64(l.MaxLabelNamesPerTimeseries))
	buf = binary.AppendUvarint(buf, l.MaxTimeseriesCount)
	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (l *WALHashdexLimits) UnmarshalBinary(data []byte) error {
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

	maxTimeseriesCount, _ := binary.Uvarint(data[offset:])
	l.MaxTimeseriesCount = maxTimeseriesCount

	return nil
}

// NewWALSnappyProtobufHashdex init new WALProtobufHashdex with limits from compressed via snappy protobuf.
func NewWALSnappyProtobufHashdex(compressedProtobuf []byte, limits WALHashdexLimits) (ShardedData, error) {
	// cluster and replica - in memory GO(protoData)
	h := &WALProtobufHashdex{
		hashdex: walProtobufHashdexCtor(limits),
	}
	runtime.SetFinalizer(h, func(h *WALProtobufHashdex) {
		walHashdexDtor(h.hashdex)
	})
	var exception []byte
	h.cluster, h.replica, exception = walProtobufHashdexSnappyPresharding(h.hashdex, compressedProtobuf)
	return h, handleException(exception)
}

// cptr - pointer to underlying c++ object.
func (h *WALProtobufHashdex) cptr() uintptr {
	return h.hashdex
}

// Cluster - get Cluster name.
func (h *WALProtobufHashdex) Cluster() string {
	return strings.Clone(h.cluster)
}

// Replica - get Replica name.
func (h *WALProtobufHashdex) Replica() string {
	return strings.Clone(h.replica)
}

// RangeMetadata calls f sequentially for each metadata present in the hashdex.
// If f returns false, range stops the iteration.
func (h *WALProtobufHashdex) RangeMetadata(f func(metadata WALScraperHashdexMetadata) bool) {
	mds := walProtobufHashdexGetMetadata(h.hashdex)

	for i := range mds {
		if !f(mds[i]) {
			break
		}
	}

	freeBytes(*(*[]byte)(unsafe.Pointer(&mds)))
}

// WALGoModelHashdex - Go wrapper for PromPP::WAL::GoModelHashdex..
type WALGoModelHashdex struct {
	hashdex uintptr
	data    []model.TimeSeries
	cluster string
	replica string
}

func (h *WALGoModelHashdex) RangeMetadata(f func(metadata WALScraperHashdexMetadata) bool) {
	//TODO implement me
	panic("implement me")
}

// cptr - pointer to underlying c++ object.
func (h *WALGoModelHashdex) cptr() uintptr {
	return h.hashdex
}

// Cluster - get Cluster name.
func (h *WALGoModelHashdex) Cluster() string {
	return strings.Clone(h.cluster)
}

// Replica - get Replica name.
func (h *WALGoModelHashdex) Replica() string {
	return strings.Clone(h.replica)
}

// NewWALGoModelHashdex - init new GoModelHashdex with limits.
func NewWALGoModelHashdex(limits WALHashdexLimits, data []model.TimeSeries) (ShardedData, error) {
	h := &WALGoModelHashdex{
		hashdex: walGoModelHashdexCtor(limits),
		data:    data,
	}
	runtime.SetFinalizer(h, func(h *WALGoModelHashdex) {
		runtime.KeepAlive(h.data)
		walHashdexDtor(h.hashdex)
	})
	var exception []byte
	h.cluster, h.replica, exception = walGoModelHashdexPresharding(h.hashdex, data)
	return h, handleException(exception)
}

// HashdexFactory - hashdex factory.
type HashdexFactory struct{}

// SnappyProtobuf - constructs compressed vis snappy Prometheus Remote Write based hashdex.
func (HashdexFactory) SnappyProtobuf(data []byte, limits WALHashdexLimits) (ShardedData, error) {
	return NewWALSnappyProtobufHashdex(data, limits)
}

// GoModel - constructs model.TimeSeries based hashdex.
func (HashdexFactory) GoModel(data []model.TimeSeries, limits WALHashdexLimits) (ShardedData, error) {
	return NewWALGoModelHashdex(limits, data)
}

// MetaInjection metedata for injection metrics.
type MetaInjection struct {
	Now       int64
	SentAt    int64
	AgentUUID string
	Hostname  string
}

// WALBasicDecoderHashdex Go wrapper for PromPP::WAL::WALBasicDecoderHashdex.
type WALBasicDecoderHashdex struct {
	hashdex  uintptr
	metadata *MetaInjection
	cluster  string
	replica  string
}

func (h *WALBasicDecoderHashdex) RangeMetadata(f func(metadata WALScraperHashdexMetadata) bool) {
	//TODO implement me
	panic("implement me")
}

// NewWALBasicDecoderHashdex init new WALBasicDecoderHashdex with c-pointer PromPP::WAL::WALBasicDecoderHashdex.
func NewWALBasicDecoderHashdex(hashdex uintptr, meta *MetaInjection, cluster, replica string) *WALBasicDecoderHashdex {
	h := &WALBasicDecoderHashdex{
		hashdex:  hashdex,
		metadata: meta,
		cluster:  cluster,
		replica:  replica,
	}
	runtime.SetFinalizer(h, func(h *WALBasicDecoderHashdex) {
		runtime.KeepAlive(h.metadata)
		if h.hashdex == 0 {
			return
		}
		walHashdexDtor(h.hashdex)
	})
	return h
}

// Cluster get Cluster name.
func (h *WALBasicDecoderHashdex) Cluster() string {
	return strings.Clone(h.cluster)
}

// Replica get Replica name.
func (h *WALBasicDecoderHashdex) Replica() string {
	return strings.Clone(h.replica)
}

// cptr pointer to underlying c++ object.
func (h *WALBasicDecoderHashdex) cptr() uintptr {
	return h.hashdex
}

const (
	// Error codes from parsing.
	scraperParseNoError uint32 = iota
	scraperParseUnexpectedToken
	scraperParseNoMetricName
	scraperInvalidUtf8
	scraperParseInvalidValue
	scraperParseInvalidTimestamp
)

var (
	// ErrScraperParseUnexpectedToken error when parse unexpected token.
	ErrScraperParseUnexpectedToken = errors.New("scraper parse unexpected token")
	// ErrScraperParseNoMetricName error when parse no metric name.
	ErrScraperParseNoMetricName = errors.New("scraper parse no metric name")
	// ErrScraperInvalidUtf8 error when parse invalid utf8.
	ErrScraperInvalidUtf8 = errors.New("scraper parse invalid utf8")
	// ErrScraperParseInvalidValue error when parse invalid value.
	ErrScraperParseInvalidValue = errors.New("scraper parse invalid value")
	// ErrScraperParseInvalidTimestamp error when parse invalid timestamp.
	ErrScraperParseInvalidTimestamp = errors.New("scraper parse invalid timestamp")

	codeToError = map[uint32]error{
		scraperParseNoError:          nil,
		scraperParseUnexpectedToken:  ErrScraperParseUnexpectedToken,
		scraperParseNoMetricName:     ErrScraperParseNoMetricName,
		scraperInvalidUtf8:           ErrScraperInvalidUtf8,
		scraperParseInvalidValue:     ErrScraperParseInvalidValue,
		scraperParseInvalidTimestamp: ErrScraperParseInvalidTimestamp,
	}
)

func errorFromCode(code uint32) error {
	if code == scraperParseNoError {
		return nil
	}

	if err, ok := codeToError[code]; ok {
		return err
	}

	return fmt.Errorf("scraper parse unknown code error: %d", code)
}

const (
	// HashdexMetadataHelp type of metadata "Help" from hashdex metadata.
	HashdexMetadataHelp uint32 = iota
	// HashdexMetadataType type of metadata "Type" from hashdex metadata.
	HashdexMetadataType
	// HashdexMetadataUnit type of metadata "Unit" from hashdex metadata.
	HashdexMetadataUnit
)

// WALScraperHashdexMetadata metadata from hashdex.
type WALScraperHashdexMetadata struct {
	MetricName string
	Text       string
	Type       uint32
}

// WALPrometheusScraperHashdex hashdex for sraped incoming data.
type WALPrometheusScraperHashdex struct {
	hashdex uintptr
	buffer  []byte
}

var _ ShardedData = (*WALPrometheusScraperHashdex)(nil)

// NewPrometheusScraperHashdex init new *WALScraperHashdex.
func NewPrometheusScraperHashdex() *WALPrometheusScraperHashdex {
	h := &WALPrometheusScraperHashdex{
		hashdex: walPrometheusScraperHashdexCtor(),
		buffer:  nil,
	}
	runtime.SetFinalizer(h, func(h *WALPrometheusScraperHashdex) {
		walHashdexDtor(h.hashdex)
	})
	return h
}

// Parse parsing incoming slice byte with default timestamp to hashdex.
func (h *WALPrometheusScraperHashdex) Parse(buffer []byte, default_timestamp int64) (uint32, error) {
	h.buffer = buffer
	scraped, errorCode := walPrometheusScraperHashdexParse(h.hashdex, h.buffer, default_timestamp)
	return scraped, errorFromCode(errorCode)
}

// RangeMetadata calls f sequentially for each metadata present in the hashdex.
// If f returns false, range stops the iteration.
func (h *WALPrometheusScraperHashdex) RangeMetadata(f func(metadata WALScraperHashdexMetadata) bool) {
	mds := walPrometheusScraperHashdexGetMetadata(h.hashdex)

	for i := range mds {
		if !f(mds[i]) {
			break
		}
	}

	freeBytes(*(*[]byte)(unsafe.Pointer(&mds)))
}

// Cluster get Cluster name.
func (*WALPrometheusScraperHashdex) Cluster() string {
	return ""
}

// Replica get Replica name.
func (*WALPrometheusScraperHashdex) Replica() string {
	return ""
}

// cptr pointer to underlying c++ object.
func (h *WALPrometheusScraperHashdex) cptr() uintptr {
	return h.hashdex
}

// WALOpenMetricsScraperHashdex hashdex for sraped incoming data.
type WALOpenMetricsScraperHashdex struct {
	hashdex uintptr
	buffer  []byte
}

var _ ShardedData = (*WALOpenMetricsScraperHashdex)(nil)

// NewOpenMetricsScraperHashdex init new *WALScraperHashdex.
func NewOpenMetricsScraperHashdex() *WALOpenMetricsScraperHashdex {
	h := &WALOpenMetricsScraperHashdex{
		hashdex: walOpenMetricsScraperHashdexCtor(),
		buffer:  nil,
	}
	runtime.SetFinalizer(h, func(h *WALOpenMetricsScraperHashdex) {
		walHashdexDtor(h.hashdex)
	})
	return h
}

// Parse parsing incoming slice byte with default timestamp to hashdex.
func (h *WALOpenMetricsScraperHashdex) Parse(buffer []byte, default_timestamp int64) (uint32, error) {
	h.buffer = buffer
	scraped, errorCode := walOpenMetricsScraperHashdexParse(h.hashdex, h.buffer, default_timestamp)
	return scraped, errorFromCode(errorCode)
}

// RangeMetadata calls f sequentially for each metadata present in the hashdex.
// If f returns false, range stops the iteration.
func (h *WALOpenMetricsScraperHashdex) RangeMetadata(f func(metadata WALScraperHashdexMetadata) bool) {
	mds := walOpenMetricsScraperHashdexGetMetadata(h.hashdex)

	for i := range mds {
		if !f(mds[i]) {
			break
		}
	}

	freeBytes(*(*[]byte)(unsafe.Pointer(&mds)))
}

// Cluster get Cluster name.
func (*WALOpenMetricsScraperHashdex) Cluster() string {
	return ""
}

// Replica get Replica name.
func (*WALOpenMetricsScraperHashdex) Replica() string {
	return ""
}

// cptr pointer to underlying c++ object.
func (h *WALOpenMetricsScraperHashdex) cptr() uintptr {
	return h.hashdex
}
