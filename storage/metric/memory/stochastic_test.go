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
	"github.com/prometheus/prometheus/utility/test"
	"testing"
)

func buildTestPersistence(f func(p metric.MetricPersistence, t test.Tester)) func(t test.Tester) {
	return func(t test.Tester) {

		p := NewMemorySeriesStorage()

		defer func() {
			err := p.Close()
			if err != nil {
				t.Errorf("Anomaly while closing database: %q\n", err)
			}
		}()

		f(p, t)
	}
}

var testBasicLifecycle = buildTestPersistence(metric.BasicLifecycleTests)

func TestBasicLifecycle(t *testing.T) {
	testBasicLifecycle(t)
}

func BenchmarkBasicLifecycle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testBasicLifecycle(b)
	}
}

var testReadEmpty = buildTestPersistence(metric.ReadEmptyTests)

func TestReadEmpty(t *testing.T) {
	testReadEmpty(t)
}

func BenchmarkReadEmpty(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testReadEmpty(b)
	}
}

var testAppendSampleAsPureSparseAppend = buildTestPersistence(metric.AppendSampleAsPureSparseAppendTests)

func TestAppendSampleAsPureSparseAppend(t *testing.T) {
	testAppendSampleAsPureSparseAppend(t)
}

func BenchmarkAppendSampleAsPureSparseAppend(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testAppendSampleAsPureSparseAppend(b)
	}
}

var testAppendSampleAsSparseAppendWithReads = buildTestPersistence(metric.AppendSampleAsSparseAppendWithReadsTests)

func TestAppendSampleAsSparseAppendWithReads(t *testing.T) {
	testAppendSampleAsSparseAppendWithReads(t)
}

func BenchmarkAppendSampleAsSparseAppendWithReads(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testAppendSampleAsSparseAppendWithReads(b)
	}
}

var testAppendSampleAsPureSingleEntityAppend = buildTestPersistence(metric.AppendSampleAsPureSingleEntityAppendTests)

func TestAppendSampleAsPureSingleEntityAppend(t *testing.T) {
	testAppendSampleAsPureSingleEntityAppend(t)
}

func BenchmarkAppendSampleAsPureSingleEntityAppend(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testAppendSampleAsPureSingleEntityAppend(b)
	}
}

func testStochastic(t test.Tester) {
	persistenceMaker := func() metric.MetricPersistence {
		return NewMemorySeriesStorage()
	}

	metric.StochasticTests(persistenceMaker, t)
}

func TestStochastic(t *testing.T) {
	testStochastic(t)
}

func BenchmarkStochastic(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testStochastic(b)
	}
}
