// Copyright 2017 The Prometheus Authors
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
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wal"
)

func newTestHead(t testing.TB, chunkRange int64, compressWAL bool) (*Head, *wal.WAL) {
	dir, err := ioutil.TempDir("", "test")
	require.NoError(t, err)
	wlog, err := wal.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compressWAL)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkRange = chunkRange
	opts.ChunkDirRoot = dir
	opts.EnableExemplarStorage = true
	opts.MaxExemplars.Store(config.DefaultExemplarsConfig.MaxExemplars)
	h, err := NewHead(nil, nil, wlog, opts, nil)
	require.NoError(t, err)

	require.NoError(t, h.chunkDiskMapper.IterateAllChunks(func(_, _ uint64, _, _ int64, _ uint16) error { return nil }))

	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(dir))
	})
	return h, wlog
}

func BenchmarkCreateSeries(b *testing.B) {
	series := genSeries(b.N, 10, 0, 0)
	h, _ := newTestHead(b, 10000, false)
	defer func() {
		require.NoError(b, h.Close())
	}()

	b.ReportAllocs()
	b.ResetTimer()

	for _, s := range series {
		h.getOrCreate(s.Labels().Hash(), s.Labels())
	}
}

func populateTestWAL(t testing.TB, w *wal.WAL, recs []interface{}) {
	var enc record.Encoder
	for _, r := range recs {
		switch v := r.(type) {
		case []record.RefSeries:
			require.NoError(t, w.Log(enc.Series(v, nil)))
		case []record.RefSample:
			require.NoError(t, w.Log(enc.Samples(v, nil)))
		case []tombstones.Stone:
			require.NoError(t, w.Log(enc.Tombstones(v, nil)))
		case []record.RefExemplar:
			require.NoError(t, w.Log(enc.Exemplars(v, nil)))
		}
	}
}

func readTestWAL(t testing.TB, dir string) (recs []interface{}) {
	sr, err := wal.NewSegmentsReader(dir)
	require.NoError(t, err)
	defer sr.Close()

	var dec record.Decoder
	r := wal.NewReader(sr)

	for r.Next() {
		rec := r.Record()

		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, nil)
			require.NoError(t, err)
			recs = append(recs, series)
		case record.Samples:
			samples, err := dec.Samples(rec, nil)
			require.NoError(t, err)
			recs = append(recs, samples)
		case record.Tombstones:
			tstones, err := dec.Tombstones(rec, nil)
			require.NoError(t, err)
			recs = append(recs, tstones)
		default:
			t.Fatalf("unknown record type")
		}
	}
	require.NoError(t, r.Err())
	return recs
}

func BenchmarkLoadWAL(b *testing.B) {
	cases := []struct {
		// Total series is (batches*seriesPerBatch).
		batches          int
		seriesPerBatch   int
		samplesPerSeries int
		mmappedChunkT    int64
	}{
		{ // Less series and more samples. 2 hour WAL with 1 second scrape interval.
			batches:          10,
			seriesPerBatch:   100,
			samplesPerSeries: 7200,
		},
		{ // More series and less samples.
			batches:          10,
			seriesPerBatch:   10000,
			samplesPerSeries: 50,
		},
		{ // In between.
			batches:          10,
			seriesPerBatch:   1000,
			samplesPerSeries: 480,
		},
		{ // 2 hour WAL with 15 second scrape interval, and mmapped chunks up to last 100 samples.
			batches:          100,
			seriesPerBatch:   1000,
			samplesPerSeries: 480,
			mmappedChunkT:    3800,
		},
	}

	labelsPerSeries := 5
	// Rough estimates of most common % of samples that have an exemplar for each scrape.
	exemplarsPercentages := []float64{0, 0.5, 1, 5}
	lastExemplarsPerSeries := -1
	for _, c := range cases {
		for _, p := range exemplarsPercentages {
			exemplarsPerSeries := int(math.RoundToEven(float64(c.samplesPerSeries) * p / 100))
			// For tests with low samplesPerSeries we could end up testing with 0 exemplarsPerSeries
			// multiple times without this check.
			if exemplarsPerSeries == lastExemplarsPerSeries {
				continue
			}
			lastExemplarsPerSeries = exemplarsPerSeries
			// fmt.Println("exemplars per series: ", exemplarsPerSeries)
			b.Run(fmt.Sprintf("batches=%d,seriesPerBatch=%d,samplesPerSeries=%d,exemplarsPerSeries=%d,mmappedChunkT=%d", c.batches, c.seriesPerBatch, c.samplesPerSeries, exemplarsPerSeries, c.mmappedChunkT),
				func(b *testing.B) {
					dir, err := ioutil.TempDir("", "test_load_wal")
					require.NoError(b, err)
					defer func() {
						require.NoError(b, os.RemoveAll(dir))
					}()

					w, err := wal.New(nil, nil, dir, false)
					require.NoError(b, err)

					// Write series.
					refSeries := make([]record.RefSeries, 0, c.seriesPerBatch)
					for k := 0; k < c.batches; k++ {
						refSeries = refSeries[:0]
						for i := k * c.seriesPerBatch; i < (k+1)*c.seriesPerBatch; i++ {
							lbls := make(map[string]string, labelsPerSeries)
							lbls[defaultLabelName] = strconv.Itoa(i)
							for j := 1; len(lbls) < labelsPerSeries; j++ {
								lbls[defaultLabelName+strconv.Itoa(j)] = defaultLabelValue + strconv.Itoa(j)
							}
							refSeries = append(refSeries, record.RefSeries{Ref: uint64(i) * 101, Labels: labels.FromMap(lbls)})
						}
						populateTestWAL(b, w, []interface{}{refSeries})
					}

					// Write samples.
					refSamples := make([]record.RefSample, 0, c.seriesPerBatch)
					for i := 0; i < c.samplesPerSeries; i++ {
						for j := 0; j < c.batches; j++ {
							refSamples = refSamples[:0]
							for k := j * c.seriesPerBatch; k < (j+1)*c.seriesPerBatch; k++ {
								refSamples = append(refSamples, record.RefSample{
									Ref: uint64(k) * 101,
									T:   int64(i) * 10,
									V:   float64(i) * 100,
								})
							}
							populateTestWAL(b, w, []interface{}{refSamples})
						}
					}

					// Write mmapped chunks.
					if c.mmappedChunkT != 0 {
						chunkDiskMapper, err := chunks.NewChunkDiskMapper(mmappedChunksDir(dir), chunkenc.NewPool(), chunks.DefaultWriteBufferSize)
						require.NoError(b, err)
						for k := 0; k < c.batches*c.seriesPerBatch; k++ {
							// Create one mmapped chunk per series, with one sample at the given time.
							s := newMemSeries(labels.Labels{}, uint64(k)*101, c.mmappedChunkT, nil)
							s.append(c.mmappedChunkT, 42, 0, chunkDiskMapper)
							s.mmapCurrentHeadChunk(chunkDiskMapper)
						}
						require.NoError(b, chunkDiskMapper.Close())
					}

					// Write exemplars.
					refExemplars := make([]record.RefExemplar, 0, c.seriesPerBatch)
					for i := 0; i < exemplarsPerSeries; i++ {
						for j := 0; j < c.batches; j++ {
							refExemplars = refExemplars[:0]
							for k := j * c.seriesPerBatch; k < (j+1)*c.seriesPerBatch; k++ {
								refExemplars = append(refExemplars, record.RefExemplar{
									Ref:    uint64(k) * 101,
									T:      int64(i) * 10,
									V:      float64(i) * 100,
									Labels: labels.FromStrings("traceID", fmt.Sprintf("trace-%d", i)),
								})
							}
							populateTestWAL(b, w, []interface{}{refExemplars})
						}
					}

					b.ResetTimer()

					// Load the WAL.
					for i := 0; i < b.N; i++ {
						opts := DefaultHeadOptions()
						opts.ChunkRange = 1000
						opts.ChunkDirRoot = w.Dir()
						h, err := NewHead(nil, nil, w, opts, nil)
						require.NoError(b, err)
						h.Init(0)
					}
					b.StopTimer()
					w.Close()
				})
		}
	}
}

func TestHead_ReadWAL(t *testing.T) {
	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			entries := []interface{}{
				[]record.RefSeries{
					{Ref: 10, Labels: labels.FromStrings("a", "1")},
					{Ref: 11, Labels: labels.FromStrings("a", "2")},
					{Ref: 100, Labels: labels.FromStrings("a", "3")},
				},
				[]record.RefSample{
					{Ref: 0, T: 99, V: 1},
					{Ref: 10, T: 100, V: 2},
					{Ref: 100, T: 100, V: 3},
				},
				[]record.RefSeries{
					{Ref: 50, Labels: labels.FromStrings("a", "4")},
					// This series has two refs pointing to it.
					{Ref: 101, Labels: labels.FromStrings("a", "3")},
				},
				[]record.RefSample{
					{Ref: 10, T: 101, V: 5},
					{Ref: 50, T: 101, V: 6},
					{Ref: 101, T: 101, V: 7},
				},
				[]tombstones.Stone{
					{Ref: 0, Intervals: []tombstones.Interval{{Mint: 99, Maxt: 101}}},
				},
				[]record.RefExemplar{
					{Ref: 10, T: 100, V: 1, Labels: labels.FromStrings("traceID", "asdf")},
				},
			}

			head, w := newTestHead(t, 1000, compress)
			defer func() {
				require.NoError(t, head.Close())
			}()

			populateTestWAL(t, w, entries)

			require.NoError(t, head.Init(math.MinInt64))
			require.Equal(t, uint64(101), head.lastSeriesID.Load())

			s10 := head.series.getByID(10)
			s11 := head.series.getByID(11)
			s50 := head.series.getByID(50)
			s100 := head.series.getByID(100)

			require.Equal(t, labels.FromStrings("a", "1"), s10.lset)
			require.Equal(t, (*memSeries)(nil), s11) // Series without samples should be garbage collected at head.Init().
			require.Equal(t, labels.FromStrings("a", "4"), s50.lset)
			require.Equal(t, labels.FromStrings("a", "3"), s100.lset)

			expandChunk := func(c chunkenc.Iterator) (x []sample) {
				for c.Next() {
					t, v := c.At()
					x = append(x, sample{t: t, v: v})
				}
				require.NoError(t, c.Err())
				return x
			}
			require.Equal(t, []sample{{100, 2}, {101, 5}}, expandChunk(s10.iterator(0, nil, head.chunkDiskMapper, nil)))
			require.Equal(t, []sample{{101, 6}}, expandChunk(s50.iterator(0, nil, head.chunkDiskMapper, nil)))
			// The samples before the new series record should be discarded since a duplicate record
			// is only possible when old samples were compacted.
			require.Equal(t, []sample{{101, 7}}, expandChunk(s100.iterator(0, nil, head.chunkDiskMapper, nil)))

			q, err := head.ExemplarQuerier(context.Background())
			require.NoError(t, err)
			e, err := q.Select(0, 1000, []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "1")})
			require.NoError(t, err)
			require.Equal(t, e[0].Exemplars[0], exemplar.Exemplar{Ts: 100, Value: 1, Labels: labels.FromStrings("traceID", "asdf")})
		})
	}
}

