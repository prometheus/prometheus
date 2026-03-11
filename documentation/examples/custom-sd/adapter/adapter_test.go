// Copyright The Prometheus Authors
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
	"context"
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery"
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
			expectedCustomSD: map[string]*customSD{
				"customSD:Consul:0000000000000000": {
					Targets: []string{},
					Labels:  map[string]string{},
				},
				"customSD:Kubernetes:0000000000000000": {
					Targets: []string{},
					Labels:  map[string]string{},
				},
			},
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
				"customSD:Azure:282a007a18fadbbb": {
					Targets: []string{
						"host1",
						"host2",
					},
					Labels: map[string]string{
						"__meta_test_label": "label_test_1",
					},
				},
				"customSD:Openshift:281c007a18ea2ad0": {
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
				"customSD:GCE:282a007a18fadbbb": {
					Targets: []string{
						"host1",
						"host2",
					},
					Labels: map[string]string{
						"__meta_test_label": "label_test_1",
					},
				},
				"customSD:Kubernetes:282e007a18fad483": {
					Targets: []string{},
					Labels: map[string]string{
						"__meta_test_label": "label_test_2",
					},
				},
			},
		},
		{
			title: "Disordered Ips in Alibaba's application management system",
			targetGroup: map[string][]*targetgroup.Group{
				"cart": {
					{
						Source: "alibaba",
						Targets: []model.LabelSet{
							{
								model.AddressLabel: "192.168.1.55",
							},
							{
								model.AddressLabel: "192.168.1.44",
							},
						},
						Labels: model.LabelSet{
							model.LabelName("__meta_test_label"): model.LabelValue("label_test_1"),
						},
					},
				},
				"buy": {
					{
						Source: "alibaba",
						Targets: []model.LabelSet{
							{
								model.AddressLabel: "192.168.1.22",
							},
							{
								model.AddressLabel: "192.168.1.33",
							},
						},
						Labels: model.LabelSet{
							model.LabelName("__meta_test_label"): model.LabelValue("label_test_1"),
						},
					},
				},
			},
			expectedCustomSD: map[string]*customSD{
				"buy:alibaba:21c0d97a1e27e6fe": {
					Targets: []string{
						"192.168.1.22",
						"192.168.1.33",
					},
					Labels: map[string]string{
						"__meta_test_label": "label_test_1",
					},
				},
				"cart:alibaba:1112e97a13b159fa": {
					Targets: []string{
						"192.168.1.44",
						"192.168.1.55",
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
		require.Equal(t, testCase.expectedCustomSD, result)
	}
}

// TestWriteOutput checks the adapter can write a file to disk.
func TestWriteOutput(t *testing.T) {
	ctx := context.Background()
	tmpfile, err := os.CreateTemp("", "sd_adapter_test")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()
	require.NoError(t, err)

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	sdMetrics, err := discovery.RegisterSDMetrics(reg, refreshMetrics)
	require.NoError(t, err)

	adapter := NewAdapter(ctx, tmpfile.Name(), "test_sd", nil, nil, sdMetrics, reg)
	require.NoError(t, adapter.writeOutput())
}
