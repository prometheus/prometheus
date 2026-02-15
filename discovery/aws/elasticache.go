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
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
	"golang.org/x/sync/errgroup"
)

const (
	elasticacheLabel     = model.MetaLabelPrefix + "elasticache_"
	elasticacheLabelType = elasticacheLabel + "type"

	// cache cluster
	elasticacheLabelCacheCluster                                   = elasticacheLabel + "cache_cluster_"
	elasticacheLabelCacheClusterARN                                = elasticacheLabelCacheCluster + "arn"
	elasticacheLabelCacheClusterAtRestEncryptionEnabled            = elasticacheLabelCacheCluster + "at_rest_encryption_enabled"
	elasticacheLabelCacheClusterAuthTokenEnabled                   = elasticacheLabelCacheCluster + "auth_token_enabled"
	elasticacheLabelCacheClusterAuthTokenLastModified              = elasticacheLabelCacheCluster + "auth_token_last_modified"
	elasticacheLabelCacheClusterAutoMinorVersionUpgrade            = elasticacheLabelCacheCluster + "auto_minor_version_upgrade"
	elasticacheLabelCacheClusterCreateTime                         = elasticacheLabelCacheCluster + "cache_cluster_create_time"
	elasticacheLabelCacheClusterID                                 = elasticacheLabelCacheCluster + "cache_cluster_id"
	elasticacheLabelCacheClusterStatus                             = elasticacheLabelCacheCluster + "cache_cluster_status"
	elasticacheLabelCacheClusterNodeType                           = elasticacheLabelCacheCluster + "cache_node_type"
	elasticacheLabelCacheClusterParameterGroup                     = elasticacheLabelCacheCluster + "cache_parameter_group"
	elasticacheLabelCacheClusterSubnetGroupName                    = elasticacheLabelCacheCluster + "cache_subnet_group_name"
	elasticacheLabelCacheClusterClientDownloadLandingPage          = elasticacheLabelCacheCluster + "client_download_landing_page"
	elasticacheLabelCacheClusterEngine                             = elasticacheLabelCacheCluster + "engine"
	elasticacheLabelCacheClusterEngineVersion                      = elasticacheLabelCacheCluster + "engine_version"
	elasticacheLabelCacheClusterIPDiscovery                        = elasticacheLabelCacheCluster + "ip_discovery"
	elasticacheLabelCacheClusterNetworkType                        = elasticacheLabelCacheCluster + "network_type"
	elasticacheLabelCacheClusterNumCacheNodes                      = elasticacheLabelCacheCluster + "num_cache_nodes"
	elasticacheLabelCacheClusterPreferredAvailabilityZone          = elasticacheLabelCacheCluster + "preferred_availability_zone"
	elasticacheLabelCacheClusterPreferredMaintenanceWindow         = elasticacheLabelCacheCluster + "preferred_maintenance_window"
	elasticacheLabelCacheClusterPreferredOutpostARN                = elasticacheLabelCacheCluster + "preferred_outpost_arn"
	elasticacheLabelCacheClusterReplicationGroupID                 = elasticacheLabelCacheCluster + "replication_group_id"
	elasticacheLabelCacheClusterReplicationGroupLogDeliveryEnabled = elasticacheLabelCacheCluster + "replication_group_log_delivery_enabled"
	elasticacheLabelCacheClusterSnapshotRetentionLimit             = elasticacheLabelCacheCluster + "snapshot_retention_limit"
	elasticacheLabelCacheClusterSnapshotWindow                     = elasticacheLabelCacheCluster + "snapshot_window"
	elasticacheLabelCacheClusterTransitEncryptionEnabled           = elasticacheLabelCacheCluster + "transit_encryption_enabled"
	elasticacheLabelCacheClusterTransitEncryptionMode              = elasticacheLabelCacheCluster + "transit_encryption_mode"

	// configuration endpoint
	elasticacheLabelCacheClusterConfigurationEndpoint        = elasticacheLabelCacheCluster + "configuration_endpoint_"
	elasticacheLabelCacheClusterConfigurationEndpointAddress = elasticacheLabelCacheClusterConfigurationEndpoint + "address"
	elasticacheLabelCacheClusterConfigurationEndpointPort    = elasticacheLabelCacheClusterConfigurationEndpoint + "port"

	// notification
	elasticacheLabelCacheClusterNotification            = elasticacheLabelCacheCluster + "notification_"
	elasticacheLabelCacheClusterNotificationTopicARN    = elasticacheLabelCacheClusterNotification + "topic_arn"
	elasticacheLabelCacheClusterNotificationTopicStatus = elasticacheLabelCacheClusterNotification + "topic_status"

	// log delivery configuration (slice - use with index)
	elasticacheLabelCacheClusterLogDeliveryConfiguration                = elasticacheLabelCacheCluster + "log_delivery_configuration_"
	elasticacheLabelCacheClusterLogDeliveryConfigurationDestinationType = elasticacheLabelCacheClusterLogDeliveryConfiguration + "destination_type"
	elasticacheLabelCacheClusterLogDeliveryConfigurationLogFormat       = elasticacheLabelCacheClusterLogDeliveryConfiguration + "log_format"
	elasticacheLabelCacheClusterLogDeliveryConfigurationLogType         = elasticacheLabelCacheClusterLogDeliveryConfiguration + "log_type"
	elasticacheLabelCacheClusterLogDeliveryConfigurationStatus          = elasticacheLabelCacheClusterLogDeliveryConfiguration + "status"
	elasticacheLabelCacheClusterLogDeliveryConfigurationMessage         = elasticacheLabelCacheClusterLogDeliveryConfiguration + "message"
	elasticacheLabelCacheClusterLogDeliveryConfigurationLogGroup        = elasticacheLabelCacheClusterLogDeliveryConfiguration + "log_group"
	elasticacheLabelCacheClusterLogDeliveryConfigurationDeliveryStream  = elasticacheLabelCacheClusterLogDeliveryConfiguration + "delivery_stream"

	// pending modified values
	elasticacheLabelCacheClusterPendingModifiedValues                         = elasticacheLabelCacheCluster + "pending_modified_values_"
	elasticacheLabelCacheClusterPendingModifiedValuesAuthTokenStatus          = elasticacheLabelCacheClusterPendingModifiedValues + "auth_token_status"
	elasticacheLabelCacheClusterPendingModifiedValuesCacheNodeType            = elasticacheLabelCacheClusterPendingModifiedValues + "cache_node_type"
	elasticacheLabelCacheClusterPendingModifiedValuesEngineVersion            = elasticacheLabelCacheClusterPendingModifiedValues + "engine_version"
	elasticacheLabelCacheClusterPendingModifiedValuesNumCacheNodes            = elasticacheLabelCacheClusterPendingModifiedValues + "num_cache_nodes"
	elasticacheLabelCacheClusterPendingModifiedValuesTransitEncryptionEnabled = elasticacheLabelCacheClusterPendingModifiedValues + "transit_encryption_enabled"
	elasticacheLabelCacheClusterPendingModifiedValuesTransitEncryptionMode    = elasticacheLabelCacheClusterPendingModifiedValues + "transit_encryption_mode"
	elasticacheLabelCacheClusterPendingModifiedValuesCacheNodeIdsToRemove     = elasticacheLabelCacheClusterPendingModifiedValues + "cache_node_ids_to_remove"

	// security group membership (slice - use with index)
	elasticacheLabelCacheClusterSecurityGroupMembership       = elasticacheLabelCacheCluster + "security_group_membership_"
	elasticacheLabelCacheClusterSecurityGroupMembershipID     = elasticacheLabelCacheClusterSecurityGroupMembership + "id"
	elasticacheLabelCacheClusterSecurityGroupMembershipStatus = elasticacheLabelCacheClusterSecurityGroupMembership + "status"

	// tags - create one label per tag key, with the format: elasticache_cache_cluster_tag_<tagkey>
	elasticacheLabelCacheClusterTag = elasticacheLabelCacheCluster + "tag_"

	// node
	elasticacheLabelCacheNode                     = elasticacheLabel + "cache_node_"
	elasticacheLabelCacheNodeCreateTime           = elasticacheLabelCacheNode + "create_time"
	elasticacheLabelCacheNodeID                   = elasticacheLabelCacheNode + "id"
	elasticacheLabelCacheNodeStatus               = elasticacheLabelCacheNode + "status"
	elasticacheLabelCacheNodeAZ                   = elasticacheLabelCacheNode + "availability_zone"
	elasticacheLabelCacheNodeCustomerOutpostARN   = elasticacheLabelCacheNode + "customer_outpost_arn"
	elasticacheLabelCacheNodeSourceCacheNodeID    = elasticacheLabelCacheNode + "source_cache_node_id"
	elasticacheLabelCacheNodeParameterGroupStatus = elasticacheLabelCacheNode + "parameter_group_status"

	// endpoint
	elasticacheLabelCacheNodeEndpoint        = elasticacheLabelCacheNode + "endpoint_"
	elasticacheLabelCacheNodeEndpointAddress = elasticacheLabelCacheNodeEndpoint + "address"
	elasticacheLabelCacheNodeEndpointPort    = elasticacheLabelCacheNodeEndpoint + "port"

	// serverless cache
	elasticacheLabelServerlessCache                       = elasticacheLabel + "serverless_cache_"
	elasticacheLabelServerlessCacheARN                    = elasticacheLabelServerlessCache + "arn"
	elasticacheLabelServerlessCacheName                   = elasticacheLabelServerlessCache + "name"
	elasticacheLabelServerlessCacheCreateTime             = elasticacheLabelServerlessCache + "create_time"
	elasticacheLabelServerlessCacheDescription            = elasticacheLabelServerlessCache + "description"
	elasticacheLabelServerlessCacheEngine                 = elasticacheLabelServerlessCache + "engine"
	elasticacheLabelServerlessCacheFullEngineVersion      = elasticacheLabelServerlessCache + "full_engine_version"
	elasticacheLabelServerlessCacheMajorEngineVersion     = elasticacheLabelServerlessCache + "major_engine_version"
	elasticacheLabelServerlessCacheStatus                 = elasticacheLabelServerlessCache + "status"
	elasticacheLabelServerlessCacheKmsKeyID               = elasticacheLabelServerlessCache + "kms_key_id"
	elasticacheLabelServerlessCacheUserGroupID            = elasticacheLabelServerlessCache + "user_group_id"
	elasticacheLabelServerlessCacheDailySnapshotTime      = elasticacheLabelServerlessCache + "daily_snapshot_time"
	elasticacheLabelServerlessCacheSnapshotRetentionLimit = elasticacheLabelServerlessCache + "snapshot_retention_limit"

	// endpoint
	elasticacheLabelServerlessCacheEndpoint              = elasticacheLabelServerlessCache + "endpoint_"
	elasticacheLabelServerlessCacheEndpointAddress       = elasticacheLabelServerlessCacheEndpoint + "address"
	elasticacheLabelServerlessCacheEndpointPort          = elasticacheLabelServerlessCacheEndpoint + "port"
	elasticacheLabelServerlessCacheReaderEndpointAddress = elasticacheLabelServerlessCacheEndpoint + "reader_address"
	elasticacheLabelServerlessCacheReaderEndpointPort    = elasticacheLabelServerlessCacheEndpoint + "reader_port"

	// security group membership (slice - use with index)
	elasticacheLabelServerlessCacheSecurityGroupID = elasticacheLabelServerlessCache + "security_group_id"

	// Subnet group membership (slice - use with index)
	elasticacheLabelServerlessCacheSubnetID = elasticacheLabelServerlessCache + "subnet_id"

	// cache usage limits
	elasticacheLabelServerlessCacheCacheUsageLimit                        = elasticacheLabelServerlessCache + "cache_usage_limit_"
	elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorage        = elasticacheLabelServerlessCacheCacheUsageLimit + "data_storage"
	elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorageMaximum = elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorage + "maximum"
	elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorageMinimum = elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorage + "minimum"
	elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorageUnit    = elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorage + "unit"
	elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecond           = elasticacheLabelServerlessCacheCacheUsageLimit + "ecpu_per_second"
	elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecondMaximum    = elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecond + "maximum"
	elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecondMinimum    = elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecond + "minimum"

	// tags - create one label per tag key, with the format: elasticache_serverless_cache_tag_<tagkey>
	elasticacheLabelServerlessCacheTag = elasticacheLabelServerlessCache + "tag_"
)

