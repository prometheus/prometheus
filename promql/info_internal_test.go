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

package promql

import (
	"runtime"
	"strconv"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
)

func BenchmarkInfoIdentifyingMatcherSets(b *testing.B) {
	testCases := []struct {
		name  string
		size  int
		mixed bool
	}{
		{name: "homogeneous/1", size: 1},
		{name: "homogeneous/10000", size: 10_000},
		{name: "mixed/3", size: 3, mixed: true},
		{name: "mixed/9999", size: 9_999, mixed: true},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			mat := make(Matrix, tc.size)
			for i := range mat {
				series := strconv.Itoa(i)
				if !tc.mixed {
					mat[i].Metric = labels.FromStrings("instance", "a", "job", "api", "series", series)
					continue
				}

				switch i % 3 {
				case 0:
					mat[i].Metric = labels.FromStrings("job", "api", "series", series)
				case 1:
					mat[i].Metric = labels.FromStrings("instance", "standalone", "series", series)
				case 2:
					mat[i].Metric = labels.FromStrings("instance", "b", "job", "worker", "series", series)
				}
			}

			b.ReportAllocs()
			var matcherSets [][]*labels.Matcher
			for b.Loop() {
				matcherSets = infoIdentifyingMatcherSets(mat, nil)
			}
			runtime.KeepAlive(matcherSets)
		})
	}
}
