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

package alibabacloud

import (
	"net/http"
	"strconv"
	"testing"

	ecs20140526 "github.com/alibabacloud-go/ecs-20140526/v7/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

func TestECSSDConfigName(t *testing.T) {
	conf := DefaultECSSDConfig
	require.Equal(t, "alibabacloud_ecs", conf.Name())
}

func TestECSSDConfigUnmarshalYAML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		yaml         string
		wantErr      bool
		validateFunc func(t *testing.T, cfg *ECSSDConfig)
	}{
		{
			name: "WithFlatFields",
			yaml: `region: cn-beijing
endpoint: ecs.aliyuncs.com
port: 9100
tags:
  - key: ack.alibabacloud.com
    value: cd386715790e44917bxxxxxxxb7e782e2`,
			validateFunc: func(t *testing.T, cfg *ECSSDConfig) {
				require.NotNil(t, cfg)
				require.Equal(t, "cn-beijing", cfg.Region)
				require.Equal(t, "ecs.aliyuncs.com", cfg.Endpoint)
				require.Equal(t, 9100, cfg.Port)
				require.Len(t, cfg.Tags, 1)
				require.Equal(t, "ack.alibabacloud.com", cfg.Tags[0].Key)
				require.Equal(t, "cd386715790e44917bxxxxxxxb7e782e2", cfg.Tags[0].Value)
			},
		},
		{
			name: "WithCredentialFields",
			yaml: `region: cn-beijing
port: 9100
credential:
  type: access_key
  access_key_id: access-key-id
  access_key_secret: access-key-secret`,
			validateFunc: func(t *testing.T, cfg *ECSSDConfig) {
				require.NotNil(t, cfg)
				require.Equal(t, "cn-beijing", cfg.Region)
				require.Equal(t, 9100, cfg.Port)
				require.NotNil(t, cfg.CredentialConfig)
				require.NotNil(t, cfg.CredentialConfig.Type)
				require.Equal(t, CredentialTypeAccessKey, *cfg.CredentialConfig.Type)
				require.NotNil(t, cfg.CredentialConfig.AccessKeyId)
				require.Equal(t, config.Secret("access-key-id"), *cfg.CredentialConfig.AccessKeyId)
				require.NotNil(t, cfg.CredentialConfig.AccessKeySecret)
				require.Equal(t, config.Secret("access-key-secret"), *cfg.CredentialConfig.AccessKeySecret)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg ECSSDConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &cfg)
			if tt.wantErr {
				require.Error(t, err, "expected error for yaml: %q", tt.yaml)
				return
			}
			require.NoError(t, err)
			tt.validateFunc(t, &cfg)
		})
	}
}

func TestNewECSDiscovery(t *testing.T) {
	t.Parallel()

	refreshMetrics := discovery.NewRefreshMetrics(prometheus.NewRegistry())
	ecsMetrics := DefaultECSSDConfig.NewDiscovererMetrics(nil, refreshMetrics)

	tests := []struct {
		name    string
		conf    *ECSSDConfig
		opts    discovery.DiscovererOptions
		wantErr bool
	}{
		{
			name: "InvalidDiscoveryMetrics",
			conf: &ECSSDConfig{
				Region: "cn-beijing",
			},
			wantErr: true,
		},
		{
			name: "ValidDiscoveryMetrics",
			conf: &ECSSDConfig{
				Region: "cn-beijing",
			},
			opts: discovery.DiscovererOptions{
				Metrics: ecsMetrics,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			discoverer, err := NewECSDiscovery(tt.conf, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, discoverer)
		})
	}
}

type mockECSClient struct {
	ecsDataStore      []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance
	defaultMaxResults int
}

var _ ecsClient = &mockECSClient{}

