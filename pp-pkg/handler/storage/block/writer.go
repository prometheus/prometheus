package block

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage"
)

// Writer is segments block writer.
type Writer struct {
	fileFn      func() (*os.File, error)
	file        *os.File
	blockHeader storage.BlockHeader
}

// Header return header of block.
func (w *Writer) Header() storage.BlockHeader {
	return w.blockHeader
}

// Append segment to block.
func (w *Writer) Append(segment model.Segment) error {
	var err error
	if w.file == nil {
		w.file, err = w.fileFn()
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
	}

	fileInfo, err := w.file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file stat: %w", err)
	}

	if fileInfo.Size() == 0 {
		if err = writeHeader(w.file, w.blockHeader); err != nil {
			return fmt.Errorf("failed to encode block header")
		}
	}

	if err = writeSegment(w.file, segment); err != nil {
		return fmt.Errorf("faield to write segment")
	}

	return nil
}

// Close writer.
func (w *Writer) Close() error {
	if w.file != nil {
		return w.file.Close()
	}

	return nil
}

// writeHeader write header to block.
func writeHeader(writer io.Writer, header storage.BlockHeader) error {
	var err error
	if err = binary.Write(writer, binary.LittleEndian, header.FileVersion); err != nil {
		return fmt.Errorf("failed to write file version: %w", err)
	}

	tenantIDLen := uint8(len(header.TenantID))
	if err = binary.Write(writer, binary.LittleEndian, tenantIDLen); err != nil {
		return fmt.Errorf("failed to write tenant id length: %w", err)
	}

	if tenantIDLen > 0 {
		if err = binary.Write(writer, binary.LittleEndian, []byte(header.TenantID)); err != nil {
			return fmt.Errorf("failed to write file version: %w", err)
		}
	}

	if err = binary.Write(writer, binary.LittleEndian, header.BlockID); err != nil {
		return fmt.Errorf("failed to write block id: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, header.ShardID); err != nil {
		return fmt.Errorf("failed to write shard id: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, header.ShardLog); err != nil {
		return fmt.Errorf("failed to write shard log: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, header.SegmentEncodingVersion); err != nil {
		return fmt.Errorf("failed to write segment encoding version: %w", err)
	}

	return nil
}

func writeSegment(writer io.Writer, segment model.Segment) error {
	var err error
	if err = binary.Write(writer, binary.LittleEndian, &segment.Size); err != nil {
		return fmt.Errorf("failed to write segment size: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &segment.CRC); err != nil {
		return fmt.Errorf("failed to write segment crc: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &segment.Body); err != nil {
		return fmt.Errorf("failed to write segment body: %w", err)
	}

	return nil
}
