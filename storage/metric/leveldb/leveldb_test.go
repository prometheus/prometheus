// Copyright 2012 Prometheus Team
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
	"github.com/matttproud/prometheus/model"
	data "github.com/matttproud/prometheus/model/generated"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"testing"
	"testing/quick"
	"time"
)

const (
	stochasticMaximumVariance = 64
)

func TestBasicLifecycle(t *testing.T) {
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

func TestReadEmpty(t *testing.T) {
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

		ddo := &data.LabelPairDDO{
			Name:  proto.String(name),
			Value: proto.String(value),
		}

		has, hasErr := persistence.HasLabelPair(ddo)

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

		ddo := &data.LabelNameDDO{
			Name: proto.String(name),
		}

		has, hasErr := persistence.HasLabelName(ddo)

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

		ddo := &data.LabelPairDDO{
			Name:  proto.String(name),
			Value: proto.String(value),
		}

		fingerprints, fingerprintsErr := persistence.GetLabelPairFingerprints(ddo)

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

		ddo := &data.LabelNameDDO{
			Name: proto.String(name),
		}

		fingerprints, fingerprintsErr := persistence.GetLabelNameFingerprints(ddo)

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

func TestAppendSampleAsPureSparseAppend(t *testing.T) {
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
		sample := &model.Sample{
			Value:     model.SampleValue(float32(x)),
			Timestamp: time.Unix(int64(x), int64(x)),
			Labels:    model.LabelPairs{string(x): string(x)},
		}

		appendErr := persistence.AppendSample(sample)

		return appendErr == nil
	}

	if appendErr := quick.Check(appendSample, nil); appendErr != nil {
		t.Error(appendErr)
	}
}

func TestAppendSampleAsSparseAppendWithReads(t *testing.T) {
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
		sample := &model.Sample{
			Value:     model.SampleValue(float32(x)),
			Timestamp: time.Unix(int64(x), int64(x)),
			Labels:    model.LabelPairs{string(x): string(x)},
		}

		appendErr := persistence.AppendSample(sample)

		if appendErr != nil {
			return false
		}

		labelNameDDO := &data.LabelNameDDO{
			Name: proto.String(string(x)),
		}

		hasLabelName, hasLabelNameErr := persistence.HasLabelName(labelNameDDO)

		if hasLabelNameErr != nil {
			return false
		}

		if !hasLabelName {
			return false
		}

		labelPairDDO := &data.LabelPairDDO{
			Name:  proto.String(string(x)),
			Value: proto.String(string(x)),
		}

		hasLabelPair, hasLabelPairErr := persistence.HasLabelPair(labelPairDDO)

		if hasLabelPairErr != nil {
			return false
		}

		if !hasLabelPair {
			return false
		}

		labelNameFingerprints, labelNameFingerprintsErr := persistence.GetLabelNameFingerprints(labelNameDDO)

		if labelNameFingerprintsErr != nil {
			return false
		}

		if labelNameFingerprints == nil {
			return false
		}

		if len(labelNameFingerprints.Member) != 1 {
			return false
		}

		labelPairFingerprints, labelPairFingerprintsErr := persistence.GetLabelPairFingerprints(labelPairDDO)

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
		sample := &model.Sample{
			Value:     model.SampleValue(float32(x)),
			Timestamp: time.Unix(int64(x), 0),
			Labels:    model.LabelPairs{"name": "my_metric"},
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
			sample := &model.Sample{
				Labels: model.LabelPairs{},
			}

			sample.Labels["name"] = fmt.Sprintf("metric_index_%d", metricIndex)

			for sharedLabelIndex := 0; sharedLabelIndex < numberOfSharedLabels; sharedLabelIndex++ {
				sample.Labels[fmt.Sprintf("shared_label_%d", sharedLabelIndex)] = fmt.Sprintf("label_%d", sharedLabelIndex)
			}

			for unsharedLabelIndex := 0; unsharedLabelIndex < numberOfUnsharedLabels; unsharedLabelIndex++ {
				sample.Labels[fmt.Sprintf("metric_index_%d_private_label_%d", metricIndex, unsharedLabelIndex)] = fmt.Sprintf("private_label_%d", unsharedLabelIndex)
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
				labelPair := &data.LabelPairDDO{
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

				labelName := &data.LabelNameDDO{
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
			labelName := &data.LabelNameDDO{
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
				labelPair := &data.LabelPairDDO{
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

				labelPairFingerprints, labelPairFingerprintsErr := persistence.GetLabelPairFingerprints(labelPair)

				if labelPairFingerprintsErr != nil {
					return false
				}

				if labelPairFingerprints == nil {
					return false
				}

				if len(labelPairFingerprints.Member) != 1 {
					return false
				}

				labelName := &data.LabelNameDDO{
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

			metric["name"] = fmt.Sprintf("metric_index_%d", metricIndex)

			for i := 0; i < numberOfSharedLabels; i++ {
				metric[fmt.Sprintf("shared_label_%d", i)] = fmt.Sprintf("label_%d", i)
			}

			for i := 0; i < numberOfUnsharedLabels; i++ {
				metric[fmt.Sprintf("metric_index_%d_private_label_%d", metricIndex, i)] = fmt.Sprintf("private_label_%d", i)
			}

			watermarks, count, watermarksErr := persistence.GetWatermarksForMetric(metric)

			if watermarksErr != nil {
				return false
			}

			if watermarks == nil {
				return false
			}

			if count != numberOfSamples {
				return false
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
