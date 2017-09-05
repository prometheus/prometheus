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
	"math/rand"
	"os"
	"testing"
	"unsafe"

	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/labels"

	promlabels "github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/stretchr/testify/require"
)

func BenchmarkCreateSeries(b *testing.B) {
	lbls, err := readPrometheusLabels("testdata/all.series", b.N)
	require.NoError(b, err)

	h, err := NewHead(nil, nil, nil, 10000)
	if err != nil {
		require.NoError(b, err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for _, l := range lbls {
		h.create(l.Hash(), l)
	}
}

func readPrometheusLabels(fn string, n int) ([]labels.Labels, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	p := textparse.New(b)
	i := 0
	var mets []labels.Labels
	hashes := map[uint64]struct{}{}

	for p.Next() && i < n {
		m := make(labels.Labels, 0, 10)
		p.Metric((*promlabels.Labels)(unsafe.Pointer(&m)))

		h := m.Hash()
		if _, ok := hashes[h]; ok {
			continue
		}
		mets = append(mets, m)
		hashes[h] = struct{}{}
		i++
	}
	if err := p.Err(); err != nil {
		return nil, err
	}
	if i != n {
		return mets, errors.Errorf("requested %d metrics but found %d", n, i)
	}
	return mets, nil
}

func TestHead_Truncate(t *testing.T) {
	h, err := NewHead(nil, nil, nil, 1000)
	require.NoError(t, err)

	h.initTime(0)

	s1 := h.create(1, labels.FromStrings("a", "1", "b", "1"))
	s2 := h.create(2, labels.FromStrings("a", "2", "b", "1"))
	s3 := h.create(3, labels.FromStrings("a", "1", "b", "2"))
	s4 := h.create(4, labels.FromStrings("a", "2", "b", "2", "c", "1"))

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

	// Truncation must be aligned.
	require.Error(t, h.Truncate(1))

	h.Truncate(2000)

	require.Equal(t, []*memChunk{
		{minTime: 2000, maxTime: 2999},
	}, h.series.getByID(s1.ref).chunks)

	require.Equal(t, []*memChunk{
		{minTime: 2000, maxTime: 2999},
		{minTime: 3000, maxTime: 3999},
	}, h.series.getByID(s2.ref).chunks)

	require.Nil(t, h.series.getByID(s3.ref))
	require.Nil(t, h.series.getByID(s4.ref))

	postingsA1, _ := expandPostings(h.postings.get("a", "1"))
	postingsA2, _ := expandPostings(h.postings.get("a", "2"))
	postingsB1, _ := expandPostings(h.postings.get("b", "1"))
	postingsB2, _ := expandPostings(h.postings.get("b", "2"))
	postingsC1, _ := expandPostings(h.postings.get("c", "1"))
	postingsAll, _ := expandPostings(h.postings.get("", ""))

	require.Equal(t, []uint64{s1.ref}, postingsA1)
	require.Equal(t, []uint64{s2.ref}, postingsA2)
	require.Equal(t, []uint64{s1.ref, s2.ref}, postingsB1)
	require.Equal(t, []uint64{s1.ref, s2.ref}, postingsAll)
	require.Nil(t, postingsB2)
	require.Nil(t, postingsC1)

	require.Equal(t, map[string]struct{}{
		"":  struct{}{}, // from 'all' postings list
		"a": struct{}{},
		"b": struct{}{},
		"1": struct{}{},
		"2": struct{}{},
	}, h.symbols)

	require.Equal(t, map[string]stringset{
		"a": stringset{"1": struct{}{}, "2": struct{}{}},
		"b": stringset{"1": struct{}{}},
		"":  stringset{"": struct{}{}},
	}, h.values)
}

// Validate various behaviors brought on by firstChunkID accounting for
// garbage collected chunks.
func TestMemSeries_truncateChunks(t *testing.T) {
	s := newMemSeries(labels.FromStrings("a", "b"), 1, 2000)

	for i := 0; i < 4000; i += 5 {
		ok, _ := s.append(int64(i), float64(i))
		require.True(t, ok, "sample appen failed")
	}

	// Check that truncate removes half of the chunks and afterwards
	// that the ID of the last chunk still gives us the same chunk afterwards.
	countBefore := len(s.chunks)
	lastID := s.chunkID(countBefore - 1)
	lastChunk := s.chunk(lastID)

	require.NotNil(t, s.chunk(0))
	require.NotNil(t, lastChunk)

	s.truncateChunksBefore(2000)

	require.Equal(t, int64(2000), s.chunks[0].minTime, "unexpected start time of first chunks")
	require.Nil(t, s.chunk(0), "first chunk not gone")
	require.Equal(t, countBefore/2, len(s.chunks), "chunks not truncated correctly")
	require.Equal(t, lastChunk, s.chunk(lastID), "last chunk does not match")

	// Validate that the series' sample buffer is applied correctly to the last chunk
	// after truncation.
	it1 := s.iterator(s.chunkID(len(s.chunks) - 1))
	_, ok := it1.(*memSafeIterator)
	require.True(t, ok, "last chunk not wrapped with sample buffer")

	it2 := s.iterator(s.chunkID(len(s.chunks) - 2))
	_, ok = it2.(*memSafeIterator)
	require.False(t, ok, "non-last chunk incorrectly wrapped with sample buffer")
}

func TestHeadDeleteSimple(t *testing.T) {
	numSamples := int64(10)

	head, err := NewHead(nil, nil, nil, 1000)
	require.NoError(t, err)

	app := head.Appender()

	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		app.Add(labels.Labels{{"a", "b"}}, i, smpls[i])
	}

	require.NoError(t, app.Commit())
	cases := []struct {
		intervals Intervals
		remaint   []int64
	}{
		{
			intervals: Intervals{{0, 3}},
			remaint:   []int64{4, 5, 6, 7, 8, 9},
		},
		{
			intervals: Intervals{{1, 3}},
			remaint:   []int64{0, 4, 5, 6, 7, 8, 9},
		},
		{
			intervals: Intervals{{1, 3}, {4, 7}},
			remaint:   []int64{0, 8, 9},
		},
		{
			intervals: Intervals{{1, 3}, {4, 700}},
			remaint:   []int64{0},
		},
		{
			intervals: Intervals{{0, 9}},
			remaint:   []int64{},
		},
	}

Outer:
	for _, c := range cases {
		// Reset the tombstones.
		head.tombstones = newEmptyTombstoneReader()

		// Delete the ranges.
		for _, r := range c.intervals {
			require.NoError(t, head.Delete(r.Mint, r.Maxt, labels.NewEqualMatcher("a", "b")))
		}

		// Compare the result.
		q := NewBlockQuerier(head.Index(), head.Chunks(), head.Tombstones(), head.MinTime(), head.MaxTime())
		res := q.Select(labels.NewEqualMatcher("a", "b"))

		expSamples := make([]sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{ts, smpls[ts]})
		}

		expss := newListSeriesSet([]Series{
			newSeries(map[string]string{"a": "b"}, expSamples),
		})

		if len(expSamples) == 0 {
			require.False(t, res.Next())
			continue
		}

		for {
			eok, rok := expss.Next(), res.Next()
			require.Equal(t, eok, rok, "next")

			if !eok {
				continue Outer
			}
			sexp := expss.At()
			sres := res.At()

			require.Equal(t, sexp.Labels(), sres.Labels(), "labels")

			smplExp, errExp := expandSeriesIterator(sexp.Iterator())
			smplRes, errRes := expandSeriesIterator(sres.Iterator())

			require.Equal(t, errExp, errRes, "samples error")
			require.Equal(t, smplExp, smplRes, "samples")
		}
	}
}