func TestHead_WALMultiRef(t *testing.T) {
	head, w := newTestHead(t, 1000, false)

	require.NoError(t, head.Init(0))

	app := head.Appender(context.Background())
	ref1, err := app.Append(0, labels.FromStrings("foo", "bar"), 100, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	// Add another sample outside chunk range to mmap a chunk.
	app = head.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 1500, 2)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 2.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	require.NoError(t, head.Truncate(1600))

	app = head.Appender(context.Background())
	ref2, err := app.Append(0, labels.FromStrings("foo", "bar"), 1700, 3)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 3.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	// Add another sample outside chunk range to mmap a chunk.
	app = head.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 2000, 4)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 4.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	require.NotEqual(t, ref1, ref2, "Refs are the same")
	require.NoError(t, head.Close())

	w, err = wal.New(nil, nil, w.Dir(), false)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = w.Dir()
	head, err = NewHead(nil, nil, w, opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(0))
	defer func() {
		require.NoError(t, head.Close())
	}()

	q, err := NewBlockQuerier(head, 0, 2100)
	require.NoError(t, err)
	series := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	// The samples before the new ref should be discarded since Head truncation
	// happens only after compacting the Head.
	require.Equal(t, map[string][]tsdbutil.Sample{`{foo="bar"}`: {
		sample{1700, 3},
		sample{2000, 4},
	}}, series)
}

func TestHead_UnknownWALRecord(t *testing.T) {
	head, w := newTestHead(t, 1000, false)
	w.Log([]byte{255, 42})
	require.NoError(t, head.Init(0))
	require.NoError(t, head.Close())
}

func TestHead_Truncate(t *testing.T) {
	h, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	s1, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1", "b", "1"))
	s2, _, _ := h.getOrCreate(2, labels.FromStrings("a", "2", "b", "1"))
	s3, _, _ := h.getOrCreate(3, labels.FromStrings("a", "1", "b", "2"))
	s4, _, _ := h.getOrCreate(4, labels.FromStrings("a", "2", "b", "2", "c", "1"))

	s1.mmappedChunks = []*mmappedChunk{
		{minTime: 0, maxTime: 999},
		{minTime: 1000, maxTime: 1999},
		{minTime: 2000, maxTime: 2999},
	}
	s2.mmappedChunks = []*mmappedChunk{
		{minTime: 1000, maxTime: 1999},
		{minTime: 2000, maxTime: 2999},
		{minTime: 3000, maxTime: 3999},
	}
	s3.mmappedChunks = []*mmappedChunk{
		{minTime: 0, maxTime: 999},
		{minTime: 1000, maxTime: 1999},
	}
	s4.mmappedChunks = []*mmappedChunk{}

	// Truncation need not be aligned.
	require.NoError(t, h.Truncate(1))

	require.NoError(t, h.Truncate(2000))

	require.Equal(t, []*mmappedChunk{
		{minTime: 2000, maxTime: 2999},
	}, h.series.getByID(s1.ref).mmappedChunks)

	require.Equal(t, []*mmappedChunk{
		{minTime: 2000, maxTime: 2999},
		{minTime: 3000, maxTime: 3999},
	}, h.series.getByID(s2.ref).mmappedChunks)

	require.Nil(t, h.series.getByID(s3.ref))
	require.Nil(t, h.series.getByID(s4.ref))

	postingsA1, _ := index.ExpandPostings(h.postings.Get("a", "1"))
	postingsA2, _ := index.ExpandPostings(h.postings.Get("a", "2"))
	postingsB1, _ := index.ExpandPostings(h.postings.Get("b", "1"))
	postingsB2, _ := index.ExpandPostings(h.postings.Get("b", "2"))
	postingsC1, _ := index.ExpandPostings(h.postings.Get("c", "1"))
	postingsAll, _ := index.ExpandPostings(h.postings.Get("", ""))

	require.Equal(t, []uint64{s1.ref}, postingsA1)
	require.Equal(t, []uint64{s2.ref}, postingsA2)
	require.Equal(t, []uint64{s1.ref, s2.ref}, postingsB1)
	require.Equal(t, []uint64{s1.ref, s2.ref}, postingsAll)
	require.Nil(t, postingsB2)
	require.Nil(t, postingsC1)

	require.Equal(t, map[string]struct{}{
		"":  {}, // from 'all' postings list
		"a": {},
		"b": {},
		"1": {},
		"2": {},
	}, h.symbols)

	values := map[string]map[string]struct{}{}
	for _, name := range h.postings.LabelNames() {
		ss, ok := values[name]
		if !ok {
			ss = map[string]struct{}{}
			values[name] = ss
		}
		for _, value := range h.postings.LabelValues(name) {
			ss[value] = struct{}{}
		}
	}
	require.Equal(t, map[string]map[string]struct{}{
		"a": {"1": struct{}{}, "2": struct{}{}},
		"b": {"1": struct{}{}},
	}, values)
}

// Validate various behaviors brought on by firstChunkID accounting for
// garbage collected chunks.
func TestMemSeries_truncateChunks(t *testing.T) {
	dir, err := ioutil.TempDir("", "truncate_chunks")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(dir))
	}()
	// This is usually taken from the Head, but passing manually here.
	chunkDiskMapper, err := chunks.NewChunkDiskMapper(dir, chunkenc.NewPool(), chunks.DefaultWriteBufferSize)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, chunkDiskMapper.Close())
	}()

	memChunkPool := sync.Pool{
		New: func() interface{} {
			return &memChunk{}
		},
	}

	s := newMemSeries(labels.FromStrings("a", "b"), 1, 2000, &memChunkPool)

	for i := 0; i < 4000; i += 5 {
		ok, _ := s.append(int64(i), float64(i), 0, chunkDiskMapper)
		require.True(t, ok, "sample append failed")
	}

	// Check that truncate removes half of the chunks and afterwards
	// that the ID of the last chunk still gives us the same chunk afterwards.
	countBefore := len(s.mmappedChunks) + 1 // +1 for the head chunk.
	lastID := s.chunkID(countBefore - 1)
	lastChunk, _, err := s.chunk(lastID, chunkDiskMapper)
	require.NoError(t, err)
	require.NotNil(t, lastChunk)

	chk, _, err := s.chunk(0, chunkDiskMapper)
	require.NotNil(t, chk)
	require.NoError(t, err)

	s.truncateChunksBefore(2000)

	require.Equal(t, int64(2000), s.mmappedChunks[0].minTime)
	_, _, err = s.chunk(0, chunkDiskMapper)
	require.Equal(t, storage.ErrNotFound, err, "first chunks not gone")
	require.Equal(t, countBefore/2, len(s.mmappedChunks)+1) // +1 for the head chunk.
	chk, _, err = s.chunk(lastID, chunkDiskMapper)
	require.NoError(t, err)
	require.Equal(t, lastChunk, chk)

	// Validate that the series' sample buffer is applied correctly to the last chunk
	// after truncation.
	it1 := s.iterator(s.chunkID(len(s.mmappedChunks)), nil, chunkDiskMapper, nil)
	_, ok := it1.(*memSafeIterator)
	require.True(t, ok)

	it2 := s.iterator(s.chunkID(len(s.mmappedChunks)-1), nil, chunkDiskMapper, nil)
	_, ok = it2.(*memSafeIterator)
	require.False(t, ok, "non-last chunk incorrectly wrapped with sample buffer")
}

func TestHeadDeleteSeriesWithoutSamples(t *testing.T) {
	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			entries := []interface{}{
				[]record.RefSeries{
					{Ref: 10, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefSample{},
				[]record.RefSeries{
					{Ref: 50, Labels: labels.FromStrings("a", "2")},
				},
				[]record.RefSample{
					{Ref: 50, T: 80, V: 1},
					{Ref: 50, T: 90, V: 1},
				},
			}
			head, w := newTestHead(t, 1000, compress)
			defer func() {
				require.NoError(t, head.Close())
			}()

			populateTestWAL(t, w, entries)

			require.NoError(t, head.Init(math.MinInt64))

			require.NoError(t, head.Delete(0, 100, labels.MustNewMatcher(labels.MatchEqual, "a", "1")))
		})
	}
}

