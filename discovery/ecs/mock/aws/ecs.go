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

package aws

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/prometheus/prometheus/discovery/ecs/mock/aws/sdk"
	"github.com/stretchr/testify/mock"
)

// MockECSListClusters mocks listing cluster ECS API call.
func MockECSListClusters(t *testing.T, m *sdk.ECSAPI, wantError bool, ids ...string) {
	var err error
	if wantError {
		err = errors.New("ListClusters wrong!")
	}
	cIds := []*string{}
	for _, id := range ids {
		tID := id
		cIds = append(cIds, &tID)
	}
	result := &ecs.ListClustersOutput{
		ClusterArns: cIds,
	}
	m.On("ListClusters", mock.AnythingOfType("*ecs.ListClustersInput")).Return(result, err)
}

// MockECSDescribeClusters mocks the description of clusters ECS API call.
func MockECSDescribeClusters(t *testing.T, m *sdk.ECSAPI, wantError bool, clusters ...*ecs.Cluster) {
	var err error
	if wantError {
		err = errors.New("DescribeClusters wrong!")
	}
	result := &ecs.DescribeClustersOutput{
		Clusters: clusters,
	}
	m.On("DescribeClusters", mock.AnythingOfType("*ecs.DescribeClustersInput")).Return(result, err)
}

// MockECSListContainerInstances mocks listing container instances ECS API call.
func MockECSListContainerInstances(t *testing.T, m *sdk.ECSAPI, wantError bool, ids ...string) {
	var err error
	if wantError {
		err = errors.New("ListContainerInstances wrong!")
	}
	ciIds := []*string{}
	for _, id := range ids {
		tID := id
		ciIds = append(ciIds, &tID)
	}
	result := &ecs.ListContainerInstancesOutput{
		ContainerInstanceArns: ciIds,
	}
	m.On("ListContainerInstances", mock.AnythingOfType("*ecs.ListContainerInstancesInput")).Run(func(args mock.Arguments) {
		i := args.Get(0).(*ecs.ListContainerInstancesInput)
		if i.Cluster == nil || aws.StringValue(i.Cluster) == "" {
			t.Errorf("Wrong api call, needs cluster ARN")
		}
	}).Return(result, err)
}

// MockECSDescribeContainerInstances mocks the description of container instances ECS API call.
func MockECSDescribeContainerInstances(t *testing.T, m *sdk.ECSAPI, wantError bool, cis ...*ecs.ContainerInstance) {
	var err error
	if wantError {
		err = errors.New("DescribeContainerInstances wrong!")
	}
	result := &ecs.DescribeContainerInstancesOutput{
		ContainerInstances: cis,
	}
	m.On("DescribeContainerInstances", mock.AnythingOfType("*ecs.DescribeContainerInstancesInput")).Run(func(args mock.Arguments) {
		i := args.Get(0).(*ecs.DescribeContainerInstancesInput)
		if i.Cluster == nil || aws.StringValue(i.Cluster) == "" {
			t.Errorf("Wrong api call, needs cluster ARN")
		}
		if len(i.ContainerInstances) == 0 {
			t.Errorf("Wrong api call, needs at least 1 container instance ARN")
		}
	}).Return(result, err)
}

// MockECSListTasks mocks listing tasks ECS API call.
func MockECSListTasks(t *testing.T, m *sdk.ECSAPI, wantError bool, ids ...string) {
	var err error
	if wantError {
		err = errors.New("ListTasks wrong!")
	}
	tIds := []*string{}
	for _, id := range ids {
		tID := id
		tIds = append(tIds, &tID)
	}
	result := &ecs.ListTasksOutput{
		TaskArns: tIds,
	}
	m.On("ListTasks", mock.AnythingOfType("*ecs.ListTasksInput")).Run(func(args mock.Arguments) {
		i := args.Get(0).(*ecs.ListTasksInput)
		if i.Cluster == nil || aws.StringValue(i.Cluster) == "" {
			t.Errorf("Wrong api call, needs cluster ARN")
		}
	}).Return(result, err)
}

// MockECSDescribeTasks mocks the description of tasks ECS API call.
func MockECSDescribeTasks(t *testing.T, m *sdk.ECSAPI, wantError bool, ts ...*ecs.Task) {
	var err error
	if wantError {
		err = errors.New("DescribeTasks wrong!")
	}
	result := &ecs.DescribeTasksOutput{
		Tasks: ts,
	}
	m.On("DescribeTasks", mock.AnythingOfType("*ecs.DescribeTasksInput")).Run(func(args mock.Arguments) {
		i := args.Get(0).(*ecs.DescribeTasksInput)
		if i.Cluster == nil || aws.StringValue(i.Cluster) == "" {
			t.Errorf("Wrong api call, needs cluster ARN")
		}
		if len(i.Tasks) == 0 {
			t.Errorf("Wrong api call, needs at least 1 task ARN")
		}
	}).Return(result, err)
}

// MockECSListServices mocks listing services ECS API call.
func MockECSListServices(t *testing.T, m *sdk.ECSAPI, wantError bool, ids ...string) {
	var err error
	if wantError {
		err = errors.New("ListServices wrong!")
	}
	sIds := []*string{}
	for _, id := range ids {
		sID := id
		sIds = append(sIds, &sID)
	}
	result := &ecs.ListServicesOutput{
		ServiceArns: sIds,
	}
	m.On("ListServices", mock.AnythingOfType("*ecs.ListServicesInput")).Run(func(args mock.Arguments) {
		i := args.Get(0).(*ecs.ListServicesInput)
		if i.Cluster == nil || aws.StringValue(i.Cluster) == "" {
			t.Errorf("Wrong api call, needs cluster ARN")
		}
	}).Return(result, err)
}

// MockECSDescribeServices mocks the description of services ECS API call.
func MockECSDescribeServices(t *testing.T, m *sdk.ECSAPI, wantError bool, ss ...*ecs.Service) {
	var err error
	if wantError {
		err = errors.New("DescribeServices wrong!")
	}
	result := &ecs.DescribeServicesOutput{
		Services: ss,
	}
	m.On("DescribeServices", mock.AnythingOfType("*ecs.DescribeServicesInput")).Run(func(args mock.Arguments) {
		i := args.Get(0).(*ecs.DescribeServicesInput)
		if i.Cluster == nil || aws.StringValue(i.Cluster) == "" {
			t.Errorf("Wrong api call, needs cluster ARN")
		}
		if len(i.Services) == 0 {
			t.Errorf("Wrong api call, needs at least 1 service ARN")
		}
	}).Return(result, err)
}

// MockECSDescribeTaskDefinition mocks the description of task definition ECS API call.
func MockECSDescribeTaskDefinition(t *testing.T, m *sdk.ECSAPI, wantErrorOn int, tds ...*ecs.TaskDefinition) {
	for i, td := range tds {
		var err error
		// If want error is 0 then means disabled.
		if wantErrorOn-1 == i {
			err = fmt.Errorf("DescribeTaskDefinition on call %d wrong!", i)
		}

		cpyTd := *td
		result := &ecs.DescribeTaskDefinitionOutput{
			TaskDefinition: &cpyTd,
		}
		// Mock the call.
		m.On("DescribeTaskDefinition", mock.AnythingOfType("*ecs.DescribeTaskDefinitionInput")).Once().Run(func(args mock.Arguments) {
			in := args.Get(0).(*ecs.DescribeTaskDefinitionInput)
			if in.TaskDefinition == nil {
				t.Errorf("Wrong api call, task definition ARN is required")
			}
		}).Return(result, err)

		// If we arror then there no more calls are being made.
		if err != nil {
			break
		}
	}
}
