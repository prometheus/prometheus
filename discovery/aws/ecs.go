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
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
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
	ecsLabel                 = model.MetaLabelPrefix + "ecs_"
	ecsLabelCluster          = ecsLabel + "cluster"
	ecsLabelClusterARN       = ecsLabel + "cluster_arn"
	ecsLabelService          = ecsLabel + "service"
	ecsLabelServiceARN       = ecsLabel + "service_arn"
	ecsLabelServiceStatus    = ecsLabel + "service_status"
	ecsLabelTaskGroup        = ecsLabel + "task_group"
	ecsLabelTaskARN          = ecsLabel + "task_arn"
	ecsLabelTaskDefinition   = ecsLabel + "task_definition"
	ecsLabelRegion           = ecsLabel + "region"
	ecsLabelAvailabilityZone = ecsLabel + "availability_zone"
	ecsLabelAZID             = ecsLabel + "availability_zone_id"
	ecsLabelSubnetID         = ecsLabel + "subnet_id"
	ecsLabelIPAddress        = ecsLabel + "ip_address"
	ecsLabelLaunchType       = ecsLabel + "launch_type"
	ecsLabelDesiredStatus    = ecsLabel + "desired_status"
	ecsLabelLastStatus       = ecsLabel + "last_status"
	ecsLabelHealthStatus     = ecsLabel + "health_status"
	ecsLabelPlatformFamily   = ecsLabel + "platform_family"
	ecsLabelPlatformVersion  = ecsLabel + "platform_version"
	ecsLabelTag              = ecsLabel + "tag_"
	ecsLabelTagCluster       = ecsLabelTag + "cluster_"
	ecsLabelTagService       = ecsLabelTag + "service_"
	ecsLabelTagTask          = ecsLabelTag + "task_"
	ecsLabelSeparator        = ","
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

	if c.Region == "" {
		cfg, err := awsConfig.LoadDefaultConfig(context.TODO())
		if err != nil {
			return err
		}
		client := imds.NewFromConfig(cfg)
		result, err := client.GetRegion(context.Background(), &imds.GetRegionInput{})
		if err != nil {
			return fmt.Errorf("ECS SD configuration requires a region. Tried to fetch it from the instance metadata: %w", err)
		}
		c.Region = result.Region
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
}