// func TestDeleteUntilCurMax(t *testing.T) {
// 	numSamples := int64(10)

// 	dir, _ := ioutil.TempDir("", "test")
// 	defer os.RemoveAll(dir)

// 	hb := createTestHead(t, dir, 0, 2*numSamples)
// 	app := hb.Appender()

// 	smpls := make([]float64, numSamples)
// 	for i := int64(0); i < numSamples; i++ {
// 		smpls[i] = rand.Float64()
// 		app.Add(labels.Labels{{"a", "b"}}, i, smpls[i])
// 	}

// 	require.NoError(t, app.Commit())
// 	require.NoError(t, hb.Delete(0, 10000, labels.NewEqualMatcher("a", "b")))
// 	app = hb.Appender()
// 	_, err := app.Add(labels.Labels{{"a", "b"}}, 11, 1)
// 	require.NoError(t, err)
// 	require.NoError(t, app.Commit())

// 	q := hb.Querier(0, 100000)
// 	res := q.Select(labels.NewEqualMatcher("a", "b"))

// 	require.True(t, res.Next())
// 	exps := res.At()
// 	it := exps.Iterator()
// 	ressmpls, err := expandSeriesIterator(it)
// 	require.NoError(t, err)
// 	require.Equal(t, []sample{{11, 1}}, ressmpls)
// }

