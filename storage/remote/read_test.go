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
	"io/ioutil"
	"net/url"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/pkg/errors"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestNoDuplicateReadConfigs(t *testing.T) {
	dir, err := ioutil.TempDir("", "TestNoDuplicateReadConfigs")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

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
			s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline)
			conf := &config.Config{
				GlobalConfig:      config.DefaultGlobalConfig,
				RemoteReadConfigs: tc.cfgs,
			}
			err := s.ApplyConfig(conf)
			gotError := err != nil
			testutil.Equals(t, tc.err, gotError)
			testutil.Ok(t, s.Close())
		})
	}
}

func TestExternalLabelsQuerierAddExternalLabels(t *testing.T) {
	tests := []struct {
		el          labels.Labels
		inMatchers  []*labels.Matcher
		outMatchers []*labels.Matcher
		added       labels.Labels
	}{
		{
			inMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
			},
			outMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
			},
			added: labels.Labels{},
		},
		{
			el: labels.Labels{
				{Name: "dc", Value: "berlin-01"},
				{Name: "region", Value: "europe"},
			},
			inMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
			},
			outMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
				labels.MustNewMatcher(labels.MatchEqual, "region", "europe"),
				labels.MustNewMatcher(labels.MatchEqual, "dc", "berlin-01"),
			},
			added: labels.Labels{
				{Name: "dc", Value: "berlin-01"},
				{Name: "region", Value: "europe"},
			},
		},
		{
			el: labels.Labels{
				{Name: "region", Value: "europe"},
				{Name: "dc", Value: "berlin-01"},
			},
			inMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
				labels.MustNewMatcher(labels.MatchEqual, "dc", "munich-02"),
			},
			outMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
				labels.MustNewMatcher(labels.MatchEqual, "region", "europe"),
				labels.MustNewMatcher(labels.MatchEqual, "dc", "munich-02"),
			},
			added: labels.Labels{
				{Name: "region", Value: "europe"},
			},
		},
	}

	for i, test := range tests {
		q := &querier{externalLabels: test.el}
		matchers, added := q.addExternalLabels(test.inMatchers)

		sort.Slice(test.outMatchers, func(i, j int) bool { return test.outMatchers[i].Name < test.outMatchers[j].Name })
		sort.Slice(matchers, func(i, j int) bool { return matchers[i].Name < matchers[j].Name })

		if !reflect.DeepEqual(matchers, test.outMatchers) {
			t.Fatalf("%d. unexpected matchers; want %v, got %v", i, test.outMatchers, matchers)
		}
		if !reflect.DeepEqual(added, test.added) {
			t.Fatalf("%d. unexpected added labels; want %v, got %v", i, test.added, added)
		}
	}
}

func TestSeriesSetFilter(t *testing.T) {
	tests := []struct {
		in       *prompb.QueryResult
		toRemove labels.Labels

		expected *prompb.QueryResult
	}{
		{
			toRemove: labels.Labels{{Name: "foo", Value: "bar"}},
			in: &prompb.QueryResult{
				Timeseries: []*prompb.TimeSeries{
					{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar", "a", "b"), nil), Samples: []prompb.Sample{}},
				},
			},
			expected: &prompb.QueryResult{
				Timeseries: []*prompb.TimeSeries{
					{Labels: labelsToLabelsProto(labels.FromStrings("a", "b"), nil), Samples: []prompb.Sample{}},
				},
			},
		},
	}

	for _, tc := range tests {
		filtered := newSeriesSetFilter(FromQueryResult(true, tc.in), tc.toRemove)
		act, ws, err := ToQueryResult(filtered, 1e6)
		testutil.Ok(t, err)
		testutil.Equals(t, 0, len(ws))
		testutil.Equals(t, tc.expected, act)
	}
}

type mockedRemoteClient struct {
	got   *prompb.Query
	store []*prompb.TimeSeries
}

func (c *mockedRemoteClient) Read(_ context.Context, query *prompb.Query) (*prompb.QueryResult, error) {
	if c.got != nil {
		return nil, errors.Errorf("expected only one call to remote client got: %v", query)
	}
	c.got = query

	matchers, err := FromLabelMatchers(query.Matchers)
	if err != nil {
		return nil, err
	}

	q := &prompb.QueryResult{}
	for _, s := range c.store {
		l := labelProtosToLabels(s.Labels)
		var notMatch bool

		for _, m := range matchers {
			if v := l.Get(m.Name); v != "" {
				if !m.Matches(v) {
					notMatch = true
					break
				}
			}
		}

		if !notMatch {
			q.Timeseries = append(q.Timeseries, &prompb.TimeSeries{Labels: s.Labels})
		}
	}
	return q, nil
}

func (c *mockedRemoteClient) reset() {
	c.got = nil
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
			readRecent: true,
			externalLabels: labels.Labels{
				{Name: "region", Value: "europe"},
			},

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
			readRecent: true,
			externalLabels: labels.Labels{
				{Name: "region", Value: "europe"},
			},

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
			readRecent: true,
			externalLabels: labels.Labels{
				{Name: "region", Value: "europe"},
			},

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
			q, err := c.Querier(context.TODO(), tc.mint, tc.maxt)
			testutil.Ok(t, err)
			defer testutil.Ok(t, q.Close())

			ss := q.Select(true, nil, tc.matchers...)
			testutil.Ok(t, err)
			testutil.Equals(t, storage.Warnings(nil), ss.Warnings())

			testutil.Equals(t, tc.expectedQuery, m.got)

			var got []labels.Labels
			for ss.Next() {
				got = append(got, ss.At().Labels())
			}
			testutil.Ok(t, ss.Err())
			testutil.Equals(t, tc.expectedSeries, got)

		})
	}
}
