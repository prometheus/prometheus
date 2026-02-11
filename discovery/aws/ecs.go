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
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
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
	ecsLabel                     = model.MetaLabelPrefix + "ecs_"
	ecsLabelCluster              = ecsLabel + "cluster"
	ecsLabelClusterARN           = ecsLabel + "cluster_arn"
	ecsLabelService              = ecsLabel + "service"
	ecsLabelServiceARN           = ecsLabel + "service_arn"
	ecsLabelServiceStatus        = ecsLabel + "service_status"
	ecsLabelTaskGroup            = ecsLabel + "task_group"
	ecsLabelTaskARN              = ecsLabel + "task_arn"
	ecsLabelTaskDefinition       = ecsLabel + "task_definition"
	ecsLabelRegion               = ecsLabel + "region"
	ecsLabelAvailabilityZone     = ecsLabel + "availability_zone"
	ecsLabelSubnetID             = ecsLabel + "subnet_id"
	ecsLabelIPAddress            = ecsLabel + "ip_address"
	ecsLabelLaunchType           = ecsLabel + "launch_type"
	ecsLabelDesiredStatus        = ecsLabel + "desired_status"
	ecsLabelLastStatus           = ecsLabel + "last_status"
	ecsLabelHealthStatus         = ecsLabel + "health_status"
	ecsLabelPlatformFamily       = ecsLabel + "platform_family"
	ecsLabelPlatformVersion      = ecsLabel + "platform_version"
	ecsLabelTag                  = ecsLabel + "tag_"
	ecsLabelTagCluster           = ecsLabelTag + "cluster_"
	ecsLabelTagService           = ecsLabelTag + "service_"
	ecsLabelTagTask              = ecsLabelTag + "task_"
	ecsLabelTagEC2               = ecsLabelTag + "ec2_"
	ecsLabelNetworkMode          = ecsLabel + "network_mode"
	ecsLabelContainerInstanceARN = ecsLabel + "container_instance_arn"
	ecsLabelEC2InstanceID        = ecsLabel + "ec2_instance_id"
	ecsLabelEC2InstanceType      = ecsLabel + "ec2_instance_type"
	ecsLabelEC2InstancePrivateIP = ecsLabel + "ec2_instance_private_ip"
	ecsLabelEC2InstancePublicIP  = ecsLabel + "ec2_instance_public_ip"
	ecsLabelPublicIP             = ecsLabel + "public_ip"
)

// DefaultECSSDConfig is the default ECS SD configuration.
var DefaultECSSDConfig = ECSSDConfig{
	Port:               80,
	RefreshInterval:    model.Duration(60 * time.Second),
	RequestConcurrency: 20, // Aligned with AWS ECS API sustained rate limits (20 req/sec)
	HTTPClientConfig:   config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&ECSSDConfig{})
}

