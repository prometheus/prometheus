package transport

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

var (
	// ErrUnknownHeader - error for unknown header.
	ErrUnknownHeader = errors.New("unknown header")
	// ErrMessageLarge - error for large message.
	ErrMessageLarge = errors.New("message size is too large")
	// ErrUnknownHeaderVersion - error for unknown version protocol.
	ErrUnknownHeaderVersion = errors.New("unknown header version")
	// ErrMessageNull - error for null message.
	ErrMessageNull = errors.New("message size is null")
)

const (
	// MaxHeaderSize - maximum header size.
	MaxHeaderSize int64 = 6
	// MaxMsgSize - maximum allowed message size.
	MaxMsgSize uint32 = 31457280
)

// MsgType - type for messages.
type MsgType int8

// Header constant types.
const (
	MsgAuth MsgType = iota
	MsgResponse
	MsgPut
	MsgDryPut
	MsgRefill
	MsgSnapshot
	MsgUnknown
)

// Header - header containing information about the message.
type Header struct {
	Version uint8
	Type    MsgType
	Size    uint32
}

// String - serialize to string.
func (h Header) String() string {
	return fmt.Sprintf(
		"Header{version: %d, type: %d, size: %d}",
		h.Version,
		h.Type,
		h.Size,
	)
}

// Validate - validate header.
func (h Header) Validate() error {
	if h.Size > MaxMsgSize {
		return ErrMessageLarge
	}

	if h.Size < 1 {
		return ErrMessageNull
	}

	if h.Type >= MsgUnknown {
		return ErrUnknownHeader
	}

	return nil
}

// RawMessage - raw message.
type RawMessage struct {
	Header  Header
	Payload []byte
}

// NewRawMessage - init new RawMessage.
func NewRawMessage(version uint8, msgType MsgType, p []byte) *RawMessage {
	return &RawMessage{
		Header: Header{
			Version: version,
			Type:    msgType,
			Size:    uint32(len(p)),
		},
		Payload: p,
	}
}

// readHeader - read data and set to Header.
func (rm *RawMessage) readHeader(reader io.Reader) error {
	if err := binary.Read(reader, binary.LittleEndian, &rm.Header); err != nil {
		return err
	}

	if err := rm.Header.Validate(); err != nil {
		return fmt.Errorf("%w: %s", err, rm.Header.String())
	}

	return nil
}

// readPayload - read data and set to Payload.
func (rm *RawMessage) readPayload(reader io.Reader) error {
	rm.Payload = make([]byte, int(rm.Header.Size))

	_, err := io.ReadFull(reader, rm.Payload)
	if err != nil {
		return err
	}

	return nil
}

// WriteRawMessage - write a message.
func WriteRawMessage(writer io.Writer, rm *RawMessage) (err error) {
	if err = binary.Write(writer, binary.LittleEndian, rm.Header); err != nil {
		return fmt.Errorf("failed write header: %w", err)
	}

	if _, err = writer.Write(rm.Payload); err != nil {
		return fmt.Errorf("failed write payload: %w", err)
	}

	return nil
}

// ReadRawMessage - read a message.
func ReadRawMessage(reader io.Reader) (*RawMessage, error) {
	raw := &RawMessage{}

	if err := raw.readHeader(reader); err != nil {
		return nil, fmt.Errorf("failed read header: %w", err)
	}

	if err := raw.readPayload(reader); err != nil {
		return nil, fmt.Errorf("failed read payload: %w", err)
	}

	return raw, nil
}

var (
	// ErrTokenEmpty - error for empty token.
	ErrTokenEmpty = errors.New("auth token is empty")
	// ErrUUIDEmpty - error for empty UUID.
	ErrUUIDEmpty = errors.New("agent uuid is empty")
)

// AuthMsg - message for authorization.
type AuthMsg struct {
	Token     string
	AgentUUID string
}

