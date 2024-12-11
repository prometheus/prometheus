package remotewriter

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/ready"
	"time"
)

type Catalog interface {
	List(filterFn func(record *catalog.Record) bool, sortLess func(lhs, rhs *catalog.Record) bool) (records []*catalog.Record, err error)
	SetCorrupted(id uuid.UUID) (*catalog.Record, error)
}

type RemoteWriter struct {
	dataDir       string
	configQueue   chan []DestinationConfig
	catalog       Catalog
	clock         clockwork.Clock
	readyNotifier ready.Notifier
}

func New(dataDir string, catalog Catalog, clock clockwork.Clock, readyNotifier ready.Notifier) *RemoteWriter {
	return &RemoteWriter{
		dataDir:       dataDir,
		catalog:       catalog,
		clock:         clock,
		configQueue:   make(chan []DestinationConfig),
		readyNotifier: readyNotifier,
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

			for _, config := range configs {
				destination, ok := destinations[config.Name]
				if !ok {
					destination = NewDestination(config)
					wl := newWriteLoop(rw.dataDir, destination, rw.catalog, rw.clock)
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
				wl := newWriteLoop(rw.dataDir, destination, rw.catalog, rw.clock)
				wlr = newWriteLoopRunner(wl)
				writeLoopRunners[config.Name] = wlr
				go wlr.run(ctx)
			}
			rw.readyNotifier.NotifyReady()
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
