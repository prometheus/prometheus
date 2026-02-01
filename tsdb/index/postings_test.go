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

package index

import (
	"container/heap"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/regexp"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMemPostings_addFor(t *testing.T) {
	p := NewMemPostings()
	p.m[allPostingsKey.Name] = map[string][]storage.SeriesRef{}
	p.m[allPostingsKey.Name][allPostingsKey.Value] = []storage.SeriesRef{1, 2, 3, 4, 6, 7, 8}

	p.addFor(5, allPostingsKey)

	require.Equal(t, []storage.SeriesRef{1, 2, 3, 4, 5, 6, 7, 8}, p.m[allPostingsKey.Name][allPostingsKey.Value])
}

func TestMemPostings_ensureOrder(t *testing.T) {
	p := NewUnorderedMemPostings()
	p.m["a"] = map[string][]storage.SeriesRef{}

	for i := range 100 {
		l := make([]storage.SeriesRef, 100)
		for j := range l {
			l[j] = storage.SeriesRef(rand.Uint64())
		}
		v := strconv.Itoa(i)

		p.m["a"][v] = l
	}

	p.EnsureOrder(0)

	for _, e := range p.m {
		for _, l := range e {
			ok := slices.IsSorted(l)
			require.True(t, ok, "postings list %v is not sorted", l)
		}
	}
}

func BenchmarkMemPostings_ensureOrder(b *testing.B) {
	tests := map[string]struct {
		numLabels         int
		numValuesPerLabel int
		numRefsPerValue   int
	}{
		"many values per label": {
			numLabels:         100,
			numValuesPerLabel: 10000,
			numRefsPerValue:   100,
		},
		"few values per label": {
			numLabels:         1000000,
			numValuesPerLabel: 1,
			numRefsPerValue:   100,
		},
		"few refs per label value": {
			numLabels:         1000,
			numValuesPerLabel: 1000,
			numRefsPerValue:   10,
		},
	}

	for testName, testData := range tests {
		b.Run(testName, func(b *testing.B) {
			p := NewUnorderedMemPostings()

			// Generate postings.
			for l := 0; l < testData.numLabels; l++ {
				labelName := strconv.Itoa(l)
				p.m[labelName] = map[string][]storage.SeriesRef{}

				for v := 0; v < testData.numValuesPerLabel; v++ {
					refs := make([]storage.SeriesRef, testData.numRefsPerValue)
					for j := range refs {
						refs[j] = storage.SeriesRef(rand.Uint64())
					}

					labelValue := strconv.Itoa(v)
					p.m[labelName][labelValue] = refs
				}
			}

			b.ResetTimer()

			for b.Loop() {
				p.EnsureOrder(0)
				p.ordered = false
			}
		})
	}
}

func TestIntersect(t *testing.T) {
	a := newListPostings(1, 2, 3)
	b := newListPostings(2, 3, 4)

	cases := []struct {
		in []Postings

		res Postings
	}{
		{
			in:  []Postings{},
			res: EmptyPostings(),
		},
		{
			in:  []Postings{a, b, EmptyPostings()},
			res: EmptyPostings(),
		},
		{
			in:  []Postings{b, a, EmptyPostings()},
			res: EmptyPostings(),
		},
		{
			in:  []Postings{EmptyPostings(), b, a},
			res: EmptyPostings(),
		},
		{
			in:  []Postings{EmptyPostings(), a, b},
			res: EmptyPostings(),
		},
		{
			in:  []Postings{a, EmptyPostings(), b},
			res: EmptyPostings(),
		},
		{
			in:  []Postings{b, EmptyPostings(), a},
			res: EmptyPostings(),
		},
		{
			in:  []Postings{b, EmptyPostings(), a, a, b, a, a, a},
			res: EmptyPostings(),
		},
		{
			in: []Postings{
				newListPostings(1, 2, 3, 4, 5),
				newListPostings(6, 7, 8, 9, 10),
			},
			res: newListPostings(),
		},
		{
			in: []Postings{
				newListPostings(1, 2, 3, 4, 5),
				newListPostings(4, 5, 6, 7, 8),
			},
			res: newListPostings(4, 5),
		},
		{
			in: []Postings{
				newListPostings(1, 2, 3, 4, 9, 10),
				newListPostings(1, 4, 5, 6, 7, 8, 10, 11),
			},
			res: newListPostings(1, 4, 10),
		},
		{
			in: []Postings{
				newListPostings(1),
				newListPostings(0, 1),
			},
			res: newListPostings(1),
		},
		{
			in: []Postings{
				newListPostings(1),
			},
			res: newListPostings(1),
		},
		{
			in: []Postings{
				newListPostings(1),
				newListPostings(),
			},
			res: newListPostings(),
		},
		{
			in: []Postings{
				newListPostings(),
				newListPostings(),
			},
			res: newListPostings(),
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			require.NotNil(t, c.res, "intersect result expectancy cannot be nil")

			expected, err := ExpandPostings(c.res)
			require.NoError(t, err)

			i := Intersect(c.in...)

			if c.res == EmptyPostings() {
				require.Equal(t, EmptyPostings(), i)
				return
			}

			require.NotEqual(t, EmptyPostings(), i, "intersect unexpected result: EmptyPostings sentinel")

			res, err := ExpandPostings(i)
			require.NoError(t, err)
			require.Equal(t, expected, res)
		})
	}
}

