// Copyright 2017 The Prometheus Authors
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

package labels

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"testing"

	"github.com/prometheus/tsdb/testutil"
)

func TestCompareAndEquals(t *testing.T) {
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

		testutil.Equals(t, c.res, Compare(a, b))
		testutil.Equals(t, c.res == 0, a.Equals(b))
	}
}

func BenchmarkSliceSort(b *testing.B) {
	lbls, err := ReadLabels(filepath.Join("..", "testdata", "20kseries.json"), 20000)
	testutil.Ok(b, err)

	for len(lbls) < 20e6 {
		lbls = append(lbls, lbls...)
	}
	for i := range lbls {
		j := rand.Intn(i + 1)
		lbls[i], lbls[j] = lbls[j], lbls[i]
	}

	for _, k := range []int{
		100, 5000, 50000, 300000, 900000, 5e6, 20e6,
	} {
		b.Run(fmt.Sprintf("%d", k), func(b *testing.B) {
			b.ReportAllocs()

			for a := 0; a < b.N; a++ {
				b.StopTimer()
				cl := make(Slice, k)
				copy(cl, Slice(lbls[:k]))
				b.StartTimer()

				sort.Sort(cl)
			}
		})
	}
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

func BenchmarkLabelSetHash(b *testing.B) {
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
	var res uint64

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		res += ls.Hash()
	}
	fmt.Println(res)
}
