package tsdb

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompareLabels(t *testing.T) {
	cases := []struct {
		a, b []Label
		res  int
	}{
		{
			a:   []Label{},
			b:   []Label{},
			res: 0,
		},
		{
			a:   []Label{{"a", ""}},
			b:   []Label{{"a", ""}, {"b", ""}},
			res: -1,
		},
		{
			a:   []Label{{"a", ""}},
			b:   []Label{{"a", ""}},
			res: 0,
		},
		{
			a:   []Label{{"aa", ""}, {"aa", ""}},
			b:   []Label{{"aa", ""}, {"ab", ""}},
			res: -1,
		},
		{
			a:   []Label{{"aa", ""}, {"abb", ""}},
			b:   []Label{{"aa", ""}, {"ab", ""}},
			res: 1,
		},
	}
	for _, c := range cases {
		// Use constructor to ensure sortedness.
		a, b := NewLabels(c.a...), NewLabels(c.b...)

		require.Equal(t, c.res, compareLabels(a, b))
	}
}

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
