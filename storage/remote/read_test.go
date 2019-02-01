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
	"reflect"
	"sort"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
)

func mustNewLabelMatcher(mt labels.MatchType, name, val string) *labels.Matcher {
	m, err := labels.NewMatcher(mt, name, val)
	if err != nil {
		panic(err)
	}
	return m
}

func TestExternalLabelsQuerierSelect(t *testing.T) {
	matchers := []*labels.Matcher{
		mustNewLabelMatcher(labels.MatchEqual, "job", "api-server"),
	}
	q := &externalLabelsQuerier{
		Querier:        mockQuerier{},
		externalLabels: model.LabelSet{"region": "europe"},
	}
	want := newSeriesSetFilter(mockSeriesSet{}, q.externalLabels)
	have, _, err := q.Select(nil, matchers...)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(want, have) {
		t.Errorf("expected series set %+v, got %+v", want, have)
	}
}

func TestExternalLabelsQuerierAddExternalLabels(t *testing.T) {
	tests := []struct {
		el          model.LabelSet
		inMatchers  []*labels.Matcher
		outMatchers []*labels.Matcher
		added       model.LabelSet
	}{
		{
			el: model.LabelSet{},
			inMatchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "job", "api-server"),
			},
			outMatchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "job", "api-server"),
			},
			added: model.LabelSet{},
		},
		{
			el: model.LabelSet{"region": "europe", "dc": "berlin-01"},
			inMatchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "job", "api-server"),
			},
			outMatchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "job", "api-server"),
				mustNewLabelMatcher(labels.MatchEqual, "region", "europe"),
				mustNewLabelMatcher(labels.MatchEqual, "dc", "berlin-01"),
			},
			added: model.LabelSet{"region": "europe", "dc": "berlin-01"},
		},
		{
			el: model.LabelSet{"region": "europe", "dc": "berlin-01"},
			inMatchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "job", "api-server"),
				mustNewLabelMatcher(labels.MatchEqual, "dc", "munich-02"),
			},
			outMatchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "job", "api-server"),
				mustNewLabelMatcher(labels.MatchEqual, "region", "europe"),
				mustNewLabelMatcher(labels.MatchEqual, "dc", "munich-02"),
			},
			added: model.LabelSet{"region": "europe"},
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

func TestExternalLabelsQuerierLabelValues(t *testing.T) {
	q := &externalLabelsQuerier{
		Querier:        mockQuerier{},
		externalLabels: model.LabelSet{"region": "europe"},
	}
	want := []string{"foo", "bar"}
	values, _, err := q.LabelValues("test")
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(want, values) {
		t.Fatalf("unexpected label values; want %v, got %v", want, values)
	}
}

func TestExternalLabelsQuerierLabelNames(t *testing.T) {
	q := &externalLabelsQuerier{
		Querier:        mockQuerier{},
		externalLabels: model.LabelSet{"region": "europe"},
	}
	want := []string{"foo", "bar"}
	names, _, err := q.LabelNames()
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(want, names) {
		t.Fatalf("unexpected label names; want %v, got %v", want, names)
	}
}

func TestSeriesSetFilter(t *testing.T) {
	tests := []struct {
		in       *prompb.QueryResult
		toRemove model.LabelSet

		expected *prompb.QueryResult
	}{
		{
			toRemove: model.LabelSet{"foo": "bar"},
			in: &prompb.QueryResult{
				Timeseries: []*prompb.TimeSeries{
					{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar", "a", "b")), Samples: []prompb.Sample{}},
				},
			},
			expected: &prompb.QueryResult{
				Timeseries: []*prompb.TimeSeries{
					{Labels: labelsToLabelsProto(labels.FromStrings("a", "b")), Samples: []prompb.Sample{}},
				},
			},
		},
	}

	for i, tc := range tests {
		filtered := newSeriesSetFilter(FromQueryResult(tc.in), tc.toRemove)
		have, err := ToQueryResult(filtered, 1e6)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(have, tc.expected) {
			t.Fatalf("%d. unexpected labels; want %v, got %v", i, tc.expected, have)
		}
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

func (mockQuerier) Select(*storage.SelectParams, ...*labels.Matcher) (storage.SeriesSet, storage.Warnings, error) {
	return mockSeriesSet{}, nil, nil
}

func (mockQuerier) LabelValues(name string) ([]string, storage.Warnings, error) {
	return []string{"foo", "bar"}, nil, nil
}

func (mockQuerier) LabelNames() ([]string, storage.Warnings, error) {
	return []string{"foo", "bar"}, nil, nil
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
			t.Errorf("%d. expected quierer %+v, got %+v", i, test.querier, q)
		}
	}
}

func TestRequiredMatchersFilter(t *testing.T) {
	ctx := context.Background()

	f := RequiredMatchersFilter(
		storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
			return mockQuerier{ctx: ctx, mint: mint, maxt: maxt}, nil
		}),
		[]*labels.Matcher{mustNewLabelMatcher(labels.MatchEqual, "special", "label")},
	)

	want := &requiredMatchersQuerier{
		Querier:          mockQuerier{ctx: ctx, mint: 0, maxt: 50},
		requiredMatchers: []*labels.Matcher{mustNewLabelMatcher(labels.MatchEqual, "special", "label")},
	}
	have, err := f.Querier(ctx, 0, 50)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(want, have) {
		t.Errorf("expected quierer %+v, got %+v", want, have)
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
				mustNewLabelMatcher(labels.MatchEqual, "special", "label"),
			},
			seriesSet: mockSeriesSet{},
		},
		{
			requiredMatchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "special", "label"),
			},
			matchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "special", "label"),
			},
			seriesSet: mockSeriesSet{},
		},
		{
			requiredMatchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "special", "label"),
			},
			matchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchRegexp, "special", "label"),
			},
			seriesSet: storage.NoopSeriesSet(),
		},
		{
			requiredMatchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "special", "label"),
			},
			matchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "special", "different"),
			},
			seriesSet: storage.NoopSeriesSet(),
		},
		{
			requiredMatchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "special", "label"),
			},
			matchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "special", "label"),
				mustNewLabelMatcher(labels.MatchEqual, "foo", "bar"),
			},
			seriesSet: mockSeriesSet{},
		},
		{
			requiredMatchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "special", "label"),
				mustNewLabelMatcher(labels.MatchEqual, "foo", "bar"),
			},
			matchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "special", "label"),
				mustNewLabelMatcher(labels.MatchEqual, "foo", "baz"),
			},
			seriesSet: storage.NoopSeriesSet(),
		},
		{
			requiredMatchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "special", "label"),
				mustNewLabelMatcher(labels.MatchEqual, "foo", "bar"),
			},
			matchers: []*labels.Matcher{
				mustNewLabelMatcher(labels.MatchEqual, "special", "label"),
				mustNewLabelMatcher(labels.MatchEqual, "foo", "bar"),
			},
			seriesSet: mockSeriesSet{},
		},
	}

	for i, test := range tests {
		q := &requiredMatchersQuerier{
			Querier:          mockQuerier{},
			requiredMatchers: test.requiredMatchers,
		}

		have, _, err := q.Select(nil, test.matchers...)
		if err != nil {
			t.Error(err)
		}
		if want := test.seriesSet; want != have {
			t.Errorf("%d. expected series set %+v, got %+v", i, want, have)
		}
		if want, have := test.requiredMatchers, q.requiredMatchers; !reflect.DeepEqual(want, have) {
			t.Errorf("%d. requiredMatchersQuerier.Select() has modified the matchers", i)
		}
	}
}

