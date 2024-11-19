package remotewriter

import (
	"context"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"os"
	"strings"
	"sync"
)

type Catalog interface {
	List(filterFn func(record catalog.Record) bool, sortLess func(lhs, rhs catalog.Record) bool) (records []catalog.Record, err error)
}

type destinationWriteLoop struct {
	destination *Destination
	writeLoop   *writeLoop
}

type RemoteWriter struct {
	mtx          sync.Mutex
	destinations map[string]*destinationWriteLoop

	catalog Catalog
}

func New(catalog Catalog) *RemoteWriter {
	return &RemoteWriter{
		catalog:      catalog,
		destinations: make(map[string]*destinationWriteLoop),
	}
}

func (rw *RemoteWriter) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (rw *RemoteWriter) Stop() {
	rw.mtx.Lock()
	defer rw.mtx.Unlock()
	for _, destination := range rw.destinations {
		destination.writeLoop.stop()
	}
}

func (rw *RemoteWriter) ApplyConfig(configs ...DestinationConfig) (err error) {
	rw.mtx.Lock()
	defer rw.mtx.Unlock()

	destinationConfigs := make(map[string]DestinationConfig)
	for _, destinationConfig := range configs {
		destinationConfigs[destinationConfig.Name] = destinationConfig
	}

	for name, destinationConfig := range destinationConfigs {
		dwl, ok := rw.destinations[name]
		if !ok {
			destination := NewDestination(destinationConfig)
			wl := newWriteLoop(destination, rw.catalog)
			rw.destinations[name] = &destinationWriteLoop{
				destination: destination,
				writeLoop:   wl,
			}
			go wl.start()
			continue
		}

		if dwl.destination.Config().EqualTo(destinationConfig) {
			continue
		}

		dwl.writeLoop.stop()
		wl := newWriteLoop(dwl.destination, rw.catalog)
		dwl.writeLoop = wl
		go wl.start()
	}

	for name, dwl := range rw.destinations {
		_, ok := destinationConfigs[name]
		if ok {
			continue
		}
		dwl.writeLoop.stop()
		delete(rw.destinations, name)
	}

	return rw.gc()
}

func (rw *RemoteWriter) gc() error {
	headRecords, err := rw.catalog.List(
		func(record catalog.Record) bool {
			return record.Status != catalog.StatusCorrupted
		},
		func(lhs, rhs catalog.Record) bool {
			return lhs.CreatedAt < rhs.CreatedAt
		},
	)

	if err != nil {
		return fmt.Errorf("failed to list head records: %w", err)
	}

	for _, headRecord := range headRecords {
		if err = rw.cleanUpDir(headRecord.Dir); err != nil {
			return fmt.Errorf("failed to clean up dir: %w", err)
		}
	}
	return nil
}

func (rw *RemoteWriter) cleanUpDir(dir string) error {
	dirFile, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("failed to open dir: %w", err)
	}
	defer func() { _ = dirFile.Close() }()

	fileNames, err := dirFile.Readdirnames(-1)
	if err != nil {
		return fmt.Errorf("failed to read dir names: %w", err)
	}

	for _, fileName := range fileNames {
		if !strings.HasSuffix(fileName, ".cursor") {
			continue
		}

		destination := strings.TrimSuffix(fileName, ".cursor")

		if _, ok := rw.destinations[destination]; !ok {
			if err = os.RemoveAll(fileName); err != nil {
				return fmt.Errorf("failed to delete file: %w", err)
			}
		}
	}

	return nil
}
