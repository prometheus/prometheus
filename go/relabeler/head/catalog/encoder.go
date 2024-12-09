package catalog

import (
	"encoding/binary"
	"fmt"
	"io"
)

type EncoderV1 struct {
}

func (EncoderV1) Encode(writer io.Writer, r *Record) (err error) {
	if err = encodeString(writer, r.id); err != nil {
		return fmt.Errorf("failed to encode id: %w", err)
	}

	if err = encodeString(writer, r.dir); err != nil {
		return fmt.Errorf("failed to encode dir: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("failed to write number of shards: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("failed to write created at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("failed to write updated at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("failed to write deleted at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.status); err != nil {
		return fmt.Errorf("failed to write status: %w", err)
	}

	return nil
}

func encodeString(writer io.Writer, value string) (err error) {
	if err = binary.Write(writer, binary.LittleEndian, uint64(len(value))); err != nil {
		return fmt.Errorf("failed to write string length: %w", err)
	}

	if _, err = writer.Write([]byte(value)); err != nil {
		return fmt.Errorf("failed to write string: %w", err)
	}

	return nil
}

type EncoderV2 struct{}

func (EncoderV2) Encode(writer io.Writer, r *Record) (err error) {
	if err = encodeString(writer, r.id); err != nil {
		return fmt.Errorf("failed to encode id: %w", err)
	}

	if err = encodeString(writer, r.dir); err != nil {
		return fmt.Errorf("failed to encode dir: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("failed to write number of shards: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("failed to write created at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("failed to write updated at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("failed to write deleted at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.corrupted); err != nil {
		return fmt.Errorf("failed to write corrupted: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.status); err != nil {
		return fmt.Errorf("failed to write status: %w", err)
	}

	return nil
}
