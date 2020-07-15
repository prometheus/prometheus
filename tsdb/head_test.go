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
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"

	"github.com/pkg/errors"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wal"
	"github.com/prometheus/prometheus/util/testutil"
)

func BenchmarkCreateSeries(b *testing.B) {
	series := genSeries(b.N, 10, 0, 0)
	h, _, closer := newTestHead(b, 10000, false)
	defer closer()
	defer func() {
		testutil.Ok(b, h.Close())
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
			testutil.Ok(t, w.Log(enc.Series(v, nil)))
		case []record.RefSample:
			testutil.Ok(t, w.Log(enc.Samples(v, nil)))
		case []tombstones.Stone:
			testutil.Ok(t, w.Log(enc.Tombstones(v, nil)))
		}
	}
}

func readTestWAL(t testing.TB, dir string) (recs []interface{}) {
	sr, err := wal.NewSegmentsReader(dir)
	testutil.Ok(t, err)
	defer sr.Close()

	var dec record.Decoder
	r := wal.NewReader(sr)

	for r.Next() {
		rec := r.Record()

		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, nil)
			testutil.Ok(t, err)
			recs = append(recs, series)
		case record.Samples:
			samples, err := dec.Samples(rec, nil)
			testutil.Ok(t, err)
			recs = append(recs, samples)
		case record.Tombstones:
			tstones, err := dec.Tombstones(rec, nil)
			testutil.Ok(t, err)
			recs = append(recs, tstones)
		default:
			t.Fatalf("unknown record type")
		}
	}
	testutil.Ok(t, r.Err())
	return recs
}

func BenchmarkLoadWAL(b *testing.B) {
	cases := []struct {
		// Total series is (batches*seriesPerBatch).
		batches          int
		seriesPerBatch   int
		samplesPerSeries int
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
	}

	labelsPerSeries := 5
	for _, c := range cases {
		b.Run(fmt.Sprintf("batches=%d,seriesPerBatch=%d,samplesPerSeries=%d", c.batches, c.seriesPerBatch, c.samplesPerSeries),
			func(b *testing.B) {
				dir, err := ioutil.TempDir("", "test_load_wal")
				testutil.Ok(b, err)
				defer func() {
					testutil.Ok(b, os.RemoveAll(dir))
				}()

				w, err := wal.New(nil, nil, dir, false)
				testutil.Ok(b, err)

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
						refSeries = append(refSeries, record.RefSeries{Ref: uint64(i) * 100, Labels: labels.FromMap(lbls)})
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
								Ref: uint64(k) * 100,
								T:   int64(i) * 10,
								V:   float64(i) * 100,
							})
						}
						populateTestWAL(b, w, []interface{}{refSamples})
					}
				}

				b.ResetTimer()

				// Load the WAL.
				for i := 0; i < b.N; i++ {
					h, err := NewHead(nil, nil, w, 1000, w.Dir(), nil, DefaultStripeSize, nil)
					testutil.Ok(b, err)
					h.Init(0)
				}
			})
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
			}

			head, w, closer := newTestHead(t, 1000, compress)
			defer closer()
			defer func() {
				testutil.Ok(t, head.Close())
			}()

			populateTestWAL(t, w, entries)

			testutil.Ok(t, head.Init(math.MinInt64))
			testutil.Equals(t, uint64(101), head.lastSeriesID)

			s10 := head.series.getByID(10)
			s11 := head.series.getByID(11)
			s50 := head.series.getByID(50)
			s100 := head.series.getByID(100)

			testutil.Equals(t, labels.FromStrings("a", "1"), s10.lset)
			testutil.Equals(t, (*memSeries)(nil), s11) // Series without samples should be garbage collected at head.Init().
			testutil.Equals(t, labels.FromStrings("a", "4"), s50.lset)
			testutil.Equals(t, labels.FromStrings("a", "3"), s100.lset)

			expandChunk := func(c chunkenc.Iterator) (x []sample) {
				for c.Next() {
					t, v := c.At()
					x = append(x, sample{t: t, v: v})
				}
				testutil.Ok(t, c.Err())
				return x
			}
			testutil.Equals(t, []sample{{100, 2}, {101, 5}}, expandChunk(s10.iterator(0, nil, head.chunkDiskMapper, nil)))
			testutil.Equals(t, []sample{{101, 6}}, expandChunk(s50.iterator(0, nil, head.chunkDiskMapper, nil)))
			testutil.Equals(t, []sample{{100, 3}, {101, 7}}, expandChunk(s100.iterator(0, nil, head.chunkDiskMapper, nil)))
		})
	}
}

func TestHead_WALMultiRef(t *testing.T) {
	head, w, closer := newTestHead(t, 1000, false)
	defer closer()

	testutil.Ok(t, head.Init(0))

	app := head.Appender()
	ref1, err := app.Add(labels.FromStrings("foo", "bar"), 100, 1)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	testutil.Equals(t, 1.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	// Add another sample outside chunk range to mmap a chunk.
	app = head.Appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), 1500, 2)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	testutil.Equals(t, 2.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	testutil.Ok(t, head.Truncate(1600))

	app = head.Appender()
	ref2, err := app.Add(labels.FromStrings("foo", "bar"), 1700, 3)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	testutil.Equals(t, 3.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	// Add another sample outside chunk range to mmap a chunk.
	app = head.Appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), 2000, 4)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	testutil.Equals(t, 4.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	testutil.Assert(t, ref1 != ref2, "Refs are the same")
	testutil.Ok(t, head.Close())

	w, err = wal.New(nil, nil, w.Dir(), false)
	testutil.Ok(t, err)

	head, err = NewHead(nil, nil, w, 1000, w.Dir(), nil, DefaultStripeSize, nil)
	testutil.Ok(t, err)
	testutil.Ok(t, head.Init(0))
	defer func() {
		testutil.Ok(t, head.Close())
	}()

	q, err := NewBlockQuerier(head, 0, 2100)
	testutil.Ok(t, err)
	series := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	testutil.Equals(t, map[string][]tsdbutil.Sample{`{foo="bar"}`: {
		sample{100, 1},
		sample{1500, 2},
		sample{1700, 3},
		sample{2000, 4},
	}}, series)
}

