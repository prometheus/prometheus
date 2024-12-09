package catalog

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/ready"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"os"
	"path/filepath"
	"time"
)

type GC struct {
	dataDir         string
	catalog         *Catalog
	readyNotifiable ready.Notifiable
	stop            chan struct{}
	stopped         chan struct{}
}

func NewGC(dataDir string, catalog *Catalog, readyNotifiable ready.Notifiable) *GC {
	return &GC{
		dataDir:         dataDir,
		catalog:         catalog,
		readyNotifiable: readyNotifiable,
		stop:            make(chan struct{}),
		stopped:         make(chan struct{}),
	}
}

func (gc *GC) Run(ctx context.Context) error {
	defer close(gc.stopped)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-gc.readyNotifiable.ReadyChan():
		case <-gc.stop:
			return errors.New("stopped")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Minute):
			gc.Iterate()
		case <-gc.stopped:
			return errors.New("stopped")
		}
	}
}

func (gc *GC) Iterate() {
	fmt.Println("CATALOG GC ITERATION STARTED")
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
		fmt.Println("CATALOG GC ITERATION: HEAD", record.ID())
		if record.ReferenceCount() > 0 {
			return
		}

		if record.Status() == StatusCorrupted {
			fmt.Println("CATALOG GC ITERATION: HEAD", record.ID(), "CORRUPTED")
			continue
		}

		if err = os.RemoveAll(filepath.Join(gc.dataDir, record.Dir())); err != nil {
			logger.Errorf("failed to remote head dir: %w", err)
			return
		}

		if err = gc.catalog.Delete(record.ID()); err != nil {
			logger.Errorf("failed to delete head record: %w", err)
			return
		}

		fmt.Println("CATALOG GC ITERATION: HEAD", record.ID(), "REMOVED")
	}
}

func (gc *GC) Stop() {
	close(gc.stop)
	<-gc.stopped
}
