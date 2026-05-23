// Copyright 2024 The Prometheus Authors
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

package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	lightsailTypes "github.com/aws/aws-sdk-go-v2/service/lightsail/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Struct for test data.
type lightsailDataStore struct {
	region    string
	instances []lightsailTypes.Instance
}

func TestLightsailDiscoveryRefresh(t *testing.T) {
	ctx := context.Background()

	// iterate through the test cases
	for _, tt := range []struct {
		name          string
		lightsailData *lightsailDataStore
		expected      []*targetgroup.Group
	}{
		{
			name: "NoPrivateIP",
			lightsailData: &lightsailDataStore{
				region: "us-east-1",
				instances: []lightsailTypes.Instance{
					{
						Name: strptr("instance-no-private-ip"),
						// No PrivateIpAddress - should be skipped
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "us-east-1",
					// Targets is nil (not empty slice) because no instances were added
					// This matches the behavior when all instances are skipped
				},
			},
		},
		{
			name: "BasicInstance",
			lightsailData: &lightsailDataStore{
				region: "us-west-2",
				instances: []lightsailTypes.Instance{
					{
						Name:             strptr("test-instance"),
						PrivateIpAddress: strptr("10.0.1.5"),
						PublicIpAddress:  strptr("203.0.113.10"),
						BlueprintId:      strptr("wordpress_5_8_0"),
						BundleId:         strptr("micro_2_0"),
						SupportCode:      strptr("support-code-123"),
						Location: &lightsailTypes.ResourceLocation{
							AvailabilityZone: strptr("us-west-2a"),
						},
						State: &lightsailTypes.InstanceState{
							Name: strptr("running"),
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "us-west-2",
					Targets: []model.LabelSet{
						{
							"__address__":                            model.LabelValue("10.0.1.5:80"),
							"__meta_lightsail_availability_zone":     model.LabelValue("us-west-2a"),
							"__meta_lightsail_blueprint_id":          model.LabelValue("wordpress_5_8_0"),
							"__meta_lightsail_bundle_id":             model.LabelValue("micro_2_0"),
							"__meta_lightsail_instance_name":         model.LabelValue("test-instance"),
							"__meta_lightsail_instance_state":        model.LabelValue("running"),
							"__meta_lightsail_instance_support_code": model.LabelValue("support-code-123"),
							"__meta_lightsail_private_ip":            model.LabelValue("10.0.1.5"),
							"__meta_lightsail_public_ip":             model.LabelValue("203.0.113.10"),
							"__meta_lightsail_region":                model.LabelValue("us-west-2"),
						},
					},
				},
			},
		},
		{
			name: "InstanceWithIPv6",
			lightsailData: &lightsailDataStore{
				region: "eu-west-1",
				instances: []lightsailTypes.Instance{
					{
						Name:             strptr("ipv6-instance"),
						PrivateIpAddress: strptr("10.0.2.10"),
						PublicIpAddress:  strptr("203.0.113.20"),
						BlueprintId:      strptr("ubuntu_20_04"),
						BundleId:         strptr("small_2_0"),
						SupportCode:      strptr("support-code-456"),
						Location: &lightsailTypes.ResourceLocation{
							AvailabilityZone: strptr("eu-west-1b"),
						},
						State: &lightsailTypes.InstanceState{
							Name: strptr("running"),
						},
						Ipv6Addresses: []string{
							"2001:db8::1",
							"2001:db8::2",
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "eu-west-1",
					Targets: []model.LabelSet{
						{
							"__address__":                            model.LabelValue("10.0.2.10:80"),
							"__meta_lightsail_availability_zone":     model.LabelValue("eu-west-1b"),
							"__meta_lightsail_blueprint_id":          model.LabelValue("ubuntu_20_04"),
							"__meta_lightsail_bundle_id":             model.LabelValue("small_2_0"),
							"__meta_lightsail_instance_name":         model.LabelValue("ipv6-instance"),
							"__meta_lightsail_instance_state":        model.LabelValue("running"),
							"__meta_lightsail_instance_support_code": model.LabelValue("support-code-456"),
							"__meta_lightsail_ipv6_addresses":        model.LabelValue(",2001:db8::1,2001:db8::2,"),
							"__meta_lightsail_private_ip":            model.LabelValue("10.0.2.10"),
							"__meta_lightsail_public_ip":             model.LabelValue("203.0.113.20"),
							"__meta_lightsail_region":                model.LabelValue("eu-west-1"),
						},
					},
				},
			},
		},
		{
			name: "InstanceWithTags",
			lightsailData: &lightsailDataStore{
				region: "ap-southeast-1",
				instances: []lightsailTypes.Instance{
					{
						Name:             strptr("tagged-instance"),
						PrivateIpAddress: strptr("10.0.3.15"),
						BlueprintId:      strptr("amazon_linux_2"),
						BundleId:         strptr("medium_2_0"),
						SupportCode:      strptr("support-code-789"),
						Location: &lightsailTypes.ResourceLocation{
							AvailabilityZone: strptr("ap-southeast-1a"),
						},
						State: &lightsailTypes.InstanceState{
							Name: strptr("running"),
						},
						Tags: []lightsailTypes.Tag{
							{Key: strptr("Environment"), Value: strptr("Production")},
							{Key: strptr("Team"), Value: strptr("Backend")},
							// Test tags with nil key or value - should be ignored
							{Key: nil, Value: strptr("ignored-value")},
							{Key: strptr("ignored-key"), Value: nil},
							{Key: nil, Value: nil},
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "ap-southeast-1",
					Targets: []model.LabelSet{
						{
							"__address__":                            model.LabelValue("10.0.3.15:80"),
							"__meta_lightsail_availability_zone":     model.LabelValue("ap-southeast-1a"),
							"__meta_lightsail_blueprint_id":          model.LabelValue("amazon_linux_2"),
							"__meta_lightsail_bundle_id":             model.LabelValue("medium_2_0"),
							"__meta_lightsail_instance_name":         model.LabelValue("tagged-instance"),
							"__meta_lightsail_instance_state":        model.LabelValue("running"),
							"__meta_lightsail_instance_support_code": model.LabelValue("support-code-789"),
							"__meta_lightsail_private_ip":            model.LabelValue("10.0.3.15"),
							"__meta_lightsail_region":                model.LabelValue("ap-southeast-1"),
							"__meta_lightsail_tag_Environment":       model.LabelValue("Production"),
							"__meta_lightsail_tag_Team":              model.LabelValue("Backend"),
						},
					},
				},
			},
		},
		{
			name: "MultipleInstances",
			lightsailData: &lightsailDataStore{
				region: "us-east-1",
				instances: []lightsailTypes.Instance{
					{
						Name:             strptr("instance-1"),
						PrivateIpAddress: strptr("10.0.4.1"),
						BlueprintId:      strptr("wordpress_5_8_0"),
						BundleId:         strptr("micro_2_0"),
						SupportCode:      strptr("support-1"),
						Location: &lightsailTypes.ResourceLocation{
							AvailabilityZone: strptr("us-east-1a"),
						},
						State: &lightsailTypes.InstanceState{
							Name: strptr("running"),
						},
					},
					{
						Name:             strptr("instance-2"),
						PrivateIpAddress: strptr("10.0.4.2"),
						PublicIpAddress:  strptr("203.0.113.30"),
						BlueprintId:      strptr("ubuntu_20_04"),
						BundleId:         strptr("small_2_0"),
						SupportCode:      strptr("support-2"),
						Location: &lightsailTypes.ResourceLocation{
							AvailabilityZone: strptr("us-east-1b"),
						},
						State: &lightsailTypes.InstanceState{
							Name: strptr("stopped"),
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "us-east-1",
					Targets: []model.LabelSet{
						{
							"__address__":                            model.LabelValue("10.0.4.1:80"),
							"__meta_lightsail_availability_zone":     model.LabelValue("us-east-1a"),
							"__meta_lightsail_blueprint_id":          model.LabelValue("wordpress_5_8_0"),
							"__meta_lightsail_bundle_id":             model.LabelValue("micro_2_0"),
							"__meta_lightsail_instance_name":         model.LabelValue("instance-1"),
							"__meta_lightsail_instance_state":        model.LabelValue("running"),
							"__meta_lightsail_instance_support_code": model.LabelValue("support-1"),
							"__meta_lightsail_private_ip":            model.LabelValue("10.0.4.1"),
							"__meta_lightsail_region":                model.LabelValue("us-east-1"),
						},
						{
							"__address__":                            model.LabelValue("10.0.4.2:80"),
							"__meta_lightsail_availability_zone":     model.LabelValue("us-east-1b"),
							"__meta_lightsail_blueprint_id":          model.LabelValue("ubuntu_20_04"),
							"__meta_lightsail_bundle_id":             model.LabelValue("small_2_0"),
							"__meta_lightsail_instance_name":         model.LabelValue("instance-2"),
							"__meta_lightsail_instance_state":        model.LabelValue("stopped"),
							"__meta_lightsail_instance_support_code": model.LabelValue("support-2"),
							"__meta_lightsail_private_ip":            model.LabelValue("10.0.4.2"),
							"__meta_lightsail_public_ip":             model.LabelValue("203.0.113.30"),
							"__meta_lightsail_region":                model.LabelValue("us-east-1"),
						},
					},
				},
			},
		},
		{
			name: "CustomPort",
			lightsailData: &lightsailDataStore{
				region: "us-west-1",
				instances: []lightsailTypes.Instance{
					{
						Name:             strptr("custom-port-instance"),
						PrivateIpAddress: strptr("10.0.5.5"),
						BlueprintId:      strptr("nginx"),
						BundleId:         strptr("nano_2_0"),
						SupportCode:      strptr("support-custom"),
						Location: &lightsailTypes.ResourceLocation{
							AvailabilityZone: strptr("us-west-1a"),
						},
						State: &lightsailTypes.InstanceState{
							Name: strptr("running"),
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "us-west-1",
					Targets: []model.LabelSet{
						{
							"__address__":                            model.LabelValue("10.0.5.5:9090"),
							"__meta_lightsail_availability_zone":     model.LabelValue("us-west-1a"),
							"__meta_lightsail_blueprint_id":          model.LabelValue("nginx"),
							"__meta_lightsail_bundle_id":             model.LabelValue("nano_2_0"),
							"__meta_lightsail_instance_name":         model.LabelValue("custom-port-instance"),
							"__meta_lightsail_instance_state":        model.LabelValue("running"),
							"__meta_lightsail_instance_support_code": model.LabelValue("support-custom"),
							"__meta_lightsail_private_ip":            model.LabelValue("10.0.5.5"),
							"__meta_lightsail_region":                model.LabelValue("us-west-1"),
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockLightsailClient(tt.lightsailData)

			// Use custom port for the CustomPort test case
			port := 80
			if tt.name == "CustomPort" {
				port = 9090
			}

			d := &LightsailDiscovery{
				cfg: &LightsailSDConfig{
					Port:   port,
					Region: client.lightsailData.region,
				},
				lightsail: client,
			}

			g, err := d.refresh(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.expected, g)
		})
	}
}

// Lightsail client mock.
type mockLightsailClient struct {
	lightsailData lightsailDataStore
}

func newMockLightsailClient(lightsailData *lightsailDataStore) *mockLightsailClient {
	client := mockLightsailClient{
		lightsailData: *lightsailData,
	}
	return &client
}

func (m *mockLightsailClient) GetInstances(_ context.Context, _ *lightsail.GetInstancesInput, _ ...func(*lightsail.Options)) (*lightsail.GetInstancesOutput, error) {
	return &lightsail.GetInstancesOutput{
		Instances: m.lightsailData.instances,
	}, nil
}
