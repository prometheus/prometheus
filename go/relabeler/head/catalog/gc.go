package catalog

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/ready"
	"os"
	"time"
)

type GC struct {
	catalog         *Catalog
	readyNotifiable ready.Notifiable
	targetRecordID  *string
}

func NewGC(catalog *Catalog, readyNotifiable ready.Notifiable) *GC {
	return &GC{
		catalog:         catalog,
		readyNotifiable: readyNotifiable,
	}
}

func (gc *GC) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-gc.readyNotifiable.ReadyChan():
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Minute):
			gc.run()
		}
	}
}

func (gc *GC) run() {
	records, err := gc.catalog.List(
		nil,
		func(lhs, rhs *Record) bool {
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)
	if err != nil {
		fmt.Println("catalog gc failed", err)
		return
	}

	for _, record := range records {
		if gc.targetRecordID == nil {
			recordID := record.ID()
			gc.targetRecordID = &recordID
		}
		if *gc.targetRecordID == record.ID() {

		}

	}
}

func isDeletable(record *Record) (bool, error) {
	dir, err := os.Open(record.Dir())
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

	return len(files) == int(record.NumberOfShards()), nil
}
