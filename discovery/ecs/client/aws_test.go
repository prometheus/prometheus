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
	"context"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"

	awsmock "github.com/prometheus/prometheus/discovery/ecs/mock/aws"
	"github.com/prometheus/prometheus/discovery/ecs/mock/aws/sdk"
	"github.com/prometheus/prometheus/discovery/ecs/types"
)

func TestCreateServiceInstances(t *testing.T) {
	// Cluster
	cls := &ecs.Cluster{ClusterName: aws.String("prod-cluster-infra")}

	// Task
	tsk := &ecs.Task{
		Containers: []*ecs.Container{
			&ecs.Container{
				Name: aws.String("myService"),
				NetworkBindings: []*ecs.NetworkBinding{
					&ecs.NetworkBinding{
						HostPort:      aws.Int64(36112),
						ContainerPort: aws.Int64(8080),
						Protocol:      aws.String("tcp"),
					},
					&ecs.NetworkBinding{
						HostPort:      aws.Int64(24567),
						ContainerPort: aws.Int64(1568),
						Protocol:      aws.String("udp"),
					},
				},
			},
			&ecs.Container{
				Name: aws.String("nginx"),
				NetworkBindings: []*ecs.NetworkBinding{&ecs.NetworkBinding{
					HostPort:      aws.Int64(30987),
					ContainerPort: aws.Int64(8081),
					Protocol:      aws.String("tcp"),
				}},
			},
			&ecs.Container{
				Name:            aws.String("worker"),
				NetworkBindings: []*ecs.NetworkBinding{},
			},
		},
	}

	// Task definition
	tDef := &ecs.TaskDefinition{
		ContainerDefinitions: []*ecs.ContainerDefinition{
			&ecs.ContainerDefinition{
				Name:  aws.String("myService"),
				Image: aws.String("000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e"),
				DockerLabels: map[string]*string{
					"monitor": aws.String("true"),
					"kind":    aws.String("main"),
				},
			},
			&ecs.ContainerDefinition{
				Name:  aws.String("nginx"),
				Image: aws.String("nginx:latest"),
				DockerLabels: map[string]*string{
					"kind": aws.String("front-http"),
				},
			},
			&ecs.ContainerDefinition{
				Name:  aws.String("worker"),
				Image: aws.String("000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e"),
				DockerLabels: map[string]*string{
					"kind": aws.String("worker"),
				},
			},
		},
	}

	// EC2 instance
	intc := &ec2.Instance{
		InstanceId:       aws.String("i-0ef582cb6c1bbbdd3"),
		PrivateIpAddress: aws.String("10.0.250.65"),
		Tags: []*ec2.Tag{
			&ec2.Tag{Key: aws.String("env"), Value: aws.String("prod")},
			&ec2.Tag{Key: aws.String("kind"), Value: aws.String("ecs")},
			&ec2.Tag{Key: aws.String("cluster"), Value: aws.String("infra")},
		},
	}

	// Service
	srv := &ecs.Service{ServiceName: aws.String("myService")}

	want := []*types.ServiceInstance{
		&types.ServiceInstance{
			Cluster:            "prod-cluster-infra",
			Service:            "myService",
			Addr:               "10.0.250.65:36112",
			Container:          "myService",
			ContainerPort:      "8080",
			ContainerPortProto: "tcp",
			Image:              "000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e",
			Labels:             map[string]string{"monitor": "true", "kind": "main"},
			Tags:               map[string]string{"env": "prod", "kind": "ecs", "cluster": "infra"},
		},
		&types.ServiceInstance{
			Cluster:            "prod-cluster-infra",
			Service:            "myService",
			Addr:               "10.0.250.65:24567",
			Container:          "myService",
			ContainerPort:      "1568",
			ContainerPortProto: "udp",
			Image:              "000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e",
			Labels:             map[string]string{"monitor": "true", "kind": "main"},
			Tags:               map[string]string{"env": "prod", "kind": "ecs", "cluster": "infra"},
		},
		&types.ServiceInstance{
			Cluster:            "prod-cluster-infra",
			Service:            "myService",
			Addr:               "10.0.250.65:30987",
			Container:          "nginx",
			ContainerPort:      "8081",
			ContainerPortProto: "tcp",
			Image:              "nginx:latest",
			Labels:             map[string]string{"kind": "front-http"},
			Tags:               map[string]string{"env": "prod", "kind": "ecs", "cluster": "infra"},
		},
	}

	// Create our object and test
	eTT := &ecsTargetTask{
		cluster:  cls,
		task:     tsk,
		instance: intc,
		taskDef:  tDef,
		service:  srv,
	}

	res := eTT.createServiceInstances()

	if len(res) != len(want) {
		t.Errorf("The length of the result service instances is wrong, want: %d; got: %d", len(want), len(res))
	}

	for i, got := range res {
		w := want[i]
		if !reflect.DeepEqual(w, got) {
			t.Errorf("- Received service instance taget is wrong want: %+v; got: %+v", w, got)
		}
	}

}

