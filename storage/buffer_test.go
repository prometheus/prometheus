package storage

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSampleRing(t *testing.T) {
	cases := []struct {
		input []int64
		delta int64
		size  int
	}{
		{
			input: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			delta: 2,
			size:  1,
		},
		{
			input: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			delta: 2,
			size:  2,
		},
		{
			input: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			delta: 7,
			size:  3,
		},
		{
			input: []int64{1, 2, 3, 4, 5, 16, 17, 18, 19, 20},
			delta: 7,
			size:  1,
		},
	}
	for _, c := range cases {
		r := newSampleRing(c.delta, c.size)

		input := []sample{}
		for _, t := range c.input {
			input = append(input, sample{
				t: t,
				v: float64(rand.Intn(100)),
			})
		}

		for i, s := range input {
			r.add(s.t, s.v)
			buffered := r.samples()

			for _, sold := range input[:i] {
				found := false
				for _, bs := range buffered {
					if bs.t == sold.t && bs.v == sold.v {
						found = true
						break
					}
				}
				if sold.t >= s.t-c.delta && !found {
					t.Fatalf("%d: expected sample %d to be in buffer but was not; buffer %v", i, sold.t, buffered)
				}
				if sold.t < s.t-c.delta && found {
					t.Fatalf("%d: unexpected sample %d in buffer; buffer %v", i, sold.t, buffered)
				}
			}
		}
	}
}

func TestBufferedSeriesIterator(t *testing.T) {
	var it *BufferedSeriesIterator

	bufferEq := func(exp []sample) {
		var b []sample
		bit := it.Buffer()
		for bit.Next() {
			t, v := bit.Values()
			b = append(b, sample{t: t, v: v})
		}
		require.Equal(t, exp, b, "buffer mismatch")
	}
	sampleEq := func(ets int64, ev float64) {
		ts, v := it.Values()
		require.Equal(t, ets, ts, "timestamp mismatch")
		require.Equal(t, ev, v, "value mismatch")
	}

	it = NewBuffer(newListSeriesIterator([]sample{
		{t: 1, v: 2},
		{t: 2, v: 3},
		{t: 3, v: 4},
		{t: 4, v: 5},
		{t: 5, v: 6},
		{t: 99, v: 8},
		{t: 100, v: 9},
		{t: 101, v: 10},
	}), 2)

	require.True(t, it.Seek(-123), "seek failed")
	sampleEq(1, 2)
	bufferEq(nil)

	require.True(t, it.Next(), "next failed")
	sampleEq(2, 3)
	bufferEq([]sample{{t: 1, v: 2}})

	require.True(t, it.Next(), "next failed")
	require.True(t, it.Next(), "next failed")
	require.True(t, it.Next(), "next failed")
	sampleEq(5, 6)
	bufferEq([]sample{{t: 2, v: 3}, {t: 3, v: 4}, {t: 4, v: 5}})

	require.True(t, it.Seek(5), "seek failed")
	sampleEq(5, 6)
	bufferEq([]sample{{t: 2, v: 3}, {t: 3, v: 4}, {t: 4, v: 5}})

	require.True(t, it.Seek(101), "seek failed")
	sampleEq(101, 10)
	bufferEq([]sample{{t: 99, v: 8}, {t: 100, v: 9}})

	require.False(t, it.Next(), "next succeeded unexpectedly")
}