func TestHeadDeleteSimple(t *testing.T) {
	buildSmpls := func(s []int64) []sample {
		ss := make([]sample, 0, len(s))
		for _, t := range s {
			ss = append(ss, sample{t: t, v: float64(t)})
		}
		return ss
	}
	smplsAll := buildSmpls([]int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	lblDefault := labels.Label{Name: "a", Value: "b"}

	cases := []struct {
		dranges    tombstones.Intervals
		addSamples []sample // Samples to add after delete.
		smplsExp   []sample
	}{
		{
			dranges:  tombstones.Intervals{{Mint: 0, Maxt: 3}},
			smplsExp: buildSmpls([]int64{4, 5, 6, 7, 8, 9}),
		},
		{
			dranges:  tombstones.Intervals{{Mint: 1, Maxt: 3}},
			smplsExp: buildSmpls([]int64{0, 4, 5, 6, 7, 8, 9}),
		},
		{
			dranges:  tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 7}},
			smplsExp: buildSmpls([]int64{0, 8, 9}),
		},
		{
			dranges:  tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 700}},
			smplsExp: buildSmpls([]int64{0}),
		},
		{ // This case is to ensure that labels and symbols are deleted.
			dranges:  tombstones.Intervals{{Mint: 0, Maxt: 9}},
			smplsExp: buildSmpls([]int64{}),
		},
		{
			dranges:    tombstones.Intervals{{Mint: 1, Maxt: 3}},
			addSamples: buildSmpls([]int64{11, 13, 15}),
			smplsExp:   buildSmpls([]int64{0, 4, 5, 6, 7, 8, 9, 11, 13, 15}),
		},
		{
			// After delete, the appended samples in the deleted range should be visible
			// as the tombstones are clamped to head min/max time.
			dranges:    tombstones.Intervals{{Mint: 7, Maxt: 20}},
			addSamples: buildSmpls([]int64{11, 13, 15}),
			smplsExp:   buildSmpls([]int64{0, 1, 2, 3, 4, 5, 6, 11, 13, 15}),
		},
	}

	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			for _, c := range cases {
				head, w := newTestHead(t, 1000, compress)

				app := head.Appender(context.Background())
				for _, smpl := range smplsAll {
					_, err := app.Append(0, labels.Labels{lblDefault}, smpl.t, smpl.v)
					require.NoError(t, err)

				}
				require.NoError(t, app.Commit())

				// Delete the ranges.
				for _, r := range c.dranges {
					require.NoError(t, head.Delete(r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, lblDefault.Name, lblDefault.Value)))
				}

				// Add more samples.
				app = head.Appender(context.Background())
				for _, smpl := range c.addSamples {
					_, err := app.Append(0, labels.Labels{lblDefault}, smpl.t, smpl.v)
					require.NoError(t, err)

				}
				require.NoError(t, app.Commit())

				// Compare the samples for both heads - before and after the reloadBlocks.
				reloadedW, err := wal.New(nil, nil, w.Dir(), compress) // Use a new wal to ensure deleted samples are gone even after a reloadBlocks.
				require.NoError(t, err)
				opts := DefaultHeadOptions()
				opts.ChunkRange = 1000
				opts.ChunkDirRoot = reloadedW.Dir()
				reloadedHead, err := NewHead(nil, nil, reloadedW, opts, nil)
				require.NoError(t, err)
				require.NoError(t, reloadedHead.Init(0))

				// Compare the query results for both heads - before and after the reloadBlocks.
			Outer:
				for _, h := range []*Head{head, reloadedHead} {
					q, err := NewBlockQuerier(h, h.MinTime(), h.MaxTime())
					require.NoError(t, err)
					actSeriesSet := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, lblDefault.Name, lblDefault.Value))
					require.NoError(t, q.Close())
					expSeriesSet := newMockSeriesSet([]storage.Series{
						storage.NewListSeries(labels.Labels{lblDefault}, func() []tsdbutil.Sample {
							ss := make([]tsdbutil.Sample, 0, len(c.smplsExp))
							for _, s := range c.smplsExp {
								ss = append(ss, s)
							}
							return ss
						}(),
						),
					})

					for {
						eok, rok := expSeriesSet.Next(), actSeriesSet.Next()
						require.Equal(t, eok, rok)

						if !eok {
							require.NoError(t, h.Close())
							require.NoError(t, actSeriesSet.Err())
							require.Equal(t, 0, len(actSeriesSet.Warnings()))
							continue Outer
						}
						expSeries := expSeriesSet.At()
						actSeries := actSeriesSet.At()

						require.Equal(t, expSeries.Labels(), actSeries.Labels())

						smplExp, errExp := storage.ExpandSamples(expSeries.Iterator(), nil)
						smplRes, errRes := storage.ExpandSamples(actSeries.Iterator(), nil)

						require.Equal(t, errExp, errRes)
						require.Equal(t, smplExp, smplRes)
					}
				}
			}
		})
	}
}

func TestDeleteUntilCurMax(t *testing.T) {
	hb, _ := newTestHead(t, 1000000, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	numSamples := int64(10)
	app := hb.Appender(context.Background())
	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		_, err := app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, i, smpls[i])
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
	require.NoError(t, hb.Delete(0, 10000, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))

	// Test the series returns no samples. The series is cleared only after compaction.
	q, err := NewBlockQuerier(hb, 0, 100000)
	require.NoError(t, err)
	res := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.True(t, res.Next(), "series is not present")
	s := res.At()
	it := s.Iterator()
	require.False(t, it.Next(), "expected no samples")
	for res.Next() {
	}
	require.NoError(t, res.Err())
	require.Equal(t, 0, len(res.Warnings()))

	// Add again and test for presence.
	app = hb.Appender(context.Background())
	_, err = app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 11, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	q, err = NewBlockQuerier(hb, 0, 100000)
	require.NoError(t, err)
	res = q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.True(t, res.Next(), "series don't exist")
	exps := res.At()
	it = exps.Iterator()
	resSamples, err := storage.ExpandSamples(it, newSample)
	require.NoError(t, err)
	require.Equal(t, []tsdbutil.Sample{sample{11, 1}}, resSamples)
	for res.Next() {
	}
	require.NoError(t, res.Err())
	require.Equal(t, 0, len(res.Warnings()))
}

func TestDeletedSamplesAndSeriesStillInWALAfterCheckpoint(t *testing.T) {
	numSamples := 10000

	// Enough samples to cause a checkpoint.
	hb, w := newTestHead(t, int64(numSamples)*10, false)

	for i := 0; i < numSamples; i++ {
		app := hb.Appender(context.Background())
		_, err := app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, int64(i), 0)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}
	require.NoError(t, hb.Delete(0, int64(numSamples), labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
	require.NoError(t, hb.Truncate(1))
	require.NoError(t, hb.Close())

	// Confirm there's been a checkpoint.
	cdir, _, err := wal.LastCheckpoint(w.Dir())
	require.NoError(t, err)
	// Read in checkpoint and WAL.
	recs := readTestWAL(t, cdir)
	recs = append(recs, readTestWAL(t, w.Dir())...)

	var series, samples, stones int
	for _, rec := range recs {
		switch rec.(type) {
		case []record.RefSeries:
			series++
		case []record.RefSample:
			samples++
		case []tombstones.Stone:
			stones++
		default:
			t.Fatalf("unknown record type")
		}
	}
	require.Equal(t, 1, series)
	require.Equal(t, 9999, samples)
	require.Equal(t, 1, stones)

}

func TestDelete_e2e(t *testing.T) {
	numDatapoints := 1000
	numRanges := 1000
	timeInterval := int64(2)
	// Create 8 series with 1000 data-points of different ranges, delete and run queries.
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
	seriesMap := map[string][]tsdbutil.Sample{}
	for _, l := range lbls {
		seriesMap[labels.New(l...).String()] = []tsdbutil.Sample{}
	}

	hb, _ := newTestHead(t, 100000, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	app := hb.Appender(context.Background())
	for _, l := range lbls {
		ls := labels.New(l...)
		series := []tsdbutil.Sample{}
		ts := rand.Int63n(300)
		for i := 0; i < numDatapoints; i++ {
			v := rand.Float64()
			_, err := app.Append(0, ls, ts, v)
			require.NoError(t, err)
			series = append(series, sample{ts, v})
			ts += rand.Int63n(timeInterval) + 1
		}
		seriesMap[labels.New(l...).String()] = series
	}
	require.NoError(t, app.Commit())
	// Delete a time-range from each-selector.
	dels := []struct {
		ms     []*labels.Matcher
		drange tombstones.Intervals
	}{
		{
			ms:     []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "b")},
			drange: tombstones.Intervals{{Mint: 300, Maxt: 500}, {Mint: 600, Maxt: 670}},
		},
		{
			ms: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "b"),
				labels.MustNewMatcher(labels.MatchEqual, "job", "prom-k8s"),
			},
			drange: tombstones.Intervals{{Mint: 300, Maxt: 500}, {Mint: 100, Maxt: 670}},
		},
		{
			ms: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "c"),
				labels.MustNewMatcher(labels.MatchEqual, "instance", "localhost:9090"),
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
			},
			drange: tombstones.Intervals{{Mint: 300, Maxt: 400}, {Mint: 100, Maxt: 6700}},
		},
		// TODO: Add Regexp Matchers.
	}
	for _, del := range dels {
		for _, r := range del.drange {
			require.NoError(t, hb.Delete(r.Mint, r.Maxt, del.ms...))
		}
		matched := labels.Slice{}
		for _, ls := range lbls {
			s := labels.Selector(del.ms)
			if s.Matches(ls) {
				matched = append(matched, ls)
			}
		}
		sort.Sort(matched)
		for i := 0; i < numRanges; i++ {
			q, err := NewBlockQuerier(hb, 0, 100000)
			require.NoError(t, err)
			defer q.Close()
			ss := q.Select(true, nil, del.ms...)
			// Build the mockSeriesSet.
			matchedSeries := make([]storage.Series, 0, len(matched))
			for _, m := range matched {
				smpls := seriesMap[m.String()]
				smpls = deletedSamples(smpls, del.drange)
				// Only append those series for which samples exist as mockSeriesSet
				// doesn't skip series with no samples.
				// TODO: But sometimes SeriesSet returns an empty chunkenc.Iterator
				if len(smpls) > 0 {
					matchedSeries = append(matchedSeries, storage.NewListSeries(m, smpls))
				}
			}
			expSs := newMockSeriesSet(matchedSeries)
			// Compare both SeriesSets.
			for {
				eok, rok := expSs.Next(), ss.Next()
				// Skip a series if iterator is empty.
				if rok {
					for !ss.At().Iterator().Next() {
						rok = ss.Next()
						if !rok {
							break
						}
					}
				}
				require.Equal(t, eok, rok)
				if !eok {
					break
				}
				sexp := expSs.At()
				sres := ss.At()
				require.Equal(t, sexp.Labels(), sres.Labels())
				smplExp, errExp := storage.ExpandSamples(sexp.Iterator(), nil)
				smplRes, errRes := storage.ExpandSamples(sres.Iterator(), nil)
				require.Equal(t, errExp, errRes)
				require.Equal(t, smplExp, smplRes)
			}
			require.NoError(t, ss.Err())
			require.Equal(t, 0, len(ss.Warnings()))
		}
	}
}

func boundedSamples(full []tsdbutil.Sample, mint, maxt int64) []tsdbutil.Sample {
	for len(full) > 0 {
		if full[0].T() >= mint {
			break
		}
		full = full[1:]
	}
	for i, s := range full {
		// labels.Labelinate on the first sample larger than maxt.
		if s.T() > maxt {
			return full[:i]
		}
	}
	// maxt is after highest sample.
	return full
}

func deletedSamples(full []tsdbutil.Sample, dranges tombstones.Intervals) []tsdbutil.Sample {
	ds := make([]tsdbutil.Sample, 0, len(full))
Outer:
	for _, s := range full {
		for _, r := range dranges {
			if r.InBounds(s.T()) {
				continue Outer
			}
		}
		ds = append(ds, s)
	}

	return ds
}

func TestComputeChunkEndTime(t *testing.T) {
	cases := []struct {
		start, cur, max int64
		res             int64
	}{
		{
			start: 0,
			cur:   250,
			max:   1000,
			res:   1000,
		},
		{
			start: 100,
			cur:   200,
			max:   1000,
			res:   550,
		},
		// Case where we fit floored 0 chunks. Must catch division by 0
		// and default to maximum time.
		{
			start: 0,
			cur:   500,
			max:   1000,
			res:   1000,
		},
		// Catch division by zero for cur == start. Strictly not a possible case.
		{
			start: 100,
			cur:   100,
			max:   1000,
			res:   104,
		},
	}

	for _, c := range cases {
		got := computeChunkEndTime(c.start, c.cur, c.max)
		if got != c.res {
			t.Errorf("expected %d for (start: %d, cur: %d, max: %d), got %d", c.res, c.start, c.cur, c.max, got)
		}
	}
}

