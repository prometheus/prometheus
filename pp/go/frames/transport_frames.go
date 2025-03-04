package frames

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"time"
)

// FrameReader - read frame from net.
type FrameReader interface {
	// Read - read frame from io.Reader.
	Read(ctx context.Context, r io.Reader) error
}

// FrameWriter - frame for write to net.
type FrameWriter interface {
	// WriteTo implements io.WriterTo interface
	WriteTo(w io.Writer) (int64, error)
}

const (
	//	8(SentAt=int64)+
	//	4(ID=uint32)+
	//	4(Size=uint32)+
	//	4(CRC=uint32)
	segmentSizeV4 int = 20

	//	8(SentAt=int64)+
	//	4(SegmentID=uint32)+
	//	2(Code=uint16)+
	//	4(lengthText=uint32)
	responseSizeV4 int = 18

	// RefillSegmentSizeV4 - sum
	//	4(ID=uint32)+
	//	4(Size=uint32)+
	//	4(CRC=uint32)
	RefillSegmentSizeV4 int = 12
)

// WriteSegmentV4 - segment for send.
type WriteSegmentV4 struct {
	SentAt  int64
	ID      uint32
	Size    uint32
	CRC     uint32
	Payload WritePayload
}

var _ FrameWriter = (*WriteSegmentV4)(nil)

// NewWriteSegmentV4 - init new WriteSegmentV4 via stream.
func NewWriteSegmentV4(id uint32, payload WritePayload) *WriteSegmentV4 {
	f := &WriteSegmentV4{
		ID:      id,
		Payload: payload,
		SentAt:  time.Now().UnixNano(),
	}
	if payload == nil {
		return f
	}
	f.Size = uint32(payload.Size())
	chksum := crc32.NewIEEE()
	_, _ = payload.WriteTo(chksum)
	f.CRC = chksum.Sum32()
	return f
}

// WriteTo implements io.WriterTo interface
func (f *WriteSegmentV4) WriteTo(w io.Writer) (int64, error) {
	buf := make([]byte, segmentSizeV4)
	var offset int
	// write SentAt and move offset
	binary.LittleEndian.PutUint64(buf[offset:offset+sizeOfUint64], uint64(f.SentAt))
	offset += sizeOfUint64

	// write ID and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], f.ID)
	offset += sizeOfUint32

	// write Size and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], f.Size)
	offset += sizeOfUint32

	// write CRC and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], f.CRC)

	k, err := w.Write(buf)
	if err != nil {
		return int64(k), err
	}
	if f.Size == 0 {
		return int64(k), nil
	}

	n, err := f.Payload.WriteTo(w)
	n += int64(k)
	if err != nil {
		return n, err
	}

	return n, nil
}

// ReadSegmentV4 - segment for read.
type ReadSegmentV4 struct {
	SentAt int64
	ID     uint32
	Size   uint32
	CRC    uint32
	Body   []byte
}

var _ FrameReader = (*ReadSegmentV4)(nil)

// NewReadSegmentV4Empty - init new empty ReadSegmentV4 via stream.
func NewReadSegmentV4Empty() *ReadSegmentV4 {
	return new(ReadSegmentV4)
}

// GetBody - return body segment.
func (f *ReadSegmentV4) GetBody() []byte {
	return f.Body
}

// GetChksum - return checksum.
func (f *ReadSegmentV4) GetChksum() uint32 {
	return f.CRC
}

// GetSize - return size segment.
func (f *ReadSegmentV4) GetSize() uint32 {
	return f.Size
}

// Read - read frame from io.Reader.
func (f *ReadSegmentV4) Read(ctx context.Context, r io.Reader) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	buf := make([]byte, segmentSizeV4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	var offset int
	// read SentAt and move offset
	f.SentAt = int64(binary.LittleEndian.Uint64(buf[offset : offset+sizeOfUint64]))
	offset += sizeOfUint64

	// read ID and move offset
	f.ID = binary.LittleEndian.Uint32(buf[offset : offset+sizeOfUint32])
	offset += sizeOfUint32

	// read Size and move offset
	f.Size = binary.LittleEndian.Uint32(buf[offset : offset+sizeOfUint32])
	offset += sizeOfUint32

	// read CRC
	f.CRC = binary.LittleEndian.Uint32(buf[offset : offset+sizeOfUint32])

	if f.Size == 0 {
		return nil
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	// read Payload
	f.Body = make([]byte, f.Size)
	if _, err := io.ReadFull(r, f.Body); err != nil {
		return fmt.Errorf("read segment Payload: %w", err)
	}

	return f.Validate()
}

// Validate - validate segment.
func (f *ReadSegmentV4) Validate() error {
	return NotEqualChecksum(f.GetChksum(), crc32.ChecksumIEEE(f.GetBody()))
}

// ResponseV4 - response msg for read/write.
type ResponseV4 struct {
	SentAt    int64
	SegmentID uint32
	Code      uint16
	Text      string
}

var _ FrameReader = (*ResponseV4)(nil)
var _ FrameWriter = (*ResponseV4)(nil)

// NewResponseV4 - init new ResponseV4 via stream.
func NewResponseV4(sentAt int64, segmentID uint32, code uint16, text string) *ResponseV4 {
	return &ResponseV4{
		SentAt:    sentAt,
		SegmentID: segmentID,
		Code:      code,
		Text:      text,
	}
}

// NewResponseV4Empty - init new empty ResponseV4 via stream.
func NewResponseV4Empty() *ResponseV4 {
	return new(ResponseV4)
}

// Read - read frame from io.Reader.
func (f *ResponseV4) Read(ctx context.Context, r io.Reader) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	buf := make([]byte, responseSizeV4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	var offset int
	// read SentAt and move offset
	f.SentAt = int64(binary.LittleEndian.Uint64(buf[offset : offset+sizeOfUint64]))
	offset += sizeOfUint64

	// read SegmentID and move offset
	f.SegmentID = binary.LittleEndian.Uint32(buf[offset : offset+sizeOfUint32])
	offset += sizeOfUint32

	// read Code and move offset
	f.Code = binary.LittleEndian.Uint16(buf[offset : offset+sizeOfUint16])
	offset += sizeOfUint16

	// read length Text
	lengthText := binary.LittleEndian.Uint32(buf[offset : offset+sizeOfUint32])
	if lengthText == 0 {
		return nil
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	// read Text
	txtb := make([]byte, lengthText)
	if _, err := io.ReadFull(r, txtb); err != nil {
		return fmt.Errorf("read response Text: %w", err)
	}
	f.Text = string(txtb)

	return nil
}

// WriteTo - implements io.WriterTo interface.
func (f *ResponseV4) WriteTo(w io.Writer) (int64, error) {
	buf := make([]byte, responseSizeV4)
	var offset int
	// write SentAt and move offset
	binary.LittleEndian.PutUint64(buf[offset:offset+sizeOfUint64], uint64(f.SentAt))
	offset += sizeOfUint64

	// write SegmentID and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], f.SegmentID)
	offset += sizeOfUint32

	// write Code and move offset
	binary.LittleEndian.PutUint16(buf[offset:offset+sizeOfUint16], f.Code)
	offset += sizeOfUint16

	// write length Text and move offset
	lengthText := len([]byte(f.Text))
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], uint32(lengthText))

	k, err := w.Write(buf)
	if err != nil {
		return int64(k), err
	}
	if lengthText == 0 {
		return int64(k), nil
	}
	n, err := w.Write([]byte(f.Text))
	n += k
	if err != nil {
		return int64(n), err
	}

	return int64(n), nil
}

