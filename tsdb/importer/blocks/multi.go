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
	"github.com/go-kit/kit/log"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/index"
)

type errAppender struct{ err error }

func (a errAppender) Add(l labels.Labels, t int64, v float64) (uint64, error) { return 0, a.err }
func (a errAppender) AddFast(ref uint64, t int64, v float64) error            { return a.err }
func (a errAppender) Commit() error                                           { return a.err }
func (a errAppender) Rollback() error                                         { return a.err }

func rangeForTimestamp(t int64, width int64) (maxt int64) {
	return (t/width)*width + width
}

type MultiWriter struct {
	blocks          map[index.Range]Writer
	activeAppenders map[index.Range]storage.Appender

	logger log.Logger
	dir    string
	// TODO(bwplotka): Allow more complex compaction levels.
	sizeMillis int64
}

func NewMultiWriter(logger log.Logger, dir string, sizeMillis int64) *MultiWriter {
	return &MultiWriter{
		logger:          logger,
		dir:             dir,
		sizeMillis:      sizeMillis,
		blocks:          map[index.Range]Writer{},
		activeAppenders: map[index.Range]storage.Appender{},
	}
}

// Appender is not thread-safe. Returned Appender is not thread-save as well.
// TODO(bwplotka): Consider making it thread safe.
func (w *MultiWriter) Appender() storage.Appender { return w }

func (w *MultiWriter) getOrCreate(t int64) storage.Appender {
	maxt := rangeForTimestamp(t, w.sizeMillis)
	hash := index.Range{Start: maxt - w.sizeMillis, End: maxt}
	if a, ok := w.activeAppenders[hash]; ok {
		return a
	}

	nw, err := NewTSDBWriter(w.logger, w.dir)
	if err != nil {
		return errAppender{err: errors.Wrap(err, "new tsdb writer")}
	}

	w.blocks[hash] = nw
	w.activeAppenders[hash] = nw.Appender()
	return w.activeAppenders[hash]
}

func (w *MultiWriter) Add(l labels.Labels, t int64, v float64) (uint64, error) {
	return w.getOrCreate(t).Add(l, t, v)
}

func (w *MultiWriter) AddFast(ref uint64, t int64, v float64) error {
	return w.getOrCreate(t).AddFast(ref, t, v)
}

func (w *MultiWriter) Commit() error {
	var merr tsdb_errors.MultiError
	for _, a := range w.activeAppenders {
		merr.Add(a.Commit())
	}
	return merr.Err()
}

func (w *MultiWriter) Rollback() error {
	var merr tsdb_errors.MultiError
	for _, a := range w.activeAppenders {
		merr.Add(a.Rollback())
	}
	return merr.Err()
}

func (w *MultiWriter) Flush() ([]ulid.ULID, error) {
	ids := make([]ulid.ULID, 0, len(w.blocks))
	for _, b := range w.blocks {
		id, err := b.Flush()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id...)
	}
	return ids, nil
}

func (w *MultiWriter) Close() error {
	var merr tsdb_errors.MultiError
	for _, b := range w.blocks {
		merr.Add(b.Close())
	}
	return merr.Err()
}
