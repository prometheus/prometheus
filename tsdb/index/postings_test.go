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
	"encoding/binary"
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/prometheus/prometheus/tsdb/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMemPostings_addFor(t *testing.T) {
	p := NewMemPostings()
	p.m[allPostingsKey.Name] = map[string][]uint64{}
	p.m[allPostingsKey.Name][allPostingsKey.Value] = []uint64{1, 2, 3, 4, 6, 7, 8}

	p.addFor(5, allPostingsKey)

	testutil.Equals(t, []uint64{1, 2, 3, 4, 5, 6, 7, 8}, p.m[allPostingsKey.Name][allPostingsKey.Value])
}

func TestMemPostings_ensureOrder(t *testing.T) {
	p := NewUnorderedMemPostings()
	p.m["a"] = map[string][]uint64{}

	for i := 0; i < 100; i++ {
		l := make([]uint64, 100)
		for j := range l {
			l[j] = rand.Uint64()
		}
		v := fmt.Sprintf("%d", i)

		p.m["a"][v] = l
	}

	p.EnsureOrder()

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

func TestIntersect(t *testing.T) {
	a := newListPostings(1, 2, 3)
	b := newListPostings(2, 3, 4)

	var cases = []struct {
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
			testutil.Ok(t, err)

			i := Intersect(c.in...)

			if c.res == EmptyPostings() {
				testutil.Equals(t, EmptyPostings(), i)
				return
			}

			if i == EmptyPostings() {
				t.Fatal("intersect unexpected result: EmptyPostings sentinel")
			}

			res, err := ExpandPostings(i)
			testutil.Ok(t, err)
			testutil.Equals(t, expected, res)
		})
	}
}

func TestMultiIntersect(t *testing.T) {
	var cases = []struct {
		p   [][]uint64
		res []uint64
	}{
		{
			p: [][]uint64{
				{1, 2, 3, 4, 5, 6, 1000, 1001},
				{2, 4, 5, 6, 7, 8, 999, 1001},
				{1, 2, 5, 6, 7, 8, 1001, 1200},
			},
			res: []uint64{2, 5, 6, 1001},
		},
		// One of the reproduceable cases for:
		// https://github.com/prometheus/prometheus/issues/2616
		// The initialisation of intersectPostings was moving the iterator forward
		// prematurely making us miss some postings.
		{
			p: [][]uint64{
				{1, 2},
				{1, 2},
				{1, 2},
				{2},
			},
			res: []uint64{2},
		},
	}

	for _, c := range cases {
		ps := make([]Postings, 0, len(c.p))
		for _, postings := range c.p {
			ps = append(ps, newListPostings(postings...))
		}

		res, err := ExpandPostings(Intersect(ps...))

		testutil.Ok(t, err)
		testutil.Equals(t, c.res, res)
	}
}

func BenchmarkIntersect(t *testing.B) {
	t.Run("LongPostings1", func(bench *testing.B) {
		var a, b, c, d []uint64

		for i := 0; i < 10000000; i += 2 {
			a = append(a, uint64(i))
		}
		for i := 5000000; i < 5000100; i += 4 {
			b = append(b, uint64(i))
		}
		for i := 5090000; i < 5090600; i += 4 {
			b = append(b, uint64(i))
		}
		for i := 4990000; i < 5100000; i++ {
			c = append(c, uint64(i))
		}
		for i := 4000000; i < 6000000; i++ {
			d = append(d, uint64(i))
		}

		i1 := newListPostings(a...)
		i2 := newListPostings(b...)
		i3 := newListPostings(c...)
		i4 := newListPostings(d...)

		bench.ResetTimer()
		bench.ReportAllocs()
		for i := 0; i < bench.N; i++ {
			if _, err := ExpandPostings(Intersect(i1, i2, i3, i4)); err != nil {
				bench.Fatal(err)
			}
		}
	})

	t.Run("LongPostings2", func(bench *testing.B) {
		var a, b, c, d []uint64

		for i := 0; i < 12500000; i++ {
			a = append(a, uint64(i))
		}
		for i := 7500000; i < 12500000; i++ {
			b = append(b, uint64(i))
		}
		for i := 9000000; i < 20000000; i++ {
			c = append(c, uint64(i))
		}
		for i := 10000000; i < 12000000; i++ {
			d = append(d, uint64(i))
		}

		i1 := newListPostings(a...)
		i2 := newListPostings(b...)
		i3 := newListPostings(c...)
		i4 := newListPostings(d...)

		bench.ResetTimer()
		bench.ReportAllocs()
		for i := 0; i < bench.N; i++ {
			if _, err := ExpandPostings(Intersect(i1, i2, i3, i4)); err != nil {
				bench.Fatal(err)
			}
		}
	})

	// Many matchers(k >> n).
	t.Run("ManyPostings", func(bench *testing.B) {
		var its []Postings

		// 100000 matchers(k=100000).
		for i := 0; i < 100000; i++ {
			var temp []uint64
			for j := 1; j < 100; j++ {
				temp = append(temp, uint64(j))
			}
			its = append(its, newListPostings(temp...))
		}

		bench.ResetTimer()
		bench.ReportAllocs()
		for i := 0; i < bench.N; i++ {
			if _, err := ExpandPostings(Intersect(its...)); err != nil {
				bench.Fatal(err)
			}
		}
	})
}

