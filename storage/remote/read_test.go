// Copyright 2017 The Prometheus Authors
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

package remote

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestNoDuplicateReadConfigs(t *testing.T) {
	dir := t.TempDir()

	cfg1 := config.RemoteReadConfig{
		Name: "write-1",
		URL: &config_util.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost1",
			},
		},
	}
	cfg2 := config.RemoteReadConfig{
		Name: "write-2",
		URL: &config_util.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost2",
			},
		},
	}
	cfg3 := config.RemoteReadConfig{
		URL: &config_util.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost3",
			},
		},
	}

	type testcase struct {
		cfgs []*config.RemoteReadConfig
		err  bool
	}

	cases := []testcase{
		{ // Duplicates but with different names, we should not get an error.
			cfgs: []*config.RemoteReadConfig{
				&cfg1,
				&cfg2,
			},
			err: false,
		},
		{ // Duplicates but one with no name, we should not get an error.
			cfgs: []*config.RemoteReadConfig{
				&cfg1,
				&cfg3,
			},
			err: false,
		},
		{ // Duplicates both with no name, we should get an error.
			cfgs: []*config.RemoteReadConfig{
				&cfg3,
				&cfg3,
			},
			err: true,
		},
	}

	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil)
			conf := &config.Config{
				GlobalConfig:      config.DefaultGlobalConfig,
				RemoteReadConfigs: tc.cfgs,
			}
			err := s.ApplyConfig(conf)
			prometheus.Unregister(s.rws.highestTimestamp)
			gotError := err != nil
			require.Equal(t, tc.err, gotError)
			require.NoError(t, s.Close())
		})
	}
}

func TestExternalLabelsQuerierAddExternalLabels(t *testing.T) {
	tests := []struct {
		el          labels.Labels
		inMatchers  []*labels.Matcher
		outMatchers []*labels.Matcher
		added       []string
	}{
		{
			inMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
			},
			outMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
			},
			added: []string{},
		},
		{
			el: labels.FromStrings("dc", "berlin-01", "region", "europe"),
			inMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
			},
			outMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
				labels.MustNewMatcher(labels.MatchEqual, "region", "europe"),
				labels.MustNewMatcher(labels.MatchEqual, "dc", "berlin-01"),
			},
			added: []string{"dc", "region"},
		},
		{
			el: labels.FromStrings("dc", "berlin-01", "region", "europe"),
			inMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
				labels.MustNewMatcher(labels.MatchEqual, "dc", "munich-02"),
			},
			outMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
				labels.MustNewMatcher(labels.MatchEqual, "region", "europe"),
				labels.MustNewMatcher(labels.MatchEqual, "dc", "munich-02"),
			},
			added: []string{"region"},
		},
	}

	for i, test := range tests {
		q := &querier{externalLabels: test.el}
		matchers, added := q.addExternalLabels(test.inMatchers)

		sort.Slice(test.outMatchers, func(i, j int) bool { return test.outMatchers[i].Name < test.outMatchers[j].Name })
		sort.Slice(matchers, func(i, j int) bool { return matchers[i].Name < matchers[j].Name })

		require.Equal(t, test.outMatchers, matchers, "%d", i)
		require.Equal(t, test.added, added, "%d", i)
	}
}

func TestSeriesSetFilter(t *testing.T) {
	tests := []struct {
		in       *prompb.QueryResult
		toRemove []string

		expected *prompb.QueryResult
	}{
		{
			toRemove: []string{"foo"},
			in: &prompb.QueryResult{
				Timeseries: []*prompb.TimeSeries{
					{Labels: prompb.FromLabels(labels.FromStrings("foo", "bar", "a", "b"), nil)},
				},
			},
			expected: &prompb.QueryResult{
				Timeseries: []*prompb.TimeSeries{
					{Labels: prompb.FromLabels(labels.FromStrings("a", "b"), nil)},
				},
			},
		},
	}

	for _, tc := range tests {
		filtered := newSeriesSetFilter(FromQueryResult(true, tc.in), tc.toRemove)
		act, ws, err := ToQueryResult(filtered, 1e6)
		require.NoError(t, err)
		require.Empty(t, ws)
		require.Equal(t, tc.expected, act)
	}
}

