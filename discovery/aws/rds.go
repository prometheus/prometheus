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
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
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
	rdsLabel = model.MetaLabelPrefix + "rds_"

	// DB cluster labels.
	rdsLabelCluster                                       = rdsLabel + "cluster_"
	rdsLabelClusterActivityStreamKinesisStreamName        = rdsLabelCluster + "activity_stream_kinesis_stream_name"
	rdsLabelClusterActivityStreamKMSKeyID                 = rdsLabelCluster + "activity_stream_kms_key_id"
	rdsLabelClusterActivityStreamMode                     = rdsLabelCluster + "activity_stream_mode"
	rdsLabelClusterActivityStreamStatus                   = rdsLabelCluster + "activity_stream_status"
	rdsLabelClusterAllocatedStorage                       = rdsLabelCluster + "allocated_storage"
	rdsLabelClusterAutoMinorVersionUpgrade                = rdsLabelCluster + "auto_minor_version_upgrade"
	rdsLabelClusterAutomaticRestartTime                   = rdsLabelCluster + "automatic_restart_time"
	rdsLabelClusterAwsBackupRecoveryPointArn              = rdsLabelCluster + "aws_backup_recovery_point_arn"
	rdsLabelClusterBacktrackConsumedChangeRecords         = rdsLabelCluster + "backtrack_consumed_change_records"
	rdsLabelClusterBacktrackWindow                        = rdsLabelCluster + "backtrack_window"
	rdsLabelClusterBackupRetentionPeriod                  = rdsLabelCluster + "backup_retention_period"
	rdsLabelClusterCapacity                               = rdsLabelCluster + "capacity"
	rdsLabelClusterCharacterSetName                       = rdsLabelCluster + "character_set_name"
	rdsLabelClusterCloneGroupID                           = rdsLabelCluster + "clone_group_id"
	rdsLabelClusterClusterCreateTime                      = rdsLabelCluster + "cluster_create_time"
	rdsLabelClusterClusterScalabilityType                 = rdsLabelCluster + "cluster_scalability_type"
	rdsLabelClusterCopyTagsToSnapshot                     = rdsLabelCluster + "copy_tags_to_snapshot"
	rdsLabelClusterCrossAccountClone                      = rdsLabelCluster + "cross_account_clone"
	rdsLabelClusterDBClusterArn                           = rdsLabelCluster + "arn"
	rdsLabelClusterDBClusterIdentifier                    = rdsLabelCluster + "identifier"
	rdsLabelClusterDBClusterInstanceClass                 = rdsLabelCluster + "instance_class"
	rdsLabelClusterDBClusterParameterGroup                = rdsLabelCluster + "parameter_group"
	rdsLabelClusterDBSubnetGroup                          = rdsLabelCluster + "subnet_group"
	rdsLabelClusterDBSystemID                             = rdsLabelCluster + "db_system_id"
	rdsLabelClusterDatabaseInsightsMode                   = rdsLabelCluster + "database_insights_mode"
	rdsLabelClusterDatabaseName                           = rdsLabelCluster + "database_name"
	rdsLabelClusterDBClusterResourceID                    = rdsLabelCluster + "resource_id"
	rdsLabelClusterDeletionProtection                     = rdsLabelCluster + "deletion_protection"
	rdsLabelClusterEarliestBacktrackTime                  = rdsLabelCluster + "earliest_backtrack_time"
	rdsLabelClusterEarliestRestorableTime                 = rdsLabelCluster + "earliest_restorable_time"
	rdsLabelClusterEndpoint                               = rdsLabelCluster + "endpoint"
	rdsLabelClusterEngine                                 = rdsLabelCluster + "engine"
	rdsLabelClusterEngineLifecycleSupport                 = rdsLabelCluster + "engine_lifecycle_support"
	rdsLabelClusterEngineMode                             = rdsLabelCluster + "engine_mode"
	rdsLabelClusterEngineVersion                          = rdsLabelCluster + "engine_version"
	rdsLabelClusterGlobalClusterIdentifier                = rdsLabelCluster + "global_cluster_identifier"
	rdsLabelClusterGlobalWriteForwardingRequested         = rdsLabelCluster + "global_write_forwarding_requested"
	rdsLabelClusterGlobalWriteForwardingStatus            = rdsLabelCluster + "global_write_forwarding_status"
	rdsLabelClusterHostedZoneID                           = rdsLabelCluster + "hosted_zone_id"
	rdsLabelClusterHTTPEndpointEnabled                    = rdsLabelCluster + "http_endpoint_enabled"
	rdsLabelClusterIAMDatabaseAuthenticationEnabled       = rdsLabelCluster + "iam_database_authentication_enabled"
	rdsLabelClusterIOOptimizedNextAllowedModificationTime = rdsLabelCluster + "io_optimized_next_allowed_modification_time"
	rdsLabelClusterIops                                   = rdsLabelCluster + "iops"
	rdsLabelClusterKMSKeyID                               = rdsLabelCluster + "kms_key_id"
	rdsLabelClusterLatestRestorableTime                   = rdsLabelCluster + "latest_restorable_time"
	rdsLabelClusterLocalWriteForwardingStatus             = rdsLabelCluster + "local_write_forwarding_status"
	rdsLabelClusterMasterUsername                         = rdsLabelCluster + "master_username"
	rdsLabelClusterMonitoringInterval                     = rdsLabelCluster + "monitoring_interval"
	rdsLabelClusterMonitoringRoleArn                      = rdsLabelCluster + "monitoring_role_arn"
	rdsLabelClusterMultiAZ                                = rdsLabelCluster + "multi_az"
	rdsLabelClusterNetworkType                            = rdsLabelCluster + "network_type"
	rdsLabelClusterPercentProgress                        = rdsLabelCluster + "percent_progress"
	rdsLabelClusterPerformanceInsightsEnabled             = rdsLabelCluster + "performance_insights_enabled"
	rdsLabelClusterPerformanceInsightsKMSKeyID            = rdsLabelCluster + "performance_insights_kms_key_id"
	rdsLabelClusterPerformanceInsightsRetentionPeriod     = rdsLabelCluster + "performance_insights_retention_period"
	rdsLabelClusterPort                                   = rdsLabelCluster + "port"
	rdsLabelClusterPreferredBackupWindow                  = rdsLabelCluster + "preferred_backup_window"
	rdsLabelClusterPreferredMaintenanceWindow             = rdsLabelCluster + "preferred_maintenance_window"
	rdsLabelClusterPubliclyAccessible                     = rdsLabelCluster + "publicly_accessible"
	rdsLabelClusterReaderEndpoint                         = rdsLabelCluster + "reader_endpoint"
	rdsLabelClusterReplicationSourceIdentifier            = rdsLabelCluster + "replication_source_identifier"
	rdsLabelClusterServerlessV2PlatformVersion            = rdsLabelCluster + "serverless_v2_platform_version"
	rdsLabelClusterStatus                                 = rdsLabelCluster + "status"
	rdsLabelClusterStorageEncrypted                       = rdsLabelCluster + "storage_encrypted"
	rdsLabelClusterStorageEncryptionType                  = rdsLabelCluster + "storage_encryption_type"
	rdsLabelClusterStorageThroughput                      = rdsLabelCluster + "storage_throughput"
	rdsLabelClusterStorageType                            = rdsLabelCluster + "storage_type"
	rdsLabelClusterUpgradeRolloutOrder                    = rdsLabelCluster + "upgrade_rollout_order"

	// DB cluster tags - create one label per tag key, with the format: rds_cluster_tag_<tagkey>.
	rdsLabelClusterTag = rdsLabelCluster + "tag_"

	// DB instance labels.
	rdsLabelInstance                                              = rdsLabel + "instance_"
	rdsLabelInstanceIsClusterWriter                               = rdsLabelInstance + "is_cluster_writer"
	rdsLabelInstanceActivityStreamEngineNativeAuditFieldsIncluded = rdsLabelInstance + "activity_stream_engine_native_audit_fields_included"
	rdsLabelInstanceActivityStreamKinesisStreamName               = rdsLabelInstance + "activity_stream_kinesis_stream_name"
	rdsLabelInstanceActivityStreamKmsKeyID                        = rdsLabelInstance + "activity_stream_kms_key_id"
	rdsLabelInstanceActivityStreamMode                            = rdsLabelInstance + "activity_stream_mode"
	rdsLabelInstanceActivityStreamPolicyStatus                    = rdsLabelInstance + "activity_stream_policy_status"
	rdsLabelInstanceActivityStreamStatus                          = rdsLabelInstance + "activity_stream_status"
	rdsLabelInstanceAllocatedStorage                              = rdsLabelInstance + "allocated_storage"
	rdsLabelInstanceAutoMinorVersionUpgrade                       = rdsLabelInstance + "auto_minor_version_upgrade"
	rdsLabelInstanceAutomaticRestartTime                          = rdsLabelInstance + "automatic_restart_time"
	rdsLabelInstanceAutomationMode                                = rdsLabelInstance + "automation_mode"
	rdsLabelInstanceAvailabilityZone                              = rdsLabelInstance + "availability_zone"
	rdsLabelInstanceAwsBackupRecoveryPointArn                     = rdsLabelInstance + "aws_backup_recovery_point_arn"
	rdsLabelInstanceBackupRetentionPeriod                         = rdsLabelInstance + "backup_retention_period"
	rdsLabelInstanceBackupTarget                                  = rdsLabelInstance + "backup_target"
	rdsLabelInstanceCACertificateIdentifier                       = rdsLabelInstance + "ca_certificate_identifier"
	rdsLabelInstanceCharacterSetName                              = rdsLabelInstance + "character_set_name"
	rdsLabelInstanceCopyTagsToSnapshot                            = rdsLabelInstance + "copy_tags_to_snapshot"
	rdsLabelInstanceCustomIamInstanceProfile                      = rdsLabelInstance + "custom_iam_instance_profile"
	rdsLabelInstanceCustomerOwnedIPEnabled                        = rdsLabelInstance + "customer_owned_ip_enabled"
	rdsLabelInstanceDBClusterIdentifier                           = rdsLabelInstance + "db_cluster_identifier"
	rdsLabelInstanceDBInstanceArn                                 = rdsLabelInstance + "arn"
	rdsLabelInstanceDBInstanceClass                               = rdsLabelInstance + "class"
	rdsLabelInstanceDBInstanceIdentifier                          = rdsLabelInstance + "identifier"
	rdsLabelInstanceDBInstanceStatus                              = rdsLabelInstance + "status"
	rdsLabelInstanceDBName                                        = rdsLabelInstance + "db_name"
	rdsLabelInstanceDBSubnetGroup                                 = rdsLabelInstance + "subnet_group"
	rdsLabelInstanceDBSystemID                                    = rdsLabelInstance + "db_system_id"
	rdsLabelInstanceDatabaseInsightsMode                          = rdsLabelInstance + "database_insights_mode"
	rdsLabelInstanceDBInstancePort                                = rdsLabelInstance + "port"
	rdsLabelInstanceDBResourceID                                  = rdsLabelInstance + "resource_id"
	rdsLabelInstanceDedicatedLogVolume                            = rdsLabelInstance + "dedicated_log_volume"
	rdsLabelInstanceDeletionProtection                            = rdsLabelInstance + "deletion_protection"
	rdsLabelInstanceEndpointAddress                               = rdsLabelInstance + "endpoint_address"
	rdsLabelInstanceEndpointHostedZoneID                          = rdsLabelInstance + "endpoint_hosted_zone_id"
	rdsLabelInstanceEndpointPort                                  = rdsLabelInstance + "endpoint_port"
	rdsLabelInstanceEngine                                        = rdsLabelInstance + "engine"
	rdsLabelInstanceEngineLifecycleSupport                        = rdsLabelInstance + "engine_lifecycle_support"
	rdsLabelInstanceEngineVersion                                 = rdsLabelInstance + "engine_version"
	rdsLabelInstanceEnhancedMonitoringResourceArn                 = rdsLabelInstance + "enhanced_monitoring_resource_arn"
	rdsLabelInstanceIAMDatabaseAuthenticationEnabled              = rdsLabelInstance + "iam_database_authentication_enabled"
	rdsLabelInstanceInstanceCreateTime                            = rdsLabelInstance + "instance_create_time"
	rdsLabelInstanceIops                                          = rdsLabelInstance + "iops"
	rdsLabelInstanceIsStorageConfigUpgradeAvailable               = rdsLabelInstance + "is_storage_config_upgrade_available"
	rdsLabelInstanceKMSKeyID                                      = rdsLabelInstance + "kms_key_id"
	rdsLabelInstanceLatestRestorableTime                          = rdsLabelInstance + "latest_restorable_time"
	rdsLabelInstanceLicenseModel                                  = rdsLabelInstance + "license_model"
	rdsLabelInstanceListenerEndpointAddress                       = rdsLabelInstance + "listener_endpoint_address"
	rdsLabelInstanceListenerEndpointHostedZoneID                  = rdsLabelInstance + "listener_endpoint_hosted_zone_id"
	rdsLabelInstanceListenerEndpointPort                          = rdsLabelInstance + "listener_endpoint_port"
	rdsLabelInstanceMasterUsername                                = rdsLabelInstance + "master_username"
	rdsLabelInstanceMaxAllocatedStorage                           = rdsLabelInstance + "max_allocated_storage"
	rdsLabelInstanceMonitoringInterval                            = rdsLabelInstance + "monitoring_interval"
	rdsLabelInstanceMonitoringRoleArn                             = rdsLabelInstance + "monitoring_role_arn"
	rdsLabelInstanceMultiAZ                                       = rdsLabelInstance + "multi_az"
	rdsLabelInstanceMultiTenant                                   = rdsLabelInstance + "multi_tenant"
	rdsLabelInstanceNcharCharacterSetName                         = rdsLabelInstance + "nchar_character_set_name"
	rdsLabelInstanceNetworkType                                   = rdsLabelInstance + "network_type"
	rdsLabelInstancePercentProgress                               = rdsLabelInstance + "percent_progress"
	rdsLabelInstancePerformanceInsightsEnabled                    = rdsLabelInstance + "performance_insights_enabled"
	rdsLabelInstancePerformanceInsightsKMSKeyID                   = rdsLabelInstance + "performance_insights_kms_key_id"
	rdsLabelInstancePerformanceInsightsRetentionPeriod            = rdsLabelInstance + "performance_insights_retention_period"
	rdsLabelInstancePreferredBackupWindow                         = rdsLabelInstance + "preferred_backup_window"
	rdsLabelInstancePreferredMaintenanceWindow                    = rdsLabelInstance + "preferred_maintenance_window"
	rdsLabelInstancePromotionTier                                 = rdsLabelInstance + "promotion_tier"
	rdsLabelInstancePubliclyAccessible                            = rdsLabelInstance + "publicly_accessible"
	rdsLabelInstanceReadReplicaSourceDBClusterIdentifier          = rdsLabelInstance + "read_replica_source_db_cluster_identifier"
	rdsLabelInstanceReadReplicaSourceDBInstanceIdentifier         = rdsLabelInstance + "read_replica_source_db_instance_identifier"
	rdsLabelInstanceReplicaMode                                   = rdsLabelInstance + "replica_mode"
	rdsLabelInstanceResumeFullAutomationModeTime                  = rdsLabelInstance + "resume_full_automation_mode_time"
	rdsLabelInstanceSecondaryAvailabilityZone                     = rdsLabelInstance + "secondary_availability_zone"
	rdsLabelInstanceStorageEncrypted                              = rdsLabelInstance + "storage_encrypted"
	rdsLabelInstanceStorageEncryptionType                         = rdsLabelInstance + "storage_encryption_type"
	rdsLabelInstanceStorageThroughput                             = rdsLabelInstance + "storage_throughput"
	rdsLabelInstanceStorageType                                   = rdsLabelInstance + "storage_type"
	rdsLabelInstanceStorageVolumeStatus                           = rdsLabelInstance + "storage_volume_status"
	rdsLabelInstanceTdeCredentialArn                              = rdsLabelInstance + "tde_credential_arn"
	rdsLabelInstanceTimezone                                      = rdsLabelInstance + "timezone"
	rdsLabelInstanceUpgradeRolloutOrder                           = rdsLabelInstance + "upgrade_rollout_order"

	// DB instance tags - create one label per tag key, with the format: rds_instance_tag_<tagkey>.
	rdsLabelInstanceTag = rdsLabelInstance + "tag_"
)

