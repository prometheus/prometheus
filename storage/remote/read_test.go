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
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
)

func mustNewLabelMatcher(mt labels.MatchType, name, val string) *labels.Matcher {
	m, err := labels.NewMatcher(mt, name, val)
	if err != nil {
		panic(err)
	}
	return m
}

func TestValidateLabelsAndMetricName(t *testing.T) {
	tests := []struct {
		input       prompb.TimeSeries
		expectedErr string
		shouldPass  bool
	}{
		{
			input: prompb.TimeSeries{
				Labels: []*prompb.Label{
					&prompb.Label{
						Name:  "__name__",
						Value: "name",
					},
					&prompb.Label{
						Name:  "testLabel",
						Value: "value0",
					},
				},
				Samples: []*prompb.Sample{},
			},
			expectedErr: "",
			shouldPass:  true,
		},
		{
			input: prompb.TimeSeries{
				Labels: []*prompb.Label{
					&prompb.Label{
						Name:  "__name__",
						Value: "name",
					},
					&prompb.Label{
						Name:  "@labelName",
						Value: "labelValue",
					},
				},
				Samples: []*prompb.Sample{},
			},
			expectedErr: "Invalid label name: @labelName",
			shouldPass:  false,
		},
		{
			input: prompb.TimeSeries{
				Labels: []*prompb.Label{
					&prompb.Label{
						Name:  "__name__",
						Value: "name",
					},
					&prompb.Label{
						Name:  "123labelName",
						Value: "labelValue",
					},
				},
				Samples: []*prompb.Sample{},
			},
			expectedErr: "Invalid label name: 123labelName",
			shouldPass:  false,
		},
		{
			input: prompb.TimeSeries{
				Labels: []*prompb.Label{
					&prompb.Label{
						Name:  "__name__",
						Value: "name",
					},
					&prompb.Label{
						Name:  "",
						Value: "labelValue",
					},
				},
				Samples: []*prompb.Sample{},
			},
			expectedErr: "Invalid label name: ",
			shouldPass:  false,
		},
		{
			input: prompb.TimeSeries{
				Labels: []*prompb.Label{
					&prompb.Label{
						Name:  "__name__",
						Value: "name",
					},
					&prompb.Label{
						Name:  "labelName",
						Value: string([]byte{0xff}),
					},
				},
				Samples: []*prompb.Sample{},
			},
			expectedErr: "Invalid label value: " + string([]byte{0xff}),
			shouldPass:  false,
		},
	}

	for _, test := range tests {
		err := validateLabels(test.input)
		if test.shouldPass {
			if err != nil {
				t.Fatalf("Test should pass, got unexpected error: %v", err)
			}
			continue
		}
		if err != nil {
			if err.Error() != test.expectedErr {
				t.Fatalf("Unexpected error, got: %v, expected: %v", err, test.expectedErr)
			}
		} else {
			t.Fatalf("Expected error %v, got none", test.expectedErr)
		}
	}
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