func (c *mockECSClient) DescribeInstances(req *ecs20140526.DescribeInstancesRequest) (*ecs20140526.DescribeInstancesResponse, error) {
	if c.ecsDataStore == nil {
		c.ecsDataStore = []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance{}
	}

	nextTokenStr := req.NextToken
	nextToken := 0
	var err error
	if nextTokenStr != nil && *nextTokenStr != "" {
		nextToken, err = strconv.Atoi(*nextTokenStr)
		if err != nil {
			return nil, err
		}
	}
	if nextToken > len(c.ecsDataStore) {
		nextToken = len(c.ecsDataStore)
	}

	maxResults := c.defaultMaxResults
	maxResultsPtr := req.MaxResults
	if maxResultsPtr != nil {
		maxResults = int(*maxResultsPtr)
	}
	end := min(nextToken+maxResults, len(c.ecsDataStore))

	instances := c.ecsDataStore[nextToken:end]
	return &ecs20140526.DescribeInstancesResponse{
		StatusCode: new(int32(http.StatusOK)),
		Body: &ecs20140526.DescribeInstancesResponseBody{
			Instances: &ecs20140526.DescribeInstancesResponseBodyInstances{
				Instance: instances,
			},
			TotalCount: new(int32(len(c.ecsDataStore))),
			NextToken:  new(strconv.Itoa(end)),
		},
	}, nil
}

func TestDescribeInstancesPaginatorHasMorePages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		instances []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance
		expected  bool
	}{
		{
			name:      "NoMorePage",
			instances: []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance{},
			expected:  false,
		},
		{
			name: "HasMorePage",
			instances: []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance{
				{}, {}, {},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ecs20140526.DescribeInstancesRequest{}
			ecsClient := mockECSClient{
				ecsDataStore:      tt.instances,
				defaultMaxResults: 1,
			}
			paginator := newDescribeInstancesPaginator(&ecsClient, &req)

			// It is determined whether there is a next page by checking
			// if the "nextToken" field is the same in the two responses,
			// so at least call nextPage twice to determine if there are more pages
			_, err := paginator.nextPage()
			require.NoError(t, err)
			_, err = paginator.nextPage()
			require.NoError(t, err)
			hasMore := paginator.hasMorePages()
			require.Equal(t, tt.expected, hasMore)
		})
	}
}

