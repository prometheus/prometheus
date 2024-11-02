package catalog

import (
	"errors"
	"fmt"
	"github.com/jonboulle/clockwork"
	"io"
	"sort"
)

const (
	logFileName    = "head.log"
	MaxLogFileSize = 32 * 1024
)

type Status uint8

const (
	StatusNew Status = iota
	StatusRotated
	StatusCorrupted
	StatusPersisted
)

type Record struct {
	ID             string // uuid
	Dir            string // location
	NumberOfShards uint16 // number of shards
	CreatedAt      int64  // time of record creation
	UpdatedAt      int64
	DeletedAt      int64
	Status         Status // status
}

type Log interface {
	Write(record Record) error
	ReWrite(records ...Record) error
	Read(record *Record) error
	Size() int
}

type Catalog struct {
	clock   clockwork.Clock
	log     Log
	records map[string]Record
}

func New(clock clockwork.Clock, log Log) (*Catalog, error) {
	catalog := &Catalog{
		clock:   clock,
		log:     log,
		records: make(map[string]Record),
	}

	if err := catalog.sync(); err != nil {
		return nil, fmt.Errorf("faield to sync catalog: %w", err)
	}

	return catalog, nil
}

func (c *Catalog) List(filterFn func(record Record) bool, sortLess func(lhs, rhs Record) bool) (records []Record, err error) {
	records = make([]Record, 0, len(c.records))
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

func (c *Catalog) Create(id, dir string, numberOfShards uint16) (r Record, err error) {
	if _, ok := c.records[id]; ok {
		return r, fmt.Errorf("already exists: %s", id)
	}

	r.ID = id
	r.Dir = dir
	r.NumberOfShards = numberOfShards
	r.CreatedAt = c.clock.Now().UnixMilli()
	r.UpdatedAt = r.CreatedAt
	r.Status = StatusNew
	if err = c.log.Write(r); err != nil {
		return r, fmt.Errorf("failed to write log: %w", err)
	}
	c.records[id] = r

	return r, c.compactIfNeeded()
}

func (c *Catalog) SetStatus(id string, status Status) (Record, error) {
	r, ok := c.records[id]
	if !ok {
		return r, fmt.Errorf("not found: %s", id)
	}

	r.Status = status
	r.UpdatedAt = c.clock.Now().UnixMilli()

	if err := c.log.Write(r); err != nil {
		return r, fmt.Errorf("failed to write log: %w", err)
	}

	c.records[id] = r

	return r, c.compactIfNeeded()
}

func (c *Catalog) Delete(id string) error {
	r, ok := c.records[id]
	if !ok || r.DeletedAt > 0 {
		return nil
	}

	r.DeletedAt = c.clock.Now().UnixMilli()
	r.UpdatedAt = r.DeletedAt

	if err := c.log.Write(r); err != nil {
		return fmt.Errorf("failed to write log: %w", err)
	}

	delete(c.records, r.ID)

	return c.compactIfNeeded()
}

func (c *Catalog) Compact() error {
	records := make([]Record, 0, len(c.records))
	for _, record := range c.records {
		records = append(records, record)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt < records[j].CreatedAt
	})

	return c.log.ReWrite(records...)
}

func (c *Catalog) sync() error {
	var r Record
	var err error
	for {
		err = c.log.Read(&r)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return fmt.Errorf("failed to read log: %w", err)
			}
			return nil
		}
		c.records[r.ID] = r
	}
}

func (c *Catalog) compactIfNeeded() error {
	if c.log.Size() < MaxLogFileSize {
		return nil
	}

	return c.Compact()
}
