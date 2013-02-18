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

package memory

import (
	"github.com/prometheus/prometheus/storage/metric"
	"testing"
)

var testGetFingerprintsForLabelSet = buildTestPersistence(metric.GetFingerprintsForLabelSetTests)

func TestGetFingerprintsForLabelSet(t *testing.T) {
	testGetFingerprintsForLabelSet(t)
}

func BenchmarkGetFingerprintsForLabelSet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testGetFingerprintsForLabelSet(b)
	}
}

var testGetFingerprintsForLabelName = buildTestPersistence(metric.GetFingerprintsForLabelNameTests)

func TestGetFingerprintsForLabelName(t *testing.T) {
	testGetFingerprintsForLabelName(t)
}

func BenchmarkGetFingerprintsForLabelName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testGetFingerprintsForLabelName(b)
	}
}

var testGetMetricForFingerprint = buildTestPersistence(metric.GetMetricForFingerprintTests)

func TestGetMetricForFingerprint(t *testing.T) {
	testGetMetricForFingerprint(t)
}

func BenchmarkGetMetricForFingerprint(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testGetMetricForFingerprint(b)
	}
}

var testAppendRepeatingValues = buildTestPersistence(metric.AppendRepeatingValuesTests)

func TestAppendRepeatingValues(t *testing.T) {
	testAppendRepeatingValues(t)
}

func BenchmarkAppendRepeatingValues(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testAppendRepeatingValues(b)
	}
}
