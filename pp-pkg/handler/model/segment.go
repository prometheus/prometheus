package model

import (
	"encoding/binary"
	"hash/crc32"
	"io"
)

const (
	ProcessingStatusOk       uint16 = 200
	ProcessingStatusRejected uint16 = 400
)

//
// Segment
//

// Segment handle segment data.
type Segment struct {
	Timestamp int64 // sentAt
	ID        uint32
	Size      uint32
	CRC       uint32
	Body      []byte
}

// IsValid check segment body on crc.
func (s Segment) IsValid() bool {
	return crc32.ChecksumIEEE(s.Body) == s.CRC
}

//
// SegmentEncoder
//

type SegmentEncoder struct {
	writer io.Writer
}

func NewSegmentEncoder(writer io.Writer) *SegmentEncoder {
	return &SegmentEncoder{writer: writer}
}

func (e *SegmentEncoder) Encode(segment Segment) (err error) {
	buf := make([]byte, 8+4+4+4+len(segment.Body))
	binary.LittleEndian.PutUint64(buf[:8], uint64(segment.Timestamp))
	binary.LittleEndian.PutUint32(buf[8:12], segment.ID)
	binary.LittleEndian.PutUint32(buf[12:16], segment.Size)
	binary.LittleEndian.PutUint32(buf[16:20], segment.CRC)
	copy(buf[20:], segment.Body)
	_, err = e.writer.Write(buf)
	return err
}

type SegmentDecoder struct {
	reader io.Reader
}

func NewSegmentDecoder(reader io.Reader) *SegmentDecoder {
	return &SegmentDecoder{reader: reader}
}

func (d *SegmentDecoder) Decode(segment *Segment) error {
	header := make([]byte, 8+4+4+4)
	if _, err := io.ReadFull(d.reader, header); err != nil {
		return err
	}

	segment.ID = binary.LittleEndian.Uint32(header[8:12])
	segment.Timestamp = int64(binary.LittleEndian.Uint64(header[:8]))
	segment.Size = binary.LittleEndian.Uint32(header[12:16])
	segment.CRC = binary.LittleEndian.Uint32(header[16:20])

	if segment.Size == 0 {
		return nil
	}

	segment.Body = make([]byte, segment.Size)
	if _, err := io.ReadFull(d.reader, segment.Body); err != nil {
		return err
	}

	return nil
}

//
// SegmentProcessingStatus
//

// SegmentProcessingStatus status of processing segment.
type SegmentProcessingStatus struct {
	SegmentID uint32
	Code      uint16
	Message   string
	Timestamp int64
}

type SegmentProcessingStatusEncoder struct {
	writer io.Writer
}

func NewSegmentProcessingStatusEncoder(writer io.Writer) *SegmentProcessingStatusEncoder {
	return &SegmentProcessingStatusEncoder{writer: writer}
}

func (e *SegmentProcessingStatusEncoder) Encode(status SegmentProcessingStatus) error {
	buf := make([]byte, 8+4+2+4+len(status.Message))
	binary.LittleEndian.PutUint64(buf, uint64(status.Timestamp))
	binary.LittleEndian.PutUint32(buf[8:], status.SegmentID)
	binary.LittleEndian.PutUint16(buf[12:], status.Code)
	binary.LittleEndian.PutUint32(buf[14:], uint32(len(status.Message)))
	copy(buf[18:], status.Message)

	_, err := e.writer.Write(buf)
	return err
}

type SegmentProcessingStatusDecoder struct {
	reader io.Reader
}

func NewSegmentProcessingStatusDecoder(reader io.Reader) *SegmentProcessingStatusDecoder {
	return &SegmentProcessingStatusDecoder{reader: reader}
}

func (d *SegmentProcessingStatusDecoder) Decode(status *SegmentProcessingStatus) error {
	header := make([]byte, 8+4+2+4)
	if _, err := io.ReadFull(d.reader, header); err != nil {
		return err
	}

	status.Timestamp = int64(binary.LittleEndian.Uint64(header[:8]))
	status.SegmentID = binary.LittleEndian.Uint32(header[8:12])
	status.Code = binary.LittleEndian.Uint16(header[12:14])
	messageLen := binary.LittleEndian.Uint32(header[14:18])
	if messageLen == 0 {
		return nil
	}

	message := make([]byte, messageLen)
	if _, err := io.ReadFull(d.reader, message); err != nil {
		return err
	}

	status.Message = string(message)

	return nil
}
