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
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"sort"
	"testing"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/tsdb/chunkenc"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/index"
	"github.com/prometheus/tsdb/labels"
	"github.com/prometheus/tsdb/testutil"
	"github.com/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/tsdb/wal"
)

func BenchmarkCreateSeries(b *testing.B) {
	lbls, err := labels.ReadLabels(filepath.Join("testdata", "20kseries.json"), b.N)
	testutil.Ok(b, err)

	h, err := NewHead(nil, nil, nil, 10000)
	testutil.Ok(b, err)
	defer h.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for _, l := range lbls {
		h.getOrCreate(l.Hash(), l)
	}
}

func populateTestWAL(t testing.TB, w *wal.WAL, recs []interface{}) {
	var enc RecordEncoder
	for _, r := range recs {
		switch v := r.(type) {
		case []RefSeries:
			testutil.Ok(t, w.Log(enc.Series(v, nil)))
		case []RefSample:
			testutil.Ok(t, w.Log(enc.Samples(v, nil)))
		case []Stone:
			testutil.Ok(t, w.Log(enc.Tombstones(v, nil)))
		}
	}
}

func readTestWAL(t testing.TB, dir string) (recs []interface{}) {
	sr, err := wal.NewSegmentsReader(dir)
	testutil.Ok(t, err)
	defer sr.Close()

	var dec RecordDecoder
	r := wal.NewReader(sr)

	for r.Next() {
		rec := r.Record()

		switch dec.Type(rec) {
		case RecordSeries:
			series, err := dec.Series(rec, nil)
			testutil.Ok(t, err)
			recs = append(recs, series)
		case RecordSamples:
			samples, err := dec.Samples(rec, nil)
			testutil.Ok(t, err)
			recs = append(recs, samples)
		case RecordTombstones:
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

func TestHead_ReadWAL(t *testing.T) {
	entries := []interface{}{
		[]RefSeries{
			{Ref: 10, Labels: labels.FromStrings("a", "1")},
			{Ref: 11, Labels: labels.FromStrings("a", "2")},
			{Ref: 100, Labels: labels.FromStrings("a", "3")},
		},
		[]RefSample{
			{Ref: 0, T: 99, V: 1},
			{Ref: 10, T: 100, V: 2},
			{Ref: 100, T: 100, V: 3},
		},
		[]RefSeries{
			{Ref: 50, Labels: labels.FromStrings("a", "4")},
		},
		[]RefSample{
			{Ref: 10, T: 101, V: 5},
			{Ref: 50, T: 101, V: 6},
		},
	}
	dir, err := ioutil.TempDir("", "test_read_wal")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	w, err := wal.New(nil, nil, dir)
	testutil.Ok(t, err)
	defer w.Close()
	populateTestWAL(t, w, entries)

	head, err := NewHead(nil, nil, w, 1000)
	testutil.Ok(t, err)

	testutil.Ok(t, head.Init(math.MinInt64))
	testutil.Equals(t, uint64(100), head.lastSeriesID)

	s10 := head.series.getByID(10)
	s11 := head.series.getByID(11)
	s50 := head.series.getByID(50)
	s100 := head.series.getByID(100)

	testutil.Equals(t, labels.FromStrings("a", "1"), s10.lset)
	testutil.Equals(t, (*memSeries)(nil), s11) // Series without samples should be garbage colected at head.Init().
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

	testutil.Equals(t, []sample{{100, 2}, {101, 5}}, expandChunk(s10.iterator(0)))
	testutil.Equals(t, []sample{{101, 6}}, expandChunk(s50.iterator(0)))
	testutil.Equals(t, []sample{{100, 3}}, expandChunk(s100.iterator(0)))
}

func TestHead_Truncate(t *testing.T) {
	h, err := NewHead(nil, nil, nil, 1000)
	testutil.Ok(t, err)
	defer h.Close()

	h.initTime(0)

	s1, _ := h.getOrCreate(1, labels.FromStrings("a", "1", "b", "1"))
	s2, _ := h.getOrCreate(2, labels.FromStrings("a", "2", "b", "1"))
	s3, _ := h.getOrCreate(3, labels.FromStrings("a", "1", "b", "2"))
	s4, _ := h.getOrCreate(4, labels.FromStrings("a", "2", "b", "2", "c", "1"))

	s1.chunks = []*memChunk{
		{minTime: 0, maxTime: 999},
		{minTime: 1000, maxTime: 1999},
		{minTime: 2000, maxTime: 2999},
	}
	s2.chunks = []*memChunk{
		{minTime: 1000, maxTime: 1999},
		{minTime: 2000, maxTime: 2999},
		{minTime: 3000, maxTime: 3999},
	}
	s3.chunks = []*memChunk{
		{minTime: 0, maxTime: 999},
		{minTime: 1000, maxTime: 1999},
	}
	s4.chunks = []*memChunk{}

	// Truncation need not be aligned.
	testutil.Ok(t, h.Truncate(1))

	testutil.Ok(t, h.Truncate(2000))

	testutil.Equals(t, []*memChunk{
		{minTime: 2000, maxTime: 2999},
	}, h.series.getByID(s1.ref).chunks)

	testutil.Equals(t, []*memChunk{
		{minTime: 2000, maxTime: 2999},
		{minTime: 3000, maxTime: 3999},
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
		ok, _ := s.append(int64(i), float64(i))
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
	it1 := s.iterator(s.chunkID(len(s.chunks) - 1))
	_, ok := it1.(*memSafeIterator)
	testutil.Assert(t, ok == true, "")

	it2 := s.iterator(s.chunkID(len(s.chunks) - 2))
	_, ok = it2.(*memSafeIterator)
	testutil.Assert(t, ok == false, "non-last chunk incorrectly wrapped with sample buffer")
}

func TestHeadDeleteSeriesWithoutSamples(t *testing.T) {
	entries := []interface{}{
		[]RefSeries{
			{Ref: 10, Labels: labels.FromStrings("a", "1")},
		},
		[]RefSample{},
		[]RefSeries{
			{Ref: 50, Labels: labels.FromStrings("a", "2")},
		},
		[]RefSample{
			{Ref: 50, T: 80, V: 1},
			{Ref: 50, T: 90, V: 1},
		},
	}
	dir, err := ioutil.TempDir("", "test_delete_series")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	w, err := wal.New(nil, nil, dir)
	testutil.Ok(t, err)
	defer w.Close()
	populateTestWAL(t, w, entries)

	head, err := NewHead(nil, nil, w, 1000)
	testutil.Ok(t, err)

	testutil.Ok(t, head.Init(math.MinInt64))

	testutil.Ok(t, head.Delete(0, 100, labels.NewEqualMatcher("a", "1")))
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
	lblDefault := labels.Label{"a", "b"}

	cases := []struct {
		dranges  Intervals
		smplsExp []sample
	}{
		{
			dranges:  Intervals{{0, 3}},
			smplsExp: buildSmpls([]int64{4, 5, 6, 7, 8, 9}),
		},
		{
			dranges:  Intervals{{1, 3}},
			smplsExp: buildSmpls([]int64{0, 4, 5, 6, 7, 8, 9}),
		},
		{
			dranges:  Intervals{{1, 3}, {4, 7}},
			smplsExp: buildSmpls([]int64{0, 8, 9}),
		},
		{
			dranges:  Intervals{{1, 3}, {4, 700}},
			smplsExp: buildSmpls([]int64{0}),
		},
		{ // This case is to ensure that labels and symbols are deleted.
			dranges:  Intervals{{0, 9}},
			smplsExp: buildSmpls([]int64{}),
		},
	}

Outer:
	for _, c := range cases {
		dir, err := ioutil.TempDir("", "test_wal_reload")
		testutil.Ok(t, err)
		defer os.RemoveAll(dir)

		w, err := wal.New(nil, nil, path.Join(dir, "wal"))
		testutil.Ok(t, err)

		head, err := NewHead(nil, nil, w, 1000)
		testutil.Ok(t, err)

		app := head.Appender()
		for _, smpl := range smplsAll {
			_, err = app.Add(labels.Labels{lblDefault}, smpl.t, smpl.v)
			testutil.Ok(t, err)

		}
		testutil.Ok(t, app.Commit())

		// Delete the ranges.
		for _, r := range c.dranges {
			testutil.Ok(t, head.Delete(r.Mint, r.Maxt, labels.NewEqualMatcher(lblDefault.Name, lblDefault.Value)))
		}

		// Compare the samples for both heads - before and after the reload.
		reloadedW, err := wal.New(nil, nil, w.Dir()) // Use a new wal to ensure deleted samples are gone even after a reload.
		testutil.Ok(t, err)
		reloadedHead, err := NewHead(nil, nil, reloadedW, 1000)
		testutil.Ok(t, err)
		testutil.Ok(t, reloadedHead.Init(0))
		for _, h := range []*Head{head, reloadedHead} {
			indexr, err := h.Index()
			testutil.Ok(t, err)
			// Use an emptyTombstoneReader explicitly to get all the samples.
			css, err := LookupChunkSeries(indexr, emptyTombstoneReader, labels.NewEqualMatcher(lblDefault.Name, lblDefault.Value))
			testutil.Ok(t, err)

			// Getting the actual samples.
			actSamples := make([]sample, 0)
			for css.Next() {
				lblsAct, chkMetas, intv := css.At()
				testutil.Equals(t, labels.Labels{lblDefault}, lblsAct)
				testutil.Equals(t, 0, len(intv))

				chunkr, err := h.Chunks()
				testutil.Ok(t, err)
				for _, meta := range chkMetas {
					chk, err := chunkr.Chunk(meta.Ref)
					testutil.Ok(t, err)
					ii := chk.Iterator()
					for ii.Next() {
						t, v := ii.At()
						actSamples = append(actSamples, sample{t: t, v: v})
					}
				}
			}

			testutil.Ok(t, css.Err())
			testutil.Equals(t, c.smplsExp, actSamples)
		}

		// Compare the query results for both heads - before and after the reload.
		expSeriesSet := newMockSeriesSet([]Series{
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
			actSeriesSet, err := q.Select(labels.NewEqualMatcher(lblDefault.Name, lblDefault.Value))
			testutil.Ok(t, err)

			lns, err := q.LabelNames()
			testutil.Ok(t, err)
			lvs, err := q.LabelValues(lblDefault.Name)
			testutil.Ok(t, err)
			// When all samples are deleted we expect that no labels should exist either.
			if len(c.smplsExp) == 0 {
				testutil.Equals(t, 0, len(lns))
				testutil.Equals(t, 0, len(lvs))
				testutil.Assert(t, actSeriesSet.Next() == false, "")
				testutil.Ok(t, h.Close())
				continue
			} else {
				testutil.Equals(t, 1, len(lns))
				testutil.Equals(t, 1, len(lvs))
				testutil.Equals(t, lblDefault.Name, lns[0])
				testutil.Equals(t, lblDefault.Value, lvs[0])
			}

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
}

func TestDeleteUntilCurMax(t *testing.T) {
	numSamples := int64(10)
	hb, err := NewHead(nil, nil, nil, 1000000)
	testutil.Ok(t, err)
	defer hb.Close()
	app := hb.Appender()
	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		_, err := app.Add(labels.Labels{{"a", "b"}}, i, smpls[i])
		testutil.Ok(t, err)
	}
	testutil.Ok(t, app.Commit())
	testutil.Ok(t, hb.Delete(0, 10000, labels.NewEqualMatcher("a", "b")))

	// Test the series have been deleted.
	q, err := NewBlockQuerier(hb, 0, 100000)
	testutil.Ok(t, err)
	res, err := q.Select(labels.NewEqualMatcher("a", "b"))
	testutil.Ok(t, err)
	testutil.Assert(t, !res.Next(), "series didn't get deleted")

	// Add again and test for presence.
	app = hb.Appender()
	_, err = app.Add(labels.Labels{{"a", "b"}}, 11, 1)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	q, err = NewBlockQuerier(hb, 0, 100000)
	testutil.Ok(t, err)
	res, err = q.Select(labels.NewEqualMatcher("a", "b"))
	testutil.Ok(t, err)
	testutil.Assert(t, res.Next(), "series don't exist")
	exps := res.At()
	it := exps.Iterator()
	ressmpls, err := expandSeriesIterator(it)
	testutil.Ok(t, err)
	testutil.Equals(t, []tsdbutil.Sample{sample{11, 1}}, ressmpls)
}
func TestDelete_e2e(t *testing.T) {
	numDatapoints := 1000
	numRanges := 1000
	timeInterval := int64(2)
	// Create 8 series with 1000 data-points of different ranges, delete and run queries.
	lbls := [][]labels.Label{
		{
			{"a", "b"},
			{"instance", "localhost:9090"},
			{"job", "prometheus"},
		},
		{
			{"a", "b"},
			{"instance", "127.0.0.1:9090"},
			{"job", "prometheus"},
		},
		{
			{"a", "b"},
			{"instance", "127.0.0.1:9090"},
			{"job", "prom-k8s"},
		},
		{
			{"a", "b"},
			{"instance", "localhost:9090"},
			{"job", "prom-k8s"},
		},
		{
			{"a", "c"},
			{"instance", "localhost:9090"},
			{"job", "prometheus"},
		},
		{
			{"a", "c"},
			{"instance", "127.0.0.1:9090"},
			{"job", "prometheus"},
		},
		{
			{"a", "c"},
			{"instance", "127.0.0.1:9090"},
			{"job", "prom-k8s"},
		},
		{
			{"a", "c"},
			{"instance", "localhost:9090"},
			{"job", "prom-k8s"},
		},
	}
	seriesMap := map[string][]tsdbutil.Sample{}
	for _, l := range lbls {
		seriesMap[labels.New(l...).String()] = []tsdbutil.Sample{}
	}
	dir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(dir)
	hb, err := NewHead(nil, nil, nil, 100000)
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
		ms     []labels.Matcher
		drange Intervals
	}{
		{
			ms:     []labels.Matcher{labels.NewEqualMatcher("a", "b")},
			drange: Intervals{{300, 500}, {600, 670}},
		},
		{
			ms: []labels.Matcher{
				labels.NewEqualMatcher("a", "b"),
				labels.NewEqualMatcher("job", "prom-k8s"),
			},
			drange: Intervals{{300, 500}, {100, 670}},
		},
		{
			ms: []labels.Matcher{
				labels.NewEqualMatcher("a", "c"),
				labels.NewEqualMatcher("instance", "localhost:9090"),
				labels.NewEqualMatcher("job", "prometheus"),
			},
			drange: Intervals{{300, 400}, {100, 6700}},
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
			ss, err := q.Select(del.ms...)
			testutil.Ok(t, err)
			// Build the mockSeriesSet.
			matchedSeries := make([]Series, 0, len(matched))
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

func deletedSamples(full []tsdbutil.Sample, dranges Intervals) []tsdbutil.Sample {
	ds := make([]tsdbutil.Sample, 0, len(full))
Outer:
	for _, s := range full {
		for _, r := range dranges {
			if r.inBounds(s.T()) {
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
	ok, chunkCreated := s.append(998, 1)
	testutil.Assert(t, ok, "append failed")
	testutil.Assert(t, chunkCreated, "first sample created chunk")

	ok, chunkCreated = s.append(999, 2)
	testutil.Assert(t, ok, "append failed")
	testutil.Assert(t, !chunkCreated, "second sample should use same chunk")

	ok, chunkCreated = s.append(1000, 3)
	testutil.Assert(t, ok, "append failed")
	testutil.Assert(t, chunkCreated, "expected new chunk on boundary")

	ok, chunkCreated = s.append(1001, 4)
	testutil.Assert(t, ok, "append failed")
	testutil.Assert(t, !chunkCreated, "second sample should use same chunk")

	testutil.Assert(t, s.chunks[0].minTime == 998 && s.chunks[0].maxTime == 999, "wrong chunk range")
	testutil.Assert(t, s.chunks[1].minTime == 1000 && s.chunks[1].maxTime == 1001, "wrong chunk range")

	// Fill the range [1000,2000) with many samples. Intermediate chunks should be cut
	// at approximately 120 samples per chunk.
	for i := 1; i < 1000; i++ {
		ok, _ := s.append(1001+int64(i), float64(i))
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
	h, err := NewHead(nil, nil, nil, 1000)
	testutil.Ok(t, err)
	defer h.Close()

	h.initTime(0)

	s, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))
	s.chunks = []*memChunk{
		{minTime: 0, maxTime: 999},
		{minTime: 1000, maxTime: 1999},
	}

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

	cr := h.chunksRange(0, 1500)
	_, err = cr.Chunk(chunks[0].Ref)
	testutil.Ok(t, err)
	_, err = cr.Chunk(chunks[1].Ref)
	testutil.Ok(t, err)

	h.Truncate(1500) // Remove a chunk.

	_, err = cr.Chunk(chunks[0].Ref)
	testutil.Equals(t, ErrNotFound, err)
	_, err = cr.Chunk(chunks[1].Ref)
	testutil.Ok(t, err)
}

func TestGCSeriesAccess(t *testing.T) {
	// Put a series, select it. GC it and then access it.
	h, err := NewHead(nil, nil, nil, 1000)
	testutil.Ok(t, err)
	defer h.Close()

	h.initTime(0)

	s, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))
	s.chunks = []*memChunk{
		{minTime: 0, maxTime: 999},
		{minTime: 1000, maxTime: 1999},
	}

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

	cr := h.chunksRange(0, 2000)
	_, err = cr.Chunk(chunks[0].Ref)
	testutil.Ok(t, err)
	_, err = cr.Chunk(chunks[1].Ref)
	testutil.Ok(t, err)

	h.Truncate(2000) // Remove the series.

	testutil.Equals(t, (*memSeries)(nil), h.series.getByID(1))

	_, err = cr.Chunk(chunks[0].Ref)
	testutil.Equals(t, ErrNotFound, err)
	_, err = cr.Chunk(chunks[1].Ref)
	testutil.Equals(t, ErrNotFound, err)
}

func TestUncommittedSamplesNotLostOnTruncate(t *testing.T) {
	h, err := NewHead(nil, nil, nil, 1000)
	testutil.Ok(t, err)
	defer h.Close()

	h.initTime(0)

	app := h.appender()
	lset := labels.FromStrings("a", "1")
	_, err = app.Add(lset, 2100, 1)
	testutil.Ok(t, err)

	testutil.Ok(t, h.Truncate(2000))
	testutil.Assert(t, nil != h.series.getByHash(lset.Hash(), lset), "series should not have been garbage collected")

	testutil.Ok(t, app.Commit())

	q, err := NewBlockQuerier(h, 1500, 2500)
	testutil.Ok(t, err)
	defer q.Close()

	ss, err := q.Select(labels.NewEqualMatcher("a", "1"))
	testutil.Ok(t, err)

	testutil.Equals(t, true, ss.Next())
}

func TestRemoveSeriesAfterRollbackAndTruncate(t *testing.T) {
	h, err := NewHead(nil, nil, nil, 1000)
	testutil.Ok(t, err)
	defer h.Close()

	h.initTime(0)

	app := h.appender()
	lset := labels.FromStrings("a", "1")
	_, err = app.Add(lset, 2100, 1)
	testutil.Ok(t, err)

	testutil.Ok(t, h.Truncate(2000))
	testutil.Assert(t, nil != h.series.getByHash(lset.Hash(), lset), "series should not have been garbage collected")

	testutil.Ok(t, app.Rollback())

	q, err := NewBlockQuerier(h, 1500, 2500)
	testutil.Ok(t, err)
	defer q.Close()

	ss, err := q.Select(labels.NewEqualMatcher("a", "1"))
	testutil.Ok(t, err)

	testutil.Equals(t, false, ss.Next())

	// Truncate again, this time the series should be deleted
	testutil.Ok(t, h.Truncate(2050))
	testutil.Equals(t, (*memSeries)(nil), h.series.getByHash(lset.Hash(), lset))
}

func TestHead_LogRollback(t *testing.T) {
	dir, err := ioutil.TempDir("", "wal_rollback")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	w, err := wal.New(nil, nil, dir)
	testutil.Ok(t, err)
	defer w.Close()
	h, err := NewHead(nil, nil, w, 1000)
	testutil.Ok(t, err)

	app := h.Appender()
	_, err = app.Add(labels.FromStrings("a", "b"), 1, 2)
	testutil.Ok(t, err)

	testutil.Ok(t, app.Rollback())
	recs := readTestWAL(t, w.Dir())

	testutil.Equals(t, 1, len(recs))

	series, ok := recs[0].([]RefSeries)
	testutil.Assert(t, ok, "expected series record but got %+v", recs[0])
	testutil.Equals(t, []RefSeries{{Ref: 1, Labels: labels.FromStrings("a", "b")}}, series)
}

func TestWalRepair(t *testing.T) {
	var enc RecordEncoder
	for name, test := range map[string]struct {
		corrFunc  func(rec []byte) []byte // Func that applies the corruption to a record.
		rec       []byte
		totalRecs int
		expRecs   int
	}{
		"invalid_record": {
			func(rec []byte) []byte {
				rec[0] = byte(RecordInvalid)
				return rec
			},
			enc.Series([]RefSeries{{Ref: 1, Labels: labels.FromStrings("a", "b")}}, []byte{}),
			9,
			5,
		},
		"decode_series": {
			func(rec []byte) []byte {
				return rec[:3]
			},
			enc.Series([]RefSeries{{Ref: 1, Labels: labels.FromStrings("a", "b")}}, []byte{}),
			9,
			5,
		},
		"decode_samples": {
			func(rec []byte) []byte {
				return rec[:3]
			},
			enc.Samples([]RefSample{{Ref: 0, T: 99, V: 1}}, []byte{}),
			9,
			5,
		},
		"decode_tombstone": {
			func(rec []byte) []byte {
				return rec[:3]
			},
			enc.Tombstones([]Stone{{ref: 1, intervals: Intervals{}}}, []byte{}),
			9,
			5,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "wal_head_repair")
			testutil.Ok(t, err)
			defer os.RemoveAll(dir)

			w, err := wal.New(nil, nil, dir)
			testutil.Ok(t, err)
			defer w.Close()

			for i := 1; i <= test.totalRecs; i++ {
				// At this point insert a corrupted record.
				if i-1 == test.expRecs {
					testutil.Ok(t, w.Log(test.corrFunc(test.rec)))
					continue
				}
				testutil.Ok(t, w.Log(test.rec))
			}

			h, err := NewHead(nil, nil, w, 1)
			testutil.Ok(t, err)
			testutil.Equals(t, 0.0, prom_testutil.ToFloat64(h.metrics.walCorruptionsTotal))
			testutil.Ok(t, h.Init(math.MinInt64))
			testutil.Equals(t, 1.0, prom_testutil.ToFloat64(h.metrics.walCorruptionsTotal))

			sr, err := wal.NewSegmentsReader(dir)
			testutil.Ok(t, err)
			defer sr.Close()
			r := wal.NewReader(sr)

			var actRec int
			for r.Next() {
				actRec++
			}
			testutil.Ok(t, r.Err())
			testutil.Equals(t, test.expRecs, actRec, "Wrong number of intact records")
		})
	}

}