type mockedRemoteClient struct {
	got         *prompb.Query
	gotMultiple []*prompb.Query
	store       []*prompb.TimeSeries
	b           labels.ScratchBuilder
}

func (c *mockedRemoteClient) Read(_ context.Context, query *prompb.Query, sortSeries bool) (storage.SeriesSet, error) {
	if c.got != nil {
		return nil, fmt.Errorf("expected only one call to remote client got: %v", query)
	}
	c.got = query

	matchers, err := FromLabelMatchers(query.Matchers)
	if err != nil {
		return nil, err
	}

	q := &prompb.QueryResult{}
	for _, s := range c.store {
		l := s.ToLabels(&c.b, nil)
		var notMatch bool

		for _, m := range matchers {
			if v := l.Get(m.Name); v != "" {
				if !m.Matches(v) {
					notMatch = true
					break
				}
			}
		}

		if notMatch {
			continue
		}
		// Filter samples by query time range
		var filteredSamples []prompb.Sample
		for _, sample := range s.Samples {
			if sample.Timestamp >= query.StartTimestampMs && sample.Timestamp <= query.EndTimestampMs {
				filteredSamples = append(filteredSamples, sample)
			}
		}
		q.Timeseries = append(q.Timeseries, &prompb.TimeSeries{Labels: s.Labels, Samples: filteredSamples})
	}
	return FromQueryResult(sortSeries, q), nil
}

func (c *mockedRemoteClient) ReadMultiple(_ context.Context, queries []*prompb.Query, sortSeries bool) (storage.SeriesSet, error) {
	// Store the queries for verification
	c.gotMultiple = make([]*prompb.Query, len(queries))
	copy(c.gotMultiple, queries)

	// Simulate the same behavior as the real client
	var results []*prompb.QueryResult
	for _, query := range queries {
		matchers, err := FromLabelMatchers(query.Matchers)
		if err != nil {
			return nil, err
		}

		q := &prompb.QueryResult{}
		for _, s := range c.store {
			l := s.ToLabels(&c.b, nil)
			var notMatch bool

			for _, m := range matchers {
				v := l.Get(m.Name)
				if !m.Matches(v) {
					notMatch = true
					break
				}
			}

			if notMatch {
				continue
			}
			// Filter samples by query time range
			var filteredSamples []prompb.Sample
			for _, sample := range s.Samples {
				if sample.Timestamp >= query.StartTimestampMs && sample.Timestamp <= query.EndTimestampMs {
					filteredSamples = append(filteredSamples, sample)
				}
			}
			q.Timeseries = append(q.Timeseries, &prompb.TimeSeries{Labels: s.Labels, Samples: filteredSamples})
		}
		results = append(results, q)
	}

	// Use the same logic as the real client
	return combineQueryResults(results, sortSeries)
}

func (c *mockedRemoteClient) reset() {
	c.got = nil
	c.gotMultiple = nil
}

