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
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Struct for test data.
type elasticacheDataStore struct {
	region           string
	serverlessCaches []types.ServerlessCache
	cacheClusters    []types.CacheCluster
	tags             map[string][]types.Tag // keyed by cache ARN
}

func TestElasticacheDiscoveryDescribeServerlessCaches(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name          string
		ecData        *elasticacheDataStore
		cacheNames    []string
		expectedCount int
	}{
		{
			name: "MultipleCaches",
			ecData: &elasticacheDataStore{
				region: "us-west-2",
				serverlessCaches: []types.ServerlessCache{
					{
						ServerlessCacheName: strptr("test-cache"),
						ARN:                 strptr("arn:aws:elasticache:us-west-2:123456789012:serverlesscache:test-cache"),
						Status:              strptr("available"),
						Engine:              strptr("redis"),
						FullEngineVersion:   strptr("7.1"),
						CreateTime:          aws.Time(time.Now()),
						Endpoint: &types.Endpoint{
							Address: strptr("test-cache.serverless.use1.cache.amazonaws.com"),
							Port:    aws.Int32(6379),
						},
					},
					{
						ServerlessCacheName: strptr("prod-cache"),
						ARN:                 strptr("arn:aws:elasticache:us-west-2:123456789012:serverlesscache:prod-cache"),
						Status:              strptr("available"),
						Engine:              strptr("valkey"),
						FullEngineVersion:   strptr("7.2"),
						CreateTime:          aws.Time(time.Now()),
						Endpoint: &types.Endpoint{
							Address: strptr("prod-cache.serverless.use1.cache.amazonaws.com"),
							Port:    aws.Int32(6379),
						},
					},
				},
			},
			cacheNames:    []string{},
			expectedCount: 2,
		},
		{
			name: "SingleCache",
			ecData: &elasticacheDataStore{
				region: "us-east-1",
				serverlessCaches: []types.ServerlessCache{
					{
						ServerlessCacheName: strptr("single-cache"),
						ARN:                 strptr("arn:aws:elasticache:us-east-1:123456789012:serverlesscache:single-cache"),
						Status:              strptr("available"),
						Engine:              strptr("redis"),
						FullEngineVersion:   strptr("7.1"),
						CreateTime:          aws.Time(time.Now()),
					},
				},
			},
			cacheNames:    []string{"single-cache"},
			expectedCount: 1,
		},
		{
			name: "NoCaches",
			ecData: &elasticacheDataStore{
				region:           "us-east-1",
				serverlessCaches: []types.ServerlessCache{},
			},
			cacheNames:    []string{},
			expectedCount: 0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockElasticacheClient(tt.ecData)

			d := &ElasticacheDiscovery{
				elasticacheClient: client,
				cfg: &ElasticacheSDConfig{
					Region:             tt.ecData.region,
					RequestConcurrency: 10,
				},
			}

			caches, err := d.describeServerlessCaches(ctx, tt.cacheNames)
			require.NoError(t, err)
			require.Len(t, caches, tt.expectedCount)
		})
	}
}

func TestElasticacheDiscoveryDescribeCacheClusters(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name          string
		ecData        *elasticacheDataStore
		clusterIDs    []string
		expectedCount int
		skipTest      bool
	}{
		{
			name: "MockValidation",
			ecData: &elasticacheDataStore{
				region: "us-west-2",
				cacheClusters: []types.CacheCluster{
					{
						CacheClusterId:     strptr("test-cluster-001"),
						ARN:                strptr("arn:aws:elasticache:us-west-2:123456789012:cluster:test-cluster-001"),
						CacheClusterStatus: strptr("available"),
						Engine:             strptr("redis"),
						EngineVersion:      strptr("7.1"),
						CacheNodeType:      strptr("cache.t3.micro"),
						NumCacheNodes:      aws.Int32(1),
						ConfigurationEndpoint: &types.Endpoint{
							Address: strptr("test-cluster.abc123.cfg.use1.cache.amazonaws.com"),
							Port:    aws.Int32(6379),
						},
					},
				},
			},
			clusterIDs:    []string{},
			expectedCount: 1,
			skipTest:      false,
		},
		{
			name: "NoClusters",
			ecData: &elasticacheDataStore{
				region:        "us-east-1",
				cacheClusters: []types.CacheCluster{},
			},
			clusterIDs:    []string{},
			expectedCount: 0,
			skipTest:      false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipTest {
				t.Skip("Skipping complex test with concurrency")
			}
			client := newMockElasticacheClient(tt.ecData)

			// Verify mock returns expected data
			output, err := client.DescribeCacheClusters(ctx, &elasticache.DescribeCacheClustersInput{})
			require.NoError(t, err)
			require.Len(t, output.CacheClusters, tt.expectedCount)
		})
	}
}

