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
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wal"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestUnsupported(t *testing.T) {
	promAgentDir := t.TempDir()

	opts := DefaultOptions()
	logger := log.NewNopLogger()

	s, err := Open(logger, prometheus.NewRegistry(), nil, promAgentDir, opts)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, s.Close())
	}()

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

	promAgentDir := t.TempDir()

	lbls := labelsForTest(t.Name(), numSeries)
	opts := DefaultOptions()
	logger := log.NewNopLogger()
	reg := prometheus.NewRegistry()
	remoteStorage := remote.NewStorage(log.With(logger, "component", "remote"), reg, startTime, promAgentDir, time.Second*30, nil)
	defer func(rs *remote.Storage) {
		require.NoError(t, rs.Close())
	}(remoteStorage)

	s, err := Open(logger, reg, remoteStorage, promAgentDir, opts)
	require.NoError(t, err)

	a := s.Appender(context.TODO())

	for _, l := range lbls {
		lset := labels.New(l...)

		for i := 0; i < numDatapoints; i++ {
			sample := tsdbutil.GenerateSamples(0, 1)
			_, err := a.Append(0, lset, sample[0].T(), sample[0].V())
			require.NoError(t, err)
		}
	}

	require.NoError(t, a.Commit())
	require.NoError(t, s.Close())

	// Read records from WAL and check for expected count of series and samples.
	walSeriesCount := 0
	walSamplesCount := 0

	reg = prometheus.NewRegistry()
	remoteStorage = remote.NewStorage(log.With(logger, "component", "remote"), reg, startTime, promAgentDir, time.Second*30, nil)
	defer func() {
		require.NoError(t, remoteStorage.Close())
	}()

	s1, err := Open(logger, nil, remoteStorage, promAgentDir, opts)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, s1.Close())
	}()

	var dec record.Decoder

	if err == nil {
		sr, err := wal.NewSegmentsReader(s1.wal.Dir())
		require.NoError(t, err)
		defer func() {
			require.NoError(t, sr.Close())
		}()

		r := wal.NewReader(sr)
		seriesPool := sync.Pool{
			New: func() interface{} {
				return []record.RefSeries{}
			},
		}
		samplesPool := sync.Pool{
			New: func() interface{} {
				return []record.RefSample{}
			},
		}

		for r.Next() {
			rec := r.Record()
			switch dec.Type(rec) {
			case record.Series:
				series := seriesPool.Get().([]record.RefSeries)[:0]
				series, _ = dec.Series(rec, series)
				walSeriesCount += len(series)
			case record.Samples:
				samples := samplesPool.Get().([]record.RefSample)[:0]
				samples, _ = dec.Samples(rec, samples)
				walSamplesCount += len(samples)
			default:
			}
		}
	}

	// Retrieved series count from WAL should match the count of series been added to the WAL.
	require.Equal(t, walSeriesCount, numSeries)

	// Retrieved samples count from WAL should match the count of samples been added to the WAL.
	require.Equal(t, walSamplesCount, numSeries*numDatapoints)
}

func TestRollback(t *testing.T) {
	const (
		numDatapoints = 1000
		numSeries     = 8
	)

	promAgentDir := t.TempDir()

	lbls := labelsForTest(t.Name(), numSeries)
	opts := DefaultOptions()
	logger := log.NewNopLogger()
	reg := prometheus.NewRegistry()
	remoteStorage := remote.NewStorage(log.With(logger, "component", "remote"), reg, startTime, promAgentDir, time.Second*30, nil)
	defer func(rs *remote.Storage) {
		require.NoError(t, rs.Close())
	}(remoteStorage)

	s, err := Open(logger, reg, remoteStorage, promAgentDir, opts)
	require.NoError(t, err)

	a := s.Appender(context.TODO())

	for _, l := range lbls {
		lset := labels.New(l...)

		for i := 0; i < numDatapoints; i++ {
			sample := tsdbutil.GenerateSamples(0, 1)
			_, err := a.Append(0, lset, sample[0].T(), sample[0].V())
			require.NoError(t, err)
		}
	}

	require.NoError(t, a.Rollback())
	require.NoError(t, s.Close())

	// Read records from WAL and check for expected count of series and samples.
	walSeriesCount := 0
	walSamplesCount := 0

	reg = prometheus.NewRegistry()
	remoteStorage = remote.NewStorage(log.With(logger, "component", "remote"), reg, startTime, promAgentDir, time.Second*30, nil)
	defer func() {
		require.NoError(t, remoteStorage.Close())
	}()

	s1, err := Open(logger, nil, remoteStorage, promAgentDir, opts)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, s1.Close())
	}()

	var dec record.Decoder

	if err == nil {
		sr, err := wal.NewSegmentsReader(s1.wal.Dir())
		require.NoError(t, err)
		defer func() {
			require.NoError(t, sr.Close())
		}()

		r := wal.NewReader(sr)
		seriesPool := sync.Pool{
			New: func() interface{} {
				return []record.RefSeries{}
			},
		}
		samplesPool := sync.Pool{
			New: func() interface{} {
				return []record.RefSample{}
			},
		}

		for r.Next() {
			rec := r.Record()
			switch dec.Type(rec) {
			case record.Series:
				series := seriesPool.Get().([]record.RefSeries)[:0]
				series, _ = dec.Series(rec, series)
				walSeriesCount += len(series)
			case record.Samples:
				samples := samplesPool.Get().([]record.RefSample)[:0]
				samples, _ = dec.Samples(rec, samples)
				walSamplesCount += len(samples)
			default:
			}
		}
	}

	// Retrieved series count from WAL should be zero.
	require.Equal(t, walSeriesCount, 0)

	// Retrieved samples count from WAL should be zero.
	require.Equal(t, walSamplesCount, 0)
}