func TestGetClusters(t *testing.T) {
	tests := []struct {
		clusters  []*ecs.Cluster
		errorList bool
		errorDesc bool
		wantError bool
	}{
		{
			clusters: []*ecs.Cluster{
				&ecs.Cluster{ClusterArn: aws.String("c1")},
			},
		},
		{
			clusters: []*ecs.Cluster{
				&ecs.Cluster{ClusterArn: aws.String("c1")},
			},
			errorList: true,
			wantError: true,
		},
		{
			clusters: []*ecs.Cluster{
				&ecs.Cluster{ClusterArn: aws.String("c1")},
			},
			errorDesc: true,
			wantError: true,
		},
		{
			clusters: []*ecs.Cluster{
				&ecs.Cluster{ClusterArn: aws.String("c1")},
				&ecs.Cluster{ClusterArn: aws.String("c2")},
				&ecs.Cluster{ClusterArn: aws.String("c3")},
				&ecs.Cluster{ClusterArn: aws.String("c4")},
			},
		},
	}

	for _, test := range tests {

		cIDs := make([]string, len(test.clusters))
		for i, cs := range test.clusters {
			cIDs[i] = aws.StringValue(cs.ClusterArn)
		}

		// Mock
		mockECS := &sdk.ECSAPI{}
		awsmock.MockECSListClusters(t, mockECS, test.errorList, cIDs...)
		awsmock.MockECSDescribeClusters(t, mockECS, test.errorDesc, test.clusters...)

		r := &AWSRetriever{
			ecsCli: mockECS,
		}

		res, err := r.getClusters(context.TODO())

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
				t.Errorf("- %+v\n -Getting clusters should error, it didn't", test)
			}
		}
	}
}

func TestGetContainerInstances(t *testing.T) {
	tests := []struct {
		cInstances []*ecs.ContainerInstance
		errorList  bool
		errorDesc  bool
		wantError  bool
	}{
		{
			cInstances: []*ecs.ContainerInstance{
				&ecs.ContainerInstance{ContainerInstanceArn: aws.String("ci1")},
			},
		},
		{
			cInstances: []*ecs.ContainerInstance{
				&ecs.ContainerInstance{ContainerInstanceArn: aws.String("ci1")},
			},
			errorList: true,
			wantError: true,
		},
		{
			cInstances: []*ecs.ContainerInstance{
				&ecs.ContainerInstance{ContainerInstanceArn: aws.String("ci1")},
			},
			errorDesc: true,
			wantError: true,
		},
		{
			cInstances: []*ecs.ContainerInstance{
				&ecs.ContainerInstance{ContainerInstanceArn: aws.String("ci1")},
				&ecs.ContainerInstance{ContainerInstanceArn: aws.String("ci2")},
				&ecs.ContainerInstance{ContainerInstanceArn: aws.String("ci3")},
				&ecs.ContainerInstance{ContainerInstanceArn: aws.String("ci4")},
				&ecs.ContainerInstance{ContainerInstanceArn: aws.String("ci5")},
				&ecs.ContainerInstance{ContainerInstanceArn: aws.String("ci6")},
			},
		},
	}

	for _, test := range tests {

		ciIDs := make([]string, len(test.cInstances))
		for i, ci := range test.cInstances {
			ciIDs[i] = aws.StringValue(ci.ContainerInstanceArn)
		}

		// Mock
		mockECS := &sdk.ECSAPI{}
		awsmock.MockECSListContainerInstances(t, mockECS, test.errorList, ciIDs...)
		awsmock.MockECSDescribeContainerInstances(t, mockECS, test.errorDesc, test.cInstances...)

		r := &AWSRetriever{
			ecsCli: mockECS,
		}

		res, err := r.getContainerInstances(context.TODO(), &ecs.Cluster{ClusterArn: aws.String("c1")})

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
				t.Errorf("- %+v\n -Getting Container instances should error, it didn't", test)
			}
		}
	}
}

