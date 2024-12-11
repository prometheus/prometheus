package catalog

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"io"
	"sort"
	"sync"
	"sync/atomic"
)

const (
	logFileName    = "head.log"
	MaxLogFileSize = 32 * 1024
)

type Log interface {
	Write(record *Record) error
	ReWrite(records ...*Record) error
	Read(record *Record) error
	Size() int
}

type Catalog struct {
	mtx     sync.Mutex
	clock   clockwork.Clock
	log     Log
	records map[uuid.UUID]*Record
}

func New(clock clockwork.Clock, log Log) (*Catalog, error) {
	catalog := &Catalog{
		clock:   clock,
		log:     log,
		records: make(map[uuid.UUID]*Record),
	}

	if err := catalog.sync(); err != nil {
		return nil, fmt.Errorf("faield to sync catalog: %w", err)
	}

	return catalog, nil
}

func (c *Catalog) List(filterFn func(record *Record) bool, sortLess func(lhs, rhs *Record) bool) (records []*Record, err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	records = make([]*Record, 0, len(c.records))
	for _, record := range c.records {
		if filterFn != nil && !filterFn(record) {
			continue
		}
		records = append(records, record)
	}

	if sortLess != nil {
		sort.Slice(records, func(i, j int) bool {
			return sortLess(records[i], records[j])
		})
	}

	return records, nil
}

func (c *Catalog) Create(id uuid.UUID, numberOfShards uint16) (r *Record, err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if _, ok := c.records[id]; ok {
		return r, fmt.Errorf("already exists: %s", id)
	}

	now := c.clock.Now().UnixMilli()
	r = &Record{
		id:               id,
		numberOfShards:   numberOfShards,
		createdAt:        now,
		updatedAt:        now,
		deletedAt:        0,
		referenceCounter: &atomic.Int64{},
		status:           StatusNew,
	}

	if err = c.log.Write(r); err != nil {
		return r, fmt.Errorf("failed to write log: %w", err)
	}
	c.records[id] = r

	return r, c.compactIfNeeded()
}

func (c *Catalog) Get(id uuid.UUID) (*Record, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	r, ok := c.records[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}

	return r, nil
}

func (c *Catalog) SetStatus(id uuid.UUID, status Status) (*Record, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	r, ok := c.records[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}

	r.status = status
	r.updatedAt = c.clock.Now().UnixMilli()

	if err := c.log.Write(r); err != nil {
		return nil, fmt.Errorf("failed to write log: %w", err)
	}

	c.records[id] = r

	return r, c.compactIfNeeded()
}

func (c *Catalog) SetCorrupted(id uuid.UUID) (*Record, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	r, ok := c.records[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}

	if r.corrupted {
		return r, nil
	}

	r.corrupted = true
	r.updatedAt = c.clock.Now().UnixMilli()

	if err := c.log.Write(r); err != nil {
		return nil, fmt.Errorf("failed to write log: %w", err)
	}

	c.records[id] = r

	return r, c.compactIfNeeded()
}

func (c *Catalog) Delete(id uuid.UUID) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	r, ok := c.records[id]
	if !ok || r.deletedAt > 0 {
		return nil
	}

	r.deletedAt = c.clock.Now().UnixMilli()
	r.updatedAt = r.deletedAt

	if err := c.log.Write(r); err != nil {
		return fmt.Errorf("failed to write log: %w", err)
	}

	delete(c.records, r.id)

	return c.compactIfNeeded()
}

func (c *Catalog) Compact() error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.compact()
}

func (c *Catalog) sync() error {
	for {
		r := NewRecord()
		var err error
		err = c.log.Read(r)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return fmt.Errorf("failed to read log: %w", err)
			}
			return nil
		}
		c.records[r.id] = r
	}
}

func (c *Catalog) compactIfNeeded() error {
	if c.log.Size() < MaxLogFileSize {
		return nil
	}

	return c.compact()
}

func (c *Catalog) compact() error {
	records := make([]*Record, 0, len(c.records))
	for _, record := range c.records {
		if record.deletedAt == 0 {
			records = append(records, record)
		}
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].createdAt < records[j].createdAt
	})

	return c.log.ReWrite(records...)
}
