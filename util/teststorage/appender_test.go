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
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/testutil"
)

func testAppendableV1(t *testing.T, appTest *Appendable, a storage.Appendable) {
	for _, commit := range []bool{true, false} {
		appTest.ResultReset()

		app := a.Appender(t.Context())

		ref1, err := app.Append(0, labels.FromStrings(model.MetricNameLabel, "test_metric1", "app", "v1"), 1, 2)
		require.NoError(t, err)

		h := tsdbutil.GenerateTestHistogram(0)
		_, err = app.AppendHistogram(0, labels.FromStrings(model.MetricNameLabel, "test_metric2", "app", "v1"), 2, h, nil)
		require.NoError(t, err)

		fh := tsdbutil.GenerateTestFloatHistogram(0)
		_, err = app.AppendHistogram(0, labels.FromStrings(model.MetricNameLabel, "test_metric3", "app", "v1"), 3, nil, fh)
		require.NoError(t, err)

		// Update meta of first series.
		m1 := metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}
		_, err = app.UpdateMetadata(ref1, labels.FromStrings(model.MetricNameLabel, "test_metric1", "app", "v1"), m1)
		require.NoError(t, err)

		// Add exemplars to the first series.
		e1 := exemplar.Exemplar{Labels: labels.FromStrings(model.MetricNameLabel, "yolo"), HasTs: true, Ts: 1}
		_, err = app.AppendExemplar(ref1, labels.FromStrings(model.MetricNameLabel, "test_metric1", "app", "v1"), e1)
		require.NoError(t, err)

		exp := []Sample{
			{L: labels.FromStrings(model.MetricNameLabel, "test_metric1", "app", "v1"), M: m1, T: 1, V: 2, ES: []exemplar.Exemplar{e1}},
			{L: labels.FromStrings(model.MetricNameLabel, "test_metric2", "app", "v1"), T: 2, H: h},
			{L: labels.FromStrings(model.MetricNameLabel, "test_metric3", "app", "v1"), T: 3, FH: fh},
		}
		testutil.RequireEqual(t, exp, appTest.PendingSamples())
		require.Nil(t, appTest.ResultSamples())
		require.Nil(t, appTest.RolledbackSamples())

		if commit {
			require.NoError(t, app.Commit())
			require.Nil(t, appTest.PendingSamples())
			testutil.RequireEqual(t, exp, appTest.ResultSamples())
			require.Nil(t, appTest.RolledbackSamples())
			break
		}

		require.NoError(t, app.Rollback())
		require.Nil(t, appTest.PendingSamples())
		require.Nil(t, appTest.ResultSamples())
		testutil.RequireEqual(t, exp, appTest.RolledbackSamples())
	}
}

func testAppendableV2(t *testing.T, appTest *Appendable, a storage.AppendableV2) {
	for _, commit := range []bool{true, false} {
		appTest.ResultReset()

		app := a.AppenderV2(t.Context())

		m1 := metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}
		e1 := exemplar.Exemplar{Labels: labels.FromStrings(model.MetricNameLabel, "yolo"), HasTs: true, Ts: 1}
		_, err := app.Append(0, labels.FromStrings(model.MetricNameLabel, "test_metric1", "app", "v2"), -1, 1, 2, nil, nil, storage.AOptions{
			MetricFamilyName: "test_metric1",
			Metadata:         m1,
			Exemplars:        []exemplar.Exemplar{e1},
		})
		require.NoError(t, err)

		h := tsdbutil.GenerateTestHistogram(0)
		_, err = app.Append(0, labels.FromStrings(model.MetricNameLabel, "test_metric2", "app", "v2"), -2, 2, 0, h, nil, storage.AOptions{})
		require.NoError(t, err)

		fh := tsdbutil.GenerateTestFloatHistogram(0)
		_, err = app.Append(0, labels.FromStrings(model.MetricNameLabel, "test_metric3", "app", "v2"), -3, 3, 0, nil, fh, storage.AOptions{})
		require.NoError(t, err)

		exp := []Sample{
			{L: labels.FromStrings(model.MetricNameLabel, "test_metric1", "app", "v2"), MF: "test_metric1", M: m1, ST: -1, T: 1, V: 2, ES: []exemplar.Exemplar{e1}},
			{L: labels.FromStrings(model.MetricNameLabel, "test_metric2", "app", "v2"), ST: -2, T: 2, H: h},
			{L: labels.FromStrings(model.MetricNameLabel, "test_metric3", "app", "v2"), ST: -3, T: 3, FH: fh},
		}
		testutil.RequireEqual(t, exp, appTest.PendingSamples())
		require.Nil(t, appTest.ResultSamples())
		require.Nil(t, appTest.RolledbackSamples())

		if commit {
			require.NoError(t, app.Commit())
			require.Nil(t, appTest.PendingSamples())
			testutil.RequireEqual(t, exp, appTest.ResultSamples())
			require.Nil(t, appTest.RolledbackSamples())
			break
		}

		require.NoError(t, app.Rollback())
		require.Nil(t, appTest.PendingSamples())
		require.Nil(t, appTest.ResultSamples())
		testutil.RequireEqual(t, exp, appTest.RolledbackSamples())
	}
}