func TestGetTasks(t *testing.T) {
	tests := []struct {
		tasks     []*ecs.Task
		errorList bool
		errorDesc bool
		wantError bool
	}{
		{
			tasks: []*ecs.Task{
				&ecs.Task{TaskArn: aws.String("t1")},
			},
		},
		{
			tasks: []*ecs.Task{
				&ecs.Task{TaskArn: aws.String("t1")},
			},
			errorList: true,
			wantError: true,
		},
		{
			tasks: []*ecs.Task{
				&ecs.Task{TaskArn: aws.String("t1")},
			},
			errorDesc: true,
			wantError: true,
		},
		{
			tasks: []*ecs.Task{
				&ecs.Task{TaskArn: aws.String("t1")},
				&ecs.Task{TaskArn: aws.String("t2")},
				&ecs.Task{TaskArn: aws.String("t3")},
				&ecs.Task{TaskArn: aws.String("t4")},
				&ecs.Task{TaskArn: aws.String("t5")},
				&ecs.Task{TaskArn: aws.String("t6")},
				&ecs.Task{TaskArn: aws.String("t7")},
			},
		},
	}

	for _, test := range tests {

		tIDs := make([]string, len(test.tasks))
		for i, t := range test.tasks {
			tIDs[i] = aws.StringValue(t.TaskArn)
		}

		// Mock
		mockECS := &sdk.ECSAPI{}
		awsmock.MockECSListTasks(t, mockECS, test.errorList, tIDs...)
		awsmock.MockECSDescribeTasks(t, mockECS, test.errorDesc, test.tasks...)

		r := &AWSRetriever{
			ecsCli: mockECS,
		}

		res, err := r.getTasks(context.TODO(), &ecs.Cluster{ClusterArn: aws.String("c1")})

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
				t.Errorf("- %+v\n -Getting tasks should error, it didn't", test)
			}
		}
	}
}

func TestGetInstances(t *testing.T) {
	tests := []struct {
		instances []*ec2.Instance
		errorDesc bool
		wantError bool
	}{
		{
			instances: []*ec2.Instance{
				&ec2.Instance{InstanceId: aws.String("i1")},
			},
		},
		{
			instances: []*ec2.Instance{
				&ec2.Instance{InstanceId: aws.String("i1")},
			},
			errorDesc: true,
			wantError: true,
		},
		{
			instances: []*ec2.Instance{
				&ec2.Instance{InstanceId: aws.String("i1")},
				&ec2.Instance{InstanceId: aws.String("i2")},
				&ec2.Instance{InstanceId: aws.String("i3")},
				&ec2.Instance{InstanceId: aws.String("i4")},
				&ec2.Instance{InstanceId: aws.String("i5")},
			},
		},
	}

	for _, test := range tests {

		iIDs := make([]*string, len(test.instances))
		for i, it := range test.instances {
			iIDs[i] = it.InstanceId
		}

		// Mock
		mockEC2 := &sdk.EC2API{}
		awsmock.MockEC2DescribeInstances(t, mockEC2, test.errorDesc, test.instances...)

		r := &AWSRetriever{
			ec2Cli: mockEC2,
		}

		res, err := r.getInstances(context.TODO(), iIDs)

		if !test.wantError {
			if len(res) != len(test.instances) {
				t.Errorf("- %+v\n -The length of the retrieved instances differ, want: %d; got: %d", test, len(test.instances), len(res))
			}

			for i, got := range res {
				want := test.instances[i]
				if !reflect.DeepEqual(want, got) {
					t.Errorf("\n- %v\n-  Received instance from API is wrong, want: %v; got: %v", test, want, got)
				}
			}

		} else {
			if err == nil {
				t.Errorf("- %+v\n -Getting instances should error, it didn't", test)
			}
		}
	}
}

