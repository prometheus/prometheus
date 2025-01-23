package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/util/optional"
	"io"
)

type DecoderV1 struct {
}

func (DecoderV1) Decode(reader io.ReadSeeker, r *Record) (err error) {
	var size uint64
	if err = binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return fmt.Errorf("failed to read id size: %w", err)
	}

	defer func() {
		if err != nil && errors.Is(err, io.EOF) {
			err = fmt.Errorf("%s: %w", err.Error(), io.ErrUnexpectedEOF)
		}
	}()

	buf := make([]byte, size)
	if _, err = reader.Read(buf); err != nil {
		return fmt.Errorf("failed to read id: %w", err)
	}
	r.id = uuid.MustParse(string(buf))

	if err = binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return fmt.Errorf("failed to read dir size: %w", err)
	}

	buf = make([]byte, size)
	if _, err = reader.Read(buf); err != nil {
		return fmt.Errorf("failed to read dir: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("failed to read number of shards: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("failed to read created at: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("failed to read updated at: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("failed to read deleted at: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &r.status); err != nil {
		return fmt.Errorf("failed to read status: %w", err)
	}

	return nil
}

type DecoderV2 struct{}

type readerWithCounter struct {
	reader io.Reader
	n      int
}

func (r *readerWithCounter) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	r.n += n
	return n, err
}

func (r *readerWithCounter) BytesRead() int {
	return r.n
}

func newReaderWithCounter(reader io.Reader) *readerWithCounter {
	return &readerWithCounter{reader: reader}
}

func (DecoderV2) Decode(reader io.ReadSeeker, r *Record) (err error) {
	var size uint8
	if err = binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return fmt.Errorf("failed to read record size: %w", err)
	}

	rReader := newReaderWithCounter(reader)

	defer func() {
		if err != nil && errors.Is(err, io.EOF) || size != uint8(rReader.BytesRead()) {
			err = errors.Join(err, io.ErrUnexpectedEOF)
		}
	}()

	if err = binary.Read(rReader, binary.LittleEndian, &r.id); err != nil {
		return fmt.Errorf("failed to read recird id: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("failed to read number of shards: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("failed to read created at: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("failed to read updated at: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("failed to read deleted at: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &r.corrupted); err != nil {
		return fmt.Errorf("failed to read currupted: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &r.status); err != nil {
		return fmt.Errorf("failed to read status: %w", err)
	}

	if err = decodeOptionalValue(rReader, binary.LittleEndian, &r.lastAppendedSegmentID); err != nil {
		return fmt.Errorf("failed to read last written segment id: %w", err)
	}

	return nil
}

func decodeOptionalValue[T any](reader io.Reader, byteOrder binary.ByteOrder, valueRef *optional.Optional[T]) (err error) {
	var nilIndicator uint8
	if err = binary.Read(reader, byteOrder, &nilIndicator); err != nil {
		return err
	}
	if nilIndicator == 0 {
		return nil
	}

	var value T
	if err = binary.Read(reader, byteOrder, &value); err != nil {
		return err
	}
	valueRef.Set(value)
	return nil
}
