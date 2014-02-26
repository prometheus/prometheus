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
	"math"
	"math/rand"
	"sort"
	"testing"
	"testing/quick"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/coding/indexable"
	"github.com/prometheus/prometheus/utility/test"

	dto "github.com/prometheus/prometheus/model/generated"
)

const stochasticMaximumVariance = 8

func BasicLifecycleTests(p MetricPersistence, t test.Tester) {
	if p == nil {
		t.Errorf("Received nil Metric Persistence.\n")
		return
	}
}

func ReadEmptyTests(p MetricPersistence, t test.Tester) {
	hasLabelPair := func(x int) (success bool) {
		name := clientmodel.LabelName(string(x))
		value := clientmodel.LabelValue(string(x))

		labelSet := clientmodel.LabelSet{
			name: value,
		}

		fingerprints, err := p.GetFingerprintsForLabelSet(labelSet)
		if err != nil {
			t.Error(err)
			return
		}

		success = len(fingerprints) == 0
		if !success {
			t.Errorf("unexpected fingerprint length %d, got %d", 0, len(fingerprints))
		}

		return
	}

	err := quick.Check(hasLabelPair, nil)
	if err != nil {
		t.Error(err)
		return
	}
}

func AppendSampleAsPureSparseAppendTests(p MetricPersistence, t test.Tester) {
	appendSample := func(x int) (success bool) {
		v := clientmodel.SampleValue(x)
		ts := clientmodel.TimestampFromUnix(int64(x))
		labelName := clientmodel.LabelName(x)
		labelValue := clientmodel.LabelValue(x)
		l := clientmodel.Metric{labelName: labelValue}

		sample := &clientmodel.Sample{
			Value:     v,
			Timestamp: ts,
			Metric:    l,
		}

		err := p.AppendSamples(clientmodel.Samples{sample})

		success = err == nil
		if !success {
			t.Error(err)
		}

		return
	}

	if err := quick.Check(appendSample, nil); err != nil {
		t.Error(err)
	}
}

func AppendSampleAsSparseAppendWithReadsTests(p MetricPersistence, t test.Tester) {
	appendSample := func(x int) (success bool) {
		v := clientmodel.SampleValue(x)
		ts := clientmodel.TimestampFromUnix(int64(x))
		labelName := clientmodel.LabelName(x)
		labelValue := clientmodel.LabelValue(x)
		l := clientmodel.Metric{labelName: labelValue}

		sample := &clientmodel.Sample{
			Value:     v,
			Timestamp: ts,
			Metric:    l,
		}

		err := p.AppendSamples(clientmodel.Samples{sample})
		if err != nil {
			t.Error(err)
			return
		}

		fingerprints, err := p.GetFingerprintsForLabelSet(clientmodel.LabelSet{
			labelName: labelValue,
		})
		if err != nil {
			t.Error(err)
			return
		}
		if len(fingerprints) != 1 {
			t.Errorf("expected fingerprint count of %d, got %d", 1, len(fingerprints))
			return
		}

		return true
	}

	if err := quick.Check(appendSample, nil); err != nil {
		t.Error(err)
	}
}

func AppendSampleAsPureSingleEntityAppendTests(p MetricPersistence, t test.Tester) {
	appendSample := func(x int) bool {
		sample := &clientmodel.Sample{
			Value:     clientmodel.SampleValue(x),
			Timestamp: clientmodel.TimestampFromUnix(int64(x)),
			Metric:    clientmodel.Metric{clientmodel.MetricNameLabel: "my_metric"},
		}

		err := p.AppendSamples(clientmodel.Samples{sample})

		return err == nil
	}

	if err := quick.Check(appendSample, nil); err != nil {
		t.Error(err)
	}
}

func levelDBGetRangeValues(l *LevelDBMetricPersistence, fp *clientmodel.Fingerprint, i Interval) (samples Values, err error) {
	fpDto := &dto.Fingerprint{}
	dumpFingerprint(fpDto, fp)
	k := &dto.SampleKey{
		Fingerprint: fpDto,
		Timestamp:   indexable.EncodeTime(i.OldestInclusive),
	}

	iterator, err := l.MetricSamples.NewIterator(true)
	if err != nil {
		panic(err)
	}
	defer iterator.Close()

	for valid := iterator.Seek(k); valid; valid = iterator.Next() {
		retrievedKey, err := extractSampleKey(iterator)
		if err != nil {
			return samples, err
		}

		if retrievedKey.FirstTimestamp.After(i.NewestInclusive) {
			break
		}

		if !retrievedKey.Fingerprint.Equal(fp) {
			break
		}

		retrievedValues := unmarshalValues(iterator.RawValue())
		samples = append(samples, retrievedValues...)
	}

	return
}

