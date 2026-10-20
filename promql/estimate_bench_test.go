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

package promql_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/tsdb"
)

func BenchmarkEstimateCost(b *testing.B) {
	for _, tc := range []struct {
		name       string
		series     int
		blocks     int
		histograms bool
		query      string
	}{
		{name: "small", series: 10, query: "rate(metric[5m])"},
		{name: "high_cardinality", series: 10000, query: "rate(metric[5m])"},
		{name: "many_blocks", series: 100, blocks: 8, query: "rate(metric[5m])"},
		{name: "histograms", series: 1000, histograms: true, query: "rate(metric[5m])"},
		{name: "repeated_selectors", series: 1000, query: "rate(metric[5m]) + rate(metric[5m]) + rate(metric[5m]) + rate(metric[5m])"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			var input strings.Builder
			input.WriteString("load 10s\n")
			points := max(tc.blocks, 1) * 120
			for i := range tc.series {
				if tc.histograms {
					fmt.Fprintf(&input, "metric{instance=\"%d\"} {{schema:0 sum:5 count:4 buckets:[1 2 1]}}+{{schema:0 sum:5 count:4 buckets:[1 2 1]}}x%d\n", i, points)
				} else {
					fmt.Fprintf(&input, "metric{instance=\"%d\"} 0+1x%d\n", i, points)
				}
			}
			s := promqltest.LoadedStorage(b, input.String())
			b.Cleanup(func() { require.NoError(b, s.Close()) })
			for i := range tc.blocks {
				mint := int64(i) * 1200000
				require.NoError(b, s.CompactHead(tsdb.NewRangeHead(s.Head(), mint, mint+1199999)))
			}
			if tc.blocks > 0 {
				require.Len(b, s.Blocks(), tc.blocks)
			}
			start, end := time.Unix(300, 0), time.Unix(int64(points*10), 0)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, _, err := promql.EstimateCost(context.Background(), s, estimateTestParser, tc.query, start, end, time.Minute, 5*time.Minute, time.Minute, 10*time.Second)
				require.NoError(b, err)
			}
		})
	}
}
