package frames

import (
	"bytes"
	"context"
	"encoding"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"sort"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/util"
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
||           BinaryBody(Segment)           ||
|+--------------------or-------------------+|
||                 Statuses                ||
|+--------------------or-------------------+|
||              RejectStatuses             ||
|+--------------------or-------------------+|
||              RefillShardEOF             ||
|+-----------------------------------------+|
+-------------------------------------------+
*/

// ReadFrame - frame readed from file or something.
type ReadFrame struct {
	Header *Header
	Body   []byte
}

// NewFrame - init new Frame.
func NewFrame(
	version, contentVersion uint8,
	typeFrame TypeFrame,
	b []byte,
) (*ReadFrame, error) {
	h, err := NewHeader(version, contentVersion, typeFrame, uint32(len(b)))
	if err != nil {
		return nil, err
	}
	h.SetChksum(crc32.ChecksumIEEE(b))
	h.SetCreatedAt(time.Now().UnixNano())

	return &ReadFrame{
		Header: h,
		Body:   b,
	}, nil
}

// NewFrameEmpty - init new Frame for read.
func NewFrameEmpty() *ReadFrame {
	return new(ReadFrame)
}

// GetHeader - return header frame.
func (fr *ReadFrame) GetHeader() Header {
	return *fr.Header
}

// GetVersion - return version.
func (fr *ReadFrame) GetVersion() uint8 {
	return fr.Header.GetVersion()
}

// GetType - return type frame.
func (fr *ReadFrame) GetType() TypeFrame {
	return fr.Header.GetType()
}

// GetCreatedAt - return created time unix nano.
func (fr *ReadFrame) GetCreatedAt() int64 {
	return fr.Header.GetCreatedAt()
}

// GetChksum - return checksum.
func (fr *ReadFrame) GetChksum() uint32 {
	return fr.Header.GetChksum()
}

// GetBody - return body frame.
func (fr *ReadFrame) GetBody() []byte {
	return fr.Body
}

// SizeOf - get size frame.
func (fr *ReadFrame) SizeOf() int {
	size := fr.Header.SizeOf()

	if fr.Body != nil {
		size += len(fr.Body)
	}

	return size
}

// EncodeBinary - encoding to byte.
func (fr *ReadFrame) EncodeBinary() []byte {
	buf := make([]byte, 0, fr.SizeOf())
	buf = append(buf, fr.Header.EncodeBinary()...)
	if fr.Body != nil {
		buf = append(buf, fr.Body...)
	}

	return buf
}

// Read - read frame from io.Reader.
func (fr *ReadFrame) Read(ctx context.Context, r io.Reader) error {
	h, err := ReadHeader(ctx, r)
	if err != nil {
		return err
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	fr.Header = h
	fr.Body = make([]byte, int(fr.Header.GetSize()))
	if _, err = io.ReadFull(r, fr.Body); err != nil {
		return err
	}

	return fr.Validate()
}

// WriteTo implements io.WriterTo interface
func (fr *ReadFrame) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(fr.EncodeBinary())

	return int64(n), err
}

// Validate - validate frame.
func (fr *ReadFrame) Validate() error {
	return NotEqualChecksum(fr.GetChksum(), crc32.ChecksumIEEE(fr.GetBody()))
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
func NewAuthFrame(
	version uint8,
	token, agentUUID, productName, agentHostname, blockID string,
	shardID uint16,
) (*ReadFrame, error) {
	body, err := NewAuthMsg(token, agentUUID, productName, agentHostname, blockID, shardID).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, ContentVersion1, AuthType, body)
}

// NewAuthFrameWithMsg - init new frame with msg.
func NewAuthFrameWithMsg(version uint8, msg *AuthMsg) (*ReadFrame, error) {
	body, err := msg.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, ContentVersion1, AuthType, body)
}

// AuthMsg - message for authorization.
type AuthMsg struct {
	Token         string
	AgentUUID     string
	ProductName   string
	AgentHostname string
	BlockID       string
	ShardID       uint16
}

// NewAuthMsg - init new AuthMsg.
func NewAuthMsg(token, agentUUID, productName, agentHostname, blockID string, shardID uint16) *AuthMsg {
	return &AuthMsg{
		Token:         token,
		AgentUUID:     agentUUID,
		ProductName:   productName,
		AgentHostname: agentHostname,
		BlockID:       blockID,
		ShardID:       shardID,
	}
}

