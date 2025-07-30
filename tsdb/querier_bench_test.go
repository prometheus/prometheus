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
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/index"
)

// Make entries ~50B in size, to emulate real-world high cardinality.
const (
	postingsBenchSuffix = "aaaaaaaaaabbbbbbbbbbccccccccccdddddddddd"
)

func BenchmarkQuerier(b *testing.B) {
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = b.TempDir()
	h, err := NewHead(nil, nil, nil, nil, opts, nil)
	require.NoError(b, err)
	defer func() {
		require.NoError(b, h.Close())
	}()

	app := h.Appender(context.Background())
	addSeries := func(l labels.Labels) {
		app.Append(0, l, 0, 0)
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
		require.NoError(b, app.Commit())
		app = h.Appender(context.Background())
	}
	require.NoError(b, app.Commit())

	b.Run("Head", func(b *testing.B) {
		ir, err := h.Index()
		require.NoError(b, err)
		defer func() {
			require.NoError(b, ir.Close())
		}()

		b.Run("PostingsForMatchers", func(b *testing.B) {
			benchmarkPostingsForMatchers(b, ir)
		})
		b.Run("labelValuesWithMatchers", func(b *testing.B) {
			benchmarkLabelValuesWithMatchers(b, ir)
		})
	})

	b.Run("Block", func(b *testing.B) {
		blockdir := createBlockFromHead(b, b.TempDir(), h)
		block, err := OpenBlock(nil, blockdir, nil, nil)
		require.NoError(b, err)
		defer func() {
			require.NoError(b, block.Close())
		}()

		ir, err := block.Index()
		require.NoError(b, err)
		defer func() {
			require.NoError(b, ir.Close())
		}()

		b.Run("PostingsForMatchers", func(b *testing.B) {
			benchmarkPostingsForMatchers(b, ir)
		})
		b.Run("labelValuesWithMatchers", func(b *testing.B) {
			benchmarkLabelValuesWithMatchers(b, ir)
		})
	})
}

