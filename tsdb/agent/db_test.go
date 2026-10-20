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
	"context"
	"io"
	"math"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
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

	rs := remote.NewStorage(promslog.NewNopLogger(), reg, startTime, dbDir, time.Second*30, nil, false)
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
	dbDir := t.TempDir()
	opts := DefaultOptions()
	rs1 := remote.NewStorage(promslog.NewNopLogger(), nil, startTime, dbDir, time.Second*30, nil, false)
	db, err := Open(promslog.NewNopLogger(), nil, rs1, dbDir, opts)
	require.NoError(t, err)

	app := db.AppenderV2(context.Background())

	metricNames := []string{"foo", "bar", "baz", "blerg"}
	originalSeriesRefs := make([]chunks.HeadSeriesRef, 0, len(metricNames))
	for _, metricName := range metricNames {
		lbls := labels.FromMap(map[string]string{"__name__": metricName})

		ref, err := app.Append(storage.SeriesRef(0), lbls, 0, int64(0), 10.0, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		originalSeriesRefs = append(originalSeriesRefs, chunks.HeadSeriesRef(ref))
		ref2, err := app.Append(ref, lbls, 0, int64(10), 100.0, nil, nil, storage.AOptions{})
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
	for _, metricName := range metricNames {
		lbls := labels.FromMap(map[string]string{"__name__": metricName})
		ref, err := app.Append(storage.SeriesRef(0), lbls, 0, int64(20), 10.0, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		duplicateSeriesRefs = append(duplicateSeriesRefs, chunks.HeadSeriesRef(ref))
	}
	require.NoError(t, app.Commit())

	// The duplicate SeriesRefs should be in series.
	for _, ref := range duplicateSeriesRefs {
		require.NotNil(t, db.series.GetByID(ref))
	}

	// Close the WAL before we have a chance to remove the original RefIDs.
	// Both db and rs1 must be closed to release all file handles before
	// reopening the same directory — important on Windows.
	require.NoError(t, db.Close())
	require.NoError(t, rs1.Close())

	rs2 := remote.NewStorage(promslog.NewNopLogger(), nil, startTime, dbDir, time.Second*30, nil, false)
	t.Cleanup(func() { require.NoError(t, rs2.Close()) })
	db, err = Open(promslog.NewNopLogger(), nil, rs2, dbDir, opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	// The original SeriesRefs should be in series.
	for _, ref := range originalSeriesRefs {
		require.NotNil(t, db.series.GetByID(ref))
		require.NotContains(t, db.deleted, ref)
	}

	// The duplicated SeriesRefs should be considered deleted.
	for _, ref := range duplicateSeriesRefs {
		require.Nil(t, db.series.GetByID(ref))
		require.Contains(t, db.deleted, ref)
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