func TestFullTruncateWAL(t *testing.T) {
	const (
		numDatapoints = 1000
		numSeries     = 800
		lastTs        = 500
	)

	promAgentDir := t.TempDir()

	lbls := labelsForTest(t.Name(), numSeries)
	opts := DefaultOptions()
	opts.TruncateFrequency = time.Minute * 2
	logger := log.NewNopLogger()
	reg := prometheus.NewRegistry()
	remoteStorage := remote.NewStorage(log.With(logger, "component", "remote"), reg, startTime, promAgentDir, time.Second*30, nil)
	defer func() {
		require.NoError(t, remoteStorage.Close())
	}()

	s, err := Open(logger, reg, remoteStorage, promAgentDir, opts)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, s.Close())
	}()

	a := s.Appender(context.TODO())

	for _, l := range lbls {
		lset := labels.New(l...)

		for i := 0; i < numDatapoints; i++ {
			_, err := a.Append(0, lset, int64(lastTs), 0)
			require.NoError(t, err)
		}
		require.NoError(t, a.Commit())
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

	promAgentDir := t.TempDir()

	opts := DefaultOptions()
	opts.TruncateFrequency = time.Minute * 2
	logger := log.NewNopLogger()
	reg := prometheus.NewRegistry()
	remoteStorage := remote.NewStorage(log.With(logger, "component", "remote"), reg, startTime, promAgentDir, time.Second*30, nil)
	defer func() {
		require.NoError(t, remoteStorage.Close())
	}()

	s, err := Open(logger, reg, remoteStorage, promAgentDir, opts)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, s.Close())
	}()

	a := s.Appender(context.TODO())

	var lastTs int64

	// Create first batch of 800 series with 1000 data-points with a fixed lastTs as 500.
	lastTs = 500
	lbls := labelsForTest(t.Name()+"batch-1", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		for i := 0; i < numDatapoints; i++ {
			_, err := a.Append(0, lset, lastTs, 0)
			require.NoError(t, err)
		}
		require.NoError(t, a.Commit())
	}

	// Create second batch of 800 series with 1000 data-points with a fixed lastTs as 600.
	lastTs = 600

	lbls = labelsForTest(t.Name()+"batch-2", numSeries)
	for _, l := range lbls {
		lset := labels.New(l...)

		for i := 0; i < numDatapoints; i++ {
			_, err := a.Append(0, lset, lastTs, 0)
			require.NoError(t, err)
		}
		require.NoError(t, a.Commit())
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

	promAgentDir := t.TempDir()

	lbls := labelsForTest(t.Name(), numSeries)
	opts := DefaultOptions()

	logger := log.NewNopLogger()
	reg := prometheus.NewRegistry()
	remoteStorage := remote.NewStorage(log.With(logger, "component", "remote"), reg, startTime, promAgentDir, time.Second*30, nil)
	defer func() {
		require.NoError(t, remoteStorage.Close())
	}()

	s, err := Open(logger, reg, remoteStorage, promAgentDir, opts)
	require.NoError(t, err)

	a := s.Appender(context.TODO())

	for _, l := range lbls {
		lset := labels.New(l...)

		for i := 0; i < numDatapoints; i++ {
			_, err := a.Append(0, lset, lastTs, 0)
			require.NoError(t, err)
		}
	}

	require.NoError(t, a.Commit())
	require.NoError(t, s.Close())

	restartOpts := DefaultOptions()
	restartLogger := log.NewNopLogger()
	restartReg := prometheus.NewRegistry()

	// Open a new DB with the same WAL to check that series from the previous DB
	// get replayed.
	replayDB, err := Open(restartLogger, restartReg, nil, promAgentDir, restartOpts)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, replayDB.Close())
	}()

	// Check if all the series are retrieved back from the WAL.
	m := gatherFamily(t, restartReg, "prometheus_agent_active_series")
	require.Equal(t, float64(numSeries), m.Metric[0].Gauge.GetValue(), "agent wal replay mismatch of active series count")

	// Check if lastTs of the samples retrieved from the WAL is retained.
	metrics := replayDB.series.series
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