func TestDescribeInstancesPaginatorNextPage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		instances  []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance
		maxResults int
		expected   []int
	}{
		{
			name:       "NoPage",
			instances:  []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance{},
			maxResults: 2,
			expected:   []int{},
		},
		{
			name: "TwoPages",
			instances: []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance{
				{}, {}, {},
			},
			maxResults: 2,
			expected:   []int{2, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ecsClient := mockECSClient{
				ecsDataStore:      tt.instances,
				defaultMaxResults: tt.maxResults,
			}
			paginator := newDescribeInstancesPaginator(&ecsClient, nil)

			actual := []int{}
			for paginator.hasMorePages() {
				resp, err := paginator.nextPage()
				require.NoError(t, err)
				count := len(resp.Body.Instances.Instance)
				if count == 0 {
					continue
				}
				actual = append(actual, count)
			}
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestECSDiscoveryRefresh(t *testing.T) {
	t.Parallel()
	region := "cn-beijing"
	refreshMetrics := discovery.NewRefreshMetrics(prometheus.NewRegistry())
	ecsMetrics := DefaultECSSDConfig.NewDiscovererMetrics(nil, refreshMetrics)
	discovererOptions := discovery.DiscovererOptions{Metrics: ecsMetrics}
	ecsSDConfig := ECSSDConfig{
		Port:   9100,
		Region: region,
	}
	ecsDiscovery, err := NewECSDiscovery(&ecsSDConfig, discovererOptions)
	require.NoError(t, err)
	require.NotNil(t, ecsDiscovery)

	tests := []struct {
		name       string
		instances  []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance
		maxResults int
		expected   []*targetgroup.Group
	}{
		{
			name:       "NoPages",
			instances:  []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance{},
			maxResults: 2,
			expected: []*targetgroup.Group{
				{
					Source: region,
				},
			},
		},
		{
			name: "TwoPages",
			instances: []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance{
				{
					VpcAttributes: &ecs20140526.DescribeInstancesResponseBodyInstancesInstanceVpcAttributes{
						PrivateIpAddress: &ecs20140526.DescribeInstancesResponseBodyInstancesInstanceVpcAttributesPrivateIpAddress{
							IpAddress: []*string{
								new("192.168.0.100"),
								new("192.168.0.101"),
							},
						},
					},
					EipAddress: &ecs20140526.DescribeInstancesResponseBodyInstancesInstanceEipAddress{
						IpAddress: new("192.168.0.102"),
					},
					InnerIpAddress: &ecs20140526.DescribeInstancesResponseBodyInstancesInstanceInnerIpAddress{
						IpAddress: []*string{
							new("192.168.0.103"),
							new("192.168.0.104"),
						},
					},
					PublicIpAddress: &ecs20140526.DescribeInstancesResponseBodyInstancesInstancePublicIpAddress{
						IpAddress: []*string{
							new("192.168.0.105"),
							new("192.168.0.106"),
							new("192.168.0.107"),
						},
					},
					InstanceId:          new("i-wz9alwxxxxxxxq2o6kyf"),
					InstanceName:        new("instance-name"),
					OSType:              new("linux"),
					InstanceType:        new("ecs.g7.xlarge"),
					InstanceTypeFamily:  new("ecs.g7"),
					ImageId:             new("image.vhd"),
					ZoneId:              new("cn-beijing-d"),
					Status:              new("Running"),
					InstanceNetworkType: new("vpc"),
					NetworkInterfaces: &ecs20140526.DescribeInstancesResponseBodyInstancesInstanceNetworkInterfaces{
						NetworkInterface: []*ecs20140526.DescribeInstancesResponseBodyInstancesInstanceNetworkInterfacesNetworkInterface{
							{
								Type:             new("Primary"),
								PrimaryIpAddress: new("192.168.0.108"),
							},
							{
								Type:             new("Trunk"),
								PrimaryIpAddress: new("192.168.0.109"),
							},
						},
					},
					Tags: &ecs20140526.DescribeInstancesResponseBodyInstancesInstanceTags{
						Tag: []*ecs20140526.DescribeInstancesResponseBodyInstancesInstanceTagsTag{
							{
								TagKey:   new("ack.alibabacloud.com/nodepool-id"),
								TagValue: new("npaaexxxxxxxad4194aaa5f15c8f672216"),
							},
							{
								TagKey:   new("ack.alibabacloud.com"),
								TagValue: new("cd386xxxxxxx44917b65842f9b7e782e2"),
							},
						},
					},
				},
			},
			maxResults: 2,
			expected: []*targetgroup.Group{
				{
					Source: region,
					Targets: []model.LabelSet{
						{
							ecsLabelRegion:                 model.LabelValue(region),
							ecsLabelVPCPrivateIP:           model.LabelValue("192.168.0.100,192.168.0.101"),
							ecsLabelEIP:                    model.LabelValue("192.168.0.102"),
							ecsLabelInnerIP:                model.LabelValue("192.168.0.103,192.168.0.104"),
							ecsLabelPublicIP:               model.LabelValue("192.168.0.105,192.168.0.106,192.168.0.107"),
							ecsLabelECSInstanceID:          model.LabelValue("i-wz9alwxxxxxxxq2o6kyf"),
							ecsLabelECSInstanceName:        model.LabelValue("instance-name"),
							ecsLabelOSType:                 model.LabelValue("linux"),
							ecsLabelECSInstanceType:        model.LabelValue("ecs.g7.xlarge"),
							ecsLabelECSInstanceTypeFamily:  model.LabelValue("ecs.g7"),
							ecsLabelECSImageID:             model.LabelValue("image.vhd"),
							ecsLabelZone:                   model.LabelValue("cn-beijing-d"),
							ecsLabelECSInstanceStatus:      model.LabelValue("Running"),
							ecsLabelECSInstanceNetworkType: model.LabelValue("vpc"),
							model.AddressLabel:             model.LabelValue("192.168.0.108:9100"),
							ecsLabelTag + "ack_alibabacloud_com_nodepool_id": model.LabelValue("npaaexxxxxxxad4194aaa5f15c8f672216"),
							ecsLabelTag + "ack_alibabacloud_com":             model.LabelValue("cd386xxxxxxx44917b65842f9b7e782e2"),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ecsDiscovery.ecs = &mockECSClient{
				ecsDataStore:      tt.instances,
				defaultMaxResults: 2,
			}
			targetGroups, err := ecsDiscovery.refresh(t.Context())
			require.NoError(t, err)
			require.Equal(t, tt.expected, targetGroups)
		})
	}
}
