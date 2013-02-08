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
	"fmt"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility/test"
	"io"
	"io/ioutil"
	"os"
)

type purger struct {
	path string
}

func (p purger) Close() error {
	return os.RemoveAll(p.path)
}

func buildTestPersistencesMaker(name string, t test.Tester) func() (metric.MetricPersistence, io.Closer) {
	return func() (metric.MetricPersistence, io.Closer) {
		temporaryDirectory, err := ioutil.TempDir("", "get_value_at_time")
		if err != nil {
			t.Errorf("Could not create test directory: %q\n", err)
		}

		p, err := NewLevelDBMetricPersistence(temporaryDirectory)
		if err != nil {
			t.Errorf("Could not start up LevelDB: %q\n", err)
		}

		purger := purger{
			path: temporaryDirectory,
		}

		return p, purger
	}

}

func buildTestPersistence(name string, f func(p metric.MetricPersistence, t test.Tester)) func(t test.Tester) {
	return func(t test.Tester) {
		temporaryDirectory, err := ioutil.TempDir("", fmt.Sprintf("test_leveldb_%s", name))

		if err != nil {
			t.Errorf("Could not create test directory: %q\n", err)
			return
		}

		defer func() {
			err := os.RemoveAll(temporaryDirectory)
			if err != nil {
				t.Errorf("Could not remove temporary directory: %q\n", err)
			}
		}()

		p, err := NewLevelDBMetricPersistence(temporaryDirectory)
		if err != nil {
			t.Errorf("Could not create LevelDB Metric Persistence: %q\n", err)
		}

		defer func() {
			err := p.Close()
			if err != nil {
				t.Errorf("Anomaly while closing database: %q\n", err)
			}
		}()

		f(p, t)
	}
}
