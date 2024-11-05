package catalog

import (
	"encoding/binary"
	"fmt"
	"io"
)

type DefaultEncoder struct {
}

func (DefaultEncoder) Encode(writer io.Writer, r Record) (err error) {
	if err = encodeString(writer, r.ID); err != nil {
		return fmt.Errorf("failed to encode id: %w", err)
	}

	if err = encodeString(writer, r.Dir); err != nil {
		return fmt.Errorf("failed to encode dir: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.NumberOfShards); err != nil {
		return fmt.Errorf("failed to write number of shards: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.CreatedAt); err != nil {
		return fmt.Errorf("failed to write created at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.UpdatedAt); err != nil {
		return fmt.Errorf("failed to write updated at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.DeletedAt); err != nil {
		return fmt.Errorf("failed to write deleted at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.Status); err != nil {
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
