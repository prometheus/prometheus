// Copyright 2021 The Prometheus Authors
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
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/prometheus/util/strutil"
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	ecsLabel           = model.MetaLabelPrefix + "ecs_"
	ecsLabelRegion     = ecsLabel + "region"
	ecsLabelAZ         = ecsLabel + "availability_zone"
	ecsLabelAZID       = ecsLabel + "availability_zone_id"
	ecsLabelClusterArn = ecsLabel + "cluster_arn"

	ecsLabelTaskLaunchType      = ecsLabel + "task_launch_type"
	ecsLabelTaskFamily          = ecsLabel + "task_family"
	ecsLabelTaskFamilyRev       = ecsLabel + "task_family_revision"
	ecsLabelTaskGroup           = ecsLabel + "task_group"
	ecsLabelTaskStatus          = ecsLabel + "task_status"
	ecsLabelTaskDesiredStatus   = ecsLabel + "task_desired_status"
	ecsLabelTaskNetworkMode     = ecsLabel + "task_network_mode"
	ecsLabelTaskPlatformFamily  = ecsLabel + "task_platform_family"
	ecsLabelTaskPlatformVersion = ecsLabel + "task_platform_version"
	ecsLabelTaskVersion         = ecsLabel + "task_version"
	ecsLabelTaskTag             = ecsLabel + "task_tag_"
)

// DefaultECSSDConfig is the default ECS SD configuration.
var DefaultECSSDConfig = ECSSDConfig{
	RefreshInterval:    model.Duration(60 * time.Second),
	HTTPClientConfig:   config.DefaultHTTPClientConfig,
	RequestConcurrency: 5,
	Port:               80,
}

func init() {
	discovery.RegisterConfig(&ECSSDConfig{})
}

// ECSSDConfig is the configuration for ECS-based service discovery.
type ECSSDConfig struct {
	Endpoint  string        `yaml:"endpoint"`
	Region    string        `yaml:"region"`
	AccessKey string        `yaml:"access_key,omitempty"`
	SecretKey config.Secret `yaml:"secret_key,omitempty"`
	Profile   string        `yaml:"profile,omitempty"`
	RoleARN   string        `yaml:"role_arn,omitempty"`

	Clusters           []string       `yaml:"clusters,omitempty"`
	Port               int            `yaml:"port,omitempty"`
	RefreshInterval    model.Duration `yaml:"refresh_interval,omitempty"`
	RequestConcurrency int            `yaml:"request_concurrency,omitempty"`

	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
}

func (c *ECSSDConfig) BaseEndpoint() *string {
	if c.Endpoint == "" {
		return nil
	}
	return &c.Endpoint
}

// NewDiscovererMetrics implements discovery.Config.
func (*ECSSDConfig) NewDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &ecsMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the ECS Config.
func (*ECSSDConfig) Name() string { return "ecs" }