// WriteRefillSegmentV4 - segment for send as refill.
type WriteRefillSegmentV4 struct {
	ID      uint32
	Size    uint32
	CRC     uint32
	Payload WritePayload
}

var _ FrameWriter = (*WriteRefillSegmentV4)(nil)

// NewWriteRefillSegmentV4 - init new WriteRefillSegmentV4 via refill.
func NewWriteRefillSegmentV4(id uint32, payload WritePayload) *WriteRefillSegmentV4 {
	f := &WriteRefillSegmentV4{
		ID:      id,
		Payload: payload,
	}
	if payload == nil {
		return f
	}
	f.Size = uint32(payload.Size())
	chksum := crc32.NewIEEE()
	_, _ = payload.WriteTo(chksum)
	f.CRC = chksum.Sum32()
	return f
}

// WriteTo - implements io.WriterTo interface.
func (f *WriteRefillSegmentV4) WriteTo(w io.Writer) (int64, error) {
	buf := make([]byte, RefillSegmentSizeV4)
	var offset int
	// write ID and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], f.ID)
	offset += sizeOfUint32

	// write Size and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], f.Size)
	offset += sizeOfUint32

	// write CRC and move offset
	binary.LittleEndian.PutUint32(buf[offset:offset+sizeOfUint32], f.CRC)

	k, err := w.Write(buf)
	if err != nil {
		return int64(k), err
	}
	if f.Size == 0 {
		return int64(k), nil
	}

	n, err := f.Payload.WriteTo(w)
	n += int64(k)
	if err != nil {
		return n, err
	}

	return n, nil
}

// ReadRefillSegmentV4 - segment for read from refill.
type ReadRefillSegmentV4 struct {
	ID   uint32
	Size uint32
	CRC  uint32
	Body []byte
}

var _ FrameReader = (*ReadRefillSegmentV4)(nil)

// NewReadRefillSegmentV4Empty - init new empty ReadRefillSegmentV4 via refill.
func NewReadRefillSegmentV4Empty() *ReadRefillSegmentV4 {
	return new(ReadRefillSegmentV4)
}

// GetBody - return body segment.
func (f *ReadRefillSegmentV4) GetBody() []byte {
	return f.Body
}

// GetChksum - return checksum.
func (f *ReadRefillSegmentV4) GetChksum() uint32 {
	return f.CRC
}

// GetSize - return size segment.
func (f *ReadRefillSegmentV4) GetSize() uint32 {
	return f.Size
}

// Read - read frame from io.Reader.
func (f *ReadRefillSegmentV4) Read(ctx context.Context, r io.Reader) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	buf := make([]byte, RefillSegmentSizeV4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	var offset int
	// read ID and move offset
	f.ID = binary.LittleEndian.Uint32(buf[offset : offset+sizeOfUint32])
	offset += sizeOfUint32

	// read Size and move offset
	f.Size = binary.LittleEndian.Uint32(buf[offset : offset+sizeOfUint32])
	offset += sizeOfUint32

	// read CRC
	f.CRC = binary.LittleEndian.Uint32(buf[offset : offset+sizeOfUint32])

	if f.Size == 0 {
		return nil
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	// read Payload
	f.Body = make([]byte, f.Size)
	if _, err := io.ReadFull(r, f.Body); err != nil {
		return fmt.Errorf("read segment Payload: %w", err)
	}

	return f.validate()
}

// Validate - validate segment.
func (f *ReadRefillSegmentV4) validate() error {
	return NotEqualChecksum(f.GetChksum(), crc32.ChecksumIEEE(f.GetBody()))
}
