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
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility/test"
	"io/ioutil"
	"testing"
)

var testBasicLifecycle = buildTestPersistence("basic_lifecycle", metric.BasicLifecycleTests)

func TestBasicLifecycle(t *testing.T) {
	testBasicLifecycle(t)
}

func BenchmarkBasicLifecycle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testBasicLifecycle(b)
	}
}

var testReadEmpty = buildTestPersistence("read_empty", metric.ReadEmptyTests)

func TestReadEmpty(t *testing.T) {
	testReadEmpty(t)
}

func BenchmarkReadEmpty(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testReadEmpty(b)
	}
}

var testAppendSampleAsPureSparseAppend = buildTestPersistence("append_sample_as_pure_sparse_append", metric.AppendSampleAsPureSparseAppendTests)

func TestAppendSampleAsPureSparseAppend(t *testing.T) {
	testAppendSampleAsPureSparseAppend(t)
}

func BenchmarkAppendSampleAsPureSparseAppend(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testAppendSampleAsPureSparseAppend(b)
	}
}

var testAppendSampleAsSparseAppendWithReads = buildTestPersistence("append_sample_as_sparse_append_with_reads", metric.AppendSampleAsSparseAppendWithReadsTests)

func TestAppendSampleAsSparseAppendWithReads(t *testing.T) {
	testAppendSampleAsSparseAppendWithReads(t)
}

func BenchmarkAppendSampleAsSparseAppendWithReads(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testAppendSampleAsSparseAppendWithReads(b)
	}
}

var testAppendSampleAsPureSingleEntityAppend = buildTestPersistence("append_sample_as_pure_single_entity_append", metric.AppendSampleAsPureSingleEntityAppendTests)

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
		temporaryDirectory, err := ioutil.TempDir("", "test_leveldb_stochastic")
		if err != nil {
			t.Errorf("Could not create test directory: %q\n", err)
		}

		p, err := NewLevelDBMetricPersistence(temporaryDirectory)
		if err != nil {
			t.Errorf("Could not start up LevelDB: %q\n", err)
		}

		return p
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
