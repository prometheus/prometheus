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
	"maps"
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
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	elasticacheLabel                 = model.MetaLabelPrefix + "elasticache_"
	elasticacheLabelDeploymentOption = elasticacheLabel + "deployment_option"

	// cache cluster.
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

	// configuration endpoint.
	elasticacheLabelCacheClusterConfigurationEndpoint        = elasticacheLabelCacheCluster + "configuration_endpoint_"
	elasticacheLabelCacheClusterConfigurationEndpointAddress = elasticacheLabelCacheClusterConfigurationEndpoint + "address"
	elasticacheLabelCacheClusterConfigurationEndpointPort    = elasticacheLabelCacheClusterConfigurationEndpoint + "port"

	// notification.
	elasticacheLabelCacheClusterNotification            = elasticacheLabelCacheCluster + "notification_"
	elasticacheLabelCacheClusterNotificationTopicARN    = elasticacheLabelCacheClusterNotification + "topic_arn"
	elasticacheLabelCacheClusterNotificationTopicStatus = elasticacheLabelCacheClusterNotification + "topic_status"

	// log delivery configuration (slice - use with index).
	elasticacheLabelCacheClusterLogDeliveryConfiguration                = elasticacheLabelCacheCluster + "log_delivery_configuration_"
	elasticacheLabelCacheClusterLogDeliveryConfigurationDestinationType = elasticacheLabelCacheClusterLogDeliveryConfiguration + "destination_type"
	elasticacheLabelCacheClusterLogDeliveryConfigurationLogFormat       = elasticacheLabelCacheClusterLogDeliveryConfiguration + "log_format"
	elasticacheLabelCacheClusterLogDeliveryConfigurationLogType         = elasticacheLabelCacheClusterLogDeliveryConfiguration + "log_type"
	elasticacheLabelCacheClusterLogDeliveryConfigurationStatus          = elasticacheLabelCacheClusterLogDeliveryConfiguration + "status"
	elasticacheLabelCacheClusterLogDeliveryConfigurationMessage         = elasticacheLabelCacheClusterLogDeliveryConfiguration + "message"
	elasticacheLabelCacheClusterLogDeliveryConfigurationLogGroup        = elasticacheLabelCacheClusterLogDeliveryConfiguration + "log_group"
	elasticacheLabelCacheClusterLogDeliveryConfigurationDeliveryStream  = elasticacheLabelCacheClusterLogDeliveryConfiguration + "delivery_stream"

	// pending modified values.
	elasticacheLabelCacheClusterPendingModifiedValues                         = elasticacheLabelCacheCluster + "pending_modified_values_"
	elasticacheLabelCacheClusterPendingModifiedValuesAuthTokenStatus          = elasticacheLabelCacheClusterPendingModifiedValues + "auth_token_status"
	elasticacheLabelCacheClusterPendingModifiedValuesCacheNodeType            = elasticacheLabelCacheClusterPendingModifiedValues + "cache_node_type"
	elasticacheLabelCacheClusterPendingModifiedValuesEngineVersion            = elasticacheLabelCacheClusterPendingModifiedValues + "engine_version"
	elasticacheLabelCacheClusterPendingModifiedValuesNumCacheNodes            = elasticacheLabelCacheClusterPendingModifiedValues + "num_cache_nodes"
	elasticacheLabelCacheClusterPendingModifiedValuesTransitEncryptionEnabled = elasticacheLabelCacheClusterPendingModifiedValues + "transit_encryption_enabled"
	elasticacheLabelCacheClusterPendingModifiedValuesTransitEncryptionMode    = elasticacheLabelCacheClusterPendingModifiedValues + "transit_encryption_mode"
	elasticacheLabelCacheClusterPendingModifiedValuesCacheNodeIDsToRemove     = elasticacheLabelCacheClusterPendingModifiedValues + "cache_node_ids_to_remove"

	// security group membership (slice - use with index).
	elasticacheLabelCacheClusterSecurityGroupMembership       = elasticacheLabelCacheCluster + "security_group_membership_"
	elasticacheLabelCacheClusterSecurityGroupMembershipID     = elasticacheLabelCacheClusterSecurityGroupMembership + "id"
	elasticacheLabelCacheClusterSecurityGroupMembershipStatus = elasticacheLabelCacheClusterSecurityGroupMembership + "status"

	// tags - create one label per tag key, with the format: elasticache_cache_cluster_tag_<tagkey>.
	elasticacheLabelCacheClusterTag = elasticacheLabelCacheCluster + "tag_"

	// node.
	elasticacheLabelCacheClusterNode                     = elasticacheLabelCacheCluster + "node_"
	elasticacheLabelCacheClusterNodeCreateTime           = elasticacheLabelCacheClusterNode + "create_time"
	elasticacheLabelCacheClusterNodeID                   = elasticacheLabelCacheClusterNode + "id"
	elasticacheLabelCacheClusterNodeStatus               = elasticacheLabelCacheClusterNode + "status"
	elasticacheLabelCacheClusterNodeAZ                   = elasticacheLabelCacheClusterNode + "availability_zone"
	elasticacheLabelCacheClusterNodeCustomerOutpostARN   = elasticacheLabelCacheClusterNode + "customer_outpost_arn"
	elasticacheLabelCacheClusterNodeSourceCacheNodeID    = elasticacheLabelCacheClusterNode + "source_cache_node_id"
	elasticacheLabelCacheClusterNodeParameterGroupStatus = elasticacheLabelCacheClusterNode + "parameter_group_status"

	// endpoint.
	elasticacheLabelCacheClusterNodeEndpoint        = elasticacheLabelCacheClusterNode + "endpoint_"
	elasticacheLabelCacheClusterNodeEndpointAddress = elasticacheLabelCacheClusterNodeEndpoint + "address"
	elasticacheLabelCacheClusterNodeEndpointPort    = elasticacheLabelCacheClusterNodeEndpoint + "port"

	// serverless cache.
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

	// endpoint.
	elasticacheLabelServerlessCacheEndpoint              = elasticacheLabelServerlessCache + "endpoint_"
	elasticacheLabelServerlessCacheEndpointAddress       = elasticacheLabelServerlessCacheEndpoint + "address"
	elasticacheLabelServerlessCacheEndpointPort          = elasticacheLabelServerlessCacheEndpoint + "port"
	elasticacheLabelServerlessCacheReaderEndpointAddress = elasticacheLabelServerlessCacheEndpoint + "reader_address"
	elasticacheLabelServerlessCacheReaderEndpointPort    = elasticacheLabelServerlessCacheEndpoint + "reader_port"

	// security group membership (slice - use with index).
	elasticacheLabelServerlessCacheSecurityGroupID = elasticacheLabelServerlessCache + "security_group_id"

	// Subnet group membership (slice - use with index).
	elasticacheLabelServerlessCacheSubnetID = elasticacheLabelServerlessCache + "subnet_id"

	// cache usage limits.
	elasticacheLabelServerlessCacheCacheUsageLimit                        = elasticacheLabelServerlessCache + "cache_usage_limit_"
	elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorage        = elasticacheLabelServerlessCacheCacheUsageLimit + "data_storage"
	elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorageMaximum = elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorage + "maximum"
	elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorageMinimum = elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorage + "minimum"
	elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorageUnit    = elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorage + "unit"
	elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecond           = elasticacheLabelServerlessCacheCacheUsageLimit + "ecpu_per_second"
	elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecondMaximum    = elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecond + "maximum"
	elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecondMinimum    = elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecond + "minimum"

	// tags - create one label per tag key, with the format: elasticache_serverless_cache_tag_<tagkey>.
	elasticacheLabelServerlessCacheTag = elasticacheLabelServerlessCache + "tag_"
)

