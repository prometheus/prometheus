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

// clean removed fields not used in metricGroupChange for result
// transformations, so they are not updated.
func clean(m metricGroupChange) metricGroupChange {
	m.ValuePromQL = ""
	return m
}

func TestEngine_FindVariants(t *testing.T) {
	for _, tcase := range []struct {
		schemaURL string
		matchers  []*labels.Matcher

		expectedVariants []*variant
		expectedErr      error
	}{
		// Only original.
		{
			schemaURL: "./testdata/1.0.0", matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "./testdata/1.0.0"),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_some_elements"),
				labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
			},
			expectedVariants: []*variant{
				{
					// Original.
					matchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_some_elements"),
						labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
					},
				},
			},
		},
		// Asking for my_app_latency_seconds.2 should give us original and one backward variant.
		/*
			{
				schemaURL: "./testdata/1.1.0", matchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "./testdata/1.1.0"),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds"),
					labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
				},
				expectedVariants: []*variant{
					{
						// Original.
						matchers: []*labels.Matcher{
							labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds"),
							labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
						},
					},
					{
						// Backward.
						matchers: []*labels.Matcher{
							labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds"),
							labels.MustNewMatcher(labels.MatchEqual, "__unit__", "milliseconds"),
							labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
						},
						result: resultTransform{
							to:   clean(testdataLatencyChanges[0].Forward),
							from: clean(testdataLatencyChanges[0].Backward),
							vt:   mustNewValueTransformerFromPromQL("value{} / 1000"),
						},
					},
				},
			},
			// Asking for my_app_latency_seconds should give us original and one forward variant.
			{
				schemaURL: "./testdata/1.0.0", matchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "./testdata/1.0.0"),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds"),
					labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
				},
				expectedVariants: []*variant{
					{
						// Original.
						matchers: []*labels.Matcher{
							labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds"),
							labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
						},
					},
					{
						// Forward.
						matchers: []*labels.Matcher{
							labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds"),
							labels.MustNewMatcher(labels.MatchEqual, "__unit__", "seconds"),
							labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
						},
						result: resultTransform{
							to:   clean(testdataLatencyChanges[0].Backward),
							from: clean(testdataLatencyChanges[0].Forward),
							vt:   mustNewValueTransformerFromPromQL("value{} * 1000"),
						},
					},
				},
			},
		*/
		// TODO(bwplotka): Test ambiguous matcher errors etc.
		// Asking for my_app_custom_elements.2 should give us the original and one backward and one forward variant.
		{
			schemaURL: "./testdata/1.1.0", matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "./testdata/1.1.0"),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_elements_changed_total"),
				labels.MustNewMatcher(labels.MatchNotEqual, "number", "2"),
				labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
				labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
			},
			expectedVariants: []*variant{
				{
					// Original.
					matchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_elements_changed_total"),
						labels.MustNewMatcher(labels.MatchNotEqual, "number", "2"),
						labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
						labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
					},
				},
				{
					// Backward.
					matchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_elements_total"),
						labels.MustNewMatcher(labels.MatchNotEqual, "integer", "2"),
						labels.MustNewMatcher(labels.MatchRegexp, "category", "first|other"),
						labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
					},
					result: resultTransform{
						to:   testdataElementsChanges[1].Forward,
						from: testdataElementsChanges[1].Backward,
					},
				},
				{
					// Forward.
					matchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_elements_changed_total"),
						labels.MustNewMatcher(labels.MatchNotEqual, "my_number", "2"),
						labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
						labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
					},
					result: resultTransform{
						to:   testdataElementsChanges[0].Backward,
						from: testdataElementsChanges[0].Forward,
					},
				},
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			e := newSchemaEngine()
			got, err := e.FindVariants(tcase.schemaURL, tcase.matchers)
			if tcase.expectedErr != nil {
				require.ErrorContains(t, err, tcase.expectedErr.Error())
				return
			}
			require.NoError(t, err)
			testutil.RequireEqualWithOptions(t, tcase.expectedVariants, got, []cmp.Option{
				cmp.Comparer(func(a, b *labels.Matcher) bool {
					return a.Name == b.Name && a.Type == b.Type && a.Value == b.Value
				}),
				cmp.AllowUnexported(variant{}, resultTransform{}, valueTransformer{}),
			})
		})
	}
}
