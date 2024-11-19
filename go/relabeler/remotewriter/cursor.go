package remotewriter

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
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

	br := &byteReader{r: c.file}
	segmentIDU64, err := binary.ReadUvarint(br)
	if err != nil {
		return data, err
	}
	configCRC32U64, err := binary.ReadUvarint(br)
	if err != nil {
		return data, err
	}
	data.SegmentID = uint32(segmentIDU64)
	data.ConfigCRC32 = uint32(configCRC32U64)
	return data, nil
}

func (c *Cursor) Write(data CursorData) error {
	_, err := c.file.Seek(0, 0)
	if err != nil {
		return err
	}

	size := binary.PutUvarint(c.buf[:], uint64(data.SegmentID))
	_, err = c.file.Write(c.buf[:size])

	size = binary.PutUvarint(c.buf[:], uint64(data.ConfigCRC32))
	_, err = c.file.WriteAt(c.buf[:size], int64(size))

	return err
}

func (c *Cursor) Close() error {
	return c.file.Close()
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