// ECSSDConfig is the configuration for ECS based service discovery.
type ECSSDConfig struct {
	Region          string         `yaml:"region"`
	Endpoint        string         `yaml:"endpoint"`
	AccessKey       string         `yaml:"access_key,omitempty"`
	SecretKey       config.Secret  `yaml:"secret_key,omitempty"`
	Profile         string         `yaml:"profile,omitempty"`
	RoleARN         string         `yaml:"role_arn,omitempty"`
	Clusters        []string       `yaml:"clusters,omitempty"`
	Port            int            `yaml:"port"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`

	// RequestConcurrency controls the maximum number of concurrent ECS API requests.
	// Default is 20, which aligns with AWS ECS sustained rate limits:
	// - Cluster read actions (DescribeClusters, ListClusters): 20 req/sec sustained
	// - Service read actions (DescribeServices, ListServices): 20 req/sec sustained
	// - Cluster resource read actions (DescribeTasks, ListTasks): 20 req/sec sustained
	// See: https://docs.aws.amazon.com/AmazonECS/latest/APIReference/request-throttling.html
	RequestConcurrency int `yaml:"request_concurrency,omitempty"`

	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*ECSSDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &ecsMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the ECS Config.
func (*ECSSDConfig) Name() string { return "ecs" }

// NewDiscoverer returns a Discoverer for the EC2 Config.
func (c *ECSSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewECSDiscovery(c, opts)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the ECS Config.
func (c *ECSSDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultECSSDConfig
	type plain ECSSDConfig
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

type ecsClient interface {
	ListClusters(context.Context, *ecs.ListClustersInput, ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
	DescribeClusters(context.Context, *ecs.DescribeClustersInput, ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error)
	ListServices(context.Context, *ecs.ListServicesInput, ...func(*ecs.Options)) (*ecs.ListServicesOutput, error)
	DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	ListTasks(context.Context, *ecs.ListTasksInput, ...func(*ecs.Options)) (*ecs.ListTasksOutput, error)
	DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
	DescribeContainerInstances(context.Context, *ecs.DescribeContainerInstancesInput, ...func(*ecs.Options)) (*ecs.DescribeContainerInstancesOutput, error)
}

type ecsEC2Client interface {
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	DescribeNetworkInterfaces(context.Context, *ec2.DescribeNetworkInterfacesInput, ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
}

// ECSDiscovery periodically performs ECS-SD requests. It implements
// the Discoverer interface.
type ECSDiscovery struct {
	*refresh.Discovery
	logger *slog.Logger
	cfg    *ECSSDConfig
	ecs    ecsClient
	ec2    ecsEC2Client
}

// NewECSDiscovery returns a new ECSDiscovery which periodically refreshes its targets.
func NewECSDiscovery(conf *ECSSDConfig, opts discovery.DiscovererOptions) (*ECSDiscovery, error) {
	m, ok := opts.Metrics.(*ecsMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if opts.Logger == nil {
		opts.Logger = promslog.NewNopLogger()
	}
	d := &ECSDiscovery{
		logger: opts.Logger,
		cfg:    conf,
	}
	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "ecs",
			Interval:            time.Duration(d.cfg.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *ECSDiscovery) initEcsClient(ctx context.Context) error {
	if d.ecs != nil && d.ec2 != nil {
		return nil
	}

	if d.cfg.Region == "" {
		return errors.New("region must be set for ECS service discovery")
	}

	// Build the HTTP client from the provided HTTPClientConfig.
	client, err := config.NewClientFromConfig(d.cfg.HTTPClientConfig, "ecs_sd")
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

	d.ecs = ecs.NewFromConfig(cfg, func(options *ecs.Options) {
		if d.cfg.Endpoint != "" {
			options.BaseEndpoint = &d.cfg.Endpoint
		}
		options.HTTPClient = client
	})

	d.ec2 = ec2.NewFromConfig(cfg, func(options *ec2.Options) {
		options.HTTPClient = client
	})

	// Test credentials by making a simple API call
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err = d.ecs.DescribeClusters(testCtx, &ecs.DescribeClustersInput{})
	if err != nil {
		d.logger.Error("Failed to test ECS credentials", "error", err)
		return fmt.Errorf("ECS credential test failed: %w", err)
	}

	return nil
}

// listClusterARNs returns a slice of cluster arns.
// This method does not use concurrency as it's a simple paginated call.
func (d *ECSDiscovery) listClusterARNs(ctx context.Context) ([]string, error) {
	var (
		clusterARNs []string
		nextToken   *string
	)
	for {
		resp, err := d.ecs.ListClusters(ctx, &ecs.ListClustersInput{
			NextToken:  nextToken,
			MaxResults: aws.Int32(100),
		})
		if err != nil {
			return nil, fmt.Errorf("could not list clusters: %w", err)
		}

		clusterARNs = append(clusterARNs, resp.ClusterArns...)

		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}

	return clusterARNs, nil
}

// describeClusters returns a map of cluster ARN to a slice of clusters.
// Uses concurrent requests limited by RequestConcurrency to respect AWS API throttling.
// Clusters are described in batches of 100 to respect AWS API limits (DescribeClusters allows up to 100 clusters per call).
func (d *ECSDiscovery) describeClusters(ctx context.Context, clusters []string) (map[string]types.Cluster, error) {
	mu := sync.Mutex{}
	clusterMap := make(map[string]types.Cluster)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for batch := range slices.Chunk(clusters, 100) {
		errg.Go(func() error {
			resp, err := d.ecs.DescribeClusters(ectx, &ecs.DescribeClustersInput{
				Clusters: batch,
				Include:  []types.ClusterField{"TAGS"},
			})
			if err != nil {
				d.logger.Error("Failed to describe clusters", "clusters", batch, "error", err)
				return fmt.Errorf("could not describe clusters %v: %w", batch, err)
			}

			for _, cluster := range resp.Clusters {
				if cluster.ClusterArn != nil {
					mu.Lock()
					clusterMap[*cluster.ClusterArn] = cluster
					mu.Unlock()
				}
			}
			return nil
		})
	}

	return clusterMap, errg.Wait()
}

// listServiceARNs returns a map of cluster ARN to a slice of service ARNs.
// Uses concurrent requests limited by RequestConcurrency to respect AWS API throttling.
// Services are listed in batches of 100 to respect AWS API limits (ListServices allows up to 100 services per call).
func (d *ECSDiscovery) listServiceARNs(ctx context.Context, clusters []string) (map[string][]string, error) {
	mu := sync.Mutex{}
	services := make(map[string][]string)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for _, clusterARN := range clusters {
		errg.Go(func() error {
			var nextToken *string
			var serviceARNs []string
			for {
				resp, err := d.ecs.ListServices(ectx, &ecs.ListServicesInput{
					Cluster:    aws.String(clusterARN),
					NextToken:  nextToken,
					MaxResults: aws.Int32(100),
				})
				if err != nil {
					return fmt.Errorf("could not list services for cluster %q: %w", clusterARN, err)
				}

				serviceARNs = append(serviceARNs, resp.ServiceArns...)

				if resp.NextToken == nil {
					break
				}
				nextToken = resp.NextToken
			}

			mu.Lock()
			services[clusterARN] = serviceARNs
			mu.Unlock()
			return nil
		})
	}

	return services, errg.Wait()
}

// describeServices returns a map of service name to service.
// Uses concurrent requests limited by RequestConcurrency to respect AWS API throttling.
// Services are described in batches of 10 to respect AWS API limits (DescribeServices allows up to 10 services per call).
func (d *ECSDiscovery) describeServices(ctx context.Context, clusterARN string, serviceARNS []string) (map[string]types.Service, error) {
	mu := sync.Mutex{}
	services := make(map[string]types.Service)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for batch := range slices.Chunk(serviceARNS, 10) {
		errg.Go(func() error {
			resp, err := d.ecs.DescribeServices(ectx, &ecs.DescribeServicesInput{
				Cluster:  aws.String(clusterARN),
				Services: batch,
				Include:  []types.ServiceField{"TAGS"},
			})
			if err != nil {
				d.logger.Error("Failed to describe services", "cluster", clusterARN, "batch", batch, "error", err)
				return fmt.Errorf("could not describe services for cluster %q: batch %v: %w", clusterARN, batch, err)
			}

			for _, service := range resp.Services {
				if service.ServiceArn != nil {
					mu.Lock()
					services[*service.ServiceName] = service
					mu.Unlock()
				}
			}
			return nil
		})
	}

	return services, errg.Wait()
}

// listTaskARNs returns a map of clustersARN to a slice of task ARNs.
// Uses concurrent requests limited by RequestConcurrency to respect AWS API throttling.
// Tasks are listed in batches of 100 to respect AWS API limits (ListTasks allows up to 100 tasks per call).
// This method also uses pagination to handle cases where there are more than 100 tasks in a cluster.
func (d *ECSDiscovery) listTaskARNs(ctx context.Context, clusterARNs []string) (map[string][]string, error) {
	mu := sync.Mutex{}
	tasks := make(map[string][]string)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for _, clusterARN := range clusterARNs {
		errg.Go(func() error {
			var (
				nextToken *string
				taskARNs  []string
			)
			for {
				resp, err := d.ecs.ListTasks(ectx, &ecs.ListTasksInput{
					Cluster:    aws.String(clusterARN),
					NextToken:  nextToken,
					MaxResults: aws.Int32(100),
				})
				if err != nil {
					return fmt.Errorf("could not list tasks for cluster %q: %w", clusterARN, err)
				}

				taskARNs = append(taskARNs, resp.TaskArns...)

				if resp.NextToken == nil {
					break
				}
				nextToken = resp.NextToken
			}

			mu.Lock()
			tasks[clusterARN] = taskARNs
			mu.Unlock()
			return nil
		})
	}

	return tasks, errg.Wait()
}

// describeTasks returns a slice of tasks.
// Uses concurrent requests limited by RequestConcurrency to respect AWS API throttling.
// Tasks are described in batches of 100 to respect AWS API limits (DescribeTasks allows up to 100 tasks per call).
func (d *ECSDiscovery) describeTasks(ctx context.Context, clusterARN string, taskARNs []string) ([]types.Task, error) {
	mu := sync.Mutex{}
	var tasks []types.Task
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for batch := range slices.Chunk(taskARNs, 100) {
		errg.Go(func() error {
			resp, err := d.ecs.DescribeTasks(ectx, &ecs.DescribeTasksInput{
				Cluster: aws.String(clusterARN),
				Tasks:   batch,
				Include: []types.TaskField{"TAGS"},
			})
			if err != nil {
				d.logger.Error("Failed to describe tasks", "cluster", clusterARN, "batch", batch, "error", err)
				return fmt.Errorf("could not describe tasks in cluster %q: batch %v: %w", clusterARN, batch, err)
			}

			mu.Lock()
			tasks = append(tasks, resp.Tasks...)
			mu.Unlock()
			return nil
		})
	}

	return tasks, errg.Wait()
}

// describeContainerInstances returns a map of container instance ARN to EC2 instance ID
// Uses concurrent requests limited by RequestConcurrency to respect AWS API throttling.
// Container instances are described in batches of 100 to respect AWS API limits (DescribeContainerInstances allows up to 100 container instances per call).
func (d *ECSDiscovery) describeContainerInstances(ctx context.Context, clusterARN string, tasks []types.Task) (map[string]string, error) {
	containerInstanceARNs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task.ContainerInstanceArn != nil {
			containerInstanceARNs = append(containerInstanceARNs, *task.ContainerInstanceArn)
		}
	}

	if len(containerInstanceARNs) == 0 {
		return make(map[string]string), nil
	}

	mu := sync.Mutex{}
	containerInstToEC2 := make(map[string]string)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for batch := range slices.Chunk(containerInstanceARNs, 100) {
		errg.Go(func() error {
			resp, err := d.ecs.DescribeContainerInstances(ectx, &ecs.DescribeContainerInstancesInput{
				Cluster:            aws.String(clusterARN),
				ContainerInstances: batch,
			})
			if err != nil {
				return fmt.Errorf("could not describe container instances: %w", err)
			}

			for _, ci := range resp.ContainerInstances {
				if ci.ContainerInstanceArn != nil && ci.Ec2InstanceId != nil {
					mu.Lock()
					containerInstToEC2[*ci.ContainerInstanceArn] = *ci.Ec2InstanceId
					mu.Unlock()
				}
			}
			return nil
		})
	}

	return containerInstToEC2, errg.Wait()
}

// ec2InstanceInfo holds information retrieved from EC2 DescribeInstances.
type ec2InstanceInfo struct {
	privateIP    string
	publicIP     string
	subnetID     string
	instanceType string
	tags         map[string]string
}

// describeEC2Instances returns a map of EC2 instance ID to instance information.
// Uses concurrent requests limited by RequestConcurrency to respect AWS API throttling.
// This method does not use concurrency as it's a simple paginated call.
func (d *ECSDiscovery) describeEC2Instances(ctx context.Context, instanceIDs []string) (map[string]ec2InstanceInfo, error) {
	if len(instanceIDs) == 0 {
		return make(map[string]ec2InstanceInfo), nil
	}

	instanceInfo := make(map[string]ec2InstanceInfo)
	var nextToken *string

	for {
		resp, err := d.ec2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: instanceIDs,
			NextToken:   nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("could not describe EC2 instances: %w", err)
		}

		for _, reservation := range resp.Reservations {
			for _, instance := range reservation.Instances {
				if instance.InstanceId != nil && instance.PrivateIpAddress != nil {
					info := ec2InstanceInfo{
						privateIP: *instance.PrivateIpAddress,
						tags:      make(map[string]string),
					}
					if instance.PublicIpAddress != nil {
						info.publicIP = *instance.PublicIpAddress
					}
					if instance.SubnetId != nil {
						info.subnetID = *instance.SubnetId
					}
					if instance.InstanceType != "" {
						info.instanceType = string(instance.InstanceType)
					}
					// Collect EC2 instance tags
					for _, tag := range instance.Tags {
						if tag.Key != nil && tag.Value != nil {
							info.tags[*tag.Key] = *tag.Value
						}
					}
					instanceInfo[*instance.InstanceId] = info
				}
			}
		}

		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}

	return instanceInfo, nil
}

// describeNetworkInterfaces returns a map of ENI ID to public IP address.
// This is needed to get the public IP for tasks using awsvpc network mode, as the ENI is what gets the public IP, not the EC2 instance.
// This method does not use concurrency as it's a simple paginated call.
func (d *ECSDiscovery) describeNetworkInterfaces(ctx context.Context, tasks []types.Task) (map[string]string, error) {
	eniIDs := make([]string, 0, len(tasks))

	for _, task := range tasks {
		for _, attachment := range task.Attachments {
			if attachment.Type != nil && *attachment.Type == "ElasticNetworkInterface" {
				for _, detail := range attachment.Details {
					if detail.Name != nil && *detail.Name == "networkInterfaceId" && detail.Value != nil {
						eniIDs = append(eniIDs, *detail.Value)
						break
					}
				}
				break
			}
		}
	}

	if len(eniIDs) == 0 {
		return make(map[string]string), nil
	}

	eniToPublicIP := make(map[string]string)
	var nextToken *string

	for {
		resp, err := d.ec2.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
			NetworkInterfaceIds: eniIDs,
			NextToken:           nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("could not describe network interfaces: %w", err)
		}

		for _, eni := range resp.NetworkInterfaces {
			if eni.NetworkInterfaceId != nil && eni.Association != nil && eni.Association.PublicIp != nil {
				eniToPublicIP[*eni.NetworkInterfaceId] = *eni.Association.PublicIp
			}
		}

		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}

	return eniToPublicIP, nil
}

func (d *ECSDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	err := d.initEcsClient(ctx)
	if err != nil {
		return nil, err
	}

	var clusters []string
	if len(d.cfg.Clusters) == 0 {
		clusters, err = d.listClusterARNs(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		clusters = d.cfg.Clusters
	}

	if len(clusters) == 0 {
		return []*targetgroup.Group{
			{
				Source: d.cfg.Region,
			},
		}, nil
	}

	tg := &targetgroup.Group{
		Source: d.cfg.Region,
	}

	// Fetch cluster details, service ARNs, and task ARNs in parallel
	var (
		clusterMap map[string]types.Cluster
		serviceMap map[string][]string
		taskMap    map[string][]string
	)

	clusterErrg, clusterCtx := errgroup.WithContext(ctx)
	clusterErrg.Go(func() error {
		var err error
		clusterMap, err = d.describeClusters(clusterCtx, clusters)
		return err
	})
	clusterErrg.Go(func() error {
		var err error
		serviceMap, err = d.listServiceARNs(clusterCtx, clusters)
		return err
	})
	clusterErrg.Go(func() error {
		var err error
		taskMap, err = d.listTaskARNs(clusterCtx, clusters)
		return err
	})

	if err := clusterErrg.Wait(); err != nil {
		return nil, err
	}

	// Use goroutines to process clusters in parallel
	var (
		clusterWg      sync.WaitGroup
		clusterMu      sync.Mutex
		clusterTargets []model.LabelSet
	)

	for clusterARN, taskARNs := range taskMap {
		if len(taskARNs) == 0 {
			continue
		}

		clusterWg.Add(1)

		go func(cluster types.Cluster, serviceARNs, taskARNs []string) {
			defer clusterWg.Done()

			// Fetch services and tasks in parallel (they're independent)
			var (
				services map[string]types.Service
				tasks    []types.Task
			)

			resourceErrg, resourceCtx := errgroup.WithContext(ctx)
			resourceErrg.Go(func() error {
				var err error
				services, err = d.describeServices(resourceCtx, *cluster.ClusterArn, serviceARNs)
				if err != nil {
					d.logger.Error("Failed to describe services for cluster", "cluster", *cluster.ClusterArn, "error", err)
				}
				return err
			})
			resourceErrg.Go(func() error {
				var err error
				tasks, err = d.describeTasks(resourceCtx, *cluster.ClusterArn, taskARNs)
				if err != nil {
					d.logger.Error("Failed to describe tasks for cluster", "cluster", *cluster.ClusterArn, "error", err)
				}
				return err
			})

			if err := resourceErrg.Wait(); err != nil {
				return
			}

			// Fetch container instances and network interfaces in parallel (both depend on tasks)
			var (
				containerInstances map[string]string
				eniToPublicIP      map[string]string
			)

			instanceErrg, instanceCtx := errgroup.WithContext(ctx)
			instanceErrg.Go(func() error {
				var err error
				containerInstances, err = d.describeContainerInstances(instanceCtx, *cluster.ClusterArn, tasks)
				if err != nil {
					d.logger.Error("Failed to describe container instances for cluster", "cluster", *cluster.ClusterArn, "error", err)
				}
				return err
			})
			instanceErrg.Go(func() error {
				var err error
				eniToPublicIP, err = d.describeNetworkInterfaces(instanceCtx, tasks)
				if err != nil {
					d.logger.Error("Failed to describe network interfaces for cluster", "cluster", *cluster.ClusterArn, "error", err)
				}
				return err
			})

			if err := instanceErrg.Wait(); err != nil {
				return
			}

			ec2Instances := make(map[string]ec2InstanceInfo)
			if len(containerInstances) > 0 {
				// Deduplicate EC2 instance IDs (multiple tasks can share the same instance)
				ec2InstanceIDSet := make(map[string]struct{})
				for _, ec2ID := range containerInstances {
					ec2InstanceIDSet[ec2ID] = struct{}{}
				}
				ec2InstanceIDs := make([]string, 0, len(ec2InstanceIDSet))
				for ec2ID := range ec2InstanceIDSet {
					ec2InstanceIDs = append(ec2InstanceIDs, ec2ID)
				}
				ec2Instances, err = d.describeEC2Instances(ctx, ec2InstanceIDs)
				if err != nil {
					d.logger.Error("Failed to describe EC2 instances for cluster", "cluster", *cluster.ClusterArn, "error", err)
					return
				}
			}

			var (
				taskWg      sync.WaitGroup
				taskMu      sync.Mutex
				taskTargets []model.LabelSet
			)

			for _, task := range tasks {
				taskWg.Add(1)

				go func(cluster types.Cluster, services map[string]types.Service, task types.Task, containerInstances map[string]string, ec2Instances map[string]ec2InstanceInfo, eniToPublicIP map[string]string) {
					defer taskWg.Done()

					var (
						ipAddress, subnetID, publicIP                                             string
						networkMode                                                               string
						ec2InstanceID, ec2InstanceType, ec2InstancePrivateIP, ec2InstancePublicIP string
					)

					// Try to get IP from ENI attachment (awsvpc mode)
					var eniAttachment *types.Attachment
					for _, attachment := range task.Attachments {
						if attachment.Type != nil && *attachment.Type == "ElasticNetworkInterface" {
							eniAttachment = &attachment
							break
						}
					}

					if eniAttachment != nil {
						// awsvpc networking mode - get IP from ENI
						networkMode = "awsvpc"
						var eniID string
						for _, detail := range eniAttachment.Details {
							switch *detail.Name {
							case "privateIPv4Address":
								ipAddress = *detail.Value
							case "subnetId":
								subnetID = *detail.Value
							case "networkInterfaceId":
								eniID = *detail.Value
							}
						}
						// Get public IP from ENI if available
						if eniID != "" {
							if pub, ok := eniToPublicIP[eniID]; ok {
								publicIP = pub
							}
						}
					} else if task.ContainerInstanceArn != nil {
						// bridge/host networking mode - need to get EC2 instance IP and subnet
						networkMode = "bridge"
						var ok bool
						ec2InstanceID, ok = containerInstances[*task.ContainerInstanceArn]
						if ok {
							info, ok := ec2Instances[ec2InstanceID]
							if ok {
								ipAddress = info.privateIP
								publicIP = info.publicIP
								subnetID = info.subnetID
								ec2InstanceType = info.instanceType
								ec2InstancePrivateIP = info.privateIP
								ec2InstancePublicIP = info.publicIP
							} else {
								d.logger.Debug("EC2 instance info not found", "instance", ec2InstanceID, "task", *task.TaskArn)
							}
						} else {
							d.logger.Debug("Container instance not found in map", "arn", *task.ContainerInstanceArn, "task", *task.TaskArn)
						}
					}

					// Get EC2 instance metadata for awsvpc tasks running on EC2
					// We want the instance type and the host IPs for advanced use cases
					if networkMode == "awsvpc" && task.ContainerInstanceArn != nil {
						var ok bool
						ec2InstanceID, ok = containerInstances[*task.ContainerInstanceArn]
						if ok {
							info, ok := ec2Instances[ec2InstanceID]
							if ok {
								ec2InstanceType = info.instanceType
								ec2InstancePrivateIP = info.privateIP
								ec2InstancePublicIP = info.publicIP
							}
						}
					}

					if ipAddress == "" {
						return
					}

					labels := model.LabelSet{
						ecsLabelClusterARN:       model.LabelValue(*cluster.ClusterArn),
						ecsLabelCluster:          model.LabelValue(*cluster.ClusterName),
						ecsLabelTaskGroup:        model.LabelValue(*task.Group),
						ecsLabelTaskARN:          model.LabelValue(*task.TaskArn),
						ecsLabelTaskDefinition:   model.LabelValue(*task.TaskDefinitionArn),
						ecsLabelIPAddress:        model.LabelValue(ipAddress),
						ecsLabelRegion:           model.LabelValue(d.cfg.Region),
						ecsLabelLaunchType:       model.LabelValue(task.LaunchType),
						ecsLabelAvailabilityZone: model.LabelValue(*task.AvailabilityZone),
						ecsLabelDesiredStatus:    model.LabelValue(*task.DesiredStatus),
						ecsLabelLastStatus:       model.LabelValue(*task.LastStatus),
						ecsLabelHealthStatus:     model.LabelValue(task.HealthStatus),
						ecsLabelNetworkMode:      model.LabelValue(networkMode),
					}

					// Add subnet ID when available (awsvpc mode from ENI, bridge/host from EC2 instance)
					if subnetID != "" {
						labels[ecsLabelSubnetID] = model.LabelValue(subnetID)
					}

					// Add container instance and EC2 instance info for EC2 launch type
					if task.ContainerInstanceArn != nil {
						labels[ecsLabelContainerInstanceARN] = model.LabelValue(*task.ContainerInstanceArn)
					}
					if ec2InstanceID != "" {
						labels[ecsLabelEC2InstanceID] = model.LabelValue(ec2InstanceID)
					}
					if ec2InstanceType != "" {
						labels[ecsLabelEC2InstanceType] = model.LabelValue(ec2InstanceType)
					}
					if ec2InstancePrivateIP != "" {
						labels[ecsLabelEC2InstancePrivateIP] = model.LabelValue(ec2InstancePrivateIP)
					}
					if ec2InstancePublicIP != "" {
						labels[ecsLabelEC2InstancePublicIP] = model.LabelValue(ec2InstancePublicIP)
					}
					if publicIP != "" {
						labels[ecsLabelPublicIP] = model.LabelValue(publicIP)
					}

					if task.PlatformFamily != nil {
						labels[ecsLabelPlatformFamily] = model.LabelValue(*task.PlatformFamily)
					}
					if task.PlatformVersion != nil {
						labels[ecsLabelPlatformVersion] = model.LabelValue(*task.PlatformVersion)
					}

					labels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(ipAddress, strconv.Itoa(d.cfg.Port)))

					// Add cluster tags
					for _, clusterTag := range cluster.Tags {
						if clusterTag.Key != nil && clusterTag.Value != nil {
							labels[model.LabelName(ecsLabelTagCluster+strutil.SanitizeLabelName(*clusterTag.Key))] = model.LabelValue(*clusterTag.Value)
						}
					}

					// If this is not a standalone task, add service information and tags
					if !isStandaloneTask(task) {
						service, ok := services[getServiceNameFromTaskGroup(task)]
						if !ok {
							d.logger.Debug("Service not found for task", "task", *task.TaskArn, "service", getServiceNameFromTaskGroup(task))
						}
						if service.ServiceName != nil {
							labels[ecsLabelService] = model.LabelValue(*service.ServiceName)
						}
						if service.ServiceArn != nil {
							labels[ecsLabelServiceARN] = model.LabelValue(*service.ServiceArn)
						}
						if service.Status != nil {
							labels[ecsLabelServiceStatus] = model.LabelValue(*service.Status)
						}

						// Add service tags
						for _, serviceTag := range service.Tags {
							if serviceTag.Key != nil && serviceTag.Value != nil {
								labels[model.LabelName(ecsLabelTagService+strutil.SanitizeLabelName(*serviceTag.Key))] = model.LabelValue(*serviceTag.Value)
							}
						}
					}

					// Add task tags
					for _, taskTag := range task.Tags {
						if taskTag.Key != nil && taskTag.Value != nil {
							labels[model.LabelName(ecsLabelTagTask+strutil.SanitizeLabelName(*taskTag.Key))] = model.LabelValue(*taskTag.Value)
						}
					}

					// Add EC2 instance tags (if running on EC2)
					if ec2InstanceID != "" {
						if info, ok := ec2Instances[ec2InstanceID]; ok {
							for tagKey, tagValue := range info.tags {
								labels[model.LabelName(ecsLabelTagEC2+strutil.SanitizeLabelName(tagKey))] = model.LabelValue(tagValue)
							}
						}
					}

					taskMu.Lock()
					taskTargets = append(taskTargets, labels)
					taskMu.Unlock()
				}(cluster, services, task, containerInstances, ec2Instances, eniToPublicIP)
			}

			taskWg.Wait()

			// Add this cluster's task targets to the overall collection
			clusterMu.Lock()
			clusterTargets = append(clusterTargets, taskTargets...)
			clusterMu.Unlock()
		}(clusterMap[clusterARN], serviceMap[clusterARN], taskARNs)
	}

	clusterWg.Wait()

	// Set all targets to the target group
	tg.Targets = clusterTargets

	return []*targetgroup.Group{tg}, nil
}

func isStandaloneTask(task types.Task) bool {
	// A standalone task will have a group of "family:task-def-name"
	return task.Group != nil && strings.HasPrefix(*task.Group, "family:")
}

func getServiceNameFromTaskGroup(task types.Task) string {
	return strings.Split(*task.Group, ":")[1]
}
