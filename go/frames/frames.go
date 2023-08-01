package frames

import (
	"bytes"
	"context"
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"sort"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/google/uuid"
)

/*
+-------------------------------------------+
|                   Frame                   |
|+-----------------------------------------+|
||                  Header                 ||
|+-----------------------------------------+|
||                 24 byte                 ||
||            1 version uin t8             ||
||            1 typeFrame uint8            ||
||            2 shardID uint16             ||
||            4 segmentID uint32           ||
||            4 size uint32                ||
||            4 chksum uint32              ||
||            8 createdAt int64            ||
|+-----------------------------------------+|
|+-----------------------------------------+|
||                   Body                  ||
|+-----------------------------------------+|
||                   Auth                  ||
|+--------------------or-------------------+|
||                 Response                ||
|+--------------------or-------------------+|
||                  Refill                 ||
|+--------------------or-------------------+|
||                  Title                  ||
|+--------------------or-------------------+|
||             DestinationNames            ||
|+--------------------or-------------------+|
|| BinaryBody(Snapshot/Segment/DrySegment) ||
|+--------------------or-------------------+|
||                 Statuses                ||
|+--------------------or-------------------+|
||              RejectStatuses             ||
|+--------------------or-------------------+|
||              RefillShardEOF             ||
|+-----------------------------------------+|
+-------------------------------------------+
*/

var (
	// ErrUnknownFrameType - error for unknown type frame.
	ErrUnknownFrameType = errors.New("unknown frame type")
	// ErrHeaderIsCorrupted - error for corrupted header in frame(not equal magic byte).
	ErrHeaderIsCorrupted = errors.New("header is corrupted")
	// ErrHeaderIsNil - error for nil header in frame.
	ErrHeaderIsNil = errors.New("header is nil")
	// ErrFrameTypeNotMatch - error for frame type does not match the requested one.
	ErrFrameTypeNotMatch = errors.New("frame type does not match")
	// ErrBodyLarge - error for large body.
	ErrBodyLarge = errors.New("body size is too large")
	// ErrBodyNull - error for null message.
	ErrBodyNull = errors.New("body size is null")
	// ErrTokenEmpty - error for empty token.
	ErrTokenEmpty = errors.New("auth token is empty")
	// ErrUUIDEmpty - error for empty UUID.
	ErrUUIDEmpty = errors.New("agent uuid is empty")
)

// TypeFrame - type of frame.
type TypeFrame uint8

// Validate - validate type frame.
func (tf TypeFrame) Validate() error {
	if tf < AuthType || tf > RefillShardEOFType {
		return fmt.Errorf("%w: %d", ErrUnknownFrameType, tf)
	}

	return nil
}

const (
	// constant size of type
	sizeOfTypeFrame = 1
	sizeOfUint8     = 1
	sizeOfUint16    = 2
	sizeOfUint32    = 4
	sizeOfUint64    = 8
	sizeOfUUID      = 16

	// default version
	defaultVersion uint8 = 3
	// magic byte for header
	magicByte byte = 165
)

const (
	// UnknownType - unknown type frame.
	UnknownType TypeFrame = iota
	// AuthType - authentication type frame.
	AuthType
	// ResponseType - refill type frame.
	ResponseType
	// RefillType - response type frame.
	RefillType
	// TitleType - title type frame.
	TitleType
	// DestinationNamesType - destination names type frame.
	DestinationNamesType
	// SnapshotType - snapshot type frame.
	SnapshotType
	// SegmentType - segment type frame.
	SegmentType
	// DrySegmentType - dry segment type frame.
	DrySegmentType
	// StatusType - destinations states type frame.
	StatusType
	// RejectStatusType - reject statuses type frame.
	RejectStatusType
	// RefillShardEOFType - refill shard EOF type frame.
	RefillShardEOFType
)

// Frame - frame for write file.
type Frame struct {
	Header *Header
	Body   []byte
}

// NewFrame - init new Frame.
func NewFrame(version uint8, typeFrame TypeFrame, b []byte, shardID uint16, segmentID uint32) *Frame {
	h := NewHeader(version, typeFrame, shardID, segmentID, uint32(len(b)))
	h.SetChksum(crc32.ChecksumIEEE(b))
	h.SetCreatedAt(time.Now().UnixNano())

	return &Frame{
		Header: h,
		Body:   b,
	}
}

// NewFrameEmpty - init new Frame for read.
func NewFrameEmpty() *Frame {
	return new(Frame)
}

// NewFrameWithHeader - init new Frame with header.
func NewFrameWithHeader(header *Header) *Frame {
	return &Frame{
		Header: header,
	}
}

// GetHeader - return header frame.
func (fr *Frame) GetHeader() Header {
	return *fr.Header
}

// GetVersion - return version.
func (fr *Frame) GetVersion() uint8 {
	return fr.Header.version
}

// GetType - return type frame.
func (fr *Frame) GetType() TypeFrame {
	return fr.Header.typeFrame
}

// GetCreatedAt - return created time unix nano.
func (fr *Frame) GetCreatedAt() int64 {
	return fr.Header.createdAt
}