// NOTE: We don't need to test ChunkQuerier as it's uses querier for all operations anyway.
func TestSampleAndChunkQueryableClient(t *testing.T) {
	m := &mockedRemoteClient{
		// Samples does not matter for below tests.
		store: []*prompb.TimeSeries{
			{Labels: []prompb.Label{{Name: "a", Value: "b"}}},
			{Labels: []prompb.Label{{Name: "a", Value: "b3"}, {Name: "region", Value: "us"}}},
			{Labels: []prompb.Label{{Name: "a", Value: "b2"}, {Name: "region", Value: "europe"}}},
		},
		b: labels.NewScratchBuilder(0),
	}

	for _, tc := range []struct {
		name             string
		matchers         []*labels.Matcher
		mint, maxt       int64
		externalLabels   labels.Labels
		requiredMatchers []*labels.Matcher
		readRecent       bool
		callback         startTimeCallback

		expectedQuery  *prompb.Query
		expectedSeries []labels.Labels
	}{
		{
			name: "empty",
			mint: 1, maxt: 2,
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchNotEqual, "a", "something"),
			},
			readRecent: true,

			expectedQuery: &prompb.Query{
				StartTimestampMs: 1,
				EndTimestampMs:   2,
				Matchers: []*prompb.LabelMatcher{
					{Type: prompb.LabelMatcher_NEQ, Name: "a", Value: "something"},
				},
			},
			expectedSeries: []labels.Labels{
				labels.FromStrings("a", "b"),
				labels.FromStrings("a", "b2", "region", "europe"),
				labels.FromStrings("a", "b3", "region", "us"),
			},
		},
		{
			name: "external labels specified, not explicitly requested",
			mint: 1, maxt: 2,
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchNotEqual, "a", "something"),
			},
			readRecent:     true,
			externalLabels: labels.FromStrings("region", "europe"),

			expectedQuery: &prompb.Query{
				StartTimestampMs: 1,
				EndTimestampMs:   2,
				Matchers: []*prompb.LabelMatcher{
					{Type: prompb.LabelMatcher_NEQ, Name: "a", Value: "something"},
					{Type: prompb.LabelMatcher_EQ, Name: "region", Value: "europe"},
				},
			},
			expectedSeries: []labels.Labels{
				labels.FromStrings("a", "b"),
				labels.FromStrings("a", "b2"),
			},
		},
		{
			name: "external labels specified, explicitly requested europe",
			mint: 1, maxt: 2,
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchNotEqual, "a", "something"),
				labels.MustNewMatcher(labels.MatchEqual, "region", "europe"),
			},
			readRecent:     true,
			externalLabels: labels.FromStrings("region", "europe"),

			expectedQuery: &prompb.Query{
				StartTimestampMs: 1,
				EndTimestampMs:   2,
				Matchers: []*prompb.LabelMatcher{
					{Type: prompb.LabelMatcher_NEQ, Name: "a", Value: "something"},
					{Type: prompb.LabelMatcher_EQ, Name: "region", Value: "europe"},
				},
			},
			expectedSeries: []labels.Labels{
				labels.FromStrings("a", "b"),
				labels.FromStrings("a", "b2", "region", "europe"),
			},
		},
		{
			name: "external labels specified, explicitly requested not europe",
			mint: 1, maxt: 2,
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchNotEqual, "a", "something"),
				labels.MustNewMatcher(labels.MatchEqual, "region", "us"),
			},
			readRecent:     true,
			externalLabels: labels.FromStrings("region", "europe"),

			expectedQuery: &prompb.Query{
				StartTimestampMs: 1,
				EndTimestampMs:   2,
				Matchers: []*prompb.LabelMatcher{
					{Type: prompb.LabelMatcher_NEQ, Name: "a", Value: "something"},
					{Type: prompb.LabelMatcher_EQ, Name: "region", Value: "us"},
				},
			},
			expectedSeries: []labels.Labels{
				labels.FromStrings("a", "b"),
				labels.FromStrings("a", "b3", "region", "us"),
			},
		},
		{
			name: "prefer local storage",
			mint: 0, maxt: 50,
			callback:   func() (i int64, err error) { return 100, nil },
			readRecent: false,

			expectedQuery: &prompb.Query{
				StartTimestampMs: 0,
				EndTimestampMs:   50,
				Matchers:         []*prompb.LabelMatcher{},
			},
			expectedSeries: []labels.Labels{
				labels.FromStrings("a", "b"),
				labels.FromStrings("a", "b2", "region", "europe"),
				labels.FromStrings("a", "b3", "region", "us"),
			},
		},
		{
			name: "prefer local storage, limited time",
			mint: 0, maxt: 50,
			callback:   func() (i int64, err error) { return 20, nil },
			readRecent: false,

			expectedQuery: &prompb.Query{
				StartTimestampMs: 0,
				EndTimestampMs:   20,
				Matchers:         []*prompb.LabelMatcher{},
			},
			expectedSeries: []labels.Labels{
				labels.FromStrings("a", "b"),
				labels.FromStrings("a", "b2", "region", "europe"),
				labels.FromStrings("a", "b3", "region", "us"),
			},
		},
		{
			name: "prefer local storage, skipped",
			mint: 30, maxt: 50,
			callback:   func() (i int64, err error) { return 20, nil },
			readRecent: false,

			expectedQuery:  nil,
			expectedSeries: nil, // Noop should be used.
		},
		{
			name: "required matcher specified, user also specifies same",
			mint: 1, maxt: 2,
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "b2"),
			},
			readRecent: true,
			requiredMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "b2"),
			},

			expectedQuery: &prompb.Query{
				StartTimestampMs: 1,
				EndTimestampMs:   2,
				Matchers: []*prompb.LabelMatcher{
					{Type: prompb.LabelMatcher_EQ, Name: "a", Value: "b2"},
				},
			},
			expectedSeries: []labels.Labels{
				labels.FromStrings("a", "b2", "region", "europe"),
			},
		},
		{
			name: "required matcher specified",
			mint: 1, maxt: 2,
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "b2"),
			},
			readRecent: true,
			requiredMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "b2"),
			},

			expectedQuery: &prompb.Query{
				StartTimestampMs: 1,
				EndTimestampMs:   2,
				Matchers: []*prompb.LabelMatcher{
					{Type: prompb.LabelMatcher_EQ, Name: "a", Value: "b2"},
				},
			},
			expectedSeries: []labels.Labels{
				labels.FromStrings("a", "b2", "region", "europe"),
			},
		},
		{
			name: "required matcher specified, given matcher does not match",
			mint: 1, maxt: 2,
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchNotEqual, "a", "something"),
			},
			readRecent: true,
			requiredMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "b2"),
			},

			expectedQuery:  nil,
			expectedSeries: nil, // Given matchers does not match with required ones, noop expected.
		},
		{
			name: "required matcher specified, given matcher does not match2",
			mint: 1, maxt: 2,
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchNotEqual, "x", "something"),
			},
			readRecent: true,
			requiredMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "b2"),
			},
			expectedQuery:  nil,
			expectedSeries: nil, // Given matchers does not match with required ones, noop expected.
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m.reset()

			c := NewSampleAndChunkQueryableClient(
				m,
				tc.externalLabels,
				tc.requiredMatchers,
				tc.readRecent,
				tc.callback,
			)
			q, err := c.Querier(tc.mint, tc.maxt)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, q.Close())
			}()

			ss := q.Select(context.Background(), true, nil, tc.matchers...)
			require.NoError(t, err)
			require.Equal(t, annotations.Annotations(nil), ss.Warnings())

			require.Equal(t, tc.expectedQuery, m.got)

			var got []labels.Labels
			for ss.Next() {
				got = append(got, ss.At().Labels())
			}
			require.NoError(t, ss.Err())
			testutil.RequireEqual(t, tc.expectedSeries, got)
		})
	}
}

