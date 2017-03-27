package tsdb

import (
	"encoding/binary"
	"math/rand"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockPostings struct {
	next  func() bool
	seek  func(uint32) bool
	value func() uint32
	err   func() error
}

func (m *mockPostings) Next() bool         { return m.next() }
func (m *mockPostings) Seek(v uint32) bool { return m.seek(v) }
func (m *mockPostings) Value() uint32      { return m.value() }
func (m *mockPostings) Err() error         { return m.err() }

func TestIntersect(t *testing.T) {
	var cases = []struct {
		a, b []uint32
		res  []uint32
	}{
		{
			a:   []uint32{1, 2, 3, 4, 5},
			b:   []uint32{6, 7, 8, 9, 10},
			res: nil,
		},
		{
			a:   []uint32{1, 2, 3, 4, 5},
			b:   []uint32{4, 5, 6, 7, 8},
			res: []uint32{4, 5},
		},
		{
			a:   []uint32{1, 2, 3, 4, 9, 10},
			b:   []uint32{1, 4, 5, 6, 7, 8, 10, 11},
			res: []uint32{1, 4, 10},
		}, {
			a:   []uint32{1},
			b:   []uint32{0, 1},
			res: []uint32{1},
		},
	}

	for i, c := range cases {
		a := newListPostings(c.a)
		b := newListPostings(c.b)

		res, err := expandPostings(Intersect(a, b))
		if err != nil {
			t.Fatalf("%d: Unexpected error: %s", i, err)
		}
		if !reflect.DeepEqual(res, c.res) {
			t.Fatalf("%d: Expected %v but got %v", i, c.res, res)
		}
	}
}

func TestMultiIntersect(t *testing.T) {
	var cases = []struct {
		a, b, c []uint32
		res     []uint32
	}{
		{
			a:   []uint32{1, 2, 3, 4, 5, 6, 1000, 1001},
			b:   []uint32{2, 4, 5, 6, 7, 8, 999, 1001},
			c:   []uint32{1, 2, 5, 6, 7, 8, 1001, 1200},
			res: []uint32{2, 5, 6, 1001},
		},
	}

	for _, c := range cases {
		pa := newListPostings(c.a)
		pb := newListPostings(c.b)
		pc := newListPostings(c.c)

		res, err := expandPostings(Intersect(pa, pb, pc))
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		if !reflect.DeepEqual(res, c.res) {
			t.Fatalf("Expected %v but got %v", c.res, res)
		}
	}
}

func BenchmarkIntersect(t *testing.B) {
	var a, b, c, d []uint32

	for i := 0; i < 10000000; i += 2 {
		a = append(a, uint32(i))
	}
	for i := 5000000; i < 5000100; i += 4 {
		b = append(b, uint32(i))
	}
	for i := 5090000; i < 5090600; i += 4 {
		b = append(b, uint32(i))
	}
	for i := 4990000; i < 5100000; i++ {
		c = append(c, uint32(i))
	}
	for i := 4000000; i < 6000000; i++ {
		d = append(d, uint32(i))
	}

	i1 := newListPostings(a)
	i2 := newListPostings(b)
	i3 := newListPostings(c)
	i4 := newListPostings(d)

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		if _, err := expandPostings(Intersect(i1, i2, i3, i4)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMultiMerge(t *testing.T) {
	var cases = []struct {
		a, b, c []uint32
		res     []uint32
	}{
		{
			a:   []uint32{1, 2, 3, 4, 5, 6, 1000, 1001},
			b:   []uint32{2, 4, 5, 6, 7, 8, 999, 1001},
			c:   []uint32{1, 2, 5, 6, 7, 8, 1001, 1200},
			res: []uint32{1, 2, 3, 4, 5, 6, 7, 8, 999, 1000, 1001, 1200},
		},
	}

	for _, c := range cases {
		i1 := newListPostings(c.a)
		i2 := newListPostings(c.b)
		i3 := newListPostings(c.c)

		res, err := expandPostings(Merge(i1, i2, i3))
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		if !reflect.DeepEqual(res, c.res) {
			t.Fatalf("Expected %v but got %v", c.res, res)
		}
	}
}

func TestMerge(t *testing.T) {
	var cases = []struct {
		a, b []uint32
		res  []uint32
	}{
		{
			a:   []uint32{1, 2, 3, 4, 5},
			b:   []uint32{6, 7, 8, 9, 10},
			res: []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			a:   []uint32{1, 2, 3, 4, 5},
			b:   []uint32{4, 5, 6, 7, 8},
			res: []uint32{1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			a:   []uint32{1, 2, 3, 4, 9, 10},
			b:   []uint32{1, 4, 5, 6, 7, 8, 10, 11},
			res: []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
		},
	}

	for _, c := range cases {
		a := newListPostings(c.a)
		b := newListPostings(c.b)

		res, err := expandPostings(newMergePostings(a, b))
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		if !reflect.DeepEqual(res, c.res) {
			t.Fatalf("Expected %v but got %v", c.res, res)
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
			require.Equal(t, ls[i], bep.At())
		}

		require.False(t, bep.Next())
		require.Nil(t, bep.Err())
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
		bep.Next()

		for _, v := range table {
			require.Equal(t, v.found, bep.Seek(v.seek))
			// Once you seek beyond, At() will panic.
			if v.found {
				require.Equal(t, v.val, bep.At())
				require.Nil(t, bep.Err())
			}
		}
	})
}
