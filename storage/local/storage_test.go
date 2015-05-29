// Copyright 2014 The Prometheus Authors
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

package local

import (
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"
	"time"

	"github.com/prometheus/log"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestFingerprintsForLabelMatchers(t *testing.T) {
	storage, closer := NewTestStorage(t, 1)
	defer closer.Close()

	samples := make([]*clientmodel.Sample, 100)
	fingerprints := make(clientmodel.Fingerprints, 100)

	for i := range samples {
		metric := clientmodel.Metric{
			clientmodel.MetricNameLabel: clientmodel.LabelValue(fmt.Sprintf("test_metric_%d", i)),
			"label1":                    clientmodel.LabelValue(fmt.Sprintf("test_%d", i/10)),
			"label2":                    clientmodel.LabelValue(fmt.Sprintf("test_%d", (i+5)/10)),
		}
		samples[i] = &clientmodel.Sample{
			Metric:    metric,
			Timestamp: clientmodel.Timestamp(i),
			Value:     clientmodel.SampleValue(i),
		}
		fingerprints[i] = metric.FastFingerprint()
	}
	for _, s := range samples {
		storage.Append(s)
	}
	storage.WaitForIndexing()

	newMatcher := func(matchType metric.MatchType, name clientmodel.LabelName, value clientmodel.LabelValue) *metric.LabelMatcher {
		lm, err := metric.NewLabelMatcher(matchType, name, value)
		if err != nil {
			t.Fatalf("error creating label matcher: %s", err)
		}
		return lm
	}

	var matcherTests = []struct {
		matchers metric.LabelMatchers
		expected clientmodel.Fingerprints
	}{
		{
			matchers: metric.LabelMatchers{newMatcher(metric.Equal, "label1", "x")},
			expected: fingerprints[:0],
		},
		{
			matchers: metric.LabelMatchers{newMatcher(metric.Equal, "label1", "test_0")},
			expected: fingerprints[:10],
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.Equal, "label1", "test_0"),
				newMatcher(metric.Equal, "label2", "test_1"),
			},
			expected: fingerprints[5:10],
		},
		{
			matchers: metric.LabelMatchers{newMatcher(metric.NotEqual, "label1", "x")},
			expected: fingerprints,
		},
		{
			matchers: metric.LabelMatchers{newMatcher(metric.NotEqual, "label1", "test_0")},
			expected: fingerprints[10:],
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.NotEqual, "label1", "test_0"),
				newMatcher(metric.NotEqual, "label1", "test_1"),
				newMatcher(metric.NotEqual, "label1", "test_2"),
			},
			expected: fingerprints[30:],
		},
		{
			matchers: metric.LabelMatchers{newMatcher(metric.RegexMatch, "label1", `test_[3-5]`)},
			expected: fingerprints[30:60],
		},
		{
			matchers: metric.LabelMatchers{newMatcher(metric.RegexNoMatch, "label1", `test_[3-5]`)},
			expected: append(append(clientmodel.Fingerprints{}, fingerprints[:30]...), fingerprints[60:]...),
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.RegexMatch, "label1", `test_[3-5]`),
				newMatcher(metric.RegexMatch, "label2", `test_[4-6]`),
			},
			expected: fingerprints[35:60],
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.RegexMatch, "label1", `test_[3-5]`),
				newMatcher(metric.NotEqual, "label2", `test_4`),
			},
			expected: append(append(clientmodel.Fingerprints{}, fingerprints[30:35]...), fingerprints[45:60]...),
		},
	}

	for _, mt := range matcherTests {
		resfps := storage.FingerprintsForLabelMatchers(mt.matchers)
		if len(mt.expected) != len(resfps) {
			t.Fatalf("expected %d matches for %q, found %d", len(mt.expected), mt.matchers, len(resfps))
		}
		for _, fp1 := range resfps {
			found := false
			for _, fp2 := range mt.expected {
				if fp1 == fp2 {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected fingerprint %s for %q not in result", fp1, mt.matchers)
			}
		}
	}
}

