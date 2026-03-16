// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pprofutil

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"
)

const (
	collectionInterval = 3 * time.Second
	heapPprofFilename  = "heap.pprof"
)

type HeapPprofCollector struct {
	dir    string
	logger *slog.Logger
	stopc  chan struct{}
	donec  chan struct{}
}

func NewHeapPprofCollector(basePath, name string, logger *slog.Logger) (*HeapPprofCollector, error) {
	dir, err := os.MkdirTemp(basePath, name)
	if err != nil {
		return nil, fmt.Errorf("create pprof output directory under %s: %w", basePath, err)
	}
	return &HeapPprofCollector{
		dir:    dir,
		logger: logger.With("name", name, "dir", dir),
		stopc:  make(chan struct{}),
		donec:  make(chan struct{}),
	}, nil
}

func (d *HeapPprofCollector) Start() {
	go d.run()
	d.logger.Info("Heap pprof collector started")
}

func (d *HeapPprofCollector) Stop() {
	close(d.stopc)
	<-d.donec
	d.logger.Info("Heap pprof collector stopped")
}

func (d *HeapPprofCollector) run() {
	defer close(d.donec)

	// get up-to-date GC statistics.
	runtime.GC()
	if err := d.collect(); err != nil {
		d.logger.Warn("Failed to write heap pprof", "err", err)
	}

	timer := time.NewTimer(collectionInterval)
	defer timer.Stop()

	// Periodically overwrite the heap profile to ensure a recent snapshot
	// is available in case of OOM.
	for {
		select {
		case <-d.stopc:
			// Take a final snapshot.
			runtime.GC()
			if err := d.collect(); err != nil {
				d.logger.Warn("Failed to write heap pprof", "err", err)
			}
			return
		case <-timer.C:
			if err := d.collect(); err != nil {
				d.logger.Warn("Failed to write heap pprof", "err", err)
			}
			timer.Reset(collectionInterval)
		}
	}
}

func (d *HeapPprofCollector) collect() error {
	tmpName := filepath.Join(d.dir, heapPprofFilename+".tmp")
	tmp, err := os.Create(tmpName)
	if err != nil {
		return fmt.Errorf("create temp pprof file: %w", err)
	}

	writeErr := pprof.WriteHeapProfile(tmp)
	closeErr := tmp.Close()

	if writeErr != nil {
		return fmt.Errorf("write heap pprof: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close temp pprof file: %w", closeErr)
	}

	dest := filepath.Join(d.dir, heapPprofFilename)
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("rename pprof file to %s: %w", dest, err)
	}
	return nil
}
