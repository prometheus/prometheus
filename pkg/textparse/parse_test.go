package textparse

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/require"
)

func BenchmarkParse(b *testing.B) {
	f, err := os.Open("testdata.txt")
	// f, err := os.Open("../../../../fabxc/tsdb/cmd/tsdb/testdata.1m")
	require.NoError(b, err)
	defer f.Close()

	var tbuf []byte

	buf, err := ioutil.ReadAll(f)
	require.NoError(b, err)

	for i := 0; i*1e6 < b.N; i++ {
		tbuf = append(tbuf, buf...)
		tbuf = append(tbuf, '\n')
	}

	p := New(tbuf)
	i := 0
	var m labels.Labels
	total := 0
	// res := make(labels.Labels, 0, 5)

	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(tbuf)))

	for p.Next() && i < b.N {
		p.At()

		// res = res[:0]

		// err = ParseMetric(mb, &res)
		// require.NoError(b, err)

		// total += len(res)
		i++
	}
	_ = m
	fmt.Println(total)
	require.NoError(b, p.Err())
}