func TestRetentionCutoff(t *testing.T) {
	now := clientmodel.Now()
	insertStart := now.Add(-2 * time.Hour)

	s, closer := NewTestStorage(t, 1)
	defer closer.Close()

	// Stop maintenance loop to prevent actual purging.
	s.loopStopping <- struct{}{}

	s.dropAfter = 1 * time.Hour

	samples := make(clientmodel.Samples, 120)
	for i := range samples {
		smpl := &clientmodel.Sample{
			Metric:    clientmodel.Metric{"job": "test"},
			Timestamp: insertStart.Add(time.Duration(i) * time.Minute), // 1 minute intervals.
			Value:     1,
		}
		s.Append(smpl)
	}
	s.WaitForIndexing()

	lm, err := metric.NewLabelMatcher(metric.Equal, "job", "test")
	if err != nil {
		t.Fatalf("error creating label matcher: %s", err)
	}
	fp := s.FingerprintsForLabelMatchers(metric.LabelMatchers{lm})[0]

	pl := s.NewPreloader()
	defer pl.Close()

	// Preload everything.
	err = pl.PreloadRange(fp, insertStart, now, 5*time.Minute)
	if err != nil {
		t.Fatalf("Error preloading outdated chunks: %s", err)
	}

	it := s.NewIterator(fp)

	vals := it.ValueAtTime(now.Add(-61 * time.Minute))
	if len(vals) != 0 {
		t.Errorf("unexpected result for timestamp before retention period")
	}

	vals = it.RangeValues(metric.Interval{insertStart, now})
	// We get 59 values here because the clientmodel.Now() is slightly later
	// than our now.
	if len(vals) != 59 {
		t.Errorf("expected 59 values but got %d", len(vals))
	}
	if expt := now.Add(-1 * time.Hour).Add(time.Minute); vals[0].Timestamp != expt {
		t.Errorf("unexpected timestamp for first sample: %v, expected %v", vals[0].Timestamp.Time(), expt.Time())
	}

	vals = it.BoundaryValues(metric.Interval{insertStart, now})
	if len(vals) != 2 {
		t.Errorf("expected 2 values but got %d", len(vals))
	}
	if expt := now.Add(-1 * time.Hour).Add(time.Minute); vals[0].Timestamp != expt {
		t.Errorf("unexpected timestamp for first sample: %v, expected %v", vals[0].Timestamp.Time(), expt.Time())
	}
}

// TestLoop is just a smoke test for the loop method, if we can switch it on and
// off without disaster.
func TestLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}
	samples := make(clientmodel.Samples, 1000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(2 * i),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	directory := testutil.NewTemporaryDirectory("test_storage", t)
	defer directory.Close()
	o := &MemorySeriesStorageOptions{
		MemoryChunks:               50,
		MaxChunksToPersist:         1000000,
		PersistenceRetentionPeriod: 24 * 7 * time.Hour,
		PersistenceStoragePath:     directory.Path(),
		CheckpointInterval:         250 * time.Millisecond,
		SyncStrategy:               Adaptive,
	}
	storage := NewMemorySeriesStorage(o)
	if err := storage.Start(); err != nil {
		t.Fatalf("Error starting storage: %s", err)
	}
	for _, s := range samples {
		storage.Append(s)
	}
	storage.WaitForIndexing()
	series, _ := storage.(*memorySeriesStorage).fpToSeries.get(clientmodel.Metric{}.FastFingerprint())
	cdsBefore := len(series.chunkDescs)
	time.Sleep(fpMaxWaitDuration + time.Second) // TODO(beorn7): Ugh, need to wait for maintenance to kick in.
	cdsAfter := len(series.chunkDescs)
	storage.Stop()
	if cdsBefore <= cdsAfter {
		t.Errorf(
			"Number of chunk descriptors should have gone down by now. Got before %d, after %d.",
			cdsBefore, cdsAfter,
		)
	}
}

