package frames

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

// SegmentInfoSize = sum 2(ShardID=uint16)+4(SegmentID=uint32)
const SegmentInfoSize int = 6

// BinaryBody - unsent segment for save refill.
type BinaryBody interface {
	Bytes() []byte
	GetSegmentID() uint32
	GetShardID() uint16
	GetBody() *BinaryBodyV1
	GetSegmentInfo() *SegmentInfo
	Size() int64
	WriteTo(w io.Writer) (int64, error)
}

// ReadFrameSegment - read frame from position pos and return BinaryBody.
func ReadFrameSegment(ctx context.Context, r io.Reader) (BinaryBody, error) {
	h, err := ReadHeader(ctx, r)
	if err != nil {
		return nil, err
	}

	if h.typeFrame != SegmentType {
		return nil, fmt.Errorf("expected %d instead of %d: %w", SegmentType, h.typeFrame, ErrFrameTypeNotMatch)
	}

	var bb BinaryBody
	switch h.GetContentVersion() {
	case ContentVersion1:
		bbv1, errv1 := ReadBinaryBodyV1(ctx, r, int(h.GetSize()))
		if errv1 != nil {
			return nil, errv1
		}
		bb = NewBinaryBodyV2(bbv1.Bytes(), h.GetShardID(), h.GetSegmentID())
	case ContentVersion2:
		bb, err = ReadBinaryBodyV2(ctx, r, int(h.GetSize()))
		if err != nil {
			return nil, err
		}
	}

	return bb, nil
}

// ReadBinaryBodyV1 - read body to BinaryBody with Reader.
func ReadBinaryBodyV1(ctx context.Context, r io.Reader, size int) (*BinaryBodyV1, error) {
	bb := NewBinaryBodyV1Empty()
	if err := bb.ReadFrom(ctx, r, size); err != nil {
		return nil, err
	}
	return bb, nil
}

// BinaryBodyV1 - unsent segment for save refill.
type BinaryBodyV1 struct {
	data []byte
}

var _ WritePayload = (*BinaryBodyV1)(nil)

// NewBinaryBodyV1 - init BinaryBody with data segment.
func NewBinaryBodyV1(binaryData []byte) *BinaryBodyV1 {
	return &BinaryBodyV1{data: binaryData}
}

// NewBinaryBodyV1Empty - init empty BinaryBodyV1.
func NewBinaryBodyV1Empty() *BinaryBodyV1 {
	return new(BinaryBodyV1)
}

// Bytes returns data as is
func (sb *BinaryBodyV1) Bytes() []byte {
	return sb.data
}

// ReadFrom - read frame from io.Reader.
func (sb *BinaryBodyV1) ReadFrom(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	sb.data = make([]byte, size)
	if _, err := io.ReadFull(r, sb.data); err != nil {
		return err
	}
	return nil
}

// Size - get size body
func (sb *BinaryBodyV1) Size() int64 {
	return int64(len(sb.data))
}

func (sb *BinaryBodyV1) CRC32() uint32 {
	return crc32.ChecksumIEEE(sb.data)
}

// WriteTo - implements io.WriterTo inerface.
func (sb *BinaryBodyV1) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(sb.data)
	return int64(n), err
}

// BinaryBodyV2 - unsent segment for save refill.
type BinaryBodyV2 struct {
	si   *SegmentInfo
	body *BinaryBodyV1
}

var _ WritePayload = (*BinaryBodyV2)(nil)
var _ BinaryBody = (*BinaryBodyV2)(nil)

// ReadBinaryBodyV2 - read body to BinaryBodyV2 with Reader.
func ReadBinaryBodyV2(ctx context.Context, r io.Reader, size int) (*BinaryBodyV2, error) {
	bb := NewBinaryBodyV2Empty()
	if err := bb.ReadFrom(ctx, r, size); err != nil {
		return nil, err
	}

	return bb, nil
}

// NewBinaryBodyV2 - init BinaryBodyV2 with data segment.
func NewBinaryBodyV2(binaryData []byte, shardID uint16, segmentID uint32) *BinaryBodyV2 {
	return &BinaryBodyV2{
		si:   NewSegmentInfo(shardID, segmentID),
		body: NewBinaryBodyV1(binaryData),
	}
}

// NewBinaryBodyV2Empty - init empty BinaryBodyV2.
func NewBinaryBodyV2Empty() *BinaryBodyV2 {
	return &BinaryBodyV2{si: NewSegmentInfoEmpty(), body: NewBinaryBodyV1Empty()}
}

// Bytes returns data as is.
func (sb *BinaryBodyV2) Bytes() []byte {
	return sb.body.Bytes()
}

// GetBody - return body.
func (sb *BinaryBodyV2) GetBody() *BinaryBodyV1 {
	return sb.body
}

// GetSegmentID - return segmentID.
func (sb *BinaryBodyV2) GetSegmentID() uint32 {
	return sb.si.segmentID
}

// GetShardID - return shardID.
func (sb *BinaryBodyV2) GetShardID() uint16 {
	return sb.si.shardID
}

