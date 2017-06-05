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
	"reflect"
	"sort"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/metric"
)

func mustNewLabelMatcher(mt metric.MatchType, name model.LabelName, val model.LabelValue) *metric.LabelMatcher {
	m, err := metric.NewLabelMatcher(mt, name, val)
	if err != nil {
		panic(err)
	}
	return m
}

func TestAddExternalLabels(t *testing.T) {
	tests := []struct {
		el          model.LabelSet
		inMatchers  metric.LabelMatchers
		outMatchers metric.LabelMatchers
		added       model.LabelSet
	}{
		{
			el: model.LabelSet{},
			inMatchers: metric.LabelMatchers{
				mustNewLabelMatcher(metric.Equal, "job", "api-server"),
			},
			outMatchers: metric.LabelMatchers{
				mustNewLabelMatcher(metric.Equal, "job", "api-server"),
			},
			added: model.LabelSet{},
		},
		{
			el: model.LabelSet{"region": "europe", "dc": "berlin-01"},
			inMatchers: metric.LabelMatchers{
				mustNewLabelMatcher(metric.Equal, "job", "api-server"),
			},
			outMatchers: metric.LabelMatchers{
				mustNewLabelMatcher(metric.Equal, "job", "api-server"),
				mustNewLabelMatcher(metric.Equal, "region", "europe"),
				mustNewLabelMatcher(metric.Equal, "dc", "berlin-01"),
			},
			added: model.LabelSet{"region": "europe", "dc": "berlin-01"},
		},
		{
			el: model.LabelSet{"region": "europe", "dc": "berlin-01"},
			inMatchers: metric.LabelMatchers{
				mustNewLabelMatcher(metric.Equal, "job", "api-server"),
				mustNewLabelMatcher(metric.Equal, "dc", "munich-02"),
			},
			outMatchers: metric.LabelMatchers{
				mustNewLabelMatcher(metric.Equal, "job", "api-server"),
				mustNewLabelMatcher(metric.Equal, "region", "europe"),
				mustNewLabelMatcher(metric.Equal, "dc", "munich-02"),
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