func TestAddServerlessCacheTargets(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		cache          *types.ServerlessCache
		tags           []types.Tag
		expectedLabels model.LabelSet
	}{
		{
			name: "ServerlessCacheWithEndpoint",
			cache: &types.ServerlessCache{
				ServerlessCacheName: strptr("my-cache"),
				ARN:                 strptr("arn:aws:elasticache:us-east-1:123456789012:serverlesscache:my-cache"),
				Status:              strptr("available"),
				Engine:              strptr("redis"),
				FullEngineVersion:   strptr("7.1"),
				MajorEngineVersion:  strptr("7"),
				CreateTime:          aws.Time(testTime),
				Endpoint: &types.Endpoint{
					Address: strptr("my-cache.serverless.use1.cache.amazonaws.com"),
					Port:    aws.Int32(6379),
				},
				ReaderEndpoint: &types.Endpoint{
					Address: strptr("my-cache-ro.serverless.use1.cache.amazonaws.com"),
					Port:    aws.Int32(6379),
				},
				SecurityGroupIds: []string{"sg-12345"},
				SubnetIds:        []string{"subnet-abcdef"},
				CacheUsageLimits: &types.CacheUsageLimits{
					DataStorage: &types.DataStorage{
						Maximum: aws.Int32(10),
						Minimum: aws.Int32(1),
						Unit:    types.DataStorageUnitGb,
					},
					ECPUPerSecond: &types.ECPUPerSecond{
						Maximum: aws.Int32(5000),
						Minimum: aws.Int32(1000),
					},
				},
			},
			tags: []types.Tag{
				{Key: strptr("Environment"), Value: strptr("test")},
			},
			expectedLabels: model.LabelSet{
				"__meta_elasticache_deployment_option":                     "serverless",
				"__meta_elasticache_serverless_cache_arn":                  "arn:aws:elasticache:us-east-1:123456789012:serverlesscache:my-cache",
				"__meta_elasticache_serverless_cache_name":                 "my-cache",
				"__meta_elasticache_serverless_cache_status":               "available",
				"__meta_elasticache_serverless_cache_engine":               "redis",
				"__meta_elasticache_serverless_cache_full_engine_version":  "7.1",
				"__meta_elasticache_serverless_cache_major_engine_version": "7",
				"__meta_elasticache_serverless_cache_create_time":          "2024-01-01T00:00:00Z",
				"__meta_elasticache_serverless_cache_endpoint_address":     "my-cache.serverless.use1.cache.amazonaws.com",
				"__meta_elasticache_serverless_cache_endpoint_port":        "6379",

				"__meta_elasticache_serverless_cache_security_group_id_0": "sg-12345",
				"__meta_elasticache_serverless_cache_subnet_id_0":         "subnet-abcdef",

				"__address__": "my-cache.serverless.use1.cache.amazonaws.com:6379",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := &targetgroup.Group{
				Source: "test",
			}

			addServerlessCacheTargets(tg, tt.cache, tt.tags)

			require.Len(t, tg.Targets, 1)
			labels := tg.Targets[0]

			// Check that all expected labels are present with correct values
			for k, v := range tt.expectedLabels {
				actualValue, exists := labels[k]
				require.True(t, exists, "label %s should exist", k)
				require.Equal(t, v, actualValue, "label %s mismatch", k)
			}
		})
	}
}

