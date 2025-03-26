package semconv

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/testutil"
)

func mustParsePromQL(p string) parser.Expr {
	e := parser.NewParser(p)
	expr, err := e.ParseExpr()
	if err != nil {
		panic(err)
	}
	return expr
}

func TestEngine_FindVariants(t *testing.T) {
	for _, tcase := range []struct {
		schemaURL string
		matchers  []*labels.Matcher

		expectedVariants []*variant
		expectedErr      error
	}{
		// TODO(bwplotka): Add only original variant case.
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
					matchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds"),
						labels.MustNewMatcher(labels.MatchEqual, "__unit__", "milliseconds"),
						labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
					},
					result: resultTransform{
						to:   testdataLatencyChanges[0].Forward,
						from: testdataLatencyChanges[0].Backward,
						vt:   &valueTransformer{expr: mustParsePromQL("value{} / 1000")},
					},
				},
			},
		},
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
					matchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds"),
						labels.MustNewMatcher(labels.MatchEqual, "__unit__", "seconds"),
						labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
					},
					result: resultTransform{
						to:   testdataLatencyChanges[0].Backward,
						from: testdataLatencyChanges[0].Forward,
						vt:   &valueTransformer{expr: mustParsePromQL("value{} * 1000")},
					},
				},
			},
		},
		// TODO(bwplotka): Test ambiguous matcher errors etc.
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
					matchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_custom_elements_total"),
						labels.MustNewMatcher(labels.MatchNotEqual, "integer", "2"),
						labels.MustNewMatcher(labels.MatchRegexp, "category", "first|other"),
						labels.MustNewMatcher(labels.MatchEqual, "fraction", "1.2"),
					},
					result: resultTransform{
						to:   testdataElementsChanges[0].Forward,
						from: testdataElementsChanges[0].Backward,
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