func TestMultiIntersect(t *testing.T) {
	cases := []struct {
		p   [][]storage.SeriesRef
		res []storage.SeriesRef
	}{
		{
			p: [][]storage.SeriesRef{
				{1, 2, 3, 4, 5, 6, 1000, 1001},
				{2, 4, 5, 6, 7, 8, 999, 1001},
				{1, 2, 5, 6, 7, 8, 1001, 1200},
			},
			res: []storage.SeriesRef{2, 5, 6, 1001},
		},
		// One of the reproducible cases for:
		// https://github.com/prometheus/prometheus/issues/2616
		// The initialisation of intersectPostings was moving the iterator forward
		// prematurely making us miss some postings.
		{
			p: [][]storage.SeriesRef{
				{1, 2},
				{1, 2},
				{1, 2},
				{2},
			},
			res: []storage.SeriesRef{2},
		},
	}

	for _, c := range cases {
		ps := make([]Postings, 0, len(c.p))
		for _, postings := range c.p {
			ps = append(ps, newListPostings(postings...))
		}

		res, err := ExpandPostings(Intersect(ps...))

		require.NoError(t, err)
		require.Equal(t, c.res, res)
	}
}

func consumePostings(p Postings) error {
	for p.Next() {
		p.At()
	}
	return p.Err()
}

func newListPostings(list ...storage.SeriesRef) *listPostings {
	if !slices.IsSorted(list) {
		panic("newListPostings: list is not sorted")
	}
	return &listPostings{list: list}
}

// Create ListPostings for a benchmark, collecting the original sets of references
// so they can be reset without additional memory allocations.
func createPostings(lps *[]*listPostings, refs *[][]storage.SeriesRef, params ...storage.SeriesRef) {
	var temp []storage.SeriesRef
	for i := 0; i < len(params); i += 3 {
		for j := params[i]; j < params[i+1]; j += params[i+2] {
			temp = append(temp, j)
		}
	}
	*lps = append(*lps, newListPostings(temp...))
	*refs = append(*refs, temp)
}

// Reset the ListPostings to their original values each time round the benchmark loop.
func resetPostings(its []Postings, lps []*listPostings, refs [][]storage.SeriesRef) {
	for j := range refs {
		lps[j].list = refs[j]
		its[j] = lps[j]
	}
}

func BenchmarkIntersect(t *testing.B) {
	t.Run("LongPostings1", func(bench *testing.B) {
		var lps []*listPostings
		var refs [][]storage.SeriesRef
		createPostings(&lps, &refs, 0, 10000000, 2)
		createPostings(&lps, &refs, 5000000, 5000100, 4, 5090000, 5090600, 4)
		createPostings(&lps, &refs, 4990000, 5100000, 1)
		createPostings(&lps, &refs, 4000000, 6000000, 1)
		its := make([]Postings, len(refs))

		bench.ResetTimer()
		bench.ReportAllocs()
		for bench.Loop() {
			resetPostings(its, lps, refs)
			if err := consumePostings(Intersect(its...)); err != nil {
				bench.Fatal(err)
			}
		}
	})

	t.Run("LongPostings2", func(bench *testing.B) {
		var lps []*listPostings
		var refs [][]storage.SeriesRef
		createPostings(&lps, &refs, 0, 12500000, 1)
		createPostings(&lps, &refs, 7500000, 12500000, 1)
		createPostings(&lps, &refs, 9000000, 20000000, 1)
		createPostings(&lps, &refs, 10000000, 12000000, 1)
		its := make([]Postings, len(refs))

		bench.ResetTimer()
		bench.ReportAllocs()
		for bench.Loop() {
			resetPostings(its, lps, refs)
			if err := consumePostings(Intersect(its...)); err != nil {
				bench.Fatal(err)
			}
		}
	})

	t.Run("ManyPostings", func(bench *testing.B) {
		var lps []*listPostings
		var refs [][]storage.SeriesRef
		for range 100 {
			createPostings(&lps, &refs, 1, 100, 1)
		}

		its := make([]Postings, len(refs))
		bench.ResetTimer()
		bench.ReportAllocs()
		for bench.Loop() {
			resetPostings(its, lps, refs)
			if err := consumePostings(Intersect(its...)); err != nil {
				bench.Fatal(err)
			}
		}
	})
}

func BenchmarkMerge(t *testing.B) {
	var lps []*listPostings
	var refs [][]storage.SeriesRef

	// Create 100000 matchers(k=100000), making sure all memory allocation is done before starting the loop.
	for i := range 100000 {
		var temp []storage.SeriesRef
		for j := 1; j < 100; j++ {
			temp = append(temp, storage.SeriesRef(i+j*100000))
		}
		lps = append(lps, newListPostings(temp...))
		refs = append(refs, temp)
	}

	its := make([]*listPostings, len(refs))
	for _, nSeries := range []int{1, 10, 10000, 100000} {
		t.Run(strconv.Itoa(nSeries), func(bench *testing.B) {
			ctx := context.Background()
			for bench.Loop() {
				// Reset the ListPostings to their original values each time round the loop.
				for j := range refs[:nSeries] {
					lps[j].list = refs[j]
					its[j] = lps[j]
				}
				if err := consumePostings(Merge(ctx, its[:nSeries]...)); err != nil {
					bench.Fatal(err)
				}
			}
		})
	}
}

func TestMultiMerge(t *testing.T) {
	i1 := newListPostings(1, 2, 3, 4, 5, 6, 1000, 1001)
	i2 := newListPostings(2, 4, 5, 6, 7, 8, 999, 1001)
	i3 := newListPostings(1, 2, 5, 6, 7, 8, 1001, 1200)

	res, err := ExpandPostings(Merge(context.Background(), i1, i2, i3))
	require.NoError(t, err)
	require.Equal(t, []storage.SeriesRef{1, 2, 3, 4, 5, 6, 7, 8, 999, 1000, 1001, 1200}, res)
}