// ECSDiscovery periodically performs ECS-SD requests. It implements
// the Discoverer interface.
type ECSDiscovery struct {
	*refresh.Discovery
	logger *slog.Logger
	cfg    *ECSSDConfig
	ecs    ecsClient
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
	if d.ecs != nil {
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
// AWS ECS Cluster read actions have burst=50, sustained=20 req/sec limits.
func (d *ECSDiscovery) listClusterARNs(ctx context.Context) ([]string, error) {
	var (
		clusterARNs []string
		nextToken   *string
	)
	for {
		resp, err := d.ecs.ListClusters(ctx, &ecs.ListClustersInput{
			NextToken: nextToken,
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
// This method processes clusters in batches without concurrency as it's typically
// a single call handling up to 100 clusters. AWS ECS Cluster read actions have
// burst=50, sustained=20 req/sec limits.
func (d *ECSDiscovery) describeClusters(ctx context.Context, clusters []string) (map[string]types.Cluster, error) {
	clusterMap := make(map[string]types.Cluster)

	// AWS DescribeClusters can handle up to 100 clusters per call
	batchSize := 100
	for _, batch := range batchSlice(clusters, batchSize) {
		resp, err := d.ecs.DescribeClusters(ctx, &ecs.DescribeClustersInput{
			Clusters: batch,
			Include:  []types.ClusterField{"TAGS"},
		})
		if err != nil {
			d.logger.Error("Failed to describe clusters", "clusters", batch, "error", err)
			return nil, fmt.Errorf("could not describe clusters %v: %w", batch, err)
		}

		for _, c := range resp.Clusters {
			if c.ClusterArn != nil {
				clusterMap[*c.ClusterArn] = c
			}
		}
	}

	return clusterMap, nil
}

// listServiceARNs returns a map of cluster ARN to a slice of service ARNs.
// Uses concurrent requests limited by RequestConcurrency to respect AWS API throttling.
// AWS ECS Service read actions have burst=100, sustained=20 req/sec limits.
func (d *ECSDiscovery) listServiceARNs(ctx context.Context, clusters []string) (map[string][]string, error) {
	serviceARNsMu := sync.Mutex{}
	serviceARNs := make(map[string][]string)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for _, clusterARN := range clusters {
		errg.Go(func() error {
			var nextToken *string
			var clusterServiceARNs []string
			for {
				resp, err := d.ecs.ListServices(ectx, &ecs.ListServicesInput{
					Cluster:   aws.String(clusterARN),
					NextToken: nextToken,
				})
				if err != nil {
					return fmt.Errorf("could not list services for cluster %q: %w", clusterARN, err)
				}

				clusterServiceARNs = append(clusterServiceARNs, resp.ServiceArns...)

				if resp.NextToken == nil {
					break
				}
				nextToken = resp.NextToken
			}

			serviceARNsMu.Lock()
			serviceARNs[clusterARN] = clusterServiceARNs
			serviceARNsMu.Unlock()
			return nil
		})
	}

	return serviceARNs, errg.Wait()
}

// describeServices returns a map of cluster ARN to services.
// Uses concurrent requests with batching (10 services per request) to respect AWS API limits.
// AWS ECS Service read actions have burst=100, sustained=20 req/sec limits.
func (d *ECSDiscovery) describeServices(ctx context.Context, clusterServiceARNsMap map[string][]string) (map[string][]types.Service, error) {
	batchSize := 10 // AWS DescribeServices API limit is 10 services per request
	serviceMu := sync.Mutex{}
	services := make(map[string][]types.Service)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for clusterARN, serviceARNs := range clusterServiceARNsMap {
		for _, batch := range batchSlice(serviceARNs, batchSize) {
			errg.Go(func() error {
				resp, err := d.ecs.DescribeServices(ectx, &ecs.DescribeServicesInput{
					Services: batch,
					Cluster:  aws.String(clusterARN),
					Include:  []types.ServiceField{"TAGS"},
				})
				if err != nil {
					d.logger.Error("Failed to describe services", "cluster", clusterARN, "batch", batch, "error", err)
					return fmt.Errorf("could not describe services for cluster %q: %w", clusterARN, err)
				}

				serviceMu.Lock()
				services[clusterARN] = append(services[clusterARN], resp.Services...)
				serviceMu.Unlock()

				return nil
			})
		}
	}

	return services, errg.Wait()
}

// listTaskARNs returns a map of service ARN to a slice of task ARNs.
// Uses concurrent requests limited by RequestConcurrency to respect AWS API throttling.
// AWS ECS Cluster resource read actions have burst=100, sustained=20 req/sec limits.
func (d *ECSDiscovery) listTaskARNs(ctx context.Context, services []types.Service) (map[string][]string, error) {
	taskARNsMu := sync.Mutex{}
	taskARNs := make(map[string][]string)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for _, service := range services {
		errg.Go(func() error {
			serviceArn := aws.ToString(service.ServiceArn)

			var nextToken *string
			var serviceTaskARNs []string
			for {
				resp, err := d.ecs.ListTasks(ectx, &ecs.ListTasksInput{
					Cluster:     aws.String(*service.ClusterArn),
					ServiceName: aws.String(*service.ServiceName),
					NextToken:   nextToken,
				})
				if err != nil {
					return fmt.Errorf("could not list tasks for service %q: %w", serviceArn, err)
				}

				serviceTaskARNs = append(serviceTaskARNs, resp.TaskArns...)

				if resp.NextToken == nil {
					break
				}
				nextToken = resp.NextToken
			}

			taskARNsMu.Lock()
			taskARNs[serviceArn] = serviceTaskARNs
			taskARNsMu.Unlock()
			return nil
		})
	}

	return taskARNs, errg.Wait()
}

// describeTasks returns a map of task arn to a slice task.
// Uses concurrent requests with batching (100 tasks per request) to respect AWS API limits.
// AWS ECS Cluster resource read actions have burst=100, sustained=20 req/sec limits.
func (d *ECSDiscovery) describeTasks(ctx context.Context, clusterARN string, taskARNsMap map[string][]string) (map[string][]types.Task, error) {
	batchSize := 100 // AWS DescribeTasks API limit is 100 tasks per request
	taskMu := sync.Mutex{}
	tasks := make(map[string][]types.Task)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for serviceARN, taskARNs := range taskARNsMap {
		for _, batch := range batchSlice(taskARNs, batchSize) {
			errg.Go(func() error {
				resp, err := d.ecs.DescribeTasks(ectx, &ecs.DescribeTasksInput{
					Cluster: aws.String(clusterARN),
					Tasks:   batch,
					Include: []types.TaskField{"TAGS"},
				})
				if err != nil {
					d.logger.Error("Failed to describe tasks", "service", serviceARN, "cluster", clusterARN, "batch", batch, "error", err)
					return fmt.Errorf("could not describe tasks for service %q in cluster %q: %w", serviceARN, clusterARN, err)
				}

				taskMu.Lock()
				tasks[serviceARN] = append(tasks[serviceARN], resp.Tasks...)
				taskMu.Unlock()

				return nil
			})
		}
	}

	return tasks, errg.Wait()
}

func batchSlice[T any](a []T, size int) [][]T {
	batches := make([][]T, 0, len(a)/size+1)
	for i := 0; i < len(a); i += size {
		end := min(i+size, len(a))
		batches = append(batches, a[i:end])
	}
	return batches
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

	clusterARNMap, err := d.describeClusters(ctx, clusters)
	if err != nil {
		return nil, err
	}

	clusterServiceARNMap, err := d.listServiceARNs(ctx, clusters)
	if err != nil {
		return nil, err
	}

	clusterServicesMap, err := d.describeServices(ctx, clusterServiceARNMap)
	if err != nil {
		return nil, err
	}

	// Use goroutines to process clusters in parallel
	var (
		targetsMu sync.Mutex
		wg        sync.WaitGroup
	)

	for clusterArn, clusterServices := range clusterServicesMap {
		if len(clusterServices) == 0 {
			continue
		}

		wg.Add(1)
		go func(clusterArn string, clusterServices []types.Service) {
			defer wg.Done()

			serviceTaskARNMap, err := d.listTaskARNs(ctx, clusterServices)
			if err != nil {
				d.logger.Error("Failed to list task ARNs for cluster", "cluster", clusterArn, "error", err)
				return
			}

			serviceTaskMap, err := d.describeTasks(ctx, clusterArn, serviceTaskARNMap)
			if err != nil {
				d.logger.Error("Failed to describe tasks for cluster", "cluster", clusterArn, "error", err)
				return
			}

			// Process services within this cluster in parallel
			var (
				serviceWg      sync.WaitGroup
				localTargets   []model.LabelSet
				localTargetsMu sync.Mutex
			)

			for _, clusterService := range clusterServices {
				serviceWg.Add(1)
				go func(clusterService types.Service) {
					defer serviceWg.Done()

					serviceArn := *clusterService.ServiceArn

					if tasks, exists := serviceTaskMap[serviceArn]; exists {
						var serviceTargets []model.LabelSet

						for _, task := range tasks {
							// Find the ENI attachment to get the private IP address
							var eniAttachment *types.Attachment
							for _, attachment := range task.Attachments {
								if attachment.Type != nil && *attachment.Type == "ElasticNetworkInterface" {
									eniAttachment = &attachment
									break
								}
							}
							if eniAttachment == nil {
								continue
							}

							var ipAddress, subnetID string
							for _, detail := range eniAttachment.Details {
								switch *detail.Name {
								case "privateIPv4Address":
									ipAddress = *detail.Value
								case "subnetId":
									subnetID = *detail.Value
								}
							}
							if ipAddress == "" {
								continue
							}

							labels := model.LabelSet{
								ecsLabelClusterARN:       model.LabelValue(*clusterService.ClusterArn),
								ecsLabelService:          model.LabelValue(*clusterService.ServiceName),
								ecsLabelServiceARN:       model.LabelValue(*clusterService.ServiceArn),
								ecsLabelServiceStatus:    model.LabelValue(*clusterService.Status),
								ecsLabelTaskGroup:        model.LabelValue(*task.Group),
								ecsLabelTaskARN:          model.LabelValue(*task.TaskArn),
								ecsLabelTaskDefinition:   model.LabelValue(*task.TaskDefinitionArn),
								ecsLabelIPAddress:        model.LabelValue(ipAddress),
								ecsLabelSubnetID:         model.LabelValue(subnetID),
								ecsLabelRegion:           model.LabelValue(d.cfg.Region),
								ecsLabelLaunchType:       model.LabelValue(task.LaunchType),
								ecsLabelAvailabilityZone: model.LabelValue(*task.AvailabilityZone),
								ecsLabelDesiredStatus:    model.LabelValue(*task.DesiredStatus),
								ecsLabelLastStatus:       model.LabelValue(*task.LastStatus),
								ecsLabelHealthStatus:     model.LabelValue(task.HealthStatus),
							}

							if task.PlatformFamily != nil {
								labels[ecsLabelPlatformFamily] = model.LabelValue(*task.PlatformFamily)
							}
							if task.PlatformVersion != nil {
								labels[ecsLabelPlatformVersion] = model.LabelValue(*task.PlatformVersion)
							}

							labels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(ipAddress, strconv.Itoa(d.cfg.Port)))

							// Add cluster tags
							if cluster, exists := clusterARNMap[*clusterService.ClusterArn]; exists {
								if cluster.ClusterName != nil {
									labels[ecsLabelCluster] = model.LabelValue(*cluster.ClusterName)
								}

								for _, clusterTag := range cluster.Tags {
									if clusterTag.Key != nil && clusterTag.Value != nil {
										labels[model.LabelName(ecsLabelTagCluster+strutil.SanitizeLabelName(*clusterTag.Key))] = model.LabelValue(*clusterTag.Value)
									}
								}
							}

							// Add service tags
							for _, serviceTag := range clusterService.Tags {
								if serviceTag.Key != nil && serviceTag.Value != nil {
									labels[model.LabelName(ecsLabelTagService+strutil.SanitizeLabelName(*serviceTag.Key))] = model.LabelValue(*serviceTag.Value)
								}
							}

							// Add task tags
							for _, taskTag := range task.Tags {
								if taskTag.Key != nil && taskTag.Value != nil {
									labels[model.LabelName(ecsLabelTagTask+strutil.SanitizeLabelName(*taskTag.Key))] = model.LabelValue(*taskTag.Value)
								}
							}

							serviceTargets = append(serviceTargets, labels)
						}

						// Add service targets to local targets with mutex protection
						localTargetsMu.Lock()
						localTargets = append(localTargets, serviceTargets...)
						localTargetsMu.Unlock()
					}
				}(clusterService)
			}

			serviceWg.Wait()

			// Add all local targets to main target group with mutex protection
			targetsMu.Lock()
			tg.Targets = append(tg.Targets, localTargets...)
			targetsMu.Unlock()
		}(clusterArn, clusterServices)
	}

	wg.Wait()

	return []*targetgroup.Group{tg}, nil
}
