package head

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"hash/crc32"
	"io"
)

type ShardWal struct {
	encoder             *cppbridge.HeadWalEncoder
	writeCloser         io.WriteCloser
	fileHeaderIsWritten bool
	buf                 [binary.MaxVarintLen32]byte
	bufferedWriter      *bufio.Writer
}

func newShardWal(encoder *cppbridge.HeadWalEncoder, fileHeaderIsWritten bool, writeCloser io.WriteCloser) *ShardWal {
	return &ShardWal{
		encoder:             encoder,
		writeCloser:         writeCloser,
		bufferedWriter:      bufio.NewWriter(writeCloser),
		fileHeaderIsWritten: fileHeaderIsWritten,
	}
}

func (w *ShardWal) Write(innerSeriesSlice []*cppbridge.InnerSeries) error {
	err := w.encoder.Encode(innerSeriesSlice)
	if err != nil {
		return fmt.Errorf("failed to encode inner series: %w", err)
	}

	segment, err := w.encoder.Finalize()
	if err != nil {
		return fmt.Errorf("failed to finalize segment: %w", err)
	}

	if !w.fileHeaderIsWritten {
		_, err = WriteHeader(w.writeCloser, 1, w.encoder.Version())
		if err != nil {
			return fmt.Errorf("failed to write file header: %w", err)
		}
		w.fileHeaderIsWritten = true
	}

	_, err = WriteSegment(w.writeCloser, segment)
	if err != nil {
		return fmt.Errorf("failed to write segment: %w", err)
	}

	// todo: flush?

	return nil
}

func (w *ShardWal) Close() error {
	return w.writeCloser.Close()
}

func WriteHeader(writer io.Writer, fileFormatVersion uint8, encoderVersion uint8) (n int, err error) {
	var buf [binary.MaxVarintLen32]byte
	var size int
	var bytesWritten int

	size = binary.PutUvarint(buf[:], uint64(fileFormatVersion))
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("failed to write file format version: %w", err)
	}
	n += bytesWritten

	size = binary.PutUvarint(buf[:], uint64(encoderVersion))
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("failed to write encoder version: %w", err)
	}
	n += bytesWritten

	return n, nil
}

type byteReader struct {
	r io.Reader
	n int
}

func (r *byteReader) ReadByte() (byte, error) {
	b := make([]byte, 1)
	n, err := r.r.Read(b)
	if err != nil {
		return 0, err
	}
	r.n += n
	return b[0], nil
}

func ReadHeader(reader io.Reader) (fileFormatVersion uint8, encoderVersion uint8, n int, err error) {
	br := &byteReader{r: reader}
	fileFormatVersionU64, err := binary.ReadUvarint(br)
	if err != nil {
		return 0, 0, n, fmt.Errorf("failed to read file format version: %w", err)
	}
	fileFormatVersion = uint8(fileFormatVersionU64)
	n = br.n

	encoderVersionU64, err := binary.ReadUvarint(br)
	if err != nil {
		return 0, 0, n, fmt.Errorf("failed to read encoder version: %w", err)
	}
	encoderVersion = uint8(encoderVersionU64)
	n = br.n

	return fileFormatVersion, encoderVersion, n, nil
}

func WriteSegment(writer io.Writer, segment []byte) (n int, err error) {
	var buf [binary.MaxVarintLen32]byte
	var size int
	var bytesWritten int

	segmentSize := uint64(len(segment))
	size = binary.PutUvarint(buf[:], segmentSize)
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("failed to write segment size: %w", err)
	}
	n += bytesWritten

	crc32Hash := crc32.ChecksumIEEE(segment)
	size = binary.PutUvarint(buf[:], uint64(crc32Hash))
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("failed to write segment crc32 hash: %w", err)
	}
	n += bytesWritten

	bytesWritten, err = writer.Write(segment)
	if err != nil {
		return n, fmt.Errorf("failed to write segment data: %w", err)
	}
	n += bytesWritten

	return n, nil
}

func ReadSegment(reader io.Reader) (size uint64, crc32Hash uint32, data []byte, n int, err error) {
	br := &byteReader{r: reader}
	size, err = binary.ReadUvarint(br)
	if err != nil {
		return size, crc32Hash, data, br.n, fmt.Errorf("failed to read segment size: %w", err)
	}

	crc32HashU64, err := binary.ReadUvarint(br)
	if err != nil {
		return size, crc32Hash, data, br.n, fmt.Errorf("failed to read segment crc32 hash: %w", err)
	}
	crc32Hash = uint32(crc32HashU64)

	data = make([]byte, size)
	n, err = reader.Read(data)
	if err != nil {
		return size, crc32Hash, data, br.n, fmt.Errorf("failed to read segment data: %w", err)
	}
	n += br.n

	return size, crc32Hash, data, n, nil
}
