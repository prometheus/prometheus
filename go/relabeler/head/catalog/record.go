package catalog

import (
	"github.com/google/uuid"
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
)

// size (1) + 16 +2 + 8 + 8 + 8 + 1 + 1 + (1 + 4)
type Record struct {
	id                    uuid.UUID // uuid
	numberOfShards        uint16    // number of shards
	createdAt             int64     // time of record creation
	updatedAt             int64
	deletedAt             int64
	corrupted             bool
	lastAppendedSegmentID *uint32
	referenceCounter      *atomic.Int64
	status                Status // status
}

func NewRecord() *Record {
	return &Record{
		referenceCounter: &atomic.Int64{},
	}
}

func (r *Record) ID() uuid.UUID {
	return r.id
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
	return r.referenceCounter.Load()
}

func (r *Record) Acquire() func() {
	r.referenceCounter.Add(1)
	var onceRelease sync.Once
	return func() {
		onceRelease.Do(func() {
			r.referenceCounter.Add(-1)
		})
	}
}

func (r *Record) LastAppendedSegmentID() *uint32 {
	return r.lastAppendedSegmentID
}

func (r *Record) SetLastAppendedSegmentID(segmentID uint32) {
	r.lastAppendedSegmentID = &segmentID
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
	rc := atomic.Int64{}
	rc.Add(referenceCount)
	return &Record{
		id:                    id,
		numberOfShards:        numberOfShards,
		createdAt:             createdAt,
		updatedAt:             updatedAt,
		deletedAt:             deletedAt,
		corrupted:             corrupted,
		referenceCounter:      &rc,
		status:                status,
		lastAppendedSegmentID: lastAppendedSegmentID,
	}
}