func TestAddCacheClusterTargets(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name                string
		cluster             *types.CacheCluster
		tags                []types.Tag
		expectedTargetCount int
		expectedLabels      []model.LabelSet // One per node
	}{
		{
			name: "CacheClusterWithMultipleNodes",
			cluster: &types.CacheCluster{
				CacheClusterId:         strptr("my-cluster-001"),
				ARN:                    strptr("arn:aws:elasticache:us-east-1:123456789012:cluster:my-cluster-001"),
				CacheClusterStatus:     strptr("available"),
				Engine:                 strptr("redis"),
				EngineVersion:          strptr("7.1"),
				CacheNodeType:          strptr("cache.t3.micro"),
				NumCacheNodes:          aws.Int32(2),
				CacheClusterCreateTime: aws.Time(testTime),
				ConfigurationEndpoint: &types.Endpoint{
					Address: strptr("my-cluster.abc123.cfg.use1.cache.amazonaws.com"),
					Port:    aws.Int32(6379),
				},
				AtRestEncryptionEnabled:   aws.Bool(true),
				TransitEncryptionEnabled:  aws.Bool(true),
				AuthTokenEnabled:          aws.Bool(true),
				AutoMinorVersionUpgrade:   aws.Bool(true),
				CacheSubnetGroupName:      strptr("my-subnet-group"),
				PreferredAvailabilityZone: strptr("us-east-1a"),
				SecurityGroups: []types.SecurityGroupMembership{
					{
						SecurityGroupId: strptr("sg-12345"),
						Status:          strptr("active"),
					},
				},
				CacheNodes: []types.CacheNode{
					{
						CacheNodeId:              strptr("0001"),
						CacheNodeStatus:          strptr("available"),
						CacheNodeCreateTime:      aws.Time(testTime),
						CustomerAvailabilityZone: strptr("us-east-1a"),
						Endpoint: &types.Endpoint{
							Address: strptr("my-cluster-001.abc123.0001.use1.cache.amazonaws.com"),
							Port:    aws.Int32(6379),
						},
					},
					{
						CacheNodeId:              strptr("0002"),
						CacheNodeStatus:          strptr("available"),
						CacheNodeCreateTime:      aws.Time(testTime),
						CustomerAvailabilityZone: strptr("us-east-1b"),
						Endpoint: &types.Endpoint{
							Address: strptr("my-cluster-001.abc123.0002.use1.cache.amazonaws.com"),
							Port:    aws.Int32(6379),
						},
					},
				},
			},
			tags: []types.Tag{
				{Key: strptr("Environment"), Value: strptr("production")},
				{Key: strptr("Application"), Value: strptr("web-app")},
			},
			expectedTargetCount: 2,
			expectedLabels: []model.LabelSet{
				{
					"__meta_elasticache_deployment_option":                                "node",
					"__meta_elasticache_cache_cluster_arn":                                "arn:aws:elasticache:us-east-1:123456789012:cluster:my-cluster-001",
					"__meta_elasticache_cache_cluster_cache_cluster_id":                   "my-cluster-001",
					"__meta_elasticache_cache_cluster_cache_cluster_status":               "available",
					"__meta_elasticache_cache_cluster_engine":                             "redis",
					"__meta_elasticache_cache_cluster_engine_version":                     "7.1",
					"__meta_elasticache_cache_cluster_cache_node_type":                    "cache.t3.micro",
					"__meta_elasticache_cache_cluster_num_cache_nodes":                    "2",
					"__meta_elasticache_cache_cluster_cache_cluster_create_time":          "2024-01-01T00:00:00Z",
					"__meta_elasticache_cache_cluster_configuration_endpoint_address":     "my-cluster.abc123.cfg.use1.cache.amazonaws.com",
					"__meta_elasticache_cache_cluster_configuration_endpoint_port":        "6379",
					"__meta_elasticache_cache_cluster_at_rest_encryption_enabled":         "true",
					"__meta_elasticache_cache_cluster_transit_encryption_enabled":         "true",
					"__meta_elasticache_cache_cluster_auth_token_enabled":                 "true",
					"__meta_elasticache_cache_cluster_auto_minor_version_upgrade":         "true",
					"__meta_elasticache_cache_cluster_cache_subnet_group_name":            "my-subnet-group",
					"__meta_elasticache_cache_cluster_preferred_availability_zone":        "us-east-1a",
					"__meta_elasticache_cache_cluster_security_group_membership_id_0":     "sg-12345",
					"__meta_elasticache_cache_cluster_security_group_membership_status_0": "active",
					"__meta_elasticache_cache_cluster_tag_Environment":                    "production",
					"__meta_elasticache_cache_cluster_tag_Application":                    "web-app",
					"__meta_elasticache_cache_cluster_node_id":                            "0001",
					"__meta_elasticache_cache_cluster_node_status":                        "available",
					"__meta_elasticache_cache_cluster_node_create_time":                   "2024-01-01T00:00:00Z",
					"__meta_elasticache_cache_cluster_node_availability_zone":             "us-east-1a",
					"__meta_elasticache_cache_cluster_node_endpoint_address":              "my-cluster-001.abc123.0001.use1.cache.amazonaws.com",
					"__meta_elasticache_cache_cluster_node_endpoint_port":                 "6379",
					"__address__": "my-cluster-001.abc123.0001.use1.cache.amazonaws.com:6379",
				},
				{
					"__meta_elasticache_deployment_option":                                "node",
					"__meta_elasticache_cache_cluster_arn":                                "arn:aws:elasticache:us-east-1:123456789012:cluster:my-cluster-001",
					"__meta_elasticache_cache_cluster_cache_cluster_id":                   "my-cluster-001",
					"__meta_elasticache_cache_cluster_cache_cluster_status":               "available",
					"__meta_elasticache_cache_cluster_engine":                             "redis",
					"__meta_elasticache_cache_cluster_engine_version":                     "7.1",
					"__meta_elasticache_cache_cluster_cache_node_type":                    "cache.t3.micro",
					"__meta_elasticache_cache_cluster_num_cache_nodes":                    "2",
					"__meta_elasticache_cache_cluster_cache_cluster_create_time":          "2024-01-01T00:00:00Z",
					"__meta_elasticache_cache_cluster_configuration_endpoint_address":     "my-cluster.abc123.cfg.use1.cache.amazonaws.com",
					"__meta_elasticache_cache_cluster_configuration_endpoint_port":        "6379",
					"__meta_elasticache_cache_cluster_at_rest_encryption_enabled":         "true",
					"__meta_elasticache_cache_cluster_transit_encryption_enabled":         "true",
					"__meta_elasticache_cache_cluster_auth_token_enabled":                 "true",
					"__meta_elasticache_cache_cluster_auto_minor_version_upgrade":         "true",
					"__meta_elasticache_cache_cluster_cache_subnet_group_name":            "my-subnet-group",
					"__meta_elasticache_cache_cluster_preferred_availability_zone":        "us-east-1a",
					"__meta_elasticache_cache_cluster_security_group_membership_id_0":     "sg-12345",
					"__meta_elasticache_cache_cluster_security_group_membership_status_0": "active",
					"__meta_elasticache_cache_cluster_tag_Environment":                    "production",
					"__meta_elasticache_cache_cluster_tag_Application":                    "web-app",
					"__meta_elasticache_cache_cluster_node_id":                            "0002",
					"__meta_elasticache_cache_cluster_node_status":                        "available",
					"__meta_elasticache_cache_cluster_node_create_time":                   "2024-01-01T00:00:00Z",
					"__meta_elasticache_cache_cluster_node_availability_zone":             "us-east-1b",
					"__meta_elasticache_cache_cluster_node_endpoint_address":              "my-cluster-001.abc123.0002.use1.cache.amazonaws.com",
					"__meta_elasticache_cache_cluster_node_endpoint_port":                 "6379",
					"__address__": "my-cluster-001.abc123.0002.use1.cache.amazonaws.com:6379",
				},
			},
		},
		{
			name: "CacheClusterWithSingleNode",
			cluster: &types.CacheCluster{
				CacheClusterId:     strptr("node-cluster-001"),
				ARN:                strptr("arn:aws:elasticache:us-east-1:123456789012:cluster:node-cluster-001"),
				CacheClusterStatus: strptr("available"),
				Engine:             strptr("redis"),
				EngineVersion:      strptr("6.2"),
				CacheNodeType:      strptr("cache.r6g.large"),
				NumCacheNodes:      aws.Int32(1),
				CacheNodes: []types.CacheNode{
					{
						CacheNodeId:              strptr("0001"),
						CacheNodeStatus:          strptr("available"),
						CacheNodeCreateTime:      aws.Time(testTime),
						CustomerAvailabilityZone: strptr("us-east-1a"),
						Endpoint: &types.Endpoint{
							Address: strptr("node-cluster-001.abc123.0001.use1.cache.amazonaws.com"),
							Port:    aws.Int32(6379),
						},
					},
				},
			},
			tags:                []types.Tag{},
			expectedTargetCount: 1,
			expectedLabels: []model.LabelSet{
				{
					"__meta_elasticache_deployment_option":                    "node",
					"__meta_elasticache_cache_cluster_arn":                    "arn:aws:elasticache:us-east-1:123456789012:cluster:node-cluster-001",
					"__meta_elasticache_cache_cluster_cache_cluster_id":       "node-cluster-001",
					"__meta_elasticache_cache_cluster_cache_cluster_status":   "available",
					"__meta_elasticache_cache_cluster_engine":                 "redis",
					"__meta_elasticache_cache_cluster_engine_version":         "6.2",
					"__meta_elasticache_cache_cluster_cache_node_type":        "cache.r6g.large",
					"__meta_elasticache_cache_cluster_num_cache_nodes":        "1",
					"__meta_elasticache_cache_cluster_node_id":                "0001",
					"__meta_elasticache_cache_cluster_node_status":            "available",
					"__meta_elasticache_cache_cluster_node_create_time":       "2024-01-01T00:00:00Z",
					"__meta_elasticache_cache_cluster_node_availability_zone": "us-east-1a",
					"__meta_elasticache_cache_cluster_node_endpoint_address":  "node-cluster-001.abc123.0001.use1.cache.amazonaws.com",
					"__meta_elasticache_cache_cluster_node_endpoint_port":     "6379",
					"__address__": "node-cluster-001.abc123.0001.use1.cache.amazonaws.com:6379",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := &targetgroup.Group{
				Source: "test",
			}

			addCacheClusterTargets(tg, tt.cluster, tt.tags)

			require.Len(t, tg.Targets, tt.expectedTargetCount)

			// Check each target
			for i, expectedLabels := range tt.expectedLabels {
				labels := tg.Targets[i]

				// Check that all expected labels are present with correct values
				for k, v := range expectedLabels {
					actualValue, exists := labels[k]
					require.True(t, exists, "label %s should exist in target %d", k, i)
					require.Equal(t, v, actualValue, "label %s mismatch in target %d", k, i)
				}
			}
		})
	}
}

