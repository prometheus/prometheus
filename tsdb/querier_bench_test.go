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
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

// Make entries ~50B in size, to emulate real-world high cardinality.
const (
	postingsBenchSuffix = "aaaaaaaaaabbbbbbbbbbccccccccccdddddddddd"
)

func BenchmarkPostingsForMatchers(b *testing.B) {
	chunkDir, err := ioutil.TempDir("", "chunk_dir")
	testutil.Ok(b, err)
	defer func() {
		testutil.Ok(b, os.RemoveAll(chunkDir))
	}()
	h, err := NewHead(nil, nil, nil, 1000, chunkDir, nil, DefaultStripeSize, nil)
	testutil.Ok(b, err)
	defer func() {
		testutil.Ok(b, h.Close())
	}()

	app := h.Appender(context.Background())
	addSeries := func(l labels.Labels) {
		app.Add(l, 0, 0)
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
	testutil.Ok(b, app.Commit())

	ir, err := h.Index()
	testutil.Ok(b, err)
	b.Run("Head", func(b *testing.B) {
		benchmarkPostingsForMatchers(b, ir)
	})

	tmpdir, err := ioutil.TempDir("", "test_benchpostingsformatchers")
	testutil.Ok(b, err)
	defer func() {
		testutil.Ok(b, os.RemoveAll(tmpdir))
	}()

	blockdir := createBlockFromHead(b, tmpdir, h)
	block, err := OpenBlock(nil, blockdir, nil)
	testutil.Ok(b, err)
	defer func() {
		testutil.Ok(b, block.Close())
	}()
	ir, err = block.Index()
	testutil.Ok(b, err)
	defer ir.Close()
	b.Run("Block", func(b *testing.B) {
		benchmarkPostingsForMatchers(b, ir)
	})
}

func benchmarkPostingsForMatchers(b *testing.B, ir IndexReader) {
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
	iNot2 := labels.MustNewMatcher(labels.MatchNotEqual, "n", "2"+postingsBenchSuffix)
	iNot2Star := labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^2.*$")
	iNotStar2Star := labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^.*2.*$")

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
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, err := PostingsForMatchers(ir, c.matchers...)
				testutil.Ok(b, err)
			}
		})
	}
}

func BenchmarkQuerierSelect(b *testing.B) {
	tmpDir, err := ioutil.TempDir("", "test_querier_select_dir")
	testutil.Ok(b, err)
	b.Cleanup(func() { testutil.Ok(b, os.RemoveAll(tmpDir)) })

	for i, tcase := range []struct {
		series           int
		samplesPerSeries int
	}{
		//{
		//	series:           1,
		//	samplesPerSeries: 1e8,
		//},
		{
			series:           1e3,
			samplesPerSeries: 1e5,
		},
		//{
		//	series:           1e6,
		//	samplesPerSeries: 1,
		//},
	} {
		b.Run(fmt.Sprintf("series=%v_with_samples=%v", tcase.series, tcase.samplesPerSeries), func(b *testing.B) {
			h, err := NewHead(
				nil, nil, nil,
				DefaultBlockDuration,
				filepath.Join(tmpDir, fmt.Sprintf("%v", i)),
				nil,
				DefaultStripeSize,
				nil,
			)
			testutil.Ok(b, err)
			b.Cleanup(func() { h.Close() })

			app := h.Appender(context.Background())
			r := rand.New(rand.NewSource(123))
			for i := 0; i < tcase.series; i++ {
				ref, err := app.Add(labels.FromStrings("foo", "bar", "i", fmt.Sprintf("%d%s", i, postingsBenchSuffix)), int64(i*tcase.samplesPerSeries), r.Float64())
				testutil.Ok(b, err)
				for j := 1; j < tcase.samplesPerSeries; j++ {
					testutil.Ok(b, app.AddFast(ref, int64(i*tcase.samplesPerSeries+j), r.Float64()))
				}
			}
			testutil.Ok(b, app.Commit())

			bench := func(b *testing.B, br BlockReader, sorted bool) {
				matcher := labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")

				cases := []int{1, tcase.series / 2, tcase.series}
				if tcase.series == 1 {
					cases = []int{1}
				}
				for _, s := range cases {
					b.Run(fmt.Sprintf("%dof%d", s, tcase.series), func(b *testing.B) {
						q, err := NewBlockQuerier(br, 0, int64(s*tcase.samplesPerSeries-1))
						testutil.Ok(b, err)
						defer q.Close()

						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							counter := 0
							ss := q.Select(sorted, nil, matcher)
							for ss.Next() {
								at := ss.At()
								counter++
								it := at.Iterator()
								for it.Next() {
									_, _ = it.At()
								}
								testutil.Ok(b, it.Err())
							}
							testutil.Ok(b, ss.Err())
							testutil.Equals(b, 0, len(ss.Warnings()))
							testutil.Equals(b, s, counter)
						}

					})
				}
			}

			//b.Run("Head", func(b *testing.B) {
			//	bench(b, h, false)
			//})
			//b.Run("SortedHead", func(b *testing.B) {
			//	bench(b, h, true)
			//})

			block, err := OpenBlock(nil, createBlockFromHead(b, filepath.Join(tmpDir, fmt.Sprintf("block%v", i)), h), nil)
			testutil.Ok(b, err)
			b.Cleanup(func() { testutil.Ok(b, block.Close()) })
			b.Run("Block", func(b *testing.B) {
				bench(b, block, false)
			})
		})
	}
}