func TestAppendable(t *testing.T) {
	appTest := NewAppendable()
	testAppendableV1(t, appTest, appTest)
	testAppendableV2(t, appTest, appTest)
}

func TestAppendable_Then(t *testing.T) {
	nextAppTest := NewAppendable()
	app := NewAppendable().Then(nextAppTest)

	// Ensure next mock record all the appends when appending to app.
	testAppendableV1(t, nextAppTest, app)

	// V2 should fail as Then was supplied with Appendable V1.
	require.Error(t, app.AppenderV2(t.Context()).Commit())
}

func TestAppendable_ThenV2(t *testing.T) {
	nextAppTest := NewAppendable()
	app := NewAppendable().ThenV2(nextAppTest)

	// Ensure next mock record all the appends when appending to app.
	testAppendableV2(t, nextAppTest, app)

	// V1 should fail as ThenV2 was supplied with Appendable V2.
	require.Error(t, app.Appender(t.Context()).Commit())
}

// TestSample_RequireEqual ensures standard testutil.RequireEqual is enough for comparisons.
// This is thanks to the fact metadata has now Equals method.
func TestSample_RequireEqual(t *testing.T) {
	a := []Sample{
		{},
		{L: labels.FromStrings(model.MetricNameLabel, "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings(model.MetricNameLabel, "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 123.123},
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings(model.MetricNameLabel, "yolo")}}},
	}
	testutil.RequireEqual(t, a, a)

	b1 := []Sample{
		{},
		{L: labels.FromStrings(model.MetricNameLabel, "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings(model.MetricNameLabel, "test_metric2_diff", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 123.123}, // test_metric2_diff is different.
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings(model.MetricNameLabel, "yolo")}}},
	}
	requireNotEqual(t, a, b1)

	b2 := []Sample{
		{},
		{L: labels.FromStrings(model.MetricNameLabel, "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings(model.MetricNameLabel, "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 123.123},
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings(model.MetricNameLabel, "yolo2")}}}, // exemplar is different.
	}
	requireNotEqual(t, a, b2)

	b3 := []Sample{
		{},
		{L: labels.FromStrings(model.MetricNameLabel, "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings(model.MetricNameLabel, "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 123.123, T: 123}, // Timestamp is different.
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings(model.MetricNameLabel, "yolo")}}},
	}
	requireNotEqual(t, a, b3)

	b4 := []Sample{
		{},
		{L: labels.FromStrings(model.MetricNameLabel, "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings(model.MetricNameLabel, "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 456.456}, // Value is different.
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings(model.MetricNameLabel, "yolo")}}},
	}
	requireNotEqual(t, a, b4)

	b5 := []Sample{
		{},
		{L: labels.FromStrings(model.MetricNameLabel, "test_metric_total"), M: metadata.Metadata{Type: "counter2", Unit: "metric", Help: "some help text"}}, // Different type.
		{L: labels.FromStrings(model.MetricNameLabel, "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 123.123},
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings(model.MetricNameLabel, "yolo")}}},
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

func TestConcurrentAppenderV2_ReturnsErrAppender(t *testing.T) {
	a := NewAppendable()

	// Non-concurrent multiple use if fine.
	app := a.AppenderV2(t.Context())
	require.Equal(t, int32(1), a.openAppenders.Load())
	require.NoError(t, app.Commit())
	// Repeated commit fails.
	require.Error(t, app.Commit())

	app = a.AppenderV2(t.Context())
	require.NoError(t, app.Rollback())
	// Commit after rollback fails.
	require.Error(t, app.Commit())

	a.WithErrs(
		nil,
		nil,
		errors.New("commit err"),
	)
	app = a.AppenderV2(t.Context())
	require.Error(t, app.Commit())

	a.WithErrs(nil, nil, nil)
	app = a.AppenderV2(t.Context())
	require.NoError(t, app.Commit())
	require.Equal(t, int32(0), a.openAppenders.Load())

	// Concurrent use should return appender that errors.
	_ = a.AppenderV2(t.Context())
	app = a.AppenderV2(t.Context())
	_, err := app.Append(0, labels.EmptyLabels(), 0, 0, 0, nil, nil, storage.AOptions{})
	require.Error(t, err)
	require.Error(t, app.Commit())
	require.Error(t, app.Rollback())
}
