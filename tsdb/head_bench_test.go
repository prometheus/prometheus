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
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/prometheus/prometheus/tsdb/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func BenchmarkHeadStripeSeriesCreate(b *testing.B) {
	// Put a series, select it. GC it and then access it.
	h, err := NewHead(nil, nil, nil, 1000)
	testutil.Ok(b, err)
	defer h.Close()

	for i := 0; i < b.N; i++ {
		h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(i)))
	}
}

func BenchmarkHeadStripeSeriesCreateParallel(b *testing.B) {
	// Put a series, select it. GC it and then access it.
	h, err := NewHead(nil, nil, nil, 1000)
	testutil.Ok(b, err)
	defer h.Close()

	var count int64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&count, 1)
			h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(int(i))))
		}
	})
}

func BenchmarkHeadPostingForMatchers(b *testing.B) {
	h, err := NewHead(nil, nil, nil, 1000)
	testutil.Ok(b, err)
	defer func() {
		testutil.Ok(b, h.Close())
	}()

	var ref uint64

	addSeries := func(l labels.Labels) {
		ref++
		h.getOrCreateWithID(ref, l.Hash(), l)
	}

	for n := 0; n < 10; n++ {
		for i := 0; i < 100000; i++ {
			addSeries(labels.FromStrings("i", strconv.Itoa(i), "n", strconv.Itoa(n), "j", "foo"))
			// Have some series that won't be matched, to properly test inverted matches.
			addSeries(labels.FromStrings("i", strconv.Itoa(i), "n", strconv.Itoa(n), "j", "bar"))
			addSeries(labels.FromStrings("i", strconv.Itoa(i), "n", "0_"+strconv.Itoa(n), "j", "bar"))
			addSeries(labels.FromStrings("i", strconv.Itoa(i), "n", "1_"+strconv.Itoa(n), "j", "bar"))
			addSeries(labels.FromStrings("i", strconv.Itoa(i), "n", "2_"+strconv.Itoa(n), "j", "foo"))
		}
	}

	n1 := labels.NewEqualMatcher("n", "1")

	jFoo := labels.NewEqualMatcher("j", "foo")
	jNotFoo := labels.Not(jFoo)

	iStar := labels.NewMustRegexpMatcher("i", "^.*$")
	iPlus := labels.NewMustRegexpMatcher("i", "^.+$")
	i1Plus := labels.NewMustRegexpMatcher("i", "^1.+$")
	iEmptyRe := labels.NewMustRegexpMatcher("i", "^$")
	iNotEmpty := labels.Not(labels.NewEqualMatcher("i", ""))
	iNot2 := labels.Not(labels.NewEqualMatcher("n", "2"))
	iNot2Star := labels.Not(labels.NewMustRegexpMatcher("i", "^2.*$"))

	cases := []struct {
		name     string
		matchers []labels.Matcher
	}{
		{`n="1"`, []labels.Matcher{n1}},
		{`n="1",j="foo"`, []labels.Matcher{n1, jFoo}},
		{`j="foo",n="1"`, []labels.Matcher{jFoo, n1}},
		{`n="1",j!="foo"`, []labels.Matcher{n1, jNotFoo}},
		{`i=~".*"`, []labels.Matcher{iStar}},
		{`i=~".+"`, []labels.Matcher{iPlus}},
		{`i=~""`, []labels.Matcher{iEmptyRe}},
		{`i!=""`, []labels.Matcher{iNotEmpty}},
		{`n="1",i=~".*",j="foo"`, []labels.Matcher{n1, iStar, jFoo}},
		{`n="1",i=~".*",i!="2",j="foo"`, []labels.Matcher{n1, iStar, iNot2, jFoo}},
		{`n="1",i!=""`, []labels.Matcher{n1, iNotEmpty}},
		{`n="1",i!="",j="foo"`, []labels.Matcher{n1, iNotEmpty, jFoo}},
		{`n="1",i=~".+",j="foo"`, []labels.Matcher{n1, iPlus, jFoo}},
		{`n="1",i=~"1.+",j="foo"`, []labels.Matcher{n1, i1Plus, jFoo}},
		{`n="1",i=~".+",i!="2",j="foo"`, []labels.Matcher{n1, iPlus, iNot2, jFoo}},
		{`n="1",i=~".+",i!~"2.*",j="foo"`, []labels.Matcher{n1, iPlus, iNot2Star, jFoo}},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, err := PostingsForMatchers(h.indexRange(0, 1000), c.matchers...)
				testutil.Ok(b, err)
			}
		})
	}
}
