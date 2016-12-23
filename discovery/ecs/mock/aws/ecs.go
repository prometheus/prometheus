package aws

import (
	"errors"
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
