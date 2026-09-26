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

package histogramconv

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestNewSelector(t *testing.T) {
	var (
		all  = newRepresentations(Representations()...)
		name = func(t labels.MatchType, v string) *labels.Matcher {
			return labels.MustNewMatcher(t, model.MetricNameLabel, v)
		}
		le = func(t labels.MatchType, v string) *labels.Matcher {
			return labels.MustNewMatcher(t, labels.BucketLabel, v)
		}
		job = labels.MustNewMatcher(labels.MatchEqual, "job", "a")
	)
	for _, tc := range []struct {
		name        string
		matchers    []*labels.Matcher
		convertFrom representations

		// The expected selector, with matchers in their string format. The
		// stored series are always selected with the given matchers.
		from           representations
		sourceMatchers []string
		suffix         string
		leMatchers     []string
	}{
		{
			name:           "classic buckets",
			matchers:       []*labels.Matcher{name(labels.MatchEqual, "foo_bucket")},
			convertFrom:    all,
			from:           newRepresentations(NHCB, NHE),
			sourceMatchers: []string{`__name__="foo"`},
			suffix:         "_bucket",
		},
		{
			name:           "classic count with le and other matchers",
			matchers:       []*labels.Matcher{job, name(labels.MatchEqual, "foo_count"), le(labels.MatchEqual, "1.0")},
			convertFrom:    newRepresentations(NHCB),
			from:           newRepresentations(NHCB),
			sourceMatchers: []string{`job="a"`, `__name__="foo"`},
			suffix:         "_count",
			leMatchers:     []string{`le="1.0"`},
		},
		{
			name:        "classic sum without native histograms to convert from",
			matchers:    []*labels.Matcher{name(labels.MatchEqual, "foo_sum")},
			convertFrom: newRepresentations(Classic),
		},
		{
			name:           "native histogram",
			matchers:       []*labels.Matcher{name(labels.MatchEqual, "foo"), job},
			convertFrom:    all,
			from:           newRepresentations(Classic),
			sourceMatchers: []string{`job="a"`, `__name__=~"foo(_bucket|_count|_sum)"`},
		},
		{
			name:           "native histogram with regexp meta characters in the name",
			matchers:       []*labels.Matcher{name(labels.MatchEqual, "foo.bar")},
			convertFrom:    all,
			from:           newRepresentations(Classic),
			sourceMatchers: []string{`__name__=~"foo\\.bar(_bucket|_count|_sum)"`},
		},
		{
			name:           "native histogram with a le matcher matching the empty value",
			matchers:       []*labels.Matcher{name(labels.MatchEqual, "foo"), le(labels.MatchNotEqual, "1.0")},
			convertFrom:    all,
			from:           newRepresentations(Classic),
			sourceMatchers: []string{`__name__=~"foo(_bucket|_count|_sum)"`},
		},
		{
			name:        "native histogram with a le matcher not matching the empty value",
			matchers:    []*labels.Matcher{name(labels.MatchEqual, "foo"), le(labels.MatchEqual, "1.0")},
			convertFrom: all,
		},
		{
			name:        "native histogram without classic histograms to convert from",
			matchers:    []*labels.Matcher{name(labels.MatchEqual, "foo")},
			convertFrom: newRepresentations(NHCB, NHE),
		},
		{
			name:        "native histogram with invalid UTF-8 in the name",
			matchers:    []*labels.Matcher{name(labels.MatchEqual, "foo\xff")},
			convertFrom: all,
		},
		{
			name:           "suffix without a base name",
			matchers:       []*labels.Matcher{name(labels.MatchEqual, "_bucket")},
			convertFrom:    all,
			from:           newRepresentations(Classic),
			sourceMatchers: []string{`__name__=~"_bucket(_bucket|_count|_sum)"`},
		},
		{
			name:        "regexp name matcher",
			matchers:    []*labels.Matcher{name(labels.MatchRegexp, "foo_bucket")},
			convertFrom: all,
		},
		{
			name:        "negative name matcher",
			matchers:    []*labels.Matcher{name(labels.MatchNotEqual, "foo")},
			convertFrom: all,
		},
		{
			name:        "several name matchers",
			matchers:    []*labels.Matcher{name(labels.MatchEqual, "foo_bucket"), name(labels.MatchNotEqual, "bar")},
			convertFrom: all,
		},
		{
			name:        "no name matcher",
			matchers:    []*labels.Matcher{job},
			convertFrom: all,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sel := newSelector(tc.matchers, tc.convertFrom)
			require.Equal(t, tc.matchers, sel.matchers)
			require.Equal(t, tc.from, sel.from)
			require.Equal(t, tc.sourceMatchers, matcherStrings(sel.sourceMatchers))
			require.Equal(t, tc.suffix, sel.suffix)
			require.Equal(t, tc.leMatchers, matcherStrings(sel.leMatchers))
		})
	}
}

// matcherStrings returns the string format of the given matchers, nil if there
// are none.
func matcherStrings(ms []*labels.Matcher) []string {
	var s []string
	for _, m := range ms {
		s = append(s, m.String())
	}
	return s
}
