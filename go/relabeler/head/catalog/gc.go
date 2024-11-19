package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

type GC struct {
	catalog *Catalog
}

func NewGC(catalog *Catalog) *GC {
	return &GC{
		catalog: catalog,
	}
}

func (gc *GC) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Minute):
			gc.run()
		}
	}
}

func (gc *GC) run() {
	records, err := gc.catalog.List(
		func(record Record) bool {
			return record.Status == StatusPersisted
		},
		func(lhs, rhs Record) bool {
			return lhs.CreatedAt < rhs.CreatedAt
		},
	)
	if err != nil {
		fmt.Println("catalog gc failed", err)
		return
	}

	for _, record := range records {
		var deletable bool
		deletable, err = isDeletable(record)
		if err != nil {
			fmt.Println("catalog gc failed", err)
			return
		}
		if !deletable {
			return
		}
		if err = os.RemoveAll(record.Dir); err != nil {
			fmt.Println("catalog gc failed", err)
			return
		}
		if err = gc.catalog.Delete(record.ID); err != nil {
			fmt.Println("catalog gc failed", err)
			return
		}
	}
}

func isDeletable(record Record) (bool, error) {
	dir, err := os.Open(record.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	defer func() { _ = dir.Close() }()

	files, err := dir.Readdirnames(-1)
	if err != nil {
		return false, err
	}

	return len(files) == int(record.NumberOfShards), nil
}