func TestHead_Truncate(t *testing.T) {
	h, _, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, h.Close())
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
	testutil.Ok(t, h.Truncate(1))

	testutil.Ok(t, h.Truncate(2000))

	testutil.Equals(t, []*mmappedChunk{
		{minTime: 2000, maxTime: 2999},
	}, h.series.getByID(s1.ref).mmappedChunks)

	testutil.Equals(t, []*mmappedChunk{
		{minTime: 2000, maxTime: 2999},
		{minTime: 3000, maxTime: 3999},
	}, h.series.getByID(s2.ref).mmappedChunks)

	testutil.Assert(t, h.series.getByID(s3.ref) == nil, "")
	testutil.Assert(t, h.series.getByID(s4.ref) == nil, "")

	postingsA1, _ := index.ExpandPostings(h.postings.Get("a", "1"))
	postingsA2, _ := index.ExpandPostings(h.postings.Get("a", "2"))
	postingsB1, _ := index.ExpandPostings(h.postings.Get("b", "1"))
	postingsB2, _ := index.ExpandPostings(h.postings.Get("b", "2"))
	postingsC1, _ := index.ExpandPostings(h.postings.Get("c", "1"))
	postingsAll, _ := index.ExpandPostings(h.postings.Get("", ""))

	testutil.Equals(t, []uint64{s1.ref}, postingsA1)
	testutil.Equals(t, []uint64{s2.ref}, postingsA2)
	testutil.Equals(t, []uint64{s1.ref, s2.ref}, postingsB1)
	testutil.Equals(t, []uint64{s1.ref, s2.ref}, postingsAll)
	testutil.Assert(t, postingsB2 == nil, "")
	testutil.Assert(t, postingsC1 == nil, "")

	testutil.Equals(t, map[string]struct{}{
		"":  {}, // from 'all' postings list
		"a": {},
		"b": {},
		"1": {},
		"2": {},
	}, h.symbols)

	testutil.Equals(t, map[string]stringset{
		"a": {"1": struct{}{}, "2": struct{}{}},
		"b": {"1": struct{}{}},
		"":  {"": struct{}{}},
	}, h.values)
}

// Validate various behaviors brought on by firstChunkID accounting for
// garbage collected chunks.
func TestMemSeries_truncateChunks(t *testing.T) {
	dir, err := ioutil.TempDir("", "truncate_chunks")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()
	// This is usually taken from the Head, but passing manually here.
	chunkDiskMapper, err := chunks.NewChunkDiskMapper(dir, chunkenc.NewPool())
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, chunkDiskMapper.Close())
	}()

	memChunkPool := sync.Pool{
		New: func() interface{} {
			return &memChunk{}
		},
	}

	s := newMemSeries(labels.FromStrings("a", "b"), 1, 2000, &memChunkPool)

	for i := 0; i < 4000; i += 5 {
		ok, _ := s.append(int64(i), float64(i), 0, chunkDiskMapper)
		testutil.Assert(t, ok == true, "sample append failed")
	}

	// Check that truncate removes half of the chunks and afterwards
	// that the ID of the last chunk still gives us the same chunk afterwards.
	countBefore := len(s.mmappedChunks) + 1 // +1 for the head chunk.
	lastID := s.chunkID(countBefore - 1)
	lastChunk, _, err := s.chunk(lastID, chunkDiskMapper)
	testutil.Ok(t, err)
	testutil.Assert(t, lastChunk != nil, "")

	chk, _, err := s.chunk(0, chunkDiskMapper)
	testutil.Assert(t, chk != nil, "")
	testutil.Ok(t, err)

	s.truncateChunksBefore(2000)

	testutil.Equals(t, int64(2000), s.mmappedChunks[0].minTime)
	_, _, err = s.chunk(0, chunkDiskMapper)
	testutil.Assert(t, err == storage.ErrNotFound, "first chunks not gone")
	testutil.Equals(t, countBefore/2, len(s.mmappedChunks)+1) // +1 for the head chunk.
	chk, _, err = s.chunk(lastID, chunkDiskMapper)
	testutil.Ok(t, err)
	testutil.Equals(t, lastChunk, chk)

	// Validate that the series' sample buffer is applied correctly to the last chunk
	// after truncation.
	it1 := s.iterator(s.chunkID(len(s.mmappedChunks)), nil, chunkDiskMapper, nil)
	_, ok := it1.(*memSafeIterator)
	testutil.Assert(t, ok == true, "")

	it2 := s.iterator(s.chunkID(len(s.mmappedChunks)-1), nil, chunkDiskMapper, nil)
	_, ok = it2.(*memSafeIterator)
	testutil.Assert(t, ok == false, "non-last chunk incorrectly wrapped with sample buffer")
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
			head, w, closer := newTestHead(t, 1000, compress)
			defer closer()
			defer func() {
				testutil.Ok(t, head.Close())
			}()

			populateTestWAL(t, w, entries)

			testutil.Ok(t, head.Init(math.MinInt64))

			testutil.Ok(t, head.Delete(0, 100, labels.MustNewMatcher(labels.MatchEqual, "a", "1")))
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
				head, w, closer := newTestHead(t, 1000, compress)
				defer closer()

				app := head.Appender()
				for _, smpl := range smplsAll {
					_, err := app.Add(labels.Labels{lblDefault}, smpl.t, smpl.v)
					testutil.Ok(t, err)

				}
				testutil.Ok(t, app.Commit())

				// Delete the ranges.
				for _, r := range c.dranges {
					testutil.Ok(t, head.Delete(r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, lblDefault.Name, lblDefault.Value)))
				}

				// Add more samples.
				app = head.Appender()
				for _, smpl := range c.addSamples {
					_, err := app.Add(labels.Labels{lblDefault}, smpl.t, smpl.v)
					testutil.Ok(t, err)

				}
				testutil.Ok(t, app.Commit())

				// Compare the samples for both heads - before and after the reload.
				reloadedW, err := wal.New(nil, nil, w.Dir(), compress) // Use a new wal to ensure deleted samples are gone even after a reload.
				testutil.Ok(t, err)
				reloadedHead, err := NewHead(nil, nil, reloadedW, 1000, reloadedW.Dir(), nil, DefaultStripeSize, nil)
				testutil.Ok(t, err)
				testutil.Ok(t, reloadedHead.Init(0))

				// Compare the query results for both heads - before and after the reload.
			Outer:
				for _, h := range []*Head{head, reloadedHead} {
					q, err := NewBlockQuerier(h, h.MinTime(), h.MaxTime())
					testutil.Ok(t, err)
					actSeriesSet := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, lblDefault.Name, lblDefault.Value))
					testutil.Ok(t, q.Close())
					expSeriesSet := newMockSeriesSet([]storage.Series{
						newSeries(map[string]string{lblDefault.Name: lblDefault.Value}, func() []tsdbutil.Sample {
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
						testutil.Equals(t, eok, rok)

						if !eok {
							testutil.Ok(t, h.Close())
							testutil.Ok(t, actSeriesSet.Err())
							testutil.Equals(t, 0, len(actSeriesSet.Warnings()))
							continue Outer
						}
						expSeries := expSeriesSet.At()
						actSeries := actSeriesSet.At()

						testutil.Equals(t, expSeries.Labels(), actSeries.Labels())

						smplExp, errExp := expandSeriesIterator(expSeries.Iterator())
						smplRes, errRes := expandSeriesIterator(actSeries.Iterator())

						testutil.Equals(t, errExp, errRes)
						testutil.Equals(t, smplExp, smplRes)
					}
				}
			}
		})
	}
}