type timeslice []clientmodel.Timestamp

func (t timeslice) Len() int {
	return len(t)
}

func (t timeslice) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func (t timeslice) Less(i, j int) bool {
	return t[i].Before(t[j])
}

func StochasticTests(persistenceMaker func() (MetricPersistence, test.Closer), t test.Tester) {
	stochastic := func(x int) (success bool) {
		p, closer := persistenceMaker()
		defer closer.Close()
		defer p.Close()

		seed := rand.NewSource(int64(x))
		random := rand.New(seed)

		numberOfMetrics := random.Intn(stochasticMaximumVariance) + 1
		numberOfSharedLabels := random.Intn(stochasticMaximumVariance)
		numberOfUnsharedLabels := random.Intn(stochasticMaximumVariance)
		numberOfSamples := random.Intn(stochasticMaximumVariance) + 2
		numberOfRangeScans := random.Intn(stochasticMaximumVariance)

		metricTimestamps := map[int]map[int64]bool{}
		metricEarliestSample := map[int]int64{}
		metricNewestSample := map[int]int64{}

		for metricIndex := 0; metricIndex < numberOfMetrics; metricIndex++ {
			sample := &clientmodel.Sample{
				Metric: clientmodel.Metric{},
			}

			v := clientmodel.LabelValue(fmt.Sprintf("metric_index_%d", metricIndex))
			sample.Metric[clientmodel.MetricNameLabel] = v

			for sharedLabelIndex := 0; sharedLabelIndex < numberOfSharedLabels; sharedLabelIndex++ {
				l := clientmodel.LabelName(fmt.Sprintf("shared_label_%d", sharedLabelIndex))
				v := clientmodel.LabelValue(fmt.Sprintf("label_%d", sharedLabelIndex))

				sample.Metric[l] = v
			}

			for unsharedLabelIndex := 0; unsharedLabelIndex < numberOfUnsharedLabels; unsharedLabelIndex++ {
				l := clientmodel.LabelName(fmt.Sprintf("metric_index_%d_private_label_%d", metricIndex, unsharedLabelIndex))
				v := clientmodel.LabelValue(fmt.Sprintf("private_label_%d", unsharedLabelIndex))

				sample.Metric[l] = v
			}

			timestamps := map[int64]bool{}
			metricTimestamps[metricIndex] = timestamps
			var newestSample int64 = math.MinInt64
			var oldestSample int64 = math.MaxInt64
			var nextTimestamp func() int64

			nextTimestamp = func() int64 {
				var candidate int64
				candidate = random.Int63n(math.MaxInt32 - 1)

				if _, has := timestamps[candidate]; has {
					// WART
					candidate = nextTimestamp()
				}

				timestamps[candidate] = true

				if candidate < oldestSample {
					oldestSample = candidate
				}

				if candidate > newestSample {
					newestSample = candidate
				}

				return candidate
			}

			// BUG(matt): Invariant of the in-memory database assumes this.
			sortedTimestamps := timeslice{}
			for sampleIndex := 0; sampleIndex < numberOfSamples; sampleIndex++ {
				sortedTimestamps = append(sortedTimestamps, clientmodel.TimestampFromUnix(nextTimestamp()))
			}
			sort.Sort(sortedTimestamps)

			for sampleIndex := 0; sampleIndex < numberOfSamples; sampleIndex++ {
				sample.Timestamp = sortedTimestamps[sampleIndex]
				sample.Value = clientmodel.SampleValue(sampleIndex)

				err := p.AppendSamples(clientmodel.Samples{sample})

				if err != nil {
					t.Error(err)
					return
				}
			}

			metricEarliestSample[metricIndex] = oldestSample
			metricNewestSample[metricIndex] = newestSample

			for sharedLabelIndex := 0; sharedLabelIndex < numberOfSharedLabels; sharedLabelIndex++ {
				labelPair := clientmodel.LabelSet{
					clientmodel.LabelName(fmt.Sprintf("shared_label_%d", sharedLabelIndex)): clientmodel.LabelValue(fmt.Sprintf("label_%d", sharedLabelIndex)),
				}

				fingerprints, err := p.GetFingerprintsForLabelSet(labelPair)
				if err != nil {
					t.Error(err)
					return
				}
				if len(fingerprints) == 0 {
					t.Errorf("expected fingerprint count of %d, got %d", 0, len(fingerprints))
					return
				}
			}
		}

		for metricIndex := 0; metricIndex < numberOfMetrics; metricIndex++ {
			for unsharedLabelIndex := 0; unsharedLabelIndex < numberOfUnsharedLabels; unsharedLabelIndex++ {
				labelName := clientmodel.LabelName(fmt.Sprintf("metric_index_%d_private_label_%d", metricIndex, unsharedLabelIndex))
				labelValue := clientmodel.LabelValue(fmt.Sprintf("private_label_%d", unsharedLabelIndex))
				labelSet := clientmodel.LabelSet{
					labelName: labelValue,
				}

				fingerprints, err := p.GetFingerprintsForLabelSet(labelSet)
				if err != nil {
					t.Error(err)
					return
				}
				if len(fingerprints) != 1 {
					t.Errorf("expected fingerprint count of %d, got %d", 1, len(fingerprints))
					return
				}
			}

			metric := clientmodel.Metric{}
			metric[clientmodel.MetricNameLabel] = clientmodel.LabelValue(fmt.Sprintf("metric_index_%d", metricIndex))

			for i := 0; i < numberOfSharedLabels; i++ {
				l := clientmodel.LabelName(fmt.Sprintf("shared_label_%d", i))
				v := clientmodel.LabelValue(fmt.Sprintf("label_%d", i))

				metric[l] = v
			}

			for i := 0; i < numberOfUnsharedLabels; i++ {
				l := clientmodel.LabelName(fmt.Sprintf("metric_index_%d_private_label_%d", metricIndex, i))
				v := clientmodel.LabelValue(fmt.Sprintf("private_label_%d", i))

				metric[l] = v
			}

			for i := 0; i < numberOfRangeScans; i++ {
				timestamps := metricTimestamps[metricIndex]

				var first int64
				var second int64

				for {
					firstCandidate := random.Int63n(int64(len(timestamps)))
					secondCandidate := random.Int63n(int64(len(timestamps)))

					smallest := int64(-1)
					largest := int64(-1)

					if firstCandidate == secondCandidate {
						continue
					} else if firstCandidate > secondCandidate {
						largest = firstCandidate
						smallest = secondCandidate
					} else {
						largest = secondCandidate
						smallest = firstCandidate
					}

					j := int64(0)
					for i := range timestamps {
						if j == smallest {
							first = i
						} else if j == largest {
							second = i
							break
						}
						j++
					}

					break
				}

				begin := first
				end := second

				if second < first {
					begin, end = second, first
				}

				interval := Interval{
					OldestInclusive: clientmodel.TimestampFromUnix(begin),
					NewestInclusive: clientmodel.TimestampFromUnix(end),
				}

				samples := Values{}
				fp := &clientmodel.Fingerprint{}
				fp.LoadFromMetric(metric)
				switch persistence := p.(type) {
				case View:
					samples = persistence.GetRangeValues(fp, interval)
					if len(samples) < 2 {
						t.Fatalf("expected sample count greater than %d, got %d", 2, len(samples))
					}
				case *LevelDBMetricPersistence:
					var err error
					samples, err = levelDBGetRangeValues(persistence, fp, interval)
					if err != nil {
						t.Fatal(err)
					}
					if len(samples) < 2 {
						t.Fatalf("expected sample count greater than %d, got %d", 2, len(samples))
					}
				default:
					t.Error("Unexpected type of MetricPersistence.")
				}
			}
		}

		return true
	}

	if err := quick.Check(stochastic, nil); err != nil {
		t.Error(err)
	}
}

