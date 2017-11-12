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
	if have := q.Select(matchers...); !reflect.DeepEqual(want, have) {
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
					{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar", "a", "b")), Samples: []*prompb.Sample{}},
				},
			},
			expected: &prompb.QueryResult{
				Timeseries: []*prompb.TimeSeries{
					{Labels: labelsToLabelsProto(labels.FromStrings("a", "b")), Samples: []*prompb.Sample{}},
				},
			},
		},
	}

	for i, tc := range tests {
		filtered := newSeriesSetFilter(FromQueryResult(tc.in), tc.toRemove)
		have, err := ToQueryResult(filtered)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(have, tc.expected) {
			t.Fatalf("%d. unexpected labels; want %v, got %v", i, tc.expected, have)
		}
	}
}

type testQuerier struct {
	ctx        context.Context
	mint, maxt int64

	storage.Querier
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
			querier:        testQuerier{ctx: ctx, mint: 0, maxt: 50},
		},
		{
			localStartTime: int64(20),
			mint:           int64(0),
			maxt:           int64(50),
			querier:        testQuerier{ctx: ctx, mint: 0, maxt: 20},
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
				return testQuerier{ctx: ctx, mint: mint, maxt: maxt}, nil
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
