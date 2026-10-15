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

package tsdb

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
)

func BenchmarkMetadataAppendCommit(b *testing.B) {
	for _, v2 := range []bool{false, true} {
		for _, tc := range []struct {
			name     string
			series   int
			samples  int
			helpSize int
			update   bool
		}{
			{name: "repeated", series: 1, samples: 64, helpSize: 64 << 10, update: true},
			{name: "single", series: 1, samples: 1, helpSize: 128, update: true},
			{name: "many_single", series: 1000, samples: 1, helpSize: 128, update: true},
			{name: "unchanged", series: 1, samples: 64, helpSize: 128},
			{name: "many_unchanged", series: 1000, samples: 1, helpSize: 128},
		} {
			b.Run(fmt.Sprintf("v2=%t/%s", v2, tc.name), func(b *testing.B) {
				opts := DefaultOptions()
				opts.EnableMetadataWALRecords = true
				db := newTestDB(b, withOpts(opts))
				db.DisableCompactions()
				sets := make([]labels.Labels, tc.series)
				refs := make([]storage.SeriesRef, tc.series)
				help := strings.Repeat("h", tc.helpSize)
				meta := [2]metadata.Metadata{{Type: model.MetricTypeGauge, Help: help}, {Type: model.MetricTypeCounter, Help: help}}
				app := newMetadataTestAppender(b, db, v2)
				for i := range sets {
					sets[i] = labels.FromStrings("__name__", "m", "series", strconv.Itoa(i))
					// Warm up the buffers with the measured number of samples per series.
					for sample := range tc.samples {
						refs[i] = appendMetadataTestSample(b, app, refs[i], sets[i], int64(sample), nil, &meta[0])
					}
				}
				require.NoError(b, app.Commit())
				initialSize, err := db.head.wal.Size()
				require.NoError(b, err)
				ts := int64(tc.samples)
				iteration := 0
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					m := &meta[0]
					if tc.update {
						// Change metadata each transaction so every iteration exercises pending updates.
						m = &meta[(iteration+1)%2]
					}
					app := newMetadataTestAppender(b, db, v2)
					for i, ls := range sets {
						for sample := range tc.samples {
							appendMetadataTestSample(b, app, refs[i], ls, ts+int64(sample), nil, m)
						}
					}
					require.NoError(b, app.Commit())
					ts += int64(tc.samples)
					iteration++
				}
				finalSize, err := db.head.wal.Size()
				require.NoError(b, err)
				b.ReportMetric(float64(finalSize-initialSize)/float64(b.N), "wal-B/op")
			})
		}
	}
}