func TestDeleteUntilCurMax(t *testing.T) {
	hb, _, closer := newTestHead(t, 1000000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, hb.Close())
	}()

	numSamples := int64(10)
	app := hb.Appender()
	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		_, err := app.Add(labels.Labels{{Name: "a", Value: "b"}}, i, smpls[i])
		testutil.Ok(t, err)
	}
	testutil.Ok(t, app.Commit())
	testutil.Ok(t, hb.Delete(0, 10000, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))

	// Test the series returns no samples. The series is cleared only after compaction.
	q, err := NewBlockQuerier(hb, 0, 100000)
	testutil.Ok(t, err)
	res := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	testutil.Assert(t, res.Next(), "series is not present")
	s := res.At()
	it := s.Iterator()
	testutil.Assert(t, !it.Next(), "expected no samples")
	for res.Next() {
	}
	testutil.Ok(t, res.Err())
	testutil.Equals(t, 0, len(res.Warnings()))

	// Add again and test for presence.
	app = hb.Appender()
	_, err = app.Add(labels.Labels{{Name: "a", Value: "b"}}, 11, 1)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	q, err = NewBlockQuerier(hb, 0, 100000)
	testutil.Ok(t, err)
	res = q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	testutil.Assert(t, res.Next(), "series don't exist")
	exps := res.At()
	it = exps.Iterator()
	resSamples, err := expandSeriesIterator(it)
	testutil.Ok(t, err)
	testutil.Equals(t, []tsdbutil.Sample{sample{11, 1}}, resSamples)
	for res.Next() {
	}
	testutil.Ok(t, res.Err())
	testutil.Equals(t, 0, len(res.Warnings()))
}

func TestDeletedSamplesAndSeriesStillInWALAfterCheckpoint(t *testing.T) {
	numSamples := 10000

	// Enough samples to cause a checkpoint.
	hb, w, closer := newTestHead(t, int64(numSamples)*10, false)
	defer closer()

	for i := 0; i < numSamples; i++ {
		app := hb.Appender()
		_, err := app.Add(labels.Labels{{Name: "a", Value: "b"}}, int64(i), 0)
		testutil.Ok(t, err)
		testutil.Ok(t, app.Commit())
	}
	testutil.Ok(t, hb.Delete(0, int64(numSamples), labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
	testutil.Ok(t, hb.Truncate(1))
	testutil.Ok(t, hb.Close())

	// Confirm there's been a checkpoint.
	cdir, _, err := wal.LastCheckpoint(w.Dir())
	testutil.Ok(t, err)
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
	testutil.Equals(t, 1, series)
	testutil.Equals(t, 9999, samples)
	testutil.Equals(t, 1, stones)

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

	hb, _, closer := newTestHead(t, 100000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, hb.Close())
	}()

	app := hb.Appender()
	for _, l := range lbls {
		ls := labels.New(l...)
		series := []tsdbutil.Sample{}
		ts := rand.Int63n(300)
		for i := 0; i < numDatapoints; i++ {
			v := rand.Float64()
			_, err := app.Add(ls, ts, v)
			testutil.Ok(t, err)
			series = append(series, sample{ts, v})
			ts += rand.Int63n(timeInterval) + 1
		}
		seriesMap[labels.New(l...).String()] = series
	}
	testutil.Ok(t, app.Commit())
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
			testutil.Ok(t, hb.Delete(r.Mint, r.Maxt, del.ms...))
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
			testutil.Ok(t, err)
			defer q.Close()
			ss := q.Select(true, nil, del.ms...)
			// Build the mockSeriesSet.
			matchedSeries := make([]storage.Series, 0, len(matched))
			for _, m := range matched {
				smpls := seriesMap[m.String()]
				smpls = deletedSamples(smpls, del.drange)
				// Only append those series for which samples exist as mockSeriesSet
				// doesn't skip series with no samples.
				// TODO: But sometimes SeriesSet returns an empty SeriesIterator
				if len(smpls) > 0 {
					matchedSeries = append(matchedSeries, newSeries(
						m.Map(),
						smpls,
					))
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
				testutil.Equals(t, eok, rok)
				if !eok {
					break
				}
				sexp := expSs.At()
				sres := ss.At()
				testutil.Equals(t, sexp.Labels(), sres.Labels())
				smplExp, errExp := expandSeriesIterator(sexp.Iterator())
				smplRes, errRes := expandSeriesIterator(sres.Iterator())
				testutil.Equals(t, errExp, errRes)
				testutil.Equals(t, smplExp, smplRes)
			}
			testutil.Ok(t, ss.Err())
			testutil.Equals(t, 0, len(ss.Warnings()))
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
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()
	// This is usually taken from the Head, but passing manually here.
	chunkDiskMapper, err := chunks.NewChunkDiskMapper(dir, chunkenc.NewPool())
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, chunkDiskMapper.Close())
	}()

	s := newMemSeries(labels.Labels{}, 1, 500, nil)

	// Add first two samples at the very end of a chunk range and the next two
	// on and after it.
	// New chunk must correctly be cut at 1000.
	ok, chunkCreated := s.append(998, 1, 0, chunkDiskMapper)
	testutil.Assert(t, ok, "append failed")
	testutil.Assert(t, chunkCreated, "first sample created chunk")

	ok, chunkCreated = s.append(999, 2, 0, chunkDiskMapper)
	testutil.Assert(t, ok, "append failed")
	testutil.Assert(t, !chunkCreated, "second sample should use same chunk")

	ok, chunkCreated = s.append(1000, 3, 0, chunkDiskMapper)
	testutil.Assert(t, ok, "append failed")
	testutil.Assert(t, chunkCreated, "expected new chunk on boundary")

	ok, chunkCreated = s.append(1001, 4, 0, chunkDiskMapper)
	testutil.Assert(t, ok, "append failed")
	testutil.Assert(t, !chunkCreated, "second sample should use same chunk")

	testutil.Assert(t, len(s.mmappedChunks) == 1, "there should be only 1 mmapped chunk")
	testutil.Assert(t, s.mmappedChunks[0].minTime == 998 && s.mmappedChunks[0].maxTime == 999, "wrong chunk range")
	testutil.Assert(t, s.headChunk.minTime == 1000 && s.headChunk.maxTime == 1001, "wrong chunk range")

	// Fill the range [1000,2000) with many samples. Intermediate chunks should be cut
	// at approximately 120 samples per chunk.
	for i := 1; i < 1000; i++ {
		ok, _ := s.append(1001+int64(i), float64(i), 0, chunkDiskMapper)
		testutil.Assert(t, ok, "append failed")
	}

	testutil.Assert(t, len(s.mmappedChunks)+1 > 7, "expected intermediate chunks")

	// All chunks but the first and last should now be moderately full.
	for i, c := range s.mmappedChunks[1:] {
		chk, err := chunkDiskMapper.Chunk(c.ref)
		testutil.Ok(t, err)
		testutil.Assert(t, chk.NumSamples() > 100, "unexpected small chunk %d of length %d", i, chk.NumSamples())
	}
}

