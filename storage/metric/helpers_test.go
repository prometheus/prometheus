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
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/utility/test"
	"time"
)

var (
	// ``hg clone https://code.google.com/p/go ; cd go ; hg log | tail -n 20``
	usEastern, _ = time.LoadLocation("US/Eastern")
	testInstant  = time.Date(1972, 7, 18, 19, 5, 45, 0, usEastern).In(time.UTC)
)

func testAppendSample(p MetricPersistence, s model.Sample, t test.Tester) {
	err := p.AppendSample(s)
	if err != nil {
		t.Fatal(err)
	}
}

func buildLevelDBTestPersistencesMaker(name string, t test.Tester) func() (MetricPersistence, test.Closer) {
	return func() (MetricPersistence, test.Closer) {
		temporaryDirectory := test.NewTemporaryDirectory("get_value_at_time", t)

		p, err := NewLevelDBMetricPersistence(temporaryDirectory.Path())
		if err != nil {
			t.Errorf("Could not start up LevelDB: %q\n", err)
		}

		return p, temporaryDirectory
	}
}

func buildLevelDBTestPersistence(name string, f func(p MetricPersistence, t test.Tester)) func(t test.Tester) {
	return func(t test.Tester) {
		temporaryDirectory := test.NewTemporaryDirectory(fmt.Sprintf("test_leveldb_%s", name), t)
		defer temporaryDirectory.Close()

		p, err := NewLevelDBMetricPersistence(temporaryDirectory.Path())
		if err != nil {
			t.Errorf("Could not create LevelDB Metric Persistence: %q\n", err)
		}

		defer p.Close()

		f(p, t)
	}
}

func buildMemoryTestPersistence(f func(p MetricPersistence, t test.Tester)) func(t test.Tester) {
	return func(t test.Tester) {

		p := NewMemorySeriesStorage()

		defer p.Close()

		f(p, t)
	}
}

type testTieredStorageCloser struct {
	storage   *TieredStorage
	directory test.Closer
}

func (t testTieredStorageCloser) Close() {
	t.storage.Close()
	t.directory.Close()
}

func NewTestTieredStorage(t test.Tester) (storage *TieredStorage, closer test.Closer) {
	var directory test.TemporaryDirectory
	directory = test.NewTemporaryDirectory("test_tiered_storage", t)
	storage, err := NewTieredStorage(2500, 1000, 5*time.Second, 0*time.Second, directory.Path())

	if err != nil {
		if storage != nil {
			storage.Close()
		}
		directory.Close()
		t.Fatalf("Error creating storage: %s", err)
	}

	if storage == nil {
		directory.Close()
		t.Fatalf("storage == nil")
	}

	go storage.Serve()
	closer = &testTieredStorageCloser{
		storage:   storage,
		directory: directory,
	}
	return
}
