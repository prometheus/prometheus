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
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
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

func TestAddExternalLabels(t *testing.T) {
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
		q := querier{
			externalLabels: test.el,
		}

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

type mockMergeQuerier struct{ queriersCount int }

func (*mockMergeQuerier) Select(...*labels.Matcher) storage.SeriesSet { return nil }
func (*mockMergeQuerier) LabelValues(name string) ([]string, error)   { return nil, nil }
func (*mockMergeQuerier) Close() error                                { return nil }

func TestRemoteStorageQuerier(t *testing.T) {
	tests := []struct {
		localStartTime        int64
		readRecentClients     []bool
		mint                  int64
		maxt                  int64
		expectedQueriersCount int
	}{
		{
			localStartTime:    int64(20),
			readRecentClients: []bool{true, true, false},
			mint:              int64(0),
			maxt:              int64(50),
			expectedQueriersCount: 3,
		},
		{
			localStartTime:    int64(20),
			readRecentClients: []bool{true, true, false},
			mint:              int64(30),
			maxt:              int64(50),
			expectedQueriersCount: 2,
		},
	}

	for i, test := range tests {
		s := NewStorage(nil, func() (int64, error) { return test.localStartTime, nil })
		s.clients = []*Client{}
		for _, readRecent := range test.readRecentClients {
			c, _ := NewClient(0, &ClientConfig{
				URL:              nil,
				Timeout:          model.Duration(30 * time.Second),
				HTTPClientConfig: config.HTTPClientConfig{},
				ReadRecent:       readRecent,
			})
			s.clients = append(s.clients, c)
		}
		// overrides mergeQuerier to mockMergeQuerier so we can reflect its type
		newMergeQueriers = func(queriers []storage.Querier) storage.Querier {
			return &mockMergeQuerier{queriersCount: len(queriers)}
		}

		querier, _ := s.Querier(context.Background(), test.mint, test.maxt)
		actualQueriersCount := reflect.ValueOf(querier).Interface().(*mockMergeQuerier).queriersCount

		if !reflect.DeepEqual(actualQueriersCount, test.expectedQueriersCount) {
			t.Fatalf("%d. unexpected queriers count; want %v, got %v", i, test.expectedQueriersCount, actualQueriersCount)
		}
	}
}
