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

package blocks

import (
	"context"
	"io/ioutil"
	"math"
	"os"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// Writer is interface to write time series into Prometheus blocks.
type Writer interface {
	storage.Appendable

	// Flush writes current data to disk.
	// The block or blocks will contain values accumulated by `Write`.
	Flush() ([]ulid.ULID, error)

	// Close releases all resources.
	Close() error
}

var _ Writer = &TSDBWriter{}

// Writer is a block writer that allows appending and flushing series to disk.
type TSDBWriter struct {
	logger log.Logger
	dir    string

	head   *tsdb.Head
	tmpDir string
}

func durationToMillis(t time.Duration) int64 {
	return int64(t.Seconds() * 1000)
}

// NewTSDBWriter create a new block writer.
//
// The returned writer accumulates all the series in memory until `Flush` is called.
//
// Note that the writer will not check if the target directory exists or
// contains anything at all. It is the caller's responsibility to
// ensure that the resulting blocks do not overlap etc.
// Writer ensures the block flush is atomic (via rename).
func NewTSDBWriter(logger log.Logger, dir string) (*TSDBWriter, error) {
	res := &TSDBWriter{
		logger: logger,
		dir:    dir,
	}
	return res, res.initHead()
}

// initHead creates and initialises a new TSDB head.
// blockSize should be in milliseconds.
func (w *TSDBWriter) initHead(blockSize int64) error {
	logger := w.logger

	tmpDir, err := ioutil.TempDir(os.TempDir(), "head")
	if err != nil {
		return errors.Wrap(err, "create temp dir")
	}
	w.tmpDir = tmpDir

	h, err := tsdb.NewHead(nil, logger, nil, blockSize, w.tmpDir, nil, tsdb.DefaultStripeSize, nil)
	if err != nil {
		return errors.Wrap(err, "tsdb.NewHead")
	}

	w.head = h
	return w.head.Init(math.MinInt64)
}

// Appender returns a new appender on the database.
// Appender can't be called concurrently. However, the returned Appender can safely be used concurrently.
func (w *TSDBWriter) Appender() storage.Appender {
	return w.head.Appender()
}

// Flush implements the Writer interface. This is where actual block writing
// happens. After flush completes, no writes can be done.
func (w *TSDBWriter) Flush() ([]ulid.ULID, error) {
	seriesCount := w.head.NumSeries()
	if w.head.NumSeries() == 0 {
		return nil, errors.New("no series appended; aborting.")
	}

	mint := w.head.MinTime()
	maxt := w.head.MaxTime()
	level.Info(w.logger).Log("msg", "flushing", "series_count", seriesCount, "mint", timestamp.Time(mint), "maxt", timestamp.Time(maxt))

	compactor, err := tsdb.NewLeveledCompactor(
		context.Background(),
		nil,
		w.logger,
		[]int64{durationToMillis(2 * time.Hour)},
		chunkenc.NewPool())
	if err != nil {
		return nil, errors.Wrap(err, "create leveled compactor")
	}
	id, err := compactor.Write(w.dir, w.head, mint, maxt, nil)
	if err != nil {
		return nil, errors.Wrap(err, "compactor write")
	}

	return []ulid.ULID{id}, nil
}

func (w *TSDBWriter) Close() error {
	_ = os.RemoveAll(w.tmpDir)
	return w.head.Close()
}
