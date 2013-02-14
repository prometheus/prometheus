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

package leveldb

import (
	"code.google.com/p/goprotobuf/proto"
	"fmt"
	"github.com/prometheus/prometheus/model"
	dto "github.com/prometheus/prometheus/model/generated"
	"github.com/prometheus/prometheus/utility/test"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"testing"
	"testing/quick"
	"time"
)

const (
	stochasticMaximumVariance = 8
)

var testBasicLifecycle func(t test.Tester) = func(t test.Tester) {
	temporaryDirectory, temporaryDirectoryErr := ioutil.TempDir("", "leveldb_metric_persistence_test")

	if temporaryDirectoryErr != nil {
		t.Errorf("Could not create test directory: %q\n", temporaryDirectoryErr)
		return
	}

	defer func() {
		if removeAllErr := os.RemoveAll(temporaryDirectory); removeAllErr != nil {
			t.Errorf("Could not remove temporary directory: %q\n", removeAllErr)
		}
	}()

	persistence, openErr := NewLevelDBMetricPersistence(temporaryDirectory)

	if openErr != nil {
		t.Errorf("Could not create LevelDB Metric Persistence: %q\n", openErr)
	}

	if persistence == nil {
		t.Errorf("Received nil LevelDB Metric Persistence.\n")
		return
	}

	closeErr := persistence.Close()

	if closeErr != nil {
		t.Errorf("Could not close LevelDB Metric Persistence: %q\n", closeErr)
	}
}

func TestBasicLifecycle(t *testing.T) {
	testBasicLifecycle(t)
}

func BenchmarkBasicLifecycle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testBasicLifecycle(b)
	}
}

var testReadEmpty func(t test.Tester) = func(t test.Tester) {
	temporaryDirectory, _ := ioutil.TempDir("", "leveldb_metric_persistence_test")

	defer func() {
		if removeAllErr := os.RemoveAll(temporaryDirectory); removeAllErr != nil {
			t.Errorf("Could not remove temporary directory: %q\n", removeAllErr)
		}
	}()

	persistence, _ := NewLevelDBMetricPersistence(temporaryDirectory)

	defer func() {
		persistence.Close()
	}()

	hasLabelPair := func(x int) bool {
		name := string(x)
		value := string(x)

		dto := &dto.LabelPair{
			Name:  proto.String(name),
			Value: proto.String(value),
		}

		has, hasErr := persistence.HasLabelPair(dto)

		if hasErr != nil {
			return false
		}

		return has == false
	}

	if hasPairErr := quick.Check(hasLabelPair, nil); hasPairErr != nil {
		t.Error(hasPairErr)
	}
	hasLabelName := func(x int) bool {
		name := string(x)

		dto := &dto.LabelName{
			Name: proto.String(name),
		}

		has, hasErr := persistence.HasLabelName(dto)

		if hasErr != nil {
			return false
		}

		return has == false
	}

	if hasNameErr := quick.Check(hasLabelName, nil); hasNameErr != nil {
		t.Error(hasNameErr)
	}

	getLabelPairFingerprints := func(x int) bool {
		name := string(x)
		value := string(x)

		dto := &dto.LabelPair{
			Name:  proto.String(name),
			Value: proto.String(value),
		}

		fingerprints, fingerprintsErr := persistence.getFingerprintsForLabelSet(dto)

		if fingerprintsErr != nil {
			return false
		}

		if fingerprints == nil {
			return false
		}

		return len(fingerprints.Member) == 0
	}

	if labelPairFingerprintsErr := quick.Check(getLabelPairFingerprints, nil); labelPairFingerprintsErr != nil {
		t.Error(labelPairFingerprintsErr)
	}

	getLabelNameFingerprints := func(x int) bool {
		name := string(x)

		dto := &dto.LabelName{
			Name: proto.String(name),
		}

		fingerprints, fingerprintsErr := persistence.GetLabelNameFingerprints(dto)

		if fingerprintsErr != nil {
			return false
		}

		if fingerprints == nil {
			return false
		}

		return len(fingerprints.Member) == 0
	}

	if labelNameFingerprintsErr := quick.Check(getLabelNameFingerprints, nil); labelNameFingerprintsErr != nil {
		t.Error(labelNameFingerprintsErr)
	}
}