func TestMultiMerge(t *testing.T) {
	i1 := newListPostings(1, 2, 3, 4, 5, 6, 1000, 1001)
	i2 := newListPostings(2, 4, 5, 6, 7, 8, 999, 1001)
	i3 := newListPostings(1, 2, 5, 6, 7, 8, 1001, 1200)

	res, err := ExpandPostings(Merge(i1, i2, i3))
	testutil.Ok(t, err)
	testutil.Equals(t, []uint64{1, 2, 3, 4, 5, 6, 7, 8, 999, 1000, 1001, 1200}, res)
}

func TestMergedPostings(t *testing.T) {
	var cases = []struct {
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

			expected, err := ExpandPostings(c.res)
			testutil.Ok(t, err)

			m := Merge(c.in...)

			if c.res == EmptyPostings() {
				testutil.Equals(t, EmptyPostings(), m)
				return
			}

			if m == EmptyPostings() {
				t.Fatal("merge unexpected result: EmptyPostings sentinel")
			}

			res, err := ExpandPostings(m)
			testutil.Ok(t, err)
			testutil.Equals(t, expected, res)
		})
	}
}

func TestMergedPostingsSeek(t *testing.T) {
	var cases = []struct {
		a, b []uint64

		seek    uint64
		success bool
		res     []uint64
	}{
		{
			a: []uint64{2, 3, 4, 5},
			b: []uint64{6, 7, 8, 9, 10},

			seek:    1,
			success: true,
			res:     []uint64{2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			a: []uint64{1, 2, 3, 4, 5},
			b: []uint64{6, 7, 8, 9, 10},

			seek:    2,
			success: true,
			res:     []uint64{2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			a: []uint64{1, 2, 3, 4, 5},
			b: []uint64{4, 5, 6, 7, 8},

			seek:    9,
			success: false,
			res:     nil,
		},
		{
			a: []uint64{1, 2, 3, 4, 9, 10},
			b: []uint64{1, 4, 5, 6, 7, 8, 10, 11},

			seek:    10,
			success: true,
			res:     []uint64{10, 11},
		},
	}

	for _, c := range cases {
		a := newListPostings(c.a...)
		b := newListPostings(c.b...)

		p := Merge(a, b)

		testutil.Equals(t, c.success, p.Seek(c.seek))

		// After Seek(), At() should be called.
		if c.success {
			start := p.At()
			lst, err := ExpandPostings(p)
			testutil.Ok(t, err)

			lst = append([]uint64{start}, lst...)
			testutil.Equals(t, c.res, lst)
		}
	}
}

func TestRemovedPostings(t *testing.T) {
	var cases = []struct {
		a, b []uint64
		res  []uint64
	}{
		{
			a:   nil,
			b:   nil,
			res: []uint64(nil),
		},
		{
			a:   []uint64{1, 2, 3, 4},
			b:   nil,
			res: []uint64{1, 2, 3, 4},
		},
		{
			a:   nil,
			b:   []uint64{1, 2, 3, 4},
			res: []uint64(nil),
		},
		{
			a:   []uint64{1, 2, 3, 4, 5},
			b:   []uint64{6, 7, 8, 9, 10},
			res: []uint64{1, 2, 3, 4, 5},
		},
		{
			a:   []uint64{1, 2, 3, 4, 5},
			b:   []uint64{4, 5, 6, 7, 8},
			res: []uint64{1, 2, 3},
		},
		{
			a:   []uint64{1, 2, 3, 4, 9, 10},
			b:   []uint64{1, 4, 5, 6, 7, 8, 10, 11},
			res: []uint64{2, 3, 9},
		},
		{
			a:   []uint64{1, 2, 3, 4, 9, 10},
			b:   []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
			res: []uint64(nil),
		},
	}

	for _, c := range cases {
		a := newListPostings(c.a...)
		b := newListPostings(c.b...)

		res, err := ExpandPostings(newRemovedPostings(a, b))
		testutil.Ok(t, err)
		testutil.Equals(t, c.res, res)
	}

}

func TestRemovedNextStackoverflow(t *testing.T) {
	var full []uint64
	var remove []uint64

	var i uint64
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

	testutil.Ok(t, rp.Err())
	testutil.Assert(t, !gotElem, "")
}

func TestRemovedPostingsSeek(t *testing.T) {
	var cases = []struct {
		a, b []uint64

		seek    uint64
		success bool
		res     []uint64
	}{
		{
			a: []uint64{2, 3, 4, 5},
			b: []uint64{6, 7, 8, 9, 10},

			seek:    1,
			success: true,
			res:     []uint64{2, 3, 4, 5},
		},
		{
			a: []uint64{1, 2, 3, 4, 5},
			b: []uint64{6, 7, 8, 9, 10},

			seek:    2,
			success: true,
			res:     []uint64{2, 3, 4, 5},
		},
		{
			a: []uint64{1, 2, 3, 4, 5},
			b: []uint64{4, 5, 6, 7, 8},

			seek:    9,
			success: false,
			res:     nil,
		},
		{
			a: []uint64{1, 2, 3, 4, 9, 10},
			b: []uint64{1, 4, 5, 6, 7, 8, 10, 11},

			seek:    10,
			success: false,
			res:     nil,
		},
		{
			a: []uint64{1, 2, 3, 4, 9, 10},
			b: []uint64{1, 4, 5, 6, 7, 8, 11},

			seek:    4,
			success: true,
			res:     []uint64{9, 10},
		},
		{
			a: []uint64{1, 2, 3, 4, 9, 10},
			b: []uint64{1, 4, 5, 6, 7, 8, 11},

			seek:    5,
			success: true,
			res:     []uint64{9, 10},
		},
		{
			a: []uint64{1, 2, 3, 4, 9, 10},
			b: []uint64{1, 4, 5, 6, 7, 8, 11},

			seek:    10,
			success: true,
			res:     []uint64{10},
		},
	}

	for _, c := range cases {
		a := newListPostings(c.a...)
		b := newListPostings(c.b...)

		p := newRemovedPostings(a, b)

		testutil.Equals(t, c.success, p.Seek(c.seek))

		// After Seek(), At() should be called.
		if c.success {
			start := p.At()
			lst, err := ExpandPostings(p)
			testutil.Ok(t, err)

			lst = append([]uint64{start}, lst...)
			testutil.Equals(t, c.res, lst)
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
			testutil.Assert(t, bep.Next() == true, "")
			testutil.Equals(t, uint64(ls[i]), bep.At())
		}

		testutil.Assert(t, bep.Next() == false, "")
		testutil.Assert(t, bep.Err() == nil, "")
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
			testutil.Equals(t, v.found, bep.Seek(uint64(v.seek)))
			testutil.Equals(t, uint64(v.val), bep.At())
			testutil.Assert(t, bep.Err() == nil, "")
		}
	})
}