// Test Definitions Follow

var testLevelDBBasicLifecycle = buildLevelDBTestPersistence("basic_lifecycle", BasicLifecycleTests)

func TestLevelDBBasicLifecycle(t *testing.T) {
	testLevelDBBasicLifecycle(t)
}

func BenchmarkLevelDBBasicLifecycle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBBasicLifecycle(b)
	}
}

var testLevelDBReadEmpty = buildLevelDBTestPersistence("read_empty", ReadEmptyTests)

func TestLevelDBReadEmpty(t *testing.T) {
	testLevelDBReadEmpty(t)
}

func BenchmarkLevelDBReadEmpty(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBReadEmpty(b)
	}
}

var testLevelDBAppendSampleAsPureSparseAppend = buildLevelDBTestPersistence("append_sample_as_pure_sparse_append", AppendSampleAsPureSparseAppendTests)

func TestLevelDBAppendSampleAsPureSparseAppend(t *testing.T) {
	testLevelDBAppendSampleAsPureSparseAppend(t)
}

func BenchmarkLevelDBAppendSampleAsPureSparseAppend(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBAppendSampleAsPureSparseAppend(b)
	}
}

var testLevelDBAppendSampleAsSparseAppendWithReads = buildLevelDBTestPersistence("append_sample_as_sparse_append_with_reads", AppendSampleAsSparseAppendWithReadsTests)

