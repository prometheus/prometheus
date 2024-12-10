// Copyright 2021 The Prometheus Authors
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
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestDB_InvalidSeries(t *testing.T) {
	s := createTestAgentDB(t, nil, DefaultOptions())
	defer s.Close()

	app := s.Appender(context.Background())

	t.Run("Samples", func(t *testing.T) {
		_, err := app.Append(0, labels.Labels{}, 0, 0)
		require.ErrorIs(t, err, tsdb.ErrInvalidSample, "should reject empty labels")

		_, err = app.Append(0, labels.FromStrings("a", "1", "a", "2"), 0, 0)
		require.ErrorIs(t, err, tsdb.ErrInvalidSample, "should reject duplicate labels")
	})

	t.Run("Histograms", func(t *testing.T) {
		_, err := app.AppendHistogram(0, labels.Labels{}, 0, tsdbutil.GenerateTestHistograms(1)[0], nil)
		require.ErrorIs(t, err, tsdb.ErrInvalidSample, "should reject empty labels")

		_, err = app.AppendHistogram(0, labels.FromStrings("a", "1", "a", "2"), 0, tsdbutil.GenerateTestHistograms(1)[0], nil)
		require.ErrorIs(t, err, tsdb.ErrInvalidSample, "should reject duplicate labels")
	})

	t.Run("Exemplars", func(t *testing.T) {
		sRef, err := app.Append(0, labels.FromStrings("a", "1"), 0, 0)
		require.NoError(t, err, "should not reject valid series")

		_, err = app.AppendExemplar(0, labels.EmptyLabels(), exemplar.Exemplar{})
		require.EqualError(t, err, "unknown series ref when trying to add exemplar: 0")

		e := exemplar.Exemplar{Labels: labels.FromStrings("a", "1", "a", "2")}
		_, err = app.AppendExemplar(sRef, labels.EmptyLabels(), e)
		require.ErrorIs(t, err, tsdb.ErrInvalidExemplar, "should reject duplicate labels")

		e = exemplar.Exemplar{Labels: labels.FromStrings("a_somewhat_long_trace_id", "nYJSNtFrFTY37VR7mHzEE/LIDt7cdAQcuOzFajgmLDAdBSRHYPDzrxhMA4zz7el8naI/AoXFv9/e/G0vcETcIoNUi3OieeLfaIRQci2oa")}
		_, err = app.AppendExemplar(sRef, labels.EmptyLabels(), e)
		require.ErrorIs(t, err, storage.ErrExemplarLabelLength, "should reject too long label length")

		// Inverse check
		e = exemplar.Exemplar{Labels: labels.FromStrings("a", "1"), Value: 20, Ts: 10, HasTs: true}
		_, err = app.AppendExemplar(sRef, labels.EmptyLabels(), e)
		require.NoError(t, err, "should not reject valid exemplars")
	})
}