// DefaultRDSSDConfig is the default RDS SD configuration.
var DefaultRDSSDConfig = RDSSDConfig{
	Port:               80,
	RefreshInterval:    model.Duration(60 * time.Second),
	RequestConcurrency: 10,
	HTTPClientConfig:   config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&RDSSDConfig{})
}

// RDSSDConfig is the configuration for RDS based service discovery.
type RDSSDConfig struct {
	Region          string         `yaml:"region"`
	Endpoint        string         `yaml:"endpoint"`
	AccessKey       string         `yaml:"access_key,omitempty"`
	SecretKey       config.Secret  `yaml:"secret_key,omitempty"`
	Profile         string         `yaml:"profile,omitempty"`
	RoleARN         string         `yaml:"role_arn,omitempty"`
	Clusters        []string       `yaml:"clusters,omitempty"`
	Port            int            `yaml:"port"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`

	RequestConcurrency int                     `yaml:"request_concurrency,omitempty"`
	HTTPClientConfig   config.HTTPClientConfig `yaml:",inline"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*RDSSDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &rdsMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the RDS Config.
func (*RDSSDConfig) Name() string { return "rds" }

// NewDiscoverer returns a Discoverer for the RDS Config.
func (c *RDSSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewRDSDiscovery(c, opts)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the RDS Config.
func (c *RDSSDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultRDSSDConfig
	type plain RDSSDConfig
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

type rdsClient interface {
	DescribeDBClusters(context.Context, *rds.DescribeDBClustersInput, ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
	DescribeDBInstances(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
}

// RDSDiscovery periodically performs RDS-SD requests. It implements
// the Discoverer interface.
type RDSDiscovery struct {
	*refresh.Discovery
	logger *slog.Logger
	cfg    *RDSSDConfig
	rds    rdsClient
}

// NewRDSDiscovery returns a new RDSDiscovery which periodically refreshes its targets.
func NewRDSDiscovery(conf *RDSSDConfig, opts discovery.DiscovererOptions) (*RDSDiscovery, error) {
	m, ok := opts.Metrics.(*rdsMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if opts.Logger == nil {
		opts.Logger = promslog.NewNopLogger()
	}
	d := &RDSDiscovery{
		logger: opts.Logger,
		cfg:    conf,
	}
	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "rds",
			Interval:            time.Duration(d.cfg.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *RDSDiscovery) initRdsClient(ctx context.Context) error {
	if d.rds != nil {
		return nil
	}

	if d.cfg.Region == "" {
		return errors.New("region must be set for RDS service discovery")
	}

	// Build the HTTP client from the provided HTTPClientConfig.
	client, err := config.NewClientFromConfig(d.cfg.HTTPClientConfig, "rds_sd")
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

	d.rds = rds.NewFromConfig(cfg, func(options *rds.Options) {
		if d.cfg.Endpoint != "" {
			options.BaseEndpoint = &d.cfg.Endpoint
		}
		options.HTTPClient = client
	})

	// Test credentials by making a simple API call
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err = d.rds.DescribeDBClusters(testCtx, &rds.DescribeDBClustersInput{})
	if err != nil {
		d.logger.Error("Failed to test RDS credentials", "error", err)
		return fmt.Errorf("RDS credential test failed: %w", err)
	}

	return nil
}

func (d *RDSDiscovery) describeAllDBClusters(ctx context.Context) (map[string]types.DBCluster, error) {
	dbClustersByARN := make(map[string]types.DBCluster)
	var nextToken *string

	for {
		output, err := d.rds.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{
			Marker:     nextToken,
			MaxRecords: aws.Int32(100),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe DB clusters: %w", err)
		}
		for _, dbCluster := range output.DBClusters {
			if dbCluster.DBClusterArn != nil {
				dbClustersByARN[*dbCluster.DBClusterArn] = dbCluster
			}
		}
		if output.Marker == nil {
			break
		}
		nextToken = output.Marker
	}

	return dbClustersByARN, nil
}

func (d *RDSDiscovery) describeDBClusters(ctx context.Context, dbClusterARNS []string) (map[string]types.DBCluster, error) {
	mu := &sync.Mutex{}
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	dbClustersByARN := make(map[string]types.DBCluster)
	var nextToken *string

	for _, arn := range dbClusterARNS {
		errg.Go(func() error {
			for {
				output, err := d.rds.DescribeDBClusters(ectx, &rds.DescribeDBClustersInput{
					DBClusterIdentifier: aws.String(arn),
					Marker:              nextToken,
					MaxRecords:          aws.Int32(100),
				})
				if err != nil {
					return fmt.Errorf("failed to describe DB cluster %s: %w", arn, err)
				}
				if len(output.DBClusters) == 0 {
					return fmt.Errorf("no DB cluster found for ARN %s", arn)
				}

				for _, dbCluster := range output.DBClusters {
					mu.Lock()
					dbClustersByARN[arn] = dbCluster
					mu.Unlock()
				}
				if output.Marker == nil {
					break
				}
				nextToken = output.Marker
			}
			return nil
		})
	}
	return dbClustersByARN, errg.Wait()
}

func (d *RDSDiscovery) describeDBInstances(ctx context.Context, dbClusterARN string) ([]types.DBInstance, error) {
	mu := &sync.Mutex{}
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	dbInstances := []types.DBInstance{}
	var nextToken *string
	for {
		output, err := d.rds.DescribeDBInstances(ectx, &rds.DescribeDBInstancesInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("db-cluster-id"),
					Values: []string{dbClusterARN},
				},
			},
			Marker:     nextToken,
			MaxRecords: aws.Int32(100),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe DB instances for cluster ARN %s: %w", dbClusterARN, err)
		}
		if len(output.DBInstances) == 0 {
			return nil, fmt.Errorf("no DB instances found for cluster ARN %s", dbClusterARN)
		}

		for _, dbInstance := range output.DBInstances {
			mu.Lock()
			dbInstances = append(dbInstances, dbInstance)
			mu.Unlock()
		}
		if output.Marker == nil {
			break
		}
		nextToken = output.Marker
	}
	return dbInstances, errg.Wait()
}

func (d *RDSDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	err := d.initRdsClient(ctx)
	if err != nil {
		return nil, err
	}

	tg := &targetgroup.Group{
		Source: d.cfg.Region,
	}

	var clusters map[string]types.DBCluster
	if len(d.cfg.Clusters) == 0 {
		clusters, err = d.describeAllDBClusters(ctx)
		if err != nil {
			return nil, fmt.Errorf("error describing all DB clusters: %w", err)
		}
	} else {
		clusters, err = d.describeDBClusters(ctx, d.cfg.Clusters)
		if err != nil {
			return nil, fmt.Errorf("error describing DB clusters: %w", err)
		}
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for _, cluster := range clusters {
		wg.Add(1)

		instances, err := d.describeDBInstances(ctx, *cluster.DBClusterArn)
		if err != nil {
			return nil, fmt.Errorf("error describing DB instances: %w", err)
		}

		go func(cluster types.DBCluster, instances []types.DBInstance) {
			defer wg.Done()

			// Build a map of instance identifiers to their IsClusterWriter status
			writerMap := make(map[string]bool)
			for _, member := range cluster.DBClusterMembers {
				if member.DBInstanceIdentifier != nil && member.IsClusterWriter != nil {
					writerMap[*member.DBInstanceIdentifier] = *member.IsClusterWriter
				}
			}

			for _, instance := range instances {
				labels := model.LabelSet{}

				// Cluster labels
				if cluster.DBClusterArn != nil {
					labels[rdsLabelClusterDBClusterArn] = model.LabelValue(*cluster.DBClusterArn)
				}
				if cluster.DBClusterIdentifier != nil {
					labels[rdsLabelClusterDBClusterIdentifier] = model.LabelValue(*cluster.DBClusterIdentifier)
				}
				if cluster.ActivityStreamKinesisStreamName != nil {
					labels[rdsLabelClusterActivityStreamKinesisStreamName] = model.LabelValue(*cluster.ActivityStreamKinesisStreamName)
				}
				if cluster.ActivityStreamKmsKeyId != nil {
					labels[rdsLabelClusterActivityStreamKMSKeyID] = model.LabelValue(*cluster.ActivityStreamKmsKeyId)
				}
				if cluster.ActivityStreamMode != "" {
					labels[rdsLabelClusterActivityStreamMode] = model.LabelValue(cluster.ActivityStreamMode)
				}
				if cluster.ActivityStreamStatus != "" {
					labels[rdsLabelClusterActivityStreamStatus] = model.LabelValue(cluster.ActivityStreamStatus)
				}
				if cluster.AllocatedStorage != nil {
					labels[rdsLabelClusterAllocatedStorage] = model.LabelValue(strconv.Itoa(int(*cluster.AllocatedStorage)))
				}
				if cluster.AutoMinorVersionUpgrade != nil {
					labels[rdsLabelClusterAutoMinorVersionUpgrade] = model.LabelValue(strconv.FormatBool(*cluster.AutoMinorVersionUpgrade))
				}
				if cluster.AutomaticRestartTime != nil {
					labels[rdsLabelClusterAutomaticRestartTime] = model.LabelValue(cluster.AutomaticRestartTime.Format(time.RFC3339))
				}
				if cluster.AwsBackupRecoveryPointArn != nil {
					labels[rdsLabelClusterAwsBackupRecoveryPointArn] = model.LabelValue(*cluster.AwsBackupRecoveryPointArn)
				}
				if cluster.BacktrackConsumedChangeRecords != nil {
					labels[rdsLabelClusterBacktrackConsumedChangeRecords] = model.LabelValue(strconv.FormatInt(*cluster.BacktrackConsumedChangeRecords, 10))
				}
				if cluster.BacktrackWindow != nil {
					labels[rdsLabelClusterBacktrackWindow] = model.LabelValue(strconv.FormatInt(*cluster.BacktrackWindow, 10))
				}
				if cluster.BackupRetentionPeriod != nil {
					labels[rdsLabelClusterBackupRetentionPeriod] = model.LabelValue(strconv.Itoa(int(*cluster.BackupRetentionPeriod)))
				}
				if cluster.Capacity != nil {
					labels[rdsLabelClusterCapacity] = model.LabelValue(strconv.Itoa(int(*cluster.Capacity)))
				}
				if cluster.CharacterSetName != nil {
					labels[rdsLabelClusterCharacterSetName] = model.LabelValue(*cluster.CharacterSetName)
				}
				if cluster.CloneGroupId != nil {
					labels[rdsLabelClusterCloneGroupID] = model.LabelValue(*cluster.CloneGroupId)
				}
				if cluster.ClusterCreateTime != nil {
					labels[rdsLabelClusterClusterCreateTime] = model.LabelValue(cluster.ClusterCreateTime.Format(time.RFC3339))
				}
				if cluster.ClusterScalabilityType != "" {
					labels[rdsLabelClusterClusterScalabilityType] = model.LabelValue(cluster.ClusterScalabilityType)
				}
				if cluster.CopyTagsToSnapshot != nil {
					labels[rdsLabelClusterCopyTagsToSnapshot] = model.LabelValue(strconv.FormatBool(*cluster.CopyTagsToSnapshot))
				}
				if cluster.CrossAccountClone != nil {
					labels[rdsLabelClusterCrossAccountClone] = model.LabelValue(strconv.FormatBool(*cluster.CrossAccountClone))
				}
				if cluster.DBClusterInstanceClass != nil {
					labels[rdsLabelClusterDBClusterInstanceClass] = model.LabelValue(*cluster.DBClusterInstanceClass)
				}
				if cluster.DBClusterParameterGroup != nil {
					labels[rdsLabelClusterDBClusterParameterGroup] = model.LabelValue(*cluster.DBClusterParameterGroup)
				}
				if cluster.DBSubnetGroup != nil {
					labels[rdsLabelClusterDBSubnetGroup] = model.LabelValue(*cluster.DBSubnetGroup)
				}
				if cluster.DBSystemId != nil {
					labels[rdsLabelClusterDBSystemID] = model.LabelValue(*cluster.DBSystemId)
				}
				if cluster.DatabaseInsightsMode != "" {
					labels[rdsLabelClusterDatabaseInsightsMode] = model.LabelValue(cluster.DatabaseInsightsMode)
				}
				if cluster.DatabaseName != nil {
					labels[rdsLabelClusterDatabaseName] = model.LabelValue(*cluster.DatabaseName)
				}
				if cluster.DbClusterResourceId != nil {
					labels[rdsLabelClusterDBClusterResourceID] = model.LabelValue(*cluster.DbClusterResourceId)
				}
				if cluster.DeletionProtection != nil {
					labels[rdsLabelClusterDeletionProtection] = model.LabelValue(strconv.FormatBool(*cluster.DeletionProtection))
				}
				if cluster.EarliestBacktrackTime != nil {
					labels[rdsLabelClusterEarliestBacktrackTime] = model.LabelValue(cluster.EarliestBacktrackTime.Format(time.RFC3339))
				}
				if cluster.EarliestRestorableTime != nil {
					labels[rdsLabelClusterEarliestRestorableTime] = model.LabelValue(cluster.EarliestRestorableTime.Format(time.RFC3339))
				}
				if cluster.Endpoint != nil {
					labels[rdsLabelClusterEndpoint] = model.LabelValue(*cluster.Endpoint)
				}
				if cluster.Engine != nil {
					labels[rdsLabelClusterEngine] = model.LabelValue(*cluster.Engine)
				}
				if cluster.EngineLifecycleSupport != nil {
					labels[rdsLabelClusterEngineLifecycleSupport] = model.LabelValue(*cluster.EngineLifecycleSupport)
				}
				if cluster.EngineMode != nil {
					labels[rdsLabelClusterEngineMode] = model.LabelValue(*cluster.EngineMode)
				}
				if cluster.EngineVersion != nil {
					labels[rdsLabelClusterEngineVersion] = model.LabelValue(*cluster.EngineVersion)
				}
				if cluster.GlobalClusterIdentifier != nil {
					labels[rdsLabelClusterGlobalClusterIdentifier] = model.LabelValue(*cluster.GlobalClusterIdentifier)
				}
				if cluster.GlobalWriteForwardingRequested != nil {
					labels[rdsLabelClusterGlobalWriteForwardingRequested] = model.LabelValue(strconv.FormatBool(*cluster.GlobalWriteForwardingRequested))
				}
				if cluster.GlobalWriteForwardingStatus != "" {
					labels[rdsLabelClusterGlobalWriteForwardingStatus] = model.LabelValue(cluster.GlobalWriteForwardingStatus)
				}
				if cluster.HostedZoneId != nil {
					labels[rdsLabelClusterHostedZoneID] = model.LabelValue(*cluster.HostedZoneId)
				}
				if cluster.HttpEndpointEnabled != nil {
					labels[rdsLabelClusterHTTPEndpointEnabled] = model.LabelValue(strconv.FormatBool(*cluster.HttpEndpointEnabled))
				}
				if cluster.IAMDatabaseAuthenticationEnabled != nil {
					labels[rdsLabelClusterIAMDatabaseAuthenticationEnabled] = model.LabelValue(strconv.FormatBool(*cluster.IAMDatabaseAuthenticationEnabled))
				}
				if cluster.IOOptimizedNextAllowedModificationTime != nil {
					labels[rdsLabelClusterIOOptimizedNextAllowedModificationTime] = model.LabelValue(cluster.IOOptimizedNextAllowedModificationTime.Format(time.RFC3339))
				}
				if cluster.Iops != nil {
					labels[rdsLabelClusterIops] = model.LabelValue(strconv.Itoa(int(*cluster.Iops)))
				}
				if cluster.KmsKeyId != nil {
					labels[rdsLabelClusterKMSKeyID] = model.LabelValue(*cluster.KmsKeyId)
				}
				if cluster.LatestRestorableTime != nil {
					labels[rdsLabelClusterLatestRestorableTime] = model.LabelValue(cluster.LatestRestorableTime.Format(time.RFC3339))
				}
				if cluster.LocalWriteForwardingStatus != "" {
					labels[rdsLabelClusterLocalWriteForwardingStatus] = model.LabelValue(cluster.LocalWriteForwardingStatus)
				}
				if cluster.MasterUsername != nil {
					labels[rdsLabelClusterMasterUsername] = model.LabelValue(*cluster.MasterUsername)
				}
				if cluster.MonitoringInterval != nil {
					labels[rdsLabelClusterMonitoringInterval] = model.LabelValue(strconv.Itoa(int(*cluster.MonitoringInterval)))
				}
				if cluster.MonitoringRoleArn != nil {
					labels[rdsLabelClusterMonitoringRoleArn] = model.LabelValue(*cluster.MonitoringRoleArn)
				}
				if cluster.MultiAZ != nil {
					labels[rdsLabelClusterMultiAZ] = model.LabelValue(strconv.FormatBool(*cluster.MultiAZ))
				}
				if cluster.NetworkType != nil {
					labels[rdsLabelClusterNetworkType] = model.LabelValue(*cluster.NetworkType)
				}
				if cluster.PercentProgress != nil {
					labels[rdsLabelClusterPercentProgress] = model.LabelValue(*cluster.PercentProgress)
				}
				if cluster.PerformanceInsightsEnabled != nil {
					labels[rdsLabelClusterPerformanceInsightsEnabled] = model.LabelValue(strconv.FormatBool(*cluster.PerformanceInsightsEnabled))
				}
				if cluster.PerformanceInsightsKMSKeyId != nil {
					labels[rdsLabelClusterPerformanceInsightsKMSKeyID] = model.LabelValue(*cluster.PerformanceInsightsKMSKeyId)
				}
				if cluster.PerformanceInsightsRetentionPeriod != nil {
					labels[rdsLabelClusterPerformanceInsightsRetentionPeriod] = model.LabelValue(strconv.Itoa(int(*cluster.PerformanceInsightsRetentionPeriod)))
				}
				if cluster.Port != nil {
					labels[rdsLabelClusterPort] = model.LabelValue(strconv.Itoa(int(*cluster.Port)))
				}
				if cluster.PreferredBackupWindow != nil {
					labels[rdsLabelClusterPreferredBackupWindow] = model.LabelValue(*cluster.PreferredBackupWindow)
				}
				if cluster.PreferredMaintenanceWindow != nil {
					labels[rdsLabelClusterPreferredMaintenanceWindow] = model.LabelValue(*cluster.PreferredMaintenanceWindow)
				}
				if cluster.PubliclyAccessible != nil {
					labels[rdsLabelClusterPubliclyAccessible] = model.LabelValue(strconv.FormatBool(*cluster.PubliclyAccessible))
				}
				if cluster.ReaderEndpoint != nil {
					labels[rdsLabelClusterReaderEndpoint] = model.LabelValue(*cluster.ReaderEndpoint)
				}
				if cluster.ReplicationSourceIdentifier != nil {
					labels[rdsLabelClusterReplicationSourceIdentifier] = model.LabelValue(*cluster.ReplicationSourceIdentifier)
				}
				if cluster.ServerlessV2PlatformVersion != nil {
					labels[rdsLabelClusterServerlessV2PlatformVersion] = model.LabelValue(*cluster.ServerlessV2PlatformVersion)
				}
				if cluster.Status != nil {
					labels[rdsLabelClusterStatus] = model.LabelValue(*cluster.Status)
				}
				if cluster.StorageEncrypted != nil {
					labels[rdsLabelClusterStorageEncrypted] = model.LabelValue(strconv.FormatBool(*cluster.StorageEncrypted))
				}
				if cluster.StorageEncryptionType != "" {
					labels[rdsLabelClusterStorageEncryptionType] = model.LabelValue(cluster.StorageEncryptionType)
				}
				if cluster.StorageThroughput != nil {
					labels[rdsLabelClusterStorageThroughput] = model.LabelValue(strconv.Itoa(int(*cluster.StorageThroughput)))
				}
				if cluster.StorageType != nil {
					labels[rdsLabelClusterStorageType] = model.LabelValue(*cluster.StorageType)
				}
				if cluster.UpgradeRolloutOrder != "" {
					labels[rdsLabelClusterUpgradeRolloutOrder] = model.LabelValue(cluster.UpgradeRolloutOrder)
				}

				// Cluster tags
				for _, tag := range cluster.TagList {
					if tag.Key != nil && tag.Value != nil {
						labels[model.LabelName(rdsLabelClusterTag+strutil.SanitizeLabelName(*tag.Key))] = model.LabelValue(*tag.Value)
					}
				}

				// Instance labels
				if instance.DBInstanceArn != nil {
					labels[rdsLabelInstanceDBInstanceArn] = model.LabelValue(*instance.DBInstanceArn)
				}
				if instance.DBInstanceIdentifier != nil {
					labels[rdsLabelInstanceDBInstanceIdentifier] = model.LabelValue(*instance.DBInstanceIdentifier)
					// Set IsClusterWriter based on cluster membership information
					if isWriter, found := writerMap[*instance.DBInstanceIdentifier]; found {
						labels[rdsLabelInstanceIsClusterWriter] = model.LabelValue(strconv.FormatBool(isWriter))
					}
				}
				if instance.ActivityStreamEngineNativeAuditFieldsIncluded != nil {
					labels[rdsLabelInstanceActivityStreamEngineNativeAuditFieldsIncluded] = model.LabelValue(strconv.FormatBool(*instance.ActivityStreamEngineNativeAuditFieldsIncluded))
				}
				if instance.ActivityStreamKinesisStreamName != nil {
					labels[rdsLabelInstanceActivityStreamKinesisStreamName] = model.LabelValue(*instance.ActivityStreamKinesisStreamName)
				}
				if instance.ActivityStreamKmsKeyId != nil {
					labels[rdsLabelInstanceActivityStreamKmsKeyID] = model.LabelValue(*instance.ActivityStreamKmsKeyId)
				}
				if instance.ActivityStreamMode != "" {
					labels[rdsLabelInstanceActivityStreamMode] = model.LabelValue(instance.ActivityStreamMode)
				}
				if instance.ActivityStreamPolicyStatus != "" {
					labels[rdsLabelInstanceActivityStreamPolicyStatus] = model.LabelValue(instance.ActivityStreamPolicyStatus)
				}
				if instance.ActivityStreamStatus != "" {
					labels[rdsLabelInstanceActivityStreamStatus] = model.LabelValue(instance.ActivityStreamStatus)
				}
				if instance.AllocatedStorage != nil {
					labels[rdsLabelInstanceAllocatedStorage] = model.LabelValue(strconv.Itoa(int(*instance.AllocatedStorage)))
				}
				if instance.AutoMinorVersionUpgrade != nil {
					labels[rdsLabelInstanceAutoMinorVersionUpgrade] = model.LabelValue(strconv.FormatBool(*instance.AutoMinorVersionUpgrade))
				}
				if instance.AutomaticRestartTime != nil {
					labels[rdsLabelInstanceAutomaticRestartTime] = model.LabelValue(instance.AutomaticRestartTime.Format(time.RFC3339))
				}
				if instance.AutomationMode != "" {
					labels[rdsLabelInstanceAutomationMode] = model.LabelValue(instance.AutomationMode)
				}
				if instance.AvailabilityZone != nil {
					labels[rdsLabelInstanceAvailabilityZone] = model.LabelValue(*instance.AvailabilityZone)
				}
				if instance.AwsBackupRecoveryPointArn != nil {
					labels[rdsLabelInstanceAwsBackupRecoveryPointArn] = model.LabelValue(*instance.AwsBackupRecoveryPointArn)
				}
				if instance.BackupRetentionPeriod != nil {
					labels[rdsLabelInstanceBackupRetentionPeriod] = model.LabelValue(strconv.Itoa(int(*instance.BackupRetentionPeriod)))
				}
				if instance.BackupTarget != nil {
					labels[rdsLabelInstanceBackupTarget] = model.LabelValue(*instance.BackupTarget)
				}
				if instance.CACertificateIdentifier != nil {
					labels[rdsLabelInstanceCACertificateIdentifier] = model.LabelValue(*instance.CACertificateIdentifier)
				}
				if instance.CharacterSetName != nil {
					labels[rdsLabelInstanceCharacterSetName] = model.LabelValue(*instance.CharacterSetName)
				}
				if instance.CopyTagsToSnapshot != nil {
					labels[rdsLabelInstanceCopyTagsToSnapshot] = model.LabelValue(strconv.FormatBool(*instance.CopyTagsToSnapshot))
				}
				if instance.CustomIamInstanceProfile != nil {
					labels[rdsLabelInstanceCustomIamInstanceProfile] = model.LabelValue(*instance.CustomIamInstanceProfile)
				}
				if instance.CustomerOwnedIpEnabled != nil {
					labels[rdsLabelInstanceCustomerOwnedIPEnabled] = model.LabelValue(strconv.FormatBool(*instance.CustomerOwnedIpEnabled))
				}
				if instance.DBClusterIdentifier != nil {
					labels[rdsLabelInstanceDBClusterIdentifier] = model.LabelValue(*instance.DBClusterIdentifier)
				}
				if instance.DBInstanceClass != nil {
					labels[rdsLabelInstanceDBInstanceClass] = model.LabelValue(*instance.DBInstanceClass)
				}
				if instance.DBInstanceStatus != nil {
					labels[rdsLabelInstanceDBInstanceStatus] = model.LabelValue(*instance.DBInstanceStatus)
				}
				if instance.DBName != nil {
					labels[rdsLabelInstanceDBName] = model.LabelValue(*instance.DBName)
				}
				if instance.DbInstancePort != nil {
					labels[rdsLabelInstanceDBInstancePort] = model.LabelValue(strconv.Itoa(int(*instance.DbInstancePort)))
				}
				if instance.DbiResourceId != nil {
					labels[rdsLabelInstanceDBResourceID] = model.LabelValue(*instance.DbiResourceId)
				}
				if instance.DedicatedLogVolume != nil {
					labels[rdsLabelInstanceDedicatedLogVolume] = model.LabelValue(strconv.FormatBool(*instance.DedicatedLogVolume))
				}
				if instance.DeletionProtection != nil {
					labels[rdsLabelInstanceDeletionProtection] = model.LabelValue(strconv.FormatBool(*instance.DeletionProtection))
				}
				if instance.Endpoint != nil {
					if instance.Endpoint.Address != nil {
						labels[rdsLabelInstanceEndpointAddress] = model.LabelValue(*instance.Endpoint.Address)
					}
					if instance.Endpoint.HostedZoneId != nil {
						labels[rdsLabelInstanceEndpointHostedZoneID] = model.LabelValue(*instance.Endpoint.HostedZoneId)
					}
					if instance.Endpoint.Port != nil {
						labels[rdsLabelInstanceEndpointPort] = model.LabelValue(strconv.Itoa(int(*instance.Endpoint.Port)))
					}
				}
				if instance.Engine != nil {
					labels[rdsLabelInstanceEngine] = model.LabelValue(*instance.Engine)
				}
				if instance.EngineLifecycleSupport != nil {
					labels[rdsLabelInstanceEngineLifecycleSupport] = model.LabelValue(*instance.EngineLifecycleSupport)
				}
				if instance.EngineVersion != nil {
					labels[rdsLabelInstanceEngineVersion] = model.LabelValue(*instance.EngineVersion)
				}
				if instance.EnhancedMonitoringResourceArn != nil {
					labels[rdsLabelInstanceEnhancedMonitoringResourceArn] = model.LabelValue(*instance.EnhancedMonitoringResourceArn)
				}
				if instance.IAMDatabaseAuthenticationEnabled != nil {
					labels[rdsLabelInstanceIAMDatabaseAuthenticationEnabled] = model.LabelValue(strconv.FormatBool(*instance.IAMDatabaseAuthenticationEnabled))
				}
				if instance.InstanceCreateTime != nil {
					labels[rdsLabelInstanceInstanceCreateTime] = model.LabelValue(instance.InstanceCreateTime.Format(time.RFC3339))
				}
				if instance.Iops != nil {
					labels[rdsLabelInstanceIops] = model.LabelValue(strconv.Itoa(int(*instance.Iops)))
				}
				if instance.IsStorageConfigUpgradeAvailable != nil {
					labels[rdsLabelInstanceIsStorageConfigUpgradeAvailable] = model.LabelValue(strconv.FormatBool(*instance.IsStorageConfigUpgradeAvailable))
				}
				if instance.KmsKeyId != nil {
					labels[rdsLabelInstanceKMSKeyID] = model.LabelValue(*instance.KmsKeyId)
				}
				if instance.LatestRestorableTime != nil {
					labels[rdsLabelInstanceLatestRestorableTime] = model.LabelValue(instance.LatestRestorableTime.Format(time.RFC3339))
				}
				if instance.LicenseModel != nil {
					labels[rdsLabelInstanceLicenseModel] = model.LabelValue(*instance.LicenseModel)
				}
				if instance.ListenerEndpoint != nil {
					if instance.ListenerEndpoint.Address != nil {
						labels[rdsLabelInstanceListenerEndpointAddress] = model.LabelValue(*instance.ListenerEndpoint.Address)
					}
					if instance.ListenerEndpoint.HostedZoneId != nil {
						labels[rdsLabelInstanceListenerEndpointHostedZoneID] = model.LabelValue(*instance.ListenerEndpoint.HostedZoneId)
					}
					if instance.ListenerEndpoint.Port != nil {
						labels[rdsLabelInstanceListenerEndpointPort] = model.LabelValue(strconv.Itoa(int(*instance.ListenerEndpoint.Port)))
					}
				}
				if instance.MasterUsername != nil {
					labels[rdsLabelInstanceMasterUsername] = model.LabelValue(*instance.MasterUsername)
				}
				if instance.MaxAllocatedStorage != nil {
					labels[rdsLabelInstanceMaxAllocatedStorage] = model.LabelValue(strconv.Itoa(int(*instance.MaxAllocatedStorage)))
				}
				if instance.MonitoringInterval != nil {
					labels[rdsLabelInstanceMonitoringInterval] = model.LabelValue(strconv.Itoa(int(*instance.MonitoringInterval)))
				}
				if instance.MonitoringRoleArn != nil {
					labels[rdsLabelInstanceMonitoringRoleArn] = model.LabelValue(*instance.MonitoringRoleArn)
				}
				if instance.MultiAZ != nil {
					labels[rdsLabelInstanceMultiAZ] = model.LabelValue(strconv.FormatBool(*instance.MultiAZ))
				}
				if instance.MultiTenant != nil {
					labels[rdsLabelInstanceMultiTenant] = model.LabelValue(strconv.FormatBool(*instance.MultiTenant))
				}
				if instance.NcharCharacterSetName != nil {
					labels[rdsLabelInstanceNcharCharacterSetName] = model.LabelValue(*instance.NcharCharacterSetName)
				}
				if instance.NetworkType != nil {
					labels[rdsLabelInstanceNetworkType] = model.LabelValue(*instance.NetworkType)
				}
				if instance.PercentProgress != nil {
					labels[rdsLabelInstancePercentProgress] = model.LabelValue(*instance.PercentProgress)
				}
				if instance.PerformanceInsightsEnabled != nil {
					labels[rdsLabelInstancePerformanceInsightsEnabled] = model.LabelValue(strconv.FormatBool(*instance.PerformanceInsightsEnabled))
				}
				if instance.PerformanceInsightsKMSKeyId != nil {
					labels[rdsLabelInstancePerformanceInsightsKMSKeyID] = model.LabelValue(*instance.PerformanceInsightsKMSKeyId)
				}
				if instance.PerformanceInsightsRetentionPeriod != nil {
					labels[rdsLabelInstancePerformanceInsightsRetentionPeriod] = model.LabelValue(strconv.Itoa(int(*instance.PerformanceInsightsRetentionPeriod)))
				}
				if instance.PreferredBackupWindow != nil {
					labels[rdsLabelInstancePreferredBackupWindow] = model.LabelValue(*instance.PreferredBackupWindow)
				}
				if instance.PreferredMaintenanceWindow != nil {
					labels[rdsLabelInstancePreferredMaintenanceWindow] = model.LabelValue(*instance.PreferredMaintenanceWindow)
				}
				if instance.PromotionTier != nil {
					labels[rdsLabelInstancePromotionTier] = model.LabelValue(strconv.Itoa(int(*instance.PromotionTier)))
				}
				if instance.PubliclyAccessible != nil {
					labels[rdsLabelInstancePubliclyAccessible] = model.LabelValue(strconv.FormatBool(*instance.PubliclyAccessible))
				}
				if instance.ReadReplicaSourceDBClusterIdentifier != nil {
					labels[rdsLabelInstanceReadReplicaSourceDBClusterIdentifier] = model.LabelValue(*instance.ReadReplicaSourceDBClusterIdentifier)
				}
				if instance.ReadReplicaSourceDBInstanceIdentifier != nil {
					labels[rdsLabelInstanceReadReplicaSourceDBInstanceIdentifier] = model.LabelValue(*instance.ReadReplicaSourceDBInstanceIdentifier)
				}
				if instance.ReplicaMode != "" {
					labels[rdsLabelInstanceReplicaMode] = model.LabelValue(instance.ReplicaMode)
				}
				if instance.ResumeFullAutomationModeTime != nil {
					labels[rdsLabelInstanceResumeFullAutomationModeTime] = model.LabelValue(instance.ResumeFullAutomationModeTime.Format(time.RFC3339))
				}
				if instance.SecondaryAvailabilityZone != nil {
					labels[rdsLabelInstanceSecondaryAvailabilityZone] = model.LabelValue(*instance.SecondaryAvailabilityZone)
				}
				if instance.StorageEncrypted != nil {
					labels[rdsLabelInstanceStorageEncrypted] = model.LabelValue(strconv.FormatBool(*instance.StorageEncrypted))
				}
				if instance.StorageEncryptionType != "" {
					labels[rdsLabelInstanceStorageEncryptionType] = model.LabelValue(instance.StorageEncryptionType)
				}
				if instance.StorageThroughput != nil {
					labels[rdsLabelInstanceStorageThroughput] = model.LabelValue(strconv.Itoa(int(*instance.StorageThroughput)))
				}
				if instance.StorageType != nil {
					labels[rdsLabelInstanceStorageType] = model.LabelValue(*instance.StorageType)
				}
				if instance.StorageVolumeStatus != nil {
					labels[rdsLabelInstanceStorageVolumeStatus] = model.LabelValue(*instance.StorageVolumeStatus)
				}
				if instance.TdeCredentialArn != nil {
					labels[rdsLabelInstanceTdeCredentialArn] = model.LabelValue(*instance.TdeCredentialArn)
				}
				if instance.Timezone != nil {
					labels[rdsLabelInstanceTimezone] = model.LabelValue(*instance.Timezone)
				}
				if instance.UpgradeRolloutOrder != "" {
					labels[rdsLabelInstanceUpgradeRolloutOrder] = model.LabelValue(instance.UpgradeRolloutOrder)
				}
				if instance.DBSubnetGroup != nil && instance.DBSubnetGroup.DBSubnetGroupName != nil {
					labels[rdsLabelInstanceDBSubnetGroup] = model.LabelValue(*instance.DBSubnetGroup.DBSubnetGroupName)
				}
				if instance.DBSystemId != nil {
					labels[rdsLabelInstanceDBSystemID] = model.LabelValue(*instance.DBSystemId)
				}
				if instance.DatabaseInsightsMode != "" {
					labels[rdsLabelInstanceDatabaseInsightsMode] = model.LabelValue(instance.DatabaseInsightsMode)
				}

				// Instance tags
				for _, tag := range instance.TagList {
					if tag.Key != nil && tag.Value != nil {
						labels[model.LabelName(rdsLabelInstanceTag+strutil.SanitizeLabelName(*tag.Key))] = model.LabelValue(*tag.Value)
					}
				}

				// Set the address label
				if instance.Endpoint != nil && instance.Endpoint.Address != nil && instance.Endpoint.Port != nil {
					labels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(*instance.Endpoint.Address, strconv.Itoa(d.cfg.Port)))
				}

				mu.Lock()
				tg.Targets = append(tg.Targets, labels)
				mu.Unlock()
			}
		}(cluster, instances)
	}

	wg.Wait()
	return []*targetgroup.Group{tg}, nil
}
