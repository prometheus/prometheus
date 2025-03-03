package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	mimirblock "github.com/grafana/mimir/pkg/storage/tsdb/block"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// createTSDBBlock creates a new TSDB block with the specified number of series
func createTSDBBlock(numSeries int, outputDir string, logger *slog.Logger) (string, error) {
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Create series with samples
	timestamp := time.Now().Unix() * 1000

	series := make([]storage.Series, 0, numSeries)

	for i := 0; i < numSeries; i++ {
		lbls := labels.FromStrings("__name__", fmt.Sprintf("grafana_recovery_test_series_%d", i))

		samples := []chunks.Sample{}
		for j := 0; j < 100; j++ { // append a sample every 30 seconds
			v := float64(j)
			samples = append(samples, newSample(timestamp+int64(j)*30*1000, v, nil, nil))
		}

		series = append(series, storage.NewListSeries(lbls, samples))
	}

	blockFName, err := CreateBlock(
		series,
		outputDir,
		60*time.Minute.Milliseconds(),
		logger,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create block: %w", err)
	}

	// Read and unmarshal the meta.json file
	metaFilePath := filepath.Join(blockFName, "meta.json")
	metaFile, err := os.ReadFile(metaFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read meta.json: %w", err)
	}

	var meta mimirblock.Meta
	if err := json.Unmarshal(metaFile, &meta); err != nil {
		return "", fmt.Errorf("failed to unmarshal meta.json: %w", err)
	}

	// To make it quicker
	meta.Compaction.Level = 2
	// Add the hint the compactor is expecting
	meta.Compaction.Hints = []string{"remove-quiet-zero-nans"}
	meta.Thanos.Source = mimirblock.CompactorRepairSource

	// Marshal the updated meta back to JSON
	updatedMetaFile, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal updated meta.json: %w", err)
	}

	// Write the updated meta.json back to the file
	if err := os.WriteFile(metaFilePath, updatedMetaFile, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to write updated meta.json: %w", err)
	}

	return blockFName, nil
}

type sample struct {
	t  int64
	f  float64
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
}

func newSample(t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram) chunks.Sample {
	return sample{t, v, h, fh}
}

func (s sample) T() int64                      { return s.t }
func (s sample) F() float64                    { return s.f }
func (s sample) H() *histogram.Histogram       { return s.h }
func (s sample) FH() *histogram.FloatHistogram { return s.fh }

func (s sample) Type() chunkenc.ValueType {
	switch {
	case s.h != nil:
		return chunkenc.ValHistogram
	case s.fh != nil:
		return chunkenc.ValFloatHistogram
	default:
		return chunkenc.ValFloat
	}
}

func (s sample) Copy() chunks.Sample {
	c := sample{t: s.t, f: s.f}
	if s.h != nil {
		c.h = s.h.Copy()
	}
	if s.fh != nil {
		c.fh = s.fh.Copy()
	}
	return c
}

func CreateBlock(series []storage.Series, dir string, chunkRange int64, logger *slog.Logger) (string, error) {
	if chunkRange == 0 {
		chunkRange = tsdb.DefaultBlockDuration
	}
	if chunkRange < 0 {
		return "", tsdb.ErrInvalidTimes
	}

	w, err := tsdb.NewBlockWriter(logger, dir, chunkRange)
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
