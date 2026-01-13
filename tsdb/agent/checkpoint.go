package agent

import (
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"os"

	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

type checkpointFlusher struct {
	enc         record.Encoder
	checkpoint  *wlog.WL
	seriesBuff  []byte
	samplesBuff []byte

	seriesRecords []record.RefSeries
	sampleRecords []record.RefSample
	offset        int
	batchSize     int
}

func newCheckpointFlusher(checkpoint *wlog.WL, batchSize int) *checkpointFlusher {
	return &checkpointFlusher{
		batchSize:     batchSize,
		checkpoint:    checkpoint,
		seriesRecords: make([]record.RefSeries, batchSize),
		sampleRecords: make([]record.RefSample, batchSize),
	}
}

func (cf *checkpointFlusher) flushRecords(withSamples bool) error {
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
	cf.offset = 0
	return nil
}

func (cf *checkpointFlusher) writeSeries(seriesIter iter.Seq[ActiveSeries]) error {
	for series := range seriesIter {
		// If we filled the buffers, write them out and reset.
		if cf.offset == cf.batchSize {
			if err := cf.flushRecords(true); err != nil {
				return fmt.Errorf("flush active series: %w", err)
			}
		}

		cf.seriesRecords[cf.offset] = record.RefSeries{
			Ref:    series.Ref(),
			Labels: series.Labels(),
		}

		// Sample value is irrelevant, we only need the timestamp.
		cf.sampleRecords[cf.offset] = record.RefSample{
			Ref: series.Ref(),
			T:   series.LastSampleTimestamp(),
			V:   0,
		}

		cf.offset++
	}

	// Clear the last batch if we have one
	if cf.offset != 0 {
		return cf.flushRecords(true)
	}

	return nil
}

func (cf *checkpointFlusher) writeDeletedRecords(seriesRefIter iter.Seq[chunks.HeadSeriesRef]) error {
	for ref := range seriesRefIter {
		// If we filled the buffers, write them out and reset.
		if cf.offset == cf.batchSize {
			if err := cf.flushRecords(false); err != nil {
				return fmt.Errorf("flush deleted series: %w", err)
			}
		}

		// We don't care about timestamps here, so no samples.
		cf.seriesRecords[cf.offset] = record.RefSeries{
			Ref: ref,
		}

		cf.offset++
	}

	// Clear the last batch if we have one
	if cf.offset != 0 {
		return cf.flushRecords(false)
	}

	return nil
}

const defaultBatchSize = 1000

type CheckpointParams struct {
	// AtIndex is index at which a checkpoint should be created.
	AtIndex int

	// BatchSize is size of a single WAL log entry chunk to be flushed.
	BatchSize int

	// ActiveSeries is an iterator over active series.
	ActiveSeries iter.Seq[ActiveSeries]

	// DeletedSeries is iterator over recently deleted series.
	DeletedSeries iter.Seq[chunks.HeadSeriesRef]
}

func (p CheckpointParams) withDefaults() CheckpointParams {
	if p.BatchSize == 0 {
		p.BatchSize = defaultBatchSize
	}

	return p
}

// Checkpoint creates an unindexed checkpoint containing record.RefSeries and
// record.RefSample for ActiveSeries and a record.RefSeries for the recentlyDeleted series.
//
// The difference between this implementation and [wlog.Checkpoint] is that it skips re-read current checkpoint + segments
// and relies on data in memory.
func Checkpoint(logger *slog.Logger, w *wlog.WL, p CheckpointParams) error {
	p = p.withDefaults()
	logger.Info("creating checkpoint from WAL", "atIndex", p.AtIndex)

	dir, idx, err := wlog.LastCheckpoint(w.Dir())
	if err != nil && !errors.Is(err, record.ErrNotFound) {
		return fmt.Errorf("can't find last checkpoint: %w", err)
	}

	if idx >= p.AtIndex {
		logger.Info(
			"checkpoint already exists",
			"dir", dir,
			"index", idx,
			"requested_index", p.AtIndex,
		)
		return nil
	}

	// TODO: cleanup old temp checkpoints
	cpDir := wlog.CheckpointDir(w.Dir(), p.AtIndex)
	cpTmpDir := cpDir + ".tmp"
	if err := os.RemoveAll(cpTmpDir); err != nil {
		return fmt.Errorf("remove previous temporary checkpoint dir: %w", err)
	}

	if err := os.MkdirAll(cpTmpDir, os.ModePerm); err != nil {
		return fmt.Errorf("create checkpoint dir: %q", err)
	}

	cp, err := wlog.New(logger, nil, cpTmpDir, w.CompressionType())
	if err != nil {
		return fmt.Errorf("open checkpoint: %w", err)
	}

	defer func() {
		os.RemoveAll(cpTmpDir)
		cp.Close()
	}()

	flusher := newCheckpointFlusher(cp, p.BatchSize)
	if err := flusher.writeSeries(p.ActiveSeries); err != nil {
		return err
	}

	if err := flusher.writeDeletedRecords(p.DeletedSeries); err != nil {
		return err
	}

	if err := cp.Close(); err != nil {
		return fmt.Errorf("close checkpoint: %w", err)
	}

	// Sync temporary directory before rename.
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