func TestReadMultiple(t *testing.T) {
	const sampleIntervalMs = 250

	// Helper function to generate samples at 250ms intervals for a time range
	generateSamples := func(startMs, endMs int64) []prompb.Sample {
		var samples []prompb.Sample
		for ts := startMs; ts <= endMs; ts += sampleIntervalMs {
			samples = append(samples, prompb.Sample{
				Timestamp: ts,
				Value:     float64(ts), // Use timestamp as value for simplicity
			})
		}
		return samples
	}

	m := &mockedRemoteClient{
		store: []*prompb.TimeSeries{
			{
				Labels:  []prompb.Label{{Name: "job", Value: "prometheus"}},
				Samples: generateSamples(0, 10000),
			},
			{
				Labels:  []prompb.Label{{Name: "job", Value: "node_exporter"}},
				Samples: generateSamples(0, 10000),
			},
			{
				Labels:  []prompb.Label{{Name: "job", Value: "cadvisor"}, {Name: "region", Value: "us"}},
				Samples: generateSamples(0, 10000),
			},
			{
				Labels:  []prompb.Label{{Name: "instance", Value: "localhost:9090"}},
				Samples: generateSamples(0, 10000),
			},
		},
		b: labels.NewScratchBuilder(0),
	}

	// Expected result structure
	type expectedSeries struct {
		labels      labels.Labels
		sampleCount int
	}

	testCases := []struct {
		name            string
		queries         []*prompb.Query
		expectedResults []expectedSeries
	}{
		{
			name: "single query",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "prometheus"},
					},
				},
			},
			expectedResults: []expectedSeries{
				{
					labels:      labels.FromStrings("job", "prometheus"),
					sampleCount: 5,
				},
			},
		},
		{
			name: "multiple queries - different matchers",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "prometheus"},
					},
				},
				{
					StartTimestampMs: 1500,
					EndTimestampMs:   2500,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "node_exporter"},
					},
				},
			},
			expectedResults: []expectedSeries{
				{
					labels:      labels.FromStrings("job", "node_exporter"),
					sampleCount: 5,
				},
				{
					labels:      labels.FromStrings("job", "prometheus"),
					sampleCount: 5,
				},
			},
		},
		{
			name: "multiple queries - overlapping results",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_RE, Name: "job", Value: "prometheus|node_exporter"},
					},
				},
				{
					StartTimestampMs: 1500,
					EndTimestampMs:   2500,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "region", Value: "us"},
					},
				},
			},
			expectedResults: []expectedSeries{
				{
					labels:      labels.FromStrings("job", "cadvisor", "region", "us"),
					sampleCount: 5,
				},
				{
					labels:      labels.FromStrings("job", "node_exporter"),
					sampleCount: 5,
				},
				{
					labels:      labels.FromStrings("job", "prometheus"),
					sampleCount: 5,
				},
			},
		},
		{
			name: "query with no results",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "nonexistent"},
					},
				},
			},
			expectedResults: nil, // empty result
		},
		{
			name:            "empty query list",
			queries:         []*prompb.Query{},
			expectedResults: nil,
		},
		{
			name: "three queries with mixed results",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "prometheus"},
					},
				},
				{
					StartTimestampMs: 1500,
					EndTimestampMs:   2500,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "nonexistent"},
					},
				},
				{
					StartTimestampMs: 2000,
					EndTimestampMs:   3000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "instance", Value: "localhost:9090"},
					},
				},
			},
			expectedResults: []expectedSeries{
				{
					labels:      labels.FromStrings("instance", "localhost:9090"),
					sampleCount: 5,
				},
				{
					labels:      labels.FromStrings("job", "prometheus"),
					sampleCount: 5,
				},
			},
		},
		{
			name: "same matchers with overlapping time ranges",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   5000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "region", Value: "us"},
					},
				},
				{
					StartTimestampMs: 3000,
					EndTimestampMs:   8000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "region", Value: "us"},
					},
				},
			},
			expectedResults: []expectedSeries{
				{
					labels:      labels.FromStrings("job", "cadvisor", "region", "us"),
					sampleCount: 29, // Union of [1000,5000] and [3000,8000] = [1000,8000] = (8000-1000)/250 + 1 (on the edge) = 29 unique samples
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m.reset()

			result, err := m.ReadMultiple(context.Background(), tc.queries, true)
			require.NoError(t, err)

			// Verify the queries were stored correctly
			require.Equal(t, tc.queries, m.gotMultiple)

			// Verify the combined result matches expected
			var got []expectedSeries
			for result.Next() {
				series := result.At()
				sampleCount := 0
				iterator := series.Iterator(nil)
				for iterator.Next() != chunkenc.ValNone {
					sampleCount++
				}
				require.NoError(t, iterator.Err())

				got = append(got, expectedSeries{
					labels:      series.Labels(),
					sampleCount: sampleCount,
				})
			}
			require.NoError(t, result.Err())

			// Sort both expected and got to ensure deterministic comparison
			sort.Slice(tc.expectedResults, func(i, j int) bool {
				return tc.expectedResults[i].labels.String() < tc.expectedResults[j].labels.String()
			})
			sort.Slice(got, func(i, j int) bool {
				return got[i].labels.String() < got[j].labels.String()
			})

			// Compare by label string representation and sample count to handle dedupelabels build tag
			require.Equal(t, len(tc.expectedResults), len(got), "number of series should match")
			for i := range tc.expectedResults {
				require.Equal(t, tc.expectedResults[i].labels.String(), got[i].labels.String(), "labels should match")
				require.Equal(t, tc.expectedResults[i].sampleCount, got[i].sampleCount, "sample count should match")
			}
		})
	}
}