// GetBody - return body frame.
func (fr *Frame) GetBody() []byte {
	return fr.Body
}

// SizeOf - get size frame.
func (fr *Frame) SizeOf() int {
	size := fr.Header.SizeOf()

	if fr.Body != nil {
		size += len(fr.Body)
	}

	return size
}

// EncodeBinary - encoding to byte.
func (fr *Frame) EncodeBinary() []byte {
	buf := make([]byte, 0, fr.SizeOf())
	buf = append(buf, fr.Header.EncodeBinary()...)
	if fr.Body != nil {
		buf = append(buf, fr.Body...)
	}

	return buf
}

// Read - read frame from io.Reader.
func (fr *Frame) Read(ctx context.Context, r io.Reader) error {
	h, err := ReadHeader(ctx, r)
	if err != nil {
		return err
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	fr.Header = h
	fr.Body = make([]byte, int(fr.Header.size))
	if _, err = io.ReadFull(r, fr.Body); err != nil {
		return err
	}

	return nil
}

// Write - write frame to io.Writer.
func (fr *Frame) Write(ctx context.Context, w io.Writer) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if _, err := w.Write(fr.EncodeBinary()); err != nil {
		return err
	}

	return nil
}

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

// ReadAtHeader - read and return only header and skip body.
func ReadAtHeader(ctx context.Context, r io.ReaderAt, off int64) (*Header, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	h := NewHeaderEmpty()
	if err := h.DecodeBinaryAt(r, off); err != nil {
		return nil, err
	}

	return h, nil
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

// DecodeBinaryAt - decoding from byte with ReaderAt.
//
//revive:disable-next-line:function-length long but readable
//revive:disable-next-line:cyclomatic is readable
func (h *Header) DecodeBinaryAt(r io.ReaderAt, pos int64) error {
	// read magic
	buf := make([]byte, sizeOfUint8)
	if _, err := r.ReadAt(buf, pos); err != nil {
		return err
	}
	h.magic = buf[0]
	pos += sizeOfUint8
	// validate magic byte
	if h.magic != magicByte {
		return fmt.Errorf("%w: pos: %d", ErrHeaderIsCorrupted, pos)
	}

	// read version
	if _, err := r.ReadAt(buf, pos); err != nil {
		return err
	}
	h.version = buf[0]
	pos += sizeOfUint8

	// read typeFrame
	if _, err := r.ReadAt(buf, pos); err != nil {
		return err
	}
	h.typeFrame = TypeFrame(buf[0])
	pos += sizeOfTypeFrame

	// validate type frame
	if err := h.typeFrame.Validate(); err != nil {
		return err
	}

	// read shardID
	buf = append(buf[:0], make([]byte, sizeOfUint16)...)
	if _, err := r.ReadAt(buf, pos); err != nil {
		return err
	}
	h.shardID = binary.LittleEndian.Uint16(buf)
	pos += sizeOfUint16

	// read segmentID
	buf = append(buf[:0], make([]byte, sizeOfUint32)...)
	if _, err := r.ReadAt(buf, pos); err != nil {
		return err
	}
	h.segmentID = binary.LittleEndian.Uint32(buf)
	pos += sizeOfUint32

	// read size frame
	if _, err := r.ReadAt(buf, pos); err != nil {
		return err
	}
	h.size = binary.LittleEndian.Uint32(buf)
	pos += sizeOfUint32

	// read chksum
	if _, err := r.ReadAt(buf, pos); err != nil {
		return err
	}
	h.chksum = binary.LittleEndian.Uint32(buf)
	pos += sizeOfUint32

	// read createdAt
	buf = append(buf[:0], make([]byte, sizeOfUint64)...)
	if _, err := r.ReadAt(buf, pos); err != nil {
		return err
	}
	h.createdAt = int64(binary.LittleEndian.Uint64(buf))

	return h.Validate()
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

//
// Frame Types
//

// ReadAtAuthMsg - read body to AuthMsg with ReaderAt.
func ReadAtAuthMsg(ctx context.Context, r io.ReaderAt, off int64, size int) (*AuthMsg, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	am := NewAuthMsgEmpty()
	if err := am.ReadAt(ctx, r, off, size); err != nil {
		return nil, err
	}

	return am, nil
}

// ReadAuthMsg - read body to AuthMsg with Reader.
func ReadAuthMsg(ctx context.Context, r io.Reader, size int) (*AuthMsg, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	am := NewAuthMsgEmpty()
	if err := am.Read(ctx, r, size); err != nil {
		return nil, err
	}

	return am, nil
}

// NewAuthFrame - init new frame.
func NewAuthFrame(version uint8, token, agentUUID string) (*Frame, error) {
	body, err := NewAuthMsg(token, agentUUID).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, AuthType, body, 0, 0), nil
}

// NewAuthFrameWithMsg - init new frame with msg.
func NewAuthFrameWithMsg(version uint8, msg *AuthMsg) (*Frame, error) {
	body, err := msg.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, AuthType, body, 0, 0), nil
}

// AuthMsg - message for authorization.
type AuthMsg struct {
	Token     string
	AgentUUID string
}

