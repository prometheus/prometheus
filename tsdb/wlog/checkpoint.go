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

package wlog

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

// CheckpointStats returns stats about a created checkpoint.
type CheckpointStats struct {
	DroppedSeries     int
	DroppedSamples    int // Includes histograms.
	DroppedTombstones int
	DroppedExemplars  int
	DroppedMetadata   int
	TotalSeries       int // Processed series including dropped ones.
	TotalSamples      int // Processed float and histogram samples including dropped ones.
	TotalTombstones   int // Processed tombstones including dropped ones.
	TotalExemplars    int // Processed exemplars including dropped ones.
	TotalMetadata     int // Processed metadata including dropped ones.
}

// LastCheckpoint returns the directory name and index of the most recent checkpoint.
// If dir does not contain any checkpoints, ErrNotFound is returned.
func LastCheckpoint(dir string) (string, int, error) {
	checkpoints, err := listCheckpoints(dir)
	if err != nil {
		return "", 0, err
	}

	if len(checkpoints) == 0 {
		return "", 0, record.ErrNotFound
	}

	checkpoint := checkpoints[len(checkpoints)-1]
	return filepath.Join(dir, checkpoint.name), checkpoint.index, nil
}

// DeleteCheckpoints deletes all checkpoints in a directory below a given index.
func DeleteCheckpoints(dir string, maxIndex int) error {
	checkpoints, err := listCheckpoints(dir)
	if err != nil {
		return err
	}

	errs := tsdb_errors.NewMulti()
	for _, checkpoint := range checkpoints {
		if checkpoint.index >= maxIndex {
			break
		}
		errs.Add(os.RemoveAll(filepath.Join(dir, checkpoint.name)))
	}
	return errs.Err()
}

// CheckpointPrefix is the prefix used for checkpoint files.
const CheckpointPrefix = "checkpoint."

