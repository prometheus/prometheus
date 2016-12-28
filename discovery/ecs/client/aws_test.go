package client

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/golang/mock/gomock"

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
				NetworkBindings: []*ecs.NetworkBinding{&ecs.NetworkBinding{
					HostPort:      aws.Int64(36112),
					ContainerPort: aws.Int64(8080),
					Protocol:      aws.String("tcp"),
				}},
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
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockECS := sdk.NewMockECSAPI(ctrl)
		awsmock.MockECSListClusters(t, mockECS, test.errorList, cIDs...)
		awsmock.MockECSDescribeClusters(t, mockECS, test.errorDesc, test.clusters...)

		r := &AWSRetriever{
			ecsCli: mockECS,
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
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockECS := sdk.NewMockECSAPI(ctrl)
		awsmock.MockECSListContainerInstances(t, mockECS, test.errorList, ciIDs...)
		awsmock.MockECSDescribeContainerInstances(t, mockECS, test.errorDesc, test.cInstances...)

		r := &AWSRetriever{
			ecsCli: mockECS,
		}

		res, err := r.getContainerInstances(&ecs.Cluster{ClusterArn: aws.String("c1")})

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
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockECS := sdk.NewMockECSAPI(ctrl)
		awsmock.MockECSListTasks(t, mockECS, test.errorList, tIDs...)
		awsmock.MockECSDescribeTasks(t, mockECS, test.errorDesc, test.tasks...)

		r := &AWSRetriever{
			ecsCli: mockECS,
		}

		res, err := r.getTasks(&ecs.Cluster{ClusterArn: aws.String("c1")})

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
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockEC2 := sdk.NewMockEC2API(ctrl)
		awsmock.MockEC2DescribeInstances(t, mockEC2, test.errorDesc, test.instances...)

		r := &AWSRetriever{
			ec2Cli: mockEC2,
		}

		res, err := r.getInstances(iIDs)

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
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockECS := sdk.NewMockECSAPI(ctrl)
		awsmock.MockECSListServices(t, mockECS, test.errorList, sIDs...)
		awsmock.MockECSDescribeServices(t, mockECS, test.errorDesc, test.services...)

		r := &AWSRetriever{
			ecsCli: mockECS,
		}

		res, err := r.getServices(&ecs.Cluster{ClusterArn: aws.String("c1")})

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
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockECS := sdk.NewMockECSAPI(ctrl)
		awsmock.MockECSDescribeTaskDefinition(t, mockECS, test.errorWhen, test.taskDefs...)

		r := &AWSRetriever{
			ecsCli: mockECS,
			cache:  newAWSCache(),
		}

		res, err := r.getTaskDefinitions(tIDs, false)

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
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockECS := sdk.NewMockECSAPI(ctrl)
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

		res, err := r.getTaskDefinitions(tIDs, test.wantCached)
		if err != nil {
			t.Errorf("- %+v\n -Getting task definitions shouldn't error: %s", test, err)
		}
		if len(res) != test.wantRetrieved {
			t.Errorf("- %+v\n -The length of the retrieved task definitions differ, want: %d; got: %d", test, test.wantRetrieved, len(res))
		}
	}
}
