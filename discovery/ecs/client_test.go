package ecs

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	awsecs "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/golang/mock/gomock"

	awsmock "github.com/prometheus/prometheus/discovery/ecs/mock/aws"
	"github.com/prometheus/prometheus/discovery/ecs/mock/aws/sdk"
)

func TestGetClusters(t *testing.T) {
	tests := []struct {
		clusters  []*awsecs.Cluster
		errorList bool
		errorDesc bool
		wantError bool
	}{
		{
			clusters: []*awsecs.Cluster{
				&awsecs.Cluster{ClusterArn: aws.String("c1")},
			},
		},
		{
			clusters: []*awsecs.Cluster{
				&awsecs.Cluster{ClusterArn: aws.String("c1")},
			},
			errorList: true,
			wantError: true,
		},
		{
			clusters: []*awsecs.Cluster{
				&awsecs.Cluster{ClusterArn: aws.String("c1")},
			},
			errorDesc: true,
			wantError: true,
		},
		{
			clusters: []*awsecs.Cluster{
				&awsecs.Cluster{ClusterArn: aws.String("c1")},
				&awsecs.Cluster{ClusterArn: aws.String("c2")},
				&awsecs.Cluster{ClusterArn: aws.String("c3")},
				&awsecs.Cluster{ClusterArn: aws.String("c4")},
			},
		},
	}

	for _, test := range tests {

		cIDs := make([]string, len(test.clusters))
		for i, cs := range test.clusters {
			cIDs[i] = aws.StringValue(cs.ClusterArn)
		}

		// Mock
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockECS := sdk.NewMockECSAPI(ctrl)
		awsmock.MockECSListClusters(t, mockECS, test.errorList, cIDs...)
		awsmock.MockECSDescribeClusters(t, mockECS, test.errorDesc, test.clusters...)

		r := &awsRetriever{
			client: mockECS,
		}

		res, err := r.getClusters()

		if !test.wantError {
			if len(res) != len(test.clusters) {
				t.Errorf("- %+v\n -The length of the retrieved clusters differ, want: %d; got: %d", test, len(test.clusters), len(res))
			}

			for i, got := range res {
				want := test.clusters[i]
				if !reflect.DeepEqual(want, got) {
					t.Errorf("\n- %v\n-  Received clusters from API is wrong, want: %v; got: %v", test, want, got)
				}
			}

		} else {
			if err == nil {
				t.Errorf("- %+v\n -Getting clusters shoud error, it didn't", test)
			}
		}
	}
}

func TestGetContainerInstances(t *testing.T) {
	tests := []struct {
		cInstances []*awsecs.ContainerInstance
		errorList  bool
		errorDesc  bool
		wantError  bool
	}{
		{
			cInstances: []*awsecs.ContainerInstance{
				&awsecs.ContainerInstance{ContainerInstanceArn: aws.String("ci1")},
			},
		},
		{
			cInstances: []*awsecs.ContainerInstance{
				&awsecs.ContainerInstance{ContainerInstanceArn: aws.String("ci1")},
			},
			errorList: true,
			wantError: true,
		},
		{
			cInstances: []*awsecs.ContainerInstance{
				&awsecs.ContainerInstance{ContainerInstanceArn: aws.String("ci1")},
			},
			errorDesc: true,
			wantError: true,
		},
		{
			cInstances: []*awsecs.ContainerInstance{
				&awsecs.ContainerInstance{ContainerInstanceArn: aws.String("ci1")},
				&awsecs.ContainerInstance{ContainerInstanceArn: aws.String("ci2")},
				&awsecs.ContainerInstance{ContainerInstanceArn: aws.String("ci3")},
				&awsecs.ContainerInstance{ContainerInstanceArn: aws.String("ci4")},
				&awsecs.ContainerInstance{ContainerInstanceArn: aws.String("ci5")},
				&awsecs.ContainerInstance{ContainerInstanceArn: aws.String("ci6")},
			},
		},
	}

	for _, test := range tests {

		ciIDs := make([]string, len(test.cInstances))
		for i, ci := range test.cInstances {
			ciIDs[i] = aws.StringValue(ci.ContainerInstanceArn)
		}

		// Mock
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockECS := sdk.NewMockECSAPI(ctrl)
		awsmock.MockECSListContainerInstances(t, mockECS, test.errorList, ciIDs...)
		awsmock.MockECSDescribeContainerInstances(t, mockECS, test.errorDesc, test.cInstances...)

		r := &awsRetriever{
			client: mockECS,
		}

		res, err := r.getContainerInstances(&awsecs.Cluster{ClusterArn: aws.String("c1")})

		if !test.wantError {
			if len(res) != len(test.cInstances) {
				t.Errorf("- %+v\n -The length of the retrieved container instances differ, want: %d; got: %d", test, len(test.cInstances), len(res))
			}

			for i, got := range res {
				want := test.cInstances[i]
				if !reflect.DeepEqual(want, got) {
					t.Errorf("\n- %v\n-  Received container instances from API is wrong, want: %v; got: %v", test, want, got)
				}
			}

		} else {
			if err == nil {
				t.Errorf("- %+v\n -Getting Container instances shoud error, it didn't", test)
			}
		}
	}
}

func TestGetTasks(t *testing.T) {
	tests := []struct {
		tasks     []*awsecs.Task
		errorList bool
		errorDesc bool
		wantError bool
	}{
		{
			tasks: []*awsecs.Task{
				&awsecs.Task{TaskArn: aws.String("t1")},
			},
		},
		{
			tasks: []*awsecs.Task{
				&awsecs.Task{TaskArn: aws.String("t1")},
			},
			errorList: true,
			wantError: true,
		},
		{
			tasks: []*awsecs.Task{
				&awsecs.Task{TaskArn: aws.String("t1")},
			},
			errorDesc: true,
			wantError: true,
		},
		{
			tasks: []*awsecs.Task{
				&awsecs.Task{TaskArn: aws.String("t1")},
				&awsecs.Task{TaskArn: aws.String("t2")},
				&awsecs.Task{TaskArn: aws.String("t3")},
				&awsecs.Task{TaskArn: aws.String("t4")},
				&awsecs.Task{TaskArn: aws.String("t5")},
				&awsecs.Task{TaskArn: aws.String("t6")},
				&awsecs.Task{TaskArn: aws.String("t7")},
			},
		},
	}

	for _, test := range tests {

		tIDs := make([]string, len(test.tasks))
		for i, t := range test.tasks {
			tIDs[i] = aws.StringValue(t.TaskArn)
		}

		// Mock
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockECS := sdk.NewMockECSAPI(ctrl)
		awsmock.MockECSListTasks(t, mockECS, test.errorList, tIDs...)
		awsmock.MockECSDescribeTasks(t, mockECS, test.errorDesc, test.tasks...)

		r := &awsRetriever{
			client: mockECS,
		}

		res, err := r.getTasks(&awsecs.Cluster{ClusterArn: aws.String("c1")})

		if !test.wantError {
			if len(res) != len(test.tasks) {
				t.Errorf("- %+v\n -The length of the retrieved tasks differ, want: %d; got: %d", test, len(test.tasks), len(res))
			}

			for i, got := range res {
				want := test.tasks[i]
				if !reflect.DeepEqual(want, got) {
					t.Errorf("\n- %v\n-  Received tasks from API is wrong, want: %v; got: %v", test, want, got)
				}
			}

		} else {
			if err == nil {
				t.Errorf("- %+v\n -Getting tasks shoud error, it didn't", test)
			}
		}
	}
}