func TestGetServices(t *testing.T) {
	tests := []struct {
		services  []*ecs.Service
		errorList bool
		errorDesc bool
		wantError bool
	}{
		{
			services: []*ecs.Service{
				&ecs.Service{
					ServiceName: aws.String("Service1"),
					Deployments: []*ecs.Deployment{
						&ecs.Deployment{
							Status:         aws.String("PRIMARY"),
							TaskDefinition: aws.String("td2"),
						},
						&ecs.Deployment{
							Status:         aws.String("ACTIVE"),
							TaskDefinition: aws.String("td1"),
						},
					},
				},
			},
		},
		{
			services: []*ecs.Service{
				&ecs.Service{
					ServiceName: aws.String("Service1"),
					Deployments: []*ecs.Deployment{
						&ecs.Deployment{
							Status:         aws.String("PRIMARY"),
							TaskDefinition: aws.String("td2"),
						},
						&ecs.Deployment{
							Status:         aws.String("ACTIVE"),
							TaskDefinition: aws.String("td1"),
						},
					},
				},
			},
			errorList: true,
			wantError: true,
		},
		{
			services: []*ecs.Service{
				&ecs.Service{
					ServiceName: aws.String("Service1"),
					Deployments: []*ecs.Deployment{
						&ecs.Deployment{
							Status:         aws.String("PRIMARY"),
							TaskDefinition: aws.String("td2"),
						},
						&ecs.Deployment{
							Status:         aws.String("ACTIVE"),
							TaskDefinition: aws.String("td1"),
						},
					},
				},
			},
			errorDesc: true,
			wantError: true,
		},
		{
			services: []*ecs.Service{
				&ecs.Service{
					ServiceName: aws.String("Service1"),
					Deployments: []*ecs.Deployment{
						&ecs.Deployment{
							Status:         aws.String("PRIMARY"),
							TaskDefinition: aws.String("td2"),
						},
						&ecs.Deployment{
							Status:         aws.String("ACTIVE"),
							TaskDefinition: aws.String("td1"),
						},
					},
				},
				&ecs.Service{
					ServiceName: aws.String("Service2"),
					Deployments: []*ecs.Deployment{
						&ecs.Deployment{
							Status:         aws.String("PRIMARY"),
							TaskDefinition: aws.String("td1"),
						},
					},
				},
				&ecs.Service{
					ServiceName: aws.String("Service3"),
					Deployments: []*ecs.Deployment{
						&ecs.Deployment{
							Status:         aws.String("PRIMARY"),
							TaskDefinition: aws.String("td1"),
						},
					},
				},
			},
		},
	}

	for _, test := range tests {

		sIDs := make([]string, len(test.services))
		for i, s := range test.services {
			sIDs[i] = aws.StringValue(s.ServiceArn)
		}

		// Mock
		mockECS := &sdk.ECSAPI{}
		awsmock.MockECSListServices(t, mockECS, test.errorList, sIDs...)
		awsmock.MockECSDescribeServices(t, mockECS, test.errorDesc, test.services...)

		r := &AWSRetriever{
			ecsCli: mockECS,
		}

		res, err := r.getServices(context.TODO(), &ecs.Cluster{ClusterArn: aws.String("c1")})

		if !test.wantError {
			if len(res) != len(test.services) {
				t.Errorf("- %+v\n -The length of the retrieved services differ, want: %d; got: %d", test, len(test.services), len(res))
			}

			for i, got := range res {
				want := test.services[i]
				if !reflect.DeepEqual(want, got) {
					t.Errorf("\n- %v\n-  Received service from API is wrong, want: %v; got: %v", test, want, got)
				}
			}

		} else {
			if err == nil {
				t.Errorf("- %+v\n -Getting services should error, it didn't", test)
			}
		}
	}
}

