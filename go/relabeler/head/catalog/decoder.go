package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

type DefaultDecoder struct {
}

func (DefaultDecoder) Decode(reader io.Reader, r *Record) (err error) {
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
	r.id = string(buf)

	if err = binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return fmt.Errorf("failed to read dir size: %w", err)
	}

	buf = make([]byte, size)
	if _, err = reader.Read(buf); err != nil {
		return fmt.Errorf("failed to read dir: %w", err)
	}
	r.dir = string(buf)

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