func TestGCChunkAccess(t *testing.T) {
	// Put a chunk, select it. GC it and then access it.
	h, _, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, h.Close())
	}()

	h.initTime(0)

	s, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))

	// Appending 2 samples for the first chunk.
	ok, chunkCreated := s.append(0, 0, 0, h.chunkDiskMapper)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(999, 999, 0, h.chunkDiskMapper)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, !chunkCreated, "chunks was created")

	// A new chunks should be created here as it's beyond the chunk range.
	ok, chunkCreated = s.append(1000, 1000, 0, h.chunkDiskMapper)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(1999, 1999, 0, h.chunkDiskMapper)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, !chunkCreated, "chunks was created")

	idx := h.indexRange(0, 1500)
	var (
		lset   labels.Labels
		chunks []chunks.Meta
	)
	testutil.Ok(t, idx.Series(1, &lset, &chunks))

	testutil.Equals(t, labels.Labels{{
		Name: "a", Value: "1",
	}}, lset)
	testutil.Equals(t, 2, len(chunks))

	cr, err := h.chunksRange(0, 1500, nil)
	testutil.Ok(t, err)
	_, err = cr.Chunk(chunks[0].Ref)
	testutil.Ok(t, err)
	_, err = cr.Chunk(chunks[1].Ref)
	testutil.Ok(t, err)

	testutil.Ok(t, h.Truncate(1500)) // Remove a chunk.

	_, err = cr.Chunk(chunks[0].Ref)
	testutil.Equals(t, storage.ErrNotFound, err)
	_, err = cr.Chunk(chunks[1].Ref)
	testutil.Ok(t, err)
}

func TestGCSeriesAccess(t *testing.T) {
	// Put a series, select it. GC it and then access it.
	h, _, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, h.Close())
	}()

	h.initTime(0)

	s, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))

	// Appending 2 samples for the first chunk.
	ok, chunkCreated := s.append(0, 0, 0, h.chunkDiskMapper)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(999, 999, 0, h.chunkDiskMapper)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, !chunkCreated, "chunks was created")

	// A new chunks should be created here as it's beyond the chunk range.
	ok, chunkCreated = s.append(1000, 1000, 0, h.chunkDiskMapper)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(1999, 1999, 0, h.chunkDiskMapper)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, !chunkCreated, "chunks was created")

	idx := h.indexRange(0, 2000)
	var (
		lset   labels.Labels
		chunks []chunks.Meta
	)
	testutil.Ok(t, idx.Series(1, &lset, &chunks))

	testutil.Equals(t, labels.Labels{{
		Name: "a", Value: "1",
	}}, lset)
	testutil.Equals(t, 2, len(chunks))

	cr, err := h.chunksRange(0, 2000, nil)
	testutil.Ok(t, err)
	_, err = cr.Chunk(chunks[0].Ref)
	testutil.Ok(t, err)
	_, err = cr.Chunk(chunks[1].Ref)
	testutil.Ok(t, err)

	testutil.Ok(t, h.Truncate(2000)) // Remove the series.

	testutil.Equals(t, (*memSeries)(nil), h.series.getByID(1))

	_, err = cr.Chunk(chunks[0].Ref)
	testutil.Equals(t, storage.ErrNotFound, err)
	_, err = cr.Chunk(chunks[1].Ref)
	testutil.Equals(t, storage.ErrNotFound, err)
}

