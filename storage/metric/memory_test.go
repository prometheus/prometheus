// Copyright 2013 Prometheus Team
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

package metric

import (
	"fmt"
	"github.com/prometheus/prometheus/model"
	"runtime"
	"testing"
	"time"
)

func BenchmarkStreamAdd(b *testing.B) {
	b.StopTimer()
	s := newStream(clientmodel.Metric{})
	times := make([]time.Time, 0, b.N)
	samples := make([]model.SampleValue, 0, b.N)
	for i := 0; i < b.N; i++ {
		times = append(times, time.Date(i, 0, 0, 0, 0, 0, 0, time.UTC))
		samples = append(samples, model.SampleValue(i))
	}

	b.StartTimer()

	var pre runtime.MemStats
	runtime.ReadMemStats(&pre)

	for i := 0; i < b.N; i++ {
		s.add(times[i], samples[i])
	}

	var post runtime.MemStats
	runtime.ReadMemStats(&post)

	b.Logf("%d cycles with %f bytes per cycle, totalling %d", b.N, float32(post.TotalAlloc-pre.TotalAlloc)/float32(b.N), post.TotalAlloc-pre.TotalAlloc)
}

func benchmarkAppendSample(b *testing.B, labels int) {
	b.StopTimer()
	s := NewMemorySeriesStorage(MemorySeriesOptions{})

	metric := clientmodel.Metric{}

	for i := 0; i < labels; i++ {
		metric[model.LabelName(fmt.Sprintf("label_%d", i))] = model.LabelValue(fmt.Sprintf("value_%d", i))
	}
	samples := make(clientmodel.Samples, 0, b.N)
	for i := 0; i < b.N; i++ {
		samples = append(samples, clientmodel.Sample{
			Metric:    metric,
			Value:     model.SampleValue(i),
			Timestamp: time.Date(i, 0, 0, 0, 0, 0, 0, time.UTC),
		})
	}

	b.StartTimer()
	var pre runtime.MemStats
	runtime.ReadMemStats(&pre)

	for i := 0; i < b.N; i++ {
		s.AppendSample(samples[i])
	}

	var post runtime.MemStats
	runtime.ReadMemStats(&post)

	b.Logf("%d cycles with %f bytes per cycle, totalling %d", b.N, float32(post.TotalAlloc-pre.TotalAlloc)/float32(b.N), post.TotalAlloc-pre.TotalAlloc)
}

func BenchmarkAppendSample1(b *testing.B) {
	benchmarkAppendSample(b, 1)
}

func BenchmarkAppendSample10(b *testing.B) {
	benchmarkAppendSample(b, 10)
}

func BenchmarkAppendSample100(b *testing.B) {
	benchmarkAppendSample(b, 100)
}

func BenchmarkAppendSample1000(b *testing.B) {
	benchmarkAppendSample(b, 1000)
}