func TestDisabledQuerier(t *testing.T) {
	matchers := []*labels.Matcher{
		mustNewLabelMatcher(labels.MatchEqual, "job", "api-server"),
	}
	q := &disabledQuerier{mockQuerier{}}
	ss, _, err := q.Select(nil, matchers...)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(ss, mockSeriesSet{}) {
		t.Errorf("expected get %+v, got %+v", mockSeriesSet{}, ss)
	}

	values, _, err := q.LabelValues("test")
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(values, []string{}) {
		t.Errorf("expected get label values %+v, got %+v", []string{}, values)
	}
	names, _, err := q.LabelNames()
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(names, []string{}) {
		t.Errorf("expected get label names %+v, got %+v", []string{}, names)
	}
}

func TestGetLabelValues(t *testing.T) {

	tests := []struct {
		seriesSet []storage.Series
		name      string
		want      []string
	}{
		{
			[]storage.Series{
				&concreteSeries{
					labels:  labels.FromStrings("foo", "bar"),
					samples: []prompb.Sample{{Value: 1, Timestamp: 2}},
				},
				&concreteSeries{
					labels:  labels.FromStrings("foo", "baz"),
					samples: []prompb.Sample{{Value: 3, Timestamp: 4}},
				},
			},
			"foo",
			[]string{"bar", "baz"},
		},
		{
			[]storage.Series{
				&concreteSeries{
					labels:  labels.FromStrings("test1", "test"),
					samples: []prompb.Sample{{Value: 1, Timestamp: 2}},
				},
			},
			"test1",
			[]string{"test"},
		},
		{
			[]storage.Series{},
			"test",
			[]string{},
		},
	}
	for i, test := range tests {
		c := &concreteSeriesSet{series: test.seriesSet}
		got, err := getLabelValues(test.name, c)
		if err != nil {
			t.Errorf("Error getting label values for label %s", test.name)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%d. expected label values %+v, got %+v", i, test.want, got)
		}
	}
}

func TestGetLabelNames(t *testing.T) {
	tests := []struct {
		seriesSet []storage.Series
		want      []string
	}{
		{
			[]storage.Series{
				&concreteSeries{
					labels:  labels.FromStrings("foo", "bar"),
					samples: []prompb.Sample{{Value: 1, Timestamp: 2}},
				},
				&concreteSeries{
					labels:  labels.FromStrings("fiz", "baz"),
					samples: []prompb.Sample{{Value: 1, Timestamp: 2}},
				},
			},
			[]string{"fiz", "foo"},
		},
		{
			[]storage.Series{
				&concreteSeries{
					labels:  labels.FromStrings("test1", "test"),
					samples: []prompb.Sample{{Value: 1, Timestamp: 2}},
				},
			},
			[]string{"test1"},
		},
		{
			[]storage.Series{},
			[]string{},
		},
	}
	for i, test := range tests {
		c := &concreteSeriesSet{series: test.seriesSet}
		got, err := getLabelNames(c)
		if err != nil {
			t.Errorf("Error getting label names")
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%d. expected label names %+v, got %+v", i, test.want, got)
		}
	}
}
