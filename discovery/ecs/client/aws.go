// Copyright 2016 The Prometheus Authors
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

package client

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
	"github.com/prometheus/common/log"
	"github.com/prometheus/prometheus/discovery/ecs/types"
	"golang.org/x/sync/errgroup"
)

// Generate ECS API mocks running go generate.
//go:generate mockery -dir ../../vendor/github.com/aws/aws-sdk-go/service/ecs/ecsiface/ -name ECSAPI -output ./mock/aws/sdk/  -outpkg sdk
//go:generate mockery -dir ../../vendor/github.com/aws/aws-sdk-go/service/ec2/ec2iface/ -name EC2API -output ./mock/aws/sdk/  -outpkg sdk

const (
	maxAPIRes = 100
)

// ecsTargetTask is a helper that has all the objects required to create a final service instances.
type ecsTargetTask struct {
	cluster  *ecs.Cluster
	task     *ecs.Task
	instance *ec2.Instance
	taskDef  *ecs.TaskDefinition
	service  *ecs.Service
}

func (a *ecsTargetTask) createServiceInstances() []*types.ServiceInstance {
	// Create ec2 tags.
	tags := map[string]string{}
	for _, t := range a.instance.Tags {
		tags[aws.StringValue(t.Key)] = aws.StringValue(t.Value)
	}

	// Create container definition index.
	cDefIdx := map[string]*ecs.ContainerDefinition{}
	for _, c := range a.taskDef.ContainerDefinitions {
		cDefIdx[aws.StringValue(c.Name)] = c
	}

	sis := []*types.ServiceInstance{}
	for _, c := range a.task.Containers {
		cDef := cDefIdx[aws.StringValue(c.Name)]

		// Create container labels.
		labels := map[string]string{}
		for k, v := range cDef.DockerLabels {
			labels[k] = aws.StringValue(v)
		}

		for _, n := range c.NetworkBindings {
			if n.HostPort != nil {
				addr := fmt.Sprintf("%s:%d", aws.StringValue(a.instance.PrivateIpAddress), aws.Int64Value(n.HostPort))
				s := &types.ServiceInstance{
					Cluster:            aws.StringValue(a.cluster.ClusterName),
					Addr:               addr,
					Service:            aws.StringValue(a.service.ServiceName),
					Tags:               tags,
					Image:              aws.StringValue(cDef.Image),
					Container:          aws.StringValue(c.Name),
					ContainerPort:      strconv.FormatInt(aws.Int64Value(n.ContainerPort), 10),
					ContainerPortProto: aws.StringValue(n.Protocol),
					Labels:             labels,
				}
				sis = append(sis, s)
			}
		}
	}
	return sis
}

// AWSRetriever is the wrapper around AWS client.
type AWSRetriever struct {
	ecsCli ecsiface.ECSAPI
	ec2Cli ec2iface.EC2API

	cache  *awsCache // cache will store all the retrieved objects in order to compose the targets at the final stage.
	logger log.Logger
}

// NewAWSRetriever will create a new AWS API retriever.
func NewAWSRetriever(l log.Logger, accessKey, secretKey, region, profile string) (*AWSRetriever, error) {
	creds := credentials.NewStaticCredentials(accessKey, secretKey, "")
	if accessKey == "" && secretKey == "" {
		creds = nil
	}
	cfg := aws.Config{
		Region:      aws.String(region),
		Credentials: creds,
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		Config:  cfg,
		Profile: profile,
	})

	if err != nil {
		return nil, err
	}

	return &AWSRetriever{
		ecsCli: ecs.New(sess),
		ec2Cli: ec2.New(sess),
		cache:  newAWSCache(),
		logger: l.With("client", "AWS"),
	}, nil

}

func (c *AWSRetriever) getClusters(ctx context.Context) ([]*ecs.Cluster, error) {

	clusters := []*ecs.Cluster{}

	select {
	case <-ctx.Done():
		return clusters, nil
	default:
	}

	params := &ecs.ListClustersInput{
		MaxResults: aws.Int64(maxAPIRes),
	}

	// Start listing clusters.
	for {
		resp, err := c.ecsCli.ListClusters(params)
		if err != nil {
			return nil, err
		}

		cArns := []*string{}
		for _, cl := range resp.ClusterArns {
			cArns = append(cArns, cl)
		}

		// Describe clusters.
		params2 := &ecs.DescribeClustersInput{
			Clusters: cArns,
		}

		resp2, err := c.ecsCli.DescribeClusters(params2)
		if err != nil {
			return nil, err
		}

		for _, cl := range resp2.Clusters {
			clusters = append(clusters, cl)
		}

		if resp.NextToken == nil || aws.StringValue(resp.NextToken) == "" {
			break
		}
		params.NextToken = resp.NextToken
	}
	return clusters, nil
}

