package client

import (
	"github.com/prometheus/prometheus/discovery/ecs/types"
)

// Retriever interface will be the way of retrieving the stuff wanted
type Retriever interface {
	// Retrieve will retrieve all the service instances
	Retrieve() ([]*types.ServiceInstance, error)
}
