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

package index

import (
	"container/heap"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
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

	for i := 0; i < 100; i++ {
		l := make([]storage.SeriesRef, 100)
		for j := range l {
			l[j] = storage.SeriesRef(rand.Uint64())
		}
		v := fmt.Sprintf("%d", i)

		p.m["a"][v] = l
	}

	p.EnsureOrder(0)

	for _, e := range p.m {
		for _, l := range e {
			ok := sort.SliceIsSorted(l, func(i, j int) bool {
				return l[i] < l[j]
			})
			if !ok {
				t.Fatalf("postings list %v is not sorted", l)
			}
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

			for n := 0; n < b.N; n++ {
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
			if c.res == nil {
				t.Fatal("intersect result expectancy cannot be nil")
			}

			expected, err := ExpandPostings(c.res)
			require.NoError(t, err)

			i := Intersect(c.in...)

			if c.res == EmptyPostings() {
				require.Equal(t, EmptyPostings(), i)
				return
			}

			if i == EmptyPostings() {
				t.Fatal("intersect unexpected result: EmptyPostings sentinel")
			}

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

func BenchmarkIntersect(t *testing.B) {
	t.Run("LongPostings1", func(bench *testing.B) {
		var a, b, c, d []storage.SeriesRef

		for i := 0; i < 10000000; i += 2 {
			a = append(a, storage.SeriesRef(i))
		}
		for i := 5000000; i < 5000100; i += 4 {
			b = append(b, storage.SeriesRef(i))
		}
		for i := 5090000; i < 5090600; i += 4 {
			b = append(b, storage.SeriesRef(i))
		}
		for i := 4990000; i < 5100000; i++ {
			c = append(c, storage.SeriesRef(i))
		}
		for i := 4000000; i < 6000000; i++ {
			d = append(d, storage.SeriesRef(i))
		}

		bench.ResetTimer()
		bench.ReportAllocs()
		for i := 0; i < bench.N; i++ {
			i1 := newListPostings(a...)
			i2 := newListPostings(b...)
			i3 := newListPostings(c...)
			i4 := newListPostings(d...)
			if err := consumePostings(Intersect(i1, i2, i3, i4)); err != nil {
				bench.Fatal(err)
			}
		}
	})

	t.Run("LongPostings2", func(bench *testing.B) {
		var a, b, c, d []storage.SeriesRef

		for i := 0; i < 12500000; i++ {
			a = append(a, storage.SeriesRef(i))
		}
		for i := 7500000; i < 12500000; i++ {
			b = append(b, storage.SeriesRef(i))
		}
		for i := 9000000; i < 20000000; i++ {
			c = append(c, storage.SeriesRef(i))
		}
		for i := 10000000; i < 12000000; i++ {
			d = append(d, storage.SeriesRef(i))
		}

		bench.ResetTimer()
		bench.ReportAllocs()
		for i := 0; i < bench.N; i++ {
			i1 := newListPostings(a...)
			i2 := newListPostings(b...)
			i3 := newListPostings(c...)
			i4 := newListPostings(d...)
			if err := consumePostings(Intersect(i1, i2, i3, i4)); err != nil {
				bench.Fatal(err)
			}
		}
	})

	// Many matchers(k >> n).
	t.Run("ManyPostings", func(bench *testing.B) {
		var lps []*ListPostings
		var refs [][]storage.SeriesRef

		// Create 100000 matchers(k=100000), making sure all memory allocation is done before starting the loop.
		for i := 0; i < 100000; i++ {
			var temp []storage.SeriesRef
			for j := storage.SeriesRef(1); j < 100; j++ {
				temp = append(temp, j)
			}
			lps = append(lps, newListPostings(temp...))
			refs = append(refs, temp)
		}

		its := make([]Postings, len(refs))
		bench.ResetTimer()
		bench.ReportAllocs()
		for i := 0; i < bench.N; i++ {
			// Reset the ListPostings to their original values each time round the loop.
			for j := range refs {
				lps[j].list = refs[j]
				its[j] = lps[j]
			}
			if err := consumePostings(Intersect(its...)); err != nil {
				bench.Fatal(err)
			}
		}
	})
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
			if c.res == nil {
				t.Fatal("merge result expectancy cannot be nil")
			}

			ctx := context.Background()

			expected, err := ExpandPostings(c.res)
			require.NoError(t, err)

			m := Merge(ctx, c.in...)

			if c.res == EmptyPostings() {
				require.Equal(t, EmptyPostings(), m)
				return
			}

			if m == EmptyPostings() {
				t.Fatal("merge unexpected result: EmptyPostings sentinel")
			}

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

	var i storage.SeriesRef
	for i = 0; i < 1e7; i++ {
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
	for i := 0; i < num; i++ {
		b := beLst[i*4 : i*4+4]
		binary.BigEndian.PutUint32(b, ls[i])
	}

	t.Run("Iteration", func(t *testing.T) {
		bep := newBigEndianPostings(beLst)
		for i := 0; i < num; i++ {
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
			if c.res == nil {
				t.Fatal("without result expectancy cannot be nil")
			}

			expected, err := ExpandPostings(c.res)
			require.NoError(t, err)

			w := Without(c.base, c.drop)

			if c.res == EmptyPostings() {
				require.Equal(t, EmptyPostings(), w)
				return
			}

			if w == EmptyPostings() {
				t.Fatal("without unexpected result: EmptyPostings sentinel")
			}

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
	for i := 0; i < 20; i++ {
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
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		p.Stats("__name__", 10)
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
	stats := p.Stats("label", 10)

	// assert that the expected statistics were calculated
	require.Equal(t, uint64(2), stats.CardinalityMetricsStats[0].Count)
	require.Equal(t, "value1", stats.CardinalityMetricsStats[0].Name)

	require.Equal(t, uint64(3), stats.CardinalityLabelStats[0].Count)
	require.Equal(t, "label", stats.CardinalityLabelStats[0].Name)

	require.Equal(t, uint64(24), stats.LabelValueStats[0].Count)
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

	before := p.Get(allPostingsKey.Name, allPostingsKey.Value)
	p.Delete(map[storage.SeriesRef]struct{}{
		2: {},
	})
	after := p.Get(allPostingsKey.Name, allPostingsKey.Value)

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

	deleted := p.Get("lbl1", "b")
	expanded, err = ExpandPostings(deleted)
	require.NoError(t, err)
	require.Equal(t, 0, len(expanded), "expected empty postings, got %v", expanded)
}

func TestPostingsCloner(t *testing.T) {
	for _, tc := range []struct {
		name  string
		check func(testing.TB, *PostingsCloner)
	}{
		{
			name: "seek beyond highest value of postings, then other clone seeks higher",
			check: func(t testing.TB, pc *PostingsCloner) {
				p1 := pc.Clone()
				require.False(t, p1.Seek(9))

				p2 := pc.Clone()
				require.False(t, p2.Seek(10))
			},
		},
		{
			name: "seek beyond highest value of postings, then other clone seeks lower",
			check: func(t testing.TB, pc *PostingsCloner) {
				p1 := pc.Clone()
				require.False(t, p1.Seek(9))

				p2 := pc.Clone()
				require.True(t, p2.Seek(2))
				require.Equal(t, storage.SeriesRef(2), p2.At())
			},
		},
		{
			name: "seek to posting with value 3 or higher",
			check: func(t testing.TB, pc *PostingsCloner) {
				p := pc.Clone()
				require.True(t, p.Seek(3))
				require.Equal(t, storage.SeriesRef(4), p.At())
				require.True(t, p.Seek(4))
				require.Equal(t, storage.SeriesRef(4), p.At())
			},
		},
		{
			name: "seek alternatively on different postings",
			check: func(t testing.TB, pc *PostingsCloner) {
				p1 := pc.Clone()
				require.True(t, p1.Seek(1))
				require.Equal(t, storage.SeriesRef(1), p1.At())

				p2 := pc.Clone()
				require.True(t, p2.Seek(2))
				require.Equal(t, storage.SeriesRef(2), p2.At())

				p3 := pc.Clone()
				require.True(t, p3.Seek(4))
				require.Equal(t, storage.SeriesRef(4), p3.At())

				p4 := pc.Clone()
				require.True(t, p4.Seek(5))
				require.Equal(t, storage.SeriesRef(8), p4.At())

				require.True(t, p1.Seek(3))
				require.Equal(t, storage.SeriesRef(4), p1.At())
				require.True(t, p1.Seek(4))
				require.Equal(t, storage.SeriesRef(4), p1.At())
			},
		},
		{
			name: "iterate through the postings",
			check: func(t testing.TB, pc *PostingsCloner) {
				p1 := pc.Clone()
				p2 := pc.Clone()

				// both one step
				require.True(t, p1.Next())
				require.Equal(t, storage.SeriesRef(1), p1.At())
				require.True(t, p2.Next())
				require.Equal(t, storage.SeriesRef(1), p2.At())

				require.True(t, p1.Next())
				require.Equal(t, storage.SeriesRef(2), p1.At())
				require.True(t, p1.Next())
				require.Equal(t, storage.SeriesRef(4), p1.At())
				require.True(t, p1.Next())
				require.Equal(t, storage.SeriesRef(8), p1.At())
				require.False(t, p1.Next())

				require.True(t, p2.Next())
				require.Equal(t, storage.SeriesRef(2), p2.At())
				require.True(t, p2.Next())
				require.Equal(t, storage.SeriesRef(4), p2.At())
			},
		},
		{
			name: "ensure a failed seek doesn't allow more next calls",
			check: func(t testing.TB, pc *PostingsCloner) {
				p := pc.Clone()
				require.False(t, p.Seek(9))
				require.False(t, p.Next())
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pc := NewPostingsCloner(newListPostings(1, 2, 4, 8))
			tc.check(t, pc)
		})
	}

	t.Run("cloning an err postings", func(t *testing.T) {
		expectedErr := fmt.Errorf("foobar")
		pc := NewPostingsCloner(ErrPostings(expectedErr))
		p := pc.Clone()
		require.False(t, p.Next())
		require.Equal(t, expectedErr, p.Err())
	})
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
		require.False(t, p.Next())
		require.False(t, p.Seek(10))
		require.False(t, p.Next())
		require.NoError(t, p.Err())
	})

	t.Run("one posting", func(t *testing.T) {
		t.Run("next", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10})
			require.True(t, p.Next())
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.False(t, p.Next())
			require.NoError(t, p.Err())
		})
		t.Run("seek less", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10})
			require.True(t, p.Seek(5))
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.True(t, p.Seek(5))
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.False(t, p.Next())
			require.NoError(t, p.Err())
		})
		t.Run("seek equal", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10})
			require.True(t, p.Seek(10))
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.False(t, p.Next())
			require.NoError(t, p.Err())
		})
		t.Run("seek more", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10})
			require.False(t, p.Seek(15))
			require.False(t, p.Next())
			require.NoError(t, p.Err())
		})
		t.Run("seek after next", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10})
			require.True(t, p.Next())
			require.False(t, p.Seek(15))
			require.False(t, p.Next())
			require.NoError(t, p.Err())
		})
	})

	t.Run("multiple postings", func(t *testing.T) {
		t.Run("next", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10, 20})
			require.True(t, p.Next())
			require.Equal(t, storage.SeriesRef(10), p.At())
			require.True(t, p.Next())
			require.Equal(t, storage.SeriesRef(20), p.At())
			require.False(t, p.Next())
			require.NoError(t, p.Err())
		})
		t.Run("seek", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10, 20})
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
		})
		t.Run("seek lest than last", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10, 20, 30, 40, 50})
			require.True(t, p.Seek(45))
			require.Equal(t, storage.SeriesRef(50), p.At())
			require.False(t, p.Next())
		})
		t.Run("seek exactly last", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10, 20, 30, 40, 50})
			require.True(t, p.Seek(50))
			require.Equal(t, storage.SeriesRef(50), p.At())
			require.False(t, p.Next())
		})
		t.Run("seek more than last", func(t *testing.T) {
			p := NewListPostings([]storage.SeriesRef{10, 20, 30, 40, 50})
			require.False(t, p.Seek(60))
			require.False(t, p.Next())
		})
	})

	t.Run("seek", func(t *testing.T) {
		for _, c := range []int{2, 8, 9, 10} {
			t.Run(fmt.Sprintf("count=%d", c), func(t *testing.T) {
				list := make([]storage.SeriesRef, c)
				for i := 0; i < c; i++ {
					list[i] = storage.SeriesRef(i * 10)
				}

				t.Run("all one by one", func(t *testing.T) {
					p := NewListPostings(list)
					for i := 0; i < c; i++ {
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
	for i := 0; i < maxCount; i++ {
		input[i] = storage.SeriesRef(i << 2)
	}

	for _, count := range []int{100, 1e3, 10e3, 100e3, maxCount} {
		b.Run(fmt.Sprintf("count=%d", count), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
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