func (c *AWSRetriever) getContainerInstances(ctx context.Context, cluster *ecs.Cluster) ([]*ecs.ContainerInstance, error) {
	contInsts := []*ecs.ContainerInstance{}

	select {
	case <-ctx.Done():
		return contInsts, nil
	default:
	}

	params := &ecs.ListContainerInstancesInput{
		Cluster:    cluster.ClusterArn,
		MaxResults: aws.Int64(maxAPIRes),
	}
	// Start listing container instances.
	for {
		resp, err := c.ecsCli.ListContainerInstances(params)
		if err != nil {
			return nil, err
		}

		ciArns := []*string{}
		for _, ci := range resp.ContainerInstanceArns {
			ciArns = append(ciArns, ci)
		}

		// Describe container instances.
		params2 := &ecs.DescribeContainerInstancesInput{
			Cluster:            cluster.ClusterArn,
			ContainerInstances: ciArns,
		}

		resp2, err := c.ecsCli.DescribeContainerInstances(params2)
		if err != nil {
			return nil, err
		}

		for _, ci := range resp2.ContainerInstances {
			contInsts = append(contInsts, ci)
		}

		if resp.NextToken == nil || aws.StringValue(resp.NextToken) == "" {
			break
		}
		params.NextToken = resp.NextToken

	}
	return contInsts, nil
}

func (c *AWSRetriever) getTasks(ctx context.Context, cluster *ecs.Cluster) ([]*ecs.Task, error) {
	tasks := []*ecs.Task{}

	select {
	case <-ctx.Done():
		return tasks, nil
	default:
	}

	params := &ecs.ListTasksInput{
		Cluster:    cluster.ClusterArn,
		MaxResults: aws.Int64(maxAPIRes),
	}

	// Start listing tasks.
	for {
		resp, err := c.ecsCli.ListTasks(params)
		if err != nil {
			return nil, err
		}

		tArns := []*string{}
		for _, t := range resp.TaskArns {
			tArns = append(tArns, t)
		}

		// Describe tasks.
		params2 := &ecs.DescribeTasksInput{
			Cluster: cluster.ClusterArn,
			Tasks:   tArns,
		}

		resp2, err := c.ecsCli.DescribeTasks(params2)
		if err != nil {
			return nil, err
		}

		for _, t := range resp2.Tasks {
			// We don't want stopped tasks.
			if t.StoppedAt == nil {
				tasks = append(tasks, t)
			}
		}

		if resp.NextToken == nil || aws.StringValue(resp.NextToken) == "" {
			break
		}
		params.NextToken = resp.NextToken

	}
	return tasks, nil
}

func (c *AWSRetriever) getServices(ctx context.Context, cluster *ecs.Cluster) ([]*ecs.Service, error) {
	srvs := []*ecs.Service{}

	select {
	case <-ctx.Done():
		return srvs, nil
	default:
	}

	params := &ecs.ListServicesInput{
		Cluster:    cluster.ClusterArn,
		MaxResults: aws.Int64(maxAPIRes),
	}
	// Start listing services.
	for {
		resp, err := c.ecsCli.ListServices(params)
		if err != nil {
			return nil, err
		}

		srvArns := []*string{}
		for _, srv := range resp.ServiceArns {
			srvArns = append(srvArns, srv)
		}

		// Describe services.
		params2 := &ecs.DescribeServicesInput{
			Cluster:  cluster.ClusterArn,
			Services: srvArns,
		}

		resp2, err := c.ecsCli.DescribeServices(params2)
		if err != nil {
			return nil, err
		}

		for _, srv := range resp2.Services {
			srvs = append(srvs, srv)
		}

		if resp.NextToken == nil || aws.StringValue(resp.NextToken) == "" {
			break
		}
		params.NextToken = resp.NextToken
	}
	return srvs, nil
}

func (c *AWSRetriever) getTaskDefinitions(ctx context.Context, tDIDs []*string, useCache bool) ([]*ecs.TaskDefinition, error) {
	tDefs := []*ecs.TaskDefinition{}

	select {
	case <-ctx.Done():
		return tDefs, nil
	default:
	}

	var mErr error
	var wg sync.WaitGroup
	cached := 0
	for _, tID := range tDIDs {
		// If we want cache then check if already present (and save as retrieved to emulate a hit on cache).
		if td, ok := c.cache.getTaskDefinition(aws.StringValue(tID)); useChache && ok {
			tDefs = append(tDefs, td)
			cached++
			continue
		}
		wg.Add(1)
		go func(tID string) {
			defer wg.Done()
			params := &ecs.DescribeTaskDefinitionInput{
				TaskDefinition: aws.String(tID),
			}
			if mErr != nil {
				return
			}
			resp, err := c.ecsCli.DescribeTaskDefinition(params)
			if err != nil {
				mErr = err
				return
			}

			tDefs = append(tDefs, resp.TaskDefinition)
		}(*tID)
	}
	wg.Wait()

	log.Debugf("Retrieved %d new task definitions, cached %d", len(tDefs), cached)
	return tDefs, mErr
}

