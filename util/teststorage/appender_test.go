// Copyright The Prometheus Authors
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

package teststorage

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/util/testutil"
)

// TestSample_RequireEqual ensures standard testutil.RequireEqual is enough for comparisons.
// This is thanks to the fact metadata has now Equals method.
func TestSample_RequireEqual(t *testing.T) {
	a := []Sample{
		{},
		{L: labels.FromStrings("__name__", "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings("__name__", "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 123.123},
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings("__name__", "yolo")}}},
	}
	testutil.RequireEqual(t, a, a)

	b1 := []Sample{
		{},
		{L: labels.FromStrings("__name__", "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings("__name__", "test_metric2_diff", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 123.123}, // test_metric2_diff is different.
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings("__name__", "yolo")}}},
	}
	requireNotEqual(t, a, b1)

	b2 := []Sample{
		{},
		{L: labels.FromStrings("__name__", "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings("__name__", "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 123.123},
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings("__name__", "yolo2")}}}, // exemplar is different.
	}
	requireNotEqual(t, a, b2)

	b3 := []Sample{
		{},
		{L: labels.FromStrings("__name__", "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings("__name__", "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 123.123, T: 123}, // Timestamp is different.
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings("__name__", "yolo")}}},
	}
	requireNotEqual(t, a, b3)

	b4 := []Sample{
		{},
		{L: labels.FromStrings("__name__", "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings("__name__", "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 456.456}, // Value is different.
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings("__name__", "yolo")}}},
	}
	requireNotEqual(t, a, b4)

	b5 := []Sample{
		{},
		{L: labels.FromStrings("__name__", "test_metric_total"), M: metadata.Metadata{Type: "counter2", Unit: "metric", Help: "some help text"}}, // Different type.
		{L: labels.FromStrings("__name__", "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 123.123},
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings("__name__", "yolo")}}},
	}
	requireNotEqual(t, a, b5)
}

// TODO(bwplotka): While this mimick testutil.RequireEqual just making it negative, this does not literally test
// testutil.RequireEqual. Either build test suita that mocks `testing.TB` or get rid of testutil.RequireEqual somehow.
func requireNotEqual(t testing.TB, a, b any) {
	t.Helper()
	if !cmp.Equal(a, b, cmp.Comparer(labels.Equal)) {
		return
	}
	require.Fail(t, fmt.Sprintf("Equal, but expected not: \n"+
		"a: %s\n"+
		"b: %s", a, b))
}

func TestConcurrentAppender_ReturnsErrAppender(t *testing.T) {
	a := NewAppendable()

	// Non-concurrent multiple use if fine.
	app := a.Appender(t.Context())
	require.Equal(t, int32(1), a.openAppenders.Load())
	require.NoError(t, app.Commit())
	// Repeated commit fails.
	require.Error(t, app.Commit())

	app = a.Appender(t.Context())
	require.NoError(t, app.Rollback())
	// Commit after rollback fails.
	require.Error(t, app.Commit())

	a.WithErrs(
		nil,
		nil,
		errors.New("commit err"),
	)
	app = a.Appender(t.Context())
	require.Error(t, app.Commit())

	a.WithErrs(nil, nil, nil)
	app = a.Appender(t.Context())
	require.NoError(t, app.Commit())
	require.Equal(t, int32(0), a.openAppenders.Load())

	// Concurrent use should return appender that errors.
	_ = a.Appender(t.Context())
	app = a.Appender(t.Context())
	_, err := app.Append(0, labels.EmptyLabels(), 0, 0)
	require.Error(t, err)
	_, err = app.AppendHistogram(0, labels.EmptyLabels(), 0, nil, nil)
	require.Error(t, err)
	require.Error(t, app.Commit())
	require.Error(t, app.Rollback())
}