func TestMemSeries_append(t *testing.T) {
	dir, err := ioutil.TempDir("", "append")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(dir))
	}()
	// This is usually taken from the Head, but passing manually here.
	chunkDiskMapper, err := chunks.NewChunkDiskMapper(dir, chunkenc.NewPool(), chunks.DefaultWriteBufferSize)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, chunkDiskMapper.Close())
	}()

	s := newMemSeries(labels.Labels{}, 1, 500, nil)

	// Add first two samples at the very end of a chunk range and the next two
	// on and after it.
	// New chunk must correctly be cut at 1000.
	ok, chunkCreated := s.append(998, 1, 0, chunkDiskMapper)
	require.True(t, ok, "append failed")
	require.True(t, chunkCreated, "first sample created chunk")

	ok, chunkCreated = s.append(999, 2, 0, chunkDiskMapper)
	require.True(t, ok, "append failed")
	require.False(t, chunkCreated, "second sample should use same chunk")

	ok, chunkCreated = s.append(1000, 3, 0, chunkDiskMapper)
	require.True(t, ok, "append failed")
	require.True(t, chunkCreated, "expected new chunk on boundary")

	ok, chunkCreated = s.append(1001, 4, 0, chunkDiskMapper)
	require.True(t, ok, "append failed")
	require.False(t, chunkCreated, "second sample should use same chunk")

	require.Equal(t, 1, len(s.mmappedChunks), "there should be only 1 mmapped chunk")
	require.Equal(t, int64(998), s.mmappedChunks[0].minTime, "wrong chunk range")
	require.Equal(t, int64(999), s.mmappedChunks[0].maxTime, "wrong chunk range")
	require.Equal(t, int64(1000), s.headChunk.minTime, "wrong chunk range")
	require.Equal(t, int64(1001), s.headChunk.maxTime, "wrong chunk range")

	// Fill the range [1000,2000) with many samples. Intermediate chunks should be cut
	// at approximately 120 samples per chunk.
	for i := 1; i < 1000; i++ {
		ok, _ := s.append(1001+int64(i), float64(i), 0, chunkDiskMapper)
		require.True(t, ok, "append failed")
	}

	require.Greater(t, len(s.mmappedChunks)+1, 7, "expected intermediate chunks")

	// All chunks but the first and last should now be moderately full.
	for i, c := range s.mmappedChunks[1:] {
		chk, err := chunkDiskMapper.Chunk(c.ref)
		require.NoError(t, err)
		require.Greater(t, chk.NumSamples(), 100, "unexpected small chunk %d of length %d", i, chk.NumSamples())
	}
}

func TestGCChunkAccess(t *testing.T) {
	// Put a chunk, select it. GC it and then access it.
	h, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	s, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))

	// Appending 2 samples for the first chunk.
	ok, chunkCreated := s.append(0, 0, 0, h.chunkDiskMapper)
	require.True(t, ok, "series append failed")
	require.True(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(999, 999, 0, h.chunkDiskMapper)
	require.True(t, ok, "series append failed")
	require.False(t, chunkCreated, "chunks was created")

	// A new chunks should be created here as it's beyond the chunk range.
	ok, chunkCreated = s.append(1000, 1000, 0, h.chunkDiskMapper)
	require.True(t, ok, "series append failed")
	require.True(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(1999, 1999, 0, h.chunkDiskMapper)
	require.True(t, ok, "series append failed")
	require.False(t, chunkCreated, "chunks was created")

	idx := h.indexRange(0, 1500)
	var (
		lset   labels.Labels
		chunks []chunks.Meta
	)
	require.NoError(t, idx.Series(1, &lset, &chunks))

	require.Equal(t, labels.Labels{{
		Name: "a", Value: "1",
	}}, lset)
	require.Equal(t, 2, len(chunks))

	cr, err := h.chunksRange(0, 1500, nil)
	require.NoError(t, err)
	_, err = cr.Chunk(chunks[0].Ref)
	require.NoError(t, err)
	_, err = cr.Chunk(chunks[1].Ref)
	require.NoError(t, err)

	require.NoError(t, h.Truncate(1500)) // Remove a chunk.

	_, err = cr.Chunk(chunks[0].Ref)
	require.Equal(t, storage.ErrNotFound, err)
	_, err = cr.Chunk(chunks[1].Ref)
	require.NoError(t, err)
}

func TestGCSeriesAccess(t *testing.T) {
	// Put a series, select it. GC it and then access it.
	h, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	s, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))

	// Appending 2 samples for the first chunk.
	ok, chunkCreated := s.append(0, 0, 0, h.chunkDiskMapper)
	require.True(t, ok, "series append failed")
	require.True(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(999, 999, 0, h.chunkDiskMapper)
	require.True(t, ok, "series append failed")
	require.False(t, chunkCreated, "chunks was created")

	// A new chunks should be created here as it's beyond the chunk range.
	ok, chunkCreated = s.append(1000, 1000, 0, h.chunkDiskMapper)
	require.True(t, ok, "series append failed")
	require.True(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(1999, 1999, 0, h.chunkDiskMapper)
	require.True(t, ok, "series append failed")
	require.False(t, chunkCreated, "chunks was created")

	idx := h.indexRange(0, 2000)
	var (
		lset   labels.Labels
		chunks []chunks.Meta
	)
	require.NoError(t, idx.Series(1, &lset, &chunks))

	require.Equal(t, labels.Labels{{
		Name: "a", Value: "1",
	}}, lset)
	require.Equal(t, 2, len(chunks))

	cr, err := h.chunksRange(0, 2000, nil)
	require.NoError(t, err)
	_, err = cr.Chunk(chunks[0].Ref)
	require.NoError(t, err)
	_, err = cr.Chunk(chunks[1].Ref)
	require.NoError(t, err)

	require.NoError(t, h.Truncate(2000)) // Remove the series.

	require.Equal(t, (*memSeries)(nil), h.series.getByID(1))

	_, err = cr.Chunk(chunks[0].Ref)
	require.Equal(t, storage.ErrNotFound, err)
	_, err = cr.Chunk(chunks[1].Ref)
	require.Equal(t, storage.ErrNotFound, err)
}

func TestUncommittedSamplesNotLostOnTruncate(t *testing.T) {
	h, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	app := h.appender()
	lset := labels.FromStrings("a", "1")
	_, err := app.Append(0, lset, 2100, 1)
	require.NoError(t, err)

	require.NoError(t, h.Truncate(2000))
	require.NotNil(t, h.series.getByHash(lset.Hash(), lset), "series should not have been garbage collected")

	require.NoError(t, app.Commit())

	q, err := NewBlockQuerier(h, 1500, 2500)
	require.NoError(t, err)
	defer q.Close()

	ss := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "1"))
	require.Equal(t, true, ss.Next())
	for ss.Next() {
	}
	require.NoError(t, ss.Err())
	require.Equal(t, 0, len(ss.Warnings()))
}

func TestRemoveSeriesAfterRollbackAndTruncate(t *testing.T) {
	h, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	app := h.appender()
	lset := labels.FromStrings("a", "1")
	_, err := app.Append(0, lset, 2100, 1)
	require.NoError(t, err)

	require.NoError(t, h.Truncate(2000))
	require.NotNil(t, h.series.getByHash(lset.Hash(), lset), "series should not have been garbage collected")

	require.NoError(t, app.Rollback())

	q, err := NewBlockQuerier(h, 1500, 2500)
	require.NoError(t, err)

	ss := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "1"))
	require.Equal(t, false, ss.Next())
	require.Equal(t, 0, len(ss.Warnings()))
	require.NoError(t, q.Close())

	// Truncate again, this time the series should be deleted
	require.NoError(t, h.Truncate(2050))
	require.Equal(t, (*memSeries)(nil), h.series.getByHash(lset.Hash(), lset))
}

func TestHead_LogRollback(t *testing.T) {
	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			h, w := newTestHead(t, 1000, compress)
			defer func() {
				require.NoError(t, h.Close())
			}()

			app := h.Appender(context.Background())
			_, err := app.Append(0, labels.FromStrings("a", "b"), 1, 2)
			require.NoError(t, err)

			require.NoError(t, app.Rollback())
			recs := readTestWAL(t, w.Dir())

			require.Equal(t, 1, len(recs))

			series, ok := recs[0].([]record.RefSeries)
			require.True(t, ok, "expected series record but got %+v", recs[0])
			require.Equal(t, []record.RefSeries{{Ref: 1, Labels: labels.FromStrings("a", "b")}}, series)
		})
	}
}

// TestWalRepair_DecodingError ensures that a repair is run for an error
// when decoding a record.
func TestWalRepair_DecodingError(t *testing.T) {
	var enc record.Encoder
	for name, test := range map[string]struct {
		corrFunc  func(rec []byte) []byte // Func that applies the corruption to a record.
		rec       []byte
		totalRecs int
		expRecs   int
	}{
		"decode_series": {
			func(rec []byte) []byte {
				return rec[:3]
			},
			enc.Series([]record.RefSeries{{Ref: 1, Labels: labels.FromStrings("a", "b")}}, []byte{}),
			9,
			5,
		},
		"decode_samples": {
			func(rec []byte) []byte {
				return rec[:3]
			},
			enc.Samples([]record.RefSample{{Ref: 0, T: 99, V: 1}}, []byte{}),
			9,
			5,
		},
		"decode_tombstone": {
			func(rec []byte) []byte {
				return rec[:3]
			},
			enc.Tombstones([]tombstones.Stone{{Ref: 1, Intervals: tombstones.Intervals{}}}, []byte{}),
			9,
			5,
		},
	} {
		for _, compress := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s,compress=%t", name, compress), func(t *testing.T) {
				dir, err := ioutil.TempDir("", "wal_repair")
				require.NoError(t, err)
				defer func() {
					require.NoError(t, os.RemoveAll(dir))
				}()

				// Fill the wal and corrupt it.
				{
					w, err := wal.New(nil, nil, filepath.Join(dir, "wal"), compress)
					require.NoError(t, err)

					for i := 1; i <= test.totalRecs; i++ {
						// At this point insert a corrupted record.
						if i-1 == test.expRecs {
							require.NoError(t, w.Log(test.corrFunc(test.rec)))
							continue
						}
						require.NoError(t, w.Log(test.rec))
					}

					opts := DefaultHeadOptions()
					opts.ChunkRange = 1
					opts.ChunkDirRoot = w.Dir()
					h, err := NewHead(nil, nil, w, opts, nil)
					require.NoError(t, err)
					require.Equal(t, 0.0, prom_testutil.ToFloat64(h.metrics.walCorruptionsTotal))
					initErr := h.Init(math.MinInt64)

					err = errors.Cause(initErr) // So that we can pick up errors even if wrapped.
					_, corrErr := err.(*wal.CorruptionErr)
					require.True(t, corrErr, "reading the wal didn't return corruption error")
					require.NoError(t, w.Close())
				}

				// Open the db to trigger a repair.
				{
					db, err := Open(dir, nil, nil, DefaultOptions(), nil)
					require.NoError(t, err)
					defer func() {
						require.NoError(t, db.Close())
					}()
					require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal))
				}

				// Read the wal content after the repair.
				{
					sr, err := wal.NewSegmentsReader(filepath.Join(dir, "wal"))
					require.NoError(t, err)
					defer sr.Close()
					r := wal.NewReader(sr)

					var actRec int
					for r.Next() {
						actRec++
					}
					require.NoError(t, r.Err())
					require.Equal(t, test.expRecs, actRec, "Wrong number of intact records")
				}
			})
		}
	}
}