// NewDiscoverer returns a Discoverer for the ECS Config.
func (c *ECSSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewECSDiscovery(c, opts.Logger, opts.Metrics)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the ECS Config.
func (c *ECSSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
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
	ecs.ListClustersAPIClient
	ecs.ListTasksAPIClient
	DescribeTasks(ctx context.Context, params *ecs.DescribeTasksInput, optFns ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
	DescribeTaskDefinition(ctx context.Context, params *ecs.DescribeTaskDefinitionInput, optFns ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
	DescribeContainerInstances(ctx context.Context, params *ecs.DescribeContainerInstancesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeContainerInstancesOutput, error)
}

// ECSDiscovery periodically performs ECS-SD requests. It implements
// the Discoverer interface.
type ECSDiscovery struct {
	*refresh.Discovery
	logger *slog.Logger
	cfg    *ECSSDConfig
	ecs    ecsClient
	ec2    ec2Client

	// azToAZID maps this account's availability zones to their underlying AZ
	// ID, e.g. eu-west-2a -> euw2-az2. Refreshes are performed sequentially, so
	// no locking is required.
	azToAZID map[string]string

	taskDefCache map[string]types.TaskDefinition
}

// NewECSDiscovery returns a new ECSDiscovery which periodically refreshes its targets.
func NewECSDiscovery(conf *ECSSDConfig, logger *slog.Logger, metrics discovery.DiscovererMetrics) (*ECSDiscovery, error) {
	m, ok := metrics.(*ecsMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if logger == nil {
		logger = promslog.NewNopLogger()
	}

	d := &ECSDiscovery{
		logger:       logger,
		cfg:          conf,
		taskDefCache: make(map[string]types.TaskDefinition),
	}

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              logger,
			Mech:                "ecs",
			Interval:            time.Duration(d.cfg.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)

	return d, nil
}

func (d *ECSDiscovery) initAWSClients(ctx context.Context) error {
	if d.ecs != nil && d.ec2 != nil {
		return nil
	}

	var creds aws.CredentialsProvider
	creds = aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(d.cfg.AccessKey, string(d.cfg.SecretKey), ""))
	if d.cfg.AccessKey == "" && d.cfg.SecretKey == "" {
		creds = nil
	}

	cfg, err := awsConfig.LoadDefaultConfig(ctx, func(options *awsConfig.LoadOptions) error {
		options.Region = d.cfg.Region
		options.Credentials = creds
		options.SharedConfigProfile = d.cfg.Profile
		return nil
	})
	if err != nil {
		return fmt.Errorf("could not create aws config: %w", err)
	}

	if d.cfg.RoleARN != "" {
		creds = stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), d.cfg.RoleARN)
	}

	client, err := config.NewClientFromConfig(d.cfg.HTTPClientConfig, "ecs_sd")
	if err != nil {
		return err
	}

	d.ecs = ecs.NewFromConfig(cfg, func(options *ecs.Options) {
		options.BaseEndpoint = d.cfg.BaseEndpoint()
		options.HTTPClient = client
	})

	d.ec2 = ec2.NewFromConfig(cfg, func(options *ec2.Options) {
		options.BaseEndpoint = d.cfg.BaseEndpoint()
		options.HTTPClient = client
	})

	return nil
}

func (d *ECSDiscovery) refreshAZIDs(ctx context.Context) error {
	azs, err := d.ec2.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{})
	if err != nil {
		return err
	}
	d.azToAZID = make(map[string]string, len(azs.AvailabilityZones))
	for _, az := range azs.AvailabilityZones {
		d.azToAZID[*az.ZoneName] = *az.ZoneId
	}

	return nil
}

