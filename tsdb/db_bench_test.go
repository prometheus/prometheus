// Copyright 2023 The Prometheus Authors
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

package tsdb

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func BenchmarkQuerier_LabelValues(b *testing.B) {
	db := openTestDB(b, nil, []int64{1000})
	b.Cleanup(func() {
		require.NoError(b, db.Close())
	})

	ctx := context.Background()

	app := db.head.Appender(ctx)
	addSeries := func(l labels.Labels) {
		app.Append(0, l, 0, 0)
	}

	for n := 0; n < 10; n++ {
		for i := 0; i < 100000; i++ {
			addSeries(labels.FromStrings("i", strconv.Itoa(i)+postingsBenchSuffix, "n", strconv.Itoa(n)+postingsBenchSuffix, "j", "foo", "i_times_n", strconv.Itoa(i*n)))
			// Have some series that won't be matched, to properly test inverted matchers.
			addSeries(labels.FromStrings("i", strconv.Itoa(i)+postingsBenchSuffix, "n", strconv.Itoa(n)+postingsBenchSuffix, "j", "bar"))
			addSeries(labels.FromStrings("i", strconv.Itoa(i)+postingsBenchSuffix, "n", "0_"+strconv.Itoa(n)+postingsBenchSuffix, "j", "bar"))
			addSeries(labels.FromStrings("i", strconv.Itoa(i)+postingsBenchSuffix, "n", "1_"+strconv.Itoa(n)+postingsBenchSuffix, "j", "bar"))
			addSeries(labels.FromStrings("i", strconv.Itoa(i)+postingsBenchSuffix, "n", "2_"+strconv.Itoa(n)+postingsBenchSuffix, "j", "foo"))
		}
	}
	require.NoError(b, app.Commit())
	require.NoError(b, db.reloadBlocks())

	querier, err := db.Querier(0, 1)
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, querier.Close()) })

	i1 := labels.MustNewMatcher(labels.MatchEqual, "i", "1")
	iStar := labels.MustNewMatcher(labels.MatchRegexp, "i", "^.*$")
	jNotFoo := labels.MustNewMatcher(labels.MatchNotEqual, "j", "foo")
	jXXXYYY := labels.MustNewMatcher(labels.MatchRegexp, "j", "XXX|YYY")
	jXplus := labels.MustNewMatcher(labels.MatchRegexp, "j", "X.+")
	n1 := labels.MustNewMatcher(labels.MatchEqual, "n", "1"+postingsBenchSuffix)
	nX := labels.MustNewMatcher(labels.MatchNotEqual, "n", "X"+postingsBenchSuffix)
	nPlus := labels.MustNewMatcher(labels.MatchRegexp, "i", "^.+$")
	primesTimes := labels.MustNewMatcher(labels.MatchEqual, "i_times_n", "533701") // = 76243*7, ie. multiplication of primes. It will match a single i*n combination.
	nonPrimesTimes := labels.MustNewMatcher(labels.MatchEqual, "i_times_n", "20")  // 1*20, 2*10, 4*5, 5*4
	times12 := labels.MustNewMatcher(labels.MatchRegexp, "i_times_n", "12.*")

	cases := []struct {
		name      string
		labelName string
		matchers  []*labels.Matcher
	}{
		{`i with i="1"`, "i", []*labels.Matcher{i1}},
		// i has 100k values.
		{`i with n="1"`, "i", []*labels.Matcher{n1}},
		{`i with n="^.+$"`, "i", []*labels.Matcher{nPlus}},
		{`i with n="1",j!="foo"`, "i", []*labels.Matcher{n1, jNotFoo}},
		{`i with n="1",j=~"X.+"`, "i", []*labels.Matcher{n1, jXplus}},
		{`i with n="1",j=~"XXX|YYY"`, "i", []*labels.Matcher{n1, jXXXYYY}},
		{`i with n="X",j!="foo"`, "i", []*labels.Matcher{nX, jNotFoo}},
		{`i with n="1",i=~"^.*$",j!="foo"`, "i", []*labels.Matcher{n1, iStar, jNotFoo}},
		{`i with i_times_n=533701`, "i", []*labels.Matcher{primesTimes}},
		{`i with i_times_n=20`, "i", []*labels.Matcher{nonPrimesTimes}},
		{`i with i_times_n=~"12.*""`, "i", []*labels.Matcher{times12}},
		// n has 10 values.
		{`n with j!="foo"`, "n", []*labels.Matcher{jNotFoo}},
		{`n with i="1"`, "n", []*labels.Matcher{i1}},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, warnings, err := querier.LabelValues(ctx, c.labelName, c.matchers...)
				require.NoError(b, err)
				require.Empty(b, warnings)
			}
		})
	}
}