// GetSegmentInfo - return SegmentInfo.
func (sb *BinaryBodyV2) GetSegmentInfo() *SegmentInfo {
	return sb.si
}

// ReadFrom - read frame from io.Reader.
func (sb *BinaryBodyV2) ReadFrom(ctx context.Context, r io.Reader, size int) error {
	if err := sb.si.ReadFrom(ctx, r); err != nil {
		return err
	}

	return sb.body.ReadFrom(ctx, r, size-SegmentInfoSize)
}

// Size - get size.
func (sb *BinaryBodyV2) Size() int64 {
	return sb.body.Size() + sb.si.Size()
}

func (sb *BinaryBodyV2) CRC32() uint32 {
	return crc32.ChecksumIEEE(sb.body.data)
}

// WriteTo implements WritePayload.
func (sb *BinaryBodyV2) WriteTo(w io.Writer) (int64, error) {
	n, err := sb.si.WriteTo(w)
	if err != nil {
		return 0, err
	}

	k, err := sb.body.WriteTo(w)
	if err != nil {
		return 0, err
	}
	return n + k, err
}

// SegmentInfo - segment information.
type SegmentInfo struct {
	shardID   uint16
	segmentID uint32
}

// NewSegmentInfo - init SegmentInfo.
func NewSegmentInfo(shardID uint16, segmentID uint32) *SegmentInfo {
	return &SegmentInfo{
		shardID:   shardID,
		segmentID: segmentID,
	}
}

// NewSegmentInfoEmpty - init empty SegmentInfo.
func NewSegmentInfoEmpty() *SegmentInfo {
	return new(SegmentInfo)
}

// GetSegmentID - return segmentID.
func (si *SegmentInfo) GetSegmentID() uint32 {
	return si.segmentID
}

// GetShardID - return shardID.
func (si *SegmentInfo) GetShardID() uint16 {
	return si.shardID
}

// Size - get size.
func (*SegmentInfo) Size() int64 {
	return int64(SegmentInfoSize)
}

// WriteTo - implements io.WriterTo inerface.
func (si *SegmentInfo) WriteTo(w io.Writer) (int64, error) {
	buf := make([]byte, SegmentInfoSize)
	var offset int
	// write shardID and move offset
	binary.LittleEndian.PutUint16(buf[offset:offset+sizeOfUint16], si.shardID)
	offset += sizeOfUint16

	// write segmentID
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], si.segmentID)

	n, err := w.Write(buf)
	return int64(n), err
}

// ReadSegmentInfo - read and decode segment information.
func (si *SegmentInfo) ReadSegmentInfo(ctx context.Context, r io.Reader, h *Header) (int, error) {
	var n int
	switch h.GetContentVersion() {
	case ContentVersion1:
		si.shardID = h.GetShardID()
		si.segmentID = h.GetSegmentID()
	case ContentVersion2:
		if err := si.ReadFrom(ctx, r); err != nil {
			return 0, err
		}
		n = SegmentInfoSize
	default:
		return 0, fmt.Errorf("%w: version %d", ErrUnknownSegmentVersion, h.GetContentVersion())
	}

	return n, nil
}

// ReadFrom - read from reader segment info.
func (si *SegmentInfo) ReadFrom(ctx context.Context, r io.Reader) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	data := make([]byte, SegmentInfoSize)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}
	si.unmarshalBinaryInfo(data)
	return nil
}

// UnmarshalBinary - decoding from byte.
// Only for test.
func (si *SegmentInfo) UnmarshalBinary(data []byte) error {
	si.unmarshalBinaryInfo(data)
	return nil
}

// unmarshalBinaryInfo - decoding from byte.
func (si *SegmentInfo) unmarshalBinaryInfo(data []byte) int {
	var offset int
	si.shardID = binary.LittleEndian.Uint16(data[offset:sizeOfUint16])
	offset += sizeOfUint16

	si.segmentID = binary.LittleEndian.Uint32(data[offset : offset+sizeOfUint32])
	offset += sizeOfUint32

	return offset
}

// BinaryWrapper - wrap segment from memory for send.
type BinaryWrapper struct {
	SegmentInfo
	WritePayload
}

// NewBinaryWrapper - init new BinaryWrapper.
func NewBinaryWrapper(shardID uint16, segmentID uint32, p WritePayload) *BinaryWrapper {
	return &BinaryWrapper{
		SegmentInfo: SegmentInfo{
			shardID:   shardID,
			segmentID: segmentID,
		},
		WritePayload: p,
	}
}

// WriteTo - implements io.WriterTo inerface.
func (bw *BinaryWrapper) WriteTo(w io.Writer) (int64, error) {
	n, err := bw.SegmentInfo.WriteTo(w)
	if err != nil {
		return 0, err
	}

	k, err := bw.WritePayload.WriteTo(w)
	if err != nil {
		return 0, err
	}

	return n + k, nil
}

// Size - get size body.
func (bw *BinaryWrapper) Size() int64 {
	return bw.WritePayload.Size() + int64(SegmentInfoSize)
}