func createTestAgentDB(t testing.TB, reg prometheus.Registerer, opts *Options) *DB {
	t.Helper()

	dbDir := t.TempDir()
	rs := remote.NewStorage(promslog.NewNopLogger(), reg, startTime, dbDir, time.Second*30, nil, false)
	t.Cleanup(func() {
		require.NoError(t, rs.Close())
	})

	db, err := Open(promslog.NewNopLogger(), reg, rs, dbDir, opts)
	require.NoError(t, err)
	return db
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

		for i := 0; i < numDatapoints; i++ {
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

		for i := 0; i < numHistograms; i++ {
			_, err := app.AppendHistogram(0, lset, int64(i), histograms[i], nil)
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_float_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numHistograms)

		for i := 0; i < numHistograms; i++ {
			_, err := app.AppendHistogram(0, lset, int64(i), nil, floatHistograms[i])
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
		dec = record.NewDecoder(labels.NewSymbolTable())

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

		case record.Samples:
			var samples []record.RefSample
			samples, err = dec.Samples(rec, samples)
			require.NoError(t, err)
			walSamplesCount += len(samples)

		case record.HistogramSamples:
			var histograms []record.RefHistogramSample
			histograms, err = dec.HistogramSamples(rec, histograms)
			require.NoError(t, err)
			walHistogramCount += len(histograms)

		case record.FloatHistogramSamples:
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
	require.Equal(t, numSeries*3, walSeriesCount, "unexpected number of series")
	require.Equal(t, numSeries*numDatapoints, walSamplesCount, "unexpected number of samples")
	require.Equal(t, numSeries*numDatapoints, walExemplarsCount, "unexpected number of exemplars")
	require.Equal(t, numSeries*numHistograms, walHistogramCount, "unexpected number of histograms")
	require.Equal(t, numSeries*numHistograms, walFloatHistogramCount, "unexpected number of float histograms")
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

		for i := 0; i < numDatapoints; i++ {
			sample := chunks.GenerateSamples(0, 1)
			_, err := app.Append(0, lset, sample[0].T(), sample[0].F())
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestHistograms(numHistograms)

		for i := 0; i < numHistograms; i++ {
			_, err := app.AppendHistogram(0, lset, int64(i), histograms[i], nil)
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_float_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numHistograms)

		for i := 0; i < numHistograms; i++ {
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
		dec = record.NewDecoder(labels.NewSymbolTable())

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

		case record.Samples:
			var samples []record.RefSample
			samples, err = dec.Samples(rec, samples)
			require.NoError(t, err)
			walSamplesCount += len(samples)

		case record.Exemplars:
			var exemplars []record.RefExemplar
			exemplars, err = dec.Exemplars(rec, exemplars)
			require.NoError(t, err)
			walExemplarsCount += len(exemplars)

		case record.HistogramSamples:
			var histograms []record.RefHistogramSample
			histograms, err = dec.HistogramSamples(rec, histograms)
			require.NoError(t, err)
			walHistogramCount += len(histograms)

		case record.FloatHistogramSamples:
			var floatHistograms []record.RefFloatHistogramSample
			floatHistograms, err = dec.FloatHistogramSamples(rec, floatHistograms)
			require.NoError(t, err)
			walFloatHistogramCount += len(floatHistograms)

		default:
		}
	}

	// Check that only series get stored after calling Rollback.
	require.Equal(t, numSeries*3, walSeriesCount, "series should have been written to WAL")
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

		for i := 0; i < numDatapoints; i++ {
			_, err := app.Append(0, lset, int64(lastTs), 0)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestHistograms(numHistograms)

		for i := 0; i < numHistograms; i++ {
			_, err := app.AppendHistogram(0, lset, int64(lastTs), histograms[i], nil)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_float_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numHistograms)

		for i := 0; i < numHistograms; i++ {
			_, err := app.AppendHistogram(0, lset, int64(lastTs), nil, floatHistograms[i])
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Truncate WAL with mint to GC all the samples.
	s.truncate(lastTs + 1)

	m := gatherFamily(t, reg, "prometheus_agent_deleted_series")
	require.Equal(t, float64(numSeries*3), m.Metric[0].Gauge.GetValue(), "agent wal truncate mismatch of deleted series count")
}

func TestPartialTruncateWAL(t *testing.T) {
	const (
		numDatapoints = 1000
		numSeries     = 800
	)

	opts := DefaultOptions()
	opts.TruncateFrequency = time.Minute * 2

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

		for i := 0; i < numDatapoints; i++ {
			_, err := app.Append(0, lset, lastTs, 0)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_histogram_batch-1", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestHistograms(numDatapoints)

		for i := 0; i < numDatapoints; i++ {
			_, err := app.AppendHistogram(0, lset, lastTs, histograms[i], nil)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_float_histogram_batch-1", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numDatapoints)

		for i := 0; i < numDatapoints; i++ {
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

		for i := 0; i < numDatapoints; i++ {
			_, err := app.Append(0, lset, lastTs, 0)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_histogram_batch-2", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestHistograms(numDatapoints)

		for i := 0; i < numDatapoints; i++ {
			_, err := app.AppendHistogram(0, lset, lastTs, histograms[i], nil)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	lbls = labelsForTest(t.Name()+"_float_histogram_batch-2", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numDatapoints)

		for i := 0; i < numDatapoints; i++ {
			_, err := app.AppendHistogram(0, lset, lastTs, nil, floatHistograms[i])
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Truncate WAL with mint to GC only the first batch of 800 series and retaining 2nd batch of 800 series.
	s.truncate(lastTs - 1)

	m := gatherFamily(t, reg, "prometheus_agent_deleted_series")
	require.Equal(t, float64(numSeries*3), m.Metric[0].Gauge.GetValue(), "agent wal truncate mismatch of deleted series count")
}

func TestWALReplay(t *testing.T) {
	const (
		numDatapoints = 1000
		numHistograms = 100
		numSeries     = 8
		lastTs        = 500
	)

	s := createTestAgentDB(t, nil, DefaultOptions())
	app := s.Appender(context.TODO())

	lbls := labelsForTest(t.Name(), numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		for i := 0; i < numDatapoints; i++ {
			_, err := app.Append(0, lset, lastTs, 0)
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestHistograms(numHistograms)

		for i := 0; i < numHistograms; i++ {
			_, err := app.AppendHistogram(0, lset, lastTs, histograms[i], nil)
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_float_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numHistograms)

		for i := 0; i < numHistograms; i++ {
			_, err := app.AppendHistogram(0, lset, lastTs, nil, floatHistograms[i])
			require.NoError(t, err)
		}
	}

	require.NoError(t, app.Commit())
	require.NoError(t, s.Close())

	// Hack: s.wal.Dir() is the /wal subdirectory of the original storage path.
	// We need the original directory so we can recreate the storage for replay.
	storageDir := filepath.Dir(s.wal.Dir())

	reg := prometheus.NewRegistry()
	replayStorage, err := Open(s.logger, reg, nil, storageDir, s.opts)
	if err != nil {
		t.Fatalf("unable to create storage for the agent: %v", err)
	}
	defer func() {
		require.NoError(t, replayStorage.Close())
	}()

	// Check if all the series are retrieved back from the WAL.
	m := gatherFamily(t, reg, "prometheus_agent_active_series")
	require.Equal(t, float64(numSeries*3), m.Metric[0].Gauge.GetValue(), "agent wal replay mismatch of active series count")

	// Check if lastTs of the samples retrieved from the WAL is retained.
	metrics := replayStorage.series.series
	for i := 0; i < len(metrics); i++ {
		mp := metrics[i]
		for _, v := range mp {
			require.Equal(t, v.lastTs, int64(lastTs))
		}
	}
}

func TestLockfile(t *testing.T) {
	tsdbutil.TestDirLockerUsage(t, func(t *testing.T, data string, createLock bool) (*tsdbutil.DirLocker, testutil.Closer) {
		logger := promslog.NewNopLogger()
		reg := prometheus.NewRegistry()
		rs := remote.NewStorage(logger, reg, startTime, data, time.Second*30, nil, false)
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

func Test_ExistingWAL_NextRef(t *testing.T) {
	dbDir := t.TempDir()
	rs := remote.NewStorage(promslog.NewNopLogger(), nil, startTime, dbDir, time.Second*30, nil, false)
	defer func() {
		require.NoError(t, rs.Close())
	}()

	db, err := Open(promslog.NewNopLogger(), nil, rs, dbDir, DefaultOptions())
	require.NoError(t, err)

	seriesCount := 10

	// Append <seriesCount> series
	app := db.Appender(context.Background())
	for i := 0; i < seriesCount; i++ {
		lset := labels.FromStrings(model.MetricNameLabel, fmt.Sprintf("series_%d", i))
		_, err := app.Append(0, lset, 0, 100)
		require.NoError(t, err)
	}

	histogramCount := 10
	histograms := tsdbutil.GenerateTestHistograms(histogramCount)
	// Append <histogramCount> series
	for i := 0; i < histogramCount; i++ {
		lset := labels.FromStrings(model.MetricNameLabel, fmt.Sprintf("histogram_%d", i))
		_, err := app.AppendHistogram(0, lset, 0, histograms[i], nil)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Truncate the WAL to force creation of a new segment.
	require.NoError(t, db.truncate(0))
	require.NoError(t, db.Close())

	// Create a new storage and see what nextRef is initialized to.
	db, err = Open(promslog.NewNopLogger(), nil, rs, dbDir, DefaultOptions())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	require.Equal(t, uint64(seriesCount+histogramCount), db.nextRef.Load(), "nextRef should be equal to the number of series written across the entire WAL")
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

	for i := 0; i < seriesCount; i++ {
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

func TestStorage_DuplicateExemplarsIgnored(t *testing.T) {
	s := createTestAgentDB(t, nil, DefaultOptions())
	app := s.Appender(context.Background())
	defer s.Close()

	sRef, err := app.Append(0, labels.FromStrings("a", "1"), 0, 0)
	require.NoError(t, err, "should not reject valid series")

	// Write a few exemplars to our appender and call Commit().
	// If the Labels, Value or Timestamp are different than the last exemplar,
	// then a new one should be appended; Otherwise, it should be skipped.
	e := exemplar.Exemplar{Labels: labels.FromStrings("a", "1"), Value: 20, Ts: 10, HasTs: true}
	_, _ = app.AppendExemplar(sRef, labels.EmptyLabels(), e)
	_, _ = app.AppendExemplar(sRef, labels.EmptyLabels(), e)

	e.Labels = labels.FromStrings("b", "2")
	_, _ = app.AppendExemplar(sRef, labels.EmptyLabels(), e)
	_, _ = app.AppendExemplar(sRef, labels.EmptyLabels(), e)
	_, _ = app.AppendExemplar(sRef, labels.EmptyLabels(), e)

	e.Value = 42
	_, _ = app.AppendExemplar(sRef, labels.EmptyLabels(), e)
	_, _ = app.AppendExemplar(sRef, labels.EmptyLabels(), e)

	e.Ts = 25
	_, _ = app.AppendExemplar(sRef, labels.EmptyLabels(), e)
	_, _ = app.AppendExemplar(sRef, labels.EmptyLabels(), e)

	require.NoError(t, app.Commit())

	// Read back what was written to the WAL.
	var walExemplarsCount int
	sr, err := wlog.NewSegmentsReader(s.wal.Dir())
	require.NoError(t, err)
	defer sr.Close()
	r := wlog.NewReader(sr)

	dec := record.NewDecoder(labels.NewSymbolTable())
	for r.Next() {
		rec := r.Record()
		if dec.Type(rec) == record.Exemplars {
			var exemplars []record.RefExemplar
			exemplars, err = dec.Exemplars(rec, exemplars)
			require.NoError(t, err)
			walExemplarsCount += len(exemplars)
		}
	}

	// We had 9 calls to AppendExemplar but only 4 of those should have gotten through.
	require.Equal(t, 4, walExemplarsCount)
}

func TestDBAllowOOOSamples(t *testing.T) {
	const (
		numDatapoints = 5
		numHistograms = 5
		numSeries     = 4
		offset        = 100
	)

	reg := prometheus.NewRegistry()
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = math.MaxInt64
	s := createTestAgentDB(t, reg, opts)
	app := s.Appender(context.TODO())

	// Let's add some samples in the [offset, offset+numDatapoints) range.
	lbls := labelsForTest(t.Name(), numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		for i := offset; i < numDatapoints+offset; i++ {
			ref, err := app.Append(0, lset, int64(i), float64(i))
			require.NoError(t, err)

			e := exemplar.Exemplar{
				Labels: lset,
				Ts:     int64(i) * 2,
				Value:  float64(i),
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

		for i := offset; i < numDatapoints+offset; i++ {
			_, err := app.AppendHistogram(0, lset, int64(i), histograms[i-offset], nil)
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_float_histogram", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numHistograms)

		for i := offset; i < numDatapoints+offset; i++ {
			_, err := app.AppendHistogram(0, lset, int64(i), nil, floatHistograms[i-offset])
			require.NoError(t, err)
		}
	}

	require.NoError(t, app.Commit())
	m := gatherFamily(t, reg, "prometheus_agent_samples_appended_total")
	require.Equal(t, float64(20), m.Metric[0].Counter.GetValue(), "agent wal mismatch of total appended samples")
	require.Equal(t, float64(40), m.Metric[1].Counter.GetValue(), "agent wal mismatch of total appended histograms")
	require.NoError(t, s.Close())

	// Hack: s.wal.Dir() is the /wal subdirectory of the original storage path.
	// We need the original directory so we can recreate the storage for replay.
	storageDir := filepath.Dir(s.wal.Dir())

	// Replay the storage so that the lastTs for each series is recorded.
	reg2 := prometheus.NewRegistry()
	db, err := Open(s.logger, reg2, nil, storageDir, s.opts)
	if err != nil {
		t.Fatalf("unable to create storage for the agent: %v", err)
	}

	app = db.Appender(context.Background())

	// Now the lastTs will have been recorded successfully.
	// Let's try appending twice as many OOO samples in the [0, numDatapoints) range.
	lbls = labelsForTest(t.Name()+"_histogram", numSeries*2)
	for _, l := range lbls {
		lset := labels.New(l...)

		for i := 0; i < numDatapoints; i++ {
			ref, err := app.Append(0, lset, int64(i), float64(i))
			require.NoError(t, err)

			e := exemplar.Exemplar{
				Labels: lset,
				Ts:     int64(i) * 2,
				Value:  float64(i),
				HasTs:  true,
			}
			_, err = app.AppendExemplar(ref, lset, e)
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_histogram", numSeries*2)
	for _, l := range lbls {
		lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestHistograms(numHistograms)

		for i := 0; i < numDatapoints; i++ {
			_, err := app.AppendHistogram(0, lset, int64(i), histograms[i], nil)
			require.NoError(t, err)
		}
	}

	lbls = labelsForTest(t.Name()+"_float_histogram", numSeries*2)
	for _, l := range lbls {
		lset := labels.New(l...)

		floatHistograms := tsdbutil.GenerateTestFloatHistograms(numHistograms)

		for i := 0; i < numDatapoints; i++ {
			_, err := app.AppendHistogram(0, lset, int64(i), nil, floatHistograms[i])
			require.NoError(t, err)
		}
	}

	require.NoError(t, app.Commit())
	m = gatherFamily(t, reg2, "prometheus_agent_samples_appended_total")
	require.Equal(t, float64(40), m.Metric[0].Counter.GetValue(), "agent wal mismatch of total appended samples")
	require.Equal(t, float64(80), m.Metric[1].Counter.GetValue(), "agent wal mismatch of total appended histograms")
	require.NoError(t, db.Close())
}

func TestDBOutOfOrderTimeWindow(t *testing.T) {
	tc := []struct {
		outOfOrderTimeWindow, firstTs, secondTs int64
		expectedError                           error
	}{
		{0, 100, 101, nil},
		{0, 100, 100, storage.ErrOutOfOrderSample},
		{0, 100, 99, storage.ErrOutOfOrderSample},
		{100, 100, 1, nil},
		{100, 100, 0, storage.ErrOutOfOrderSample},
	}

	for _, c := range tc {
		t.Run(fmt.Sprintf("outOfOrderTimeWindow=%d, firstTs=%d, secondTs=%d, expectedError=%s", c.outOfOrderTimeWindow, c.firstTs, c.secondTs, c.expectedError), func(t *testing.T) {
			reg := prometheus.NewRegistry()
			opts := DefaultOptions()
			opts.OutOfOrderTimeWindow = c.outOfOrderTimeWindow
			s := createTestAgentDB(t, reg, opts)
			app := s.Appender(context.TODO())

			lbls := labelsForTest(t.Name()+"_histogram", 1)
			lset := labels.New(lbls[0]...)
			_, err := app.AppendHistogram(0, lset, c.firstTs, tsdbutil.GenerateTestHistograms(1)[0], nil)
			require.NoError(t, err)
			err = app.Commit()
			require.NoError(t, err)
			_, err = app.AppendHistogram(0, lset, c.secondTs, tsdbutil.GenerateTestHistograms(1)[0], nil)
			require.ErrorIs(t, err, c.expectedError)

			lbls = labelsForTest(t.Name(), 1)
			lset = labels.New(lbls[0]...)
			_, err = app.Append(0, lset, c.firstTs, 0)
			require.NoError(t, err)
			err = app.Commit()
			require.NoError(t, err)
			_, err = app.Append(0, lset, c.secondTs, 0)
			require.ErrorIs(t, err, c.expectedError)

			expectedAppendedSamples := float64(2)
			if c.expectedError != nil {
				expectedAppendedSamples = 1
			}
			m := gatherFamily(t, reg, "prometheus_agent_samples_appended_total")
			require.Equal(t, expectedAppendedSamples, m.Metric[0].Counter.GetValue(), "agent wal mismatch of total appended samples")
			require.Equal(t, expectedAppendedSamples, m.Metric[1].Counter.GetValue(), "agent wal mismatch of total appended histograms")
			require.NoError(t, s.Close())
		})
	}
}

type walSample struct {
	t    int64
	f    float64
	h    *histogram.Histogram
	lbls labels.Labels
	ref  storage.SeriesRef
}

func TestDBCreatedTimestampSamplesIngestion(t *testing.T) {
	t.Parallel()

	type appendableSample struct {
		t            int64
		ct           int64
		v            float64
		lbls         labels.Labels
		h            *histogram.Histogram
		expectsError bool
	}

	testHistogram := tsdbutil.GenerateTestHistograms(1)[0]
	zeroHistogram := &histogram.Histogram{}

	lbls := labelsForTest(t.Name(), 1)
	defLbls := labels.New(lbls[0]...)

	testCases := []struct {
		name                string
		inputSamples        []appendableSample
		expectedSamples     []*walSample
		expectedSeriesCount int
	}{
		{
			name: "in order ct+normal sample/floatSamples",
			inputSamples: []appendableSample{
				{t: 100, ct: 1, v: 10, lbls: defLbls},
				{t: 101, ct: 1, v: 10, lbls: defLbls},
			},
			expectedSamples: []*walSample{
				{t: 1, f: 0, lbls: defLbls},
				{t: 100, f: 10, lbls: defLbls},
				{t: 101, f: 10, lbls: defLbls},
			},
		},
		{
			name: "CT+float && CT+histogram samples",
			inputSamples: []appendableSample{
				{
					t:    100,
					ct:   30,
					v:    20,
					lbls: defLbls,
				},
				{
					t:    300,
					ct:   230,
					h:    testHistogram,
					lbls: defLbls,
				},
			},
			expectedSamples: []*walSample{
				{t: 30, f: 0, lbls: defLbls},
				{t: 100, f: 20, lbls: defLbls},
				{t: 230, h: zeroHistogram, lbls: defLbls},
				{t: 300, h: testHistogram, lbls: defLbls},
			},
			expectedSeriesCount: 1,
		},
		{
			name: "CT+float && CT+histogram samples with error",
			inputSamples: []appendableSample{
				{
					// invalid CT
					t:            100,
					ct:           100,
					v:            10,
					lbls:         defLbls,
					expectsError: true,
				},
				{
					// invalid CT histogram
					t:            300,
					ct:           300,
					h:            testHistogram,
					lbls:         defLbls,
					expectsError: true,
				},
			},
			expectedSamples: []*walSample{
				{t: 100, f: 10, lbls: defLbls},
				{t: 300, h: testHistogram, lbls: defLbls},
			},
			expectedSeriesCount: 0,
		},
		{
			name: "In order ct+normal sample/histogram",
			inputSamples: []appendableSample{
				{t: 100, h: testHistogram, ct: 1, lbls: defLbls},
				{t: 101, h: testHistogram, ct: 1, lbls: defLbls},
			},
			expectedSamples: []*walSample{
				{t: 1, h: &histogram.Histogram{}},
				{t: 100, h: testHistogram},
				{t: 101, h: &histogram.Histogram{CounterResetHint: histogram.NotCounterReset}},
			},
		},
		{
			name: "ct+normal then OOO sample/float",
			inputSamples: []appendableSample{
				{t: 60_000, ct: 40_000, v: 10, lbls: defLbls},
				{t: 120_000, ct: 40_000, v: 10, lbls: defLbls},
				{t: 180_000, ct: 40_000, v: 10, lbls: defLbls},
				{t: 50_000, ct: 40_000, v: 10, lbls: defLbls},
			},
			expectedSamples: []*walSample{
				{t: 40_000, f: 0, lbls: defLbls},
				{t: 50_000, f: 10, lbls: defLbls},
				{t: 60_000, f: 10, lbls: defLbls},
				{t: 120_000, f: 10, lbls: defLbls},
				{t: 180_000, f: 10, lbls: defLbls},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reg := prometheus.NewRegistry()
			opts := DefaultOptions()
			opts.OutOfOrderTimeWindow = 360_000
			s := createTestAgentDB(t, reg, opts)
			app := s.Appender(context.TODO())

			for _, sample := range tc.inputSamples {
				// We supposed to write a Histogram to the WAL
				if sample.h != nil {
					_, err := app.AppendHistogramCTZeroSample(0, sample.lbls, sample.t, sample.ct, zeroHistogram, nil)
					if !errors.Is(err, storage.ErrOutOfOrderCT) {
						require.Equal(t, sample.expectsError, err != nil, "expected error: %v, got: %v", sample.expectsError, err)
					}

					_, err = app.AppendHistogram(0, sample.lbls, sample.t, sample.h, nil)
					require.NoError(t, err)
				} else {
					// We supposed to write a float sample to the WAL
					_, err := app.AppendCTZeroSample(0, sample.lbls, sample.t, sample.ct)
					if !errors.Is(err, storage.ErrOutOfOrderCT) {
						require.Equal(t, sample.expectsError, err != nil, "expected error: %v, got: %v", sample.expectsError, err)
					}

					_, err = app.Append(0, sample.lbls, sample.t, sample.v)
					require.NoError(t, err)
				}
			}

			require.NoError(t, app.Commit())
			// Close the DB to ensure all data is flushed to the WAL
			require.NoError(t, s.Close())

			// Check that we dont have any OOO samples in the WAL by checking metrics
			families, err := reg.Gather()
			require.NoError(t, err, "failed to gather metrics")
			for _, f := range families {
				if f.GetName() == "prometheus_agent_out_of_order_samples_total" {
					t.Fatalf("unexpected metric %s", f.GetName())
				}
			}

			outputSamples := readWALSamples(t, s.wal.Dir())

			require.Equal(t, len(tc.expectedSamples), len(outputSamples), "Expected %d samples", len(tc.expectedSamples))

			for i, expectedSample := range tc.expectedSamples {
				for _, sample := range outputSamples {
					if sample.t == expectedSample.t && sample.lbls.String() == expectedSample.lbls.String() {
						if expectedSample.h != nil {
							require.Equal(t, expectedSample.h, sample.h, "histogram value mismatch (sample index %d)", i)
						} else {
							require.Equal(t, expectedSample.f, sample.f, "value mismatch (sample index %d)", i)
						}
					}
				}
			}
		})
	}
}

func readWALSamples(t *testing.T, walDir string) []*walSample {
	t.Helper()
	sr, err := wlog.NewSegmentsReader(walDir)
	require.NoError(t, err)
	defer func(sr io.ReadCloser) {
		err := sr.Close()
		require.NoError(t, err)
	}(sr)

	r := wlog.NewReader(sr)
	dec := record.NewDecoder(labels.NewSymbolTable())

	var (
		samples    []record.RefSample
		histograms []record.RefHistogramSample

		lastSeries    record.RefSeries
		outputSamples = make([]*walSample, 0)
	)

	for r.Next() {
		rec := r.Record()
		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, nil)
			require.NoError(t, err)
			lastSeries = series[0]
		case record.Samples:
			samples, err = dec.Samples(rec, samples[:0])
			require.NoError(t, err)
			for _, s := range samples {
				outputSamples = append(outputSamples, &walSample{
					t:    s.T,
					f:    s.V,
					lbls: lastSeries.Labels.Copy(),
					ref:  storage.SeriesRef(lastSeries.Ref),
				})
			}
		case record.HistogramSamples:
			histograms, err = dec.HistogramSamples(rec, histograms[:0])
			require.NoError(t, err)
			for _, h := range histograms {
				outputSamples = append(outputSamples, &walSample{
					t:    h.T,
					h:    h.H,
					lbls: lastSeries.Labels.Copy(),
					ref:  storage.SeriesRef(lastSeries.Ref),
				})
			}
		}
	}

	return outputSamples
}

func BenchmarkCreateSeries(b *testing.B) {
	s := createTestAgentDB(b, nil, DefaultOptions())
	defer s.Close()

	app := s.Appender(context.Background()).(*appender)
	lbls := make([]labels.Labels, b.N)

	for i, l := range labelsForTest("benchmark", b.N) {
		lbls[i] = labels.New(l...)
	}

	b.ResetTimer()

	for _, l := range lbls {
		app.getOrCreate(l)
	}
}
