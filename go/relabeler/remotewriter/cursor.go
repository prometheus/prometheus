package remotewriter

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
)

type CursorData struct {
	SegmentID   uint32
	ConfigCRC32 uint32
}

type Cursor struct {
	file *os.File
	buf  [binary.MaxVarintLen32]byte
}

func NewCursor(fileName string) (*Cursor, error) {
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	c := &Cursor{file: file}

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to stat cursor file: %w", err), c.Close())
	}

	if fileInfo.Size() == 0 {
		if err = c.Write(CursorData{
			SegmentID:   math.MaxUint32,
			ConfigCRC32: math.MaxUint32,
		}); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to write cursor value: %w", err), c.Close())
		}
	}

	return c, nil
}

func (c *Cursor) Read() (data CursorData, err error) {
	_, err = c.file.Seek(0, 0)
	if err != nil {
		return data, err
	}

	if err = binary.Read(c.file, binary.LittleEndian, &data.SegmentID); err != nil {
		return data, fmt.Errorf("failed to read segment id: %w", err)
	}
	if err = binary.Read(c.file, binary.LittleEndian, &data.ConfigCRC32); err != nil {
		return data, fmt.Errorf("failed to read config crc32: %w", err)
	}

	return data, nil
}

func (c *Cursor) Write(data CursorData) error {
	_, err := c.file.Seek(0, 0)
	if err != nil {
		return err
	}

	if err = binary.Write(c.file, binary.LittleEndian, data.SegmentID); err != nil {
		return fmt.Errorf("failed to write segment id: %w", err)
	}

	if err = binary.Write(c.file, binary.LittleEndian, data.ConfigCRC32); err != nil {
		return fmt.Errorf("failed to write config crc32: %w", err)
	}

	return err
}

func (c *Cursor) Close() error {
	return c.file.Close()
}
