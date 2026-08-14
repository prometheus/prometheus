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

package testwal

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
)

func TestGenerateRecordsStartTimestamps(t *testing.T) {
	for _, tc := range []struct {
		name string
		noST bool
	}{
		{name: "with start timestamp"},
		{name: "without start timestamp", noST: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recs := GenerateRecords(RecordsCase{
				NoST:                     tc.noST,
				Series:                   1,
				SamplesPerSeries:         2,
				HistogramsPerSeries:      2,
				FloatHistogramsPerSeries: 2,
				TsFn: func(_, j int) int64 {
					return int64(100 + j)
				},
			})

			wantST := func(ts int64) int64 {
				if tc.noST {
					return 0
				}
				return ts - 1
			}

			require.Len(t, recs.Samples, 2)
			for _, s := range recs.Samples {
				require.Equal(t, wantST(s.T), s.ST)
			}
			require.Len(t, recs.Histograms, 2)
			for _, h := range recs.Histograms {
				require.Equal(t, wantST(h.T), h.ST)
			}
			require.Len(t, recs.FloatHistograms, 2)
			for _, fh := range recs.FloatHistograms {
				require.Equal(t, wantST(fh.T), fh.ST)
			}
		})
	}
}

// BenchmarkGenerateRecords checks data generator performance.
// Recommended CLI:
/*
	export bench=genRecs && go test ./util/testwal/... \
		-run '^$' -bench '^BenchmarkGenerateRecords' \
		-benchtime 1s -count 6 -cpu 2 -timeout 999m -benchmem \
		| tee ${bench}.txt
*/
func BenchmarkGenerateRecords(b *testing.B) {
	n := 2 * config.DefaultQueueConfig.MaxSamplesPerSend

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		// This will generate 16M samples and 4k series.
		GenerateRecords(RecordsCase{Series: n, SamplesPerSeries: n})
	}
}
