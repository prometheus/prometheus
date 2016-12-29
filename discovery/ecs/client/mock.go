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

// Retrieve will return the mocked service instances
func (c *MockRetriever) Retrieve() ([]*types.ServiceInstance, error) {
	if c.ShouldError {
		return nil, fmt.Errorf("Error wanted")
	}
	return c.Instances, nil
}