func TestHeadReadWriterRepair(t *testing.T) {
	dir, err := ioutil.TempDir("", "head_read_writer_repair")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(dir))
	}()

	const chunkRange = 1000

	walDir := filepath.Join(dir, "wal")
	// Fill the chunk segments and corrupt it.
	{
		w, err := wal.New(nil, nil, walDir, false)
		require.NoError(t, err)

		opts := DefaultHeadOptions()
		opts.ChunkRange = chunkRange
		opts.ChunkDirRoot = dir
		h, err := NewHead(nil, nil, w, opts, nil)
		require.NoError(t, err)
		require.Equal(t, 0.0, prom_testutil.ToFloat64(h.metrics.mmapChunkCorruptionTotal))
		require.NoError(t, h.Init(math.MinInt64))

		s, created, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))
		require.True(t, created, "series was not created")

		for i := 0; i < 7; i++ {
			ok, chunkCreated := s.append(int64(i*chunkRange), float64(i*chunkRange), 0, h.chunkDiskMapper)
			require.True(t, ok, "series append failed")
			require.True(t, chunkCreated, "chunk was not created")
			ok, chunkCreated = s.append(int64(i*chunkRange)+chunkRange-1, float64(i*chunkRange), 0, h.chunkDiskMapper)
			require.True(t, ok, "series append failed")
			require.False(t, chunkCreated, "chunk was created")
			require.NoError(t, h.chunkDiskMapper.CutNewFile())
		}
		require.NoError(t, h.Close())

		// Verify that there are 7 segment files.
		files, err := ioutil.ReadDir(mmappedChunksDir(dir))
		require.NoError(t, err)
		require.Equal(t, 7, len(files))

		// Corrupt the 4th file by writing a random byte to series ref.
		f, err := os.OpenFile(filepath.Join(mmappedChunksDir(dir), files[3].Name()), os.O_WRONLY, 0666)
		require.NoError(t, err)
		n, err := f.WriteAt([]byte{67, 88}, chunks.HeadChunkFileHeaderSize+2)
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.NoError(t, f.Close())
	}

	// Open the db to trigger a repair.
	{
		db, err := Open(dir, nil, nil, DefaultOptions(), nil)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, db.Close())
		}()
		require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.mmapChunkCorruptionTotal))
	}

	// Verify that there are 3 segment files after the repair.
	// The segments from the corrupt segment should be removed.
	{
		files, err := ioutil.ReadDir(mmappedChunksDir(dir))
		require.NoError(t, err)
		require.Equal(t, 3, len(files))
	}
}

func TestNewWalSegmentOnTruncate(t *testing.T) {
	h, wlog := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, h.Close())
	}()
	add := func(ts int64) {
		app := h.Appender(context.Background())
		_, err := app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, ts, 0)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	add(0)
	_, last, err := wal.Segments(wlog.Dir())
	require.NoError(t, err)
	require.Equal(t, 0, last)

	add(1)
	require.NoError(t, h.Truncate(1))
	_, last, err = wal.Segments(wlog.Dir())
	require.NoError(t, err)
	require.Equal(t, 1, last)

	add(2)
	require.NoError(t, h.Truncate(2))
	_, last, err = wal.Segments(wlog.Dir())
	require.NoError(t, err)
	require.Equal(t, 2, last)
}

func TestAddDuplicateLabelName(t *testing.T) {
	h, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	add := func(labels labels.Labels, labelName string) {
		app := h.Appender(context.Background())
		_, err := app.Append(0, labels, 0, 0)
		require.Error(t, err)
		require.Equal(t, fmt.Sprintf(`label name "%s" is not unique: invalid sample`, labelName), err.Error())
	}

	add(labels.Labels{{Name: "a", Value: "c"}, {Name: "a", Value: "b"}}, "a")
	add(labels.Labels{{Name: "a", Value: "c"}, {Name: "a", Value: "c"}}, "a")
	add(labels.Labels{{Name: "__name__", Value: "up"}, {Name: "job", Value: "prometheus"}, {Name: "le", Value: "500"}, {Name: "le", Value: "400"}, {Name: "unit", Value: "s"}}, "le")
}

func TestMemSeriesIsolation(t *testing.T) {
	// Put a series, select it. GC it and then access it.
	lastValue := func(h *Head, maxAppendID uint64) int {
		idx, err := h.Index()

		require.NoError(t, err)

		iso := h.iso.State(math.MinInt64, math.MaxInt64)
		iso.maxAppendID = maxAppendID

		chunks, err := h.chunksRange(math.MinInt64, math.MaxInt64, iso)
		require.NoError(t, err)
		// Hm.. here direct block chunk querier might be required?
		querier := blockQuerier{
			blockBaseQuerier: &blockBaseQuerier{
				index:      idx,
				chunks:     chunks,
				tombstones: tombstones.NewMemTombstones(),

				mint: 0,
				maxt: 10000,
			},
		}

		require.NoError(t, err)
		defer querier.Close()

		ss := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
		_, seriesSet, ws, err := expandSeriesSet(ss)
		require.NoError(t, err)
		require.Equal(t, 0, len(ws))

		for _, series := range seriesSet {
			return int(series[len(series)-1].v)
		}
		return -1
	}

	addSamples := func(h *Head) int {
		i := 1
		for ; i <= 1000; i++ {
			var app storage.Appender
			// To initialize bounds.
			if h.MinTime() == math.MaxInt64 {
				app = &initAppender{head: h}
			} else {
				a := h.appender()
				a.cleanupAppendIDsBelow = 0
				app = a
			}

			_, err := app.Append(0, labels.FromStrings("foo", "bar"), int64(i), float64(i))
			require.NoError(t, err)
			require.NoError(t, app.Commit())
		}
		return i
	}

	testIsolation := func(h *Head, i int) {
	}

	// Test isolation without restart of Head.
	hb, _ := newTestHead(t, 1000, false)
	i := addSamples(hb)
	testIsolation(hb, i)

	// Test simple cases in different chunks when no appendID cleanup has been performed.
	require.Equal(t, 10, lastValue(hb, 10))
	require.Equal(t, 130, lastValue(hb, 130))
	require.Equal(t, 160, lastValue(hb, 160))
	require.Equal(t, 240, lastValue(hb, 240))
	require.Equal(t, 500, lastValue(hb, 500))
	require.Equal(t, 750, lastValue(hb, 750))
	require.Equal(t, 995, lastValue(hb, 995))
	require.Equal(t, 999, lastValue(hb, 999))

	// Cleanup appendIDs below 500.
	app := hb.appender()
	app.cleanupAppendIDsBelow = 500
	_, err := app.Append(0, labels.FromStrings("foo", "bar"), int64(i), float64(i))
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	i++

	// We should not get queries with a maxAppendID below 500 after the cleanup,
	// but they only take the remaining appendIDs into account.
	require.Equal(t, 499, lastValue(hb, 10))
	require.Equal(t, 499, lastValue(hb, 130))
	require.Equal(t, 499, lastValue(hb, 160))
	require.Equal(t, 499, lastValue(hb, 240))
	require.Equal(t, 500, lastValue(hb, 500))
	require.Equal(t, 995, lastValue(hb, 995))
	require.Equal(t, 999, lastValue(hb, 999))

	// Cleanup appendIDs below 1000, which means the sample buffer is
	// the only thing with appendIDs.
	app = hb.appender()
	app.cleanupAppendIDsBelow = 1000
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), int64(i), float64(i))
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 999, lastValue(hb, 998))
	require.Equal(t, 999, lastValue(hb, 999))
	require.Equal(t, 1000, lastValue(hb, 1000))
	require.Equal(t, 1001, lastValue(hb, 1001))
	require.Equal(t, 1002, lastValue(hb, 1002))
	require.Equal(t, 1002, lastValue(hb, 1003))

	i++
	// Cleanup appendIDs below 1001, but with a rollback.
	app = hb.appender()
	app.cleanupAppendIDsBelow = 1001
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), int64(i), float64(i))
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	require.Equal(t, 1000, lastValue(hb, 999))
	require.Equal(t, 1000, lastValue(hb, 1000))
	require.Equal(t, 1001, lastValue(hb, 1001))
	require.Equal(t, 1002, lastValue(hb, 1002))
	require.Equal(t, 1002, lastValue(hb, 1003))

	require.NoError(t, hb.Close())

	// Test isolation with restart of Head. This is to verify the num samples of chunks after m-map chunk replay.
	hb, w := newTestHead(t, 1000, false)
	i = addSamples(hb)
	require.NoError(t, hb.Close())

	wlog, err := wal.NewSize(nil, nil, w.Dir(), 32768, false)
	require.NoError(t, err)
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = wlog.Dir()
	hb, err = NewHead(nil, nil, wlog, opts, nil)
	defer func() { require.NoError(t, hb.Close()) }()
	require.NoError(t, err)
	require.NoError(t, hb.Init(0))

	// No appends after restarting. Hence all should return the last value.
	require.Equal(t, 1000, lastValue(hb, 10))
	require.Equal(t, 1000, lastValue(hb, 130))
	require.Equal(t, 1000, lastValue(hb, 160))
	require.Equal(t, 1000, lastValue(hb, 240))
	require.Equal(t, 1000, lastValue(hb, 500))

	// Cleanup appendIDs below 1000, which means the sample buffer is
	// the only thing with appendIDs.
	app = hb.appender()
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), int64(i), float64(i))
	i++
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 1001, lastValue(hb, 998))
	require.Equal(t, 1001, lastValue(hb, 999))
	require.Equal(t, 1001, lastValue(hb, 1000))
	require.Equal(t, 1001, lastValue(hb, 1001))
	require.Equal(t, 1001, lastValue(hb, 1002))
	require.Equal(t, 1001, lastValue(hb, 1003))

	// Cleanup appendIDs below 1002, but with a rollback.
	app = hb.appender()
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), int64(i), float64(i))
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	require.Equal(t, 1001, lastValue(hb, 999))
	require.Equal(t, 1001, lastValue(hb, 1000))
	require.Equal(t, 1001, lastValue(hb, 1001))
	require.Equal(t, 1001, lastValue(hb, 1002))
	require.Equal(t, 1001, lastValue(hb, 1003))
}