// func TestDelete_e2e(t *testing.T) {
// 	numDatapoints := 1000
// 	numRanges := 1000
// 	timeInterval := int64(2)
// 	maxTime := int64(2 * 1000)
// 	minTime := int64(200)
// 	// Create 8 series with 1000 data-points of different ranges, delete and run queries.
// 	lbls := [][]labels.Label{
// 		{
// 			{"a", "b"},
// 			{"instance", "localhost:9090"},
// 			{"job", "prometheus"},
// 		},
// 		{
// 			{"a", "b"},
// 			{"instance", "127.0.0.1:9090"},
// 			{"job", "prometheus"},
// 		},
// 		{
// 			{"a", "b"},
// 			{"instance", "127.0.0.1:9090"},
// 			{"job", "prom-k8s"},
// 		},
// 		{
// 			{"a", "b"},
// 			{"instance", "localhost:9090"},
// 			{"job", "prom-k8s"},
// 		},
// 		{
// 			{"a", "c"},
// 			{"instance", "localhost:9090"},
// 			{"job", "prometheus"},
// 		},
// 		{
// 			{"a", "c"},
// 			{"instance", "127.0.0.1:9090"},
// 			{"job", "prometheus"},
// 		},
// 		{
// 			{"a", "c"},
// 			{"instance", "127.0.0.1:9090"},
// 			{"job", "prom-k8s"},
// 		},
// 		{
// 			{"a", "c"},
// 			{"instance", "localhost:9090"},
// 			{"job", "prom-k8s"},
// 		},
// 	}

// 	seriesMap := map[string][]sample{}
// 	for _, l := range lbls {
// 		seriesMap[labels.New(l...).String()] = []sample{}
// 	}

// 	dir, _ := ioutil.TempDir("", "test")
// 	defer os.RemoveAll(dir)

// 	hb := createTestHead(t, dir, minTime, maxTime)
// 	app := hb.Appender()

// 	for _, l := range lbls {
// 		ls := labels.New(l...)
// 		series := []sample{}

// 		ts := rand.Int63n(300)
// 		for i := 0; i < numDatapoints; i++ {
// 			v := rand.Float64()
// 			if ts >= minTime && ts <= maxTime {
// 				series = append(series, sample{ts, v})
// 			}

// 			_, err := app.Add(ls, ts, v)
// 			if ts >= minTime && ts <= maxTime {
// 				require.NoError(t, err)
// 			} else {
// 				require.EqualError(t, err, ErrOutOfBounds.Error())
// 			}

// 			ts += rand.Int63n(timeInterval) + 1
// 		}

// 		seriesMap[labels.New(l...).String()] = series
// 	}

// 	require.NoError(t, app.Commit())

