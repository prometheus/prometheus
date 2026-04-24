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

package agent

import (
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"os"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

const defaultBatchSize = 1000

// ActiveSeries describes a live series to be written by [Checkpoint].
//
// This interface is intentionally exported so downstream users of this package
// can use [Checkpoint] without depending on Prometheus internal series types.
type ActiveSeries interface {
	Ref() chunks.HeadSeriesRef
	Labels() labels.Labels
	LastSampleTimestamp() int64
}

// DeletedSeries describes a deleted series to be written by [Checkpoint].
//
// This interface is intentionally exported so downstream users of this package
// can use [Checkpoint] without depending on Prometheus internal series types.
type DeletedSeries interface {
	Ref() chunks.HeadSeriesRef
	Labels() labels.Labels
}

// Checkpoint creates an unindexed checkpoint containing record.RefSeries and
// last timestamp for ActiveSeries and record.RefSeries for DeletedSeries.
//
// This API accepts interfaces so downstream users of this package can provide
// their own series storage while reusing Prometheus checkpoint writing logic.
//
// The difference between this implementation and [wlog.Checkpoint] is that it skips re-read current checkpoint + segments
// and relies on data in memory.
func Checkpoint(logger *slog.Logger, w *wlog.WL, atIndex, batchSize int, activeSeries iter.Seq[ActiveSeries], deletedSeries iter.Seq[DeletedSeries]) error {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	logger.Info("Creating checkpoint", "atIndex", atIndex)

	dir, idx, err := wlog.LastCheckpoint(w.Dir())
	if err != nil && !errors.Is(err, record.ErrNotFound) {
		return fmt.Errorf("can't find last checkpoint: %w", err)
	}

	if idx >= atIndex {
		logger.Info(
			"checkpoint already exists",
			"dir", dir,
			"index", idx,
			"requested_index", atIndex,
		)
		return nil
	}

	if err := wlog.DeleteTempCheckpoints(logger, w.Dir()); err != nil {
		return fmt.Errorf("failed to cleanup temporary checkpoints: %w", err)
	}

	cpDir := wlog.CheckpointDir(w.Dir(), atIndex)
	cpTmpDir := cpDir + wlog.CheckpointTempFileSuffix

	if err := os.MkdirAll(cpTmpDir, os.ModePerm); err != nil {
		return fmt.Errorf("create checkpoint dir: %w", err)
	}

	cp, err := wlog.New(logger, nil, cpTmpDir, w.CompressionType())
	if err != nil {
		return fmt.Errorf("open checkpoint: %w", err)
	}

	success := false
	defer func() {
		if !success {
			cp.Close()
		}
		os.RemoveAll(cpTmpDir)
	}()

	flusher := newCheckpointFlusher(cp, batchSize)
	if err := flusher.writeSeries(activeSeries); err != nil {
		return err
	}

	if err := flusher.writeDeletedRecords(deletedSeries); err != nil {
		return err
	}

	success = true
	if err := cp.Close(); err != nil {
		return fmt.Errorf("close checkpoint: %w", err)
	}

	df, err := fileutil.OpenDir(cpTmpDir)
	if err != nil {
		return fmt.Errorf("open temporary checkpoint directory: %w", err)
	}
	if err := df.Sync(); err != nil {
		df.Close()
		return fmt.Errorf("sync temporary checkpoint directory: %w", err)
	}
	if err = df.Close(); err != nil {
		return fmt.Errorf("close temporary checkpoint directory: %w", err)
	}

	if err := fileutil.Replace(cpTmpDir, cpDir); err != nil {
		return fmt.Errorf("rename checkpoint directory: %w", err)
	}

	return nil
}

type checkpointWriter struct {
	enc         record.Encoder
	checkpoint  *wlog.WL
	seriesBuff  []byte
	samplesBuff []byte

	seriesRecords []record.RefSeries
	sampleRecords []record.RefSample
	batchSize     int
}

func newCheckpointFlusher(checkpoint *wlog.WL, batchSize int) *checkpointWriter {
	return &checkpointWriter{
		batchSize:     batchSize,
		checkpoint:    checkpoint,
		seriesRecords: make([]record.RefSeries, 0, batchSize),
		sampleRecords: make([]record.RefSample, 0, batchSize),
	}
}

func (cf *checkpointWriter) flushRecords() error {
	withSamples := len(cf.sampleRecords) > 0
	cf.seriesBuff = cf.enc.Series(cf.seriesRecords, cf.seriesBuff)

	var err error
	if withSamples {
		cf.samplesBuff = cf.enc.Samples(cf.sampleRecords, cf.samplesBuff)
		err = cf.checkpoint.Log(cf.seriesBuff, cf.samplesBuff)
	} else {
		err = cf.checkpoint.Log(cf.seriesBuff)
	}

	if err != nil {
		return fmt.Errorf("flush records: %w", err)
	}

	cf.seriesBuff = cf.seriesBuff[:0]
	cf.samplesBuff = cf.samplesBuff[:0]
	cf.seriesRecords = cf.seriesRecords[:0]
	cf.sampleRecords = cf.sampleRecords[:0]
	return nil
}

func (cf *checkpointWriter) writeSeries(seriesIter iter.Seq[ActiveSeries]) error {
	for series := range seriesIter {
		// If we filled the buffers, write them out and reset.
		if len(cf.seriesRecords) == cf.batchSize {
			if err := cf.flushRecords(); err != nil {
				return fmt.Errorf("flush active series: %w", err)
			}
		}

		cf.seriesRecords = append(cf.seriesRecords, record.RefSeries{
			Ref:    series.Ref(),
			Labels: series.Labels(),
		})

		// Sample value is irrelevant, we only need the timestamp.
		cf.sampleRecords = append(cf.sampleRecords, record.RefSample{
			Ref: series.Ref(),
			T:   series.LastSampleTimestamp(),
			V:   0,
		})
	}

	// Flush the last batch if we have one
	if len(cf.seriesRecords) != 0 {
		return cf.flushRecords()
	}

	return nil
}

func (cf *checkpointWriter) writeDeletedRecords(seriesRefIter iter.Seq[DeletedSeries]) error {
	for series := range seriesRefIter {
		// If we filled the buffers, write them out and reset.
		if len(cf.seriesRecords) == cf.batchSize {
			if err := cf.flushRecords(); err != nil {
				return fmt.Errorf("flush deleted series: %w", err)
			}
		}

		// We don't care about timestamps here, so no samples.
		cf.seriesRecords = append(cf.seriesRecords, record.RefSeries{
			Ref:    series.Ref(),
			Labels: series.Labels(),
		})
	}

	// Clear the last batch if we have one
	if len(cf.seriesRecords) != 0 {
		return cf.flushRecords()
	}

	return nil
}
