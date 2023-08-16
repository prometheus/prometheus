package frames

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// headerSize -constant size.
	//
	// sum
	//
	//	1(magic=byte)+
	//	1(Version=uint8)+
	//	1(Type=uint8)+
	//	2(ShardID=uint16)+
	//	4(SegmentID=uint32)+
	//	4(Size=uint32)+
	//	4(Chksum=uint64)+
	//	8(CreatedAt=int64)
	headerSize int = 25
	// maxBodySize - maximum allowed message size.
	maxBodySize uint32 = 120 << 20 // 120 MB
)

// Header - header frame.
type Header struct {
	magic     byte
	version   uint8
	typeFrame TypeFrame
	shardID   uint16
	segmentID uint32
	size      uint32
	chksum    uint32
	createdAt int64
}

// NewHeader - init Header with parameter.
func NewHeader(version uint8, typeFrame TypeFrame, shardID uint16, segmentID, size uint32) *Header {
	return &Header{
		magic:     magicByte,
		version:   version,
		typeFrame: typeFrame,
		shardID:   shardID,
		segmentID: segmentID,
		size:      size,
	}
}

// NewHeaderEmpty - init Header for read.
func NewHeaderEmpty() *Header {
	return &Header{}
}

// ReadHeader - read and return only header and skip body.
func ReadHeader(ctx context.Context, r io.Reader) (*Header, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	h := NewHeaderEmpty()
	if err := h.DecodeBinary(r); err != nil {
		return nil, err
	}

	return h, nil
}

// String - serialize to string.
func (h Header) String() string {
	return fmt.Sprintf(
		"Header{version: %d, type: %d, shardID: %d, segmentID: %d, size: %d, createdAt: %d, chksum: %d}",
		h.version,
		h.typeFrame,
		h.shardID,
		h.segmentID,
		h.size,
		h.createdAt,
		h.chksum,
	)
}

// Validate - validate header.
func (h Header) Validate() error {
	if h.magic != magicByte {
		return ErrHeaderIsCorrupted
	}

	if h.size > maxBodySize {
		return ErrBodyLarge
	}

	if h.size < 1 {
		return ErrBodyNull
	}

	return h.typeFrame.Validate()
}

// GetVersion - return version.
func (h Header) GetVersion() uint8 {
	return h.version
}

// GetType - return type frame.
func (h Header) GetType() TypeFrame {
	return h.typeFrame
}

// GetShardID - return shardID.
func (h Header) GetShardID() uint16 {
	return h.shardID
}

// GetSegmentID - return segmentID.
func (h Header) GetSegmentID() uint32 {
	return h.segmentID
}

// GetSize - return size body.
func (h Header) GetSize() uint32 {
	return h.size
}

// GetChksum - return checksum.
func (h Header) GetChksum() uint32 {
	return h.chksum
}

// GetCreatedAt - return created time unix nano.
func (h Header) GetCreatedAt() int64 {
	return h.createdAt
}

// SizeOf - size of Header.
func (Header) SizeOf() int {
	return headerSize
}

// FullSize - size of Header + size body.
func (h Header) FullSize() int32 {
	return int32(h.size) + int32(headerSize)
}

// SetChksum - set checksum.
func (h *Header) SetChksum(chs uint32) {
	h.chksum = chs
}

// SetCreatedAt - set createdAt.
func (h *Header) SetCreatedAt(createdAt int64) {
	h.createdAt = createdAt
}

// EncodeBinary - encoding to byte.
func (h Header) EncodeBinary() []byte {
	var offset int
	buf := make([]byte, h.SizeOf())

	// write magic and move offset
	buf[0] = h.magic
	offset += sizeOfUint8

	// write version and move offset
	buf[1] = h.version
	offset += sizeOfUint8

	// write typeFrame and move offset
	//revive:disable-next-line:add-constant this not constant
	buf[2] = byte(h.typeFrame)
	offset += sizeOfTypeFrame

	// write shardID and move offset
	binary.LittleEndian.PutUint16(buf[offset:offset+sizeOfUint16], h.shardID)
	offset += sizeOfUint16

	// write segmentID and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], h.segmentID)
	offset += sizeOfUint32

	// write size frame and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], h.size)
	offset += sizeOfUint32

	// write chksum and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], h.chksum)
	offset += sizeOfUint32

	// write createdAt and move offset
	binary.LittleEndian.PutUint64(buf[offset:offset+sizeOfUint64], uint64(h.createdAt))

	return buf
}

// DecodeBinary - decoding from byte with Reader.
func (h *Header) DecodeBinary(r io.Reader) error {
	buf := make([]byte, h.SizeOf())
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	var pos int64
	// read magic
	h.magic = buf[0]
	// read version
	h.version = buf[1]
	// read typeFrame
	h.typeFrame = TypeFrame(buf[2])
	pos += sizeOfUint8 + sizeOfUint8 + sizeOfTypeFrame
	// read shardID sizeOfUint16
	h.shardID = binary.LittleEndian.Uint16(buf[pos : pos+sizeOfUint16])
	pos += sizeOfUint16
	// read segmentID
	h.segmentID = binary.LittleEndian.Uint32(buf[pos : pos+sizeOfUint32])
	pos += sizeOfUint32
	// read size frame
	h.size = binary.LittleEndian.Uint32(buf[pos : pos+sizeOfUint32])
	pos += sizeOfUint32
	// read chksum
	h.chksum = binary.LittleEndian.Uint32(buf[pos : pos+sizeOfUint32])
	pos += sizeOfUint32
	// read createdAt
	h.createdAt = int64(binary.LittleEndian.Uint64(buf[pos : pos+sizeOfUint64]))

	return h.Validate()
}