func TestUncommittedSamplesNotLostOnTruncate(t *testing.T) {
	h, _, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, h.Close())
	}()

	h.initTime(0)

	app := h.appender()
	lset := labels.FromStrings("a", "1")
	_, err := app.Add(lset, 2100, 1)
	testutil.Ok(t, err)

	testutil.Ok(t, h.Truncate(2000))
	testutil.Assert(t, nil != h.series.getByHash(lset.Hash(), lset), "series should not have been garbage collected")

	testutil.Ok(t, app.Commit())

	q, err := NewBlockQuerier(h, 1500, 2500)
	testutil.Ok(t, err)
	defer q.Close()

	ss := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "1"))
	testutil.Equals(t, true, ss.Next())
	for ss.Next() {
	}
	testutil.Ok(t, ss.Err())
	testutil.Equals(t, 0, len(ss.Warnings()))
}

func TestRemoveSeriesAfterRollbackAndTruncate(t *testing.T) {
	h, _, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, h.Close())
	}()

	h.initTime(0)

	app := h.appender()
	lset := labels.FromStrings("a", "1")
	_, err := app.Add(lset, 2100, 1)
	testutil.Ok(t, err)

	testutil.Ok(t, h.Truncate(2000))
	testutil.Assert(t, nil != h.series.getByHash(lset.Hash(), lset), "series should not have been garbage collected")

	testutil.Ok(t, app.Rollback())

	q, err := NewBlockQuerier(h, 1500, 2500)
	testutil.Ok(t, err)
	defer q.Close()

	ss := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "1"))
	testutil.Equals(t, false, ss.Next())
	testutil.Equals(t, 0, len(ss.Warnings()))

	// Truncate again, this time the series should be deleted
	testutil.Ok(t, h.Truncate(2050))
	testutil.Equals(t, (*memSeries)(nil), h.series.getByHash(lset.Hash(), lset))
}

func TestHead_LogRollback(t *testing.T) {
	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			h, w, closer := newTestHead(t, 1000, compress)
			defer closer()
			defer func() {
				testutil.Ok(t, h.Close())
			}()

			app := h.Appender()
			_, err := app.Add(labels.FromStrings("a", "b"), 1, 2)
			testutil.Ok(t, err)

			testutil.Ok(t, app.Rollback())
			recs := readTestWAL(t, w.Dir())

			testutil.Equals(t, 1, len(recs))

			series, ok := recs[0].([]record.RefSeries)
			testutil.Assert(t, ok, "expected series record but got %+v", recs[0])
			testutil.Equals(t, []record.RefSeries{{Ref: 1, Labels: labels.FromStrings("a", "b")}}, series)
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
		"invalid_record": {
			func(rec []byte) []byte {
				// Do not modify the base record because it is Logged multiple times.
				res := make([]byte, len(rec))
				copy(res, rec)
				res[0] = byte(record.Invalid)
				return res
			},
			enc.Series([]record.RefSeries{{Ref: 1, Labels: labels.FromStrings("a", "b")}}, []byte{}),
			9,
			5,
		},
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
				testutil.Ok(t, err)
				defer func() {
					testutil.Ok(t, os.RemoveAll(dir))
				}()

				// Fill the wal and corrupt it.
				{
					w, err := wal.New(nil, nil, filepath.Join(dir, "wal"), compress)
					testutil.Ok(t, err)

					for i := 1; i <= test.totalRecs; i++ {
						// At this point insert a corrupted record.
						if i-1 == test.expRecs {
							testutil.Ok(t, w.Log(test.corrFunc(test.rec)))
							continue
						}
						testutil.Ok(t, w.Log(test.rec))
					}

					h, err := NewHead(nil, nil, w, 1, w.Dir(), nil, DefaultStripeSize, nil)
					testutil.Ok(t, err)
					testutil.Equals(t, 0.0, prom_testutil.ToFloat64(h.metrics.walCorruptionsTotal))
					initErr := h.Init(math.MinInt64)

					err = errors.Cause(initErr) // So that we can pick up errors even if wrapped.
					_, corrErr := err.(*wal.CorruptionErr)
					testutil.Assert(t, corrErr, "reading the wal didn't return corruption error")
					testutil.Ok(t, w.Close())
				}

				// Open the db to trigger a repair.
				{
					db, err := Open(dir, nil, nil, DefaultOptions())
					testutil.Ok(t, err)
					defer func() {
						testutil.Ok(t, db.Close())
					}()
					testutil.Equals(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal))
				}

				// Read the wal content after the repair.
				{
					sr, err := wal.NewSegmentsReader(filepath.Join(dir, "wal"))
					testutil.Ok(t, err)
					defer sr.Close()
					r := wal.NewReader(sr)

					var actRec int
					for r.Next() {
						actRec++
					}
					testutil.Ok(t, r.Err())
					testutil.Equals(t, test.expRecs, actRec, "Wrong number of intact records")
				}
			})
		}
	}
}

func TestHeadReadWriterRepair(t *testing.T) {
	dir, err := ioutil.TempDir("", "head_read_writer_repair")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	const chunkRange = 1000

	walDir := filepath.Join(dir, "wal")
	// Fill the chunk segments and corrupt it.
	{
		w, err := wal.New(nil, nil, walDir, false)
		testutil.Ok(t, err)

		h, err := NewHead(nil, nil, w, chunkRange, dir, nil, DefaultStripeSize, nil)
		testutil.Ok(t, err)
		testutil.Equals(t, 0.0, prom_testutil.ToFloat64(h.metrics.mmapChunkCorruptionTotal))
		testutil.Ok(t, h.Init(math.MinInt64))

		s, created, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))
		testutil.Assert(t, created, "series was not created")

		for i := 0; i < 7; i++ {
			ok, chunkCreated := s.append(int64(i*chunkRange), float64(i*chunkRange), 0, h.chunkDiskMapper)
			testutil.Assert(t, ok, "series append failed")
			testutil.Assert(t, chunkCreated, "chunk was not created")
			ok, chunkCreated = s.append(int64(i*chunkRange)+chunkRange-1, float64(i*chunkRange), 0, h.chunkDiskMapper)
			testutil.Assert(t, ok, "series append failed")
			testutil.Assert(t, !chunkCreated, "chunk was created")
			testutil.Ok(t, h.chunkDiskMapper.CutNewFile())
		}
		testutil.Ok(t, h.Close())

		// Verify that there are 7 segment files.
		files, err := ioutil.ReadDir(mmappedChunksDir(dir))
		testutil.Ok(t, err)
		testutil.Equals(t, 7, len(files))

		// Corrupt the 4th file by writing a random byte to series ref.
		f, err := os.OpenFile(filepath.Join(mmappedChunksDir(dir), files[3].Name()), os.O_WRONLY, 0666)
		testutil.Ok(t, err)
		n, err := f.WriteAt([]byte{67, 88}, chunks.HeadChunkFileHeaderSize+2)
		testutil.Ok(t, err)
		testutil.Equals(t, 2, n)
		testutil.Ok(t, f.Close())
	}

	// Open the db to trigger a repair.
	{
		db, err := Open(dir, nil, nil, DefaultOptions())
		testutil.Ok(t, err)
		defer func() {
			testutil.Ok(t, db.Close())
		}()
		testutil.Equals(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.mmapChunkCorruptionTotal))
	}

	// Verify that there are 3 segment files after the repair.
	// The segments from the corrupt segment should be removed.
	{
		files, err := ioutil.ReadDir(mmappedChunksDir(dir))
		testutil.Ok(t, err)
		testutil.Equals(t, 3, len(files))
	}
}

