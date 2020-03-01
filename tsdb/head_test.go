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
	"path"
	"path/filepath"
	"sort"
	"strconv"
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

	h, err := NewHead(nil, nil, nil, 10000, DefaultStripeSize)
	testutil.Ok(b, err)
	defer h.Close()

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
					h, err := NewHead(nil, nil, w, 10000, DefaultStripeSize)
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
			dir, err := ioutil.TempDir("", "test_read_wal")
			testutil.Ok(t, err)
			defer func() {
				testutil.Ok(t, os.RemoveAll(dir))
			}()

			w, err := wal.New(nil, nil, dir, compress)
			testutil.Ok(t, err)
			defer w.Close()
			populateTestWAL(t, w, entries)

			head, err := NewHead(nil, nil, w, 10000, DefaultStripeSize)
			testutil.Ok(t, err)

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
			testutil.Equals(t, []sample{{100, 2}, {101, 5}}, expandChunk(s10.iterator(0, nil, nil)))
			testutil.Equals(t, []sample{{101, 6}}, expandChunk(s50.iterator(0, nil, nil)))
			testutil.Equals(t, []sample{{100, 3}, {101, 7}}, expandChunk(s100.iterator(0, nil, nil)))
		})
	}
}

func TestHead_WALMultiRef(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_wal_multi_ref")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	w, err := wal.New(nil, nil, dir, false)
	testutil.Ok(t, err)

	head, err := NewHead(nil, nil, w, 10000, DefaultStripeSize)
	testutil.Ok(t, err)

	testutil.Ok(t, head.Init(0))
	app := head.Appender()
	ref1, err := app.Add(labels.FromStrings("foo", "bar"), 100, 1)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	testutil.Ok(t, head.Truncate(200))

	app = head.Appender()
	ref2, err := app.Add(labels.FromStrings("foo", "bar"), 300, 2)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	if ref1 == ref2 {
		t.Fatal("Refs are the same")
	}
	testutil.Ok(t, head.Close())

	w, err = wal.New(nil, nil, dir, false)
	testutil.Ok(t, err)

	head, err = NewHead(nil, nil, w, 10000, DefaultStripeSize)
	testutil.Ok(t, err)
	testutil.Ok(t, head.Init(0))
	defer head.Close()

	q, err := NewBlockQuerier(head, 0, 300)
	testutil.Ok(t, err)
	series := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	testutil.Equals(t, map[string][]tsdbutil.Sample{`{foo="bar"}`: {sample{100, 1}, sample{300, 2}}}, series)
}