// NewAuthMsg - init new AuthMsg.
func NewAuthMsg(token, agentUUID string) *AuthMsg {
	return &AuthMsg{
		Token:     token,
		AgentUUID: agentUUID,
	}
}

// NewAuthMsgEmpty - init AuthMsg for read.
func NewAuthMsgEmpty() *AuthMsg {
	return new(AuthMsg)
}

// MarshalBinary - encoding to byte.
func (am *AuthMsg) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 0, 2+len(am.Token)+len(am.AgentUUID))

	buf = binary.AppendUvarint(buf, uint64(len(am.Token)))
	buf = append(buf, am.Token...)

	buf = binary.AppendUvarint(buf, uint64(len(am.AgentUUID)))
	buf = append(buf, am.AgentUUID...)
	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (am *AuthMsg) UnmarshalBinary(data []byte) error {
	var offset int
	lenStr, n := binary.Uvarint(data)
	offset += n

	am.Token = string(data[offset : offset+int(lenStr)])
	offset += int(lenStr)

	lenStr, n = binary.Uvarint(data[offset:])
	offset += n

	am.AgentUUID = string(data[offset : offset+int(lenStr)])

	return nil
}

// ReadAt - read AuthMsg with ReaderAt.
func (am *AuthMsg) ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, off); err != nil {
		return err
	}

	return am.UnmarshalBinary(buf)
}

// Read - read AuthMsg with Reader.
func (am *AuthMsg) Read(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	return am.UnmarshalBinary(buf)
}

// Validate - validate message.
func (am *AuthMsg) Validate() error {
	if am.Token == "" {
		return ErrTokenEmpty
	}

	if am.AgentUUID == "" {
		return ErrUUIDEmpty
	}

	return nil
}

// ReadAtResponseMsg - read body to ResponseMsg with ReaderAt.
func ReadAtResponseMsg(ctx context.Context, r io.ReaderAt, off int64, size int) (*ResponseMsg, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	rm := NewResponseMsgEmpty()
	if err := rm.ReadAt(ctx, r, off, size); err != nil {
		return nil, err
	}

	return rm, nil
}

// ReadResponseMsg - read body to ResponseMsg with Reader.
func ReadResponseMsg(ctx context.Context, r io.Reader, size int) (*ResponseMsg, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	rm := NewResponseMsgEmpty()
	if err := rm.Read(ctx, r, size); err != nil {
		return nil, err
	}

	return rm, nil
}

// NewResponseFrame - init new frame.
func NewResponseFrame(version uint8, text string, code, segmentID uint32, sendAt int64) (*Frame, error) {
	body, err := NewResponseMsg(text, code, segmentID, sendAt).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, ResponseType, body, 0, segmentID), nil
}

// NewResponseFrameWithMsg - init new frame with msg.
func NewResponseFrameWithMsg(version uint8, msg *ResponseMsg) (*Frame, error) {
	body, err := msg.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, ResponseType, body, 0, msg.SegmentID), nil
}

// NewResponseMsgEmpty - init ResponseMsg for read.
func NewResponseMsgEmpty() *ResponseMsg {
	return new(ResponseMsg)
}

// ResponseMsg - message for Response.
type ResponseMsg struct {
	Text      string
	Code      uint32
	SegmentID uint32
	SendAt    int64
}

// NewResponseMsg - init new ResponseMsg.
func NewResponseMsg(text string, code, segmentID uint32, sendAt int64) *ResponseMsg {
	return &ResponseMsg{
		Text:      text,
		Code:      code,
		SegmentID: segmentID,
		SendAt:    sendAt,
	}
}

// MarshalBinary - encoding to byte.
func (rm *ResponseMsg) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 0, 17+len(rm.Text))

	buf = binary.AppendUvarint(buf, uint64(len(rm.Text)))
	buf = append(buf, rm.Text...)
	buf = binary.AppendUvarint(buf, uint64(rm.Code))
	buf = binary.AppendUvarint(buf, uint64(rm.SegmentID))
	buf = binary.AppendUvarint(buf, uint64(rm.SendAt))
	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (rm *ResponseMsg) UnmarshalBinary(data []byte) error {
	var (
		offset int
	)

	lenStr, n := binary.Uvarint(data[offset:])
	offset += n
	rm.Text = string(data[offset : offset+int(lenStr)])
	offset += int(lenStr)

	code, n := binary.Uvarint(data[offset:])
	rm.Code = uint32(code)
	offset += n

	id, n := binary.Uvarint(data[offset:])
	rm.SegmentID = uint32(id)
	offset += n

	sendAt, _ := binary.Uvarint(data[offset:])
	rm.SendAt = int64(sendAt)

	return nil
}

// ReadAt - read AuthMsg with ReaderAt.
func (rm *ResponseMsg) ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, off); err != nil {
		return err
	}

	return rm.UnmarshalBinary(buf)
}

// Read - read AuthMsg with Reader.
func (rm *ResponseMsg) Read(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	return rm.UnmarshalBinary(buf)
}

// ReadAtRefillMsg - read body to RefillMsg with ReaderAt.
func ReadAtRefillMsg(ctx context.Context, r io.ReaderAt, off int64, size int) (*RefillMsg, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	rm := NewRefillMsgEmpty()
	if err := rm.ReadAt(ctx, r, off, size); err != nil {
		return nil, err
	}

	return rm, nil
}

