package catalog

import (
	"context"
	"errors"
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
		case <-gc.stop:
			return errors.New("stopped")
		}
	}
}

func (gc *GC) Iterate() {
	logger.Debugf("catalog gc iteration: head started")
	defer func() {
		logger.Debugf("catalog gc iteration: head ended")
	}()

	records, err := gc.catalog.List(
		func(record *Record) bool {
			return record.DeletedAt() == 0
		},
		func(lhs, rhs *Record) bool {
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)
	if err != nil {
		logger.Debugf("catalog gc failed", err)
		return
	}

	for _, record := range records {
		if record.deletedAt != 0 {
			continue
		}
		logger.Debugf("catalog gc iteration: head", record.ID())
		if record.ReferenceCount() > 0 {
			return
		}

		if record.Corrupted() {
			logger.Debugf("catalog gc iteration: head", record.ID(), "corrupted")
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

		logger.Debugf("catalog gc iteration: head", record.ID(), "removed")
	}
}

func (gc *GC) Stop() {
	close(gc.stop)
	<-gc.stopped
}