// NewAuthMsgEmpty - init AuthMsg for read.
func NewAuthMsgEmpty() *AuthMsg {
	return new(AuthMsg)
}

// MarshalBinary - encoding to byte.
func (am *AuthMsg) MarshalBinary() ([]byte, error) {
	//revive:disable-next-line:add-constant this not constant
	length := 4 + len(am.Token) + len(am.AgentUUID) + len(am.ProductName) + len(am.AgentHostname) + len(am.BlockID) + 2
	buf := make([]byte, 0, length)

	buf = binary.AppendUvarint(buf, uint64(len(am.Token)))
	buf = append(buf, am.Token...)

	buf = binary.AppendUvarint(buf, uint64(len(am.AgentUUID)))
	buf = append(buf, am.AgentUUID...)

	buf = binary.AppendUvarint(buf, uint64(len(am.ProductName)))
	buf = append(buf, am.ProductName...)

	buf = binary.AppendUvarint(buf, uint64(len(am.AgentHostname)))
	buf = append(buf, am.AgentHostname...)

	buf = binary.AppendUvarint(buf, uint64(len(am.BlockID)))
	buf = append(buf, am.BlockID...)

	buf = binary.AppendUvarint(buf, uint64(am.ShardID))
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
	offset += int(lenStr)

	lenStr, n = binary.Uvarint(data[offset:])
	offset += n
	am.ProductName = string(data[offset : offset+int(lenStr)])
	offset += int(lenStr)

	lenStr, n = binary.Uvarint(data[offset:])
	offset += n
	am.AgentHostname = string(data[offset : offset+int(lenStr)])
	offset += int(lenStr)

	if offset >= len(data) {
		return nil
	}

	lenStr, n = binary.Uvarint(data[offset:])
	offset += n
	am.BlockID = string(data[offset : offset+int(lenStr)])
	offset += int(lenStr)

	shardID, _ := binary.Uvarint(data[offset:])
	am.ShardID = uint16(shardID)
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
func NewResponseFrame(version uint8, text string, code, segmentID uint32, sendAt int64) (*ReadFrame, error) {
	body, err := NewResponseMsg(text, code, segmentID, sendAt).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, ContentVersion1, ResponseType, body)
}

// NewResponseFrameWithMsg - init new frame with msg.
func NewResponseFrameWithMsg(version uint8, msg *ResponseMsg) (*ReadFrame, error) {
	body, err := msg.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, ContentVersion1, ResponseType, body)
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
func NewRefillFrame(version uint8, msgs []MessageData) (*ReadFrame, error) {
	body, err := NewRefillMsg(msgs).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, ContentVersion1, RefillType, body)
}

// NewRefillFrameWithMsg - init new frame with msg.
func NewRefillFrameWithMsg(version uint8, msg *RefillMsg) (*ReadFrame, error) {
	body, err := msg.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, ContentVersion1, RefillType, body)
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

// Size returns bytes length of message after encoding
func (rm *RefillMsg) Size() int64 {
	var n int64
	n += util.VarintLen(uint64(len(rm.Messages)))
	for i := range rm.Messages {
		n += util.VarintLen(uint64(rm.Messages[i].ID))
		n += util.VarintLen(uint64(rm.Messages[i].Size))
		n += util.VarintLen(uint64(rm.Messages[i].Typemsg))
	}
	return n
}

// WriteTo implements io.WriterTo interface
func (rm *RefillMsg) WriteTo(w io.Writer) (int64, error) {
	buf := make([]byte, 0, rm.Size())

	// write length statuses on destinations and move offset
	buf = binary.AppendUvarint(buf, uint64(len(rm.Messages)))

	for i := range rm.Messages {
		buf = binary.AppendUvarint(buf, uint64(rm.Messages[i].ID))
		buf = binary.AppendUvarint(buf, uint64(rm.Messages[i].Size))
		buf = binary.AppendUvarint(buf, uint64(rm.Messages[i].Typemsg))
	}

	n, err := w.Write(buf)
	return int64(n), err
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
func NewDestinationsNamesFrame(version uint8, names ...string) (*ReadFrame, error) {
	body, err := NewDestinationsNames(names...).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, ContentVersion1, DestinationNamesType, body)
}