func TestReadMultipleErrorHandling(t *testing.T) {
	m := &mockedRemoteClient{
		store: []*prompb.TimeSeries{
			{Labels: []prompb.Label{{Name: "job", Value: "prometheus"}}},
		},
		b: labels.NewScratchBuilder(0),
	}

	// Test with invalid matcher - should return error
	queries := []*prompb.Query{
		{
			StartTimestampMs: 1000,
			EndTimestampMs:   2000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_Type(999), Name: "job", Value: "prometheus"}, // invalid matcher type
			},
		},
	}

	result, err := m.ReadMultiple(context.Background(), queries, true)
	require.Error(t, err)
	require.Nil(t, result)
}

func TestReadMultipleSorting(t *testing.T) {
	// Test data with labels designed to test sorting behavior
	// When sorted: aaa < bbb < ccc
	// When unsorted: order depends on processing order
	m := &mockedRemoteClient{
		store: []*prompb.TimeSeries{
			{Labels: []prompb.Label{{Name: "series", Value: "ccc"}}}, // Will be returned by query 1
			{Labels: []prompb.Label{{Name: "series", Value: "aaa"}}}, // Will be returned by query 2
			{Labels: []prompb.Label{{Name: "series", Value: "bbb"}}}, // Will be returned by both queries (overlapping)
		},
		b: labels.NewScratchBuilder(0),
	}

	testCases := []struct {
		name          string
		queries       []*prompb.Query
		sortSeries    bool
		expectedOrder []string
	}{
		{
			name: "multiple queries with sortSeries=true - should be sorted",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_RE, Name: "series", Value: "ccc|bbb"}, // Returns: ccc, bbb (unsorted in store)
					},
				},
				{
					StartTimestampMs: 1500,
					EndTimestampMs:   2500,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_RE, Name: "series", Value: "aaa|bbb"}, // Returns: aaa, bbb (unsorted in store)
					},
				},
			},
			sortSeries:    true,
			expectedOrder: []string{"aaa", "bbb", "ccc"}, // Should be sorted after merge
		},
		{
			name: "multiple queries with sortSeries=false - concatenates without deduplication",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_RE, Name: "series", Value: "ccc|bbb"}, // Returns: ccc, bbb (unsorted)
					},
				},
				{
					StartTimestampMs: 1500,
					EndTimestampMs:   2500,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_RE, Name: "series", Value: "aaa|bbb"}, // Returns: aaa, bbb (unsorted)
					},
				},
			},
			sortSeries:    false,
			expectedOrder: []string{"ccc", "bbb", "aaa", "bbb"}, // Concatenated results - duplicates included
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m.reset()

			result, err := m.ReadMultiple(context.Background(), tc.queries, tc.sortSeries)
			require.NoError(t, err)

			// Collect the actual results
			var actualOrder []string
			for result.Next() {
				series := result.At()
				seriesValue := series.Labels().Get("series")
				actualOrder = append(actualOrder, seriesValue)
			}
			require.NoError(t, result.Err())

			// Verify the expected order matches actual order
			// For sortSeries=true: results should be in sorted order
			// For sortSeries=false: results should be in concatenated order (with duplicates)
			testutil.RequireEqual(t, tc.expectedOrder, actualOrder)
		})
	}
}