// ReadRefillMsg - read body to RefillMsg with Reader.
func ReadRefillMsg(ctx context.Context, r io.Reader, size int) (*RefillMsg, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	rm := NewRefillMsgEmpty()
	if err := rm.Read(ctx, r, size); err != nil {
		return nil, err
	}

	return rm, nil
}

// NewRefillFrame - init new frame.
func NewRefillFrame(version uint8, msgs []MessageData) (*Frame, error) {
	body, err := NewRefillMsg(msgs).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, RefillType, body, 0, 0), nil
}

// NewRefillFrameWithMsg - init new frame with msg.
func NewRefillFrameWithMsg(version uint8, msg *RefillMsg) (*Frame, error) {
	body, err := msg.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, RefillType, body, 0, 0), nil
}

// MessageData - data for the start message of the refill.
type MessageData struct {
	ID      uint32
	Size    uint32
	Typemsg TypeFrame
}

// RefillMsg - message for Refill.
type RefillMsg struct {
	Messages []MessageData
}

// NewRefillMsg - init new RefillMsg.
func NewRefillMsg(msgs []MessageData) *RefillMsg {
	return &RefillMsg{
		Messages: msgs,
	}
}

// NewRefillMsgEmpty - init RefillMsg for read.
func NewRefillMsgEmpty() *RefillMsg {
	return new(RefillMsg)
}

// MarshalBinary - encoding to byte.
func (rm *RefillMsg) MarshalBinary() ([]byte, error) {
	// Uvarint use only 1 byte, and in the remaining cases x2 is enough for everything, so we allocate 1 byte each
	const (
		reserveSizeForUvarint32 = 1
		reserveSizeForUvarint8  = 1
	)

	// ID, Size, Typemsg
	msgSize := reserveSizeForUvarint32 + reserveSizeForUvarint32 + reserveSizeForUvarint8
	// number of messages + messages
	bufSize := reserveSizeForUvarint32 + msgSize*len(rm.Messages)

	buf := make([]byte, 0, bufSize)

	// write length statuses on destinations and move offset
	buf = binary.AppendUvarint(buf, uint64(len(rm.Messages)))

	for i := range rm.Messages {
		buf = binary.AppendUvarint(buf, uint64(rm.Messages[i].ID))
		buf = binary.AppendUvarint(buf, uint64(rm.Messages[i].Size))
		buf = binary.AppendUvarint(buf, uint64(rm.Messages[i].Typemsg))
	}

	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (rm *RefillMsg) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	length, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("fail read length: %w", err)
	}

	rm.Messages = make([]MessageData, 0, length)
	var id, size, typemsg uint64
	for i := 0; i < int(length); i++ {
		id, err = binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("fail read id: %w", err)
		}

		size, err = binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("fail read size: %w", err)
		}

		typemsg, err = binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("fail read typemsg: %w", err)
		}

		rm.Messages = append(
			rm.Messages,
			MessageData{
				ID:      uint32(id),
				Size:    uint32(size),
				Typemsg: TypeFrame(typemsg),
			},
		)
	}

	return nil
}

// ReadAt - read AuthMsg with ReaderAt.
func (rm *RefillMsg) ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, off); err != nil {
		return err
	}

	return rm.UnmarshalBinary(buf)
}

// Read - read AuthMsg with Reader.
func (rm *RefillMsg) Read(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	return rm.UnmarshalBinary(buf)
}

// ReadAtTitle - read body to Title with ReaderAt.
func ReadAtTitle(ctx context.Context, r io.ReaderAt, off int64, size int) (*Title, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	tb := NewTitleEmpty()
	if err := tb.ReadAt(ctx, r, off, size); err != nil {
		return nil, err
	}

	return tb, nil
}

// ReadTitle - read body to Title with Reader.
func ReadTitle(ctx context.Context, r io.Reader, size int) (*Title, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	tb := NewTitleEmpty()
	if err := tb.Read(ctx, r, size); err != nil {
		return nil, err
	}

	return tb, nil
}

// NewTitleFrame - init new frame.
func NewTitleFrame(snp uint8, blockID uuid.UUID) (*Frame, error) {
	body, err := NewTitle(snp, blockID).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(defaultVersion, TitleType, body, 0, 0), nil
}

// titleSize -contant size.
// sum = 1(numberOfShards=uint8)+16(blockID=uuid.UUID)
const titleSize int = 17

// Title - title body frame with number of shards and block ID.
type Title struct {
	shardsNumberPower uint8
	blockID           uuid.UUID
}

// NewTitle - init Title.
func NewTitle(snp uint8, blockID uuid.UUID) *Title {
	return &Title{
		shardsNumberPower: snp,
		blockID:           blockID,
	}
}

// NewTitleEmpty - init Title for read.
func NewTitleEmpty() *Title {
	return new(Title)
}

// GetShardsNumberPower - get number of shards.
func (tb *Title) GetShardsNumberPower() uint8 {
	return tb.shardsNumberPower
}

