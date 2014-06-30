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

package tiered

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

func BenchmarkStreamAdd(b *testing.B) {
	b.StopTimer()
	s := newArrayStream(clientmodel.Metric{})
	samples := make(metric.Values, b.N)
	for i := 0; i < b.N; i++ {
		samples = append(samples, metric.SamplePair{
			Timestamp: clientmodel.TimestampFromTime(time.Date(i, 0, 0, 0, 0, 0, 0, time.UTC)),
			Value:     clientmodel.SampleValue(i),
		})
	}

	b.StartTimer()
	s.add(samples)
}

func TestStreamAdd(t *testing.T) {
	s := newArrayStream(clientmodel.Metric{})
	// Add empty to empty.
	v := metric.Values{}
	expected := metric.Values{}
	s.add(v)
	if got := s.values; !reflect.DeepEqual(expected, got) {
		t.Fatalf("Expected values %#v in stream, got %#v.", expected, got)
	}
	// Add something to empty.
	v = metric.Values{
		metric.SamplePair{Timestamp: 1, Value: -1},
	}
	expected = append(expected, v...)
	s.add(v)
	if got := s.values; !reflect.DeepEqual(expected, got) {
		t.Fatalf("Expected values %#v in stream, got %#v.", expected, got)
	}
	// Add something to something.
	v = metric.Values{
		metric.SamplePair{Timestamp: 2, Value: -2},
		metric.SamplePair{Timestamp: 5, Value: -5},
	}
	expected = append(expected, v...)
	s.add(v)
	if got := s.values; !reflect.DeepEqual(expected, got) {
		t.Fatalf("Expected values %#v in stream, got %#v.", expected, got)
	}
	// Add something outdated to something.
	v = metric.Values{
		metric.SamplePair{Timestamp: 3, Value: -3},
		metric.SamplePair{Timestamp: 4, Value: -4},
	}
	s.add(v)
	if got := s.values; !reflect.DeepEqual(expected, got) {
		t.Fatalf("Expected values %#v in stream, got %#v.", expected, got)
	}
	// Add something partially outdated to something.
	v = metric.Values{
		metric.SamplePair{Timestamp: 3, Value: -3},
		metric.SamplePair{Timestamp: 6, Value: -6},
	}
	expected = append(expected, metric.SamplePair{Timestamp: 6, Value: -6})
	s.add(v)
	if got := s.values; !reflect.DeepEqual(expected, got) {
		t.Fatalf("Expected values %#v in stream, got %#v.", expected, got)
	}
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
	for i := 0; i < b.N; i++ {
		s.AppendSample(samples[i])
	}
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
	fps, err := s.GetFingerprintsForLabelMatchers(labelMatchersFromLabelSet(common))
	if err != nil {
		t.Fatal(err)
	}
	if len(fps) != 2 {
		t.Fatalf("Got %d fingerprints, expected 2", len(fps))
	}

	toDisk := make(chan clientmodel.Samples, 2)
	flushOlderThan := clientmodel.TimestampFromTime(time.Date(2001, 0, 0, 0, 0, 0, 0, time.UTC))
	s.Flush(flushOlderThan, toDisk)
	if len(toDisk) != 1 {
		t.Fatalf("Got %d disk sample lists, expected 1", len(toDisk))
	}
	diskSamples := <-toDisk
	if len(diskSamples) != 1 {
		t.Fatalf("Got %d disk samples, expected 1", len(diskSamples))
	}
	s.Evict(flushOlderThan)

	fps, err = s.GetFingerprintsForLabelMatchers(labelMatchersFromLabelSet(common))
	if err != nil {
		t.Fatal(err)
	}
	if len(fps) != 1 {
		t.Fatalf("Got %d fingerprints, expected 1", len(fps))
	}
}

func TestReaderWriterDeadlockRegression(t *testing.T) {
	mp := runtime.GOMAXPROCS(2)
	defer func(mp int) {
		runtime.GOMAXPROCS(mp)
	}(mp)

	s := NewMemorySeriesStorage(MemorySeriesOptions{})
	lms := metric.LabelMatchers{}

	for i := 0; i < 100; i++ {
		lm, err := metric.NewLabelMatcher(metric.NotEqual, clientmodel.MetricNameLabel, "testmetric")
		if err != nil {
			t.Fatal(err)
		}
		lms = append(lms, lm)
	}

	wg := sync.WaitGroup{}
	wg.Add(2)

	start := time.Now()
	runDuration := 250 * time.Millisecond

	writer := func() {
		for time.Since(start) < runDuration {
			s.AppendSamples(clientmodel.Samples{
				&clientmodel.Sample{
					Metric: clientmodel.Metric{
						clientmodel.MetricNameLabel: "testmetric",
					},
					Value:     1,
					Timestamp: 0,
				},
			})
		}
		wg.Done()
	}

	reader := func() {
		for time.Since(start) < runDuration {
			s.GetFingerprintsForLabelMatchers(lms)
		}
		wg.Done()
	}

	go reader()
	go writer()

	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		allDone <- struct{}{}
	}()

	select {
	case <-allDone:
		break
	case <-time.NewTimer(5 * time.Second).C:
		t.Fatalf("Deadlock timeout")
	}
}

func BenchmarkGetFingerprintsForNotEqualMatcher1000(b *testing.B) {
	numSeries := 1000
	samples := make(clientmodel.Samples, 0, numSeries)
	for i := 0; i < numSeries; i++ {
		samples = append(samples, &clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testmetric",
				"instance":                  clientmodel.LabelValue(fmt.Sprint("instance_", i)),
			},
			Value:     1,
			Timestamp: clientmodel.TimestampFromTime(time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)),
		})
	}

	s := NewMemorySeriesStorage(MemorySeriesOptions{})
	if err := s.AppendSamples(samples); err != nil {
		b.Fatal(err)
	}

	m, err := metric.NewLabelMatcher(metric.NotEqual, "instance", "foo")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.GetFingerprintsForLabelMatchers(metric.LabelMatchers{m})
	}
}
