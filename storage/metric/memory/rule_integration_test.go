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
	"io"
	"io/ioutil"
	"testing"
)

func testGetValueAtTime(t test.Tester) {
	persistenceMaker := func() (metric.MetricPersistence, io.Closer) {
		return NewMemorySeriesStorage(), ioutil.NopCloser(nil)
	}

	metric.GetValueAtTimeTests(persistenceMaker, t)
}

func TestGetValueAtTime(t *testing.T) {
	testGetValueAtTime(t)
}

func BenchmarkGetValueAtTime(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testGetValueAtTime(b)
	}
}

func testGetBoundaryValues(t test.Tester) {
	persistenceMaker := func() (metric.MetricPersistence, io.Closer) {
		return NewMemorySeriesStorage(), ioutil.NopCloser(nil)
	}

	metric.GetBoundaryValuesTests(persistenceMaker, t)
}

func TestGetBoundaryValues(t *testing.T) {
	testGetBoundaryValues(t)
}

func BenchmarkGetBoundaryValues(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testGetBoundaryValues(b)
	}
}

func testGetRangeValues(t test.Tester) {
	persistenceMaker := func() (metric.MetricPersistence, io.Closer) {
		return NewMemorySeriesStorage(), ioutil.NopCloser(nil)
	}

	metric.GetRangeValuesTests(persistenceMaker, t)
}

func TestGetRangeValues(t *testing.T) {
	testGetRangeValues(t)
}

func BenchmarkGetRangeValues(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testGetRangeValues(b)
	}
}