func TestIsolationRollback(t *testing.T) {
	// Rollback after a failed append and test if the low watermark has progressed anyway.
	hb, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	app := hb.Appender(context.Background())
	_, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(1), hb.iso.lowWatermark())

	app = hb.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 1, 1)
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("foo", "bar", "foo", "baz"), 2, 2)
	require.Error(t, err)
	require.NoError(t, app.Rollback())
	require.Equal(t, uint64(2), hb.iso.lowWatermark())

	app = hb.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 3, 3)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(3), hb.iso.lowWatermark(), "Low watermark should proceed to 3 even if append #2 was rolled back.")
}

func TestIsolationLowWatermarkMonotonous(t *testing.T) {
	hb, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	app1 := hb.Appender(context.Background())
	_, err := app1.Append(0, labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)
	require.NoError(t, app1.Commit())
	require.Equal(t, uint64(1), hb.iso.lowWatermark(), "Low watermark should by 1 after 1st append.")

	app1 = hb.Appender(context.Background())
	_, err = app1.Append(0, labels.FromStrings("foo", "bar"), 1, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(2), hb.iso.lowWatermark(), "Low watermark should be two, even if append is not committed yet.")

	app2 := hb.Appender(context.Background())
	_, err = app2.Append(0, labels.FromStrings("foo", "baz"), 1, 1)
	require.NoError(t, err)
	require.NoError(t, app2.Commit())
	require.Equal(t, uint64(2), hb.iso.lowWatermark(), "Low watermark should stay two because app1 is not committed yet.")

	is := hb.iso.State(math.MinInt64, math.MaxInt64)
	require.Equal(t, uint64(2), hb.iso.lowWatermark(), "After simulated read (iso state retrieved), low watermark should stay at 2.")

	require.NoError(t, app1.Commit())
	require.Equal(t, uint64(2), hb.iso.lowWatermark(), "Even after app1 is committed, low watermark should stay at 2 because read is still ongoing.")

	is.Close()
	require.Equal(t, uint64(3), hb.iso.lowWatermark(), "After read has finished (iso state closed), low watermark should jump to three.")
}

func TestIsolationAppendIDZeroIsNoop(t *testing.T) {
	h, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	s, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))

	ok, _ := s.append(0, 0, 0, h.chunkDiskMapper)
	require.True(t, ok, "Series append failed.")
	require.Equal(t, 0, s.txs.txIDCount, "Series should not have an appendID after append with appendID=0.")
}

func TestHeadSeriesChunkRace(t *testing.T) {
	for i := 0; i < 1000; i++ {
		testHeadSeriesChunkRace(t)
	}
}

func TestIsolationWithoutAdd(t *testing.T) {
	hb, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	app := hb.Appender(context.Background())
	require.NoError(t, app.Commit())

	app = hb.Appender(context.Background())
	_, err := app.Append(0, labels.FromStrings("foo", "baz"), 1, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, hb.iso.lastAppendID(), hb.iso.lowWatermark(), "High watermark should be equal to the low watermark")
}

func TestOutOfOrderSamplesMetric(t *testing.T) {
	dir, err := ioutil.TempDir("", "test")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(dir))
	}()

	db, err := Open(dir, nil, nil, DefaultOptions(), nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()
	db.DisableCompactions()

	ctx := context.Background()
	app := db.Appender(ctx)
	for i := 1; i <= 5; i++ {
		_, err = app.Append(0, labels.FromStrings("a", "b"), int64(i), 99)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Test out of order metric.
	require.Equal(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))
	app = db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 2, 99)
	require.Equal(t, storage.ErrOutOfOrderSample, err)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))

	_, err = app.Append(0, labels.FromStrings("a", "b"), 3, 99)
	require.Equal(t, storage.ErrOutOfOrderSample, err)
	require.Equal(t, 2.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))

	_, err = app.Append(0, labels.FromStrings("a", "b"), 4, 99)
	require.Equal(t, storage.ErrOutOfOrderSample, err)
	require.Equal(t, 3.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))
	require.NoError(t, app.Commit())

	// Compact Head to test out of bound metric.
	app = db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("a", "b"), DefaultBlockDuration*2, 99)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, int64(math.MinInt64), db.head.minValidTime.Load())
	require.NoError(t, db.Compact())
	require.Greater(t, db.head.minValidTime.Load(), int64(0))

	app = db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("a", "b"), db.head.minValidTime.Load()-2, 99)
	require.Equal(t, storage.ErrOutOfBounds, err)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.outOfBoundSamples))

	_, err = app.Append(0, labels.FromStrings("a", "b"), db.head.minValidTime.Load()-1, 99)
	require.Equal(t, storage.ErrOutOfBounds, err)
	require.Equal(t, 2.0, prom_testutil.ToFloat64(db.head.metrics.outOfBoundSamples))
	require.NoError(t, app.Commit())

	// Some more valid samples for out of order.
	app = db.Appender(ctx)
	for i := 1; i <= 5; i++ {
		_, err = app.Append(0, labels.FromStrings("a", "b"), db.head.minValidTime.Load()+DefaultBlockDuration+int64(i), 99)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Test out of order metric.
	app = db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("a", "b"), db.head.minValidTime.Load()+DefaultBlockDuration+2, 99)
	require.Equal(t, storage.ErrOutOfOrderSample, err)
	require.Equal(t, 4.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))

	_, err = app.Append(0, labels.FromStrings("a", "b"), db.head.minValidTime.Load()+DefaultBlockDuration+3, 99)
	require.Equal(t, storage.ErrOutOfOrderSample, err)
	require.Equal(t, 5.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))

	_, err = app.Append(0, labels.FromStrings("a", "b"), db.head.minValidTime.Load()+DefaultBlockDuration+4, 99)
	require.Equal(t, storage.ErrOutOfOrderSample, err)
	require.Equal(t, 6.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))
	require.NoError(t, app.Commit())
}

func testHeadSeriesChunkRace(t *testing.T) {
	h, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, h.Close())
	}()
	require.NoError(t, h.Init(0))
	app := h.Appender(context.Background())

	s2, err := app.Append(0, labels.FromStrings("foo2", "bar"), 5, 0)
	require.NoError(t, err)
	for ts := int64(6); ts < 11; ts++ {
		_, err = app.Append(s2, nil, ts, 0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	var wg sync.WaitGroup
	matcher := labels.MustNewMatcher(labels.MatchEqual, "", "")
	q, err := NewBlockQuerier(h, 18, 22)
	require.NoError(t, err)
	defer q.Close()

	wg.Add(1)
	go func() {
		h.updateMinMaxTime(20, 25)
		h.gc()
		wg.Done()
	}()
	ss := q.Select(false, nil, matcher)
	for ss.Next() {
	}
	require.NoError(t, ss.Err())
	wg.Wait()
}

func TestHeadLabelNamesValuesWithMinMaxRange(t *testing.T) {
	head, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	const (
		firstSeriesTimestamp  int64 = 100
		secondSeriesTimestamp int64 = 200
		lastSeriesTimestamp   int64 = 300
	)
	var (
		seriesTimestamps = []int64{firstSeriesTimestamp,
			secondSeriesTimestamp,
			lastSeriesTimestamp,
		}
		expectedLabelNames  = []string{"a", "b", "c"}
		expectedLabelValues = []string{"d", "e", "f"}
	)

	app := head.Appender(context.Background())
	for i, name := range expectedLabelNames {
		_, err := app.Append(0, labels.Labels{{Name: name, Value: expectedLabelValues[i]}}, seriesTimestamps[i], 0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
	require.Equal(t, head.MinTime(), firstSeriesTimestamp)
	require.Equal(t, head.MaxTime(), lastSeriesTimestamp)

	var testCases = []struct {
		name           string
		mint           int64
		maxt           int64
		expectedNames  []string
		expectedValues []string
	}{
		{"maxt less than head min", head.MaxTime() - 10, head.MinTime() - 10, []string{}, []string{}},
		{"mint less than head max", head.MaxTime() + 10, head.MinTime() + 10, []string{}, []string{}},
		{"mint and maxt outside head", head.MaxTime() + 10, head.MinTime() - 10, []string{}, []string{}},
		{"mint and maxt within head", head.MaxTime() - 10, head.MinTime() + 10, expectedLabelNames, expectedLabelValues},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			headIdxReader := head.indexRange(tt.mint, tt.maxt)
			actualLabelNames, err := headIdxReader.LabelNames()
			require.NoError(t, err)
			require.Equal(t, tt.expectedNames, actualLabelNames)
			if len(tt.expectedValues) > 0 {
				for i, name := range expectedLabelNames {
					actualLabelValue, err := headIdxReader.SortedLabelValues(name)
					require.NoError(t, err)
					require.Equal(t, []string{tt.expectedValues[i]}, actualLabelValue)
				}
			}
		})
	}
}

func TestHeadLabelValuesWithMatchers(t *testing.T) {
	head, _ := newTestHead(t, 1000, false)
	t.Cleanup(func() { require.NoError(t, head.Close()) })

	app := head.Appender(context.Background())
	for i := 0; i < 100; i++ {
		_, err := app.Append(0, labels.Labels{
			{Name: "unique", Value: fmt.Sprintf("value%d", i)},
			{Name: "tens", Value: fmt.Sprintf("value%d", i/10)},
		}, 100, 0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	var testCases = []struct {
		name           string
		labelName      string
		matchers       []*labels.Matcher
		expectedValues []string
	}{
		{
			name:           "get tens based on unique id",
			labelName:      "tens",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "unique", "value35")},
			expectedValues: []string{"value3"},
		}, {
			name:           "get unique ids based on a ten",
			labelName:      "unique",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "tens", "value1")},
			expectedValues: []string{"value10", "value11", "value12", "value13", "value14", "value15", "value16", "value17", "value18", "value19"},
		}, {
			name:           "get tens by pattern matching on unique id",
			labelName:      "tens",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "unique", "value[5-7]5")},
			expectedValues: []string{"value5", "value6", "value7"},
		}, {
			name:           "get tens by matching for absence of unique label",
			labelName:      "tens",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, "unique", "")},
			expectedValues: []string{"value0", "value1", "value2", "value3", "value4", "value5", "value6", "value7", "value8", "value9"},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			headIdxReader := head.indexRange(0, 200)

			actualValues, err := headIdxReader.SortedLabelValues(tt.labelName, tt.matchers...)
			require.NoError(t, err)
			require.Equal(t, tt.expectedValues, actualValues)

			actualValues, err = headIdxReader.LabelValues(tt.labelName, tt.matchers...)
			sort.Strings(actualValues)
			require.NoError(t, err)
			require.Equal(t, tt.expectedValues, actualValues)
		})
	}
}