func TestGetTaskDefinitions(t *testing.T) {
	tests := []struct {
		taskDefs  []*ecs.TaskDefinition
		errorWhen int
		wantError bool
	}{
		{
			taskDefs: []*ecs.TaskDefinition{
				&ecs.TaskDefinition{
					TaskDefinitionArn: aws.String("t1"),
					ContainerDefinitions: []*ecs.ContainerDefinition{
						&ecs.ContainerDefinition{
							Name:  aws.String("myService"),
							Image: aws.String("000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e"),
							DockerLabels: map[string]*string{
								"monitor": aws.String("true"),
								"kind":    aws.String("main"),
							},
						},
						&ecs.ContainerDefinition{
							Name:  aws.String("nginx"),
							Image: aws.String("nginx:latest"),
							DockerLabels: map[string]*string{
								"kind": aws.String("front-http"),
							},
						},
						&ecs.ContainerDefinition{
							Name:  aws.String("worker"),
							Image: aws.String("000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e"),
							DockerLabels: map[string]*string{
								"kind": aws.String("worker"),
							},
						},
					},
				},
			},
		},
		{
			taskDefs: []*ecs.TaskDefinition{
				&ecs.TaskDefinition{
					TaskDefinitionArn: aws.String("t1"),
					ContainerDefinitions: []*ecs.ContainerDefinition{
						&ecs.ContainerDefinition{
							Name:  aws.String("myService"),
							Image: aws.String("000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e"),
							DockerLabels: map[string]*string{
								"monitor": aws.String("true"),
								"kind":    aws.String("main"),
							},
						},
						&ecs.ContainerDefinition{
							Name:  aws.String("nginx"),
							Image: aws.String("nginx:latest"),
							DockerLabels: map[string]*string{
								"kind": aws.String("front-http"),
							},
						},
						&ecs.ContainerDefinition{
							Name:  aws.String("worker"),
							Image: aws.String("000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e"),
							DockerLabels: map[string]*string{
								"kind": aws.String("worker"),
							},
						},
					},
				},
			},
			errorWhen: 1,
			wantError: true,
		},
		{
			taskDefs: []*ecs.TaskDefinition{
				&ecs.TaskDefinition{
					TaskDefinitionArn: aws.String("t1"),
					ContainerDefinitions: []*ecs.ContainerDefinition{
						&ecs.ContainerDefinition{
							Name:  aws.String("myService"),
							Image: aws.String("000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e"),
							DockerLabels: map[string]*string{
								"monitor": aws.String("true"),
								"kind":    aws.String("main"),
							},
						},
					},
				},
				&ecs.TaskDefinition{
					TaskDefinitionArn: aws.String("t2"),
					ContainerDefinitions: []*ecs.ContainerDefinition{
						&ecs.ContainerDefinition{
							Name:  aws.String("myService2"),
							Image: aws.String("000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService2:29f323e"),
							DockerLabels: map[string]*string{
								"monitor": aws.String("true"),
								"kind":    aws.String("main2"),
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {

		tIDs := make([]*string, len(test.taskDefs))
		for i, td := range test.taskDefs {
			tIDs[i] = td.TaskDefinitionArn
		}

		// Mock
		mockECS := &sdk.ECSAPI{}
		awsmock.MockECSDescribeTaskDefinition(t, mockECS, test.errorWhen, test.taskDefs...)

		r := &AWSRetriever{
			ecsCli: mockECS,
			cache:  newAWSCache(),
		}

		res, err := r.getTaskDefinitions(context.TODO(), tIDs, false)

		if !test.wantError {
			if len(res) != len(test.taskDefs) {
				t.Errorf("- %+v\n -The length of the retrieved task definitions differ, want: %d; got: %d", test, len(test.taskDefs), len(res))
			}
		} else {
			if err == nil {
				t.Errorf("- %+v\n -Getting task definitions should error, it didn't", test)
			}
		}
	}
}

func TestGetTaskDefinitionsCached(t *testing.T) {
	tests := []struct {
		cachedTaskDefs []*ecs.TaskDefinition
		newTaskDefs    []*ecs.TaskDefinition
		wantRetrieved  int
		wantCached     bool
	}{
		{
			cachedTaskDefs: []*ecs.TaskDefinition{
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("ct1")},
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("ct2")},
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("ct3")},
			},
			newTaskDefs: []*ecs.TaskDefinition{
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("t1")},
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("t2")},
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("t3")},
			},
			wantRetrieved: 6,
			wantCached:    false,
		},
		{
			cachedTaskDefs: []*ecs.TaskDefinition{
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("ct1")},
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("ct2")},
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("ct3")},
			},
			newTaskDefs: []*ecs.TaskDefinition{
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("t1")},
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("t2")},
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("t3")},
			},
			wantRetrieved: 3,
			wantCached:    true,
		},
		{
			cachedTaskDefs: []*ecs.TaskDefinition{},
			newTaskDefs: []*ecs.TaskDefinition{
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("t1")},
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("t2")},
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("t3")},
			},
			wantRetrieved: 3,
			wantCached:    false,
		},
		{
			cachedTaskDefs: []*ecs.TaskDefinition{},
			newTaskDefs: []*ecs.TaskDefinition{
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("t1")},
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("t2")},
				&ecs.TaskDefinition{TaskDefinitionArn: aws.String("t3")},
			},
			wantRetrieved: 3,
			wantCached:    true,
		},
	}

	for _, test := range tests {

		tIDs := []*string{}
		allTaskDefs := []*ecs.TaskDefinition{}
		for _, td := range test.cachedTaskDefs {
			tIDs = append(tIDs, td.TaskDefinitionArn)
			allTaskDefs = append(allTaskDefs, td)
		}
		for _, td := range test.newTaskDefs {
			tIDs = append(tIDs, td.TaskDefinitionArn)
			allTaskDefs = append(allTaskDefs, td)
		}

		// Mock
		mockECS := &sdk.ECSAPI{}
		if test.wantCached {
			awsmock.MockECSDescribeTaskDefinition(t, mockECS, 0, test.newTaskDefs...)
		} else {

			awsmock.MockECSDescribeTaskDefinition(t, mockECS, 0, allTaskDefs...)
		}

		r := &AWSRetriever{
			ecsCli: mockECS,
			cache:  newAWSCache(),
		}

		// Fill cache
		for _, t := range test.cachedTaskDefs {
			r.cache.setTaskDefinition(t)
		}

		res, err := r.getTaskDefinitions(context.TODO(), tIDs, test.wantCached)
		if err != nil {
			t.Errorf("- %+v\n -Getting task definitions shouldn't error: %s", test, err)
		}
		if len(res) != test.wantRetrieved {
			t.Errorf("- %+v\n -The length of the retrieved task definitions differ, want: %d; got: %d", test, test.wantRetrieved, len(res))
		}
	}
}

