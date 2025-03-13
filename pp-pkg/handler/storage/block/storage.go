package block

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/prometheus/prometheus/pp-pkg/handler/storage"
)

const (
	FileVersion = 1
)

// Storage represents read and write block with segments.
type Storage struct {
	dir string
}

// NewStorage init new Storage.
func NewStorage(dir string) *Storage {
	return &Storage{dir: dir}
}

// Writer return block writer.
func (s *Storage) Writer(blockID uuid.UUID, shardID uint16, shardLog, segmentEncodingVersion uint8) storage.BlockWriter {
	return &Writer{
		fileFn: func() (*os.File, error) {
			fileName := path.Join(s.dir, fmt.Sprintf("%s-%d", blockID.String(), shardID))
			dir := filepath.Dir(fileName)
			if err := os.MkdirAll(dir, 0o744); err != nil {
				return nil, err
			}
			return os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
		},
		blockHeader: storage.BlockHeader{
			FileVersion:            FileVersion,
			TenantID:               "",
			BlockID:                blockID,
			ShardID:                shardID,
			ShardLog:               shardLog,
			SegmentEncodingVersion: segmentEncodingVersion,
		},
	}
}

// Reader return block reader.
func (s *Storage) Reader(blockID uuid.UUID, shardID uint16) (storage.BlockReader, error) {
	file, err := os.Open(path.Join(s.dir, fmt.Sprintf("%s-%d", blockID.String(), shardID)))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrNoBlock
		}

		return nil, err
	}

	blockHeader, err := readBlockHeader(file)
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}

	r := &Reader{
		file:        file,
		blockHeader: blockHeader,
		lastReadSegmentHeader: storage.SegmentHeader{
			ID: math.MaxUint32,
		},
	}

	return r, nil
}

// Delete block.
func (s *Storage) Delete(blockID uuid.UUID, shardID uint16) error {
	err := os.Remove(path.Join(s.dir, fmt.Sprintf("%s-%d", blockID.String(), shardID)))
	if err != nil {
		if os.IsNotExist(err) {
			return storage.ErrNoBlock
		}

		return err
	}

	return nil
}

// readBlockHeader read header from block.
func readBlockHeader(reader io.Reader) (head storage.BlockHeader, err error) {
	if err = binary.Read(reader, binary.LittleEndian, &head.FileVersion); err != nil {
		return head, err
	}

	var tenantIDLen uint8
	if err = binary.Read(reader, binary.LittleEndian, &tenantIDLen); err != nil {
		return head, err
	}

	if tenantIDLen > 0 {
		tenantID := make([]byte, tenantIDLen)
		if err = binary.Read(reader, binary.LittleEndian, &tenantID); err != nil {
			return head, err
		}

		head.TenantID = string(tenantID)
	}

	if err = binary.Read(reader, binary.LittleEndian, &head.BlockID); err != nil {
		return head, err
	}

	if err = binary.Read(reader, binary.LittleEndian, &head.ShardID); err != nil {
		return head, err
	}

	if err = binary.Read(reader, binary.LittleEndian, &head.ShardLog); err != nil {
		return head, err
	}

	if err = binary.Read(reader, binary.LittleEndian, &head.SegmentEncodingVersion); err != nil {
		return head, err
	}

	return head, nil
}