func TestNewWalSegmentOnTruncate(t *testing.T) {
	h, wlog, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, h.Close())
	}()
	add := func(ts int64) {
		app := h.Appender()
		_, err := app.Add(labels.Labels{{Name: "a", Value: "b"}}, ts, 0)
		testutil.Ok(t, err)
		testutil.Ok(t, app.Commit())
	}

	add(0)
	_, last, err := wlog.Segments()
	testutil.Ok(t, err)
	testutil.Equals(t, 0, last)

	add(1)
	testutil.Ok(t, h.Truncate(1))
	_, last, err = wlog.Segments()
	testutil.Ok(t, err)
	testutil.Equals(t, 1, last)

	add(2)
	testutil.Ok(t, h.Truncate(2))
	_, last, err = wlog.Segments()
	testutil.Ok(t, err)
	testutil.Equals(t, 2, last)
}

func TestAddDuplicateLabelName(t *testing.T) {
	h, _, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, h.Close())
	}()

	add := func(labels labels.Labels, labelName string) {
		app := h.Appender()
		_, err := app.Add(labels, 0, 0)
		testutil.NotOk(t, err)
		testutil.Equals(t, fmt.Sprintf(`label name "%s" is not unique: invalid sample`, labelName), err.Error())
	}

	add(labels.Labels{{Name: "a", Value: "c"}, {Name: "a", Value: "b"}}, "a")
	add(labels.Labels{{Name: "a", Value: "c"}, {Name: "a", Value: "c"}}, "a")
	add(labels.Labels{{Name: "__name__", Value: "up"}, {Name: "job", Value: "prometheus"}, {Name: "le", Value: "500"}, {Name: "le", Value: "400"}, {Name: "unit", Value: "s"}}, "le")
}

func TestMemSeriesIsolation(t *testing.T) {
	// Put a series, select it. GC it and then access it.

	lastValue := func(h *Head, maxAppendID uint64) int {
		idx, err := h.Index()

		testutil.Ok(t, err)

		iso := h.iso.State()
		iso.maxAppendID = maxAppendID

		chunks, err := h.chunksRange(math.MinInt64, math.MaxInt64, iso)
		testutil.Ok(t, err)
		querier := &blockQuerier{
			mint:       0,
			maxt:       10000,
			index:      idx,
			chunks:     chunks,
			tombstones: tombstones.NewMemTombstones(),
		}

		testutil.Ok(t, err)
		defer querier.Close()

		ss := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
		_, seriesSet, ws, err := expandSeriesSet(ss)
		testutil.Ok(t, err)
		testutil.Equals(t, 0, len(ws))

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

			_, err := app.Add(labels.FromStrings("foo", "bar"), int64(i), float64(i))
			testutil.Ok(t, err)
			testutil.Ok(t, app.Commit())
		}
		return i
	}

	testIsolation := func(h *Head, i int) {
	}

	// Test isolation without restart of Head.
	hb, _, closer := newTestHead(t, 1000, false)
	i := addSamples(hb)
	testIsolation(hb, i)

	// Test simple cases in different chunks when no appendID cleanup has been performed.
	testutil.Equals(t, 10, lastValue(hb, 10))
	testutil.Equals(t, 130, lastValue(hb, 130))
	testutil.Equals(t, 160, lastValue(hb, 160))
	testutil.Equals(t, 240, lastValue(hb, 240))
	testutil.Equals(t, 500, lastValue(hb, 500))
	testutil.Equals(t, 750, lastValue(hb, 750))
	testutil.Equals(t, 995, lastValue(hb, 995))
	testutil.Equals(t, 999, lastValue(hb, 999))

	// Cleanup appendIDs below 500.
	app := hb.appender()
	app.cleanupAppendIDsBelow = 500
	_, err := app.Add(labels.FromStrings("foo", "bar"), int64(i), float64(i))
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	i++

	// We should not get queries with a maxAppendID below 500 after the cleanup,
	// but they only take the remaining appendIDs into account.
	testutil.Equals(t, 499, lastValue(hb, 10))
	testutil.Equals(t, 499, lastValue(hb, 130))
	testutil.Equals(t, 499, lastValue(hb, 160))
	testutil.Equals(t, 499, lastValue(hb, 240))
	testutil.Equals(t, 500, lastValue(hb, 500))
	testutil.Equals(t, 995, lastValue(hb, 995))
	testutil.Equals(t, 999, lastValue(hb, 999))

	// Cleanup appendIDs below 1000, which means the sample buffer is
	// the only thing with appendIDs.
	app = hb.appender()
	app.cleanupAppendIDsBelow = 1000
	_, err = app.Add(labels.FromStrings("foo", "bar"), int64(i), float64(i))
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	testutil.Equals(t, 999, lastValue(hb, 998))
	testutil.Equals(t, 999, lastValue(hb, 999))
	testutil.Equals(t, 1000, lastValue(hb, 1000))
	testutil.Equals(t, 1001, lastValue(hb, 1001))
	testutil.Equals(t, 1002, lastValue(hb, 1002))
	testutil.Equals(t, 1002, lastValue(hb, 1003))

	i++
	// Cleanup appendIDs below 1001, but with a rollback.
	app = hb.appender()
	app.cleanupAppendIDsBelow = 1001
	_, err = app.Add(labels.FromStrings("foo", "bar"), int64(i), float64(i))
	testutil.Ok(t, err)
	testutil.Ok(t, app.Rollback())
	testutil.Equals(t, 1000, lastValue(hb, 999))
	testutil.Equals(t, 1000, lastValue(hb, 1000))
	testutil.Equals(t, 1001, lastValue(hb, 1001))
	testutil.Equals(t, 1002, lastValue(hb, 1002))
	testutil.Equals(t, 1002, lastValue(hb, 1003))

	testutil.Ok(t, hb.Close())
	closer()

	// Test isolation with restart of Head. This is to verify the num samples of chunks after m-map chunk replay.
	hb, w, closer := newTestHead(t, 1000, false)
	defer closer()
	i = addSamples(hb)
	testutil.Ok(t, hb.Close())

	wlog, err := wal.NewSize(nil, nil, w.Dir(), 32768, false)
	testutil.Ok(t, err)
	hb, err = NewHead(nil, nil, wlog, 1000, wlog.Dir(), nil, DefaultStripeSize, nil)
	defer func() { testutil.Ok(t, hb.Close()) }()
	testutil.Ok(t, err)
	testutil.Ok(t, hb.Init(0))

	// No appends after restarting. Hence all should return the last value.
	testutil.Equals(t, 1000, lastValue(hb, 10))
	testutil.Equals(t, 1000, lastValue(hb, 130))
	testutil.Equals(t, 1000, lastValue(hb, 160))
	testutil.Equals(t, 1000, lastValue(hb, 240))
	testutil.Equals(t, 1000, lastValue(hb, 500))

	// Cleanup appendIDs below 1000, which means the sample buffer is
	// the only thing with appendIDs.
	app = hb.appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), int64(i), float64(i))
	i++
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	testutil.Equals(t, 1001, lastValue(hb, 998))
	testutil.Equals(t, 1001, lastValue(hb, 999))
	testutil.Equals(t, 1001, lastValue(hb, 1000))
	testutil.Equals(t, 1001, lastValue(hb, 1001))
	testutil.Equals(t, 1001, lastValue(hb, 1002))
	testutil.Equals(t, 1001, lastValue(hb, 1003))

	// Cleanup appendIDs below 1002, but with a rollback.
	app = hb.appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), int64(i), float64(i))
	testutil.Ok(t, err)
	testutil.Ok(t, app.Rollback())
	testutil.Equals(t, 1001, lastValue(hb, 999))
	testutil.Equals(t, 1001, lastValue(hb, 1000))
	testutil.Equals(t, 1001, lastValue(hb, 1001))
	testutil.Equals(t, 1001, lastValue(hb, 1002))
	testutil.Equals(t, 1001, lastValue(hb, 1003))
}