func TestIntersectWithMerge(t *testing.T) {
	// One of the reproducible cases for:
	// https://github.com/prometheus/prometheus/issues/2616
	a := newListPostings(21, 22, 23, 24, 25, 30)

	b := Merge(
		newListPostings(10, 20, 30),
		newListPostings(15, 26, 30),
	)

	p := Intersect(a, b)
	res, err := ExpandPostings(p)

	testutil.Ok(t, err)
	testutil.Equals(t, []uint64{30}, res)
}

func TestWithoutPostings(t *testing.T) {
	var cases = []struct {
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
			testutil.Ok(t, err)

			w := Without(c.base, c.drop)

			if c.res == EmptyPostings() {
				testutil.Equals(t, EmptyPostings(), w)
				return
			}

			if w == EmptyPostings() {
				t.Fatal("without unexpected result: EmptyPostings sentinel")
			}

			res, err := ExpandPostings(w)
			testutil.Ok(t, err)
			testutil.Equals(t, expected, res)
		})
	}
}

func TestMemPostings_Delete(t *testing.T) {
	p := NewMemPostings()
	p.Add(1, labels.FromStrings("lbl1", "a"))
	p.Add(2, labels.FromStrings("lbl1", "b"))
	p.Add(3, labels.FromStrings("lbl2", "a"))

	before := p.Get(allPostingsKey.Name, allPostingsKey.Value)
	p.Delete(map[uint64]struct{}{
		2: struct{}{},
	})
	after := p.Get(allPostingsKey.Name, allPostingsKey.Value)

	// Make sure postings gotten before the delete have the old data when
	// iterated over.
	expanded, err := ExpandPostings(before)
	testutil.Ok(t, err)
	testutil.Equals(t, []uint64{1, 2, 3}, expanded)

	// Make sure postings gotten after the delete have the new data when
	// iterated over.
	expanded, err = ExpandPostings(after)
	testutil.Ok(t, err)
	testutil.Equals(t, []uint64{1, 3}, expanded)

	deleted := p.Get("lbl1", "b")
	expanded, err = ExpandPostings(deleted)
	testutil.Ok(t, err)
	testutil.Assert(t, 0 == len(expanded), "expected empty postings, got %v", expanded)
}