// GetBlockID - get block ID.
func (tb *Title) GetBlockID() uuid.UUID {
	return tb.blockID
}

// SizeOf - get size body.
func (*Title) SizeOf() int {
	return titleSize
}

// MarshalBinary - encoding to byte.
func (tb *Title) MarshalBinary() ([]byte, error) {
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
func (tb *Title) UnmarshalBinary(data []byte) error {
	// read numberOfShards
	var off int
	tb.shardsNumberPower = data[0]
	off += sizeOfUint8

	// read blockID
	return tb.blockID.UnmarshalBinary(data[off:])
}

// ReadAt - read Title with ReaderAt.
func (tb *Title) ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error {
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
func (tb *Title) Read(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	return tb.UnmarshalBinary(buf)
}

// ReadAtDestinationsNames - read body to DestinationsNames with ReaderAt.
func ReadAtDestinationsNames(ctx context.Context, r io.ReaderAt, off int64, size int) (*DestinationsNames, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	dn := NewDestinationsNamesEmpty()
	if err := dn.ReadAt(ctx, r, off, size); err != nil {
		return nil, err
	}

	return dn, nil
}

// ReadDestinationsNames - read body to DestinationsNames with Reader.
func ReadDestinationsNames(ctx context.Context, r io.Reader, size int) (*DestinationsNames, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	dn := NewDestinationsNamesEmpty()
	if err := dn.Read(ctx, r, size); err != nil {
		return nil, err
	}

	return dn, nil
}

// NewDestinationsNamesFrame - init new frame.
func NewDestinationsNamesFrame(version uint8, names ...string) (*Frame, error) {
	body, err := NewDestinationsNames(names...).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, DestinationNamesType, body, 0, 0), nil
}

// NewDestinationsNamesFrameWithMsg - init new frame with msg.
func NewDestinationsNamesFrameWithMsg(version uint8, msg *DestinationsNames) (*Frame, error) {
	body, err := msg.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, DestinationNamesType, body, 0, 0), nil
}

// stringViewSize -contant size.
// sum = 4(begin=int32)+4(length=int32)
const stringViewSize = 8

// stringView - string view for compact storage.
type stringView struct {
	begin  int32
	length int32
}

// toString - serialize to string.
func (sv stringView) toString(data []byte) string {
	b := data[sv.begin : sv.begin+sv.length]
	return *(*string)(unsafe.Pointer(&b)) //nolint:gosec // this is memory optimisation
}

// NotFoundName - not found name.
const NotFoundName = -1

// DestinationsNames - list of destinations to form the state of writers.
type DestinationsNames struct {
	names []stringView
	data  []byte
}

// NewDestinationsNames - init DestinationsNames with names.
func NewDestinationsNames(names ...string) *DestinationsNames {
	sort.Strings(names)
	d := make([]byte, 0)
	n := make([]stringView, 0, len(names))
	for _, name := range names {
		byteName := []byte(name)
		n = append(
			n,
			stringView{
				begin:  int32(len(d)),
				length: int32(len(byteName)),
			},
		)
		d = append(d, byteName...)
	}

	return &DestinationsNames{
		names: n,
		data:  d,
	}
}

// NewDestinationsNamesEmpty - init DestinationsNames for read.
func NewDestinationsNamesEmpty() *DestinationsNames {
	return new(DestinationsNames)
}

// Len - number of Destinations.
func (dn *DestinationsNames) Len() int {
	return len(dn.names)
}

// Equal - equal current DestinationsNames with new.
func (dn *DestinationsNames) Equal(names ...string) bool {
	if len(names) != len(dn.names) {
		return false
	}

	sort.Strings(names)

	for i, ns := range dn.ToString() {
		if ns != names[i] {
			return false
		}
	}

	return true
}

// ToString - serialize to string.
func (dn *DestinationsNames) ToString() []string {
	namesString := make([]string, 0, len(dn.names))
	for _, nv := range dn.names {
		namesString = append(namesString, nv.toString(dn.data))
	}

	return namesString
}

// IDToString - search name for id.
func (dn *DestinationsNames) IDToString(id int32) string {
	if id > int32(len(dn.names)-1) || id < 0 {
		return ""
	}

	return dn.names[int(id)].toString(dn.data)
}

// StringToID - search id for name.
func (dn *DestinationsNames) StringToID(name string) int32 {
	for i, nameView := range dn.names {
		if name == nameView.toString(dn.data) {
			return int32(i)
		}
	}

	return NotFoundName
}

// Range - calls f sequentially for each key and value present in the stlice struct.
// If f returns false, range stops the iteration.
func (dn *DestinationsNames) Range(fn func(name string, id int) bool) {
	for i, nameView := range dn.names {
		if !fn(nameView.toString(dn.data), i) {
			return
		}
	}
}

