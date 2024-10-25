package catalog

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	LogFileVersion uint64 = 1
)

type Encoder interface {
	Encode(writer io.Writer, r Record) error
}

type Decoder interface {
	Decode(reader io.Reader, r *Record) error
}

type FileLog struct {
	file    *FileHandler
	encoder Encoder
	decoder Decoder
}

func NewFileLog(fileName string, encoder Encoder, decoder Decoder) (l *FileLog, err error) {
	file, err := NewFileHandler(fileName)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = file.Close()
		}
	}()

	var version uint64
	if file.Size() == 0 {
		version = LogFileVersion
		if err = binary.Write(file, binary.LittleEndian, version); err != nil {
			return nil, fmt.Errorf("failed to write log file version: %w", err)
		}
	} else {
		if err = binary.Read(file, binary.LittleEndian, &version); err != nil {
			return nil, fmt.Errorf("failed to read log file version: %w", err)
		}
		if version != LogFileVersion {
			return nil, fmt.Errorf("invalid log file version: %d", version)
		}
	}

	return &FileLog{
		file:    file,
		encoder: encoder,
		decoder: decoder,
	}, nil
}

func (l *FileLog) Write(r Record) error {
	if err := l.encoder.Encode(l.file, r); err != nil {
		return fmt.Errorf("failed encode record: %w", err)
	}

	return nil
}

func (l *FileLog) ReWrite(records ...Record) error {
	buffer := bytes.NewBuffer(nil)
	version := LogFileVersion
	if err := binary.Write(buffer, binary.LittleEndian, version); err != nil {
		return fmt.Errorf("failed to write log file version: %w", err)
	}
	for _, record := range records {
		if err := l.encoder.Encode(buffer, record); err != nil {
			return fmt.Errorf("failed to encode record: %w", err)
		}
	}

	if _, err := l.file.WriteAt(buffer.Bytes(), 0); err != nil {
		return err
	}

	return nil
}

func (l *FileLog) Read(r *Record) error {
	if err := l.decoder.Decode(l.file, r); err != nil {
		return fmt.Errorf("failed to decode record: %w", err)
	}
	return nil
}

func (l *FileLog) Size() int {
	return l.file.Size()
}

func (l *FileLog) Close() error {
	return errors.Join(l.file.Close())
}

type FileHandler struct {
	file   *os.File
	size   int
	offset int
}

func (fh *FileHandler) WriteAt(p []byte, off int64) (n int, err error) {
	n, err = fh.file.WriteAt(p, off)
	if err != nil {
		return 0, err
	}
	fh.size = int(off) + n
	fh.offset = fh.size

	if _, err = fh.file.Seek(int64(fh.offset), 0); err != nil {
		return n, err
	}

	if err = fh.file.Truncate(int64(fh.size)); err != nil {
		return n, err
	}

	return n, nil
}

func NewFileHandler(fileName string) (*FileHandler, error) {
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close())
		}
	}()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to read file info: %w", err)
	}

	return &FileHandler{file: file, size: int(fileInfo.Size())}, nil
}

func (fh *FileHandler) Write(p []byte) (n int, err error) {
	n, err = fh.file.Write(p)
	fh.size += n
	fh.offset += n
	return n, err
}

func (fh *FileHandler) Read(p []byte) (n int, err error) {
	n, err = fh.file.Read(p)
	fh.offset += n
	return n, err
}

func (fh *FileHandler) Size() int {
	return fh.size
}

func (fh *FileHandler) Close() error {
	return fh.file.Close()
}