func benchmarkPostingsForMatchers(b *testing.B, ir IndexReader) {
	ctx := context.Background()

	n1 := labels.MustNewMatcher(labels.MatchEqual, "n", "1"+postingsBenchSuffix)
	nX := labels.MustNewMatcher(labels.MatchEqual, "n", "X"+postingsBenchSuffix)

	jFoo := labels.MustNewMatcher(labels.MatchEqual, "j", "foo")
	jNotFoo := labels.MustNewMatcher(labels.MatchNotEqual, "j", "foo")

	iStar := labels.MustNewMatcher(labels.MatchRegexp, "i", ".*")
	i1Star := labels.MustNewMatcher(labels.MatchRegexp, "i", "1.*")
	iStar1 := labels.MustNewMatcher(labels.MatchRegexp, "i", ".*1")
	iStar1Star := labels.MustNewMatcher(labels.MatchRegexp, "i", ".*1.*")
	iPlus := labels.MustNewMatcher(labels.MatchRegexp, "i", ".+")
	i1Plus := labels.MustNewMatcher(labels.MatchRegexp, "i", "1.+")
	iEmptyRe := labels.MustNewMatcher(labels.MatchRegexp, "i", "")
	iNotEmpty := labels.MustNewMatcher(labels.MatchNotEqual, "i", "")
	iNot2 := labels.MustNewMatcher(labels.MatchNotEqual, "i", "2"+postingsBenchSuffix)
	iNot2Star := labels.MustNewMatcher(labels.MatchNotRegexp, "i", "2.*")
	iNotStar2Star := labels.MustNewMatcher(labels.MatchNotRegexp, "i", ".*2.*")
	jFooBar := labels.MustNewMatcher(labels.MatchRegexp, "j", "foo|bar")
	jXXXYYY := labels.MustNewMatcher(labels.MatchRegexp, "j", "XXX|YYY")
	jXplus := labels.MustNewMatcher(labels.MatchRegexp, "j", "X.+")
	iCharSet := labels.MustNewMatcher(labels.MatchRegexp, "i", "1[0-9]")
	iAlternate := labels.MustNewMatcher(labels.MatchRegexp, "i", "(1|2|3|4|5|6|20|55)")
	iNotAlternate := labels.MustNewMatcher(labels.MatchNotRegexp, "i", "(1|2|3|4|5|6|20|55)")
	iXYZ := labels.MustNewMatcher(labels.MatchRegexp, "i", "X|Y|Z")
	iNotXYZ := labels.MustNewMatcher(labels.MatchNotRegexp, "i", "X|Y|Z")
	cases := []struct {
		name     string
		matchers []*labels.Matcher
	}{
		{`n="1"`, []*labels.Matcher{n1}},
		{`n="X"`, []*labels.Matcher{nX}},
		{`n="1",j="foo"`, []*labels.Matcher{n1, jFoo}},
		{`n="X",j="foo"`, []*labels.Matcher{nX, jFoo}},
		{`j="foo",n="1"`, []*labels.Matcher{jFoo, n1}},
		{`n="1",j!="foo"`, []*labels.Matcher{n1, jNotFoo}},
		{`n="1",i!="2"`, []*labels.Matcher{n1, iNot2}},
		{`n="X",j!="foo"`, []*labels.Matcher{nX, jNotFoo}},
		{`i=~"1[0-9]",j=~"foo|bar"`, []*labels.Matcher{iCharSet, jFooBar}},
		{`j=~"foo|bar"`, []*labels.Matcher{jFooBar}},
		{`j=~"XXX|YYY"`, []*labels.Matcher{jXXXYYY}},
		{`j=~"X.+"`, []*labels.Matcher{jXplus}},
		{`i=~"(1|2|3|4|5|6|20|55)"`, []*labels.Matcher{iAlternate}},
		{`i!~"(1|2|3|4|5|6|20|55)"`, []*labels.Matcher{iNotAlternate}},
		{`i=~"X|Y|Z"`, []*labels.Matcher{iXYZ}},
		{`i!~"X|Y|Z"`, []*labels.Matcher{iNotXYZ}},
		{`i=~".*"`, []*labels.Matcher{iStar}},
		{`i=~"1.*"`, []*labels.Matcher{i1Star}},
		{`i=~".*1"`, []*labels.Matcher{iStar1}},
		{`i=~".+"`, []*labels.Matcher{iPlus}},
		{`i=~".+",j=~"X.+"`, []*labels.Matcher{iPlus, jXplus}},
		{`i=~""`, []*labels.Matcher{iEmptyRe}},
		{`i!=""`, []*labels.Matcher{iNotEmpty}},
		{`n="1",i=~".*",j="foo"`, []*labels.Matcher{n1, iStar, jFoo}},
		{`n="X",i=~".*",j="foo"`, []*labels.Matcher{nX, iStar, jFoo}},
		{`n="1",i=~".*",i!="2",j="foo"`, []*labels.Matcher{n1, iStar, iNot2, jFoo}},
		{`n="1",i!=""`, []*labels.Matcher{n1, iNotEmpty}},
		{`n="1",i!="",j="foo"`, []*labels.Matcher{n1, iNotEmpty, jFoo}},
		{`n="1",i!="",j=~"X.+"`, []*labels.Matcher{n1, iNotEmpty, jXplus}},
		{`n="1",i!="",j=~"XXX|YYY"`, []*labels.Matcher{n1, iNotEmpty, jXXXYYY}},
		{`n="1",i=~"X|Y|Z",j="foo"`, []*labels.Matcher{n1, iXYZ, jFoo}},
		{`n="1",i!~"X|Y|Z",j="foo"`, []*labels.Matcher{n1, iNotXYZ, jFoo}},
		{`n="1",i=~".+",j="foo"`, []*labels.Matcher{n1, iPlus, jFoo}},
		{`n="1",i=~"1.+",j="foo"`, []*labels.Matcher{n1, i1Plus, jFoo}},
		{`n="1",i=~".*1.*",j="foo"`, []*labels.Matcher{n1, iStar1Star, jFoo}},
		{`n="1",i=~".+",i!="2",j="foo"`, []*labels.Matcher{n1, iPlus, iNot2, jFoo}},
		{`n="1",i=~".+",i!~"2.*",j="foo"`, []*labels.Matcher{n1, iPlus, iNot2Star, jFoo}},
		{`n="1",i=~".+",i!~".*2.*",j="foo"`, []*labels.Matcher{n1, iPlus, iNotStar2Star, jFoo}},
		{`n="X",i=~".+",i!~".*2.*",j="foo"`, []*labels.Matcher{nX, iPlus, iNotStar2Star, jFoo}},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := PostingsForMatchers(ctx, ir, c.matchers...)
				require.NoError(b, err)
			}
		})
	}
}

