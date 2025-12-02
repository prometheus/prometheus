// Copyright 2020 The Prometheus Authors
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
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"

	"github.com/oklog/ulid/v2"

	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// BlockWriter is a block writer that allows appending and flushing series to disk.
type BlockWriter struct {
	logger         *slog.Logger
	destinationDir string

	head      *Head
	blockSize int64 // in ms
	chunkDir  string
}

// ErrNoSeriesAppended is returned if the series count is zero while flushing blocks.
var ErrNoSeriesAppended = errors.New("no series appended, aborting")

// NewBlockWriter creates a new block writer.
//
// The returned writer accumulates all the series in the Head block until `Flush` is called.
//
// Note that the writer will not check if the target directory exists or
// contains anything at all. It is the caller's responsibility to
// ensure that the resulting blocks do not overlap etc.
// Writer ensures the block flush is atomic (via rename).
func NewBlockWriter(logger *slog.Logger, dir string, blockSize int64) (*BlockWriter, error) {
	w := &BlockWriter{
		logger:         logger,
		destinationDir: dir,
		blockSize:      blockSize,
	}
	if err := w.initHead(); err != nil {
		return nil, err
	}
	return w, nil
}

// initHead creates and initialises a new TSDB head.
func (w *BlockWriter) initHead() error {
	chunkDir, err := os.MkdirTemp(os.TempDir(), "head")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	w.chunkDir = chunkDir
	opts := DefaultHeadOptions()
	opts.ChunkRange = w.blockSize
	opts.ChunkDirRoot = w.chunkDir
	h, err := NewHead(nil, w.logger, nil, nil, opts, NewHeadStats())
	if err != nil {
		return fmt.Errorf("tsdb.NewHead: %w", err)
	}

	w.head = h
	return w.head.Init(math.MinInt64)
}

// Appender returns a new appender on the database.
// Appender can't be called concurrently. However, the returned Appender can safely be used concurrently.
func (w *BlockWriter) Appender(ctx context.Context) storage.Appender {
	return w.head.Appender(ctx)
}

// AppenderV2 returns a new appender on the database.
// AppenderV2 can't be called concurrently. However, the returned AppenderV2 can safely be used concurrently.
func (w *BlockWriter) AppenderV2(ctx context.Context) storage.AppenderV2 {
	return w.head.AppenderV2(ctx)
}

// Flush implements the Writer interface. This is where actual block writing
// happens. After flush completes, no writes can be done.
func (w *BlockWriter) Flush(ctx context.Context) (ulid.ULID, error) {
	mint := w.head.MinTime()
	// Add +1 millisecond to block maxt because block intervals are half-open: [b.MinTime, b.MaxTime).
	// Because of this block intervals are always +1 than the total samples it includes.
	maxt := w.head.MaxTime() + 1
	w.logger.Info("flushing", "series_count", w.head.NumSeries(), "mint", timestamp.Time(mint), "maxt", timestamp.Time(maxt))

	compactor, err := NewLeveledCompactor(ctx,
		nil,
		w.logger,
		[]int64{w.blockSize},
		chunkenc.NewPool(), nil)
	if err != nil {
		return ulid.ULID{}, fmt.Errorf("create leveled compactor: %w", err)
	}
	ids, err := compactor.Write(w.destinationDir, w.head, mint, maxt, nil)
	if err != nil {
		return ulid.ULID{}, fmt.Errorf("compactor write: %w", err)
	}

	// No block was produced. Caller is responsible to check empty
	// ulid.ULID based on its use case.
	if len(ids) == 0 {
		return ulid.ULID{}, nil
	}
	return ids[0], nil
}

func (w *BlockWriter) Close() error {
	defer func() {
		if err := os.RemoveAll(w.chunkDir); err != nil {
			w.logger.Error("error in deleting BlockWriter files", "err", err)
		}
	}()
	return w.head.Close()
}