func mockFullAPI(t *testing.T, mockECS *sdk.ECSAPI, mockEC2 *sdk.EC2API,
	errLClusters, errDClusters, errLCInstances, errDCInstances, errDInstances,
	errLTasks, errDTasks, errLServices, errDServices bool) {

	// Mock our cluster
	cluster := &ecs.Cluster{
		ClusterArn:  aws.String("c1"),
		ClusterName: aws.String("cluster1"),
	}
	awsmock.MockECSListClusters(t, mockECS, errLClusters, "c1")
	if errLClusters {
		return
	}

	awsmock.MockECSDescribeClusters(t, mockECS, errDClusters, cluster)
	if errDClusters {
		return
	}

	// Mock container instances
	cInstance1 := &ecs.ContainerInstance{
		ContainerInstanceArn: aws.String("ci1"),
		Ec2InstanceId:        aws.String("ec2i1"),
	}
	cInstance2 := &ecs.ContainerInstance{
		ContainerInstanceArn: aws.String("ci2"),
		Ec2InstanceId:        aws.String("ec2i2"),
	}
	awsmock.MockECSListContainerInstances(t, mockECS, errLCInstances, "ci1", "ci2")
	if errLCInstances {
		return
	}
	awsmock.MockECSDescribeContainerInstances(t, mockECS, errDCInstances, cInstance1, cInstance2)
	if errDCInstances {
		return
	}

	// Mock ec2 instances
	ec2Instance1 := &ec2.Instance{
		InstanceId:       aws.String("ec2i1"),
		PrivateIpAddress: aws.String("10.0.250.65"),
		Tags: []*ec2.Tag{
			&ec2.Tag{Key: aws.String("env"), Value: aws.String("prod")},
			&ec2.Tag{Key: aws.String("number"), Value: aws.String("1")},
			&ec2.Tag{Key: aws.String("cluster"), Value: aws.String("c1")},
		},
	}
	ec2Instance2 := &ec2.Instance{
		InstanceId:       aws.String("ec2i2"),
		PrivateIpAddress: aws.String("10.0.056.17"),
		Tags: []*ec2.Tag{
			&ec2.Tag{Key: aws.String("env"), Value: aws.String("prod")},
			&ec2.Tag{Key: aws.String("number"), Value: aws.String("2")},
			&ec2.Tag{Key: aws.String("cluster"), Value: aws.String("c1")},
		},
	}
	awsmock.MockEC2DescribeInstances(t, mockEC2, errDInstances, ec2Instance1, ec2Instance2)
	if errDInstances {
		return
	}
	// Mock tasks
	task1 := &ecs.Task{
		TaskArn:              aws.String("task1"),
		TaskDefinitionArn:    aws.String("taskdef1"),
		ClusterArn:           aws.String("c1"),
		ContainerInstanceArn: aws.String("ci1"),
		Containers: []*ecs.Container{
			&ecs.Container{
				Name: aws.String("service1"),
				NetworkBindings: []*ecs.NetworkBinding{
					&ecs.NetworkBinding{
						HostPort:      aws.Int64(20001),
						ContainerPort: aws.Int64(8000),
						Protocol:      aws.String("tcp"),
					},
					&ecs.NetworkBinding{
						HostPort:      aws.Int64(33000),
						ContainerPort: aws.Int64(9782),
						Protocol:      aws.String("udp"),
					},
				},
			},
			&ecs.Container{
				Name: aws.String("nginx"),
				NetworkBindings: []*ecs.NetworkBinding{&ecs.NetworkBinding{
					HostPort:      aws.Int64(30987),
					ContainerPort: aws.Int64(8081),
					Protocol:      aws.String("tcp"),
				}},
			},
			&ecs.Container{
				Name:            aws.String("worker"),
				NetworkBindings: []*ecs.NetworkBinding{},
			},
		},
	}

	task2 := &ecs.Task{
		TaskArn:              aws.String("task2"),
		TaskDefinitionArn:    aws.String("taskdef2"),
		ClusterArn:           aws.String("c1"),
		ContainerInstanceArn: aws.String("ci2"),
		Containers: []*ecs.Container{
			&ecs.Container{
				Name: aws.String("service20"),
				NetworkBindings: []*ecs.NetworkBinding{
					&ecs.NetworkBinding{
						HostPort:      aws.Int64(35000),
						ContainerPort: aws.Int64(8080),
						Protocol:      aws.String("tcp"),
					},
				},
			},
			&ecs.Container{
				Name: aws.String("service21"),
				NetworkBindings: []*ecs.NetworkBinding{&ecs.NetworkBinding{
					HostPort:      aws.Int64(36000),
					ContainerPort: aws.Int64(8080),
					Protocol:      aws.String("tcp"),
				}},
			},
			&ecs.Container{
				Name: aws.String("service22"),
				NetworkBindings: []*ecs.NetworkBinding{&ecs.NetworkBinding{
					HostPort:      aws.Int64(35057),
					ContainerPort: aws.Int64(8080),
					Protocol:      aws.String("tcp"),
				}},
			},
		},
	}

	awsmock.MockECSListTasks(t, mockECS, errLTasks, "task1", "task2")
	if errLTasks {
		return
	}
	awsmock.MockECSDescribeTasks(t, mockECS, errDTasks, task1, task2)
	if errDTasks {
		return
	}

	// Mock services
	service1 := &ecs.Service{
		ServiceName: aws.String("service1"),
		Deployments: []*ecs.Deployment{
			&ecs.Deployment{
				Status:         aws.String("PRIMARY"),
				TaskDefinition: aws.String("taskdef1"),
			},
			&ecs.Deployment{
				Status:         aws.String("INACTIVE"),
				TaskDefinition: aws.String("taskdef3"),
			},
		},
	}

	service2 := &ecs.Service{
		ServiceName: aws.String("service2"),
		Deployments: []*ecs.Deployment{
			&ecs.Deployment{
				Status:         aws.String("PRIMARY"),
				TaskDefinition: aws.String("taskdef2"),
			},
		},
	}

	awsmock.MockECSListServices(t, mockECS, errLServices, "service1", "service2")
	if errLServices {
		return
	}
	awsmock.MockECSDescribeServices(t, mockECS, errDServices, service1, service2)
	if errDServices {
		return
	}

	// Mock task definitions
	taskdef1 := &ecs.TaskDefinition{
		TaskDefinitionArn: aws.String("taskdef1"),
		ContainerDefinitions: []*ecs.ContainerDefinition{
			&ecs.ContainerDefinition{
				Name:  aws.String("service1"),
				Image: aws.String("myCompany/service1:29f323e"),
				DockerLabels: map[string]*string{
					"monitor": aws.String("true"),
					"kind":    aws.String("main"),
				},
			},
			&ecs.ContainerDefinition{
				Name:  aws.String("nginx"),
				Image: aws.String("nginx:latest"),
				DockerLabels: map[string]*string{
					"kind": aws.String("front-http"),
				},
			},
			&ecs.ContainerDefinition{
				Name:  aws.String("worker"),
				Image: aws.String("myCompany/service1:29f323e"),
				DockerLabels: map[string]*string{
					"kind": aws.String("worker"),
				},
			},
		},
	}

	taskdef2 := &ecs.TaskDefinition{
		TaskDefinitionArn: aws.String("taskdef2"),
		ContainerDefinitions: []*ecs.ContainerDefinition{
			&ecs.ContainerDefinition{
				Name:  aws.String("service20"),
				Image: aws.String("myCompany/service20:b7bb21f"),
				DockerLabels: map[string]*string{
					"monitor": aws.String("true"),
					"kind":    aws.String("service2"),
				},
			},
			&ecs.ContainerDefinition{
				Name:  aws.String("service21"),
				Image: aws.String("myCompany/service21:b7bb21f"),
				DockerLabels: map[string]*string{
					"kind":   aws.String("service2"),
					"backup": aws.String("true"),
				},
			},
			&ecs.ContainerDefinition{
				Name:  aws.String("service22"),
				Image: aws.String("myCompany/service22:5f4d97f"),
				DockerLabels: map[string]*string{
					"kind":   aws.String("service2"),
					"backup": aws.String("true"),
				},
			},
		},
	}
	awsmock.MockECSDescribeTaskDefinition(t, mockECS, 0, taskdef1, taskdef2)
}

