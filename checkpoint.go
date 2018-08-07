// Copyright 2018 The Prometheus Authors

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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/fileutil"
	"github.com/prometheus/tsdb/wal"
)

// CheckpointStats returns stats about a created checkpoint.
type CheckpointStats struct {
	DroppedSeries     int
	DroppedSamples    int
	DroppedTombstones int
	TotalSeries       int // Processed series including dropped ones.
	TotalSamples      int // Processed samples inlcuding dropped ones.
	TotalTombstones   int // Processed tombstones including dropped ones.
}

// LastCheckpoint returns the directory name of the most recent checkpoint.
// If dir does not contain any checkpoints, ErrNotFound is returned.
func LastCheckpoint(dir string) (string, int, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", 0, err
	}
	// Traverse list backwards since there may be multiple checkpoints left.
	for i := len(files) - 1; i >= 0; i-- {
		fi := files[i]

		if !strings.HasPrefix(fi.Name(), checkpointPrefix) {
			continue
		}
		if !fi.IsDir() {
			return "", 0, errors.Errorf("checkpoint %s is not a directory", fi.Name())
		}
		k, err := strconv.Atoi(fi.Name()[len(checkpointPrefix):])
		if err != nil {
			continue
		}
		return fi.Name(), k, nil
	}
	return "", 0, ErrNotFound
}

// DeleteCheckpoints deletes all checkpoints in dir that have an index
// below n.
func DeleteCheckpoints(dir string, n int) error {
	var errs MultiError

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, fi := range files {
		if !strings.HasPrefix(fi.Name(), checkpointPrefix) {
			continue
		}
		k, err := strconv.Atoi(fi.Name()[len(checkpointPrefix):])
		if err != nil || k >= n {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, fi.Name())); err != nil {
			errs.Add(err)
		}
	}
	return errs.Err()
}

const checkpointPrefix = "checkpoint."