func (d *ECSDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {

	err := d.initAWSClients(ctx)
	if err != nil {
		return nil, err
	}

	// Only refresh the AZ ID map if we have never been able to build one.
	// Prometheus requires a reload if AWS adds a new AZ to the region.
	if d.azToAZID == nil {
		if err := d.refreshAZIDs(ctx); err != nil {
			d.logger.Debug("Unable to describe availability zones", "err", err)
		}
	}

	var clusters []string // short names or ARNs (doesn't matter)
	if len(d.cfg.Clusters) == 0 {
		clusters, err = d.listClusterARNs(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		clusters = d.cfg.Clusters
	}

	tg := &targetgroup.Group{
		Source: d.cfg.Region,
	}

	clusterTaskARNs, err := d.listTaskARNs(ctx, clusters)
	if err != nil {
		return nil, err
	}

	clusterTasks, err := d.describeTasks(ctx, clusterTaskARNs)
	if err != nil {
		return nil, err
	}

	// build a map with container instance ARNs per cluster
	clusterInstanceARNs := map[string][]string{}
	for cluster, tasks := range clusterTasks {
		for _, task := range tasks {
			if task.ContainerInstanceArn == nil {
				continue
			}
			clusterInstanceARNs[cluster] = append(clusterInstanceARNs[cluster], *task.ContainerInstanceArn)
		}
	}

	var (
		containerInstances map[string]types.ContainerInstance
		ec2Instances       map[string]ec2types.Instance
		tasks              []Task
	)

	errg, ectx := errgroup.WithContext(ctx)
	errg.Go(func() error {
		var err error
		containerInstances, err = d.describeContainerInstances(ectx, clusterInstanceARNs)
		if err != nil {
			return err
		}
		ec2Instances, err = d.describeEC2Instances(ectx, containerInstances)
		return err
	})
	errg.Go(func() error {
		res, err := d.describeTaskDefinitions(ctx, clusterTasks)
		tasks = res
		return err
	})
	if err := errg.Wait(); err != nil {
		return nil, err
	}

	for _, task := range tasks {

		labels := model.LabelSet{
			ecsLabelRegion:     model.LabelValue(d.cfg.Region),
			ecsLabelClusterArn: model.LabelValue(*task.ClusterArn),
			ecsLabelAZ:         model.LabelValue(*task.AvailabilityZone),

			ecsLabelTaskLaunchType:    model.LabelValue(task.LaunchType),
			ecsLabelTaskFamily:        model.LabelValue(*task.Family),
			ecsLabelTaskFamilyRev:     model.LabelValue(strconv.Itoa(int(task.Revision))),
			ecsLabelTaskGroup:         model.LabelValue(*task.Group),
			ecsLabelTaskStatus:        model.LabelValue(task.Status),
			ecsLabelTaskDesiredStatus: model.LabelValue(*task.DesiredStatus),
			ecsLabelTaskNetworkMode:   model.LabelValue(task.NetworkMode),
			ecsLabelTaskVersion:       model.LabelValue(strconv.Itoa(int(task.Version))),
		}

		var host string

		azID, ok := d.azToAZID[*task.AvailabilityZone]
		if !ok && d.azToAZID != nil {
			d.logger.Debug("Availability zone ID not found", "az", *task.AvailabilityZone)
		} else {
			labels[ecsLabelAZID] = model.LabelValue(azID)
		}

		switch task.LaunchType {
		case types.LaunchTypeEc2:
			if task.ContainerInstanceArn == nil {
				continue
			}

			containerInstance, found := containerInstances[*task.ContainerInstanceArn]
			if !found {
				continue
			}

			ec2Instance, found := ec2Instances[*containerInstance.Ec2InstanceId]
			if !found {
				continue
			}

			if ec2Instance.PrivateIpAddress != nil {
				host = *ec2Instance.PrivateIpAddress
			}

		case types.LaunchTypeFargate:

			labels[ecsLabelTaskPlatformFamily] = model.LabelValue(*task.PlatformFamily)
			labels[ecsLabelTaskPlatformVersion] = model.LabelValue(*task.PlatformVersion)

		containersLoop:
			for _, container := range task.Task.Containers {
				for _, ni := range container.NetworkInterfaces {
					if ni.PrivateIpv4Address == nil || *ni.PrivateIpv4Address == "" {
						continue
					}
					host = *ni.PrivateIpv4Address
					break containersLoop
				}
			}
		case types.LaunchTypeExternal:
			// not supported
			continue
		}

		addr := net.JoinHostPort(host, strconv.Itoa(d.cfg.Port))
		labels[model.AddressLabel] = model.LabelValue(addr)

		for _, t := range task.Tags {
			if t.Key == nil || t.Value == nil {
				continue
			}
			name := strutil.SanitizeLabelName(*t.Key)
			labels[ecsLabelTaskTag+model.LabelName(name)] = model.LabelValue(*t.Value)
		}

		tg.Targets = append(tg.Targets, labels)
	}

	return []*targetgroup.Group{tg}, nil
}

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

func (d *ECSDiscovery) listTaskARNs(ctx context.Context, clusters []string) (map[string][]string, error) {
	taskARNsMu := sync.Mutex{}
	taskARNs := make(map[string][]string)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for _, cluster := range clusters {
		errg.Go(func() error {
			var nextToken *string
			for {
				resp, err := d.ecs.ListTasks(ectx, &ecs.ListTasksInput{
					Cluster:   aws.String(cluster),
					NextToken: nextToken,
				})
				if err != nil {
					return fmt.Errorf("could not list tasks for cluster %s: %w", cluster, err)
				}

				taskARNsMu.Lock()
				for _, taskARN := range resp.TaskArns {
					taskARNs[cluster] = append(taskARNs[cluster], taskARN)
				}
				taskARNsMu.Unlock()

				if resp.NextToken == nil {
					break
				}

				nextToken = resp.NextToken
			}
			return nil
		})
	}
	return taskARNs, errg.Wait()
}

func (d *ECSDiscovery) describeTasks(ctx context.Context, taskARNsMap map[string][]string) (map[string][]types.Task, error) {
	batchSize := 100
	taskMu := sync.Mutex{}
	tasks := make(map[string][]types.Task)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for clusterARN, taskARNs := range taskARNsMap {
		for _, batch := range batchSlice(taskARNs, batchSize) {
			errg.Go(func() error {
				resp, err := d.ecs.DescribeTasks(ectx, &ecs.DescribeTasksInput{
					Tasks:   batch,
					Cluster: aws.String(clusterARN),
					Include: []types.TaskField{"TAGS"},
				})
				if err != nil {
					return fmt.Errorf("could not describe tasks for clusters %q: %w", clusterARN, err)
				}

				taskMu.Lock()
				tasks[clusterARN] = append(tasks[clusterARN], resp.Tasks...)
				taskMu.Unlock()

				return nil
			})
		}
	}

	return tasks, errg.Wait()
}

type Task struct {
	Cluster string
	types.Task
	types.TaskDefinition
}

func (d *ECSDiscovery) describeTaskDefinitions(ctx context.Context, clusterTasks map[string][]types.Task) ([]Task, error) {
	taskMu := sync.Mutex{}
	tasks := []Task{}
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for cluster, taskARNs := range clusterTasks {
		for _, task := range taskARNs {
			if def, found := d.taskDefCache[*task.TaskDefinitionArn]; found {
				taskMu.Lock()
				tasks = append(tasks, Task{
					Cluster:        cluster,
					Task:           task,
					TaskDefinition: def,
				})
				taskMu.Unlock()
				continue
			}

			errg.Go(func() error {
				resp, err := d.ecs.DescribeTaskDefinition(ectx, &ecs.DescribeTaskDefinitionInput{
					TaskDefinition: task.TaskDefinitionArn,
				})
				if err != nil {
					return err
				}

				taskMu.Lock()
				tasks = append(tasks, Task{
					Cluster:        cluster,
					Task:           task,
					TaskDefinition: *resp.TaskDefinition,
				})
				taskMu.Unlock()

				return nil
			})
		}
	}
	err := errg.Wait()

	d.taskDefCache = map[string]types.TaskDefinition{}
	for _, task := range tasks {
		d.taskDefCache[*task.Task.TaskDefinitionArn] = task.TaskDefinition
	}

	return tasks, err
}

func (d *ECSDiscovery) describeContainerInstances(ctx context.Context, clusterInstanceARNs map[string][]string) (map[string]types.ContainerInstance, error) {
	batchSize := 100
	instancesMu := sync.Mutex{}
	instances := make(map[string]types.ContainerInstance)

	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for cluster, instanceARNs := range clusterInstanceARNs {
		for _, batch := range batchSlice(instanceARNs, batchSize) {
			errg.Go(func() error {
				resp, err := d.ecs.DescribeContainerInstances(ectx, &ecs.DescribeContainerInstancesInput{
					Cluster:            aws.String(cluster),
					ContainerInstances: batch,
				})
				if err != nil {
					return fmt.Errorf("could not describe container instances for clusters %s: %w", cluster, err)
				}

				instancesMu.Lock()
				for _, ci := range resp.ContainerInstances {
					instances[*ci.ContainerInstanceArn] = ci
				}
				instancesMu.Unlock()

				return nil
			})
		}
	}

	return instances, errg.Wait()
}

func (d *ECSDiscovery) describeEC2Instances(ctx context.Context, containerInstances map[string]types.ContainerInstance) (map[string]ec2types.Instance, error) {
	instancesIDs := make([]string, 0, len(containerInstances))
	for _, containerInstance := range containerInstances {
		instancesIDs = append(instancesIDs, *containerInstance.Ec2InstanceId)
	}

	batchSize := 25
	instancesMu := sync.Mutex{}
	instances := make(map[string]ec2types.Instance)
	errg, ectx := errgroup.WithContext(ctx)
	errg.SetLimit(d.cfg.RequestConcurrency)
	for _, batch := range batchSlice(instancesIDs, batchSize) {
		errg.Go(func() error {
			for {
				resp, err := d.ec2.DescribeInstances(ectx, &ec2.DescribeInstancesInput{
					InstanceIds: batch,
				})
				if err != nil {
					return fmt.Errorf("could not describe ec2 instance: %w", err)
				}

				instancesMu.Lock()
				for _, reservation := range resp.Reservations {
					for _, instance := range reservation.Instances {
						instances[*instance.InstanceId] = instance
					}
				}
				instancesMu.Unlock()

				if resp.NextToken == nil {
					break
				}
			}
			return nil
		})
	}

	return instances, errg.Wait()
}

func batchSlice[T any](a []T, size int) [][]T {
	batches := make([][]T, 0, len(a)/size+1)
	for i := 0; i < len(a); i += size {
		end := i + size
		if end > len(a) {
			end = len(a)
		}
		batches = append(batches, a[i:end])
	}
	return batches
}