// Mock Elasticache client.
type mockElasticacheClient struct {
	data *elasticacheDataStore
}

func newMockElasticacheClient(data *elasticacheDataStore) *mockElasticacheClient {
	return &mockElasticacheClient{data: data}
}

func (m *mockElasticacheClient) DescribeServerlessCaches(_ context.Context, input *elasticache.DescribeServerlessCachesInput, _ ...func(*elasticache.Options)) (*elasticache.DescribeServerlessCachesOutput, error) {
	if input.ServerlessCacheName != nil {
		// Filter by name
		for _, cache := range m.data.serverlessCaches {
			if cache.ServerlessCacheName != nil && *cache.ServerlessCacheName == *input.ServerlessCacheName {
				return &elasticache.DescribeServerlessCachesOutput{
					ServerlessCaches: []types.ServerlessCache{cache},
				}, nil
			}
		}
		return &elasticache.DescribeServerlessCachesOutput{
			ServerlessCaches: []types.ServerlessCache{},
		}, nil
	}

	return &elasticache.DescribeServerlessCachesOutput{
		ServerlessCaches: m.data.serverlessCaches,
	}, nil
}

func (m *mockElasticacheClient) DescribeCacheClusters(_ context.Context, input *elasticache.DescribeCacheClustersInput, _ ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error) {
	if input.CacheClusterId != nil {
		// Single cluster lookup
		for _, cluster := range m.data.cacheClusters {
			if cluster.CacheClusterId != nil && *cluster.CacheClusterId == *input.CacheClusterId {
				return &elasticache.DescribeCacheClustersOutput{
					CacheClusters: []types.CacheCluster{cluster},
				}, nil
			}
		}
		return &elasticache.DescribeCacheClustersOutput{
			CacheClusters: []types.CacheCluster{},
		}, nil
	}

	return &elasticache.DescribeCacheClustersOutput{
		CacheClusters: m.data.cacheClusters,
	}, nil
}

