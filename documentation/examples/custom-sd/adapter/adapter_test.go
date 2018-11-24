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

package adapter

import (
	"reflect"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// TestGenerateTargetGroups checks that the target is correctly generated.
// It covers the case when the target is empty.
func TestGenerateTargetGroups(t *testing.T) {
	testCases := []struct {
		title            string
		targetGroup      map[string][]*targetgroup.Group
		expectedCustomSD map[string]*customSD
	}{
		{
			title: "Empty targetGroup",
			targetGroup: map[string][]*targetgroup.Group{
				"customSD": {
					{
						Source: "Consul",
					},
					{
						Source: "Kubernetes",
					},
				},
			},
			expectedCustomSD: map[string]*customSD{},
		},
		{
			title: "targetGroup filled",
			targetGroup: map[string][]*targetgroup.Group{
				"customSD": {
					{
						Source: "Azure",
						Targets: []model.LabelSet{
							{
								model.AddressLabel: "host1",
							},
							{
								model.AddressLabel: "host2",
							},
						},
						Labels: model.LabelSet{
							model.LabelName("__meta_test_label"): model.LabelValue("label_test_1"),
						},
					},
					{
						Source: "Openshift",
						Targets: []model.LabelSet{
							{
								model.AddressLabel: "host3",
							},
							{
								model.AddressLabel: "host4",
							},
						},
						Labels: model.LabelSet{
							model.LabelName("__meta_test_label"): model.LabelValue("label_test_2"),
						},
					},
				},
			},
			expectedCustomSD: map[string]*customSD{
				"customSD:Azure:0": {
					Targets: []string{
						"host1",
						"host2",
					},
					Labels: map[string]string{
						"__meta_test_label": "label_test_1",
					},
				},
				"customSD:Openshift:1": {
					Targets: []string{
						"host3",
						"host4",
					},
					Labels: map[string]string{
						"__meta_test_label": "label_test_2",
					},
				},
			},
		},
		{
			title: "Mixed between empty targetGroup and targetGroup filled",
			targetGroup: map[string][]*targetgroup.Group{
				"customSD": {
					{
						Source: "GCE",
						Targets: []model.LabelSet{
							{
								model.AddressLabel: "host1",
							},
							{
								model.AddressLabel: "host2",
							},
						},
						Labels: model.LabelSet{
							model.LabelName("__meta_test_label"): model.LabelValue("label_test_1"),
						},
					},
					{
						Source: "Kubernetes",
						Labels: model.LabelSet{
							model.LabelName("__meta_test_label"): model.LabelValue("label_test_2"),
						},
					},
				},
			},
			expectedCustomSD: map[string]*customSD{
				"customSD:GCE:0": {
					Targets: []string{
						"host1",
						"host2",
					},
					Labels: map[string]string{
						"__meta_test_label": "label_test_1",
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		result := generateTargetGroups(testCase.targetGroup)

		if !reflect.DeepEqual(result, testCase.expectedCustomSD) {
			t.Errorf("%q failed\ngot: %#v\nexpected: %v",
				testCase.title,
				result,
				testCase.expectedCustomSD)
		}

	}
}