// DefaultElasticacheSDConfig is the default Elasticache SD configuration.
var DefaultElasticacheSDConfig = ElasticacheSDConfig{
	Port:               80,
	RefreshInterval:    model.Duration(60 * time.Second),
	RequestConcurrency: 10,
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
	ExternalID      string         `yaml:"external_id,omitempty"`
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

// SetDirectory joins any relative file paths with dir.
func (c *ElasticacheSDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the Elasticache Config.
// Region resolution is deferred to initElasticacheClient; see loadRegion.
func (c *ElasticacheSDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultElasticacheSDConfig
	type plain ElasticacheSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	return c.HTTPClientConfig.Validate()
}

type elasticacheClient interface {
	DescribeServerlessCaches(ctx context.Context, params *elasticache.DescribeServerlessCachesInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeServerlessCachesOutput, error)
	DescribeCacheClusters(ctx context.Context, params *elasticache.DescribeCacheClustersInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error)
	ListTagsForResource(ctx context.Context, params *elasticache.ListTagsForResourceInput, optFns ...func(*elasticache.Options)) (*elasticache.ListTagsForResourceOutput, error)
}

// elasticacheClientAdapter captures only the ElastiCache API calls AWS
// discovery uses as method-value closures, keeping the concrete
// *elasticache.Client out of any interface-boxed struct field. See
// ec2ClientAdapter for the full rationale: this stops the linker from retaining
// the entire ElastiCache API surface (~2.5 MB).
type elasticacheClientAdapter struct {
	describeServerlessCaches func(ctx context.Context, params *elasticache.DescribeServerlessCachesInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeServerlessCachesOutput, error)
	describeCacheClusters    func(ctx context.Context, params *elasticache.DescribeCacheClustersInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error)
	listTagsForResource      func(ctx context.Context, params *elasticache.ListTagsForResourceInput, optFns ...func(*elasticache.Options)) (*elasticache.ListTagsForResourceOutput, error)
}

func newElastiCacheClientAdapter(c *elasticache.Client) elasticacheClientAdapter {
	return elasticacheClientAdapter{
		describeServerlessCaches: c.DescribeServerlessCaches,
		describeCacheClusters:    c.DescribeCacheClusters,
		listTagsForResource:      c.ListTagsForResource,
	}
}

func (a elasticacheClientAdapter) DescribeServerlessCaches(ctx context.Context, params *elasticache.DescribeServerlessCachesInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeServerlessCachesOutput, error) {
	return a.describeServerlessCaches(ctx, params, optFns...)
}

func (a elasticacheClientAdapter) DescribeCacheClusters(ctx context.Context, params *elasticache.DescribeCacheClustersInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error) {
	return a.describeCacheClusters(ctx, params, optFns...)
}

func (a elasticacheClientAdapter) ListTagsForResource(ctx context.Context, params *elasticache.ListTagsForResourceInput, optFns ...func(*elasticache.Options)) (*elasticache.ListTagsForResourceOutput, error) {
	return a.listTagsForResource(ctx, params, optFns...)
}

// ElasticacheDiscovery periodically performs Elasticache-SD requests.
// It implements the Discoverer interface.
type ElasticacheDiscovery struct {
	*refresh.Discovery
	logger            *slog.Logger
	cfg               *ElasticacheSDConfig
	elasticacheClient elasticacheClient

	// region is the resolved region used for the AWS client and for the
	// Source label. Lazily populated by initElasticacheClient.
	region string
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

	// Build the HTTP client from the provided HTTPClientConfig.
	client, err := config.NewClientFromConfig(d.cfg.HTTPClientConfig, "elasticache_sd")
	if err != nil {
		return err
	}

	// Resolve the region lazily. See ElasticacheSDConfig.UnmarshalYAML.
	d.region, err = loadRegion(ctx, d.cfg.Region)
	if err != nil {
		return err
	}

	// Build the AWS config with the resolved region.
	var configOptions []func(*awsConfig.LoadOptions) error
	configOptions = append(configOptions, awsConfig.WithRegion(d.region))
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
		assumeProvider := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), d.cfg.RoleARN, func(o *stscreds.AssumeRoleOptions) {
			if d.cfg.ExternalID != "" {
				o.ExternalID = aws.String(d.cfg.ExternalID)
			}
		})
		cfg.Credentials = aws.NewCredentialsCache(assumeProvider)
	}

	d.elasticacheClient = newElastiCacheClientAdapter(elasticache.NewFromConfig(cfg, func(options *elasticache.Options) {
		if d.cfg.Endpoint != "" {
			options.BaseEndpoint = &d.cfg.Endpoint
		}
		options.HTTPClient = client
	}))

	// Test credentials by making a simple API call
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err = d.elasticacheClient.DescribeCacheClusters(testCtx, &elasticache.DescribeCacheClustersInput{})
	if err != nil {
		d.logger.Error("Failed to test Elasticache credentials", "error", err)
		return fmt.Errorf("elasticache credential test failed: %w", err)
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
					MaxResults:          aws.Int32(50),
					NextToken:           nil,
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
						MaxRecords:                              aws.Int32(100),
						Marker:                                  nil,
						ShowCacheNodeInfo:                       aws.Bool(true),
						ShowCacheClustersNotInReplicationGroups: aws.Bool(showCacheClustersNotInReplicationGroupsBool),
						CacheClusterId:                          aws.String(cacheID),
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
	clustersMu := sync.Mutex{}
	serverlessCacheIDs, cacheClusterIDs := splitCacheDeploymentOptions(d.cfg.Clusters)

	clusterErrg, clusterCtx := errgroup.WithContext(ctx)
	clusterErrg.Go(func() error {
		caches, err := d.describeServerlessCaches(clusterCtx, serverlessCacheIDs)
		if err != nil {
			return fmt.Errorf("failed to describe serverless caches: %w", err)
		}
		for _, cache := range caches {
			clustersMu.Lock()
			clusters = append(clusters, *cache.ARN)
			clustersMu.Unlock()
		}
		return nil
	})

	clusterErrg.Go(func() error {
		cacheClusters, err := d.describeCacheClusters(clusterCtx, cacheClusterIDs)
		if err != nil {
			return fmt.Errorf("failed to describe cache clusters: %w", err)
		}
		for _, cluster := range cacheClusters {
			clustersMu.Lock()
			clusters = append(clusters, *cluster.ARN)
			clustersMu.Unlock()
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
		Source: d.region,
	}

	errg, ectx := errgroup.WithContext(ctx)
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
// Cache clusters are in the format arn:aws:elasticache:<REGION>:<ACCOUNT_ID>:replicationgroup:<CACHE_CLUSTER_ID>.
func splitCacheDeploymentOptions(caches []string) (serverlessCacheIDs, cacheClusterIDs []string) {
	for _, cacheARN := range caches {
		if cacheARN == "" {
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
		elasticacheLabelDeploymentOption:                  model.LabelValue("serverless"),
		elasticacheLabelServerlessCacheARN:                model.LabelValue(*cache.ARN),
		elasticacheLabelServerlessCacheName:               model.LabelValue(*cache.ServerlessCacheName),
		elasticacheLabelServerlessCacheStatus:             model.LabelValue(*cache.Status),
		elasticacheLabelServerlessCacheEngine:             model.LabelValue(*cache.Engine),
		elasticacheLabelServerlessCacheFullEngineVersion:  model.LabelValue(*cache.FullEngineVersion),
		elasticacheLabelServerlessCacheMajorEngineVersion: model.LabelValue(*cache.MajorEngineVersion),
	}

	setStringLabel(labels, elasticacheLabelServerlessCacheDescription, cache.Description)
	setTimeLabel(labels, elasticacheLabelServerlessCacheCreateTime, cache.CreateTime)
	setStringLabel(labels, elasticacheLabelServerlessCacheKmsKeyID, cache.KmsKeyId)
	setStringLabel(labels, elasticacheLabelServerlessCacheUserGroupID, cache.UserGroupId)
	setStringLabel(labels, elasticacheLabelServerlessCacheDailySnapshotTime, cache.DailySnapshotTime)
	setIntLabel(labels, elasticacheLabelServerlessCacheSnapshotRetentionLimit, cache.SnapshotRetentionLimit)

	if cache.Endpoint != nil {
		setStringLabel(labels, elasticacheLabelServerlessCacheEndpointAddress, cache.Endpoint.Address)
		setIntLabel(labels, elasticacheLabelServerlessCacheEndpointPort, cache.Endpoint.Port)
	}

	if cache.ReaderEndpoint != nil {
		setStringLabel(labels, elasticacheLabelServerlessCacheReaderEndpointAddress, cache.ReaderEndpoint.Address)
		setIntLabel(labels, elasticacheLabelServerlessCacheReaderEndpointPort, cache.ReaderEndpoint.Port)
	}

	for i, sgID := range cache.SecurityGroupIds {
		labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelServerlessCacheSecurityGroupID, i))] = model.LabelValue(sgID)
	}

	for i, subnetID := range cache.SubnetIds {
		labels[model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelServerlessCacheSubnetID, i))] = model.LabelValue(subnetID)
	}

	if cache.CacheUsageLimits != nil {
		if cache.CacheUsageLimits.DataStorage != nil {
			setIntLabel(labels, elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorageMaximum, cache.CacheUsageLimits.DataStorage.Maximum)
			setIntLabel(labels, elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorageMinimum, cache.CacheUsageLimits.DataStorage.Minimum)
			labels[elasticacheLabelServerlessCacheCacheUsageLimitCacheDataStorageUnit] = model.LabelValue(cache.CacheUsageLimits.DataStorage.Unit)
		}
		if cache.CacheUsageLimits.ECPUPerSecond != nil {
			setIntLabel(labels, elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecondMaximum, cache.CacheUsageLimits.ECPUPerSecond.Maximum)
			setIntLabel(labels, elasticacheLabelServerlessCacheCacheUsageLimitECPUPerSecondMinimum, cache.CacheUsageLimits.ECPUPerSecond.Minimum)
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
// Creates one target per cache node for individual scraping.
func addCacheClusterTargets(tg *targetgroup.Group, cluster *types.CacheCluster, tags []types.Tag) {
	// Build common labels that apply to all nodes in this cluster
	commonLabels := model.LabelSet{
		elasticacheLabelDeploymentOption:   model.LabelValue("node"),
		elasticacheLabelCacheClusterARN:    model.LabelValue(*cluster.ARN),
		elasticacheLabelCacheClusterID:     model.LabelValue(*cluster.CacheClusterId),
		elasticacheLabelCacheClusterStatus: model.LabelValue(*cluster.CacheClusterStatus),
	}

	setBoolLabel(commonLabels, elasticacheLabelCacheClusterAtRestEncryptionEnabled, cluster.AtRestEncryptionEnabled)
	setBoolLabel(commonLabels, elasticacheLabelCacheClusterAuthTokenEnabled, cluster.AuthTokenEnabled)
	setTimeLabel(commonLabels, elasticacheLabelCacheClusterAuthTokenLastModified, cluster.AuthTokenLastModifiedDate)
	setBoolLabel(commonLabels, elasticacheLabelCacheClusterAutoMinorVersionUpgrade, cluster.AutoMinorVersionUpgrade)
	setTimeLabel(commonLabels, elasticacheLabelCacheClusterCreateTime, cluster.CacheClusterCreateTime)
	setStringLabel(commonLabels, elasticacheLabelCacheClusterNodeType, cluster.CacheNodeType)

	if cluster.CacheParameterGroup != nil {
		setStringLabel(commonLabels, elasticacheLabelCacheClusterParameterGroup, cluster.CacheParameterGroup.CacheParameterGroupName)
	}

	setStringLabel(commonLabels, elasticacheLabelCacheClusterSubnetGroupName, cluster.CacheSubnetGroupName)
	setStringLabel(commonLabels, elasticacheLabelCacheClusterClientDownloadLandingPage, cluster.ClientDownloadLandingPage)

	if cluster.ConfigurationEndpoint != nil {
		setStringLabel(commonLabels, elasticacheLabelCacheClusterConfigurationEndpointAddress, cluster.ConfigurationEndpoint.Address)
		setIntLabel(commonLabels, elasticacheLabelCacheClusterConfigurationEndpointPort, cluster.ConfigurationEndpoint.Port)
	}

	setStringLabel(commonLabels, elasticacheLabelCacheClusterEngine, cluster.Engine)
	setStringLabel(commonLabels, elasticacheLabelCacheClusterEngineVersion, cluster.EngineVersion)
	setNonEmptyLabel(commonLabels, elasticacheLabelCacheClusterIPDiscovery, cluster.IpDiscovery)
	setNonEmptyLabel(commonLabels, elasticacheLabelCacheClusterNetworkType, cluster.NetworkType)

	if cluster.NotificationConfiguration != nil {
		setStringLabel(commonLabels, elasticacheLabelCacheClusterNotificationTopicARN, cluster.NotificationConfiguration.TopicArn)
		setStringLabel(commonLabels, elasticacheLabelCacheClusterNotificationTopicStatus, cluster.NotificationConfiguration.TopicStatus)
	}

	setIntLabel(commonLabels, elasticacheLabelCacheClusterNumCacheNodes, cluster.NumCacheNodes)
	setStringLabel(commonLabels, elasticacheLabelCacheClusterPreferredAvailabilityZone, cluster.PreferredAvailabilityZone)
	setStringLabel(commonLabels, elasticacheLabelCacheClusterPreferredMaintenanceWindow, cluster.PreferredMaintenanceWindow)
	setStringLabel(commonLabels, elasticacheLabelCacheClusterPreferredOutpostARN, cluster.PreferredOutpostArn)
	setStringLabel(commonLabels, elasticacheLabelCacheClusterReplicationGroupID, cluster.ReplicationGroupId)
	setBoolLabel(commonLabels, elasticacheLabelCacheClusterReplicationGroupLogDeliveryEnabled, cluster.ReplicationGroupLogDeliveryEnabled)
	setIntLabel(commonLabels, elasticacheLabelCacheClusterSnapshotRetentionLimit, cluster.SnapshotRetentionLimit)
	setStringLabel(commonLabels, elasticacheLabelCacheClusterSnapshotWindow, cluster.SnapshotWindow)
	setBoolLabel(commonLabels, elasticacheLabelCacheClusterTransitEncryptionEnabled, cluster.TransitEncryptionEnabled)
	setNonEmptyLabel(commonLabels, elasticacheLabelCacheClusterTransitEncryptionMode, cluster.TransitEncryptionMode)

	// Log delivery configurations (slice)
	for i, logDelivery := range cluster.LogDeliveryConfigurations {
		setNonEmptyLabel(commonLabels, model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationDestinationType, i)), logDelivery.DestinationType)
		setNonEmptyLabel(commonLabels, model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationLogFormat, i)), logDelivery.LogFormat)
		setNonEmptyLabel(commonLabels, model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationLogType, i)), logDelivery.LogType)
		setNonEmptyLabel(commonLabels, model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationStatus, i)), logDelivery.Status)
		setStringLabel(commonLabels, model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationMessage, i)), logDelivery.Message)
		if logDelivery.DestinationDetails != nil {
			if logDelivery.DestinationDetails.CloudWatchLogsDetails != nil {
				setStringLabel(commonLabels, model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationLogGroup, i)), logDelivery.DestinationDetails.CloudWatchLogsDetails.LogGroup)
			}
			if logDelivery.DestinationDetails.KinesisFirehoseDetails != nil {
				setStringLabel(commonLabels, model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterLogDeliveryConfigurationDeliveryStream, i)), logDelivery.DestinationDetails.KinesisFirehoseDetails.DeliveryStream)
			}
		}
	}

	// Pending modified values
	if cluster.PendingModifiedValues != nil {
		setNonEmptyLabel(commonLabels, elasticacheLabelCacheClusterPendingModifiedValuesAuthTokenStatus, cluster.PendingModifiedValues.AuthTokenStatus)
		setStringLabel(commonLabels, elasticacheLabelCacheClusterPendingModifiedValuesCacheNodeType, cluster.PendingModifiedValues.CacheNodeType)
		setStringLabel(commonLabels, elasticacheLabelCacheClusterPendingModifiedValuesEngineVersion, cluster.PendingModifiedValues.EngineVersion)
		setIntLabel(commonLabels, elasticacheLabelCacheClusterPendingModifiedValuesNumCacheNodes, cluster.PendingModifiedValues.NumCacheNodes)
		setBoolLabel(commonLabels, elasticacheLabelCacheClusterPendingModifiedValuesTransitEncryptionEnabled, cluster.PendingModifiedValues.TransitEncryptionEnabled)
		setNonEmptyLabel(commonLabels, elasticacheLabelCacheClusterPendingModifiedValuesTransitEncryptionMode, cluster.PendingModifiedValues.TransitEncryptionMode)
		if len(cluster.PendingModifiedValues.CacheNodeIdsToRemove) > 0 {
			commonLabels[elasticacheLabelCacheClusterPendingModifiedValuesCacheNodeIDsToRemove] = model.LabelValue(strings.Join(cluster.PendingModifiedValues.CacheNodeIdsToRemove, ","))
		}
	}

	// Security group membership (slice)
	for i, sg := range cluster.SecurityGroups {
		setStringLabel(commonLabels, model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterSecurityGroupMembershipID, i)), sg.SecurityGroupId)
		setStringLabel(commonLabels, model.LabelName(fmt.Sprintf("%s_%d", elasticacheLabelCacheClusterSecurityGroupMembershipStatus, i)), sg.Status)
	}

	// Tags
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			commonLabels[model.LabelName(elasticacheLabelCacheClusterTag+strutil.SanitizeLabelName(*tag.Key))] = model.LabelValue(*tag.Value)
		}
	}

	// Create one target per cache node
	for _, node := range cluster.CacheNodes {
		// Clone common labels for this node
		labels := make(model.LabelSet, len(commonLabels))
		maps.Copy(labels, commonLabels)

		// Add node-specific labels
		setStringLabel(labels, elasticacheLabelCacheClusterNodeID, node.CacheNodeId)
		setStringLabel(labels, elasticacheLabelCacheClusterNodeStatus, node.CacheNodeStatus)
		setTimeLabel(labels, elasticacheLabelCacheClusterNodeCreateTime, node.CacheNodeCreateTime)
		setStringLabel(labels, elasticacheLabelCacheClusterNodeAZ, node.CustomerAvailabilityZone)
		setStringLabel(labels, elasticacheLabelCacheClusterNodeCustomerOutpostARN, node.CustomerOutpostArn)
		setStringLabel(labels, elasticacheLabelCacheClusterNodeSourceCacheNodeID, node.SourceCacheNodeId)
		setStringLabel(labels, elasticacheLabelCacheClusterNodeParameterGroupStatus, node.ParameterGroupStatus)

		if node.Endpoint != nil {
			setStringLabel(labels, elasticacheLabelCacheClusterNodeEndpointAddress, node.Endpoint.Address)
			setIntLabel(labels, elasticacheLabelCacheClusterNodeEndpointPort, node.Endpoint.Port)

			// Set the address label to this node's endpoint
			if node.Endpoint.Address != nil && node.Endpoint.Port != nil {
				labels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(*node.Endpoint.Address, strconv.Itoa(int(*node.Endpoint.Port))))
			}
		}

		tg.Targets = append(tg.Targets, labels)
	}
}
