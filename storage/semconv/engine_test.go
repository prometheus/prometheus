// Copyright 2025 The Prometheus Authors
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

package semconv

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func mustNewValueTransformerFromPromQL(p string) valueTransformer {
	ret, err := valueTransformer{}.AddPromQL(p)
	if err != nil {
		panic(err)
	}
	return ret
}

// Change logs in order old to new.
// On disk change logs are in order new to old.
var (
	testdataElementsChangesOldToNew = []change{
		{
			Forward: metricGroupChange{
				MetricName:  "my_app_custom_changed_elements_total",
				Unit:        "",
				ValuePromQL: "",
				Attributes:  []attribute{{Tag: "number"}, {Tag: "class", Members: []attributeMember{{Value: "FIRST"}, {Value: "SECOND"}, {Value: "OTHER"}}}},
			},
			Backward: metricGroupChange{
				MetricName:  "my_app_custom_elements_total",
				Unit:        "",
				ValuePromQL: "",
				Attributes:  []attribute{{Tag: "integer"}, {Tag: "category", Members: []attributeMember{{Value: "first"}, {Value: "second"}, {Value: "other"}}}},
			},
		},
		{
			Forward: metricGroupChange{
				MetricName:  "",
				Unit:        "",
				ValuePromQL: "",
				Attributes:  []attribute{{Tag: "my_number"}},
			},
			Backward: metricGroupChange{
				MetricName:  "",
				Unit:        "",
				ValuePromQL: "",
				Attributes:  []attribute{{Tag: "number"}},
			},
		},
	}
	testdataLatencyChangesOldToNew = []change{
		{
			Forward:  metricGroupChange{MetricName: "my_app_latency_seconds", Unit: "{second}", ValuePromQL: "value{} / 1000"},
			Backward: metricGroupChange{MetricName: "my_app_latency_milliseconds", Unit: "{millisecond}", ValuePromQL: "value{} * 1000"},
		},
	}
)

