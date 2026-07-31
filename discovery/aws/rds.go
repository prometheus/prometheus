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
	ExternalID      string         `yaml:"external_id,omitempty"`
	Clusters        []string       `yaml:"clusters,omitempty"`
	Port            int            `yaml:"port"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Filters         []*Filter      `yaml:"filters"`

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

// SetDirectory joins any relative file paths with dir.
func (c *RDSSDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the RDS Config.
// Region resolution is deferred to initRdsClient; see loadRegion.
func (c *RDSSDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultRDSSDConfig
	type plain RDSSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	return c.HTTPClientConfig.Validate()
}

type rdsClient interface {
	DescribeDBClusters(context.Context, *rds.DescribeDBClustersInput, ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
	DescribeDBInstances(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
}

// rdsClientAdapter captures only the RDS API calls AWS discovery uses as
// method-value closures, keeping the concrete *rds.Client out of any
// interface-boxed struct field. See ec2ClientAdapter for the full rationale:
// this stops the linker from retaining the entire RDS API surface (~5 MB).
type rdsClientAdapter struct {
	describeDBClusters  func(context.Context, *rds.DescribeDBClustersInput, ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
	describeDBInstances func(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
}

func newRDSClientAdapter(c *rds.Client) rdsClientAdapter {
	return rdsClientAdapter{
		describeDBClusters:  c.DescribeDBClusters,
		describeDBInstances: c.DescribeDBInstances,
	}
}

func (a rdsClientAdapter) DescribeDBClusters(ctx context.Context, params *rds.DescribeDBClustersInput, optFns ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
	return a.describeDBClusters(ctx, params, optFns...)
}

func (a rdsClientAdapter) DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	return a.describeDBInstances(ctx, params, optFns...)
}

// RDSDiscovery periodically performs RDS-SD requests. It implements
// the Discoverer interface.
type RDSDiscovery struct {
	*refresh.Discovery
	logger *slog.Logger
	cfg    *RDSSDConfig
	rds    rdsClient

	// region is the resolved region used for the AWS client and for the
	// Source label. Lazily populated by initRdsClient.
	region string
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

	// Build the HTTP client from the provided HTTPClientConfig.
	client, err := config.NewClientFromConfig(d.cfg.HTTPClientConfig, "rds_sd")
	if err != nil {
		return err
	}

	// Resolve the region lazily. See RDSSDConfig.UnmarshalYAML.
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

	d.rds = newRDSClientAdapter(rds.NewFromConfig(cfg, func(options *rds.Options) {
		if d.cfg.Endpoint != "" {
			options.BaseEndpoint = &d.cfg.Endpoint
		}
		options.HTTPClient = client
	}))

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
	dbInstances := []types.DBInstance{}
	var nextToken *string

	filters := []types.Filter{
		{
			Name:   aws.String("db-cluster-id"),
			Values: []string{dbClusterARN},
		},
	}

	for _, f := range d.cfg.Filters {
		filters = append(filters, types.Filter{
			Name:   aws.String(f.Name),
			Values: f.Values,
		})
	}

	for {
		output, err := d.rds.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
			Filters:    filters,
			Marker:     nextToken,
			MaxRecords: aws.Int32(100),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe DB instances for cluster ARN %s: %w", dbClusterARN, err)
		}

		dbInstances = append(dbInstances, output.DBInstances...)

		if output.Marker == nil {
			break
		}
		nextToken = output.Marker
	}
	return dbInstances, nil
}

// rdsClusterWriters maps the instance identifier of every cluster member to its
// IsClusterWriter status.
func rdsClusterWriters(cluster types.DBCluster) map[string]bool {
	writers := make(map[string]bool, len(cluster.DBClusterMembers))
	for _, member := range cluster.DBClusterMembers {
		if member.DBInstanceIdentifier != nil && member.IsClusterWriter != nil {
			writers[*member.DBInstanceIdentifier] = *member.IsClusterWriter
		}
	}
	return writers
}

// rdsClusterLabels returns the labels describing the DB cluster. The same set
// applies to every instance of the cluster, so callers must copy it before
// adding instance specific labels to it.
func rdsClusterLabels(cluster types.DBCluster) model.LabelSet {
	labels := model.LabelSet{}

	setStringLabel(labels, rdsLabelClusterDBClusterArn, cluster.DBClusterArn)
	setStringLabel(labels, rdsLabelClusterDBClusterIdentifier, cluster.DBClusterIdentifier)
	setStringLabel(labels, rdsLabelClusterActivityStreamKinesisStreamName, cluster.ActivityStreamKinesisStreamName)
	setStringLabel(labels, rdsLabelClusterActivityStreamKMSKeyID, cluster.ActivityStreamKmsKeyId)
	setNonEmptyLabel(labels, rdsLabelClusterActivityStreamMode, cluster.ActivityStreamMode)
	setNonEmptyLabel(labels, rdsLabelClusterActivityStreamStatus, cluster.ActivityStreamStatus)
	setIntLabel(labels, rdsLabelClusterAllocatedStorage, cluster.AllocatedStorage)
	setBoolLabel(labels, rdsLabelClusterAutoMinorVersionUpgrade, cluster.AutoMinorVersionUpgrade)
	setTimeLabel(labels, rdsLabelClusterAutomaticRestartTime, cluster.AutomaticRestartTime)
	setStringLabel(labels, rdsLabelClusterAwsBackupRecoveryPointArn, cluster.AwsBackupRecoveryPointArn)
	setIntLabel(labels, rdsLabelClusterBacktrackConsumedChangeRecords, cluster.BacktrackConsumedChangeRecords)
	setIntLabel(labels, rdsLabelClusterBacktrackWindow, cluster.BacktrackWindow)
	setIntLabel(labels, rdsLabelClusterBackupRetentionPeriod, cluster.BackupRetentionPeriod)
	setIntLabel(labels, rdsLabelClusterCapacity, cluster.Capacity)
	setStringLabel(labels, rdsLabelClusterCharacterSetName, cluster.CharacterSetName)
	setStringLabel(labels, rdsLabelClusterCloneGroupID, cluster.CloneGroupId)
	setTimeLabel(labels, rdsLabelClusterClusterCreateTime, cluster.ClusterCreateTime)
	setNonEmptyLabel(labels, rdsLabelClusterClusterScalabilityType, cluster.ClusterScalabilityType)
	setBoolLabel(labels, rdsLabelClusterCopyTagsToSnapshot, cluster.CopyTagsToSnapshot)
	setBoolLabel(labels, rdsLabelClusterCrossAccountClone, cluster.CrossAccountClone)
	setStringLabel(labels, rdsLabelClusterDBClusterInstanceClass, cluster.DBClusterInstanceClass)
	setStringLabel(labels, rdsLabelClusterDBClusterParameterGroup, cluster.DBClusterParameterGroup)
	setStringLabel(labels, rdsLabelClusterDBSubnetGroup, cluster.DBSubnetGroup)
	setStringLabel(labels, rdsLabelClusterDBSystemID, cluster.DBSystemId)
	setNonEmptyLabel(labels, rdsLabelClusterDatabaseInsightsMode, cluster.DatabaseInsightsMode)
	setStringLabel(labels, rdsLabelClusterDatabaseName, cluster.DatabaseName)
	setStringLabel(labels, rdsLabelClusterDBClusterResourceID, cluster.DbClusterResourceId)
	setBoolLabel(labels, rdsLabelClusterDeletionProtection, cluster.DeletionProtection)
	setTimeLabel(labels, rdsLabelClusterEarliestBacktrackTime, cluster.EarliestBacktrackTime)
	setTimeLabel(labels, rdsLabelClusterEarliestRestorableTime, cluster.EarliestRestorableTime)
	setStringLabel(labels, rdsLabelClusterEndpoint, cluster.Endpoint)
	setStringLabel(labels, rdsLabelClusterEngine, cluster.Engine)
	setStringLabel(labels, rdsLabelClusterEngineLifecycleSupport, cluster.EngineLifecycleSupport)
	setStringLabel(labels, rdsLabelClusterEngineMode, cluster.EngineMode)
	setStringLabel(labels, rdsLabelClusterEngineVersion, cluster.EngineVersion)
	setStringLabel(labels, rdsLabelClusterGlobalClusterIdentifier, cluster.GlobalClusterIdentifier)
	setBoolLabel(labels, rdsLabelClusterGlobalWriteForwardingRequested, cluster.GlobalWriteForwardingRequested)
	setNonEmptyLabel(labels, rdsLabelClusterGlobalWriteForwardingStatus, cluster.GlobalWriteForwardingStatus)
	setStringLabel(labels, rdsLabelClusterHostedZoneID, cluster.HostedZoneId)
	setBoolLabel(labels, rdsLabelClusterHTTPEndpointEnabled, cluster.HttpEndpointEnabled)
	setBoolLabel(labels, rdsLabelClusterIAMDatabaseAuthenticationEnabled, cluster.IAMDatabaseAuthenticationEnabled)
	setTimeLabel(labels, rdsLabelClusterIOOptimizedNextAllowedModificationTime, cluster.IOOptimizedNextAllowedModificationTime)
	setIntLabel(labels, rdsLabelClusterIops, cluster.Iops)
	setStringLabel(labels, rdsLabelClusterKMSKeyID, cluster.KmsKeyId)
	setTimeLabel(labels, rdsLabelClusterLatestRestorableTime, cluster.LatestRestorableTime)
	setNonEmptyLabel(labels, rdsLabelClusterLocalWriteForwardingStatus, cluster.LocalWriteForwardingStatus)
	setStringLabel(labels, rdsLabelClusterMasterUsername, cluster.MasterUsername)
	setIntLabel(labels, rdsLabelClusterMonitoringInterval, cluster.MonitoringInterval)
	setStringLabel(labels, rdsLabelClusterMonitoringRoleArn, cluster.MonitoringRoleArn)
	setBoolLabel(labels, rdsLabelClusterMultiAZ, cluster.MultiAZ)
	setStringLabel(labels, rdsLabelClusterNetworkType, cluster.NetworkType)
	setStringLabel(labels, rdsLabelClusterPercentProgress, cluster.PercentProgress)
	setBoolLabel(labels, rdsLabelClusterPerformanceInsightsEnabled, cluster.PerformanceInsightsEnabled)
	setStringLabel(labels, rdsLabelClusterPerformanceInsightsKMSKeyID, cluster.PerformanceInsightsKMSKeyId)
	setIntLabel(labels, rdsLabelClusterPerformanceInsightsRetentionPeriod, cluster.PerformanceInsightsRetentionPeriod)
	setIntLabel(labels, rdsLabelClusterPort, cluster.Port)
	setStringLabel(labels, rdsLabelClusterPreferredBackupWindow, cluster.PreferredBackupWindow)
	setStringLabel(labels, rdsLabelClusterPreferredMaintenanceWindow, cluster.PreferredMaintenanceWindow)
	setBoolLabel(labels, rdsLabelClusterPubliclyAccessible, cluster.PubliclyAccessible)
	setStringLabel(labels, rdsLabelClusterReaderEndpoint, cluster.ReaderEndpoint)
	setStringLabel(labels, rdsLabelClusterReplicationSourceIdentifier, cluster.ReplicationSourceIdentifier)
	setStringLabel(labels, rdsLabelClusterServerlessV2PlatformVersion, cluster.ServerlessV2PlatformVersion)
	setStringLabel(labels, rdsLabelClusterStatus, cluster.Status)
	setBoolLabel(labels, rdsLabelClusterStorageEncrypted, cluster.StorageEncrypted)
	setNonEmptyLabel(labels, rdsLabelClusterStorageEncryptionType, cluster.StorageEncryptionType)
	setIntLabel(labels, rdsLabelClusterStorageThroughput, cluster.StorageThroughput)
	setStringLabel(labels, rdsLabelClusterStorageType, cluster.StorageType)
	setNonEmptyLabel(labels, rdsLabelClusterUpgradeRolloutOrder, cluster.UpgradeRolloutOrder)

	// Cluster tags.
	for _, tag := range cluster.TagList {
		if tag.Key != nil && tag.Value != nil {
			labels[model.LabelName(rdsLabelClusterTag+strutil.SanitizeLabelName(*tag.Key))] = model.LabelValue(*tag.Value)
		}
	}

	return labels
}

// addRDSInstanceLabels adds the labels describing the DB instance to labels.
// writers maps instance identifiers to their IsClusterWriter status, as
// returned by rdsClusterWriters.
func addRDSInstanceLabels(labels model.LabelSet, instance types.DBInstance, writers map[string]bool) {
	setStringLabel(labels, rdsLabelInstanceDBInstanceArn, instance.DBInstanceArn)
	if instance.DBInstanceIdentifier != nil {
		labels[rdsLabelInstanceDBInstanceIdentifier] = model.LabelValue(*instance.DBInstanceIdentifier)
		// Set IsClusterWriter based on cluster membership information.
		if isWriter, found := writers[*instance.DBInstanceIdentifier]; found {
			labels[rdsLabelInstanceIsClusterWriter] = model.LabelValue(strconv.FormatBool(isWriter))
		}
	}
	setBoolLabel(labels, rdsLabelInstanceActivityStreamEngineNativeAuditFieldsIncluded, instance.ActivityStreamEngineNativeAuditFieldsIncluded)
	setStringLabel(labels, rdsLabelInstanceActivityStreamKinesisStreamName, instance.ActivityStreamKinesisStreamName)
	setStringLabel(labels, rdsLabelInstanceActivityStreamKmsKeyID, instance.ActivityStreamKmsKeyId)
	setNonEmptyLabel(labels, rdsLabelInstanceActivityStreamMode, instance.ActivityStreamMode)
	setNonEmptyLabel(labels, rdsLabelInstanceActivityStreamPolicyStatus, instance.ActivityStreamPolicyStatus)
	setNonEmptyLabel(labels, rdsLabelInstanceActivityStreamStatus, instance.ActivityStreamStatus)
	setIntLabel(labels, rdsLabelInstanceAllocatedStorage, instance.AllocatedStorage)
	setBoolLabel(labels, rdsLabelInstanceAutoMinorVersionUpgrade, instance.AutoMinorVersionUpgrade)
	setTimeLabel(labels, rdsLabelInstanceAutomaticRestartTime, instance.AutomaticRestartTime)
	setNonEmptyLabel(labels, rdsLabelInstanceAutomationMode, instance.AutomationMode)
	setStringLabel(labels, rdsLabelInstanceAvailabilityZone, instance.AvailabilityZone)
	setStringLabel(labels, rdsLabelInstanceAwsBackupRecoveryPointArn, instance.AwsBackupRecoveryPointArn)
	setIntLabel(labels, rdsLabelInstanceBackupRetentionPeriod, instance.BackupRetentionPeriod)
	setStringLabel(labels, rdsLabelInstanceBackupTarget, instance.BackupTarget)
	setStringLabel(labels, rdsLabelInstanceCACertificateIdentifier, instance.CACertificateIdentifier)
	setStringLabel(labels, rdsLabelInstanceCharacterSetName, instance.CharacterSetName)
	setBoolLabel(labels, rdsLabelInstanceCopyTagsToSnapshot, instance.CopyTagsToSnapshot)
	setStringLabel(labels, rdsLabelInstanceCustomIamInstanceProfile, instance.CustomIamInstanceProfile)
	setBoolLabel(labels, rdsLabelInstanceCustomerOwnedIPEnabled, instance.CustomerOwnedIpEnabled)
	setStringLabel(labels, rdsLabelInstanceDBClusterIdentifier, instance.DBClusterIdentifier)
	setStringLabel(labels, rdsLabelInstanceDBInstanceClass, instance.DBInstanceClass)
	setStringLabel(labels, rdsLabelInstanceDBInstanceStatus, instance.DBInstanceStatus)
	setStringLabel(labels, rdsLabelInstanceDBName, instance.DBName)
	setIntLabel(labels, rdsLabelInstanceDBInstancePort, instance.DbInstancePort)
	setStringLabel(labels, rdsLabelInstanceDBResourceID, instance.DbiResourceId)
	setBoolLabel(labels, rdsLabelInstanceDedicatedLogVolume, instance.DedicatedLogVolume)
	setBoolLabel(labels, rdsLabelInstanceDeletionProtection, instance.DeletionProtection)
	if instance.Endpoint != nil {
		setStringLabel(labels, rdsLabelInstanceEndpointAddress, instance.Endpoint.Address)
		setStringLabel(labels, rdsLabelInstanceEndpointHostedZoneID, instance.Endpoint.HostedZoneId)
		setIntLabel(labels, rdsLabelInstanceEndpointPort, instance.Endpoint.Port)
	}
	setStringLabel(labels, rdsLabelInstanceEngine, instance.Engine)
	setStringLabel(labels, rdsLabelInstanceEngineLifecycleSupport, instance.EngineLifecycleSupport)
	setStringLabel(labels, rdsLabelInstanceEngineVersion, instance.EngineVersion)
	setStringLabel(labels, rdsLabelInstanceEnhancedMonitoringResourceArn, instance.EnhancedMonitoringResourceArn)
	setBoolLabel(labels, rdsLabelInstanceIAMDatabaseAuthenticationEnabled, instance.IAMDatabaseAuthenticationEnabled)
	setTimeLabel(labels, rdsLabelInstanceInstanceCreateTime, instance.InstanceCreateTime)
	setIntLabel(labels, rdsLabelInstanceIops, instance.Iops)
	setBoolLabel(labels, rdsLabelInstanceIsStorageConfigUpgradeAvailable, instance.IsStorageConfigUpgradeAvailable)
	setStringLabel(labels, rdsLabelInstanceKMSKeyID, instance.KmsKeyId)
	setTimeLabel(labels, rdsLabelInstanceLatestRestorableTime, instance.LatestRestorableTime)
	setStringLabel(labels, rdsLabelInstanceLicenseModel, instance.LicenseModel)
	if instance.ListenerEndpoint != nil {
		setStringLabel(labels, rdsLabelInstanceListenerEndpointAddress, instance.ListenerEndpoint.Address)
		setStringLabel(labels, rdsLabelInstanceListenerEndpointHostedZoneID, instance.ListenerEndpoint.HostedZoneId)
		setIntLabel(labels, rdsLabelInstanceListenerEndpointPort, instance.ListenerEndpoint.Port)
	}
	setStringLabel(labels, rdsLabelInstanceMasterUsername, instance.MasterUsername)
	setIntLabel(labels, rdsLabelInstanceMaxAllocatedStorage, instance.MaxAllocatedStorage)
	setIntLabel(labels, rdsLabelInstanceMonitoringInterval, instance.MonitoringInterval)
	setStringLabel(labels, rdsLabelInstanceMonitoringRoleArn, instance.MonitoringRoleArn)
	setBoolLabel(labels, rdsLabelInstanceMultiAZ, instance.MultiAZ)
	setBoolLabel(labels, rdsLabelInstanceMultiTenant, instance.MultiTenant)
	setStringLabel(labels, rdsLabelInstanceNcharCharacterSetName, instance.NcharCharacterSetName)
	setStringLabel(labels, rdsLabelInstanceNetworkType, instance.NetworkType)
	setStringLabel(labels, rdsLabelInstancePercentProgress, instance.PercentProgress)
	setBoolLabel(labels, rdsLabelInstancePerformanceInsightsEnabled, instance.PerformanceInsightsEnabled)
	setStringLabel(labels, rdsLabelInstancePerformanceInsightsKMSKeyID, instance.PerformanceInsightsKMSKeyId)
	setIntLabel(labels, rdsLabelInstancePerformanceInsightsRetentionPeriod, instance.PerformanceInsightsRetentionPeriod)
	setStringLabel(labels, rdsLabelInstancePreferredBackupWindow, instance.PreferredBackupWindow)
	setStringLabel(labels, rdsLabelInstancePreferredMaintenanceWindow, instance.PreferredMaintenanceWindow)
	setIntLabel(labels, rdsLabelInstancePromotionTier, instance.PromotionTier)
	setBoolLabel(labels, rdsLabelInstancePubliclyAccessible, instance.PubliclyAccessible)
	setStringLabel(labels, rdsLabelInstanceReadReplicaSourceDBClusterIdentifier, instance.ReadReplicaSourceDBClusterIdentifier)
	setStringLabel(labels, rdsLabelInstanceReadReplicaSourceDBInstanceIdentifier, instance.ReadReplicaSourceDBInstanceIdentifier)
	setNonEmptyLabel(labels, rdsLabelInstanceReplicaMode, instance.ReplicaMode)
	setTimeLabel(labels, rdsLabelInstanceResumeFullAutomationModeTime, instance.ResumeFullAutomationModeTime)
	setStringLabel(labels, rdsLabelInstanceSecondaryAvailabilityZone, instance.SecondaryAvailabilityZone)
	setBoolLabel(labels, rdsLabelInstanceStorageEncrypted, instance.StorageEncrypted)
	setNonEmptyLabel(labels, rdsLabelInstanceStorageEncryptionType, instance.StorageEncryptionType)
	setIntLabel(labels, rdsLabelInstanceStorageThroughput, instance.StorageThroughput)
	setStringLabel(labels, rdsLabelInstanceStorageType, instance.StorageType)
	setStringLabel(labels, rdsLabelInstanceStorageVolumeStatus, instance.StorageVolumeStatus)
	setStringLabel(labels, rdsLabelInstanceTdeCredentialArn, instance.TdeCredentialArn)
	setStringLabel(labels, rdsLabelInstanceTimezone, instance.Timezone)
	setNonEmptyLabel(labels, rdsLabelInstanceUpgradeRolloutOrder, instance.UpgradeRolloutOrder)
	if instance.DBSubnetGroup != nil {
		setStringLabel(labels, rdsLabelInstanceDBSubnetGroup, instance.DBSubnetGroup.DBSubnetGroupName)
	}
	setStringLabel(labels, rdsLabelInstanceDBSystemID, instance.DBSystemId)
	setNonEmptyLabel(labels, rdsLabelInstanceDatabaseInsightsMode, instance.DatabaseInsightsMode)

	// Instance tags.
	for _, tag := range instance.TagList {
		if tag.Key != nil && tag.Value != nil {
			labels[model.LabelName(rdsLabelInstanceTag+strutil.SanitizeLabelName(*tag.Key))] = model.LabelValue(*tag.Value)
		}
	}
}

func (d *RDSDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	err := d.initRdsClient(ctx)
	if err != nil {
		return nil, err
	}

	tg := &targetgroup.Group{
		Source: d.region,
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

			// The cluster labels and the writer status of its members are the
			// same for every instance of the cluster, so compute them once.
			clusterLabels := rdsClusterLabels(cluster)
			writers := rdsClusterWriters(cluster)

			for _, instance := range instances {
				labels := clusterLabels.Clone()
				addRDSInstanceLabels(labels, instance, writers)

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
