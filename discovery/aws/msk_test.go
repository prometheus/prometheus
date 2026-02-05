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

package aws

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/aws/aws-sdk-go-v2/service/kafka/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Struct for test data.
type mskDataStore struct {
	region   string
	clusters []types.Cluster
	nodes    map[string][]types.NodeInfo // keyed by cluster ARN
}

func TestMSKDiscoveryListClusters(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name     string
		mskData  *mskDataStore
		expected []types.Cluster
	}{
		{
			name: "MultipleClusters",
			mskData: &mskDataStore{
				region: "us-west-2",
				clusters: []types.Cluster{
					{
						ClusterName: strptr("test-cluster"),
						ClusterArn:  strptr("arn:aws:kafka:us-west-2:123456789012:cluster/test-cluster/abc-123"),
						State:       types.ClusterStateActive,
						ClusterType: types.ClusterTypeProvisioned,
					},
					{
						ClusterName: strptr("prod-cluster"),
						ClusterArn:  strptr("arn:aws:kafka:us-west-2:123456789012:cluster/prod-cluster/def-456"),
						State:       types.ClusterStateActive,
						ClusterType: types.ClusterTypeProvisioned,
					},
				},
			},
			expected: []types.Cluster{
				{
					ClusterName: strptr("test-cluster"),
					ClusterArn:  strptr("arn:aws:kafka:us-west-2:123456789012:cluster/test-cluster/abc-123"),
					State:       types.ClusterStateActive,
					ClusterType: types.ClusterTypeProvisioned,
				},
				{
					ClusterName: strptr("prod-cluster"),
					ClusterArn:  strptr("arn:aws:kafka:us-west-2:123456789012:cluster/prod-cluster/def-456"),
					State:       types.ClusterStateActive,
					ClusterType: types.ClusterTypeProvisioned,
				},
			},
		},
		{
			name: "SingleCluster",
			mskData: &mskDataStore{
				region: "us-east-1",
				clusters: []types.Cluster{
					{
						ClusterName: strptr("single-cluster"),
						ClusterArn:  strptr("arn:aws:kafka:us-east-1:123456789012:cluster/single-cluster/xyz-789"),
						State:       types.ClusterStateActive,
						ClusterType: types.ClusterTypeProvisioned,
					},
				},
			},
			expected: []types.Cluster{
				{
					ClusterName: strptr("single-cluster"),
					ClusterArn:  strptr("arn:aws:kafka:us-east-1:123456789012:cluster/single-cluster/xyz-789"),
					State:       types.ClusterStateActive,
					ClusterType: types.ClusterTypeProvisioned,
				},
			},
		},
		{
			name: "NoClusters",
			mskData: &mskDataStore{
				region:   "us-east-1",
				clusters: []types.Cluster{},
			},
			expected: nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockMSKClient(tt.mskData)

			d := &MSKDiscovery{
				msk: client,
				cfg: &MSKSDConfig{
					Region: tt.mskData.region,
				},
			}

			clusters, err := d.listClusters(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.expected, clusters)
		})
	}
}

