package labels

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"testing"

	"github.com/bradfitz/slice"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"
)

func TestCompare(t *testing.T) {
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
		{
			a: []Label{
				{"__name__", "go_gc_duration_seconds"},
				{"job", "prometheus"},
				{"quantile", "0.75"},
			},
			b: []Label{
				{"__name__", "go_gc_duration_seconds"},
				{"job", "prometheus"},
				{"quantile", "1"},
			},
			res: -1,
		},
		{
			a: []Label{
				{"handler", "prometheus"},
				{"instance", "localhost:9090"},
			},
			b: []Label{
				{"handler", "query"},
				{"instance", "localhost:9090"},
			},
			res: -1,
		},
	}
	for _, c := range cases {
		// Use constructor to ensure sortedness.
		a, b := New(c.a...), New(c.b...)

		require.Equal(t, c.res, Compare(a, b))
	}
}

func BenchmarkLabelsSliceSort(b *testing.B) {
	f, err := os.Open("../cmd/tsdb/testdata.100k")
	require.NoError(b, err)

	lbls, err := readPrometheusLabels(f, 5000)
	require.NoError(b, err)

	b.Run("", func(b *testing.B) {
		clbls := make([]Labels, len(lbls))
		copy(clbls, lbls)

		for i := range clbls {
			j := rand.Intn(i + 1)
			clbls[i], clbls[j] = clbls[j], clbls[i]
		}
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			slice.Sort(clbls, func(i, j int) bool {
				return Compare(clbls[i], clbls[j]) < 0
			})
		}
	})
}

func readPrometheusLabels(r io.Reader, n int) ([]Labels, error) {
	dec := expfmt.NewDecoder(r, expfmt.FmtProtoText)

	var mets []Labels
	var mf dto.MetricFamily

	for i := 0; i < n; i++ {
		if err := dec.Decode(&mf); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		for _, m := range mf.GetMetric() {
			met := make([]Label, 0, len(m.GetLabel())+1)
			met = append(met, Label{"__name__", mf.GetName()})

			for _, l := range m.GetLabel() {
				met = append(met, Label{l.GetName(), l.GetValue()})
			}
			mets = append(mets, met)
		}
	}
	fmt.Println("read metrics", len(mets[:n]))

	return mets[:n], nil
}

func BenchmarkLabelSetFromMap(b *testing.B) {
	m := map[string]string{
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
	}
	var ls Labels
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ls = FromMap(m)
	}
	_ = ls
}

func BenchmarkMapFromLabels(b *testing.B) {
	m := map[string]string{
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
	}
	ls := FromMap(m)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		m = ls.Map()
	}
}

func BenchmarkLabelSetEquals(b *testing.B) {
	// The vast majority of comparisons will be against a matching label set.
	m := map[string]string{
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
	}
	ls := FromMap(m)
	var res bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		res = ls.Equals(ls)
	}
	_ = res
}