func TestReadEmpty(t *testing.T) {
	testReadEmpty(t)
}

func BenchmarkReadEmpty(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testReadEmpty(b)
	}
}

var testAppendSampleAsPureSparseAppend = func(t test.Tester) {
	temporaryDirectory, _ := ioutil.TempDir("", "leveldb_metric_persistence_test")

	defer func() {
		if removeAllErr := os.RemoveAll(temporaryDirectory); removeAllErr != nil {
			t.Errorf("Could not remove temporary directory: %q\n", removeAllErr)
		}
	}()

	persistence, _ := NewLevelDBMetricPersistence(temporaryDirectory)

	defer func() {
		persistence.Close()
	}()

	appendSample := func(x int) bool {
		v := model.SampleValue(x)
		t := time.Unix(int64(x), int64(x))
		l := model.Metric{model.LabelName(x): model.LabelValue(x)}

		sample := model.Sample{
			Value:     v,
			Timestamp: t,
			Metric:    l,
		}

		appendErr := persistence.AppendSample(sample)

		return appendErr == nil
	}

	if appendErr := quick.Check(appendSample, nil); appendErr != nil {
		t.Error(appendErr)
	}
}

func TestAppendSampleAsPureSparseAppend(t *testing.T) {
	testAppendSampleAsPureSparseAppend(t)
}

func BenchmarkAppendSampleAsPureSparseAppend(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testAppendSampleAsPureSparseAppend(b)
	}
}

var testAppendSampleAsSparseAppendWithReads func(t test.Tester) = func(t test.Tester) {
	temporaryDirectory, _ := ioutil.TempDir("", "leveldb_metric_persistence_test")

	defer func() {
		if removeAllErr := os.RemoveAll(temporaryDirectory); removeAllErr != nil {
			t.Errorf("Could not remove temporary directory: %q\n", removeAllErr)
		}
	}()

	persistence, _ := NewLevelDBMetricPersistence(temporaryDirectory)

	defer func() {
		persistence.Close()
	}()

	appendSample := func(x int) bool {
		v := model.SampleValue(x)
		t := time.Unix(int64(x), int64(x))
		l := model.Metric{model.LabelName(x): model.LabelValue(x)}

		sample := model.Sample{
			Value:     v,
			Timestamp: t,
			Metric:    l,
		}

		appendErr := persistence.AppendSample(sample)

		if appendErr != nil {
			return false
		}

		labelNameDTO := &dto.LabelName{
			Name: proto.String(string(x)),
		}

		hasLabelName, hasLabelNameErr := persistence.HasLabelName(labelNameDTO)

		if hasLabelNameErr != nil {
			return false
		}

		if !hasLabelName {
			return false
		}

		labelPairDTO := &dto.LabelPair{
			Name:  proto.String(string(x)),
			Value: proto.String(string(x)),
		}

		hasLabelPair, hasLabelPairErr := persistence.HasLabelPair(labelPairDTO)

		if hasLabelPairErr != nil {
			return false
		}

		if !hasLabelPair {
			return false
		}

		labelNameFingerprints, labelNameFingerprintsErr := persistence.GetLabelNameFingerprints(labelNameDTO)

		if labelNameFingerprintsErr != nil {
			return false
		}

		if labelNameFingerprints == nil {
			return false
		}

		if len(labelNameFingerprints.Member) != 1 {
			return false
		}

		labelPairFingerprints, labelPairFingerprintsErr := persistence.getFingerprintsForLabelSet(labelPairDTO)

		if labelPairFingerprintsErr != nil {
			return false
		}

		if labelPairFingerprints == nil {
			return false
		}

		if len(labelPairFingerprints.Member) != 1 {
			return false
		}

		return true
	}

	if appendErr := quick.Check(appendSample, nil); appendErr != nil {
		t.Error(appendErr)
	}
}