func TestHeadLabelNamesWithMatchers(t *testing.T) {
	head, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	app := head.Appender(context.Background())
	for i := 0; i < 100; i++ {
		_, err := app.Append(0, labels.Labels{
			{Name: "unique", Value: fmt.Sprintf("value%d", i)},
		}, 100, 0)
		require.NoError(t, err)

		if i%10 == 0 {
			_, err := app.Append(0, labels.Labels{
				{Name: "unique", Value: fmt.Sprintf("value%d", i)},
				{Name: "tens", Value: fmt.Sprintf("value%d", i/10)},
			}, 100, 0)
			require.NoError(t, err)
		}

		if i%20 == 0 {
			_, err := app.Append(0, labels.Labels{
				{Name: "unique", Value: fmt.Sprintf("value%d", i)},
				{Name: "tens", Value: fmt.Sprintf("value%d", i/10)},
				{Name: "twenties", Value: fmt.Sprintf("value%d", i/20)},
			}, 100, 0)
			require.NoError(t, err)
		}
	}
	require.NoError(t, app.Commit())

	testCases := []struct {
		name          string
		labelName     string
		matchers      []*labels.Matcher
		expectedNames []string
	}{
		{
			name:          "get with non-empty unique: all",
			matchers:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, "unique", "")},
			expectedNames: []string{"tens", "twenties", "unique"},
		}, {
			name:          "get with unique ending in 1: only unique",
			matchers:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "unique", "value.*1")},
			expectedNames: []string{"unique"},
		}, {
			name:          "get with unique = value20: all",
			matchers:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "unique", "value20")},
			expectedNames: []string{"tens", "twenties", "unique"},
		}, {
			name:          "get tens = 1: unique & tens",
			matchers:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "tens", "value1")},
			expectedNames: []string{"tens", "unique"},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			headIdxReader := head.indexRange(0, 200)

			actualNames, err := headIdxReader.LabelNames(tt.matchers...)
			require.NoError(t, err)
			require.Equal(t, tt.expectedNames, actualNames)
		})
	}
}

func TestErrReuseAppender(t *testing.T) {
	head, _ := newTestHead(t, 1000, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	app := head.Appender(context.Background())
	_, err := app.Append(0, labels.Labels{{Name: "test", Value: "test"}}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Error(t, app.Commit())
	require.Error(t, app.Rollback())

	app = head.Appender(context.Background())
	_, err = app.Append(0, labels.Labels{{Name: "test", Value: "test"}}, 1, 0)
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	require.Error(t, app.Rollback())
	require.Error(t, app.Commit())

	app = head.Appender(context.Background())
	_, err = app.Append(0, labels.Labels{{Name: "test", Value: "test"}}, 2, 0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Error(t, app.Rollback())
	require.Error(t, app.Commit())

	app = head.Appender(context.Background())
	_, err = app.Append(0, labels.Labels{{Name: "test", Value: "test"}}, 3, 0)
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	require.Error(t, app.Commit())
	require.Error(t, app.Rollback())
}

func TestHeadMintAfterTruncation(t *testing.T) {
	chunkRange := int64(2000)
	head, _ := newTestHead(t, chunkRange, false)

	app := head.Appender(context.Background())
	_, err := app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 100, 100)
	require.NoError(t, err)
	_, err = app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 4000, 200)
	require.NoError(t, err)
	_, err = app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 8000, 300)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Truncating outside the appendable window and actual mint being outside
	// appendable window should leave mint at the actual mint.
	require.NoError(t, head.Truncate(3500))
	require.Equal(t, int64(4000), head.MinTime())
	require.Equal(t, int64(4000), head.minValidTime.Load())

	// After truncation outside the appendable window if the actual min time
	// is in the appendable window then we should leave mint at the start of appendable window.
	require.NoError(t, head.Truncate(5000))
	require.Equal(t, head.appendableMinValidTime(), head.MinTime())
	require.Equal(t, head.appendableMinValidTime(), head.minValidTime.Load())

	// If the truncation time is inside the appendable window, then the min time
	// should be the truncation time.
	require.NoError(t, head.Truncate(7500))
	require.Equal(t, int64(7500), head.MinTime())
	require.Equal(t, int64(7500), head.minValidTime.Load())

	require.NoError(t, head.Close())
}

func TestHeadExemplars(t *testing.T) {
	chunkRange := int64(2000)
	head, _ := newTestHead(t, chunkRange, false)
	app := head.Appender(context.Background())

	l := labels.FromStrings("traceId", "123")
	// It is perfectly valid to add Exemplars before the current start time -
	// histogram buckets that haven't been update in a while could still be
	// exported exemplars from an hour ago.
	ref, err := app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 100, 100)
	require.NoError(t, err)
	_, err = app.AppendExemplar(ref, l, exemplar.Exemplar{
		Labels: l,
		HasTs:  true,
		Ts:     -1000,
		Value:  1,
	})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.NoError(t, head.Close())
}

func BenchmarkHeadLabelValuesWithMatchers(b *testing.B) {
	chunkRange := int64(2000)
	head, _ := newTestHead(b, chunkRange, false)
	b.Cleanup(func() { require.NoError(b, head.Close()) })

	app := head.Appender(context.Background())

	metricCount := 1000000
	for i := 0; i < metricCount; i++ {
		_, err := app.Append(0, labels.Labels{
			{Name: "unique", Value: fmt.Sprintf("value%d", i)},
			{Name: "tens", Value: fmt.Sprintf("value%d", i/(metricCount/10))},
			{Name: "ninety", Value: fmt.Sprintf("value%d", i/(metricCount/10)/9)}, // "0" for the first 90%, then "1"
		}, 100, 0)
		require.NoError(b, err)
	}
	require.NoError(b, app.Commit())

	headIdxReader := head.indexRange(0, 200)
	matchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "ninety", "value0")}

	b.ResetTimer()
	b.ReportAllocs()

	for benchIdx := 0; benchIdx < b.N; benchIdx++ {
		actualValues, err := headIdxReader.LabelValues("tens", matchers...)
		require.NoError(b, err)
		require.Equal(b, 9, len(actualValues))
	}
}

func TestMemSafeIteratorSeekIntoBuffer(t *testing.T) {
	dir, err := ioutil.TempDir("", "iterator_seek")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(dir))
	}()
	// This is usually taken from the Head, but passing manually here.
	chunkDiskMapper, err := chunks.NewChunkDiskMapper(dir, chunkenc.NewPool(), chunks.DefaultWriteBufferSize)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, chunkDiskMapper.Close())
	}()

	s := newMemSeries(labels.Labels{}, 1, 500, nil)

	for i := 0; i < 7; i++ {
		ok, _ := s.append(int64(i), float64(i), 0, chunkDiskMapper)
		require.True(t, ok, "sample append failed")
	}

	it := s.iterator(s.chunkID(len(s.mmappedChunks)), nil, chunkDiskMapper, nil)
	_, ok := it.(*memSafeIterator)
	require.True(t, ok)

	// First point.
	ok = it.Seek(0)
	require.True(t, ok)
	ts, val := it.At()
	require.Equal(t, int64(0), ts)
	require.Equal(t, float64(0), val)

	// Advance one point.
	ok = it.Next()
	require.True(t, ok)
	ts, val = it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, float64(1), val)

	// Seeking an older timestamp shouldn't cause the iterator to go backwards.
	ok = it.Seek(0)
	require.True(t, ok)
	ts, val = it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, float64(1), val)

	// Seek into the buffer.
	ok = it.Seek(3)
	require.True(t, ok)
	ts, val = it.At()
	require.Equal(t, int64(3), ts)
	require.Equal(t, float64(3), val)

	// Iterate through the rest of the buffer.
	for i := 4; i < 7; i++ {
		ok = it.Next()
		require.True(t, ok)
		ts, val = it.At()
		require.Equal(t, int64(i), ts)
		require.Equal(t, float64(i), val)
	}

	// Run out of elements in the iterator.
	ok = it.Next()
	require.False(t, ok)
	ok = it.Seek(7)
	require.False(t, ok)
}

// Tests https://github.com/prometheus/prometheus/issues/8221.
func TestChunkNotFoundHeadGCRace(t *testing.T) {
	db := newTestDB(t)
	db.DisableCompactions()

	var (
		app        = db.Appender(context.Background())
		ref        = uint64(0)
		mint, maxt = int64(0), int64(0)
		err        error
	)

	// Appends samples to span over 1.5 block ranges.
	// 7 chunks with 15s scrape interval.
	for i := int64(0); i <= 120*7; i++ {
		ts := i * DefaultBlockDuration / (4 * 120)
		ref, err = app.Append(ref, labels.FromStrings("a", "b"), ts, float64(i))
		require.NoError(t, err)
		maxt = ts
	}
	require.NoError(t, app.Commit())

	// Get a querier before compaction (or when compaction is about to begin).
	q, err := db.Querier(context.Background(), mint, maxt)
	require.NoError(t, err)

	// Query the compacted range and get the first series before compaction.
	ss := q.Select(true, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.True(t, ss.Next())
	s := ss.At()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Compacting head while the querier spans the compaction time.
		require.NoError(t, db.Compact())
		require.Greater(t, len(db.Blocks()), 0)
	}()

	// Give enough time for compaction to finish.
	// We expect it to be blocked until querier is closed.
	<-time.After(3 * time.Second)

	// Now consume after compaction when it's gone.
	it := s.Iterator()
	for it.Next() {
		_, _ = it.At()
	}
	// It should error here without any fix for the mentioned issue.
	require.NoError(t, it.Err())
	for ss.Next() {
		s = ss.At()
		it := s.Iterator()
		for it.Next() {
			_, _ = it.At()
		}
		require.NoError(t, it.Err())
	}
	require.NoError(t, ss.Err())

	require.NoError(t, q.Close())
	wg.Wait()
}