func TestRetrieveOk(t *testing.T) {
	want := []*types.ServiceInstance{
		&types.ServiceInstance{
			Cluster:            "cluster1",
			Service:            "service1",
			Addr:               "10.0.250.65:20001",
			Container:          "service1",
			ContainerPort:      "8000",
			ContainerPortProto: "tcp",
			Image:              "myCompany/service1:29f323e",
			Labels:             map[string]string{"monitor": "true", "kind": "main"},
			Tags:               map[string]string{"env": "prod", "number": "1", "cluster": "c1"},
		},
		&types.ServiceInstance{
			Cluster:            "cluster1",
			Service:            "service1",
			Addr:               "10.0.250.65:33000",
			Container:          "service1",
			ContainerPort:      "9782",
			ContainerPortProto: "udp",
			Image:              "myCompany/service1:29f323e",
			Labels:             map[string]string{"monitor": "true", "kind": "main"},
			Tags:               map[string]string{"env": "prod", "number": "1", "cluster": "c1"},
		},
		&types.ServiceInstance{
			Cluster:            "cluster1",
			Service:            "service1",
			Addr:               "10.0.250.65:30987",
			Container:          "nginx",
			ContainerPort:      "8081",
			ContainerPortProto: "tcp",
			Image:              "nginx:latest",
			Labels:             map[string]string{"kind": "front-http"},
			Tags:               map[string]string{"env": "prod", "number": "1", "cluster": "c1"},
		},
		&types.ServiceInstance{
			Cluster:            "cluster1",
			Service:            "service2",
			Addr:               "10.0.056.17:35000",
			Container:          "service20",
			ContainerPort:      "8080",
			ContainerPortProto: "tcp",
			Image:              "myCompany/service20:b7bb21f",
			Labels:             map[string]string{"monitor": "true", "kind": "service2"},
			Tags:               map[string]string{"env": "prod", "number": "2", "cluster": "c1"},
		},
		&types.ServiceInstance{
			Cluster:            "cluster1",
			Service:            "service2",
			Addr:               "10.0.056.17:36000",
			Container:          "service21",
			ContainerPort:      "8080",
			ContainerPortProto: "tcp",
			Image:              "myCompany/service21:b7bb21f",
			Labels:             map[string]string{"backup": "true", "kind": "service2"},
			Tags:               map[string]string{"env": "prod", "number": "2", "cluster": "c1"},
		},
		&types.ServiceInstance{
			Cluster:            "cluster1",
			Service:            "service2",
			Addr:               "10.0.056.17:35057",
			Container:          "service22",
			ContainerPort:      "8080",
			ContainerPortProto: "tcp",
			Image:              "myCompany/service22:5f4d97f",
			Labels:             map[string]string{"backup": "true", "kind": "service2"},
			Tags:               map[string]string{"env": "prod", "number": "2", "cluster": "c1"},
		},
	}

	// Mock
	mockECS := &sdk.ECSAPI{}
	mockEC2 := &sdk.EC2API{}

	// Mock all the API
	mockFullAPI(t, mockECS, mockEC2, false, false, false, false, false, false, false, false, false)

	r := &AWSRetriever{
		ecsCli: mockECS,
		ec2Cli: mockEC2,
		cache:  newAWSCache(),
	}

	// Retrieve the information
	got, err := r.Retrieve()

	if err != nil {
		t.Errorf("Retrieval shound't error, it did: %s", err)
	}

	if len(got) != len(want) {
		t.Errorf("The retrieved length of service instances is not correct, got: %d; want: %d", len(got), len(want))
	}

	for i, gotT := range got {
		w := want[i]
		if !reflect.DeepEqual(w, gotT) {
			t.Errorf("- Received service instance taget is wrong want: %+v; got: %+v", w, gotT)
		}
	}
}