func TestMSKDiscoveryDescribeClusters(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name        string
		mskData     *mskDataStore
		clusterARNs []string
		expected    []types.Cluster
	}{
		{
			name: "SingleCluster",
			mskData: &mskDataStore{
				region: "us-west-2",
				clusters: []types.Cluster{
					{
						ClusterName:    strptr("test-cluster"),
						ClusterArn:     strptr("arn:aws:kafka:us-west-2:123456789012:cluster/test-cluster/abc-123"),
						State:          types.ClusterStateActive,
						ClusterType:    types.ClusterTypeProvisioned,
						CurrentVersion: strptr("1.2.3"),
						Tags: map[string]string{
							"Environment": "production",
							"Team":        "platform",
						},
					},
				},
			},
			clusterARNs: []string{"arn:aws:kafka:us-west-2:123456789012:cluster/test-cluster/abc-123"},
			expected: []types.Cluster{
				{
					ClusterName:    strptr("test-cluster"),
					ClusterArn:     strptr("arn:aws:kafka:us-west-2:123456789012:cluster/test-cluster/abc-123"),
					State:          types.ClusterStateActive,
					ClusterType:    types.ClusterTypeProvisioned,
					CurrentVersion: strptr("1.2.3"),
					Tags: map[string]string{
						"Environment": "production",
						"Team":        "platform",
					},
				},
			},
		},
		{
			name: "MultipleClusters",
			mskData: &mskDataStore{
				region: "us-east-1",
				clusters: []types.Cluster{
					{
						ClusterName: strptr("cluster-1"),
						ClusterArn:  strptr("arn:aws:kafka:us-east-1:123456789012:cluster/cluster-1/xyz-789"),
						State:       types.ClusterStateActive,
						ClusterType: types.ClusterTypeProvisioned,
					},
					{
						ClusterName: strptr("cluster-2"),
						ClusterArn:  strptr("arn:aws:kafka:us-east-1:123456789012:cluster/cluster-2/def-456"),
						State:       types.ClusterStateActive,
						ClusterType: types.ClusterTypeProvisioned,
						Tags: map[string]string{
							"Stage": "prod",
						},
					},
				},
			},
			clusterARNs: []string{
				"arn:aws:kafka:us-east-1:123456789012:cluster/cluster-1/xyz-789",
				"arn:aws:kafka:us-east-1:123456789012:cluster/cluster-2/def-456",
			},
			expected: []types.Cluster{
				{
					ClusterName: strptr("cluster-1"),
					ClusterArn:  strptr("arn:aws:kafka:us-east-1:123456789012:cluster/cluster-1/xyz-789"),
					State:       types.ClusterStateActive,
					ClusterType: types.ClusterTypeProvisioned,
				},
				{
					ClusterName: strptr("cluster-2"),
					ClusterArn:  strptr("arn:aws:kafka:us-east-1:123456789012:cluster/cluster-2/def-456"),
					State:       types.ClusterStateActive,
					ClusterType: types.ClusterTypeProvisioned,
					Tags: map[string]string{
						"Stage": "prod",
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockMSKClient(tt.mskData)

			d := &MSKDiscovery{
				msk: client,
				cfg: &MSKSDConfig{
					Region: tt.mskData.region,
				},
			}

			clusters, err := d.describeClusters(ctx, tt.clusterARNs)
			require.NoError(t, err)

			// Sort clusters by ARN to handle non-deterministic ordering from goroutines
			sort.Slice(clusters, func(i, j int) bool {
				return aws.ToString(clusters[i].ClusterArn) < aws.ToString(clusters[j].ClusterArn)
			})
			sort.Slice(tt.expected, func(i, j int) bool {
				return aws.ToString(tt.expected[i].ClusterArn) < aws.ToString(tt.expected[j].ClusterArn)
			})

			require.Equal(t, tt.expected, clusters)
		})
	}
}

func TestMSKDiscoveryListNodes(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name       string
		mskData    *mskDataStore
		clusterARN string
		expected   []types.NodeInfo
	}{
		{
			name: "ClusterWithBrokers",
			mskData: &mskDataStore{
				region: "us-west-2",
				nodes: map[string][]types.NodeInfo{
					"arn:aws:kafka:us-west-2:123456789012:cluster/test-cluster/abc-123": {
						{
							NodeARN:            strptr("arn:aws:kafka:us-west-2:123456789012:node/broker-1"),
							AddedToClusterTime: strptr("2023-01-01T00:00:00Z"),
							InstanceType:       strptr("kafka.m5.large"),
							BrokerNodeInfo: &types.BrokerNodeInfo{
								BrokerId:           aws.Float64(1),
								ClientSubnet:       strptr("subnet-12345"),
								ClientVpcIpAddress: strptr("10.0.1.100"),
								Endpoints:          []string{"b-1.test-cluster.abc123.kafka.us-west-2.amazonaws.com"},
								AttachedENIId:      strptr("eni-12345"),
							},
						},
						{
							NodeARN:            strptr("arn:aws:kafka:us-west-2:123456789012:node/broker-2"),
							AddedToClusterTime: strptr("2023-01-01T00:00:00Z"),
							InstanceType:       strptr("kafka.m5.large"),
							BrokerNodeInfo: &types.BrokerNodeInfo{
								BrokerId:           aws.Float64(2),
								ClientSubnet:       strptr("subnet-67890"),
								ClientVpcIpAddress: strptr("10.0.1.101"),
								Endpoints:          []string{"b-2.test-cluster.abc123.kafka.us-west-2.amazonaws.com"},
								AttachedENIId:      strptr("eni-67890"),
							},
						},
					},
				},
			},
			clusterARN: "arn:aws:kafka:us-west-2:123456789012:cluster/test-cluster/abc-123",
			expected: []types.NodeInfo{
				{
					NodeARN:            strptr("arn:aws:kafka:us-west-2:123456789012:node/broker-1"),
					AddedToClusterTime: strptr("2023-01-01T00:00:00Z"),
					InstanceType:       strptr("kafka.m5.large"),
					BrokerNodeInfo: &types.BrokerNodeInfo{
						BrokerId:           aws.Float64(1),
						ClientSubnet:       strptr("subnet-12345"),
						ClientVpcIpAddress: strptr("10.0.1.100"),
						Endpoints:          []string{"b-1.test-cluster.abc123.kafka.us-west-2.amazonaws.com"},
						AttachedENIId:      strptr("eni-12345"),
					},
				},
				{
					NodeARN:            strptr("arn:aws:kafka:us-west-2:123456789012:node/broker-2"),
					AddedToClusterTime: strptr("2023-01-01T00:00:00Z"),
					InstanceType:       strptr("kafka.m5.large"),
					BrokerNodeInfo: &types.BrokerNodeInfo{
						BrokerId:           aws.Float64(2),
						ClientSubnet:       strptr("subnet-67890"),
						ClientVpcIpAddress: strptr("10.0.1.101"),
						Endpoints:          []string{"b-2.test-cluster.abc123.kafka.us-west-2.amazonaws.com"},
						AttachedENIId:      strptr("eni-67890"),
					},
				},
			},
		},
		{
			name: "ClusterWithNoNodes",
			mskData: &mskDataStore{
				region: "us-west-2",
				nodes: map[string][]types.NodeInfo{
					"arn:aws:kafka:us-west-2:123456789012:cluster/empty-cluster/xyz-789": {},
				},
			},
			clusterARN: "arn:aws:kafka:us-west-2:123456789012:cluster/empty-cluster/xyz-789",
			expected:   nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockMSKClient(tt.mskData)

			d := &MSKDiscovery{
				msk: client,
				cfg: &MSKSDConfig{
					Region: tt.mskData.region,
				},
			}

			nodes, err := d.listNodes(ctx, tt.clusterARN)
			require.NoError(t, err)
			require.Equal(t, tt.expected, nodes)
		})
	}
}