// Tests https://github.com/prometheus/prometheus/issues/9079.
func TestDataMissingOnQueryDuringCompaction(t *testing.T) {
	db := newTestDB(t)
	db.DisableCompactions()

	var (
		app        = db.Appender(context.Background())
		ref        = uint64(0)
		mint, maxt = int64(0), int64(0)
		err        error
	)

	// Appends samples to span over 1.5 block ranges.
	expSamples := make([]tsdbutil.Sample, 0)
	// 7 chunks with 15s scrape interval.
	for i := int64(0); i <= 120*7; i++ {
		ts := i * DefaultBlockDuration / (4 * 120)
		ref, err = app.Append(ref, labels.FromStrings("a", "b"), ts, float64(i))
		require.NoError(t, err)
		maxt = ts
		expSamples = append(expSamples, sample{ts, float64(i)})
	}
	require.NoError(t, app.Commit())

	// Get a querier before compaction (or when compaction is about to begin).
	q, err := db.Querier(context.Background(), mint, maxt)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Compacting head while the querier spans the compaction time.
		require.NoError(t, db.Compact())
		require.Greater(t, len(db.Blocks()), 0)
	}()

	// Give enough time for compaction to finish.
	// We expect it to be blocked until querier is closed.
	<-time.After(3 * time.Second)

	// Querying the querier that was got before compaction.
	series := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.Equal(t, map[string][]tsdbutil.Sample{`{a="b"}`: expSamples}, series)

	wg.Wait()
}

func TestIsQuerierCollidingWithTruncation(t *testing.T) {
	db := newTestDB(t)
	db.DisableCompactions()

	var (
		app = db.Appender(context.Background())
		ref = uint64(0)
		err error
	)

	for i := int64(0); i <= 3000; i++ {
		ref, err = app.Append(ref, labels.FromStrings("a", "b"), i, float64(i))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// This mocks truncation.
	db.head.memTruncationInProcess.Store(true)
	db.head.lastMemoryTruncationTime.Store(2000)

	// Test that IsQuerierValid suggests correct querier ranges.
	cases := []struct {
		mint, maxt                int64 // For the querier.
		expShouldClose, expGetNew bool
		expNewMint                int64
	}{
		{-200, -100, true, false, 0},
		{-200, 300, true, false, 0},
		{100, 1900, true, false, 0},
		{1900, 2200, true, true, 2000},
		{2000, 2500, false, false, 0},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("mint=%d,maxt=%d", c.mint, c.maxt), func(t *testing.T) {
			shouldClose, getNew, newMint := db.head.IsQuerierCollidingWithTruncation(c.mint, c.maxt)
			require.Equal(t, c.expShouldClose, shouldClose)
			require.Equal(t, c.expGetNew, getNew)
			if getNew {
				require.Equal(t, c.expNewMint, newMint)
			}
		})
	}
}

func TestWaitForPendingReadersInTimeRange(t *testing.T) {
	db := newTestDB(t)
	db.DisableCompactions()

	sampleTs := func(i int64) int64 { return i * DefaultBlockDuration / (4 * 120) }

	var (
		app = db.Appender(context.Background())
		ref = uint64(0)
		err error
	)

	for i := int64(0); i <= 3000; i++ {
		ts := sampleTs(i)
		ref, err = app.Append(ref, labels.FromStrings("a", "b"), ts, float64(i))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	truncMint, truncMaxt := int64(1000), int64(2000)
	cases := []struct {
		mint, maxt int64
		shouldWait bool
	}{
		{0, 500, false},     // Before truncation range.
		{500, 1500, true},   // Overlaps with truncation at the start.
		{1200, 1700, true},  // Within truncation range.
		{1800, 2500, true},  // Overlaps with truncation at the end.
		{2000, 2500, false}, // After truncation range.
		{2100, 2500, false}, // After truncation range.
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("mint=%d,maxt=%d,shouldWait=%t", c.mint, c.maxt, c.shouldWait), func(t *testing.T) {
			checkWaiting := func(cl io.Closer) {
				var waitOver atomic.Bool
				go func() {
					db.head.WaitForPendingReadersInTimeRange(truncMint, truncMaxt)
					waitOver.Store(true)
				}()
				<-time.After(550 * time.Millisecond)
				require.Equal(t, !c.shouldWait, waitOver.Load())
				require.NoError(t, cl.Close())
				<-time.After(550 * time.Millisecond)
				require.True(t, waitOver.Load())
			}

			q, err := db.Querier(context.Background(), c.mint, c.maxt)
			require.NoError(t, err)
			checkWaiting(q)

			cq, err := db.ChunkQuerier(context.Background(), c.mint, c.maxt)
			require.NoError(t, err)
			checkWaiting(cq)
		})
	}
}

func TestChunkSnapshot(t *testing.T) {
	head, _ := newTestHead(t, 120*4, false)
	defer func() {
		head.opts.EnableMemorySnapshotOnShutdown = false
		require.NoError(t, head.Close())
	}()

	numSeries := 10
	expSeries := make(map[string][]tsdbutil.Sample)
	expTombstones := make(map[uint64]tombstones.Intervals)
	{ // Initial data that goes into snapshot.
		// Add some initial samples with >=1 m-map chunk.
		app := head.Appender(context.Background())
		for i := 1; i <= numSeries; i++ {
			lbls := labels.Labels{labels.Label{Name: "foo", Value: fmt.Sprintf("bar%d", i)}}
			lblStr := lbls.String()
			// 240 samples should m-map at least 1 chunk.
			for ts := int64(1); ts <= 240; ts++ {
				val := rand.Float64()
				expSeries[lblStr] = append(expSeries[lblStr], sample{ts, val})
				_, err := app.Append(0, lbls, ts, val)
				require.NoError(t, err)
			}
		}
		require.NoError(t, app.Commit())

		// Add some tombstones.
		var enc record.Encoder
		for i := 1; i <= numSeries; i++ {
			ref := uint64(i)
			itvs := tombstones.Intervals{
				{Mint: 1234, Maxt: 2345},
				{Mint: 3456, Maxt: 4567},
			}
			for _, itv := range itvs {
				expTombstones[ref].Add(itv)
			}
			head.tombstones.AddInterval(ref, itvs...)
			err := head.wal.Log(enc.Tombstones([]tombstones.Stone{
				{Ref: ref, Intervals: itvs},
			}, nil))
			require.NoError(t, err)
		}
	}

	// These references should be the ones used for the snapshot.
	wlast, woffset, err := head.wal.LastSegmentAndOffset()
	require.NoError(t, err)

	{ // Creating snapshot and verifying it.
		head.opts.EnableMemorySnapshotOnShutdown = true
		require.NoError(t, head.Close()) // This will create a snapshot.

		_, sidx, soffset, err := LastChunkSnapshot(head.opts.ChunkDirRoot)
		require.NoError(t, err)
		require.Equal(t, wlast, sidx)
		require.Equal(t, woffset, soffset)
	}

	{ // Test the replay of snapshot.
		// Create new Head which should replay this snapshot.
		w, err := wal.NewSize(nil, nil, head.wal.Dir(), 32768, false)
		require.NoError(t, err)
		head, err = NewHead(nil, nil, w, head.opts, nil)
		require.NoError(t, err)
		require.NoError(t, head.Init(math.MinInt64))

		// Test query for snapshot replay.
		q, err := NewBlockQuerier(head, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		series := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", ".*"))
		require.Equal(t, expSeries, series)

		// Check the tombstones.
		tr, err := head.Tombstones()
		require.NoError(t, err)
		actTombstones := make(map[uint64]tombstones.Intervals)
		require.NoError(t, tr.Iter(func(ref uint64, itvs tombstones.Intervals) error {
			for _, itv := range itvs {
				actTombstones[ref].Add(itv)
			}
			return nil
		}))
		require.Equal(t, expTombstones, actTombstones)
	}

	{ // Additional data to only include in WAL and m-mapped chunks and not snapshot. This mimics having an old snapshot on disk.

		// Add more samples.
		app := head.Appender(context.Background())
		for i := 1; i <= numSeries; i++ {
			lbls := labels.Labels{labels.Label{Name: "foo", Value: fmt.Sprintf("bar%d", i)}}
			lblStr := lbls.String()
			// 240 samples should m-map at least 1 chunk.
			for ts := int64(241); ts <= 480; ts++ {
				val := rand.Float64()
				expSeries[lblStr] = append(expSeries[lblStr], sample{ts, val})
				_, err := app.Append(0, lbls, ts, val)
				require.NoError(t, err)
			}
		}
		require.NoError(t, app.Commit())

		// Add more tombstones.
		var enc record.Encoder
		for i := 1; i <= numSeries; i++ {
			ref := uint64(i)
			itvs := tombstones.Intervals{
				{Mint: 12345, Maxt: 23456},
				{Mint: 34567, Maxt: 45678},
			}
			for _, itv := range itvs {
				expTombstones[ref].Add(itv)
			}
			head.tombstones.AddInterval(ref, itvs...)
			err := head.wal.Log(enc.Tombstones([]tombstones.Stone{
				{Ref: ref, Intervals: itvs},
			}, nil))
			require.NoError(t, err)
		}
	}

	{ // Close Head and verify that new snapshot was not created.
		head.opts.EnableMemorySnapshotOnShutdown = false
		require.NoError(t, head.Close()) // This should not create a snapshot.

		_, sidx, soffset, err := LastChunkSnapshot(head.opts.ChunkDirRoot)
		require.NoError(t, err)
		require.Equal(t, wlast, sidx)
		require.Equal(t, woffset, soffset)
	}

	{ // Test the replay of snapshot, m-map chunks, and WAL.
		// Create new Head to replay snapshot, m-map chunks, and WAL.
		w, err := wal.NewSize(nil, nil, head.wal.Dir(), 32768, false)
		require.NoError(t, err)
		head, err = NewHead(nil, nil, w, head.opts, nil)
		require.NoError(t, err)
		require.NoError(t, head.Init(math.MinInt64))

		// Test query when data is replayed from snapshot, m-map chunks, and WAL.
		q, err := NewBlockQuerier(head, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		series := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", ".*"))
		require.Equal(t, expSeries, series)

		// Check the tombstones.
		tr, err := head.Tombstones()
		require.NoError(t, err)
		actTombstones := make(map[uint64]tombstones.Intervals)
		require.NoError(t, tr.Iter(func(ref uint64, itvs tombstones.Intervals) error {
			for _, itv := range itvs {
				actTombstones[ref].Add(itv)
			}
			return nil
		}))
		require.Equal(t, expTombstones, actTombstones)
	}
}
