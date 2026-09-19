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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/mq"
	"github.com/aws/aws-sdk-go-v2/service/mq/types"
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
	// endpointPrefix is the prefix for the broker endpoint that we want to scrape metrics from.
	// The /metrics endpoint is only exposed over https, not http, so we need to check for this prefix.
	endpointPrefix = "https://"

	// data labels are determined from the DescribeBrokerOutput struct returned by the DescribeBroker API call.
	// https://github.com/aws/aws-sdk-go-v2/blob/main/service/mq/api_op_DescribeBroker.go
	mqLabel = model.MetaLabelPrefix + "mq_"

	mqLabelAuthenticationStrategy  = mqLabel + "authentication_strategy"
	mqLabelAutoMinorVersionUpgrade = mqLabel + "auto_minor_version_upgrade"
	mqLabelBroker                  = mqLabel + "broker_"
	mqLabelBrokerName              = mqLabelBroker + "name"
	mqLabelBrokerState             = mqLabelBroker + "state"
	mqLabelBrokerARN               = mqLabelBroker + "arn"
	mqLabelBrokerID                = mqLabelBroker + "id"

	// Broker instance labels.
	mqLabelBrokerInstance          = mqLabelBroker + "instance_"
	mqLabelBrokerInstanceIPAddress = mqLabelBrokerInstance + "ip_address"
	mqLabelBrokerInstanceEndpoint  = mqLabelBrokerInstance + "endpoint"

	// Configurations labels.
	mqLabelConfigurations                = mqLabel + "configurations_"
	mqLabelConfigurationsCurrent         = mqLabelConfigurations + "current_"
	mqLabelConfigurationsCurrentID       = mqLabelConfigurationsCurrent + "id"
	mqLabelConfigurationsCurrentRevision = mqLabelConfigurationsCurrent + "revision"

	mqLabelCreated = mqLabel + "created"

	// Data replication labels.
	mqLabelDataReplication     = mqLabel + "data_replication_"
	mqLabelDataReplicationMode = mqLabelDataReplication + "mode"
	// Data replication metadata labels.
	mqLabelDataReplicationMetadata                    = mqLabelDataReplication + "metadata_"
	mqLabelDataReplicationMetadataDataReplicationRole = mqLabelDataReplicationMetadata + "data_replication_role"
	// Data replication counterpart labels.
	mqLabelDataReplicationMetadataDataReplicationCounterpart         = mqLabelDataReplicationMetadata + "data_replication_counterpart_"
	mqLabelDataReplicationMetadataDataReplicationCounterpartBrokerID = mqLabelDataReplicationMetadataDataReplicationCounterpart + "broker_id"
	mqLabelDataReplicationMetadataDataReplicationCounterpartRegion   = mqLabelDataReplicationMetadataDataReplicationCounterpart + "region"

	mqLabelDeploymentMode = mqLabel + "deployment_mode"

	// Encryption options labels.
	mqLabelEncryptionOptions            = mqLabel + "encryption_options_"
	mqLabelEncryptionOptionsUseAwsOwned = mqLabelEncryptionOptions + "use_aws_owned_key"
	mqLabelEncryptionOptionsKms         = mqLabelEncryptionOptions + "kms_"
	mqLabelEncryptionOptionsKmsKeyID    = mqLabelEncryptionOptionsKms + "key_id"

	// Engine labels.
	mqLabelEngine        = mqLabel + "engine_"
	mqLabelEngineType    = mqLabelEngine + "type"
	mqLabelEngineVersion = mqLabelEngine + "version"

	mqLabelHostInstanceType = mqLabel + "host_instance_type"

	// LDAP server metadata labels.
	mqLabelLdapServerMetadata                       = mqLabel + "ldap_server_metadata_"
	mqLabelLdapServerMetadataHosts                  = mqLabelLdapServerMetadata + "hosts"
	mqLabelLdapServerMetadataRoleBase               = mqLabelLdapServerMetadata + "role_base"
	mqLabelLdapServerMetadataRoleSearchMatching     = mqLabelLdapServerMetadata + "role_search_matching"
	mqLabelLdapServerMetadataServiceAccountUsername = mqLabelLdapServerMetadata + "service_account_username"
	mqLabelLdapServerMetadataUserBase               = mqLabelLdapServerMetadata + "user_base"
	mqLabelLdapServerMetadataUserSearchMatching     = mqLabelLdapServerMetadata + "user_search_matching"
	mqLabelLdapServerMetadataRoleName               = mqLabelLdapServerMetadata + "role_name"
	mqLabelLdapServerMetadataRoleSearchSubtree      = mqLabelLdapServerMetadata + "role_search_subtree"
	mqLabelLdapServerMetadataUserRoleName           = mqLabelLdapServerMetadata + "user_role_name"
	mqLabelLdapServerMetadataUserSearchSubtree      = mqLabelLdapServerMetadata + "user_search_subtree"

	// Logs labels.
	mqLabelLogs = mqLabel + "logs_"
	// Audit logs labels.
	mqLabelLogsAudit         = mqLabelLogs + "audit"
	mqLabelLogsAuditLogGroup = mqLabelLogsAudit + "_log_group"
	// General logs labels.
	mqLabelLogsGeneral         = mqLabelLogs + "general"
	mqLabelLogsGeneralLogGroup = mqLabelLogsGeneral + "_log_group"
	// Pending logs labels.
	mqLabelLogsPending        = mqLabelLogs + "pending_"
	mqLabelLogsPendingAudit   = mqLabelLogsPending + "audit"
	mqLabelLogsPendingGeneral = mqLabelLogsPending + "general"

	// Maintenance window start time labels.
	mqLabelMaintenanceWindowStartTime          = mqLabel + "maintenance_window_start_time"
	mqLabelMaintenanceWindowStartTimeDayOfWeek = mqLabelMaintenanceWindowStartTime + "_day_of_week"
	mqLabelMaintenanceWindowStartTimeTimeOfDay = mqLabelMaintenanceWindowStartTime + "_time_of_day"
	mqLabelMaintenanceWindowStartTimeTimeZone  = mqLabelMaintenanceWindowStartTime + "_time_zone"

	// Pending.
	mqLabelPending                       = mqLabel + "pending_"
	mqLabelPendingAuthenticationStrategy = mqLabelPending + "authentication_strategy"
	// Data replication labels.
	mqLabelPendingDataReplication     = mqLabelPending + "data_replication_"
	mqLabelPendingDataReplicationMode = mqLabelPendingDataReplication + "mode"
	// Data replication metadata labels.
	mqLabelPendingDataReplicationMetadata                    = mqLabelPendingDataReplication + "metadata_"
	mqLabelPendingDataReplicationMetadataDataReplicationRole = mqLabelPendingDataReplicationMetadata + "data_replication_role"
	// Data replication counterpart labels.
	mqLabelPendingDataReplicationMetadataDataReplicationCounterpart         = mqLabelPendingDataReplicationMetadata + "data_replication_counterpart_"
	mqLabelPendingDataReplicationMetadataDataReplicationCounterpartBrokerID = mqLabelPendingDataReplicationMetadataDataReplicationCounterpart + "broker_id"
	mqLabelPendingDataReplicationMetadataDataReplicationCounterpartRegion   = mqLabelPendingDataReplicationMetadataDataReplicationCounterpart + "region"

	mqLabelPendingEngineVersion    = mqLabelPending + "engine_version"
	mqLabelPendingHostInstanceType = mqLabelPending + "host_instance_type"

	// LDAP server metadata labels.
	mqLabelPendingLdapServerMetadata                       = mqLabelPending + "ldap_server_metadata_"
	mqLabelPendingLdapServerMetadataHosts                  = mqLabelPendingLdapServerMetadata + "hosts"
	mqLabelPendingLdapServerMetadataRoleBase               = mqLabelPendingLdapServerMetadata + "role_base"
	mqLabelPendingLdapServerMetadataRoleSearchMatching     = mqLabelPendingLdapServerMetadata + "role_search_matching"
	mqLabelPendingLdapServerMetadataServiceAccountUsername = mqLabelPendingLdapServerMetadata + "service_account_username"
	mqLabelPendingLdapServerMetadataUserBase               = mqLabelPendingLdapServerMetadata + "user_base"
	mqLabelPendingLdapServerMetadataUserSearchMatching     = mqLabelPendingLdapServerMetadata + "user_search_matching"
	mqLabelPendingLdapServerMetadataRoleName               = mqLabelPendingLdapServerMetadata + "role_name"
	mqLabelPendingLdapServerMetadataRoleSearchSubtree      = mqLabelPendingLdapServerMetadata + "role_search_subtree"
	mqLabelPendingLdapServerMetadataUserRoleName           = mqLabelPendingLdapServerMetadata + "user_role_name"
	mqLabelPendingLdapServerMetadataUserSearchSubtree      = mqLabelPendingLdapServerMetadata + "user_search_subtree"

	mqLabelPendingSecurityGroups = mqLabelPending + "security_groups"
	mqLabelPendingStorageSize    = mqLabelPending + "storage_size"

	mqLabelPubliclyAccessible = mqLabel + "publicly_accessible"
	mqLabelSecurityGroups     = mqLabel + "security_groups"
	mqLabelStorageSize        = mqLabel + "storage_size"
	mqLabelStorageType        = mqLabel + "storage_type"
	mqLabelSubnetIDs          = mqLabel + "subnet_ids"
	mqLabelTags               = mqLabel + "tags_"
)

