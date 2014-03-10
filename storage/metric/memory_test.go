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
	"runtime"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"
)

func BenchmarkStreamAdd(b *testing.B) {
	b.StopTimer()
	s := newArrayStream(clientmodel.Metric{})
	samples := make(Values, b.N)
	for i := 0; i < b.N; i++ {
		samples = append(samples, SamplePair{
			Timestamp: clientmodel.TimestampFromTime(time.Date(i, 0, 0, 0, 0, 0, 0, time.UTC)),
			Value:     clientmodel.SampleValue(i),
		})
	}

	b.StartTimer()

	var pre runtime.MemStats
	runtime.ReadMemStats(&pre)

	s.add(samples)

	var post runtime.MemStats
	runtime.ReadMemStats(&post)

	b.Logf("%d cycles with %f bytes per cycle, totalling %d", b.N, float32(post.TotalAlloc-pre.TotalAlloc)/float32(b.N), post.TotalAlloc-pre.TotalAlloc)
}

func benchmarkAppendSamples(b *testing.B, labels int) {
	b.StopTimer()
	s := NewMemorySeriesStorage(MemorySeriesOptions{})

	metric := clientmodel.Metric{}

	for i := 0; i < labels; i++ {
		metric[clientmodel.LabelName(fmt.Sprintf("label_%d", i))] = clientmodel.LabelValue(fmt.Sprintf("value_%d", i))
	}
	samples := make(clientmodel.Samples, 0, b.N)
	for i := 0; i < b.N; i++ {
		samples = append(samples, &clientmodel.Sample{
			Metric:    metric,
			Value:     clientmodel.SampleValue(i),
			Timestamp: clientmodel.TimestampFromTime(time.Date(i, 0, 0, 0, 0, 0, 0, time.UTC)),
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
	benchmarkAppendSamples(b, 1)
}

func BenchmarkAppendSample10(b *testing.B) {
	benchmarkAppendSamples(b, 10)
}

func BenchmarkAppendSample100(b *testing.B) {
	benchmarkAppendSamples(b, 100)
}

func BenchmarkAppendSample1000(b *testing.B) {
	benchmarkAppendSamples(b, 1000)
}

// Regression test for https://github.com/prometheus/prometheus/issues/381.
//
// 1. Creates samples for two timeseries with one common labelpair.
// 2. Flushes memory storage such that only one series is dropped from memory.
// 3. Gets fingerprints for common labelpair.
// 4. Checks that exactly one fingerprint remains.
func TestDroppedSeriesIndexRegression(t *testing.T) {
	samples := clientmodel.Samples{
		&clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testmetric",
				"different":                 "differentvalue1",
				"common":                    "samevalue",
			},
			Value:     1,
			Timestamp: clientmodel.TimestampFromTime(time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)),
		},
		&clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testmetric",
				"different":                 "differentvalue2",
				"common":                    "samevalue",
			},
			Value:     2,
			Timestamp: clientmodel.TimestampFromTime(time.Date(2002, 0, 0, 0, 0, 0, 0, time.UTC)),
		},
	}

	s := NewMemorySeriesStorage(MemorySeriesOptions{})
	s.AppendSamples(samples)

	common := clientmodel.LabelSet{"common": "samevalue"}
	fps, err := s.GetFingerprintsForLabelSet(common)
	if err != nil {
		t.Fatal(err)
	}
	if len(fps) != 2 {
		t.Fatalf("Got %d fingerprints, expected 2", len(fps))
	}

	toDisk := make(chan clientmodel.Samples, 2)
	s.Flush(clientmodel.TimestampFromTime(time.Date(2001, 0, 0, 0, 0, 0, 0, time.UTC)), toDisk)
	if len(toDisk) != 1 {
		t.Fatalf("Got %d disk sample lists, expected 1", len(toDisk))
	}
	diskSamples := <-toDisk
	if len(diskSamples) != 1 {
		t.Fatalf("Got %d disk samples, expected 1", len(diskSamples))
	}

	fps, err = s.GetFingerprintsForLabelSet(common)
	if err != nil {
		t.Fatal(err)
	}
	if len(fps) != 1 {
		t.Fatalf("Got %d fingerprints, expected 1", len(fps))
	}
}