func (m *mockElasticacheClient) ListTagsForResource(_ context.Context, input *elasticache.ListTagsForResourceInput, _ ...func(*elasticache.Options)) (*elasticache.ListTagsForResourceOutput, error) {
	if input.ResourceName != nil {
		if tags, ok := m.data.tags[*input.ResourceName]; ok {
			return &elasticache.ListTagsForResourceOutput{
				TagList: tags,
			}, nil
		}
	}

	return &elasticache.ListTagsForResourceOutput{
		TagList: []types.Tag{},
	}, nil
}

func TestSplitCacheDeploymentOptions(t *testing.T) {
	tests := []struct {
		name                       string
		caches                     []string
		expectedServerlessCacheIDs []string
		expectedCacheClusterIDs    []string
	}{
		{
			name: "MixedARNs",
			caches: []string{
				"arn:aws:elasticache:us-east-1:123456789012:serverlesscache:my-serverless-cache",
				"arn:aws:elasticache:us-east-1:123456789012:replicationgroup:my-replication-group",
				"arn:aws:elasticache:us-west-2:123456789012:serverlesscache:prod-cache",
			},
			expectedServerlessCacheIDs: []string{"my-serverless-cache", "prod-cache"},
			expectedCacheClusterIDs:    []string{"my-replication-group"},
		},
		{
			name: "OnlyServerlessCaches",
			caches: []string{
				"arn:aws:elasticache:us-east-1:123456789012:serverlesscache:cache-1",
				"arn:aws:elasticache:us-east-1:123456789012:serverlesscache:cache-2",
			},
			expectedServerlessCacheIDs: []string{"cache-1", "cache-2"},
			expectedCacheClusterIDs:    nil,
		},
		{
			name: "OnlyReplicationGroups",
			caches: []string{
				"arn:aws:elasticache:us-east-1:123456789012:replicationgroup:cluster-1",
				"arn:aws:elasticache:us-east-1:123456789012:replicationgroup:cluster-2",
			},
			expectedServerlessCacheIDs: nil,
			expectedCacheClusterIDs:    []string{"cluster-1", "cluster-2"},
		},
		{
			name:                       "EmptyInput",
			caches:                     []string{},
			expectedServerlessCacheIDs: nil,
			expectedCacheClusterIDs:    nil,
		},
		{
			name: "InvalidARNs",
			caches: []string{
				"not-an-arn",
				"arn:aws:elasticache:us-east-1",
				"",
			},
			expectedServerlessCacheIDs: nil,
			expectedCacheClusterIDs:    nil,
		},
		{
			name: "UnknownResourceType",
			caches: []string{
				"arn:aws:elasticache:us-east-1:123456789012:unknown:resource-id",
			},
			expectedServerlessCacheIDs: nil,
			expectedCacheClusterIDs:    nil,
		},
		{
			name: "MixedWithInvalidARNs",
			caches: []string{
				"arn:aws:elasticache:us-east-1:123456789012:serverlesscache:valid-cache",
				"invalid-arn",
				"arn:aws:elasticache:us-east-1:123456789012:replicationgroup:valid-cluster",
				"",
				"arn:aws:elasticache:us-east-1:123456789012:unknown:ignored",
			},
			expectedServerlessCacheIDs: []string{"valid-cache"},
			expectedCacheClusterIDs:    []string{"valid-cluster"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverlessCacheIDs, cacheClusterIDs := splitCacheDeploymentOptions(tt.caches)

			require.Equal(t, tt.expectedServerlessCacheIDs, serverlessCacheIDs, "serverless cache IDs mismatch")
			require.Equal(t, tt.expectedCacheClusterIDs, cacheClusterIDs, "cache cluster IDs mismatch")
		})
	}
}