func testChunk(t *testing.T, encoding chunkEncoding) {
	samples := make(clientmodel.Samples, 500000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(i),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(t, encoding)
	defer closer.Close()

	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	for m := range s.fpToSeries.iter() {
		s.fpLocker.Lock(m.fp)

		var values metric.Values
		for _, cd := range m.series.chunkDescs {
			if cd.isEvicted() {
				continue
			}
			for sample := range cd.c.newIterator().values() {
				values = append(values, *sample)
			}
		}

		for i, v := range values {
			if samples[i].Timestamp != v.Timestamp {
				t.Errorf("%d. Got %v; want %v", i, v.Timestamp, samples[i].Timestamp)
			}
			if samples[i].Value != v.Value {
				t.Errorf("%d. Got %v; want %v", i, v.Value, samples[i].Value)
			}
		}
		s.fpLocker.Unlock(m.fp)
	}
	log.Info("test done, closing")
}

func TestChunkType0(t *testing.T) {
	testChunk(t, 0)
}

func TestChunkType1(t *testing.T) {
	testChunk(t, 1)
}

func testValueAtTime(t *testing.T, encoding chunkEncoding) {
	samples := make(clientmodel.Samples, 10000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(2 * i),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(t, encoding)
	defer closer.Close()

	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	fp := clientmodel.Metric{}.FastFingerprint()

	it := s.NewIterator(fp)

	// #1 Exactly on a sample.
	for i, expected := range samples {
		actual := it.ValueAtTime(expected.Timestamp)

		if len(actual) != 1 {
			t.Fatalf("1.%d. Expected exactly one result, got %d.", i, len(actual))
		}
		if expected.Timestamp != actual[0].Timestamp {
			t.Errorf("1.%d. Got %v; want %v", i, actual[0].Timestamp, expected.Timestamp)
		}
		if expected.Value != actual[0].Value {
			t.Errorf("1.%d. Got %v; want %v", i, actual[0].Value, expected.Value)
		}
	}

	// #2 Between samples.
	for i, expected1 := range samples {
		if i == len(samples)-1 {
			continue
		}
		expected2 := samples[i+1]
		actual := it.ValueAtTime(expected1.Timestamp + 1)

		if len(actual) != 2 {
			t.Fatalf("2.%d. Expected exactly 2 results, got %d.", i, len(actual))
		}
		if expected1.Timestamp != actual[0].Timestamp {
			t.Errorf("2.%d. Got %v; want %v", i, actual[0].Timestamp, expected1.Timestamp)
		}
		if expected1.Value != actual[0].Value {
			t.Errorf("2.%d. Got %v; want %v", i, actual[0].Value, expected1.Value)
		}
		if expected2.Timestamp != actual[1].Timestamp {
			t.Errorf("2.%d. Got %v; want %v", i, actual[1].Timestamp, expected1.Timestamp)
		}
		if expected2.Value != actual[1].Value {
			t.Errorf("2.%d. Got %v; want %v", i, actual[1].Value, expected1.Value)
		}
	}

	// #3 Corner cases: Just before the first sample, just after the last.
	expected := samples[0]
	actual := it.ValueAtTime(expected.Timestamp - 1)
	if len(actual) != 1 {
		t.Fatalf("3.1. Expected exactly one result, got %d.", len(actual))
	}
	if expected.Timestamp != actual[0].Timestamp {
		t.Errorf("3.1. Got %v; want %v", actual[0].Timestamp, expected.Timestamp)
	}
	if expected.Value != actual[0].Value {
		t.Errorf("3.1. Got %v; want %v", actual[0].Value, expected.Value)
	}
	expected = samples[len(samples)-1]
	actual = it.ValueAtTime(expected.Timestamp + 1)
	if len(actual) != 1 {
		t.Fatalf("3.2. Expected exactly one result, got %d.", len(actual))
	}
	if expected.Timestamp != actual[0].Timestamp {
		t.Errorf("3.2. Got %v; want %v", actual[0].Timestamp, expected.Timestamp)
	}
	if expected.Value != actual[0].Value {
		t.Errorf("3.2. Got %v; want %v", actual[0].Value, expected.Value)
	}
}

func TestValueAtTimeChunkType0(t *testing.T) {
	testValueAtTime(t, 0)
}

func TestValueAtTimeChunkType1(t *testing.T) {
	testValueAtTime(t, 1)
}

func benchmarkValueAtTime(b *testing.B, encoding chunkEncoding) {
	samples := make(clientmodel.Samples, 10000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(2 * i),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(b, encoding)
	defer closer.Close()

	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	fp := clientmodel.Metric{}.FastFingerprint()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		it := s.NewIterator(fp)

		// #1 Exactly on a sample.
		for i, expected := range samples {
			actual := it.ValueAtTime(expected.Timestamp)

			if len(actual) != 1 {
				b.Fatalf("1.%d. Expected exactly one result, got %d.", i, len(actual))
			}
			if expected.Timestamp != actual[0].Timestamp {
				b.Errorf("1.%d. Got %v; want %v", i, actual[0].Timestamp, expected.Timestamp)
			}
			if expected.Value != actual[0].Value {
				b.Errorf("1.%d. Got %v; want %v", i, actual[0].Value, expected.Value)
			}
		}

		// #2 Between samples.
		for i, expected1 := range samples {
			if i == len(samples)-1 {
				continue
			}
			expected2 := samples[i+1]
			actual := it.ValueAtTime(expected1.Timestamp + 1)

			if len(actual) != 2 {
				b.Fatalf("2.%d. Expected exactly 2 results, got %d.", i, len(actual))
			}
			if expected1.Timestamp != actual[0].Timestamp {
				b.Errorf("2.%d. Got %v; want %v", i, actual[0].Timestamp, expected1.Timestamp)
			}
			if expected1.Value != actual[0].Value {
				b.Errorf("2.%d. Got %v; want %v", i, actual[0].Value, expected1.Value)
			}
			if expected2.Timestamp != actual[1].Timestamp {
				b.Errorf("2.%d. Got %v; want %v", i, actual[1].Timestamp, expected1.Timestamp)
			}
			if expected2.Value != actual[1].Value {
				b.Errorf("2.%d. Got %v; want %v", i, actual[1].Value, expected1.Value)
			}
		}
	}
}

func BenchmarkValueAtTimeChunkType0(b *testing.B) {
	benchmarkValueAtTime(b, 0)
}

func BenchmarkValueAtTimeChunkType1(b *testing.B) {
	benchmarkValueAtTime(b, 1)
}

func testRangeValues(t *testing.T, encoding chunkEncoding) {
	samples := make(clientmodel.Samples, 10000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(2 * i),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(t, encoding)
	defer closer.Close()

	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	fp := clientmodel.Metric{}.FastFingerprint()

	it := s.NewIterator(fp)

	// #1 Zero length interval at sample.
	for i, expected := range samples {
		actual := it.RangeValues(metric.Interval{
			OldestInclusive: expected.Timestamp,
			NewestInclusive: expected.Timestamp,
		})

		if len(actual) != 1 {
			t.Fatalf("1.%d. Expected exactly one result, got %d.", i, len(actual))
		}
		if expected.Timestamp != actual[0].Timestamp {
			t.Errorf("1.%d. Got %v; want %v.", i, actual[0].Timestamp, expected.Timestamp)
		}
		if expected.Value != actual[0].Value {
			t.Errorf("1.%d. Got %v; want %v.", i, actual[0].Value, expected.Value)
		}
	}

	// #2 Zero length interval off sample.
	for i, expected := range samples {
		actual := it.RangeValues(metric.Interval{
			OldestInclusive: expected.Timestamp + 1,
			NewestInclusive: expected.Timestamp + 1,
		})

		if len(actual) != 0 {
			t.Fatalf("2.%d. Expected no result, got %d.", i, len(actual))
		}
	}

	// #3 2sec interval around sample.
	for i, expected := range samples {
		actual := it.RangeValues(metric.Interval{
			OldestInclusive: expected.Timestamp - 1,
			NewestInclusive: expected.Timestamp + 1,
		})

		if len(actual) != 1 {
			t.Fatalf("3.%d. Expected exactly one result, got %d.", i, len(actual))
		}
		if expected.Timestamp != actual[0].Timestamp {
			t.Errorf("3.%d. Got %v; want %v.", i, actual[0].Timestamp, expected.Timestamp)
		}
		if expected.Value != actual[0].Value {
			t.Errorf("3.%d. Got %v; want %v.", i, actual[0].Value, expected.Value)
		}
	}

	// #4 2sec interval sample to sample.
	for i, expected1 := range samples {
		if i == len(samples)-1 {
			continue
		}
		expected2 := samples[i+1]
		actual := it.RangeValues(metric.Interval{
			OldestInclusive: expected1.Timestamp,
			NewestInclusive: expected1.Timestamp + 2,
		})

		if len(actual) != 2 {
			t.Fatalf("4.%d. Expected exactly 2 results, got %d.", i, len(actual))
		}
		if expected1.Timestamp != actual[0].Timestamp {
			t.Errorf("4.%d. Got %v for 1st result; want %v.", i, actual[0].Timestamp, expected1.Timestamp)
		}
		if expected1.Value != actual[0].Value {
			t.Errorf("4.%d. Got %v for 1st result; want %v.", i, actual[0].Value, expected1.Value)
		}
		if expected2.Timestamp != actual[1].Timestamp {
			t.Errorf("4.%d. Got %v for 2nd result; want %v.", i, actual[1].Timestamp, expected2.Timestamp)
		}
		if expected2.Value != actual[1].Value {
			t.Errorf("4.%d. Got %v for 2nd result; want %v.", i, actual[1].Value, expected2.Value)
		}
	}

	// #5 corner cases: Interval ends at first sample, interval starts
	// at last sample, interval entirely before/after samples.
	expected := samples[0]
	actual := it.RangeValues(metric.Interval{
		OldestInclusive: expected.Timestamp - 2,
		NewestInclusive: expected.Timestamp,
	})
	if len(actual) != 1 {
		t.Fatalf("5.1. Expected exactly one result, got %d.", len(actual))
	}
	if expected.Timestamp != actual[0].Timestamp {
		t.Errorf("5.1. Got %v; want %v.", actual[0].Timestamp, expected.Timestamp)
	}
	if expected.Value != actual[0].Value {
		t.Errorf("5.1. Got %v; want %v.", actual[0].Value, expected.Value)
	}
	expected = samples[len(samples)-1]
	actual = it.RangeValues(metric.Interval{
		OldestInclusive: expected.Timestamp,
		NewestInclusive: expected.Timestamp + 2,
	})
	if len(actual) != 1 {
		t.Fatalf("5.2. Expected exactly one result, got %d.", len(actual))
	}
	if expected.Timestamp != actual[0].Timestamp {
		t.Errorf("5.2. Got %v; want %v.", actual[0].Timestamp, expected.Timestamp)
	}
	if expected.Value != actual[0].Value {
		t.Errorf("5.2. Got %v; want %v.", actual[0].Value, expected.Value)
	}
	firstSample := samples[0]
	actual = it.RangeValues(metric.Interval{
		OldestInclusive: firstSample.Timestamp - 4,
		NewestInclusive: firstSample.Timestamp - 2,
	})
	if len(actual) != 0 {
		t.Fatalf("5.3. Expected no results, got %d.", len(actual))
	}
	lastSample := samples[len(samples)-1]
	actual = it.RangeValues(metric.Interval{
		OldestInclusive: lastSample.Timestamp + 2,
		NewestInclusive: lastSample.Timestamp + 4,
	})
	if len(actual) != 0 {
		t.Fatalf("5.3. Expected no results, got %d.", len(actual))
	}
}

func TestRangeValuesChunkType0(t *testing.T) {
	testRangeValues(t, 0)
}

func TestRangeValuesChunkType1(t *testing.T) {
	testRangeValues(t, 1)
}

func benchmarkRangeValues(b *testing.B, encoding chunkEncoding) {
	samples := make(clientmodel.Samples, 10000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(2 * i),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(b, encoding)
	defer closer.Close()

	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	fp := clientmodel.Metric{}.FastFingerprint()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {

		it := s.NewIterator(fp)

		for _, sample := range samples {
			actual := it.RangeValues(metric.Interval{
				OldestInclusive: sample.Timestamp - 20,
				NewestInclusive: sample.Timestamp + 20,
			})

			if len(actual) < 10 {
				b.Fatalf("not enough samples found")
			}
		}
	}
}

func BenchmarkRangeValuesChunkType0(b *testing.B) {
	benchmarkRangeValues(b, 0)
}

func BenchmarkRangeValuesChunkType1(b *testing.B) {
	benchmarkRangeValues(b, 1)
}

func testEvictAndPurgeSeries(t *testing.T, encoding chunkEncoding) {
	samples := make(clientmodel.Samples, 10000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(2 * i),
			Value:     clientmodel.SampleValue(float64(i * i)),
		}
	}
	s, closer := NewTestStorage(t, encoding)
	defer closer.Close()

	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	fp := clientmodel.Metric{}.FastFingerprint()

	// Drop ~half of the chunks.
	s.maintainMemorySeries(fp, 10000)
	it := s.NewIterator(fp)
	actual := it.BoundaryValues(metric.Interval{
		OldestInclusive: 0,
		NewestInclusive: 100000,
	})
	if len(actual) != 2 {
		t.Fatal("expected two results after purging half of series")
	}
	if actual[0].Timestamp < 6000 || actual[0].Timestamp > 10000 {
		t.Errorf("1st timestamp out of expected range: %v", actual[0].Timestamp)
	}
	want := clientmodel.Timestamp(19998)
	if actual[1].Timestamp != want {
		t.Errorf("2nd timestamp: want %v, got %v", want, actual[1].Timestamp)
	}

	// Drop everything.
	s.maintainMemorySeries(fp, 100000)
	it = s.NewIterator(fp)
	actual = it.BoundaryValues(metric.Interval{
		OldestInclusive: 0,
		NewestInclusive: 100000,
	})
	if len(actual) != 0 {
		t.Fatal("expected zero results after purging the whole series")
	}

	// Recreate series.
	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	series, ok := s.fpToSeries.get(fp)
	if !ok {
		t.Fatal("could not find series")
	}

	// Persist head chunk so we can safely archive.
	series.headChunkClosed = true
	s.maintainMemorySeries(fp, clientmodel.Earliest)

	// Archive metrics.
	s.fpToSeries.del(fp)
	if err := s.persistence.archiveMetric(
		fp, series.metric, series.firstTime(), series.head().lastTime(),
	); err != nil {
		t.Fatal(err)
	}

	archived, _, _, err := s.persistence.hasArchivedMetric(fp)
	if err != nil {
		t.Fatal(err)
	}
	if !archived {
		t.Fatal("not archived")
	}

	// Drop ~half of the chunks of an archived series.
	s.maintainArchivedSeries(fp, 10000)
	archived, _, _, err = s.persistence.hasArchivedMetric(fp)
	if err != nil {
		t.Fatal(err)
	}
	if !archived {
		t.Fatal("archived series purged although only half of the chunks dropped")
	}

	// Drop everything.
	s.maintainArchivedSeries(fp, 100000)
	archived, _, _, err = s.persistence.hasArchivedMetric(fp)
	if err != nil {
		t.Fatal(err)
	}
	if archived {
		t.Fatal("archived series not dropped")
	}

	// Recreate series.
	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	series, ok = s.fpToSeries.get(fp)
	if !ok {
		t.Fatal("could not find series")
	}

	// Persist head chunk so we can safely archive.
	series.headChunkClosed = true
	s.maintainMemorySeries(fp, clientmodel.Earliest)

	// Archive metrics.
	s.fpToSeries.del(fp)
	if err := s.persistence.archiveMetric(
		fp, series.metric, series.firstTime(), series.head().lastTime(),
	); err != nil {
		t.Fatal(err)
	}

	archived, _, _, err = s.persistence.hasArchivedMetric(fp)
	if err != nil {
		t.Fatal(err)
	}
	if !archived {
		t.Fatal("not archived")
	}

	// Unarchive metrics.
	s.getOrCreateSeries(fp, clientmodel.Metric{})

	series, ok = s.fpToSeries.get(fp)
	if !ok {
		t.Fatal("could not find series")
	}
	archived, _, _, err = s.persistence.hasArchivedMetric(fp)
	if err != nil {
		t.Fatal(err)
	}
	if archived {
		t.Fatal("archived")
	}

	// This will archive again, but must not drop it completely, despite the
	// memorySeries being empty.
	s.maintainMemorySeries(fp, 10000)
	archived, _, _, err = s.persistence.hasArchivedMetric(fp)
	if err != nil {
		t.Fatal(err)
	}
	if !archived {
		t.Fatal("series purged completely")
	}
}

func TestEvictAndPurgeSeriesChunkType0(t *testing.T) {
	testEvictAndPurgeSeries(t, 0)
}

func TestEvictAndPurgeSeriesChunkType1(t *testing.T) {
	testEvictAndPurgeSeries(t, 1)
}

func benchmarkAppend(b *testing.B, encoding chunkEncoding) {
	samples := make(clientmodel.Samples, b.N)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: clientmodel.LabelValue(fmt.Sprintf("test_metric_%d", i%10)),
				"label1":                    clientmodel.LabelValue(fmt.Sprintf("test_metric_%d", i%10)),
				"label2":                    clientmodel.LabelValue(fmt.Sprintf("test_metric_%d", i%10)),
			},
			Timestamp: clientmodel.Timestamp(i),
			Value:     clientmodel.SampleValue(i),
		}
	}
	b.ResetTimer()
	s, closer := NewTestStorage(b, encoding)
	defer closer.Close()

	for _, sample := range samples {
		s.Append(sample)
	}
}

