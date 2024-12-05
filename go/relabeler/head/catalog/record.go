package catalog

import "sync/atomic"

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

func (r *Record) ReferenceCounter() *ReferenceCounter {
	return NewReferenceCounter(&r.referenceCounter)
}

func NewReferenceCounter(global *atomic.Int64) *ReferenceCounter {
	return &ReferenceCounter{
		local:  atomic.Int64{},
		global: global,
	}
}

type ReferenceCounter struct {
	local  atomic.Int64
	global *atomic.Int64
}

func (rc *ReferenceCounter) Inc() int64 {
	if rc.local.Add(1) > 0 {
		return rc.global.Add(1)
	}

	return rc.global.Load()
}

func (rc *ReferenceCounter) Dec() int64 {
	if rc.local.Add(-1) >= 0 {
		return rc.global.Add(-1)
	}
	return rc.global.Load()
}

func (rc *ReferenceCounter) Value() int64 {
	return rc.global.Load()
}