func TestMergedPostings(t *testing.T) {
	cases := []struct {
		in []Postings

		res Postings
	}{
		{
			in:  []Postings{},
			res: EmptyPostings(),
		},
		{
			in: []Postings{
				newListPostings(),
				newListPostings(),
			},
			res: EmptyPostings(),
		},
		{
			in: []Postings{
				newListPostings(),
			},
			res: newListPostings(),
		},
		{
			in: []Postings{
				EmptyPostings(),
				EmptyPostings(),
				EmptyPostings(),
				EmptyPostings(),
			},
			res: EmptyPostings(),
		},
		{
			in: []Postings{
				newListPostings(1, 2, 3, 4, 5),
				newListPostings(6, 7, 8, 9, 10),
			},
			res: newListPostings(1, 2, 3, 4, 5, 6, 7, 8, 9, 10),
		},
		{
			in: []Postings{
				newListPostings(1, 2, 3, 4, 5),
				newListPostings(4, 5, 6, 7, 8),
			},
			res: newListPostings(1, 2, 3, 4, 5, 6, 7, 8),
		},
		{
			in: []Postings{
				newListPostings(1, 2, 3, 4, 9, 10),
				newListPostings(1, 4, 5, 6, 7, 8, 10, 11),
			},
			res: newListPostings(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11),
		},
		{
			in: []Postings{
				newListPostings(1, 2, 3, 4, 9, 10),
				EmptyPostings(),
				newListPostings(1, 4, 5, 6, 7, 8, 10, 11),
			},
			res: newListPostings(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11),
		},
		{
			in: []Postings{
				newListPostings(1, 2),
				newListPostings(),
			},
			res: newListPostings(1, 2),
		},
		{
			in: []Postings{
				newListPostings(1, 2),
				EmptyPostings(),
			},
			res: newListPostings(1, 2),
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			require.NotNil(t, c.res, "merge result expectancy cannot be nil")

			ctx := context.Background()

			expected, err := ExpandPostings(c.res)
			require.NoError(t, err)

			m := Merge(ctx, c.in...)

			if c.res == EmptyPostings() {
				require.False(t, m.Next())
				return
			}

			require.NotEqual(t, EmptyPostings(), m, "merge unexpected result: EmptyPostings sentinel")

			res, err := ExpandPostings(m)
			require.NoError(t, err)
			require.Equal(t, expected, res)
		})
	}
}