func BenchmarkAppendType0(b *testing.B) {
	benchmarkAppend(b, 0)
}

func BenchmarkAppendType1(b *testing.B) {
	benchmarkAppend(b, 1)
}

// Append a large number of random samples and then check if we can get them out
// of the storage alright.
func testFuzz(t *testing.T, encoding chunkEncoding) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

	check := func(seed int64) bool {
		rand.Seed(seed)
		s, c := NewTestStorage(t, encoding)
		defer c.Close()

		samples := createRandomSamples("test_fuzz", 10000)
		for _, sample := range samples {
			s.Append(sample)
		}
		return verifyStorage(t, s, samples, 24*7*time.Hour)
	}

	if err := quick.Check(check, nil); err != nil {
		t.Fatal(err)
	}
}

func TestFuzzChunkType0(t *testing.T) {
	testFuzz(t, 0)
}

func TestFuzzChunkType1(t *testing.T) {
	testFuzz(t, 1)
}

// benchmarkFuzz is the benchmark version of testFuzz. The storage options are
// set such that evictions, checkpoints, and purging will happen concurrently,
// too. This benchmark will have a very long runtime (up to minutes). You can
// use it as an actual benchmark. Run it like this:
//
// go test -cpu 1,2,4,8 -run=NONE -bench BenchmarkFuzzChunkType -benchmem
//
// You can also use it as a test for races. In that case, run it like this (will
// make things even slower):
//
// go test -race -cpu 8 -short -bench BenchmarkFuzzChunkType
func benchmarkFuzz(b *testing.B, encoding chunkEncoding) {
	*defaultChunkEncoding = int(encoding)
	const samplesPerRun = 100000
	rand.Seed(42)
	directory := testutil.NewTemporaryDirectory("test_storage", b)
	defer directory.Close()
	o := &MemorySeriesStorageOptions{
		MemoryChunks:               100,
		MaxChunksToPersist:         1000000,
		PersistenceRetentionPeriod: time.Hour,
		PersistenceStoragePath:     directory.Path(),
		CheckpointInterval:         time.Second,
		SyncStrategy:               Adaptive,
	}
	s := NewMemorySeriesStorage(o)
	if err := s.Start(); err != nil {
		b.Fatalf("Error starting storage: %s", err)
	}
	s.Start()
	defer s.Stop()

	samples := createRandomSamples("benchmark_fuzz", samplesPerRun*b.N)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := samplesPerRun * i
		end := samplesPerRun * (i + 1)
		middle := (start + end) / 2
		for _, sample := range samples[start:middle] {
			s.Append(sample)
		}
		verifyStorage(b, s.(*memorySeriesStorage), samples[:middle], o.PersistenceRetentionPeriod)
		for _, sample := range samples[middle:end] {
			s.Append(sample)
		}
		verifyStorage(b, s.(*memorySeriesStorage), samples[:end], o.PersistenceRetentionPeriod)
	}
}

