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
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
	"unsafe"
	"weak"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/testutil"
)

func createTestAgentDB(t testing.TB, reg prometheus.Registerer, opts *Options) *DB {
	t.Helper()

	dbDir := t.TempDir()
	opts = validateOptions(opts)

	rs := remote.NewStorage(promslog.NewNopLogger(), reg, startTime, dbDir, time.Second*30, nil, false, opts.EnableMetadataWALRecords, opts.EnableSTStorage)
	t.Cleanup(func() {
		require.NoError(t, rs.Close())
	})

	db, err := Open(promslog.NewNopLogger(), reg, rs, dbDir, opts)
	require.NoError(t, err)
	return db
}

func TestAppenderBufferResetAfterWALWriteError(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*appenderBase)
		log     func(*appenderBase) error
	}{
		{
			name: "log",
			prepare: func(a *appenderBase) {
				a.pendingSamples = append(a.pendingSamples, record.RefSample{Ref: 1, T: 1, V: 1})
			},
			log: func(a *appenderBase) error {
				return a.log()
			},
		},
		{
			name: "logSeries",
			prepare: func(a *appenderBase) {
				a.pendingSeries = append(a.pendingSeries, record.RefSeries{
					Ref:    1,
					Labels: labels.FromStrings("__name__", "test_metric"),
				})
			},
			log: func(a *appenderBase) error {
				return a.logSeries()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wal, err := wlog.New(promslog.NewNopLogger(), nil, t.TempDir(), DefaultOptions().WALCompression)
			require.NoError(t, err)
			require.NoError(t, wal.Close())

			db := &DB{wal: wal, opts: DefaultOptions()}
			app := &appenderBase{DB: db}
			test.prepare(app)

			for range 100 {
				db.bufPool.New = func() any {
					return make([]byte, 0, 1024)
				}
				require.Error(t, test.log(app))

				// A pool may discard buffers, especially under -race. Disable New so
				// a fresh buffer cannot satisfy the assertion.
				db.bufPool.New = nil
				buf := db.bufPool.Get()
				if buf == nil {
					continue
				}
				require.Empty(t, buf.([]byte))
				return
			}
			t.Fatal("no buffer returned from pool after 100 attempts")
		})
	}
}

func TestAppenderClearedAfterWALWriteError(t *testing.T) {
	lset := labels.FromStrings("__name__", "test_metric")
	appenders := []struct {
		name   string
		append func(t *testing.T, db *DB) (*appenderBase, storage.AppenderTransaction)
	}{
		{
			name: "appender",
			append: func(t *testing.T, db *DB) (*appenderBase, storage.AppenderTransaction) {
				app := db.Appender(t.Context())
				_, err := app.Append(0, lset, 1, 1)
				require.NoError(t, err)
				return &app.(*appender).appenderBase, app
			},
		},
		{
			name: "appenderV2",
			append: func(t *testing.T, db *DB) (*appenderBase, storage.AppenderTransaction) {
				app := db.AppenderV2(t.Context())
				_, err := app.Append(0, lset, 0, 1, 1, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				return &app.(*appenderV2).appenderBase, app
			},
		},
	}
	finishers := []struct {
		name   string
		finish func(storage.AppenderTransaction) error
	}{
		{name: "commit", finish: storage.AppenderTransaction.Commit},
		{name: "rollback", finish: storage.AppenderTransaction.Rollback},
	}

	for _, a := range appenders {
		for _, f := range finishers {
			t.Run(a.name+"/"+f.name, func(t *testing.T) {
				db := createTestAgentDB(t, nil, DefaultOptions())
				// Close the WAL so writes fail. db.Close then fails on the second WAL close.
				require.NoError(t, db.wal.Close())
				t.Cleanup(func() { require.Error(t, db.Close()) })

				base, app := a.append(t, db)
				require.Error(t, f.finish(app))

				// The appender is back in the pool, so pending data would leak into the next one.
				require.Empty(t, base.pendingSeries)
				require.Empty(t, base.pendingSamples)
				require.Empty(t, base.sampleSeries)
			})
		}
	}
}

// TestConcurrentAppendSameLabels verifies that concurrent appends for the same
// label set produce exactly one series in memory and one series record in the WAL.
func TestConcurrentAppendSameLabels(t *testing.T) {
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = math.MaxInt64
	db := createTestAgentDB(t, nil, opts)
	lset := labels.FromStrings("__name__", "test_metric")

	const n = 100
	var wg sync.WaitGroup
	start := make(chan struct{})

	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			app := db.AppenderV2(context.Background())
			<-start
			_, err := app.Append(0, lset, 0, 1000, 1.0, nil, nil, storage.AOptions{})
			require.NoError(t, err)
			require.NoError(t, app.Commit())
		}()
	}
	close(start)
	wg.Wait()

	var total int
	for i := range db.series.size {
		db.series.locks[i].RLock()
		total += len(db.series.series[i])
		db.series.locks[i].RUnlock()
	}
	require.Equal(t, 1, total)
	require.NoError(t, db.Close())

	sr, err := wlog.NewSegmentsReader(db.wal.Dir())
	require.NoError(t, err)
	defer func() { require.NoError(t, sr.Close()) }()

	r := wlog.NewReader(sr)
	dec := record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
	var walSeries int
	for r.Next() {
		rec := r.Record()
		if dec.Type(rec) == record.Series {
			var s []record.RefSeries
			s, err = dec.Series(rec, s)
			require.NoError(t, err)
			walSeries += len(s)
		}
	}
	require.NoError(t, r.Err())
	require.Equal(t, 1, walSeries)
}

func TestUnsupportedFunctions(t *testing.T) {
	s := createTestAgentDB(t, nil, DefaultOptions())
	defer s.Close()

	t.Run("Querier", func(t *testing.T) {
		_, err := s.Querier(0, 0)
		require.Equal(t, err, ErrUnsupported)
	})

	t.Run("ChunkQuerier", func(t *testing.T) {
		_, err := s.ChunkQuerier(0, 0)
		require.Equal(t, err, ErrUnsupported)
	})

	t.Run("ExemplarQuerier", func(t *testing.T) {
		_, err := s.ExemplarQuerier(context.TODO())
		require.Equal(t, err, ErrUnsupported)
	})
}