// EncodeBinary - encoding to byte.
func (am *AuthMsg) EncodeBinary() []byte {
	buf := make([]byte, 0, 8+len(am.Token)+len(am.AgentUUID))

	buf = binary.AppendUvarint(buf, uint64(len(am.Token)))
	buf = append(buf, am.Token...)

	buf = binary.AppendUvarint(buf, uint64(len(am.AgentUUID)))
	buf = append(buf, am.AgentUUID...)
	return buf
}

// DecodeBinary - decoding from byte.
func (am *AuthMsg) DecodeBinary(data []byte) {
	var offset int
	lenStr, n := binary.Uvarint(data)
	offset += n

	am.Token = string(data[offset : offset+int(lenStr)])
	offset += int(lenStr)

	lenStr, n = binary.Uvarint(data[offset:])
	offset += n

	am.AgentUUID = string(data[offset : offset+int(lenStr)])
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

// ResponseMsg - message for Response.
type ResponseMsg struct {
	Text      string
	Code      uint32
	SegmentID uint32
}

// EncodeBinary - encoding to byte.
func (rm *ResponseMsg) EncodeBinary() []byte {
	buf := make([]byte, 0, 12+len(rm.Text))

	buf = binary.AppendUvarint(buf, uint64(len(rm.Text)))
	buf = append(buf, rm.Text...)

	buf = binary.AppendUvarint(buf, uint64(rm.Code))

	buf = binary.AppendUvarint(buf, uint64(rm.SegmentID))
	return buf
}

// DecodeBinary - decoding from byte.
func (rm *ResponseMsg) DecodeBinary(data []byte) {
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

	id, _ := binary.Uvarint(data[offset:])
	rm.SegmentID = uint32(id)
}

// MessageData - data for the start message of the refill.
type MessageData struct {
	ID      uint32
	Size    uint32
	Typemsg MsgType
}

// RefillMsg - message for Refill.
type RefillMsg struct {
	Messages []MessageData
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

	buf := make(
		[]byte,
		0,
		bufSize,
	)

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
				Typemsg: MsgType(typemsg),
			},
		)
	}

	return nil
}

// Config - config for Transport.
type Config struct {
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// Transport - transport implementation.
type Transport struct {
	conn net.Conn
	cfg  *Config
}

// New - init new Transport with conn.
func New(cfg *Config, conn net.Conn) *Transport {
	return &Transport{
		conn: conn,
		cfg:  cfg,
	}
}

// Read - read msg from connection.
func (nt *Transport) Read(ctx context.Context) (*RawMessage, error) {
	if nt.conn == nil {
		return nil, net.ErrClosed
	}

	ctx, cancel := context.WithTimeout(ctx, nt.cfg.ReadTimeout)
	defer cancel()

	deadline, _ := ctx.Deadline()
	if err := nt.conn.SetReadDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}
	defer func() {
		if nt.conn != nil {
			_ = nt.conn.SetReadDeadline(time.Time{})
		}
	}()

	raw, err := ReadRawMessage(nt.conn)
	if err != nil {
		return nil, fmt.Errorf("read raw message: %w", err)
	}

	return raw, nil
}

// write - write msg in connection.
func (nt *Transport) Write(ctx context.Context, rm *RawMessage) error {
	if nt.conn == nil {
		return net.ErrClosed
	}

	ctx, cancel := context.WithTimeout(ctx, nt.cfg.WriteTimeout)
	defer cancel()

	deadline, _ := ctx.Deadline()
	if err := nt.conn.SetWriteDeadline(deadline); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	defer func() {
		_ = nt.conn.SetWriteDeadline(time.Time{})
	}()

	if err := WriteRawMessage(nt.conn, rm); err != nil {
		return fmt.Errorf("write raw message: %w", err)
	}

	return nil
}

// Close - close connection.
func (nt *Transport) Close() error {
	if nt.conn == nil {
		return nil
	}

	return nt.conn.Close()
}
