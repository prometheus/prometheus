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
func NewDecoder() *Decoder {
	return &Decoder{
		decoder: internal.CDecoderCtor(),
	}
}

// Decode - decodes incoming encoding data and return protobuf.
func (d *Decoder) Decode(ctx context.Context, segment []byte) (DecodedSegment, uint32, error) {
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}

	result := internal.NewGoDecodedSegment()

	segmentID := internal.CDecoderDecode(d.decoder, segment, result)

	return result, segmentID, nil
}

// DecodeDry - decode incoming encoding data, restores decoder.
func (d *Decoder) DecodeDry(ctx context.Context, segment []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	segmentID := internal.CDecoderDecodeDry(
		d.decoder,
		segment,
	)

	// TODO  return segmentID
	_ = segmentID

	return nil
}

// Snapshot - decodes incoming snapshot, restore decoder.
func (d *Decoder) Snapshot(ctx context.Context, snapshot []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	internal.CDecoderDecodeSnapshot(d.decoder, snapshot)

	return nil
}

// Destroy - destructor for C-Decoder.
func (d *Decoder) Destroy() {
	internal.CDecoderDtor(d.decoder)
}