func TestLevelDBAppendSampleAsSparseAppendWithReads(t *testing.T) {
	testLevelDBAppendSampleAsSparseAppendWithReads(t)
}

func BenchmarkLevelDBAppendSampleAsSparseAppendWithReads(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBAppendSampleAsSparseAppendWithReads(b)
	}
}

var testLevelDBAppendSampleAsPureSingleEntityAppend = buildLevelDBTestPersistence("append_sample_as_pure_single_entity_append", AppendSampleAsPureSingleEntityAppendTests)

func TestLevelDBAppendSampleAsPureSingleEntityAppend(t *testing.T) {
	testLevelDBAppendSampleAsPureSingleEntityAppend(t)
}

func BenchmarkLevelDBAppendSampleAsPureSingleEntityAppend(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBAppendSampleAsPureSingleEntityAppend(b)
	}
}

func testLevelDBStochastic(t test.Tester) {
	persistenceMaker := func() (MetricPersistence, test.Closer) {
		temporaryDirectory := test.NewTemporaryDirectory("test_leveldb_stochastic", t)

		p, err := NewLevelDBMetricPersistence(temporaryDirectory.Path())
		if err != nil {
			t.Errorf("Could not start up LevelDB: %q\n", err)
		}

		return p, temporaryDirectory
	}

	StochasticTests(persistenceMaker, t)
}

func TestLevelDBStochastic(t *testing.T) {
	testLevelDBStochastic(t)
}

func BenchmarkLevelDBStochastic(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBStochastic(b)
	}
}

var testMemoryBasicLifecycle = buildMemoryTestPersistence(BasicLifecycleTests)

func TestMemoryBasicLifecycle(t *testing.T) {
	testMemoryBasicLifecycle(t)
}

func BenchmarkMemoryBasicLifecycle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryBasicLifecycle(b)
	}
}

var testMemoryReadEmpty = buildMemoryTestPersistence(ReadEmptyTests)

func TestMemoryReadEmpty(t *testing.T) {
	testMemoryReadEmpty(t)
}

func BenchmarkMemoryReadEmpty(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryReadEmpty(b)
	}
}

var testMemoryAppendSampleAsPureSparseAppend = buildMemoryTestPersistence(AppendSampleAsPureSparseAppendTests)

func TestMemoryAppendSampleAsPureSparseAppend(t *testing.T) {
	testMemoryAppendSampleAsPureSparseAppend(t)
}

func BenchmarkMemoryAppendSampleAsPureSparseAppend(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryAppendSampleAsPureSparseAppend(b)
	}
}

var testMemoryAppendSampleAsSparseAppendWithReads = buildMemoryTestPersistence(AppendSampleAsSparseAppendWithReadsTests)

func TestMemoryAppendSampleAsSparseAppendWithReads(t *testing.T) {
	testMemoryAppendSampleAsSparseAppendWithReads(t)
}

func BenchmarkMemoryAppendSampleAsSparseAppendWithReads(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryAppendSampleAsSparseAppendWithReads(b)
	}
}

var testMemoryAppendSampleAsPureSingleEntityAppend = buildMemoryTestPersistence(AppendSampleAsPureSingleEntityAppendTests)

func TestMemoryAppendSampleAsPureSingleEntityAppend(t *testing.T) {
	testMemoryAppendSampleAsPureSingleEntityAppend(t)
}

func BenchmarkMemoryAppendSampleAsPureSingleEntityAppend(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryAppendSampleAsPureSingleEntityAppend(b)
	}
}

func testMemoryStochastic(t test.Tester) {
	persistenceMaker := func() (MetricPersistence, test.Closer) {
		return NewMemorySeriesStorage(MemorySeriesOptions{}), test.NilCloser
	}

	StochasticTests(persistenceMaker, t)
}

func TestMemoryStochastic(t *testing.T) {
	testMemoryStochastic(t)
}

func BenchmarkMemoryStochastic(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryStochastic(b)
	}
}