// DefaultElasticacheSDConfig is the default Elasticache SD configuration.
var DefaultElasticacheSDConfig = ElasticacheSDConfig{
	Port:               80,
	RefreshInterval:    model.Duration(60 * time.Second),
	RequestConcurrency: 20,
	HTTPClientConfig:   config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&ElasticacheSDConfig{})
}

// ElasticacheSDConfig is the configuration for Elasticache based service discovery.
type ElasticacheSDConfig struct {
	Region          string         `yaml:"region"`
	Endpoint        string         `yaml:"endpoint"`
	AccessKey       string         `yaml:"access_key,omitempty"`
	SecretKey       config.Secret  `yaml:"secret_key,omitempty"`
	Profile         string         `yaml:"profile,omitempty"`
	RoleARN         string         `yaml:"role_arn,omitempty"`
	Clusters        []string       `yaml:"clusters,omitempty"`
	Port            int            `yaml:"port"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`

	// RequestConcurrency controls the maximum number of concurrent Elasticache API requests.
	RequestConcurrency int `yaml:"request_concurrency,omitempty"`

	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*ElasticacheSDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &elasticacheMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the Elasticache Config.
func (*ElasticacheSDConfig) Name() string { return "elasticache" }

// NewDiscoverer returns a Discoverer for the Elasticache Config.
func (c *ElasticacheSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewElasticacheDiscovery(c, opts)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the Elasticache Config.
func (c *ElasticacheSDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultElasticacheSDConfig
	type plain ElasticacheSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	c.Region, err = loadRegion(context.Background(), c.Region)
	if err != nil {
		return fmt.Errorf("could not determine AWS region: %w", err)
	}

	return c.HTTPClientConfig.Validate()
}

type elasticacheClient interface {
	DescribeServerlessCaches(ctx context.Context, params *elasticache.DescribeServerlessCachesInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeServerlessCachesOutput, error)
	DescribeCacheClusters(ctx context.Context, params *elasticache.DescribeCacheClustersInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error)
	ListTagsForResource(ctx context.Context, params *elasticache.ListTagsForResourceInput, optFns ...func(*elasticache.Options)) (*elasticache.ListTagsForResourceOutput, error)
}

// ElasticacheDiscovery periodically performs Elasticache-SD requests.
// It implements the Discoverer interface.
type ElasticacheDiscovery struct {
	*refresh.Discovery
	logger            *slog.Logger
	cfg               *ElasticacheSDConfig
	elasticacheClient elasticacheClient
}

// NewElasticacheDiscovery returns a new ElasticacheDiscovery which periodically refreshes its targets.
func NewElasticacheDiscovery(conf *ElasticacheSDConfig, opts discovery.DiscovererOptions) (*ElasticacheDiscovery, error) {
	m, ok := opts.Metrics.(*elasticacheMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if opts.Logger == nil {
		opts.Logger = promslog.NewNopLogger()
	}
	d := &ElasticacheDiscovery{
		logger: opts.Logger,
		cfg:    conf,
	}
	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "elasticache",
			Interval:            time.Duration(d.cfg.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *ElasticacheDiscovery) initElasticacheClient(ctx context.Context) error {
	if d.elasticacheClient != nil {
		return nil
	}

	if d.cfg.Region == "" {
		return errors.New("region must be set for Elasticache service discovery")
	}

	// Build the HTTP client from the provided HTTPClientConfig.
	client, err := config.NewClientFromConfig(d.cfg.HTTPClientConfig, "elasticache_sd")
	if err != nil {
		return err
	}

	// Build the AWS config with the provided region.
	var configOptions []func(*awsConfig.LoadOptions) error
	configOptions = append(configOptions, awsConfig.WithRegion(d.cfg.Region))
	configOptions = append(configOptions, awsConfig.WithHTTPClient(client))

	// Only set static credentials if both access key and secret key are provided
	// Otherwise, let AWS SDK use its default credential chain
	if d.cfg.AccessKey != "" && d.cfg.SecretKey != "" {
		credProvider := credentials.NewStaticCredentialsProvider(d.cfg.AccessKey, string(d.cfg.SecretKey), "")
		configOptions = append(configOptions, awsConfig.WithCredentialsProvider(credProvider))
	}

	if d.cfg.Profile != "" {
		configOptions = append(configOptions, awsConfig.WithSharedConfigProfile(d.cfg.Profile))
	}

	cfg, err := awsConfig.LoadDefaultConfig(ctx, configOptions...)
	if err != nil {
		d.logger.Error("Failed to create AWS config", "error", err)
		return fmt.Errorf("could not create aws config: %w", err)
	}

	// If the role ARN is set, assume the role to get credentials and set the credentials provider in the config.
	if d.cfg.RoleARN != "" {
		assumeProvider := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), d.cfg.RoleARN)
		cfg.Credentials = aws.NewCredentialsCache(assumeProvider)
	}

	d.elasticacheClient = elasticache.NewFromConfig(cfg, func(options *elasticache.Options) {
		if d.cfg.Endpoint != "" {
			options.BaseEndpoint = &d.cfg.Endpoint
		}
		options.HTTPClient = client
	})

	d.elasticacheClient = elasticache.NewFromConfig(cfg, func(options *elasticache.Options) {
		options.HTTPClient = client
	})

	// Test credentials by making a simple API call
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err = d.elasticacheClient.DescribeCacheClusters(testCtx, &elasticache.DescribeCacheClustersInput{})
	if err != nil {
		d.logger.Error("Failed to test Elasticache credentials", "error", err)
		return fmt.Errorf("Elasticache credential test failed: %w", err)
	}

	return nil
}

// describeServerlessCaches calls DescribeServerlessCaches API for the given cache IDs (or all caches if no IDs are provided) and returns the list of serverless caches.
func (d *ElasticacheDiscovery) describeServerlessCaches(ctx context.Context, caches []string) ([]types.ServerlessCache, error) {
	mu := &sync.Mutex{}
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	var serverlessCaches []types.ServerlessCache
	if len(caches) == 0 {
		errg.Go(func() error {
			var nextToken *string
			for {
				output, err := d.elasticacheClient.DescribeServerlessCaches(ectx, &elasticache.DescribeServerlessCachesInput{
					MaxResults: aws.Int32(50),
					NextToken:  nextToken,
				})
				if err != nil {
					return fmt.Errorf("failed to describe serverless caches: %w", err)
				}
				mu.Lock()
				serverlessCaches = append(serverlessCaches, output.ServerlessCaches...)
				mu.Unlock()
				if output.NextToken == nil {
					break
				}
				nextToken = output.NextToken
			}
			return nil
		})
	} else {
		for _, cacheID := range caches {
			errg.Go(func() error {
				output, err := d.elasticacheClient.DescribeServerlessCaches(ectx, &elasticache.DescribeServerlessCachesInput{
					ServerlessCacheName: aws.String(cacheID),
				})
				if err != nil {
					return fmt.Errorf("failed to describe serverless cache %s: %w", cacheID, err)
				}
				mu.Lock()
				serverlessCaches = append(serverlessCaches, output.ServerlessCaches...)
				mu.Unlock()
				return nil
			})
		}
	}

	return serverlessCaches, errg.Wait()
}

// describeCacheClusters calls DescribeCacheClusters API for the given cache cluster IDs (or all cache clusters if no IDs are provided) and returns the list of cache clusters.
func (d *ElasticacheDiscovery) describeCacheClusters(ctx context.Context, caches []string) ([]types.CacheCluster, error) {
	mu := &sync.Mutex{}
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	showCacheClustersNotInReplicationGroupsBools := []bool{false, true}
	var cacheClusters []types.CacheCluster
	if len(caches) == 0 {
		for _, showCacheClustersNotInReplicationGroupsBool := range showCacheClustersNotInReplicationGroupsBools {
			errg.Go(func() error {
				var nextToken *string
				for {
					output, err := d.elasticacheClient.DescribeCacheClusters(ectx, &elasticache.DescribeCacheClustersInput{
						MaxRecords:                              aws.Int32(100),
						Marker:                                  nextToken,
						ShowCacheNodeInfo:                       aws.Bool(true),
						ShowCacheClustersNotInReplicationGroups: aws.Bool(showCacheClustersNotInReplicationGroupsBool),
					})
					if err != nil {
						return fmt.Errorf("failed to describe cache clusters: %w", err)
					}
					mu.Lock()
					cacheClusters = append(cacheClusters, output.CacheClusters...)
					mu.Unlock()
					if output.Marker == nil {
						break
					}
					nextToken = output.Marker
				}
				return nil
			})
		}
	} else {
		for _, cacheID := range caches {
			for _, showCacheClustersNotInReplicationGroupsBool := range showCacheClustersNotInReplicationGroupsBools {
				errg.Go(func() error {
					output, err := d.elasticacheClient.DescribeCacheClusters(ectx, &elasticache.DescribeCacheClustersInput{
						CacheClusterId:                          aws.String(cacheID),
						ShowCacheNodeInfo:                       aws.Bool(true),
						ShowCacheClustersNotInReplicationGroups: aws.Bool(showCacheClustersNotInReplicationGroupsBool),
						MaxRecords:                              aws.Int32(100),
						Marker:                                  nil,
					})
					if err != nil {
						return fmt.Errorf("failed to describe cache cluster %s: %w", cacheID, err)
					}
					mu.Lock()
					cacheClusters = append(cacheClusters, output.CacheClusters...)
					mu.Unlock()
					return nil
				})
			}
		}
	}

	return cacheClusters, errg.Wait()
}

// listTagsForResource calls ListTagsForResource API for the given resource ARNs and returns a map of resource ARN to list of tags.
func (d *ElasticacheDiscovery) listTagsForResource(ctx context.Context, resourceARNs []string) (map[string][]types.Tag, error) {
	mu := &sync.Mutex{}
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	tagsByResourceARN := make(map[string][]types.Tag)
	for _, resourceARN := range resourceARNs {
		errg.Go(func() error {
			output, err := d.elasticacheClient.ListTagsForResource(ectx, &elasticache.ListTagsForResourceInput{
				ResourceName: aws.String(resourceARN),
			})
			if err != nil {
				return fmt.Errorf("failed to list tags for resource %s: %w", resourceARN, err)
			}
			mu.Lock()
			tagsByResourceARN[resourceARN] = output.TagList
			mu.Unlock()
			return nil
		})
	}
	return tagsByResourceARN, errg.Wait()
}

func (d *ElasticacheDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	err := d.initElasticacheClient(ctx)
	if err != nil {
		return nil, err
	}

	var clusters []string
	serverlessCacheIDs, cacheClusterIDs := splitCacheTypes(d.cfg.Clusters)

	clusterErrg, clusterCtx := errgroup.WithContext(ctx)
	clusterErrg.SetLimit(2)
	clusterErrg.Go(func() error {
		caches, err := d.describeServerlessCaches(clusterCtx, serverlessCacheIDs)
		if err != nil {
			return fmt.Errorf("failed to describe serverless caches: %w", err)
		}
		for _, cache := range caches {
			clusters = append(clusters, *cache.ARN)
		}
		return nil
	})

	clusterErrg.Go(func() error {
		cacheClusters, err := d.describeCacheClusters(clusterCtx, cacheClusterIDs)
		if err != nil {
			return fmt.Errorf("failed to describe cache clusters: %w", err)
		}
		for _, cluster := range cacheClusters {
			clusters = append(clusters, *cluster.ARN)
		}
		return nil
	})

	if err := clusterErrg.Wait(); err != nil {
		return nil, err
	}

	tagsByResourceARN, err := d.listTagsForResource(ctx, clusters)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags for resources: %w", err)
	}

	tg := &targetgroup.Group{
		Source: d.cfg.Region,
	}

	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(2)
	errg.Go(func() error {
		caches, err := d.describeServerlessCaches(ectx, serverlessCacheIDs)
		if err != nil {
			return fmt.Errorf("failed to describe serverless caches: %w", err)
		}
		for _, cache := range caches {
			addServerlessCacheTargets(tg, &cache, tagsByResourceARN[*cache.ARN])
		}
		return nil
	})

	errg.Go(func() error {
		cacheClusters, err := d.describeCacheClusters(ectx, cacheClusterIDs)
		if err != nil {
			return fmt.Errorf("failed to describe cache clusters: %w", err)
		}
		for _, cluster := range cacheClusters {
			addCacheClusterTargets(tg, &cluster, tagsByResourceARN[*cluster.ARN])
		}
		return nil
	})

	if err := errg.Wait(); err != nil {
		return nil, err
	}

	return []*targetgroup.Group{tg}, nil
}

// splitCacheTypes takes a list of cache ARNs and splits them into serverless cache IDs and cache cluster IDs based on their format.
// Serverless caches are in the format arn:aws:elasticache:<REGION>:<ACCOUNT_ID>:serverlesscache:<CACHE_NAME>
// Cache clusters are in the format arn:aws:elasticache:<REGION>:<ACCOUNT_ID>:replicationgroup:<CACHE_CLUSTER_ID>
func splitCacheTypes(caches []string) (serverlessCacheIDs []string, cacheClusterIDs []string) {

	for _, cacheARN := range caches {
		if len(cacheARN) == 0 {
			continue
		}
		parts := strings.Split(cacheARN, ":")
		if len(parts) < 6 {
			continue
		}
		resourceType := parts[5]
		resourceID := parts[6]
		switch resourceType {
		case "serverlesscache":
			serverlessCacheIDs = append(serverlessCacheIDs, resourceID)
		case "replicationgroup":
			cacheClusterIDs = append(cacheClusterIDs, resourceID)
		default:
			continue
		}
	}
	return serverlessCacheIDs, cacheClusterIDs
}

// addServerlessCacheTargets adds targets for a serverless cache to the target group.
func addServerlessCacheTargets(tg *targetgroup.Group, cache *types.ServerlessCache, tags []types.Tag) {
	labels := model.LabelSet{
		elasticacheLabelType:                              model.LabelValue("serverless_cache"),
		elasticacheLabelServerlessCacheARN:                model.LabelValue(*cache.ARN),
		elasticacheLabelServerlessCacheName:               model.LabelValue(*cache.ServerlessCacheName),
		elasticacheLabelServerlessCacheStatus:             model.LabelValue(*cache.Status),
		elasticacheLabelServerlessCacheEngine:             model.LabelValue(*cache.Engine),
		elasticacheLabelServerlessCacheFullEngineVersion:  model.LabelValue(*cache.FullEngineVersion),
		elasticacheLabelServerlessCacheMajorEngineVersion: model.LabelValue(*cache.MajorEngineVersion),
	}

	if cache.Description != nil {
		labels[elasticacheLabelServerlessCacheDescription] = model.LabelValue(*cache.Description)
	}

	if cache.CreateTime != nil {
		labels[elasticacheLabelServerlessCacheCreateTime] = model.LabelValue(cache.CreateTime.Format(time.RFC3339))
	}

	if cache.KmsKeyId != nil {
		labels[elasticacheLabelServerlessCacheKmsKeyID] = model.LabelValue(*cache.KmsKeyId)
	}

	if cache.UserGroupId != nil {
		labels[elasticacheLabelServerlessCacheUserGroupID] = model.LabelValue(*cache.UserGroupId)
	}

	if cache.DailySnapshotTime != nil {
		labels[elasticacheLabelServerlessCacheDailySnapshotTime] = model.LabelValue(*cache.DailySnapshotTime)
	}

	if cache.SnapshotRetentionLimit != nil {
		labels[elasticacheLabelServerlessCacheSnapshotRetentionLimit] = model.LabelValue(fmt.Sprintf("%d", *cache.SnapshotRetentionLimit))
	}

	if cache.Endpoint != nil {
		if cache.Endpoint.Address != nil {
			labels[elasticacheLabelServerlessCacheEndpointAddress] = model.LabelValue(*cache.Endpoint.Address)
		}
		if cache.Endpoint.Port != nil {
			labels[elasticacheLabelServerlessCacheEndpointPort] = model.LabelValue(fmt.Sprintf("%d", *cache.Endpoint.Port))
		}
	}

	if cache.ReaderEndpoint != nil {
		if cache.ReaderEndpoint.Address != nil {
			labels[elasticacheLabelServerlessCacheReaderEndpointAddress] = model.LabelValue(*cache.ReaderEndpoint.Address)
		}
		if cache.ReaderEndpoint.Port != nil {
			labels[elasticacheLabelServerlessCacheReaderEndpointPort] = model.LabelValue(fmt.Sprintf("%d", *cache.ReaderEndpoint.Port))
		}
	}

	for i, sgID := range cache.SecurityGroupIds {
		labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelServerlessCacheSecurityGroupID, i))] = model.LabelValue(sgID)
	}

	for i, subnetID := range cache.SubnetIds {
		labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelServerlessCacheSubnetID, i))] = model.LabelValue(subnetID)
	}

	if cache.CacheUsageLimits != nil {
		if cache.CacheUsageLimits.DataStorage != nil {
			if cache.CacheUsageLimits.DataStorage.Maximum != nil {
				labels[elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorageMaximum] = model.LabelValue(fmt.Sprintf("%d", *cache.CacheUsageLimits.DataStorage.Maximum))
			}
			if cache.CacheUsageLimits.DataStorage.Minimum != nil {
				labels[elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorageMinimum] = model.LabelValue(fmt.Sprintf("%d", *cache.CacheUsageLimits.DataStorage.Minimum))
			}
			labels[elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorageUnit] = model.LabelValue(cache.CacheUsageLimits.DataStorage.Unit)
		}
		if cache.CacheUsageLimits.ECPUPerSecond != nil {
			if cache.CacheUsageLimits.ECPUPerSecond.Maximum != nil {
				labels[elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecondMaximum] = model.LabelValue(fmt.Sprintf("%d", *cache.CacheUsageLimits.ECPUPerSecond.Maximum))
			}
			if cache.CacheUsageLimits.ECPUPerSecond.Minimum != nil {
				labels[elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecondMinimum] = model.LabelValue(fmt.Sprintf("%d", *cache.CacheUsageLimits.ECPUPerSecond.Minimum))
			}
		}
	}

	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			labels[model.LabelName(elasticacheLabelServerlessCacheTag+strutil.SanitizeLabelName(*tag.Key))] = model.LabelValue(*tag.Value)
		}
	}

	// Set the address label using the endpoint
	if cache.Endpoint != nil && cache.Endpoint.Address != nil && cache.Endpoint.Port != nil {
		labels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(*cache.Endpoint.Address, strconv.Itoa(int(*cache.Endpoint.Port))))
	}

	tg.Targets = append(tg.Targets, labels)
}