func TestMergedPostingsSeek(t *testing.T) {
	cases := []struct {
		a, b []storage.SeriesRef

		seek    storage.SeriesRef
		success bool
		res     []storage.SeriesRef
	}{
		{
			a: []storage.SeriesRef{2, 3, 4, 5},
			b: []storage.SeriesRef{6, 7, 8, 9, 10},

			seek:    1,
			success: true,
			res:     []storage.SeriesRef{2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			a: []storage.SeriesRef{1, 2, 3, 4, 5},
			b: []storage.SeriesRef{6, 7, 8, 9, 10},

			seek:    2,
			success: true,
			res:     []storage.SeriesRef{2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			a: []storage.SeriesRef{1, 2, 3, 4, 5},
			b: []storage.SeriesRef{4, 5, 6, 7, 8},

			seek:    9,
			success: false,
			res:     nil,
		},
		{
			a: []storage.SeriesRef{1, 2, 3, 4, 9, 10},
			b: []storage.SeriesRef{1, 4, 5, 6, 7, 8, 10, 11},

			seek:    10,
			success: true,
			res:     []storage.SeriesRef{10, 11},
		},
	}

	for _, c := range cases {
		ctx := context.Background()

		a := newListPostings(c.a...)
		b := newListPostings(c.b...)

		p := Merge(ctx, a, b)

		require.Equal(t, c.success, p.Seek(c.seek))

		// After Seek(), At() should be called.
		if c.success {
			start := p.At()
			lst, err := ExpandPostings(p)
			require.NoError(t, err)

			lst = append([]storage.SeriesRef{start}, lst...)
			require.Equal(t, c.res, lst)
		}
	}
}

func TestRemovedPostings(t *testing.T) {
	cases := []struct {
		a, b []storage.SeriesRef
		res  []storage.SeriesRef
	}{
		{
			a:   nil,
			b:   nil,
			res: []storage.SeriesRef(nil),
		},
		{
			a:   []storage.SeriesRef{1, 2, 3, 4},
			b:   nil,
			res: []storage.SeriesRef{1, 2, 3, 4},
		},
		{
			a:   nil,
			b:   []storage.SeriesRef{1, 2, 3, 4},
			res: []storage.SeriesRef(nil),
		},
		{
			a:   []storage.SeriesRef{1, 2, 3, 4, 5},
			b:   []storage.SeriesRef{6, 7, 8, 9, 10},
			res: []storage.SeriesRef{1, 2, 3, 4, 5},
		},
		{
			a:   []storage.SeriesRef{1, 2, 3, 4, 5},
			b:   []storage.SeriesRef{4, 5, 6, 7, 8},
			res: []storage.SeriesRef{1, 2, 3},
		},
		{
			a:   []storage.SeriesRef{1, 2, 3, 4, 9, 10},
			b:   []storage.SeriesRef{1, 4, 5, 6, 7, 8, 10, 11},
			res: []storage.SeriesRef{2, 3, 9},
		},
		{
			a:   []storage.SeriesRef{1, 2, 3, 4, 9, 10},
			b:   []storage.SeriesRef{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
			res: []storage.SeriesRef(nil),
		},
	}

	for _, c := range cases {
		a := newListPostings(c.a...)
		b := newListPostings(c.b...)

		res, err := ExpandPostings(newRemovedPostings(a, b))
		require.NoError(t, err)
		require.Equal(t, c.res, res)
	}
}

func TestRemovedNextStackoverflow(t *testing.T) {
	var full []storage.SeriesRef
	var remove []storage.SeriesRef

	for i := range storage.SeriesRef(1e7) {
		full = append(full, i)
		remove = append(remove, i)
	}

	flp := newListPostings(full...)
	rlp := newListPostings(remove...)
	rp := newRemovedPostings(flp, rlp)
	gotElem := false
	for rp.Next() {
		gotElem = true
	}

	require.NoError(t, rp.Err())
	require.False(t, gotElem)
}

func TestRemovedPostingsSeek(t *testing.T) {
	cases := []struct {
		a, b []storage.SeriesRef

		seek    storage.SeriesRef
		success bool
		res     []storage.SeriesRef
	}{
		{
			a: []storage.SeriesRef{2, 3, 4, 5},
			b: []storage.SeriesRef{6, 7, 8, 9, 10},

			seek:    1,
			success: true,
			res:     []storage.SeriesRef{2, 3, 4, 5},
		},
		{
			a: []storage.SeriesRef{1, 2, 3, 4, 5},
			b: []storage.SeriesRef{6, 7, 8, 9, 10},

			seek:    2,
			success: true,
			res:     []storage.SeriesRef{2, 3, 4, 5},
		},
		{
			a: []storage.SeriesRef{1, 2, 3, 4, 5},
			b: []storage.SeriesRef{4, 5, 6, 7, 8},

			seek:    9,
			success: false,
			res:     nil,
		},
		{
			a: []storage.SeriesRef{1, 2, 3, 4, 9, 10},
			b: []storage.SeriesRef{1, 4, 5, 6, 7, 8, 10, 11},

			seek:    10,
			success: false,
			res:     nil,
		},
		{
			a: []storage.SeriesRef{1, 2, 3, 4, 9, 10},
			b: []storage.SeriesRef{1, 4, 5, 6, 7, 8, 11},

			seek:    4,
			success: true,
			res:     []storage.SeriesRef{9, 10},
		},
		{
			a: []storage.SeriesRef{1, 2, 3, 4, 9, 10},
			b: []storage.SeriesRef{1, 4, 5, 6, 7, 8, 11},

			seek:    5,
			success: true,
			res:     []storage.SeriesRef{9, 10},
		},
		{
			a: []storage.SeriesRef{1, 2, 3, 4, 9, 10},
			b: []storage.SeriesRef{1, 4, 5, 6, 7, 8, 11},

			seek:    10,
			success: true,
			res:     []storage.SeriesRef{10},
		},
	}

	for _, c := range cases {
		a := newListPostings(c.a...)
		b := newListPostings(c.b...)

		p := newRemovedPostings(a, b)

		require.Equal(t, c.success, p.Seek(c.seek))

		// After Seek(), At() should be called.
		if c.success {
			start := p.At()
			lst, err := ExpandPostings(p)
			require.NoError(t, err)

			lst = append([]storage.SeriesRef{start}, lst...)
			require.Equal(t, c.res, lst)
		}
	}
}

func TestBigEndian(t *testing.T) {
	num := 1000
	// mock a list as postings
	ls := make([]uint32, num)
	ls[0] = 2
	for i := 1; i < num; i++ {
		ls[i] = ls[i-1] + uint32(rand.Int31n(25)) + 2
	}

	beLst := make([]byte, num*4)
	for i := range num {
		b := beLst[i*4 : i*4+4]
		binary.BigEndian.PutUint32(b, ls[i])
	}

	t.Run("Iteration", func(t *testing.T) {
		bep := newBigEndianPostings(beLst)
		for i := range num {
			require.True(t, bep.Next())
			require.Equal(t, storage.SeriesRef(ls[i]), bep.At())
		}

		require.False(t, bep.Next())
		require.NoError(t, bep.Err())
	})

	t.Run("Seek", func(t *testing.T) {
		table := []struct {
			seek  uint32
			val   uint32
			found bool
		}{
			{
				ls[0] - 1, ls[0], true,
			},
			{
				ls[4], ls[4], true,
			},
			{
				ls[500] - 1, ls[500], true,
			},
			{
				ls[600] + 1, ls[601], true,
			},
			{
				ls[600] + 1, ls[601], true,
			},
			{
				ls[600] + 1, ls[601], true,
			},
			{
				ls[0], ls[601], true,
			},
			{
				ls[600], ls[601], true,
			},
			{
				ls[999], ls[999], true,
			},
			{
				ls[999] + 10, ls[999], false,
			},
		}

		bep := newBigEndianPostings(beLst)

		for _, v := range table {
			require.Equal(t, v.found, bep.Seek(storage.SeriesRef(v.seek)))
			require.Equal(t, storage.SeriesRef(v.val), bep.At())
			require.NoError(t, bep.Err())
		}
	})
}

func TestIntersectWithMerge(t *testing.T) {
	// One of the reproducible cases for:
	// https://github.com/prometheus/prometheus/issues/2616
	a := newListPostings(21, 22, 23, 24, 25, 30)

	b := Merge(
		context.Background(),
		newListPostings(10, 20, 30),
		newListPostings(15, 26, 30),
	)

	p := Intersect(a, b)
	res, err := ExpandPostings(p)

	require.NoError(t, err)
	require.Equal(t, []storage.SeriesRef{30}, res)
}

func TestWithoutPostings(t *testing.T) {
	cases := []struct {
		base Postings
		drop Postings

		res Postings
	}{
		{
			base: EmptyPostings(),
			drop: EmptyPostings(),

			res: EmptyPostings(),
		},
		{
			base: EmptyPostings(),
			drop: newListPostings(1, 2),

			res: EmptyPostings(),
		},
		{
			base: newListPostings(1, 2),
			drop: EmptyPostings(),

			res: newListPostings(1, 2),
		},
		{
			base: newListPostings(),
			drop: newListPostings(),

			res: newListPostings(),
		},
		{
			base: newListPostings(1, 2, 3),
			drop: newListPostings(),

			res: newListPostings(1, 2, 3),
		},
		{
			base: newListPostings(1, 2, 3),
			drop: newListPostings(4, 5, 6),

			res: newListPostings(1, 2, 3),
		},
		{
			base: newListPostings(1, 2, 3),
			drop: newListPostings(3, 4, 5),

			res: newListPostings(1, 2),
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			require.NotNil(t, c.res, "without result expectancy cannot be nil")

			expected, err := ExpandPostings(c.res)
			require.NoError(t, err)

			w := Without(c.base, c.drop)

			if c.res == EmptyPostings() {
				require.Equal(t, EmptyPostings(), w)
				return
			}

			require.NotEqual(t, EmptyPostings(), w, "without unexpected result: EmptyPostings sentinel")

			res, err := ExpandPostings(w)
			require.NoError(t, err)
			require.Equal(t, expected, res)
		})
	}
}

func BenchmarkPostings_Stats(b *testing.B) {
	p := NewMemPostings()

	var seriesID storage.SeriesRef

	createPostingsLabelValues := func(name, valuePrefix string, count int) {
		for n := 1; n < count; n++ {
			value := fmt.Sprintf("%s-%d", valuePrefix, n)
			p.Add(seriesID, labels.FromStrings(name, value))
			seriesID++
		}
	}
	createPostingsLabelValues("__name__", "metrics_name_can_be_very_big_and_bad", 1e3)
	for i := range 20 {
		createPostingsLabelValues(fmt.Sprintf("host-%d", i), "metrics_name_can_be_very_big_and_bad", 1e3)
		createPostingsLabelValues(fmt.Sprintf("instance-%d", i), "10.0.IP.", 1e3)
		createPostingsLabelValues(fmt.Sprintf("job-%d", i), "Small_Job_name", 1e3)
		createPostingsLabelValues(fmt.Sprintf("err-%d", i), "avg_namespace-", 1e3)
		createPostingsLabelValues(fmt.Sprintf("team-%d", i), "team-", 1e3)
		createPostingsLabelValues(fmt.Sprintf("container_name-%d", i), "pod-", 1e3)
		createPostingsLabelValues(fmt.Sprintf("cluster-%d", i), "newcluster-", 1e3)
		createPostingsLabelValues(fmt.Sprintf("uid-%d", i), "123412312312312311-", 1e3)
		createPostingsLabelValues(fmt.Sprintf("area-%d", i), "new_area_of_work-", 1e3)
		createPostingsLabelValues(fmt.Sprintf("request_id-%d", i), "owner_name_work-", 1e3)
	}

	for b.Loop() {
		p.Stats("__name__", 10, labels.SizeOfLabels)
	}
}

func TestMemPostingsStats(t *testing.T) {
	// create a new MemPostings
	p := NewMemPostings()

	// add some postings to the MemPostings
	p.Add(1, labels.FromStrings("label", "value1"))
	p.Add(1, labels.FromStrings("label", "value2"))
	p.Add(1, labels.FromStrings("label", "value3"))
	p.Add(2, labels.FromStrings("label", "value1"))

	// call the Stats method to calculate the cardinality statistics
	// passing a fake calculation so we get the same result regardless of compilation -tags.
	stats := p.Stats("label", 10, func(name, value string, n uint64) uint64 { return uint64(len(name)+len(value)) * n })

	// assert that the expected statistics were calculated
	require.Equal(t, uint64(2), stats.CardinalityMetricsStats[0].Count)
	require.Equal(t, "value1", stats.CardinalityMetricsStats[0].Name)

	require.Equal(t, uint64(3), stats.CardinalityLabelStats[0].Count)
	require.Equal(t, "label", stats.CardinalityLabelStats[0].Name)

	require.Equal(t, uint64(44), stats.LabelValueStats[0].Count)
	require.Equal(t, "label", stats.LabelValueStats[0].Name)

	require.Equal(t, uint64(2), stats.LabelValuePairsStats[0].Count)
	require.Equal(t, "label=value1", stats.LabelValuePairsStats[0].Name)

	require.Equal(t, 3, stats.NumLabelPairs)
}

func TestMemPostings_Delete(t *testing.T) {
	p := NewMemPostings()
	p.Add(1, labels.FromStrings("lbl1", "a"))
	p.Add(2, labels.FromStrings("lbl1", "b"))
	p.Add(3, labels.FromStrings("lbl2", "a"))

	before := p.Postings(context.Background(), allPostingsKey.Name, allPostingsKey.Value)
	deletedRefs := map[storage.SeriesRef]struct{}{
		2: {},
	}
	affectedLabels := map[labels.Label]struct{}{
		{Name: "lbl1", Value: "b"}: {},
	}
	p.Delete(deletedRefs, affectedLabels)
	after := p.Postings(context.Background(), allPostingsKey.Name, allPostingsKey.Value)

	// Make sure postings gotten before the delete have the old data when
	// iterated over.
	expanded, err := ExpandPostings(before)
	require.NoError(t, err)
	require.Equal(t, []storage.SeriesRef{1, 2, 3}, expanded)

	// Make sure postings gotten after the delete have the new data when
	// iterated over.
	expanded, err = ExpandPostings(after)
	require.NoError(t, err)
	require.Equal(t, []storage.SeriesRef{1, 3}, expanded)

	deleted := p.Postings(context.Background(), "lbl1", "b")
	expanded, err = ExpandPostings(deleted)
	require.NoError(t, err)
	require.Empty(t, expanded, "expected empty postings, got %v", expanded)
}

// BenchmarkMemPostings_Delete is quite heavy, so consider running it with
// -benchtime=10x or similar to get more stable and comparable results.
func BenchmarkMemPostings_Delete(b *testing.B) {
	internedItoa := map[int]string{}
	var mtx sync.RWMutex
	itoa := func(i int) string {
		mtx.RLock()
		s, ok := internedItoa[i]
		mtx.RUnlock()
		if ok {
			return s
		}
		mtx.Lock()
		s = strconv.Itoa(i)
		internedItoa[i] = s
		mtx.Unlock()
		return s
	}

	const total = 1e6
	allSeries := [total]labels.Labels{}
	nameValues := make([]string, 0, 100)
	for i := range int(total) {
		nameValues = nameValues[:0]

		// A thousand labels like lbl_x_of_1000, each with total/1000 values
		thousand := "lbl_" + itoa(i%1000) + "_of_1000"
		nameValues = append(nameValues, thousand, itoa(i/1000))
		// A hundred labels like lbl_x_of_100, each with total/100 values.
		hundred := "lbl_" + itoa(i%100) + "_of_100"
		nameValues = append(nameValues, hundred, itoa(i/100))

		if i < 100 {
			ten := "lbl_" + itoa(i%10) + "_of_10"
			nameValues = append(nameValues, ten, itoa(i%10))
		}
		allSeries[i] = labels.FromStrings(append(nameValues, "first", "a", "second", "a", "third", "a")...)
	}

	for _, refs := range []int{1, 100, 10_000} {
		b.Run(fmt.Sprintf("refs=%d", refs), func(b *testing.B) {
			for _, reads := range []int{0, 1, 10} {
				b.Run(fmt.Sprintf("readers=%d", reads), func(b *testing.B) {
					if b.N > total/refs {
						// Just to make sure that benchmark still makes sense.
						panic("benchmark not prepared")
					}

					p := NewMemPostings()
					for i := range allSeries {
						p.Add(storage.SeriesRef(i), allSeries[i])
					}

					stop := make(chan struct{})
					wg := sync.WaitGroup{}
					for i := range reads {
						wg.Add(1)
						go func(i int) {
							lbl := "lbl_" + itoa(i) + "_of_100"
							defer wg.Done()
							for {
								select {
								case <-stop:
									return
								default:
									// Get a random value of this label.
									p.Postings(context.Background(), lbl, itoa(rand.Intn(10000))).Next()
								}
							}
						}(i)
					}
					b.Cleanup(func() {
						close(stop)
						wg.Wait()
					})

					b.ResetTimer()
					for n := 0; b.Loop(); n++ {
						deleted := make(map[storage.SeriesRef]struct{}, refs)
						affected := make(map[labels.Label]struct{}, refs)
						for i := range refs {
							ref := storage.SeriesRef(n*refs + i)
							deleted[ref] = struct{}{}
							allSeries[ref].Range(func(l labels.Label) {
								affected[l] = struct{}{}
							})
						}
						p.Delete(deleted, affected)
					}
				})
			}
		})
	}
}

func TestFindIntersectingPostings(t *testing.T) {
	t.Run("multiple intersections", func(t *testing.T) {
		p := NewListPostings([]storage.SeriesRef{10, 15, 20, 25, 30, 35, 40, 45, 50})
		candidates := []Postings{
			0: NewListPostings([]storage.SeriesRef{7, 13, 14, 27}), // Does not intersect.
			1: NewListPostings([]storage.SeriesRef{10, 20}),        // Does intersect.
			2: NewListPostings([]storage.SeriesRef{29, 30, 31}),    // Does intersect.
			3: NewListPostings([]storage.SeriesRef{29, 30, 31}),    // Does intersect (same again).
			4: NewListPostings([]storage.SeriesRef{60}),            // Does not intersect.
			5: NewListPostings([]storage.SeriesRef{45}),            // Does intersect.
			6: EmptyPostings(),                                     // Does not intersect.
		}

		indexes, err := FindIntersectingPostings(p, candidates)
		require.NoError(t, err)
		sort.Ints(indexes)
		require.Equal(t, []int{1, 2, 3, 5}, indexes)
	})

	t.Run("no intersections", func(t *testing.T) {
		p := NewListPostings([]storage.SeriesRef{10, 15, 20, 25, 30, 35, 40, 45, 50})
		candidates := []Postings{
			0: NewListPostings([]storage.SeriesRef{7, 13, 14, 27}), // Does not intersect.
			1: NewListPostings([]storage.SeriesRef{60}),            // Does not intersect.
			2: EmptyPostings(),                                     // Does not intersect.
		}

		indexes, err := FindIntersectingPostings(p, candidates)
		require.NoError(t, err)
		require.Empty(t, indexes)
	})

	t.Run("p is ErrPostings", func(t *testing.T) {
		p := ErrPostings(context.Canceled)
		candidates := []Postings{NewListPostings([]storage.SeriesRef{1})}
		_, err := FindIntersectingPostings(p, candidates)
		require.Error(t, err)
	})

	t.Run("one of the candidates is ErrPostings", func(t *testing.T) {
		p := NewListPostings([]storage.SeriesRef{1})
		candidates := []Postings{
			NewListPostings([]storage.SeriesRef{1}),
			ErrPostings(context.Canceled),
		}
		_, err := FindIntersectingPostings(p, candidates)
		require.Error(t, err)
	})

	t.Run("one of the candidates fails on nth call", func(t *testing.T) {
		p := NewListPostings([]storage.SeriesRef{10, 20, 30, 40, 50, 60, 70})
		candidates := []Postings{
			NewListPostings([]storage.SeriesRef{7, 13, 14, 27}),
			&postingsFailingAfterNthCall{2, NewListPostings([]storage.SeriesRef{29, 30, 31, 40})},
		}
		_, err := FindIntersectingPostings(p, candidates)
		require.Error(t, err)
	})

	t.Run("p fails on the nth call", func(t *testing.T) {
		p := &postingsFailingAfterNthCall{2, NewListPostings([]storage.SeriesRef{10, 20, 30, 40, 50, 60, 70})}
		candidates := []Postings{
			NewListPostings([]storage.SeriesRef{7, 13, 14, 27}),
			NewListPostings([]storage.SeriesRef{29, 30, 31, 40}),
		}
		_, err := FindIntersectingPostings(p, candidates)
		require.Error(t, err)
	})
}

type postingsFailingAfterNthCall struct {
	ttl int
	Postings
}

func (p *postingsFailingAfterNthCall) Seek(v storage.SeriesRef) bool {
	p.ttl--
	if p.ttl <= 0 {
		return false
	}
	return p.Postings.Seek(v)
}

func (p *postingsFailingAfterNthCall) Next() bool {
	p.ttl--
	if p.ttl <= 0 {
		return false
	}
	return p.Postings.Next()
}

func (p *postingsFailingAfterNthCall) Err() error {
	if p.ttl <= 0 {
		return errors.New("ttl exceeded")
	}
	return p.Postings.Err()
}

func TestPostingsWithIndexHeap(t *testing.T) {
	t.Run("iterate", func(t *testing.T) {
		h := postingsWithIndexHeap{
			{index: 0, p: NewListPostings([]storage.SeriesRef{10, 20, 30})},
			{index: 1, p: NewListPostings([]storage.SeriesRef{1, 5})},
			{index: 2, p: NewListPostings([]storage.SeriesRef{25, 50})},
		}
		for _, node := range h {
			node.p.Next()
		}
		heap.Init(&h)

		for _, expected := range []storage.SeriesRef{1, 5, 10, 20, 25, 30, 50} {
			require.Equal(t, expected, h.at())
			require.NoError(t, h.next())
		}
		require.True(t, h.empty())
	})

	t.Run("pop", func(t *testing.T) {
		h := postingsWithIndexHeap{
			{index: 0, p: NewListPostings([]storage.SeriesRef{10, 20, 30})},
			{index: 1, p: NewListPostings([]storage.SeriesRef{1, 5})},
			{index: 2, p: NewListPostings([]storage.SeriesRef{25, 50})},
		}
		for _, node := range h {
			node.p.Next()
		}
		heap.Init(&h)

		for _, expected := range []storage.SeriesRef{1, 5, 10, 20} {
			require.Equal(t, expected, h.at())
			require.NoError(t, h.next())
		}
		require.Equal(t, storage.SeriesRef(25), h.at())
		node := heap.Pop(&h).(postingsWithIndex)
		require.Equal(t, 2, node.index)
		require.Equal(t, storage.SeriesRef(25), node.p.At())
	})
}

func TestListPostings(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		p := NewListPostings(nil)
		require.Equal(t, 0, p.(*listPostings).Len())
		require.False(t, p.Next())
		require.False(t, p.Seek(10))
		require.False(t, p.Next())
		require.NoError(t, p.Err())
		require.Equal(t, 0, p.(*listPostings).Len())
	})

	t.Run("one posting", func(t *testing.T) {
		t.Run("next", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10})
			require.Equal(t, 1, p.(*listPostings).Len())
			require.True(t, p.Next())
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.False(t, p.Next())
			require.NoError(t, p.Err())
			require.Equal(t, 0, p.(*listPostings).Len())
		})
		t.Run("seek less", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10})
			require.Equal(t, 1, p.(*listPostings).Len())
			require.True(t, p.Seek(5))
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.True(t, p.Seek(5))
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.False(t, p.Next())
			require.NoError(t, p.Err())
			require.Equal(t, 0, p.(*listPostings).Len())
		})
		t.Run("seek equal", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10})
			require.Equal(t, 1, p.(*listPostings).Len())
			require.True(t, p.Seek(10))
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.False(t, p.Next())
			require.NoError(t, p.Err())
			require.Equal(t, 0, p.(*listPostings).Len())
		})
		t.Run("seek more", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10})
			require.Equal(t, 1, p.(*listPostings).Len())
			require.False(t, p.Seek(15))
			require.False(t, p.Next())
			require.NoError(t, p.Err())
			require.Equal(t, 0, p.(*listPostings).Len())
		})
		t.Run("seek after next", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10})
			require.Equal(t, 1, p.(*listPostings).Len())
			require.True(t, p.Next())
			require.False(t, p.Seek(15))
			require.False(t, p.Next())
			require.NoError(t, p.Err())
			require.Equal(t, 0, p.(*listPostings).Len())
		})
	})

	t.Run("multiple postings", func(t *testing.T) {
		t.Run("next", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10, 20})
			require.Equal(t, 2, p.(*listPostings).Len())
			require.True(t, p.Next())
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.True(t, p.Next())
			require.Equal(t, storage.SeriesRef(20), p.At())
			require.False(t, p.Next())
			require.NoError(t, p.Err())
			require.Equal(t, 0, p.(*listPostings).Len())
		})
		t.Run("seek", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10, 20})
			require.Equal(t, 2, p.(*listPostings).Len())
			require.True(t, p.Seek(5))
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.True(t, p.Seek(5))
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.True(t, p.Seek(10))
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.True(t, p.Next())
			require.Equal(t, storage.SeriesRef(20), p.At())
			require.True(t, p.Seek(10))
			require.Equal(t, storage.SeriesRef(20), p.At())
			require.True(t, p.Seek(20))
			require.Equal(t, storage.SeriesRef(20), p.At())
			require.False(t, p.Next())
			require.NoError(t, p.Err())
			require.Equal(t, 0, p.(*listPostings).Len())
		})
		t.Run("seek lest than last", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10, 20, 30, 40, 50})
			require.Equal(t, 5, p.(*listPostings).Len())
			require.True(t, p.Seek(45))
			require.Equal(t, storage.SeriesRef(50), p.At())
			require.False(t, p.Next())
			require.Equal(t, 0, p.(*listPostings).Len())
		})
		t.Run("seek exactly last", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10, 20, 30, 40, 50})
			require.Equal(t, 5, p.(*listPostings).Len())
			require.True(t, p.Seek(50))
			require.Equal(t, storage.SeriesRef(50), p.At())
			require.False(t, p.Next())
			require.Equal(t, 0, p.(*listPostings).Len())
		})
		t.Run("seek more than last", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10, 20, 30, 40, 50})
			require.Equal(t, 5, p.(*listPostings).Len())
			require.False(t, p.Seek(60))
			require.False(t, p.Next())
			require.Equal(t, 0, p.(*listPostings).Len())
		})
	})

	t.Run("seek", func(t *testing.T) {
		for _, c := range []int{2, 8, 9, 10} {
			t.Run(fmt.Sprintf("count=%d", c), func(t *testing.T) {
				list := make([]storage.SeriesRef, c)
				for i := range c {
					list[i] = storage.SeriesRef(i * 10)
				}

				t.Run("all one by one", func(t *testing.T) {
					p := NewListPostings(list)
					for i := range c {
						require.True(t, p.Seek(storage.SeriesRef(i*10)))
						require.Equal(t, storage.SeriesRef(i*10), p.At())
					}
					require.False(t, p.Seek(storage.SeriesRef(c*10)))
				})

				t.Run("each one", func(t *testing.T) {
					for _, ref := range list {
						p := NewListPostings(list)
						require.True(t, p.Seek(ref))
						require.Equal(t, ref, p.At())
					}
				})
			})
		}
	})
}

