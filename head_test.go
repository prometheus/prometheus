package tsdb

import (
	"io/ioutil"
	"os"
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

func BenchmarkCreateSeries(b *testing.B) {
	f, err := os.Open("cmd/tsdb/testdata.1m")
	require.NoError(b, err)
	defer f.Close()

	lbls, err := readPrometheusLabels(f, 1e6)
	require.NoError(b, err)

	b.Run("", func(b *testing.B) {
		dir, err := ioutil.TempDir("", "create_series_bench")
		require.NoError(b, err)
		defer os.RemoveAll(dir)

		h, err := createHeadBlock(dir, 0, nil, 0, 1)
		require.NoError(b, err)

		b.ReportAllocs()
		b.ResetTimer()

		for _, l := range lbls[:b.N] {
			h.create(l.Hash(), l)
		}
	})
}
