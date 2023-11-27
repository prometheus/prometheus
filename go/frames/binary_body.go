package frames

import (
	"context"
	"fmt"
	"io"
)

// ReadFrameSegment - read frame from position pos and return BinaryBody.
func ReadFrameSegment(ctx context.Context, r io.Reader) (*BinaryBody, error) {
	h, err := ReadHeader(ctx, r)
	if err != nil {
		return nil, err
	}

	if h.typeFrame != SegmentType {
		return nil, fmt.Errorf("expected %d instead of %d: %w", SegmentType, h.typeFrame, ErrFrameTypeNotMatch)
	}

	return ReadBinaryBody(ctx, r, int(h.GetSize()))
}

// ReadBinaryBody - read body to BinaryBody with Reader.
func ReadBinaryBody(ctx context.Context, r io.Reader, size int) (*BinaryBody, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	data := make([]byte, size)
	if _, err := r.Read(data); err != nil {
		return nil, err
	}
	return NewBinaryBody(data), nil
}

// BinaryBody - unsent segment for save refill.
type BinaryBody struct {
	data []byte
}

var _ WritePayload = &BinaryBody{}

// NewBinaryBody - init BinaryBody with data segment.
func NewBinaryBody(binaryData []byte) *BinaryBody {
	return &BinaryBody{data: binaryData}
}

// Size - get size body
func (sb *BinaryBody) Size() int64 {
	return int64(len(sb.data))
}

// WriteTo implements WritePayload
func (sb *BinaryBody) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(sb.data)
	return int64(n), err
}

// Bytes returns data as is
func (sb *BinaryBody) Bytes() []byte {
	return sb.data
}
