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
	"github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/aws/aws-sdk-go-v2/service/kafka/types"
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

type NodeType string

const (
	NodeTypeBroker     NodeType = "BROKER"
	NodeTypeController NodeType = "CONTROLLER"
)

const (
	mskLabel = model.MetaLabelPrefix + "msk_"

	// Cluster labels.
	mskLabelCluster                      = mskLabel + "cluster_"
	mskLabelClusterName                  = mskLabelCluster + "name"
	mskLabelClusterARN                   = mskLabelCluster + "arn"
	mskLabelClusterState                 = mskLabelCluster + "state"
	mskLabelClusterType                  = mskLabelCluster + "type"
	mskLabelClusterVersion               = mskLabelCluster + "version"
	mskLabelClusterJmxExporterEnabled    = mskLabelCluster + "jmx_exporter_enabled"
	mskLabelClusterConfigurationARN      = mskLabelCluster + "configuration_arn"
	mskLabelClusterConfigurationRevision = mskLabelCluster + "configuration_revision"
	mskLabelClusterKafkaVersion          = mskLabelCluster + "kafka_version"
	mskLabelClusterTags                  = mskLabelCluster + "tag_"

	// Node labels.
	mskLabelNode             = mskLabel + "node_"
	mskLabelNodeType         = mskLabelNode + "type"
	mskLabelNodeARN          = mskLabelNode + "arn"
	mskLabelNodeAddedTime    = mskLabelNode + "added_time"
	mskLabelNodeInstanceType = mskLabelNode + "instance_type"
	mskLabelNodeAttachedENI  = mskLabelNode + "attached_eni"

	// Broker labels.
	mskLabelBroker                    = mskLabel + "broker_"
	mskLabelBrokerEndpointIndex       = mskLabelBroker + "endpoint_index"
	mskLabelBrokerID                  = mskLabelBroker + "id"
	mskLabelBrokerClientSubnet        = mskLabelBroker + "client_subnet"
	mskLabelBrokerClientVPCIP         = mskLabelBroker + "client_vpc_ip"
	mskLabelBrokerNodeExporterEnabled = mskLabelBroker + "node_exporter_enabled"

	// Controller labels.
	mskLabelController              = mskLabel + "controller_"
	mskLabelControllerEndpointIndex = mskLabelController + "endpoint_index"
)

// DefaultMSKSDConfig is the default MSK SD configuration.
var DefaultMSKSDConfig = MSKSDConfig{
	Port:               80,
	RefreshInterval:    model.Duration(60 * time.Second),
	RequestConcurrency: 10,
	HTTPClientConfig:   config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&MSKSDConfig{})
}

