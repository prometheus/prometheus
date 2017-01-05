package tsdb

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPositionMapper(t *testing.T) {
	cases := []struct {
		in  []int
		res []int
	}{
		{
			in:  []int{5, 4, 3, 2, 1, 0},
			res: []int{5, 4, 3, 2, 1, 0},
		},
		{
			in:  []int{1, 2, 0, 3},
			res: []int{1, 2, 0, 3},
		},
		{
			in:  []int{1, 2, 0, 3, 10, 100, -10},
			res: []int{2, 3, 1, 4, 5, 6, 0},
		},
	}

	for _, c := range cases {
		m := newPositionMapper(sort.IntSlice(c.in))

		require.True(t, sort.IsSorted(m.sortable))
		require.Equal(t, c.res, m.fw)
	}
}