// TODO(bwplotka): Test ambiguous matcher errors etc.
func TestEngine_FindMatcherVariants(t *testing.T) {
	for _, tcase := range []struct {
		schemaURL string
		matchers  []*labels.Matcher

		expectedVariants     [][]*labels.Matcher
		expectedQueryContext queryContext
		expectedErr          error
	}{
		// Only original.
		{
			schemaURL: "./testdata/1.0.0", matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "./testdata/1.0.0"),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_some_elements"),
				labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
			},
			expectedVariants: [][]*labels.Matcher{
				{
					// Original matchers.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_some_elements"),
					labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
				},
			},
			expectedQueryContext: queryContext{
				mID: "my_app_some_elements",
			},
		},
		// Asking for my_app_latency_seconds.2 should give us original and one backward variant.
		{
			schemaURL: "./testdata/1.1.0", matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "./testdata/1.1.0"),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds"),
				labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
			},
			expectedVariants: [][]*labels.Matcher{
				{
					// Original.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds"),
					labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
				},
				{
					// Backward.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds"),
					labels.MustNewMatcher(labels.MatchEqual, "__unit__", "milliseconds"),
					labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
				},
			},
			expectedQueryContext: queryContext{
				mID:     "my_app_latency.2",
				changes: testdataLatencyChangesOldToNew,
			},
		},
		// Asking for my_app_latency_seconds should give us original and one forward variant.
		{
			schemaURL: "./testdata/1.0.0", matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "./testdata/1.0.0"),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds"),
				labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
			},
			expectedVariants: [][]*labels.Matcher{
				{
					// Original.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds"),
					labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
				},
				{
					// Forward.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds"),
					labels.MustNewMatcher(labels.MatchEqual, "__unit__", "seconds"),
					labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
				},
			},
			expectedQueryContext: queryContext{
				mID:     "my_app_latency",
				changes: testdataLatencyChangesOldToNew,
			},
		},
		// Asking for my_app_custom_elements.2 should give us the original and one backward and one forward variant.
		{
			schemaURL: "./testdata/1.1.0", matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "./testdata/1.1.0"),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_changed_elements_total"),
				labels.MustNewMatcher(labels.MatchNotEqual, "number", "2"),
				labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
				labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
			},
			expectedVariants: [][]*labels.Matcher{
				{
					// Original matchers.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_changed_elements_total"),
					labels.MustNewMatcher(labels.MatchNotEqual, "number", "2"),
					labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
					labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
				},
				{
					// Backward.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_elements_total"),
					labels.MustNewMatcher(labels.MatchNotEqual, "integer", "2"),
					labels.MustNewMatcher(labels.MatchRegexp, "category", "first|other"),
					labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
				},
				{
					// Forward.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_changed_elements_total"),
					labels.MustNewMatcher(labels.MatchNotEqual, "my_number", "2"),
					labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
					labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
				},
			},
			expectedQueryContext: queryContext{
				mID:     "my_app_custom_elements.2",
				changes: testdataElementsChangesOldToNew,
			},
		},
		// Asking for my_app_custom_elements.3 should give us the original and 2 backward variants.
		{
			schemaURL: "./testdata/1.2.0", matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "./testdata/1.2.0"),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_changed_elements_total"),
				labels.MustNewMatcher(labels.MatchNotEqual, "my_number", "2"),
				labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
				labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
			},
			expectedVariants: [][]*labels.Matcher{
				{
					// Original matchers.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_changed_elements_total"),
					labels.MustNewMatcher(labels.MatchNotEqual, "my_number", "2"),
					labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
					labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
				},
				{
					// Backward 1.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_changed_elements_total"),
					labels.MustNewMatcher(labels.MatchNotEqual, "number", "2"),
					labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
					labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
				},
				{
					// Backward 2.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_elements_total"),
					labels.MustNewMatcher(labels.MatchNotEqual, "integer", "2"),
					labels.MustNewMatcher(labels.MatchRegexp, "category", "first|other"),
					labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
				},
			},
			expectedQueryContext: queryContext{
				mID:     "my_app_custom_elements.3",
				changes: testdataElementsChangesOldToNew,
			},
		},
		// Asking for my_app_custom_elements should give us the original and 2 forward variants.
		{
			schemaURL: "./testdata/1.0.0", matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "./testdata/1.0.0"),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_elements_total"),
				labels.MustNewMatcher(labels.MatchNotEqual, "integer", "2"),
				labels.MustNewMatcher(labels.MatchRegexp, "category", "first|other"),
				labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
			},
			expectedVariants: [][]*labels.Matcher{
				{
					// Original matchers.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_elements_total"),
					labels.MustNewMatcher(labels.MatchNotEqual, "integer", "2"),
					labels.MustNewMatcher(labels.MatchRegexp, "category", "first|other"),
					labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
				},
				{
					// Forward 1.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_changed_elements_total"),
					labels.MustNewMatcher(labels.MatchNotEqual, "number", "2"),
					labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
					labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
				},
				{
					// Forward 2.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_changed_elements_total"),
					labels.MustNewMatcher(labels.MatchNotEqual, "my_number", "2"),
					labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
					labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
				},
			},
			expectedQueryContext: queryContext{
				mID:     "my_app_custom_elements",
				changes: testdataElementsChangesOldToNew,
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			e := newSchemaEngine()
			got, gotQ, err := e.FindMatcherVariants(tcase.schemaURL, tcase.matchers)
			if tcase.expectedErr != nil {
				require.ErrorContains(t, err, tcase.expectedErr.Error())
				return
			}
			require.NoError(t, err)
			testutil.RequireEqualWithOptions(t, tcase.expectedVariants, got, []cmp.Option{
				cmp.Comparer(func(a, b *labels.Matcher) bool {
					return a.Name == b.Name && a.Type == b.Type && a.Value == b.Value
				}),
			})
			testutil.RequireEqualWithOptions(t, tcase.expectedQueryContext, gotQ, []cmp.Option{
				cmp.AllowUnexported(queryContext{}),
			})
		})
	}
}