// Checkpoint creates a compacted checkpoint of segments in range [m, n] in the given WAL.
// It includes the most recent checkpoint if it exists.
// All series not satisfying keep and samples below mint are dropped.
//
// The checkpoint is stored in a directory named checkpoint.N in the same
// segmented format as the original WAL itself.
// This makes it easy to read it through the WAL package and concatenate
// it with the original WAL.
//
// Non-critical errors are logged and not returned.
func Checkpoint(logger log.Logger, w *wal.WAL, m, n int, keep func(id uint64) bool, mint int64) (*CheckpointStats, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	stats := &CheckpointStats{}

	var sr io.Reader
	{
		lastFn, k, err := LastCheckpoint(w.Dir())
		if err != nil && err != ErrNotFound {
			return nil, errors.Wrap(err, "find last checkpoint")
		}
		if err == nil {
			if m > k+1 {
				return nil, errors.New("unexpected gap to last checkpoint")
			}
			// Ignore WAL files below the checkpoint. They shouldn't exist to begin with.
			m = k + 1

			last, err := wal.NewSegmentsReader(filepath.Join(w.Dir(), lastFn))
			if err != nil {
				return nil, errors.Wrap(err, "open last checkpoint")
			}
			defer last.Close()
			sr = last
		}

		segsr, err := wal.NewSegmentsRangeReader(w.Dir(), m, n)
		if err != nil {
			return nil, errors.Wrap(err, "create segment reader")
		}
		defer segsr.Close()

		if sr != nil {
			sr = io.MultiReader(sr, segsr)
		} else {
			sr = segsr
		}
	}

	cpdir := filepath.Join(w.Dir(), fmt.Sprintf("checkpoint.%06d", n))
	cpdirtmp := cpdir + ".tmp"

	if err := os.MkdirAll(cpdirtmp, 0777); err != nil {
		return nil, errors.Wrap(err, "create checkpoint dir")
	}
	cp, err := wal.New(nil, nil, cpdirtmp)
	if err != nil {
		return nil, errors.Wrap(err, "open checkpoint")
	}

	r := wal.NewReader(sr)

	var (
		series  []RefSeries
		samples []RefSample
		tstones []Stone
		dec     RecordDecoder
		enc     RecordEncoder
		buf     []byte
		recs    [][]byte
	)
	for r.Next() {
		series, samples, tstones = series[:0], samples[:0], tstones[:0]

		// We don't reset the buffer since we batch up multiple records
		// before writing them to the checkpoint.
		// Remember where the record for this iteration starts.
		start := len(buf)
		rec := r.Record()

		switch dec.Type(rec) {
		case RecordSeries:
			series, err = dec.Series(rec, series)
			if err != nil {
				return nil, errors.Wrap(err, "decode series")
			}
			// Drop irrelevant series in place.
			repl := series[:0]
			for _, s := range series {
				if keep(s.Ref) {
					repl = append(repl, s)
				}
			}
			if len(repl) > 0 {
				buf = enc.Series(repl, buf)
			}
			stats.TotalSeries += len(series)
			stats.DroppedSeries += len(series) - len(repl)

		case RecordSamples:
			samples, err = dec.Samples(rec, samples)
			if err != nil {
				return nil, errors.Wrap(err, "decode samples")
			}
			// Drop irrelevant samples in place.
			repl := samples[:0]
			for _, s := range samples {
				if s.T >= mint {
					repl = append(repl, s)
				}
			}
			if len(repl) > 0 {
				buf = enc.Samples(repl, buf)
			}
			stats.TotalSamples += len(samples)
			stats.DroppedSamples += len(samples) - len(repl)

		case RecordTombstones:
			tstones, err = dec.Tombstones(rec, tstones)
			if err != nil {
				return nil, errors.Wrap(err, "decode deletes")
			}
			// Drop irrelevant tombstones in place.
			repl := tstones[:0]
			for _, s := range tstones {
				for _, iv := range s.intervals {
					if iv.Maxt >= mint {
						repl = append(repl, s)
						break
					}
				}
			}
			if len(repl) > 0 {
				buf = enc.Tombstones(repl, buf)
			}
			stats.TotalTombstones += len(tstones)
			stats.DroppedTombstones += len(tstones) - len(repl)

		default:
			return nil, errors.New("invalid record type")
		}
		if len(buf[start:]) == 0 {
			continue // All contents discarded.
		}
		recs = append(recs, buf[start:])

		// Flush records in 1 MB increments.
		if len(buf) > 1*1024*1024 {
			if err := cp.Log(recs...); err != nil {
				return nil, errors.Wrap(err, "flush records")
			}
			buf, recs = buf[:0], recs[:0]
		}
	}
	// If we hit any corruption during checkpointing, repairing is not an option.
	// The head won't know which series records are lost.
	if r.Err() != nil {
		return nil, errors.Wrap(r.Err(), "read segments")
	}

	// Flush remaining records.
	if err := cp.Log(recs...); err != nil {
		return nil, errors.Wrap(err, "flush records")
	}
	if err := cp.Close(); err != nil {
		return nil, errors.Wrap(err, "close checkpoint")
	}
	if err := fileutil.Replace(cpdirtmp, cpdir); err != nil {
		return nil, errors.Wrap(err, "rename checkpoint directory")
	}
	if err := w.Truncate(n + 1); err != nil {
		// If truncating fails, we'll just try again at the next checkpoint.
		// Leftover segments will just be ignored in the future if there's a checkpoint
		// that supersedes them.
		level.Error(logger).Log("msg", "truncating segments failed", "err", err)
	}
	if err := DeleteCheckpoints(w.Dir(), n); err != nil {
		// Leftover old checkpoints do not cause problems down the line beyond
		// occupying disk space.
		// They will just be ignored since a higher checkpoint exists.
		level.Error(logger).Log("msg", "delete old checkpoints", "err", err)
	}
	return stats, nil
}
