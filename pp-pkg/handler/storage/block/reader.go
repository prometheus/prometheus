package block

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage"
)

// Reader is segments block reader.
type Reader struct {
	file                  *os.File
	blockHeader           storage.BlockHeader
	lastReadSegmentHeader storage.SegmentHeader
}

// Header return header of block.
func (r *Reader) Header() storage.BlockHeader {
	return r.blockHeader
}

// Next read and return next segment.
func (r *Reader) Next() (segment model.Segment, err error) {
	headerSize := 4 + 4
	header := make([]byte, headerSize)
	bytesRead, err := io.ReadFull(r.file, header)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return segment, storage.ErrEndOfBlock
		}

		return segment, fmt.Errorf("failed to read segment header: %w", err)
	}

	if bytesRead != headerSize {
		return segment, fmt.Errorf("failed to read segment header, bytes read: %d, header size: %d", bytesRead, headerSize)
	}

	segmentHeader := storage.SegmentHeader{
		ID:    r.lastReadSegmentHeader.ID + 1,
		Size:  binary.LittleEndian.Uint32(header[:4]),
		CRC32: binary.LittleEndian.Uint32(header[4:8]),
	}

	segment.Body = make([]byte, segmentHeader.Size)
	if _, err = io.ReadFull(r.file, segment.Body); err != nil {
		return segment, fmt.Errorf("failed to read segment body: %w", err)
	}

	segment.ID = segmentHeader.ID
	segment.Size = segmentHeader.Size
	segment.CRC = segmentHeader.CRC32

	r.lastReadSegmentHeader = segmentHeader

	return segment, nil
}

// Close reader.
func (r *Reader) Close() error {
	return r.file.Close()
}