func TestEngine_TransformSeries(t *testing.T) {
	for _, tcase := range []struct {
		q    queryContext
		lbls labels.Labels

		expectedLabels labels.Labels
		expectedVT     valueTransformer
		expectedErr    error
	}{
		{
			q: queryContext{
				mID:     "my_app_latency.2",
				changes: testdataLatencyChangesOldToNew,
			},
			lbls: labels.FromStrings(
				schemaURLLabel, testSchemaURL("1.1.0"),
				labels.MetricName, "my_app_latency_seconds",
				"foo", "bar",
			),
			expectedLabels: labels.FromStrings(
				labels.MetricName, "my_app_latency_seconds",
				"foo", "bar",
			),
		},
		{
			// Same but with unit label.
			q: queryContext{
				mID:     "my_app_latency.2",
				changes: testdataLatencyChangesOldToNew,
			},
			lbls: testdataLatencySeriesNew,
			expectedLabels: labels.FromStrings(
				"__name__", "my_app_latency_seconds",
				"__type__", "histogram",
				"__unit__", "seconds",
				"code", "200",
				"test", "new",
			),
		},
		{
			q: queryContext{
				mID:     "my_app_latency.2",
				changes: testdataLatencyChangesOldToNew,
			},
			lbls: testdataLatencySeriesOld,
			expectedLabels: labels.FromStrings(
				"__name__", "my_app_latency_seconds",
				"__type__", "histogram",
				"__unit__", "seconds",
				"code", "200",
				"test", "old",
			),
			expectedVT: mustNewValueTransformerFromPromQL(testdataLatencyChangesOldToNew[0].Forward.ValuePromQL),
		},
		{
			q: queryContext{
				mID:     "my_app_latency",
				changes: testdataLatencyChangesOldToNew,
			},
			lbls: testdataLatencySeriesOld,
			expectedLabels: labels.FromStrings(
				"__name__", "my_app_latency_milliseconds",
				"__type__", "histogram",
				"__unit__", "milliseconds",
				"code", "200",
				"test", "old",
			),
		},
		{
			q: queryContext{
				mID:     "my_app_latency",
				changes: testdataLatencyChangesOldToNew,
			},
			lbls: testdataLatencySeriesNew,
			expectedLabels: labels.FromStrings(
				"__name__", "my_app_latency_milliseconds",
				"__type__", "histogram",
				"__unit__", "milliseconds",
				"code", "200",
				"test", "new",
			),
			expectedVT: mustNewValueTransformerFromPromQL(testdataLatencyChangesOldToNew[0].Backward.ValuePromQL),
		},
		{
			q: queryContext{
				mID:     "my_app_custom_elements",
				changes: testdataElementsChangesOldToNew,
			},
			lbls: testdataElementsSeriesOld,
			expectedLabels: labels.FromStrings(
				"__name__", "my_app_custom_elements_total",
				"__type__", "counter",
				"integer", "1",
				"category", "first",
				"fraction", "1.243",
				"test", "old",
			),
		},
		{
			q: queryContext{
				mID:     "my_app_custom_elements",
				changes: testdataElementsChangesOldToNew,
			},
			lbls: testdataElementsSeriesNew,
			expectedLabels: labels.FromStrings(
				"__name__", "my_app_custom_elements_total",
				"__type__", "counter",
				"integer", "1",
				"category", "first",
				"fraction", "1.243",
				"test", "new",
			),
		},
		{
			q: queryContext{
				mID:     "my_app_custom_elements.2",
				changes: testdataElementsChangesOldToNew,
			},
			lbls: testdataElementsSeriesNew,
			expectedLabels: labels.FromStrings(
				"__name__", "my_app_custom_changed_elements_total",
				"__type__", "counter",
				"number", "1",
				"class", "FIRST",
				"fraction", "1.243",
				"test", "new",
			),
		},
		{
			q: queryContext{
				mID:     "my_app_custom_elements.2",
				changes: testdataElementsChangesOldToNew,
			},
			lbls: testdataElementsSeriesOld,
			expectedLabels: labels.FromStrings(
				"__name__", "my_app_custom_changed_elements_total",
				"__type__", "counter",
				"number", "1",
				"class", "FIRST",
				"fraction", "1.243",
				"test", "old",
			),
		},
		{
			q: queryContext{
				mID:     "my_app_custom_elements.3",
				changes: testdataElementsChangesOldToNew,
			},
			lbls: testdataElementsSeriesOld,
			expectedLabels: labels.FromStrings(
				"__name__", "my_app_custom_changed_elements_total",
				"__type__", "counter",
				"my_number", "1",
				"class", "FIRST",
				"fraction", "1.243",
				"test", "old",
			),
		},
		{
			q: queryContext{
				mID:     "my_app_custom_elements.3",
				changes: testdataElementsChangesOldToNew,
			},
			lbls: testdataElementsSeriesNew,
			expectedLabels: labels.FromStrings(
				"__name__", "my_app_custom_changed_elements_total",
				"__type__", "counter",
				"my_number", "1",
				"class", "FIRST",
				"fraction", "1.243",
				"test", "new",
			),
		},
	} {
		t.Run("", func(t *testing.T) {
			e := newSchemaEngine()
			got, gotVT, err := e.TransformSeries(tcase.q, tcase.lbls)
			if tcase.expectedErr != nil {
				require.ErrorContains(t, err, tcase.expectedErr.Error())
				return
			}
			require.NoError(t, err)
			testutil.RequireEqual(t, tcase.expectedLabels, got)
			testutil.RequireEqualWithOptions(t, tcase.expectedVT, gotVT, []cmp.Option{
				cmp.Comparer(func(a, b *labels.Matcher) bool {
					return a.Name == b.Name && a.Type == b.Type && a.Value == b.Value
				}),
				cmp.AllowUnexported(valueTransformer{}),
			})
		})
	}
}
