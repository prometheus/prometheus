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

// RestoreFromStream - restore from incoming encoding data, restores decoder.
func (d *Decoder) RestoreFromStream(
	ctx context.Context,
	buf []byte,
	requiredSegmentID uint32,
) (offset uint64, restoredID uint32, err error) {
	if ctx.Err() != nil {
		return 0, 0, ctx.Err()
	}

	cerr := internal.NewGoErrorInfo()
	result := internal.NewGoRestoredResult(requiredSegmentID)
	internal.CDecoderRestoreFromStream(
		d.decoder,
		buf,
		result,
		cerr,
	)

	return result.Offset(), result.RestoredSegmentID(), cerr.GetError()
}

// Destroy - destructor for C-Decoder.
func (d *Decoder) Destroy() {
	internal.CDecoderDtor(d.decoder)
}