func benchmarkLabelValuesWithMatchers(b *testing.B, ir IndexReader) {
	i1 := labels.MustNewMatcher(labels.MatchEqual, "i", "1")
	i1Plus := labels.MustNewMatcher(labels.MatchRegexp, "i", "1.+")
	i1PostingsBenchSuffix := labels.MustNewMatcher(labels.MatchEqual, "i", "1"+postingsBenchSuffix)
	iSuffix := labels.MustNewMatcher(labels.MatchRegexp, "i", ".+ddd")
	iStar := labels.MustNewMatcher(labels.MatchRegexp, "i", ".*")
	jNotFoo := labels.MustNewMatcher(labels.MatchNotEqual, "j", "foo")
	jXXXYYY := labels.MustNewMatcher(labels.MatchRegexp, "j", "XXX|YYY")
	jXplus := labels.MustNewMatcher(labels.MatchRegexp, "j", "X.+")
	n1 := labels.MustNewMatcher(labels.MatchEqual, "n", "1"+postingsBenchSuffix)
	nX := labels.MustNewMatcher(labels.MatchNotEqual, "n", "X"+postingsBenchSuffix)
	nPlus := labels.MustNewMatcher(labels.MatchRegexp, "n", ".+")

	ctx := context.Background()

	cases := []struct {
		name      string
		labelName string
		matchers  []*labels.Matcher
	}{
		// i is never "1", this isn't matching anything.
		{`i with i="1"`, "i", []*labels.Matcher{i1}},
		// i has 100k values.
		{`i with n="1"`, "i", []*labels.Matcher{n1}},
		{`i with n=".+"`, "i", []*labels.Matcher{nPlus}},
		{`i with n="1",j!="foo"`, "i", []*labels.Matcher{n1, jNotFoo}},
		{`i with n="1",j=~"X.+"`, "i", []*labels.Matcher{n1, jXplus}},
		{`i with n="1",j=~"XXX|YYY"`, "i", []*labels.Matcher{n1, jXXXYYY}},
		{`i with n="X",j!="foo"`, "i", []*labels.Matcher{nX, jNotFoo}},
		{`i with n="1",i=~".*",j!="foo"`, "i", []*labels.Matcher{n1, iStar, jNotFoo}},
		// matchers on i itself
		{`i with i="1aaa...ddd"`, "i", []*labels.Matcher{i1PostingsBenchSuffix}},
		{`i with i=~"1.+"`, "i", []*labels.Matcher{i1Plus}},
		{`i with i=~"1.+",i=~".+ddd"`, "i", []*labels.Matcher{i1Plus, iSuffix}},
		// n has 10 values.
		{`n with j!="foo"`, "n", []*labels.Matcher{jNotFoo}},
		{`n with i="1"`, "n", []*labels.Matcher{i1}},
		{`n with i=~"1.+"`, "n", []*labels.Matcher{i1Plus}},
		// there's no label "none"
		{`none with i=~"1"`, "none", []*labels.Matcher{i1Plus}},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, err := labelValuesWithMatchers(ctx, ir, c.labelName, nil, c.matchers...)
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkMergedStringIter(b *testing.B) {
	numSymbols := 100000
	s := make([]string, numSymbols)
	for i := 0; i < numSymbols; i++ {
		s[i] = fmt.Sprintf("symbol%v", i)
	}

	for i := 0; i < b.N; i++ {
		it := NewMergedStringIter(index.NewStringListIter(s), index.NewStringListIter(s))
		for j := 0; j < 100; j++ {
			it = NewMergedStringIter(it, index.NewStringListIter(s))
		}

		for it.Next() {
			require.NotNil(b, it.At())
			require.NoError(b, it.Err())
		}
	}

	b.ReportAllocs()
}

func createHeadForBenchmarkSelect(b *testing.B, numSeries int, addSeries func(app storage.Appender, i int)) (*Head, *DB) {
	dir := b.TempDir()
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 255
	opts.OutOfOrderTimeWindow = 1000
	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(b, err)
	b.Cleanup(func() {
		require.NoError(b, db.Close())
	})
	h := db.Head()

	app := h.Appender(context.Background())
	for i := 0; i < numSeries; i++ {
		addSeries(app, i)
		if i%1000 == 999 { // Commit every so often, so the appender doesn't get too big.
			require.NoError(b, app.Commit())
			app = h.Appender(context.Background())
		}
	}
	require.NoError(b, app.Commit())
	return h, db
}

func benchmarkSelect(b *testing.B, queryable storage.Queryable, sorted bool, minT, maxT int64, matchers ...*labels.Matcher) {
	q, err := queryable.Querier(minT, maxT)
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ss := q.Select(context.Background(), sorted, nil, matchers...)
		var chunkIterator chunkenc.Iterator
		for ss.Next() {
			// Access labels and get an iterator to keep the comparison more realistic to a real usage pattern
			_ = ss.At().Labels()
			chunkIterator = ss.At().Iterator(chunkIterator)
		}
		require.NoError(b, ss.Err())
	}
	q.Close()
}

func (pb *Block) Querier(mint, maxt int64) (storage.Querier, error) {
	return NewBlockQuerier(pb, mint, maxt)
}

func BenchmarkQuerierSelectWithOutOfOrder(b *testing.B) {
	numSeries := 1000000
	_, db := createHeadForBenchmarkSelect(b, numSeries, func(app storage.Appender, i int) {
		l := labels.FromStrings("foo", "bar", "i", fmt.Sprintf("%d%s", i, postingsBenchSuffix))
		ref, err := app.Append(0, l, int64(i+1), 0)
		if err != nil {
			b.Fatal(err)
		}
		_, err = app.Append(ref, l, int64(i), 1) // Out of order sample
		if err != nil {
			b.Fatal(err)
		}
	})
	matcher := labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")
	b.Run("Head", func(b *testing.B) {
		b.ResetTimer()
		for s := 1; s <= numSeries; s *= 10 {
			b.Run(fmt.Sprintf("%dof%d", s, numSeries), func(b *testing.B) {
				benchmarkSelect(b, db, false, 0, int64(s-1), matcher)
			})
		}
	})
}

var benchmarkCases = []struct {
	name     string
	matchers []*labels.Matcher
}{
	{
		name: "SingleMetricAllSeries",
		matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "__name__", "test_metric_1"),
		},
	},
	{
		name: "SingleMetricReducedSeries",
		matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "__name__", "test_metric_1"),
			labels.MustNewMatcher(labels.MatchEqual, "instance", "instance-1"),
		},
	},
	{
		name: "SingleMetricOneSeries",
		matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "__name__", "test_metric_1"),
			labels.MustNewMatcher(labels.MatchEqual, "instance", "instance-2"),
			labels.MustNewMatcher(labels.MatchEqual, "region", "region-1"),
			labels.MustNewMatcher(labels.MatchEqual, "zone", "zone-3"),
			labels.MustNewMatcher(labels.MatchEqual, "service", "service-10"),
			labels.MustNewMatcher(labels.MatchEqual, "environment", "environment-1"),
		},
	},
	{
		name: "SingleMetricSparseSeries",
		matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "__name__", "test_metric_1"),
			labels.MustNewMatcher(labels.MatchEqual, "service", "service-1"),
			labels.MustNewMatcher(labels.MatchEqual, "environment", "environment-0"),
		},
	},
	{
		name: "NonExistentSeries",
		matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "__name__", "test_metric_1"),
			labels.MustNewMatcher(labels.MatchEqual, "environment", "non-existent-environment"),
		},
	},
	{
		name: "MultipleMetricsRange",
		matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchRegexp, "__name__", "test_metric_[1-5]"),
		},
	},
	{
		name: "MultipleMetricsSparse",
		matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchRegexp, "__name__", "test_metric_(1|5|10|15|20)"),
		},
	},
	{
		name: "NegativeRegexSingleMetric",
		matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "__name__", "test_metric_1"),
			labels.MustNewMatcher(labels.MatchNotRegexp, "instance", "(instance-1.*|instance-2.*)"),
		},
	},
	{
		name: "NegativeRegexMultipleMetrics",
		matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchRegexp, "__name__", "test_metric_[1-3]"),
			labels.MustNewMatcher(labels.MatchNotRegexp, "instance", "(instance-1.*|instance-2.*)"),
		},
	},
	{
		name: "ExpensiveRegexSingleMetric",
		matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "__name__", "test_metric_1"),
			labels.MustNewMatcher(labels.MatchRegexp, "instance", "(container-1|instance-2|container-3|instance-4|container-5)"),
		},
	},
	{
		name: "ExpensiveRegexMultipleMetrics",
		matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchRegexp, "__name__", "test_metric_[1-3]"),
			labels.MustNewMatcher(labels.MatchRegexp, "instance", "(container-1|container-2|container-3|container-4|container-5)"),
		},
	},
	{
		name: "NoopRegexMatcher",
		matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "__name__", "test_metric_1"),
			labels.MustNewMatcher(labels.MatchRegexp, "environment", ".+"),
		},
	},
}

