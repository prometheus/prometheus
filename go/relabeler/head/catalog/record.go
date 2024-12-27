package catalog

import (
	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/util/optional"
	"sync"
	"sync/atomic"
)

type Status uint8

const (
	StatusNew Status = iota
	StatusRotated
	// Deprecated
	StatusCorrupted
	StatusPersisted
	StatusActive
)

type Record struct {
	id                    uuid.UUID // uuid
	numberOfShards        uint16    // number of shards
	createdAt             int64     // time of record creation
	updatedAt             int64
	deletedAt             int64
	corrupted             bool
	lastAppendedSegmentID optional.Optional[uint32]
	referenceCount        int64
	status                Status // status
}

func NewRecord() *Record {
	return &Record{}
}

func (r *Record) ID() string {
	return r.id.String()
}

func (r *Record) Dir() string {
	return r.id.String()
}

func (r *Record) NumberOfShards() uint16 {
	return r.numberOfShards
}

func (r *Record) CreatedAt() int64 {
	return r.createdAt
}

func (r *Record) UpdatedAt() int64 {
	return r.updatedAt
}

func (r *Record) DeletedAt() int64 {
	return r.deletedAt
}

func (r *Record) Corrupted() bool {
	return r.corrupted
}

func (r *Record) Status() Status {
	return r.status
}

func (r *Record) ReferenceCount() int64 {
	return atomic.LoadInt64(&r.referenceCount)
}

func (r *Record) Acquire() func() {
	atomic.AddInt64(&r.referenceCount, 1)
	var onceRelease sync.Once
	return func() {
		onceRelease.Do(func() {
			atomic.AddInt64(&r.referenceCount, -1)
		})
	}
}

func (r *Record) LastAppendedSegmentID() *uint32 {
	return r.lastAppendedSegmentID.RawValue()
}

func (r *Record) SetLastAppendedSegmentID(segmentID uint32) {
	r.lastAppendedSegmentID.Set(segmentID)
}

func NewRecordWithData(id uuid.UUID,
	numberOfShards uint16,
	createdAt int64,
	updatedAt int64,
	deletedAt int64,
	corrupted bool,
	referenceCount int64,
	status Status,
	lastAppendedSegmentID *uint32,
) *Record {
	return &Record{
		id:                    id,
		numberOfShards:        numberOfShards,
		createdAt:             createdAt,
		updatedAt:             updatedAt,
		deletedAt:             deletedAt,
		corrupted:             corrupted,
		referenceCount:        referenceCount,
		status:                status,
		lastAppendedSegmentID: optional.WithRawValue(lastAppendedSegmentID),
	}
}
