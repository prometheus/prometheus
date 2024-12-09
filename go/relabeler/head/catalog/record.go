package catalog

import (
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

type Record struct {
	id               string // uuid
	dir              string // location
	numberOfShards   uint16 // number of shards
	createdAt        int64  // time of record creation
	updatedAt        int64
	deletedAt        int64
	corrupted        bool
	referenceCounter *atomic.Int64
	status           Status // status
}

func NewRecord() *Record {
	return &Record{
		referenceCounter: &atomic.Int64{},
	}
}

func (r *Record) ID() string {
	return r.id
}

func (r *Record) Dir() string {
	return r.dir
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

func NewRecordWithData(id string,
	dir string,
	numberOfShards uint16,
	createdAt int64,
	updatedAt int64,
	deletedAt int64,
	corrupted bool,
	referenceCount int64,
	status Status) *Record {
	rc := atomic.Int64{}
	rc.Add(referenceCount)
	return &Record{
		id:               id,
		dir:              dir,
		numberOfShards:   numberOfShards,
		createdAt:        createdAt,
		updatedAt:        updatedAt,
		deletedAt:        deletedAt,
		corrupted:        corrupted,
		referenceCounter: &rc,
		status:           status,
	}
}
