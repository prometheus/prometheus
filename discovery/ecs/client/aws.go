package client

import (
	"fmt"
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
					Cluster:   aws.StringValue(a.cluster.ClusterName),
					Addr:      addr,
					Service:   aws.StringValue(a.service.ServiceName),
					Container: aws.StringValue(c.Name),
					Tags:      tags,
					Labels:    labels,
					Image:     aws.StringValue(cDef.Image),
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
			tasks = append(tasks, t)
		}

		if resp.NextToken == nil || aws.StringValue(resp.NextToken) == "" {
			break
		}
		params.NextToken = resp.NextToken

	}
	log.Debugf("Retrieved %d tasks on cluster %s", len(tasks), aws.StringValue(cluster.ClusterName))
	return tasks, nil
}

func (c *AWSRetriever) getInstances(ec2Ids []*string) ([]*ec2.Instance, error) {

	instances := []*ec2.Instance{}
	log.Debugf("Getting ec2 instances")

	params := &ec2.DescribeInstancesInput{
		InstanceIds: ec2Ids,
	}

	// Start listing tasks
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
	clsIndex := map[string]*ecs.Cluster{}
	for _, cl := range clusters {
		clsIndex[aws.StringValue(cl.ClusterArn)] = cl
	}

	// Get all the container instances and tasks on the cluster and map them
	cisIndex := map[string]*ecs.ContainerInstance{} // container instance cache
	ciMutex := sync.Mutex{}
	tsIndex := map[string]*ecs.Task{} // task cache
	tMutex := sync.Mutex{}
	iIndex := map[string]*ec2.Instance{} // instance cache
	iMutex := sync.Mutex{}
	var wg sync.WaitGroup
	var getterErr error

	// For each cluster get its container instances and tasks
	wg.Add(len(clsIndex))
	for _, v := range clsIndex {
		// use argument by value
		go func(cluster ecs.Cluster) {
			defer wg.Done()

			// Get cluster container instance
			cInstances, err := c.getContainerInstances(&cluster)
			if err != nil {
				getterErr = err
				return
			}

			ciMutex.Lock()
			ec2Ids := []*string{}
			for _, ci := range cInstances {
				cisIndex[aws.StringValue(ci.ContainerInstanceArn)] = ci
				ec2Ids = append(ec2Ids, ci.Ec2InstanceId)
			}
			ciMutex.Unlock()

			// Check if someone errored before continuing
			if getterErr != nil {
				log.Debugf("Error from other goroutine, stopping gathering: %s", getterErr)
				return
			}

			// Get cluster ec2 instances
			instances, err := c.getInstances(ec2Ids)
			if err != nil {
				getterErr = err
				return
			}

			iMutex.Lock()
			for _, i := range instances {
				iIndex[aws.StringValue(i.InstanceId)] = i
			}
			iMutex.Unlock()

			// Check if someone errored before continuing
			if getterErr != nil {
				log.Debugf("Error from other goroutine, stopping gathering: %s", getterErr)
				return
			}

			// Get cluster tasks
			ts, err := c.getTasks(&cluster)
			if err != nil {
				getterErr = err
				return
			}

			tMutex.Lock()
			for _, t := range ts {
				tsIndex[aws.StringValue(t.TaskArn)] = t
			}
			tMutex.Unlock()
		}(*v)
	}

	wg.Wait()

	sInstances := []*types.ServiceInstance{}
	for _, v := range tsIndex {
		clID := aws.StringValue(v.ClusterArn)
		cl, ok := clsIndex[clID]
		if !ok {
			return nil, fmt.Errorf("cluster not available: %s", clID)
		}

		ciID := aws.StringValue(v.ContainerInstanceArn)
		ci, ok := cisIndex[ciID]
		if !ok {
			return nil, fmt.Errorf("container not available: %s", ciID)
		}

		iID := aws.StringValue(ci.Ec2InstanceId)
		ec2I, ok := iIndex[iID]
		if !ok {
			return nil, fmt.Errorf("EC2 instance not available: %s", iID)
		}

		t := &ecsTargetTask{
			cluster:  cl,
			task:     v,
			instance: ec2I,
		}

		sis := t.createServiceInstances()
		for _, i := range sis {
			sInstances = append(sInstances, i)
		}
	}

	return sInstances, getterErr
}
