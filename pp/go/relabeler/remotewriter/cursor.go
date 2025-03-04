package remotewriter

import (
	"errors"
	"fmt"
	"github.com/edsrzf/mmap-go"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"os"
	"unsafe"
)

type MMapFile struct {
	file *os.File
	mmap mmap.MMap
}

func NewMMapFile(fileName string, flag int, perm os.FileMode, targetSize int64) (*MMapFile, error) {
	file, err := os.OpenFile(fileName, flag, perm)
	if err != nil {
		return nil, fmt.Errorf("failed to open file")
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to stat file: %w", err), file.Close())
	}

	if fileInfo.Size() < targetSize {
		if err = fileutil.Preallocate(file, targetSize, true); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to preallocate file: %w", err), file.Close())
		}
		if err = file.Sync(); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to sync file: %w", err), file.Close())
		}
	}

	mapped, err := mmap.Map(file, mmap.RDWR, 0)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to map file: %w", err), file.Close())
	}

	if err = mapped.Lock(); err != nil {
		return nil, errors.Join(fmt.Errorf("failed to lock mapped file: %w", err), mapped.Unmap(), file.Close())
	}

	return &MMapFile{
		file: file,
		mmap: mapped,
	}, nil
}

func (f *MMapFile) Sync() error {
	return f.mmap.Flush()
}

func (f *MMapFile) Bytes() []byte {
	return f.mmap
}

func (f *MMapFile) Close() error {
	return errors.Join(f.mmap.Unlock(), f.mmap.Unmap(), f.file.Close())
}

type Cursor struct {
	targetSegmentID uint32
	configCRC32     uint32
}

type CursorReadWriter struct {
	cursor       *Cursor
	failedShards []byte
	file         *MMapFile
}

func NewCursorReadWriter(fileName string, numberOfShards uint16) (*CursorReadWriter, error) {
	cursorSize := int64(unsafe.Sizeof(Cursor{}))
	fileSize := cursorSize + int64(numberOfShards)
	file, err := NewMMapFile(fileName, os.O_CREATE|os.O_RDWR, 0600, fileSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	crw := &CursorReadWriter{
		cursor:       (*Cursor)(unsafe.Pointer(&file.Bytes()[0])),
		failedShards: unsafe.Slice(unsafe.SliceData(file.Bytes()[cursorSize:]), numberOfShards),
		file:         file,
	}

	return crw, nil
}

func (crw *CursorReadWriter) GetTargetSegmentID() uint32 {
	return crw.cursor.targetSegmentID
}
func (crw *CursorReadWriter) SetTargetSegmentID(segmentID uint32) error {
	crw.cursor.targetSegmentID = segmentID
	return crw.file.Sync()
}

func (crw *CursorReadWriter) GetConfigCRC32() uint32 {
	return crw.cursor.configCRC32
}

func (crw *CursorReadWriter) SetConfigCRC32(configCRC32 uint32) error {
	crw.cursor.configCRC32 = configCRC32
	return crw.file.Sync()
}

func (crw *CursorReadWriter) ShardIsCorrupted(shardID uint16) bool {
	return crw.failedShards[shardID] > 0
}

func (crw *CursorReadWriter) SetShardCorrupted(shardID uint16) error {
	crw.failedShards[shardID] = 1
	return crw.file.Sync()
}

func (crw *CursorReadWriter) Close() error {
	if crw.file != nil {
		err := crw.file.Close()
		if err == nil {
			crw.file = nil
		}
		return err
	}

	return nil
}