func (c *AWSRetriever) getInstances(ctx context.Context, ec2IDs []*string) ([]*ec2.Instance, error) {
	instances := []*ec2.Instance{}

	select {
	case <-ctx.Done():
		return instances, nil
	default:
	}

	params := &ec2.DescribeInstancesInput{
		InstanceIds: ec2IDs,
	}

	// Start describing instances.
	for {
		resp, err := c.ec2Cli.DescribeInstances(params)
		if err != nil {
			return nil, err
		}

		for _, r := range resp.Reservations {
			for _, i := range r.Instances {
				instances = append(instances, i)
			}
		}

		if resp.NextToken == nil || aws.StringValue(resp.NextToken) == "" {
			break
		}
		params.NextToken = resp.NextToken
	}

	return instances, nil
}

// cleanStaleCache will flush all the caches that have dynamic data (everything except task definitions).
func (c *AWSRetriever) cleanStaleCache() {
	c.cache.flushClusters()
	c.cache.flushContainerInstances()
	c.cache.flushTasks()
	c.cache.flushInstances()
	c.cache.flushServices()
}

// Retrieve will get all the service instance calling multiple AWS APIs.
func (c *AWSRetriever) Retrieve() ([]*types.ServiceInstance, error) {
	// Create a ne context and errgroup for this retrieval iteration.
	ctx := context.Background()
	g, ctx := errgroup.WithContext(ctx)

	// First get the clusters and map them.
	clusters, err := c.getClusters(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.SetClusters(clusters...)

	// Temporal variable to store task definitions so we can flush the task defs cache freely.
	tTaskDefs := []*ecs.TaskDefinition{}
	var tTaskDefsMutex sync.Mutex

	// For each cluster get all the required objects from AWS.
	for _, cluster := range clusters {
		clstCopy := cluster // Copy cluster so we don't use the same variable in all iterations of the loop.
		g.Go(func() error {
			// Get cluster container instances.
			cInstances, err2 := c.getContainerInstances(ctx, clstCopy)
			if err2 != nil {
				return err2
			}

			ec2Ids := []*string{}
			c.cache.setContainerInstances(cInstances...)
			for _, ci := range cInstances {
				ec2Ids = append(ec2Ids, ci.Ec2InstanceId)
			}

			// Get cluster ec2 instances.
			instances, err2 := c.getInstances(ctx, ec2Ids)
			if err2 != nil {
				return err2
			}

			c.cache.setInstances(instances...)

			// Get cluster tasks.
			ts, err2 := c.getTasks(ctx, clstCopy)
			if err2 != nil {
				return err2
			}

			c.cache.setTasks(ts...)
			tDefIDs := []*string{}
			for _, t := range ts {
				tDefIDs = append(tDefIDs, t.TaskDefinitionArn)
			}

			// Get cluster services.
			srvs, err2 := c.getServices(ctx, clstCopy)
			if err2 != nil {
				return err2
			}

			c.cache.setServices(srvs...)
			// Get task definitions.
			tds, err2 := c.getTaskDefinitions(ctx, tDefIDs, true)
			if err2 != nil {
				return err2
			}
			tTaskDefsMutex.Lock()
			tTaskDefs = append(tTaskDefs, tds...)
			tTaskDefsMutex.Unlock()

			return nil
		})
	}

	if err = g.Wait(); err != nil {
		return nil, err
	}

	// Flush stale task definitions and set the new ones (including the old ones been hit).
	c.cache.flushTaskDefinitions()
	c.cache.setTaskDefinition(tTaskDefs...)

	// Create the targets.
	sInstances := []*types.ServiceInstance{}
	for _, v := range c.cache.tasks {
		clID := aws.StringValue(v.ClusterArn)
		cl, ok := c.cache.getCluster(clID)
		if !ok {
			return nil, fmt.Errorf("cluster not available: %s", clID)
		}

		ciID := aws.StringValue(v.ContainerInstanceArn)
		ci, ok := c.cache.getContainerInstance(ciID)
		if !ok {
			return nil, fmt.Errorf("container not available: %s", ciID)
		}

		iID := aws.StringValue(ci.Ec2InstanceId)
		ec2I, ok := c.cache.getIntance(iID)
		if !ok {
			return nil, fmt.Errorf("ec2 instance not available: %s", iID)
		}

		tdID := aws.StringValue(v.TaskDefinitionArn)
		srv, ok := c.cache.getService(tdID)
		if !ok {
			return nil, fmt.Errorf("service not available: %s", tdID)
		}

		td, ok := c.cache.getTaskDefinition(tdID)
		if !ok {
			return nil, fmt.Errorf("task definition not available: %s", tdID)
		}

		t := &ecsTargetTask{
			cluster:  cl,
			task:     v,
			instance: ec2I,
			service:  srv,
			taskDef:  td,
		}

		sis := t.createServiceInstances()
		for _, i := range sis {
			sInstances = append(sInstances, i)
		}
	}

	return sInstances, nil
}
