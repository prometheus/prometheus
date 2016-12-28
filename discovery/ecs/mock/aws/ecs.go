package aws

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/golang/mock/gomock"
	"github.com/prometheus/common/log"
	"github.com/prometheus/prometheus/discovery/ecs/mock/aws/sdk"
)

// MockECSListClusters mocks listing cluster ECS API call
func MockECSListClusters(t *testing.T, mockMatcher *sdk.MockECSAPI, wantError bool, ids ...string) {
	log.Warnf("Mocking AWS iface: ListClusters")
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
	mockMatcher.EXPECT().ListClusters(gomock.Any()).Do(func(input interface{}) {
	}).AnyTimes().Return(result, err)
}

// MockECSDescribeClusters mocks the description of clusters ECS API call
func MockECSDescribeClusters(t *testing.T, mockMatcher *sdk.MockECSAPI, wantError bool, clusters ...*ecs.Cluster) {
	log.Warnf("Mocking AWS iface: DescribeClusters")
	var err error
	if wantError {
		err = errors.New("DescribeClusters wrong!")
	}
	result := &ecs.DescribeClustersOutput{
		Clusters: clusters,
	}
	mockMatcher.EXPECT().DescribeClusters(gomock.Any()).Do(func(input interface{}) {
	}).AnyTimes().Return(result, err)
}

// MockECSListContainerInstances mocks listing container instances ECS API call
func MockECSListContainerInstances(t *testing.T, mockMatcher *sdk.MockECSAPI, wantError bool, ids ...string) {
	log.Warnf("Mocking AWS iface: ListContainerInstances")
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
	mockMatcher.EXPECT().ListContainerInstances(gomock.Any()).Do(func(input interface{}) {
		i := input.(*ecs.ListContainerInstancesInput)
		if i.Cluster == nil || aws.StringValue(i.Cluster) == "" {
			t.Errorf("Wrong api call, needs cluster ARN")
		}
	}).AnyTimes().Return(result, err)
}

// MockECSDescribeContainerInstances mocks the description of container instances ECS API call
func MockECSDescribeContainerInstances(t *testing.T, mockMatcher *sdk.MockECSAPI, wantError bool, cis ...*ecs.ContainerInstance) {
	log.Warnf("Mocking AWS iface: DescribeContainerInstances")
	var err error
	if wantError {
		err = errors.New("DescribeContainerInstances wrong!")
	}
	result := &ecs.DescribeContainerInstancesOutput{
		ContainerInstances: cis,
	}
	mockMatcher.EXPECT().DescribeContainerInstances(gomock.Any()).Do(func(input interface{}) {
		i := input.(*ecs.DescribeContainerInstancesInput)
		if i.Cluster == nil || aws.StringValue(i.Cluster) == "" {
			t.Errorf("Wrong api call, needs cluster ARN")
		}
		if len(i.ContainerInstances) == 0 {
			t.Errorf("Wrong api call, needs at least 1 container instance ARN")
		}
	}).AnyTimes().Return(result, err)
}

// MockECSListTasks mocks listing tasks ECS API call
func MockECSListTasks(t *testing.T, mockMatcher *sdk.MockECSAPI, wantError bool, ids ...string) {
	log.Warnf("Mocking AWS iface: ListTasks")
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
	mockMatcher.EXPECT().ListTasks(gomock.Any()).Do(func(input interface{}) {
		i := input.(*ecs.ListTasksInput)
		if i.Cluster == nil || aws.StringValue(i.Cluster) == "" {
			t.Errorf("Wrong api call, needs cluster ARN")
		}
	}).AnyTimes().Return(result, err)
}

// MockECSDescribeTasks mocks the description of tasks ECS API call
func MockECSDescribeTasks(t *testing.T, mockMatcher *sdk.MockECSAPI, wantError bool, ts ...*ecs.Task) {
	log.Warnf("Mocking AWS iface: DescribeTasks")
	var err error
	if wantError {
		err = errors.New("DescribeTasks wrong!")
	}
	result := &ecs.DescribeTasksOutput{
		Tasks: ts,
	}
	mockMatcher.EXPECT().DescribeTasks(gomock.Any()).Do(func(input interface{}) {
		i := input.(*ecs.DescribeTasksInput)
		if i.Cluster == nil || aws.StringValue(i.Cluster) == "" {
			t.Errorf("Wrong api call, needs cluster ARN")
		}
		if len(i.Tasks) == 0 {
			t.Errorf("Wrong api call, needs at least 1 task ARN")
		}
	}).AnyTimes().Return(result, err)
}

// MockECSListServices mocks listing services ECS API call
func MockECSListServices(t *testing.T, mockMatcher *sdk.MockECSAPI, wantError bool, ids ...string) {
	log.Warnf("Mocking AWS iface: ListServices")
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
	mockMatcher.EXPECT().ListServices(gomock.Any()).Do(func(input interface{}) {
		i := input.(*ecs.ListServicesInput)
		if i.Cluster == nil || aws.StringValue(i.Cluster) == "" {
			t.Errorf("Wrong api call, needs cluster ARN")
		}
	}).AnyTimes().Return(result, err)
}

// MockECSDescribeServices mocks the description of services ECS API call
func MockECSDescribeServices(t *testing.T, mockMatcher *sdk.MockECSAPI, wantError bool, ss ...*ecs.Service) {
	log.Warnf("Mocking AWS iface: DescribeServices")
	var err error
	if wantError {
		err = errors.New("DescribeServices wrong!")
	}
	result := &ecs.DescribeServicesOutput{
		Services: ss,
	}
	mockMatcher.EXPECT().DescribeServices(gomock.Any()).Do(func(input interface{}) {
		i := input.(*ecs.DescribeServicesInput)
		if i.Cluster == nil || aws.StringValue(i.Cluster) == "" {
			t.Errorf("Wrong api call, needs cluster ARN")
		}
		if len(i.Services) == 0 {
			t.Errorf("Wrong api call, needs at least 1 service ARN")
		}
	}).AnyTimes().Return(result, err)
}

// MockECSDescribeTaskDefinition mocks the description of task definition ECS API call
func MockECSDescribeTaskDefinition(t *testing.T, mockMatcher *sdk.MockECSAPI, wantErrorOn int, tds ...*ecs.TaskDefinition) {
	log.Warnf("Mocking AWS iface: DescribeTaskDefinition")

	calls := make([]*gomock.Call, len(tds))
	for i, td := range tds {
		var err error
		// if want error is 0 then means disabled
		if wantErrorOn-1 == i {
			err = fmt.Errorf("DescribeTaskDefinition on call %d wrong!", i)
		}

		cpyTd := *td
		result := &ecs.DescribeTaskDefinitionOutput{
			TaskDefinition: &cpyTd,
		}
		call := mockMatcher.EXPECT().DescribeTaskDefinition(gomock.Any()).Do(func(input interface{}) {
			i := input.(*ecs.DescribeTaskDefinitionInput)
			if i.TaskDefinition == nil {
				t.Errorf("Wrong api call, task definition ARN is required")
			}
		}).Return(result, err)

		calls[i] = call
	}

	gomock.InOrder(calls...)
}