func TestRetrieveError(t *testing.T) {
	tests := []struct {
		errLClusters   bool
		errDClusters   bool
		errLCInstances bool
		errDCInstances bool
		errDInstances  bool
		errLTasks      bool
		errDTasks      bool
		errLServices   bool
		errDServices   bool
	}{
		{errLClusters: true, errDClusters: false, errLCInstances: false, errDCInstances: false, errDInstances: false, errLTasks: false, errDTasks: false, errLServices: false, errDServices: false},
		{errLClusters: false, errDClusters: true, errLCInstances: false, errDCInstances: false, errDInstances: false, errLTasks: false, errDTasks: false, errLServices: false, errDServices: false},
		{errLClusters: false, errDClusters: false, errLCInstances: true, errDCInstances: false, errDInstances: false, errLTasks: false, errDTasks: false, errLServices: false, errDServices: false},
		{errLClusters: false, errDClusters: false, errLCInstances: false, errDCInstances: true, errDInstances: false, errLTasks: false, errDTasks: false, errLServices: false, errDServices: false},
		{errLClusters: false, errDClusters: false, errLCInstances: false, errDCInstances: false, errDInstances: true, errLTasks: false, errDTasks: false, errLServices: false, errDServices: false},
		{errLClusters: false, errDClusters: false, errLCInstances: false, errDCInstances: false, errDInstances: false, errLTasks: true, errDTasks: false, errLServices: false, errDServices: false},
		{errLClusters: false, errDClusters: false, errLCInstances: false, errDCInstances: false, errDInstances: false, errLTasks: false, errDTasks: true, errLServices: false, errDServices: false},
		{errLClusters: false, errDClusters: false, errLCInstances: false, errDCInstances: false, errDInstances: false, errLTasks: false, errDTasks: false, errLServices: true, errDServices: false},
		{errLClusters: false, errDClusters: false, errLCInstances: false, errDCInstances: false, errDInstances: false, errLTasks: false, errDTasks: false, errLServices: false, errDServices: true},
	}

	for _, test := range tests {
		// Mock
		mockECS := &sdk.ECSAPI{}
		mockEC2 := &sdk.EC2API{}

		// Mock all the API
		mockFullAPI(t, mockECS, mockEC2, test.errLClusters, test.errDClusters, test.errLCInstances, test.errDCInstances, test.errDInstances, test.errLTasks, test.errDTasks, test.errLServices, test.errDServices)

		r := &AWSRetriever{
			ecsCli: mockECS,
			ec2Cli: mockEC2,
			cache:  newAWSCache(),
		}

		// Retrieve the information
		_, err := r.Retrieve()

		if err == nil {
			t.Errorf("Retrieval should error, it didn't")
		}
	}

}