// MarshalBinary - encoding to byte.
func (dn *DestinationsNames) MarshalBinary() ([]byte, error) {
	var offset int
	size := sizeOfUint32 + stringViewSize*len(dn.names) + sizeOfUint32 + len(dn.data)
	buf := make([]byte, size)

	// write len names and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+4], uint32(len(dn.names)))
	offset += sizeOfUint32

	// write names and move offset
	for _, nameView := range dn.names {
		binary.LittleEndian.PutUint32(buf[offset:offset+4], uint32(nameView.begin))
		binary.LittleEndian.PutUint32(buf[offset+4:offset+8], uint32(nameView.length))
		offset += stringViewSize
	}

	// write len data and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+4], uint32(len(dn.data)))
	offset += sizeOfUint32

	// write data
	buf = append(buf[:offset], dn.data...)

	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (dn *DestinationsNames) UnmarshalBinary(data []byte) error {
	// read len names
	var off int
	lenSLice := binary.LittleEndian.Uint32(data[off:sizeOfUint32])
	off += sizeOfUint32

	// read names
	dn.names = make([]stringView, lenSLice)
	for i := 0; i < int(lenSLice); i++ {
		dn.names[i] = stringView{
			begin:  int32(binary.LittleEndian.Uint32(data[off : off+sizeOfUint32])),
			length: int32(binary.LittleEndian.Uint32(data[off+sizeOfUint32 : off+sizeOfUint32+sizeOfUint32])),
		}
		off += stringViewSize
	}

	// read len data
	lenSLice = binary.LittleEndian.Uint32(data[off : off+sizeOfUint32])
	off += sizeOfUint32

	// read data
	dn.data = make([]byte, lenSLice)
	copy(dn.data, data[off:])

	return nil
}

// ReadAt - read DestinationsNames with ReaderAt.
func (dn *DestinationsNames) ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, off); err != nil {
		return err
	}

	return dn.UnmarshalBinary(buf)
}

// Read - read DestinationsNames with Reader.
func (dn *DestinationsNames) Read(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	return dn.UnmarshalBinary(buf)
}

// ReadAtBinaryBody - read body to BinaryBody with ReaderAt.
func ReadAtBinaryBody(ctx context.Context, r io.ReaderAt, off int64, size int) (*BinaryBody, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	binaryBody := NewBinaryBodyEmpty()
	if err := binaryBody.ReadAt(ctx, r, off, size); err != nil {
		return nil, err
	}

	return binaryBody, nil
}

// ReadFrameSnapshot - read frame from position pos and return BinaryBody.
func ReadAtFrameSnapshot(ctx context.Context, r io.ReaderAt, off int64) (*BinaryBody, error) {
	h, err := ReadAtHeader(ctx, r, off)
	if err != nil {
		return nil, err
	}
	off += int64(h.SizeOf())

	if h.typeFrame != SnapshotType {
		return nil, ErrFrameTypeNotMatch
	}

	return ReadAtBinaryBody(ctx, r, off, int(h.GetSize()))
}

// ReadFrameSegment - read frame from position pos and return BinaryBody.
func ReadAtFrameSegment(ctx context.Context, r io.ReaderAt, off int64) (*BinaryBody, error) {
	h, err := ReadAtHeader(ctx, r, off)
	if err != nil {
		return nil, err
	}
	off += int64(h.SizeOf())

	if h.typeFrame != SegmentType {
		return nil, ErrFrameTypeNotMatch
	}

	return ReadAtBinaryBody(ctx, r, off, int(h.GetSize()))
}

// ReadBinaryBody - read body to BinaryBody with Reader.
func ReadBinaryBody(ctx context.Context, r io.Reader, size int) (*BinaryBody, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	binaryBody := NewBinaryBodyEmpty()
	if err := binaryBody.Read(ctx, r, size); err != nil {
		return nil, err
	}

	return binaryBody, nil
}

// NewSnapshotFrame - init new frame.
func NewSnapshotFrame(version uint8, shardID uint16, segmentID uint32, snapshotData []byte) (*Frame, error) {
	body, err := NewBinaryBody(snapshotData).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, SnapshotType, body, shardID, segmentID), nil
}

// NewSegmentFrame - init new frame.
func NewSegmentFrame(version uint8, shardID uint16, segmentID uint32, segmentData []byte) (*Frame, error) {
	body, err := NewBinaryBody(segmentData).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, SegmentType, body, shardID, segmentID), nil
}

// NewDrySegmentFrame - init new frame.
func NewDrySegmentFrame(version uint8, shardID uint16, segmentID uint32, drySegmentData []byte) (*Frame, error) {
	body, err := NewBinaryBody(drySegmentData).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, DrySegmentType, body, shardID, segmentID), nil
}

// BinaryBody - unsent segment/snapshot for save refill.
type BinaryBody struct {
	data []byte
}

// NewBinaryBody - init BinaryBody with data segment/snapshot.
func NewBinaryBody(binaryData []byte) *BinaryBody {
	return &BinaryBody{data: binaryData}
}

// NewBinaryBodyEmpty - init SegmentBody for read.
func NewBinaryBodyEmpty() *BinaryBody {
	return new(BinaryBody)
}

// Bytes - get body data in bytes.
func (sb *BinaryBody) Bytes() []byte {
	return sb.data
}

// SizeOf - get size body.
func (sb *BinaryBody) SizeOf() int {
	return sizeOfUint32 + len(sb.data)
}

