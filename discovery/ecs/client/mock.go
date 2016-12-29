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
