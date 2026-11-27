// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package writev2

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestSymbolsTable(t *testing.T) {
	s := NewSymbolTable()
	require.Equal(t, []string{""}, s.Symbols(), "required empty reference does not exist")
	require.Equal(t, uint32(0), s.Symbolize(""))
	require.Equal(t, []string{""}, s.Symbols())

	require.Equal(t, uint32(1), s.Symbolize("abc"))
	require.Equal(t, []string{"", "abc"}, s.Symbols())

	require.Equal(t, uint32(2), s.Symbolize("__name__"))
	require.Equal(t, []string{"", "abc", "__name__"}, s.Symbols())

	require.Equal(t, uint32(3), s.Symbolize("foo"))
	require.Equal(t, []string{"", "abc", "__name__", "foo"}, s.Symbols())

	s.Reset()
	require.Equal(t, []string{""}, s.Symbols(), "required empty reference does not exist")
	require.Equal(t, uint32(0), s.Symbolize(""))

	require.Equal(t, uint32(1), s.Symbolize("__name__"))
	require.Equal(t, []string{"", "__name__"}, s.Symbols())

	require.Equal(t, uint32(2), s.Symbolize("abc"))
	require.Equal(t, []string{"", "__name__", "abc"}, s.Symbols())

	ls := labels.FromStrings("__name__", "qwer", "zxcv", "1234")
	encoded := s.SymbolizeLabels(ls, nil)
	require.Equal(t, []uint32{1, 3, 4, 5}, encoded)
	b := labels.NewScratchBuilder(len(encoded))
	decoded, err := desymbolizeLabels(&b, encoded, s.Symbols())
	require.NoError(t, err)
	require.Equal(t, ls, decoded)

	// Different buf.
	ls = labels.FromStrings("__name__", "qwer", "zxcv2222", "1234")
	encoded = s.SymbolizeLabels(ls, []uint32{1, 3, 4, 5})
	require.Equal(t, []uint32{1, 3, 6, 5}, encoded)
}

// Recommended CLI invocation:
//
//	export bench=symbols && go test ./prompb/io/prometheus/write/v2 \
//		-run '^$' -bench '^BenchmarkSymbolsTable$' \
//		-benchtime 2s -count 6 -cpu 2 -timeout 999m -benchmem \
//		| tee ${bench}.txt
func BenchmarkSymbolsTable(b *testing.B) {
	const batchSize = 2_000
	series := make([]labels.Labels, batchSize)
	for i := range series {
		series[i] = labels.FromStrings(
			"__name__", "http_requests_total",
			"instance", "pod-"+strconv.Itoa(i),
			"job", "api-server",
			"namespace", "prod",
		)
	}
	st := NewSymbolTable()
	buf := make([]uint32, 0, 16)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		for _, ls := range series {
			buf = st.SymbolizeLabels(ls, buf)
			_ = st.Symbolize("")
		}
		st.Reset()
	}
}

func TestDesymbolizeLabels(t *testing.T) {
	symbols := []string{"", "__name__", "test_metric", "foo", "bar", "f", "g", "b", "c"}
	b := labels.NewScratchBuilder(0)

	t.Run("valid sorted labels", func(t *testing.T) {
		// __name__="test_metric", b="c", foo="bar" (sorted by name)
		refs := []uint32{1, 2, 7, 8, 3, 4}
		lbls, err := desymbolizeLabels(&b, refs, symbols)
		require.NoError(t, err)
		testutil.RequireEqual(t, labels.FromStrings("__name__", "test_metric", "b", "c", "foo", "bar"), lbls)
	})

	t.Run("sorted strings with decreasing symbol refs", func(t *testing.T) {
		// Symbols: 1: "__name__", 7: "b", 5: "f". String order: __name__ < b < f. Refs: 1, 7, 5 (decreasing from 7 to 5).
		refs := []uint32{1, 2, 7, 8, 5, 6}
		lbls, err := desymbolizeLabels(&b, refs, symbols)
		require.NoError(t, err)
		testutil.RequireEqual(t, labels.FromStrings("__name__", "test_metric", "b", "c", "f", "g"), lbls)
	})

	t.Run("unsorted labels", func(t *testing.T) {
		// __name__="test_metric", foo="bar", b="c" (unsorted: foo comes before b)
		refs := []uint32{1, 2, 3, 4, 7, 8}
		_, err := desymbolizeLabels(&b, refs, symbols)
		require.EqualError(t, err, "labels are not sorted lexicographically: b is not after foo")
	})

	t.Run("duplicate label names", func(t *testing.T) {
		// __name__="test_metric", foo="bar", foo="g"
		refs := []uint32{1, 2, 3, 4, 3, 6}
		_, err := desymbolizeLabels(&b, refs, symbols)
		require.EqualError(t, err, "duplicate label with name: foo")
	})

	t.Run("odd number of label refs", func(t *testing.T) {
		refs := []uint32{1, 2, 3}
		_, err := desymbolizeLabels(&b, refs, symbols)
		require.EqualError(t, err, "invalid labelRefs length 3")
	})

	t.Run("symbol ref out of bounds", func(t *testing.T) {
		refs := []uint32{1, 2, 99, 2}
		_, err := desymbolizeLabels(&b, refs, symbols)
		require.EqualError(t, err, "labelRefs 99 (name) = 2 (value) outside of symbols table (size 9)")
	})

	t.Run("32-bit overflow symbol ref", func(t *testing.T) {
		refs := []uint32{1, 2, 0x80000001, 2}
		_, err := desymbolizeLabels(&b, refs, symbols)
		require.EqualError(t, err, "labelRefs 2147483649 (name) = 2 (value) outside of symbols table (size 9)")
	})

	t.Run("empty label refs", func(t *testing.T) {
		refs := []uint32{}
		lbls, err := desymbolizeLabels(&b, refs, symbols)
		require.NoError(t, err)
		require.True(t, lbls.IsEmpty())
	})
}

func BenchmarkTimeSeries_ToLabels(b *testing.B) {
	symbols := []string{"", "__name__", "http_requests_total", "method", "POST", "handler", "/api/v1/query", "status", "200", "instance", "localhost:9090", "job", "prometheus"}
	ts := TimeSeries{
		// Sorted: __name__, handler, instance, job, method, status
		LabelsRefs: []uint32{1, 2, 5, 6, 9, 10, 11, 12, 3, 4, 7, 8},
	}
	builder := labels.NewScratchBuilder(0)

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		lbls, err := ts.ToLabels(&builder, symbols)
		if err != nil {
			b.Fatal(err)
		}
		if lbls.Len() != 6 {
			b.Fatalf("unexpected labels length %d", lbls.Len())
		}
	}
}
