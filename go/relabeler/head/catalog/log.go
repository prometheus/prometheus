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
	defaultBufferSize = 4096

	LogFileVersionV1 uint64 = 1
	LogFileVersionV2 uint64 = 2
)

type Encoder interface {
	Encode(writer io.Writer, r *Record) error
}

type Decoder interface {
	Decode(reader io.ReadSeeker, r *Record) error
}

type FileLog struct {
	version uint64
	file    *FileHandler
	encoder Encoder
	decoder Decoder
}

func NewFileLogV1(fileName string) (fl *FileLog, err error) {
	file, err := NewFileHandler(fileName)
	if err != nil {
		return nil, err
	}

	fl = &FileLog{
		version: LogFileVersionV1,
		file:    file,
		encoder: EncoderV1{},
		decoder: DecoderV1{},
	}

	defer func() {
		if err != nil {
			_ = fl.Close()
		}
	}()

	if file.Size() == 0 {
		if err = binary.Write(file, binary.LittleEndian, fl.version); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to write log file version: %w", err), fl.Close())
		}
	} else {
		var version uint64
		if err = binary.Read(file, binary.LittleEndian, &version); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to read log file version: %w", err), fl.Close())
		}
		if version != fl.version {
			return nil, errors.Join(fmt.Errorf("invalid log file version: %d", version), fl.Close())
		}
	}

	return fl, nil
}

func NewFileLogV2(fileName string) (fl *FileLog, err error) {
	file, err := NewFileHandler(fileName)
	if err != nil {
		return nil, err
	}

	fl = &FileLog{
		version: LogFileVersionV2,
		file:    file,
		encoder: NewEncoderV2(),
		decoder: DecoderV2{},
	}

	defer func() {
		if err != nil {
			_ = fl.Close()
		}
	}()

	if file.Size() == 0 {
		if err = binary.Write(file, binary.LittleEndian, fl.version); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to write log file version: %w", err), fl.Close())
		}
	} else {
		var version uint64
		if err = binary.Read(file, binary.LittleEndian, &version); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to read log file version: %w", err), fl.Close())
		}
		if version != fl.version {
			if err = migrate(file, version, fl.version); err != nil {
				return nil, errors.Join(fmt.Errorf("failed to migrate from %d to %d: %w", version, fl.version, err), fl.Close())
			}
		}
	}

	return fl, nil
}

func migrate(file *FileHandler, from, to uint64) error {
	if !(from == LogFileVersionV1 && to == LogFileVersionV2) {
		return fmt.Errorf("unsupported version combination: %d -> %d, supported only %d -> %d", from, to, LogFileVersionV1, LogFileVersionV2)
	}

	decoder := DecoderV1{}
	records := make([]*Record, 0, 10)
	for {
		record := Record{}
		if err := decoder.Decode(file, &record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("failed to decode record: %w", err)
		}
		records = append(records, &record)
	}

	for _, record := range records {
		if record.status == StatusCorrupted {
			record.corrupted = true
			record.status = StatusRotated
		}
	}

	encoder := NewEncoderV2()
	buffer := bytes.NewBuffer(nil)
	if err := binary.Write(buffer, binary.LittleEndian, to); err != nil {
		return fmt.Errorf("failed to write log file version: %w", err)
	}
	offset := buffer.Len()
	for _, record := range records {
		if err := encoder.Encode(buffer, record); err != nil {
			return fmt.Errorf("failed to encode record: %w", err)
		}
	}

	if _, err := file.WriteAt(buffer.Bytes(), 0); err != nil {
		return fmt.Errorf("failed to write buffer: %w", err)
	}

	if _, err := file.Seek(int64(offset), 0); err != nil {
		return fmt.Errorf("failed to set offset: %w", err)
	}

	return nil
}

func (fl *FileLog) Write(r *Record) error {
	if err := fl.encoder.Encode(fl.file, r); err != nil {
		return fmt.Errorf("failed encode record: %w", err)
	}

	return nil
}

func (fl *FileLog) ReWrite(records ...*Record) error {
	buffer := bytes.NewBuffer(nil)
	if err := binary.Write(buffer, binary.LittleEndian, fl.version); err != nil {
		return fmt.Errorf("failed to write log file version: %w", err)
	}
	for _, record := range records {
		if err := fl.encoder.Encode(buffer, record); err != nil {
			return fmt.Errorf("failed to encode record: %w", err)
		}
	}

	if _, err := fl.file.WriteAt(buffer.Bytes(), 0); err != nil {
		return err
	}

	return nil
}

func (fl *FileLog) Read(r *Record) error {
	if err := fl.decoder.Decode(fl.file, r); err != nil {
		return fmt.Errorf("failed to decode record: %w", err)
	}
	return nil
}

func (fl *FileLog) Size() int {
	return fl.file.Size()
}

func (fl *FileLog) Close() error {
	return fl.file.Close()
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

	return &FileHandler{
		file: file,
		size: int(fileInfo.Size()),
	}, nil
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

func (fh *FileHandler) Seek(offset int64, whence int) (ret int64, err error) {
	ret, err = fh.file.Seek(offset, whence)
	if err != nil {
		return ret, err
	}
	fh.offset = int(ret)
	return ret, nil
}

func (fh *FileHandler) Size() int {
	return fh.size
}

func (fh *FileHandler) Close() error {
	return fh.file.Close()
}
