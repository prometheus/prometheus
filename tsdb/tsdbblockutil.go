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

package tsdb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

var ErrInvalidTimes = errors.New("max time is lesser than min time")

// CreateBlock creates a chunkrange block from the samples passed to it, and writes it to disk.
func CreateBlock(series []storage.Series, dir string, chunkRange int64, logger *slog.Logger) (string, error) {
	if chunkRange == 0 {
		chunkRange = DefaultBlockDuration
	}
	if chunkRange < 0 {
		return "", ErrInvalidTimes
	}

	w, err := NewBlockWriter(logger, dir, chunkRange)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := w.Close(); err != nil {
			logger.Error("err closing blockwriter", "err", err.Error())
		}
	}()

	sampleCount := 0
	const commitAfter = 10000
	ctx := context.Background()
	app := w.Appender(ctx)
	var it chunkenc.Iterator

	for _, s := range series {
		ref := storage.SeriesRef(0)
		it = s.Iterator(it)
		lset := s.Labels()
		typ := it.Next()
		lastTyp := typ
		for ; typ != chunkenc.ValNone; typ = it.Next() {
			if lastTyp != typ {
				// The behaviour of appender is undefined if samples of different types
				// are appended to the same series in a single Commit().
				if err = app.Commit(); err != nil {
					return "", err
				}
				app = w.Appender(ctx)
				sampleCount = 0
			}

			switch typ {
			case chunkenc.ValFloat:
				t, v := it.At()
				ref, err = app.Append(ref, lset, t, v)
			case chunkenc.ValHistogram:
				t, h := it.AtHistogram(nil)
				ref, err = app.AppendHistogram(ref, lset, t, h, nil)
			case chunkenc.ValFloatHistogram:
				t, fh := it.AtFloatHistogram(nil)
				ref, err = app.AppendHistogram(ref, lset, t, nil, fh)
			default:
				return "", fmt.Errorf("unknown sample type %s", typ.String())
			}
			if err != nil {
				return "", err
			}
			sampleCount++
			lastTyp = typ
		}
		if it.Err() != nil {
			return "", it.Err()
		}
		// Commit and make a new appender periodically, to avoid building up data in memory.
		if sampleCount > commitAfter {
			if err = app.Commit(); err != nil {
				return "", err
			}
			app = w.Appender(ctx)
			sampleCount = 0
		}
	}

	if err = app.Commit(); err != nil {
		return "", err
	}

	ulid, err := w.Flush(ctx)
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, ulid.String()), nil
}
