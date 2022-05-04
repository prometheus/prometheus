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
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-kit/log"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wal"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestDB_InvalidSeries(t *testing.T) {
	s := createTestAgentDB(t, nil, DefaultOptions())
	defer s.Close()

	app := s.Appender(context.Background())

	t.Run("Samples", func(t *testing.T) {
		_, err := app.Append(0, labels.Labels{}, 0, 0)
		require.ErrorIs(t, err, tsdb.ErrInvalidSample, "should reject empty labels")

		_, err = app.Append(0, labels.Labels{{Name: "a", Value: "1"}, {Name: "a", Value: "2"}}, 0, 0)
		require.ErrorIs(t, err, tsdb.ErrInvalidSample, "should reject duplicate labels")
	})

	t.Run("Exemplars", func(t *testing.T) {
		sRef, err := app.Append(0, labels.Labels{{Name: "a", Value: "1"}}, 0, 0)
		require.NoError(t, err, "should not reject valid series")

		_, err = app.AppendExemplar(0, nil, exemplar.Exemplar{})
		require.EqualError(t, err, "unknown series ref when trying to add exemplar: 0")

		e := exemplar.Exemplar{Labels: labels.Labels{{Name: "a", Value: "1"}, {Name: "a", Value: "2"}}}
		_, err = app.AppendExemplar(sRef, nil, e)
		require.ErrorIs(t, err, tsdb.ErrInvalidExemplar, "should reject duplicate labels")

		e = exemplar.Exemplar{Labels: labels.Labels{{Name: "a_somewhat_long_trace_id", Value: "nYJSNtFrFTY37VR7mHzEE/LIDt7cdAQcuOzFajgmLDAdBSRHYPDzrxhMA4zz7el8naI/AoXFv9/e/G0vcETcIoNUi3OieeLfaIRQci2oa"}}}
		_, err = app.AppendExemplar(sRef, nil, e)
		require.ErrorIs(t, err, storage.ErrExemplarLabelLength, "should reject too long label length")

		// Inverse check
		e = exemplar.Exemplar{Labels: labels.Labels{{Name: "a", Value: "1"}}, Value: 20, Ts: 10, HasTs: true}
		_, err = app.AppendExemplar(sRef, nil, e)
		require.NoError(t, err, "should not reject valid exemplars")
	})
}

func createTestAgentDB(t *testing.T, reg prometheus.Registerer, opts *Options) *DB {
	t.Helper()

	dbDir := t.TempDir()
	rs := remote.NewStorage(log.NewNopLogger(), reg, startTime, dbDir, time.Second*30, nil)
	t.Cleanup(func() {
		require.NoError(t, rs.Close())
	})

	db, err := Open(log.NewNopLogger(), reg, rs, dbDir, opts)
	require.NoError(t, err)
	return db
}

func TestUnsupportedFunctions(t *testing.T) {
	s := createTestAgentDB(t, nil, DefaultOptions())
	defer s.Close()

	t.Run("Querier", func(t *testing.T) {
		_, err := s.Querier(context.TODO(), 0, 0)
		require.Equal(t, err, ErrUnsupported)
	})

	t.Run("ChunkQuerier", func(t *testing.T) {
		_, err := s.ChunkQuerier(context.TODO(), 0, 0)
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
		numSeries     = 8
	)

	s := createTestAgentDB(t, nil, DefaultOptions())
	app := s.Appender(context.TODO())

	lbls := labelsForTest(t.Name(), numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		for i := 0; i < numDatapoints; i++ {
			sample := tsdbutil.GenerateSamples(0, 1)
			ref, err := app.Append(0, lset, sample[0].T(), sample[0].V())
			require.NoError(t, err)

			e := exemplar.Exemplar{
				Labels: lset,
				Ts:     sample[0].T() + int64(i),
				Value:  sample[0].V(),
				HasTs:  true,
			}
			_, err = app.AppendExemplar(ref, lset, e)
			require.NoError(t, err)
		}
	}

	require.NoError(t, app.Commit())
	require.NoError(t, s.Close())

	sr, err := wal.NewSegmentsReader(s.wal.Dir())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, sr.Close())
	}()

	// Read records from WAL and check for expected count of series, samples, and exemplars.
	var (
		r   = wal.NewReader(sr)
		dec record.Decoder

		walSeriesCount, walSamplesCount, walExemplarsCount int
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

		default:
		}
	}

	// Check that the WAL contained the same number of committed series/samples/exemplars.
	require.Equal(t, numSeries, walSeriesCount, "unexpected number of series")
	require.Equal(t, numSeries*numDatapoints, walSamplesCount, "unexpected number of samples")
	require.Equal(t, numSeries*numDatapoints, walExemplarsCount, "unexpected number of exemplars")
}

func TestRollback(t *testing.T) {
	const (
		numDatapoints = 1000
		numSeries     = 8
	)

	s := createTestAgentDB(t, nil, DefaultOptions())
	app := s.Appender(context.TODO())

	lbls := labelsForTest(t.Name(), numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		for i := 0; i < numDatapoints; i++ {
			sample := tsdbutil.GenerateSamples(0, 1)
			_, err := app.Append(0, lset, sample[0].T(), sample[0].V())
			require.NoError(t, err)
		}
	}

	// Do a rollback, which should clear uncommitted data. A followup call to
	// commit should persist nothing to the WAL.
	require.NoError(t, app.Rollback())
	require.NoError(t, app.Commit())
	require.NoError(t, s.Close())

	sr, err := wal.NewSegmentsReader(s.wal.Dir())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, sr.Close())
	}()

	// Read records from WAL and check for expected count of series and samples.
	var (
		r   = wal.NewReader(sr)
		dec record.Decoder

		walSeriesCount, walSamplesCount, walExemplarsCount int
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

		default:
		}
	}

	// Check that the rollback ensured nothing got stored.
	require.Equal(t, 0, walSeriesCount, "series should not have been written to WAL")
	require.Equal(t, 0, walSamplesCount, "samples should not have been written to WAL")
	require.Equal(t, 0, walExemplarsCount, "exemplars should not have been written to WAL")
}

