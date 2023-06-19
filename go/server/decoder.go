package server

import (
	"context"
)

// Decoder -
type Decoder struct {
	decoder cDecoder
}

// NewDecoder - init new Decoder.
func NewDecoder() *Decoder {
	return &Decoder{
		decoder: cDecoderCtor(),
	}
}

// Decode - decode income encoding data and return protobuf.
func (d *Decoder) Decode(ctx context.Context, segment []byte) (*GoSliceByte, uint32, error) {
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}

	cprotobuf := NewGoSliceByte()

	segmentID := cDecoderDecode(d.decoder, segment, cprotobuf)

	return cprotobuf, segmentID, nil
}

// DecodeDry - decode income encoding data, restore decoder.
func (d *Decoder) DecodeDry(ctx context.Context, segment []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	segmentID := cDecoderDecodeDry(
		d.decoder,
		segment,
	)

	// TODO  return segmentID
	_ = segmentID

	return nil
}

// Snapshot - decode income snapshot, restore decoder.
func (d *Decoder) Snapshot(ctx context.Context, snapshot []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	cDecoderDecodeSnapshot(d.decoder, snapshot)

	return nil
}

// Destroy - distructor for C-Decoder.
func (d *Decoder) Destroy() {
	cDecoderDtor(d.decoder)
}