// Checkpoint creates a compacted checkpoint of segments in range [from, to] in the given WAL.
// It includes the most recent checkpoint if it exists.
// All series not satisfying keep, samples/tombstones/exemplars below mint and
// metadata that are not the latest are dropped.
//
// The checkpoint is stored in a directory named checkpoint.N in the same
// segmented format as the original WAL itself.
// This makes it easy to read it through the WAL package and concatenate
// it with the original WAL.
func Checkpoint(logger *slog.Logger, w *WL, from, to int, keep func(id chunks.HeadSeriesRef) bool, mint int64) (*CheckpointStats, error) {
	stats := &CheckpointStats{}
	var sgmReader io.ReadCloser

	logger.Info("Creating checkpoint", "from_segment", from, "to_segment", to, "mint", mint)

	{
		var sgmRange []SegmentRange
		dir, idx, err := LastCheckpoint(w.Dir())
		if err != nil && !errors.Is(err, record.ErrNotFound) {
			return nil, fmt.Errorf("find last checkpoint: %w", err)
		}
		last := idx + 1
		if err == nil {
			if from > last {
				return nil, fmt.Errorf("unexpected gap to last checkpoint. expected:%v, requested:%v", last, from)
			}
			// Ignore WAL files below the checkpoint. They shouldn't exist to begin with.
			from = last

			sgmRange = append(sgmRange, SegmentRange{Dir: dir, Last: math.MaxInt32})
		}

		sgmRange = append(sgmRange, SegmentRange{Dir: w.Dir(), First: from, Last: to})
		sgmReader, err = NewSegmentsRangeReader(sgmRange...)
		if err != nil {
			return nil, fmt.Errorf("create segment reader: %w", err)
		}
		defer sgmReader.Close()
	}

	cpdir := checkpointDir(w.Dir(), to)
	cpdirtmp := cpdir + ".tmp"

	if err := os.RemoveAll(cpdirtmp); err != nil {
		return nil, fmt.Errorf("remove previous temporary checkpoint dir: %w", err)
	}

	if err := os.MkdirAll(cpdirtmp, 0o777); err != nil {
		return nil, fmt.Errorf("create checkpoint dir: %w", err)
	}
	cp, err := New(nil, nil, cpdirtmp, w.CompressionType())
	if err != nil {
		return nil, fmt.Errorf("open checkpoint: %w", err)
	}

	// Ensures that an early return caused by an error doesn't leave any tmp files.
	defer func() {
		cp.Close()
		os.RemoveAll(cpdirtmp)
	}()

	r := NewReader(sgmReader)

	var (
		series                []record.RefSeries
		samples               []record.RefSample
		histogramSamples      []record.RefHistogramSample
		floatHistogramSamples []record.RefFloatHistogramSample
		tstones               []tombstones.Stone
		exemplars             []record.RefExemplar
		metadata              []record.RefMetadata
		st                    = labels.NewSymbolTable() // Needed for decoding; labels do not outlive this function.
		dec                   = record.NewDecoder(st)
		enc                   record.Encoder
		buf                   []byte
		recs                  [][]byte

		latestMetadataMap = make(map[chunks.HeadSeriesRef]record.RefMetadata)
	)
	for r.Next() {
		series, samples, histogramSamples, floatHistogramSamples, tstones, exemplars, metadata = series[:0], samples[:0], histogramSamples[:0], floatHistogramSamples[:0], tstones[:0], exemplars[:0], metadata[:0]

		// We don't reset the buffer since we batch up multiple records
		// before writing them to the checkpoint.
		// Remember where the record for this iteration starts.
		start := len(buf)
		rec := r.Record()

		switch dec.Type(rec) {
		case record.Series:
			series, err = dec.Series(rec, series)
			if err != nil {
				return nil, fmt.Errorf("decode series: %w", err)
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

		case record.Samples:
			samples, err = dec.Samples(rec, samples)
			if err != nil {
				return nil, fmt.Errorf("decode samples: %w", err)
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

		case record.HistogramSamples:
			histogramSamples, err = dec.HistogramSamples(rec, histogramSamples)
			if err != nil {
				return nil, fmt.Errorf("decode histogram samples: %w", err)
			}
			// Drop irrelevant histogramSamples in place.
			repl := histogramSamples[:0]
			for _, h := range histogramSamples {
				if h.T >= mint {
					repl = append(repl, h)
				}
			}
			if len(repl) > 0 {
				buf, _ = enc.HistogramSamples(repl, buf)
			}
			stats.TotalSamples += len(histogramSamples)
			stats.DroppedSamples += len(histogramSamples) - len(repl)
		case record.CustomBucketsHistogramSamples:
			histogramSamples, err = dec.HistogramSamples(rec, histogramSamples)
			if err != nil {
				return nil, fmt.Errorf("decode histogram samples: %w", err)
			}
			// Drop irrelevant histogramSamples in place.
			repl := histogramSamples[:0]
			for _, h := range histogramSamples {
				if h.T >= mint {
					repl = append(repl, h)
				}
			}
			if len(repl) > 0 {
				buf = enc.CustomBucketsHistogramSamples(repl, buf)
			}
			stats.TotalSamples += len(histogramSamples)
			stats.DroppedSamples += len(histogramSamples) - len(repl)
		case record.FloatHistogramSamples:
			floatHistogramSamples, err = dec.FloatHistogramSamples(rec, floatHistogramSamples)
			if err != nil {
				return nil, fmt.Errorf("decode float histogram samples: %w", err)
			}
			// Drop irrelevant floatHistogramSamples in place.
			repl := floatHistogramSamples[:0]
			for _, fh := range floatHistogramSamples {
				if fh.T >= mint {
					repl = append(repl, fh)
				}
			}
			if len(repl) > 0 {
				buf, _ = enc.FloatHistogramSamples(repl, buf)
			}
			stats.TotalSamples += len(floatHistogramSamples)
			stats.DroppedSamples += len(floatHistogramSamples) - len(repl)
		case record.CustomBucketsFloatHistogramSamples:
			floatHistogramSamples, err = dec.FloatHistogramSamples(rec, floatHistogramSamples)
			if err != nil {
				return nil, fmt.Errorf("decode float histogram samples: %w", err)
			}
			// Drop irrelevant floatHistogramSamples in place.
			repl := floatHistogramSamples[:0]
			for _, fh := range floatHistogramSamples {
				if fh.T >= mint {
					repl = append(repl, fh)
				}
			}
			if len(repl) > 0 {
				buf = enc.CustomBucketsFloatHistogramSamples(repl, buf)
			}
			stats.TotalSamples += len(floatHistogramSamples)
			stats.DroppedSamples += len(floatHistogramSamples) - len(repl)
		case record.Tombstones:
			tstones, err = dec.Tombstones(rec, tstones)
			if err != nil {
				return nil, fmt.Errorf("decode deletes: %w", err)
			}
			// Drop irrelevant tombstones in place.
			repl := tstones[:0]
			for _, s := range tstones {
				for _, iv := range s.Intervals {
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

		case record.Exemplars:
			exemplars, err = dec.Exemplars(rec, exemplars)
			if err != nil {
				return nil, fmt.Errorf("decode exemplars: %w", err)
			}
			// Drop irrelevant exemplars in place.
			repl := exemplars[:0]
			for _, e := range exemplars {
				if e.T >= mint {
					repl = append(repl, e)
				}
			}
			if len(repl) > 0 {
				buf = enc.Exemplars(repl, buf)
			}
			stats.TotalExemplars += len(exemplars)
			stats.DroppedExemplars += len(exemplars) - len(repl)
		case record.Metadata:
			metadata, err := dec.Metadata(rec, metadata)
			if err != nil {
				return nil, fmt.Errorf("decode metadata: %w", err)
			}
			// Only keep reference to the latest found metadata for each refID.
			repl := 0
			for _, m := range metadata {
				if keep(m.Ref) {
					if _, ok := latestMetadataMap[m.Ref]; !ok {
						repl++
					}
					latestMetadataMap[m.Ref] = m
				}
			}
			stats.TotalMetadata += len(metadata)
			stats.DroppedMetadata += len(metadata) - repl
		default:
			// Unknown record type, probably from a future Prometheus version.
			continue
		}
		if len(buf[start:]) == 0 {
			continue // All contents discarded.
		}
		recs = append(recs, buf[start:])

		// Flush records in 1 MB increments.
		if len(buf) > 1*1024*1024 {
			if err := cp.Log(recs...); err != nil {
				return nil, fmt.Errorf("flush records: %w", err)
			}
			buf, recs = buf[:0], recs[:0]
		}
	}
	// If we hit any corruption during checkpointing, repairing is not an option.
	// The head won't know which series records are lost.
	if r.Err() != nil {
		return nil, fmt.Errorf("read segments: %w", r.Err())
	}

	// Flush remaining records.
	if err := cp.Log(recs...); err != nil {
		return nil, fmt.Errorf("flush records: %w", err)
	}

	// Flush latest metadata records for each series.
	if len(latestMetadataMap) > 0 {
		latestMetadata := make([]record.RefMetadata, 0, len(latestMetadataMap))
		for _, m := range latestMetadataMap {
			latestMetadata = append(latestMetadata, m)
		}
		if err := cp.Log(enc.Metadata(latestMetadata, buf[:0])); err != nil {
			return nil, fmt.Errorf("flush metadata records: %w", err)
		}
	}

	if err := cp.Close(); err != nil {
		return nil, fmt.Errorf("close checkpoint: %w", err)
	}

	// Sync temporary directory before rename.
	df, err := fileutil.OpenDir(cpdirtmp)
	if err != nil {
		return nil, fmt.Errorf("open temporary checkpoint directory: %w", err)
	}
	if err := df.Sync(); err != nil {
		df.Close()
		return nil, fmt.Errorf("sync temporary checkpoint directory: %w", err)
	}
	if err = df.Close(); err != nil {
		return nil, fmt.Errorf("close temporary checkpoint directory: %w", err)
	}

	if err := fileutil.Replace(cpdirtmp, cpdir); err != nil {
		return nil, fmt.Errorf("rename checkpoint directory: %w", err)
	}

	return stats, nil
}

func checkpointDir(dir string, i int) string {
	return filepath.Join(dir, fmt.Sprintf(CheckpointPrefix+"%08d", i))
}

type checkpointRef struct {
	name  string
	index int
}

func listCheckpoints(dir string) (refs []checkpointRef, err error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(files); i++ {
		fi := files[i]
		if !strings.HasPrefix(fi.Name(), CheckpointPrefix) {
			continue
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("checkpoint %s is not a directory", fi.Name())
		}
		idx, err := strconv.Atoi(fi.Name()[len(CheckpointPrefix):])
		if err != nil {
			continue
		}

		refs = append(refs, checkpointRef{name: fi.Name(), index: idx})
	}

	slices.SortFunc(refs, func(a, b checkpointRef) int {
		return a.index - b.index
	})

	return refs, nil
}