// DefaultMQSDConfig is the default MQ SD configuration.
var DefaultMQSDConfig = MQSDConfig{
	RefreshInterval:    model.Duration(60 * time.Second),
	RequestConcurrency: 10,
	HTTPClientConfig:   config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&MQSDConfig{})
}

// MQSDConfig is the configuration for MQ based service discovery.
type MQSDConfig struct {
	Region          string         `yaml:"region"`
	Endpoint        string         `yaml:"endpoint"`
	AccessKey       string         `yaml:"access_key,omitempty"`
	SecretKey       config.Secret  `yaml:"secret_key,omitempty"`
	Profile         string         `yaml:"profile,omitempty"`
	RoleARN         string         `yaml:"role_arn,omitempty"`
	ExternalID      string         `yaml:"external_id,omitempty"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`

	RequestConcurrency int                     `yaml:"request_concurrency,omitempty"`
	HTTPClientConfig   config.HTTPClientConfig `yaml:",inline"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*MQSDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &mqMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the MQ Config.
func (*MQSDConfig) Name() string { return "mq" }

// NewDiscoverer returns a Discoverer for the MQ Config.
func (c *MQSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewMQDiscovery(c, opts)
}

// SetDirectory joins any relative file paths with dir.
func (c *MQSDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the MQ Config.
// Region resolution is deferred to initMqClient; see loadRegion.
func (c *MQSDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultMQSDConfig
	type plain MQSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	return c.HTTPClientConfig.Validate()
}

type mqClient interface {
	DescribeBroker(context.Context, *mq.DescribeBrokerInput, ...func(*mq.Options)) (*mq.DescribeBrokerOutput, error)
	ListBrokers(context.Context, *mq.ListBrokersInput, ...func(*mq.Options)) (*mq.ListBrokersOutput, error)
}

// mqClientAdapter captures only the MQ API calls AWS discovery uses
// as method-value closures, keeping the concrete *mq.Client out of any
// interface-boxed struct field. See ec2ClientAdapter for the full rationale:
// this stops the linker from retaining the entire MQ API surface (~1.4 MB).
type mqClientAdapter struct {
	describeBroker func(context.Context, *mq.DescribeBrokerInput, ...func(*mq.Options)) (*mq.DescribeBrokerOutput, error)
	listBrokers    func(context.Context, *mq.ListBrokersInput, ...func(*mq.Options)) (*mq.ListBrokersOutput, error)
}

func newMQClientAdapter(c *mq.Client) mqClientAdapter {
	return mqClientAdapter{
		describeBroker: c.DescribeBroker,
		listBrokers:    c.ListBrokers,
	}
}

func (a mqClientAdapter) DescribeBroker(ctx context.Context, params *mq.DescribeBrokerInput, optFns ...func(*mq.Options)) (*mq.DescribeBrokerOutput, error) {
	return a.describeBroker(ctx, params, optFns...)
}

func (a mqClientAdapter) ListBrokers(ctx context.Context, params *mq.ListBrokersInput, optFns ...func(*mq.Options)) (*mq.ListBrokersOutput, error) {
	return a.listBrokers(ctx, params, optFns...)
}

// MQDiscovery periodically performs MQ-SD requests. It implements
// the Discoverer interface.
type MQDiscovery struct {
	*refresh.Discovery
	logger *slog.Logger
	cfg    *MQSDConfig
	mq     mqClient

	// region is the resolved region used for the AWS client and for the
	// Source label. Lazily populated by initMqClient.
	region string
}

// NewMQDiscovery returns a new MQDiscovery which periodically refreshes its targets.
func NewMQDiscovery(conf *MQSDConfig, opts discovery.DiscovererOptions) (*MQDiscovery, error) {
	m, ok := opts.Metrics.(*mqMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if opts.Logger == nil {
		opts.Logger = promslog.NewNopLogger()
	}
	d := &MQDiscovery{
		logger: opts.Logger,
		cfg:    conf,
	}
	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "mq",
			Interval:            time.Duration(d.cfg.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *MQDiscovery) initMqClient(ctx context.Context) error {
	if d.mq != nil {
		return nil
	}

	// Build the HTTP client from the provided HTTPClientConfig.
	client, err := config.NewClientFromConfig(d.cfg.HTTPClientConfig, "mq_sd")
	if err != nil {
		return err
	}

	// Resolve the region lazily. See MQSDConfig.UnmarshalYAML.
	d.region, err = loadRegion(ctx, d.cfg.Region)
	if err != nil {
		return err
	}

	// Build the AWS config with the resolved region.
	var configOptions []func(*awsConfig.LoadOptions) error
	configOptions = append(configOptions, awsConfig.WithRegion(d.region))
	configOptions = append(configOptions, awsConfig.WithHTTPClient(client))

	// Only set static credentials if both access key and secret key are provided
	// Otherwise, let AWS SDK use its default credential chain.
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

	d.mq = newMQClientAdapter(mq.NewFromConfig(cfg, func(options *mq.Options) {
		if d.cfg.Endpoint != "" {
			options.BaseEndpoint = &d.cfg.Endpoint
		}
		options.HTTPClient = client
	}))

	// Test credentials by making a simple API call.
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err = d.mq.ListBrokers(testCtx, &mq.ListBrokersInput{})
	if err != nil {
		d.logger.Error("Failed to test MQ credentials", "error", err)
		return fmt.Errorf("MQ credential test failed: %w", err)
	}

	return nil
}

// describeBrokers describes the brokers with the given broker IDs concurrently,
// returning a slice of DescribeBrokerOutput.
func (d *MQDiscovery) describeBrokers(ctx context.Context, brokerIDs []string) ([]*mq.DescribeBrokerOutput, error) {
	var (
		brokers []*mq.DescribeBrokerOutput
		mu      sync.Mutex
	)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for _, brokerID := range brokerIDs {
		errg.Go(func() error {
			output, err := d.mq.DescribeBroker(ectx, &mq.DescribeBrokerInput{
				BrokerId: aws.String(brokerID),
			})
			if err != nil {
				return fmt.Errorf("error describing broker %s: %w", brokerID, err)
			}
			mu.Lock()
			brokers = append(brokers, output)
			mu.Unlock()
			return nil
		})
	}

	return brokers, errg.Wait()
}

// listBrokers lists all broker IDs in the AWS account,
// handling pagination and returning a slice of broker IDs.
func (d *MQDiscovery) listBrokers(ctx context.Context) ([]string, error) {
	var (
		brokerIDs []string
		nextToken *string
	)
	for {
		resp, err := d.mq.ListBrokers(ctx, &mq.ListBrokersInput{
			NextToken:  nextToken,
			MaxResults: aws.Int32(100),
		})
		if err != nil {
			return nil, err
		}
		for _, broker := range resp.BrokerSummaries {
			brokerIDs = append(brokerIDs, aws.ToString(broker.BrokerId))
		}
		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}
	return brokerIDs, nil
}

func (d *MQDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	err := d.initMqClient(ctx)
	if err != nil {
		return nil, err
	}

	tg := &targetgroup.Group{
		Source: d.region,
	}

	brokerIDs, err := d.listBrokers(ctx)
	if err != nil {
		return nil, fmt.Errorf("error listing brokers: %w", err)
	}

	brokers, err := d.describeBrokers(ctx, brokerIDs)
	if err != nil {
		return nil, fmt.Errorf("error describing brokers: %w", err)
	}

	var (
		targetsMu sync.Mutex
		wg        sync.WaitGroup
	)
	for _, broker := range brokers {
		wg.Add(1)

		go func(broker *mq.DescribeBrokerOutput) {
			defer wg.Done()

			// Add broker-level mandatory labels.
			labels := model.LabelSet{
				mqLabelAuthenticationStrategy: model.LabelValue(string(broker.AuthenticationStrategy)),
				mqLabelBrokerState:            model.LabelValue(string(broker.BrokerState)),
				mqLabelDeploymentMode:         model.LabelValue(string(broker.DeploymentMode)),
				mqLabelEngineType:             model.LabelValue(string(broker.EngineType)),
				mqLabelSecurityGroups:         model.LabelValue(strings.Join(broker.SecurityGroups, ",")),
				mqLabelStorageType:            model.LabelValue(string(broker.StorageType)),
				mqLabelSubnetIDs:              model.LabelValue(strings.Join(broker.SubnetIds, ",")),
			}

			// Add broker-level optional labels.
			// Auto minor version upgrade label.
			if broker.AutoMinorVersionUpgrade != nil {
				labels[mqLabelAutoMinorVersionUpgrade] = model.LabelValue(strconv.FormatBool(*broker.AutoMinorVersionUpgrade))
			}

			// Add broker-level optional labels.
			if broker.BrokerArn != nil {
				labels[mqLabelBrokerARN] = model.LabelValue(aws.ToString(broker.BrokerArn))
			}

			// Add broker-level optional labels.
			if broker.BrokerId != nil {
				labels[mqLabelBrokerID] = model.LabelValue(aws.ToString(broker.BrokerId))
			}

			// Add broker-name label if it exists.
			if broker.BrokerName != nil {
				labels[mqLabelBrokerName] = model.LabelValue(aws.ToString(broker.BrokerName))
			}

			// configuration labels.
			maps.Copy(labels, mqConfigurationLabels(broker.Configurations))

			// created label.
			if broker.Created != nil {
				labels[mqLabelCreated] = model.LabelValue(broker.Created.Format(time.RFC3339))
			}

			// data replication mode label.
			if broker.DataReplicationMode != "" {
				labels[mqLabelDataReplicationMode] = model.LabelValue(string(broker.DataReplicationMode))
			}

			// data replication metadata labels.
			maps.Copy(labels, mqDataReplicationMetadataLabels(broker.DataReplicationMetadata, false))

			// encryption options labels.
			maps.Copy(labels, mqEncryptionOptionsLabels(broker.EncryptionOptions))

			// engine labels
			if broker.EngineVersion != nil {
				labels[mqLabelEngineVersion] = model.LabelValue(aws.ToString(broker.EngineVersion))
			}

			// host instance type label.
			if broker.HostInstanceType != nil {
				labels[mqLabelHostInstanceType] = model.LabelValue(aws.ToString(broker.HostInstanceType))
			}

			// LDAP server metadata labels.
			maps.Copy(labels, mqLdapServerMetadataLabels(broker.LdapServerMetadata, false))

			// logs labels.
			maps.Copy(labels, mqLogsLabels(broker.Logs))

			// maintenance window start time labels.
			maps.Copy(labels, mqMaintenanceWindowStartTimeLabels(broker.MaintenanceWindowStartTime))

			// pending authentication strategy label.
			if broker.PendingAuthenticationStrategy != "" {
				labels[mqLabelPendingAuthenticationStrategy] = model.LabelValue(string(broker.PendingAuthenticationStrategy))
			}

			// pending data replication mode label.
			if broker.PendingDataReplicationMode != "" {
				labels[mqLabelPendingDataReplicationMode] = model.LabelValue(string(broker.PendingDataReplicationMode))
			}

			// pending data replication labels.
			maps.Copy(labels, mqDataReplicationMetadataLabels(broker.PendingDataReplicationMetadata, true))

			// pending engine version label.
			if broker.PendingEngineVersion != nil {
				labels[mqLabelPendingEngineVersion] = model.LabelValue(aws.ToString(broker.PendingEngineVersion))
			}

			// pending host instance type label.
			if broker.PendingHostInstanceType != nil {
				labels[mqLabelPendingHostInstanceType] = model.LabelValue(aws.ToString(broker.PendingHostInstanceType))
			}

			// pending LDAP server metadata labels.
			maps.Copy(labels, mqLdapServerMetadataLabels(broker.PendingLdapServerMetadata, true))

			// pending security groups label.
			if broker.PendingSecurityGroups != nil {
				labels[mqLabelPendingSecurityGroups] = model.LabelValue(strings.Join(broker.PendingSecurityGroups, ","))
			}

			// pending storage size label.
			if broker.PendingStorageSize != nil {
				labels[mqLabelPendingStorageSize] = model.LabelValue(strconv.FormatInt(int64(*broker.PendingStorageSize), 10))
			}

			// publicly accessible label.
			if broker.PubliclyAccessible != nil {
				labels[mqLabelPubliclyAccessible] = model.LabelValue(strconv.FormatBool(*broker.PubliclyAccessible))
			}

			// storage size label.
			if broker.StorageSize != nil {
				labels[mqLabelStorageSize] = model.LabelValue(strconv.FormatInt(int64(*broker.StorageSize), 10))
			}

			// tags labels.
			for k, v := range broker.Tags {
				labels[model.LabelName(mqLabelTags+strutil.SanitizeLabelName(k))] = model.LabelValue(v)
			}

			// broker instance labels.
			// Each https endpoint is its own scrape target. A single BrokerInstance
			// can list more than one: RabbitMQ CLUSTER_MULTI_AZ brokers report all
			// their nodes as multiple https endpoints (same host, distinct ports) on
			// one BrokerInstance, while ActiveMQ ACTIVE_STANDBY_MULTI_AZ brokers use
			// one BrokerInstance per node, each with a single https endpoint.
			// IpAddress does not apply to RabbitMQ brokers, so it is only set for
			// instances that report one.
			for _, instance := range broker.BrokerInstances {
				// https is the /metrics endpoint, which is what we want to scrape.
				// Brokers only expose the /metrics endpoint over https, not http, and
				// because the port changes per node we can't hardcode it.
				for _, endpoint := range instance.Endpoints {
					if !strings.HasPrefix(endpoint, endpointPrefix) {
						continue
					}

					instanceLabels := labels.Clone()
					instanceLabels[mqLabelBrokerInstanceEndpoint] = model.LabelValue(endpoint)
					instanceLabels[model.AddressLabel] = model.LabelValue(strings.TrimPrefix(endpoint, endpointPrefix))

					// broker instance IP address label
					if instance.IpAddress != nil {
						instanceLabels[mqLabelBrokerInstanceIPAddress] = model.LabelValue(aws.ToString(instance.IpAddress))
					}

					targetsMu.Lock()
					tg.Targets = append(tg.Targets, instanceLabels)
					targetsMu.Unlock()
				}
			}
		}(broker)
	}
	wg.Wait()

	return []*targetgroup.Group{tg}, nil
}

// mqConfigurationLabels returns a set of labels for the given broker's configuration.
func mqConfigurationLabels(output *types.Configurations) model.LabelSet {
	labels := model.LabelSet{}

	if output == nil {
		return labels
	}

	if output.Current != nil {
		if output.Current.Id != nil {
			labels[mqLabelConfigurationsCurrentID] = model.LabelValue(aws.ToString(output.Current.Id))
		}
		if output.Current.Revision != nil {
			labels[mqLabelConfigurationsCurrentRevision] = model.LabelValue(strconv.Itoa(int(*output.Current.Revision)))
		}
	}

	return labels
}

// mqDataReplicationMetadataLabels returns a set of labels for the given broker's
// data replication metadata. pending selects between the current and the
// Pending-prefixed label names, since this helper is used for both
// broker.DataReplicationMetadata and broker.PendingDataReplicationMetadata and
// the two must not collide under the same label names.
func mqDataReplicationMetadataLabels(output *types.DataReplicationMetadataOutput, pending bool) model.LabelSet {
	labels := model.LabelSet{}

	if output == nil {
		return labels
	}

	roleLabel := model.LabelName(mqLabelDataReplicationMetadataDataReplicationRole)
	counterpartBrokerIDLabel := model.LabelName(mqLabelDataReplicationMetadataDataReplicationCounterpartBrokerID)
	counterpartRegionLabel := model.LabelName(mqLabelDataReplicationMetadataDataReplicationCounterpartRegion)
	if pending {
		roleLabel = mqLabelPendingDataReplicationMetadataDataReplicationRole
		counterpartBrokerIDLabel = mqLabelPendingDataReplicationMetadataDataReplicationCounterpartBrokerID
		counterpartRegionLabel = mqLabelPendingDataReplicationMetadataDataReplicationCounterpartRegion
	}

	if output.DataReplicationRole != nil {
		labels[roleLabel] = model.LabelValue(*output.DataReplicationRole)
	}
	if output.DataReplicationCounterpart != nil {
		if output.DataReplicationCounterpart.BrokerId != nil {
			labels[counterpartBrokerIDLabel] = model.LabelValue(aws.ToString(output.DataReplicationCounterpart.BrokerId))
		}
		if output.DataReplicationCounterpart.Region != nil {
			labels[counterpartRegionLabel] = model.LabelValue(aws.ToString(output.DataReplicationCounterpart.Region))
		}
	}

	return labels
}

// mqEncryptionOptionsLabels returns a set of labels for the given broker's encryption options.
func mqEncryptionOptionsLabels(output *types.EncryptionOptions) model.LabelSet {
	labels := model.LabelSet{}

	if output == nil {
		return labels
	}

	labels[mqLabelEncryptionOptionsUseAwsOwned] = model.LabelValue(strconv.FormatBool(*output.UseAwsOwnedKey))
	if output.KmsKeyId != nil {
		labels[mqLabelEncryptionOptionsKmsKeyID] = model.LabelValue(aws.ToString(output.KmsKeyId))
	}

	return labels
}

// mqLdapServerMetadataLabels returns a set of labels for the given broker's
// LDAP server metadata. pending selects between the current and the
// Pending-prefixed label names, since this helper is used for both
// broker.LdapServerMetadata and broker.PendingLdapServerMetadata and the two
// must not collide under the same label names.
func mqLdapServerMetadataLabels(output *types.LdapServerMetadataOutput, pending bool) model.LabelSet {
	labels := model.LabelSet{}

	if output == nil {
		return labels
	}

	hostsLabel := model.LabelName(mqLabelLdapServerMetadataHosts)
	roleBaseLabel := model.LabelName(mqLabelLdapServerMetadataRoleBase)
	roleSearchMatchingLabel := model.LabelName(mqLabelLdapServerMetadataRoleSearchMatching)
	serviceAccountUsernameLabel := model.LabelName(mqLabelLdapServerMetadataServiceAccountUsername)
	userBaseLabel := model.LabelName(mqLabelLdapServerMetadataUserBase)
	userSearchMatchingLabel := model.LabelName(mqLabelLdapServerMetadataUserSearchMatching)
	roleNameLabel := model.LabelName(mqLabelLdapServerMetadataRoleName)
	roleSearchSubtreeLabel := model.LabelName(mqLabelLdapServerMetadataRoleSearchSubtree)
	userRoleNameLabel := model.LabelName(mqLabelLdapServerMetadataUserRoleName)
	userSearchSubtreeLabel := model.LabelName(mqLabelLdapServerMetadataUserSearchSubtree)
	if pending {
		hostsLabel = mqLabelPendingLdapServerMetadataHosts
		roleBaseLabel = mqLabelPendingLdapServerMetadataRoleBase
		roleSearchMatchingLabel = mqLabelPendingLdapServerMetadataRoleSearchMatching
		serviceAccountUsernameLabel = mqLabelPendingLdapServerMetadataServiceAccountUsername
		userBaseLabel = mqLabelPendingLdapServerMetadataUserBase
		userSearchMatchingLabel = mqLabelPendingLdapServerMetadataUserSearchMatching
		roleNameLabel = mqLabelPendingLdapServerMetadataRoleName
		roleSearchSubtreeLabel = mqLabelPendingLdapServerMetadataRoleSearchSubtree
		userRoleNameLabel = mqLabelPendingLdapServerMetadataUserRoleName
		userSearchSubtreeLabel = mqLabelPendingLdapServerMetadataUserSearchSubtree
	}

	if output.Hosts != nil {
		labels[hostsLabel] = model.LabelValue(strings.Join(output.Hosts, ","))
	}
	if output.RoleBase != nil {
		labels[roleBaseLabel] = model.LabelValue(aws.ToString(output.RoleBase))
	}
	if output.RoleSearchMatching != nil {
		labels[roleSearchMatchingLabel] = model.LabelValue(aws.ToString(output.RoleSearchMatching))
	}
	if output.ServiceAccountUsername != nil {
		labels[serviceAccountUsernameLabel] = model.LabelValue(aws.ToString(output.ServiceAccountUsername))
	}
	if output.UserBase != nil {
		labels[userBaseLabel] = model.LabelValue(aws.ToString(output.UserBase))
	}
	if output.UserSearchMatching != nil {
		labels[userSearchMatchingLabel] = model.LabelValue(aws.ToString(output.UserSearchMatching))
	}
	if output.RoleName != nil {
		labels[roleNameLabel] = model.LabelValue(aws.ToString(output.RoleName))
	}
	if output.RoleSearchSubtree != nil {
		labels[roleSearchSubtreeLabel] = model.LabelValue(strconv.FormatBool(*output.RoleSearchSubtree))
	}
	if output.UserRoleName != nil {
		labels[userRoleNameLabel] = model.LabelValue(aws.ToString(output.UserRoleName))
	}
	if output.UserSearchSubtree != nil {
		labels[userSearchSubtreeLabel] = model.LabelValue(strconv.FormatBool(*output.UserSearchSubtree))
	}

	return labels
}

// mqLogsLabels returns a set of labels for the given broker's logs configuration.
func mqLogsLabels(output *types.LogsSummary) model.LabelSet {
	labels := model.LabelSet{}

	if output == nil {
		return labels
	}

	if output.Audit != nil {
		labels[mqLabelLogsAudit] = model.LabelValue(strconv.FormatBool(*output.Audit))
	}
	if output.AuditLogGroup != nil {
		labels[mqLabelLogsAuditLogGroup] = model.LabelValue(aws.ToString(output.AuditLogGroup))
	}
	if output.General != nil {
		labels[mqLabelLogsGeneral] = model.LabelValue(strconv.FormatBool(*output.General))
	}
	if output.GeneralLogGroup != nil {
		labels[mqLabelLogsGeneralLogGroup] = model.LabelValue(aws.ToString(output.GeneralLogGroup))
	}
	if output.Pending != nil {
		if output.Pending.Audit != nil {
			labels[mqLabelLogsPendingAudit] = model.LabelValue(strconv.FormatBool(*output.Pending.Audit))
		}
		if output.Pending.General != nil {
			labels[mqLabelLogsPendingGeneral] = model.LabelValue(strconv.FormatBool(*output.Pending.General))
		}
	}

	return labels
}

// mqMaintenanceWindowStartTimeLabels returns a set of labels for the given broker's maintenance window start time.
func mqMaintenanceWindowStartTimeLabels(output *types.WeeklyStartTime) model.LabelSet {
	labels := model.LabelSet{}

	if output == nil {
		return labels
	}

	if output.DayOfWeek != "" {
		labels[mqLabelMaintenanceWindowStartTimeDayOfWeek] = model.LabelValue(string(output.DayOfWeek))
	}
	if output.TimeOfDay != nil {
		labels[mqLabelMaintenanceWindowStartTimeTimeOfDay] = model.LabelValue(aws.ToString(output.TimeOfDay))
	}
	if output.TimeZone != nil {
		labels[mqLabelMaintenanceWindowStartTimeTimeZone] = model.LabelValue(aws.ToString(output.TimeZone))
	}

	return labels
}