// addCacheClusterTargets adds targets for a cache cluster to the target group.
func addCacheClusterTargets(tg *targetgroup.Group, cluster *types.CacheCluster, tags []types.Tag) {
	labels := model.LabelSet{
		elasticacheLabelType:               model.LabelValue("cache_cluster"),
		elasticacheLabelCacheClusterARN:    model.LabelValue(*cluster.ARN),
		elasticacheLabelCacheClusterID:     model.LabelValue(*cluster.CacheClusterId),
		elasticacheLabelCacheClusterStatus: model.LabelValue(*cluster.CacheClusterStatus),
	}

	if cluster.AtRestEncryptionEnabled != nil {
		labels[elasticacheLabelCacheClusterAtRestEncryptionEnabled] = model.LabelValue(fmt.Sprintf("%t", *cluster.AtRestEncryptionEnabled))
	}

	if cluster.AuthTokenEnabled != nil {
		labels[elasticacheLabelCacheClusterAuthTokenEnabled] = model.LabelValue(fmt.Sprintf("%t", *cluster.AuthTokenEnabled))
	}

	if cluster.AuthTokenLastModifiedDate != nil {
		labels[elasticacheLabelCacheClusterAuthTokenLastModified] = model.LabelValue(cluster.AuthTokenLastModifiedDate.Format(time.RFC3339))
	}

	if cluster.AutoMinorVersionUpgrade != nil {
		labels[elasticacheLabelCacheClusterAutoMinorVersionUpgrade] = model.LabelValue(fmt.Sprintf("%t", *cluster.AutoMinorVersionUpgrade))
	}

	if cluster.CacheClusterCreateTime != nil {
		labels[elasticacheLabelCacheClusterCreateTime] = model.LabelValue(cluster.CacheClusterCreateTime.Format(time.RFC3339))
	}

	if cluster.CacheNodeType != nil {
		labels[elasticacheLabelCacheClusterNodeType] = model.LabelValue(*cluster.CacheNodeType)
	}

	if cluster.CacheParameterGroup != nil && cluster.CacheParameterGroup.CacheParameterGroupName != nil {
		labels[elasticacheLabelCacheClusterParameterGroup] = model.LabelValue(*cluster.CacheParameterGroup.CacheParameterGroupName)
	}

	if cluster.CacheSubnetGroupName != nil {
		labels[elasticacheLabelCacheClusterSubnetGroupName] = model.LabelValue(*cluster.CacheSubnetGroupName)
	}

	if cluster.ClientDownloadLandingPage != nil {
		labels[elasticacheLabelCacheClusterClientDownloadLandingPage] = model.LabelValue(*cluster.ClientDownloadLandingPage)
	}

	if cluster.ConfigurationEndpoint != nil {
		if cluster.ConfigurationEndpoint.Address != nil {
			labels[elasticacheLabelCacheClusterConfigurationEndpointAddress] = model.LabelValue(*cluster.ConfigurationEndpoint.Address)
		}
		if cluster.ConfigurationEndpoint.Port != nil {
			labels[elasticacheLabelCacheClusterConfigurationEndpointPort] = model.LabelValue(fmt.Sprintf("%d", *cluster.ConfigurationEndpoint.Port))
		}
	}

	if cluster.Engine != nil {
		labels[elasticacheLabelCacheClusterEngine] = model.LabelValue(*cluster.Engine)
	}

	if cluster.EngineVersion != nil {
		labels[elasticacheLabelCacheClusterEngineVersion] = model.LabelValue(*cluster.EngineVersion)
	}

	if len(cluster.IpDiscovery) > 0 {
		labels[elasticacheLabelCacheClusterIPDiscovery] = model.LabelValue(cluster.IpDiscovery)
	}

	if len(cluster.NetworkType) > 0 {
		labels[elasticacheLabelCacheClusterNetworkType] = model.LabelValue(cluster.NetworkType)
	}

	if cluster.NotificationConfiguration != nil {
		if cluster.NotificationConfiguration.TopicArn != nil {
			labels[elasticacheLabelCacheClusterNotificationTopicARN] = model.LabelValue(*cluster.NotificationConfiguration.TopicArn)
		}
		if cluster.NotificationConfiguration.TopicStatus != nil {
			labels[elasticacheLabelCacheClusterNotificationTopicStatus] = model.LabelValue(*cluster.NotificationConfiguration.TopicStatus)
		}
	}

	if cluster.NumCacheNodes != nil {
		labels[elasticacheLabelCacheClusterNumCacheNodes] = model.LabelValue(fmt.Sprintf("%d", *cluster.NumCacheNodes))
	}

	if cluster.PreferredAvailabilityZone != nil {
		labels[elasticacheLabelCacheClusterPreferredAvailabilityZone] = model.LabelValue(*cluster.PreferredAvailabilityZone)
	}

	if cluster.PreferredMaintenanceWindow != nil {
		labels[elasticacheLabelCacheClusterPreferredMaintenanceWindow] = model.LabelValue(*cluster.PreferredMaintenanceWindow)
	}

	if cluster.PreferredOutpostArn != nil {
		labels[elasticacheLabelCacheClusterPreferredOutpostARN] = model.LabelValue(*cluster.PreferredOutpostArn)
	}

	if cluster.ReplicationGroupId != nil {
		labels[elasticacheLabelCacheClusterReplicationGroupID] = model.LabelValue(*cluster.ReplicationGroupId)
	}

	if cluster.ReplicationGroupLogDeliveryEnabled != nil {
		labels[elasticacheLabelCacheClusterReplicationGroupLogDeliveryEnabled] = model.LabelValue(fmt.Sprintf("%t", *cluster.ReplicationGroupLogDeliveryEnabled))
	}

	if cluster.SnapshotRetentionLimit != nil {
		labels[elasticacheLabelCacheClusterSnapshotRetentionLimit] = model.LabelValue(fmt.Sprintf("%d", *cluster.SnapshotRetentionLimit))
	}

	if cluster.SnapshotWindow != nil {
		labels[elasticacheLabelCacheClusterSnapshotWindow] = model.LabelValue(*cluster.SnapshotWindow)
	}

	if cluster.TransitEncryptionEnabled != nil {
		labels[elasticacheLabelCacheClusterTransitEncryptionEnabled] = model.LabelValue(fmt.Sprintf("%t", *cluster.TransitEncryptionEnabled))
	}

	if len(cluster.TransitEncryptionMode) > 0 {
		labels[elasticacheLabelCacheClusterTransitEncryptionMode] = model.LabelValue(cluster.TransitEncryptionMode)
	}

	// Log delivery configurations (slice)
	for i, logDelivery := range cluster.LogDeliveryConfigurations {
		if len(logDelivery.DestinationType) > 0 {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationDestinationType, i))] = model.LabelValue(logDelivery.DestinationType)
		}
		if len(logDelivery.LogFormat) > 0 {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationLogFormat, i))] = model.LabelValue(logDelivery.LogFormat)
		}
		if len(logDelivery.LogType) > 0 {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationLogType, i))] = model.LabelValue(logDelivery.LogType)
		}
		if len(logDelivery.Status) > 0 {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationStatus, i))] = model.LabelValue(logDelivery.Status)
		}
		if logDelivery.Message != nil {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationMessage, i))] = model.LabelValue(*logDelivery.Message)
		}
		if logDelivery.DestinationDetails != nil {
			if logDelivery.DestinationDetails.CloudWatchLogsDetails != nil && logDelivery.DestinationDetails.CloudWatchLogsDetails.LogGroup != nil {
				labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationLogGroup, i))] = model.LabelValue(*logDelivery.DestinationDetails.CloudWatchLogsDetails.LogGroup)
			}
			if logDelivery.DestinationDetails.KinesisFirehoseDetails != nil && logDelivery.DestinationDetails.KinesisFirehoseDetails.DeliveryStream != nil {
				labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationDeliveryStream, i))] = model.LabelValue(*logDelivery.DestinationDetails.KinesisFirehoseDetails.DeliveryStream)
			}
		}
	}

	// Pending modified values
	if cluster.PendingModifiedValues != nil {
		if len(cluster.PendingModifiedValues.AuthTokenStatus) > 0 {
			labels[elasticacheLabelCacheClusterPendingModifiedValuesAuthTokenStatus] = model.LabelValue(cluster.PendingModifiedValues.AuthTokenStatus)
		}
		if cluster.PendingModifiedValues.CacheNodeType != nil {
			labels[elasticacheLabelCacheClusterPendingModifiedValuesCacheNodeType] = model.LabelValue(*cluster.PendingModifiedValues.CacheNodeType)
		}
		if cluster.PendingModifiedValues.EngineVersion != nil {
			labels[elasticacheLabelCacheClusterPendingModifiedValuesEngineVersion] = model.LabelValue(*cluster.PendingModifiedValues.EngineVersion)
		}
		if cluster.PendingModifiedValues.NumCacheNodes != nil {
			labels[elasticacheLabelCacheClusterPendingModifiedValuesNumCacheNodes] = model.LabelValue(fmt.Sprintf("%d", *cluster.PendingModifiedValues.NumCacheNodes))
		}
		if cluster.PendingModifiedValues.TransitEncryptionEnabled != nil {
			labels[elasticacheLabelCacheClusterPendingModifiedValuesTransitEncryptionEnabled] = model.LabelValue(fmt.Sprintf("%t", *cluster.PendingModifiedValues.TransitEncryptionEnabled))
		}
		if len(cluster.PendingModifiedValues.TransitEncryptionMode) > 0 {
			labels[elasticacheLabelCacheClusterPendingModifiedValuesTransitEncryptionMode] = model.LabelValue(cluster.PendingModifiedValues.TransitEncryptionMode)
		}
		if len(cluster.PendingModifiedValues.CacheNodeIdsToRemove) > 0 {
			labels[elasticacheLabelCacheClusterPendingModifiedValuesCacheNodeIdsToRemove] = model.LabelValue(strings.Join(cluster.PendingModifiedValues.CacheNodeIdsToRemove, ","))
		}
	}

	// Security group membership (slice)
	for i, sg := range cluster.SecurityGroups {
		if sg.SecurityGroupId != nil {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterSecurityGroupMembershipID, i))] = model.LabelValue(*sg.SecurityGroupId)
		}
		if sg.Status != nil {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterSecurityGroupMembershipStatus, i))] = model.LabelValue(*sg.Status)
		}
	}

	// Cache nodes
	for i, node := range cluster.CacheNodes {
		if node.CacheNodeId != nil {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheNodeID, i))] = model.LabelValue(*node.CacheNodeId)
		}
		if node.CacheNodeStatus != nil {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheNodeStatus, i))] = model.LabelValue(*node.CacheNodeStatus)
		}
		if node.CacheNodeCreateTime != nil {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheNodeCreateTime, i))] = model.LabelValue(node.CacheNodeCreateTime.Format(time.RFC3339))
		}
		if node.CustomerAvailabilityZone != nil {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheNodeAZ, i))] = model.LabelValue(*node.CustomerAvailabilityZone)
		}
		if node.CustomerOutpostArn != nil {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheNodeCustomerOutpostARN, i))] = model.LabelValue(*node.CustomerOutpostArn)
		}
		if node.SourceCacheNodeId != nil {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheNodeSourceCacheNodeID, i))] = model.LabelValue(*node.SourceCacheNodeId)
		}
		if node.ParameterGroupStatus != nil {
			labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheNodeParameterGroupStatus, i))] = model.LabelValue(*node.ParameterGroupStatus)
		}
		if node.Endpoint != nil {
			if node.Endpoint.Address != nil {
				labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheNodeEndpointAddress, i))] = model.LabelValue(*node.Endpoint.Address)
			}
			if node.Endpoint.Port != nil {
				labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheNodeEndpointPort, i))] = model.LabelValue(fmt.Sprintf("%d", *node.Endpoint.Port))
			}
		}
	}

	// Tags
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			labels[model.LabelName(elasticacheLabelCacheClusterTag+strutil.SanitizeLabelName(*tag.Key))] = model.LabelValue(*tag.Value)
		}
	}

	// Set the address label using configuration endpoint if available, otherwise first cache node endpoint
	if cluster.ConfigurationEndpoint != nil && cluster.ConfigurationEndpoint.Address != nil && cluster.ConfigurationEndpoint.Port != nil {
		labels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(*cluster.ConfigurationEndpoint.Address, strconv.Itoa(int(*cluster.ConfigurationEndpoint.Port))))
	} else if len(cluster.CacheNodes) > 0 && cluster.CacheNodes[0].Endpoint != nil && cluster.CacheNodes[0].Endpoint.Address != nil && cluster.CacheNodes[0].Endpoint.Port != nil {
		labels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(*cluster.CacheNodes[0].Endpoint.Address, strconv.Itoa(int(*cluster.CacheNodes[0].Endpoint.Port))))
	}

	tg.Targets = append(tg.Targets, labels)
}