// NewDestinationsNamesFrameWithMsg - init new frame with msg.
func NewDestinationsNamesFrameWithMsg(version uint8, msg *DestinationsNames) (*ReadFrame, error) {
	body, err := msg.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, ContentVersion1, DestinationNamesType, body)
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
func NewStatusesFrame(ss encoding.BinaryMarshaler) (*ReadFrame, error) {
	body, err := ss.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return NewFrame(defaultVersion, ContentVersion1, StatusType, body)
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

// Read - read Statuses with Reader.
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
func NewRejectStatusesFrame(rs encoding.BinaryMarshaler) (*ReadFrame, error) {
	body, err := rs.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return NewFrame(defaultVersion, ContentVersion1, RejectStatusType, body)
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

// Len - return length rejects.
func (rjss RejectStatuses) Len() int {
	return len(rjss)
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
func NewRefillShardEOFFrame(nameID uint32, shardID uint16) (*ReadFrame, error) {
	body, err := NewRefillShardEOF(nameID, shardID).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(defaultVersion, ContentVersion1, RefillShardEOFType, body)
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

// ReadAtFrameFinalMsg - read body to FinalMsg with ReaderAt.
func ReadAtFrameFinalMsg(ctx context.Context, r io.ReaderAt, off int64, size int) (*FinalMsg, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	fm := NewFinalMsgEmpty()
	if err := fm.ReadAt(ctx, r, off, size); err != nil {
		return nil, err
	}

	return fm, nil
}

// ReadFrameFinalMsg - read body to FinalMsg with Reader.
func ReadFrameFinalMsg(ctx context.Context, r io.Reader, size int) (*FinalMsg, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	fm := NewFinalMsgEmpty()
	if err := fm.Read(ctx, r, size); err != nil {
		return nil, err
	}

	return fm, nil
}

// NewFinalMsgFrame - init new frame.
func NewFinalMsgFrame(version uint8, hasRefill bool) (*ReadFrame, error) {
	body, err := NewFinalMsg(hasRefill).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return NewFrame(version, ContentVersion1, FinalType, body)
}

// FinalMsg - message indicating that the block has been finalized.
type FinalMsg struct {
	hasRefill uint8
}

// NewFinalMsg - init new FinalMsg, doesn't have refill.
//
//revive:disable-next-line:flag-parameter this is not a flag, but a parameter
func NewFinalMsg(hasRefill bool) *FinalMsg {
	if hasRefill {
		return &FinalMsg{
			hasRefill: 1,
		}
	}
	return &FinalMsg{
		hasRefill: 0,
	}
}

// NewFinalMsgEmpty - init new empty FinalMsg.
func NewFinalMsgEmpty() *FinalMsg {
	return &FinalMsg{}
}

// HasRefill - return flag has refill.
func (fm FinalMsg) HasRefill() bool {
	return fm.hasRefill == 1
}

// Size returns bytes length of message after encoding
func (fm *FinalMsg) Size() int64 {
	var n int64
	n += util.VarintLen(uint64(fm.hasRefill))
	return n
}

func (fm *FinalMsg) CRC32() uint32 {
	return 0
}

// WriteTo implements io.WriterTo interface
func (fm *FinalMsg) WriteTo(w io.Writer) (int64, error) {
	// error always nil
	buf, _ := fm.MarshalBinary()

	n, err := w.Write(buf)
	return int64(n), err
}

// MarshalBinary - encoding to byte.
func (fm FinalMsg) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 0, fm.Size())
	buf = binary.AppendUvarint(buf, uint64(fm.hasRefill))
	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (fm *FinalMsg) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)

	hasRefill, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("fail read hasRefill: %w", err)
	}
	fm.hasRefill = uint8(hasRefill)

	return nil
}

// ReadAt - read FinalMsg with ReaderAt.
func (fm *FinalMsg) ReadAt(ctx context.Context, r io.ReaderAt, off int64, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, off); err != nil {
		return err
	}

	return fm.UnmarshalBinary(buf)
}

// Read - read FinalMsg with Reader.
func (fm *FinalMsg) Read(ctx context.Context, r io.Reader, size int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	return fm.UnmarshalBinary(buf)
}
