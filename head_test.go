package tsdb

import (
	"io/ioutil"
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
	lbls, err := readPrometheusLabels("cmd/tsdb/testdata.1m", 1e6)
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