// MarshalBinary - encoding to byte.
func (sb *BinaryBody) MarshalBinary() ([]byte, error) {
	var offset int
	buf := make([]byte, sb.SizeOf())

	// write len data and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+4], uint32(len(sb.data)))
	offset += sizeOfUint32

	// write data
	buf = append(buf[:offset], sb.data...)

	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (sb *BinaryBody) UnmarshalBinary(data []byte) error {
	// read len data
	var off int
	lenSLice := binary.LittleEndian.Uint32(data[off:sizeOfUint32])
	off += sizeOfUint32

	// read data
	sb.data = make([]byte, lenSLice)
	copy(sb.data, data[off:])

	return nil
}

// ReadAt - read BinaryBody with ReaderAt.
func (sb *BinaryBody) ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, off); err != nil {
		return err
	}

	return sb.UnmarshalBinary(buf)
}

// Read - read BinaryBody with Reader.
func (sb *BinaryBody) Read(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	return sb.UnmarshalBinary(buf)
}

// Destroy - clear memory, for implements.
func (sb *BinaryBody) Destroy() {
	sb.data = nil
}

// ReadAtFrameStatuses - read body to Statuses with ReaderAt.
func ReadAtFrameStatuses(ctx context.Context, r io.ReaderAt, off int64, size int) (Statuses, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var ss Statuses
	if err := ss.ReadAt(ctx, r, off, size); err != nil {
		return nil, err
	}

	return ss, nil
}

// ReadFrameStatuses - read body to Statuses with Reader.
func ReadFrameStatuses(ctx context.Context, r io.Reader, size int) (Statuses, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var ss Statuses
	if err := ss.Read(ctx, r, size); err != nil {
		return nil, err
	}

	return ss, nil
}

// NewStatusesFrame - init new frame.
func NewStatusesFrame(ss encoding.BinaryMarshaler) (*Frame, error) {
	body, err := ss.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return NewFrame(defaultVersion, StatusType, body, 0, 0), nil
}

// Statuses - slice with statuses.
type Statuses []uint32

// NewStatusesEmpty - init empty Statuses.
func NewStatusesEmpty(shardsNumberPower uint8, lenDests int) Statuses {
	// create statuses at least for 1 dest
	if lenDests < 1 {
		lenDests = 1
	}

	return make(Statuses, (1<<shardsNumberPower)*lenDests)
}

// Equal - equal current Statuses with new.
func (ss Statuses) Equal(newss Statuses) bool {
	if len(ss) != len(newss) {
		return false
	}

	for i := range ss {
		if ss[i] != atomic.LoadUint32(&newss[i]) {
			return false
		}
	}

	return true
}

// Reset - reset all statuses.
func (ss Statuses) Reset() {
	for i := range ss {
		ss[i] = math.MaxUint32
	}
}

// MarshalBinary implements encoding.BinaryMarshaler
func (ss Statuses) MarshalBinary() ([]byte, error) {
	// 4(length slice as.status) + 4(status(uint32))*(number of statuses)
	buf := make([]byte, 0, sizeOfUint32+sizeOfUint32*len(ss))

	// write length statuses on destinations and move offset
	buf = binary.AppendUvarint(buf, uint64(len(ss)))

	for i := range ss {
		buf = binary.AppendUvarint(buf, uint64(atomic.LoadUint32(&ss[i])))
	}

	return buf, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler
func (ss *Statuses) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	length, err := binary.ReadUvarint(r)
	if err != nil {
		return err
	}
	if cap(*ss) < int(length) {
		*ss = make([]uint32, length)
	}
	*ss = (*ss)[:0]
	var val uint64
	for i := 0; i < int(length); i++ {
		val, err = binary.ReadUvarint(r)
		if err != nil {
			return err
		}
		*ss = append(*ss, uint32(val))
	}

	return nil
}

// ReadAt - read Statuses with ReaderAt.
func (ss *Statuses) ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, off); err != nil {
		return err
	}

	return ss.UnmarshalBinary(buf)
}

// Read - read BinaryBody with Reader.
func (ss *Statuses) Read(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	return ss.UnmarshalBinary(buf)
}

// ReadAtFrameRejectStatuses - read body to RejectStatuses with ReaderAt.
func ReadAtFrameRejectStatuses(ctx context.Context, r io.ReaderAt, off int64, size int) (RejectStatuses, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var rjss RejectStatuses
	if err := rjss.ReadAt(ctx, r, off, size); err != nil {
		return nil, err
	}

	return rjss, nil
}

// ReadFrameRejectStatuses - read body to RejectStatuses with Reader.
func ReadFrameRejectStatuses(ctx context.Context, r io.Reader, size int) (RejectStatuses, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var rjss RejectStatuses
	if err := rjss.Read(ctx, r, size); err != nil {
		return nil, err
	}

	return rjss, nil
}

// NewRejectStatusesFrame - init new frame.
func NewRejectStatusesFrame(rs encoding.BinaryMarshaler) (*Frame, error) {
	body, err := rs.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return NewFrame(defaultVersion, RejectStatusType, body, 0, 0), nil
}

// Reject - rejected segment struct.
type Reject struct {
	NameID  uint32
	Segment uint32
	ShardID uint16
}

