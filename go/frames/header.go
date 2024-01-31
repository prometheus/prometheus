package frames

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
)

// Header versions
const (
	unknownHeaderVersion uint8 = iota
	headerVersion1
	headerVersion2
	headerVersion3
	headerVersion4
)

const (
	// headerSizeMain - sum
	//	1(magic=byte)+
	//	1(version=uint8)+
	//	1(Type=uint8)
	headerSizeMain int = 3
	//	2(ShardID=uint16)+
	//	4(SegmentID=uint32)+
	//	4(Size=uint32)+
	//	4(Chksum=uint32)+
	//	8(CreatedAt=int64)
	headerSizeV3 int = 22
	// headerSizeV4 - sum
	//	1(ContentVersion=uint8)+
	//	4(Size=uint32)+
	//	4(Chksum=uint32)+
	//	8(CreatedAt=int64)
	headerSizeV4 int = 17
	// maxBodySize - maximum allowed message size.
	maxBodySize   uint32 = 200 << 20 // 200 MB
	maxheaderSize int    = headerSizeMain + headerSizeV3
)

// VersionedHeader - versioned header
type VersionedHeader interface {
	DecodeBinary(r io.Reader) error
	DecodeBuffer(buf []byte)
	EncodeBinary() []byte
	EncodeToBuffer(buf []byte)
	GetChksum() uint32
	GetContentVersion() uint8
	GetCreatedAt() int64
	GetSegmentID() uint32
	GetShardID() uint16
	GetSize() uint32
	SetChksum(chs uint32)
	SetCreatedAt(createdAt int64)
	SizeOf() int
	String() string
	validate() error
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

// Header - main header frame.
type Header struct {
	magic     byte
	version   uint8
	typeFrame TypeFrame
	VersionedHeader
}

// NewHeader - init Header with parameter.
func NewHeader(version, contentVersion uint8, typeFrame TypeFrame, size uint32) (*Header, error) {
	h := &Header{
		magic:     magicByte,
		version:   version,
		typeFrame: typeFrame,
	}
	if err := h.Validate(); err != nil {
		return nil, err
	}
	switch h.version {
	case headerVersion3:
		return nil, fmt.Errorf("%w: version %d", ErrDeprecatedHeaderVersion, h.version)
	case headerVersion4:
		h.VersionedHeader = NewHeaderV4(contentVersion, size)
	}

	if err := h.VersionedHeader.validate(); err != nil {
		return nil, err
	}
	return h, nil
}

// NewHeaderOld - init Header with parameter.
// Deprecated. Only for test.
func NewHeaderOld(
	version, contentVersion uint8,
	typeFrame TypeFrame,
	shardID uint16,
	segmentID, size uint32,
) (*Header, error) {
	h := &Header{
		magic:     magicByte,
		version:   version,
		typeFrame: typeFrame,
	}
	if err := h.Validate(); err != nil {
		return nil, err
	}
	switch h.version {
	case headerVersion3:
		h.VersionedHeader = NewHeaderV3(shardID, segmentID, size)
	case headerVersion4:
		h.VersionedHeader = NewHeaderV4(contentVersion, size)
	}
	if err := h.VersionedHeader.validate(); err != nil {
		return nil, err
	}
	return h, nil
}

// NewHeaderEmpty - init Header for read.
func NewHeaderEmpty() *Header {
	return new(Header)
}

// FrameSize - size of Header + size body.
func (h Header) FrameSize() uint32 {
	if h.VersionedHeader == nil {
		return uint32(h.SizeOf())
	}
	return uint32(h.SizeOf()) + h.GetSize()
}

// DecodeBinary - decoding from byte with Reader.
func (h *Header) DecodeBinary(r io.Reader) error {
	buf := make([]byte, maxheaderSize)
	// decode main header
	shrinkBuf := buf[:h.sizeHeader()]
	if _, err := io.ReadFull(r, shrinkBuf); err != nil {
		return err
	}
	h.decodeBuffer(shrinkBuf)
	if err := h.Validate(); err != nil {
		return err
	}
	switch h.version {
	case headerVersion3:
		h.VersionedHeader = NewHeaderV3Empty()
	case headerVersion4:
		h.VersionedHeader = NewHeaderV4Empty()
	}
	// decode versioned header
	shrinkBuf = buf[h.sizeHeader() : h.sizeHeader()+h.VersionedHeader.SizeOf()]
	if _, err := io.ReadFull(r, shrinkBuf); err != nil {
		return err
	}
	h.VersionedHeader.DecodeBuffer(shrinkBuf)
	return h.VersionedHeader.validate()
}

// decodeBuffer - decoding from buffer byte.
func (h *Header) decodeBuffer(buf []byte) {
	// read magic
	h.magic = buf[0]
	// read version
	h.version = buf[1]
	// read typeFrame
	h.typeFrame = TypeFrame(buf[2])
}

// EncodeBinary - encoding to byte.
func (h Header) EncodeBinary() []byte {
	buf := make([]byte, h.SizeOf())
	// encode main header
	shrinkBuf := buf[:h.sizeHeader()]
	h.encode(shrinkBuf)
	// encode versioned header
	shrinkBuf = buf[h.sizeHeader():]
	h.VersionedHeader.EncodeToBuffer(shrinkBuf)
	return buf
}

// encode - encoding to byte.
func (h Header) encode(buf []byte) {
	// write magic and move offset
	buf[0] = h.magic
	// write version and move offset
	buf[1] = h.version
	// write typeFrame and move offset
	//revive:disable-next-line:add-constant this not constant
	buf[2] = byte(h.typeFrame)
}

// GetType - return type frame.
func (h Header) GetType() TypeFrame {
	return h.typeFrame
}

// GetVersion - return version.
func (h Header) GetVersion() uint8 {
	return h.version
}

// SizeOf - size of Header.
func (h Header) SizeOf() int {
	if h.VersionedHeader == nil {
		return h.sizeHeader()
	}
	return h.sizeHeader() + h.VersionedHeader.SizeOf()
}

// sizeHeader - size of Header.
func (Header) sizeHeader() int {
	return headerSizeMain
}

// String - serialize to string.
func (h Header) String() string {
	return fmt.Sprintf(
		"Header{version: %d, typeFrame: %d, %s}",
		h.version,
		h.typeFrame,
		h.VersionedHeader,
	)
}

// Validate - validate header.
func (h Header) Validate() error {
	if h.magic != magicByte {
		return ErrHeaderIsCorrupted
	}

	if h.version < headerVersion3 || h.version > headerVersion4 {
		return fmt.Errorf("%w: version %d", ErrUnknownHeaderVersion, h.version)
	}

	return h.typeFrame.Validate()
}

// HeaderV3 - header version 3 frame.
type HeaderV3 struct {
	shardID   uint16
	segmentID uint32
	size      uint32
	chksum    uint32
	createdAt int64
}

var _ VersionedHeader = (*HeaderV3)(nil)

// NewHeaderV3 - init HeaderV3 with parameter.
// Deprecated. Only for test.
func NewHeaderV3(shardID uint16, segmentID, size uint32) *HeaderV3 {
	return &HeaderV3{
		shardID:   shardID,
		segmentID: segmentID,
		size:      size,
	}
}

// NewHeaderV3Empty - init HeaderV3 for read.
func NewHeaderV3Empty() *HeaderV3 {
	return new(HeaderV3)
}

// DecodeBinary - decoding from byte with Reader.
func (h *HeaderV3) DecodeBinary(r io.Reader) error {
	buf := make([]byte, h.SizeOf())
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	h.DecodeBuffer(buf)
	return nil
}

// DecodeBuffer - decoding from buffer byte.
func (h *HeaderV3) DecodeBuffer(buf []byte) {
	var pos int64
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
}

// EncodeBinary - encoding to byte.
// Deprecated. Only for test.
func (h HeaderV3) EncodeBinary() []byte {
	buf := make([]byte, h.SizeOf())
	h.EncodeToBuffer(buf)
	return buf
}

// EncodeToBuffer - encoding to buffer byte.
// Deprecated. Only for test.
func (h HeaderV3) EncodeToBuffer(buf []byte) {
	var offset int
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
}

// GetChksum - return checksum.
func (h HeaderV3) GetChksum() uint32 {
	return h.chksum
}

// GetContentVersion - return content version.
func (HeaderV3) GetContentVersion() uint8 {
	return ContentVersion1
}

// GetCreatedAt - return created time unix nano.
func (h HeaderV3) GetCreatedAt() int64 {
	return h.createdAt
}

// GetSegmentID - return segmentID.
func (h HeaderV3) GetSegmentID() uint32 {
	return h.segmentID
}

// GetShardID - return shardID.
func (h HeaderV3) GetShardID() uint16 {
	return h.shardID
}

// GetSize - return size body.
func (h HeaderV3) GetSize() uint32 {
	return h.size
}

// SetChksum - set checksum.
func (h *HeaderV3) SetChksum(chs uint32) {
	h.chksum = chs
}

// SetCreatedAt - set createdAt.
func (h *HeaderV3) SetCreatedAt(createdAt int64) {
	h.createdAt = createdAt
}

// SizeOf - size of Header.
func (HeaderV3) SizeOf() int {
	return headerSizeV3
}

// String - serialize to string.
func (h HeaderV3) String() string {
	return fmt.Sprintf(
		"shardID: %d, segmentID: %d, size: %d, createdAt: %d, chksum: %d",
		h.shardID,
		h.segmentID,
		h.size,
		h.createdAt,
		h.chksum,
	)
}

// validate - validate header.
func (h HeaderV3) validate() error {
	if h.size > maxBodySize {
		return ErrBodyLarge
	}

	if h.size < 1 {
		return ErrBodyNull
	}

	return nil
}

// HeaderV4 - header version 4 frame.
type HeaderV4 struct {
	contentVersion uint8
	size           uint32
	chksum         uint32
	createdAt      int64
}

var _ VersionedHeader = (*HeaderV4)(nil)

// NewHeaderV4 - init HeaderV4 with parameter.
func NewHeaderV4(contentVersion uint8, size uint32) *HeaderV4 {
	return &HeaderV4{
		contentVersion: contentVersion,
		size:           size,
	}
}

// NewHeaderV4Empty - init HeaderV4 for read.
func NewHeaderV4Empty() *HeaderV4 {
	return new(HeaderV4)
}

// DecodeBinary - decoding from byte with Reader.
func (h *HeaderV4) DecodeBinary(r io.Reader) error {
	buf := make([]byte, h.SizeOf())
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	h.DecodeBuffer(buf)
	return nil
}

// DecodeBuffer - decoding from buffer byte.
func (h *HeaderV4) DecodeBuffer(buf []byte) {
	var pos int64
	// read contentVersion sizeOfUint8
	h.contentVersion = buf[0]
	pos += sizeOfUint8
	// read size frame
	h.size = binary.LittleEndian.Uint32(buf[pos : pos+sizeOfUint32])
	pos += sizeOfUint32
	// read chksum
	h.chksum = binary.LittleEndian.Uint32(buf[pos : pos+sizeOfUint32])
	pos += sizeOfUint32
	// read createdAt
	h.createdAt = int64(binary.LittleEndian.Uint64(buf[pos : pos+sizeOfUint64]))
}

// EncodeBinary - encoding to byte.
func (h HeaderV4) EncodeBinary() []byte {
	buf := make([]byte, h.SizeOf())
	h.EncodeToBuffer(buf)
	return buf
}

// EncodeToBuffer - encoding to buffer byte.
func (h HeaderV4) EncodeToBuffer(buf []byte) {
	var offset int
	// write contentVersion and move offset
	buf[0] = h.contentVersion
	offset += sizeOfUint8

	// write size frame and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], h.size)
	offset += sizeOfUint32

	// write chksum and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], h.chksum)
	offset += sizeOfUint32

	// write createdAt and move offset
	binary.LittleEndian.PutUint64(buf[offset:offset+sizeOfUint64], uint64(h.createdAt))
}

// GetChksum - return checksum.
func (h HeaderV4) GetChksum() uint32 {
	return h.chksum
}

// GetContentVersion - return content version.
func (h HeaderV4) GetContentVersion() uint8 {
	return h.contentVersion
}

// GetCreatedAt - return created time unix nano.
func (h HeaderV4) GetCreatedAt() int64 {
	return h.createdAt
}

// GetSegmentID - return segmentID.
func (HeaderV4) GetSegmentID() uint32 {
	panic("does not implement")
}

// GetShardID - return shardID.
func (HeaderV4) GetShardID() uint16 {
	panic("does not implement")
}

// GetSize - return size body.
func (h HeaderV4) GetSize() uint32 {
	return h.size
}

// SetChksum - set checksum.
func (h *HeaderV4) SetChksum(chs uint32) {
	h.chksum = chs
}

// SetCreatedAt - set createdAt.
func (h *HeaderV4) SetCreatedAt(createdAt int64) {
	h.createdAt = createdAt
}

// SizeOf - size of Header.
func (HeaderV4) SizeOf() int {
	return headerSizeV4
}

// String - serialize to string.
func (h HeaderV4) String() string {
	return fmt.Sprintf(
		"contentVersion: %d, size: %d, createdAt: %d, chksum: %d",
		h.contentVersion,
		h.size,
		h.createdAt,
		h.chksum,
	)
}

// validate - validate header.
func (h HeaderV4) validate() error {
	if h.size > maxBodySize {
		return ErrBodyLarge
	}

	if h.size < 1 {
		return ErrBodyNull
	}

	return nil
}
