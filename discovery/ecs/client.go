package ecs

import (
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	awsecs "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
	"github.com/prometheus/common/log"
)

// Generate ECS API mocks running go generate
//go:generate mockgen -source ../../vendor/github.com/aws/aws-sdk-go/service/ecs/ecsiface/interface.go -package sdk -destination ./mock/aws/sdk/ecsiface_mock.go

const (
	maxAPIRes = 100
)

// client interface will be the way of retrieving the stuff wanted
type retriever interface {
	// retrieve will retrieve all the service instances
	retrieve() ([]*serviceInstance, error)
}

// mockRetriever is a Mocked client
type mockRetriever struct {
	instances   []*serviceInstance
	shouldError bool
}

func newMultipleMockRetriever(quantity int) *mockRetriever {
	sis := []*serviceInstance{}

	for i := 0; i < quantity; i++ {
		s := &serviceInstance{
			addr:    fmt.Sprintf("127.0.0.0:%d", 8000+i),
			cluster: fmt.Sprintf("cluster%d", i%3),
			service: fmt.Sprintf("service%d", i%10),
		}
		sis = append(sis, s)
	}
	return &mockRetriever{
		instances: sis,
	}
}

func (c *mockRetriever) retrieve() ([]*serviceInstance, error) {
	if c.shouldError {
		return nil, fmt.Errorf("Error wanted")
	}

	return c.instances, nil
}

// awsSrvInstance is a helper that has all the objects required to create a final service instance
type awsSrvInstance struct {
	cluster           *awsecs.Cluster
	task              *awsecs.Task
	containerInstance *awsecs.ContainerInstance
	container         *awsecs.Container
}

// awsRetriever is the wrapper around AWS client
type awsRetriever struct {
	client ecsiface.ECSAPI
}

func (c *awsRetriever) getClusters() ([]*awsecs.Cluster, error) {
	clusters := []*awsecs.Cluster{}

	params := &awsecs.ListClustersInput{
		MaxResults: aws.Int64(maxAPIRes),
	}
	log.Debugf("Getting clusters")
	// Start listing clusters
	for {
		resp, err := c.client.ListClusters(params)
		if err != nil {
			return nil, err
		}

		cArns := []*string{}
		for _, cl := range resp.ClusterArns {
			cArns = append(cArns, cl)
		}

		// Describe clusters
		params2 := &awsecs.DescribeClustersInput{
			Clusters: cArns,
		}

		resp2, err := c.client.DescribeClusters(params2)
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

func (c *awsRetriever) getContainerInstances(cluster *awsecs.Cluster) ([]*awsecs.ContainerInstance, error) {
	contInsts := []*awsecs.ContainerInstance{}

	params := &awsecs.ListContainerInstancesInput{
		Cluster:    cluster.ClusterArn,
		MaxResults: aws.Int64(maxAPIRes),
	}
	log.Debugf("Getting cluster %s container instances", aws.StringValue(cluster.ClusterName))
	// Start listing container instances
	for {
		resp, err := c.client.ListContainerInstances(params)
		if err != nil {
			return nil, err
		}

		ciArns := []*string{}
		for _, ci := range resp.ContainerInstanceArns {
			ciArns = append(ciArns, ci)
		}

		// Describe container instances
		params2 := &awsecs.DescribeContainerInstancesInput{
			Cluster:            cluster.ClusterArn,
			ContainerInstances: ciArns,
		}

		resp2, err := c.client.DescribeContainerInstances(params2)
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

func (c *awsRetriever) retrieve() ([]*serviceInstance, error) {
	// First get the clusters and map them
	clusters, err := c.getClusters()
	if err != nil {
		return nil, err
	}
	clsIndex := map[string]*awsecs.Cluster{}
	for _, cl := range clusters {
		clsIndex[aws.StringValue(cl.ClusterArn)] = cl
	}

	// Get all the container instances and tasks on the cluster and map them
	wg := &sync.WaitGroup{}
	cisIndex := map[string]*awsecs.ContainerInstance{}
	ciMutex := sync.Mutex{}
	var getterErr error

	// For each cluster get its container instances and tasks
	for _, v := range clsIndex {
		// use argument by value
		go func(cluster awsecs.Cluster) {
			wg.Add(1)
			defer wg.Done()

			// Get cluster container instance
			cInstances, err := c.getContainerInstances(&cluster)
			if err != nil {
				getterErr = err
				return
			}

			ciMutex.Lock()
			for _, ci := range cInstances {
				cisIndex[aws.StringValue(ci.ContainerInstanceArn)] = ci
			}
			ciMutex.Unlock()

			// Check if someone errored before continuing
			if getterErr != nil {
				return
			}

			// Get cluster tasks

		}(*v)
	}

	wg.Wait()

	return nil, nil
}
