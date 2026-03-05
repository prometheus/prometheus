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

	"github.com/prometheus/prometheus/config"
)

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