func TestFullTruncateWAL(t *testing.T) {
	const (
		numDatapoints = 1000
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

	// Truncate WAL with mint to GC all the samples.
	s.truncate(lastTs + 1)

	m := gatherFamily(t, reg, "prometheus_agent_deleted_series")
	require.Equal(t, float64(numSeries), m.Metric[0].Gauge.GetValue(), "agent wal truncate mismatch of deleted series count")
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

	// Truncate WAL with mint to GC only the first batch of 800 series and retaining 2nd batch of 800 series.
	s.truncate(lastTs - 1)

	m := gatherFamily(t, reg, "prometheus_agent_deleted_series")
	require.Equal(t, m.Metric[0].Gauge.GetValue(), float64(numSeries), "agent wal truncate mismatch of deleted series count")
}

func TestWALReplay(t *testing.T) {
	const (
		numDatapoints = 1000
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
	require.Equal(t, float64(numSeries), m.Metric[0].Gauge.GetValue(), "agent wal replay mismatch of active series count")

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
		logger := log.NewNopLogger()
		reg := prometheus.NewRegistry()
		rs := remote.NewStorage(logger, reg, startTime, data, time.Second*30, nil)
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
	rs := remote.NewStorage(log.NewNopLogger(), nil, startTime, dbDir, time.Second*30, nil)
	defer func() {
		require.NoError(t, rs.Close())
	}()

	db, err := Open(log.NewNopLogger(), nil, rs, dbDir, DefaultOptions())
	require.NoError(t, err)

	seriesCount := 10

	// Append <seriesCount> series
	app := db.Appender(context.Background())
	for i := 0; i < seriesCount; i++ {
		lset := labels.Labels{
			{Name: model.MetricNameLabel, Value: fmt.Sprintf("series_%d", i)},
		}
		_, err := app.Append(0, lset, 0, 100)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Truncate the WAL to force creation of a new segment.
	require.NoError(t, db.truncate(0))
	require.NoError(t, db.Close())

	// Create a new storage and see what nextRef is initialized to.
	db, err = Open(log.NewNopLogger(), nil, rs, dbDir, DefaultOptions())
	require.NoError(t, err)
	defer require.NoError(t, db.Close())

	require.Equal(t, uint64(seriesCount), db.nextRef.Load(), "nextRef should be equal to the number of series written across the entire WAL")
}

func startTime() (int64, error) {
	return time.Now().Unix() * 1000, nil
}

// Create series for tests.
func labelsForTest(lName string, seriesCount int) []labels.Labels {
	var series []labels.Labels

	for i := 0; i < seriesCount; i++ {
		lset := labels.Labels{
			{Name: "a", Value: lName},
			{Name: "job", Value: "prometheus"},
			{Name: "instance", Value: "localhost" + strconv.Itoa(i)},
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

	sRef, err := app.Append(0, labels.Labels{{Name: "a", Value: "1"}}, 0, 0)
	require.NoError(t, err, "should not reject valid series")

	// Write a few exemplars to our appender and call Commit().
	// If the Labels, Value or Timestamp are different than the last exemplar,
	// then a new one should be appended; Otherwise, it should be skipped.
	e := exemplar.Exemplar{Labels: labels.Labels{{Name: "a", Value: "1"}}, Value: 20, Ts: 10, HasTs: true}
	_, _ = app.AppendExemplar(sRef, nil, e)
	_, _ = app.AppendExemplar(sRef, nil, e)

	e.Labels = labels.Labels{{Name: "b", Value: "2"}}
	_, _ = app.AppendExemplar(sRef, nil, e)
	_, _ = app.AppendExemplar(sRef, nil, e)
	_, _ = app.AppendExemplar(sRef, nil, e)

	e.Value = 42
	_, _ = app.AppendExemplar(sRef, nil, e)
	_, _ = app.AppendExemplar(sRef, nil, e)

	e.Ts = 25
	_, _ = app.AppendExemplar(sRef, nil, e)
	_, _ = app.AppendExemplar(sRef, nil, e)

	require.NoError(t, app.Commit())

	// Read back what was written to the WAL.
	var walExemplarsCount int
	sr, err := wal.NewSegmentsReader(s.wal.Dir())
	require.NoError(t, err)
	defer sr.Close()
	r := wal.NewReader(sr)

	var dec record.Decoder
	for r.Next() {
		rec := r.Record()
		switch dec.Type(rec) {
		case record.Exemplars:
			var exemplars []record.RefExemplar
			exemplars, err = dec.Exemplars(rec, exemplars)
			require.NoError(t, err)
			walExemplarsCount += len(exemplars)
		}
	}

	// We had 9 calls to AppendExemplar but only 4 of those should have gotten through.
	require.Equal(t, 4, walExemplarsCount)
}
