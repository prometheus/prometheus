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
)

// Generate ECS API mocks running go generate
//go:generate mockgen -source ../../vendor/github.com/aws/aws-sdk-go/service/ecs/ecsiface/interface.go -package sdk -destination ./mock/aws/sdk/ecsiface_mock.go
//go:generate mockgen -source ../../vendor/github.com/aws/aws-sdk-go/service/ec2/ec2iface/interface.go -package sdk -destination ./mock/aws/sdk/ec2iface_mock.go

const (
	maxAPIRes = 100
)

// ecsTargetTask is a helper that has all the objects required to create a final service instances
type ecsTargetTask struct {
	cluster  *ecs.Cluster
	task     *ecs.Task
	instance *ec2.Instance
	taskDef  *ecs.TaskDefinition
	service  *ecs.Service
}

func (a *ecsTargetTask) createServiceInstances() []*types.ServiceInstance {
	// Create ec2 tags
	tags := map[string]string{}
	for _, t := range a.instance.Tags {
		tags[aws.StringValue(t.Key)] = aws.StringValue(t.Value)
	}

	// create container definition index
	cDefIdx := map[string]*ecs.ContainerDefinition{}
	for _, c := range a.taskDef.ContainerDefinitions {
		cDefIdx[aws.StringValue(c.Name)] = c
	}

	sis := []*types.ServiceInstance{}
	for _, c := range a.task.Containers {
		cDef := cDefIdx[aws.StringValue(c.Name)]

		// Create container labels
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

// AWSRetriever is the wrapper around AWS client
type AWSRetriever struct {
	ecsCli ecsiface.ECSAPI
	ec2Cli ec2iface.EC2API

	cache *awsCache // cache will store all the retrieved objects in order to compose the targets at the final stage
}

// NewAWSRetriever will create a new AWS API retriever
func NewAWSRetriever(accessKey, secretKey, region, profile string) (*AWSRetriever, error) {
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
	}, nil

}

func (c *AWSRetriever) getClusters() ([]*ecs.Cluster, error) {
	clusters := []*ecs.Cluster{}

	params := &ecs.ListClustersInput{
		MaxResults: aws.Int64(maxAPIRes),
	}
	log.Debugf("Getting clusters")
	// Start listing clusters
	for {
		resp, err := c.ecsCli.ListClusters(params)
		if err != nil {
			return nil, err
		}

		cArns := []*string{}
		for _, cl := range resp.ClusterArns {
			cArns = append(cArns, cl)
		}

		// Describe clusters
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
	log.Debugf("Retrieved %d clusters", len(clusters))
	return clusters, nil
}

func (c *AWSRetriever) getContainerInstances(cluster *ecs.Cluster) ([]*ecs.ContainerInstance, error) {
	contInsts := []*ecs.ContainerInstance{}

	params := &ecs.ListContainerInstancesInput{
		Cluster:    cluster.ClusterArn,
		MaxResults: aws.Int64(maxAPIRes),
	}
	log.Debugf("Getting cluster %s container instances", aws.StringValue(cluster.ClusterName))
	// Start listing container instances
	for {
		resp, err := c.ecsCli.ListContainerInstances(params)
		if err != nil {
			return nil, err
		}

		ciArns := []*string{}
		for _, ci := range resp.ContainerInstanceArns {
			ciArns = append(ciArns, ci)
		}

		// Describe container instances
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
	log.Debugf("Retrieved %d container instances on cluster %s", len(contInsts), aws.StringValue(cluster.ClusterName))
	return contInsts, nil
}

func (c *AWSRetriever) getTasks(cluster *ecs.Cluster) ([]*ecs.Task, error) {
	tasks := []*ecs.Task{}

	params := &ecs.ListTasksInput{
		Cluster:    cluster.ClusterArn,
		MaxResults: aws.Int64(maxAPIRes),
	}
	log.Debugf("Getting cluster %s tasks", aws.StringValue(cluster.ClusterName))

	// Start listing tasks
	for {
		resp, err := c.ecsCli.ListTasks(params)
		if err != nil {
			return nil, err
		}

		tArns := []*string{}
		for _, t := range resp.TaskArns {
			tArns = append(tArns, t)
		}

		// Describe tasks
		params2 := &ecs.DescribeTasksInput{
			Cluster: cluster.ClusterArn,
			Tasks:   tArns,
		}

		resp2, err := c.ecsCli.DescribeTasks(params2)
		if err != nil {
			return nil, err
		}

		for _, t := range resp2.Tasks {
			// We don't want stopped tasks
			if t.StoppedAt == nil {
				tasks = append(tasks, t)
			}
		}

		if resp.NextToken == nil || aws.StringValue(resp.NextToken) == "" {
			break
		}
		params.NextToken = resp.NextToken

	}
	log.Debugf("Retrieved %d tasks on cluster %s", len(tasks), aws.StringValue(cluster.ClusterName))
	return tasks, nil
}

func (c *AWSRetriever) getServices(cluster *ecs.Cluster) ([]*ecs.Service, error) {
	srvs := []*ecs.Service{}

	params := &ecs.ListServicesInput{
		Cluster:    cluster.ClusterArn,
		MaxResults: aws.Int64(maxAPIRes),
	}
	log.Debugf("Getting cluster %s services", aws.StringValue(cluster.ClusterName))
	// Start listing services
	for {
		resp, err := c.ecsCli.ListServices(params)
		if err != nil {
			return nil, err
		}

		srvArns := []*string{}
		for _, srv := range resp.ServiceArns {
			srvArns = append(srvArns, srv)
		}

		// Describe services
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
	log.Debugf("Retrieved %d services on cluster %s", len(srvs), aws.StringValue(cluster.ClusterName))
	return srvs, nil
}

func (c *AWSRetriever) getTaskDefinitions(tDIDs []*string, useChache bool) ([]*ecs.TaskDefinition, error) {
	tDefs := []*ecs.TaskDefinition{}
	var mErr error
	var wg sync.WaitGroup
	cached := 0
	for _, tID := range tDIDs {
		// If we want cache then check if already present
		if _, ok := c.cache.getTaskDefinition(aws.StringValue(tID)); useChache && ok {
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

func (c *AWSRetriever) getInstances(ec2IDs []*string) ([]*ec2.Instance, error) {

	instances := []*ec2.Instance{}
	log.Debugf("Getting ec2 instances")

	params := &ec2.DescribeInstancesInput{
		InstanceIds: ec2IDs,
	}

	// Start describing instances
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

	log.Debugf("Retrieved %d ec2 instances", len(instances))
	return instances, nil
}

// Retrieve will get all the service instance calling multiple AWS APIs
func (c *AWSRetriever) Retrieve() ([]*types.ServiceInstance, error) {

	// First get the clusters and map them
	clusters, err := c.getClusters()
	if err != nil {
		return nil, err
	}

	for _, cl := range clusters {
		c.cache.SetCluster(cl)
	}

	var wg sync.WaitGroup
	var globErr error
	globErrMsg := "rrror from other goroutine, stopping AWS SD objects gathering: %s"

	// For each cluster get all the required objects from AWS
	wg.Add(len(clusters))
	for _, cluster := range clusters {
		go func(cluster ecs.Cluster) {
			defer wg.Done()

			// Get cluster container instances
			cInstances, err := c.getContainerInstances(&cluster)
			if err != nil {
				globErr = err
				return
			}

			ec2Ids := []*string{}
			for _, ci := range cInstances {
				c.cache.setContainerInstance(ci)
				ec2Ids = append(ec2Ids, ci.Ec2InstanceId)
			}

			// Check if someone errored before continuing
			if globErr != nil {
				log.Debugf(globErrMsg, globErr)
				return
			}

			// Get cluster ec2 instances
			instances, err := c.getInstances(ec2Ids)
			if err != nil {
				globErr = err
				return
			}

			for _, i := range instances {
				c.cache.setInstance(i)
			}

			// Check if someone errored before continuing
			if globErr != nil {
				log.Debugf(globErrMsg, globErr)
				return
			}

			// Get cluster tasks
			ts, err := c.getTasks(&cluster)
			if err != nil {
				globErr = err
				return
			}

			tDefIDs := []*string{}
			for _, t := range ts {
				c.cache.setTask(t)
				tDefIDs = append(tDefIDs, t.TaskDefinitionArn)
			}

			// Check if someone errored before continuing
			if globErr != nil {
				log.Debugf(globErrMsg, globErr)
				return
			}

			// Get cluster services
			srvs, err := c.getServices(&cluster)
			if err != nil {
				globErr = err
				return
			}

			for _, s := range srvs {
				c.cache.setService(s)
			}

			// Check if someone errored before continuing
			if globErr != nil {
				log.Debugf(globErrMsg, globErr)
				return
			}

			// Get task definitions
			tds, err := c.getTaskDefinitions(tDefIDs, true)
			if err != nil {
				globErr = err
				return
			}

			for _, t := range tds {
				c.cache.setTaskDefinition(t)
			}

		}(*cluster)
	}

	wg.Wait()

	// Check if we errored before
	if globErr != nil {
		log.Debugf(globErrMsg, globErr)
		return nil, globErr
	}

	// Create the targets
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
			return nil, fmt.Errorf("zervice not available: %s", tdID)
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

	return sInstances, globErr
}
