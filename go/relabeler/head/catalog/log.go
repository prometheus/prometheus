package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const LogFileVersion uint64 = 1

type Encoder interface {
	Encode(writer io.Writer, r Record) error
}

type Decoder interface {
	Decode(reader io.Reader, r *Record) error
}

type FileLog struct {
	file    *os.File
	encoder Encoder
	decoder Decoder
}

func NewFileLog(fileName string, encoder Encoder, decoder Decoder) (l *FileLog, err error) {
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
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

	var version uint64
	if fileInfo.Size() == 0 {
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

	return l.compactIfNeeded()
}

func (l *FileLog) Read(r *Record) error {
	return l.decoder.Decode(l.file, r)
}

func (l *FileLog) compactIfNeeded() error {
	return nil
}

func (l *FileLog) Close() error {
	return errors.Join(l.file.Close())
}