func BenchmarkFuzzChunkType0(b *testing.B) {
	benchmarkFuzz(b, 0)
}

func BenchmarkFuzzChunkType1(b *testing.B) {
	benchmarkFuzz(b, 1)
}

func createRandomSamples(metricName string, minLen int) clientmodel.Samples {
	type valueCreator func() clientmodel.SampleValue
	type deltaApplier func(clientmodel.SampleValue) clientmodel.SampleValue

	var (
		maxMetrics         = 5
		maxStreakLength    = 500
		maxTimeDelta       = 10000
		maxTimeDeltaFactor = 10
		timestamp          = clientmodel.Now() - clientmodel.Timestamp(maxTimeDelta*maxTimeDeltaFactor*minLen/4) // So that some timestamps are in the future.
		generators         = []struct {
			createValue valueCreator
			applyDelta  []deltaApplier
		}{
			{ // "Boolean".
				createValue: func() clientmodel.SampleValue {
					return clientmodel.SampleValue(rand.Intn(2))
				},
				applyDelta: []deltaApplier{
					func(_ clientmodel.SampleValue) clientmodel.SampleValue {
						return clientmodel.SampleValue(rand.Intn(2))
					},
				},
			},
			{ // Integer with int deltas of various byte length.
				createValue: func() clientmodel.SampleValue {
					return clientmodel.SampleValue(rand.Int63() - 1<<62)
				},
				applyDelta: []deltaApplier{
					func(v clientmodel.SampleValue) clientmodel.SampleValue {
						return clientmodel.SampleValue(rand.Intn(1<<8) - 1<<7 + int(v))
					},
					func(v clientmodel.SampleValue) clientmodel.SampleValue {
						return clientmodel.SampleValue(rand.Intn(1<<16) - 1<<15 + int(v))
					},
					func(v clientmodel.SampleValue) clientmodel.SampleValue {
						return clientmodel.SampleValue(rand.Int63n(1<<32) - 1<<31 + int64(v))
					},
				},
			},
			{ // Float with float32 and float64 deltas.
				createValue: func() clientmodel.SampleValue {
					return clientmodel.SampleValue(rand.NormFloat64())
				},
				applyDelta: []deltaApplier{
					func(v clientmodel.SampleValue) clientmodel.SampleValue {
						return v + clientmodel.SampleValue(float32(rand.NormFloat64()))
					},
					func(v clientmodel.SampleValue) clientmodel.SampleValue {
						return v + clientmodel.SampleValue(rand.NormFloat64())
					},
				},
			},
		}
	)

	// Prefill result with two samples with colliding metrics (to test fingerprint mapping).
	result := clientmodel.Samples{
		&clientmodel.Sample{
			Metric: clientmodel.Metric{
				"instance": "ip-10-33-84-73.l05.ams5.s-cloud.net:24483",
				"status":   "503",
			},
			Value:     42,
			Timestamp: timestamp,
		},
		&clientmodel.Sample{
			Metric: clientmodel.Metric{
				"instance": "ip-10-33-84-73.l05.ams5.s-cloud.net:24480",
				"status":   "500",
			},
			Value:     2010,
			Timestamp: timestamp + 1,
		},
	}

	metrics := []clientmodel.Metric{}
	for n := rand.Intn(maxMetrics); n >= 0; n-- {
		metrics = append(metrics, clientmodel.Metric{
			clientmodel.MetricNameLabel:                             clientmodel.LabelValue(metricName),
			clientmodel.LabelName(fmt.Sprintf("labelname_%d", n+1)): clientmodel.LabelValue(fmt.Sprintf("labelvalue_%d", rand.Int())),
		})
	}

	for len(result) < minLen {
		// Pick a metric for this cycle.
		metric := metrics[rand.Intn(len(metrics))]
		timeDelta := rand.Intn(maxTimeDelta) + 1
		generator := generators[rand.Intn(len(generators))]
		createValue := generator.createValue
		applyDelta := generator.applyDelta[rand.Intn(len(generator.applyDelta))]
		incTimestamp := func() { timestamp += clientmodel.Timestamp(timeDelta * (rand.Intn(maxTimeDeltaFactor) + 1)) }
		switch rand.Intn(4) {
		case 0: // A single sample.
			result = append(result, &clientmodel.Sample{
				Metric:    metric,
				Value:     createValue(),
				Timestamp: timestamp,
			})
			incTimestamp()
		case 1: // A streak of random sample values.
			for n := rand.Intn(maxStreakLength); n >= 0; n-- {
				result = append(result, &clientmodel.Sample{
					Metric:    metric,
					Value:     createValue(),
					Timestamp: timestamp,
				})
				incTimestamp()
			}
		case 2: // A streak of sample values with incremental changes.
			value := createValue()
			for n := rand.Intn(maxStreakLength); n >= 0; n-- {
				result = append(result, &clientmodel.Sample{
					Metric:    metric,
					Value:     value,
					Timestamp: timestamp,
				})
				incTimestamp()
				value = applyDelta(value)
			}
		case 3: // A streak of constant sample values.
			value := createValue()
			for n := rand.Intn(maxStreakLength); n >= 0; n-- {
				result = append(result, &clientmodel.Sample{
					Metric:    metric,
					Value:     value,
					Timestamp: timestamp,
				})
				incTimestamp()
			}
		}
	}

	return result
}