// 	// Delete a time-range from each-selector.
// 	dels := []struct {
// 		ms     []labels.Matcher
// 		drange Intervals
// 	}{
// 		{
// 			ms:     []labels.Matcher{labels.NewEqualMatcher("a", "b")},
// 			drange: Intervals{{300, 500}, {600, 670}},
// 		},
// 		{
// 			ms: []labels.Matcher{
// 				labels.NewEqualMatcher("a", "b"),
// 				labels.NewEqualMatcher("job", "prom-k8s"),
// 			},
// 			drange: Intervals{{300, 500}, {100, 670}},
// 		},
// 		{
// 			ms: []labels.Matcher{
// 				labels.NewEqualMatcher("a", "c"),
// 				labels.NewEqualMatcher("instance", "localhost:9090"),
// 				labels.NewEqualMatcher("job", "prometheus"),
// 			},
// 			drange: Intervals{{300, 400}, {100, 6700}},
// 		},
// 		// TODO: Add Regexp Matchers.
// 	}

// 	for _, del := range dels {
// 		// Reset the deletes everytime.
// 		writeTombstoneFile(hb.dir, newEmptyTombstoneReader())
// 		hb.tombstones = newEmptyTombstoneReader()

// 		for _, r := range del.drange {
// 			require.NoError(t, hb.Delete(r.Mint, r.Maxt, del.ms...))
// 		}

// 		matched := labels.Slice{}
// 		for _, ls := range lbls {
// 			s := labels.Selector(del.ms)
// 			if s.Matches(ls) {
// 				matched = append(matched, ls)
// 			}
// 		}

// 		sort.Sort(matched)

// 		for i := 0; i < numRanges; i++ {
// 			mint := rand.Int63n(200)
// 			maxt := mint + rand.Int63n(timeInterval*int64(numDatapoints))

// 			q := hb.Querier(mint, maxt)
// 			ss := q.Select(del.ms...)

// 			// Build the mockSeriesSet.
// 			matchedSeries := make([]Series, 0, len(matched))
// 			for _, m := range matched {
// 				smpls := boundedSamples(seriesMap[m.String()], mint, maxt)
// 				smpls = deletedSamples(smpls, del.drange)

// 				// Only append those series for which samples exist as mockSeriesSet
// 				// doesn't skip series with no samples.
// 				// TODO: But sometimes SeriesSet returns an empty SeriesIterator
// 				if len(smpls) > 0 {
// 					matchedSeries = append(matchedSeries, newSeries(
// 						m.Map(),
// 						smpls,
// 					))
// 				}
// 			}
// 			expSs := newListSeriesSet(matchedSeries)

// 			// Compare both SeriesSets.
// 			for {
// 				eok, rok := expSs.Next(), ss.Next()

// 				// Skip a series if iterator is empty.
// 				if rok {
// 					for !ss.At().Iterator().Next() {
// 						rok = ss.Next()
// 						if !rok {
// 							break
// 						}
// 					}
// 				}
// 				require.Equal(t, eok, rok, "next")

// 				if !eok {
// 					break
// 				}
// 				sexp := expSs.At()
// 				sres := ss.At()

// 				require.Equal(t, sexp.Labels(), sres.Labels(), "labels")

// 				smplExp, errExp := expandSeriesIterator(sexp.Iterator())
// 				smplRes, errRes := expandSeriesIterator(sres.Iterator())

// 				require.Equal(t, errExp, errRes, "samples error")
// 				require.Equal(t, smplExp, smplRes, "samples")
// 			}
// 		}
// 	}

// 	return
// }

func boundedSamples(full []sample, mint, maxt int64) []sample {
	for len(full) > 0 {
		if full[0].t >= mint {
			break
		}
		full = full[1:]
	}
	for i, s := range full {
		// labels.Labelinate on the first sample larger than maxt.
		if s.t > maxt {
			return full[:i]
		}
	}
	// maxt is after highest sample.
	return full
}

func deletedSamples(full []sample, dranges Intervals) []sample {
	ds := make([]sample, 0, len(full))
Outer:
	for _, s := range full {
		for _, r := range dranges {
			if r.inBounds(s.t) {
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
		// Catch divison by zero for cur == start. Strictly not a possible case.
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
