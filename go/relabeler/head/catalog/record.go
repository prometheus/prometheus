package catalog

import (
	"sync"
	"sync/atomic"
)

type Status uint8

const (
	StatusNew Status = iota
	StatusRotated
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
	referenceCounter atomic.Int64
	status           Status // status
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