func BenchmarkSelect(b *testing.B) {
	// Use the same approach as other benchmarks
	dir := b.TempDir()
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 255
	opts.OutOfOrderTimeWindow = 1000
	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(b, err)
	b.Cleanup(func() {
		require.NoError(b, db.Close())
	})
	h := db.Head()

	app := h.Appender(context.Background())

	// 5 metrics × 100 instances × 5 regions × 10 zones × 20 services × 3 environments = 1,500,000 series
	metrics := 5
	instances := 100
	regions := 5
	zones := 10
	services := 20
	environments := 3

	totalSeries := metrics * instances * regions * zones * services * environments
	b.Logf("Generating %d series (%d metrics × %d instances × %d regions × %d zones × %d services × %d environments)",
		totalSeries, metrics, instances, regions, zones, services, environments)

	seriesCount := 0
	for m := range metrics {
		for i := range instances {
			for r := range regions {
				for z := range zones {
					for s := range services {
						for e := range environments {
							lbls := labels.FromStrings(
								"__name__", fmt.Sprintf("test_metric_%d", m),
								"instance", fmt.Sprintf("instance-%d", i),
								"region", fmt.Sprintf("region-%d", r),
								"zone", fmt.Sprintf("zone-%d", z),
								"service", fmt.Sprintf("service-%d", s),
								"environment", fmt.Sprintf("environment-%d", e),
							)
							_, _ = app.Append(0, lbls, 0, rand.Float64())
							seriesCount++
							if seriesCount%1000 == 0 { // Commit every so often, so the appender doesn't get too big.
								require.NoError(b, app.Commit())
								app = h.Appender(context.Background())
							}
						}
					}
				}
			}
		}
	}
	require.NoError(b, app.Commit())

	require.Equal(b, totalSeries, int(h.NumSeries()), "Expected number of series does not match")

	b.Run("Head", func(b *testing.B) {
		for _, bc := range benchmarkCases {
			b.Run(bc.name, func(b *testing.B) {
				benchmarkSelect(b, db, false, 0, 120, bc.matchers...)
			})
		}
	})

	b.Run("SortedHead", func(b *testing.B) {
		for _, bc := range benchmarkCases {
			b.Run(bc.name, func(b *testing.B) {
				benchmarkSelect(b, db, true, 0, 120, bc.matchers...)
			})
		}
	})

	blockDir := createBlockFromHead(b, b.TempDir(), h)
	block, err := OpenBlock(nil, blockDir, nil, nil)
	require.NoError(b, err)
	defer func() {
		require.NoError(b, block.Close())
	}()

	b.Run("Block", func(b *testing.B) {
		for _, bc := range benchmarkCases {
			b.Run(bc.name, func(b *testing.B) {
				benchmarkSelect(b, block, true, 0, 120, bc.matchers...)
			})
		}
	})
}