// BenchmarkListPostings benchmarks ListPostings by iterating Next/At sequentially.
// See also BenchmarkIntersect as it performs more `At` calls than `Next` calls when intersecting.
func BenchmarkListPostings(b *testing.B) {
	const maxCount = 1e6
	input := make([]storage.SeriesRef, maxCount)
	for i := range int(maxCount) {
		input[i] = storage.SeriesRef(i << 2)
	}

	for _, count := range []int{100, 1e3, 10e3, 100e3, maxCount} {
		b.Run(fmt.Sprintf("count=%d", count), func(b *testing.B) {
			for b.Loop() {
				p := NewListPostings(input[:count])
				var sum storage.SeriesRef
				for p.Next() {
					sum += p.At()
				}
				require.NotZero(b, sum)
			}
		})
	}
}

func slowRegexpString() string {
	nums := map[int]struct{}{}
	for i := 10_000; i < 20_000; i++ {
		if i%3 == 0 {
			nums[i] = struct{}{}
		}
	}

	var sb strings.Builder
	sb.WriteString(".*(9999")
	for i := range nums {
		sb.WriteString("|")
		sb.WriteString(strconv.Itoa(i))
	}
	sb.WriteString(").*")
	return sb.String()
}

func BenchmarkMemPostings_PostingsForLabelMatching(b *testing.B) {
	fast := regexp.MustCompile("^(100|200)$")
	slowRegexp := "^" + slowRegexpString() + "$"
	b.Logf("Slow regexp length = %d", len(slowRegexp))
	slow := regexp.MustCompile(slowRegexp)
	const seriesPerLabel = 10

	for _, labelValueCount := range []int{1_000, 10_000, 100_000} {
		b.Run(fmt.Sprintf("labels=%d", labelValueCount), func(b *testing.B) {
			mp := NewMemPostings()
			for i := range labelValueCount {
				for j := range seriesPerLabel {
					mp.Add(storage.SeriesRef(i*seriesPerLabel+j), labels.FromStrings("__name__", strconv.Itoa(j), "label", strconv.Itoa(i)))
				}
			}

			fp, err := ExpandPostings(mp.PostingsForLabelMatching(context.Background(), "label", fast.MatchString))
			require.NoError(b, err)
			b.Logf("Fast matcher matches %d series", len(fp))
			b.Run("matcher=fast", func(b *testing.B) {
				for b.Loop() {
					mp.PostingsForLabelMatching(context.Background(), "label", fast.MatchString).Next()
				}
			})

			sp, err := ExpandPostings(mp.PostingsForLabelMatching(context.Background(), "label", slow.MatchString))
			require.NoError(b, err)
			b.Logf("Slow matcher matches %d series", len(sp))
			b.Run("matcher=slow", func(b *testing.B) {
				for b.Loop() {
					mp.PostingsForLabelMatching(context.Background(), "label", slow.MatchString).Next()
				}
			})

			b.Run("matcher=all", func(b *testing.B) {
				for b.Loop() {
					// Match everything.
					p := mp.PostingsForLabelMatching(context.Background(), "label", func(_ string) bool { return true })
					var sum storage.SeriesRef
					// Iterate through all results to exercise merge function.
					for p.Next() {
						sum += p.At()
					}
				}
			})
		})
	}
}

