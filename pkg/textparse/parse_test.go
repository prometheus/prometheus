package textparse

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	input := `# HELP go_gc_duration_seconds A summary of the GC invocation durations.
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 4.9351e-05
go_gc_duration_seconds{quantile="0.25"} 7.424100000000001e-05
go_gc_duration_seconds{quantile="0.5",a="b"} 8.3835e-05
go_gc_duration_seconds_count 99
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 33`

	exp := []struct {
		lset labels.Labels
		m    string
		v    float64
	}{
		{
			m:    `go_gc_duration_seconds{quantile="0"}`,
			v:    4.9351e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0"),
		}, {
			m:    `go_gc_duration_seconds{quantile="0.25"}`,
			v:    7.424100000000001e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.25"),
		}, {
			m:    `go_gc_duration_seconds{quantile="0.5",a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.5", "a", "b"),
		}, {
			m:    `go_gc_duration_seconds_count`,
			v:    99,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds_count"),
		}, {
			m:    `go_goroutines`,
			v:    33,
			lset: labels.FromStrings("__name__", "go_goroutines"),
		},
	}

	p := New([]byte(input))
	i := 0

	var res labels.Labels

	for p.Next() {
		m, _, v := p.At()

		p.Metric(&res)

		require.Equal(t, exp[i].m, string(m))
		require.Equal(t, exp[i].v, v)
		require.Equal(t, exp[i].lset, res)

		i++
		res = res[:0]
	}

	require.NoError(t, p.Err())

}

func BenchmarkParse(b *testing.B) {
	f, err := os.Open("testdata.txt")
	require.NoError(b, err)
	defer f.Close()

	var tbuf []byte

	buf, err := ioutil.ReadAll(f)
	require.NoError(b, err)

	for i := 0; i*527 < b.N; i++ {
		tbuf = append(tbuf, buf...)
		tbuf = append(tbuf, '\n')
	}

	p := New(tbuf)
	i := 0
	var m labels.Labels
	total := 0

	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(tbuf)))

	for p.Next() && i < b.N {
		p.At()

		res := make(labels.Labels, 0, 5)
		p.Metric(&res)

		total += len(res)
		i++
		res = res[:0]
	}
	_ = m
	fmt.Println(total)
	require.NoError(b, p.Err())
}
