// Copyright 2014 Prometheus Team
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

	clientmodel "github.com/prometheus/client_golang/model"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility/test"
)

func TestGetFingerprintsForLabelMatchers(t *testing.T) {

}

// TestLoop is just a smoke test for the loop method, if we can switch it on and
// off without disaster.
func TestLoop(t *testing.T) {
	samples := make(clientmodel.Samples, 1000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(2 * i),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	directory := test.NewTemporaryDirectory("test_storage", t)
	defer directory.Close()
	o := &MemorySeriesStorageOptions{
		MemoryEvictionInterval:     100 * time.Millisecond,
		MemoryRetentionPeriod:      time.Hour,
		PersistenceRetentionPeriod: 24 * 7 * time.Hour,
		PersistenceStoragePath:     directory.Path(),
		CheckpointInterval:         250 * time.Millisecond,
	}
	storage, err := NewMemorySeriesStorage(o)
	if err != nil {
		t.Fatalf("Error creating storage: %s", err)
	}
	storage.Start()
	storage.AppendSamples(samples)
	time.Sleep(time.Second)
	storage.Stop()
}

func TestChunk(t *testing.T) {
	samples := make(clientmodel.Samples, 500000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(i),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(t)
	defer closer.Close()

	s.AppendSamples(samples)

	for m := range s.(*memorySeriesStorage).fpToSeries.iter() {
		s.(*memorySeriesStorage).fpLocker.Lock(m.fp)
		for i, v := range m.series.values() {
			if samples[i].Timestamp != v.Timestamp {
				t.Fatalf("%d. Got %v; want %v", i, v.Timestamp, samples[i].Timestamp)
			}
			if samples[i].Value != v.Value {
				t.Fatalf("%d. Got %v; want %v", i, v.Value, samples[i].Value)
			}
		}
		s.(*memorySeriesStorage).fpLocker.Unlock(m.fp)
	}
}

func TestGetValueAtTime(t *testing.T) {
	samples := make(clientmodel.Samples, 1000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(2 * i),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(t)
	defer closer.Close()

	s.AppendSamples(samples)

	fp := clientmodel.Metric{}.Fingerprint()

	it := s.NewIterator(fp)

	// #1 Exactly on a sample.
	for i, expected := range samples {
		actual := it.GetValueAtTime(expected.Timestamp)

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
		actual := it.GetValueAtTime(expected1.Timestamp + 1)

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
	actual := it.GetValueAtTime(expected.Timestamp - 1)
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
	actual = it.GetValueAtTime(expected.Timestamp + 1)
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

func TestGetRangeValues(t *testing.T) {
	samples := make(clientmodel.Samples, 1000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(2 * i),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(t)
	defer closer.Close()

	s.AppendSamples(samples)

	fp := clientmodel.Metric{}.Fingerprint()

	it := s.NewIterator(fp)

	// #1 Zero length interval at sample.
	for i, expected := range samples {
		actual := it.GetRangeValues(metric.Interval{
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
		actual := it.GetRangeValues(metric.Interval{
			OldestInclusive: expected.Timestamp + 1,
			NewestInclusive: expected.Timestamp + 1,
		})

		if len(actual) != 0 {
			t.Fatalf("2.%d. Expected no result, got %d.", i, len(actual))
		}
	}

	// #3 2sec interval around sample.
	for i, expected := range samples {
		actual := it.GetRangeValues(metric.Interval{
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
		actual := it.GetRangeValues(metric.Interval{
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
	actual := it.GetRangeValues(metric.Interval{
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
	actual = it.GetRangeValues(metric.Interval{
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
	actual = it.GetRangeValues(metric.Interval{
		OldestInclusive: firstSample.Timestamp - 4,
		NewestInclusive: firstSample.Timestamp - 2,
	})
	if len(actual) != 0 {
		t.Fatalf("5.3. Expected no results, got %d.", len(actual))
	}
	lastSample := samples[len(samples)-1]
	actual = it.GetRangeValues(metric.Interval{
		OldestInclusive: lastSample.Timestamp + 2,
		NewestInclusive: lastSample.Timestamp + 4,
	})
	if len(actual) != 0 {
		t.Fatalf("5.3. Expected no results, got %d.", len(actual))
	}
}

func TestEvictAndPurgeSeries(t *testing.T) {
	samples := make(clientmodel.Samples, 1000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(2 * i),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(t)
	defer closer.Close()

	ms := s.(*memorySeriesStorage) // Going to test the internal purgeSeries method.

	s.AppendSamples(samples)

	fp := clientmodel.Metric{}.Fingerprint()

	// Purge ~half of the chunks.
	ms.purgeSeries(fp, 1000)
	it := s.NewIterator(fp)
	actual := it.GetBoundaryValues(metric.Interval{
		OldestInclusive: 0,
		NewestInclusive: 10000,
	})
	if len(actual) != 2 {
		t.Fatal("expected two results after purging half of series")
	}
	if actual[0].Timestamp < 800 || actual[0].Timestamp > 1000 {
		t.Errorf("1st timestamp out of expected range: %v", actual[0].Timestamp)
	}
	want := clientmodel.Timestamp(1998)
	if actual[1].Timestamp != want {
		t.Errorf("2nd timestamp: want %v, got %v", want, actual[1].Timestamp)
	}

	// Purge everything.
	ms.purgeSeries(fp, 10000)
	it = s.NewIterator(fp)
	actual = it.GetBoundaryValues(metric.Interval{
		OldestInclusive: 0,
		NewestInclusive: 10000,
	})
	if len(actual) != 0 {
		t.Fatal("expected zero results after purging the whole series")
	}

	// Recreate series.
	s.AppendSamples(samples)

	series, ok := ms.fpToSeries.get(fp)
	if !ok {
		t.Fatal("could not find series")
	}

	// Evict everything except head chunk.
	allEvicted, headChunkToPersist := series.evictOlderThan(1998)
	// Head chunk not yet old enough, should get false, false:
	if allEvicted {
		t.Error("allEvicted with head chunk not yet old enough")
	}
	if headChunkToPersist != nil {
		t.Error("persistHeadChunk is not nil although head chunk is not old enough")
	}
	// Evict everything.
	allEvicted, headChunkToPersist = series.evictOlderThan(10000)
	// Since the head chunk is not yet persisted, we should get false, true:
	if allEvicted {
		t.Error("allEvicted with head chuk not yet persisted")
	}
	if headChunkToPersist == nil {
		t.Error("headChunkToPersist is nil although head chunk is old enough")
	}
	// Persist head chunk as requested.
	ms.persistQueue <- persistRequest{fp, series.head()}
	time.Sleep(time.Second) // Give time for persisting to happen.
	allEvicted, headChunkToPersist = series.evictOlderThan(10000)
	// Now we should really see everything gone.
	if !allEvicted {
		t.Error("not allEvicted")
	}
	if headChunkToPersist != nil {
		t.Error("headChunkToPersist is not nil although already persisted")
	}

	// Now archive as it would usually be done in the evictTicker loop.
	ms.fpToSeries.del(fp)
	if err := ms.persistence.archiveMetric(
		fp, series.metric, series.firstTime(), series.lastTime(),
	); err != nil {
		t.Fatal(err)
	}

	archived, _, _, err := ms.persistence.hasArchivedMetric(fp)
	if err != nil {
		t.Fatal(err)
	}
	if !archived {
		t.Fatal("not archived")
	}

	// Purge ~half of the chunks of an archived series.
	ms.purgeSeries(fp, 1000)
	archived, _, _, err = ms.persistence.hasArchivedMetric(fp)
	if err != nil {
		t.Fatal(err)
	}
	if !archived {
		t.Fatal("archived series dropped although only half of the chunks purged")
	}

	// Purge everything.
	ms.purgeSeries(fp, 10000)
	archived, _, _, err = ms.persistence.hasArchivedMetric(fp)
	if err != nil {
		t.Fatal(err)
	}
	if archived {
		t.Fatal("archived series not dropped")
	}
}

func BenchmarkAppend(b *testing.B) {
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
	s, closer := NewTestStorage(b)
	defer closer.Close()

	s.AppendSamples(samples)
}

// Append a large number of random samples and then check if we can get them out
// of the storage alright.
func TestFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

	check := func(seed int64) bool {
		rand.Seed(seed)
		s, c := NewTestStorage(t)
		defer c.Close()

		samples := createRandomSamples()
		s.AppendSamples(samples)

		return verifyStorage(t, s, samples, 24*7*time.Hour)
	}

	if err := quick.Check(check, nil); err != nil {
		t.Fatal(err)
	}
}

// BenchmarkFuzz is the benchmark version of TestFuzz. However, it will run
// several append and verify operations in parallel, if GOMAXPROC is set
// accordingly. Also, the storage options are set such that evictions,
// checkpoints, and purging will happen concurrently, too. This benchmark will
// have a very long runtime (up to minutes). You can use it as an actual
// benchmark. Run it like this:
//
// go test -cpu 1,2,4,8 -short -bench BenchmarkFuzz -benchmem
//
// You can also use it as a test for races. In that case, run it like this (will
// make things even slower):
//
// go test -race -cpu 8 -short -bench BenchmarkFuzz
func BenchmarkFuzz(b *testing.B) {
	b.StopTimer()
	rand.Seed(42)
	directory := test.NewTemporaryDirectory("test_storage", b)
	defer directory.Close()
	o := &MemorySeriesStorageOptions{
		MemoryEvictionInterval:     time.Second,
		MemoryRetentionPeriod:      10 * time.Minute,
		PersistenceRetentionPeriod: time.Hour,
		PersistenceStoragePath:     directory.Path(),
		CheckpointInterval:         3 * time.Second,
	}
	s, err := NewMemorySeriesStorage(o)
	if err != nil {
		b.Fatalf("Error creating storage: %s", err)
	}
	s.Start()
	defer s.Stop()
	b.StartTimer()

	b.RunParallel(func(pb *testing.PB) {
		var allSamples clientmodel.Samples
		for pb.Next() {
			newSamples := createRandomSamples()
			allSamples = append(allSamples, newSamples[:len(newSamples)/2]...)
			s.AppendSamples(newSamples[:len(newSamples)/2])
			verifyStorage(b, s, allSamples, o.PersistenceRetentionPeriod)
			allSamples = append(allSamples, newSamples[len(newSamples)/2:]...)
			s.AppendSamples(newSamples[len(newSamples)/2:])
			verifyStorage(b, s, allSamples, o.PersistenceRetentionPeriod)
		}
	})
}

func createRandomSamples() clientmodel.Samples {
	type valueCreator func() clientmodel.SampleValue
	type deltaApplier func(clientmodel.SampleValue) clientmodel.SampleValue

	var (
		maxMetrics         = 5
		maxCycles          = 500
		maxStreakLength    = 500
		maxTimeDelta       = 1000
		maxTimeDeltaFactor = 10
		timestamp          = clientmodel.Now() - clientmodel.Timestamp(maxTimeDelta*maxTimeDeltaFactor*maxCycles*maxStreakLength/16) // So that some timestamps are in the future.
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
						return clientmodel.SampleValue(rand.Intn(1<<32) - 1<<31 + int(v))
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

	result := clientmodel.Samples{}

	metrics := []clientmodel.Metric{}
	for n := rand.Intn(maxMetrics); n >= 0; n-- {
		metrics = append(metrics, clientmodel.Metric{
			clientmodel.LabelName(fmt.Sprintf("labelname_%d", n+1)): clientmodel.LabelValue(fmt.Sprintf("labelvalue_%d", rand.Int())),
		})
	}

	for n := rand.Intn(maxCycles); n >= 0; n-- {
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

func verifyStorage(t testing.TB, s Storage, samples clientmodel.Samples, maxAge time.Duration) bool {
	result := true
	for _, i := range rand.Perm(len(samples)) {
		sample := samples[i]
		if sample.Timestamp.Before(clientmodel.TimestampFromTime(time.Now().Add(-maxAge))) {
			continue
			// TODO: Once we have a guaranteed cutoff at the
			// retention period, we can verify here that no results
			// are returned.
		}
		fp := sample.Metric.Fingerprint()
		p := s.NewPreloader()
		p.PreloadRange(fp, sample.Timestamp, sample.Timestamp, time.Hour)
		found := s.NewIterator(fp).GetValueAtTime(sample.Timestamp)
		if len(found) != 1 {
			t.Errorf("Sample %#v: Expected exactly one value, found %d.", sample, len(found))
			result = false
			p.Close()
			continue
		}
		want := float64(sample.Value)
		got := float64(found[0].Value)
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