func verifyStorage(t testing.TB, s *memorySeriesStorage, samples clientmodel.Samples, maxAge time.Duration) bool {
	s.WaitForIndexing()
	result := true
	for _, i := range rand.Perm(len(samples)) {
		sample := samples[i]
		if sample.Timestamp.Before(clientmodel.TimestampFromTime(time.Now().Add(-maxAge))) {
			continue
			// TODO: Once we have a guaranteed cutoff at the
			// retention period, we can verify here that no results
			// are returned.
		}
		fp, err := s.mapper.mapFP(sample.Metric.FastFingerprint(), sample.Metric)
		if err != nil {
			t.Fatal(err)
		}
		p := s.NewPreloader()
		p.PreloadRange(fp, sample.Timestamp, sample.Timestamp, time.Hour)
		found := s.NewIterator(fp).ValueAtTime(sample.Timestamp)
		if len(found) != 1 {
			t.Errorf("Sample %#v: Expected exactly one value, found %d.", sample, len(found))
			result = false
			p.Close()
			continue
		}
		want := sample.Value
		got := found[0].Value
		if want != got || sample.Timestamp != found[0].Timestamp {
			t.Errorf(
				"Value (or timestamp) mismatch, want %f (at time %v), got %f (at time %v).",
				want, sample.Timestamp, got, found[0].Timestamp,
			)
			result = false
		}
		p.Close()
	}
	return result
}
