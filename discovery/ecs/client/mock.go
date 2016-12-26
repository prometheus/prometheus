package client

import (
	"fmt"

	"github.com/prometheus/prometheus/discovery/ecs/types"
)

// MockRetriever is a Mocked client
type MockRetriever struct {
	Instances   []*types.ServiceInstance
	ShouldError bool
}

// NewMultipleMockRetriever creates a mock client object with the desired number of service intances
func NewMultipleMockRetriever(quantity int) *MockRetriever {
	sis := []*types.ServiceInstance{}

	for i := 0; i < quantity; i++ {
		s := &types.ServiceInstance{
			Addr:    fmt.Sprintf("127.0.0.0:%d", 8000+i),
			Cluster: fmt.Sprintf("cluster%d", i%3),
			Service: fmt.Sprintf("service%d", i%10),
		}
		sis = append(sis, s)
	}
	return &MockRetriever{
		Instances: sis,
	}
}

func (c *MockRetriever) Retrieve() ([]*types.ServiceInstance, error) {
	if c.ShouldError {
		return nil, fmt.Errorf("Error wanted")
	}
	return c.Instances, nil
}
