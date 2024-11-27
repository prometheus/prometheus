package remotewriter

import (
	"context"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"os"
	"strings"
	"time"
)

type Catalog interface {
	List(filterFn func(record catalog.Record) bool, sortLess func(lhs, rhs catalog.Record) bool) (records []catalog.Record, err error)
}

type destinationWriteLoop struct {
	destination *Destination
	writeLoop   *writeLoop
}

type RemoteWriter struct {
	configQueue chan []DestinationConfig
	catalog     Catalog
}

func New(catalog Catalog) *RemoteWriter {
	return &RemoteWriter{
		catalog:     catalog,
		configQueue: make(chan []DestinationConfig),
	}
}

func (rw *RemoteWriter) Run(ctx context.Context) error {
	writeLoopRunners := make(map[string]*writeLoopRunner)
	defer func() {
		for _, wlr := range writeLoopRunners {
			wlr.stop()
		}
	}()

	destinations := make(map[string]*Destination)

	const gcInterval = time.Minute * 5
	gcTicker := time.NewTicker(gcInterval)
	defer gcTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case configs := <-rw.configQueue:
			for _, config := range configs {
				destination, ok := destinations[config.Name]
				if !ok {
					destination = NewDestination(config)
					wl := newWriteLoop(destination, rw.catalog)
					wlr := newWriteLoopRunner(wl)
					writeLoopRunners[config.Name] = wlr
					destinations[config.Name] = destination
					go wlr.run(ctx)
					continue
				}

				if config.EqualTo(destination.Config()) {
					continue
				}

				wlr := writeLoopRunners[config.Name]
				wlr.stop()
				destination.ResetConfig(config)
				wl := newWriteLoop(destination, rw.catalog)
				wlr = newWriteLoopRunner(wl)
				writeLoopRunners[config.Name] = wlr
				go wlr.run(ctx)
			}

			destinationConfigs := make(map[string]DestinationConfig)
			for _, destinationConfig := range configs {
				destinationConfigs[destinationConfig.Name] = destinationConfig
			}

			for _, destination := range destinations {
				name := destination.Config().Name
				if _, ok := destinationConfigs[name]; !ok {
					wlr := writeLoopRunners[name]
					wlr.stop()
					delete(destinations, name)
					delete(writeLoopRunners, name)
				}
			}

		case <-gcTicker.C:
			if err := rw.gc(destinations); err != nil {
				// todo: log?
			}
		}
	}
}

func (rw *RemoteWriter) ApplyConfig(configs ...DestinationConfig) (err error) {
	// todo: validate?
	select {
	case rw.configQueue <- configs:
		return nil
	case <-time.After(time.Minute):
		return fmt.Errorf("failed to apply remote write configs, timeout")
	}
}

func (rw *RemoteWriter) gc(runners map[string]*Destination) error {
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

		//destination := strings.TrimSuffix(fileName, ".cursor")

		//if _, ok := rw.destinations[destination]; !ok {
		//	if err = os.RemoveAll(fileName); err != nil {
		//		return fmt.Errorf("failed to delete file: %w", err)
		//	}
		//}
	}

	return nil
}

type writeLoopRunner struct {
	wl       *writeLoop
	stopc    chan struct{}
	stoppedc chan struct{}
}

func newWriteLoopRunner(wl *writeLoop) *writeLoopRunner {
	return &writeLoopRunner{
		wl:       wl,
		stopc:    make(chan struct{}),
		stoppedc: make(chan struct{}),
	}
}

func (r *writeLoopRunner) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-r.stopc
		cancel()
	}()
	r.wl.run(ctx)
	close(r.stoppedc)
}

func (r *writeLoopRunner) stop() {
	close(r.stopc)
	<-r.stoppedc
}
