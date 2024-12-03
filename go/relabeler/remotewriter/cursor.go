package remotewriter

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"os"
)

type Cursor struct {
	SegmentID   *uint32 `yaml:"segment_id"`
	ConfigCRC32 *uint32 `yaml:"config_crc_32"`
}

type CursorReadWriter struct {
	file *os.File
}

func NewCursorReadWriter(fileName string) (*CursorReadWriter, error) {
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	crw := &CursorReadWriter{file: file}

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to stat cursor file: %w", err), crw.Close())
	}

	if fileInfo.Size() > 0 {
		// todo: maybe unreadable, fix?
	}

	return crw, nil
}

func (crw *CursorReadWriter) Read() (c Cursor, err error) {
	_, err = crw.file.Seek(0, 0)
	if err != nil {
		return c, err
	}

	data, err := io.ReadAll(crw.file)
	if err != nil {
		return c, err
	}

	if err = yaml.Unmarshal(data, &c); err != nil {
		return c, errors.Join(fmt.Errorf("failed to unmarshar cursor data: %w", err), ErrCursorIsCorrupted)
	}

	return c, nil
}

func (crw *CursorReadWriter) Write(segmentID *uint32, configCRC32 *uint32) error {
	c := Cursor{
		SegmentID:   segmentID,
		ConfigCRC32: configCRC32,
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal cursor: %w", err)
	}

	bytesWritten, err := crw.file.WriteAt(data, 0)
	if err != nil {
		return fmt.Errorf("failed to write cursor: %w", err)
	}

	if err = crw.file.Truncate(int64(bytesWritten)); err != nil {
		// todo: maybe cursor is corrupted at this point?
		return fmt.Errorf("failed to truncate cursor file: %w", err)
	}

	return err
}

func (crw *CursorReadWriter) Close() error {
	if crw.file != nil {
		err := crw.file.Close()
		if err != nil {
			crw.file = nil
		}
		return err
	}

	return nil
}
