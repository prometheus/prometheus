package ecs

import (
	"fmt"

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
	// Start liting clusters
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

func (c *awsRetriever) retrieve() ([]*serviceInstance, error) {
	return nil, nil
}
