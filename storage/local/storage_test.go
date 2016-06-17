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
	"hash/fnv"
	"math"
	"math/rand"
	"os"
	"testing"
	"testing/quick"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMatches(t *testing.T) {
	storage, closer := NewTestStorage(t, 2)
	defer closer.Close()

	storage.archiveHighWatermark = 90
	samples := make([]*model.Sample, 100)
	fingerprints := make(model.Fingerprints, 100)

	for i := range samples {
		metric := model.Metric{
			model.MetricNameLabel: model.LabelValue(fmt.Sprintf("test_metric_%d", i)),
			"label1":              model.LabelValue(fmt.Sprintf("test_%d", i/10)),
			"label2":              model.LabelValue(fmt.Sprintf("test_%d", (i+5)/10)),
			"all":                 "const",
		}
		samples[i] = &model.Sample{
			Metric:    metric,
			Timestamp: model.Time(i),
			Value:     model.SampleValue(i),
		}
		fingerprints[i] = metric.FastFingerprint()
	}
	for _, s := range samples {
		storage.Append(s)
	}
	storage.WaitForIndexing()

	// Archive every tenth metric.
	for i, fp := range fingerprints {
		if i%10 != 0 {
			continue
		}
		s, ok := storage.fpToSeries.get(fp)
		if !ok {
			t.Fatal("could not retrieve series for fp", fp)
		}
		storage.fpLocker.Lock(fp)
		storage.persistence.archiveMetric(fp, s.metric, s.firstTime(), s.lastTime)
		storage.fpLocker.Unlock(fp)
	}

	newMatcher := func(matchType metric.MatchType, name model.LabelName, value model.LabelValue) *metric.LabelMatcher {
		lm, err := metric.NewLabelMatcher(matchType, name, value)
		if err != nil {
			t.Fatalf("error creating label matcher: %s", err)
		}
		return lm
	}

	var matcherTests = []struct {
		matchers metric.LabelMatchers
		expected model.Fingerprints
	}{
		{
			matchers: metric.LabelMatchers{newMatcher(metric.Equal, "label1", "x")},
			expected: model.Fingerprints{},
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
			matchers: metric.LabelMatchers{
				newMatcher(metric.Equal, "all", "const"),
				newMatcher(metric.NotEqual, "label1", "x"),
			},
			expected: fingerprints,
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.Equal, "all", "const"),
				newMatcher(metric.NotEqual, "label1", "test_0"),
			},
			expected: fingerprints[10:],
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.Equal, "all", "const"),
				newMatcher(metric.NotEqual, "label1", "test_0"),
				newMatcher(metric.NotEqual, "label1", "test_1"),
				newMatcher(metric.NotEqual, "label1", "test_2"),
			},
			expected: fingerprints[30:],
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.Equal, "label1", ""),
			},
			expected: fingerprints[:0],
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.NotEqual, "label1", "test_0"),
				newMatcher(metric.Equal, "label1", ""),
			},
			expected: fingerprints[:0],
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.NotEqual, "label1", "test_0"),
				newMatcher(metric.Equal, "label2", ""),
			},
			expected: fingerprints[:0],
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.Equal, "all", "const"),
				newMatcher(metric.NotEqual, "label1", "test_0"),
				newMatcher(metric.Equal, "not_existant", ""),
			},
			expected: fingerprints[10:],
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.RegexMatch, "label1", `test_[3-5]`),
			},
			expected: fingerprints[30:60],
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.Equal, "all", "const"),
				newMatcher(metric.RegexNoMatch, "label1", `test_[3-5]`),
			},
			expected: append(append(model.Fingerprints{}, fingerprints[:30]...), fingerprints[60:]...),
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
			expected: append(append(model.Fingerprints{}, fingerprints[30:35]...), fingerprints[45:60]...),
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.Equal, "label1", `nonexistent`),
				newMatcher(metric.RegexMatch, "label2", `test`),
			},
			expected: model.Fingerprints{},
		},
		{
			matchers: metric.LabelMatchers{
				newMatcher(metric.Equal, "label1", `test_0`),
				newMatcher(metric.RegexMatch, "label2", `nonexistent`),
			},
			expected: model.Fingerprints{},
		},
	}

	for _, mt := range matcherTests {
		res := storage.MetricsForLabelMatchers(
			model.Earliest, model.Latest,
			mt.matchers...,
		)
		if len(mt.expected) != len(res) {
			t.Fatalf("expected %d matches for %q, found %d", len(mt.expected), mt.matchers, len(res))
		}
		for fp1 := range res {
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
		// Smoketest for from/through.
		if len(storage.MetricsForLabelMatchers(
			model.Earliest, -10000,
			mt.matchers...,
		)) > 0 {
			t.Error("expected no matches with 'through' older than any sample")
		}
		if len(storage.MetricsForLabelMatchers(
			10000, model.Latest,
			mt.matchers...,
		)) > 0 {
			t.Error("expected no matches with 'from' newer than any sample")
		}
		// Now the tricky one, cut out something from the middle.
		var (
			from    model.Time = 25
			through model.Time = 75
		)
		res = storage.MetricsForLabelMatchers(
			from, through,
			mt.matchers...,
		)
		expected := model.Fingerprints{}
		for _, fp := range mt.expected {
			i := 0
			for ; fingerprints[i] != fp && i < len(fingerprints); i++ {
			}
			if i == len(fingerprints) {
				t.Fatal("expected fingerprint does not exist")
			}
			if !model.Time(i).Before(from) && !model.Time(i).After(through) {
				expected = append(expected, fp)
			}
		}
		if len(expected) != len(res) {
			t.Errorf("expected %d range-limited matches for %q, found %d", len(expected), mt.matchers, len(res))
		}
		for fp1 := range res {
			found := false
			for _, fp2 := range expected {
				if fp1 == fp2 {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected fingerprint %s for %q not in range-limited result", fp1, mt.matchers)
			}
		}

	}
}

func TestFingerprintsForLabels(t *testing.T) {
	storage, closer := NewTestStorage(t, 2)
	defer closer.Close()

	samples := make([]*model.Sample, 100)
	fingerprints := make(model.Fingerprints, 100)

	for i := range samples {
		metric := model.Metric{
			model.MetricNameLabel: model.LabelValue(fmt.Sprintf("test_metric_%d", i)),
			"label1":              model.LabelValue(fmt.Sprintf("test_%d", i/10)),
			"label2":              model.LabelValue(fmt.Sprintf("test_%d", (i+5)/10)),
		}
		samples[i] = &model.Sample{
			Metric:    metric,
			Timestamp: model.Time(i),
			Value:     model.SampleValue(i),
		}
		fingerprints[i] = metric.FastFingerprint()
	}
	for _, s := range samples {
		storage.Append(s)
	}
	storage.WaitForIndexing()

	var matcherTests = []struct {
		pairs    []model.LabelPair
		expected model.Fingerprints
	}{
		{
			pairs:    []model.LabelPair{{"label1", "x"}},
			expected: fingerprints[:0],
		},
		{
			pairs:    []model.LabelPair{{"label1", "test_0"}},
			expected: fingerprints[:10],
		},
		{
			pairs: []model.LabelPair{
				{"label1", "test_0"},
				{"label1", "test_1"},
			},
			expected: fingerprints[:0],
		},
		{
			pairs: []model.LabelPair{
				{"label1", "test_0"},
				{"label2", "test_1"},
			},
			expected: fingerprints[5:10],
		},
		{
			pairs: []model.LabelPair{
				{"label1", "test_1"},
				{"label2", "test_2"},
			},
			expected: fingerprints[15:20],
		},
	}

	for _, mt := range matcherTests {
		resfps := storage.fingerprintsForLabelPairs(mt.pairs...)
		if len(mt.expected) != len(resfps) {
			t.Fatalf("expected %d matches for %q, found %d", len(mt.expected), mt.pairs, len(resfps))
		}
		for fp1 := range resfps {
			found := false
			for _, fp2 := range mt.expected {
				if fp1 == fp2 {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected fingerprint %s for %q not in result", fp1, mt.pairs)
			}
		}
	}
}

var benchLabelMatchingRes map[model.Fingerprint]metric.Metric

func BenchmarkLabelMatching(b *testing.B) {
	s, closer := NewTestStorage(b, 2)
	defer closer.Close()

	h := fnv.New64a()
	lbl := func(x int) model.LabelValue {
		h.Reset()
		h.Write([]byte(fmt.Sprintf("%d", x)))
		return model.LabelValue(fmt.Sprintf("%d", h.Sum64()))
	}

	M := 32
	met := model.Metric{}
	for i := 0; i < M; i++ {
		met["label_a"] = lbl(i)
		for j := 0; j < M; j++ {
			met["label_b"] = lbl(j)
			for k := 0; k < M; k++ {
				met["label_c"] = lbl(k)
				for l := 0; l < M; l++ {
					met["label_d"] = lbl(l)
					s.Append(&model.Sample{
						Metric:    met.Clone(),
						Timestamp: 0,
						Value:     1,
					})
				}
			}
		}
	}
	s.WaitForIndexing()

	newMatcher := func(matchType metric.MatchType, name model.LabelName, value model.LabelValue) *metric.LabelMatcher {
		lm, err := metric.NewLabelMatcher(matchType, name, value)
		if err != nil {
			b.Fatalf("error creating label matcher: %s", err)
		}
		return lm
	}

	var matcherTests = []metric.LabelMatchers{
		{
			newMatcher(metric.Equal, "label_a", lbl(1)),
		},
		{
			newMatcher(metric.Equal, "label_a", lbl(3)),
			newMatcher(metric.Equal, "label_c", lbl(3)),
		},
		{
			newMatcher(metric.Equal, "label_a", lbl(3)),
			newMatcher(metric.Equal, "label_c", lbl(3)),
			newMatcher(metric.NotEqual, "label_d", lbl(3)),
		},
		{
			newMatcher(metric.Equal, "label_a", lbl(3)),
			newMatcher(metric.Equal, "label_b", lbl(3)),
			newMatcher(metric.Equal, "label_c", lbl(3)),
			newMatcher(metric.NotEqual, "label_d", lbl(3)),
		},
		{
			newMatcher(metric.RegexMatch, "label_a", ".+"),
		},
		{
			newMatcher(metric.Equal, "label_a", lbl(3)),
			newMatcher(metric.RegexMatch, "label_a", ".+"),
		},
		{
			newMatcher(metric.Equal, "label_a", lbl(1)),
			newMatcher(metric.RegexMatch, "label_c", "("+lbl(3)+"|"+lbl(10)+")"),
		},
		{
			newMatcher(metric.Equal, "label_a", lbl(3)),
			newMatcher(metric.Equal, "label_a", lbl(4)),
			newMatcher(metric.RegexMatch, "label_c", "("+lbl(3)+"|"+lbl(10)+")"),
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchLabelMatchingRes = map[model.Fingerprint]metric.Metric{}
		for _, mt := range matcherTests {
			benchLabelMatchingRes = s.MetricsForLabelMatchers(
				model.Earliest, model.Latest,
				mt...,
			)
		}
	}
	// Stop timer to not count the storage closing.
	b.StopTimer()
}

func TestRetentionCutoff(t *testing.T) {
	now := model.Now()
	insertStart := now.Add(-2 * time.Hour)

	s, closer := NewTestStorage(t, 2)
	defer closer.Close()

	// Stop maintenance loop to prevent actual purging.
	close(s.loopStopping)
	<-s.loopStopped
	<-s.logThrottlingStopped
	// Recreate channel to avoid panic when we really shut down.
	s.loopStopping = make(chan struct{})

	s.dropAfter = 1 * time.Hour

	for i := 0; i < 120; i++ {
		smpl := &model.Sample{
			Metric:    model.Metric{"job": "test"},
			Timestamp: insertStart.Add(time.Duration(i) * time.Minute), // 1 minute intervals.
			Value:     1,
		}
		s.Append(smpl)
	}
	s.WaitForIndexing()

	var fp model.Fingerprint
	for f := range s.fingerprintsForLabelPairs(model.LabelPair{Name: "job", Value: "test"}) {
		fp = f
		break
	}

	pl := s.NewPreloader()
	defer pl.Close()

	// Preload everything.
	it := pl.PreloadRange(fp, insertStart, now)

	val := it.ValueAtOrBeforeTime(now.Add(-61 * time.Minute))
	if val.Timestamp != model.Earliest {
		t.Errorf("unexpected result for timestamp before retention period")
	}

	vals := it.RangeValues(metric.Interval{OldestInclusive: insertStart, NewestInclusive: now})
	// We get 59 values here because the model.Now() is slightly later
	// than our now.
	if len(vals) != 59 {
		t.Errorf("expected 59 values but got %d", len(vals))
	}
	if expt := now.Add(-1 * time.Hour).Add(time.Minute); vals[0].Timestamp != expt {
		t.Errorf("unexpected timestamp for first sample: %v, expected %v", vals[0].Timestamp.Time(), expt.Time())
	}
}

func TestDropMetrics(t *testing.T) {
	now := model.Now()
	insertStart := now.Add(-2 * time.Hour)

	s, closer := NewTestStorage(t, 2)
	defer closer.Close()

	chunkFileExists := func(fp model.Fingerprint) (bool, error) {
		f, err := s.persistence.openChunkFileForReading(fp)
		if err == nil {
			f.Close()
			return true, nil
		}
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	m1 := model.Metric{model.MetricNameLabel: "test", "n1": "v1"}
	m2 := model.Metric{model.MetricNameLabel: "test", "n1": "v2"}
	m3 := model.Metric{model.MetricNameLabel: "test", "n1": "v3"}

	N := 120000

	for j, m := range []model.Metric{m1, m2, m3} {
		for i := 0; i < N; i++ {
			smpl := &model.Sample{
				Metric:    m,
				Timestamp: insertStart.Add(time.Duration(i) * time.Millisecond), // 1 millisecond intervals.
				Value:     model.SampleValue(j),
			}
			s.Append(smpl)
		}
	}
	s.WaitForIndexing()

	// Archive m3, but first maintain it so that at least something is written to disk.
	fpToBeArchived := m3.FastFingerprint()
	s.maintainMemorySeries(fpToBeArchived, 0)
	s.fpLocker.Lock(fpToBeArchived)
	s.fpToSeries.del(fpToBeArchived)
	s.persistence.archiveMetric(fpToBeArchived, m3, 0, insertStart.Add(time.Duration(N-1)*time.Millisecond))
	s.fpLocker.Unlock(fpToBeArchived)

	fps := s.fingerprintsForLabelPairs(model.LabelPair{Name: model.MetricNameLabel, Value: "test"})
	if len(fps) != 3 {
		t.Errorf("unexpected number of fingerprints: %d", len(fps))
	}

	fpList := model.Fingerprints{m1.FastFingerprint(), m2.FastFingerprint(), fpToBeArchived}

	s.DropMetricsForFingerprints(fpList[0])
	s.WaitForIndexing()

	fps2 := s.fingerprintsForLabelPairs(model.LabelPair{
		Name: model.MetricNameLabel, Value: "test",
	})
	if len(fps2) != 2 {
		t.Errorf("unexpected number of fingerprints: %d", len(fps2))
	}

	_, it := s.preloadChunksForRange(fpList[0], model.Earliest, model.Latest)
	if vals := it.RangeValues(metric.Interval{OldestInclusive: insertStart, NewestInclusive: now}); len(vals) != 0 {
		t.Errorf("unexpected number of samples: %d", len(vals))
	}

	_, it = s.preloadChunksForRange(fpList[1], model.Earliest, model.Latest)
	if vals := it.RangeValues(metric.Interval{OldestInclusive: insertStart, NewestInclusive: now}); len(vals) != N {
		t.Errorf("unexpected number of samples: %d", len(vals))
	}
	exists, err := chunkFileExists(fpList[2])
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Errorf("chunk file does not exist for fp=%v", fpList[2])
	}

	s.DropMetricsForFingerprints(fpList...)
	s.WaitForIndexing()

	fps3 := s.fingerprintsForLabelPairs(model.LabelPair{
		Name: model.MetricNameLabel, Value: "test",
	})
	if len(fps3) != 0 {
		t.Errorf("unexpected number of fingerprints: %d", len(fps3))
	}

	_, it = s.preloadChunksForRange(fpList[0], model.Earliest, model.Latest)
	if vals := it.RangeValues(metric.Interval{OldestInclusive: insertStart, NewestInclusive: now}); len(vals) != 0 {
		t.Errorf("unexpected number of samples: %d", len(vals))
	}

	_, it = s.preloadChunksForRange(fpList[1], model.Earliest, model.Latest)
	if vals := it.RangeValues(metric.Interval{OldestInclusive: insertStart, NewestInclusive: now}); len(vals) != 0 {
		t.Errorf("unexpected number of samples: %d", len(vals))
	}
	exists, err = chunkFileExists(fpList[2])
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Errorf("chunk file still exists for fp=%v", fpList[2])
	}
}

func TestQuarantineMetric(t *testing.T) {
	now := model.Now()
	insertStart := now.Add(-2 * time.Hour)

	s, closer := NewTestStorage(t, 2)
	defer closer.Close()

	chunkFileExists := func(fp model.Fingerprint) (bool, error) {
		f, err := s.persistence.openChunkFileForReading(fp)
		if err == nil {
			f.Close()
			return true, nil
		}
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	m1 := model.Metric{model.MetricNameLabel: "test", "n1": "v1"}
	m2 := model.Metric{model.MetricNameLabel: "test", "n1": "v2"}
	m3 := model.Metric{model.MetricNameLabel: "test", "n1": "v3"}

	N := 120000

	for j, m := range []model.Metric{m1, m2, m3} {
		for i := 0; i < N; i++ {
			smpl := &model.Sample{
				Metric:    m,
				Timestamp: insertStart.Add(time.Duration(i) * time.Millisecond), // 1 millisecond intervals.
				Value:     model.SampleValue(j),
			}
			s.Append(smpl)
		}
	}
	s.WaitForIndexing()

	// Archive m3, but first maintain it so that at least something is written to disk.
	fpToBeArchived := m3.FastFingerprint()
	s.maintainMemorySeries(fpToBeArchived, 0)
	s.fpLocker.Lock(fpToBeArchived)
	s.fpToSeries.del(fpToBeArchived)
	s.persistence.archiveMetric(fpToBeArchived, m3, 0, insertStart.Add(time.Duration(N-1)*time.Millisecond))
	s.fpLocker.Unlock(fpToBeArchived)

	// Corrupt the series file for m3.
	f, err := os.Create(s.persistence.fileNameForFingerprint(fpToBeArchived))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("This is clearly not the content of a series file."); err != nil {
		t.Fatal(err)
	}
	if f.Close(); err != nil {
		t.Fatal(err)
	}

	fps := s.fingerprintsForLabelPairs(model.LabelPair{Name: model.MetricNameLabel, Value: "test"})
	if len(fps) != 3 {
		t.Errorf("unexpected number of fingerprints: %d", len(fps))
	}

	pl := s.NewPreloader()
	// This will access the corrupt file and lead to quarantining.
	pl.PreloadInstant(fpToBeArchived, now.Add(-2*time.Hour), time.Minute)
	pl.Close()
	time.Sleep(time.Second) // Give time to quarantine. TODO(beorn7): Find a better way to wait.
	s.WaitForIndexing()

	fps2 := s.fingerprintsForLabelPairs(model.LabelPair{
		Name: model.MetricNameLabel, Value: "test",
	})
	if len(fps2) != 2 {
		t.Errorf("unexpected number of fingerprints: %d", len(fps2))
	}

	exists, err := chunkFileExists(fpToBeArchived)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Errorf("chunk file exists for fp=%v", fpToBeArchived)
	}
}

// TestLoop is just a smoke test for the loop method, if we can switch it on and
// off without disaster.
func TestLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}
	samples := make(model.Samples, 1000)
	for i := range samples {
		samples[i] = &model.Sample{
			Timestamp: model.Time(2 * i),
			Value:     model.SampleValue(float64(i) * 0.2),
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
		MinShrinkRatio:             0.1,
	}
	storage := NewMemorySeriesStorage(o)
	if err := storage.Start(); err != nil {
		t.Errorf("Error starting storage: %s", err)
	}
	for _, s := range samples {
		storage.Append(s)
	}
	storage.WaitForIndexing()
	series, _ := storage.(*memorySeriesStorage).fpToSeries.get(model.Metric{}.FastFingerprint())
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
	samples := make(model.Samples, 500000)
	for i := range samples {
		samples[i] = &model.Sample{
			Timestamp: model.Time(i),
			Value:     model.SampleValue(float64(i) * 0.2),
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
		defer s.fpLocker.Unlock(m.fp) // TODO remove, see below
		var values []model.SamplePair
		for _, cd := range m.series.chunkDescs {
			if cd.isEvicted() {
				continue
			}
			it := cd.c.newIterator()
			for it.scan() {
				values = append(values, it.value())
			}
			if it.err() != nil {
				t.Error(it.err())
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
		//s.fpLocker.Unlock(m.fp)
	}
	log.Info("test done, closing")
}

func TestChunkType0(t *testing.T) {
	testChunk(t, 0)
}

func TestChunkType1(t *testing.T) {
	testChunk(t, 1)
}

func TestChunkType2(t *testing.T) {
	testChunk(t, 2)
}

func testValueAtOrBeforeTime(t *testing.T, encoding chunkEncoding) {
	samples := make(model.Samples, 10000)
	for i := range samples {
		samples[i] = &model.Sample{
			Timestamp: model.Time(2 * i),
			Value:     model.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(t, encoding)
	defer closer.Close()

	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	fp := model.Metric{}.FastFingerprint()

	_, it := s.preloadChunksForRange(fp, model.Earliest, model.Latest)

	// #1 Exactly on a sample.
	for i, expected := range samples {
		actual := it.ValueAtOrBeforeTime(expected.Timestamp)

		if expected.Timestamp != actual.Timestamp {
			t.Errorf("1.%d. Got %v; want %v", i, actual.Timestamp, expected.Timestamp)
		}
		if expected.Value != actual.Value {
			t.Errorf("1.%d. Got %v; want %v", i, actual.Value, expected.Value)
		}
	}

	// #2 Between samples.
	for i, expected := range samples {
		if i == len(samples)-1 {
			continue
		}
		actual := it.ValueAtOrBeforeTime(expected.Timestamp + 1)

		if expected.Timestamp != actual.Timestamp {
			t.Errorf("2.%d. Got %v; want %v", i, actual.Timestamp, expected.Timestamp)
		}
		if expected.Value != actual.Value {
			t.Errorf("2.%d. Got %v; want %v", i, actual.Value, expected.Value)
		}
	}

	// #3 Corner cases: Just before the first sample, just after the last.
	expected := &model.Sample{Timestamp: model.Earliest}
	actual := it.ValueAtOrBeforeTime(samples[0].Timestamp - 1)
	if expected.Timestamp != actual.Timestamp {
		t.Errorf("3.1. Got %v; want %v", actual.Timestamp, expected.Timestamp)
	}
	if expected.Value != actual.Value {
		t.Errorf("3.1. Got %v; want %v", actual.Value, expected.Value)
	}
	expected = samples[len(samples)-1]
	actual = it.ValueAtOrBeforeTime(expected.Timestamp + 1)
	if expected.Timestamp != actual.Timestamp {
		t.Errorf("3.2. Got %v; want %v", actual.Timestamp, expected.Timestamp)
	}
	if expected.Value != actual.Value {
		t.Errorf("3.2. Got %v; want %v", actual.Value, expected.Value)
	}
}

func TestValueAtTimeChunkType0(t *testing.T) {
	testValueAtOrBeforeTime(t, 0)
}

func TestValueAtTimeChunkType1(t *testing.T) {
	testValueAtOrBeforeTime(t, 1)
}

func TestValueAtTimeChunkType2(t *testing.T) {
	testValueAtOrBeforeTime(t, 2)
}

func benchmarkValueAtOrBeforeTime(b *testing.B, encoding chunkEncoding) {
	samples := make(model.Samples, 10000)
	for i := range samples {
		samples[i] = &model.Sample{
			Timestamp: model.Time(2 * i),
			Value:     model.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(b, encoding)
	defer closer.Close()

	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	fp := model.Metric{}.FastFingerprint()

	_, it := s.preloadChunksForRange(fp, model.Earliest, model.Latest)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// #1 Exactly on a sample.
		for i, expected := range samples {
			actual := it.ValueAtOrBeforeTime(expected.Timestamp)

			if expected.Timestamp != actual.Timestamp {
				b.Errorf("1.%d. Got %v; want %v", i, actual.Timestamp, expected.Timestamp)
			}
			if expected.Value != actual.Value {
				b.Errorf("1.%d. Got %v; want %v", i, actual.Value, expected.Value)
			}
		}

		// #2 Between samples.
		for i, expected := range samples {
			if i == len(samples)-1 {
				continue
			}
			actual := it.ValueAtOrBeforeTime(expected.Timestamp + 1)

			if expected.Timestamp != actual.Timestamp {
				b.Errorf("2.%d. Got %v; want %v", i, actual.Timestamp, expected.Timestamp)
			}
			if expected.Value != actual.Value {
				b.Errorf("2.%d. Got %v; want %v", i, actual.Value, expected.Value)
			}
		}

		// #3 Corner cases: Just before the first sample, just after the last.
		expected := &model.Sample{Timestamp: model.Earliest}
		actual := it.ValueAtOrBeforeTime(samples[0].Timestamp - 1)
		if expected.Timestamp != actual.Timestamp {
			b.Errorf("3.1. Got %v; want %v", actual.Timestamp, expected.Timestamp)
		}
		if expected.Value != actual.Value {
			b.Errorf("3.1. Got %v; want %v", actual.Value, expected.Value)
		}
		expected = samples[len(samples)-1]
		actual = it.ValueAtOrBeforeTime(expected.Timestamp + 1)
		if expected.Timestamp != actual.Timestamp {
			b.Errorf("3.2. Got %v; want %v", actual.Timestamp, expected.Timestamp)
		}
		if expected.Value != actual.Value {
			b.Errorf("3.2. Got %v; want %v", actual.Value, expected.Value)
		}
	}
}

func BenchmarkValueAtOrBeforeTimeChunkType0(b *testing.B) {
	benchmarkValueAtOrBeforeTime(b, 0)
}

func BenchmarkValueAtTimeChunkType1(b *testing.B) {
	benchmarkValueAtOrBeforeTime(b, 1)
}

func BenchmarkValueAtTimeChunkType2(b *testing.B) {
	benchmarkValueAtOrBeforeTime(b, 2)
}

func testRangeValues(t *testing.T, encoding chunkEncoding) {
	samples := make(model.Samples, 10000)
	for i := range samples {
		samples[i] = &model.Sample{
			Timestamp: model.Time(2 * i),
			Value:     model.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(t, encoding)
	defer closer.Close()

	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	fp := model.Metric{}.FastFingerprint()

	_, it := s.preloadChunksForRange(fp, model.Earliest, model.Latest)

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

func TestRangeValuesChunkType2(t *testing.T) {
	testRangeValues(t, 2)
}

func benchmarkRangeValues(b *testing.B, encoding chunkEncoding) {
	samples := make(model.Samples, 10000)
	for i := range samples {
		samples[i] = &model.Sample{
			Timestamp: model.Time(2 * i),
			Value:     model.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(b, encoding)
	defer closer.Close()

	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	fp := model.Metric{}.FastFingerprint()

	_, it := s.preloadChunksForRange(fp, model.Earliest, model.Latest)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
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

func BenchmarkRangeValuesChunkType2(b *testing.B) {
	benchmarkRangeValues(b, 2)
}

func testEvictAndPurgeSeries(t *testing.T, encoding chunkEncoding) {
	samples := make(model.Samples, 10000)
	for i := range samples {
		samples[i] = &model.Sample{
			Timestamp: model.Time(2 * i),
			Value:     model.SampleValue(float64(i * i)),
		}
	}
	s, closer := NewTestStorage(t, encoding)
	defer closer.Close()

	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	fp := model.Metric{}.FastFingerprint()

	// Drop ~half of the chunks.
	s.maintainMemorySeries(fp, 10000)
	_, it := s.preloadChunksForRange(fp, model.Earliest, model.Latest)
	actual := it.RangeValues(metric.Interval{
		OldestInclusive: 0,
		NewestInclusive: 100000,
	})
	if len(actual) < 4000 {
		t.Fatalf("expected more than %d results after purging half of series, got %d", 4000, len(actual))
	}
	if actual[0].Timestamp < 6000 || actual[0].Timestamp > 10000 {
		t.Errorf("1st timestamp out of expected range: %v", actual[0].Timestamp)
	}
	want := model.Time(19998)
	if actual[len(actual)-1].Timestamp != want {
		t.Errorf("2nd timestamp: want %v, got %v", want, actual[1].Timestamp)
	}

	// Drop everything.
	s.maintainMemorySeries(fp, 100000)
	_, it = s.preloadChunksForRange(fp, model.Earliest, model.Latest)
	actual = it.RangeValues(metric.Interval{
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
	s.maintainMemorySeries(fp, model.Earliest)

	// Archive metrics.
	s.fpToSeries.del(fp)
	lastTime, err := series.head().lastTime()
	if err != nil {
		t.Fatal(err)
	}
	s.persistence.archiveMetric(fp, series.metric, series.firstTime(), lastTime)
	archived, _, _ := s.persistence.hasArchivedMetric(fp)
	if !archived {
		t.Fatal("not archived")
	}

	// Drop ~half of the chunks of an archived series.
	s.maintainArchivedSeries(fp, 10000)
	archived, _, _ = s.persistence.hasArchivedMetric(fp)
	if !archived {
		t.Fatal("archived series purged although only half of the chunks dropped")
	}

	// Drop everything.
	s.maintainArchivedSeries(fp, 100000)
	archived, _, _ = s.persistence.hasArchivedMetric(fp)
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
	s.maintainMemorySeries(fp, model.Earliest)

	// Archive metrics.
	s.fpToSeries.del(fp)
	lastTime, err = series.head().lastTime()
	if err != nil {
		t.Fatal(err)
	}
	s.persistence.archiveMetric(fp, series.metric, series.firstTime(), lastTime)
	archived, _, _ = s.persistence.hasArchivedMetric(fp)
	if !archived {
		t.Fatal("not archived")
	}

	// Unarchive metrics.
	s.getOrCreateSeries(fp, model.Metric{})

	series, ok = s.fpToSeries.get(fp)
	if !ok {
		t.Fatal("could not find series")
	}
	archived, _, _ = s.persistence.hasArchivedMetric(fp)
	if archived {
		t.Fatal("archived")
	}

	// Set archiveHighWatermark to a low value so that we can see it increase.
	s.archiveHighWatermark = 42

	// This will archive again, but must not drop it completely, despite the
	// memorySeries being empty.
	s.maintainMemorySeries(fp, 10000)
	archived, _, _ = s.persistence.hasArchivedMetric(fp)
	if !archived {
		t.Fatal("series purged completely")
	}
	// archiveHighWatermark must have been set by maintainMemorySeries.
	if want, got := model.Time(19998), s.archiveHighWatermark; want != got {
		t.Errorf("want archiveHighWatermark %v, got %v", want, got)
	}
}

func TestEvictAndPurgeSeriesChunkType0(t *testing.T) {
	testEvictAndPurgeSeries(t, 0)
}

func TestEvictAndPurgeSeriesChunkType1(t *testing.T) {
	testEvictAndPurgeSeries(t, 1)
}

func TestEvictAndPurgeSeriesChunkType2(t *testing.T) {
	testEvictAndPurgeSeries(t, 2)
}

func testEvictAndLoadChunkDescs(t *testing.T, encoding chunkEncoding) {
	samples := make(model.Samples, 10000)
	for i := range samples {
		samples[i] = &model.Sample{
			Timestamp: model.Time(2 * i),
			Value:     model.SampleValue(float64(i * i)),
		}
	}
	// Give last sample a timestamp of now so that the head chunk will not
	// be closed (which would then archive the time series later as
	// everything will get evicted).
	samples[len(samples)-1] = &model.Sample{
		Timestamp: model.Now(),
		Value:     model.SampleValue(3.14),
	}

	s, closer := NewTestStorage(t, encoding)
	defer closer.Close()

	// Adjust memory chunks to lower value to see evictions.
	s.maxMemoryChunks = 1

	for _, sample := range samples {
		s.Append(sample)
	}
	s.WaitForIndexing()

	fp := model.Metric{}.FastFingerprint()

	series, ok := s.fpToSeries.get(fp)
	if !ok {
		t.Fatal("could not find series")
	}

	oldLen := len(series.chunkDescs)
	// Maintain series without any dropped chunks.
	s.maintainMemorySeries(fp, 0)
	// Give the evict goroutine an opportunity to run.
	time.Sleep(250 * time.Millisecond)
	// Maintain series again to trigger chunkDesc eviction
	s.maintainMemorySeries(fp, 0)

	if oldLen <= len(series.chunkDescs) {
		t.Errorf("Expected number of chunkDescs to decrease, old number %d, current number %d.", oldLen, len(series.chunkDescs))
	}

	// Load everything back.
	p := s.NewPreloader()
	p.PreloadRange(fp, 0, 100000)

	if oldLen != len(series.chunkDescs) {
		t.Errorf("Expected number of chunkDescs to have reached old value again, old number %d, current number %d.", oldLen, len(series.chunkDescs))
	}

	p.Close()

	// Now maintain series with drops to make sure nothing crazy happens.
	s.maintainMemorySeries(fp, 100000)

	if len(series.chunkDescs) != 1 {
		t.Errorf("Expected exactly one chunkDesc left, got %d.", len(series.chunkDescs))
	}
}

func TestEvictAndLoadChunkDescsType0(t *testing.T) {
	testEvictAndLoadChunkDescs(t, 0)
}

func TestEvictAndLoadChunkDescsType1(t *testing.T) {
	testEvictAndLoadChunkDescs(t, 1)
}

func benchmarkAppend(b *testing.B, encoding chunkEncoding) {
	samples := make(model.Samples, b.N)
	for i := range samples {
		samples[i] = &model.Sample{
			Metric: model.Metric{
				model.MetricNameLabel: model.LabelValue(fmt.Sprintf("test_metric_%d", i%10)),
				"label1":              model.LabelValue(fmt.Sprintf("test_metric_%d", i%10)),
				"label2":              model.LabelValue(fmt.Sprintf("test_metric_%d", i%10)),
			},
			Timestamp: model.Time(i),
			Value:     model.SampleValue(i),
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

func BenchmarkAppendType2(b *testing.B) {
	benchmarkAppend(b, 2)
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
		if !verifyStorageRandom(t, s, samples) {
			return false
		}
		return verifyStorageSequential(t, s, samples)
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

func TestFuzzChunkType2(t *testing.T) {
	testFuzz(t, 2)
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
	DefaultChunkEncoding = encoding
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
		MinShrinkRatio:             0.1,
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
		verifyStorageRandom(b, s.(*memorySeriesStorage), samples[:middle])
		for _, sample := range samples[middle:end] {
			s.Append(sample)
		}
		verifyStorageRandom(b, s.(*memorySeriesStorage), samples[:end])
		verifyStorageSequential(b, s.(*memorySeriesStorage), samples)
	}
}

func BenchmarkFuzzChunkType0(b *testing.B) {
	benchmarkFuzz(b, 0)
}

func BenchmarkFuzzChunkType1(b *testing.B) {
	benchmarkFuzz(b, 1)
}

func BenchmarkFuzzChunkType2(b *testing.B) {
	benchmarkFuzz(b, 2)
}

func createRandomSamples(metricName string, minLen int) model.Samples {
	type valueCreator func() model.SampleValue
	type deltaApplier func(model.SampleValue) model.SampleValue

	var (
		maxMetrics      = 5
		maxStreakLength = 2000
		maxTimeDelta    = 10000
		timestamp       = model.Now() - model.Time(maxTimeDelta*minLen) // So that some timestamps are in the future.
		generators      = []struct {
			createValue valueCreator
			applyDelta  []deltaApplier
		}{
			{ // "Boolean".
				createValue: func() model.SampleValue {
					return model.SampleValue(rand.Intn(2))
				},
				applyDelta: []deltaApplier{
					func(_ model.SampleValue) model.SampleValue {
						return model.SampleValue(rand.Intn(2))
					},
				},
			},
			{ // Integer with int deltas of various byte length.
				createValue: func() model.SampleValue {
					return model.SampleValue(rand.Int63() - 1<<62)
				},
				applyDelta: []deltaApplier{
					func(v model.SampleValue) model.SampleValue {
						return model.SampleValue(rand.Intn(1<<8) - 1<<7 + int(v))
					},
					func(v model.SampleValue) model.SampleValue {
						return model.SampleValue(rand.Intn(1<<16) - 1<<15 + int(v))
					},
					func(v model.SampleValue) model.SampleValue {
						return model.SampleValue(rand.Int63n(1<<32) - 1<<31 + int64(v))
					},
				},
			},
			{ // Float with float32 and float64 deltas.
				createValue: func() model.SampleValue {
					return model.SampleValue(rand.NormFloat64())
				},
				applyDelta: []deltaApplier{
					func(v model.SampleValue) model.SampleValue {
						return v + model.SampleValue(float32(rand.NormFloat64()))
					},
					func(v model.SampleValue) model.SampleValue {
						return v + model.SampleValue(rand.NormFloat64())
					},
				},
			},
		}
		timestampIncrementers = []func(baseDelta model.Time) model.Time{
			// Regular increments.
			func(delta model.Time) model.Time {
				return delta
			},
			// Jittered increments. σ is 1/100 of delta, e.g. 10ms for 10s scrape interval.
			func(delta model.Time) model.Time {
				return delta + model.Time(rand.NormFloat64()*float64(delta)/100)
			},
			// Regular increments, but missing a scrape with 10% chance.
			func(delta model.Time) model.Time {
				i := rand.Intn(100)
				if i < 90 {
					return delta
				}
				if i < 99 {
					return 2 * delta
				}
				return 3 * delta
				// Ignoring the case with more than two missed scrapes in a row.
			},
		}
	)

	// Prefill result with two samples with colliding metrics (to test fingerprint mapping).
	result := model.Samples{
		&model.Sample{
			Metric: model.Metric{
				"instance": "ip-10-33-84-73.l05.ams5.s-cloud.net:24483",
				"status":   "503",
			},
			Value:     42,
			Timestamp: timestamp,
		},
		&model.Sample{
			Metric: model.Metric{
				"instance": "ip-10-33-84-73.l05.ams5.s-cloud.net:24480",
				"status":   "500",
			},
			Value:     2010,
			Timestamp: timestamp + 1,
		},
	}

	metrics := []model.Metric{}
	for n := rand.Intn(maxMetrics); n >= 0; n-- {
		metrics = append(metrics, model.Metric{
			model.MetricNameLabel:                             model.LabelValue(metricName),
			model.LabelName(fmt.Sprintf("labelname_%d", n+1)): model.LabelValue(fmt.Sprintf("labelvalue_%d", rand.Int())),
		})
	}

	for len(result) < minLen {
		var (
			// Pick a metric for this cycle.
			metric       = metrics[rand.Intn(len(metrics))]
			timeDelta    = model.Time(rand.Intn(maxTimeDelta) + 1)
			generator    = generators[rand.Intn(len(generators))]
			createValue  = generator.createValue
			applyDelta   = generator.applyDelta[rand.Intn(len(generator.applyDelta))]
			incTimestamp = timestampIncrementers[rand.Intn(len(timestampIncrementers))]
		)

		switch rand.Intn(4) {
		case 0: // A single sample.
			result = append(result, &model.Sample{
				Metric:    metric,
				Value:     createValue(),
				Timestamp: timestamp,
			})
			timestamp += incTimestamp(timeDelta)
		case 1: // A streak of random sample values.
			for n := rand.Intn(maxStreakLength); n >= 0; n-- {
				result = append(result, &model.Sample{
					Metric:    metric,
					Value:     createValue(),
					Timestamp: timestamp,
				})
				timestamp += incTimestamp(timeDelta)
			}
		case 2: // A streak of sample values with incremental changes.
			value := createValue()
			for n := rand.Intn(maxStreakLength); n >= 0; n-- {
				result = append(result, &model.Sample{
					Metric:    metric,
					Value:     value,
					Timestamp: timestamp,
				})
				timestamp += incTimestamp(timeDelta)
				value = applyDelta(value)
			}
		case 3: // A streak of constant sample values.
			value := createValue()
			for n := rand.Intn(maxStreakLength); n >= 0; n-- {
				result = append(result, &model.Sample{
					Metric:    metric,
					Value:     value,
					Timestamp: timestamp,
				})
				timestamp += incTimestamp(timeDelta)
			}
		}
	}

	return result
}

func verifyStorageRandom(t testing.TB, s *memorySeriesStorage, samples model.Samples) bool {
	s.WaitForIndexing()
	result := true
	for _, i := range rand.Perm(len(samples)) {
		sample := samples[i]
		fp := s.mapper.mapFP(sample.Metric.FastFingerprint(), sample.Metric)
		p := s.NewPreloader()
		it := p.PreloadInstant(fp, sample.Timestamp, 0)
		found := it.ValueAtOrBeforeTime(sample.Timestamp)
		startTime := it.(*boundedIterator).start
		switch {
		case found.Timestamp != model.Earliest && sample.Timestamp.Before(startTime):
			t.Errorf("Sample #%d %#v: Expected outdated sample to be excluded.", i, sample)
			result = false
		case found.Timestamp == model.Earliest && !sample.Timestamp.Before(startTime):
			t.Errorf("Sample #%d %#v: Expected sample not found.", i, sample)
			result = false
		case found.Timestamp == model.Earliest && sample.Timestamp.Before(startTime):
			// All good. Outdated sample dropped.
		case sample.Value != found.Value || sample.Timestamp != found.Timestamp:
			t.Errorf(
				"Sample #%d %#v: Value (or timestamp) mismatch, want %f (at time %v), got %f (at time %v).",
				i, sample, sample.Value, sample.Timestamp, found.Value, found.Timestamp,
			)
			result = false
		}
		p.Close()
	}
	return result
}

func verifyStorageSequential(t testing.TB, s *memorySeriesStorage, samples model.Samples) bool {
	s.WaitForIndexing()
	var (
		result = true
		fp     model.Fingerprint
		p      = s.NewPreloader()
		it     SeriesIterator
		r      []model.SamplePair
		j      int
	)
	defer func() {
		p.Close()
	}()
	for i, sample := range samples {
		newFP := s.mapper.mapFP(sample.Metric.FastFingerprint(), sample.Metric)
		if it == nil || newFP != fp {
			fp = newFP
			p.Close()
			p = s.NewPreloader()
			it = p.PreloadRange(fp, sample.Timestamp, model.Latest)
			r = it.RangeValues(metric.Interval{
				OldestInclusive: sample.Timestamp,
				NewestInclusive: model.Latest,
			})
			j = -1
		}
		startTime := it.(*boundedIterator).start
		if sample.Timestamp.Before(startTime) {
			continue
		}
		j++
		if j >= len(r) {
			t.Errorf(
				"Sample #%d %v not found.",
				i, sample,
			)
			result = false
			continue
		}
		found := r[j]
		if sample.Value != found.Value || sample.Timestamp != found.Timestamp {
			t.Errorf(
				"Sample #%d %v: Value (or timestamp) mismatch, want %f (at time %v), got %f (at time %v).",
				i, sample, sample.Value, sample.Timestamp, found.Value, found.Timestamp,
			)
			result = false
		}
	}
	return result
}

func TestAppendOutOfOrder(t *testing.T) {
	s, closer := NewTestStorage(t, 2)
	defer closer.Close()

	m := model.Metric{
		model.MetricNameLabel: "out_of_order",
	}

	tests := []struct {
		name      string
		timestamp model.Time
		value     model.SampleValue
		wantErr   error
	}{
		{
			name:      "1st sample",
			timestamp: 0,
			value:     0,
			wantErr:   nil,
		},
		{
			name:      "regular append",
			timestamp: 2,
			value:     1,
			wantErr:   nil,
		},
		{
			name:      "same timestamp, same value (no-op)",
			timestamp: 2,
			value:     1,
			wantErr:   nil,
		},
		{
			name:      "same timestamp, different value",
			timestamp: 2,
			value:     2,
			wantErr:   ErrDuplicateSampleForTimestamp,
		},
		{
			name:      "earlier timestamp, same value",
			timestamp: 1,
			value:     2,
			wantErr:   ErrOutOfOrderSample,
		},
		{
			name:      "earlier timestamp, different value",
			timestamp: 1,
			value:     3,
			wantErr:   ErrOutOfOrderSample,
		},
		{
			name:      "regular append of NaN",
			timestamp: 3,
			value:     model.SampleValue(math.NaN()),
			wantErr:   nil,
		},
		{
			name:      "no-op append of NaN",
			timestamp: 3,
			value:     model.SampleValue(math.NaN()),
			wantErr:   nil,
		},
		{
			name:      "append of NaN with earlier timestamp",
			timestamp: 2,
			value:     model.SampleValue(math.NaN()),
			wantErr:   ErrOutOfOrderSample,
		},
		{
			name:      "append of normal sample after NaN with same timestamp",
			timestamp: 3,
			value:     3.14,
			wantErr:   ErrDuplicateSampleForTimestamp,
		},
	}

	for _, test := range tests {
		gotErr := s.Append(&model.Sample{
			Metric:    m,
			Timestamp: test.timestamp,
			Value:     test.value,
		})
		if gotErr != test.wantErr {
			t.Errorf("%s: got %q, want %q", test.name, gotErr, test.wantErr)
		}
	}

	fp := s.mapper.mapFP(m.FastFingerprint(), m)

	pl := s.NewPreloader()
	defer pl.Close()

	it := pl.PreloadRange(fp, 0, 2)

	want := []model.SamplePair{
		{
			Timestamp: 0,
			Value:     0,
		},
		{
			Timestamp: 2,
			Value:     1,
		},
		{
			Timestamp: 3,
			Value:     model.SampleValue(math.NaN()),
		},
	}
	got := it.RangeValues(metric.Interval{OldestInclusive: 0, NewestInclusive: 3})
	// Note that we cannot just reflect.DeepEqual(want, got) because it has
	// the semantics of NaN != NaN.
	for i, gotSamplePair := range got {
		wantSamplePair := want[i]
		if !wantSamplePair.Equal(&gotSamplePair) {
			t.Fatalf("want %v, got %v", wantSamplePair, gotSamplePair)
		}
	}
}
