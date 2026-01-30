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

package tsdb

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/compression"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	var isolationEnabled bool
	flag.BoolVar(&isolationEnabled, "test.tsdb-isolation", true, "enable isolation")
	flag.Parse()
	defaultIsolationDisabled = !isolationEnabled

	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("github.com/prometheus/prometheus/tsdb.(*SegmentWAL).cut.func1"),
		goleak.IgnoreTopFunction("github.com/prometheus/prometheus/tsdb.(*SegmentWAL).cut.func2"),
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"))
}

type testDBOptions struct {
	dir  string
	opts *Options
	rngs []int64
}
type testDBOpt func(o *testDBOptions)

func withDir(dir string) testDBOpt {
	return func(o *testDBOptions) {
		o.dir = dir
	}
}

func withOpts(opts *Options) testDBOpt {
	return func(o *testDBOptions) {
		o.opts = opts
	}
}

func withRngs(rngs ...int64) testDBOpt {
	return func(o *testDBOptions) {
		o.rngs = rngs
	}
}

func newTestDB(t testing.TB, opts ...testDBOpt) (db *DB) {
	var o testDBOptions
	for _, opt := range opts {
		opt(&o)
	}
	if o.opts == nil {
		o.opts = DefaultOptions()
	}
	if o.dir == "" {
		o.dir = t.TempDir()
	}

	var err error
	if len(o.rngs) == 0 {
		db, err = Open(o.dir, nil, nil, o.opts, nil)
	} else {
		o.opts, o.rngs = validateOpts(o.opts, o.rngs)
		db, err = open(o.dir, nil, nil, o.opts, o.rngs, nil)
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		// Always close. DB is safe for close-after-close.
		require.NoError(t, db.Close())
	})
	return db
}

func TestDBClose_AfterClose(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())
	require.NoError(t, db.Close())

	// Double check if we are closing correct DB after reuse.
	db = newTestDB(t)
	require.NoError(t, db.Close())
	require.NoError(t, db.Close())
}

// query runs a matcher query against the querier and fully expands its data.
func query(t testing.TB, q storage.Querier, matchers ...*labels.Matcher) map[string][]chunks.Sample {
	return queryHelper(t, q, true, matchers...)
}

// queryWithoutReplacingNaNs runs a matcher query against the querier and fully expands its data.
func queryWithoutReplacingNaNs(t testing.TB, q storage.Querier, matchers ...*labels.Matcher) map[string][]chunks.Sample {
	return queryHelper(t, q, false, matchers...)
}

// queryHelper runs a matcher query against the querier and fully expands its data.
func queryHelper(t testing.TB, q storage.Querier, withNaNReplacement bool, matchers ...*labels.Matcher) map[string][]chunks.Sample {
	ss := q.Select(context.Background(), false, nil, matchers...)
	defer func() {
		require.NoError(t, q.Close())
	}()

	var it chunkenc.Iterator
	result := map[string][]chunks.Sample{}
	for ss.Next() {
		series := ss.At()

		it = series.Iterator(it)
		var samples []chunks.Sample
		var err error
		if withNaNReplacement {
			samples, err = storage.ExpandSamples(it, newSample)
		} else {
			samples, err = storage.ExpandSamplesWithoutReplacingNaNs(it, newSample)
		}
		require.NoError(t, err)
		require.NoError(t, it.Err())

		if len(samples) == 0 {
			continue
		}

		name := series.Labels().String()
		result[name] = samples
	}
	require.NoError(t, ss.Err())
	require.Empty(t, ss.Warnings())

	return result
}

// queryAndExpandChunks runs a matcher query against the querier and fully expands its data into samples.
func queryAndExpandChunks(t testing.TB, q storage.ChunkQuerier, matchers ...*labels.Matcher) map[string][][]chunks.Sample {
	s := queryChunks(t, q, matchers...)

	res := make(map[string][][]chunks.Sample)
	for k, v := range s {
		var samples [][]chunks.Sample
		for _, chk := range v {
			sam, err := storage.ExpandSamples(chk.Chunk.Iterator(nil), nil)
			require.NoError(t, err)
			samples = append(samples, sam)
		}
		res[k] = samples
	}

	return res
}

// queryChunks runs a matcher query against the querier and expands its data.
func queryChunks(t testing.TB, q storage.ChunkQuerier, matchers ...*labels.Matcher) map[string][]chunks.Meta {
	ss := q.Select(context.Background(), false, nil, matchers...)
	defer func() {
		require.NoError(t, q.Close())
	}()

	var it chunks.Iterator
	result := map[string][]chunks.Meta{}
	for ss.Next() {
		series := ss.At()

		chks := []chunks.Meta{}
		it = series.Iterator(it)
		for it.Next() {
			chks = append(chks, it.At())
		}
		require.NoError(t, it.Err())

		if len(chks) == 0 {
			continue
		}

		name := series.Labels().String()
		result[name] = chks
	}
	require.NoError(t, ss.Err())
	require.Empty(t, ss.Warnings())
	return result
}

// Ensure that blocks are held in memory in their time order
// and not in ULID order as they are read from the directory.
func TestDB_reloadOrder(t *testing.T) {
	db := newTestDB(t)

	metas := []BlockMeta{
		{MinTime: 90, MaxTime: 100},
		{MinTime: 70, MaxTime: 80},
		{MinTime: 100, MaxTime: 110},
	}
	for _, m := range metas {
		createBlock(t, db.Dir(), genSeries(1, 1, m.MinTime, m.MaxTime))
	}

	require.NoError(t, db.reloadBlocks())
	blocks := db.Blocks()
	require.Len(t, blocks, 3)
	require.Equal(t, metas[1].MinTime, blocks[0].Meta().MinTime)
	require.Equal(t, metas[1].MaxTime, blocks[0].Meta().MaxTime)
	require.Equal(t, metas[0].MinTime, blocks[1].Meta().MinTime)
	require.Equal(t, metas[0].MaxTime, blocks[1].Meta().MaxTime)
	require.Equal(t, metas[2].MinTime, blocks[2].Meta().MinTime)
	require.Equal(t, metas[2].MaxTime, blocks[2].Meta().MaxTime)
}

func TestDataAvailableOnlyAfterCommit(t *testing.T) {
	db := newTestDB(t)

	ctx := context.Background()
	app := db.Appender(ctx)

	_, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)

	querier, err := db.Querier(0, 1)
	require.NoError(t, err)
	seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	require.Equal(t, map[string][]chunks.Sample{}, seriesSet)

	err = app.Commit()
	require.NoError(t, err)

	querier, err = db.Querier(0, 1)
	require.NoError(t, err)
	defer querier.Close()

	seriesSet = query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))

	require.Equal(t, map[string][]chunks.Sample{`{foo="bar"}`: {sample{t: 0, f: 0}}}, seriesSet)
}

// TestNoPanicAfterWALCorruption ensures that querying the db after a WAL corruption doesn't cause a panic.
// https://github.com/prometheus/prometheus/issues/7548
func TestNoPanicAfterWALCorruption(t *testing.T) {
	db := newTestDB(t, withOpts(&Options{WALSegmentSize: 32 * 1024}))

	// Append until the first mmapped head chunk.
	// This is to ensure that all samples can be read from the mmapped chunks when the WAL is corrupted.
	var expSamples []chunks.Sample
	var maxt int64
	ctx := context.Background()
	{
		// Appending 121 samples because on the 121st a new chunk will be created.
		for range 121 {
			app := db.Appender(ctx)
			_, err := app.Append(0, labels.FromStrings("foo", "bar"), maxt, 0)
			expSamples = append(expSamples, sample{t: maxt, f: 0})
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			maxt++
		}
		require.NoError(t, db.Close())
	}

	// Corrupt the WAL after the first sample of the series so that it has at least one sample and
	// it is not garbage collected.
	// The repair deletes all WAL records after the corrupted record and these are read from the mmapped chunk.
	{
		walFiles, err := os.ReadDir(path.Join(db.Dir(), "wal"))
		require.NoError(t, err)
		f, err := os.OpenFile(path.Join(db.Dir(), "wal", walFiles[0].Name()), os.O_RDWR, 0o666)
		require.NoError(t, err)
		r := wlog.NewReader(bufio.NewReader(f))
		require.True(t, r.Next(), "reading the series record")
		require.True(t, r.Next(), "reading the first sample record")
		// Write an invalid record header to corrupt everything after the first wal sample.
		_, err = f.WriteAt([]byte{99}, r.Offset())
		require.NoError(t, err)
		f.Close()
	}

	// Query the data.
	{
		db := newTestDB(t, withDir(db.Dir()))
		require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal), "WAL corruption count mismatch")

		querier, err := db.Querier(0, maxt)
		require.NoError(t, err)
		seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "", ""))
		// The last sample should be missing as it was after the WAL segment corruption.
		require.Equal(t, map[string][]chunks.Sample{`{foo="bar"}`: expSamples[0 : len(expSamples)-1]}, seriesSet)
	}
}

func TestDataNotAvailableAfterRollback(t *testing.T) {
	db := newTestDB(t)

	app := db.Appender(context.Background())
	_, err := app.Append(0, labels.FromStrings("type", "float"), 0, 0)
	require.NoError(t, err)

	_, err = app.AppendHistogram(
		0, labels.FromStrings("type", "histogram"), 0,
		&histogram.Histogram{Count: 42, Sum: math.NaN()}, nil,
	)
	require.NoError(t, err)

	_, err = app.AppendHistogram(
		0, labels.FromStrings("type", "floathistogram"), 0,
		nil, &histogram.FloatHistogram{Count: 42, Sum: math.NaN()},
	)
	require.NoError(t, err)

	err = app.Rollback()
	require.NoError(t, err)

	for _, typ := range []string{"float", "histogram", "floathistogram"} {
		querier, err := db.Querier(0, 1)
		require.NoError(t, err)
		seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "type", typ))
		require.Equal(t, map[string][]chunks.Sample{}, seriesSet)
	}

	sr, err := wlog.NewSegmentsReader(db.head.wal.Dir())
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

		case record.HistogramSamples, record.CustomBucketsHistogramSamples:
			var histograms []record.RefHistogramSample
			histograms, err = dec.HistogramSamples(rec, histograms)
			require.NoError(t, err)
			walHistogramCount += len(histograms)

		case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples:
			var floatHistograms []record.RefFloatHistogramSample
			floatHistograms, err = dec.FloatHistogramSamples(rec, floatHistograms)
			require.NoError(t, err)
			walFloatHistogramCount += len(floatHistograms)

		default:
		}
	}

	// Check that only series get stored after calling Rollback.
	require.Equal(t, 3, walSeriesCount, "series should have been written to WAL")
	require.Equal(t, 0, walSamplesCount, "samples should not have been written to WAL")
	require.Equal(t, 0, walExemplarsCount, "exemplars should not have been written to WAL")
	require.Equal(t, 0, walHistogramCount, "histograms should not have been written to WAL")
	require.Equal(t, 0, walFloatHistogramCount, "float histograms should not have been written to WAL")
}

func TestDBAppenderAddRef(t *testing.T) {
	db := newTestDB(t)

	ctx := context.Background()
	app1 := db.Appender(ctx)

	ref1, err := app1.Append(0, labels.FromStrings("a", "b"), 123, 0)
	require.NoError(t, err)

	// Reference should already work before commit.
	ref2, err := app1.Append(ref1, labels.EmptyLabels(), 124, 1)
	require.NoError(t, err)
	require.Equal(t, ref1, ref2)

	err = app1.Commit()
	require.NoError(t, err)

	app2 := db.Appender(ctx)

	// first ref should already work in next transaction.
	ref3, err := app2.Append(ref1, labels.EmptyLabels(), 125, 0)
	require.NoError(t, err)
	require.Equal(t, ref1, ref3)

	ref4, err := app2.Append(ref1, labels.FromStrings("a", "b"), 133, 1)
	require.NoError(t, err)
	require.Equal(t, ref1, ref4)

	// Reference must be valid to add another sample.
	ref5, err := app2.Append(ref2, labels.EmptyLabels(), 143, 2)
	require.NoError(t, err)
	require.Equal(t, ref1, ref5)

	// Missing labels & invalid refs should fail.
	_, err = app2.Append(9999999, labels.EmptyLabels(), 1, 1)
	require.ErrorIs(t, err, ErrInvalidSample)

	require.NoError(t, app2.Commit())

	q, err := db.Querier(0, 200)
	require.NoError(t, err)

	res := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

	require.Equal(t, map[string][]chunks.Sample{
		labels.FromStrings("a", "b").String(): {
			sample{t: 123, f: 0},
			sample{t: 124, f: 1},
			sample{t: 125, f: 0},
			sample{t: 133, f: 1},
			sample{t: 143, f: 2},
		},
	}, res)
}

func TestAppendEmptyLabelsIgnored(t *testing.T) {
	db := newTestDB(t)

	ctx := context.Background()
	app1 := db.Appender(ctx)

	ref1, err := app1.Append(0, labels.FromStrings("a", "b"), 123, 0)
	require.NoError(t, err)

	// Add with empty label.
	ref2, err := app1.Append(0, labels.FromStrings("a", "b", "c", ""), 124, 0)
	require.NoError(t, err)

	// Should be the same series.
	require.Equal(t, ref1, ref2)

	err = app1.Commit()
	require.NoError(t, err)
}

func TestDeleteSimple(t *testing.T) {
	const numSamples int64 = 10

	cases := []struct {
		Intervals tombstones.Intervals
		remaint   []int64
	}{
		{
			Intervals: tombstones.Intervals{{Mint: 0, Maxt: 3}},
			remaint:   []int64{4, 5, 6, 7, 8, 9},
		},
		{
			Intervals: tombstones.Intervals{{Mint: 1, Maxt: 3}},
			remaint:   []int64{0, 4, 5, 6, 7, 8, 9},
		},
		{
			Intervals: tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 7}},
			remaint:   []int64{0, 8, 9},
		},
		{
			Intervals: tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 700}},
			remaint:   []int64{0},
		},
		{ // This case is to ensure that labels and symbols are deleted.
			Intervals: tombstones.Intervals{{Mint: 0, Maxt: 9}},
			remaint:   []int64{},
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			db := newTestDB(t)

			ctx := context.Background()
			app := db.Appender(ctx)

			smpls := make([]float64, numSamples)
			for i := range numSamples {
				smpls[i] = rand.Float64()
				app.Append(0, labels.FromStrings("a", "b"), i, smpls[i])
			}

			require.NoError(t, app.Commit())

			// TODO(gouthamve): Reset the tombstones somehow.
			// Delete the ranges.
			for _, r := range c.Intervals {
				require.NoError(t, db.Delete(ctx, r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
			}

			// Compare the result.
			q, err := db.Querier(0, numSamples)
			require.NoError(t, err)

			res := q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

			expSamples := make([]chunks.Sample, 0, len(c.remaint))
			for _, ts := range c.remaint {
				expSamples = append(expSamples, sample{0, ts, smpls[ts], nil, nil})
			}

			expss := newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.FromStrings("a", "b"), expSamples),
			})

			for {
				eok, rok := expss.Next(), res.Next()
				require.Equal(t, eok, rok)

				if !eok {
					require.Empty(t, res.Warnings())
					break
				}
				sexp := expss.At()
				sres := res.At()

				require.Equal(t, sexp.Labels(), sres.Labels())

				smplExp, errExp := storage.ExpandSamples(sexp.Iterator(nil), nil)
				smplRes, errRes := storage.ExpandSamples(sres.Iterator(nil), nil)

				require.Equal(t, errExp, errRes)
				require.Equal(t, smplExp, smplRes)
			}
		})
	}
}

func TestAmendHistogramDatapointCausesError(t *testing.T) {
	db := newTestDB(t)

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Append(0, labels.FromStrings("a", "b"), 0, 0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 0, 0)
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 0, 1)
	require.ErrorIs(t, err, storage.ErrDuplicateSampleForTimestamp)
	require.NoError(t, app.Rollback())

	h := histogram.Histogram{
		Schema:        3,
		Count:         52,
		Sum:           2.7,
		ZeroThreshold: 0.1,
		ZeroCount:     42,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 4},
			{Offset: 10, Length: 3},
		},
		PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
	}
	fh := h.ToFloat(nil)

	app = db.Appender(ctx)
	_, err = app.AppendHistogram(0, labels.FromStrings("a", "c"), 0, h.Copy(), nil)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = db.Appender(ctx)
	_, err = app.AppendHistogram(0, labels.FromStrings("a", "c"), 0, h.Copy(), nil)
	require.NoError(t, err)
	h.Schema = 2
	_, err = app.AppendHistogram(0, labels.FromStrings("a", "c"), 0, h.Copy(), nil)
	require.Equal(t, storage.ErrDuplicateSampleForTimestamp, err)
	require.NoError(t, app.Rollback())

	// Float histogram.
	app = db.Appender(ctx)
	_, err = app.AppendHistogram(0, labels.FromStrings("a", "d"), 0, nil, fh.Copy())
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = db.Appender(ctx)
	_, err = app.AppendHistogram(0, labels.FromStrings("a", "d"), 0, nil, fh.Copy())
	require.NoError(t, err)
	fh.Schema = 2
	_, err = app.AppendHistogram(0, labels.FromStrings("a", "d"), 0, nil, fh.Copy())
	require.Equal(t, storage.ErrDuplicateSampleForTimestamp, err)
	require.NoError(t, app.Rollback())
}

func TestDuplicateNaNDatapointNoAmendError(t *testing.T) {
	db := newTestDB(t)

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Append(0, labels.FromStrings("a", "b"), 0, math.NaN())
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 0, math.NaN())
	require.NoError(t, err)
}

func TestNonDuplicateNaNDatapointsCausesAmendError(t *testing.T) {
	db := newTestDB(t)

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Append(0, labels.FromStrings("a", "b"), 0, math.Float64frombits(0x7ff0000000000001))
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 0, math.Float64frombits(0x7ff0000000000002))
	require.ErrorIs(t, err, storage.ErrDuplicateSampleForTimestamp)
}

func TestEmptyLabelsetCausesError(t *testing.T) {
	db := newTestDB(t)

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Append(0, labels.Labels{}, 0, 0)
	require.Error(t, err)
	require.Equal(t, "empty labelset: invalid sample", err.Error())
}

func TestSkippingInvalidValuesInSameTxn(t *testing.T) {
	db := newTestDB(t)

	// Append AmendedValue.
	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Append(0, labels.FromStrings("a", "b"), 0, 1)
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 0, 2)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Make sure the right value is stored.
	q, err := db.Querier(0, 10)
	require.NoError(t, err)

	ssMap := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

	require.Equal(t, map[string][]chunks.Sample{
		labels.New(labels.Label{Name: "a", Value: "b"}).String(): {sample{0, 0, 1, nil, nil}},
	}, ssMap)

	// Append Out of Order Value.
	app = db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 10, 3)
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 7, 5)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	q, err = db.Querier(0, 10)
	require.NoError(t, err)

	ssMap = query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

	require.Equal(t, map[string][]chunks.Sample{
		labels.New(labels.Label{Name: "a", Value: "b"}).String(): {sample{0, 0, 1, nil, nil}, sample{0, 10, 3, nil, nil}},
	}, ssMap)
}

func TestDB_Snapshot(t *testing.T) {
	db := newTestDB(t)

	// append data
	ctx := context.Background()
	app := db.Appender(ctx)
	mint := int64(1414141414000)
	for i := range 1000 {
		_, err := app.Append(0, labels.FromStrings("foo", "bar"), mint+int64(i), 1.0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// create snapshot
	snap := t.TempDir()
	require.NoError(t, db.Snapshot(snap, true))
	require.NoError(t, db.Close())

	// reopen DB from snapshot
	db = newTestDB(t, withDir(snap))

	querier, err := db.Querier(mint, mint+1000)
	require.NoError(t, err)
	defer func() { require.NoError(t, querier.Close()) }()

	// sum values
	seriesSet := querier.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	var series chunkenc.Iterator
	sum := 0.0
	for seriesSet.Next() {
		series = seriesSet.At().Iterator(series)
		for series.Next() == chunkenc.ValFloat {
			_, v := series.At()
			sum += v
		}
		require.NoError(t, series.Err())
	}
	require.NoError(t, seriesSet.Err())
	require.Empty(t, seriesSet.Warnings())
	require.Equal(t, 1000.0, sum)
}

// TestDB_Snapshot_ChunksOutsideOfCompactedRange ensures that a snapshot removes chunks samples
// that are outside the set block time range.
// See https://github.com/prometheus/prometheus/issues/5105
func TestDB_Snapshot_ChunksOutsideOfCompactedRange(t *testing.T) {
	db := newTestDB(t)

	ctx := context.Background()
	app := db.Appender(ctx)
	mint := int64(1414141414000)
	for i := range 1000 {
		_, err := app.Append(0, labels.FromStrings("foo", "bar"), mint+int64(i), 1.0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	snap := t.TempDir()

	// Hackingly introduce "race", by having lower max time then maxTime in last chunk.
	db.head.maxTime.Sub(10)

	require.NoError(t, db.Snapshot(snap, true))
	require.NoError(t, db.Close())

	// reopen DB from snapshot
	db = newTestDB(t, withDir(snap))

	querier, err := db.Querier(mint, mint+1000)
	require.NoError(t, err)
	defer func() { require.NoError(t, querier.Close()) }()

	// Sum values.
	seriesSet := querier.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	var series chunkenc.Iterator
	sum := 0.0
	for seriesSet.Next() {
		series = seriesSet.At().Iterator(series)
		for series.Next() == chunkenc.ValFloat {
			_, v := series.At()
			sum += v
		}
		require.NoError(t, series.Err())
	}
	require.NoError(t, seriesSet.Err())
	require.Empty(t, seriesSet.Warnings())

	// Since we snapshotted with MaxTime - 10, so expect 10 less samples.
	require.Equal(t, 1000.0-10, sum)
}

func TestDB_SnapshotWithDelete(t *testing.T) {
	const numSamples int64 = 10

	db := newTestDB(t)

	ctx := context.Background()
	app := db.Appender(ctx)

	smpls := make([]float64, numSamples)
	for i := range numSamples {
		smpls[i] = rand.Float64()
		app.Append(0, labels.FromStrings("a", "b"), i, smpls[i])
	}

	require.NoError(t, app.Commit())
	cases := []struct {
		intervals tombstones.Intervals
		remaint   []int64
	}{
		{
			intervals: tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 7}},
			remaint:   []int64{0, 8, 9},
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			// TODO(gouthamve): Reset the tombstones somehow.
			// Delete the ranges.
			for _, r := range c.intervals {
				require.NoError(t, db.Delete(ctx, r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
			}

			// create snapshot
			snap := t.TempDir()

			require.NoError(t, db.Snapshot(snap, true))

			// reopen DB from snapshot
			db := newTestDB(t, withDir(snap))

			// Compare the result.
			q, err := db.Querier(0, numSamples)
			require.NoError(t, err)
			defer func() { require.NoError(t, q.Close()) }()

			res := q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

			expSamples := make([]chunks.Sample, 0, len(c.remaint))
			for _, ts := range c.remaint {
				expSamples = append(expSamples, sample{0, ts, smpls[ts], nil, nil})
			}

			expss := newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.FromStrings("a", "b"), expSamples),
			})

			if len(expSamples) == 0 {
				require.False(t, res.Next())
				return
			}

			for {
				eok, rok := expss.Next(), res.Next()
				require.Equal(t, eok, rok)

				if !eok {
					require.Empty(t, res.Warnings())
					break
				}
				sexp := expss.At()
				sres := res.At()

				require.Equal(t, sexp.Labels(), sres.Labels())

				smplExp, errExp := storage.ExpandSamples(sexp.Iterator(nil), nil)
				smplRes, errRes := storage.ExpandSamples(sres.Iterator(nil), nil)

				require.Equal(t, errExp, errRes)
				require.Equal(t, smplExp, smplRes)
			}
		})
	}
}

func TestDB_e2e(t *testing.T) {
	const (
		numDatapoints = 1000
		numRanges     = 1000
		timeInterval  = int64(3)
	)
	// Create 8 series with 1000 data-points of different ranges and run queries.
	lbls := [][]labels.Label{
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
	}

	seriesMap := map[string][]chunks.Sample{}
	for _, l := range lbls {
		seriesMap[labels.New(l...).String()] = []chunks.Sample{}
	}

	db := newTestDB(t)

	ctx := context.Background()
	app := db.Appender(ctx)

	for _, l := range lbls {
		lset := labels.New(l...)
		series := []chunks.Sample{}

		ts := rand.Int63n(300)
		for range numDatapoints {
			v := rand.Float64()

			series = append(series, sample{0, ts, v, nil, nil})

			_, err := app.Append(0, lset, ts, v)
			require.NoError(t, err)

			ts += rand.Int63n(timeInterval) + 1
		}

		seriesMap[lset.String()] = series
	}

	require.NoError(t, app.Commit())

	// Query each selector on 1000 random time-ranges.
	queries := []struct {
		ms []*labels.Matcher
	}{
		{
			ms: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "b")},
		},
		{
			ms: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "b"),
				labels.MustNewMatcher(labels.MatchEqual, "job", "prom-k8s"),
			},
		},
		{
			ms: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "c"),
				labels.MustNewMatcher(labels.MatchEqual, "instance", "localhost:9090"),
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
			},
		},
		// TODO: Add Regexp Matchers.
	}

	for _, qry := range queries {
		matched := labels.Slice{}
		for _, l := range lbls {
			s := labels.Selector(qry.ms)
			ls := labels.New(l...)
			if s.Matches(ls) {
				matched = append(matched, ls)
			}
		}

		sort.Sort(matched)

		for range numRanges {
			mint := rand.Int63n(300)
			maxt := mint + rand.Int63n(timeInterval*int64(numDatapoints))

			expected := map[string][]chunks.Sample{}

			// Build the mockSeriesSet.
			for _, m := range matched {
				smpls := boundedSamples(seriesMap[m.String()], mint, maxt)
				if len(smpls) > 0 {
					expected[m.String()] = smpls
				}
			}

			q, err := db.Querier(mint, maxt)
			require.NoError(t, err)

			ss := q.Select(ctx, false, nil, qry.ms...)
			result := map[string][]chunks.Sample{}

			for ss.Next() {
				x := ss.At()

				smpls, err := storage.ExpandSamples(x.Iterator(nil), newSample)
				require.NoError(t, err)

				if len(smpls) > 0 {
					result[x.Labels().String()] = smpls
				}
			}

			require.NoError(t, ss.Err())
			require.Empty(t, ss.Warnings())
			require.Equal(t, expected, result)

			q.Close()
		}
	}
}

func TestWALFlushedOnDBClose(t *testing.T) {
	db := newTestDB(t)

	lbls := labels.FromStrings("labelname", "labelvalue")

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Append(0, lbls, 0, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.NoError(t, db.Close())

	db = newTestDB(t, withDir(db.Dir()))

	q, err := db.Querier(0, 1)
	require.NoError(t, err)

	values, ws, err := q.LabelValues(ctx, "labelname", nil)
	require.NoError(t, err)
	require.Empty(t, ws)
	require.Equal(t, []string{"labelvalue"}, values)
}

func TestWALSegmentSizeOptions(t *testing.T) {
	tests := map[int]func(dbdir string, segmentSize int){
		// Default Wal Size.
		0: func(dbDir string, _ int) {
			filesAndDir, err := os.ReadDir(filepath.Join(dbDir, "wal"))
			require.NoError(t, err)
			files := []os.FileInfo{}
			for _, f := range filesAndDir {
				if !f.IsDir() {
					fi, err := f.Info()
					require.NoError(t, err)
					files = append(files, fi)
				}
			}
			// All the full segment files (all but the last) should match the segment size option.
			for _, f := range files[:len(files)-1] {
				require.Equal(t, int64(DefaultOptions().WALSegmentSize), f.Size(), "WAL file size doesn't match WALSegmentSize option, filename: %v", f.Name())
			}
			lastFile := files[len(files)-1]
			require.Greater(t, int64(DefaultOptions().WALSegmentSize), lastFile.Size(), "last WAL file size is not smaller than the WALSegmentSize option, filename: %v", lastFile.Name())
		},
		// Custom Wal Size.
		2 * 32 * 1024: func(dbDir string, segmentSize int) {
			filesAndDir, err := os.ReadDir(filepath.Join(dbDir, "wal"))
			require.NoError(t, err)
			files := []os.FileInfo{}
			for _, f := range filesAndDir {
				if !f.IsDir() {
					fi, err := f.Info()
					require.NoError(t, err)
					files = append(files, fi)
				}
			}
			require.NotEmpty(t, files, "current WALSegmentSize should result in more than a single WAL file.")
			// All the full segment files (all but the last) should match the segment size option.
			for _, f := range files[:len(files)-1] {
				require.Equal(t, int64(segmentSize), f.Size(), "WAL file size doesn't match WALSegmentSize option, filename: %v", f.Name())
			}
			lastFile := files[len(files)-1]
			require.Greater(t, int64(segmentSize), lastFile.Size(), "last WAL file size is not smaller than the WALSegmentSize option, filename: %v", lastFile.Name())
		},
		// Wal disabled.
		-1: func(dbDir string, _ int) {
			// Check that WAL dir is not there.
			_, err := os.Stat(filepath.Join(dbDir, "wal"))
			require.Error(t, err)
			// Check that there is chunks dir.
			_, err = os.Stat(mmappedChunksDir(dbDir))
			require.NoError(t, err)
		},
	}
	for segmentSize, testFunc := range tests {
		t.Run(fmt.Sprintf("WALSegmentSize %d test", segmentSize), func(t *testing.T) {
			opts := DefaultOptions()
			opts.WALSegmentSize = segmentSize
			db := newTestDB(t, withOpts(opts))

			for i := range int64(155) {
				app := db.Appender(context.Background())
				ref, err := app.Append(0, labels.FromStrings("wal"+strconv.Itoa(int(i)), "size"), i, rand.Float64())
				require.NoError(t, err)
				for j := int64(1); j <= 78; j++ {
					_, err := app.Append(ref, labels.EmptyLabels(), i+j, rand.Float64())
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())
			}

			require.NoError(t, db.Close())
			testFunc(db.Dir(), opts.WALSegmentSize)
		})
	}
}

// https://github.com/prometheus/prometheus/issues/9846
// https://github.com/prometheus/prometheus/issues/9859
func TestWALReplayRaceOnSamplesLoggedBeforeSeries(t *testing.T) {
	const (
		numRuns                        = 1
		numSamplesBeforeSeriesCreation = 1000
	)

	// We test both with few and many samples appended after series creation. If samples are < 120 then there's no
	// mmap-ed chunk, otherwise there's at least 1 mmap-ed chunk when replaying the WAL.
	for _, numSamplesAfterSeriesCreation := range []int{1, 1000} {
		for run := 1; run <= numRuns; run++ {
			t.Run(fmt.Sprintf("samples after series creation = %d, run = %d", numSamplesAfterSeriesCreation, run), func(t *testing.T) {
				testWALReplayRaceOnSamplesLoggedBeforeSeries(t, numSamplesBeforeSeriesCreation, numSamplesAfterSeriesCreation)
			})
		}
	}
}

func testWALReplayRaceOnSamplesLoggedBeforeSeries(t *testing.T, numSamplesBeforeSeriesCreation, numSamplesAfterSeriesCreation int) {
	const numSeries = 1000

	db := newTestDB(t)
	db.DisableCompactions()

	for seriesRef := 1; seriesRef <= numSeries; seriesRef++ {
		// Log samples before the series is logged to the WAL.
		var enc record.Encoder
		var samples []record.RefSample

		for ts := range numSamplesBeforeSeriesCreation {
			samples = append(samples, record.RefSample{
				Ref: chunks.HeadSeriesRef(uint64(seriesRef)),
				T:   int64(ts),
				V:   float64(ts),
			})
		}

		err := db.Head().wal.Log(enc.Samples(samples, nil))
		require.NoError(t, err)

		// Add samples via appender so that they're logged after the series in the WAL.
		app := db.Appender(context.Background())
		lbls := labels.FromStrings("series_id", strconv.Itoa(seriesRef))

		for ts := numSamplesBeforeSeriesCreation; ts < numSamplesBeforeSeriesCreation+numSamplesAfterSeriesCreation; ts++ {
			_, err := app.Append(0, lbls, int64(ts), float64(ts))
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	require.NoError(t, db.Close())

	// Reopen the DB, replaying the WAL.
	db = newTestDB(t, withDir(db.Dir()))

	// Query back chunks for all series.
	q, err := db.ChunkQuerier(math.MinInt64, math.MaxInt64)
	require.NoError(t, err)

	set := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchRegexp, "series_id", ".+"))
	actualSeries := 0
	var chunksIt chunks.Iterator

	for set.Next() {
		actualSeries++
		actualChunks := 0

		chunksIt = set.At().Iterator(chunksIt)
		for chunksIt.Next() {
			actualChunks++
		}
		require.NoError(t, chunksIt.Err())

		// We expect 1 chunk every 120 samples after series creation.
		require.Equalf(t, (numSamplesAfterSeriesCreation/120)+1, actualChunks, "series: %s", set.At().Labels().String())
	}

	require.NoError(t, set.Err())
	require.Equal(t, numSeries, actualSeries)
}

func TestTombstoneClean(t *testing.T) {
	t.Parallel()
	const numSamples int64 = 10

	db := newTestDB(t)

	ctx := context.Background()
	app := db.Appender(ctx)

	smpls := make([]float64, numSamples)
	for i := range numSamples {
		smpls[i] = rand.Float64()
		app.Append(0, labels.FromStrings("a", "b"), i, smpls[i])
	}

	require.NoError(t, app.Commit())
	cases := []struct {
		intervals tombstones.Intervals
		remaint   []int64
	}{
		{
			intervals: tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 7}},
			remaint:   []int64{0, 8, 9},
		},
	}

	for _, c := range cases {
		// Delete the ranges.

		// Create snapshot.
		snap := t.TempDir()
		require.NoError(t, db.Snapshot(snap, true))
		require.NoError(t, db.Close())

		// Reopen DB from snapshot.
		db := newTestDB(t, withDir(snap))

		for _, r := range c.intervals {
			require.NoError(t, db.Delete(ctx, r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
		}

		// All of the setup for THIS line.
		require.NoError(t, db.CleanTombstones())

		// Compare the result.
		q, err := db.Querier(0, numSamples)
		require.NoError(t, err)
		defer q.Close()

		res := q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

		expSamples := make([]chunks.Sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{0, ts, smpls[ts], nil, nil})
		}

		expss := newMockSeriesSet([]storage.Series{
			storage.NewListSeries(labels.FromStrings("a", "b"), expSamples),
		})

		if len(expSamples) == 0 {
			require.False(t, res.Next())
			continue
		}

		for {
			eok, rok := expss.Next(), res.Next()
			require.Equal(t, eok, rok)

			if !eok {
				break
			}
			sexp := expss.At()
			sres := res.At()

			require.Equal(t, sexp.Labels(), sres.Labels())

			smplExp, errExp := storage.ExpandSamples(sexp.Iterator(nil), nil)
			smplRes, errRes := storage.ExpandSamples(sres.Iterator(nil), nil)

			require.Equal(t, errExp, errRes)
			require.Equal(t, smplExp, smplRes)
		}
		require.Empty(t, res.Warnings())

		for _, b := range db.Blocks() {
			require.Equal(t, tombstones.NewMemTombstones(), b.tombstones)
		}
	}
}

// TestTombstoneCleanResultEmptyBlock tests that a TombstoneClean that results in empty blocks (no timeseries)
// will also delete the resultant block.
func TestTombstoneCleanResultEmptyBlock(t *testing.T) {
	t.Parallel()
	numSamples := int64(10)

	db := newTestDB(t)

	ctx := context.Background()
	app := db.Appender(ctx)

	smpls := make([]float64, numSamples)
	for i := range numSamples {
		smpls[i] = rand.Float64()
		app.Append(0, labels.FromStrings("a", "b"), i, smpls[i])
	}

	require.NoError(t, app.Commit())
	// Interval should cover the whole block.
	intervals := tombstones.Intervals{{Mint: 0, Maxt: numSamples}}

	// Create snapshot.
	snap := t.TempDir()
	require.NoError(t, db.Snapshot(snap, true))
	require.NoError(t, db.Close())

	// Reopen DB from snapshot.
	db = newTestDB(t, withDir(snap))

	// Create tombstones by deleting all samples.
	for _, r := range intervals {
		require.NoError(t, db.Delete(ctx, r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
	}

	require.NoError(t, db.CleanTombstones())

	// After cleaning tombstones that covers the entire block, no blocks should be left behind.
	actualBlockDirs, err := blockDirs(db.Dir())
	require.NoError(t, err)
	require.Empty(t, actualBlockDirs)
}

// TestTombstoneCleanFail tests that a failing TombstoneClean doesn't leave any blocks behind.
// When TombstoneClean errors the original block that should be rebuilt doesn't get deleted so
// if TombstoneClean leaves any blocks behind these will overlap.
func TestTombstoneCleanFail(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	var oldBlockDirs []string

	// Create some blocks pending for compaction.
	// totalBlocks should be >=2 so we have enough blocks to trigger compaction failure.
	totalBlocks := 2
	for i := range totalBlocks {
		blockDir := createBlock(t, db.Dir(), genSeries(1, 1, int64(i), int64(i)+1))
		block, err := OpenBlock(nil, blockDir, nil, nil)
		require.NoError(t, err)
		// Add some fake tombstones to trigger the compaction.
		tomb := tombstones.NewMemTombstones()
		tomb.AddInterval(0, tombstones.Interval{Mint: int64(i), Maxt: int64(i) + 1})
		block.tombstones = tomb

		db.blocks = append(db.blocks, block)
		oldBlockDirs = append(oldBlockDirs, blockDir)
	}

	// Initialize the mockCompactorFailing with a room for a single compaction iteration.
	// mockCompactorFailing will fail on the second iteration so we can check if the cleanup works as expected.
	db.compactor = &mockCompactorFailing{
		t:      t,
		blocks: db.blocks,
		max:    totalBlocks + 1,
	}

	// The compactor should trigger a failure here.
	require.Error(t, db.CleanTombstones())

	// Now check that the CleanTombstones replaced the old block even after a failure.
	actualBlockDirs, err := blockDirs(db.Dir())
	require.NoError(t, err)
	// Only one block should have been replaced by a new block.
	require.Len(t, actualBlockDirs, len(oldBlockDirs))
	require.Len(t, intersection(oldBlockDirs, actualBlockDirs), len(actualBlockDirs)-1)
}

func intersection(oldBlocks, actualBlocks []string) (intersection []string) {
	hash := make(map[string]bool)
	for _, e := range oldBlocks {
		hash[e] = true
	}
	for _, e := range actualBlocks {
		// If block present in the hashmap then append intersection list.
		if hash[e] {
			intersection = append(intersection, e)
		}
	}
	return intersection
}

// mockCompactorFailing creates a new empty block on every write and fails when reached the max allowed total.
// For CompactOOO, it always fails.
type mockCompactorFailing struct {
	t      *testing.T
	blocks []*Block
	max    int
}

func (*mockCompactorFailing) Plan(string) ([]string, error) {
	return nil, nil
}

func (c *mockCompactorFailing) Write(dest string, _ BlockReader, _, _ int64, _ *BlockMeta) ([]ulid.ULID, error) {
	if len(c.blocks) >= c.max {
		return []ulid.ULID{}, errors.New("the compactor already did the maximum allowed blocks so it is time to fail")
	}

	block, err := OpenBlock(nil, createBlock(c.t, dest, genSeries(1, 1, 0, 1)), nil, nil)
	require.NoError(c.t, err)
	require.NoError(c.t, block.Close()) // Close block as we won't be using anywhere.
	c.blocks = append(c.blocks, block)

	// Now check that all expected blocks are actually persisted on disk.
	// This way we make sure that we have some blocks that are supposed to be removed.
	var expectedBlocks []string
	for _, b := range c.blocks {
		expectedBlocks = append(expectedBlocks, filepath.Join(dest, b.Meta().ULID.String()))
	}
	actualBlockDirs, err := blockDirs(dest)
	require.NoError(c.t, err)

	require.Equal(c.t, expectedBlocks, actualBlockDirs)

	return []ulid.ULID{block.Meta().ULID}, nil
}

func (*mockCompactorFailing) Compact(string, []string, []*Block) ([]ulid.ULID, error) {
	return []ulid.ULID{}, nil
}

func (*mockCompactorFailing) CompactOOO(string, *OOOCompactionHead) (result []ulid.ULID, err error) {
	return nil, errors.New("mock compaction failing CompactOOO")
}

func TestTimeRetention(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name              string
		blocks            []*BlockMeta
		expBlocks         []*BlockMeta
		retentionDuration int64
	}{
		{
			name: "Block max time delta greater than retention duration",
			blocks: []*BlockMeta{
				{MinTime: 500, MaxTime: 900}, // Oldest block, beyond retention
				{MinTime: 1000, MaxTime: 1500},
				{MinTime: 1500, MaxTime: 2000}, // Newest block
			},
			expBlocks: []*BlockMeta{
				{MinTime: 1000, MaxTime: 1500},
				{MinTime: 1500, MaxTime: 2000},
			},
			retentionDuration: 1000,
		},
		{
			name: "Block max time delta equal to retention duration",
			blocks: []*BlockMeta{
				{MinTime: 500, MaxTime: 900},   // Oldest block
				{MinTime: 1000, MaxTime: 1500}, // Coinciding exactly with the retention duration.
				{MinTime: 1500, MaxTime: 2000}, // Newest block
			},
			expBlocks: []*BlockMeta{
				{MinTime: 1500, MaxTime: 2000},
			},
			retentionDuration: 500,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t, withRngs(1000))

			for _, m := range tc.blocks {
				createBlock(t, db.Dir(), genSeries(10, 10, m.MinTime, m.MaxTime))
			}

			require.NoError(t, db.reloadBlocks())       // Reload the db to register the new blocks.
			require.Len(t, db.Blocks(), len(tc.blocks)) // Ensure all blocks are registered.

			db.opts.RetentionDuration = tc.retentionDuration
			// Reloading should truncate the blocks which are >= the retention duration vs the first block.
			require.NoError(t, db.reloadBlocks())

			actBlocks := db.Blocks()

			require.Equal(t, 1, int(prom_testutil.ToFloat64(db.metrics.timeRetentionCount)), "metric retention count mismatch")
			require.Len(t, actBlocks, len(tc.expBlocks))
			for i, eb := range tc.expBlocks {
				require.Equal(t, eb.MinTime, actBlocks[i].meta.MinTime)
				require.Equal(t, eb.MaxTime, actBlocks[i].meta.MaxTime)
			}
		})
	}
}

func TestRetentionDurationMetric(t *testing.T) {
	db := newTestDB(t, withOpts(&Options{
		RetentionDuration: 1000,
	}), withRngs(100))

	expRetentionDuration := 1.0
	actRetentionDuration := prom_testutil.ToFloat64(db.metrics.retentionDuration)
	require.Equal(t, expRetentionDuration, actRetentionDuration, "metric retention duration mismatch")
}

func TestSizeRetention(t *testing.T) {
	t.Parallel()
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = 100
	db := newTestDB(t, withOpts(opts), withRngs(100))

	blocks := []*BlockMeta{
		{MinTime: 100, MaxTime: 200}, // Oldest block
		{MinTime: 200, MaxTime: 300},
		{MinTime: 300, MaxTime: 400},
		{MinTime: 400, MaxTime: 500},
		{MinTime: 500, MaxTime: 600}, // Newest Block
	}

	for _, m := range blocks {
		createBlock(t, db.Dir(), genSeries(100, 10, m.MinTime, m.MaxTime))
	}

	headBlocks := []*BlockMeta{
		{MinTime: 700, MaxTime: 800},
	}

	// Add some data to the WAL.
	headApp := db.Head().Appender(context.Background())
	var aSeries labels.Labels
	var it chunkenc.Iterator
	for _, m := range headBlocks {
		series := genSeries(100, 10, m.MinTime, m.MaxTime+1)
		for _, s := range series {
			aSeries = s.Labels()
			it = s.Iterator(it)
			for it.Next() == chunkenc.ValFloat {
				tim, v := it.At()
				_, err := headApp.Append(0, s.Labels(), tim, v)
				require.NoError(t, err)
			}
			require.NoError(t, it.Err())
		}
	}
	require.NoError(t, headApp.Commit())
	db.Head().mmapHeadChunks()

	require.Eventually(t, func() bool {
		return db.Head().chunkDiskMapper.IsQueueEmpty()
	}, 2*time.Second, 100*time.Millisecond)

	// Test that registered size matches the actual disk size.
	require.NoError(t, db.reloadBlocks())                               // Reload the db to register the new db size.
	require.Len(t, db.Blocks(), len(blocks))                            // Ensure all blocks are registered.
	blockSize := int64(prom_testutil.ToFloat64(db.metrics.blocksBytes)) // Use the actual internal metrics.
	walSize, err := db.Head().wal.Size()
	require.NoError(t, err)
	cdmSize, err := db.Head().chunkDiskMapper.Size()
	require.NoError(t, err)
	require.NotZero(t, cdmSize)
	// Expected size should take into account block size + WAL size + Head
	// chunks size
	expSize := blockSize + walSize + cdmSize
	actSize, err := fileutil.DirSize(db.Dir())
	require.NoError(t, err)
	require.Equal(t, expSize, actSize, "registered size doesn't match actual disk size")

	// Create a WAL checkpoint, and compare sizes.
	first, last, err := wlog.Segments(db.Head().wal.Dir())
	require.NoError(t, err)
	_, err = wlog.Checkpoint(promslog.NewNopLogger(), db.Head().wal, first, last-1, func(chunks.HeadSeriesRef) bool { return false }, 0)
	require.NoError(t, err)
	blockSize = int64(prom_testutil.ToFloat64(db.metrics.blocksBytes)) // Use the actual internal metrics.
	walSize, err = db.Head().wal.Size()
	require.NoError(t, err)
	cdmSize, err = db.Head().chunkDiskMapper.Size()
	require.NoError(t, err)
	require.NotZero(t, cdmSize)
	expSize = blockSize + walSize + cdmSize
	actSize, err = fileutil.DirSize(db.Dir())
	require.NoError(t, err)
	require.Equal(t, expSize, actSize, "registered size doesn't match actual disk size")

	// Truncate Chunk Disk Mapper and compare sizes.
	require.NoError(t, db.Head().chunkDiskMapper.Truncate(900))
	cdmSize, err = db.Head().chunkDiskMapper.Size()
	require.NoError(t, err)
	require.NotZero(t, cdmSize)
	expSize = blockSize + walSize + cdmSize
	actSize, err = fileutil.DirSize(db.Dir())
	require.NoError(t, err)
	require.Equal(t, expSize, actSize, "registered size doesn't match actual disk size")

	// Add some out of order samples to check the size of WBL.
	headApp = db.Head().Appender(context.Background())
	for ts := int64(750); ts < 800; ts++ {
		_, err := headApp.Append(0, aSeries, ts, float64(ts))
		require.NoError(t, err)
	}
	require.NoError(t, headApp.Commit())

	walSize, err = db.Head().wal.Size()
	require.NoError(t, err)
	wblSize, err := db.Head().wbl.Size()
	require.NoError(t, err)
	require.NotZero(t, wblSize)
	cdmSize, err = db.Head().chunkDiskMapper.Size()
	require.NoError(t, err)
	expSize = blockSize + walSize + wblSize + cdmSize
	actSize, err = fileutil.DirSize(db.Dir())
	require.NoError(t, err)
	require.Equal(t, expSize, actSize, "registered size doesn't match actual disk size")

	// Decrease the max bytes limit so that a delete is triggered.
	// Check total size, total count and check that the oldest block was deleted.
	firstBlockSize := db.Blocks()[0].Size()
	sizeLimit := actSize - firstBlockSize
	db.opts.MaxBytes = sizeLimit          // Set the new db size limit one block smaller that the actual size.
	require.NoError(t, db.reloadBlocks()) // Reload the db to register the new db size.

	expBlocks := blocks[1:]
	actBlocks := db.Blocks()
	blockSize = int64(prom_testutil.ToFloat64(db.metrics.blocksBytes))
	walSize, err = db.Head().wal.Size()
	require.NoError(t, err)
	cdmSize, err = db.Head().chunkDiskMapper.Size()
	require.NoError(t, err)
	require.NotZero(t, cdmSize)
	// Expected size should take into account block size + WAL size + WBL size
	expSize = blockSize + walSize + wblSize + cdmSize
	actRetentionCount := int(prom_testutil.ToFloat64(db.metrics.sizeRetentionCount))
	actSize, err = fileutil.DirSize(db.Dir())
	require.NoError(t, err)

	require.Equal(t, 1, actRetentionCount, "metric retention count mismatch")
	require.Equal(t, expSize, actSize, "metric db size doesn't match actual disk size")
	require.LessOrEqual(t, expSize, sizeLimit, "actual size (%v) is expected to be less than or equal to limit (%v)", expSize, sizeLimit)
	require.Len(t, actBlocks, len(blocks)-1, "new block count should be decreased from:%v to:%v", len(blocks), len(blocks)-1)
	require.Equal(t, expBlocks[0].MaxTime, actBlocks[0].meta.MaxTime, "maxT mismatch of the first block")
	require.Equal(t, expBlocks[len(expBlocks)-1].MaxTime, actBlocks[len(actBlocks)-1].meta.MaxTime, "maxT mismatch of the last block")
}

func TestSizeRetentionMetric(t *testing.T) {
	cases := []struct {
		maxBytes    int64
		expMaxBytes int64
	}{
		{maxBytes: 1000, expMaxBytes: 1000},
		{maxBytes: 0, expMaxBytes: 0},
		{maxBytes: -1000, expMaxBytes: 0},
	}

	for _, c := range cases {
		db := newTestDB(t, withOpts(&Options{
			MaxBytes: c.maxBytes,
		}), withRngs(100))

		actMaxBytes := int64(prom_testutil.ToFloat64(db.metrics.maxBytes))
		require.Equal(t, c.expMaxBytes, actMaxBytes, "metric retention limit bytes mismatch")
	}
}

// TestRuntimeRetentionConfigChange tests that retention configuration can be
// changed at runtime via ApplyConfig and that the retention logic properly
// deletes blocks when retention is shortened. This test also ensures race-free
// concurrent access to retention settings.
func TestRuntimeRetentionConfigChange(t *testing.T) {
	const (
		initialRetentionDuration = int64(10 * time.Hour / time.Millisecond) // 10 hours
		shorterRetentionDuration = int64(1 * time.Hour / time.Millisecond)  // 1 hour
	)

	db := newTestDB(t, withOpts(&Options{
		RetentionDuration: initialRetentionDuration,
	}), withRngs(100))

	nineHoursMs := int64(9 * time.Hour / time.Millisecond)
	nineAndHalfHoursMs := int64((9*time.Hour + 30*time.Minute) / time.Millisecond)
	blocks := []*BlockMeta{
		{MinTime: 0, MaxTime: 100},                                       // 10 hours old (beyond new retention)
		{MinTime: 100, MaxTime: 200},                                     // 9.9 hours old (beyond new retention)
		{MinTime: nineHoursMs, MaxTime: nineAndHalfHoursMs},              // 1 hour old (within new retention)
		{MinTime: nineAndHalfHoursMs, MaxTime: initialRetentionDuration}, // 0.5 hours old (within new retention)
	}

	for _, m := range blocks {
		createBlock(t, db.Dir(), genSeriesFromSampleGenerator(10, 10, m.MinTime, m.MaxTime, int64(time.Minute/time.Millisecond), func(ts int64) chunks.Sample {
			return sample{t: ts, f: rand.Float64()}
		}))
	}

	// Reload blocks and verify all are loaded.
	require.NoError(t, db.reloadBlocks())
	require.Len(t, db.Blocks(), len(blocks), "expected all blocks to be loaded initially")

	cfg := &config.Config{
		StorageConfig: config.StorageConfig{
			TSDBConfig: &config.TSDBConfig{
				Retention: &config.TSDBRetentionConfig{
					Time: model.Duration(shorterRetentionDuration),
				},
			},
		},
	}

	require.NoError(t, db.ApplyConfig(cfg), "ApplyConfig should succeed")

	actualRetention := db.getRetentionDuration()
	require.Equal(t, shorterRetentionDuration, actualRetention, "retention duration should be updated")

	expectedRetentionSeconds := (time.Duration(shorterRetentionDuration) * time.Millisecond).Seconds()
	actualRetentionSeconds := prom_testutil.ToFloat64(db.metrics.retentionDuration)
	require.Equal(t, expectedRetentionSeconds, actualRetentionSeconds, "retention duration metric should be updated")

	require.NoError(t, db.reloadBlocks())

	// Verify that blocks beyond the new retention were deleted.
	// We expect only the last 2 blocks to remain (those within 1 hour).
	actBlocks := db.Blocks()
	require.Len(t, actBlocks, 2, "expected old blocks to be deleted after retention change")

	// Verify the remaining blocks are the newest ones.
	require.Equal(t, nineHoursMs, actBlocks[0].meta.MinTime, "first remaining block should be within retention")
	require.Equal(t, nineAndHalfHoursMs, actBlocks[1].meta.MinTime, "last remaining block should be the newest")

	require.Positive(t, int(prom_testutil.ToFloat64(db.metrics.timeRetentionCount)), "time retention count should be incremented")
}

func TestNotMatcherSelectsLabelsUnsetSeries(t *testing.T) {
	db := newTestDB(t)

	labelpairs := []labels.Labels{
		labels.FromStrings("a", "abcd", "b", "abcde"),
		labels.FromStrings("labelname", "labelvalue"),
	}

	ctx := context.Background()
	app := db.Appender(ctx)
	for _, lbls := range labelpairs {
		_, err := app.Append(0, lbls, 0, 1)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	cases := []struct {
		selector labels.Selector
		series   []labels.Labels
	}{{
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchNotEqual, "lname", "lvalue"),
		},
		series: labelpairs,
	}, {
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchEqual, "a", "abcd"),
			labels.MustNewMatcher(labels.MatchNotEqual, "b", "abcde"),
		},
		series: []labels.Labels{},
	}, {
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchEqual, "a", "abcd"),
			labels.MustNewMatcher(labels.MatchNotEqual, "b", "abc"),
		},
		series: []labels.Labels{labelpairs[0]},
	}, {
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchNotRegexp, "a", "abd.*"),
		},
		series: labelpairs,
	}, {
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchNotRegexp, "a", "abc.*"),
		},
		series: labelpairs[1:],
	}, {
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchNotRegexp, "c", "abd.*"),
		},
		series: labelpairs,
	}, {
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchNotRegexp, "labelname", "labelvalue"),
		},
		series: labelpairs[:1],
	}}

	q, err := db.Querier(0, 10)
	require.NoError(t, err)
	defer func() { require.NoError(t, q.Close()) }()

	for _, c := range cases {
		ss := q.Select(ctx, false, nil, c.selector...)
		lres, _, ws, err := expandSeriesSet(ss)
		require.NoError(t, err)
		require.Empty(t, ws)
		require.Equal(t, c.series, lres)
	}
}

// expandSeriesSet returns the raw labels in the order they are retrieved from
// the series set and the samples keyed by Labels().String().
func expandSeriesSet(ss storage.SeriesSet) ([]labels.Labels, map[string][]sample, annotations.Annotations, error) {
	resultLabels := []labels.Labels{}
	resultSamples := map[string][]sample{}
	var it chunkenc.Iterator
	for ss.Next() {
		series := ss.At()
		samples := []sample{}
		it = series.Iterator(it)
		for it.Next() == chunkenc.ValFloat {
			t, v := it.At()
			samples = append(samples, sample{t: t, f: v})
		}
		resultLabels = append(resultLabels, series.Labels())
		resultSamples[series.Labels().String()] = samples
	}
	return resultLabels, resultSamples, ss.Warnings(), ss.Err()
}

func TestOverlappingBlocksDetectsAllOverlaps(t *testing.T) {
	// Create 10 blocks that does not overlap (0-10, 10-20, ..., 100-110) but in reverse order to ensure our algorithm
	// will handle that.
	metas := make([]BlockMeta, 11)
	for i := 10; i >= 0; i-- {
		metas[i] = BlockMeta{MinTime: int64(i * 10), MaxTime: int64((i + 1) * 10)}
	}

	require.Empty(t, OverlappingBlocks(metas), "we found unexpected overlaps")

	// Add overlapping blocks. We've to establish order again since we aren't interested
	// in trivial overlaps caused by unorderedness.
	add := func(ms ...BlockMeta) []BlockMeta {
		repl := append(append([]BlockMeta{}, metas...), ms...)
		sort.Slice(repl, func(i, j int) bool {
			return repl[i].MinTime < repl[j].MinTime
		})
		return repl
	}

	// o1 overlaps with 10-20.
	o1 := BlockMeta{MinTime: 15, MaxTime: 17}
	require.Equal(t, Overlaps{
		{Min: 15, Max: 17}: {metas[1], o1},
	}, OverlappingBlocks(add(o1)))

	// o2 overlaps with 20-30 and 30-40.
	o2 := BlockMeta{MinTime: 21, MaxTime: 31}
	require.Equal(t, Overlaps{
		{Min: 21, Max: 30}: {metas[2], o2},
		{Min: 30, Max: 31}: {o2, metas[3]},
	}, OverlappingBlocks(add(o2)))

	// o3a and o3b overlaps with 30-40 and each other.
	o3a := BlockMeta{MinTime: 33, MaxTime: 39}
	o3b := BlockMeta{MinTime: 34, MaxTime: 36}
	require.Equal(t, Overlaps{
		{Min: 34, Max: 36}: {metas[3], o3a, o3b},
	}, OverlappingBlocks(add(o3a, o3b)))

	// o4 is 1:1 overlap with 50-60.
	o4 := BlockMeta{MinTime: 50, MaxTime: 60}
	require.Equal(t, Overlaps{
		{Min: 50, Max: 60}: {metas[5], o4},
	}, OverlappingBlocks(add(o4)))

	// o5 overlaps with 60-70, 70-80 and 80-90.
	o5 := BlockMeta{MinTime: 61, MaxTime: 85}
	require.Equal(t, Overlaps{
		{Min: 61, Max: 70}: {metas[6], o5},
		{Min: 70, Max: 80}: {o5, metas[7]},
		{Min: 80, Max: 85}: {o5, metas[8]},
	}, OverlappingBlocks(add(o5)))

	// o6a overlaps with 90-100, 100-110 and o6b, o6b overlaps with 90-100 and o6a.
	o6a := BlockMeta{MinTime: 92, MaxTime: 105}
	o6b := BlockMeta{MinTime: 94, MaxTime: 99}
	require.Equal(t, Overlaps{
		{Min: 94, Max: 99}:   {metas[9], o6a, o6b},
		{Min: 100, Max: 105}: {o6a, metas[10]},
	}, OverlappingBlocks(add(o6a, o6b)))

	// All together.
	require.Equal(t, Overlaps{
		{Min: 15, Max: 17}: {metas[1], o1},
		{Min: 21, Max: 30}: {metas[2], o2}, {Min: 30, Max: 31}: {o2, metas[3]},
		{Min: 34, Max: 36}: {metas[3], o3a, o3b},
		{Min: 50, Max: 60}: {metas[5], o4},
		{Min: 61, Max: 70}: {metas[6], o5}, {Min: 70, Max: 80}: {o5, metas[7]}, {Min: 80, Max: 85}: {o5, metas[8]},
		{Min: 94, Max: 99}: {metas[9], o6a, o6b}, {Min: 100, Max: 105}: {o6a, metas[10]},
	}, OverlappingBlocks(add(o1, o2, o3a, o3b, o4, o5, o6a, o6b)))

	// Additional case.
	var nc1 []BlockMeta
	nc1 = append(nc1, BlockMeta{MinTime: 1, MaxTime: 5})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 3})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 3})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 3})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 3})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 6})
	nc1 = append(nc1, BlockMeta{MinTime: 3, MaxTime: 5})
	nc1 = append(nc1, BlockMeta{MinTime: 5, MaxTime: 7})
	nc1 = append(nc1, BlockMeta{MinTime: 7, MaxTime: 10})
	nc1 = append(nc1, BlockMeta{MinTime: 8, MaxTime: 9})
	require.Equal(t, Overlaps{
		{Min: 2, Max: 3}: {nc1[0], nc1[1], nc1[2], nc1[3], nc1[4], nc1[5]}, // 1-5, 2-3, 2-3, 2-3, 2-3, 2,6
		{Min: 3, Max: 5}: {nc1[0], nc1[5], nc1[6]},                         // 1-5, 2-6, 3-5
		{Min: 5, Max: 6}: {nc1[5], nc1[7]},                                 // 2-6, 5-7
		{Min: 8, Max: 9}: {nc1[8], nc1[9]},                                 // 7-10, 8-9
	}, OverlappingBlocks(nc1))
}

// Regression test for https://github.com/prometheus/tsdb/issues/347
func TestChunkAtBlockBoundary(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	ctx := context.Background()
	app := db.Appender(ctx)

	blockRange := db.compactor.(*LeveledCompactor).ranges[0]
	label := labels.FromStrings("foo", "bar")

	for i := range int64(3) {
		_, err := app.Append(0, label, i*blockRange, 0)
		require.NoError(t, err)
		_, err = app.Append(0, label, i*blockRange+1000, 0)
		require.NoError(t, err)
	}

	err := app.Commit()
	require.NoError(t, err)

	err = db.Compact(ctx)
	require.NoError(t, err)

	var builder labels.ScratchBuilder

	for _, block := range db.Blocks() {
		r, err := block.Index()
		require.NoError(t, err)
		defer r.Close()

		meta := block.Meta()

		k, v := index.AllPostingsKey()
		p, err := r.Postings(ctx, k, v)
		require.NoError(t, err)

		var chks []chunks.Meta

		chunkCount := 0

		for p.Next() {
			err = r.Series(p.At(), &builder, &chks)
			require.NoError(t, err)
			for _, c := range chks {
				require.True(t, meta.MinTime <= c.MinTime && c.MaxTime <= meta.MaxTime,
					"chunk spans beyond block boundaries: [block.MinTime=%d, block.MaxTime=%d]; [chunk.MinTime=%d, chunk.MaxTime=%d]",
					meta.MinTime, meta.MaxTime, c.MinTime, c.MaxTime)
				chunkCount++
			}
		}
		require.Equal(t, 1, chunkCount, "expected 1 chunk in block %s, got %d", meta.ULID, chunkCount)
	}
}

func TestQuerierWithBoundaryChunks(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	ctx := context.Background()
	app := db.Appender(ctx)

	blockRange := db.compactor.(*LeveledCompactor).ranges[0]
	label := labels.FromStrings("foo", "bar")

	for i := range int64(5) {
		_, err := app.Append(0, label, i*blockRange, 0)
		require.NoError(t, err)
		_, err = app.Append(0, labels.FromStrings("blockID", strconv.FormatInt(i, 10)), i*blockRange, 0)
		require.NoError(t, err)
	}

	err := app.Commit()
	require.NoError(t, err)

	err = db.Compact(ctx)
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(db.blocks), 3, "invalid test, less than three blocks in DB")

	q, err := db.Querier(blockRange, 2*blockRange)
	require.NoError(t, err)
	defer q.Close()

	// The requested interval covers 2 blocks, so the querier's label values for blockID should give us 2 values, one from each block.
	b, ws, err := q.LabelValues(ctx, "blockID", nil)
	require.NoError(t, err)
	var nilAnnotations annotations.Annotations
	require.Equal(t, nilAnnotations, ws)
	require.Equal(t, []string{"1", "2"}, b)
}

// TestInitializeHeadTimestamp ensures that the h.minTime is set properly.
//   - no blocks no WAL: set to the time of the first  appended sample
//   - no blocks with WAL: set to the smallest sample from the WAL
//   - with blocks no WAL: set to the last block maxT
//   - with blocks with WAL: same as above
func TestInitializeHeadTimestamp(t *testing.T) {
	t.Parallel()
	t.Run("clean", func(t *testing.T) {
		db := newTestDB(t)

		// Should be set to init values if no WAL or blocks exist so far.
		require.Equal(t, int64(math.MaxInt64), db.head.MinTime())
		require.Equal(t, int64(math.MinInt64), db.head.MaxTime())
		require.False(t, db.head.initialized())

		// First added sample initializes the writable range.
		ctx := context.Background()
		app := db.Appender(ctx)
		_, err := app.Append(0, labels.FromStrings("a", "b"), 1000, 1)
		require.NoError(t, err)

		require.Equal(t, int64(1000), db.head.MinTime())
		require.Equal(t, int64(1000), db.head.MaxTime())
		require.True(t, db.head.initialized())
	})
	t.Run("wal-only", func(t *testing.T) {
		dir := t.TempDir()

		require.NoError(t, os.MkdirAll(path.Join(dir, "wal"), 0o777))
		w, err := wlog.New(nil, nil, path.Join(dir, "wal"), compression.None)
		require.NoError(t, err)

		var enc record.Encoder
		err = w.Log(
			enc.Series([]record.RefSeries{
				{Ref: 123, Labels: labels.FromStrings("a", "1")},
				{Ref: 124, Labels: labels.FromStrings("a", "2")},
			}, nil),
			enc.Samples([]record.RefSample{
				{Ref: 123, T: 5000, V: 1},
				{Ref: 124, T: 15000, V: 1},
			}, nil),
		)
		require.NoError(t, err)
		require.NoError(t, w.Close())

		db := newTestDB(t, withDir(dir))

		require.Equal(t, int64(5000), db.head.MinTime())
		require.Equal(t, int64(15000), db.head.MaxTime())
		require.True(t, db.head.initialized())
	})
	t.Run("existing-block", func(t *testing.T) {
		dir := t.TempDir()

		createBlock(t, dir, genSeries(1, 1, 1000, 2000))

		db := newTestDB(t, withDir(dir))

		require.Equal(t, int64(2000), db.head.MinTime())
		require.Equal(t, int64(2000), db.head.MaxTime())
		require.True(t, db.head.initialized())
	})
	t.Run("existing-block-and-wal", func(t *testing.T) {
		dir := t.TempDir()

		createBlock(t, dir, genSeries(1, 1, 1000, 6000))

		require.NoError(t, os.MkdirAll(path.Join(dir, "wal"), 0o777))
		w, err := wlog.New(nil, nil, path.Join(dir, "wal"), compression.None)
		require.NoError(t, err)

		var enc record.Encoder
		err = w.Log(
			enc.Series([]record.RefSeries{
				{Ref: 123, Labels: labels.FromStrings("a", "1")},
				{Ref: 124, Labels: labels.FromStrings("a", "2")},
			}, nil),
			enc.Samples([]record.RefSample{
				{Ref: 123, T: 5000, V: 1},
				{Ref: 124, T: 15000, V: 1},
			}, nil),
		)
		require.NoError(t, err)
		require.NoError(t, w.Close())

		db := newTestDB(t, withDir(dir))

		require.Equal(t, int64(6000), db.head.MinTime())
		require.Equal(t, int64(15000), db.head.MaxTime())
		require.True(t, db.head.initialized())
		// Check that old series has been GCed.
		require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.series))
	})
}

func TestNoEmptyBlocks(t *testing.T) {
	t.Parallel()
	db := newTestDB(t, withRngs(100))
	ctx := context.Background()

	db.DisableCompactions()

	rangeToTriggerCompaction := db.compactor.(*LeveledCompactor).ranges[0]/2*3 - 1
	defaultLabel := labels.FromStrings("foo", "bar")
	defaultMatcher := labels.MustNewMatcher(labels.MatchRegexp, "", ".*")

	t.Run("Test no blocks after compact with empty head.", func(t *testing.T) {
		require.NoError(t, db.Compact(ctx))
		actBlocks, err := blockDirs(db.Dir())
		require.NoError(t, err)
		require.Len(t, actBlocks, len(db.Blocks()))
		require.Empty(t, actBlocks)
		require.Equal(t, 0, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.Ran)), "no compaction should be triggered here")
	})

	t.Run("Test no blocks after deleting all samples from head.", func(t *testing.T) {
		app := db.Appender(ctx)
		_, err := app.Append(0, defaultLabel, 1, 0)
		require.NoError(t, err)
		_, err = app.Append(0, defaultLabel, 2, 0)
		require.NoError(t, err)
		_, err = app.Append(0, defaultLabel, 3+rangeToTriggerCompaction, 0)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		require.NoError(t, db.Delete(ctx, math.MinInt64, math.MaxInt64, defaultMatcher))
		require.NoError(t, db.Compact(ctx))
		require.Equal(t, 1, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.Ran)), "compaction should have been triggered here")

		actBlocks, err := blockDirs(db.Dir())
		require.NoError(t, err)
		require.Len(t, actBlocks, len(db.Blocks()))
		require.Empty(t, actBlocks)

		app = db.Appender(ctx)
		_, err = app.Append(0, defaultLabel, 1, 0)
		require.Equal(t, storage.ErrOutOfBounds, err, "the head should be truncated so no samples in the past should be allowed")

		// Adding new blocks.
		currentTime := db.Head().MaxTime()
		_, err = app.Append(0, defaultLabel, currentTime, 0)
		require.NoError(t, err)
		_, err = app.Append(0, defaultLabel, currentTime+1, 0)
		require.NoError(t, err)
		_, err = app.Append(0, defaultLabel, currentTime+rangeToTriggerCompaction, 0)
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		require.NoError(t, db.Compact(ctx))
		require.Equal(t, 2, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.Ran)), "compaction should have been triggered here")
		actBlocks, err = blockDirs(db.Dir())
		require.NoError(t, err)
		require.Len(t, actBlocks, len(db.Blocks()))
		require.Len(t, actBlocks, 1, "No blocks created when compacting with >0 samples")
	})

	t.Run(`When no new block is created from head, and there are some blocks on disk
	compaction should not run into infinite loop (was seen during development).`, func(t *testing.T) {
		oldBlocks := db.Blocks()
		app := db.Appender(ctx)
		currentTime := db.Head().MaxTime()
		_, err := app.Append(0, defaultLabel, currentTime, 0)
		require.NoError(t, err)
		_, err = app.Append(0, defaultLabel, currentTime+1, 0)
		require.NoError(t, err)
		_, err = app.Append(0, defaultLabel, currentTime+rangeToTriggerCompaction, 0)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		require.NoError(t, db.head.Delete(ctx, math.MinInt64, math.MaxInt64, defaultMatcher))
		require.NoError(t, db.Compact(ctx))
		require.Equal(t, 3, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.Ran)), "compaction should have been triggered here")
		require.Equal(t, oldBlocks, db.Blocks())
	})

	t.Run("Test no blocks remaining after deleting all samples from disk.", func(t *testing.T) {
		currentTime := db.Head().MaxTime()
		blocks := []*BlockMeta{
			{MinTime: currentTime, MaxTime: currentTime + db.compactor.(*LeveledCompactor).ranges[0]},
			{MinTime: currentTime + 100, MaxTime: currentTime + 100 + db.compactor.(*LeveledCompactor).ranges[0]},
		}
		for _, m := range blocks {
			createBlock(t, db.Dir(), genSeries(2, 2, m.MinTime, m.MaxTime))
		}

		oldBlocks := db.Blocks()
		require.NoError(t, db.reloadBlocks())                   // Reload the db to register the new blocks.
		require.Len(t, db.Blocks(), len(blocks)+len(oldBlocks)) // Ensure all blocks are registered.
		require.NoError(t, db.Delete(ctx, math.MinInt64, math.MaxInt64, defaultMatcher))
		require.NoError(t, db.Compact(ctx))
		require.Equal(t, 5, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.Ran)), "compaction should have been triggered here once for each block that have tombstones")

		actBlocks, err := blockDirs(db.Dir())
		require.NoError(t, err)
		require.Len(t, actBlocks, len(db.Blocks()))
		require.Len(t, actBlocks, 1, "All samples are deleted. Only the most recent block should remain after compaction.")
	})
}

func TestDB_LabelNames(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		// Add 'sampleLabels1' -> Test Head -> Compact -> Test Disk ->
		// -> Add 'sampleLabels2' -> Test Head+Disk

		sampleLabels1 [][2]string // For checking head and disk separately.
		// To test Head+Disk, sampleLabels2 should have
		// at least 1 unique label name which is not in sampleLabels1.
		sampleLabels2 [][2]string // For checking head and disk together.
		exp1          []string    // after adding sampleLabels1.
		exp2          []string    // after adding sampleLabels1 and sampleLabels2.
	}{
		{
			sampleLabels1: [][2]string{
				{"name1", "1"},
				{"name3", "3"},
				{"name2", "2"},
			},
			sampleLabels2: [][2]string{
				{"name4", "4"},
				{"name1", "1"},
			},
			exp1: []string{"name1", "name2", "name3"},
			exp2: []string{"name1", "name2", "name3", "name4"},
		},
		{
			sampleLabels1: [][2]string{
				{"name2", "2"},
				{"name1", "1"},
				{"name2", "2"},
			},
			sampleLabels2: [][2]string{
				{"name6", "6"},
				{"name0", "0"},
			},
			exp1: []string{"name1", "name2"},
			exp2: []string{"name0", "name1", "name2", "name6"},
		},
	}

	blockRange := int64(1000)
	// Appends samples into the database.
	appendSamples := func(db *DB, mint, maxt int64, sampleLabels [][2]string) {
		t.Helper()
		app := db.Appender(ctx)
		for i := mint; i <= maxt; i++ {
			for _, tuple := range sampleLabels {
				label := labels.FromStrings(tuple[0], tuple[1])
				_, err := app.Append(0, label, i*blockRange, 0)
				require.NoError(t, err)
			}
		}
		err := app.Commit()
		require.NoError(t, err)
	}
	for _, tst := range tests {
		t.Run("", func(t *testing.T) {
			ctx := context.Background()
			db := newTestDB(t)

			appendSamples(db, 0, 4, tst.sampleLabels1)

			// Testing head.
			headIndexr, err := db.head.Index()
			require.NoError(t, err)
			labelNames, err := headIndexr.LabelNames(ctx)
			require.NoError(t, err)
			require.Equal(t, tst.exp1, labelNames)
			require.NoError(t, headIndexr.Close())

			// Testing disk.
			err = db.Compact(ctx)
			require.NoError(t, err)
			// All blocks have same label names, hence check them individually.
			// No need to aggregate and check.
			for _, b := range db.Blocks() {
				blockIndexr, err := b.Index()
				require.NoError(t, err)
				labelNames, err = blockIndexr.LabelNames(ctx)
				require.NoError(t, err)
				require.Equal(t, tst.exp1, labelNames)
				require.NoError(t, blockIndexr.Close())
			}

			// Adding more samples to head with new label names
			// so that we can test (head+disk).LabelNames(ctx) (the union).
			appendSamples(db, 5, 9, tst.sampleLabels2)

			// Testing DB (union).
			q, err := db.Querier(math.MinInt64, math.MaxInt64)
			require.NoError(t, err)
			var ws annotations.Annotations
			labelNames, ws, err = q.LabelNames(ctx, nil)
			require.NoError(t, err)
			require.Empty(t, ws)
			require.NoError(t, q.Close())
			require.Equal(t, tst.exp2, labelNames)
		})
	}
}

func TestCorrectNumTombstones(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	blockRange := db.compactor.(*LeveledCompactor).ranges[0]
	name, value := "foo", "bar"
	defaultLabel := labels.FromStrings(name, value)
	defaultMatcher := labels.MustNewMatcher(labels.MatchEqual, name, value)

	ctx := context.Background()
	app := db.Appender(ctx)
	for i := range int64(3) {
		for j := range int64(15) {
			_, err := app.Append(0, defaultLabel, i*blockRange+j, 0)
			require.NoError(t, err)
		}
	}
	require.NoError(t, app.Commit())

	err := db.Compact(ctx)
	require.NoError(t, err)
	require.Len(t, db.blocks, 1)

	require.NoError(t, db.Delete(ctx, 0, 1, defaultMatcher))
	require.Equal(t, uint64(1), db.blocks[0].meta.Stats.NumTombstones)

	// {0, 1} and {2, 3} are merged to form 1 tombstone.
	require.NoError(t, db.Delete(ctx, 2, 3, defaultMatcher))
	require.Equal(t, uint64(1), db.blocks[0].meta.Stats.NumTombstones)

	require.NoError(t, db.Delete(ctx, 5, 6, defaultMatcher))
	require.Equal(t, uint64(2), db.blocks[0].meta.Stats.NumTombstones)

	require.NoError(t, db.Delete(ctx, 9, 11, defaultMatcher))
	require.Equal(t, uint64(3), db.blocks[0].meta.Stats.NumTombstones)
}

// TestBlockRanges checks the following use cases:
//   - No samples can be added with timestamps lower than the last block maxt.
//   - The compactor doesn't create overlapping blocks
//
// even when the last blocks is not within the default boundaries.
//   - Lower boundary is based on the smallest sample in the head and
//
// upper boundary is rounded to the configured block range.
//
// This ensures that a snapshot that includes the head and creates a block with a custom time range
// will not overlap with the first block created by the next compaction.
func TestBlockRanges(t *testing.T) {
	t.Parallel()
	logger := promslog.New(&promslog.Config{})
	ctx := context.Background()

	dir := t.TempDir()

	// Test that the compactor doesn't create overlapping blocks
	// when a non standard block already exists.
	firstBlockMaxT := int64(3)
	createBlock(t, dir, genSeries(1, 1, 0, firstBlockMaxT))
	db, err := open(dir, logger, nil, DefaultOptions(), []int64{10000}, nil)
	require.NoError(t, err)

	rangeToTriggerCompaction := db.compactor.(*LeveledCompactor).ranges[0]/2*3 + 1

	app := db.Appender(ctx)
	lbl := labels.FromStrings("a", "b")
	_, err = app.Append(0, lbl, firstBlockMaxT-1, rand.Float64())
	require.Error(t, err, "appending a sample with a timestamp covered by a previous block shouldn't be possible")
	_, err = app.Append(0, lbl, firstBlockMaxT+1, rand.Float64())
	require.NoError(t, err)
	_, err = app.Append(0, lbl, firstBlockMaxT+2, rand.Float64())
	require.NoError(t, err)
	secondBlockMaxt := firstBlockMaxT + rangeToTriggerCompaction
	_, err = app.Append(0, lbl, secondBlockMaxt, rand.Float64()) // Add samples to trigger a new compaction

	require.NoError(t, err)
	require.NoError(t, app.Commit())
	for range 100 {
		if len(db.Blocks()) == 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Len(t, db.Blocks(), 2, "no new block created after the set timeout")

	require.LessOrEqual(t, db.Blocks()[1].Meta().MinTime, db.Blocks()[0].Meta().MaxTime,
		"new block overlaps  old:%v,new:%v", db.Blocks()[0].Meta(), db.Blocks()[1].Meta())

	// Test that wal records are skipped when an existing block covers the same time ranges
	// and compaction doesn't create an overlapping block.
	app = db.Appender(ctx)
	db.DisableCompactions()
	_, err = app.Append(0, lbl, secondBlockMaxt+1, rand.Float64())
	require.NoError(t, err)
	_, err = app.Append(0, lbl, secondBlockMaxt+2, rand.Float64())
	require.NoError(t, err)
	_, err = app.Append(0, lbl, secondBlockMaxt+3, rand.Float64())
	require.NoError(t, err)
	_, err = app.Append(0, lbl, secondBlockMaxt+4, rand.Float64())
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.NoError(t, db.Close())

	thirdBlockMaxt := secondBlockMaxt + 2
	createBlock(t, dir, genSeries(1, 1, secondBlockMaxt+1, thirdBlockMaxt))

	db, err = open(dir, logger, nil, DefaultOptions(), []int64{10000}, nil)
	require.NoError(t, err)

	defer db.Close()
	require.Len(t, db.Blocks(), 3, "db doesn't include expected number of blocks")
	require.Equal(t, db.Blocks()[2].Meta().MaxTime, thirdBlockMaxt, "unexpected maxt of the last block")

	app = db.Appender(ctx)
	_, err = app.Append(0, lbl, thirdBlockMaxt+rangeToTriggerCompaction, rand.Float64()) // Trigger a compaction
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	for range 100 {
		if len(db.Blocks()) == 4 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	require.Len(t, db.Blocks(), 4, "no new block created after the set timeout")

	require.LessOrEqual(t, db.Blocks()[3].Meta().MinTime, db.Blocks()[2].Meta().MaxTime,
		"new block overlaps  old:%v,new:%v", db.Blocks()[2].Meta(), db.Blocks()[3].Meta())
}

// TestDBReadOnly ensures that opening a DB in readonly mode doesn't modify any files on the disk.
// It also checks that the API calls return equivalent results as a normal db.Open() mode.
func TestDBReadOnly(t *testing.T) {
	t.Parallel()
	var (
		dbDir     = t.TempDir()
		expBlocks []*Block
		expBlock  *Block
		expSeries map[string][]chunks.Sample
		expChunks map[string][][]chunks.Sample
		expDBHash []byte
		matchAll  = labels.MustNewMatcher(labels.MatchEqual, "", "")
		err       error
	)

	// Bootstrap the db.
	{
		dbBlocks := []*BlockMeta{
			// Create three 2-sample blocks.
			{MinTime: 10, MaxTime: 12},
			{MinTime: 12, MaxTime: 14},
			{MinTime: 14, MaxTime: 16},
		}

		for _, m := range dbBlocks {
			_ = createBlock(t, dbDir, genSeries(1, 1, m.MinTime, m.MaxTime))
		}

		// Add head to test DBReadOnly WAL reading capabilities.
		w, err := wlog.New(nil, nil, filepath.Join(dbDir, "wal"), compression.Snappy)
		require.NoError(t, err)
		h := createHead(t, w, genSeries(1, 1, 16, 18), dbDir)
		require.NoError(t, h.Close())
	}

	// Open a normal db to use for a comparison.
	{
		dbWritable := newTestDB(t, withDir(dbDir))
		dbWritable.DisableCompactions()

		dbSizeBeforeAppend, err := fileutil.DirSize(dbWritable.Dir())
		require.NoError(t, err)
		app := dbWritable.Appender(context.Background())
		_, err = app.Append(0, labels.FromStrings("foo", "bar"), dbWritable.Head().MaxTime()+1, 0)
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		expBlocks = dbWritable.Blocks()
		expBlock = expBlocks[0]
		expDbSize, err := fileutil.DirSize(dbWritable.Dir())
		require.NoError(t, err)
		require.Greater(t, expDbSize, dbSizeBeforeAppend, "db size didn't increase after an append")

		q, err := dbWritable.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		expSeries = query(t, q, matchAll)
		cq, err := dbWritable.ChunkQuerier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		expChunks = queryAndExpandChunks(t, cq, matchAll)

		require.NoError(t, dbWritable.Close()) // Close here to allow getting the dir hash for windows.
		expDBHash = testutil.DirHash(t, dbWritable.Dir())
	}

	// Open a read only db and ensure that the API returns the same result as the normal DB.
	dbReadOnly, err := OpenDBReadOnly(dbDir, "", nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, dbReadOnly.Close()) }()

	t.Run("blocks", func(t *testing.T) {
		blocks, err := dbReadOnly.Blocks()
		require.NoError(t, err)
		require.Len(t, blocks, len(expBlocks))
		for i, expBlock := range expBlocks {
			require.Equal(t, expBlock.Meta(), blocks[i].Meta(), "block meta mismatch")
		}
	})
	t.Run("block", func(t *testing.T) {
		blockID := expBlock.meta.ULID.String()
		block, err := dbReadOnly.Block(blockID, nil)
		require.NoError(t, err)
		require.Equal(t, expBlock.Meta(), block.Meta(), "block meta mismatch")
	})
	t.Run("invalid block ID", func(t *testing.T) {
		blockID := "01GTDVZZF52NSWB5SXQF0P2PGF"
		_, err := dbReadOnly.Block(blockID, nil)
		require.Error(t, err)
	})
	t.Run("last block ID", func(t *testing.T) {
		blockID, err := dbReadOnly.LastBlockID()
		require.NoError(t, err)
		require.Equal(t, expBlocks[2].Meta().ULID.String(), blockID)
	})
	t.Run("querier", func(t *testing.T) {
		// Open a read only db and ensure that the API returns the same result as the normal DB.
		q, err := dbReadOnly.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		readOnlySeries := query(t, q, matchAll)
		readOnlyDBHash := testutil.DirHash(t, dbDir)

		require.Len(t, readOnlySeries, len(expSeries), "total series mismatch")
		require.Equal(t, expSeries, readOnlySeries, "series mismatch")
		require.Equal(t, expDBHash, readOnlyDBHash, "after all read operations the db hash should remain the same")
	})
	t.Run("chunk querier", func(t *testing.T) {
		cq, err := dbReadOnly.ChunkQuerier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		readOnlySeries := queryAndExpandChunks(t, cq, matchAll)
		readOnlyDBHash := testutil.DirHash(t, dbDir)

		require.Len(t, readOnlySeries, len(expChunks), "total series mismatch")
		require.Equal(t, expChunks, readOnlySeries, "series chunks mismatch")
		require.Equal(t, expDBHash, readOnlyDBHash, "after all read operations the db hash should remain the same")
	})
}

// TestDBReadOnlyClosing ensures that after closing the db
// all api methods return an ErrClosed.
func TestDBReadOnlyClosing(t *testing.T) {
	t.Parallel()
	sandboxDir := t.TempDir()
	db, err := OpenDBReadOnly(t.TempDir(), sandboxDir, promslog.New(&promslog.Config{}))
	require.NoError(t, err)
	// The sandboxDir was there.
	require.DirExists(t, db.sandboxDir)
	require.NoError(t, db.Close())
	// The sandboxDir was deleted when closing.
	require.NoDirExists(t, db.sandboxDir)
	require.Equal(t, db.Close(), ErrClosed)
	_, err = db.Blocks()
	require.Equal(t, err, ErrClosed)
	_, err = db.Querier(0, 1)
	require.Equal(t, err, ErrClosed)
}

func TestDBReadOnly_FlushWAL(t *testing.T) {
	t.Parallel()
	var (
		dbDir = t.TempDir()
		err   error
		maxt  int
		ctx   = context.Background()
	)

	// Bootstrap the db.
	{
		// Append data to the WAL.
		db := newTestDB(t, withDir(dbDir))
		db.DisableCompactions()
		app := db.Appender(ctx)
		maxt = 1000
		for i := range maxt {
			_, err := app.Append(0, labels.FromStrings(defaultLabelName, "flush"), int64(i), 1.0)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
		require.NoError(t, db.Close())
	}

	// Flush WAL.
	db, err := OpenDBReadOnly(dbDir, "", nil)
	require.NoError(t, err)

	flush := t.TempDir()
	require.NoError(t, db.FlushWAL(flush))
	require.NoError(t, db.Close())

	// Reopen the DB from the flushed WAL block.
	db, err = OpenDBReadOnly(flush, "", nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	blocks, err := db.Blocks()
	require.NoError(t, err)
	require.Len(t, blocks, 1)

	querier, err := db.Querier(0, int64(maxt)-1)
	require.NoError(t, err)
	defer func() { require.NoError(t, querier.Close()) }()

	// Sum the values.
	seriesSet := querier.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, defaultLabelName, "flush"))
	var series chunkenc.Iterator

	sum := 0.0
	for seriesSet.Next() {
		series = seriesSet.At().Iterator(series)
		for series.Next() == chunkenc.ValFloat {
			_, v := series.At()
			sum += v
		}
		require.NoError(t, series.Err())
	}
	require.NoError(t, seriesSet.Err())
	require.Empty(t, seriesSet.Warnings())
	require.Equal(t, 1000.0, sum)
}

func TestDBReadOnly_Querier_NoAlteration(t *testing.T) {
	countChunks := func(dir string) int {
		files, err := os.ReadDir(mmappedChunksDir(dir))
		require.NoError(t, err)
		return len(files)
	}

	dirHash := func(dir string) (hash []byte) {
		// Windows requires the DB to be closed: "xxx\lock: The process cannot access the file because it is being used by another process."
		// But closing the DB alters the directory in this case (it'll cut a new chunk).
		if runtime.GOOS != "windows" {
			hash = testutil.DirHash(t, dir)
		}
		return hash
	}

	spinUpQuerierAndCheck := func(dir, sandboxDir string, chunksCount int) {
		dBDirHash := dirHash(dir)
		// Bootstrap a RO db from the same dir and set up a querier.
		dbReadOnly, err := OpenDBReadOnly(dir, sandboxDir, nil)
		require.NoError(t, err)
		require.Equal(t, chunksCount, countChunks(dir))
		q, err := dbReadOnly.Querier(math.MinInt, math.MaxInt)
		require.NoError(t, err)
		require.NoError(t, q.Close())
		require.NoError(t, dbReadOnly.Close())
		// The RO Head doesn't alter RW db chunks_head/.
		require.Equal(t, chunksCount, countChunks(dir))
		require.Equal(t, dirHash(dir), dBDirHash)
	}

	t.Run("doesn't cut chunks while replaying WAL", func(t *testing.T) {
		db := newTestDB(t)

		// Append until the first mmapped head chunk.
		for i := range 121 {
			app := db.Appender(context.Background())
			_, err := app.Append(0, labels.FromStrings("foo", "bar"), int64(i), 0)
			require.NoError(t, err)
			require.NoError(t, app.Commit())
		}

		spinUpQuerierAndCheck(db.Dir(), t.TempDir(), 0)

		// The RW Head should have no problem cutting its own chunk,
		// this also proves that a chunk needed to be cut.
		require.NotPanics(t, func() { db.ForceHeadMMap() })
		require.Equal(t, 1, countChunks(db.Dir()))
	})

	t.Run("doesn't truncate corrupted chunks", func(t *testing.T) {
		db := newTestDB(t)
		require.NoError(t, db.Close())

		// Simulate a corrupted chunk: without a header.
		chunk, err := os.Create(path.Join(mmappedChunksDir(db.Dir()), "000001"))
		require.NoError(t, err)
		require.NoError(t, chunk.Close())

		spinUpQuerierAndCheck(db.Dir(), t.TempDir(), 1)

		// The RW Head should have no problem truncating its corrupted file:
		// this proves that the chunk needed to be truncated.
		db = newTestDB(t, withDir(db.Dir()))

		require.NoError(t, err)
		require.Equal(t, 0, countChunks(db.Dir()))
	})
}

func TestDBCannotSeePartialCommits(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	db := newTestDB(t)

	stop := make(chan struct{})
	firstInsert := make(chan struct{})
	ctx := context.Background()

	// Insert data in batches.
	go func() {
		iter := 0
		for {
			app := db.Appender(ctx)

			for j := range 100 {
				_, err := app.Append(0, labels.FromStrings("foo", "bar", "a", strconv.Itoa(j)), int64(iter), float64(iter))
				require.NoError(t, err)
			}
			require.NoError(t, app.Commit())

			if iter == 0 {
				close(firstInsert)
			}
			iter++

			select {
			case <-stop:
				return
			default:
			}
		}
	}()

	<-firstInsert

	// This is a race condition, so do a few tests to tickle it.
	// Usually most will fail.
	inconsistencies := 0
	for range 10 {
		func() {
			querier, err := db.Querier(0, 1000000)
			require.NoError(t, err)
			defer querier.Close()

			ss := querier.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
			_, seriesSet, ws, err := expandSeriesSet(ss)
			require.NoError(t, err)
			require.Empty(t, ws)

			values := map[float64]struct{}{}
			for _, series := range seriesSet {
				values[series[len(series)-1].f] = struct{}{}
			}
			if len(values) != 1 {
				inconsistencies++
			}
		}()
	}
	stop <- struct{}{}

	require.Equal(t, 0, inconsistencies, "Some queries saw inconsistent results.")
}

func TestDBQueryDoesntSeeAppendsAfterCreation(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	db := newTestDB(t)
	querierBeforeAdd, err := db.Querier(0, 1000000)
	require.NoError(t, err)
	defer querierBeforeAdd.Close()

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)

	querierAfterAddButBeforeCommit, err := db.Querier(0, 1000000)
	require.NoError(t, err)
	defer querierAfterAddButBeforeCommit.Close()

	// None of the queriers should return anything after the Add but before the commit.
	ss := querierBeforeAdd.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err := expandSeriesSet(ss)
	require.NoError(t, err)
	require.Empty(t, ws)
	require.Equal(t, map[string][]sample{}, seriesSet)

	ss = querierAfterAddButBeforeCommit.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err = expandSeriesSet(ss)
	require.NoError(t, err)
	require.Empty(t, ws)
	require.Equal(t, map[string][]sample{}, seriesSet)

	// This commit is after the queriers are created, so should not be returned.
	err = app.Commit()
	require.NoError(t, err)

	// Nothing returned for querier created before the Add.
	ss = querierBeforeAdd.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err = expandSeriesSet(ss)
	require.NoError(t, err)
	require.Empty(t, ws)
	require.Equal(t, map[string][]sample{}, seriesSet)

	// Series exists but has no samples for querier created after Add.
	ss = querierAfterAddButBeforeCommit.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err = expandSeriesSet(ss)
	require.NoError(t, err)
	require.Empty(t, ws)
	require.Equal(t, map[string][]sample{`{foo="bar"}`: {}}, seriesSet)

	querierAfterCommit, err := db.Querier(0, 1000000)
	require.NoError(t, err)
	defer querierAfterCommit.Close()

	// Samples are returned for querier created after Commit.
	ss = querierAfterCommit.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err = expandSeriesSet(ss)
	require.NoError(t, err)
	require.Empty(t, ws)
	require.Equal(t, map[string][]sample{`{foo="bar"}`: {{t: 0, f: 0}}}, seriesSet)
}

func assureChunkFromSamples(t *testing.T, samples []chunks.Sample) chunks.Meta {
	chks, err := chunks.ChunkFromSamples(samples)
	require.NoError(t, err)
	return chks
}

// TestChunkWriter_ReadAfterWrite ensures that chunk segment are cut at the set segment size and
// that the resulted segments includes the expected chunks data.
func TestChunkWriter_ReadAfterWrite(t *testing.T) {
	chk1 := assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 1, nil, nil}})
	chk2 := assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 2, nil, nil}})
	chk3 := assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 3, nil, nil}})
	chk4 := assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 4, nil, nil}})
	chk5 := assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 5, nil, nil}})
	chunkSize := len(chk1.Chunk.Bytes()) + chunks.MaxChunkLengthFieldSize + chunks.ChunkEncodingSize + crc32.Size

	tests := []struct {
		chks [][]chunks.Meta
		segmentSize,
		expSegmentsCount int
		expSegmentSizes []int
	}{
		// 0:Last chunk ends at the segment boundary so
		// all chunks should fit in a single segment.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
				},
			},
			segmentSize:      3 * chunkSize,
			expSegmentSizes:  []int{3 * chunkSize},
			expSegmentsCount: 1,
		},
		// 1:Two chunks can fit in a single segment so the last one should result in a new segment.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
					chk4,
					chk5,
				},
			},
			segmentSize:      2 * chunkSize,
			expSegmentSizes:  []int{2 * chunkSize, 2 * chunkSize, chunkSize},
			expSegmentsCount: 3,
		},
		// 2:When the segment size is smaller than the size of 2 chunks
		// the last segment should still create a new segment.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
				},
			},
			segmentSize:      2*chunkSize - 1,
			expSegmentSizes:  []int{chunkSize, chunkSize, chunkSize},
			expSegmentsCount: 3,
		},
		// 3:When the segment is smaller than a single chunk
		// it should still be written by ignoring the max segment size.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
				},
			},
			segmentSize:      chunkSize - 1,
			expSegmentSizes:  []int{chunkSize},
			expSegmentsCount: 1,
		},
		// 4:All chunks are bigger than the max segment size, but
		// these should still be written even when this will result in bigger segment than the set size.
		// Each segment will hold a single chunk.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
				},
			},
			segmentSize:      1,
			expSegmentSizes:  []int{chunkSize, chunkSize, chunkSize},
			expSegmentsCount: 3,
		},
		// 5:Adding multiple batches of chunks.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
				},
				{
					chk4,
					chk5,
				},
			},
			segmentSize:      3 * chunkSize,
			expSegmentSizes:  []int{3 * chunkSize, 2 * chunkSize},
			expSegmentsCount: 2,
		},
		// 6:Adding multiple batches of chunks.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
				},
				{
					chk2,
					chk3,
				},
				{
					chk4,
				},
			},
			segmentSize:      2 * chunkSize,
			expSegmentSizes:  []int{2 * chunkSize, 2 * chunkSize},
			expSegmentsCount: 2,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			tempDir := t.TempDir()

			chunkw, err := chunks.NewWriter(tempDir, chunks.WithSegmentSize(chunks.SegmentHeaderSize+int64(test.segmentSize)))
			require.NoError(t, err)

			for _, chks := range test.chks {
				require.NoError(t, chunkw.WriteChunks(chks...))
			}
			require.NoError(t, chunkw.Close())

			files, err := os.ReadDir(tempDir)
			require.NoError(t, err)
			require.Len(t, files, test.expSegmentsCount, "expected segments count mismatch")

			// Verify that all data is written to the segments.
			sizeExp := 0
			sizeAct := 0

			for _, chks := range test.chks {
				for _, chk := range chks {
					l := make([]byte, binary.MaxVarintLen32)
					sizeExp += binary.PutUvarint(l, uint64(len(chk.Chunk.Bytes()))) // The length field.
					sizeExp += chunks.ChunkEncodingSize
					sizeExp += len(chk.Chunk.Bytes()) // The data itself.
					sizeExp += crc32.Size             // The 4 bytes of crc32
				}
			}
			sizeExp += test.expSegmentsCount * chunks.SegmentHeaderSize // The segment header bytes.

			for i, f := range files {
				fi, err := f.Info()
				require.NoError(t, err)
				size := int(fi.Size())
				// Verify that the segment is the same or smaller than the expected size.
				require.GreaterOrEqual(t, chunks.SegmentHeaderSize+test.expSegmentSizes[i], size, "Segment:%v should NOT be bigger than:%v actual:%v", i, chunks.SegmentHeaderSize+test.expSegmentSizes[i], size)

				sizeAct += size
			}
			require.Equal(t, sizeExp, sizeAct)

			// Check the content of the chunks.
			r, err := chunks.NewDirReader(tempDir, nil)
			require.NoError(t, err)
			defer func() { require.NoError(t, r.Close()) }()

			for _, chks := range test.chks {
				for _, chkExp := range chks {
					chkAct, iterable, err := r.ChunkOrIterable(chkExp)
					require.NoError(t, err)
					require.Nil(t, iterable)
					require.Equal(t, chkExp.Chunk.Bytes(), chkAct.Bytes())
				}
			}
		})
	}
}

func TestRangeForTimestamp(t *testing.T) {
	type args struct {
		t     int64
		width int64
	}
	tests := []struct {
		args     args
		expected int64
	}{
		{args{0, 5}, 5},
		{args{1, 5}, 5},
		{args{5, 5}, 10},
		{args{6, 5}, 10},
		{args{13, 5}, 15},
		{args{95, 5}, 100},
	}
	for _, tt := range tests {
		got := rangeForTimestamp(tt.args.t, tt.args.width)
		require.Equal(t, tt.expected, got)
	}
}

// TestChunkReader_ConcurrentReads checks that the chunk result can be read concurrently.
// Regression test for https://github.com/prometheus/prometheus/pull/6514.
func TestChunkReader_ConcurrentReads(t *testing.T) {
	t.Parallel()
	chks := []chunks.Meta{
		assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 1, nil, nil}}),
		assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 2, nil, nil}}),
		assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 3, nil, nil}}),
		assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 4, nil, nil}}),
		assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 5, nil, nil}}),
	}

	tempDir := t.TempDir()

	chunkw, err := chunks.NewWriter(tempDir)
	require.NoError(t, err)

	require.NoError(t, chunkw.WriteChunks(chks...))
	require.NoError(t, chunkw.Close())

	r, err := chunks.NewDirReader(tempDir, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for _, chk := range chks {
		for range 100 {
			wg.Add(1)
			go func(chunk chunks.Meta) {
				defer wg.Done()

				chkAct, iterable, err := r.ChunkOrIterable(chunk)
				require.NoError(t, err)
				require.Nil(t, iterable)
				require.Equal(t, chunk.Chunk.Bytes(), chkAct.Bytes())
			}(chk)
		}
		wg.Wait()
	}
	require.NoError(t, r.Close())
}

// TestCompactHead ensures that the head compaction
// creates a block that is ready for loading and
// does not cause data loss.
// This test:
// * opens a storage;
// * appends values;
// * compacts the head; and
// * queries the db to ensure the samples are present from the compacted head.
func TestCompactHead(t *testing.T) {
	t.Parallel()

	// Open a DB and append data to the WAL.
	opts := &Options{
		RetentionDuration: int64(time.Hour * 24 * 15 / time.Millisecond),
		NoLockfile:        true,
		MinBlockDuration:  int64(time.Hour * 2 / time.Millisecond),
		MaxBlockDuration:  int64(time.Hour * 2 / time.Millisecond),
		WALCompression:    compression.Snappy,
	}
	db := newTestDB(t, withOpts(opts))
	ctx := context.Background()
	app := db.Appender(ctx)
	var expSamples []sample
	maxt := 100
	for i := range maxt {
		val := rand.Float64()
		_, err := app.Append(0, labels.FromStrings("a", "b"), int64(i), val)
		require.NoError(t, err)
		expSamples = append(expSamples, sample{0, int64(i), val, nil, nil})
	}
	require.NoError(t, app.Commit())

	// Compact the Head to create a new block.
	require.NoError(t, db.CompactHead(NewRangeHead(db.Head(), 0, int64(maxt)-1)))
	require.NoError(t, db.Close())

	// Delete everything but the new block and
	// reopen the db to query it to ensure it includes the head data.
	require.NoError(t, deleteNonBlocks(db.Dir()))
	db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
	require.Len(t, db.Blocks(), 1)
	require.Equal(t, int64(maxt), db.Head().MinTime())
	defer func() { require.NoError(t, db.Close()) }()
	querier, err := db.Querier(0, int64(maxt)-1)
	require.NoError(t, err)
	defer func() { require.NoError(t, querier.Close()) }()

	seriesSet := querier.Select(ctx, false, nil, &labels.Matcher{Type: labels.MatchEqual, Name: "a", Value: "b"})
	var series chunkenc.Iterator
	var actSamples []sample

	for seriesSet.Next() {
		series = seriesSet.At().Iterator(series)
		for series.Next() == chunkenc.ValFloat {
			time, val := series.At()
			actSamples = append(actSamples, sample{0, time, val, nil, nil})
		}
		require.NoError(t, series.Err())
	}
	require.Equal(t, expSamples, actSamples)
	require.NoError(t, seriesSet.Err())
}

// TestCompactHeadWithDeletion tests https://github.com/prometheus/prometheus/issues/11585.
func TestCompactHeadWithDeletion(t *testing.T) {
	db := newTestDB(t)

	ctx := context.Background()

	app := db.Appender(ctx)
	_, err := app.Append(0, labels.FromStrings("a", "b"), 10, rand.Float64())
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	err = db.Delete(ctx, 0, 100, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.NoError(t, err)

	// This recreates the bug.
	require.NoError(t, db.CompactHead(NewRangeHead(db.Head(), 0, 100)))
}

func deleteNonBlocks(dbDir string) error {
	dirs, err := os.ReadDir(dbDir)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if ok := isBlockDir(dir); !ok {
			if err := os.RemoveAll(filepath.Join(dbDir, dir.Name())); err != nil {
				return err
			}
		}
	}
	dirs, err = os.ReadDir(dbDir)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if ok := isBlockDir(dir); !ok {
			return fmt.Errorf("root folder:%v still hase non block directory:%v", dbDir, dir.Name())
		}
	}
	return nil
}

func TestOpen_VariousBlockStates(t *testing.T) {
	tmpDir := t.TempDir()

	var (
		expectedLoadedDirs  = map[string]struct{}{}
		expectedRemovedDirs = map[string]struct{}{}
		expectedIgnoredDirs = map[string]struct{}{}
	)

	{
		// Ok blocks; should be loaded.
		expectedLoadedDirs[createBlock(t, tmpDir, genSeries(10, 2, 0, 10))] = struct{}{}
		expectedLoadedDirs[createBlock(t, tmpDir, genSeries(10, 2, 10, 20))] = struct{}{}
	}
	{
		// Block to repair; should be repaired & loaded.
		dbDir := filepath.Join("testdata", "repair_index_version", "01BZJ9WJQPWHGNC2W4J9TA62KC")
		outDir := filepath.Join(tmpDir, "01BZJ9WJQPWHGNC2W4J9TA62KC")
		expectedLoadedDirs[outDir] = struct{}{}

		// Touch chunks dir in block.
		require.NoError(t, os.MkdirAll(filepath.Join(dbDir, "chunks"), 0o777))
		defer func() {
			require.NoError(t, os.RemoveAll(filepath.Join(dbDir, "chunks")))
		}()
		require.NoError(t, os.Mkdir(outDir, os.ModePerm))
		require.NoError(t, fileutil.CopyDirs(dbDir, outDir))
	}
	{
		// Missing meta.json; should be ignored and only logged.
		// TODO(bwplotka): Probably add metric.
		dir := createBlock(t, tmpDir, genSeries(10, 2, 20, 30))
		expectedIgnoredDirs[dir] = struct{}{}
		require.NoError(t, os.Remove(filepath.Join(dir, metaFilename)))
	}
	{
		// Tmp blocks during creation; those should be removed on start.
		dir := createBlock(t, tmpDir, genSeries(10, 2, 30, 40))
		require.NoError(t, fileutil.Replace(dir, dir+tmpForCreationBlockDirSuffix))
		expectedRemovedDirs[dir+tmpForCreationBlockDirSuffix] = struct{}{}

		// Tmp blocks during deletion; those should be removed on start.
		dir = createBlock(t, tmpDir, genSeries(10, 2, 40, 50))
		require.NoError(t, fileutil.Replace(dir, dir+tmpForDeletionBlockDirSuffix))
		expectedRemovedDirs[dir+tmpForDeletionBlockDirSuffix] = struct{}{}

		// Pre-2.21 tmp blocks; those should be removed on start.
		dir = createBlock(t, tmpDir, genSeries(10, 2, 50, 60))
		require.NoError(t, fileutil.Replace(dir, dir+tmpLegacy))
		expectedRemovedDirs[dir+tmpLegacy] = struct{}{}
	}
	{
		// One ok block; but two should be replaced.
		dir := createBlock(t, tmpDir, genSeries(10, 2, 50, 60))
		expectedLoadedDirs[dir] = struct{}{}

		m, _, err := readMetaFile(dir)
		require.NoError(t, err)

		compacted := createBlock(t, tmpDir, genSeries(10, 2, 50, 55))
		expectedRemovedDirs[compacted] = struct{}{}

		m.Compaction.Parents = append(m.Compaction.Parents,
			BlockDesc{ULID: ulid.MustParse(filepath.Base(compacted))},
			BlockDesc{ULID: ulid.MustNew(1, nil)},
			BlockDesc{ULID: ulid.MustNew(123, nil)},
		)

		// Regression test: Already removed parent can be still in list, which was causing Open errors.
		m.Compaction.Parents = append(m.Compaction.Parents, BlockDesc{ULID: ulid.MustParse(filepath.Base(compacted))})
		m.Compaction.Parents = append(m.Compaction.Parents, BlockDesc{ULID: ulid.MustParse(filepath.Base(compacted))})
		_, err = writeMetaFile(promslog.New(&promslog.Config{}), dir, m)
		require.NoError(t, err)
	}
	tmpCheckpointDir := path.Join(tmpDir, "wal/checkpoint.00000001.tmp")
	err := os.MkdirAll(tmpCheckpointDir, 0o777)
	require.NoError(t, err)
	tmpChunkSnapshotDir := path.Join(tmpDir, chunkSnapshotPrefix+"0000.00000001.tmp")
	err = os.MkdirAll(tmpChunkSnapshotDir, 0o777)
	require.NoError(t, err)

	opts := DefaultOptions()
	opts.RetentionDuration = 0
	db := newTestDB(t, withDir(tmpDir), withOpts(opts))
	loadedBlocks := db.Blocks()

	var loaded int
	for _, l := range loadedBlocks {
		_, ok := expectedLoadedDirs[filepath.Join(tmpDir, l.meta.ULID.String())]
		require.True(t, ok, "unexpected block", l.meta.ULID, "was loaded")
		loaded++
	}
	require.Len(t, expectedLoadedDirs, loaded)
	require.NoError(t, db.Close())

	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)

	var ignored int
	for _, f := range files {
		_, ok := expectedRemovedDirs[filepath.Join(tmpDir, f.Name())]
		require.False(t, ok, "expected", filepath.Join(tmpDir, f.Name()), "to be removed, but still exists")
		if _, ok := expectedIgnoredDirs[filepath.Join(tmpDir, f.Name())]; ok {
			ignored++
		}
	}
	require.Len(t, expectedIgnoredDirs, ignored)
	_, err = os.Stat(tmpCheckpointDir)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(tmpChunkSnapshotDir)
	require.True(t, os.IsNotExist(err))
}

func TestOneCheckpointPerCompactCall(t *testing.T) {
	t.Parallel()
	blockRange := int64(1000)
	opts := &Options{
		RetentionDuration: blockRange * 1000,
		NoLockfile:        true,
		MinBlockDuration:  blockRange,
		MaxBlockDuration:  blockRange,
	}

	ctx := context.Background()

	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	// Case 1: Lot's of uncompacted data in Head.

	lbls := labels.FromStrings("foo_d", "choco_bar")
	// Append samples spanning 59 block ranges.
	app := db.Appender(context.Background())
	for i := range int64(60) {
		_, err := app.Append(0, lbls, blockRange*i, rand.Float64())
		require.NoError(t, err)
		_, err = app.Append(0, lbls, (blockRange*i)+blockRange/2, rand.Float64())
		require.NoError(t, err)
		// Rotate the WAL file so that there is >3 files for checkpoint to happen.
		_, err = db.head.wal.NextSegment()
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Check the existing WAL files.
	first, last, err := wlog.Segments(db.head.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 0, first)
	require.Equal(t, 60, last)

	require.Equal(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.checkpointCreationTotal))
	require.NoError(t, db.Compact(ctx))
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.checkpointCreationTotal))

	// As the data spans for 59 blocks, 58 go to disk and 1 remains in Head.
	require.Len(t, db.Blocks(), 58)
	// Though WAL was truncated only once, head should be truncated after each compaction.
	require.Equal(t, 58.0, prom_testutil.ToFloat64(db.head.metrics.headTruncateTotal))

	// The compaction should have only truncated first 2/3 of WAL (while also rotating the files).
	first, last, err = wlog.Segments(db.head.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 40, first)
	require.Equal(t, 61, last)

	// The first checkpoint would be for first 2/3rd of WAL, hence till 39.
	// That should be the last checkpoint.
	_, cno, err := wlog.LastCheckpoint(db.head.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 39, cno)

	// Case 2: Old blocks on disk.
	// The above blocks will act as old blocks.

	// Creating a block to cover the data in the Head so that
	// Head will skip the data during replay and start fresh.
	blocks := db.Blocks()
	newBlockMint := blocks[len(blocks)-1].Meta().MaxTime
	newBlockMaxt := db.Head().MaxTime() + 1
	require.NoError(t, db.Close())

	createBlock(t, db.Dir(), genSeries(1, 1, newBlockMint, newBlockMaxt))

	db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
	db.DisableCompactions()

	// 1 block more.
	require.Len(t, db.Blocks(), 59)
	// No series in Head because of this new block.
	require.Equal(t, 0, int(db.head.NumSeries()))

	// Adding sample way into the future.
	app = db.Appender(context.Background())
	_, err = app.Append(0, lbls, blockRange*120, rand.Float64())
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// The mint of head is the last block maxt, that means the gap between mint and maxt
	// of Head is too large. This will trigger many compactions.
	require.Equal(t, newBlockMaxt, db.head.MinTime())

	// Another WAL file was rotated.
	first, last, err = wlog.Segments(db.head.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 40, first)
	require.Equal(t, 62, last)

	require.Equal(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.checkpointCreationTotal))
	require.NoError(t, db.Compact(ctx))
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.checkpointCreationTotal))

	// No new blocks should be created as there was not data in between the new samples and the blocks.
	require.Len(t, db.Blocks(), 59)

	// The compaction should have only truncated first 2/3 of WAL (while also rotating the files).
	first, last, err = wlog.Segments(db.head.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 55, first)
	require.Equal(t, 63, last)

	// The first checkpoint would be for first 2/3rd of WAL, hence till 54.
	// That should be the last checkpoint.
	_, cno, err = wlog.LastCheckpoint(db.head.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 54, cno)
}

func TestNoPanicOnTSDBOpenError(t *testing.T) {
	tmpdir := t.TempDir()

	// Taking the lock will cause a TSDB startup error.
	l, err := tsdbutil.NewDirLocker(tmpdir, "tsdb", promslog.NewNopLogger(), nil)
	require.NoError(t, err)
	require.NoError(t, l.Lock())

	_, err = Open(tmpdir, nil, nil, DefaultOptions(), nil)
	require.Error(t, err)

	require.NoError(t, l.Release())
}

func TestLockfile(t *testing.T) {
	tsdbutil.TestDirLockerUsage(t, func(t *testing.T, data string, createLock bool) (*tsdbutil.DirLocker, testutil.Closer) {
		opts := DefaultOptions()
		opts.NoLockfile = !createLock

		// Create the DB. This should create lockfile and its metrics.
		db, err := Open(data, nil, nil, opts, nil)
		require.NoError(t, err)

		return db.locker, testutil.NewCallbackCloser(func() {
			require.NoError(t, db.Close())
		})
	})
}

func TestQuerier_ShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t *testing.T) {
	t.Skip("TODO: investigate why process crash in CI")

	const numRuns = 5

	for i := 1; i <= numRuns; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			testQuerierShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t)
		})
	}
}

func testQuerierShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t *testing.T) {
	const (
		numSeries                = 1000
		numStressIterations      = 10000
		minStressAllocationBytes = 128 * 1024
		maxStressAllocationBytes = 512 * 1024
	)

	db := newTestDB(t)

	// Disable compactions so we can control it.
	db.DisableCompactions()

	// Generate the metrics we're going to append.
	metrics := make([]labels.Labels, 0, numSeries)
	for i := range numSeries {
		metrics = append(metrics, labels.FromStrings(labels.MetricName, fmt.Sprintf("test_%d", i)))
	}

	// Push 1 sample every 15s for 2x the block duration period.
	ctx := context.Background()
	interval := int64(15 * time.Second / time.Millisecond)
	ts := int64(0)

	for ; ts < 2*DefaultBlockDuration; ts += interval {
		app := db.Appender(ctx)

		for _, metric := range metrics {
			_, err := app.Append(0, metric, ts, float64(ts))
			require.NoError(t, err)
		}

		require.NoError(t, app.Commit())
	}

	// Compact the TSDB head for the first time. We expect the head chunks file has been cut.
	require.NoError(t, db.Compact(ctx))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(db.Head().metrics.headTruncateTotal))

	// Push more samples for another 1x block duration period.
	for ; ts < 3*DefaultBlockDuration; ts += interval {
		app := db.Appender(ctx)

		for _, metric := range metrics {
			_, err := app.Append(0, metric, ts, float64(ts))
			require.NoError(t, err)
		}

		require.NoError(t, app.Commit())
	}

	// At this point we expect 2 mmap-ed head chunks.

	// Get a querier and make sure it's closed only once the test is over.
	querier, err := db.Querier(0, math.MaxInt64)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, querier.Close())
	}()

	// Query back all series.
	hints := &storage.SelectHints{Start: 0, End: math.MaxInt64, Step: interval}
	seriesSet := querier.Select(ctx, true, hints, labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+"))

	// Fetch samples iterators from all series.
	var iterators []chunkenc.Iterator
	actualSeries := 0
	for seriesSet.Next() {
		actualSeries++

		// Get the iterator and call Next() so that we're sure the chunk is loaded.
		it := seriesSet.At().Iterator(nil)
		it.Next()
		it.At()

		iterators = append(iterators, it)
	}
	require.NoError(t, seriesSet.Err())
	require.Equal(t, numSeries, actualSeries)

	// Compact the TSDB head again.
	require.NoError(t, db.Compact(ctx))
	require.Equal(t, float64(2), prom_testutil.ToFloat64(db.Head().metrics.headTruncateTotal))

	// At this point we expect 1 head chunk has been deleted.

	// Stress the memory and call GC. This is required to increase the chances
	// the chunk memory area is released to the kernel.
	var buf []byte
	for i := range numStressIterations {
		//nolint:staticcheck
		buf = append(buf, make([]byte, minStressAllocationBytes+rand.Int31n(maxStressAllocationBytes-minStressAllocationBytes))...)
		if i%1000 == 0 {
			buf = nil
		}
	}

	// Iterate samples. Here we're summing it just to make sure no golang compiler
	// optimization triggers in case we discard the result of it.At().
	var sum float64
	var firstErr error
	for _, it := range iterators {
		for it.Next() == chunkenc.ValFloat {
			_, v := it.At()
			sum += v
		}

		if err := it.Err(); err != nil {
			firstErr = err
		}
	}

	// After having iterated all samples we also want to be sure no error occurred or
	// the "cannot populate chunk XXX: not found" error occurred. This error can occur
	// when the iterator tries to fetch an head chunk which has been offloaded because
	// of the head compaction in the meanwhile.
	if firstErr != nil {
		require.ErrorContains(t, firstErr, "cannot populate chunk")
	}
}

func TestChunkQuerier_ShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t *testing.T) {
	t.Skip("TODO: investigate why process crash in CI")

	const numRuns = 5

	for i := 1; i <= numRuns; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			testChunkQuerierShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t)
		})
	}
}

func testChunkQuerierShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t *testing.T) {
	const (
		numSeries                = 1000
		numStressIterations      = 10000
		minStressAllocationBytes = 128 * 1024
		maxStressAllocationBytes = 512 * 1024
	)

	db := newTestDB(t)

	// Disable compactions so we can control it.
	db.DisableCompactions()

	// Generate the metrics we're going to append.
	metrics := make([]labels.Labels, 0, numSeries)
	for i := range numSeries {
		metrics = append(metrics, labels.FromStrings(labels.MetricName, fmt.Sprintf("test_%d", i)))
	}

	// Push 1 sample every 15s for 2x the block duration period.
	ctx := context.Background()
	interval := int64(15 * time.Second / time.Millisecond)
	ts := int64(0)

	for ; ts < 2*DefaultBlockDuration; ts += interval {
		app := db.Appender(ctx)

		for _, metric := range metrics {
			_, err := app.Append(0, metric, ts, float64(ts))
			require.NoError(t, err)
		}

		require.NoError(t, app.Commit())
	}

	// Compact the TSDB head for the first time. We expect the head chunks file has been cut.
	require.NoError(t, db.Compact(ctx))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(db.Head().metrics.headTruncateTotal))

	// Push more samples for another 1x block duration period.
	for ; ts < 3*DefaultBlockDuration; ts += interval {
		app := db.Appender(ctx)

		for _, metric := range metrics {
			_, err := app.Append(0, metric, ts, float64(ts))
			require.NoError(t, err)
		}

		require.NoError(t, app.Commit())
	}

	// At this point we expect 2 mmap-ed head chunks.

	// Get a querier and make sure it's closed only once the test is over.
	querier, err := db.ChunkQuerier(0, math.MaxInt64)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, querier.Close())
	}()

	// Query back all series.
	hints := &storage.SelectHints{Start: 0, End: math.MaxInt64, Step: interval}
	seriesSet := querier.Select(ctx, true, hints, labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+"))

	// Iterate all series and get their chunks.
	var it chunks.Iterator
	var chunks []chunkenc.Chunk
	actualSeries := 0
	for seriesSet.Next() {
		actualSeries++
		it = seriesSet.At().Iterator(it)
		for it.Next() {
			chunks = append(chunks, it.At().Chunk)
		}
	}
	require.NoError(t, seriesSet.Err())
	require.Equal(t, numSeries, actualSeries)

	// Compact the TSDB head again.
	require.NoError(t, db.Compact(ctx))
	require.Equal(t, float64(2), prom_testutil.ToFloat64(db.Head().metrics.headTruncateTotal))

	// At this point we expect 1 head chunk has been deleted.

	// Stress the memory and call GC. This is required to increase the chances
	// the chunk memory area is released to the kernel.
	var buf []byte
	for i := range numStressIterations {
		//nolint:staticcheck
		buf = append(buf, make([]byte, minStressAllocationBytes+rand.Int31n(maxStressAllocationBytes-minStressAllocationBytes))...)
		if i%1000 == 0 {
			buf = nil
		}
	}

	// Iterate chunks and read their bytes slice. Here we're computing the CRC32
	// just to iterate through the bytes slice. We don't really care the reason why
	// we read this data, we just need to read it to make sure the memory address
	// of the []byte is still valid.
	chkCRC32 := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	for _, chunk := range chunks {
		chkCRC32.Reset()
		_, err := chkCRC32.Write(chunk.Bytes())
		require.NoError(t, err)
	}
}

func TestQuerierShouldNotFailIfOOOCompactionOccursAfterRetrievingQuerier(t *testing.T) {
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = 3 * DefaultBlockDuration
	db := newTestDB(t, withOpts(opts))

	// Disable compactions so we can control it.
	db.DisableCompactions()

	metric := labels.FromStrings(labels.MetricName, "test_metric")
	ctx := context.Background()
	interval := int64(15 * time.Second / time.Millisecond)
	ts := int64(0)
	samplesWritten := 0

	// Capture the first timestamp - this will be the timestamp of the OOO sample we'll append below.
	oooTS := ts
	ts += interval

	// Push samples after the OOO sample we'll write below.
	for ; ts < 10*interval; ts += interval {
		app := db.Appender(ctx)
		_, err := app.Append(0, metric, ts, float64(ts))
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		samplesWritten++
	}

	// Push a single OOO sample.
	app := db.Appender(ctx)
	_, err := app.Append(0, metric, oooTS, float64(ts))
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	samplesWritten++

	// Get a querier.
	querierCreatedBeforeCompaction, err := db.ChunkQuerier(0, math.MaxInt64)
	require.NoError(t, err)

	// Start OOO head compaction.
	compactionComplete := atomic.NewBool(false)
	go func() {
		defer compactionComplete.Store(true)

		require.NoError(t, db.CompactOOOHead(ctx))
		require.Equal(t, float64(1), prom_testutil.ToFloat64(db.Head().metrics.chunksRemoved))
	}()

	// Give CompactOOOHead time to start work.
	// If it does not wait for querierCreatedBeforeCompaction to be closed, then the query will return incorrect results or fail.
	time.Sleep(time.Second)
	require.False(t, compactionComplete.Load(), "compaction completed before reading chunks or closing querier created before compaction")

	// Get another querier. This one should only use the compacted blocks from disk and ignore the chunks that will be garbage collected.
	querierCreatedAfterCompaction, err := db.ChunkQuerier(0, math.MaxInt64)
	require.NoError(t, err)

	testQuerier := func(q storage.ChunkQuerier) {
		// Query back the series.
		hints := &storage.SelectHints{Start: 0, End: math.MaxInt64, Step: interval}
		seriesSet := q.Select(ctx, true, hints, labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "test_metric"))

		// Collect the iterator for the series.
		var iterators []chunks.Iterator
		for seriesSet.Next() {
			iterators = append(iterators, seriesSet.At().Iterator(nil))
		}
		require.NoError(t, seriesSet.Err())
		require.Len(t, iterators, 1)
		iterator := iterators[0]

		// Check that we can still successfully read all samples.
		samplesRead := 0
		for iterator.Next() {
			samplesRead += iterator.At().Chunk.NumSamples()
		}

		require.NoError(t, iterator.Err())
		require.Equal(t, samplesWritten, samplesRead)
	}

	testQuerier(querierCreatedBeforeCompaction)

	require.False(t, compactionComplete.Load(), "compaction completed before closing querier created before compaction")
	require.NoError(t, querierCreatedBeforeCompaction.Close())
	require.Eventually(t, compactionComplete.Load, time.Second, 10*time.Millisecond, "compaction should complete after querier created before compaction was closed, and not wait for querier created after compaction")

	// Use the querier created after compaction and confirm it returns the expected results (ie. from the disk block created from OOO head and in-order head) without error.
	testQuerier(querierCreatedAfterCompaction)
	require.NoError(t, querierCreatedAfterCompaction.Close())
}

func TestQuerierShouldNotFailIfOOOCompactionOccursAfterSelecting(t *testing.T) {
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = 3 * DefaultBlockDuration
	db := newTestDB(t, withOpts(opts))

	// Disable compactions so we can control it.
	db.DisableCompactions()

	metric := labels.FromStrings(labels.MetricName, "test_metric")
	ctx := context.Background()
	interval := int64(15 * time.Second / time.Millisecond)
	ts := int64(0)
	samplesWritten := 0

	// Capture the first timestamp - this will be the timestamp of the OOO sample we'll append below.
	oooTS := ts
	ts += interval

	// Push samples after the OOO sample we'll write below.
	for ; ts < 10*interval; ts += interval {
		app := db.Appender(ctx)
		_, err := app.Append(0, metric, ts, float64(ts))
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		samplesWritten++
	}

	// Push a single OOO sample.
	app := db.Appender(ctx)
	_, err := app.Append(0, metric, oooTS, float64(ts))
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	samplesWritten++

	// Get a querier.
	querier, err := db.ChunkQuerier(0, math.MaxInt64)
	require.NoError(t, err)

	// Query back the series.
	hints := &storage.SelectHints{Start: 0, End: math.MaxInt64, Step: interval}
	seriesSet := querier.Select(ctx, true, hints, labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "test_metric"))

	// Start OOO head compaction.
	compactionComplete := atomic.NewBool(false)
	go func() {
		defer compactionComplete.Store(true)

		require.NoError(t, db.CompactOOOHead(ctx))
		require.Equal(t, float64(1), prom_testutil.ToFloat64(db.Head().metrics.chunksRemoved))
	}()

	// Give CompactOOOHead time to start work.
	// If it does not wait for the querier to be closed, then the query will return incorrect results or fail.
	time.Sleep(time.Second)
	require.False(t, compactionComplete.Load(), "compaction completed before reading chunks or closing querier")

	// Collect the iterator for the series.
	var iterators []chunks.Iterator
	for seriesSet.Next() {
		iterators = append(iterators, seriesSet.At().Iterator(nil))
	}
	require.NoError(t, seriesSet.Err())
	require.Len(t, iterators, 1)
	iterator := iterators[0]

	// Check that we can still successfully read all samples.
	samplesRead := 0
	for iterator.Next() {
		samplesRead += iterator.At().Chunk.NumSamples()
	}

	require.NoError(t, iterator.Err())
	require.Equal(t, samplesWritten, samplesRead)

	require.False(t, compactionComplete.Load(), "compaction completed before closing querier")
	require.NoError(t, querier.Close())
	require.Eventually(t, compactionComplete.Load, time.Second, 10*time.Millisecond, "compaction should complete after querier was closed")
}

func TestQuerierShouldNotFailIfOOOCompactionOccursAfterRetrievingIterators(t *testing.T) {
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = 3 * DefaultBlockDuration
	db := newTestDB(t, withOpts(opts))

	// Disable compactions so we can control it.
	db.DisableCompactions()

	metric := labels.FromStrings(labels.MetricName, "test_metric")
	ctx := context.Background()
	interval := int64(15 * time.Second / time.Millisecond)
	ts := int64(0)
	samplesWritten := 0

	// Capture the first timestamp - this will be the timestamp of the OOO sample we'll append below.
	oooTS := ts
	ts += interval

	// Push samples after the OOO sample we'll write below.
	for ; ts < 10*interval; ts += interval {
		app := db.Appender(ctx)
		_, err := app.Append(0, metric, ts, float64(ts))
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		samplesWritten++
	}

	// Push a single OOO sample.
	app := db.Appender(ctx)
	_, err := app.Append(0, metric, oooTS, float64(ts))
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	samplesWritten++

	// Get a querier.
	querier, err := db.ChunkQuerier(0, math.MaxInt64)
	require.NoError(t, err)

	// Query back the series.
	hints := &storage.SelectHints{Start: 0, End: math.MaxInt64, Step: interval}
	seriesSet := querier.Select(ctx, true, hints, labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "test_metric"))

	// Collect the iterator for the series.
	var iterators []chunks.Iterator
	for seriesSet.Next() {
		iterators = append(iterators, seriesSet.At().Iterator(nil))
	}
	require.NoError(t, seriesSet.Err())
	require.Len(t, iterators, 1)
	iterator := iterators[0]

	// Start OOO head compaction.
	compactionComplete := atomic.NewBool(false)
	go func() {
		defer compactionComplete.Store(true)

		require.NoError(t, db.CompactOOOHead(ctx))
		require.Equal(t, float64(1), prom_testutil.ToFloat64(db.Head().metrics.chunksRemoved))
	}()

	// Give CompactOOOHead time to start work.
	// If it does not wait for the querier to be closed, then the query will return incorrect results or fail.
	time.Sleep(time.Second)
	require.False(t, compactionComplete.Load(), "compaction completed before reading chunks or closing querier")

	// Check that we can still successfully read all samples.
	samplesRead := 0
	for iterator.Next() {
		samplesRead += iterator.At().Chunk.NumSamples()
	}

	require.NoError(t, iterator.Err())
	require.Equal(t, samplesWritten, samplesRead)

	require.False(t, compactionComplete.Load(), "compaction completed before closing querier")
	require.NoError(t, querier.Close())
	require.Eventually(t, compactionComplete.Load, time.Second, 10*time.Millisecond, "compaction should complete after querier was closed")
}

func TestOOOWALWrite(t *testing.T) {
	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }

	s := labels.NewSymbolTable()
	scratchBuilder1 := labels.NewScratchBuilderWithSymbolTable(s, 1)
	scratchBuilder1.Add("l", "v1")
	s1 := scratchBuilder1.Labels()
	scratchBuilder2 := labels.NewScratchBuilderWithSymbolTable(s, 1)
	scratchBuilder2.Add("l", "v2")
	s2 := scratchBuilder2.Labels()

	scenarios := map[string]struct {
		appendSample       func(app storage.Appender, l labels.Labels, mins int64) (storage.SeriesRef, error)
		expectedOOORecords []any
		expectedInORecords []any
	}{
		"float": {
			appendSample: func(app storage.Appender, l labels.Labels, mins int64) (storage.SeriesRef, error) {
				seriesRef, err := app.Append(0, l, minutes(mins), float64(mins))
				require.NoError(t, err)
				return seriesRef, nil
			},
			expectedOOORecords: []any{
				// The MmapRef in this are not hand calculated, and instead taken from the test run.
				// What is important here is the order of records, and that MmapRef increases for each record.
				[]record.RefMmapMarker{
					{Ref: 1},
				},
				[]record.RefSample{
					{Ref: 1, T: minutes(40), V: 40},
				},

				[]record.RefMmapMarker{
					{Ref: 2},
				},
				[]record.RefSample{
					{Ref: 2, T: minutes(42), V: 42},
				},

				[]record.RefSample{
					{Ref: 2, T: minutes(45), V: 45},
					{Ref: 1, T: minutes(35), V: 35},
				},
				[]record.RefMmapMarker{ // 3rd sample, hence m-mapped.
					{Ref: 1, MmapRef: 0x100000000 + 8},
				},
				[]record.RefSample{
					{Ref: 1, T: minutes(36), V: 36},
					{Ref: 1, T: minutes(37), V: 37},
				},

				[]record.RefMmapMarker{ // 3rd sample, hence m-mapped.
					{Ref: 1, MmapRef: 0x100000000 + 58},
				},
				[]record.RefSample{ // Does not contain the in-order sample here.
					{Ref: 1, T: minutes(50), V: 50},
				},

				// Single commit but multiple OOO records.
				[]record.RefMmapMarker{
					{Ref: 2, MmapRef: 0x100000000 + 107},
				},
				[]record.RefSample{
					{Ref: 2, T: minutes(50), V: 50},
					{Ref: 2, T: minutes(51), V: 51},
				},
				[]record.RefMmapMarker{
					{Ref: 2, MmapRef: 0x100000000 + 156},
				},
				[]record.RefSample{
					{Ref: 2, T: minutes(52), V: 52},
					{Ref: 2, T: minutes(53), V: 53},
				},
			},
			expectedInORecords: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: s1},
					{Ref: 2, Labels: s2},
				},
				[]record.RefSample{
					{Ref: 1, T: minutes(60), V: 60},
					{Ref: 2, T: minutes(60), V: 60},
				},
				[]record.RefSample{
					{Ref: 1, T: minutes(40), V: 40},
				},
				[]record.RefSample{
					{Ref: 2, T: minutes(42), V: 42},
				},
				[]record.RefSample{
					{Ref: 2, T: minutes(45), V: 45},
					{Ref: 1, T: minutes(35), V: 35},
					{Ref: 1, T: minutes(36), V: 36},
					{Ref: 1, T: minutes(37), V: 37},
				},
				[]record.RefSample{ // Contains both in-order and ooo sample.
					{Ref: 1, T: minutes(50), V: 50},
					{Ref: 2, T: minutes(65), V: 65},
				},
				[]record.RefSample{
					{Ref: 2, T: minutes(50), V: 50},
					{Ref: 2, T: minutes(51), V: 51},
					{Ref: 2, T: minutes(52), V: 52},
					{Ref: 2, T: minutes(53), V: 53},
				},
			},
		},
		"integer histogram": {
			appendSample: func(app storage.Appender, l labels.Labels, mins int64) (storage.SeriesRef, error) {
				seriesRef, err := app.AppendHistogram(0, l, minutes(mins), tsdbutil.GenerateTestHistogram(mins), nil)
				require.NoError(t, err)
				return seriesRef, nil
			},
			expectedOOORecords: []any{
				// The MmapRef in this are not hand calculated, and instead taken from the test run.
				// What is important here is the order of records, and that MmapRef increases for each record.
				[]record.RefMmapMarker{
					{Ref: 1},
				},
				[]record.RefHistogramSample{
					{Ref: 1, T: minutes(40), H: tsdbutil.GenerateTestHistogram(40)},
				},

				[]record.RefMmapMarker{
					{Ref: 2},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(42), H: tsdbutil.GenerateTestHistogram(42)},
				},

				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(45), H: tsdbutil.GenerateTestHistogram(45)},
					{Ref: 1, T: minutes(35), H: tsdbutil.GenerateTestHistogram(35)},
				},
				[]record.RefMmapMarker{ // 3rd sample, hence m-mapped.
					{Ref: 1, MmapRef: 0x100000000 + 8},
				},
				[]record.RefHistogramSample{
					{Ref: 1, T: minutes(36), H: tsdbutil.GenerateTestHistogram(36)},
					{Ref: 1, T: minutes(37), H: tsdbutil.GenerateTestHistogram(37)},
				},

				[]record.RefMmapMarker{ // 3rd sample, hence m-mapped.
					{Ref: 1, MmapRef: 0x100000000 + 89},
				},
				[]record.RefHistogramSample{ // Does not contain the in-order sample here.
					{Ref: 1, T: minutes(50), H: tsdbutil.GenerateTestHistogram(50)},
				},

				// Single commit but multiple OOO records.
				[]record.RefMmapMarker{
					{Ref: 2, MmapRef: 0x100000000 + 172},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(50), H: tsdbutil.GenerateTestHistogram(50)},
					{Ref: 2, T: minutes(51), H: tsdbutil.GenerateTestHistogram(51)},
				},
				[]record.RefMmapMarker{
					{Ref: 2, MmapRef: 0x100000000 + 257},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(52), H: tsdbutil.GenerateTestHistogram(52)},
					{Ref: 2, T: minutes(53), H: tsdbutil.GenerateTestHistogram(53)},
				},
			},
			expectedInORecords: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: s1},
					{Ref: 2, Labels: s2},
				},
				[]record.RefHistogramSample{
					{Ref: 1, T: minutes(60), H: tsdbutil.GenerateTestHistogram(60)},
					{Ref: 2, T: minutes(60), H: tsdbutil.GenerateTestHistogram(60)},
				},
				[]record.RefHistogramSample{
					{Ref: 1, T: minutes(40), H: tsdbutil.GenerateTestHistogram(40)},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(42), H: tsdbutil.GenerateTestHistogram(42)},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(45), H: tsdbutil.GenerateTestHistogram(45)},
					{Ref: 1, T: minutes(35), H: tsdbutil.GenerateTestHistogram(35)},
					{Ref: 1, T: minutes(36), H: tsdbutil.GenerateTestHistogram(36)},
					{Ref: 1, T: minutes(37), H: tsdbutil.GenerateTestHistogram(37)},
				},
				[]record.RefHistogramSample{ // Contains both in-order and ooo sample.
					{Ref: 1, T: minutes(50), H: tsdbutil.GenerateTestHistogram(50)},
					{Ref: 2, T: minutes(65), H: tsdbutil.GenerateTestHistogram(65)},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(50), H: tsdbutil.GenerateTestHistogram(50)},
					{Ref: 2, T: minutes(51), H: tsdbutil.GenerateTestHistogram(51)},
					{Ref: 2, T: minutes(52), H: tsdbutil.GenerateTestHistogram(52)},
					{Ref: 2, T: minutes(53), H: tsdbutil.GenerateTestHistogram(53)},
				},
			},
		},
		"float histogram": {
			appendSample: func(app storage.Appender, l labels.Labels, mins int64) (storage.SeriesRef, error) {
				seriesRef, err := app.AppendHistogram(0, l, minutes(mins), nil, tsdbutil.GenerateTestFloatHistogram(mins))
				require.NoError(t, err)
				return seriesRef, nil
			},
			expectedOOORecords: []any{
				// The MmapRef in this are not hand calculated, and instead taken from the test run.
				// What is important here is the order of records, and that MmapRef increases for each record.
				[]record.RefMmapMarker{
					{Ref: 1},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 1, T: minutes(40), FH: tsdbutil.GenerateTestFloatHistogram(40)},
				},

				[]record.RefMmapMarker{
					{Ref: 2},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(42), FH: tsdbutil.GenerateTestFloatHistogram(42)},
				},

				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(45), FH: tsdbutil.GenerateTestFloatHistogram(45)},
					{Ref: 1, T: minutes(35), FH: tsdbutil.GenerateTestFloatHistogram(35)},
				},
				[]record.RefMmapMarker{ // 3rd sample, hence m-mapped.
					{Ref: 1, MmapRef: 0x100000000 + 8},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 1, T: minutes(36), FH: tsdbutil.GenerateTestFloatHistogram(36)},
					{Ref: 1, T: minutes(37), FH: tsdbutil.GenerateTestFloatHistogram(37)},
				},

				[]record.RefMmapMarker{ // 3rd sample, hence m-mapped.
					{Ref: 1, MmapRef: 0x100000000 + 177},
				},
				[]record.RefFloatHistogramSample{ // Does not contain the in-order sample here.
					{Ref: 1, T: minutes(50), FH: tsdbutil.GenerateTestFloatHistogram(50)},
				},

				// Single commit but multiple OOO records.
				[]record.RefMmapMarker{
					{Ref: 2, MmapRef: 0x100000000 + 348},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(50), FH: tsdbutil.GenerateTestFloatHistogram(50)},
					{Ref: 2, T: minutes(51), FH: tsdbutil.GenerateTestFloatHistogram(51)},
				},
				[]record.RefMmapMarker{
					{Ref: 2, MmapRef: 0x100000000 + 521},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(52), FH: tsdbutil.GenerateTestFloatHistogram(52)},
					{Ref: 2, T: minutes(53), FH: tsdbutil.GenerateTestFloatHistogram(53)},
				},
			},
			expectedInORecords: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: s1},
					{Ref: 2, Labels: s2},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 1, T: minutes(60), FH: tsdbutil.GenerateTestFloatHistogram(60)},
					{Ref: 2, T: minutes(60), FH: tsdbutil.GenerateTestFloatHistogram(60)},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 1, T: minutes(40), FH: tsdbutil.GenerateTestFloatHistogram(40)},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(42), FH: tsdbutil.GenerateTestFloatHistogram(42)},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(45), FH: tsdbutil.GenerateTestFloatHistogram(45)},
					{Ref: 1, T: minutes(35), FH: tsdbutil.GenerateTestFloatHistogram(35)},
					{Ref: 1, T: minutes(36), FH: tsdbutil.GenerateTestFloatHistogram(36)},
					{Ref: 1, T: minutes(37), FH: tsdbutil.GenerateTestFloatHistogram(37)},
				},
				[]record.RefFloatHistogramSample{ // Contains both in-order and ooo sample.
					{Ref: 1, T: minutes(50), FH: tsdbutil.GenerateTestFloatHistogram(50)},
					{Ref: 2, T: minutes(65), FH: tsdbutil.GenerateTestFloatHistogram(65)},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(50), FH: tsdbutil.GenerateTestFloatHistogram(50)},
					{Ref: 2, T: minutes(51), FH: tsdbutil.GenerateTestFloatHistogram(51)},
					{Ref: 2, T: minutes(52), FH: tsdbutil.GenerateTestFloatHistogram(52)},
					{Ref: 2, T: minutes(53), FH: tsdbutil.GenerateTestFloatHistogram(53)},
				},
			},
		},
		"custom buckets histogram": {
			appendSample: func(app storage.Appender, l labels.Labels, mins int64) (storage.SeriesRef, error) {
				seriesRef, err := app.AppendHistogram(0, l, minutes(mins), tsdbutil.GenerateTestCustomBucketsHistogram(mins), nil)
				require.NoError(t, err)
				return seriesRef, nil
			},
			expectedOOORecords: []any{
				// The MmapRef in this are not hand calculated, and instead taken from the test run.
				// What is important here is the order of records, and that MmapRef increases for each record.
				[]record.RefMmapMarker{
					{Ref: 1},
				},
				[]record.RefHistogramSample{
					{Ref: 1, T: minutes(40), H: tsdbutil.GenerateTestCustomBucketsHistogram(40)},
				},

				[]record.RefMmapMarker{
					{Ref: 2},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(42), H: tsdbutil.GenerateTestCustomBucketsHistogram(42)},
				},

				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(45), H: tsdbutil.GenerateTestCustomBucketsHistogram(45)},
					{Ref: 1, T: minutes(35), H: tsdbutil.GenerateTestCustomBucketsHistogram(35)},
				},
				[]record.RefMmapMarker{ // 3rd sample, hence m-mapped.
					{Ref: 1, MmapRef: 0x100000000 + 8},
				},
				[]record.RefHistogramSample{
					{Ref: 1, T: minutes(36), H: tsdbutil.GenerateTestCustomBucketsHistogram(36)},
					{Ref: 1, T: minutes(37), H: tsdbutil.GenerateTestCustomBucketsHistogram(37)},
				},

				[]record.RefMmapMarker{ // 3rd sample, hence m-mapped.
					{Ref: 1, MmapRef: 0x100000000 + 82},
				},
				[]record.RefHistogramSample{ // Does not contain the in-order sample here.
					{Ref: 1, T: minutes(50), H: tsdbutil.GenerateTestCustomBucketsHistogram(50)},
				},

				// Single commit but multiple OOO records.
				[]record.RefMmapMarker{
					{Ref: 2, MmapRef: 0x100000000 + 160},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(50), H: tsdbutil.GenerateTestCustomBucketsHistogram(50)},
					{Ref: 2, T: minutes(51), H: tsdbutil.GenerateTestCustomBucketsHistogram(51)},
				},
				[]record.RefMmapMarker{
					{Ref: 2, MmapRef: 0x100000000 + 239},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(52), H: tsdbutil.GenerateTestCustomBucketsHistogram(52)},
					{Ref: 2, T: minutes(53), H: tsdbutil.GenerateTestCustomBucketsHistogram(53)},
				},
			},
			expectedInORecords: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: s1},
					{Ref: 2, Labels: s2},
				},
				[]record.RefHistogramSample{
					{Ref: 1, T: minutes(60), H: tsdbutil.GenerateTestCustomBucketsHistogram(60)},
					{Ref: 2, T: minutes(60), H: tsdbutil.GenerateTestCustomBucketsHistogram(60)},
				},
				[]record.RefHistogramSample{
					{Ref: 1, T: minutes(40), H: tsdbutil.GenerateTestCustomBucketsHistogram(40)},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(42), H: tsdbutil.GenerateTestCustomBucketsHistogram(42)},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(45), H: tsdbutil.GenerateTestCustomBucketsHistogram(45)},
					{Ref: 1, T: minutes(35), H: tsdbutil.GenerateTestCustomBucketsHistogram(35)},
					{Ref: 1, T: minutes(36), H: tsdbutil.GenerateTestCustomBucketsHistogram(36)},
					{Ref: 1, T: minutes(37), H: tsdbutil.GenerateTestCustomBucketsHistogram(37)},
				},
				[]record.RefHistogramSample{ // Contains both in-order and ooo sample.
					{Ref: 1, T: minutes(50), H: tsdbutil.GenerateTestCustomBucketsHistogram(50)},
					{Ref: 2, T: minutes(65), H: tsdbutil.GenerateTestCustomBucketsHistogram(65)},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: minutes(50), H: tsdbutil.GenerateTestCustomBucketsHistogram(50)},
					{Ref: 2, T: minutes(51), H: tsdbutil.GenerateTestCustomBucketsHistogram(51)},
					{Ref: 2, T: minutes(52), H: tsdbutil.GenerateTestCustomBucketsHistogram(52)},
					{Ref: 2, T: minutes(53), H: tsdbutil.GenerateTestCustomBucketsHistogram(53)},
				},
			},
		},
		"custom buckets float histogram": {
			appendSample: func(app storage.Appender, l labels.Labels, mins int64) (storage.SeriesRef, error) {
				seriesRef, err := app.AppendHistogram(0, l, minutes(mins), nil, tsdbutil.GenerateTestCustomBucketsFloatHistogram(mins))
				require.NoError(t, err)
				return seriesRef, nil
			},
			expectedOOORecords: []any{
				// The MmapRef in this are not hand calculated, and instead taken from the test run.
				// What is important here is the order of records, and that MmapRef increases for each record.
				[]record.RefMmapMarker{
					{Ref: 1},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 1, T: minutes(40), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(40)},
				},

				[]record.RefMmapMarker{
					{Ref: 2},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(42), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(42)},
				},

				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(45), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(45)},
					{Ref: 1, T: minutes(35), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(35)},
				},
				[]record.RefMmapMarker{ // 3rd sample, hence m-mapped.
					{Ref: 1, MmapRef: 0x100000000 + 8},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 1, T: minutes(36), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(36)},
					{Ref: 1, T: minutes(37), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(37)},
				},

				[]record.RefMmapMarker{ // 3rd sample, hence m-mapped.
					{Ref: 1, MmapRef: 0x100000000 + 134},
				},
				[]record.RefFloatHistogramSample{ // Does not contain the in-order sample here.
					{Ref: 1, T: minutes(50), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(50)},
				},

				// Single commit but multiple OOO records.
				[]record.RefMmapMarker{
					{Ref: 2, MmapRef: 0x100000000 + 263},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(50), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(50)},
					{Ref: 2, T: minutes(51), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(51)},
				},
				[]record.RefMmapMarker{
					{Ref: 2, MmapRef: 0x100000000 + 393},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(52), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(52)},
					{Ref: 2, T: minutes(53), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(53)},
				},
			},
			expectedInORecords: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: s1},
					{Ref: 2, Labels: s2},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 1, T: minutes(60), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(60)},
					{Ref: 2, T: minutes(60), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(60)},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 1, T: minutes(40), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(40)},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(42), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(42)},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(45), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(45)},
					{Ref: 1, T: minutes(35), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(35)},
					{Ref: 1, T: minutes(36), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(36)},
					{Ref: 1, T: minutes(37), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(37)},
				},
				[]record.RefFloatHistogramSample{ // Contains both in-order and ooo sample.
					{Ref: 1, T: minutes(50), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(50)},
					{Ref: 2, T: minutes(65), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(65)},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: minutes(50), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(50)},
					{Ref: 2, T: minutes(51), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(51)},
					{Ref: 2, T: minutes(52), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(52)},
					{Ref: 2, T: minutes(53), FH: tsdbutil.GenerateTestCustomBucketsFloatHistogram(53)},
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			testOOOWALWrite(t, scenario.appendSample, scenario.expectedOOORecords, scenario.expectedInORecords)
		})
	}
}

func testOOOWALWrite(t *testing.T,
	appendSample func(app storage.Appender, l labels.Labels, mins int64) (storage.SeriesRef, error),
	expectedOOORecords []any,
	expectedInORecords []any,
) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 2
	opts.OutOfOrderTimeWindow = 30 * time.Minute.Milliseconds()
	db := newTestDB(t, withOpts(opts))

	s1, s2 := labels.FromStrings("l", "v1"), labels.FromStrings("l", "v2")

	// Ingest sample at 1h.
	app := db.Appender(context.Background())
	appendSample(app, s1, 60)
	appendSample(app, s2, 60)
	require.NoError(t, app.Commit())

	// OOO for s1.
	app = db.Appender(context.Background())
	appendSample(app, s1, 40)
	require.NoError(t, app.Commit())

	// OOO for s2.
	app = db.Appender(context.Background())
	appendSample(app, s2, 42)
	require.NoError(t, app.Commit())

	// OOO for both s1 and s2 in the same commit.
	app = db.Appender(context.Background())
	appendSample(app, s2, 45)
	appendSample(app, s1, 35)
	appendSample(app, s1, 36) // m-maps.
	appendSample(app, s1, 37)
	require.NoError(t, app.Commit())

	// OOO for s1 but not for s2 in the same commit.
	app = db.Appender(context.Background())
	appendSample(app, s1, 50) // m-maps.
	appendSample(app, s2, 65)
	require.NoError(t, app.Commit())

	// Single commit has 2 times m-mapping and more samples after m-map.
	app = db.Appender(context.Background())
	appendSample(app, s2, 50) // m-maps.
	appendSample(app, s2, 51)
	appendSample(app, s2, 52) // m-maps.
	appendSample(app, s2, 53)
	require.NoError(t, app.Commit())

	getRecords := func(walDir string) []any {
		sr, err := wlog.NewSegmentsReader(walDir)
		require.NoError(t, err)
		r := wlog.NewReader(sr)
		defer func() {
			require.NoError(t, sr.Close())
		}()

		var records []any
		dec := record.NewDecoder(nil, promslog.NewNopLogger())
		for r.Next() {
			rec := r.Record()
			switch typ := dec.Type(rec); typ {
			case record.Series:
				series, err := dec.Series(rec, nil)
				require.NoError(t, err)
				records = append(records, series)
			case record.Samples:
				samples, err := dec.Samples(rec, nil)
				require.NoError(t, err)
				records = append(records, samples)
			case record.MmapMarkers:
				markers, err := dec.MmapMarkers(rec, nil)
				require.NoError(t, err)
				records = append(records, markers)
			case record.HistogramSamples, record.CustomBucketsHistogramSamples:
				histogramSamples, err := dec.HistogramSamples(rec, nil)
				require.NoError(t, err)
				records = append(records, histogramSamples)
			case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples:
				floatHistogramSamples, err := dec.FloatHistogramSamples(rec, nil)
				require.NoError(t, err)
				records = append(records, floatHistogramSamples)
			default:
				t.Fatalf("got a WAL record that is not series or samples: %v", typ)
			}
		}

		return records
	}

	// The normal WAL.
	actRecs := getRecords(path.Join(db.Dir(), "wal"))
	require.Equal(t, expectedInORecords, actRecs)

	// The WBL.
	actRecs = getRecords(path.Join(db.Dir(), wlog.WblDirName))
	require.Equal(t, expectedOOORecords, actRecs)
}

// Tests https://github.com/prometheus/prometheus/issues/10291#issuecomment-1044373110.
func TestDBPanicOnMmappingHeadChunk(t *testing.T) {
	var err error
	ctx := context.Background()

	db := newTestDB(t)
	db.DisableCompactions()

	// Choosing scrape interval of 45s to have chunk larger than 1h.
	itvl := int64(45 * time.Second / time.Millisecond)

	lastTs := int64(0)
	addSamples := func(numSamples int) {
		app := db.Appender(context.Background())
		var ref storage.SeriesRef
		lbls := labels.FromStrings("__name__", "testing", "foo", "bar")
		for i := range numSamples {
			ref, err = app.Append(ref, lbls, lastTs, float64(lastTs))
			require.NoError(t, err)
			lastTs += itvl
			if i%10 == 0 {
				require.NoError(t, app.Commit())
				app = db.Appender(context.Background())
			}
		}
		require.NoError(t, app.Commit())
	}

	// Ingest samples upto 2h50m to make the head "about to compact".
	numSamples := int(170*time.Minute/time.Millisecond) / int(itvl)
	addSamples(numSamples)

	require.Empty(t, db.Blocks())
	require.NoError(t, db.Compact(ctx))
	require.Empty(t, db.Blocks())

	// Restarting.
	require.NoError(t, db.Close())

	db = newTestDB(t, withDir(db.Dir()))
	db.DisableCompactions()

	// Ingest samples upto 20m more to make the head compact.
	numSamples = int(20*time.Minute/time.Millisecond) / int(itvl)
	addSamples(numSamples)

	require.Empty(t, db.Blocks())
	require.NoError(t, db.Compact(ctx))
	require.Len(t, db.Blocks(), 1)

	// More samples to m-map and panic.
	numSamples = int(120*time.Minute/time.Millisecond) / int(itvl)
	addSamples(numSamples)

	require.NoError(t, db.Close())
}

func TestMetadataInWAL(t *testing.T) {
	updateMetadata := func(t *testing.T, app storage.Appender, s labels.Labels, m metadata.Metadata) {
		_, err := app.UpdateMetadata(0, s, m)
		require.NoError(t, err)
	}

	db := newTestDB(t)
	ctx := context.Background()

	// Add some series so we can append metadata to them.
	app := db.Appender(ctx)
	s1 := labels.FromStrings("a", "b")
	s2 := labels.FromStrings("c", "d")
	s3 := labels.FromStrings("e", "f")
	s4 := labels.FromStrings("g", "h")

	for _, s := range []labels.Labels{s1, s2, s3, s4} {
		_, err := app.Append(0, s, 0, 0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Add a first round of metadata to the first three series.
	// Re-take the Appender, as the previous Commit will have it closed.
	m1 := metadata.Metadata{Type: "gauge", Unit: "unit_1", Help: "help_1"}
	m2 := metadata.Metadata{Type: "gauge", Unit: "unit_2", Help: "help_2"}
	m3 := metadata.Metadata{Type: "gauge", Unit: "unit_3", Help: "help_3"}
	app = db.Appender(ctx)
	updateMetadata(t, app, s1, m1)
	updateMetadata(t, app, s2, m2)
	updateMetadata(t, app, s3, m3)
	require.NoError(t, app.Commit())

	// Add a replicated metadata entry to the first series,
	// a completely new metadata entry for the fourth series,
	// and a changed metadata entry to the second series.
	m4 := metadata.Metadata{Type: "counter", Unit: "unit_4", Help: "help_4"}
	m5 := metadata.Metadata{Type: "counter", Unit: "unit_5", Help: "help_5"}
	app = db.Appender(ctx)
	updateMetadata(t, app, s1, m1)
	updateMetadata(t, app, s4, m4)
	updateMetadata(t, app, s2, m5)
	require.NoError(t, app.Commit())

	// Read the WAL to see if the disk storage format is correct.
	recs := readTestWAL(t, path.Join(db.Dir(), "wal"))
	var gotMetadataBlocks [][]record.RefMetadata
	for _, rec := range recs {
		if mr, ok := rec.([]record.RefMetadata); ok {
			gotMetadataBlocks = append(gotMetadataBlocks, mr)
		}
	}

	expectedMetadata := []record.RefMetadata{
		{Ref: 1, Type: record.GetMetricType(m1.Type), Unit: m1.Unit, Help: m1.Help},
		{Ref: 2, Type: record.GetMetricType(m2.Type), Unit: m2.Unit, Help: m2.Help},
		{Ref: 3, Type: record.GetMetricType(m3.Type), Unit: m3.Unit, Help: m3.Help},
		{Ref: 4, Type: record.GetMetricType(m4.Type), Unit: m4.Unit, Help: m4.Help},
		{Ref: 2, Type: record.GetMetricType(m5.Type), Unit: m5.Unit, Help: m5.Help},
	}
	require.Len(t, gotMetadataBlocks, 2)
	require.Equal(t, expectedMetadata[:3], gotMetadataBlocks[0])
	require.Equal(t, expectedMetadata[3:], gotMetadataBlocks[1])
}

func TestMetadataCheckpointingOnlyKeepsLatestEntry(t *testing.T) {
	updateMetadata := func(t *testing.T, app storage.Appender, s labels.Labels, m metadata.Metadata) {
		_, err := app.UpdateMetadata(0, s, m)
		require.NoError(t, err)
	}

	ctx := context.Background()
	numSamples := 10000
	hb, w := newTestHead(t, int64(numSamples)*10, compression.None, false)

	// Add some series so we can append metadata to them.
	app := hb.Appender(ctx)
	s1 := labels.FromStrings("a", "b")
	s2 := labels.FromStrings("c", "d")
	s3 := labels.FromStrings("e", "f")
	s4 := labels.FromStrings("g", "h")

	for _, s := range []labels.Labels{s1, s2, s3, s4} {
		_, err := app.Append(0, s, 0, 0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Add a first round of metadata to the first three series.
	// Re-take the Appender, as the previous Commit will have it closed.
	m1 := metadata.Metadata{Type: "gauge", Unit: "unit_1", Help: "help_1"}
	m2 := metadata.Metadata{Type: "gauge", Unit: "unit_2", Help: "help_2"}
	m3 := metadata.Metadata{Type: "gauge", Unit: "unit_3", Help: "help_3"}
	m4 := metadata.Metadata{Type: "gauge", Unit: "unit_4", Help: "help_4"}
	app = hb.Appender(ctx)
	updateMetadata(t, app, s1, m1)
	updateMetadata(t, app, s2, m2)
	updateMetadata(t, app, s3, m3)
	updateMetadata(t, app, s4, m4)
	require.NoError(t, app.Commit())

	// Update metadata for first series.
	m5 := metadata.Metadata{Type: "counter", Unit: "unit_5", Help: "help_5"}
	app = hb.Appender(ctx)
	updateMetadata(t, app, s1, m5)
	require.NoError(t, app.Commit())

	// Switch back-and-forth metadata for second series.
	// Since it ended on a new metadata record, we expect a single new entry.
	m6 := metadata.Metadata{Type: "counter", Unit: "unit_6", Help: "help_6"}

	app = hb.Appender(ctx)
	updateMetadata(t, app, s2, m6)
	require.NoError(t, app.Commit())

	app = hb.Appender(ctx)
	updateMetadata(t, app, s2, m2)
	require.NoError(t, app.Commit())

	app = hb.Appender(ctx)
	updateMetadata(t, app, s2, m6)
	require.NoError(t, app.Commit())

	app = hb.Appender(ctx)
	updateMetadata(t, app, s2, m2)
	require.NoError(t, app.Commit())

	app = hb.Appender(ctx)
	updateMetadata(t, app, s2, m6)
	require.NoError(t, app.Commit())

	// Let's create a checkpoint.
	first, last, err := wlog.Segments(w.Dir())
	require.NoError(t, err)
	keep := func(id chunks.HeadSeriesRef) bool {
		return id != 3
	}
	_, err = wlog.Checkpoint(promslog.NewNopLogger(), w, first, last-1, keep, 0)
	require.NoError(t, err)

	// Confirm there's been a checkpoint.
	cdir, _, err := wlog.LastCheckpoint(w.Dir())
	require.NoError(t, err)

	// Read in checkpoint and WAL.
	recs := readTestWAL(t, cdir)
	var gotMetadataBlocks [][]record.RefMetadata
	for _, rec := range recs {
		if mr, ok := rec.([]record.RefMetadata); ok {
			gotMetadataBlocks = append(gotMetadataBlocks, mr)
		}
	}

	// There should only be 1 metadata block present, with only the latest
	// metadata kept around.
	wantMetadata := []record.RefMetadata{
		{Ref: 1, Type: record.GetMetricType(m5.Type), Unit: m5.Unit, Help: m5.Help},
		{Ref: 2, Type: record.GetMetricType(m6.Type), Unit: m6.Unit, Help: m6.Help},
		{Ref: 4, Type: record.GetMetricType(m4.Type), Unit: m4.Unit, Help: m4.Help},
	}
	require.Len(t, gotMetadataBlocks, 1)
	require.Len(t, gotMetadataBlocks[0], 3)
	gotMetadataBlock := gotMetadataBlocks[0]

	sort.Slice(gotMetadataBlock, func(i, j int) bool { return gotMetadataBlock[i].Ref < gotMetadataBlock[j].Ref })
	require.Equal(t, wantMetadata, gotMetadataBlock)
	require.NoError(t, hb.Close())
}

func TestMetadataAssertInMemoryData(t *testing.T) {
	updateMetadata := func(t *testing.T, app storage.Appender, s labels.Labels, m metadata.Metadata) {
		_, err := app.UpdateMetadata(0, s, m)
		require.NoError(t, err)
	}

	db := newTestDB(t)
	ctx := context.Background()

	// Add some series so we can append metadata to them.
	app := db.Appender(ctx)
	s1 := labels.FromStrings("a", "b")
	s2 := labels.FromStrings("c", "d")
	s3 := labels.FromStrings("e", "f")
	s4 := labels.FromStrings("g", "h")

	for _, s := range []labels.Labels{s1, s2, s3, s4} {
		_, err := app.Append(0, s, 0, 0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Add a first round of metadata to the first three series.
	// The in-memory data held in the db Head should hold the metadata.
	m1 := metadata.Metadata{Type: "gauge", Unit: "unit_1", Help: "help_1"}
	m2 := metadata.Metadata{Type: "gauge", Unit: "unit_2", Help: "help_2"}
	m3 := metadata.Metadata{Type: "gauge", Unit: "unit_3", Help: "help_3"}
	app = db.Appender(ctx)
	updateMetadata(t, app, s1, m1)
	updateMetadata(t, app, s2, m2)
	updateMetadata(t, app, s3, m3)
	require.NoError(t, app.Commit())

	series1 := db.head.series.getByHash(s1.Hash(), s1)
	series2 := db.head.series.getByHash(s2.Hash(), s2)
	series3 := db.head.series.getByHash(s3.Hash(), s3)
	series4 := db.head.series.getByHash(s4.Hash(), s4)
	require.Equal(t, *series1.meta, m1)
	require.Equal(t, *series2.meta, m2)
	require.Equal(t, *series3.meta, m3)
	require.Nil(t, series4.meta)

	// Add a replicated metadata entry to the first series,
	// a changed metadata entry to the second series,
	// and a completely new metadata entry for the fourth series.
	// The in-memory data held in the db Head should be correctly updated.
	m4 := metadata.Metadata{Type: "counter", Unit: "unit_4", Help: "help_4"}
	m5 := metadata.Metadata{Type: "counter", Unit: "unit_5", Help: "help_5"}
	app = db.Appender(ctx)
	updateMetadata(t, app, s1, m1)
	updateMetadata(t, app, s4, m4)
	updateMetadata(t, app, s2, m5)
	require.NoError(t, app.Commit())

	series1 = db.head.series.getByHash(s1.Hash(), s1)
	series2 = db.head.series.getByHash(s2.Hash(), s2)
	series3 = db.head.series.getByHash(s3.Hash(), s3)
	series4 = db.head.series.getByHash(s4.Hash(), s4)
	require.Equal(t, *series1.meta, m1)
	require.Equal(t, *series2.meta, m5)
	require.Equal(t, *series3.meta, m3)
	require.Equal(t, *series4.meta, m4)

	require.NoError(t, db.Close())

	// Reopen the DB, replaying the WAL. The Head must have been replayed
	// correctly in memory.
	db = newTestDB(t, withDir(db.Dir()))
	_, err := db.head.wal.Size()
	require.NoError(t, err)

	require.Equal(t, *db.head.series.getByHash(s1.Hash(), s1).meta, m1)
	require.Equal(t, *db.head.series.getByHash(s2.Hash(), s2).meta, m5)
	require.Equal(t, *db.head.series.getByHash(s3.Hash(), s3).meta, m3)
	require.Equal(t, *db.head.series.getByHash(s4.Hash(), s4).meta, m4)
}

// TestMultipleEncodingsCommitOrder mainly serves to demonstrate when happens when committing a batch of samples for the
// same series when there are multiple encodings. With issue #15177 fixed, this now all works as expected.
func TestMultipleEncodingsCommitOrder(t *testing.T) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 24 * time.Hour.Milliseconds()

	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	series1 := labels.FromStrings("foo", "bar1")
	addSample := func(app storage.Appender, ts int64, valType chunkenc.ValueType) chunks.Sample {
		if valType == chunkenc.ValFloat {
			_, err := app.Append(0, labels.FromStrings("foo", "bar1"), ts, float64(ts))
			require.NoError(t, err)
			return sample{t: ts, f: float64(ts)}
		}
		if valType == chunkenc.ValHistogram {
			h := tsdbutil.GenerateTestHistogram(ts)
			_, err := app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, h, nil)
			require.NoError(t, err)
			return sample{t: ts, h: h}
		}
		fh := tsdbutil.GenerateTestFloatHistogram(ts)
		_, err := app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, nil, fh)
		require.NoError(t, err)
		return sample{t: ts, fh: fh}
	}

	verifySamples := func(minT, maxT int64, expSamples []chunks.Sample, oooCount int) {
		requireEqualOOOSamples(t, oooCount, db)

		// Verify samples querier.
		querier, err := db.Querier(minT, maxT)
		require.NoError(t, err)
		defer querier.Close()

		seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar1"))
		require.Len(t, seriesSet, 1)
		gotSamples := seriesSet[series1.String()]
		requireEqualSamples(t, series1.String(), expSamples, gotSamples, requireEqualSamplesIgnoreCounterResets)

		// Verify chunks querier.
		chunkQuerier, err := db.ChunkQuerier(minT, maxT)
		require.NoError(t, err)
		defer chunkQuerier.Close()

		chks := queryChunks(t, chunkQuerier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar1"))
		require.NotNil(t, chks[series1.String()])
		require.Len(t, chks, 1)
		var gotChunkSamples []chunks.Sample
		for _, chunk := range chks[series1.String()] {
			it := chunk.Chunk.Iterator(nil)
			smpls, err := storage.ExpandSamples(it, newSample)
			require.NoError(t, err)
			gotChunkSamples = append(gotChunkSamples, smpls...)
			require.NoError(t, it.Err())
		}
		requireEqualSamples(t, series1.String(), expSamples, gotChunkSamples, requireEqualSamplesIgnoreCounterResets)
	}

	var expSamples []chunks.Sample

	// Append samples with different encoding types and then commit them at once.
	app := db.Appender(context.Background())

	for i := 100; i < 105; i++ {
		s := addSample(app, int64(i), chunkenc.ValFloat)
		expSamples = append(expSamples, s)
	}
	for i := 110; i < 120; i++ {
		s := addSample(app, int64(i), chunkenc.ValHistogram)
		expSamples = append(expSamples, s)
	}
	for i := 120; i < 130; i++ {
		s := addSample(app, int64(i), chunkenc.ValFloatHistogram)
		expSamples = append(expSamples, s)
	}
	for i := 140; i < 150; i++ {
		s := addSample(app, int64(i), chunkenc.ValFloatHistogram)
		expSamples = append(expSamples, s)
	}
	// These samples will be marked as out-of-order.
	for i := 130; i < 135; i++ {
		s := addSample(app, int64(i), chunkenc.ValFloat)
		expSamples = append(expSamples, s)
	}

	require.NoError(t, app.Commit())

	sort.Slice(expSamples, func(i, j int) bool {
		return expSamples[i].T() < expSamples[j].T()
	})

	// oooCount = 5 for the samples 130 to 134.
	verifySamples(100, 150, expSamples, 5)

	// Append and commit some in-order histograms by themselves.
	app = db.Appender(context.Background())
	for i := 150; i < 160; i++ {
		s := addSample(app, int64(i), chunkenc.ValHistogram)
		expSamples = append(expSamples, s)
	}
	require.NoError(t, app.Commit())

	// oooCount remains at 5.
	verifySamples(100, 160, expSamples, 5)

	// Append and commit samples for all encoding types. This time all samples will be treated as OOO because samples
	// with newer timestamps have already been committed.
	app = db.Appender(context.Background())
	for i := 50; i < 55; i++ {
		s := addSample(app, int64(i), chunkenc.ValFloat)
		expSamples = append(expSamples, s)
	}
	for i := 60; i < 70; i++ {
		s := addSample(app, int64(i), chunkenc.ValHistogram)
		expSamples = append(expSamples, s)
	}
	for i := 70; i < 75; i++ {
		s := addSample(app, int64(i), chunkenc.ValFloat)
		expSamples = append(expSamples, s)
	}
	for i := 80; i < 90; i++ {
		s := addSample(app, int64(i), chunkenc.ValFloatHistogram)
		expSamples = append(expSamples, s)
	}
	require.NoError(t, app.Commit())

	// Sort samples again because OOO samples have been added.
	sort.Slice(expSamples, func(i, j int) bool {
		return expSamples[i].T() < expSamples[j].T()
	})

	// oooCount = 35 as we've added 30 more OOO samples.
	verifySamples(50, 160, expSamples, 35)
}

// TODO(codesome): test more samples incoming once compaction has started. To verify new samples after the start
//
//	are not included in this compaction.
func TestOOOCompaction(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOCompaction(t, scenario, false)
		})
		t.Run(name+"+extra", func(t *testing.T) {
			testOOOCompaction(t, scenario, true)
		})
	}
}

func testOOOCompaction(t *testing.T, scenario sampleTypeScenario, addExtraSamples bool) {
	ctx := context.Background()

	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions() // We want to manually call it.

	series1 := labels.FromStrings("foo", "bar1")
	series2 := labels.FromStrings("foo", "bar2")

	addSample := func(fromMins, toMins int64) {
		app := db.Appender(context.Background())
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			_, _, err := scenario.appendFunc(app, series1, ts, ts)
			require.NoError(t, err)
			_, _, err = scenario.appendFunc(app, series2, ts, 2*ts)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Add an in-order samples.
	addSample(250, 300)

	// Verify that the in-memory ooo chunk is empty.
	checkEmptyOOOChunk := func(lbls labels.Labels) {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.Nil(t, ms.ooo)
	}
	checkEmptyOOOChunk(series1)
	checkEmptyOOOChunk(series2)

	// Add ooo samples that creates multiple chunks.
	// 90 to 300 spans across 3 block ranges: [0, 120), [120, 240), [240, 360)
	addSample(90, 300)
	// Adding same samples to create overlapping chunks.
	// Since the active chunk won't start at 90 again, all the new
	// chunks will have different time ranges than the previous chunks.
	addSample(90, 300)

	var highest int64 = 300

	verifyDBSamples := func() {
		var series1Samples, series2Samples []chunks.Sample
		for _, r := range [][2]int64{{90, 119}, {120, 239}, {240, highest}} {
			fromMins, toMins := r[0], r[1]
			for m := fromMins; m <= toMins; m++ {
				ts := m * time.Minute.Milliseconds()
				series1Samples = append(series1Samples, scenario.sampleFunc(ts, ts))
				series2Samples = append(series2Samples, scenario.sampleFunc(ts, 2*ts))
			}
		}
		expRes := map[string][]chunks.Sample{
			series1.String(): series1Samples,
			series2.String(): series2Samples,
		}

		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		requireEqualSeries(t, expRes, actRes, true)
	}

	verifyDBSamples() // Before any compaction.

	// Verify that the in-memory ooo chunk is not empty.
	checkNonEmptyOOOChunk := func(lbls labels.Labels) {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.Positive(t, ms.ooo.oooHeadChunk.chunk.NumSamples())
		require.Len(t, ms.ooo.oooMmappedChunks, 13) // 7 original, 6 duplicate.
	}
	checkNonEmptyOOOChunk(series1)
	checkNonEmptyOOOChunk(series2)

	// No blocks before compaction.
	require.Empty(t, db.Blocks())

	// There is a 0th WBL file.
	require.NoError(t, db.head.wbl.Sync()) // syncing to make sure wbl is flushed in windows
	files, err := os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "00000000", files[0].Name())
	f, err := files[0].Info()
	require.NoError(t, err)
	require.Greater(t, f.Size(), int64(100))

	if addExtraSamples {
		compactOOOHeadTestingCallback = func() {
			addSample(90, 120)  // Back in time, to generate a new OOO chunk.
			addSample(300, 330) // Now some samples after the previous highest timestamp.
			addSample(300, 330) // Repeat to generate an OOO chunk at these timestamps.
		}
		highest = 330
	}

	// OOO compaction happens here.
	require.NoError(t, db.CompactOOOHead(ctx))

	// 3 blocks exist now. [0, 120), [120, 240), [240, 360)
	require.Len(t, db.Blocks(), 3)

	verifyDBSamples() // Blocks created out of OOO head now.

	// 0th WBL file will be deleted and 1st will be the only present.
	files, err = os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "00000001", files[0].Name())
	f, err = files[0].Info()
	require.NoError(t, err)

	if !addExtraSamples {
		require.Equal(t, int64(0), f.Size())
		// OOO stuff should not be present in the Head now.
		checkEmptyOOOChunk(series1)
		checkEmptyOOOChunk(series2)
	}

	verifySamples := func(block *Block, fromMins, toMins int64) {
		series1Samples := make([]chunks.Sample, 0, toMins-fromMins+1)
		series2Samples := make([]chunks.Sample, 0, toMins-fromMins+1)
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			series1Samples = append(series1Samples, scenario.sampleFunc(ts, ts))
			series2Samples = append(series2Samples, scenario.sampleFunc(ts, 2*ts))
		}
		expRes := map[string][]chunks.Sample{
			series1.String(): series1Samples,
			series2.String(): series2Samples,
		}

		q, err := NewBlockQuerier(block, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		requireEqualSeries(t, expRes, actRes, true)
	}

	// Checking for expected data in the blocks.
	verifySamples(db.Blocks()[0], 90, 119)
	verifySamples(db.Blocks()[1], 120, 239)
	verifySamples(db.Blocks()[2], 240, 299)

	// There should be a single m-map file.
	mmapDir := mmappedChunksDir(db.head.opts.ChunkDirRoot)
	files, err = os.ReadDir(mmapDir)
	require.NoError(t, err)
	require.Len(t, files, 1)

	// Compact the in-order head and expect another block.
	// Since this is a forced compaction, this block is not aligned with 2h.
	err = db.CompactHead(NewRangeHead(db.head, 250*time.Minute.Milliseconds(), 350*time.Minute.Milliseconds()))
	require.NoError(t, err)
	require.Len(t, db.Blocks(), 4) // [0, 120), [120, 240), [240, 360), [250, 351)
	verifySamples(db.Blocks()[3], 250, highest)

	verifyDBSamples() // Blocks created out of normal and OOO head now. But not merged.

	// The compaction also clears out the old m-map files. Including
	// the file that has ooo chunks.
	files, err = os.ReadDir(mmapDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "000001", files[0].Name())

	// This will merge overlapping block.
	require.NoError(t, db.Compact(ctx))

	require.Len(t, db.Blocks(), 3) // [0, 120), [120, 240), [240, 360)
	verifySamples(db.Blocks()[0], 90, 119)
	verifySamples(db.Blocks()[1], 120, 239)
	verifySamples(db.Blocks()[2], 240, highest) // Merged block.

	verifyDBSamples() // Final state. Blocks from normal and OOO head are merged.
}

// TestOOOCompactionWithNormalCompaction tests if OOO compaction is performed
// when the normal head's compaction is done.
func TestOOOCompactionWithNormalCompaction(t *testing.T) {
	t.Parallel()
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOCompactionWithNormalCompaction(t, scenario)
		})
	}
}

func testOOOCompactionWithNormalCompaction(t *testing.T, scenario sampleTypeScenario) {
	t.Parallel()
	ctx := context.Background()

	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()

	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions() // We want to manually call it.

	series1 := labels.FromStrings("foo", "bar1")
	series2 := labels.FromStrings("foo", "bar2")

	addSamples := func(fromMins, toMins int64) {
		app := db.Appender(context.Background())
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			_, _, err := scenario.appendFunc(app, series1, ts, ts)
			require.NoError(t, err)
			_, _, err = scenario.appendFunc(app, series2, ts, 2*ts)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Add an in-order samples.
	addSamples(250, 350)

	// Add ooo samples that will result into a single block.
	addSamples(90, 110)

	// Checking that ooo chunk is not empty.
	for _, lbls := range []labels.Labels{series1, series2} {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.Positive(t, ms.ooo.oooHeadChunk.chunk.NumSamples())
	}

	// If the normal Head is not compacted, the OOO head compaction does not take place.
	require.NoError(t, db.Compact(ctx))
	require.Empty(t, db.Blocks())

	// Add more in-order samples in future that would trigger the compaction.
	addSamples(400, 450)

	// No blocks before compaction.
	require.Empty(t, db.Blocks())

	// Compacts normal and OOO head.
	require.NoError(t, db.Compact(ctx))

	// 2 blocks exist now. [0, 120), [250, 360)
	require.Len(t, db.Blocks(), 2)
	require.Equal(t, int64(0), db.Blocks()[0].MinTime())
	require.Equal(t, 120*time.Minute.Milliseconds(), db.Blocks()[0].MaxTime())
	require.Equal(t, 250*time.Minute.Milliseconds(), db.Blocks()[1].MinTime())
	require.Equal(t, 360*time.Minute.Milliseconds(), db.Blocks()[1].MaxTime())

	// Checking that ooo chunk is empty.
	for _, lbls := range []labels.Labels{series1, series2} {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.Nil(t, ms.ooo)
	}

	verifySamples := func(block *Block, fromMins, toMins int64) {
		series1Samples := make([]chunks.Sample, 0, toMins-fromMins+1)
		series2Samples := make([]chunks.Sample, 0, toMins-fromMins+1)
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			series1Samples = append(series1Samples, scenario.sampleFunc(ts, ts))
			series2Samples = append(series2Samples, scenario.sampleFunc(ts, 2*ts))
		}
		expRes := map[string][]chunks.Sample{
			series1.String(): series1Samples,
			series2.String(): series2Samples,
		}

		q, err := NewBlockQuerier(block, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		requireEqualSeries(t, expRes, actRes, true)
	}

	// Checking for expected data in the blocks.
	verifySamples(db.Blocks()[0], 90, 110)
	verifySamples(db.Blocks()[1], 250, 350)
}

// TestOOOCompactionWithDisabledWriteLog tests the scenario where the TSDB is
// configured to not have wal and wbl but its able to compact both the in-order
// and out-of-order head.
func TestOOOCompactionWithDisabledWriteLog(t *testing.T) {
	t.Parallel()
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOCompactionWithDisabledWriteLog(t, scenario)
		})
	}
}

func testOOOCompactionWithDisabledWriteLog(t *testing.T, scenario sampleTypeScenario) {
	t.Parallel()
	ctx := context.Background()

	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
	opts.WALSegmentSize = -1 // disabled WAL and WBL

	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions() // We want to manually call it.

	series1 := labels.FromStrings("foo", "bar1")
	series2 := labels.FromStrings("foo", "bar2")

	addSamples := func(fromMins, toMins int64) {
		app := db.Appender(context.Background())
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			_, _, err := scenario.appendFunc(app, series1, ts, ts)
			require.NoError(t, err)
			_, _, err = scenario.appendFunc(app, series2, ts, 2*ts)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Add an in-order samples.
	addSamples(250, 350)

	// Add ooo samples that will result into a single block.
	addSamples(90, 110)

	// Checking that ooo chunk is not empty.
	for _, lbls := range []labels.Labels{series1, series2} {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.Positive(t, ms.ooo.oooHeadChunk.chunk.NumSamples())
	}

	// If the normal Head is not compacted, the OOO head compaction does not take place.
	require.NoError(t, db.Compact(ctx))
	require.Empty(t, db.Blocks())

	// Add more in-order samples in future that would trigger the compaction.
	addSamples(400, 450)

	// No blocks before compaction.
	require.Empty(t, db.Blocks())

	// Compacts normal and OOO head.
	require.NoError(t, db.Compact(ctx))

	// 2 blocks exist now. [0, 120), [250, 360)
	require.Len(t, db.Blocks(), 2)
	require.Equal(t, int64(0), db.Blocks()[0].MinTime())
	require.Equal(t, 120*time.Minute.Milliseconds(), db.Blocks()[0].MaxTime())
	require.Equal(t, 250*time.Minute.Milliseconds(), db.Blocks()[1].MinTime())
	require.Equal(t, 360*time.Minute.Milliseconds(), db.Blocks()[1].MaxTime())

	// Checking that ooo chunk is empty.
	for _, lbls := range []labels.Labels{series1, series2} {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.Nil(t, ms.ooo)
	}

	verifySamples := func(block *Block, fromMins, toMins int64) {
		series1Samples := make([]chunks.Sample, 0, toMins-fromMins+1)
		series2Samples := make([]chunks.Sample, 0, toMins-fromMins+1)
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			series1Samples = append(series1Samples, scenario.sampleFunc(ts, ts))
			series2Samples = append(series2Samples, scenario.sampleFunc(ts, 2*ts))
		}
		expRes := map[string][]chunks.Sample{
			series1.String(): series1Samples,
			series2.String(): series2Samples,
		}

		q, err := NewBlockQuerier(block, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		requireEqualSeries(t, expRes, actRes, true)
	}

	// Checking for expected data in the blocks.
	verifySamples(db.Blocks()[0], 90, 110)
	verifySamples(db.Blocks()[1], 250, 350)
}

// TestOOOQueryAfterRestartWithSnapshotAndRemovedWBL tests the scenario where the WBL goes
// missing after a restart while snapshot was enabled, but the query still returns the right
// data from the mmap chunks.
func TestOOOQueryAfterRestartWithSnapshotAndRemovedWBL(t *testing.T) {
	t.Parallel()
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOQueryAfterRestartWithSnapshotAndRemovedWBL(t, scenario)
		})
	}
}

func testOOOQueryAfterRestartWithSnapshotAndRemovedWBL(t *testing.T, scenario sampleTypeScenario) {
	ctx := context.Background()

	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 10
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
	opts.EnableMemorySnapshotOnShutdown = true

	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions() // We want to manually call it.

	series1 := labels.FromStrings("foo", "bar1")
	series2 := labels.FromStrings("foo", "bar2")

	addSamples := func(fromMins, toMins int64) {
		app := db.Appender(context.Background())
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			_, _, err := scenario.appendFunc(app, series1, ts, ts)
			require.NoError(t, err)
			_, _, err = scenario.appendFunc(app, series2, ts, 2*ts)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Add an in-order samples.
	addSamples(250, 350)

	// Add ooo samples that will result into a single block.
	addSamples(90, 110) // The sample 110 will not be in m-map chunks.

	// Checking that there are some ooo m-map chunks.
	for _, lbls := range []labels.Labels{series1, series2} {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.Len(t, ms.ooo.oooMmappedChunks, 2)
		require.NotNil(t, ms.ooo.oooHeadChunk)
	}

	// Restart DB.
	require.NoError(t, db.Close())

	// For some reason wbl goes missing.
	require.NoError(t, os.RemoveAll(path.Join(db.Dir(), "wbl")))

	db = newTestDB(t, withDir(db.Dir()))
	db.DisableCompactions() // We want to manually call it.

	// Check ooo m-map chunks again.
	for _, lbls := range []labels.Labels{series1, series2} {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.Len(t, ms.ooo.oooMmappedChunks, 2)
		require.Equal(t, 109*time.Minute.Milliseconds(), ms.ooo.oooMmappedChunks[1].maxTime)
		require.Nil(t, ms.ooo.oooHeadChunk) // Because of missing wbl.
	}

	verifySamples := func(fromMins, toMins int64) {
		series1Samples := make([]chunks.Sample, 0, toMins-fromMins+1)
		series2Samples := make([]chunks.Sample, 0, toMins-fromMins+1)
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			series1Samples = append(series1Samples, scenario.sampleFunc(ts, ts))
			series2Samples = append(series2Samples, scenario.sampleFunc(ts, ts*2))
		}
		expRes := map[string][]chunks.Sample{
			series1.String(): series1Samples,
			series2.String(): series2Samples,
		}

		q, err := db.Querier(fromMins*time.Minute.Milliseconds(), toMins*time.Minute.Milliseconds())
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		requireEqualSeries(t, expRes, actRes, true)
	}

	// Checking for expected ooo data from mmap chunks.
	verifySamples(90, 109)

	// Compaction should also work fine.
	require.Empty(t, db.Blocks())
	require.NoError(t, db.CompactOOOHead(ctx))
	require.Len(t, db.Blocks(), 1) // One block from OOO data.
	require.Equal(t, int64(0), db.Blocks()[0].MinTime())
	require.Equal(t, 120*time.Minute.Milliseconds(), db.Blocks()[0].MaxTime())

	// Checking that ooo chunk is empty in Head.
	for _, lbls := range []labels.Labels{series1, series2} {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.Nil(t, ms.ooo)
	}

	verifySamples(90, 109)
}

func TestQuerierOOOQuery(t *testing.T) {
	scenarios := map[string]struct {
		appendFunc func(app storage.Appender, ts int64, counterReset bool) (storage.SeriesRef, error)
		sampleFunc func(ts int64) chunks.Sample
	}{
		"float": {
			appendFunc: func(app storage.Appender, ts int64, _ bool) (storage.SeriesRef, error) {
				return app.Append(0, labels.FromStrings("foo", "bar1"), ts, float64(ts))
			},
			sampleFunc: func(ts int64) chunks.Sample {
				return sample{t: ts, f: float64(ts)}
			},
		},
		"integer histogram": {
			appendFunc: func(app storage.Appender, ts int64, counterReset bool) (storage.SeriesRef, error) {
				h := tsdbutil.GenerateTestHistogram(ts)
				if counterReset {
					h.CounterResetHint = histogram.CounterReset
				}
				return app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, h, nil)
			},
			sampleFunc: func(ts int64) chunks.Sample {
				return sample{t: ts, h: tsdbutil.GenerateTestHistogram(ts)}
			},
		},
		"float histogram": {
			appendFunc: func(app storage.Appender, ts int64, counterReset bool) (storage.SeriesRef, error) {
				fh := tsdbutil.GenerateTestFloatHistogram(ts)
				if counterReset {
					fh.CounterResetHint = histogram.CounterReset
				}
				return app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, nil, fh)
			},
			sampleFunc: func(ts int64) chunks.Sample {
				return sample{t: ts, fh: tsdbutil.GenerateTestFloatHistogram(ts)}
			},
		},
		"integer histogram counter resets": {
			// Adding counter reset to all histograms means each histogram will have its own chunk.
			appendFunc: func(app storage.Appender, ts int64, _ bool) (storage.SeriesRef, error) {
				h := tsdbutil.GenerateTestHistogram(ts)
				h.CounterResetHint = histogram.CounterReset // For this scenario, ignore the counterReset argument.
				return app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, h, nil)
			},
			sampleFunc: func(ts int64) chunks.Sample {
				return sample{t: ts, h: tsdbutil.GenerateTestHistogram(ts)}
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			testQuerierOOOQuery(t, scenario.appendFunc, scenario.sampleFunc)
		})
	}
}

func testQuerierOOOQuery(t *testing.T,
	appendFunc func(app storage.Appender, ts int64, counterReset bool) (storage.SeriesRef, error),
	sampleFunc func(ts int64) chunks.Sample,
) {
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = 24 * time.Hour.Milliseconds()

	series1 := labels.FromStrings("foo", "bar1")

	type filterFunc func(t int64) bool
	defaultFilterFunc := func(int64) bool { return true }

	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }
	addSample := func(db *DB, fromMins, toMins, queryMinT, queryMaxT int64, expSamples []chunks.Sample, filter filterFunc, counterReset bool) ([]chunks.Sample, int) {
		app := db.Appender(context.Background())
		totalAppended := 0
		for m := fromMins; m <= toMins; m += time.Minute.Milliseconds() {
			if !filter(m / time.Minute.Milliseconds()) {
				continue
			}
			_, err := appendFunc(app, m, counterReset)
			if m >= queryMinT && m <= queryMaxT {
				expSamples = append(expSamples, sampleFunc(m))
			}
			require.NoError(t, err)
			totalAppended++
		}
		require.NoError(t, app.Commit())
		require.Positive(t, totalAppended, 0) // Sanity check that filter is not too zealous.
		return expSamples, totalAppended
	}

	type sampleBatch struct {
		minT         int64
		maxT         int64
		filter       filterFunc
		counterReset bool
		isOOO        bool
	}

	tests := []struct {
		name      string
		oooCap    int64
		queryMinT int64
		queryMaxT int64
		batches   []sampleBatch
	}{
		{
			name:      "query interval covering ooomint and inordermaxt returns all ingested samples",
			oooCap:    30,
			queryMinT: minutes(0),
			queryMaxT: minutes(200),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: defaultFilterFunc,
				},
				{
					minT:   minutes(0),
					maxT:   minutes(99),
					filter: defaultFilterFunc,
					isOOO:  true,
				},
			},
		},
		{
			name:      "partial query interval returns only samples within interval",
			oooCap:    30,
			queryMinT: minutes(20),
			queryMaxT: minutes(180),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: defaultFilterFunc,
				},
				{
					minT:   minutes(0),
					maxT:   minutes(99),
					filter: defaultFilterFunc,
					isOOO:  true,
				},
			},
		},
		{
			name:      "alternating OOO batches", // In order: 100-200 normal. out of order first path: 0, 2, 4, ... 98 (no counter reset), second pass: 1, 3, 5, ... 99 (with counter reset).
			queryMinT: minutes(0),
			queryMaxT: minutes(200),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: defaultFilterFunc,
				},
				{
					minT:   minutes(0),
					maxT:   minutes(99),
					filter: func(t int64) bool { return t%2 == 0 },
					isOOO:  true,
				},
				{
					minT:         minutes(0),
					maxT:         minutes(99),
					filter:       func(t int64) bool { return t%2 == 1 },
					counterReset: true,
					isOOO:        true,
				},
			},
		},
		{
			name:      "query overlapping inorder and ooo samples returns all ingested samples at the end of the interval",
			oooCap:    30,
			queryMinT: minutes(0),
			queryMaxT: minutes(200),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: func(t int64) bool { return t%2 == 0 },
					isOOO:  false,
				},
				{
					minT:   minutes(170),
					maxT:   minutes(180),
					filter: func(t int64) bool { return t%2 == 1 },
					isOOO:  true,
				},
			},
		},
		{
			name:      "query overlapping inorder and ooo in-memory samples returns all ingested samples at the beginning of the interval",
			oooCap:    30,
			queryMinT: minutes(0),
			queryMaxT: minutes(200),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: func(t int64) bool { return t%2 == 0 },
					isOOO:  false,
				},
				{
					minT:   minutes(100),
					maxT:   minutes(110),
					filter: func(t int64) bool { return t%2 == 1 },
					isOOO:  true,
				},
			},
		},
		{
			name:      "query inorder contain ooo mmapped samples returns all ingested samples at the beginning of the interval",
			oooCap:    5,
			queryMinT: minutes(0),
			queryMaxT: minutes(200),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: func(t int64) bool { return t%2 == 0 },
					isOOO:  false,
				},
				{
					minT:   minutes(101),
					maxT:   minutes(101 + (5-1)*2), // Append samples to fit in a single mmapped OOO chunk and fit inside the first in-order mmapped chunk.
					filter: func(t int64) bool { return t%2 == 1 },
					isOOO:  true,
				},
				{
					minT:   minutes(191),
					maxT:   minutes(193), // Append some more OOO samples to trigger mapping the OOO chunk, but use time 151 to not overlap with in-order head chunk.
					filter: func(t int64) bool { return t%2 == 1 },
					isOOO:  true,
				},
			},
		},
		{
			name:      "query overlapping inorder and ooo mmapped samples returns all ingested samples at the beginning of the interval",
			oooCap:    30,
			queryMinT: minutes(0),
			queryMaxT: minutes(200),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: func(t int64) bool { return t%2 == 0 },
					isOOO:  false,
				},
				{
					minT:   minutes(101),
					maxT:   minutes(101 + (30-1)*2), // Append samples to fit in a single mmapped OOO chunk and overlap the first in-order mmapped chunk.
					filter: func(t int64) bool { return t%2 == 1 },
					isOOO:  true,
				},
				{
					minT:   minutes(191),
					maxT:   minutes(193), // Append some more OOO samples to trigger mapping the OOO chunk, but use time 151 to not overlap with in-order head chunk.
					filter: func(t int64) bool { return t%2 == 1 },
					isOOO:  true,
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("name=%s", tc.name), func(t *testing.T) {
			opts.OutOfOrderCapMax = tc.oooCap
			db := newTestDB(t, withOpts(opts))
			db.DisableCompactions()

			var expSamples []chunks.Sample
			var oooSamples, appendedCount int

			for _, batch := range tc.batches {
				expSamples, appendedCount = addSample(db, batch.minT, batch.maxT, tc.queryMinT, tc.queryMaxT, expSamples, batch.filter, batch.counterReset)
				if batch.isOOO {
					oooSamples += appendedCount
				}
			}

			sort.Slice(expSamples, func(i, j int) bool {
				return expSamples[i].T() < expSamples[j].T()
			})

			querier, err := db.Querier(tc.queryMinT, tc.queryMaxT)
			require.NoError(t, err)
			defer querier.Close()

			seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar1"))
			gotSamples := seriesSet[series1.String()]
			require.NotNil(t, gotSamples)
			require.Len(t, seriesSet, 1)
			requireEqualSamples(t, series1.String(), expSamples, gotSamples, requireEqualSamplesIgnoreCounterResets)
			requireEqualOOOSamples(t, oooSamples, db)
		})
	}
}

func TestChunkQuerierOOOQuery(t *testing.T) {
	nBucketHistogram := func(n int64) *histogram.Histogram {
		h := &histogram.Histogram{
			Count: uint64(n),
			Sum:   float64(n),
		}
		if n == 0 {
			h.PositiveSpans = []histogram.Span{}
			h.PositiveBuckets = []int64{}
			return h
		}
		h.PositiveSpans = []histogram.Span{{Offset: 0, Length: uint32(n)}}
		h.PositiveBuckets = make([]int64, n)
		h.PositiveBuckets[0] = 1
		return h
	}

	scenarios := map[string]struct {
		appendFunc       func(app storage.Appender, ts int64, counterReset bool) (storage.SeriesRef, error)
		sampleFunc       func(ts int64) chunks.Sample
		checkInUseBucket bool
	}{
		"float": {
			appendFunc: func(app storage.Appender, ts int64, _ bool) (storage.SeriesRef, error) {
				return app.Append(0, labels.FromStrings("foo", "bar1"), ts, float64(ts))
			},
			sampleFunc: func(ts int64) chunks.Sample {
				return sample{t: ts, f: float64(ts)}
			},
		},
		"integer histogram": {
			appendFunc: func(app storage.Appender, ts int64, counterReset bool) (storage.SeriesRef, error) {
				h := tsdbutil.GenerateTestHistogram(ts)
				if counterReset {
					h.CounterResetHint = histogram.CounterReset
				}
				return app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, h, nil)
			},
			sampleFunc: func(ts int64) chunks.Sample {
				return sample{t: ts, h: tsdbutil.GenerateTestHistogram(ts)}
			},
		},
		"float histogram": {
			appendFunc: func(app storage.Appender, ts int64, counterReset bool) (storage.SeriesRef, error) {
				fh := tsdbutil.GenerateTestFloatHistogram(ts)
				if counterReset {
					fh.CounterResetHint = histogram.CounterReset
				}
				return app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, nil, fh)
			},
			sampleFunc: func(ts int64) chunks.Sample {
				return sample{t: ts, fh: tsdbutil.GenerateTestFloatHistogram(ts)}
			},
		},
		"integer histogram counter resets": {
			// Adding counter reset to all histograms means each histogram will have its own chunk.
			appendFunc: func(app storage.Appender, ts int64, _ bool) (storage.SeriesRef, error) {
				h := tsdbutil.GenerateTestHistogram(ts)
				h.CounterResetHint = histogram.CounterReset // For this scenario, ignore the counterReset argument.
				return app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, h, nil)
			},
			sampleFunc: func(ts int64) chunks.Sample {
				return sample{t: ts, h: tsdbutil.GenerateTestHistogram(ts)}
			},
		},
		"integer histogram with recode": {
			// Histograms have increasing number of buckets so their chunks are recoded.
			appendFunc: func(app storage.Appender, ts int64, _ bool) (storage.SeriesRef, error) {
				n := ts / time.Minute.Milliseconds()
				return app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, nBucketHistogram(n), nil)
			},
			sampleFunc: func(ts int64) chunks.Sample {
				n := ts / time.Minute.Milliseconds()
				return sample{t: ts, h: nBucketHistogram(n)}
			},
			// Only check in-use buckets for this scenario.
			// Recoding adds empty buckets.
			checkInUseBucket: true,
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			testChunkQuerierOOOQuery(t, scenario.appendFunc, scenario.sampleFunc, scenario.checkInUseBucket)
		})
	}
}

func testChunkQuerierOOOQuery(t *testing.T,
	appendFunc func(app storage.Appender, ts int64, counterReset bool) (storage.SeriesRef, error),
	sampleFunc func(ts int64) chunks.Sample,
	checkInUseBuckets bool,
) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 24 * time.Hour.Milliseconds()

	series1 := labels.FromStrings("foo", "bar1")

	type filterFunc func(t int64) bool
	defaultFilterFunc := func(int64) bool { return true }

	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }
	addSample := func(db *DB, fromMins, toMins, queryMinT, queryMaxT int64, expSamples []chunks.Sample, filter filterFunc, counterReset bool) ([]chunks.Sample, int) {
		app := db.Appender(context.Background())
		totalAppended := 0
		for m := fromMins; m <= toMins; m += time.Minute.Milliseconds() {
			if !filter(m / time.Minute.Milliseconds()) {
				continue
			}
			_, err := appendFunc(app, m, counterReset)
			if m >= queryMinT && m <= queryMaxT {
				expSamples = append(expSamples, sampleFunc(m))
			}
			require.NoError(t, err)
			totalAppended++
		}
		require.NoError(t, app.Commit())
		require.Positive(t, totalAppended) // Sanity check that filter is not too zealous.
		return expSamples, totalAppended
	}

	type sampleBatch struct {
		minT         int64
		maxT         int64
		filter       filterFunc
		counterReset bool
		isOOO        bool
	}

	tests := []struct {
		name      string
		oooCap    int64
		queryMinT int64
		queryMaxT int64
		batches   []sampleBatch
	}{
		{
			name:      "query interval covering ooomint and inordermaxt returns all ingested samples",
			oooCap:    30,
			queryMinT: minutes(0),
			queryMaxT: minutes(200),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: defaultFilterFunc,
				},
				{
					minT:   minutes(0),
					maxT:   minutes(99),
					filter: defaultFilterFunc,
					isOOO:  true,
				},
			},
		},
		{
			name:      "partial query interval returns only samples within interval",
			oooCap:    30,
			queryMinT: minutes(20),
			queryMaxT: minutes(180),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: defaultFilterFunc,
				},
				{
					minT:   minutes(0),
					maxT:   minutes(99),
					filter: defaultFilterFunc,
					isOOO:  true,
				},
			},
		},
		{
			name:      "alternating OOO batches", // In order: 100-200 normal. out of order first path: 0, 2, 4, ... 98 (no counter reset), second pass: 1, 3, 5, ... 99 (with counter reset).
			queryMinT: minutes(0),
			queryMaxT: minutes(200),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: defaultFilterFunc,
				},
				{
					minT:   minutes(0),
					maxT:   minutes(99),
					filter: func(t int64) bool { return t%2 == 0 },
					isOOO:  true,
				},
				{
					minT:         minutes(0),
					maxT:         minutes(99),
					filter:       func(t int64) bool { return t%2 == 1 },
					counterReset: true,
					isOOO:        true,
				},
			},
		},
		{
			name:      "query overlapping inorder and ooo samples returns all ingested samples at the end of the interval",
			oooCap:    30,
			queryMinT: minutes(0),
			queryMaxT: minutes(200),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: func(t int64) bool { return t%2 == 0 },
					isOOO:  false,
				},
				{
					minT:   minutes(170),
					maxT:   minutes(180),
					filter: func(t int64) bool { return t%2 == 1 },
					isOOO:  true,
				},
			},
		},
		{
			name:      "query overlapping inorder and ooo in-memory samples returns all ingested samples at the beginning of the interval",
			oooCap:    30,
			queryMinT: minutes(0),
			queryMaxT: minutes(200),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: func(t int64) bool { return t%2 == 0 },
					isOOO:  false,
				},
				{
					minT:   minutes(100),
					maxT:   minutes(110),
					filter: func(t int64) bool { return t%2 == 1 },
					isOOO:  true,
				},
			},
		},
		{
			name:      "query inorder contain ooo mmapped samples returns all ingested samples at the beginning of the interval",
			oooCap:    5,
			queryMinT: minutes(0),
			queryMaxT: minutes(200),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: func(t int64) bool { return t%2 == 0 },
					isOOO:  false,
				},
				{
					minT:   minutes(101),
					maxT:   minutes(101 + (5-1)*2), // Append samples to fit in a single mmapped OOO chunk and fit inside the first in-order mmapped chunk.
					filter: func(t int64) bool { return t%2 == 1 },
					isOOO:  true,
				},
				{
					minT:   minutes(191),
					maxT:   minutes(193), // Append some more OOO samples to trigger mapping the OOO chunk, but use time 151 to not overlap with in-order head chunk.
					filter: func(t int64) bool { return t%2 == 1 },
					isOOO:  true,
				},
			},
		},
		{
			name:      "query overlapping inorder and ooo mmapped samples returns all ingested samples at the beginning of the interval",
			oooCap:    30,
			queryMinT: minutes(0),
			queryMaxT: minutes(200),
			batches: []sampleBatch{
				{
					minT:   minutes(100),
					maxT:   minutes(200),
					filter: func(t int64) bool { return t%2 == 0 },
					isOOO:  false,
				},
				{
					minT:   minutes(101),
					maxT:   minutes(101 + (30-1)*2), // Append samples to fit in a single mmapped OOO chunk and overlap the first in-order mmapped chunk.
					filter: func(t int64) bool { return t%2 == 1 },
					isOOO:  true,
				},
				{
					minT:   minutes(191),
					maxT:   minutes(193), // Append some more OOO samples to trigger mapping the OOO chunk, but use time 151 to not overlap with in-order head chunk.
					filter: func(t int64) bool { return t%2 == 1 },
					isOOO:  true,
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("name=%s", tc.name), func(t *testing.T) {
			opts.OutOfOrderCapMax = tc.oooCap
			db := newTestDB(t, withOpts(opts))
			db.DisableCompactions()

			var expSamples []chunks.Sample
			var oooSamples, appendedCount int

			for _, batch := range tc.batches {
				expSamples, appendedCount = addSample(db, batch.minT, batch.maxT, tc.queryMinT, tc.queryMaxT, expSamples, batch.filter, batch.counterReset)
				if batch.isOOO {
					oooSamples += appendedCount
				}
			}

			sort.Slice(expSamples, func(i, j int) bool {
				return expSamples[i].T() < expSamples[j].T()
			})

			querier, err := db.ChunkQuerier(tc.queryMinT, tc.queryMaxT)
			require.NoError(t, err)
			defer querier.Close()

			chks := queryChunks(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar1"))
			require.NotNil(t, chks[series1.String()])
			require.Len(t, chks, 1)
			requireEqualOOOSamples(t, oooSamples, db)
			var gotSamples []chunks.Sample
			for _, chunk := range chks[series1.String()] {
				it := chunk.Chunk.Iterator(nil)
				smpls, err := storage.ExpandSamples(it, newSample)
				require.NoError(t, err)

				// Verify that no sample is outside the chunk's time range.
				for i, s := range smpls {
					switch i {
					case 0:
						require.Equal(t, chunk.MinTime, s.T(), "first sample %v not at chunk min time %v", s, chunk.MinTime)
					case len(smpls) - 1:
						require.Equal(t, chunk.MaxTime, s.T(), "last sample %v not at chunk max time %v", s, chunk.MaxTime)
					default:
						require.GreaterOrEqual(t, s.T(), chunk.MinTime, "sample %v before chunk min time %v", s, chunk.MinTime)
						require.LessOrEqual(t, s.T(), chunk.MaxTime, "sample %v after chunk max time %v", s, chunk.MaxTime)
					}
				}

				gotSamples = append(gotSamples, smpls...)
				require.NoError(t, it.Err())
			}
			if checkInUseBuckets {
				requireEqualSamples(t, series1.String(), expSamples, gotSamples, requireEqualSamplesIgnoreCounterResets, requireEqualSamplesInUseBucketCompare)
			} else {
				requireEqualSamples(t, series1.String(), expSamples, gotSamples, requireEqualSamplesIgnoreCounterResets)
			}
		})
	}
}

// TestOOONativeHistogramsWithCounterResets verifies the counter reset headers for in-order and out-of-order samples
// upon ingestion. Note that when the counter reset(s) occur in OOO samples, the header is set to UnknownCounterReset
// rather than CounterReset. This is because with OOO native histogram samples, it cannot be definitely
// determined if a counter reset occurred because the samples are not consecutive, and another sample
// could potentially come in that would change the status of the header. In this case, the UnknownCounterReset
// headers would be re-checked at query time and updated as needed. However, this test is checking the counter
// reset headers at the time of storage.
func TestOOONativeHistogramsWithCounterResets(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			if name == intHistogram || name == floatHistogram {
				testOOONativeHistogramsWithCounterResets(t, scenario)
			}
		})
	}
}

func testOOONativeHistogramsWithCounterResets(t *testing.T, scenario sampleTypeScenario) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 24 * time.Hour.Milliseconds()

	type resetFunc func(v int64) bool
	defaultResetFunc := func(int64) bool { return false }

	lbls := labels.FromStrings("foo", "bar1")
	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }

	type sampleBatch struct {
		from                 int64
		until                int64
		shouldReset          resetFunc
		expCounterResetHints []histogram.CounterResetHint
	}

	tests := []struct {
		name            string
		queryMin        int64
		queryMax        int64
		batches         []sampleBatch
		expectedSamples []chunks.Sample
	}{
		{
			name:     "Counter reset within in-order samples",
			queryMin: minutes(40),
			queryMax: minutes(55),
			batches: []sampleBatch{
				// In-order samples
				{
					from:  40,
					until: 50,
					shouldReset: func(v int64) bool {
						return v == 45
					},
					expCounterResetHints: []histogram.CounterResetHint{histogram.UnknownCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.UnknownCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset},
				},
			},
		},
		{
			name:     "Counter reset right at beginning of OOO samples",
			queryMin: minutes(40),
			queryMax: minutes(55),
			batches: []sampleBatch{
				// In-order samples
				{
					from:                 40,
					until:                45,
					shouldReset:          defaultResetFunc,
					expCounterResetHints: []histogram.CounterResetHint{histogram.UnknownCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset},
				},
				{
					from:                 50,
					until:                55,
					shouldReset:          defaultResetFunc,
					expCounterResetHints: []histogram.CounterResetHint{histogram.UnknownCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset},
				},
				// OOO samples
				{
					from:  45,
					until: 50,
					shouldReset: func(v int64) bool {
						return v == 45
					},
					expCounterResetHints: []histogram.CounterResetHint{histogram.UnknownCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset},
				},
			},
		},
		{
			name:     "Counter resets in both in-order and OOO samples",
			queryMin: minutes(40),
			queryMax: minutes(55),
			batches: []sampleBatch{
				// In-order samples
				{
					from:  40,
					until: 45,
					shouldReset: func(v int64) bool {
						return v == 44
					},
					expCounterResetHints: []histogram.CounterResetHint{histogram.UnknownCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.UnknownCounterReset},
				},
				{
					from:                 50,
					until:                55,
					shouldReset:          defaultResetFunc,
					expCounterResetHints: []histogram.CounterResetHint{histogram.UnknownCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset},
				},
				// OOO samples
				{
					from:  45,
					until: 50,
					shouldReset: func(v int64) bool {
						return v == 49
					},
					expCounterResetHints: []histogram.CounterResetHint{histogram.UnknownCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.NotCounterReset, histogram.UnknownCounterReset},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("name=%s", tc.name), func(t *testing.T) {
			db := newTestDB(t, withOpts(opts))
			db.DisableCompactions()

			app := db.Appender(context.Background())

			expSamples := make(map[string][]chunks.Sample)

			for _, batch := range tc.batches {
				j := batch.from
				smplIdx := 0
				for i := batch.from; i < batch.until; i++ {
					resetCount := batch.shouldReset(i)
					if resetCount {
						j = 0
					}
					_, s, err := scenario.appendFunc(app, lbls, minutes(i), j)
					require.NoError(t, err)
					if s.Type() == chunkenc.ValHistogram {
						s.H().CounterResetHint = batch.expCounterResetHints[smplIdx]
					} else if s.Type() == chunkenc.ValFloatHistogram {
						s.FH().CounterResetHint = batch.expCounterResetHints[smplIdx]
					}
					expSamples[lbls.String()] = append(expSamples[lbls.String()], s)
					j++
					smplIdx++
				}
			}

			require.NoError(t, app.Commit())

			for k, v := range expSamples {
				sort.Slice(v, func(i, j int) bool {
					return v[i].T() < v[j].T()
				})
				expSamples[k] = v
			}

			querier, err := db.Querier(tc.queryMin, tc.queryMax)
			require.NoError(t, err)
			defer querier.Close()

			seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar1"))
			require.NotNil(t, seriesSet[lbls.String()])
			require.Len(t, seriesSet, 1)
			requireEqualSeries(t, expSamples, seriesSet, false)
		})
	}
}

func TestOOOInterleavedImplicitCounterResets(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOInterleavedImplicitCounterResets(t, name, scenario)
		})
	}
}

func testOOOInterleavedImplicitCounterResets(t *testing.T, name string, scenario sampleTypeScenario) {
	var appendFunc func(app storage.Appender, ts, v int64) error

	if scenario.sampleType != sampleMetricTypeHistogram {
		return
	}

	switch name {
	case intHistogram:
		appendFunc = func(app storage.Appender, ts, v int64) error {
			h := &histogram.Histogram{
				Count:           uint64(v),
				Sum:             float64(v),
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []int64{v},
			}
			_, err := app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, h, nil)
			return err
		}
	case floatHistogram:
		appendFunc = func(app storage.Appender, ts, v int64) error {
			fh := &histogram.FloatHistogram{
				Count:           float64(v),
				Sum:             float64(v),
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []float64{float64(v)},
			}
			_, err := app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, nil, fh)
			return err
		}
	case customBucketsIntHistogram:
		appendFunc = func(app storage.Appender, ts, v int64) error {
			h := &histogram.Histogram{
				Schema:          -53,
				Count:           uint64(v),
				Sum:             float64(v),
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []int64{v},
				CustomValues:    []float64{float64(1), float64(2), float64(3)},
			}
			_, err := app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, h, nil)
			return err
		}
	case customBucketsFloatHistogram:
		appendFunc = func(app storage.Appender, ts, v int64) error {
			fh := &histogram.FloatHistogram{
				Schema:          -53,
				Count:           float64(v),
				Sum:             float64(v),
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []float64{float64(v)},
				CustomValues:    []float64{float64(1), float64(2), float64(3)},
			}
			_, err := app.AppendHistogram(0, labels.FromStrings("foo", "bar1"), ts, nil, fh)
			return err
		}
	case gaugeIntHistogram, gaugeFloatHistogram:
		return
	}

	// Not a sample, we're encoding an integer counter that we convert to a
	// histogram with a single bucket.
	type tsValue struct {
		ts int64
		v  int64
	}

	type expectedTsValue struct {
		ts   int64
		v    int64
		hint histogram.CounterResetHint
	}

	type expectedChunk struct {
		hint histogram.CounterResetHint
		size int
	}

	cases := map[string]struct {
		samples []tsValue
		oooCap  int64
		// The expected samples with counter reset.
		expectedSamples []expectedTsValue
		// The expected counter reset hint for each chunk.
		expectedChunks []expectedChunk
	}{
		"counter reset in-order cleared by in-memory OOO chunk": {
			samples: []tsValue{
				{1, 40}, // New in In-order. I1.
				{4, 30}, // In-order counter reset. I2.
				{2, 40}, // New in OOO. O1.
				{3, 10}, // OOO counter reset. O2.
			},
			oooCap: 30,
			// Expect all to be set to UnknownCounterReset because we switch between
			// in-order and out-of-order samples.
			expectedSamples: []expectedTsValue{
				{1, 40, histogram.UnknownCounterReset}, // I1.
				{2, 40, histogram.UnknownCounterReset}, // O1.
				{3, 10, histogram.UnknownCounterReset}, // O2.
				{4, 30, histogram.UnknownCounterReset}, // I2. Counter reset cleared by iterator change.
			},
			expectedChunks: []expectedChunk{
				{histogram.UnknownCounterReset, 1}, // I1.
				{histogram.UnknownCounterReset, 1}, // O1.
				{histogram.UnknownCounterReset, 1}, // O2.
				{histogram.UnknownCounterReset, 1}, // I2.
			},
		},
		"counter reset in OOO mmapped chunk cleared by in-memory ooo chunk": {
			samples: []tsValue{
				{8, 30}, // In-order, new chunk. I1.
				{1, 10}, // OOO, new chunk (will be mmapped). MO1.
				{2, 20}, // OOO, no reset (will be mmapped). MO1.
				{3, 30}, // OOO, no reset (will be mmapped). MO1.
				{5, 20}, // OOO, reset (will be mmapped). MO2.
				{6, 10}, // OOO, reset (will be mmapped). MO3.
				{7, 20}, // OOO, no reset (will be mmapped). MO3.
				{4, 10}, // OOO, inserted into memory, triggers mmap. O1.
			},
			oooCap: 6,
			expectedSamples: []expectedTsValue{
				{1, 10, histogram.UnknownCounterReset}, // MO1.
				{2, 20, histogram.NotCounterReset},     // MO1.
				{3, 30, histogram.NotCounterReset},     // MO1.
				{4, 10, histogram.UnknownCounterReset}, // O1. Counter reset cleared by iterator change.
				{5, 20, histogram.UnknownCounterReset}, // MO2.
				{6, 10, histogram.UnknownCounterReset}, // MO3.
				{7, 20, histogram.NotCounterReset},     // MO3.
				{8, 30, histogram.UnknownCounterReset}, // I1.
			},
			expectedChunks: []expectedChunk{
				{histogram.UnknownCounterReset, 3}, // MO1.
				{histogram.UnknownCounterReset, 1}, // O1.
				{histogram.UnknownCounterReset, 1}, // MO2.
				{histogram.UnknownCounterReset, 2}, // MO3.
				{histogram.UnknownCounterReset, 1}, // I1.
			},
		},
		"counter reset in OOO mmapped chunk cleared by another OOO mmapped chunk": {
			samples: []tsValue{
				{8, 100}, // In-order, new chunk. I1.
				{1, 50},  // OOO, new chunk (will be mmapped). MO1.
				{5, 40},  // OOO, reset (will be mmapped). MO2.
				{6, 50},  // OOO, no reset (will be mmapped). MO2.
				{2, 10},  // OOO, new chunk no reset (will be mmapped). MO3.
				{3, 20},  // OOO, no reset (will be mmapped). MO3.
				{4, 30},  // OOO, no reset (will be mmapped). MO3.
				{7, 60},  // OOO, no reset in memory. O1.
			},
			oooCap: 3,
			expectedSamples: []expectedTsValue{
				{1, 50, histogram.UnknownCounterReset},  // MO1.
				{2, 10, histogram.UnknownCounterReset},  // MO3.
				{3, 20, histogram.NotCounterReset},      // MO3.
				{4, 30, histogram.NotCounterReset},      // MO3.
				{5, 40, histogram.UnknownCounterReset},  // MO2.
				{6, 50, histogram.NotCounterReset},      // MO2.
				{7, 60, histogram.UnknownCounterReset},  // O1.
				{8, 100, histogram.UnknownCounterReset}, // I1.
			},
			expectedChunks: []expectedChunk{
				{histogram.UnknownCounterReset, 1}, // MO1.
				{histogram.UnknownCounterReset, 3}, // MO3.
				{histogram.UnknownCounterReset, 2}, // MO2.
				{histogram.UnknownCounterReset, 1}, // O1.
				{histogram.UnknownCounterReset, 1}, // I1.
			},
		},
	}

	for tcName, tc := range cases {
		t.Run(tcName, func(t *testing.T) {
			opts := DefaultOptions()
			opts.OutOfOrderCapMax = tc.oooCap
			opts.OutOfOrderTimeWindow = 24 * time.Hour.Milliseconds()

			db := newTestDB(t, withOpts(opts))
			db.DisableCompactions()

			app := db.Appender(context.Background())
			for _, s := range tc.samples {
				require.NoError(t, appendFunc(app, s.ts, s.v))
			}
			require.NoError(t, app.Commit())

			t.Run("querier", func(t *testing.T) {
				querier, err := db.Querier(0, 10)
				require.NoError(t, err)
				defer querier.Close()

				seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar1"))
				require.Len(t, seriesSet, 1)
				samples, ok := seriesSet["{foo=\"bar1\"}"]
				require.True(t, ok)
				require.Len(t, samples, len(tc.samples))
				require.Len(t, samples, len(tc.expectedSamples))

				// We expect all unknown counter resets because we clear the counter reset
				// hint when we switch between in-order and out-of-order samples.
				for i, s := range samples {
					switch name {
					case intHistogram:
						require.Equal(t, tc.expectedSamples[i].hint, s.H().CounterResetHint, "sample %d", i)
						require.Equal(t, tc.expectedSamples[i].v, int64(s.H().Count), "sample %d", i)
					case floatHistogram:
						require.Equal(t, tc.expectedSamples[i].hint, s.FH().CounterResetHint, "sample %d", i)
						require.Equal(t, tc.expectedSamples[i].v, int64(s.FH().Count), "sample %d", i)
					case customBucketsIntHistogram:
						require.Equal(t, tc.expectedSamples[i].hint, s.H().CounterResetHint, "sample %d", i)
						require.Equal(t, tc.expectedSamples[i].v, int64(s.H().Count), "sample %d", i)
					case customBucketsFloatHistogram:
						require.Equal(t, tc.expectedSamples[i].hint, s.FH().CounterResetHint, "sample %d", i)
						require.Equal(t, tc.expectedSamples[i].v, int64(s.FH().Count), "sample %d", i)
					default:
						t.Fatalf("unexpected sample type %s", name)
					}
				}
			})

			t.Run("chunk-querier", func(t *testing.T) {
				querier, err := db.ChunkQuerier(0, 10)
				require.NoError(t, err)
				defer querier.Close()

				chunkSet := queryAndExpandChunks(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar1"))
				require.Len(t, chunkSet, 1)
				chunks, ok := chunkSet["{foo=\"bar1\"}"]
				require.True(t, ok)
				require.Len(t, chunks, len(tc.expectedChunks))
				idx := 0
				for i, samples := range chunks {
					require.Len(t, samples, tc.expectedChunks[i].size)
					for j, s := range samples {
						expectHint := tc.expectedChunks[i].hint
						if j > 0 {
							expectHint = histogram.NotCounterReset
						}
						switch name {
						case intHistogram:
							require.Equal(t, expectHint, s.H().CounterResetHint, "sample %d", idx)
							require.Equal(t, tc.expectedSamples[idx].v, int64(s.H().Count), "sample %d", idx)
						case floatHistogram:
							require.Equal(t, expectHint, s.FH().CounterResetHint, "sample %d", idx)
							require.Equal(t, tc.expectedSamples[idx].v, int64(s.FH().Count), "sample %d", idx)
						case customBucketsIntHistogram:
							require.Equal(t, expectHint, s.H().CounterResetHint, "sample %d", idx)
							require.Equal(t, tc.expectedSamples[idx].v, int64(s.H().Count), "sample %d", idx)
						case customBucketsFloatHistogram:
							require.Equal(t, expectHint, s.FH().CounterResetHint, "sample %d", idx)
							require.Equal(t, tc.expectedSamples[idx].v, int64(s.FH().Count), "sample %d", idx)
						default:
							t.Fatalf("unexpected sample type %s", name)
						}
						idx++
					}
				}
			})
		})
	}
}

func TestOOOAppendAndQuery(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOAppendAndQuery(t, scenario)
		})
	}
}

func testOOOAppendAndQuery(t *testing.T, scenario sampleTypeScenario) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 4 * time.Hour.Milliseconds()

	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	s1 := labels.FromStrings("foo", "bar1")
	s2 := labels.FromStrings("foo", "bar2")

	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }
	appendedSamples := make(map[string][]chunks.Sample)
	totalSamples := 0
	addSample := func(lbls labels.Labels, fromMins, toMins int64, faceError bool) {
		app := db.Appender(context.Background())
		key := lbls.String()
		from, to := minutes(fromMins), minutes(toMins)
		for m := from; m <= to; m += time.Minute.Milliseconds() {
			val := rand.Intn(1000)
			_, s, err := scenario.appendFunc(app, lbls, m, int64(val))
			if faceError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				appendedSamples[key] = append(appendedSamples[key], s)
				totalSamples++
			}
		}
		if faceError {
			require.NoError(t, app.Rollback())
		} else {
			require.NoError(t, app.Commit())
		}
	}

	testQuery := func(from, to int64) {
		querier, err := db.Querier(from, to)
		require.NoError(t, err)

		seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar."))

		for k, v := range appendedSamples {
			sort.Slice(v, func(i, j int) bool {
				return v[i].T() < v[j].T()
			})
			appendedSamples[k] = v
		}

		expSamples := make(map[string][]chunks.Sample)
		for k, samples := range appendedSamples {
			for _, s := range samples {
				if s.T() < from {
					continue
				}
				if s.T() > to {
					continue
				}
				expSamples[k] = append(expSamples[k], s)
			}
		}
		requireEqualSeries(t, expSamples, seriesSet, true)
		requireEqualOOOSamples(t, totalSamples-2, db)
	}

	verifyOOOMinMaxTimes := func(expMin, expMax int64) {
		require.Equal(t, minutes(expMin), db.head.MinOOOTime())
		require.Equal(t, minutes(expMax), db.head.MaxOOOTime())
	}

	// In-order samples.
	addSample(s1, 300, 300, false)
	addSample(s2, 290, 290, false)
	require.Equal(t, float64(2), prom_testutil.ToFloat64(db.head.metrics.chunksCreated))
	testQuery(math.MinInt64, math.MaxInt64)

	// Some ooo samples.
	addSample(s1, 250, 260, false)
	addSample(s2, 255, 265, false)
	verifyOOOMinMaxTimes(250, 265)
	testQuery(math.MinInt64, math.MaxInt64)
	testQuery(minutes(250), minutes(265)) // Test querying ooo data time range.
	testQuery(minutes(290), minutes(300)) // Test querying in-order data time range.
	testQuery(minutes(250), minutes(300)) // Test querying the entire range.

	// Out of time window.
	addSample(s1, 59, 59, true)
	addSample(s2, 49, 49, true)
	verifyOOOMinMaxTimes(250, 265)
	testQuery(math.MinInt64, math.MaxInt64)

	// At the edge of time window, also it would be "out of bound" without the ooo support.
	addSample(s1, 60, 65, false)
	verifyOOOMinMaxTimes(60, 265)
	testQuery(math.MinInt64, math.MaxInt64)

	// This sample is not within the time window w.r.t. the head's maxt, but it is within the window
	// w.r.t. the series' maxt. But we consider only head's maxt.
	addSample(s2, 59, 59, true)
	verifyOOOMinMaxTimes(60, 265)
	testQuery(math.MinInt64, math.MaxInt64)

	// Now the sample is within time window w.r.t. the head's maxt.
	addSample(s2, 60, 65, false)
	verifyOOOMinMaxTimes(60, 265)
	testQuery(math.MinInt64, math.MaxInt64)

	// Out of time window again.
	addSample(s1, 59, 59, true)
	addSample(s2, 49, 49, true)
	testQuery(math.MinInt64, math.MaxInt64)

	// Generating some m-map chunks. The m-map chunks here are in such a way
	// that when sorted w.r.t. mint, the last chunk's maxt is not the overall maxt
	// of the merged chunk. This tests a bug fixed in https://github.com/grafana/mimir-prometheus/pull/238/.
	require.Equal(t, float64(4), prom_testutil.ToFloat64(db.head.metrics.chunksCreated))
	addSample(s1, 180, 249, false)
	require.Equal(t, float64(6), prom_testutil.ToFloat64(db.head.metrics.chunksCreated))
	verifyOOOMinMaxTimes(60, 265)
	testQuery(math.MinInt64, math.MaxInt64)
}

func TestOOODisabled(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOODisabled(t, scenario)
		})
	}
}

func testOOODisabled(t *testing.T, scenario sampleTypeScenario) {
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = 0
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	s1 := labels.FromStrings("foo", "bar1")
	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }
	expSamples := make(map[string][]chunks.Sample)
	totalSamples := 0
	failedSamples := 0

	addSample := func(db *DB, lbls labels.Labels, fromMins, toMins int64, faceError bool) {
		app := db.Appender(context.Background())
		key := lbls.String()
		from, to := minutes(fromMins), minutes(toMins)
		for m := from; m <= to; m += time.Minute.Milliseconds() {
			_, _, err := scenario.appendFunc(app, lbls, m, m)
			if faceError {
				require.Error(t, err)
				failedSamples++
			} else {
				require.NoError(t, err)
				expSamples[key] = append(expSamples[key], scenario.sampleFunc(m, m))
				totalSamples++
			}
		}
		if faceError {
			require.NoError(t, app.Rollback())
		} else {
			require.NoError(t, app.Commit())
		}
	}

	addSample(db, s1, 300, 300, false) // In-order samples.
	addSample(db, s1, 250, 260, true)  // Some ooo samples.
	addSample(db, s1, 59, 59, true)    // Out of time window.
	addSample(db, s1, 60, 65, true)    // At the edge of time window, also it would be "out of bound" without the ooo support.
	addSample(db, s1, 59, 59, true)    // Out of time window again.
	addSample(db, s1, 301, 310, false) // More in-order samples.

	querier, err := db.Querier(math.MinInt64, math.MaxInt64)
	require.NoError(t, err)

	seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar."))
	requireEqualSeries(t, expSamples, seriesSet, true)
	requireEqualOOOSamples(t, 0, db)
	require.Equal(t, float64(failedSamples),
		prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(scenario.sampleType))+prom_testutil.ToFloat64(db.head.metrics.outOfBoundSamples.WithLabelValues(scenario.sampleType)),
		"number of ooo/oob samples mismatch")

	// Verifying that no OOO artifacts were generated.
	_, err = os.ReadDir(path.Join(db.Dir(), wlog.WblDirName))
	require.True(t, os.IsNotExist(err))

	ms, created, err := db.head.getOrCreate(s1.Hash(), s1, false)
	require.NoError(t, err)
	require.False(t, created)
	require.NotNil(t, ms)
	require.Nil(t, ms.ooo)
}

func TestWBLAndMmapReplay(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testWBLAndMmapReplay(t, scenario)
		})
	}
}

func testWBLAndMmapReplay(t *testing.T, scenario sampleTypeScenario) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 4 * time.Hour.Milliseconds()

	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	s1 := labels.FromStrings("foo", "bar1")

	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }
	expSamples := make(map[string][]chunks.Sample)
	totalSamples := 0
	addSample := func(lbls labels.Labels, fromMins, toMins int64) {
		app := db.Appender(context.Background())
		key := lbls.String()
		from, to := minutes(fromMins), minutes(toMins)
		for m := from; m <= to; m += time.Minute.Milliseconds() {
			val := rand.Intn(1000)
			_, s, err := scenario.appendFunc(app, lbls, m, int64(val))
			require.NoError(t, err)
			expSamples[key] = append(expSamples[key], s)
			totalSamples++
		}
		require.NoError(t, app.Commit())
	}

	testQuery := func(exp map[string][]chunks.Sample) {
		querier, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar."))

		for k, v := range exp {
			sort.Slice(v, func(i, j int) bool {
				return v[i].T() < v[j].T()
			})
			exp[k] = v
		}
		requireEqualSeries(t, exp, seriesSet, true)
	}

	// In-order samples.
	addSample(s1, 300, 300)
	require.Equal(t, float64(1), prom_testutil.ToFloat64(db.head.metrics.chunksCreated))

	// Some ooo samples.
	addSample(s1, 250, 260)
	addSample(s1, 195, 249) // This creates some m-map chunks.
	require.Equal(t, float64(4), prom_testutil.ToFloat64(db.head.metrics.chunksCreated))
	testQuery(expSamples)
	oooMint, oooMaxt := minutes(195), minutes(260)

	// Collect the samples only present in the ooo m-map chunks.
	ms, created, err := db.head.getOrCreate(s1.Hash(), s1, false)
	require.False(t, created)
	require.NoError(t, err)
	var s1MmapSamples []chunks.Sample
	for _, mc := range ms.ooo.oooMmappedChunks {
		chk, err := db.head.chunkDiskMapper.Chunk(mc.ref)
		require.NoError(t, err)
		it := chk.Iterator(nil)
		smpls, err := storage.ExpandSamples(it, newSample)
		require.NoError(t, err)
		s1MmapSamples = append(s1MmapSamples, smpls...)
	}
	require.NotEmpty(t, s1MmapSamples)

	require.NoError(t, db.Close())

	// Making a copy of original state of WBL and Mmap files to use it later.
	mmapDir := mmappedChunksDir(db.head.opts.ChunkDirRoot)
	wblDir := db.head.wbl.Dir()
	originalWblDir := filepath.Join(t.TempDir(), "original_wbl")
	originalMmapDir := filepath.Join(t.TempDir(), "original_mmap")
	require.NoError(t, fileutil.CopyDirs(wblDir, originalWblDir))
	require.NoError(t, fileutil.CopyDirs(mmapDir, originalMmapDir))
	resetWBLToOriginal := func() {
		require.NoError(t, os.RemoveAll(wblDir))
		require.NoError(t, fileutil.CopyDirs(originalWblDir, wblDir))
	}
	resetMmapToOriginal := func() {
		require.NoError(t, os.RemoveAll(mmapDir))
		require.NoError(t, fileutil.CopyDirs(originalMmapDir, mmapDir))
	}

	t.Run("Restart DB with both WBL and M-map files for ooo data", func(t *testing.T) {
		db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
		require.Equal(t, oooMint, db.head.MinOOOTime())
		require.Equal(t, oooMaxt, db.head.MaxOOOTime())
		testQuery(expSamples)
	})

	t.Run("Restart DB with only WBL for ooo data", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(mmapDir))

		db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
		require.Equal(t, oooMint, db.head.MinOOOTime())
		require.Equal(t, oooMaxt, db.head.MaxOOOTime())
		testQuery(expSamples)
	})

	t.Run("Restart DB with only M-map files for ooo data", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(wblDir))
		resetMmapToOriginal()

		db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
		require.Equal(t, oooMint, db.head.MinOOOTime())
		require.Equal(t, oooMaxt, db.head.MaxOOOTime())
		inOrderSample := expSamples[s1.String()][len(expSamples[s1.String()])-1]
		testQuery(map[string][]chunks.Sample{
			s1.String(): append(s1MmapSamples, inOrderSample),
		})
	})

	t.Run("Restart DB with WBL+Mmap while increasing the OOOCapMax", func(t *testing.T) {
		resetWBLToOriginal()
		resetMmapToOriginal()

		opts.OutOfOrderCapMax = 60
		db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
		require.NoError(t, err)
		require.Equal(t, oooMint, db.head.MinOOOTime())
		require.Equal(t, oooMaxt, db.head.MaxOOOTime())
		testQuery(expSamples)
	})

	t.Run("Restart DB with WBL+Mmap while decreasing the OOOCapMax", func(t *testing.T) {
		resetMmapToOriginal() // We need to reset because new duplicate chunks can be written above.

		opts.OutOfOrderCapMax = 10
		db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
		require.NoError(t, err)
		require.Equal(t, oooMint, db.head.MinOOOTime())
		require.Equal(t, oooMaxt, db.head.MaxOOOTime())
		testQuery(expSamples)
	})

	t.Run("Restart DB with WBL+Mmap while having no m-map markers in WBL", func(t *testing.T) {
		resetMmapToOriginal() // We neet to reset because new duplicate chunks can be written above.

		// Removing m-map markers in WBL by rewriting it.
		newWbl, err := wlog.New(promslog.NewNopLogger(), nil, filepath.Join(t.TempDir(), "new_wbl"), compression.None)
		require.NoError(t, err)
		sr, err := wlog.NewSegmentsReader(originalWblDir)
		require.NoError(t, err)
		dec := record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
		r, markers, addedRecs := wlog.NewReader(sr), 0, 0
		for r.Next() {
			rec := r.Record()
			if dec.Type(rec) == record.MmapMarkers {
				markers++
				continue
			}
			addedRecs++
			require.NoError(t, newWbl.Log(rec))
		}
		require.Positive(t, markers)
		require.Positive(t, addedRecs)
		require.NoError(t, newWbl.Close())
		require.NoError(t, sr.Close())
		require.NoError(t, os.RemoveAll(wblDir))
		require.NoError(t, os.Rename(newWbl.Dir(), wblDir))

		opts.OutOfOrderCapMax = 30
		db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
		require.NoError(t, err)
		require.Equal(t, oooMint, db.head.MinOOOTime())
		require.Equal(t, oooMaxt, db.head.MaxOOOTime())
		testQuery(expSamples)
	})
}

func TestOOOHistogramCompactionWithCounterResets(t *testing.T) {
	for _, floatHistogram := range []bool{false, true} {
		ctx := context.Background()

		opts := DefaultOptions()
		opts.OutOfOrderCapMax = 30
		opts.OutOfOrderTimeWindow = 500 * time.Minute.Milliseconds()

		db := newTestDB(t, withOpts(opts))
		db.DisableCompactions() // We want to manually call it.

		series1 := labels.FromStrings("foo", "bar1")
		series2 := labels.FromStrings("foo", "bar2")

		var series1ExpSamplesPreCompact, series2ExpSamplesPreCompact, series1ExpSamplesPostCompact, series2ExpSamplesPostCompact []chunks.Sample

		addSample := func(ts int64, l labels.Labels, val int, hint histogram.CounterResetHint) sample {
			app := db.Appender(context.Background())
			tsMs := ts * time.Minute.Milliseconds()
			if floatHistogram {
				h := tsdbutil.GenerateTestFloatHistogram(int64(val))
				h.CounterResetHint = hint
				_, err := app.AppendHistogram(0, l, tsMs, nil, h)
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				return sample{t: tsMs, fh: h.Copy()}
			}

			h := tsdbutil.GenerateTestHistogram(int64(val))
			h.CounterResetHint = hint
			_, err := app.AppendHistogram(0, l, tsMs, h, nil)
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			return sample{t: tsMs, h: h.Copy()}
		}

		// Add an in-order sample to each series.
		s := addSample(520, series1, 1000000, histogram.UnknownCounterReset)
		series1ExpSamplesPreCompact = append(series1ExpSamplesPreCompact, s)
		series1ExpSamplesPostCompact = append(series1ExpSamplesPostCompact, s)

		s = addSample(520, series2, 1000000, histogram.UnknownCounterReset)
		series2ExpSamplesPreCompact = append(series2ExpSamplesPreCompact, s)
		series2ExpSamplesPostCompact = append(series2ExpSamplesPostCompact, s)

		// Verify that the in-memory ooo chunk is empty.
		checkEmptyOOOChunk := func(lbls labels.Labels) {
			ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls, false)
			require.NoError(t, err)
			require.False(t, created)
			require.Nil(t, ms.ooo)
		}

		checkEmptyOOOChunk(series1)
		checkEmptyOOOChunk(series2)

		// Add samples for series1. There are three head chunks that will be created:
		// Chunk 1 - Samples between 100 - 440. One explicit counter reset at ts 250.
		// Chunk 2 - Samples between 105 - 395. Overlaps with Chunk 1. One detected counter reset at ts 165.
		// Chunk 3 - Samples between 480 - 509. All within one block boundary. One detected counter reset at 490.

		// Chunk 1.
		// First add 10 samples.
		for i := 100; i < 200; i += 10 {
			s = addSample(int64(i), series1, 100000+i, histogram.UnknownCounterReset)
			// Before compaction, all the samples have UnknownCounterReset even though they've been added to the same
			// chunk. This is because they overlap with the samples from chunk two and when merging two chunks on read,
			// the header is set as unknown when the next sample is not in the same chunk as the previous one.
			series1ExpSamplesPreCompact = append(series1ExpSamplesPreCompact, s)
			// After compaction, samples from multiple mmapped chunks will be merged, so there won't be any overlapping
			// chunks. Therefore, most samples will have the NotCounterReset header.
			// 100 is the first sample in the first chunk in the blocks, so is still set to UnknownCounterReset.
			// 120 is a block boundary - after compaction, 120 will be the first sample in a chunk, so is still set to
			// UnknownCounterReset.
			if i > 100 && i != 120 {
				s = copyWithCounterReset(s, histogram.NotCounterReset)
			}
			series1ExpSamplesPostCompact = append(series1ExpSamplesPostCompact, s)
		}
		// Explicit counter reset - the counter reset header is set to CounterReset but the value is higher
		// than for the previous timestamp. Explicit counter reset headers are actually ignored though, so when reading
		// the sample back you actually get unknown/not counter reset. This is as the chainSampleIterator ignores
		// existing headers and sets the header as UnknownCounterReset if the next sample is not in the same chunk as
		// the previous one, and counter resets always create a new chunk.
		// This case has been added to document what's happening, though it might not be the ideal behavior.
		s = addSample(250, series1, 100000+250, histogram.CounterReset)
		series1ExpSamplesPreCompact = append(series1ExpSamplesPreCompact, copyWithCounterReset(s, histogram.UnknownCounterReset))
		series1ExpSamplesPostCompact = append(series1ExpSamplesPostCompact, copyWithCounterReset(s, histogram.NotCounterReset))

		// Add 19 more samples to complete a chunk.
		for i := 260; i < 450; i += 10 {
			s = addSample(int64(i), series1, 100000+i, histogram.UnknownCounterReset)
			// The samples with timestamp less than 410 overlap with the samples from chunk 2, so before compaction,
			// they're all UnknownCounterReset. Samples greater than or equal to 410 don't overlap with other chunks
			// so they're always detected as NotCounterReset pre and post compaction.
			if i >= 410 {
				s = copyWithCounterReset(s, histogram.NotCounterReset)
			}
			series1ExpSamplesPreCompact = append(series1ExpSamplesPreCompact, s)
			//
			// 360 is a block boundary, so after compaction its header is still UnknownCounterReset.
			if i != 360 {
				s = copyWithCounterReset(s, histogram.NotCounterReset)
			}
			series1ExpSamplesPostCompact = append(series1ExpSamplesPostCompact, s)
		}

		// Chunk 2.
		// Add six OOO samples.
		for i := 105; i < 165; i += 10 {
			s = addSample(int64(i), series1, 100000+i, histogram.UnknownCounterReset)
			// Samples overlap with chunk 1 so before compaction all headers are UnknownCounterReset.
			series1ExpSamplesPreCompact = append(series1ExpSamplesPreCompact, s)
			series1ExpSamplesPostCompact = append(series1ExpSamplesPostCompact, copyWithCounterReset(s, histogram.NotCounterReset))
		}

		// Add sample that will be detected as a counter reset.
		s = addSample(165, series1, 100000, histogram.UnknownCounterReset)
		// Before compaction, sample has an UnknownCounterReset header due to the chainSampleIterator.
		series1ExpSamplesPreCompact = append(series1ExpSamplesPreCompact, s)
		// After compaction, the sample's counter reset is still UnknownCounterReset as we cannot trust CounterReset
		// headers in chunks at the moment, so when reading the first sample in a chunk, its hint is set to
		// UnknownCounterReset.
		series1ExpSamplesPostCompact = append(series1ExpSamplesPostCompact, s)

		// Add 23 more samples to complete a chunk.
		for i := 175; i < 405; i += 10 {
			s = addSample(int64(i), series1, 100000+i, histogram.UnknownCounterReset)
			// Samples between 205-255 overlap with chunk 1 so before compaction those samples will have the
			// UnknownCounterReset header.
			if i >= 205 && i < 255 {
				s = copyWithCounterReset(s, histogram.NotCounterReset)
			}
			series1ExpSamplesPreCompact = append(series1ExpSamplesPreCompact, s)
			// 245 is the first sample >= the block boundary at 240, so it's still UnknownCounterReset after compaction.
			if i != 245 {
				s = copyWithCounterReset(s, histogram.NotCounterReset)
			} else {
				s = copyWithCounterReset(s, histogram.UnknownCounterReset)
			}
			series1ExpSamplesPostCompact = append(series1ExpSamplesPostCompact, s)
		}

		// Chunk 3.
		for i := 480; i < 490; i++ {
			s = addSample(int64(i), series1, 100000+i, histogram.UnknownCounterReset)
			// No overlapping samples in other chunks, so all other samples will already be detected as NotCounterReset
			// before compaction.
			if i > 480 {
				s = copyWithCounterReset(s, histogram.NotCounterReset)
			}
			series1ExpSamplesPreCompact = append(series1ExpSamplesPreCompact, s)
			// 480 is block boundary.
			if i == 480 {
				s = copyWithCounterReset(s, histogram.UnknownCounterReset)
			}
			series1ExpSamplesPostCompact = append(series1ExpSamplesPostCompact, s)
		}
		// Counter reset.
		s = addSample(int64(490), series1, 100000, histogram.UnknownCounterReset)
		series1ExpSamplesPreCompact = append(series1ExpSamplesPreCompact, s)
		series1ExpSamplesPostCompact = append(series1ExpSamplesPostCompact, s)
		// Add some more samples after the counter reset.
		for i := 491; i < 510; i++ {
			s = addSample(int64(i), series1, 100000+i, histogram.UnknownCounterReset)
			s = copyWithCounterReset(s, histogram.NotCounterReset)
			series1ExpSamplesPreCompact = append(series1ExpSamplesPreCompact, s)
			series1ExpSamplesPostCompact = append(series1ExpSamplesPostCompact, s)
		}

		// Add samples for series2 - one chunk with one detected counter reset at 300.
		for i := 200; i < 300; i += 10 {
			s = addSample(int64(i), series2, 100000+i, histogram.UnknownCounterReset)
			if i > 200 {
				s = copyWithCounterReset(s, histogram.NotCounterReset)
			}
			series2ExpSamplesPreCompact = append(series2ExpSamplesPreCompact, s)
			if i == 240 {
				s = copyWithCounterReset(s, histogram.UnknownCounterReset)
			}
			series2ExpSamplesPostCompact = append(series2ExpSamplesPostCompact, s)
		}
		// Counter reset.
		s = addSample(int64(300), series2, 100000, histogram.UnknownCounterReset)
		series2ExpSamplesPreCompact = append(series2ExpSamplesPreCompact, s)
		series2ExpSamplesPostCompact = append(series2ExpSamplesPostCompact, s)
		// Add some more samples after the counter reset.
		for i := 310; i < 500; i += 10 {
			s := addSample(int64(i), series2, 100000+i, histogram.UnknownCounterReset)
			s = copyWithCounterReset(s, histogram.NotCounterReset)
			series2ExpSamplesPreCompact = append(series2ExpSamplesPreCompact, s)
			// 360 and 480 are block boundaries.
			if i == 360 || i == 480 {
				s = copyWithCounterReset(s, histogram.UnknownCounterReset)
			}
			series2ExpSamplesPostCompact = append(series2ExpSamplesPostCompact, s)
		}

		// Sort samples (as OOO samples not added in time-order).
		sort.Slice(series1ExpSamplesPreCompact, func(i, j int) bool {
			return series1ExpSamplesPreCompact[i].T() < series1ExpSamplesPreCompact[j].T()
		})
		sort.Slice(series1ExpSamplesPostCompact, func(i, j int) bool {
			return series1ExpSamplesPostCompact[i].T() < series1ExpSamplesPostCompact[j].T()
		})
		sort.Slice(series2ExpSamplesPreCompact, func(i, j int) bool {
			return series2ExpSamplesPreCompact[i].T() < series2ExpSamplesPreCompact[j].T()
		})
		sort.Slice(series2ExpSamplesPostCompact, func(i, j int) bool {
			return series2ExpSamplesPostCompact[i].T() < series2ExpSamplesPostCompact[j].T()
		})

		verifyDBSamples := func(s1Samples, s2Samples []chunks.Sample) {
			expRes := map[string][]chunks.Sample{
				series1.String(): s1Samples,
				series2.String(): s2Samples,
			}

			q, err := db.Querier(math.MinInt64, math.MaxInt64)
			require.NoError(t, err)
			actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
			requireEqualSeries(t, expRes, actRes, false)
		}

		// Verify DB samples before compaction.
		verifyDBSamples(series1ExpSamplesPreCompact, series2ExpSamplesPreCompact)

		// Verify that the in-memory ooo chunk is not empty.
		checkNonEmptyOOOChunk := func(lbls labels.Labels) {
			ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls, false)
			require.NoError(t, err)
			require.False(t, created)
			require.Positive(t, ms.ooo.oooHeadChunk.chunk.NumSamples())
		}

		checkNonEmptyOOOChunk(series1)
		checkNonEmptyOOOChunk(series2)

		// No blocks before compaction.
		require.Empty(t, db.Blocks())

		// There is a 0th WBL file.
		require.NoError(t, db.head.wbl.Sync()) // syncing to make sure wbl is flushed in windows
		files, err := os.ReadDir(db.head.wbl.Dir())
		require.NoError(t, err)
		require.Len(t, files, 1)
		require.Equal(t, "00000000", files[0].Name())
		f, err := files[0].Info()
		require.NoError(t, err)
		require.Greater(t, f.Size(), int64(100))

		// OOO compaction happens here.
		require.NoError(t, db.CompactOOOHead(ctx))

		// Check that blocks are created after compaction.
		require.Len(t, db.Blocks(), 5)

		// Check samples after compaction.
		verifyDBSamples(series1ExpSamplesPostCompact, series2ExpSamplesPostCompact)

		// 0th WBL file will be deleted and 1st will be the only present.
		files, err = os.ReadDir(db.head.wbl.Dir())
		require.NoError(t, err)
		require.Len(t, files, 1)
		require.Equal(t, "00000001", files[0].Name())
		f, err = files[0].Info()
		require.NoError(t, err)
		require.Equal(t, int64(0), f.Size())

		// OOO stuff should not be present in the Head now.
		checkEmptyOOOChunk(series1)
		checkEmptyOOOChunk(series2)

		verifyBlockSamples := func(block *Block, fromMins, toMins int64) {
			var series1Samples, series2Samples []chunks.Sample

			for _, s := range series1ExpSamplesPostCompact {
				if s.T() >= fromMins*time.Minute.Milliseconds() {
					// Samples should be sorted, so break out of loop when we reach a timestamp that's too big.
					if s.T() > toMins*time.Minute.Milliseconds() {
						break
					}
					series1Samples = append(series1Samples, s)
				}
			}
			for _, s := range series2ExpSamplesPostCompact {
				if s.T() >= fromMins*time.Minute.Milliseconds() {
					// Samples should be sorted, so break out of loop when we reach a timestamp that's too big.
					if s.T() > toMins*time.Minute.Milliseconds() {
						break
					}
					series2Samples = append(series2Samples, s)
				}
			}

			expRes := map[string][]chunks.Sample{}
			if len(series1Samples) != 0 {
				expRes[series1.String()] = series1Samples
			}
			if len(series2Samples) != 0 {
				expRes[series2.String()] = series2Samples
			}

			q, err := NewBlockQuerier(block, math.MinInt64, math.MaxInt64)
			require.NoError(t, err)

			actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
			requireEqualSeries(t, expRes, actRes, false)
		}

		// Checking for expected data in the blocks.
		verifyBlockSamples(db.Blocks()[0], 100, 119)
		verifyBlockSamples(db.Blocks()[1], 120, 239)
		verifyBlockSamples(db.Blocks()[2], 240, 359)
		verifyBlockSamples(db.Blocks()[3], 360, 479)
		verifyBlockSamples(db.Blocks()[4], 480, 509)

		// There should be a single m-map file.
		mmapDir := mmappedChunksDir(db.head.opts.ChunkDirRoot)
		files, err = os.ReadDir(mmapDir)
		require.NoError(t, err)
		require.Len(t, files, 1)

		// Compact the in-order head and expect another block.
		// Since this is a forced compaction, this block is not aligned with 2h.
		err = db.CompactHead(NewRangeHead(db.head, 500*time.Minute.Milliseconds(), 550*time.Minute.Milliseconds()))
		require.NoError(t, err)
		require.Len(t, db.Blocks(), 6)
		verifyBlockSamples(db.Blocks()[5], 520, 520)

		// Blocks created out of normal and OOO head now. But not merged.
		verifyDBSamples(series1ExpSamplesPostCompact, series2ExpSamplesPostCompact)

		// The compaction also clears out the old m-map files. Including
		// the file that has ooo chunks.
		files, err = os.ReadDir(mmapDir)
		require.NoError(t, err)
		require.Len(t, files, 1)
		require.Equal(t, "000001", files[0].Name())

		// This will merge overlapping block.
		require.NoError(t, db.Compact(ctx))

		require.Len(t, db.Blocks(), 5)
		verifyBlockSamples(db.Blocks()[0], 100, 119)
		verifyBlockSamples(db.Blocks()[1], 120, 239)
		verifyBlockSamples(db.Blocks()[2], 240, 359)
		verifyBlockSamples(db.Blocks()[3], 360, 479)
		verifyBlockSamples(db.Blocks()[4], 480, 520) // Merged block.

		// Final state. Blocks from normal and OOO head are merged.
		verifyDBSamples(series1ExpSamplesPostCompact, series2ExpSamplesPostCompact)
	}
}

func TestInterleavedInOrderAndOOOHistogramCompactionWithCounterResets(t *testing.T) {
	for _, floatHistogram := range []bool{false, true} {
		ctx := context.Background()

		opts := DefaultOptions()
		opts.OutOfOrderCapMax = 30
		opts.OutOfOrderTimeWindow = 500 * time.Minute.Milliseconds()

		db := newTestDB(t, withOpts(opts))
		db.DisableCompactions() // We want to manually call it.

		series1 := labels.FromStrings("foo", "bar1")

		addSample := func(ts int64, l labels.Labels, val int) sample {
			app := db.Appender(context.Background())
			tsMs := ts
			if floatHistogram {
				h := tsdbutil.GenerateTestFloatHistogram(int64(val))
				_, err := app.AppendHistogram(0, l, tsMs, nil, h)
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				return sample{t: tsMs, fh: h.Copy()}
			}

			h := tsdbutil.GenerateTestHistogram(int64(val))
			_, err := app.AppendHistogram(0, l, tsMs, h, nil)
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			return sample{t: tsMs, h: h.Copy()}
		}

		var expSamples []chunks.Sample

		s := addSample(0, series1, 0)
		expSamples = append(expSamples, s)
		s = addSample(1, series1, 10)
		expSamples = append(expSamples, copyWithCounterReset(s, histogram.NotCounterReset))
		s = addSample(3, series1, 3)
		expSamples = append(expSamples, copyWithCounterReset(s, histogram.UnknownCounterReset))
		s = addSample(2, series1, 0)
		expSamples = append(expSamples, copyWithCounterReset(s, histogram.UnknownCounterReset))

		// Sort samples (as OOO samples not added in time-order).
		sort.Slice(expSamples, func(i, j int) bool {
			return expSamples[i].T() < expSamples[j].T()
		})

		verifyDBSamples := func(s1Samples []chunks.Sample) {
			t.Helper()
			expRes := map[string][]chunks.Sample{
				series1.String(): s1Samples,
			}

			q, err := db.Querier(math.MinInt64, math.MaxInt64)
			require.NoError(t, err)
			actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
			requireEqualSeries(t, expRes, actRes, false)
		}

		// Verify DB samples before compaction.
		verifyDBSamples(expSamples)

		require.NoError(t, db.CompactOOOHead(ctx))

		// Check samples after OOO compaction.
		verifyDBSamples(expSamples)

		// Checking for expected data in the blocks.
		// Check that blocks are created after compaction.
		require.Len(t, db.Blocks(), 1)

		// Compact the in-order head and expect another block.
		// Since this is a forced compaction, this block is not aligned with 2h.
		require.NoError(t, db.CompactHead(NewRangeHead(db.head, 0, 3)))
		require.Len(t, db.Blocks(), 2)

		// Blocks created out of normal and OOO head now. But not merged.
		verifyDBSamples(expSamples)

		// This will merge overlapping block.
		require.NoError(t, db.Compact(ctx))

		require.Len(t, db.Blocks(), 1)

		// Final state. Blocks from normal and OOO head are merged.
		verifyDBSamples(expSamples)
	}
}

func copyWithCounterReset(s sample, hint histogram.CounterResetHint) sample {
	if s.h != nil {
		h := s.h.Copy()
		h.CounterResetHint = hint
		return sample{t: s.t, h: h}
	}

	h := s.fh.Copy()
	h.CounterResetHint = hint
	return sample{t: s.t, fh: h}
}

func TestOOOCompactionFailure(t *testing.T) {
	t.Parallel()
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOCompactionFailure(t, scenario)
		})
	}
}

func testOOOCompactionFailure(t *testing.T, scenario sampleTypeScenario) {
	ctx := context.Background()

	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions() // We want to manually call it.

	series1 := labels.FromStrings("foo", "bar1")

	addSample := func(fromMins, toMins int64) {
		app := db.Appender(context.Background())
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			_, _, err := scenario.appendFunc(app, series1, ts, ts)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Add an in-order samples.
	addSample(250, 350)

	// Add ooo samples that creates multiple chunks.
	addSample(90, 310)

	// No blocks before compaction.
	require.Empty(t, db.Blocks())

	// There is a 0th WBL file.
	verifyFirstWBLFileIs0 := func(count int) {
		require.NoError(t, db.head.wbl.Sync()) // Syncing to make sure wbl is flushed in windows.
		files, err := os.ReadDir(db.head.wbl.Dir())
		require.NoError(t, err)
		require.Len(t, files, count)
		require.Equal(t, "00000000", files[0].Name())
		f, err := files[0].Info()
		require.NoError(t, err)
		require.Greater(t, f.Size(), int64(100))
	}
	verifyFirstWBLFileIs0(1)

	verifyMmapFiles := func(exp ...string) {
		mmapDir := mmappedChunksDir(db.head.opts.ChunkDirRoot)
		files, err := os.ReadDir(mmapDir)
		require.NoError(t, err)
		require.Len(t, files, len(exp))
		for i, f := range files {
			require.Equal(t, exp[i], f.Name())
		}
	}

	verifyMmapFiles("000001")

	// OOO compaction fails 5 times.
	originalCompactor := db.compactor
	db.compactor = &mockCompactorFailing{t: t}
	for range 5 {
		require.Error(t, db.CompactOOOHead(ctx))
	}
	require.Empty(t, db.Blocks())

	// M-map files don't change after failed compaction.
	verifyMmapFiles("000001")

	// Because of 5 compaction attempts, there are 6 files now.
	verifyFirstWBLFileIs0(6)

	db.compactor = originalCompactor
	require.NoError(t, db.CompactOOOHead(ctx))
	oldBlocks := db.Blocks()
	require.Len(t, db.Blocks(), 3)

	// Check that the ooo chunks were removed.
	ms, created, err := db.head.getOrCreate(series1.Hash(), series1, false)
	require.NoError(t, err)
	require.False(t, created)
	require.Nil(t, ms.ooo)

	// The failed compaction should not have left the ooo Head corrupted.
	// Hence, expect no new blocks with another OOO compaction call.
	require.NoError(t, db.CompactOOOHead(ctx))
	require.Len(t, db.Blocks(), 3)
	require.Equal(t, oldBlocks, db.Blocks())

	// There should be a single m-map file.
	verifyMmapFiles("000001")

	// All but last WBL file will be deleted.
	// 8 files in total (starting at 0) because of 7 compaction calls.
	files, err := os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "00000007", files[0].Name())
	f, err := files[0].Info()
	require.NoError(t, err)
	require.Equal(t, int64(0), f.Size())

	verifySamples := func(block *Block, fromMins, toMins int64) {
		series1Samples := make([]chunks.Sample, 0, toMins-fromMins+1)
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			series1Samples = append(series1Samples, scenario.sampleFunc(ts, ts))
		}
		expRes := map[string][]chunks.Sample{
			series1.String(): series1Samples,
		}

		q, err := NewBlockQuerier(block, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		requireEqualSeries(t, expRes, actRes, true)
	}

	// Checking for expected data in the blocks.
	verifySamples(db.Blocks()[0], 90, 119)
	verifySamples(db.Blocks()[1], 120, 239)
	verifySamples(db.Blocks()[2], 240, 310)

	// Compact the in-order head and expect another block.
	// Since this is a forced compaction, this block is not aligned with 2h.
	err = db.CompactHead(NewRangeHead(db.head, 250*time.Minute.Milliseconds(), 350*time.Minute.Milliseconds()))
	require.NoError(t, err)
	require.Len(t, db.Blocks(), 4) // [0, 120), [120, 240), [240, 360), [250, 351)
	verifySamples(db.Blocks()[3], 250, 350)

	// The compaction also clears out the old m-map files. Including
	// the file that has ooo chunks.
	verifyMmapFiles("000001")
}

func TestWBLCorruption(t *testing.T) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()

	db := newTestDB(t, withOpts(opts))

	series1 := labels.FromStrings("foo", "bar1")
	var allSamples, expAfterRestart []chunks.Sample
	addSamples := func(fromMins, toMins int64, afterRestart bool) {
		app := db.Appender(context.Background())
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			_, err := app.Append(0, series1, ts, float64(ts))
			require.NoError(t, err)
			allSamples = append(allSamples, sample{t: ts, f: float64(ts)})
			if afterRestart {
				expAfterRestart = append(expAfterRestart, sample{t: ts, f: float64(ts)})
			}
		}
		require.NoError(t, app.Commit())
	}

	// Add an in-order samples.
	addSamples(340, 350, true)

	// OOO samples.
	addSamples(90, 99, true)
	addSamples(100, 119, true)
	addSamples(120, 130, true)

	// Moving onto the second file.
	_, err := db.head.wbl.NextSegment()
	require.NoError(t, err)

	// More OOO samples.
	addSamples(200, 230, true)
	addSamples(240, 255, true)

	// We corrupt WBL after the sample at 255. So everything added later
	// should be deleted after replay.

	// Checking where we corrupt it.
	require.NoError(t, db.head.wbl.Sync()) // Syncing to make sure wbl is flushed in windows.
	files, err := os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 2)
	f1, err := files[1].Info()
	require.NoError(t, err)
	corruptIndex := f1.Size()
	corruptFilePath := path.Join(db.head.wbl.Dir(), files[1].Name())

	// Corrupt the WBL by adding a malformed record.
	require.NoError(t, db.head.wbl.Log([]byte{byte(record.Samples), 99, 9, 99, 9, 99, 9, 99}))

	// More samples after the corruption point.
	addSamples(260, 280, false)
	addSamples(290, 300, false)

	// Another file.
	_, err = db.head.wbl.NextSegment()
	require.NoError(t, err)

	addSamples(310, 320, false)

	// Verifying that we have data after corruption point.
	require.NoError(t, db.head.wbl.Sync()) // Syncing to make sure wbl is flushed in windows.
	files, err = os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 3)
	f1, err = files[1].Info()
	require.NoError(t, err)
	require.Greater(t, f1.Size(), corruptIndex)
	f0, err := files[0].Info()
	require.NoError(t, err)
	require.Greater(t, f0.Size(), int64(100))
	f2, err := files[2].Info()
	require.NoError(t, err)
	require.Greater(t, f2.Size(), int64(100))

	verifySamples := func(expSamples []chunks.Sample) {
		sort.Slice(expSamples, func(i, j int) bool {
			return expSamples[i].T() < expSamples[j].T()
		})

		expRes := map[string][]chunks.Sample{
			series1.String(): expSamples,
		}

		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		require.Equal(t, expRes, actRes)
	}

	verifySamples(allSamples)

	require.NoError(t, db.Close())

	// We want everything to be replayed from the WBL. So we delete the m-map files.
	require.NoError(t, os.RemoveAll(mmappedChunksDir(db.head.opts.ChunkDirRoot)))

	// Restart does the replay and repair.
	db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
	require.NoError(t, err)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal))
	require.Less(t, len(expAfterRestart), len(allSamples))
	verifySamples(expAfterRestart)

	// Verify that it did the repair on disk.
	files, err = os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 3)
	f0, err = files[0].Info()
	require.NoError(t, err)
	require.Greater(t, f0.Size(), int64(100))
	f2, err = files[2].Info()
	require.NoError(t, err)
	require.Equal(t, int64(0), f2.Size())
	require.Equal(t, corruptFilePath, path.Join(db.head.wbl.Dir(), files[1].Name()))

	// Verifying that everything after the corruption point is set to 0.
	b, err := os.ReadFile(corruptFilePath)
	require.NoError(t, err)
	sum := 0
	for _, val := range b[corruptIndex:] {
		sum += int(val)
	}
	require.Equal(t, 0, sum)

	// Another restart, everything normal with no repair.
	require.NoError(t, db.Close())
	db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
	require.NoError(t, err)
	require.Equal(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal))
	verifySamples(expAfterRestart)
}

func TestOOOMmapCorruption(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOMmapCorruption(t, scenario)
		})
	}
}

func testOOOMmapCorruption(t *testing.T, scenario sampleTypeScenario) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 10
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()

	db := newTestDB(t, withOpts(opts))

	series1 := labels.FromStrings("foo", "bar1")
	var allSamples, expInMmapChunks []chunks.Sample
	addSamples := func(fromMins, toMins int64, inMmapAfterCorruption bool) {
		app := db.Appender(context.Background())
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			_, s, err := scenario.appendFunc(app, series1, ts, ts)
			require.NoError(t, err)
			allSamples = append(allSamples, s)
			if inMmapAfterCorruption {
				expInMmapChunks = append(expInMmapChunks, s)
			}
		}
		require.NoError(t, app.Commit())
	}

	// Add an in-order samples.
	addSamples(340, 350, true)

	// OOO samples.
	addSamples(90, 99, true)
	addSamples(100, 109, true)
	// This sample m-maps a chunk. But 120 goes into a new chunk.
	addSamples(120, 120, false)

	// Second m-map file. We will corrupt this file. Sample 120 goes into this new file.
	db.head.chunkDiskMapper.CutNewFile()

	// More OOO samples.
	addSamples(200, 230, false)
	addSamples(240, 255, false)

	db.head.chunkDiskMapper.CutNewFile()
	addSamples(260, 290, false)

	verifySamples := func(expSamples []chunks.Sample) {
		sort.Slice(expSamples, func(i, j int) bool {
			return expSamples[i].T() < expSamples[j].T()
		})

		expRes := map[string][]chunks.Sample{
			series1.String(): expSamples,
		}

		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		requireEqualSeries(t, expRes, actRes, true)
	}

	verifySamples(allSamples)

	// Verifying existing files.
	mmapDir := mmappedChunksDir(db.head.opts.ChunkDirRoot)
	files, err := os.ReadDir(mmapDir)
	require.NoError(t, err)
	require.Len(t, files, 3)

	// Corrupting the 2nd file.
	f, err := os.OpenFile(path.Join(mmapDir, files[1].Name()), os.O_RDWR, 0o666)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{99, 9, 99, 9, 99}, 20)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	firstFileName := files[0].Name()

	require.NoError(t, db.Close())

	// Moving OOO WBL to use it later.
	wblDir := db.head.wbl.Dir()
	wblDirTmp := path.Join(t.TempDir(), "wbl_tmp")
	require.NoError(t, os.Rename(wblDir, wblDirTmp))

	// Restart does the replay and repair of m-map files.
	db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
	require.NoError(t, err)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.mmapChunkCorruptionTotal))
	require.Less(t, len(expInMmapChunks), len(allSamples))

	// Since there is no WBL, only samples from m-map chunks comes in the query.
	verifySamples(expInMmapChunks)

	// Verify that it did the repair on disk. All files from the point of corruption
	// should be deleted.
	files, err = os.ReadDir(mmapDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	f0, err := files[0].Info()
	require.NoError(t, err)
	require.Greater(t, f0.Size(), int64(100))
	require.Equal(t, firstFileName, files[0].Name())

	// Another restart, everything normal with no repair.
	require.NoError(t, db.Close())
	db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
	require.NoError(t, err)
	require.Equal(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.mmapChunkCorruptionTotal))
	verifySamples(expInMmapChunks)

	// Restart again with the WBL, all samples should be present now.
	require.NoError(t, db.Close())
	require.NoError(t, os.RemoveAll(wblDir))
	require.NoError(t, os.Rename(wblDirTmp, wblDir))
	db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
	require.NoError(t, err)
	verifySamples(allSamples)
}

func TestOutOfOrderRuntimeConfig(t *testing.T) {
	t.Parallel()
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOutOfOrderRuntimeConfig(t, scenario)
		})
	}
}

func testOutOfOrderRuntimeConfig(t *testing.T, scenario sampleTypeScenario) {
	ctx := context.Background()

	getDB := func(oooTimeWindow int64) *DB {
		opts := DefaultOptions()
		opts.OutOfOrderTimeWindow = oooTimeWindow
		db := newTestDB(t, withOpts(opts))
		db.DisableCompactions()
		return db
	}

	makeConfig := func(oooTimeWindow int) *config.Config {
		return &config.Config{
			StorageConfig: config.StorageConfig{
				TSDBConfig: &config.TSDBConfig{
					OutOfOrderTimeWindow: int64(oooTimeWindow) * time.Minute.Milliseconds(),
				},
			},
		}
	}

	series1 := labels.FromStrings("foo", "bar1")
	addSamples := func(t *testing.T, db *DB, fromMins, toMins int64, success bool, allSamples []chunks.Sample) []chunks.Sample {
		app := db.Appender(context.Background())
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			_, s, err := scenario.appendFunc(app, series1, ts, ts)
			if success {
				require.NoError(t, err)
				allSamples = append(allSamples, s)
			} else {
				require.Error(t, err)
			}
		}
		require.NoError(t, app.Commit())
		return allSamples
	}

	verifySamples := func(t *testing.T, db *DB, expSamples []chunks.Sample) {
		sort.Slice(expSamples, func(i, j int) bool {
			return expSamples[i].T() < expSamples[j].T()
		})

		expRes := map[string][]chunks.Sample{
			series1.String(): expSamples,
		}

		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		requireEqualSeries(t, expRes, actRes, true)
	}

	doOOOCompaction := func(t *testing.T, db *DB) {
		// WBL is not empty.
		size, err := db.head.wbl.Size()
		require.NoError(t, err)
		require.Positive(t, size)

		require.Empty(t, db.Blocks())
		require.NoError(t, db.compactOOOHead(ctx))
		require.NotEmpty(t, db.Blocks())

		// WBL is empty.
		size, err = db.head.wbl.Size()
		require.NoError(t, err)
		require.Equal(t, int64(0), size)
	}

	t.Run("increase time window", func(t *testing.T) {
		var allSamples []chunks.Sample
		db := getDB(30 * time.Minute.Milliseconds())

		// In-order.
		allSamples = addSamples(t, db, 300, 310, true, allSamples)

		// OOO upto 30m old is success.
		allSamples = addSamples(t, db, 281, 290, true, allSamples)

		// OOO of 59m old fails.
		s := addSamples(t, db, 251, 260, false, nil)
		require.Empty(t, s)
		verifySamples(t, db, allSamples)

		oldWblPtr := fmt.Sprintf("%p", db.head.wbl)

		// Increase time window and try adding again.
		err := db.ApplyConfig(makeConfig(60))
		require.NoError(t, err)
		allSamples = addSamples(t, db, 251, 260, true, allSamples)

		// WBL does not change.
		newWblPtr := fmt.Sprintf("%p", db.head.wbl)
		require.Equal(t, oldWblPtr, newWblPtr)

		doOOOCompaction(t, db)
		verifySamples(t, db, allSamples)
	})

	t.Run("decrease time window and increase again", func(t *testing.T) {
		var allSamples []chunks.Sample
		db := getDB(60 * time.Minute.Milliseconds())

		// In-order.
		allSamples = addSamples(t, db, 300, 310, true, allSamples)

		// OOO upto 59m old is success.
		allSamples = addSamples(t, db, 251, 260, true, allSamples)

		oldWblPtr := fmt.Sprintf("%p", db.head.wbl)
		// Decrease time window.
		err := db.ApplyConfig(makeConfig(30))
		require.NoError(t, err)

		// OOO of 49m old fails.
		s := addSamples(t, db, 261, 270, false, nil)
		require.Empty(t, s)

		// WBL does not change.
		newWblPtr := fmt.Sprintf("%p", db.head.wbl)
		require.Equal(t, oldWblPtr, newWblPtr)

		verifySamples(t, db, allSamples)

		// Increase time window again and check
		err = db.ApplyConfig(makeConfig(60))
		require.NoError(t, err)
		allSamples = addSamples(t, db, 261, 270, true, allSamples)
		verifySamples(t, db, allSamples)

		// WBL does not change.
		newWblPtr = fmt.Sprintf("%p", db.head.wbl)
		require.Equal(t, oldWblPtr, newWblPtr)

		doOOOCompaction(t, db)
		verifySamples(t, db, allSamples)
	})

	t.Run("disabled to enabled", func(t *testing.T) {
		var allSamples []chunks.Sample
		db := getDB(0)

		// In-order.
		allSamples = addSamples(t, db, 300, 310, true, allSamples)

		// OOO fails.
		s := addSamples(t, db, 251, 260, false, nil)
		require.Empty(t, s)
		verifySamples(t, db, allSamples)

		require.Nil(t, db.head.wbl)

		// Increase time window and try adding again.
		err := db.ApplyConfig(makeConfig(60))
		require.NoError(t, err)
		allSamples = addSamples(t, db, 251, 260, true, allSamples)

		// WBL gets created.
		require.NotNil(t, db.head.wbl)

		verifySamples(t, db, allSamples)

		// OOO compaction works now.
		doOOOCompaction(t, db)
		verifySamples(t, db, allSamples)
	})

	t.Run("enabled to disabled", func(t *testing.T) {
		var allSamples []chunks.Sample
		db := getDB(60 * time.Minute.Milliseconds())

		// In-order.
		allSamples = addSamples(t, db, 300, 310, true, allSamples)

		// OOO upto 59m old is success.
		allSamples = addSamples(t, db, 251, 260, true, allSamples)

		oldWblPtr := fmt.Sprintf("%p", db.head.wbl)
		// Time Window to 0, hence disabled.
		err := db.ApplyConfig(makeConfig(0))
		require.NoError(t, err)

		// OOO within old time window fails.
		s := addSamples(t, db, 290, 309, false, nil)
		require.Empty(t, s)

		// WBL does not change and is not removed.
		newWblPtr := fmt.Sprintf("%p", db.head.wbl)
		require.Equal(t, oldWblPtr, newWblPtr)

		verifySamples(t, db, allSamples)

		// Compaction still works after disabling with WBL cleanup.
		doOOOCompaction(t, db)
		verifySamples(t, db, allSamples)
	})

	t.Run("disabled to disabled", func(t *testing.T) {
		var allSamples []chunks.Sample
		db := getDB(0)

		// In-order.
		allSamples = addSamples(t, db, 300, 310, true, allSamples)

		// OOO fails.
		s := addSamples(t, db, 290, 309, false, nil)
		require.Empty(t, s)
		verifySamples(t, db, allSamples)
		require.Nil(t, db.head.wbl)

		// Time window to 0.
		err := db.ApplyConfig(makeConfig(0))
		require.NoError(t, err)

		// OOO still fails.
		s = addSamples(t, db, 290, 309, false, nil)
		require.Empty(t, s)
		verifySamples(t, db, allSamples)
		require.Nil(t, db.head.wbl)
	})
}

func TestNoGapAfterRestartWithOOO(t *testing.T) {
	t.Parallel()
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testNoGapAfterRestartWithOOO(t, scenario)
		})
	}
}

func testNoGapAfterRestartWithOOO(t *testing.T, scenario sampleTypeScenario) {
	series1 := labels.FromStrings("foo", "bar1")
	addSamples := func(t *testing.T, db *DB, fromMins, toMins int64, success bool) {
		app := db.Appender(context.Background())
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			_, _, err := scenario.appendFunc(app, series1, ts, ts)
			if success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		}
		require.NoError(t, app.Commit())
	}

	verifySamples := func(t *testing.T, db *DB, fromMins, toMins int64) {
		var expSamples []chunks.Sample
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			expSamples = append(expSamples, scenario.sampleFunc(ts, ts))
		}

		expRes := map[string][]chunks.Sample{
			series1.String(): expSamples,
		}

		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		requireEqualSeries(t, expRes, actRes, true)
	}

	cases := []struct {
		inOrderMint, inOrderMaxt int64
		oooMint, oooMaxt         int64
		// After compaction.
		blockRanges        [][2]int64
		headMint, headMaxt int64
	}{
		{
			300, 490,
			489, 489,
			[][2]int64{{300, 360}, {480, 600}},
			360, 490,
		},
		{
			300, 490,
			479, 479,
			[][2]int64{{300, 360}, {360, 480}},
			360, 490,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case=%d", i), func(t *testing.T) {
			ctx := context.Background()

			opts := DefaultOptions()
			opts.OutOfOrderTimeWindow = 30 * time.Minute.Milliseconds()
			db := newTestDB(t, withOpts(opts))
			db.DisableCompactions()

			// 3h10m=190m worth in-order data.
			addSamples(t, db, c.inOrderMint, c.inOrderMaxt, true)
			verifySamples(t, db, c.inOrderMint, c.inOrderMaxt)

			// One ooo samples.
			addSamples(t, db, c.oooMint, c.oooMaxt, true)
			verifySamples(t, db, c.inOrderMint, c.inOrderMaxt)

			// We get 2 blocks. 1 from OOO, 1 from in-order.
			require.NoError(t, db.Compact(ctx))
			verifyBlockRanges := func() {
				blocks := db.Blocks()
				require.Len(t, blocks, len(c.blockRanges))
				for j, br := range c.blockRanges {
					require.Equal(t, br[0]*time.Minute.Milliseconds(), blocks[j].MinTime())
					require.Equal(t, br[1]*time.Minute.Milliseconds(), blocks[j].MaxTime())
				}
			}
			verifyBlockRanges()
			require.Equal(t, c.headMint*time.Minute.Milliseconds(), db.head.MinTime())
			require.Equal(t, c.headMaxt*time.Minute.Milliseconds(), db.head.MaxTime())

			// Restart and expect all samples to be present.
			require.NoError(t, db.Close())

			db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
			db.DisableCompactions()

			verifyBlockRanges()
			require.Equal(t, c.headMint*time.Minute.Milliseconds(), db.head.MinTime())
			require.Equal(t, c.headMaxt*time.Minute.Milliseconds(), db.head.MaxTime())
			verifySamples(t, db, c.inOrderMint, c.inOrderMaxt)
		})
	}
}

func TestWblReplayAfterOOODisableAndRestart(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testWblReplayAfterOOODisableAndRestart(t, scenario)
		})
	}
}

func testWblReplayAfterOOODisableAndRestart(t *testing.T, scenario sampleTypeScenario) {
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = 60 * time.Minute.Milliseconds()

	db := newTestDB(t, withOpts(opts))

	series1 := labels.FromStrings("foo", "bar1")
	var allSamples []chunks.Sample
	addSamples := func(fromMins, toMins int64) {
		app := db.Appender(context.Background())
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			_, s, err := scenario.appendFunc(app, series1, ts, ts)
			require.NoError(t, err)
			allSamples = append(allSamples, s)
		}
		require.NoError(t, app.Commit())
	}

	// In-order samples.
	addSamples(290, 300)
	// OOO samples.
	addSamples(250, 260)

	verifySamples := func(expSamples []chunks.Sample) {
		sort.Slice(expSamples, func(i, j int) bool {
			return expSamples[i].T() < expSamples[j].T()
		})

		expRes := map[string][]chunks.Sample{
			series1.String(): expSamples,
		}

		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		requireEqualSeries(t, expRes, actRes, true)
	}

	verifySamples(allSamples)

	// Restart DB with OOO disabled.
	require.NoError(t, db.Close())

	opts.OutOfOrderTimeWindow = 0
	db = newTestDB(t, withDir(db.Dir()), withOpts(opts))

	// We can still query OOO samples when OOO is disabled.
	verifySamples(allSamples)
}

func TestPanicOnApplyConfig(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testPanicOnApplyConfig(t, scenario)
		})
	}
}

func testPanicOnApplyConfig(t *testing.T, scenario sampleTypeScenario) {
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = 60 * time.Minute.Milliseconds()

	db := newTestDB(t, withOpts(opts))

	series1 := labels.FromStrings("foo", "bar1")
	var allSamples []chunks.Sample
	addSamples := func(fromMins, toMins int64) {
		app := db.Appender(context.Background())
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			_, s, err := scenario.appendFunc(app, series1, ts, ts)
			require.NoError(t, err)
			allSamples = append(allSamples, s)
		}
		require.NoError(t, app.Commit())
	}

	// In-order samples.
	addSamples(290, 300)
	// OOO samples.
	addSamples(250, 260)

	// Restart DB with OOO disabled.
	require.NoError(t, db.Close())

	opts.OutOfOrderTimeWindow = 0
	db = newTestDB(t, withDir(db.Dir()), withOpts(opts))

	// ApplyConfig with OOO enabled and expect no panic.
	err := db.ApplyConfig(&config.Config{
		StorageConfig: config.StorageConfig{
			TSDBConfig: &config.TSDBConfig{
				OutOfOrderTimeWindow: 60 * time.Minute.Milliseconds(),
			},
		},
	})
	require.NoError(t, err)
}

func TestDiskFillingUpAfterDisablingOOO(t *testing.T) {
	t.Parallel()
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testDiskFillingUpAfterDisablingOOO(t, scenario)
		})
	}
}

func testDiskFillingUpAfterDisablingOOO(t *testing.T, scenario sampleTypeScenario) {
	t.Parallel()
	ctx := context.Background()

	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = 60 * time.Minute.Milliseconds()

	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	series1 := labels.FromStrings("foo", "bar1")
	var allSamples []chunks.Sample
	addSamples := func(fromMins, toMins int64) {
		app := db.Appender(context.Background())
		for m := fromMins; m <= toMins; m++ {
			ts := m * time.Minute.Milliseconds()
			_, s, err := scenario.appendFunc(app, series1, ts, ts)
			require.NoError(t, err)
			allSamples = append(allSamples, s)
		}
		require.NoError(t, app.Commit())
	}

	// In-order samples.
	addSamples(290, 300)
	// OOO samples.
	addSamples(250, 299)

	// Restart DB with OOO disabled.
	require.NoError(t, db.Close())

	opts.OutOfOrderTimeWindow = 0
	db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
	db.DisableCompactions()

	ms := db.head.series.getByHash(series1.Hash(), series1)
	require.NotEmpty(t, ms.ooo.oooMmappedChunks, "OOO mmap chunk was not replayed")

	checkMmapFileContents := func(contains, notContains []string) {
		mmapDir := mmappedChunksDir(db.head.opts.ChunkDirRoot)
		files, err := os.ReadDir(mmapDir)
		require.NoError(t, err)

		fnames := make([]string, 0, len(files))
		for _, f := range files {
			fnames = append(fnames, f.Name())
		}

		for _, f := range contains {
			require.Contains(t, fnames, f)
		}
		for _, f := range notContains {
			require.NotContains(t, fnames, f)
		}
	}

	// Add in-order samples until ready for compaction..
	addSamples(301, 500)

	// Check that m-map files gets deleted properly after compactions.

	db.head.mmapHeadChunks()
	checkMmapFileContents([]string{"000001", "000002"}, nil)
	require.NoError(t, db.Compact(ctx))
	checkMmapFileContents([]string{"000002"}, []string{"000001"})
	require.Nil(t, ms.ooo, "OOO mmap chunk was not compacted")

	addSamples(501, 650)
	db.head.mmapHeadChunks()
	checkMmapFileContents([]string{"000002", "000003"}, []string{"000001"})
	require.NoError(t, db.Compact(ctx))
	checkMmapFileContents(nil, []string{"000001", "000002", "000003"})

	// Verify that WBL is empty.
	files, err := os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 1) // Last empty file after compaction.
	finfo, err := files[0].Info()
	require.NoError(t, err)
	require.Equal(t, int64(0), finfo.Size())
}

func TestHistogramAppendAndQuery(t *testing.T) {
	t.Run("integer histograms", func(t *testing.T) {
		testHistogramAppendAndQueryHelper(t, false)
	})
	t.Run("float histograms", func(t *testing.T) {
		testHistogramAppendAndQueryHelper(t, true)
	})
}

func testHistogramAppendAndQueryHelper(t *testing.T, floatHistogram bool) {
	t.Helper()
	db := newTestDB(t)
	minute := func(m int) int64 { return int64(m) * time.Minute.Milliseconds() }

	ctx := context.Background()
	appendHistogram := func(t *testing.T,
		lbls labels.Labels, tsMinute int, h *histogram.Histogram,
		exp *[]chunks.Sample, expCRH histogram.CounterResetHint,
	) {
		t.Helper()
		var err error
		app := db.Appender(ctx)
		if floatHistogram {
			_, err = app.AppendHistogram(0, lbls, minute(tsMinute), nil, h.ToFloat(nil))
			efh := h.ToFloat(nil)
			efh.CounterResetHint = expCRH
			*exp = append(*exp, sample{t: minute(tsMinute), fh: efh})
		} else {
			_, err = app.AppendHistogram(0, lbls, minute(tsMinute), h.Copy(), nil)
			eh := h.Copy()
			eh.CounterResetHint = expCRH
			*exp = append(*exp, sample{t: minute(tsMinute), h: eh})
		}
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}
	appendFloat := func(t *testing.T, lbls labels.Labels, tsMinute int, val float64, exp *[]chunks.Sample) {
		t.Helper()
		app := db.Appender(ctx)
		_, err := app.Append(0, lbls, minute(tsMinute), val)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		*exp = append(*exp, sample{t: minute(tsMinute), f: val})
	}

	testQuery := func(t *testing.T, name, value string, exp map[string][]chunks.Sample) {
		t.Helper()
		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		act := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, name, value))
		require.Equal(t, exp, act)
	}

	baseH := &histogram.Histogram{
		Count:         15,
		ZeroCount:     4,
		ZeroThreshold: 0.001,
		Sum:           35.5,
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 2, Length: 2},
		},
		PositiveBuckets: []int64{1, 1, -1, 0},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 1},
			{Offset: 1, Length: 2},
		},
		NegativeBuckets: []int64{1, 2, -1},
	}

	var (
		series1                = labels.FromStrings("foo", "bar1")
		series2                = labels.FromStrings("foo", "bar2")
		series3                = labels.FromStrings("foo", "bar3")
		series4                = labels.FromStrings("foo", "bar4")
		exp1, exp2, exp3, exp4 []chunks.Sample
	)

	// TODO(codesome): test everything for negative buckets as well.
	t.Run("series with only histograms", func(t *testing.T) {
		h := baseH.Copy() // This is shared across all sub tests.

		appendHistogram(t, series1, 100, h, &exp1, histogram.UnknownCounterReset)
		testQuery(t, "foo", "bar1", map[string][]chunks.Sample{series1.String(): exp1})

		h.PositiveBuckets[0]++
		h.NegativeBuckets[0] += 2
		h.Count += 10
		appendHistogram(t, series1, 101, h, &exp1, histogram.NotCounterReset)
		testQuery(t, "foo", "bar1", map[string][]chunks.Sample{series1.String(): exp1})

		t.Run("changing schema", func(t *testing.T) {
			h.Schema = 2
			appendHistogram(t, series1, 102, h, &exp1, histogram.UnknownCounterReset)
			testQuery(t, "foo", "bar1", map[string][]chunks.Sample{series1.String(): exp1})

			// Schema back to old.
			h.Schema = 1
			appendHistogram(t, series1, 103, h, &exp1, histogram.UnknownCounterReset)
			testQuery(t, "foo", "bar1", map[string][]chunks.Sample{series1.String(): exp1})
		})

		t.Run("new buckets incoming", func(t *testing.T) {
			// In the previous unit test, during the last histogram append, we
			// changed the schema and  that caused a new chunk creation. Because
			// of the next append the layout of the last histogram will change
			// because the chunk will be re-encoded. So this forces us to modify
			// the last histogram in exp1 so when we query we get the expected
			// results.
			if floatHistogram {
				lh := exp1[len(exp1)-1].FH().Copy()
				lh.PositiveSpans[1].Length++
				lh.PositiveBuckets = append(lh.PositiveBuckets, 0)
				exp1[len(exp1)-1] = sample{t: exp1[len(exp1)-1].T(), fh: lh}
			} else {
				lh := exp1[len(exp1)-1].H().Copy()
				lh.PositiveSpans[1].Length++
				lh.PositiveBuckets = append(lh.PositiveBuckets, -2) // -2 makes the last bucket 0.
				exp1[len(exp1)-1] = sample{t: exp1[len(exp1)-1].T(), h: lh}
			}

			// This histogram with new bucket at the end causes the re-encoding of the previous histogram.
			// Hence the previous histogram is recoded into this new layout.
			// But the query returns the histogram from the in-memory buffer, hence we don't see the recode here yet.
			h.PositiveSpans[1].Length++
			h.PositiveBuckets = append(h.PositiveBuckets, 1)
			h.Count += 3
			appendHistogram(t, series1, 104, h, &exp1, histogram.NotCounterReset)
			testQuery(t, "foo", "bar1", map[string][]chunks.Sample{series1.String(): exp1})

			// Because of the previous two histograms being on the active chunk,
			// and the next append is only adding a new bucket, the active chunk
			// will be re-encoded to the new layout.
			if floatHistogram {
				lh := exp1[len(exp1)-2].FH().Copy()
				lh.PositiveSpans[0].Length++
				lh.PositiveSpans[1].Offset--
				lh.PositiveBuckets = []float64{2, 3, 0, 2, 2, 0}
				exp1[len(exp1)-2] = sample{t: exp1[len(exp1)-2].T(), fh: lh}

				lh = exp1[len(exp1)-1].FH().Copy()
				lh.PositiveSpans[0].Length++
				lh.PositiveSpans[1].Offset--
				lh.PositiveBuckets = []float64{2, 3, 0, 2, 2, 3}
				exp1[len(exp1)-1] = sample{t: exp1[len(exp1)-1].T(), fh: lh}
			} else {
				lh := exp1[len(exp1)-2].H().Copy()
				lh.PositiveSpans[0].Length++
				lh.PositiveSpans[1].Offset--
				lh.PositiveBuckets = []int64{2, 1, -3, 2, 0, -2}
				exp1[len(exp1)-2] = sample{t: exp1[len(exp1)-2].T(), h: lh}

				lh = exp1[len(exp1)-1].H().Copy()
				lh.PositiveSpans[0].Length++
				lh.PositiveSpans[1].Offset--
				lh.PositiveBuckets = []int64{2, 1, -3, 2, 0, 1}
				exp1[len(exp1)-1] = sample{t: exp1[len(exp1)-1].T(), h: lh}
			}

			// Now we add the new buckets in between. Empty bucket is again not present for the old histogram.
			h.PositiveSpans[0].Length++
			h.PositiveSpans[1].Offset--
			h.Count += 3
			// {2, 1, -1, 0, 1} -> {2, 1, 0, -1, 0, 1}
			h.PositiveBuckets = append(h.PositiveBuckets[:2], append([]int64{0}, h.PositiveBuckets[2:]...)...)
			appendHistogram(t, series1, 105, h, &exp1, histogram.NotCounterReset)
			testQuery(t, "foo", "bar1", map[string][]chunks.Sample{series1.String(): exp1})

			// We add 4 more histograms to clear out the buffer and see the re-encoded histograms.
			appendHistogram(t, series1, 106, h, &exp1, histogram.NotCounterReset)
			appendHistogram(t, series1, 107, h, &exp1, histogram.NotCounterReset)
			appendHistogram(t, series1, 108, h, &exp1, histogram.NotCounterReset)
			appendHistogram(t, series1, 109, h, &exp1, histogram.NotCounterReset)

			// Update the expected histograms to reflect the re-encoding.
			if floatHistogram {
				l := len(exp1)
				h7 := exp1[l-7].FH()
				h7.PositiveSpans = exp1[l-1].FH().PositiveSpans
				h7.PositiveBuckets = []float64{2, 3, 0, 2, 2, 0}
				exp1[l-7] = sample{t: exp1[l-7].T(), fh: h7}

				h6 := exp1[l-6].FH()
				h6.PositiveSpans = exp1[l-1].FH().PositiveSpans
				h6.PositiveBuckets = []float64{2, 3, 0, 2, 2, 3}
				exp1[l-6] = sample{t: exp1[l-6].T(), fh: h6}
			} else {
				l := len(exp1)
				h7 := exp1[l-7].H()
				h7.PositiveSpans = exp1[l-1].H().PositiveSpans
				h7.PositiveBuckets = []int64{2, 1, -3, 2, 0, -2} // -3 and -2 are the empty buckets.
				exp1[l-7] = sample{t: exp1[l-7].T(), h: h7}

				h6 := exp1[l-6].H()
				h6.PositiveSpans = exp1[l-1].H().PositiveSpans
				h6.PositiveBuckets = []int64{2, 1, -3, 2, 0, 1} // -3 is the empty bucket.
				exp1[l-6] = sample{t: exp1[l-6].T(), h: h6}
			}

			testQuery(t, "foo", "bar1", map[string][]chunks.Sample{series1.String(): exp1})
		})

		t.Run("buckets disappearing", func(t *testing.T) {
			h.PositiveSpans[1].Length--
			h.PositiveBuckets = h.PositiveBuckets[:len(h.PositiveBuckets)-1]
			h.Count -= 3
			appendHistogram(t, series1, 110, h, &exp1, histogram.UnknownCounterReset)
			testQuery(t, "foo", "bar1", map[string][]chunks.Sample{series1.String(): exp1})
		})
	})

	t.Run("series starting with float and then getting histograms", func(t *testing.T) {
		appendFloat(t, series2, 100, 100, &exp2)
		appendFloat(t, series2, 101, 101, &exp2)
		appendFloat(t, series2, 102, 102, &exp2)
		testQuery(t, "foo", "bar2", map[string][]chunks.Sample{series2.String(): exp2})

		h := baseH.Copy()
		appendHistogram(t, series2, 103, h, &exp2, histogram.UnknownCounterReset)
		appendHistogram(t, series2, 104, h, &exp2, histogram.NotCounterReset)
		appendHistogram(t, series2, 105, h, &exp2, histogram.NotCounterReset)
		testQuery(t, "foo", "bar2", map[string][]chunks.Sample{series2.String(): exp2})

		// Switching between float and histograms again.
		appendFloat(t, series2, 106, 106, &exp2)
		appendFloat(t, series2, 107, 107, &exp2)
		testQuery(t, "foo", "bar2", map[string][]chunks.Sample{series2.String(): exp2})

		appendHistogram(t, series2, 108, h, &exp2, histogram.UnknownCounterReset)
		appendHistogram(t, series2, 109, h, &exp2, histogram.NotCounterReset)
		testQuery(t, "foo", "bar2", map[string][]chunks.Sample{series2.String(): exp2})
	})

	t.Run("series starting with histogram and then getting float", func(t *testing.T) {
		h := baseH.Copy()
		appendHistogram(t, series3, 101, h, &exp3, histogram.UnknownCounterReset)
		appendHistogram(t, series3, 102, h, &exp3, histogram.NotCounterReset)
		appendHistogram(t, series3, 103, h, &exp3, histogram.NotCounterReset)
		testQuery(t, "foo", "bar3", map[string][]chunks.Sample{series3.String(): exp3})

		appendFloat(t, series3, 104, 100, &exp3)
		appendFloat(t, series3, 105, 101, &exp3)
		appendFloat(t, series3, 106, 102, &exp3)
		testQuery(t, "foo", "bar3", map[string][]chunks.Sample{series3.String(): exp3})

		// Switching between histogram and float again.
		appendHistogram(t, series3, 107, h, &exp3, histogram.UnknownCounterReset)
		appendHistogram(t, series3, 108, h, &exp3, histogram.NotCounterReset)
		testQuery(t, "foo", "bar3", map[string][]chunks.Sample{series3.String(): exp3})

		appendFloat(t, series3, 109, 106, &exp3)
		appendFloat(t, series3, 110, 107, &exp3)
		testQuery(t, "foo", "bar3", map[string][]chunks.Sample{series3.String(): exp3})
	})

	t.Run("query mix of histogram and float series", func(t *testing.T) {
		// A float only series.
		appendFloat(t, series4, 100, 100, &exp4)
		appendFloat(t, series4, 101, 101, &exp4)
		appendFloat(t, series4, 102, 102, &exp4)

		testQuery(t, "foo", "bar.*", map[string][]chunks.Sample{
			series1.String(): exp1,
			series2.String(): exp2,
			series3.String(): exp3,
			series4.String(): exp4,
		})
	})
}

func TestQueryHistogramFromBlocksWithCompaction(t *testing.T) {
	t.Parallel()
	minute := func(m int) int64 { return int64(m) * time.Minute.Milliseconds() }

	testBlockQuerying := func(t *testing.T, blockSeries ...[]storage.Series) {
		t.Helper()

		opts := DefaultOptions()
		db := newTestDB(t, withOpts(opts))

		var it chunkenc.Iterator
		exp := make(map[string][]chunks.Sample)
		for _, series := range blockSeries {
			createBlock(t, db.Dir(), series)

			for _, s := range series {
				lbls := s.Labels().String()
				slice := exp[lbls]
				it = s.Iterator(it)
				smpls, err := storage.ExpandSamples(it, nil)
				require.NoError(t, err)
				slice = append(slice, smpls...)
				sort.Slice(slice, func(i, j int) bool {
					return slice[i].T() < slice[j].T()
				})
				exp[lbls] = slice
			}
		}

		require.Empty(t, db.Blocks())
		require.NoError(t, db.reload())
		require.Len(t, db.Blocks(), len(blockSeries))

		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		res := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*"))
		compareSeries(t, exp, res)

		// Compact all the blocks together and query again.
		blocks := db.Blocks()
		blockDirs := make([]string, 0, len(blocks))
		for _, b := range blocks {
			blockDirs = append(blockDirs, b.Dir())
		}
		ids, err := db.compactor.Compact(db.Dir(), blockDirs, blocks)
		require.NoError(t, err)
		require.Len(t, ids, 1)
		require.NoError(t, db.reload())
		require.Len(t, db.Blocks(), 1)

		q, err = db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		res = query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*"))

		// After compaction, we do not require "unknown" counter resets
		// due to origin from different overlapping chunks anymore.
		for _, ss := range exp {
			for i, s := range ss[1:] {
				if s.Type() == chunkenc.ValHistogram && ss[i].Type() == chunkenc.ValHistogram && s.H().CounterResetHint == histogram.UnknownCounterReset {
					s.H().CounterResetHint = histogram.NotCounterReset
				}
				if s.Type() == chunkenc.ValFloatHistogram && ss[i].Type() == chunkenc.ValFloatHistogram && s.FH().CounterResetHint == histogram.UnknownCounterReset {
					s.FH().CounterResetHint = histogram.NotCounterReset
				}
			}
		}
		compareSeries(t, exp, res)
	}

	for _, floatHistogram := range []bool{false, true} {
		t.Run(fmt.Sprintf("floatHistogram=%t", floatHistogram), func(t *testing.T) {
			t.Run("serial blocks with only histograms", func(t *testing.T) {
				testBlockQuerying(t,
					genHistogramSeries(10, 5, minute(0), minute(119), minute(1), floatHistogram),
					genHistogramSeries(10, 5, minute(120), minute(239), minute(1), floatHistogram),
					genHistogramSeries(10, 5, minute(240), minute(359), minute(1), floatHistogram),
				)
			})

			t.Run("serial blocks with either histograms or floats in a block and not both", func(t *testing.T) {
				testBlockQuerying(t,
					genHistogramSeries(10, 5, minute(0), minute(119), minute(1), floatHistogram),
					genSeriesFromSampleGenerator(10, 5, minute(120), minute(239), minute(1), func(ts int64) chunks.Sample {
						return sample{t: ts, f: rand.Float64()}
					}),
					genHistogramSeries(10, 5, minute(240), minute(359), minute(1), floatHistogram),
				)
			})

			t.Run("serial blocks with mix of histograms and float64", func(t *testing.T) {
				testBlockQuerying(t,
					genHistogramAndFloatSeries(10, 5, minute(0), minute(60), minute(1), floatHistogram),
					genHistogramSeries(10, 5, minute(61), minute(120), minute(1), floatHistogram),
					genHistogramAndFloatSeries(10, 5, minute(121), minute(180), minute(1), floatHistogram),
					genSeriesFromSampleGenerator(10, 5, minute(181), minute(240), minute(1), func(ts int64) chunks.Sample {
						return sample{t: ts, f: rand.Float64()}
					}),
				)
			})

			t.Run("overlapping blocks with only histograms", func(t *testing.T) {
				testBlockQuerying(t,
					genHistogramSeries(10, 5, minute(0), minute(120), minute(3), floatHistogram),
					genHistogramSeries(10, 5, minute(1), minute(120), minute(3), floatHistogram),
					genHistogramSeries(10, 5, minute(2), minute(120), minute(3), floatHistogram),
				)
			})

			t.Run("overlapping blocks with only histograms and only float in a series", func(t *testing.T) {
				testBlockQuerying(t,
					genHistogramSeries(10, 5, minute(0), minute(120), minute(3), floatHistogram),
					genSeriesFromSampleGenerator(10, 5, minute(1), minute(120), minute(3), func(ts int64) chunks.Sample {
						return sample{t: ts, f: rand.Float64()}
					}),
					genHistogramSeries(10, 5, minute(2), minute(120), minute(3), floatHistogram),
				)
			})

			t.Run("overlapping blocks with mix of histograms and float64", func(t *testing.T) {
				testBlockQuerying(t,
					genHistogramAndFloatSeries(10, 5, minute(0), minute(60), minute(3), floatHistogram),
					genHistogramSeries(10, 5, minute(46), minute(100), minute(3), floatHistogram),
					genHistogramAndFloatSeries(10, 5, minute(89), minute(140), minute(3), floatHistogram),
					genSeriesFromSampleGenerator(10, 5, minute(126), minute(200), minute(3), func(ts int64) chunks.Sample {
						return sample{t: ts, f: rand.Float64()}
					}),
				)
			})
		})
	}
}

func TestOOONativeHistogramsSettings(t *testing.T) {
	h := &histogram.Histogram{
		Count:         9,
		ZeroCount:     4,
		ZeroThreshold: 0.001,
		Sum:           35.5,
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 2, Length: 2},
		},
		PositiveBuckets: []int64{1, 1, -1, 0},
	}

	l := labels.FromStrings("foo", "bar")

	t.Run("Test OOO native histograms if OOO is disabled and Native Histograms is enabled", func(t *testing.T) {
		opts := DefaultOptions()
		opts.OutOfOrderTimeWindow = 0
		db := newTestDB(t, withOpts(opts), withRngs(100))

		app := db.Appender(context.Background())
		_, err := app.AppendHistogram(0, l, 100, h, nil)
		require.NoError(t, err)

		_, err = app.AppendHistogram(0, l, 50, h, nil)
		require.NoError(t, err) // The OOO sample is not detected until it is committed, so no error is returned

		require.NoError(t, app.Commit())

		q, err := db.Querier(math.MinInt, math.MaxInt64)
		require.NoError(t, err)
		act := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
		require.Equal(t, map[string][]chunks.Sample{
			l.String(): {sample{t: 100, h: h}},
		}, act)
	})
	t.Run("Test OOO native histograms when both OOO and Native Histograms are enabled", func(t *testing.T) {
		opts := DefaultOptions()
		opts.OutOfOrderTimeWindow = 100
		db := newTestDB(t, withOpts(opts), withRngs(100))

		// Add in-order samples
		app := db.Appender(context.Background())
		_, err := app.AppendHistogram(0, l, 200, h, nil)
		require.NoError(t, err)

		// Add OOO samples
		_, err = app.AppendHistogram(0, l, 100, h, nil)
		require.NoError(t, err)
		_, err = app.AppendHistogram(0, l, 150, h, nil)
		require.NoError(t, err)

		require.NoError(t, app.Commit())

		q, err := db.Querier(math.MinInt, math.MaxInt64)
		require.NoError(t, err)
		act := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
		requireEqualSeries(t, map[string][]chunks.Sample{
			l.String(): {sample{t: 100, h: h}, sample{t: 150, h: h}, sample{t: 200, h: h}},
		}, act, true)
	})
}

// compareSeries essentially replaces `require.Equal(t, expected, actual)` in
// situations where the actual series might contain more counter reset hints
// "unknown" than the expected series. This can easily happen for long series
// that trigger new chunks. This function therefore tolerates counter reset
// hints "CounterReset" and "NotCounterReset" in an expected series where the
// actual series contains a counter reset hint "UnknownCounterReset".
// "GaugeType" hints are still strictly checked, and any "UnknownCounterReset"
// in an expected series has to be matched precisely by the actual series.
func compareSeries(t require.TestingT, expected, actual map[string][]chunks.Sample) {
	if len(expected) != len(actual) {
		// The reason for the difference is not the counter reset hints
		// (alone), so let's use the pretty diffing by the require
		// package.
		require.Equal(t, expected, actual, "number of series differs")
	}
	for key, expSamples := range expected {
		actSamples, ok := actual[key]
		if !ok {
			require.Equal(t, expected, actual, "expected series %q not found", key)
		}
		if len(expSamples) != len(actSamples) {
			require.Equal(t, expSamples, actSamples, "number of samples for series %q differs", key)
		}

		for i, eS := range expSamples {
			aS := actSamples[i]

			// Must use the interface as Equal does not work when actual types differ
			// not only does the type differ, but chunk.Sample.FH() interface may auto convert from chunk.Sample.H()!
			require.Equal(t, eS.T(), aS.T(), "timestamp of sample %d in series %q differs", i, key)

			require.Equal(t, eS.Type(), aS.Type(), "type of sample %d in series %q differs", i, key)

			switch eS.Type() {
			case chunkenc.ValFloat:
				require.Equal(t, eS.F(), aS.F(), "sample %d in series %q differs", i, key)
			case chunkenc.ValHistogram:
				eH, aH := eS.H(), aS.H()
				if aH.CounterResetHint == histogram.UnknownCounterReset {
					eH = eH.Copy()
					// It is always safe to set the counter reset hint to UnknownCounterReset
					eH.CounterResetHint = histogram.UnknownCounterReset
					eS = sample{t: eS.T(), h: eH}
				}
				require.Equal(t, eH, aH, "histogram sample %d in series %q differs", i, key)

			case chunkenc.ValFloatHistogram:
				eFH, aFH := eS.FH(), aS.FH()
				if aFH.CounterResetHint == histogram.UnknownCounterReset {
					eFH = eFH.Copy()
					// It is always safe to set the counter reset hint to UnknownCounterReset
					eFH.CounterResetHint = histogram.UnknownCounterReset
					eS = sample{t: eS.T(), fh: eFH}
				}
				require.Equal(t, eFH, aFH, "float histogram sample %d in series %q differs", i, key)
			}
		}
	}
}

// TestChunkQuerierReadWriteRace looks for any possible race between appending
// samples and reading chunks because the head chunk that is being appended to
// can be read in parallel and we should be able to make a copy of the chunk without
// worrying about the parallel write.
func TestChunkQuerierReadWriteRace(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	lbls := labels.FromStrings("foo", "bar")

	writer := func() error {
		<-time.After(5 * time.Millisecond) // Initial pause while readers start.
		ts := 0
		for range 500 {
			app := db.Appender(context.Background())
			for range 10 {
				ts++
				_, err := app.Append(0, lbls, int64(ts), float64(ts*100))
				if err != nil {
					return err
				}
			}
			err := app.Commit()
			if err != nil {
				return err
			}
			<-time.After(time.Millisecond)
		}
		return nil
	}

	reader := func() {
		querier, err := db.ChunkQuerier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		defer func(q storage.ChunkQuerier) {
			require.NoError(t, q.Close())
		}(querier)
		ss := querier.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
		for ss.Next() {
			cs := ss.At()
			it := cs.Iterator(nil)
			for it.Next() {
				m := it.At()
				b := m.Chunk.Bytes()
				bb := make([]byte, len(b))
				copy(bb, b) // This copying of chunk bytes detects any race.
			}
		}
		require.NoError(t, ss.Err())
	}

	ch := make(chan struct{})
	var writerErr error
	go func() {
		defer close(ch)
		writerErr = writer()
	}()

Outer:
	for {
		reader()
		select {
		case <-ch:
			break Outer
		default:
		}
	}

	require.NoError(t, writerErr)
}

type mockCompactorFn struct {
	planFn    func() ([]string, error)
	compactFn func() ([]ulid.ULID, error)
	writeFn   func() ([]ulid.ULID, error)
}

func (c *mockCompactorFn) Plan(string) ([]string, error) {
	return c.planFn()
}

func (c *mockCompactorFn) Compact(string, []string, []*Block) ([]ulid.ULID, error) {
	return c.compactFn()
}

func (c *mockCompactorFn) Write(string, BlockReader, int64, int64, *BlockMeta) ([]ulid.ULID, error) {
	return c.writeFn()
}

// Regression test for https://github.com/prometheus/prometheus/pull/13754
func TestAbortBlockCompactions(t *testing.T) {
	// Create a test DB
	db := newTestDB(t)
	// It should NOT be compactable at the beginning of the test
	require.False(t, db.head.compactable(), "head should NOT be compactable")

	// Track the number of compactions run inside db.compactBlocks()
	var compactions int

	// Use a mock compactor with custom Plan() implementation
	db.compactor = &mockCompactorFn{
		planFn: func() ([]string, error) {
			// On every Plan() run increment compactions. After 4 compactions
			// update HEAD to make it compactable to force an exit from db.compactBlocks() loop.
			compactions++
			if compactions > 3 {
				chunkRange := db.head.chunkRange.Load()
				db.head.minTime.Store(0)
				db.head.maxTime.Store(chunkRange * 2)
				require.True(t, db.head.compactable(), "head should be compactable")
			}
			// Our custom Plan() will always return something to compact.
			return []string{"1", "2", "3"}, nil
		},
		compactFn: func() ([]ulid.ULID, error) {
			return []ulid.ULID{}, nil
		},
		writeFn: func() ([]ulid.ULID, error) {
			return []ulid.ULID{}, nil
		},
	}

	err := db.Compact(context.Background())
	require.NoError(t, err)
	require.True(t, db.head.compactable(), "head should be compactable")
	require.Equal(t, 4, compactions, "expected 4 compactions to be completed")
}

func TestNewCompactorFunc(t *testing.T) {
	opts := DefaultOptions()
	block1 := ulid.MustNew(1, nil)
	block2 := ulid.MustNew(2, nil)
	opts.NewCompactorFunc = func(context.Context, prometheus.Registerer, *slog.Logger, []int64, chunkenc.Pool, *Options) (Compactor, error) {
		return &mockCompactorFn{
			planFn: func() ([]string, error) {
				return []string{block1.String(), block2.String()}, nil
			},
			compactFn: func() ([]ulid.ULID, error) {
				return []ulid.ULID{block1}, nil
			},
			writeFn: func() ([]ulid.ULID, error) {
				return []ulid.ULID{block2}, nil
			},
		}, nil
	}
	db := newTestDB(t, withOpts(opts))

	plans, err := db.compactor.Plan("")
	require.NoError(t, err)
	require.Equal(t, []string{block1.String(), block2.String()}, plans)
	ulids, err := db.compactor.Compact("", nil, nil)
	require.NoError(t, err)
	require.Len(t, ulids, 1)
	require.Equal(t, block1, ulids[0])
	ulids, err = db.compactor.Write("", nil, 0, 1, nil)
	require.NoError(t, err)
	require.Len(t, ulids, 1)
	require.Equal(t, block2, ulids[0])
}

func TestBlockQuerierAndBlockChunkQuerier(t *testing.T) {
	opts := DefaultOptions()
	opts.BlockQuerierFunc = func(b BlockReader, mint, maxt int64) (storage.Querier, error) {
		// Only block with hints can be queried.
		if len(b.Meta().Compaction.Hints) > 0 {
			return NewBlockQuerier(b, mint, maxt)
		}
		return storage.NoopQuerier(), nil
	}
	opts.BlockChunkQuerierFunc = func(b BlockReader, mint, maxt int64) (storage.ChunkQuerier, error) {
		// Only level 4 compaction block can be queried.
		if b.Meta().Compaction.Level == 4 {
			return NewBlockChunkQuerier(b, mint, maxt)
		}
		return storage.NoopChunkedQuerier(), nil
	}

	db := newTestDB(t, withOpts(opts))

	metas := []BlockMeta{
		{Compaction: BlockMetaCompaction{Hints: []string{"test-hint"}}},
		{Compaction: BlockMetaCompaction{Level: 4}},
	}
	for i := range metas {
		// Include blockID into series to identify which block got touched.
		serieses := []storage.Series{storage.NewListSeries(labels.FromMap(map[string]string{"block": fmt.Sprintf("block-%d", i), labels.MetricName: "test_metric"}), []chunks.Sample{sample{t: 0, f: 1}})}
		blockDir := createBlock(t, db.Dir(), serieses)
		b, err := OpenBlock(db.logger, blockDir, db.chunkPool, nil)
		require.NoError(t, err)

		// Overwrite meta.json with compaction section for testing purpose.
		b.meta.Compaction = metas[i].Compaction
		_, err = writeMetaFile(db.logger, blockDir, &b.meta)
		require.NoError(t, err)
		require.NoError(t, b.Close())
	}
	require.NoError(t, db.reloadBlocks())
	require.Len(t, db.Blocks(), 2)

	querier, err := db.Querier(0, 500)
	require.NoError(t, err)
	defer querier.Close()
	matcher := labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "test_metric")
	seriesSet := querier.Select(context.Background(), false, nil, matcher)
	count := 0
	var lbls labels.Labels
	for seriesSet.Next() {
		count++
		lbls = seriesSet.At().Labels()
	}
	require.NoError(t, seriesSet.Err())
	require.Equal(t, 1, count)
	// Make sure only block-0 is queried.
	require.Equal(t, "block-0", lbls.Get("block"))

	chunkQuerier, err := db.ChunkQuerier(0, 500)
	require.NoError(t, err)
	defer chunkQuerier.Close()
	css := chunkQuerier.Select(context.Background(), false, nil, matcher)
	count = 0
	// Reset lbls variable.
	lbls = labels.EmptyLabels()
	for css.Next() {
		count++
		lbls = css.At().Labels()
	}
	require.NoError(t, css.Err())
	require.Equal(t, 1, count)
	// Make sure only block-1 is queried.
	require.Equal(t, "block-1", lbls.Get("block"))
}

func TestGenerateCompactionDelay(t *testing.T) {
	assertDelay := func(delay time.Duration, expectedMaxPercentDelay int) {
		t.Helper()
		require.GreaterOrEqual(t, delay, time.Duration(0))
		// Expect to generate a delay up to MaxPercentDelay of the head chunk range
		require.LessOrEqual(t, delay, (time.Duration(60000*expectedMaxPercentDelay/100) * time.Millisecond))
	}

	opts := DefaultOptions()
	cases := []struct {
		compactionDelayPercent int
	}{
		{
			compactionDelayPercent: 1,
		},
		{
			compactionDelayPercent: 10,
		},
		{
			compactionDelayPercent: 60,
		},
		{
			compactionDelayPercent: 100,
		},
	}

	opts.EnableDelayedCompaction = true

	for _, c := range cases {
		opts.CompactionDelayMaxPercent = c.compactionDelayPercent
		db := newTestDB(t, withOpts(opts), withRngs(60000))

		// The offset is generated and changed while opening.
		assertDelay(db.opts.CompactionDelay, c.compactionDelayPercent)

		for range 1000 {
			assertDelay(db.generateCompactionDelay(), c.compactionDelayPercent)
		}
	}
}

type blockedResponseRecorder struct {
	r *httptest.ResponseRecorder

	// writeBlocked is used to block writing until the test wants it to resume.
	writeBlocked chan struct{}
	// writeStarted is closed by blockedResponseRecorder to signal that writing has started.
	writeStarted chan struct{}
}

func (br *blockedResponseRecorder) Write(buf []byte) (int, error) {
	select {
	case <-br.writeStarted:
	default:
		close(br.writeStarted)
	}

	<-br.writeBlocked
	return br.r.Write(buf)
}

func (br *blockedResponseRecorder) Header() http.Header { return br.r.Header() }

func (br *blockedResponseRecorder) WriteHeader(code int) { br.r.WriteHeader(code) }

func (br *blockedResponseRecorder) Flush() { br.r.Flush() }

// TestBlockClosingBlockedDuringRemoteRead ensures that a TSDB Block is not closed while it is being queried
// through remote read. This is a regression test for https://github.com/prometheus/prometheus/issues/14422.
// TODO: Ideally, this should reside in storage/remote/read_handler_test.go once the necessary TSDB utils are accessible there.
func TestBlockClosingBlockedDuringRemoteRead(t *testing.T) {
	dir := t.TempDir()

	createBlock(t, dir, genSeries(2, 1, 0, 10))

	// Not using newTestDB as db.Close is expected to return error.
	db, err := Open(dir, nil, nil, nil, nil)
	require.NoError(t, err)
	defer db.Close()

	readAPI := remote.NewReadHandler(
		nil, nil, db,
		func() config.Config {
			return config.Config{}
		}, 0, 1, 0,
	)

	matcher, err := labels.NewMatcher(labels.MatchRegexp, "__name__", ".*")
	require.NoError(t, err)

	query, err := remote.ToQuery(0, 10, []*labels.Matcher{matcher}, nil)
	require.NoError(t, err)

	req := &prompb.ReadRequest{
		Queries:               []*prompb.Query{query},
		AcceptedResponseTypes: []prompb.ReadRequest_ResponseType{prompb.ReadRequest_STREAMED_XOR_CHUNKS},
	}
	data, err := proto.Marshal(req)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer(snappy.Encode(nil, data)))
	require.NoError(t, err)

	blockedRecorder := &blockedResponseRecorder{
		r:            httptest.NewRecorder(),
		writeBlocked: make(chan struct{}),
		writeStarted: make(chan struct{}),
	}

	readDone := make(chan struct{})
	go func() {
		readAPI.ServeHTTP(blockedRecorder, request)
		require.Equal(t, http.StatusOK, blockedRecorder.r.Code)
		close(readDone)
	}()

	// Wait for the read API to start streaming data.
	<-blockedRecorder.writeStarted

	// Try to close the queried block.
	blockClosed := make(chan struct{})
	go func() {
		for _, block := range db.Blocks() {
			block.Close()
		}
		close(blockClosed)
	}()

	// Closing the queried block should block.
	// Wait a little bit to make sure of that.
	select {
	case <-time.After(100 * time.Millisecond):
	case <-readDone:
		require.Fail(t, "read API should still be streaming data.")
	case <-blockClosed:
		require.Fail(t, "Block shouldn't get closed while being queried.")
	}

	// Resume the read API data streaming.
	close(blockedRecorder.writeBlocked)
	<-readDone

	// The block should be no longer needed and closing it should end.
	select {
	case <-time.After(10 * time.Millisecond):
		require.Fail(t, "Closing the block timed out.")
	case <-blockClosed:
	}
}

func TestBlockReloadInterval(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		reloadInterval  time.Duration
		expectedReloads float64
	}{
		{
			name:            "extremely small interval",
			reloadInterval:  1 * time.Millisecond,
			expectedReloads: 5,
		},
		{
			name:            "one second interval",
			reloadInterval:  1 * time.Second,
			expectedReloads: 5,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			db := newTestDB(t, withOpts(&Options{
				BlockReloadInterval: c.reloadInterval,
			}))
			if c.reloadInterval < 1*time.Second {
				require.Equal(t, 1*time.Second, db.opts.BlockReloadInterval, "interval should be clamped to minimum of 1 second")
			}
			require.Equal(t, float64(1), prom_testutil.ToFloat64(db.metrics.reloads), "there should be one initial reload")
			require.Eventually(t, func() bool {
				return prom_testutil.ToFloat64(db.metrics.reloads) == c.expectedReloads
			},
				5*time.Second,
				100*time.Millisecond,
			)
		})
	}
}

func TestStaleSeriesCompaction(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	var (
		nonStaleSeries, staleSeries,
		nonStaleHist, staleHist,
		nonStaleFHist, staleFHist,
		staleSeriesCrossingBoundary, staleHistCrossingBoundary, staleFHistCrossingBoundary []labels.Labels
		numSeriesPerCategory = 1
	)
	for i := range numSeriesPerCategory {
		nonStaleSeries = append(nonStaleSeries, labels.FromStrings("name", fmt.Sprintf("series%d", 1000+i)))
		nonStaleHist = append(nonStaleHist, labels.FromStrings("name", fmt.Sprintf("series%d", 2000+i)))
		nonStaleFHist = append(nonStaleFHist, labels.FromStrings("name", fmt.Sprintf("series%d", 3000+i)))

		staleSeries = append(staleSeries, labels.FromStrings("name", fmt.Sprintf("series%d", 4000+i)))
		staleHist = append(staleHist, labels.FromStrings("name", fmt.Sprintf("series%d", 5000+i)))
		staleFHist = append(staleFHist, labels.FromStrings("name", fmt.Sprintf("series%d", 6000+i)))

		staleSeriesCrossingBoundary = append(staleSeriesCrossingBoundary, labels.FromStrings("name", fmt.Sprintf("series%d", 7000+i)))
		staleHistCrossingBoundary = append(staleHistCrossingBoundary, labels.FromStrings("name", fmt.Sprintf("series%d", 8000+i)))
		staleFHistCrossingBoundary = append(staleFHistCrossingBoundary, labels.FromStrings("name", fmt.Sprintf("series%d", 9000+i)))
	}

	var (
		v       = 10.0
		staleV  = math.Float64frombits(value.StaleNaN)
		h       = tsdbutil.GenerateTestHistograms(1)[0]
		fh      = tsdbutil.GenerateTestFloatHistograms(1)[0]
		staleH  = &histogram.Histogram{Sum: staleV}
		staleFH = &histogram.FloatHistogram{Sum: staleV}
	)

	addNormalSamples := func(ts int64, floatSeries, histSeries, floatHistSeries []labels.Labels) {
		app := db.Appender(context.Background())
		for i := range len(floatSeries) {
			_, err := app.Append(0, floatSeries[i], ts, v)
			require.NoError(t, err)
			_, err = app.AppendHistogram(0, histSeries[i], ts, h, nil)
			require.NoError(t, err)
			_, err = app.AppendHistogram(0, floatHistSeries[i], ts, nil, fh)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}
	addStaleSamples := func(ts int64, floatSeries, histSeries, floatHistSeries []labels.Labels) {
		app := db.Appender(context.Background())
		for i := range len(floatSeries) {
			_, err := app.Append(0, floatSeries[i], ts, staleV)
			require.NoError(t, err)
			_, err = app.AppendHistogram(0, histSeries[i], ts, staleH, nil)
			require.NoError(t, err)
			_, err = app.AppendHistogram(0, floatHistSeries[i], ts, nil, staleFH)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Normal sample for all.
	addNormalSamples(100, nonStaleSeries, nonStaleHist, nonStaleFHist)
	addNormalSamples(100, staleSeries, staleHist, staleFHist)

	// Stale sample for the stale series. Normal sample for the non-stale series.
	addNormalSamples(200, nonStaleSeries, nonStaleHist, nonStaleFHist)
	addStaleSamples(200, staleSeries, staleHist, staleFHist)

	// Normal samples for the non-stale series later
	addNormalSamples(300, nonStaleSeries, nonStaleHist, nonStaleFHist)

	require.Equal(t, uint64(6*numSeriesPerCategory), db.Head().NumSeries())
	require.Equal(t, uint64(3*numSeriesPerCategory), db.Head().NumStaleSeries())

	// Series crossing block boundary and gets stale.
	addNormalSamples(300, staleSeriesCrossingBoundary, staleHistCrossingBoundary, staleFHistCrossingBoundary)
	addNormalSamples(700, staleSeriesCrossingBoundary, staleHistCrossingBoundary, staleFHistCrossingBoundary)
	addNormalSamples(1100, staleSeriesCrossingBoundary, staleHistCrossingBoundary, staleFHistCrossingBoundary)
	addStaleSamples(1200, staleSeriesCrossingBoundary, staleHistCrossingBoundary, staleFHistCrossingBoundary)

	require.NoError(t, db.CompactStaleHead())

	require.Equal(t, uint64(3*numSeriesPerCategory), db.Head().NumSeries())
	require.Equal(t, uint64(0), db.Head().NumStaleSeries())

	require.Len(t, db.Blocks(), 2)
	m := db.Blocks()[0].Meta()
	require.Equal(t, int64(0), m.MinTime)
	require.Equal(t, int64(1000), m.MaxTime)
	require.Truef(t, m.Compaction.FromStaleSeries(), "stale series info not found in block meta")
	m = db.Blocks()[1].Meta()
	require.Equal(t, int64(1000), m.MinTime)
	require.Equal(t, int64(2000), m.MaxTime)
	require.Truef(t, m.Compaction.FromStaleSeries(), "stale series info not found in block meta")

	// To make sure that Head is not truncated based on stale series block.
	require.NoError(t, db.reload())

	nonFirstH := h.Copy()
	nonFirstH.CounterResetHint = histogram.NotCounterReset
	nonFirstFH := fh.Copy()
	nonFirstFH.CounterResetHint = histogram.NotCounterReset

	// Verify head block.
	verifyHeadBlock := func() {
		require.Equal(t, uint64(3), db.head.NumSeries())
		require.Equal(t, uint64(0), db.head.NumStaleSeries())

		expHeadQuery := make(map[string][]chunks.Sample)
		for i := range numSeriesPerCategory {
			expHeadQuery[fmt.Sprintf(`{name="%s"}`, nonStaleSeries[i].Get("name"))] = []chunks.Sample{
				sample{t: 100, f: v}, sample{t: 200, f: v}, sample{t: 300, f: v},
			}
			expHeadQuery[fmt.Sprintf(`{name="%s"}`, nonStaleHist[i].Get("name"))] = []chunks.Sample{
				sample{t: 100, h: h}, sample{t: 200, h: nonFirstH}, sample{t: 300, h: nonFirstH},
			}
			expHeadQuery[fmt.Sprintf(`{name="%s"}`, nonStaleFHist[i].Get("name"))] = []chunks.Sample{
				sample{t: 100, fh: fh}, sample{t: 200, fh: nonFirstFH}, sample{t: 300, fh: nonFirstFH},
			}
		}

		querier, err := NewBlockQuerier(NewRangeHead(db.head, 0, 300), 0, 300)
		require.NoError(t, err)
		t.Cleanup(func() {
			querier.Close()
		})
		seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "name", "series.*"))
		require.Equal(t, expHeadQuery, seriesSet)
	}

	verifyHeadBlock()

	// Verify blocks from stale series.
	{
		expBlockQuery := make(map[string][]chunks.Sample)
		for i := range numSeriesPerCategory {
			expBlockQuery[fmt.Sprintf(`{name="%s"}`, staleSeries[i].Get("name"))] = []chunks.Sample{
				sample{t: 100, f: v}, sample{t: 200, f: staleV},
			}
			expBlockQuery[fmt.Sprintf(`{name="%s"}`, staleHist[i].Get("name"))] = []chunks.Sample{
				sample{t: 100, h: h}, sample{t: 200, h: staleH},
			}
			expBlockQuery[fmt.Sprintf(`{name="%s"}`, staleFHist[i].Get("name"))] = []chunks.Sample{
				sample{t: 100, fh: fh}, sample{t: 200, fh: staleFH},
			}
			expBlockQuery[fmt.Sprintf(`{name="%s"}`, staleSeriesCrossingBoundary[i].Get("name"))] = []chunks.Sample{
				sample{t: 300, f: v}, sample{t: 700, f: v}, sample{t: 1100, f: v}, sample{t: 1200, f: staleV},
			}
			expBlockQuery[fmt.Sprintf(`{name="%s"}`, staleHistCrossingBoundary[i].Get("name"))] = []chunks.Sample{
				sample{t: 300, h: h}, sample{t: 700, h: nonFirstH}, sample{t: 1100, h: h}, sample{t: 1200, h: staleH},
			}
			expBlockQuery[fmt.Sprintf(`{name="%s"}`, staleFHistCrossingBoundary[i].Get("name"))] = []chunks.Sample{
				sample{t: 300, fh: fh}, sample{t: 700, fh: nonFirstFH}, sample{t: 1100, fh: fh}, sample{t: 1200, fh: staleFH},
			}
		}

		querier, err := NewBlockQuerier(db.Blocks()[0], 0, 1000)
		require.NoError(t, err)
		t.Cleanup(func() {
			querier.Close()
		})
		seriesSet := queryWithoutReplacingNaNs(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "name", "series.*"))

		querier, err = NewBlockQuerier(db.Blocks()[1], 1000, 2000)
		require.NoError(t, err)
		t.Cleanup(func() {
			querier.Close()
		})
		seriesSet2 := queryWithoutReplacingNaNs(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "name", "series.*"))
		for k, v := range seriesSet2 {
			seriesSet[k] = append(seriesSet[k], v...)
		}

		require.Len(t, seriesSet, len(expBlockQuery))

		// Compare all the samples except the stale value that needs special handling.
		for _, category := range [][]labels.Labels{
			staleSeries, staleHist, staleFHist,
			staleSeriesCrossingBoundary, staleHistCrossingBoundary, staleFHistCrossingBoundary,
		} {
			for i := range numSeriesPerCategory {
				seriesKey := fmt.Sprintf(`{name="%s"}`, category[i].Get("name"))
				samples := expBlockQuery[seriesKey]
				actSamples, exists := seriesSet[seriesKey]
				require.Truef(t, exists, "series not found in result %s", seriesKey)
				require.Len(t, actSamples, len(samples))

				for i := range len(samples) - 1 {
					require.Equal(t, samples[i], actSamples[i])
				}

				l := len(samples) - 1
				require.Equal(t, samples[l].T(), actSamples[l].T())
				switch {
				case value.IsStaleNaN(samples[l].F()):
					require.True(t, value.IsStaleNaN(actSamples[l].F()))
				case samples[l].H() != nil:
					require.True(t, value.IsStaleNaN(actSamples[l].H().Sum))
				default:
					require.True(t, value.IsStaleNaN(actSamples[l].FH().Sum))
				}
			}
		}
	}

	{
		// Restart DB and verify that stale series were discarded from WAL replay.
		require.NoError(t, db.Close())
		var err error
		db, err = Open(db.Dir(), db.logger, db.registerer, db.opts, nil)
		require.NoError(t, err)

		verifyHeadBlock()
	}
}

// TestStaleSeriesCompactionWithZeroSeries verifies that CompactStaleHead handles
// an empty head (0 series) gracefully without division by zero or incorrectly
// triggering compaction. This is a regression test for issue #17949.
func TestStaleSeriesCompactionWithZeroSeries(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// Verify the head is empty.
	require.Equal(t, uint64(0), db.Head().NumSeries())
	require.Equal(t, uint64(0), db.Head().NumStaleSeries())

	// CompactStaleHead should handle zero series gracefully (no panic, no error).
	require.NoError(t, db.CompactStaleHead())

	// Should still have no blocks since there was nothing to compact.
	require.Empty(t, db.Blocks())
}
