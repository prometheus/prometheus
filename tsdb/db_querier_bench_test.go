// Copyright 2018 The Prometheus Authors
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
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func BenchmarkDBQuerier(b *testing.B) {
	dir := b.TempDir()
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 255
	opts.OutOfOrderTimeWindow = 100000000
	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(b, err)
	b.Cleanup(func() {
		require.NoError(b, db.Close())
	})
	h := db.Head()

	app := h.Appender(context.Background())
	addSeries := func(l labels.Labels) {
		app.Append(0, l, 10, 4)
		app.Append(0, l, 12, 3)
		app.Append(0, l, 14, 3)
		app.Append(0, l, 16, 3)
		app.Append(0, l, 11, 1) // Out of order sample
		app.Append(0, l, 15, 2) // Out of order sample
		app.Append(0, l, 9, 2)  // Out of order sample
		app.Append(0, l, 13, 2) // Out of order sample
	}

	for n := 0; n < 10; n++ {
		for i := 0; i < 100000; i++ {
			addSeries(labels.FromStrings("i", strconv.Itoa(i)+postingsBenchSuffix, "n", strconv.Itoa(n)+postingsBenchSuffix, "j", "foo"))
			// Have some series that won't be matched, to properly test inverted matches.
			addSeries(labels.FromStrings("i", strconv.Itoa(i)+postingsBenchSuffix, "n", strconv.Itoa(n)+postingsBenchSuffix, "j", "bar"))
			addSeries(labels.FromStrings("i", strconv.Itoa(i)+postingsBenchSuffix, "n", "0_"+strconv.Itoa(n)+postingsBenchSuffix, "j", "bar"))
			addSeries(labels.FromStrings("i", strconv.Itoa(i)+postingsBenchSuffix, "n", "1_"+strconv.Itoa(n)+postingsBenchSuffix, "j", "bar"))
			addSeries(labels.FromStrings("i", strconv.Itoa(i)+postingsBenchSuffix, "n", "2_"+strconv.Itoa(n)+postingsBenchSuffix, "j", "foo"))
		}
	}
	require.NoError(b, app.Commit())

	n1 := labels.MustNewMatcher(labels.MatchEqual, "n", "1"+postingsBenchSuffix)
	jFoo := labels.MustNewMatcher(labels.MatchEqual, "j", "foo")
	jNotFoo := labels.MustNewMatcher(labels.MatchNotEqual, "j", "foo")
	iStar := labels.MustNewMatcher(labels.MatchRegexp, "i", "^.*$")
	i1Star := labels.MustNewMatcher(labels.MatchRegexp, "i", "^1.*$")
	iStar1 := labels.MustNewMatcher(labels.MatchRegexp, "i", "^.*1$")
	iStar1Star := labels.MustNewMatcher(labels.MatchRegexp, "i", "^.*1.*$")
	iPlus := labels.MustNewMatcher(labels.MatchRegexp, "i", "^.+$")
	i1Plus := labels.MustNewMatcher(labels.MatchRegexp, "i", "^1.+$")
	iEmptyRe := labels.MustNewMatcher(labels.MatchRegexp, "i", "^$")
	iNotEmpty := labels.MustNewMatcher(labels.MatchNotEqual, "i", "")
	iNot2 := labels.MustNewMatcher(labels.MatchNotEqual, "i", "2"+postingsBenchSuffix)
	iNot2Star := labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^2.*$")
	iNotStar2Star := labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^.*2.*$")
	values := make([]string, 0, 25000)
	for i := 0; i < 25000; i++ {
		values = append(values, strconv.Itoa(i*4)+postingsBenchSuffix)
	}
	iBigValueSet := labels.MustNewMatcher(labels.MatchRegexp, "i", strings.Join(values, "|"))

	b.ResetTimer()
	cases := []struct {
		name     string
		matchers []*labels.Matcher
	}{
		{`n="1"`, []*labels.Matcher{n1}},
		{`n="1",j="foo"`, []*labels.Matcher{n1, jFoo}},
		{`j="foo",n="1"`, []*labels.Matcher{jFoo, n1}},
		{`n="1",j!="foo"`, []*labels.Matcher{n1, jNotFoo}},
		{`i=~".*"`, []*labels.Matcher{iStar}},
		{`i=~"1.*"`, []*labels.Matcher{i1Star}},
		{`i=~".*1"`, []*labels.Matcher{iStar1}},
		{`i=~".+"`, []*labels.Matcher{iPlus}},
		{`i=~""`, []*labels.Matcher{iEmptyRe}},
		{`i!=""`, []*labels.Matcher{iNotEmpty}},
		{`n="1",i=~".*",j="foo"`, []*labels.Matcher{n1, iStar, jFoo}},
		{`n="1",i=~".*",i!="2",j="foo"`, []*labels.Matcher{n1, iStar, iNot2, jFoo}},
		{`n="1",i!=""`, []*labels.Matcher{n1, iNotEmpty}},
		{`n="1",i!="",j="foo"`, []*labels.Matcher{n1, iNotEmpty, jFoo}},
		{`n="1",i=~".+",j="foo"`, []*labels.Matcher{n1, iPlus, jFoo}},
		{`n="1",i=~"1.+",j="foo"`, []*labels.Matcher{n1, i1Plus, jFoo}},
		{`n="1",i=~".*1.*",j="foo"`, []*labels.Matcher{n1, iStar1Star, jFoo}},
		{`n="1",i=~".+",i!="2",j="foo"`, []*labels.Matcher{n1, iPlus, iNot2, jFoo}},
		{`n="1",i=~".+",i!~"2.*",j="foo"`, []*labels.Matcher{n1, iPlus, iNot2Star, jFoo}},
		{`n="1",i=~".+",i!~".*2.*",j="foo"`, []*labels.Matcher{n1, iPlus, iNotStar2Star, jFoo}},
		{`n="1",i=~"big-value-set",j="foo"`, []*labels.Matcher{n1, iPlus, iBigValueSet, jFoo}},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					q, err := db.Querier(context.TODO(), math.MinInt64, math.MaxInt64)
					require.NoError(b, err)
					seriesSet := q.Select(false, nil, c.matchers...)
					for seriesSet.Next() {
					}
					require.NoError(b, seriesSet.Err())
					require.NoError(b, q.Close())
				}
			})
		})
	}
}
