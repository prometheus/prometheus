package model

import (
	"encoding/binary"
	"io"
)

//
// RefillSegmentEncoder
//

type RefillSegmentEncoder struct {
	writer io.Writer
}

func NewRefillSegmentEncoder(writer io.Writer) *RefillSegmentEncoder {
	return &RefillSegmentEncoder{writer: writer}
}

func (e *RefillSegmentEncoder) Encode(segment Segment) (err error) {
	buf := make([]byte, 4+4+4+len(segment.Body))
	binary.LittleEndian.PutUint32(buf[:4], segment.ID)
	binary.LittleEndian.PutUint32(buf[4:8], segment.Size)
	binary.LittleEndian.PutUint32(buf[8:12], segment.CRC)
	copy(buf[20:], segment.Body)

	_, err = e.writer.Write(buf)
	return err
}

//
// RefillSegmentDecoder
//

type RefillSegmentDecoder struct {
	reader io.Reader
}

func NewRefillSegmentDecoder(reader io.Reader) *RefillSegmentDecoder {
	return &RefillSegmentDecoder{reader: reader}
}

func (d *RefillSegmentDecoder) Decode(segment *Segment) error {
	header := make([]byte, 4+4+4)
	if _, err := io.ReadFull(d.reader, header); err != nil {
		return err
	}

	segment.ID = binary.LittleEndian.Uint32(header[:4])
	segment.Size = binary.LittleEndian.Uint32(header[4:8])
	segment.CRC = binary.LittleEndian.Uint32(header[8:12])

	if segment.Size == 0 {
		return nil
	}

	segment.Body = make([]byte, segment.Size)
	if _, err := io.ReadFull(d.reader, segment.Body); err != nil {
		return err
	}

	return nil
}

// RefillProcessingStatus status of processing refill.
type RefillProcessingStatus struct {
	Code    int
	Message string
}