func TestCommit(t *testing.T) {
	const (
		numDatapoints = 1000
		numHistograms = 100
		numSeries     = 8
	)

	s := createTestAgentDB(t, nil, DefaultOptions())
	app := s.Appender(context.TODO())

	lbls := labelsForTest(t.Name(), numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		for i := range numDatapoints {
			sample := chunks.GenerateSamples(0, 1)
			ref, err := app.Append(0, lset, sample[0].T(), sample[0].F())
			require.NoError(t, err)

			e := exemplar.Exemplar{
				Labels: lset,
				Ts:     sample[0].T() + int64(i),
				Value:  sample[0].F(),
				HasTs:  true,
			}
			_, err = app.AppendExemplar(ref, lset, e)
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestHistograms(numHistograms)

		for i := range numHistograms {
			_, err := app.AppendHistogram(0, lset, int64(i), histograms[i], nil)
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_custom_buckets_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		customBucketHistograms := tsdbutil.GenerateTestCustomBucketsHistograms(numHistograms)

		for i := range numHistograms {
			_, err := app.AppendHistogram(0, lset, int64(i), customBucketHistograms[i], nil)
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_float_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numHistograms)

		for i := range numHistograms {
			_, err := app.AppendHistogram(0, lset, int64(i), nil, floatHistograms[i])
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_custom_buckets_float_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		customBucketFloatHistograms := tsdbutil.GenerateTestCustomBucketsFloatHistograms(numHistograms)

		for i := range numHistograms {
			_, err := app.AppendHistogram(0, lset, int64(i), nil, customBucketFloatHistograms[i])
			require.NoError(t, err)
		}
	}

	require.NoError(t, app.Commit())
	require.NoError(t, s.Close())

	sr, err := wlog.NewSegmentsReader(s.wal.Dir())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, sr.Close())
	}()

	// Read records from WAL and check for expected count of series, samples, and exemplars.
	var (
		r   = wlog.NewReader(sr)
		dec = record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())

		walSeriesCount, walSamplesCount, walExemplarsCount, walHistogramCount, walFloatHistogramCount int
	)
	for r.Next() {
		rec := r.Record()
		switch dec.Type(rec) {
		case record.Series:
			var series []record.RefSeries
			series, err = dec.Series(rec, series)
			require.NoError(t, err)
			walSeriesCount += len(series)

		case record.Samples, record.SamplesV2:
			var samples []record.RefSample
			samples, err = dec.Samples(rec, samples)
			require.NoError(t, err)
			walSamplesCount += len(samples)

		case record.HistogramSamples, record.CustomBucketsHistogramSamples, record.HistogramSamplesV2:
			var histograms []record.RefHistogramSample
			histograms, err = dec.HistogramSamples(rec, histograms)
			require.NoError(t, err)
			walHistogramCount += len(histograms)

		case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples, record.FloatHistogramSamplesV2:
			var floatHistograms []record.RefFloatHistogramSample
			floatHistograms, err = dec.FloatHistogramSamples(rec, floatHistograms)
			require.NoError(t, err)
			walFloatHistogramCount += len(floatHistograms)

		case record.Exemplars:
			var exemplars []record.RefExemplar
			exemplars, err = dec.Exemplars(rec, exemplars)
			require.NoError(t, err)
			walExemplarsCount += len(exemplars)

		default:
		}
	}

	// Check that the WAL contained the same number of committed series/samples/exemplars.
	require.Equal(t, numSeries*5, walSeriesCount, "unexpected number of series")
	require.Equal(t, numSeries*numDatapoints, walSamplesCount, "unexpected number of samples")
	require.Equal(t, numSeries*numDatapoints, walExemplarsCount, "unexpected number of exemplars")
	require.Equal(t, numSeries*numHistograms*2, walHistogramCount, "unexpected number of histograms")
	require.Equal(t, numSeries*numHistograms*2, walFloatHistogramCount, "unexpected number of float histograms")

	// Check that we can get another appender after this - see https://github.com/prometheus/prometheus/issues/17800.
	_ = s.Appender(context.TODO())
}

func TestRollback(t *testing.T) {
	const (
		numDatapoints = 1000
		numHistograms = 100
		numSeries     = 8
	)

	s := createTestAgentDB(t, nil, DefaultOptions())
	app := s.Appender(context.TODO())

	lbls := labelsForTest(t.Name(), numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		for range numDatapoints {
			sample := chunks.GenerateSamples(0, 1)
			_, err := app.Append(0, lset, sample[0].T(), sample[0].F())
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestHistograms(numHistograms)

		for i := range numHistograms {
			_, err := app.AppendHistogram(0, lset, int64(i), histograms[i], nil)
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_custom_buckets_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestCustomBucketsHistograms(numHistograms)

		for i := range numHistograms {
			_, err := app.AppendHistogram(0, lset, int64(i), histograms[i], nil)
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_float_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numHistograms)

		for i := range numHistograms {
			_, err := app.AppendHistogram(0, lset, int64(i), nil, floatHistograms[i])
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_custom_buckets_float_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestCustomBucketsFloatHistograms(numHistograms)

		for i := range numHistograms {
			_, err := app.AppendHistogram(0, lset, int64(i), nil, floatHistograms[i])
			require.NoError(t, err)
		}
	}

	// Do a rollback, which should clear uncommitted data. A followup call to
	// commit should persist nothing to the WAL.
	require.NoError(t, app.Rollback())
	require.NoError(t, app.Commit())
	require.NoError(t, s.Close())

	sr, err := wlog.NewSegmentsReader(s.wal.Dir())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, sr.Close())
	}()

	// Read records from WAL and check for expected count of series and samples.
	var (
		r   = wlog.NewReader(sr)
		dec = record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())

		walSeriesCount, walSamplesCount, walHistogramCount, walFloatHistogramCount, walExemplarsCount int
	)
	for r.Next() {
		rec := r.Record()
		switch dec.Type(rec) {
		case record.Series:
			var series []record.RefSeries
			series, err = dec.Series(rec, series)
			require.NoError(t, err)
			walSeriesCount += len(series)

		case record.Samples, record.SamplesV2:
			var samples []record.RefSample
			samples, err = dec.Samples(rec, samples)
			require.NoError(t, err)
			walSamplesCount += len(samples)

		case record.Exemplars:
			var exemplars []record.RefExemplar
			exemplars, err = dec.Exemplars(rec, exemplars)
			require.NoError(t, err)
			walExemplarsCount += len(exemplars)

		case record.HistogramSamples, record.CustomBucketsHistogramSamples, record.HistogramSamplesV2:
			var histograms []record.RefHistogramSample
			histograms, err = dec.HistogramSamples(rec, histograms)
			require.NoError(t, err)
			walHistogramCount += len(histograms)

		case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples, record.FloatHistogramSamplesV2:
			var floatHistograms []record.RefFloatHistogramSample
			floatHistograms, err = dec.FloatHistogramSamples(rec, floatHistograms)
			require.NoError(t, err)
			walFloatHistogramCount += len(floatHistograms)

		default:
		}
	}

	// Check that only series get stored after calling Rollback.
	require.Equal(t, numSeries*5, walSeriesCount, "series should have been written to WAL")
	require.Equal(t, 0, walSamplesCount, "samples should not have been written to WAL")
	require.Equal(t, 0, walExemplarsCount, "exemplars should not have been written to WAL")
	require.Equal(t, 0, walHistogramCount, "histograms should not have been written to WAL")
	require.Equal(t, 0, walFloatHistogramCount, "float histograms should not have been written to WAL")
}