func TestHead_Truncate(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_truncate")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	w, err := wal.New(nil, nil, dir, false)
	testutil.Ok(t, err)

	h, err := NewHead(nil, nil, w, 10000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer h.Close()

	h.initTime(0)

	s1, _ := h.getOrCreate(1, labels.FromStrings("a", "1", "b", "1"))
	s2, _ := h.getOrCreate(2, labels.FromStrings("a", "2", "b", "1"))
	s3, _ := h.getOrCreate(3, labels.FromStrings("a", "1", "b", "2"))
	s4, _ := h.getOrCreate(4, labels.FromStrings("a", "2", "b", "2", "c", "1"))

	s1.chunks = []*memChunk{
		{minTime: 0, maxTime: 999, chunk: chunkenc.NewXORChunk()},
		{minTime: 1000, maxTime: 1999, chunk: chunkenc.NewXORChunk()},
		{minTime: 2000, maxTime: 2999, chunk: chunkenc.NewXORChunk()},
	}
	s2.chunks = []*memChunk{
		{minTime: 1000, maxTime: 1999, chunk: chunkenc.NewXORChunk()},
		{minTime: 2000, maxTime: 2999, chunk: chunkenc.NewXORChunk()},
		{minTime: 3000, maxTime: 3999, chunk: chunkenc.NewXORChunk()},
	}
	s3.chunks = []*memChunk{
		{minTime: 0, maxTime: 999, chunk: chunkenc.NewXORChunk()},
		{minTime: 1000, maxTime: 1999, chunk: chunkenc.NewXORChunk()},
	}
	s4.chunks = []*memChunk{}

	// Truncation need not be aligned.
	testutil.Ok(t, h.Truncate(1))

	testutil.Ok(t, h.Truncate(2000))

	testutil.Equals(t, []*memChunk{
		{minTime: 2000, maxTime: 2999, chunk: chunkenc.NewXORChunk()},
	}, h.series.getByID(s1.ref).chunks)

	testutil.Equals(t, []*memChunk{
		{minTime: 2000, maxTime: 2999, chunk: chunkenc.NewXORChunk()},
		{minTime: 3000, maxTime: 3999, chunk: chunkenc.NewXORChunk()},
	}, h.series.getByID(s2.ref).chunks)

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
	s := newMemSeries(labels.FromStrings("a", "b"), 1, 2000)

	for i := 0; i < 4000; i += 5 {
		ok, _ := s.append(int64(i), float64(i), 0)
		testutil.Assert(t, ok == true, "sample append failed")
	}

	// Check that truncate removes half of the chunks and afterwards
	// that the ID of the last chunk still gives us the same chunk afterwards.
	countBefore := len(s.chunks)
	lastID := s.chunkID(countBefore - 1)
	lastChunk := s.chunk(lastID)

	testutil.Assert(t, s.chunk(0) != nil, "")
	testutil.Assert(t, lastChunk != nil, "")

	s.truncateChunksBefore(2000)

	testutil.Equals(t, int64(2000), s.chunks[0].minTime)
	testutil.Assert(t, s.chunk(0) == nil, "first chunks not gone")
	testutil.Equals(t, countBefore/2, len(s.chunks))
	testutil.Equals(t, lastChunk, s.chunk(lastID))

	// Validate that the series' sample buffer is applied correctly to the last chunk
	// after truncation.
	it1 := s.iterator(s.chunkID(len(s.chunks)-1), nil, nil)
	_, ok := it1.(*memSafeIterator)
	testutil.Assert(t, ok == true, "")

	it2 := s.iterator(s.chunkID(len(s.chunks)-2), nil, nil)
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
			dir, err := ioutil.TempDir("", "test_delete_series")
			testutil.Ok(t, err)
			defer func() {
				testutil.Ok(t, os.RemoveAll(dir))
			}()

			w, err := wal.New(nil, nil, dir, compress)
			testutil.Ok(t, err)
			defer w.Close()
			populateTestWAL(t, w, entries)

			head, err := NewHead(nil, nil, w, 1000, DefaultStripeSize)
			testutil.Ok(t, err)

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
		Outer:
			for _, c := range cases {
				dir, err := ioutil.TempDir("", "test_wal_reload")
				testutil.Ok(t, err)
				defer func() {
					testutil.Ok(t, os.RemoveAll(dir))
				}()

				w, err := wal.New(nil, nil, path.Join(dir, "wal"), compress)
				testutil.Ok(t, err)
				defer w.Close()

				head, err := NewHead(nil, nil, w, 1000, DefaultStripeSize)
				testutil.Ok(t, err)
				defer head.Close()

				app := head.Appender()
				for _, smpl := range smplsAll {
					_, err = app.Add(labels.Labels{lblDefault}, smpl.t, smpl.v)
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
					_, err = app.Add(labels.Labels{lblDefault}, smpl.t, smpl.v)
					testutil.Ok(t, err)

				}
				testutil.Ok(t, app.Commit())

				// Compare the samples for both heads - before and after the reload.
				reloadedW, err := wal.New(nil, nil, w.Dir(), compress) // Use a new wal to ensure deleted samples are gone even after a reload.
				testutil.Ok(t, err)
				defer reloadedW.Close()
				reloadedHead, err := NewHead(nil, nil, reloadedW, 1000, DefaultStripeSize)
				testutil.Ok(t, err)
				defer reloadedHead.Close()
				testutil.Ok(t, reloadedHead.Init(0))

				// Compare the query results for both heads - before and after the reload.
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
				for _, h := range []*Head{head, reloadedHead} {
					q, err := NewBlockQuerier(h, h.MinTime(), h.MaxTime())
					testutil.Ok(t, err)
					actSeriesSet, ws, err := q.Select(nil, labels.MustNewMatcher(labels.MatchEqual, lblDefault.Name, lblDefault.Value))
					testutil.Ok(t, err)
					testutil.Equals(t, 0, len(ws))

					for {
						eok, rok := expSeriesSet.Next(), actSeriesSet.Next()
						testutil.Equals(t, eok, rok)

						if !eok {
							testutil.Ok(t, h.Close())
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
	numSamples := int64(10)
	hb, err := NewHead(nil, nil, nil, 1000000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer hb.Close()
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
	res, ws, err := q.Select(nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	testutil.Ok(t, err)
	testutil.Equals(t, 0, len(ws))
	testutil.Assert(t, res.Next(), "series is not present")
	s := res.At()
	it := s.Iterator()
	testutil.Assert(t, !it.Next(), "expected no samples")

	// Add again and test for presence.
	app = hb.Appender()
	_, err = app.Add(labels.Labels{{Name: "a", Value: "b"}}, 11, 1)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	q, err = NewBlockQuerier(hb, 0, 100000)
	testutil.Ok(t, err)
	res, ws, err = q.Select(nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	testutil.Ok(t, err)
	testutil.Equals(t, 0, len(ws))
	testutil.Assert(t, res.Next(), "series don't exist")
	exps := res.At()
	it = exps.Iterator()
	resSamples, err := expandSeriesIterator(it)
	testutil.Ok(t, err)
	testutil.Equals(t, []tsdbutil.Sample{sample{11, 1}}, resSamples)
}

func TestDeletedSamplesAndSeriesStillInWALAfterCheckpoint(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_delete_wal")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()
	wlog, err := wal.NewSize(nil, nil, dir, 32768, false)
	testutil.Ok(t, err)

	// Enough samples to cause a checkpoint.
	numSamples := 10000
	hb, err := NewHead(nil, nil, wlog, int64(numSamples)*10, DefaultStripeSize)
	testutil.Ok(t, err)
	defer hb.Close()
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
	cdir, _, err := wal.LastCheckpoint(dir)
	testutil.Ok(t, err)
	// Read in checkpoint and WAL.
	recs := readTestWAL(t, cdir)
	recs = append(recs, readTestWAL(t, dir)...)

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
	dir, _ := ioutil.TempDir("", "test")
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()
	hb, err := NewHead(nil, nil, nil, 100000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer hb.Close()
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
			ss, ws, err := q.SelectSorted(nil, del.ms...)
			testutil.Ok(t, err)
			testutil.Equals(t, 0, len(ws))
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
	s := newMemSeries(labels.Labels{}, 1, 500)

	// Add first two samples at the very end of a chunk range and the next two
	// on and after it.
	// New chunk must correctly be cut at 1000.
	ok, chunkCreated := s.append(998, 1, 0)
	testutil.Assert(t, ok, "append failed")
	testutil.Assert(t, chunkCreated, "first sample created chunk")

	ok, chunkCreated = s.append(999, 2, 0)
	testutil.Assert(t, ok, "append failed")
	testutil.Assert(t, !chunkCreated, "second sample should use same chunk")

	ok, chunkCreated = s.append(1000, 3, 0)
	testutil.Assert(t, ok, "append failed")
	testutil.Assert(t, chunkCreated, "expected new chunk on boundary")

	ok, chunkCreated = s.append(1001, 4, 0)
	testutil.Assert(t, ok, "append failed")
	testutil.Assert(t, !chunkCreated, "second sample should use same chunk")

	testutil.Assert(t, s.chunks[0].minTime == 998 && s.chunks[0].maxTime == 999, "wrong chunk range")
	testutil.Assert(t, s.chunks[1].minTime == 1000 && s.chunks[1].maxTime == 1001, "wrong chunk range")

	// Fill the range [1000,2000) with many samples. Intermediate chunks should be cut
	// at approximately 120 samples per chunk.
	for i := 1; i < 1000; i++ {
		ok, _ := s.append(1001+int64(i), float64(i), 0)
		testutil.Assert(t, ok, "append failed")
	}

	testutil.Assert(t, len(s.chunks) > 7, "expected intermediate chunks")

	// All chunks but the first and last should now be moderately full.
	for i, c := range s.chunks[1 : len(s.chunks)-1] {
		testutil.Assert(t, c.chunk.NumSamples() > 100, "unexpected small chunk %d of length %d", i, c.chunk.NumSamples())
	}
}

func TestGCChunkAccess(t *testing.T) {
	// Put a chunk, select it. GC it and then access it.
	h, err := NewHead(nil, nil, nil, 1000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer h.Close()

	h.initTime(0)

	s, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))

	// Appending 2 samples for the first chunk.
	ok, chunkCreated := s.append(0, 0, 0)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(999, 999, 0)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, !chunkCreated, "chunks was created")

	// A new chunks should be created here as it's beyond the chunk range.
	ok, chunkCreated = s.append(1000, 1000, 0)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(1999, 1999, 0)
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

	cr := h.chunksRange(0, 1500, nil)
	_, err = cr.Chunk(chunks[0].Ref)
	testutil.Ok(t, err)
	_, err = cr.Chunk(chunks[1].Ref)
	testutil.Ok(t, err)

	testutil.Ok(t, h.Truncate(1500)) // Remove a chunk.

	_, err = cr.Chunk(chunks[0].Ref)
	testutil.Equals(t, ErrNotFound, err)
	_, err = cr.Chunk(chunks[1].Ref)
	testutil.Ok(t, err)
}

func TestGCSeriesAccess(t *testing.T) {
	// Put a series, select it. GC it and then access it.
	h, err := NewHead(nil, nil, nil, 1000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer h.Close()

	h.initTime(0)

	s, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))

	// Appending 2 samples for the first chunk.
	ok, chunkCreated := s.append(0, 0, 0)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(999, 999, 0)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, !chunkCreated, "chunks was created")

	// A new chunks should be created here as it's beyond the chunk range.
	ok, chunkCreated = s.append(1000, 1000, 0)
	testutil.Assert(t, ok, "series append failed")
	testutil.Assert(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(1999, 1999, 0)
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

	cr := h.chunksRange(0, 2000, nil)
	_, err = cr.Chunk(chunks[0].Ref)
	testutil.Ok(t, err)
	_, err = cr.Chunk(chunks[1].Ref)
	testutil.Ok(t, err)

	testutil.Ok(t, h.Truncate(2000)) // Remove the series.

	testutil.Equals(t, (*memSeries)(nil), h.series.getByID(1))

	_, err = cr.Chunk(chunks[0].Ref)
	testutil.Equals(t, ErrNotFound, err)
	_, err = cr.Chunk(chunks[1].Ref)
	testutil.Equals(t, ErrNotFound, err)
}

func TestUncommittedSamplesNotLostOnTruncate(t *testing.T) {
	h, err := NewHead(nil, nil, nil, 1000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer h.Close()

	h.initTime(0)

	app := h.appender(0, 0)
	lset := labels.FromStrings("a", "1")
	_, err = app.Add(lset, 2100, 1)
	testutil.Ok(t, err)

	testutil.Ok(t, h.Truncate(2000))
	testutil.Assert(t, nil != h.series.getByHash(lset.Hash(), lset), "series should not have been garbage collected")

	testutil.Ok(t, app.Commit())

	q, err := NewBlockQuerier(h, 1500, 2500)
	testutil.Ok(t, err)
	defer q.Close()

	ss, ws, err := q.Select(nil, labels.MustNewMatcher(labels.MatchEqual, "a", "1"))
	testutil.Ok(t, err)
	testutil.Equals(t, 0, len(ws))

	testutil.Equals(t, true, ss.Next())
}

func TestRemoveSeriesAfterRollbackAndTruncate(t *testing.T) {
	h, err := NewHead(nil, nil, nil, 1000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer h.Close()

	h.initTime(0)

	app := h.appender(0, 0)
	lset := labels.FromStrings("a", "1")
	_, err = app.Add(lset, 2100, 1)
	testutil.Ok(t, err)

	testutil.Ok(t, h.Truncate(2000))
	testutil.Assert(t, nil != h.series.getByHash(lset.Hash(), lset), "series should not have been garbage collected")

	testutil.Ok(t, app.Rollback())

	q, err := NewBlockQuerier(h, 1500, 2500)
	testutil.Ok(t, err)
	defer q.Close()

	ss, ws, err := q.Select(nil, labels.MustNewMatcher(labels.MatchEqual, "a", "1"))
	testutil.Ok(t, err)
	testutil.Equals(t, 0, len(ws))

	testutil.Equals(t, false, ss.Next())

	// Truncate again, this time the series should be deleted
	testutil.Ok(t, h.Truncate(2050))
	testutil.Equals(t, (*memSeries)(nil), h.series.getByHash(lset.Hash(), lset))
}

func TestHead_LogRollback(t *testing.T) {
	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			dir, err := ioutil.TempDir("", "wal_rollback")
			testutil.Ok(t, err)
			defer func() {
				testutil.Ok(t, os.RemoveAll(dir))
			}()

			w, err := wal.New(nil, nil, dir, compress)
			testutil.Ok(t, err)
			defer w.Close()
			h, err := NewHead(nil, nil, w, 1000, DefaultStripeSize)
			testutil.Ok(t, err)

			app := h.Appender()
			_, err = app.Add(labels.FromStrings("a", "b"), 1, 2)
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

					h, err := NewHead(nil, nil, w, 1, DefaultStripeSize)
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

func TestNewWalSegmentOnTruncate(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_wal_segments")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()
	wlog, err := wal.NewSize(nil, nil, dir, 32768, false)
	testutil.Ok(t, err)

	h, err := NewHead(nil, nil, wlog, 1000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer h.Close()
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
	dir, err := ioutil.TempDir("", "test_duplicate_label_name")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()
	wlog, err := wal.NewSize(nil, nil, dir, 32768, false)
	testutil.Ok(t, err)

	h, err := NewHead(nil, nil, wlog, 1000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer h.Close()

	add := func(labels labels.Labels, labelName string) {
		app := h.Appender()
		_, err = app.Add(labels, 0, 0)
		testutil.NotOk(t, err)
		testutil.Equals(t, fmt.Sprintf(`label name "%s" is not unique: invalid sample`, labelName), err.Error())
	}

	add(labels.Labels{{Name: "a", Value: "c"}, {Name: "a", Value: "b"}}, "a")
	add(labels.Labels{{Name: "a", Value: "c"}, {Name: "a", Value: "c"}}, "a")
	add(labels.Labels{{Name: "__name__", Value: "up"}, {Name: "job", Value: "prometheus"}, {Name: "le", Value: "500"}, {Name: "le", Value: "400"}, {Name: "unit", Value: "s"}}, "le")
}

func TestHeadSeriesWithTimeBoundaries(t *testing.T) {
	h, err := NewHead(nil, nil, nil, 15, DefaultStripeSize)
	testutil.Ok(t, err)
	defer h.Close()
	app := h.Appender()

	s1, err := app.Add(labels.FromStrings("foo1", "bar"), 2, 0)
	testutil.Ok(t, err)
	for ts := int64(3); ts < 13; ts++ {
		err = app.AddFast(s1, ts, 0)
		testutil.Ok(t, err)
	}
	s2, err := app.Add(labels.FromStrings("foo2", "bar"), 5, 0)
	testutil.Ok(t, err)
	for ts := int64(6); ts < 11; ts++ {
		err = app.AddFast(s2, ts, 0)
		testutil.Ok(t, err)
	}
	s3, err := app.Add(labels.FromStrings("foo3", "bar"), 5, 0)
	testutil.Ok(t, err)
	err = app.AddFast(s3, 6, 0)
	testutil.Ok(t, err)
	_, err = app.Add(labels.FromStrings("foo4", "bar"), 9, 0)
	testutil.Ok(t, err)

	testutil.Ok(t, app.Commit())

	cases := []struct {
		mint         int64
		maxt         int64
		seriesCount  int
		samplesCount int
	}{
		// foo1 ..00000000000..
		// foo2 .....000000....
		// foo3 .....00........
		// foo4 .........0.....
		{mint: 0, maxt: 0, seriesCount: 0, samplesCount: 0},
		{mint: 0, maxt: 1, seriesCount: 0, samplesCount: 0},
		{mint: 0, maxt: 2, seriesCount: 1, samplesCount: 1},
		{mint: 2, maxt: 2, seriesCount: 1, samplesCount: 1},
		{mint: 0, maxt: 4, seriesCount: 1, samplesCount: 3},
		{mint: 0, maxt: 5, seriesCount: 3, samplesCount: 6},
		{mint: 0, maxt: 6, seriesCount: 3, samplesCount: 9},
		{mint: 0, maxt: 7, seriesCount: 3, samplesCount: 11},
		{mint: 0, maxt: 8, seriesCount: 3, samplesCount: 13},
		{mint: 0, maxt: 9, seriesCount: 4, samplesCount: 16},
		{mint: 0, maxt: 10, seriesCount: 4, samplesCount: 18},
		{mint: 0, maxt: 11, seriesCount: 4, samplesCount: 19},
		{mint: 0, maxt: 12, seriesCount: 4, samplesCount: 20},
		{mint: 0, maxt: 13, seriesCount: 4, samplesCount: 20},
		{mint: 0, maxt: 14, seriesCount: 4, samplesCount: 20},
		{mint: 2, maxt: 14, seriesCount: 4, samplesCount: 20},
		{mint: 3, maxt: 14, seriesCount: 4, samplesCount: 19},
		{mint: 4, maxt: 14, seriesCount: 4, samplesCount: 18},
		{mint: 8, maxt: 9, seriesCount: 3, samplesCount: 5},
		{mint: 9, maxt: 9, seriesCount: 3, samplesCount: 3},
		{mint: 6, maxt: 9, seriesCount: 4, samplesCount: 10},
		{mint: 11, maxt: 11, seriesCount: 1, samplesCount: 1},
		{mint: 11, maxt: 12, seriesCount: 1, samplesCount: 2},
		{mint: 11, maxt: 14, seriesCount: 1, samplesCount: 2},
		{mint: 12, maxt: 14, seriesCount: 1, samplesCount: 1},
	}

	for i, c := range cases {
		matcher := labels.MustNewMatcher(labels.MatchEqual, "", "")
		q, err := NewBlockQuerier(h, c.mint, c.maxt)
		testutil.Ok(t, err)

		seriesCount := 0
		samplesCount := 0
		ss, _, err := q.Select(nil, matcher)
		testutil.Ok(t, err)
		for ss.Next() {
			i := ss.At().Iterator()
			for i.Next() {
				samplesCount++
			}
			seriesCount++
		}
		testutil.Ok(t, ss.Err())
		testutil.Equals(t, c.seriesCount, seriesCount, "test series %d", i)
		testutil.Equals(t, c.samplesCount, samplesCount, "test samples %d", i)
		q.Close()
	}
}

func TestMemSeriesIsolation(t *testing.T) {
	// Put a series, select it. GC it and then access it.
	hb, err := NewHead(nil, nil, nil, 1000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer hb.Close()

	lastValue := func(maxAppendID uint64) int {
		idx, err := hb.Index(hb.MinTime(), hb.MaxTime())
		testutil.Ok(t, err)

		iso := hb.iso.State()
		iso.maxAppendID = maxAppendID

		querier := &blockQuerier{
			mint:       0,
			maxt:       10000,
			index:      idx,
			chunks:     hb.chunksRange(math.MinInt64, math.MaxInt64, iso),
			tombstones: tombstones.NewMemTombstones(),
		}

		testutil.Ok(t, err)
		defer querier.Close()

		ss, _, err := querier.Select(nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
		testutil.Ok(t, err)

		_, seriesSet, err := expandSeriesSet(ss)
		testutil.Ok(t, err)
		for _, series := range seriesSet {
			return int(series[len(series)-1].v)
		}
		return -1
	}

	i := 0
	for ; i <= 1000; i++ {
		var app storage.Appender
		// To initialize bounds.
		if hb.MinTime() == math.MaxInt64 {
			app = &initAppender{head: hb, appendID: uint64(i), cleanupAppendIDsBelow: 0}
		} else {
			app = hb.appender(uint64(i), 0)
		}

		_, err := app.Add(labels.FromStrings("foo", "bar"), int64(i), float64(i))
		testutil.Ok(t, err)
		testutil.Ok(t, app.Commit())
	}

	// Test simple cases in different chunks when no appendID cleanup has been performed.
	testutil.Equals(t, 10, lastValue(10))
	testutil.Equals(t, 130, lastValue(130))
	testutil.Equals(t, 160, lastValue(160))
	testutil.Equals(t, 240, lastValue(240))
	testutil.Equals(t, 500, lastValue(500))
	testutil.Equals(t, 750, lastValue(750))
	testutil.Equals(t, 995, lastValue(995))
	testutil.Equals(t, 999, lastValue(999))

	// Cleanup appendIDs below 500.
	app := hb.appender(uint64(i), 500)
	_, err = app.Add(labels.FromStrings("foo", "bar"), int64(i), float64(i))
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	i++

	// We should not get queries with a maxAppendID below 500 after the cleanup,
	// but they only take the remaining appendIDs into account.
	testutil.Equals(t, 499, lastValue(10))
	testutil.Equals(t, 499, lastValue(130))
	testutil.Equals(t, 499, lastValue(160))
	testutil.Equals(t, 499, lastValue(240))
	testutil.Equals(t, 500, lastValue(500))
	testutil.Equals(t, 995, lastValue(995))
	testutil.Equals(t, 999, lastValue(999))

	// Cleanup appendIDs below 1000, which means the sample buffer is
	// the only thing with appendIDs.
	app = hb.appender(uint64(i), 1000)
	_, err = app.Add(labels.FromStrings("foo", "bar"), int64(i), float64(i))
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	testutil.Equals(t, 999, lastValue(998))
	testutil.Equals(t, 999, lastValue(999))
	testutil.Equals(t, 1000, lastValue(1000))
	testutil.Equals(t, 1001, lastValue(1001))
	testutil.Equals(t, 1002, lastValue(1002))
	testutil.Equals(t, 1002, lastValue(1003))

	i++
	// Cleanup appendIDs below 1001, but with a rollback.
	app = hb.appender(uint64(i), 1001)
	_, err = app.Add(labels.FromStrings("foo", "bar"), int64(i), float64(i))
	testutil.Ok(t, err)
	testutil.Ok(t, app.Rollback())
	testutil.Equals(t, 1000, lastValue(999))
	testutil.Equals(t, 1000, lastValue(1000))
	testutil.Equals(t, 1001, lastValue(1001))
	testutil.Equals(t, 1002, lastValue(1002))
	testutil.Equals(t, 1002, lastValue(1003))
}

func TestIsolationRollback(t *testing.T) {
	// Rollback after a failed append and test if the low watermark has progressed anyway.
	hb, err := NewHead(nil, nil, nil, 1000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer hb.Close()

	app := hb.Appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), 0, 0)
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
	hb, err := NewHead(nil, nil, nil, 1000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer hb.Close()

	app1 := hb.Appender()
	_, err = app1.Add(labels.FromStrings("foo", "bar"), 0, 0)
	testutil.Ok(t, err)
	testutil.Ok(t, app1.Commit())
	testutil.Equals(t, uint64(1), hb.iso.lowWatermark(), "Low watermark should by 1 after 1st append.")

	app1 = hb.Appender()
	_, err = app1.Add(labels.FromStrings("foo", "bar"), 1, 1)
	testutil.Ok(t, err)
	testutil.Equals(t, uint64(2), hb.iso.lowWatermark(), "Low watermark should be two, even if append is not commited yet.")

	app2 := hb.Appender()
	_, err = app2.Add(labels.FromStrings("foo", "baz"), 1, 1)
	testutil.Ok(t, err)
	testutil.Ok(t, app2.Commit())
	testutil.Equals(t, uint64(2), hb.iso.lowWatermark(), "Low watermark should stay two because app1 is not commited yet.")

	is := hb.iso.State()
	testutil.Equals(t, uint64(2), hb.iso.lowWatermark(), "After simulated read (iso state retrieved), low watermark should stay at 2.")

	testutil.Ok(t, app1.Commit())
	testutil.Equals(t, uint64(2), hb.iso.lowWatermark(), "Even after app1 is commited, low watermark should stay at 2 because read is still ongoing.")

	is.Close()
	testutil.Equals(t, uint64(3), hb.iso.lowWatermark(), "After read has finished (iso state closed), low watermark should jump to three.")
}

func TestIsolationAppendIDZeroIsNoop(t *testing.T) {
	h, err := NewHead(nil, nil, nil, 1000, DefaultStripeSize)
	testutil.Ok(t, err)
	defer h.Close()

	h.initTime(0)

	s, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))

	ok, _ := s.append(0, 0, 0)
	testutil.Assert(t, ok, "Series append failed.")
	testutil.Equals(t, 0, s.txs.txIDCount, "Series should not have an appendID after append with appendID=0.")
}
