package cppbridge

import (
	"context"
	"io"
	"runtime"

	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/prometheus/pp/go/frames"
)

// ProtobufStats - stats data for decoded segment to RemoteWrite protobuf.
type ProtobufStats interface {
	// CreatedAt - return timestamp in ns when data was start writed to encoder.
	CreatedAt() int64
	// EncodedAt - return timestamp in ns when segment was encoded.
	EncodedAt() int64
	// Samples - return number of samples in segment.
	Samples() uint32
	// SegmentID - return processed segment id.
	SegmentID() uint32
	// Series - return number of series in segment.
	Series() uint32
}

// DecodedSegmentStats - stats data for decoded segment.
type DecodedSegmentStats struct {
	createdAt int64
	encodedAt int64
	samples   uint32
	series    uint32
	segmentID uint32
}

var _ ProtobufStats = (*DecodedSegmentStats)(nil)

// CreatedAt - return timestamp in ns when data was start writed to encoder.
func (s DecodedSegmentStats) CreatedAt() int64 {
	return s.createdAt
}

// EncodedAt - return timestamp in ns when segment was encoded.
func (s DecodedSegmentStats) EncodedAt() int64 {
	return s.encodedAt
}

// Samples - return number of samples in segment.
func (s DecodedSegmentStats) Samples() uint32 {
	return s.samples
}

// SegmentID - return processed segment id.
func (s DecodedSegmentStats) SegmentID() uint32 {
	return s.segmentID
}

// Series - return number of series in segment.
func (s DecodedSegmentStats) Series() uint32 {
	return s.series
}

// ProtobufContent - decoded to RemoteWrite protobuf segment
type ProtobufContent interface {
	frames.WritePayload
	CreatedAt() int64
	EncodedAt() int64
	Samples() uint32
	SegmentID() uint32
	Series() uint32
	UnmarshalTo(proto.Unmarshaler) error
}

// DecodedProtobuf - is GO wrapper for decoded RemoteWrite protobuf content.
type DecodedProtobuf struct {
	buf []byte
	DecodedSegmentStats
}

var _ ProtobufContent = (*DecodedProtobuf)(nil)

// NewDecodedProtobuf - init new DecodedProtobuf.
func NewDecodedProtobuf(b []byte, stats DecodedSegmentStats) *DecodedProtobuf {
	p := &DecodedProtobuf{
		buf:                 b,
		DecodedSegmentStats: stats,
	}
	runtime.SetFinalizer(p, func(p *DecodedProtobuf) {
		freeBytes(p.buf)
	})
	return p
}

// Size - returns len of bytes.
func (p *DecodedProtobuf) Size() int64 {
	return int64(len(p.buf))
}

// WriteTo - implements io.WriterTo interface.
func (p *DecodedProtobuf) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(p.buf)
	runtime.KeepAlive(p)
	return int64(n), err
}

// UnmarshalTo - unmarshals data to given protobuf message.
func (p *DecodedProtobuf) UnmarshalTo(v proto.Unmarshaler) error {
	err := v.Unmarshal(p.buf)
	runtime.KeepAlive(p)
	return err
}

// WALDecoder - go wrapper for C-WALDecoder.
//
//	decoder - pointer to a C++ decoder initiated in C++ memory;
type WALDecoder struct {
	decoder uintptr
}

// NewWALDecoder - init new Decoder.
func NewWALDecoder() *WALDecoder {
	d := &WALDecoder{
		decoder: walDecoderCtor(),
	}
	runtime.SetFinalizer(d, func(d *WALDecoder) {
		walDecoderDtor(d.decoder)
	})
	return d
}

// Decode - decodes incoming encoding data and return protobuf.
func (d *WALDecoder) Decode(ctx context.Context, segment []byte) (ProtobufContent, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	stats, protobuf, exception := walDecoderDecode(d.decoder, segment)
	return NewDecodedProtobuf(protobuf, stats), handleException(exception)
}

// DecodeDry - decode incoming encoding data, restores decoder.
func (d *WALDecoder) DecodeDry(ctx context.Context, segment []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	exception := walDecoderDecodeDry(d.decoder, segment)
	return handleException(exception)
}

// RestoreFromStream - restore from incoming encoding data, restores decoder.
func (d *WALDecoder) RestoreFromStream(
	ctx context.Context,
	buf []byte,
	requiredSegmentID uint32,
) (offset uint64, restoredID uint32, err error) {
	if ctx.Err() != nil {
		return 0, 0, ctx.Err()
	}
	var exception []byte
	offset, restoredID, exception = walDecoderRestoreFromStream(d.decoder, buf, requiredSegmentID)
	return offset, restoredID, handleException(exception)
}
