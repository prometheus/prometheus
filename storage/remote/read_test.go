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
				Host:   "localhost",
			},
		},
	}
	cfg2 := config.RemoteReadConfig{
		Name: "write-2",
		URL: &config_util.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
	}
	cfg3 := config.RemoteReadConfig{
		URL: &config_util.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
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
		s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline)
		conf := &config.Config{
			GlobalConfig:      config.DefaultGlobalConfig,
			RemoteReadConfigs: tc.cfgs,
		}
		err := s.ApplyConfig(conf)
		gotError := err != nil
		testutil.Equals(t, tc.err, gotError)

		err = s.Close()
		testutil.Ok(t, err)
	}
}

func TestExternalLabelsQuerierSelect(t *testing.T) {
	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "job", "api-server"),
	}
	q := &externalLabelsQuerier{
		Querier: mockQuerier{},
		externalLabels: labels.Labels{
			{Name: "region", Value: "europe"},
		},
	}
	want := newSeriesSetFilter(mockSeriesSet{}, q.externalLabels)
	have := q.Select(false, nil, matchers...)

	if !reflect.DeepEqual(want, have) {
		t.Errorf("expected series set %+v, got %+v", want, have)
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
		q := &externalLabelsQuerier{Querier: mockQuerier{}, externalLabels: test.el}
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

type mockQuerier struct {
	ctx        context.Context
	mint, maxt int64

	storage.Querier
}

type mockSeriesSet struct {
	storage.SeriesSet
}

func (mockQuerier) Select(bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
	return mockSeriesSet{}
}

func TestPreferLocalStorageFilter(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		localStartTime int64
		mint           int64
		maxt           int64
		querier        storage.Querier
	}{
		{
			localStartTime: int64(100),
			mint:           int64(0),
			maxt:           int64(50),
			querier:        mockQuerier{ctx: ctx, mint: 0, maxt: 50},
		},
		{
			localStartTime: int64(20),
			mint:           int64(0),
			maxt:           int64(50),
			querier:        mockQuerier{ctx: ctx, mint: 0, maxt: 20},
		},
		{
			localStartTime: int64(20),
			mint:           int64(30),
			maxt:           int64(50),
			querier:        storage.NoopQuerier(),
		},
	}

	for i, test := range tests {
		f := PreferLocalStorageFilter(
			storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
				return mockQuerier{ctx: ctx, mint: mint, maxt: maxt}, nil
			}),
			func() (int64, error) { return test.localStartTime, nil },
		)

		q, err := f.Querier(ctx, test.mint, test.maxt)
		if err != nil {
			t.Fatal(err)
		}

		if test.querier != q {
			t.Errorf("%d. expected querier %+v, got %+v", i, test.querier, q)
		}
	}
}

func TestRequiredMatchersFilter(t *testing.T) {
	ctx := context.Background()

	f := RequiredMatchersFilter(
		storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
			return mockQuerier{ctx: ctx, mint: mint, maxt: maxt}, nil
		}),
		[]*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "special", "label")},
	)

	want := &requiredMatchersQuerier{
		Querier:          mockQuerier{ctx: ctx, mint: 0, maxt: 50},
		requiredMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "special", "label")},
	}
	have, err := f.Querier(ctx, 0, 50)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(want, have) {
		t.Errorf("expected querier %+v, got %+v", want, have)
	}
}

func TestRequiredLabelsQuerierSelect(t *testing.T) {
	tests := []struct {
		requiredMatchers []*labels.Matcher
		matchers         []*labels.Matcher
		seriesSet        storage.SeriesSet
	}{
		{
			requiredMatchers: []*labels.Matcher{},
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "special", "label"),
			},
			seriesSet: mockSeriesSet{},
		},
		{
			requiredMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "special", "label"),
			},
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "special", "label"),
			},
			seriesSet: mockSeriesSet{},
		},
		{
			requiredMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "special", "label"),
			},
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, "special", "label"),
			},
			seriesSet: storage.NoopSeriesSet(),
		},
		{
			requiredMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "special", "label"),
			},
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "special", "different"),
			},
			seriesSet: storage.NoopSeriesSet(),
		},
		{
			requiredMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "special", "label"),
			},
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "special", "label"),
				labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
			},
			seriesSet: mockSeriesSet{},
		},
		{
			requiredMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "special", "label"),
				labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
			},
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "special", "label"),
				labels.MustNewMatcher(labels.MatchEqual, "foo", "baz"),
			},
			seriesSet: storage.NoopSeriesSet(),
		},
		{
			requiredMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "special", "label"),
				labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
			},
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "special", "label"),
				labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
			},
			seriesSet: mockSeriesSet{},
		},
	}

	for i, test := range tests {
		q := &requiredMatchersQuerier{
			Querier:          mockQuerier{},
			requiredMatchers: test.requiredMatchers,
		}

		have := q.Select(false, nil, test.matchers...)

		if want := test.seriesSet; want != have {
			t.Errorf("%d. expected series set %+v, got %+v", i, want, have)
		}
		if want, have := test.requiredMatchers, q.requiredMatchers; !reflect.DeepEqual(want, have) {
			t.Errorf("%d. requiredMatchersQuerier.Select() has modified the matchers", i)
		}
	}
}