func TestIsolationRollback(t *testing.T) {
	// Rollback after a failed append and test if the low watermark has progressed anyway.
	hb, _, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, hb.Close())
	}()

	app := hb.Appender()
	_, err := app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	testutil.Equals(t, uint64(1), hb.iso.lowWatermark())

	app = hb.Appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), 1, 1)
	testutil.Ok(t, err)
	_, err = app.Add(labels.FromStrings("foo", "bar", "foo", "baz"), 2, 2)
	testutil.NotOk(t, err)
	testutil.Ok(t, app.Rollback())
	testutil.Equals(t, uint64(2), hb.iso.lowWatermark())

	app = hb.Appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), 3, 3)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	testutil.Equals(t, uint64(3), hb.iso.lowWatermark(), "Low watermark should proceed to 3 even if append #2 was rolled back.")
}

func TestIsolationLowWatermarkMonotonous(t *testing.T) {
	hb, _, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, hb.Close())
	}()

	app1 := hb.Appender()
	_, err := app1.Add(labels.FromStrings("foo", "bar"), 0, 0)
	testutil.Ok(t, err)
	testutil.Ok(t, app1.Commit())
	testutil.Equals(t, uint64(1), hb.iso.lowWatermark(), "Low watermark should by 1 after 1st append.")

	app1 = hb.Appender()
	_, err = app1.Add(labels.FromStrings("foo", "bar"), 1, 1)
	testutil.Ok(t, err)
	testutil.Equals(t, uint64(2), hb.iso.lowWatermark(), "Low watermark should be two, even if append is not committed yet.")

	app2 := hb.Appender()
	_, err = app2.Add(labels.FromStrings("foo", "baz"), 1, 1)
	testutil.Ok(t, err)
	testutil.Ok(t, app2.Commit())
	testutil.Equals(t, uint64(2), hb.iso.lowWatermark(), "Low watermark should stay two because app1 is not committed yet.")

	is := hb.iso.State()
	testutil.Equals(t, uint64(2), hb.iso.lowWatermark(), "After simulated read (iso state retrieved), low watermark should stay at 2.")

	testutil.Ok(t, app1.Commit())
	testutil.Equals(t, uint64(2), hb.iso.lowWatermark(), "Even after app1 is committed, low watermark should stay at 2 because read is still ongoing.")

	is.Close()
	testutil.Equals(t, uint64(3), hb.iso.lowWatermark(), "After read has finished (iso state closed), low watermark should jump to three.")
}

func TestIsolationAppendIDZeroIsNoop(t *testing.T) {
	h, _, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, h.Close())
	}()

	h.initTime(0)

	s, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))

	ok, _ := s.append(0, 0, 0, h.chunkDiskMapper)
	testutil.Assert(t, ok, "Series append failed.")
	testutil.Equals(t, 0, s.txs.txIDCount, "Series should not have an appendID after append with appendID=0.")
}

func TestHeadSeriesChunkRace(t *testing.T) {
	for i := 0; i < 1000; i++ {
		testHeadSeriesChunkRace(t)
	}
}

