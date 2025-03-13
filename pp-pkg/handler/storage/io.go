package storage

import (
	"errors"

	"github.com/google/uuid"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
)

var (
	ErrNoBlock    = errors.New("no block")
	ErrEndOfBlock = errors.New("end of block")
)

type BlockHeader struct {
	FileVersion            uint8
	TenantID               string
	BlockID                uuid.UUID
	ShardID                uint16
	ShardLog               uint8
	SegmentEncodingVersion uint8
}

type SegmentHeader struct {
	ID    uint32
	Size  uint32
	CRC32 uint32
}

type BlockReader interface {
	Header() BlockHeader
	Next() (model.Segment, error)
	Close() error
}

type BlockWriter interface {
	Header() BlockHeader
	Append(segment model.Segment) error
	Close() error
}