func TestAppendSampleAsSparseAppendWithReads(t *testing.T) {
	testAppendSampleAsSparseAppendWithReads(t)
}

func BenchmarkAppendSampleAsSparseAppendWithReads(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testAppendSampleAsSparseAppendWithReads(b)
	}
}

func TestAppendSampleAsPureSingleEntityAppend(t *testing.T) {
	temporaryDirectory, _ := ioutil.TempDir("", "leveldb_metric_persistence_test")

	defer func() {
		if removeAllErr := os.RemoveAll(temporaryDirectory); removeAllErr != nil {
			t.Errorf("Could not remove temporary directory: %q\n", removeAllErr)
		}
	}()

	persistence, _ := NewLevelDBMetricPersistence(temporaryDirectory)

	defer func() {
		persistence.Close()
	}()

	appendSample := func(x int) bool {
		sample := model.Sample{
			Value:     model.SampleValue(float32(x)),
			Timestamp: time.Unix(int64(x), 0),
			Metric:    model.Metric{"name": "my_metric"},
		}

		appendErr := persistence.AppendSample(sample)

		return appendErr == nil
	}

	if appendErr := quick.Check(appendSample, nil); appendErr != nil {
		t.Error(appendErr)
	}
}

func TestStochastic(t *testing.T) {
	stochastic := func(x int) bool {
		s := time.Now()
		seed := rand.NewSource(int64(x))
		random := rand.New(seed)

		numberOfMetrics := random.Intn(stochasticMaximumVariance) + 1
		numberOfSharedLabels := random.Intn(stochasticMaximumVariance)
		numberOfUnsharedLabels := random.Intn(stochasticMaximumVariance)
		numberOfSamples := random.Intn(stochasticMaximumVariance) + 2
		numberOfRangeScans := random.Intn(stochasticMaximumVariance)

		temporaryDirectory, _ := ioutil.TempDir("", "leveldb_metric_persistence_test")

		defer func() {
			if removeAllErr := os.RemoveAll(temporaryDirectory); removeAllErr != nil {
				t.Errorf("Could not remove temporary directory: %q\n", removeAllErr)
			}
		}()

		persistence, _ := NewLevelDBMetricPersistence(temporaryDirectory)

		defer func() {
			persistence.Close()
		}()

		metricTimestamps := make(map[int]map[int64]bool)
		metricEarliestSample := make(map[int]int64)
		metricNewestSample := make(map[int]int64)

		for metricIndex := 0; metricIndex < numberOfMetrics; metricIndex++ {
			sample := model.Sample{
				Metric: model.Metric{},
			}

			v := model.LabelValue(fmt.Sprintf("metric_index_%d", metricIndex))
			sample.Metric["name"] = v

			for sharedLabelIndex := 0; sharedLabelIndex < numberOfSharedLabels; sharedLabelIndex++ {
				l := model.LabelName(fmt.Sprintf("shared_label_%d", sharedLabelIndex))
				v := model.LabelValue(fmt.Sprintf("label_%d", sharedLabelIndex))

				sample.Metric[l] = v
			}

			for unsharedLabelIndex := 0; unsharedLabelIndex < numberOfUnsharedLabels; unsharedLabelIndex++ {
				l := model.LabelName(fmt.Sprintf("metric_index_%d_private_label_%d", metricIndex, unsharedLabelIndex))
				v := model.LabelValue(fmt.Sprintf("private_label_%d", unsharedLabelIndex))

				sample.Metric[l] = v
			}

			timestamps := make(map[int64]bool)
			metricTimestamps[metricIndex] = timestamps
			var newestSample int64 = math.MinInt64
			var oldestSample int64 = math.MaxInt64
			var nextTimestamp func() int64

			nextTimestamp = func() int64 {
				var candidate int64
				candidate = random.Int63n(math.MaxInt32 - 1)

				if _, has := timestamps[candidate]; has {
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

			for sampleIndex := 0; sampleIndex < numberOfSamples; sampleIndex++ {
				sample.Timestamp = time.Unix(nextTimestamp(), 0)
				sample.Value = model.SampleValue(sampleIndex)

				appendErr := persistence.AppendSample(sample)

				if appendErr != nil {
					return false
				}
			}

			metricEarliestSample[metricIndex] = oldestSample
			metricNewestSample[metricIndex] = newestSample

			for sharedLabelIndex := 0; sharedLabelIndex < numberOfSharedLabels; sharedLabelIndex++ {
				labelPair := &dto.LabelPair{
					Name:  proto.String(fmt.Sprintf("shared_label_%d", sharedLabelIndex)),
					Value: proto.String(fmt.Sprintf("label_%d", sharedLabelIndex)),
				}

				hasLabelPair, hasLabelPairErr := persistence.HasLabelPair(labelPair)

				if hasLabelPairErr != nil {
					return false
				}

				if hasLabelPair != true {
					return false
				}

				labelName := &dto.LabelName{
					Name: proto.String(fmt.Sprintf("shared_label_%d", sharedLabelIndex)),
				}

				hasLabelName, hasLabelNameErr := persistence.HasLabelName(labelName)

				if hasLabelNameErr != nil {
					return false
				}

				if hasLabelName != true {
					return false
				}
			}
		}

		for sharedIndex := 0; sharedIndex < numberOfSharedLabels; sharedIndex++ {
			labelName := &dto.LabelName{
				Name: proto.String(fmt.Sprintf("shared_label_%d", sharedIndex)),
			}
			fingerprints, fingerprintsErr := persistence.GetLabelNameFingerprints(labelName)

			if fingerprintsErr != nil {
				return false
			}

			if fingerprints == nil {
				return false
			}

			if len(fingerprints.Member) != numberOfMetrics {
				return false
			}
		}

		for metricIndex := 0; metricIndex < numberOfMetrics; metricIndex++ {
			for unsharedLabelIndex := 0; unsharedLabelIndex < numberOfUnsharedLabels; unsharedLabelIndex++ {
				labelPair := &dto.LabelPair{
					Name:  proto.String(fmt.Sprintf("metric_index_%d_private_label_%d", metricIndex, unsharedLabelIndex)),
					Value: proto.String(fmt.Sprintf("private_label_%d", unsharedLabelIndex)),
				}

				hasLabelPair, hasLabelPairErr := persistence.HasLabelPair(labelPair)

				if hasLabelPairErr != nil {
					return false
				}

				if hasLabelPair != true {
					return false
				}

				labelPairFingerprints, labelPairFingerprintsErr := persistence.getFingerprintsForLabelSet(labelPair)

				if labelPairFingerprintsErr != nil {
					return false
				}

				if labelPairFingerprints == nil {
					return false
				}

				if len(labelPairFingerprints.Member) != 1 {
					return false
				}

				labelName := &dto.LabelName{
					Name: proto.String(fmt.Sprintf("metric_index_%d_private_label_%d", metricIndex, unsharedLabelIndex)),
				}

				hasLabelName, hasLabelNameErr := persistence.HasLabelName(labelName)

				if hasLabelNameErr != nil {
					return false
				}

				if hasLabelName != true {
					return false
				}

				labelNameFingerprints, labelNameFingerprintsErr := persistence.GetLabelNameFingerprints(labelName)

				if labelNameFingerprintsErr != nil {
					return false
				}

				if labelNameFingerprints == nil {
					return false
				}

				if len(labelNameFingerprints.Member) != 1 {
					return false
				}
			}

			metric := make(model.Metric)

			metric["name"] = model.LabelValue(fmt.Sprintf("metric_index_%d", metricIndex))

			for i := 0; i < numberOfSharedLabels; i++ {
				l := model.LabelName(fmt.Sprintf("shared_label_%d", i))
				v := model.LabelValue(fmt.Sprintf("label_%d", i))

				metric[l] = v
			}

			for i := 0; i < numberOfUnsharedLabels; i++ {
				l := model.LabelName(fmt.Sprintf("metric_index_%d_private_label_%d", metricIndex, i))
				v := model.LabelValue(fmt.Sprintf("private_label_%d", i))

				metric[l] = v
			}

			for i := 0; i < numberOfRangeScans; i++ {
				timestamps := metricTimestamps[metricIndex]

				var first int64 = 0
				var second int64 = 0

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

				interval := model.Interval{
					OldestInclusive: time.Unix(begin, 0),
					NewestInclusive: time.Unix(end, 0),
				}

				rangeValues, rangeErr := persistence.GetSamplesForMetric(metric, interval)

				if rangeErr != nil {
					return false
				}

				if len(rangeValues) < 2 {
					return false
				}
			}
		}

		fmt.Printf("Duration %q\n", time.Now().Sub(s))

		return true
	}

	if stochasticError := quick.Check(stochastic, nil); stochasticError != nil {
		t.Error(stochasticError)
	}
}

func TestGetFingerprintsForLabelSet(t *testing.T) {
	temporaryDirectory, _ := ioutil.TempDir("", "leveldb_metric_persistence_test")

	defer func() {
		if removeAllErr := os.RemoveAll(temporaryDirectory); removeAllErr != nil {
			t.Errorf("Could not remove temporary directory: %q\n", removeAllErr)
		}
	}()

	persistence, _ := NewLevelDBMetricPersistence(temporaryDirectory)

	defer func() {
		persistence.Close()
	}()

	appendErr := persistence.AppendSample(model.Sample{
		Value:     model.SampleValue(0),
		Timestamp: time.Unix(0, 0),
		Metric: model.Metric{
			"name":         "my_metric",
			"request_type": "your_mom",
		},
	})

	if appendErr != nil {
		t.Error(appendErr)
	}

	appendErr = persistence.AppendSample(model.Sample{
		Value:     model.SampleValue(0),
		Timestamp: time.Unix(int64(0), 0),
		Metric: model.Metric{
			"name":         "my_metric",
			"request_type": "your_dad",
		},
	})

	if appendErr != nil {
		t.Error(appendErr)
	}

	result, getErr := persistence.GetFingerprintsForLabelSet(model.LabelSet{
		model.LabelName("name"): model.LabelValue("my_metric"),
	})

	if getErr != nil {
		t.Error(getErr)
	}

	if len(result) != 2 {
		t.Errorf("Expected two elements.")
	}

	result, getErr = persistence.GetFingerprintsForLabelSet(model.LabelSet{
		model.LabelName("request_type"): model.LabelValue("your_mom"),
	})

	if getErr != nil {
		t.Error(getErr)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	result, getErr = persistence.GetFingerprintsForLabelSet(model.LabelSet{
		model.LabelName("request_type"): model.LabelValue("your_dad"),
	})

	if getErr != nil {
		t.Error(getErr)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}
}

func TestGetFingerprintsForLabelName(t *testing.T) {
	temporaryDirectory, _ := ioutil.TempDir("", "leveldb_metric_persistence_test")

	defer func() {
		if removeAllErr := os.RemoveAll(temporaryDirectory); removeAllErr != nil {
			t.Errorf("Could not remove temporary directory: %q\n", removeAllErr)
		}
	}()

	persistence, _ := NewLevelDBMetricPersistence(temporaryDirectory)

	defer func() {
		persistence.Close()
	}()

	appendErr := persistence.AppendSample(model.Sample{
		Value:     model.SampleValue(0),
		Timestamp: time.Unix(0, 0),
		Metric: model.Metric{
			"name":         "my_metric",
			"request_type": "your_mom",
			"language":     "english",
		},
	})

	if appendErr != nil {
		t.Error(appendErr)
	}

	appendErr = persistence.AppendSample(model.Sample{
		Value:     model.SampleValue(0),
		Timestamp: time.Unix(int64(0), 0),
		Metric: model.Metric{
			"name":         "my_metric",
			"request_type": "your_dad",
			"sprache":      "deutsch",
		},
	})

	if appendErr != nil {
		t.Error(appendErr)
	}

	b := model.LabelName("name")
	result, getErr := persistence.GetFingerprintsForLabelName(&b)

	if getErr != nil {
		t.Error(getErr)
	}

	if len(result) != 2 {
		t.Errorf("Expected two elements.")
	}

	b = model.LabelName("request_type")
	result, getErr = persistence.GetFingerprintsForLabelName(&b)

	if getErr != nil {
		t.Error(getErr)
	}

	if len(result) != 2 {
		t.Errorf("Expected two elements.")
	}

	b = model.LabelName("language")
	result, getErr = persistence.GetFingerprintsForLabelName(&b)

	if getErr != nil {
		t.Error(getErr)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	b = model.LabelName("sprache")
	result, getErr = persistence.GetFingerprintsForLabelName(&b)

	if getErr != nil {
		t.Error(getErr)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}
}

func TestGetMetricForFingerprint(t *testing.T) {
	temporaryDirectory, _ := ioutil.TempDir("", "leveldb_metric_persistence_test")

	defer func() {
		if removeAllErr := os.RemoveAll(temporaryDirectory); removeAllErr != nil {
			t.Errorf("Could not remove temporary directory: %q\n", removeAllErr)
		}
	}()

	persistence, _ := NewLevelDBMetricPersistence(temporaryDirectory)

	defer func() {
		persistence.Close()
	}()

	appendErr := persistence.AppendSample(model.Sample{
		Value:     model.SampleValue(0),
		Timestamp: time.Unix(0, 0),
		Metric: model.Metric{
			"request_type": "your_mom",
		},
	})

	if appendErr != nil {
		t.Error(appendErr)
	}

	appendErr = persistence.AppendSample(model.Sample{
		Value:     model.SampleValue(0),
		Timestamp: time.Unix(int64(0), 0),
		Metric: model.Metric{
			"request_type": "your_dad",
			"one-off":      "value",
		},
	})

	if appendErr != nil {
		t.Error(appendErr)
	}

	result, getErr := persistence.GetFingerprintsForLabelSet(model.LabelSet{
		model.LabelName("request_type"): model.LabelValue("your_mom"),
	})

	if getErr != nil {
		t.Error(getErr)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	v, e := persistence.GetMetricForFingerprint(result[0])
	if e != nil {
		t.Error(e)
	}

	if len(*v) != 1 {
		t.Errorf("Expected one-dimensional metric.")
	}

	if (*v)["request_type"] != "your_mom" {
		t.Errorf("Expected metric to match.")
	}

	result, getErr = persistence.GetFingerprintsForLabelSet(model.LabelSet{
		model.LabelName("request_type"): model.LabelValue("your_dad"),
	})

	if getErr != nil {
		t.Error(getErr)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	v, e = persistence.GetMetricForFingerprint(result[0])

	if e != nil {
		t.Error(e)
	}

	if len(*v) != 2 {
		t.Errorf("Expected one-dimensional metric.")
	}

	if (*v)["request_type"] != "your_dad" {
		t.Errorf("Expected metric to match.")
	}

	if (*v)["one-off"] != "value" {
		t.Errorf("Expected metric to match.")
	}
}