func TestIsolationWithoutAdd(t *testing.T) {
	hb, _, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, hb.Close())
	}()

	app := hb.Appender()
	testutil.Ok(t, app.Commit())

	app = hb.Appender()
	_, err := app.Add(labels.FromStrings("foo", "baz"), 1, 1)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	testutil.Equals(t, hb.iso.lastAppendID(), hb.iso.lowWatermark(), "High watermark should be equal to the low watermark")
}

func TestOutOfOrderSamplesMetric(t *testing.T) {
	dir, err := ioutil.TempDir("", "test")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	db, err := Open(dir, nil, nil, DefaultOptions())
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, db.Close())
	}()
	db.DisableCompactions()

	app := db.Appender()
	for i := 1; i <= 5; i++ {
		_, err = app.Add(labels.FromStrings("a", "b"), int64(i), 99)
		testutil.Ok(t, err)
	}
	testutil.Ok(t, app.Commit())

	// Test out of order metric.
	testutil.Equals(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))
	app = db.Appender()
	_, err = app.Add(labels.FromStrings("a", "b"), 2, 99)
	testutil.Equals(t, storage.ErrOutOfOrderSample, err)
	testutil.Equals(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))

	_, err = app.Add(labels.FromStrings("a", "b"), 3, 99)
	testutil.Equals(t, storage.ErrOutOfOrderSample, err)
	testutil.Equals(t, 2.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))

	_, err = app.Add(labels.FromStrings("a", "b"), 4, 99)
	testutil.Equals(t, storage.ErrOutOfOrderSample, err)
	testutil.Equals(t, 3.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))
	testutil.Ok(t, app.Commit())

	// Compact Head to test out of bound metric.
	app = db.Appender()
	_, err = app.Add(labels.FromStrings("a", "b"), DefaultBlockDuration*2, 99)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	testutil.Equals(t, int64(math.MinInt64), db.head.minValidTime)
	testutil.Ok(t, db.Compact())
	testutil.Assert(t, db.head.minValidTime > 0, "")

	app = db.Appender()
	_, err = app.Add(labels.FromStrings("a", "b"), db.head.minValidTime-2, 99)
	testutil.Equals(t, storage.ErrOutOfBounds, err)
	testutil.Equals(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.outOfBoundSamples))

	_, err = app.Add(labels.FromStrings("a", "b"), db.head.minValidTime-1, 99)
	testutil.Equals(t, storage.ErrOutOfBounds, err)
	testutil.Equals(t, 2.0, prom_testutil.ToFloat64(db.head.metrics.outOfBoundSamples))
	testutil.Ok(t, app.Commit())

	// Some more valid samples for out of order.
	app = db.Appender()
	for i := 1; i <= 5; i++ {
		_, err = app.Add(labels.FromStrings("a", "b"), db.head.minValidTime+DefaultBlockDuration+int64(i), 99)
		testutil.Ok(t, err)
	}
	testutil.Ok(t, app.Commit())

	// Test out of order metric.
	app = db.Appender()
	_, err = app.Add(labels.FromStrings("a", "b"), db.head.minValidTime+DefaultBlockDuration+2, 99)
	testutil.Equals(t, storage.ErrOutOfOrderSample, err)
	testutil.Equals(t, 4.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))

	_, err = app.Add(labels.FromStrings("a", "b"), db.head.minValidTime+DefaultBlockDuration+3, 99)
	testutil.Equals(t, storage.ErrOutOfOrderSample, err)
	testutil.Equals(t, 5.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))

	_, err = app.Add(labels.FromStrings("a", "b"), db.head.minValidTime+DefaultBlockDuration+4, 99)
	testutil.Equals(t, storage.ErrOutOfOrderSample, err)
	testutil.Equals(t, 6.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples))
	testutil.Ok(t, app.Commit())
}

func testHeadSeriesChunkRace(t *testing.T) {
	h, _, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, h.Close())
	}()
	testutil.Ok(t, h.Init(0))
	app := h.Appender()

	s2, err := app.Add(labels.FromStrings("foo2", "bar"), 5, 0)
	testutil.Ok(t, err)
	for ts := int64(6); ts < 11; ts++ {
		err = app.AddFast(s2, ts, 0)
		testutil.Ok(t, err)
	}
	testutil.Ok(t, app.Commit())

	var wg sync.WaitGroup
	matcher := labels.MustNewMatcher(labels.MatchEqual, "", "")
	q, err := NewBlockQuerier(h, 18, 22)
	testutil.Ok(t, err)
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
	testutil.Ok(t, ss.Err())
	wg.Wait()
}

func newTestHead(t testing.TB, chunkRange int64, compressWAL bool) (*Head, *wal.WAL, func()) {
	dir, err := ioutil.TempDir("", "test")
	testutil.Ok(t, err)
	wlog, err := wal.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compressWAL)
	testutil.Ok(t, err)

	h, err := NewHead(nil, nil, wlog, chunkRange, dir, nil, DefaultStripeSize, nil)
	testutil.Ok(t, err)

	testutil.Ok(t, h.chunkDiskMapper.IterateAllChunks(func(_, _ uint64, _, _ int64, _ uint16) error { return nil }))

	return h, wlog, func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}
}

func TestHeadLabelNamesValuesWithMinMaxRange(t *testing.T) {
	head, _, closer := newTestHead(t, 1000, false)
	defer closer()
	defer func() {
		testutil.Ok(t, head.Close())
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

	app := head.Appender()
	for i, name := range expectedLabelNames {
		_, err := app.Add(labels.Labels{{Name: name, Value: expectedLabelValues[i]}}, seriesTimestamps[i], 0)
		testutil.Ok(t, err)
	}
	testutil.Ok(t, app.Commit())
	testutil.Equals(t, head.MinTime(), firstSeriesTimestamp)
	testutil.Equals(t, head.MaxTime(), lastSeriesTimestamp)

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
			testutil.Ok(t, err)
			testutil.Equals(t, tt.expectedNames, actualLabelNames)
			if len(tt.expectedValues) > 0 {
				for i, name := range expectedLabelNames {
					actualLabelValue, err := headIdxReader.SortedLabelValues(name)
					testutil.Ok(t, err)
					testutil.Equals(t, []string{tt.expectedValues[i]}, actualLabelValue)
				}
			}
		})
	}
}
