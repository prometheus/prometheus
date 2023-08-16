package common

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/common/internal"
)

// Decoder -
type Decoder struct {
	decoder internal.CDecoder
}

// NewDecoder - init new Decoder.
func NewDecoder() (*Decoder, error) {
	cerr := internal.NewGoErrorInfo()

	return &Decoder{
		decoder: internal.CDecoderCtor(cerr),
	}, cerr.GetError()
}

// Decode - decodes incoming encoding data and return protobuf.
func (d *Decoder) Decode(ctx context.Context, segment []byte) (DecodedSegment, uint32, error) {
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}

	cerr := internal.NewGoErrorInfo()

	result := internal.NewGoDecodedSegment()
	segmentID := internal.CDecoderDecode(d.decoder, segment, result, cerr)

	return result, segmentID, cerr.GetError()
}

// DecodeDry - decode incoming encoding data, restores decoder.
func (d *Decoder) DecodeDry(ctx context.Context, segment []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	cerr := internal.NewGoErrorInfo()

	segmentID := internal.CDecoderDecodeDry(
		d.decoder,
		segment,
		cerr,
	)

	// TODO  return segmentID
	_ = segmentID

	return cerr.GetError()
}

// Snapshot - decodes incoming snapshot, restore decoder.
func (d *Decoder) Snapshot(ctx context.Context, snapshot []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	cerr := internal.NewGoErrorInfo()

	internal.CDecoderDecodeSnapshot(d.decoder, snapshot, cerr)

	return cerr.GetError()
}

// Destroy - destructor for C-Decoder.
func (d *Decoder) Destroy() {
	internal.CDecoderDtor(d.decoder)
}
