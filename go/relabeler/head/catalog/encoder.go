package catalog

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/util/optional"
	"io"
)

type EncoderV1 struct {
}

func (EncoderV1) Encode(writer io.Writer, r *Record) (err error) {
	if err = encodeString(writer, r.id.String()); err != nil {
		return fmt.Errorf("failed to encode id: %w", err)
	}

	if err = encodeString(writer, r.id.String()); err != nil {
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
	body := bytes.NewBuffer(nil)

	if err = binary.Write(body, binary.LittleEndian, r.id); err != nil {
		return fmt.Errorf("failed to encode id: %w", err)
	}

	if err = binary.Write(body, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("failed to write number of shards: %w", err)
	}

	if err = binary.Write(body, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("failed to write created at: %w", err)
	}

	if err = binary.Write(body, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("failed to write updated at: %w", err)
	}

	if err = binary.Write(body, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("failed to write deleted at: %w", err)
	}

	if err = binary.Write(body, binary.LittleEndian, &r.corrupted); err != nil {
		return fmt.Errorf("failed to write corrupted: %w", err)
	}

	if err = binary.Write(body, binary.LittleEndian, &r.status); err != nil {
		return fmt.Errorf("failed to write status: %w", err)
	}

	if err = encodeOptionalValue(body, binary.LittleEndian, r.lastAppendedSegmentID); err != nil {
		return fmt.Errorf("failed to write last written segment id: %w", err)
	}

	writeBuffer := bytes.NewBuffer(nil)
	if err = binary.Write(writeBuffer, binary.LittleEndian, uint8(body.Len())); err != nil {
		return fmt.Errorf("failed to write size: %w", err)
	}

	if _, err = body.WriteTo(writeBuffer); err != nil {
		return fmt.Errorf("failed to write body: %w", err)
	}

	if _, err = writeBuffer.WriteTo(writer); err != nil {
		return fmt.Errorf("failed to write record: %w", err)
	}

	return nil
}

func encodeOptionalValue[T any](writer io.Writer, byteOrder binary.ByteOrder, value optional.Optional[T]) (err error) {
	var nilIndicator uint8
	if value.IsNil() {
		return binary.Write(writer, byteOrder, nilIndicator)
	}
	nilIndicator = 1
	if err = binary.Write(writer, byteOrder, nilIndicator); err != nil {
		return err
	}
	if err = binary.Write(writer, byteOrder, value.Value()); err != nil {
		return err
	}

	return nil
}
