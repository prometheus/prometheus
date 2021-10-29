// Copyright 2021 The Prometheus Authors

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

package tsdb

import (
	"context"
	"io"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

// LazyReader represents a directory of time series data covering a continuous time range.
type LazyReader struct {
	ctx    context.Context
	mtx    sync.RWMutex
	logger log.Logger

	dir  string
	pool chunkenc.Pool

	chunkr     ChunkReader
	indexr     IndexReader
	tombstones tombstones.Reader
	stats      ReaderStats

	lastUsed      atomic.Time
	unmapAfter    time.Duration
	checkInterval time.Duration
	isMMapped     atomic.Bool
}

type ReaderStats struct {
	symbolTableSize   uint64
	numBytesChunks    int64
	numBytesIndex     int64
	numBytesTombstone int64
}

// NewLazyReader opens block readers and mmaps them when block contents are called. It unmaps the readers when
// lastUsed >= unmapAfter, every checkInterval.
// The provided ctx must be cancelled while closing a block to avoid stalls in graceful shutdowns.
func NewLazyReader(ctx context.Context, logger log.Logger, dir string, unmapAfter time.Duration, checkInterval time.Duration, pool chunkenc.Pool) *LazyReader {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	return &LazyReader{
		ctx:           ctx,
		dir:           dir,
		logger:        logger,
		unmapAfter:    unmapAfter,
		checkInterval: checkInterval,
		pool:          pool,
	}
}

func (lb *LazyReader) Index() (IndexReader, error) {
	if err := lb.load(); err != nil {
		return nil, errors.Wrap(err, "load")
	}
	return lb.indexr, nil
}

func (lb *LazyReader) Chunks() (ChunkReader, error) {
	if err := lb.load(); err != nil {
		return nil, errors.Wrap(err, "load")
	}
	return lb.chunkr, nil
}

func (lb *LazyReader) Tombstones() (tombstones.Reader, error) {
	if err := lb.load(); err != nil {
		return nil, errors.Wrap(err, "load")
	}
	return lb.tombstones, nil
}

func (lb *LazyReader) UpdateTombstones(stones tombstones.Reader) error {
	if err := lb.load(); err != nil {
		return errors.Wrap(err, "load")
	}
	lb.tombstones = stones
	return nil
}

func (lb *LazyReader) Stats() ReaderStats {
	lb.mtx.RLock()
	defer lb.mtx.RUnlock()
	return lb.stats
}

// load the block contents into (mmap) memory. It is go-routine safe.
func (lb *LazyReader) load() (err error) {
	lb.mtx.Lock()
	defer lb.mtx.Unlock()

	lb.lastUsed.Store(time.Now())
	if lb.isMMapped.Load() {
		// Readers already unmapped, skip.
		return nil
	}

	var closers []io.Closer
	defer func() {
		if err != nil {
			err = tsdb_errors.NewMulti(err, tsdb_errors.CloseAll(closers)).Err()
		}
	}()

	ir, err := index.NewFileReader(filepath.Join(lb.dir, indexFilename))
	if err != nil {
		return err
	}
	closers = append(closers, ir)

	cr, err := chunks.NewDirReader(chunkDir(lb.dir), lb.pool)
	if err != nil {
		return err
	}
	closers = append(closers, cr)

	tr, sizeTomb, err := tombstones.ReadTombstones(lb.dir)
	if err != nil {
		return err
	}

	lb.indexr = ir
	lb.chunkr = cr
	lb.tombstones = tr

	// Update size whenever we load.
	lb.stats.numBytesIndex = ir.Size()
	lb.stats.symbolTableSize = ir.SymbolTableSize()
	lb.stats.numBytesChunks = cr.Size()
	lb.stats.numBytesTombstone = sizeTomb

	closers = append(closers, tr)

	lb.lastUsed.Store(time.Now()) // Update last used as loading block contents can take time.
	lb.isMMapped.Store(true)

	go lb.unmapRoutine()

	return nil
}

// unmapRoutine checks if the lastUsed >= unmapAfter and unmaps the block and exits.
func (lb *LazyReader) unmapRoutine() {
	if !lb.isMMapped.Load() {
		// Readers not mmapped, hence watching last used is of no use.
		return
	}
	check := time.NewTicker(lb.checkInterval)
	defer check.Stop()

	lb.isMMapped.Store(true)

	unmap := func() {
		lb.mtx.Lock()
		defer lb.mtx.Unlock()
		if err := lb.chunkr.Close(); err != nil {
			level.Error(lb.logger).Log("msg", "error unmapping chunk reader", "err", err)
		}
		if err := lb.indexr.Close(); err != nil {
			level.Error(lb.logger).Log("msg", "error unmapping index reader", "err", err)
		}
		if err := lb.tombstones.Close(); err != nil {
			level.Error(lb.logger).Log("msg", "error unmapping tombstones", "err", err)
		}
		lb.isMMapped.Store(false)
	}

	for {
		select {
		case <-lb.ctx.Done():
			// Return to avoid stalls in graceful shutdowns.
			unmap()
			return
		case <-check.C:
		}

		if time.Since(lb.lastUsed.Load()) >= lb.unmapAfter {
			unmap()
			return
		}
	}
}
