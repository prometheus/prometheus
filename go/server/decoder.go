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
func (d *Decoder) Decode(_ context.Context, segment []byte) (*GoSliceByte, uint32, error) {
	cprotobuf := NewGoSliceByte()

	segmentID := cDecoderDecode(d.decoder, segment, cprotobuf)

	return cprotobuf, segmentID, nil
}

// DecodeDry - decode income encoding data, restore decoder.
func (d *Decoder) DecodeDry(_ context.Context, segment []byte) error {
	segmentID := cDecoderDecodeDry(d.decoder, segment)

	// TODO  return segmentID
	_ = segmentID

	return nil
}

// Snapshot - decode income snapshot, restore decoder.
func (d *Decoder) Snapshot(_ context.Context, snapshot []byte) error {
	cDecoderDecodeSnapshot(d.decoder, snapshot)

	return nil
}

// Destroy - distructor for C-Decoder.
func (d *Decoder) Destroy() {
	cDecoderDtor(d.decoder)
}