func TestFullTruncateWAL(t *testing.T) {
	const (
		numDatapoints = 1000
		numHistograms = 100
		numSeries     = 800
		lastTs        = 500
	)

	reg := prometheus.NewRegistry()
	opts := DefaultOptions()
	opts.TruncateFrequency = time.Minute * 2

	s := createTestAgentDB(t, reg, opts)
	defer func() {
		require.NoError(t, s.Close())
	}()
	app := s.Appender(context.TODO())

	lbls := labelsForTest(t.Name(), numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		for range numDatapoints {
			_, err := app.Append(0, lset, int64(lastTs), 0)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestHistograms(numHistograms)

		for i := range numHistograms {
			_, err := app.AppendHistogram(0, lset, int64(lastTs), histograms[i], nil)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_custom_buckets_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestCustomBucketsHistograms(numHistograms)

		for i := range numHistograms {
			_, err := app.AppendHistogram(0, lset, int64(lastTs), histograms[i], nil)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_float_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numHistograms)

		for i := range numHistograms {
			_, err := app.AppendHistogram(0, lset, int64(lastTs), nil, floatHistograms[i])
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_custom_buckets_float_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestCustomBucketsFloatHistograms(numHistograms)

		for i := range numHistograms {
			_, err := app.AppendHistogram(0, lset, int64(lastTs), nil, floatHistograms[i])
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Truncate WAL with mint to GC all the samples.
	s.truncate(lastTs + 1)

	m := gatherFamily(t, reg, "prometheus_agent_deleted_series")
	require.Equal(t, float64(numSeries*5), m.Metric[0].Gauge.GetValue(), "agent wal truncate mismatch of deleted series count")
}

func TestPartialTruncateWAL(t *testing.T) {
	const (
		numDatapoints = 1000
		numSeries     = 800
	)

	opts := DefaultOptions()

	reg := prometheus.NewRegistry()
	s := createTestAgentDB(t, reg, opts)
	defer func() {
		require.NoError(t, s.Close())
	}()
	app := s.Appender(context.TODO())

	// Create first batch of 800 series with 1000 data-points with a fixed lastTs as 500.
	var lastTs int64 = 500
	lbls := labelsForTest(t.Name()+"batch-1", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		for range numDatapoints {
			_, err := app.Append(0, lset, lastTs, 0)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_histogram_batch-1", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestHistograms(numDatapoints)

		for i := range numDatapoints {
			_, err := app.AppendHistogram(0, lset, lastTs, histograms[i], nil)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_custom_buckets_histogram_batch-1", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestCustomBucketsHistograms(numDatapoints)

		for i := range numDatapoints {
			_, err := app.AppendHistogram(0, lset, lastTs, histograms[i], nil)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_float_histogram_batch-1", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numDatapoints)

		for i := range numDatapoints {
			_, err := app.AppendHistogram(0, lset, lastTs, nil, floatHistograms[i])
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_custom_buckets_float_histogram_batch-1", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestCustomBucketsFloatHistograms(numDatapoints)

		for i := range numDatapoints {
			_, err := app.AppendHistogram(0, lset, lastTs, nil, floatHistograms[i])
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Create second batch of 800 series with 1000 data-points with a fixed lastTs as 600.
	lastTs = 600
	lbls = labelsForTest(t.Name()+"batch-2", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		for range numDatapoints {
			_, err := app.Append(0, lset, lastTs, 0)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_histogram_batch-2", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestHistograms(numDatapoints)

		for i := range numDatapoints {
			_, err := app.AppendHistogram(0, lset, lastTs, histograms[i], nil)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_custom_buckets_histogram_batch-2", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestCustomBucketsHistograms(numDatapoints)

		for i := range numDatapoints {
			_, err := app.AppendHistogram(0, lset, lastTs, histograms[i], nil)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_float_histogram_batch-2", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numDatapoints)

		for i := range numDatapoints {
			_, err := app.AppendHistogram(0, lset, lastTs, nil, floatHistograms[i])
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_custom_buckets_float_histogram_batch-2", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestCustomBucketsFloatHistograms(numDatapoints)

		for i := range numDatapoints {
			_, err := app.AppendHistogram(0, lset, lastTs, nil, floatHistograms[i])
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Truncate WAL with mint to GC only the first batch of 800 series and retaining 2nd batch of 800 series.
	s.truncate(lastTs - 1)

	m := gatherFamily(t, reg, "prometheus_agent_deleted_series")
	require.Len(t, m.Metric, 1)
	require.Equal(t, float64(numSeries*5), m.Metric[0].Gauge.GetValue(), "agent wal truncate mismatch of deleted series count")
}

func TestWALReplay(t *testing.T) {
	for _, unknownRecord := range []bool{false, true} {
		t.Run(fmt.Sprintf("unknownRecord=%t", unknownRecord), func(t *testing.T) {
			const (
				numDatapoints = 1000
				numHistograms = 100
				numSeries     = 8
				lastTs        = 500
			)

			s := createTestAgentDB(t, nil, DefaultOptions())
			if unknownRecord {
				// Valid records following an unknown type must still be replayed.
				require.NoError(t, s.wal.Log([]byte{255, 1, 2, 3}))
			}
			app := s.Appender(context.TODO())

			lbls := labelsForTest(t.Name(), numSeries)
			for _, l := range lbls {
				lset := labels.New(l...)

				for range numDatapoints {
					_, err := app.Append(0, lset, lastTs, 0)
					require.NoError(t, err)
				}
			}

			lbls = labelsForTest(t.Name()+"_histogram", numSeries)
			for _, l := range lbls {
				lset := labels.New(l...)

				histograms := tsdbutil.GenerateTestHistograms(numHistograms)

				for i := range numHistograms {
					_, err := app.AppendHistogram(0, lset, lastTs, histograms[i], nil)
					require.NoError(t, err)
				}
			}

			lbls = labelsForTest(t.Name()+"_custom_buckets_histogram", numSeries)
			for _, l := range lbls {
				lset := labels.New(l...)

				histograms := tsdbutil.GenerateTestCustomBucketsHistograms(numHistograms)

				for i := range numHistograms {
					_, err := app.AppendHistogram(0, lset, lastTs, histograms[i], nil)
					require.NoError(t, err)
				}
			}

			lbls = labelsForTest(t.Name()+"_float_histogram", numSeries)
			for _, l := range lbls {
				lset := labels.New(l...)

				floatHistograms := tsdbutil.GenerateTestFloatHistograms(numHistograms)

				for i := range numHistograms {
					_, err := app.AppendHistogram(0, lset, lastTs, nil, floatHistograms[i])
					require.NoError(t, err)
				}
			}

			lbls = labelsForTest(t.Name()+"_custom_buckets_float_histogram", numSeries)
			for _, l := range lbls {
				lset := labels.New(l...)

				floatHistograms := tsdbutil.GenerateTestCustomBucketsFloatHistograms(numHistograms)

				for i := range numHistograms {
					_, err := app.AppendHistogram(0, lset, lastTs, nil, floatHistograms[i])
					require.NoError(t, err)
				}
			}

			require.NoError(t, app.Commit())
			require.NoError(t, s.Close())

			// Hack: s.wal.Dir() is the /wal subdirectory of the original storage path.
			// We need the original directory so we can recreate the storage for replay.
			storageDir := filepath.Dir(s.wal.Dir())

			// Replay twice to ensure unknown records do not trigger destructive WAL repair.
			for range 2 {
				reg := prometheus.NewRegistry()
				replayStorage, err := Open(s.logger, reg, nil, storageDir, s.opts)
				if err != nil {
					t.Fatalf("unable to create storage for the agent: %v", err)
				}

				// Check if all the series are retrieved back from the WAL.
				m := gatherFamily(t, reg, "prometheus_agent_active_series")
				require.Equal(t, float64(numSeries*5), m.Metric[0].Gauge.GetValue(), "agent wal replay mismatch of active series count")

				// Check if lastTs of the samples retrieved from the WAL is retained.
				metrics := replayStorage.series.series
				for i := range metrics {
					mp := metrics[i]
					for _, v := range mp {
						require.Equal(t, v.lastTs, int64(lastTs))
					}
				}
				require.NoError(t, replayStorage.Close())
			}
		})
	}
}

func TestWALTruncationTime(t *testing.T) {
	opts := DefaultOptions()
	nowTS := time.Now().UnixMilli()
	for _, tc := range []struct {
		name string
		sent int64
		want int64
	}{
		{"current", nowTS, nowTS - opts.MinWALTime},
		{"lagging", nowTS - time.Hour.Milliseconds(), nowTS - time.Hour.Milliseconds() - opts.MinWALTime},
		{"stalled", 0, nowTS - opts.MaxWALTime},
		{"future", nowTS + (5 * time.Hour).Milliseconds(), nowTS - opts.MinWALTime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, walTruncationTime(opts, tc.sent, nowTS))
		})
	}
}

// blockingWriter blocks writes containing msg until release is closed.
type blockingWriter struct {
	msg     []byte
	once    sync.Once
	reached chan struct{}
	release chan struct{}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, w.msg) {
		w.once.Do(func() { close(w.reached) })
		<-w.release
	}
	return len(p), nil
}

func TestCloseDuringTruncate(t *testing.T) {
	// Pause the truncate loop on the log line just before truncate takes db.mtx.
	w := &blockingWriter{
		msg:     []byte("series GC started"),
		reached: make(chan struct{}),
		release: make(chan struct{}),
	}

	dbDir := t.TempDir()
	rs := remote.NewStorage(promslog.NewNopLogger(), nil, startTime, dbDir, time.Second*30, nil, false, false, false)
	t.Cleanup(func() {
		require.NoError(t, rs.Close())
	})

	opts := DefaultOptions()
	opts.TruncateFrequency = time.Millisecond
	db, err := Open(slog.New(slog.NewTextHandler(w, nil)), nil, rs, dbDir, opts)
	require.NoError(t, err)

	<-w.reached
	closed := make(chan error)
	go func() { closed <- db.Close() }()
	// Close has started stopping the loop. Let truncate continue.
	<-db.stopc
	close(w.release)

	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Close deadlocked against truncate")
	}
}

func TestLockfile(t *testing.T) {
	tsdbutil.TestDirLockerUsage(t, func(t *testing.T, data string, createLock bool) (*tsdbutil.DirLocker, testutil.Closer) {
		logger := promslog.NewNopLogger()
		reg := prometheus.NewRegistry()
		rs := remote.NewStorage(logger, reg, startTime, data, time.Second*30, nil, false, false, false)
		t.Cleanup(func() {
			require.NoError(t, rs.Close())
		})

		opts := DefaultOptions()
		opts.NoLockfile = !createLock

		// Create the DB. This should create lockfile and its metrics.
		db, err := Open(logger, nil, rs, data, opts)
		require.NoError(t, err)

		return db.locker, testutil.NewCallbackCloser(func() {
			require.NoError(t, db.Close())
		})
	})
}

func Test_validateOptions(t *testing.T) {
	t.Run("Apply defaults to zero values", func(t *testing.T) {
		opts := validateOptions(&Options{})
		require.Equal(t, DefaultOptions(), opts)
	})

	t.Run("Defaults are already valid", func(t *testing.T) {
		require.Equal(t, DefaultOptions(), validateOptions(nil))
	})

	t.Run("MaxWALTime should not be lower than TruncateFrequency", func(t *testing.T) {
		opts := validateOptions(&Options{
			MaxWALTime:        int64(time.Hour / time.Millisecond),
			TruncateFrequency: 2 * time.Hour,
		})
		require.Equal(t, int64(2*time.Hour/time.Millisecond), opts.MaxWALTime)
	})
}

func startTime() (int64, error) {
	return time.Now().Unix() * 1000, nil
}

// Create series for tests.
func labelsForTest(lName string, seriesCount int) [][]labels.Label {
	var series [][]labels.Label

	for i := range seriesCount {
		lset := []labels.Label{
			{Name: "a", Value: lName},
			{Name: "instance", Value: "localhost" + strconv.Itoa(i)},
			{Name: "job", Value: "prometheus"},
		}
		series = append(series, lset)
	}

	return series
}

func gatherFamily(t *testing.T, reg prometheus.Gatherer, familyName string) *dto.MetricFamily {
	t.Helper()

	families, err := reg.Gather()
	require.NoError(t, err, "failed to gather metrics")

	for _, f := range families {
		if f.GetName() == familyName {
			return f
		}
	}

	t.Fatalf("could not find family %s", familyName)

	return nil
}

type walSample struct {
	st   int64
	t    int64
	f    float64
	h    *histogram.Histogram
	fh   *histogram.FloatHistogram
	lbls labels.Labels
	ref  storage.SeriesRef
}

func TestDuplicateSeriesRefsByHash(t *testing.T) {
	for _, inMemCheckpoint := range []bool{false, true} {
		t.Run("CheckpointFromInMemorySeries="+strconv.FormatBool(inMemCheckpoint), func(t *testing.T) {
			dbDir := t.TempDir()
			opts := DefaultOptions()
			opts.EnableMetadataWALRecords = true
			opts.CheckpointFromInMemorySeries = inMemCheckpoint
			rs1 := remote.NewStorage(promslog.NewNopLogger(), nil, startTime, dbDir, time.Second*30, nil, false, opts.EnableMetadataWALRecords, opts.EnableSTStorage)
			db, err := Open(promslog.NewNopLogger(), nil, rs1, dbDir, opts)
			require.NoError(t, err)

			app := db.AppenderV2(context.Background())

			initialMeta := metadata.Metadata{Type: model.MetricTypeGauge, Unit: "bytes", Help: "initial metadata"}
			updatedMeta := metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: "updated metadata"}

			metricNames := []string{"foo", "bar", "baz", "blerg"}
			originalSeriesRefs := make([]chunks.HeadSeriesRef, 0, len(metricNames))
			for _, metricName := range metricNames {
				lbls := labels.FromMap(map[string]string{"__name__": metricName})

				ref, err := app.Append(storage.SeriesRef(0), lbls, 0, int64(0), 10.0, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				originalSeriesRefs = append(originalSeriesRefs, chunks.HeadSeriesRef(ref))
				ref2, err := app.Append(ref, lbls, 0, int64(10), 100.0, nil, nil, storage.AOptions{Metadata: initialMeta})
				require.NoError(t, err)
				require.Equal(t, ref, ref2)
			}
			require.NoError(t, app.Commit())

			// Forcefully create a bunch of new segments to force a truncation.
			for range 3 {
				_, err := db.wal.NextSegmentSync()
				require.NoError(t, err)
			}
			// No series should be deleted yet
			require.Empty(t, db.deleted)

			// Truncate at 1 ms higher than the highest timestamp.
			err = db.truncate(11)
			require.NoError(t, err)

			// The original SeriesRefs should be considered deleted.
			for _, ref := range originalSeriesRefs {
				require.Nil(t, db.series.GetByID(ref))
				require.Contains(t, db.deleted, ref)
			}

			duplicateSeriesRefs := make([]chunks.HeadSeriesRef, 0, len(metricNames))
			for i, metricName := range metricNames {
				lbls := labels.FromMap(map[string]string{"__name__": metricName})
				var aopts storage.AOptions
				// Leave i==0 without a new metadata record in the segment so replay must
				// recover initialMeta from the deleted series record in the checkpoint.
				if i > 0 {
					aopts.Metadata = updatedMeta
				}
				ref, err := app.Append(storage.SeriesRef(0), lbls, 0, int64(20), 10.0, nil, nil, aopts)
				require.NoError(t, err)
				duplicateSeriesRefs = append(duplicateSeriesRefs, chunks.HeadSeriesRef(ref))
			}
			require.NoError(t, app.Commit())

			// Write a raw metadata record for a non-existent series ref to exercise the nil series skip path on replay.
			var enc record.Encoder
			require.NoError(t, db.wal.Log(enc.Metadata([]record.RefMetadata{{
				Ref:  9999,
				Type: record.GetMetricType(model.MetricTypeGauge),
				Unit: "bytes",
				Help: "orphan metadata",
			}}, nil)))

			// The duplicate SeriesRefs should be in series.
			for _, ref := range duplicateSeriesRefs {
				require.NotNil(t, db.series.GetByID(ref))
			}

			// Close the WAL before we have a chance to remove the original RefIDs.
			// Both db and rs1 must be closed to release all file handles before
			// reopening the same directory — important on Windows.
			require.NoError(t, db.Close())
			require.NoError(t, rs1.Close())

			rs2 := remote.NewStorage(promslog.NewNopLogger(), nil, startTime, dbDir, time.Second*30, nil, false, opts.EnableMetadataWALRecords, opts.EnableSTStorage)
			t.Cleanup(func() { require.NoError(t, rs2.Close()) })
			db, err = Open(promslog.NewNopLogger(), nil, rs2, dbDir, opts)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })

			// The original SeriesRefs should be in series with initialMeta (for i==0, recovered from
			// the deleted series in the checkpoint) or updatedMeta (for i>0, remapped from duplicateSeriesRefs).
			for i, ref := range originalSeriesRefs {
				s := db.series.GetByID(ref)
				require.NotNil(t, s)
				if i == 0 {
					require.Equal(t, initialMeta, s.Metadata().Value())
				} else {
					require.Equal(t, updatedMeta, s.Metadata().Value())
				}
				require.NotContains(t, db.deleted, ref)
			}

			// The duplicated SeriesRefs should be considered deleted.
			for i, ref := range duplicateSeriesRefs {
				require.Nil(t, db.series.GetByID(ref))
				require.Contains(t, db.deleted, ref)
				if inMemCheckpoint && i > 0 {
					require.Equal(t, updatedMeta, db.deleted[ref].meta.Value())
				}
			}
		})
	}
}

func readWALSamples(t *testing.T, walDir string) []walSample {
	t.Helper()
	sr, err := wlog.NewSegmentsReader(walDir)
	require.NoError(t, err)
	defer func(sr io.ReadCloser) {
		err := sr.Close()
		require.NoError(t, err)
	}(sr)

	r := wlog.NewReader(sr)
	dec := record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())

	var (
		samples         []record.RefSample
		histograms      []record.RefHistogramSample
		floatHistograms []record.RefFloatHistogramSample

		lastSeries    record.RefSeries
		outputSamples = make([]walSample, 0)
	)

	for r.Next() {
		rec := r.Record()
		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, nil)
			require.NoError(t, err)
			lastSeries = series[0]
		case record.Samples, record.SamplesV2:
			samples, err = dec.Samples(rec, samples[:0])
			require.NoError(t, err)
			for _, s := range samples {
				outputSamples = append(outputSamples, walSample{
					st:   s.ST,
					t:    s.T,
					f:    s.V,
					lbls: lastSeries.Labels.Copy(),
					ref:  storage.SeriesRef(lastSeries.Ref),
				})
			}
		case record.HistogramSamples, record.CustomBucketsHistogramSamples, record.HistogramSamplesV2:
			histograms, err = dec.HistogramSamples(rec, histograms[:0])
			require.NoError(t, err)
			for _, h := range histograms {
				outputSamples = append(outputSamples, walSample{
					st:   h.ST,
					t:    h.T,
					h:    h.H,
					lbls: lastSeries.Labels.Copy(),
					ref:  storage.SeriesRef(lastSeries.Ref),
				})
			}
		case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples, record.FloatHistogramSamplesV2:
			floatHistograms, err = dec.FloatHistogramSamples(rec, floatHistograms[:0])
			require.NoError(t, err)
			for _, h := range floatHistograms {
				outputSamples = append(outputSamples, walSample{
					st:   h.ST,
					t:    h.T,
					fh:   h.FH,
					lbls: lastSeries.Labels.Copy(),
					ref:  storage.SeriesRef(lastSeries.Ref),
				})
			}
		}
	}
	return outputSamples
}

func BenchmarkGetOrCreate(b *testing.B) {
	// NOTE: This benchmarks appenderBase, so it does not matter if it's V1 or V2.
	const n = 1_000

	b.Run("new", func(b *testing.B) {
		s := createTestAgentDB(b, nil, DefaultOptions())
		defer s.Close()
		app := s.AppenderV2(context.Background()).(*appenderV2)

		// Fixed-size label set. Before each pass through the set we GC all series
		// (they are created with lastTs==math.MinInt64, so mint=math.MaxInt64
		// evicts everything) so every timed getOrCreate call takes the creation
		// path. This keeps the stripe-series table at a stable size regardless of
		// b.N, preventing per-op cost from growing with the benchmark iteration
		// count.
		lbls := make([]labels.Labels, n)
		for i, l := range labelsForTest("benchmark_new", n) {
			lbls[i] = labels.New(l...)
		}

		b.ResetTimer()

		for i := range b.N {
			if i%n == 0 && i > 0 {
				b.StopTimer()
				_ = s.series.GC(math.MaxInt64, false)
				b.StartTimer()
			}
			app.getOrCreate(0, lbls[i%n])
		}
	})

	b.Run("existing", func(b *testing.B) {
		s := createTestAgentDB(b, nil, DefaultOptions())
		defer s.Close()
		app := s.AppenderV2(context.Background()).(*appenderV2)

		lbls := make([]labels.Labels, n)
		for i, l := range labelsForTest("benchmark_existing", n) {
			lbls[i] = labels.New(l...)
		}

		// Pre-populate all series so every timed call finds an existing series.
		for _, l := range lbls {
			app.getOrCreate(0, l)
		}

		b.ResetTimer()

		for i := range b.N {
			app.getOrCreate(0, lbls[i%n])
		}
	})
}

func readTestWAL(t testing.TB, dir string) (recs []any) {
	sr, err := wlog.NewSegmentsReader(dir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, sr.Close())
	}()

	dec := record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
	r := wlog.NewReader(sr)

	for r.Next() {
		rec := r.Record()

		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, nil)
			require.NoError(t, err)
			recs = append(recs, series)
		case record.Samples, record.SamplesV2:
			samples, err := dec.Samples(rec, nil)
			require.NoError(t, err)
			recs = append(recs, samples)
		case record.HistogramSamples, record.CustomBucketsHistogramSamples, record.HistogramSamplesV2:
			samples, err := dec.HistogramSamples(rec, nil)
			require.NoError(t, err)
			recs = append(recs, samples)
		case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples, record.FloatHistogramSamplesV2:
			samples, err := dec.FloatHistogramSamples(rec, nil)
			require.NoError(t, err)
			recs = append(recs, samples)
		case record.Tombstones:
			tstones, err := dec.Tombstones(rec, nil)
			require.NoError(t, err)
			recs = append(recs, tstones)
		case record.Metadata:
			meta, err := dec.Metadata(rec, nil)
			require.NoError(t, err)
			recs = append(recs, meta)
		case record.Exemplars:
			exemplars, err := dec.Exemplars(rec, nil)
			require.NoError(t, err)
			recs = append(recs, exemplars)
		case record.MinValidTime:
			_, err := dec.MinValidTime(rec)
			require.NoError(t, err)
		default:
			require.Fail(t, "unknown record type")
		}
	}
	require.NoError(t, r.Err())
	return recs
}

func TestUpdateMetadata(t *testing.T) {
	t.Run("feature disabled", func(t *testing.T) {
		opts := DefaultOptions()
		opts.EnableMetadataWALRecords = false
		s := createTestAgentDB(t, nil, opts)
		defer s.Close()

		app := s.Appender(t.Context())
		lbls := labels.FromStrings("__name__", "m1")
		ref, err := app.Append(0, lbls, 1000, 1.0)
		require.NoError(t, err)

		m := metadata.Metadata{Type: model.MetricTypeGauge, Unit: "bytes", Help: "help"}
		_, err = app.UpdateMetadata(ref, lbls, m)
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		// WAL should not contain any metadata records.
		recs := readTestWAL(t, s.wal.Dir())
		require.NotEmpty(t, recs)
		for _, rec := range recs {
			_, ok := rec.([]record.RefMetadata)
			require.False(t, ok, "unexpected metadata record in WAL when feature is disabled")
		}
	})

	t.Run("empty metadata", func(t *testing.T) {
		opts := DefaultOptions()
		opts.EnableMetadataWALRecords = true
		s := createTestAgentDB(t, nil, opts)
		defer s.Close()

		app := s.Appender(t.Context())
		lbls := labels.FromStrings("__name__", "m1")
		ref, err := app.Append(0, lbls, 1000, 1.0)
		require.NoError(t, err)

		metaRef, err := app.UpdateMetadata(ref, lbls, metadata.Metadata{})
		require.NoError(t, err)
		require.Equal(t, storage.SeriesRef(0), metaRef)
		require.NoError(t, app.Commit())

		memS := s.series.GetByID(chunks.HeadSeriesRef(ref))
		require.NotNil(t, memS)
		require.True(t, metadata.IsZeroHandle(memS.Metadata()))

		recs := readTestWAL(t, s.wal.Dir())
		require.NotEmpty(t, recs)
		for _, rec := range recs {
			_, ok := rec.([]record.RefMetadata)
			require.False(t, ok, "unexpected metadata record in WAL for empty metadata")
		}
	})

	t.Run("unknown series", func(t *testing.T) {
		opts := DefaultOptions()
		opts.EnableMetadataWALRecords = true
		s := createTestAgentDB(t, nil, opts)
		defer s.Close()

		app := s.Appender(t.Context())
		m := metadata.Metadata{Type: model.MetricTypeGauge, Unit: "bytes", Help: "help"}
		_, err := app.UpdateMetadata(0, labels.FromStrings("__name__", "unknown"), m)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown series")
	})

	t.Run("metadata logged, deduplicated, and updated", func(t *testing.T) {
		opts := DefaultOptions()
		opts.EnableMetadataWALRecords = true
		s := createTestAgentDB(t, nil, opts)
		defer s.Close()

		ctx := t.Context()
		s1 := labels.FromStrings("__name__", "m1")
		s2 := labels.FromStrings("__name__", "m2")

		m1 := metadata.Metadata{Type: model.MetricTypeGauge, Unit: "seconds", Help: "help 1"}
		m2 := metadata.Metadata{Type: model.MetricTypeCounter, Unit: "bytes", Help: "help 2"}

		// First transaction: create series and update metadata.
		app := s.Appender(ctx)
		ref1, err := app.Append(0, s1, 1000, 1.0)
		require.NoError(t, err)
		ref2, err := app.Append(0, s2, 1000, 2.0)
		require.NoError(t, err)

		gotRef1, err := app.UpdateMetadata(ref1, s1, m1)
		require.NoError(t, err)
		require.Equal(t, ref1, gotRef1)
		// Pass 0 as ref to exercise the GetByHash fallback for an existing series.
		gotRef2, err := app.UpdateMetadata(0, s2, m2)
		require.NoError(t, err)
		require.Equal(t, ref2, gotRef2)
		require.NoError(t, app.Commit())

		// Check in-memory metadata.
		memS1 := s.series.GetByID(chunks.HeadSeriesRef(ref1))
		require.NotNil(t, memS1)
		require.Equal(t, m1, memS1.Metadata().Value())

		memS2 := s.series.GetByID(chunks.HeadSeriesRef(ref2))
		require.NotNil(t, memS2)
		require.Equal(t, m2, memS2.Metadata().Value())

		// Second transaction: append same metadata (dedup) for s1, new metadata for s2.
		m2Updated := metadata.Metadata{Type: model.MetricTypeCounter, Unit: "bytes", Help: "help 2 updated"}
		app = s.Appender(ctx)
		_, err = app.Append(ref1, s1, 2000, 1.5)
		require.NoError(t, err)
		_, err = app.UpdateMetadata(ref1, s1, m1)
		require.NoError(t, err)

		_, err = app.Append(ref2, s2, 2000, 2.5)
		require.NoError(t, err)
		_, err = app.UpdateMetadata(ref2, s2, m2Updated)
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		// Verify in-memory updated metadata.
		require.Equal(t, m2Updated, memS2.Metadata().Value())

		// Verify WAL content: expected 2 metadata blocks (first has m1, m2; second has only m2Updated).
		recs := readTestWAL(t, s.wal.Dir())
		var gotMeta [][]record.RefMetadata
		for _, rec := range recs {
			if mr, ok := rec.([]record.RefMetadata); ok {
				gotMeta = append(gotMeta, mr)
			}
		}
		require.Len(t, gotMeta, 2)
		require.Equal(t, []record.RefMetadata{
			{Ref: chunks.HeadSeriesRef(ref1), Type: record.GetMetricType(m1.Type), Unit: m1.Unit, Help: m1.Help},
			{Ref: chunks.HeadSeriesRef(ref2), Type: record.GetMetricType(m2.Type), Unit: m2.Unit, Help: m2.Help},
		}, gotMeta[0])
		require.Equal(t, []record.RefMetadata{
			{Ref: chunks.HeadSeriesRef(ref2), Type: record.GetMetricType(m2Updated.Type), Unit: m2Updated.Unit, Help: m2Updated.Help},
		}, gotMeta[1])
	})

	t.Run("rollback does not update metadata", func(t *testing.T) {
		opts := DefaultOptions()
		opts.EnableMetadataWALRecords = true
		s := createTestAgentDB(t, nil, opts)
		defer s.Close()

		ctx := t.Context()
		s1 := labels.FromStrings("__name__", "m1")
		m1 := metadata.Metadata{Type: model.MetricTypeGauge, Unit: "seconds", Help: "help 1"}

		app := s.Appender(ctx)
		ref1, err := app.Append(0, s1, 1000, 1.0)
		require.NoError(t, err)
		_, err = app.UpdateMetadata(ref1, s1, m1)
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		// Start second transaction and rollback.
		m1New := metadata.Metadata{Type: model.MetricTypeGauge, Unit: "minutes", Help: "help 1 changed"}
		app = s.Appender(ctx)
		_, err = app.UpdateMetadata(ref1, s1, m1New)
		require.NoError(t, err)
		require.NoError(t, app.Rollback())

		// Verify in-memory metadata was NOT changed.
		memS1 := s.series.GetByID(chunks.HeadSeriesRef(ref1))
		require.NotNil(t, memS1)
		require.Equal(t, m1, memS1.Metadata().Value())
	})
}

func TestWALReplayMetadata(t *testing.T) {
	opts := DefaultOptions()
	opts.EnableMetadataWALRecords = true
	dir := t.TempDir()

	l := promslog.NewNopLogger()
	rs := remote.NewStorage(promslog.NewNopLogger(), nil, startTime, dir, 30*time.Second, nil, false, opts.EnableMetadataWALRecords, opts.EnableSTStorage)
	defer rs.Close()

	db, err := Open(l, nil, rs, dir, opts)
	require.NoError(t, err)

	ctx := t.Context()
	s1 := labels.FromStrings("__name__", "cpu_usage")
	s2 := labels.FromStrings("__name__", "mem_bytes")

	m1 := metadata.Metadata{Type: model.MetricTypeGauge, Unit: "percent", Help: "CPU usage percentage"}
	m2 := metadata.Metadata{Type: model.MetricTypeGauge, Unit: "bytes", Help: "Memory used in bytes"}

	app := db.AppenderV2(ctx)
	ref1, err := app.Append(0, s1, 0, 1000, 50.0, nil, nil, storage.AOptions{Metadata: m1})
	require.NoError(t, err)
	ref2, err := app.Append(0, s2, 0, 1000, 1024.0, nil, nil, storage.AOptions{Metadata: m2})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.NoError(t, db.Close())

	// Reopen DB with EnableMetadataWALRecords=true and verify metadata is restored during WAL replay.
	db2, err := Open(l, nil, rs, dir, opts)
	require.NoError(t, err)

	mem1 := db2.series.GetByID(chunks.HeadSeriesRef(ref1))
	require.NotNil(t, mem1)
	require.Equal(t, m1, mem1.Metadata().Value())

	mem2 := db2.series.GetByID(chunks.HeadSeriesRef(ref2))
	require.NotNil(t, mem2)
	require.Equal(t, m2, mem2.Metadata().Value())
	require.NoError(t, db2.Close())

	// Reopen DB with EnableMetadataWALRecords=false and verify existing metadata in WAL is still restored.
	optsDisabled := DefaultOptions()
	optsDisabled.EnableMetadataWALRecords = false
	db3, err := Open(l, nil, rs, dir, optsDisabled)
	require.NoError(t, err)
	defer db3.Close()

	mem1 = db3.series.GetByID(chunks.HeadSeriesRef(ref1))
	require.NotNil(t, mem1)
	require.Equal(t, m1, mem1.Metadata().Value())

	mem2 = db3.series.GetByID(chunks.HeadSeriesRef(ref2))
	require.NotNil(t, mem2)
	require.Equal(t, m2, mem2.Metadata().Value())
}

func TestAppender_GetRef(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	rs := remote.NewStorage(promslog.NewNopLogger(), nil, startTime, dir, 30*time.Second, nil, false, opts.EnableMetadataWALRecords, opts.EnableSTStorage)
	t.Cleanup(func() {
		require.NoError(t, rs.Close())
	})

	db, err := Open(promslog.NewNopLogger(), nil, rs, dir, opts)
	require.NoError(t, err)

	ctx := t.Context()
	stored := labels.FromStrings(model.MetricNameLabel, "test_metric", "job", "prometheus")
	lookup := labels.FromStrings(model.MetricNameLabel, "test_metric", "job", "prometheus")
	require.NotSame(t, unsafe.StringData(stored.Get("job")), unsafe.StringData(lookup.Get("job")))

	// Series miss returns (0, EmptyLabels).
	app := db.Appender(ctx)
	gr, ok := app.(storage.GetRef)
	require.True(t, ok)
	gotRef, gotLset := gr.GetRef(lookup, lookup.Hash())
	require.Zero(t, gotRef)
	require.True(t, gotLset.IsEmpty())

	// Append series and verify GetRef returns the canonical ref and label backing memory.
	wantRef, err := app.Append(0, stored, 1000, 1.0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = db.Appender(ctx)
	gr = app.(storage.GetRef)
	gotRef, gotLset = gr.GetRef(lookup, lookup.Hash())
	require.Equal(t, wantRef, gotRef)
	testutil.RequireEqual(t, stored, gotLset)
	require.Same(t, unsafe.StringData(stored.Get("job")), unsafe.StringData(gotLset.Get("job")))
	require.NotSame(t, unsafe.StringData(lookup.Get("job")), unsafe.StringData(gotLset.Get("job")))
	require.NoError(t, app.Rollback())

	// Close and reopen DB to replay the WAL; GetRef must return the replayed series and its canonical labels.
	require.NoError(t, db.Close())

	db2, err := Open(promslog.NewNopLogger(), nil, rs, dir, opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db2.Close())
	})

	replayed := db2.series.GetByID(chunks.HeadSeriesRef(wantRef))
	require.NotNil(t, replayed)

	app2 := db2.Appender(ctx)
	gr2 := app2.(storage.GetRef)
	gotRef, gotLset = gr2.GetRef(lookup, lookup.Hash())
	require.Equal(t, wantRef, gotRef)
	testutil.RequireEqual(t, stored, gotLset)
	require.Same(t, unsafe.StringData(replayed.Labels().Get("job")), unsafe.StringData(gotLset.Get("job")))
	require.NotSame(t, unsafe.StringData(lookup.Get("job")), unsafe.StringData(gotLset.Get("job")))
	require.NoError(t, app2.Rollback())

	// GC the inactive series and verify GetRef returns (0, EmptyLabels).
	db2.gc(1001)

	app2 = db2.Appender(ctx)
	gr2 = app2.(storage.GetRef)
	gotRef, gotLset = gr2.GetRef(lookup, lookup.Hash())
	require.Zero(t, gotRef)
	require.True(t, gotLset.IsEmpty())
	require.NoError(t, app2.Rollback())
}

type pooledAppenderWeakRefs struct {
	msFloat      weak.Pointer[memSeries]
	msHist       weak.Pointer[memSeries]
	msFloatHist  weak.Pointer[memSeries]
	hist         weak.Pointer[histogram.Histogram]
	floatHist    weak.Pointer[histogram.FloatHistogram]
	seriesLabel  weak.Pointer[byte]
	exemplarLbl  weak.Pointer[byte]
	metadataHelp weak.Pointer[byte]
}

func newIsolatedLabels(name, key string) labels.Labels {
	sb := labels.NewScratchBuilder(2)
	sb.SetSymbolTable(labels.NewSymbolTable())
	sb.Add("__name__", name)
	sb.Add(key, string(bytes.Repeat([]byte{'x'}, 64)))
	sb.Sort()
	return sb.Labels()
}

//go:noinline
func populateAppenderForWeakRefTest(t *testing.T, db *DB, appV2, commit bool) (*appenderBase, pooledAppenderWeakRefs) {
	t.Helper()

	lFloat := newIsolatedLabels("test_float", "k")
	lHist := newIsolatedLabels("test_hist", "k")
	lFloatHist := newIsolatedLabels("test_float_hist", "k")
	lExemplar := newIsolatedLabels("trace", "trace_id")
	metaHelp := string(bytes.Repeat([]byte{'h'}, 64))
	metaUnit := string(bytes.Repeat([]byte{'u'}, 32))
	meta := metadata.Metadata{
		Type: model.MetricTypeGauge,
		Unit: metaUnit,
		Help: metaHelp,
	}

	h := tsdbutil.GenerateTestHistogram(1)
	fh := tsdbutil.GenerateTestFloatHistogram(2)
	ex := exemplar.Exemplar{Labels: lExemplar, Value: 1, Ts: 1000, HasTs: true}

	var (
		base                     *appenderBase
		tx                       storage.AppenderTransaction
		refFloat, refHist, refFH storage.SeriesRef
		err                      error
	)
	if appV2 {
		app := db.AppenderV2(t.Context())
		base = &app.(*appenderV2).appenderBase
		tx = app

		refFloat, err = app.Append(0, lFloat, 0, 1000, 1.5, nil, nil, storage.AOptions{
			Exemplars: []exemplar.Exemplar{ex},
			Metadata:  meta,
		})
		require.NoError(t, err)
		refHist, err = app.Append(0, lHist, 0, 1000, 0, h, nil, storage.AOptions{})
		require.NoError(t, err)
		refFH, err = app.Append(0, lFloatHist, 0, 1000, 0, nil, fh, storage.AOptions{})
		require.NoError(t, err)
	} else {
		app := db.Appender(t.Context())
		app.SetOptions(&storage.AppendOptions{DiscardOutOfOrder: true})
		base = &app.(*appender).appenderBase
		tx = app

		refFloat, err = app.Append(0, lFloat, 1000, 1.5)
		require.NoError(t, err)
		_, err = app.AppendExemplar(refFloat, lFloat, ex)
		require.NoError(t, err)
		_, err = app.UpdateMetadata(refFloat, lFloat, meta)
		require.NoError(t, err)
		refHist, err = app.AppendHistogram(0, lHist, 1000, h, nil)
		require.NoError(t, err)
		refFH, err = app.AppendHistogram(0, lFloatHist, 1000, nil, fh)
		require.NoError(t, err)
	}

	msFloat := db.series.GetByID(chunks.HeadSeriesRef(refFloat))
	msHist := db.series.GetByID(chunks.HeadSeriesRef(refHist))
	msFloatHist := db.series.GetByID(chunks.HeadSeriesRef(refFH))
	require.NotNil(t, msFloat)
	require.NotNil(t, msHist)
	require.NotNil(t, msFloatHist)

	refs := pooledAppenderWeakRefs{
		msFloat:      weak.Make(msFloat),
		msHist:       weak.Make(msHist),
		msFloatHist:  weak.Make(msFloatHist),
		hist:         weak.Make(h),
		floatHist:    weak.Make(fh),
		seriesLabel:  weak.Make(unsafe.StringData(lFloat.Get("k"))),
		exemplarLbl:  weak.Make(unsafe.StringData(lExemplar.Get("trace_id"))),
		metadataHelp: weak.Make(unsafe.StringData(metaHelp)),
	}

	if commit {
		require.NoError(t, tx.Commit())
	} else {
		require.NoError(t, tx.Rollback())
	}
	if !appV2 {
		require.Nil(t, tx.(*appender).hints)
	}
	return base, refs
}

func TestPooledAppenderClearsReferences(t *testing.T) {
	for _, appV2 := range []bool{false, true} {
		for _, commit := range []bool{true, false} {
			finish := "rollback"
			if commit {
				finish = "commit"
			}
			t.Run(fmt.Sprintf("appV2=%v/finish=%s", appV2, finish), func(t *testing.T) {
				opts := DefaultOptions()
				opts.EnableMetadataWALRecords = true
				db := createTestAgentDB(t, nil, opts)
				t.Cleanup(func() { require.NoError(t, db.Close()) })

				base, refs := populateAppenderForWeakRefTest(t, db, appV2, commit)
				// Keep base alive across GC cycles to model a warm appender sitting in sync.Pool.
				defer runtime.KeepAlive(base)

				// Evict all series and exemplars from the active stripe map so only a bug in
				// clearData could keep them or their labels/metadata reachable.
				_ = db.series.GC(math.MaxInt64, false)

				for range 4 {
					runtime.GC()
				}

				require.Nil(t, refs.hist.Value(), "pooled appender retained *histogram.Histogram")
				require.Nil(t, refs.floatHist.Value(), "pooled appender retained *histogram.FloatHistogram")
				require.Nil(t, refs.msFloat.Value(), "pooled appender retained float *memSeries")
				require.Nil(t, refs.msHist.Value(), "pooled appender retained histogram *memSeries")
				require.Nil(t, refs.msFloatHist.Value(), "pooled appender retained float histogram *memSeries")
				require.Nil(t, refs.seriesLabel.Value(), "pooled appender retained series label string")
				require.Nil(t, refs.exemplarLbl.Value(), "pooled appender retained exemplar label string")
				require.Nil(t, refs.metadataHelp.Value(), "pooled appender retained metadata help string")

				require.NotZero(t, cap(base.pendingSeries))
				for i, v := range base.pendingSeries[:cap(base.pendingSeries)] {
					require.Zero(t, v, "non-zero pendingSeries[%d]", i)
				}
				require.NotZero(t, cap(base.pendingHistograms))
				for i, v := range base.pendingHistograms[:cap(base.pendingHistograms)] {
					require.Zero(t, v, "non-zero pendingHistograms[%d]", i)
				}
				require.NotZero(t, cap(base.pendingFloatHistograms))
				for i, v := range base.pendingFloatHistograms[:cap(base.pendingFloatHistograms)] {
					require.Zero(t, v, "non-zero pendingFloatHistograms[%d]", i)
				}
				require.NotZero(t, cap(base.pendingExamplars))
				for i, v := range base.pendingExamplars[:cap(base.pendingExamplars)] {
					require.Zero(t, v, "non-zero pendingExamplars[%d]", i)
				}
				require.NotZero(t, cap(base.pendingMetadata))
				for i, v := range base.pendingMetadata[:cap(base.pendingMetadata)] {
					require.Zero(t, v, "non-zero pendingMetadata[%d]", i)
				}
				require.NotZero(t, cap(base.sampleSeries))
				for i, v := range base.sampleSeries[:cap(base.sampleSeries)] {
					require.Nil(t, v, "non-nil sampleSeries[%d]", i)
				}
				require.NotZero(t, cap(base.histogramSeries))
				for i, v := range base.histogramSeries[:cap(base.histogramSeries)] {
					require.Nil(t, v, "non-nil histogramSeries[%d]", i)
				}
				require.NotZero(t, cap(base.floatHistogramSeries))
				for i, v := range base.floatHistogramSeries[:cap(base.floatHistogramSeries)] {
					require.Nil(t, v, "non-nil floatHistogramSeries[%d]", i)
				}
				require.NotZero(t, cap(base.metadataSeries))
				for i, v := range base.metadataSeries[:cap(base.metadataSeries)] {
					require.Nil(t, v, "non-nil metadataSeries[%d]", i)
				}
			})
		}
	}
}

// Recommended CLI invocation:
/*
	export bench=agentClearData && go test ./tsdb/agent/... \
		-run '^$' -bench '^BenchmarkAppenderClearData$' \
		-benchtime 1s -count 6 -cpu 2 -timeout 999m -benchmem \
		| tee ${bench}.txt
*/
func BenchmarkAppenderClearData(b *testing.B) {
	b.Run("fresh_appender_100_samples", func(b *testing.B) {
		db := createTestAgentDB(b, nil, DefaultOptions())
		b.Cleanup(func() { require.NoError(b, db.Close()) })

		const batchSize = 100
		lsets := make([]labels.Labels, batchSize)
		refs := make([]storage.SeriesRef, batchSize)
		initApp := db.AppenderV2(b.Context())
		for i := range batchSize {
			lsets[i] = labels.FromStrings("__name__", "bench_metric", "i", strconv.Itoa(i))
			ref, err := initApp.Append(0, lsets[i], 0, 1, 1.0, nil, nil, storage.AOptions{})
			require.NoError(b, err)
			refs[i] = ref
		}
		require.NoError(b, initApp.Commit())

		var ts int64 = 2
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			ts++
			app := db.appenderV2Pool.New().(storage.AppenderV2)
			for i := range batchSize {
				if _, err := app.Append(refs[i], lsets[i], 0, ts, float64(ts), nil, nil, storage.AOptions{}); err != nil {
					b.Fatal(err)
				}
			}
			if err := app.Commit(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("warm_appender_500_samples", func(b *testing.B) {
		db := createTestAgentDB(b, nil, DefaultOptions())
		b.Cleanup(func() { require.NoError(b, db.Close()) })

		const batchSize = 500
		lsets := make([]labels.Labels, batchSize)
		refs := make([]storage.SeriesRef, batchSize)
		app := db.AppenderV2(b.Context())
		for i := range batchSize {
			lsets[i] = labels.FromStrings("__name__", "bench_metric", "i", strconv.Itoa(i))
			ref, err := app.Append(0, lsets[i], 0, 1, 1.0, nil, nil, storage.AOptions{})
			require.NoError(b, err)
			refs[i] = ref
		}
		require.NoError(b, app.Commit())

		var ts int64 = 2
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			ts++
			app := db.AppenderV2(b.Context())
			for i := range batchSize {
				_, err := app.Append(refs[i], lsets[i], 0, ts, float64(ts), nil, nil, storage.AOptions{})
				if err != nil {
					b.Fatal(err)
				}
			}
			if err := app.Commit(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