// MSKSDConfig is the configuration for MSK based service discovery.
type MSKSDConfig struct {
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
func (*MSKSDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &mskMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the MSK Config.
func (*MSKSDConfig) Name() string { return "msk" }

// NewDiscoverer returns a Discoverer for the MSK Config.
func (c *MSKSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewMSKDiscovery(c, opts)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the MSK Config.
func (c *MSKSDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultMSKSDConfig
	type plain MSKSDConfig
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

type mskClient interface {
	DescribeClusterV2(context.Context, *kafka.DescribeClusterV2Input, ...func(*kafka.Options)) (*kafka.DescribeClusterV2Output, error)
	ListClustersV2(context.Context, *kafka.ListClustersV2Input, ...func(*kafka.Options)) (*kafka.ListClustersV2Output, error)
	ListNodes(context.Context, *kafka.ListNodesInput, ...func(*kafka.Options)) (*kafka.ListNodesOutput, error)
}

// MSKDiscovery periodically performs MSK-SD requests. It implements
// the Discoverer interface.
type MSKDiscovery struct {
	*refresh.Discovery
	logger *slog.Logger
	cfg    *MSKSDConfig
	msk    mskClient
}

// NewMSKDiscovery returns a new MSKDiscovery which periodically refreshes its targets.
func NewMSKDiscovery(conf *MSKSDConfig, opts discovery.DiscovererOptions) (*MSKDiscovery, error) {
	m, ok := opts.Metrics.(*mskMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if opts.Logger == nil {
		opts.Logger = promslog.NewNopLogger()
	}
	d := &MSKDiscovery{
		logger: opts.Logger,
		cfg:    conf,
	}
	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "msk",
			Interval:            time.Duration(d.cfg.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *MSKDiscovery) initMskClient(ctx context.Context) error {
	if d.msk != nil {
		return nil
	}

	if d.cfg.Region == "" {
		return errors.New("region must be set for MSK service discovery")
	}

	// Build the HTTP client from the provided HTTPClientConfig.
	client, err := config.NewClientFromConfig(d.cfg.HTTPClientConfig, "msk_sd")
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

	d.msk = kafka.NewFromConfig(cfg, func(options *kafka.Options) {
		if d.cfg.Endpoint != "" {
			options.BaseEndpoint = &d.cfg.Endpoint
		}
		options.HTTPClient = client
	})

	// Test credentials by making a simple API call
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err = d.msk.ListClustersV2(testCtx, &kafka.ListClustersV2Input{})
	if err != nil {
		d.logger.Error("Failed to test MSK credentials", "error", err)
		return fmt.Errorf("MSK credential test failed: %w", err)
	}

	return nil
}

// describeClusters describes the clusters with the given ARNs and returns their details.
func (d *MSKDiscovery) describeClusters(ctx context.Context, clusterARNs []string) ([]types.Cluster, error) {
	var (
		clusters []types.Cluster
		mu       sync.Mutex
	)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for _, clusterARN := range clusterARNs {
		errg.Go(func() error {
			cluster, err := d.msk.DescribeClusterV2(ectx, &kafka.DescribeClusterV2Input{
				ClusterArn: aws.String(clusterARN),
			})
			if err != nil {
				return fmt.Errorf("could not describe cluster %v: %w", clusterARN, err)
			}
			mu.Lock()
			clusters = append(clusters, *cluster.ClusterInfo)
			mu.Unlock()
			return nil
		})
	}

	return clusters, errg.Wait()
}

// listClusters lists all MSK clusters in the configured region and returns their details.
func (d *MSKDiscovery) listClusters(ctx context.Context) ([]types.Cluster, error) {
	var (
		clusters  []types.Cluster
		nextToken *string
	)
	for {
		listClustersInput := kafka.ListClustersV2Input{
			ClusterTypeFilter: aws.String("PROVISIONED"),
			MaxResults:        aws.Int32(100),
			NextToken:         nextToken,
		}

		resp, err := d.msk.ListClustersV2(ctx, &listClustersInput)
		if err != nil {
			return nil, fmt.Errorf("could not list clusters: %w", err)
		}

		clusters = append(clusters, resp.ClusterInfoList...)
		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}

	return clusters, nil
}

// listNodes lists all nodes for the given clusters and returns a map of cluster ARN to its nodes.
func (d *MSKDiscovery) listNodes(ctx context.Context, clusters []types.Cluster) (map[string][]types.NodeInfo, error) {
	clusterNodeMap := make(map[string][]types.NodeInfo)
	mu := sync.Mutex{}
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for _, cluster := range clusters {
		clusterARN := aws.ToString(cluster.ClusterArn)
		errg.Go(func() error {
			var clusterNodes []types.NodeInfo
			var nextToken *string
			for {
				resp, err := d.msk.ListNodes(ectx, &kafka.ListNodesInput{
					ClusterArn: aws.String(clusterARN),
					MaxResults: aws.Int32(100),
					NextToken:  nextToken,
				})
				if err != nil {
					return fmt.Errorf("could not list nodes for cluster %v: %w", clusterARN, err)
				}

				clusterNodes = append(clusterNodes, resp.NodeInfoList...)
				if resp.NextToken == nil {
					break
				}
				nextToken = resp.NextToken
			}

			mu.Lock()
			clusterNodeMap[clusterARN] = clusterNodes
			mu.Unlock()
			return nil
		})
	}

	return clusterNodeMap, errg.Wait()
}

func (d *MSKDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	err := d.initMskClient(ctx)
	if err != nil {
		return nil, err
	}

	tg := &targetgroup.Group{
		Source: d.cfg.Region,
	}

	var clusters []types.Cluster
	if len(d.cfg.Clusters) > 0 {
		clusters, err = d.describeClusters(ctx, d.cfg.Clusters)
		if err != nil {
			return nil, err
		}
	} else {
		clusters, err = d.listClusters(ctx)
		if err != nil {
			return nil, err
		}
	}

	clusterNodeMap, err := d.listNodes(ctx, clusters)
	if err != nil {
		return nil, err
	}

	var (
		targetsMu sync.Mutex
		wg        sync.WaitGroup
	)
	for _, cluster := range clusters {
		wg.Add(1)

		go func(cluster types.Cluster, nodes []types.NodeInfo) {
			defer wg.Done()
			for _, node := range nodes {
				labels := model.LabelSet{
					mskLabelClusterName:                  model.LabelValue(aws.ToString(cluster.ClusterName)),
					mskLabelClusterARN:                   model.LabelValue(aws.ToString(cluster.ClusterArn)),
					mskLabelClusterState:                 model.LabelValue(string(cluster.State)),
					mskLabelClusterType:                  model.LabelValue(string(cluster.ClusterType)),
					mskLabelClusterVersion:               model.LabelValue(aws.ToString(cluster.CurrentVersion)),
					mskLabelNodeARN:                      model.LabelValue(aws.ToString(node.NodeARN)),
					mskLabelNodeAddedTime:                model.LabelValue(aws.ToString(node.AddedToClusterTime)),
					mskLabelNodeInstanceType:             model.LabelValue(aws.ToString(node.InstanceType)),
					mskLabelClusterJmxExporterEnabled:    model.LabelValue(strconv.FormatBool(*cluster.Provisioned.OpenMonitoring.Prometheus.JmxExporter.EnabledInBroker)),
					mskLabelClusterConfigurationARN:      model.LabelValue(aws.ToString(cluster.Provisioned.CurrentBrokerSoftwareInfo.ConfigurationArn)),
					mskLabelClusterConfigurationRevision: model.LabelValue(strconv.FormatInt(*cluster.Provisioned.CurrentBrokerSoftwareInfo.ConfigurationRevision, 10)),
					mskLabelClusterKafkaVersion:          model.LabelValue(aws.ToString(cluster.Provisioned.CurrentBrokerSoftwareInfo.KafkaVersion)),
				}

				for key, value := range cluster.Tags {
					labels[model.LabelName(mskLabelClusterTags+strutil.SanitizeLabelName(key))] = model.LabelValue(value)
				}

				switch nodeType(node) {
				case NodeTypeBroker:
					labels[mskLabelNodeType] = model.LabelValue(NodeTypeBroker)
					labels[mskLabelNodeAttachedENI] = model.LabelValue(aws.ToString(node.BrokerNodeInfo.AttachedENIId))
					labels[mskLabelBrokerID] = model.LabelValue(fmt.Sprintf("%.0f", aws.ToFloat64(node.BrokerNodeInfo.BrokerId)))
					labels[mskLabelBrokerClientSubnet] = model.LabelValue(aws.ToString(node.BrokerNodeInfo.ClientSubnet))
					labels[mskLabelBrokerClientVPCIP] = model.LabelValue(aws.ToString(node.BrokerNodeInfo.ClientVpcIpAddress))
					labels[mskLabelBrokerNodeExporterEnabled] = model.LabelValue(strconv.FormatBool(*cluster.Provisioned.OpenMonitoring.Prometheus.NodeExporter.EnabledInBroker))

					for idx, endpoint := range node.BrokerNodeInfo.Endpoints {
						endpointLabels := labels.Clone()
						endpointLabels[mskLabelBrokerEndpointIndex] = model.LabelValue(strconv.Itoa(idx))
						endpointLabels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(endpoint, strconv.Itoa(d.cfg.Port)))

						targetsMu.Lock()
						tg.Targets = append(tg.Targets, endpointLabels)
						targetsMu.Unlock()
					}

				case NodeTypeController:
					labels[mskLabelNodeType] = model.LabelValue(NodeTypeController)

					for idx, endpoint := range node.ControllerNodeInfo.Endpoints {
						endpointLabels := labels.Clone()
						endpointLabels[mskLabelControllerEndpointIndex] = model.LabelValue(strconv.Itoa(idx))
						endpointLabels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(endpoint, strconv.Itoa(d.cfg.Port)))

						targetsMu.Lock()
						tg.Targets = append(tg.Targets, endpointLabels)
						targetsMu.Unlock()
					}
				default:
					continue
				}
			}
		}(cluster, clusterNodeMap[aws.ToString(cluster.ClusterArn)])
	}
	wg.Wait()

	return []*targetgroup.Group{tg}, nil
}

func nodeType(node types.NodeInfo) NodeType {
	if node.BrokerNodeInfo != nil {
		return NodeTypeBroker
	} else if node.ControllerNodeInfo != nil {
		return NodeTypeController
	}
	return ""
}
