// Copyright 2025 The Prometheus Authors

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

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func createTSDBBlock(numSeries int, outputDir string, dimensions int, cardinality int, mint int64, logger *slog.Logger) (string, error) {
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	series := make([]storage.Series, 0)

	for seriesIdx := 0; seriesIdx < numSeries; seriesIdx++ {
		generateSeriesWithDimensions(seriesIdx, dimensions, cardinality, 0, nil, &series, mint)
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

	bm, _, err := tsdb.ReadMetaFile(blockFName)
	if err != nil {
		return "", fmt.Errorf("failed to read meta file: %w", err)
	}
	bm.Compaction.Level = 1
	_, err = tsdb.WriteMetaFile(logger, blockFName, bm)
	if err != nil {
		return "", fmt.Errorf("failed to write meta file: %w", err)
	}

	return blockFName, nil
}

// generateSeriesWithDimensions recursively generates all combinations of dimension values
func generateSeriesWithDimensions(seriesIdx, dimensions, cardinality, currentDim int, currentLabels []labels.Label, series *[]storage.Series, mint int64) {
	if currentDim == dimensions {
		labelPairs := append(
			[]labels.Label{
				{Name: "__name__", Value: fmt.Sprintf("tsdb2columnar_gauge_%d", seriesIdx)},
			},
			currentLabels...,
		)

		lbls := labels.New(labelPairs...)

		samples := []chunks.Sample{}
		for j := 0; j < 500; j++ {
			v := float64(j)
			ts := mint + int64(j)*14*1000
			samples = append(samples, newSample(ts, v, nil, nil))
		}

		*series = append(*series, storage.NewListSeries(lbls, samples))
		return
	}

	labelName := fmt.Sprintf("dim_%d", currentDim)
	for val := 0; val < cardinality; val++ {
		labelValue := fmt.Sprintf("val_%d", val)

		newLabels := append(currentLabels, labels.Label{
			Name:  labelName,
			Value: labelValue,
		})

		generateSeriesWithDimensions(seriesIdx, dimensions, cardinality, currentDim+1, newLabels, series, mint)
	}
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
