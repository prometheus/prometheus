package frames

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"
)

const (
	// titleV1Size -contant size.
	// sum = 1(logOfNumberOfShards=uint8)+16(blockID=uuid.UUID)
	titleV1Size int = 17
	// titleV2Size -contant size.
	// sum = 1(logOfNumberOfShards=uint8)+16(blockID=uuid.UUID)+1(encodersVersion=uint8)
	titleV2Size int = 18
)

// Title - title body frame with number of shards, block ID, encoders version.
type Title interface {
	// GetBlockID - get block ID.
	GetBlockID() uuid.UUID
	// GetEncodersVersion - return version encoders.
	GetEncodersVersion() uint8
	// GetShardsNumberPower - get number of shards.
	GetShardsNumberPower() uint8
	// Read - read Title with Reader.
	Read(ctx context.Context, r io.Reader, size int) error
	// ReadAt - read Title with ReaderAt.
	ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error
}

// ReadAtTitle - read body to Title with ReaderAt.
func ReadAtTitle(ctx context.Context, r io.ReaderAt, off int64, size int, contentVersion uint8) (Title, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var tb Title
	switch contentVersion {
	case ContentVersion1:
		tb = NewTitleV1Empty()
	case ContentVersion2:
		tb = NewTitleV2Empty()
	default:
		return nil, fmt.Errorf("%w: version %d", ErrUnknownTitleVersion, contentVersion)
	}

	if err := tb.ReadAt(ctx, r, off, size); err != nil {
		return nil, err
	}

	return tb, nil
}

// ReadTitle - read body to Title with Reader.
func ReadTitle(ctx context.Context, r io.Reader, size int, contentVersion uint8) (Title, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var tb Title
	switch contentVersion {
	case ContentVersion1:
		tb = NewTitleV1Empty()
	case ContentVersion2:
		tb = NewTitleV2Empty()
	default:
		return nil, fmt.Errorf("%w: version %d", ErrUnknownTitleVersion, contentVersion)
	}

	if err := tb.Read(ctx, r, size); err != nil {
		return nil, err
	}

	return tb, nil
}

// TitleV1 - title body frame with number of shards and block ID.
type TitleV1 struct {
	shardsNumberPower uint8
	blockID           uuid.UUID
}

var _ Title = (*TitleV1)(nil)

// NewTitleFrameV1 - init new frame.
// Deprecated. Only for test.
func NewTitleFrameV1(snp uint8, blockID uuid.UUID) (*ReadFrame, error) {
	body, err := NewTitleV1(snp, blockID).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(defaultVersion, ContentVersion1, TitleType, body)
}

// NewTitleV1 - init TitleV1.
func NewTitleV1(snp uint8, blockID uuid.UUID) *TitleV1 {
	return &TitleV1{
		shardsNumberPower: snp,
		blockID:           blockID,
	}
}

// NewTitleV1Empty - init TitleV1 for read.
func NewTitleV1Empty() *TitleV1 {
	return new(TitleV1)
}

// GetBlockID - get block ID.
func (tb *TitleV1) GetBlockID() uuid.UUID {
	return tb.blockID
}

// GetEncodersVersion - return version encoders.
func (*TitleV1) GetEncodersVersion() uint8 {
	return ContentVersion1
}

// GetShardsNumberPower - get number of shards.
func (tb *TitleV1) GetShardsNumberPower() uint8 {
	return tb.shardsNumberPower
}

// SizeOf - get size body.
func (*TitleV1) SizeOf() int {
	return titleV1Size
}

// MarshalBinary - encoding to byte.
func (tb *TitleV1) MarshalBinary() ([]byte, error) {
	var offset int
	buf := make([]byte, tb.SizeOf())

	// write numberOfShards and move offset
	buf[0] = tb.shardsNumberPower
	offset += sizeOfUint8

	// write blockID and move offset
	buf = append(buf[:offset], tb.blockID[:]...)

	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (tb *TitleV1) UnmarshalBinary(data []byte) error {
	// read numberOfShards
	var off int
	tb.shardsNumberPower = data[0]
	off += sizeOfUint8

	// read blockID
	return tb.blockID.UnmarshalBinary(data[off:])
}

// ReadAt - read Title with ReaderAt.
func (tb *TitleV1) ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, off); err != nil {
		return err
	}

	return tb.UnmarshalBinary(buf)
}

// Read - read Title with Reader.
func (tb *TitleV1) Read(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	return tb.UnmarshalBinary(buf)
}

// TitleV2 - title body frame with number of shards, block ID, encoders version.
type TitleV2 struct {
	shardsNumberPower uint8
	encodersVersion   uint8
	blockID           uuid.UUID
}

var _ Title = (*TitleV2)(nil)

// NewTitleFrameV2 - init new frame.
func NewTitleFrameV2(snp, encodersVersion uint8, blockID uuid.UUID) (*ReadFrame, error) {
	body, err := NewTitleV2(snp, encodersVersion, blockID).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(defaultVersion, ContentVersion2, TitleType, body)
}

// NewTitleV2 - init TitleV2.
func NewTitleV2(snp, encodersVersion uint8, blockID uuid.UUID) *TitleV2 {
	return &TitleV2{
		shardsNumberPower: snp,
		blockID:           blockID,
		encodersVersion:   encodersVersion,
	}
}

// NewTitleV2Empty - init TitleV2 for read.
func NewTitleV2Empty() *TitleV2 {
	return new(TitleV2)
}

// GetBlockID - get block ID.
func (tb *TitleV2) GetBlockID() uuid.UUID {
	return tb.blockID
}

// GetEncodersVersion - return version encoders.
func (tb *TitleV2) GetEncodersVersion() uint8 {
	return tb.encodersVersion
}

// GetShardsNumberPower - get number of shards.
func (tb *TitleV2) GetShardsNumberPower() uint8 {
	return tb.shardsNumberPower
}

// SizeOf - get size body.
func (*TitleV2) SizeOf() int {
	return titleV2Size
}

// MarshalBinary - encoding to byte.
func (tb *TitleV2) MarshalBinary() ([]byte, error) {
	var offset int
	buf := make([]byte, tb.SizeOf())

	// write numberOfShards and move offset
	buf[0] = tb.shardsNumberPower
	offset += sizeOfUint8

	// write encodersVersion and move offset
	buf[1] = tb.encodersVersion
	offset += sizeOfUint8

	// write blockID and move offset
	buf = append(buf[:offset], tb.blockID[:]...)

	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (tb *TitleV2) UnmarshalBinary(data []byte) error {
	var off int
	// read numberOfShards
	tb.shardsNumberPower = data[0]
	off += sizeOfUint8

	// read encodersVersion
	tb.encodersVersion = data[1]
	off += sizeOfUint8

	// read blockID
	return tb.blockID.UnmarshalBinary(data[off:])
}

// ReadAt - read Title with ReaderAt.
func (tb *TitleV2) ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, off); err != nil {
		return err
	}

	return tb.UnmarshalBinary(buf)
}

// Read - read Title with Reader.
func (tb *TitleV2) Read(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	return tb.UnmarshalBinary(buf)
}