func TestMemPostings_PostingsForLabelMatching(t *testing.T) {
	mp := NewMemPostings()
	mp.Add(1, labels.FromStrings("foo", "1"))
	mp.Add(2, labels.FromStrings("foo", "2"))
	mp.Add(3, labels.FromStrings("foo", "3"))
	mp.Add(4, labels.FromStrings("foo", "4"))

	isEven := func(v string) bool {
		iv, err := strconv.Atoi(v)
		if err != nil {
			panic(err)
		}
		return iv%2 == 0
	}

	p := mp.PostingsForLabelMatching(context.Background(), "foo", isEven)
	require.NoError(t, p.Err())
	refs, err := ExpandPostings(p)
	require.NoError(t, err)
	require.Equal(t, []storage.SeriesRef{2, 4}, refs)
}

func TestMemPostings_PostingsForAllLabelValues(t *testing.T) {
	mp := NewMemPostings()
	mp.Add(1, labels.FromStrings("foo", "1"))
	mp.Add(2, labels.FromStrings("foo", "2"))
	mp.Add(3, labels.FromStrings("foo", "3"))
	mp.Add(4, labels.FromStrings("foo", "4"))

	p := mp.PostingsForAllLabelValues(context.Background(), "foo")
	require.NoError(t, p.Err())
	refs, err := ExpandPostings(p)
	require.NoError(t, err)
	// All postings for the label should be returned.
	require.Equal(t, []storage.SeriesRef{1, 2, 3, 4}, refs)
}

func TestMemPostings_PostingsForLabelMatchingHonorsContextCancel(t *testing.T) {
	memP := NewMemPostings()
	seriesCount := 10 * checkContextEveryNIterations
	for i := 1; i <= seriesCount; i++ {
		memP.Add(storage.SeriesRef(i), labels.FromStrings("__name__", fmt.Sprintf("%4d", i)))
	}

	failAfter := uint64(seriesCount / 2 / checkContextEveryNIterations)
	ctx := &testutil.MockContextErrAfter{FailAfter: failAfter}
	p := memP.PostingsForLabelMatching(ctx, "__name__", func(string) bool {
		return true
	})
	require.Error(t, p.Err())
	require.Equal(t, failAfter+1, ctx.Count()) // Plus one for the Err() call that puts the error in the result.
}
