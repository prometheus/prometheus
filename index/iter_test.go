package index

import (
	"reflect"
	"testing"
)

func TestMultiIntersect(t *testing.T) {
	var cases = []struct {
		a, b, c []DocID
		res     []DocID
	}{
		{
			a:   []DocID{1, 2, 3, 4, 5, 6, 1000, 1001},
			b:   []DocID{2, 4, 5, 6, 7, 8, 999, 1001},
			c:   []DocID{1, 2, 5, 6, 7, 8, 1001, 1200},
			res: []DocID{2, 5, 6, 1001},
		},
	}

	for _, c := range cases {
		i1 := newPlainListIterator(c.a)
		i2 := newPlainListIterator(c.b)
		i3 := newPlainListIterator(c.c)

		res, err := ExpandIterator(Intersect(i1, i2, i3))
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		if !reflect.DeepEqual(res, c.res) {
			t.Fatalf("Expected %v but got %v", c.res, res)
		}
	}
}

func TestIntersectIterator(t *testing.T) {
	var cases = []struct {
		a, b []DocID
		res  []DocID
	}{
		{
			a:   []DocID{1, 2, 3, 4, 5},
			b:   []DocID{6, 7, 8, 9, 10},
			res: []DocID{},
		},
		{
			a:   []DocID{1, 2, 3, 4, 5},
			b:   []DocID{4, 5, 6, 7, 8},
			res: []DocID{4, 5},
		},
		{
			a:   []DocID{1, 2, 3, 4, 9, 10},
			b:   []DocID{1, 4, 5, 6, 7, 8, 10, 11},
			res: []DocID{1, 4, 10},
		}, {
			a:   []DocID{1},
			b:   []DocID{0, 1},
			res: []DocID{1},
		},
	}

	for _, c := range cases {
		i1 := newPlainListIterator(c.a)
		i2 := newPlainListIterator(c.b)

		res, err := ExpandIterator(&intersectIterator{i1: i1, i2: i2})
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		if !reflect.DeepEqual(res, c.res) {
			t.Fatalf("Expected %v but got %v", c.res, res)
		}
	}
}

func TestMergeIntersect(t *testing.T) {
	var cases = []struct {
		a, b, c []DocID
		res     []DocID
	}{
		{
			a:   []DocID{1, 2, 3, 4, 5, 6, 1000, 1001},
			b:   []DocID{2, 4, 5, 6, 7, 8, 999, 1001},
			c:   []DocID{1, 2, 5, 6, 7, 8, 1001, 1200},
			res: []DocID{1, 2, 3, 4, 5, 6, 7, 8, 999, 1000, 1001, 1200},
		},
	}

	for _, c := range cases {
		i1 := newPlainListIterator(c.a)
		i2 := newPlainListIterator(c.b)
		i3 := newPlainListIterator(c.c)

		res, err := ExpandIterator(Merge(i1, i2, i3))
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		if !reflect.DeepEqual(res, c.res) {
			t.Fatalf("Expected %v but got %v", c.res, res)
		}
	}
}

func BenchmarkIntersect(t *testing.B) {
	var a, b, c, d []DocID

	for i := 0; i < 10000000; i += 2 {
		a = append(a, DocID(i))
	}
	for i := 5000000; i < 5000100; i += 4 {
		b = append(b, DocID(i))
	}
	for i := 5090000; i < 5090600; i += 4 {
		b = append(b, DocID(i))
	}
	for i := 4990000; i < 5100000; i++ {
		c = append(c, DocID(i))
	}
	for i := 4000000; i < 6000000; i++ {
		d = append(d, DocID(i))
	}

	i1 := newPlainListIterator(a)
	i2 := newPlainListIterator(b)
	i3 := newPlainListIterator(c)
	i4 := newPlainListIterator(d)

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		if _, err := ExpandIterator(Intersect(i1, i2, i3, i4)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMergeIterator(t *testing.T) {
	var cases = []struct {
		a, b []DocID
		res  []DocID
	}{
		{
			a:   []DocID{1, 2, 3, 4, 5},
			b:   []DocID{6, 7, 8, 9, 10},
			res: []DocID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			a:   []DocID{1, 2, 3, 4, 5},
			b:   []DocID{4, 5, 6, 7, 8},
			res: []DocID{1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			a:   []DocID{1, 2, 3, 4, 9, 10},
			b:   []DocID{1, 4, 5, 6, 7, 8, 10, 11},
			res: []DocID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
		},
	}

	for _, c := range cases {
		i1 := newPlainListIterator(c.a)
		i2 := newPlainListIterator(c.b)

		res, err := ExpandIterator(&mergeIterator{i1: i1, i2: i2})
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		if !reflect.DeepEqual(res, c.res) {
			t.Fatalf("Expected %v but got %v", c.res, res)
		}
	}
}

func TestSkippingIterator(t *testing.T) {
	var cases = []struct {
		skiplist skiplistIterator
		its      iteratorStore
		res      []DocID
	}{
		{
			skiplist: newPlainSkiplistIterator(map[DocID]uint64{
				5:   3,
				50:  2,
				500: 1,
			}),
			its: testIteratorStore{
				3: newPlainListIterator(list{5, 7, 8, 9}),
				2: newPlainListIterator(list{54, 60, 61}),
				1: newPlainListIterator(list{1200, 1300, 100000}),
			},
			res: []DocID{5, 7, 8, 9, 54, 60, 61, 1200, 1300, 100000},
		},
		{
			skiplist: newPlainSkiplistIterator(map[DocID]uint64{
				0:  3,
				50: 2,
			}),
			its: testIteratorStore{
				3: newPlainListIterator(list{5, 7, 8, 9}),
				2: newPlainListIterator(list{54, 60, 61}),
			},
			res: []DocID{5, 7, 8, 9, 54, 60, 61},
		},
	}

	for _, c := range cases {
		it := &skippingIterator{
			skiplist:  c.skiplist,
			iterators: c.its,
		}
		res, err := ExpandIterator(it)
		if err != nil {
			t.Fatalf("Unexpected error", err)
		}
		if !reflect.DeepEqual(res, c.res) {
			t.Fatalf("Expected %v but got %v", c.res, res)
		}
	}
}

type testIteratorStore map[uint64]Iterator

func (s testIteratorStore) get(id uint64) (Iterator, error) {
	it, ok := s[id]
	if !ok {
		return nil, errNotFound
	}
	return it, nil
}