// RejectStatuses - RejectStatuses - slice with rejected segment struct.
type RejectStatuses []Reject

// the number is chosen with a finger to the sky
const defaultCapacityRejectStatuses = 512

// NewRejectStatusesEmpty - init RejectStatuses.
func NewRejectStatusesEmpty() RejectStatuses {
	return make([]Reject, 0, defaultCapacityRejectStatuses)
}

// MarshalBinary implements encoding.MarshalBinary
func (rjss RejectStatuses) MarshalBinary() ([]byte, error) {
	// 4(length slice status) + (4(NameID(uint32))+4(Segment(uint32))+2(ShardID(uint16)))*(number of statuses)
	buf := make([]byte, 0, sizeOfUint32+((sizeOfUint32+sizeOfUint32+sizeOfUint16)*len(rjss)))

	// write length statuses on destinations and move offset
	buf = binary.AppendUvarint(buf, uint64(len(rjss)))

	for i := range rjss {
		buf = binary.AppendUvarint(buf, uint64(rjss[i].NameID))
		buf = binary.AppendUvarint(buf, uint64(rjss[i].Segment))
		buf = binary.AppendUvarint(buf, uint64(rjss[i].ShardID))
	}

	return buf, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler
func (rjss *RejectStatuses) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	length, err := binary.ReadUvarint(r)
	if err != nil {
		return err
	}
	if cap(*rjss) < int(length) {
		*rjss = make(RejectStatuses, length)
	}
	*rjss = (*rjss)[:0]
	var nameID, segment, shardID uint64
	for i := 0; i < int(length); i++ {
		nameID, err = binary.ReadUvarint(r)
		if err != nil {
			return err
		}
		segment, err = binary.ReadUvarint(r)
		if err != nil {
			return err
		}
		shardID, err = binary.ReadUvarint(r)
		if err != nil {
			return err
		}
		*rjss = append(*rjss, Reject{
			NameID:  uint32(nameID),
			Segment: uint32(segment),
			ShardID: uint16(shardID),
		})
	}

	return nil
}

// ReadAt - read RejectStatuses with ReaderAt.
func (rjss *RejectStatuses) ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, off); err != nil {
		return err
	}

	return rjss.UnmarshalBinary(buf)
}

// Read - read RejectStatuses with Reader.
func (rjss *RejectStatuses) Read(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	return rjss.UnmarshalBinary(buf)
}

// ReadAtFrameRefillShardEOF - read body to RefillShardEOF with ReaderAt.
func ReadAtFrameRefillShardEOF(ctx context.Context, r io.ReaderAt, off int64, size int) (*RefillShardEOF, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	rs := NewRefillShardEOFEmpty()
	if err := rs.ReadAt(ctx, r, off, size); err != nil {
		return nil, err
	}

	return rs, nil
}

// ReadFrameRefillShardEOF - read body to RefillShardEOF with Reader.
func ReadFrameRefillShardEOF(ctx context.Context, r io.Reader, size int) (*RefillShardEOF, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	rs := NewRefillShardEOFEmpty()
	if err := rs.Read(ctx, r, size); err != nil {
		return nil, err
	}

	return rs, nil
}

// NewRefillShardEOFFrame - init new frame.
func NewRefillShardEOFFrame(nameID uint32, shardID uint16) (*Frame, error) {
	body, err := NewRefillShardEOF(nameID, shardID).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(defaultVersion, RefillShardEOFType, body, shardID, 0), nil
}

// RefillShardEOF - a message to mark that all segments have been sent.
type RefillShardEOF struct {
	NameID  uint32
	ShardID uint16
}

// NewRefillShardEOF - init new RefillShardEOF.
func NewRefillShardEOF(nameID uint32, shardID uint16) *RefillShardEOF {
	return &RefillShardEOF{
		NameID:  nameID,
		ShardID: shardID,
	}
}

// NewRefillShardEOFEmpty - init new empty RefillShardEOF.
func NewRefillShardEOFEmpty() *RefillShardEOF {
	return &RefillShardEOF{
		NameID:  math.MaxUint32,
		ShardID: math.MaxUint16,
	}
}

// MarshalBinary - encoding to byte.
func (rs RefillShardEOF) MarshalBinary() ([]byte, error) {
	// (4(NameID(uint32))+2(ShardID(uint16)))
	buf := make([]byte, 0, sizeOfUint32+sizeOfUint16)

	buf = binary.AppendUvarint(buf, uint64(rs.NameID))
	buf = binary.AppendUvarint(buf, uint64(rs.ShardID))

	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (rs *RefillShardEOF) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)

	nameID, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("fail read nameID: %w", err)
	}
	rs.NameID = uint32(nameID)

	shardID, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("fail read shardID: %w", err)
	}
	rs.ShardID = uint16(shardID)

	return nil
}

// ReadAt - read RefillShardEOF with ReaderAt.
func (rs *RefillShardEOF) ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, off); err != nil {
		return err
	}

	return rs.UnmarshalBinary(buf)
}

// Read - read RefillShardEOF with Reader.
func (rs *RefillShardEOF) Read(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	return rs.UnmarshalBinary(buf)
}