func TestMSKDiscoveryRefresh(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		mskData  *mskDataStore
		config   *MSKSDConfig
		expected []*targetgroup.Group
	}{
		{
			name: "ClusterWithBrokersUsingClustersConfig",
			mskData: &mskDataStore{
				region: "us-west-2",
				clusters: []types.Cluster{
					{
						ClusterName:    strptr("test-cluster"),
						ClusterArn:     strptr("arn:aws:kafka:us-west-2:123456789012:cluster/test-cluster/abc-123"),
						State:          types.ClusterStateActive,
						ClusterType:    types.ClusterTypeProvisioned,
						CurrentVersion: strptr("1.2.3"),
						Tags: map[string]string{
							"Environment": "production",
							"Team":        "platform",
						},
						Provisioned: &types.Provisioned{
							CurrentBrokerSoftwareInfo: &types.BrokerSoftwareInfo{
								ConfigurationArn:      strptr("arn:aws:kafka:us-west-2:123456789012:configuration/my-config/abc-123"),
								ConfigurationRevision: aws.Int64(1),
								KafkaVersion:          strptr("2.8.1"),
							},
							OpenMonitoring: &types.OpenMonitoringInfo{
								Prometheus: &types.PrometheusInfo{
									JmxExporter: &types.JmxExporterInfo{
										EnabledInBroker: aws.Bool(true),
									},
									NodeExporter: &types.NodeExporterInfo{
										EnabledInBroker: aws.Bool(true),
									},
								},
							},
						},
					},
				},
				nodes: map[string][]types.NodeInfo{
					"arn:aws:kafka:us-west-2:123456789012:cluster/test-cluster/abc-123": {
						{
							NodeARN:            strptr("arn:aws:kafka:us-west-2:123456789012:node/broker-1"),
							AddedToClusterTime: strptr("2023-01-01T00:00:00Z"),
							InstanceType:       strptr("kafka.m5.large"),
							BrokerNodeInfo: &types.BrokerNodeInfo{
								BrokerId:           aws.Float64(1),
								ClientSubnet:       strptr("subnet-12345"),
								ClientVpcIpAddress: strptr("10.0.1.100"),
								Endpoints:          []string{"b-1.test-cluster.abc123.kafka.us-west-2.amazonaws.com"},
								AttachedENIId:      strptr("eni-12345"),
							},
						},
					},
				},
			},
			config: &MSKSDConfig{
				Region:   "us-west-2",
				Port:     80,
				Clusters: []string{"arn:aws:kafka:us-west-2:123456789012:cluster/test-cluster/abc-123"},
			},
			expected: []*targetgroup.Group{
				{
					Source: "us-west-2",
					Targets: []model.LabelSet{
						{
							model.AddressLabel:                          model.LabelValue("b-1.test-cluster.abc123.kafka.us-west-2.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("test-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-west-2:123456789012:cluster/test-cluster/abc-123"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("1.2.3"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-west-2:123456789012:configuration/my-config/abc-123"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("1"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("2.8.1"),
							"__meta_msk_cluster_tag_Environment":        model.LabelValue("production"),
							"__meta_msk_cluster_tag_Team":               model.LabelValue("platform"),
							"__meta_msk_node_type":                      model.LabelValue("BROKER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-west-2:123456789012:node/broker-1"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-01-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.large"),
							"__meta_msk_node_attached_eni":              model.LabelValue("eni-12345"),
							"__meta_msk_broker_id":                      model.LabelValue("1"),
							"__meta_msk_broker_client_subnet":           model.LabelValue("subnet-12345"),
							"__meta_msk_broker_client_vpc_ip":           model.LabelValue("10.0.1.100"),
							"__meta_msk_broker_node_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_broker_endpoint_index":          model.LabelValue("0"),
						},
					},
				},
			},
		},
		{
			name: "NoClustersWithEmptyClustersConfig",
			mskData: &mskDataStore{
				region:   "us-east-1",
				clusters: []types.Cluster{},
			},
			config: &MSKSDConfig{
				Region:   "us-east-1",
				Port:     80,
				Clusters: []string{}, // Empty clusters list uses listClusters
			},
			expected: []*targetgroup.Group{
				{
					Source: "us-east-1",
				},
			},
		},
		{
			name: "ClusterWithBrokersUsingListClusters",
			mskData: &mskDataStore{
				region: "us-west-2",
				clusters: []types.Cluster{
					{
						ClusterName:    strptr("auto-discovered-cluster"),
						ClusterArn:     strptr("arn:aws:kafka:us-west-2:123456789012:cluster/auto-discovered-cluster/xyz-123"),
						State:          types.ClusterStateActive,
						ClusterType:    types.ClusterTypeProvisioned,
						CurrentVersion: strptr("1.0.0"),
						Provisioned: &types.Provisioned{
							CurrentBrokerSoftwareInfo: &types.BrokerSoftwareInfo{
								ConfigurationArn:      strptr("arn:aws:kafka:us-west-2:123456789012:configuration/config/xyz"),
								ConfigurationRevision: aws.Int64(1),
								KafkaVersion:          strptr("3.3.1"),
							},
							OpenMonitoring: &types.OpenMonitoringInfo{
								Prometheus: &types.PrometheusInfo{
									JmxExporter: &types.JmxExporterInfo{
										EnabledInBroker: aws.Bool(true),
									},
									NodeExporter: &types.NodeExporterInfo{
										EnabledInBroker: aws.Bool(true),
									},
								},
							},
						},
					},
				},
				nodes: map[string][]types.NodeInfo{
					"arn:aws:kafka:us-west-2:123456789012:cluster/auto-discovered-cluster/xyz-123": {
						{
							NodeARN:            strptr("arn:aws:kafka:us-west-2:123456789012:node/broker-auto"),
							AddedToClusterTime: strptr("2023-01-01T00:00:00Z"),
							InstanceType:       strptr("kafka.m5.large"),
							BrokerNodeInfo: &types.BrokerNodeInfo{
								BrokerId:           aws.Float64(1),
								ClientSubnet:       strptr("subnet-auto"),
								ClientVpcIpAddress: strptr("10.0.1.200"),
								Endpoints:          []string{"b-auto.cluster.kafka.us-west-2.amazonaws.com"},
								AttachedENIId:      strptr("eni-auto"),
							},
						},
					},
				},
			},
			config: &MSKSDConfig{
				Region:   "us-west-2",
				Port:     80,
				Clusters: nil, // nil clusters list uses listClusters (backward compatibility)
			},
			expected: []*targetgroup.Group{
				{
					Source: "us-west-2",
					Targets: []model.LabelSet{
						{
							model.AddressLabel:                          model.LabelValue("b-auto.cluster.kafka.us-west-2.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("auto-discovered-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-west-2:123456789012:cluster/auto-discovered-cluster/xyz-123"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("1.0.0"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-west-2:123456789012:configuration/config/xyz"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("1"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("3.3.1"),
							"__meta_msk_node_type":                      model.LabelValue("BROKER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-west-2:123456789012:node/broker-auto"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-01-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.large"),
							"__meta_msk_node_attached_eni":              model.LabelValue("eni-auto"),
							"__meta_msk_broker_id":                      model.LabelValue("1"),
							"__meta_msk_broker_client_subnet":           model.LabelValue("subnet-auto"),
							"__meta_msk_broker_client_vpc_ip":           model.LabelValue("10.0.1.200"),
							"__meta_msk_broker_node_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_broker_endpoint_index":          model.LabelValue("0"),
						},
					},
				},
			},
		},
		{
			name: "ClusterWithBrokersAndControllersUsingClustersConfig",
			mskData: &mskDataStore{
				region: "us-west-2",
				clusters: []types.Cluster{
					{
						ClusterName:    strptr("kraft-cluster"),
						ClusterArn:     strptr("arn:aws:kafka:us-west-2:123456789012:cluster/kraft-cluster/xyz-789"),
						State:          types.ClusterStateActive,
						ClusterType:    types.ClusterTypeProvisioned,
						CurrentVersion: strptr("1.0.0"),
						Tags: map[string]string{
							"Type": "kraft",
						},
						Provisioned: &types.Provisioned{
							CurrentBrokerSoftwareInfo: &types.BrokerSoftwareInfo{
								ConfigurationArn:      strptr("arn:aws:kafka:us-west-2:123456789012:configuration/config/xyz"),
								ConfigurationRevision: aws.Int64(2),
								KafkaVersion:          strptr("3.3.1"),
							},
							OpenMonitoring: &types.OpenMonitoringInfo{
								Prometheus: &types.PrometheusInfo{
									JmxExporter: &types.JmxExporterInfo{
										EnabledInBroker: aws.Bool(true),
									},
									NodeExporter: &types.NodeExporterInfo{
										EnabledInBroker: aws.Bool(false),
									},
								},
							},
						},
					},
				},
				nodes: map[string][]types.NodeInfo{
					"arn:aws:kafka:us-west-2:123456789012:cluster/kraft-cluster/xyz-789": {
						{
							NodeARN:            strptr("arn:aws:kafka:us-west-2:123456789012:node/broker-1"),
							AddedToClusterTime: strptr("2023-06-01T00:00:00Z"),
							InstanceType:       strptr("kafka.m5.large"),
							BrokerNodeInfo: &types.BrokerNodeInfo{
								BrokerId:           aws.Float64(1),
								ClientSubnet:       strptr("subnet-abc123"),
								ClientVpcIpAddress: strptr("10.0.2.100"),
								Endpoints:          []string{"b-1.kraft-cluster.xyz789.kafka.us-west-2.amazonaws.com"},
								AttachedENIId:      strptr("eni-broker-1"),
							},
						},
						{
							NodeARN:            strptr("arn:aws:kafka:us-west-2:123456789012:node/broker-2"),
							AddedToClusterTime: strptr("2023-06-01T00:00:00Z"),
							InstanceType:       strptr("kafka.m5.large"),
							BrokerNodeInfo: &types.BrokerNodeInfo{
								BrokerId:           aws.Float64(2),
								ClientSubnet:       strptr("subnet-abc124"),
								ClientVpcIpAddress: strptr("10.0.2.101"),
								Endpoints:          []string{"b-2.kraft-cluster.xyz789.kafka.us-west-2.amazonaws.com"},
								AttachedENIId:      strptr("eni-broker-2"),
							},
						},
						{
							NodeARN:            strptr("arn:aws:kafka:us-west-2:123456789012:node/controller-1"),
							AddedToClusterTime: strptr("2023-06-01T00:00:00Z"),
							InstanceType:       strptr("kafka.m5.large"),
							ControllerNodeInfo: &types.ControllerNodeInfo{
								Endpoints: []string{"c-1.kraft-cluster.xyz789.kafka.us-west-2.amazonaws.com"},
							},
						},
						{
							NodeARN:            strptr("arn:aws:kafka:us-west-2:123456789012:node/controller-2"),
							AddedToClusterTime: strptr("2023-06-01T00:00:00Z"),
							InstanceType:       strptr("kafka.m5.large"),
							ControllerNodeInfo: &types.ControllerNodeInfo{
								Endpoints: []string{"c-2.kraft-cluster.xyz789.kafka.us-west-2.amazonaws.com"},
							},
						},
					},
				},
			},
			config: &MSKSDConfig{
				Region:   "us-west-2",
				Port:     80,
				Clusters: []string{"arn:aws:kafka:us-west-2:123456789012:cluster/kraft-cluster/xyz-789"},
			},
			expected: []*targetgroup.Group{
				{
					Source: "us-west-2",
					Targets: []model.LabelSet{
						{
							model.AddressLabel:                          model.LabelValue("b-1.kraft-cluster.xyz789.kafka.us-west-2.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("kraft-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-west-2:123456789012:cluster/kraft-cluster/xyz-789"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("1.0.0"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-west-2:123456789012:configuration/config/xyz"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("2"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("3.3.1"),
							"__meta_msk_cluster_tag_Type":               model.LabelValue("kraft"),
							"__meta_msk_node_type":                      model.LabelValue("BROKER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-west-2:123456789012:node/broker-1"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-06-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.large"),
							"__meta_msk_node_attached_eni":              model.LabelValue("eni-broker-1"),
							"__meta_msk_broker_id":                      model.LabelValue("1"),
							"__meta_msk_broker_client_subnet":           model.LabelValue("subnet-abc123"),
							"__meta_msk_broker_client_vpc_ip":           model.LabelValue("10.0.2.100"),
							"__meta_msk_broker_node_exporter_enabled":   model.LabelValue("false"),
							"__meta_msk_broker_endpoint_index":          model.LabelValue("0"),
						},
						{
							model.AddressLabel:                          model.LabelValue("b-2.kraft-cluster.xyz789.kafka.us-west-2.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("kraft-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-west-2:123456789012:cluster/kraft-cluster/xyz-789"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("1.0.0"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-west-2:123456789012:configuration/config/xyz"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("2"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("3.3.1"),
							"__meta_msk_cluster_tag_Type":               model.LabelValue("kraft"),
							"__meta_msk_node_type":                      model.LabelValue("BROKER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-west-2:123456789012:node/broker-2"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-06-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.large"),
							"__meta_msk_node_attached_eni":              model.LabelValue("eni-broker-2"),
							"__meta_msk_broker_id":                      model.LabelValue("2"),
							"__meta_msk_broker_client_subnet":           model.LabelValue("subnet-abc124"),
							"__meta_msk_broker_client_vpc_ip":           model.LabelValue("10.0.2.101"),
							"__meta_msk_broker_node_exporter_enabled":   model.LabelValue("false"),
							"__meta_msk_broker_endpoint_index":          model.LabelValue("0"),
						},
						{
							model.AddressLabel:                          model.LabelValue("c-1.kraft-cluster.xyz789.kafka.us-west-2.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("kraft-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-west-2:123456789012:cluster/kraft-cluster/xyz-789"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("1.0.0"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-west-2:123456789012:configuration/config/xyz"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("2"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("3.3.1"),
							"__meta_msk_cluster_tag_Type":               model.LabelValue("kraft"),
							"__meta_msk_node_type":                      model.LabelValue("CONTROLLER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-west-2:123456789012:node/controller-1"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-06-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.large"),
							"__meta_msk_controller_endpoint_index":      model.LabelValue("0"),
						},
						{
							model.AddressLabel:                          model.LabelValue("c-2.kraft-cluster.xyz789.kafka.us-west-2.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("kraft-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-west-2:123456789012:cluster/kraft-cluster/xyz-789"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("1.0.0"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-west-2:123456789012:configuration/config/xyz"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("2"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("3.3.1"),
							"__meta_msk_cluster_tag_Type":               model.LabelValue("kraft"),
							"__meta_msk_node_type":                      model.LabelValue("CONTROLLER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-west-2:123456789012:node/controller-2"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-06-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.large"),
							"__meta_msk_controller_endpoint_index":      model.LabelValue("0"),
						},
					},
				},
			},
		},
		{
			name: "NodesWithMultipleEndpointsUsingClustersConfig",
			mskData: &mskDataStore{
				region: "us-east-1",
				clusters: []types.Cluster{
					{
						ClusterName:    strptr("multi-endpoint-cluster"),
						ClusterArn:     strptr("arn:aws:kafka:us-east-1:123456789012:cluster/multi-endpoint-cluster/abc-999"),
						State:          types.ClusterStateActive,
						ClusterType:    types.ClusterTypeProvisioned,
						CurrentVersion: strptr("2.0.0"),
						Provisioned: &types.Provisioned{
							CurrentBrokerSoftwareInfo: &types.BrokerSoftwareInfo{
								ConfigurationArn:      strptr("arn:aws:kafka:us-east-1:123456789012:configuration/config/abc"),
								ConfigurationRevision: aws.Int64(1),
								KafkaVersion:          strptr("3.4.0"),
							},
							OpenMonitoring: &types.OpenMonitoringInfo{
								Prometheus: &types.PrometheusInfo{
									JmxExporter: &types.JmxExporterInfo{
										EnabledInBroker: aws.Bool(true),
									},
									NodeExporter: &types.NodeExporterInfo{
										EnabledInBroker: aws.Bool(true),
									},
								},
							},
						},
					},
				},
				nodes: map[string][]types.NodeInfo{
					"arn:aws:kafka:us-east-1:123456789012:cluster/multi-endpoint-cluster/abc-999": {
						{
							NodeARN:            strptr("arn:aws:kafka:us-east-1:123456789012:node/broker-multi"),
							AddedToClusterTime: strptr("2023-08-01T00:00:00Z"),
							InstanceType:       strptr("kafka.m5.xlarge"),
							BrokerNodeInfo: &types.BrokerNodeInfo{
								BrokerId:           aws.Float64(3),
								ClientSubnet:       strptr("subnet-multi-1"),
								ClientVpcIpAddress: strptr("10.0.3.50"),
								// Multiple endpoints for this broker
								Endpoints:     []string{"b-3-1.cluster.kafka.us-east-1.amazonaws.com", "b-3-2.cluster.kafka.us-east-1.amazonaws.com", "b-3-3.cluster.kafka.us-east-1.amazonaws.com"},
								AttachedENIId: strptr("eni-multi-broker"),
							},
						},
						{
							NodeARN:            strptr("arn:aws:kafka:us-east-1:123456789012:node/controller-multi"),
							AddedToClusterTime: strptr("2023-08-01T00:00:00Z"),
							InstanceType:       strptr("kafka.m5.large"),
							ControllerNodeInfo: &types.ControllerNodeInfo{
								// Multiple endpoints for this controller
								Endpoints: []string{"c-1-1.cluster.kafka.us-east-1.amazonaws.com", "c-1-2.cluster.kafka.us-east-1.amazonaws.com", "c-1-3.cluster.kafka.us-east-1.amazonaws.com", "c-1-4.cluster.kafka.us-east-1.amazonaws.com"},
							},
						},
					},
				},
			},
			config: &MSKSDConfig{
				Region:   "us-east-1",
				Port:     80,
				Clusters: []string{"arn:aws:kafka:us-east-1:123456789012:cluster/multi-endpoint-cluster/abc-999"},
			},
			expected: []*targetgroup.Group{
				{
					Source: "us-east-1",
					Targets: []model.LabelSet{
						// Broker with 3 endpoints - creates 3 targets with different endpoint indices
						{
							model.AddressLabel:                          model.LabelValue("b-3-1.cluster.kafka.us-east-1.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("multi-endpoint-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-east-1:123456789012:cluster/multi-endpoint-cluster/abc-999"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("2.0.0"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-east-1:123456789012:configuration/config/abc"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("1"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("3.4.0"),
							"__meta_msk_node_type":                      model.LabelValue("BROKER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-east-1:123456789012:node/broker-multi"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-08-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.xlarge"),
							"__meta_msk_node_attached_eni":              model.LabelValue("eni-multi-broker"),
							"__meta_msk_broker_id":                      model.LabelValue("3"),
							"__meta_msk_broker_client_subnet":           model.LabelValue("subnet-multi-1"),
							"__meta_msk_broker_client_vpc_ip":           model.LabelValue("10.0.3.50"),
							"__meta_msk_broker_node_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_broker_endpoint_index":          model.LabelValue("0"),
						},
						{
							model.AddressLabel:                          model.LabelValue("b-3-2.cluster.kafka.us-east-1.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("multi-endpoint-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-east-1:123456789012:cluster/multi-endpoint-cluster/abc-999"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("2.0.0"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-east-1:123456789012:configuration/config/abc"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("1"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("3.4.0"),
							"__meta_msk_node_type":                      model.LabelValue("BROKER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-east-1:123456789012:node/broker-multi"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-08-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.xlarge"),
							"__meta_msk_node_attached_eni":              model.LabelValue("eni-multi-broker"),
							"__meta_msk_broker_id":                      model.LabelValue("3"),
							"__meta_msk_broker_client_subnet":           model.LabelValue("subnet-multi-1"),
							"__meta_msk_broker_client_vpc_ip":           model.LabelValue("10.0.3.50"),
							"__meta_msk_broker_node_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_broker_endpoint_index":          model.LabelValue("1"),
						},
						{
							model.AddressLabel:                          model.LabelValue("b-3-3.cluster.kafka.us-east-1.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("multi-endpoint-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-east-1:123456789012:cluster/multi-endpoint-cluster/abc-999"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("2.0.0"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-east-1:123456789012:configuration/config/abc"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("1"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("3.4.0"),
							"__meta_msk_node_type":                      model.LabelValue("BROKER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-east-1:123456789012:node/broker-multi"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-08-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.xlarge"),
							"__meta_msk_node_attached_eni":              model.LabelValue("eni-multi-broker"),
							"__meta_msk_broker_id":                      model.LabelValue("3"),
							"__meta_msk_broker_client_subnet":           model.LabelValue("subnet-multi-1"),
							"__meta_msk_broker_client_vpc_ip":           model.LabelValue("10.0.3.50"),
							"__meta_msk_broker_node_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_broker_endpoint_index":          model.LabelValue("2"),
						},
						// Controller with 4 endpoints - creates 4 targets with different endpoint indices
						{
							model.AddressLabel:                          model.LabelValue("c-1-1.cluster.kafka.us-east-1.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("multi-endpoint-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-east-1:123456789012:cluster/multi-endpoint-cluster/abc-999"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("2.0.0"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-east-1:123456789012:configuration/config/abc"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("1"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("3.4.0"),
							"__meta_msk_node_type":                      model.LabelValue("CONTROLLER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-east-1:123456789012:node/controller-multi"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-08-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.large"),
							"__meta_msk_controller_endpoint_index":      model.LabelValue("0"),
						},
						{
							model.AddressLabel:                          model.LabelValue("c-1-2.cluster.kafka.us-east-1.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("multi-endpoint-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-east-1:123456789012:cluster/multi-endpoint-cluster/abc-999"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("2.0.0"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-east-1:123456789012:configuration/config/abc"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("1"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("3.4.0"),
							"__meta_msk_node_type":                      model.LabelValue("CONTROLLER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-east-1:123456789012:node/controller-multi"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-08-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.large"),
							"__meta_msk_controller_endpoint_index":      model.LabelValue("1"),
						},
						{
							model.AddressLabel:                          model.LabelValue("c-1-3.cluster.kafka.us-east-1.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("multi-endpoint-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-east-1:123456789012:cluster/multi-endpoint-cluster/abc-999"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("2.0.0"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-east-1:123456789012:configuration/config/abc"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("1"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("3.4.0"),
							"__meta_msk_node_type":                      model.LabelValue("CONTROLLER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-east-1:123456789012:node/controller-multi"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-08-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.large"),
							"__meta_msk_controller_endpoint_index":      model.LabelValue("2"),
						},
						{
							model.AddressLabel:                          model.LabelValue("c-1-4.cluster.kafka.us-east-1.amazonaws.com:80"),
							"__meta_msk_cluster_name":                   model.LabelValue("multi-endpoint-cluster"),
							"__meta_msk_cluster_arn":                    model.LabelValue("arn:aws:kafka:us-east-1:123456789012:cluster/multi-endpoint-cluster/abc-999"),
							"__meta_msk_cluster_state":                  model.LabelValue("ACTIVE"),
							"__meta_msk_cluster_type":                   model.LabelValue("PROVISIONED"),
							"__meta_msk_cluster_version":                model.LabelValue("2.0.0"),
							"__meta_msk_cluster_jmx_exporter_enabled":   model.LabelValue("true"),
							"__meta_msk_cluster_configuration_arn":      model.LabelValue("arn:aws:kafka:us-east-1:123456789012:configuration/config/abc"),
							"__meta_msk_cluster_configuration_revision": model.LabelValue("1"),
							"__meta_msk_cluster_kafka_version":          model.LabelValue("3.4.0"),
							"__meta_msk_node_type":                      model.LabelValue("CONTROLLER"),
							"__meta_msk_node_arn":                       model.LabelValue("arn:aws:kafka:us-east-1:123456789012:node/controller-multi"),
							"__meta_msk_node_added_time":                model.LabelValue("2023-08-01T00:00:00Z"),
							"__meta_msk_node_instance_type":             model.LabelValue("kafka.m5.large"),
							"__meta_msk_controller_endpoint_index":      model.LabelValue("3"),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockMSKClient(tt.mskData)

			config := tt.config
			if config == nil {
				// Default config for backward compatibility
				config = &MSKSDConfig{
					Region: tt.mskData.region,
					Port:   80,
				}
			}

			d := &MSKDiscovery{
				msk: client,
				cfg: config,
			}

			groups, err := d.refresh(ctx)
			require.NoError(t, err)

			// Sort targets within each group by address to handle non-deterministic ordering from goroutines
			for _, group := range groups {
				if len(group.Targets) > 0 {
					sort.Slice(group.Targets, func(i, j int) bool {
						return string(group.Targets[i][model.AddressLabel]) < string(group.Targets[j][model.AddressLabel])
					})
				}
			}
			for _, group := range tt.expected {
				if len(group.Targets) > 0 {
					sort.Slice(group.Targets, func(i, j int) bool {
						return string(group.Targets[i][model.AddressLabel]) < string(group.Targets[j][model.AddressLabel])
					})
				}
			}

			require.Equal(t, tt.expected, groups)
		})
	}
}

func TestNodeType(t *testing.T) {
	tests := []struct {
		name     string
		node     types.NodeInfo
		expected NodeType
	}{
		{
			name: "BrokerNode",
			node: types.NodeInfo{
				BrokerNodeInfo: &types.BrokerNodeInfo{},
			},
			expected: NodeTypeBroker,
		},
		{
			name: "ControllerNode",
			node: types.NodeInfo{
				ControllerNodeInfo: &types.ControllerNodeInfo{},
			},
			expected: NodeTypeController,
		},
		{
			name:     "UnknownNode",
			node:     types.NodeInfo{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nodeType(tt.node)
			require.Equal(t, tt.expected, result)
		})
	}
}

// MSK client mock.
type mockMSKClient struct {
	mskData mskDataStore
}

func newMockMSKClient(mskData *mskDataStore) *mockMSKClient {
	return &mockMSKClient{
		mskData: *mskData,
	}
}

func (m *mockMSKClient) DescribeClusterV2(_ context.Context, input *kafka.DescribeClusterV2Input, _ ...func(*kafka.Options)) (*kafka.DescribeClusterV2Output, error) {
	inputARN := aws.ToString(input.ClusterArn)
	for i := range m.mskData.clusters {
		cluster := &m.mskData.clusters[i]
		if aws.ToString(cluster.ClusterArn) == inputARN {
			return &kafka.DescribeClusterV2Output{
				ClusterInfo: cluster,
			}, nil
		}
	}

	return nil, fmt.Errorf("cluster not found: %s", inputARN)
}

func (m *mockMSKClient) ListClustersV2(_ context.Context, input *kafka.ListClustersV2Input, _ ...func(*kafka.Options)) (*kafka.ListClustersV2Output, error) {
	var clusters []types.Cluster

	for _, cluster := range m.mskData.clusters {
		// Apply cluster name filter if specified
		if input.ClusterNameFilter != nil && *input.ClusterNameFilter != "" {
			if cluster.ClusterName != nil && *cluster.ClusterName != *input.ClusterNameFilter {
				continue
			}
		}

		// Apply cluster type filter if specified
		if input.ClusterTypeFilter != nil && *input.ClusterTypeFilter != "" {
			if string(cluster.ClusterType) != *input.ClusterTypeFilter {
				continue
			}
		}

		clusters = append(clusters, cluster)
	}

	return &kafka.ListClustersV2Output{
		ClusterInfoList: clusters,
	}, nil
}

func (m *mockMSKClient) ListNodes(_ context.Context, input *kafka.ListNodesInput, _ ...func(*kafka.Options)) (*kafka.ListNodesOutput, error) {
	clusterARN := aws.ToString(input.ClusterArn)
	nodes, exists := m.mskData.nodes[clusterARN]
	if !exists {
		return &kafka.ListNodesOutput{
			NodeInfoList: nil,
		}, nil
	}

	return &kafka.ListNodesOutput{
		NodeInfoList: nodes,
	}, nil
}
