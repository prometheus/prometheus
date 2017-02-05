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
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
)

const (
	serviceTaskStatusInactive = "INACTIVE"
)

// awsCache is a temporal store to map between ids on AWS API objects
type awsCache struct {
	// Store indexes
	clusters   map[string]*ecs.Cluster           // cluster cache
	cInstances map[string]*ecs.ContainerInstance // container instance cache
	tasks      map[string]*ecs.Task              // task cache
	instances  map[string]*ec2.Instance          // instance cache
	services   map[string]*ecs.Service           // service cache
	taskDefs   map[string]*ecs.TaskDefinition    // task definition cache

	// mutexes
	clustersMutex   sync.Mutex
	cInstancesMutex sync.Mutex
	tasksMutex      sync.Mutex
	instancesMutex  sync.Mutex
	servicesMutex   sync.Mutex
	taskDefsMutex   sync.Mutex
}

func newAWSCache() *awsCache {
	a := &awsCache{
		clusters:   map[string]*ecs.Cluster{},
		cInstances: map[string]*ecs.ContainerInstance{},
		tasks:      map[string]*ecs.Task{},
		instances:  map[string]*ec2.Instance{},
		services:   map[string]*ecs.Service{},
		taskDefs:   map[string]*ecs.TaskDefinition{},
	}

	return a
}

func (a *awsCache) getCluster(clusterARN string) (cluster *ecs.Cluster, ok bool) {
	a.clustersMutex.Lock()
	defer a.clustersMutex.Unlock()
	cluster, ok = a.clusters[clusterARN]
	return
}

func (a *awsCache) SetClusters(clusters ...*ecs.Cluster) {
	a.clustersMutex.Lock()
	defer a.clustersMutex.Unlock()
	for _, c := range clusters {
		a.clusters[aws.StringValue(c.ClusterArn)] = c
	}
}

func (a *awsCache) flushClusters() {
	a.clustersMutex.Lock()
	defer a.clustersMutex.Unlock()
	a.clusters = map[string]*ecs.Cluster{}
}

func (a *awsCache) getContainerInstance(cIARN string) (cInstance *ecs.ContainerInstance, ok bool) {
	a.cInstancesMutex.Lock()
	defer a.cInstancesMutex.Unlock()
	cInstance, ok = a.cInstances[cIARN]
	return
}

func (a *awsCache) setContainerInstances(cInstances ...*ecs.ContainerInstance) {
	a.cInstancesMutex.Lock()
	defer a.cInstancesMutex.Unlock()
	for _, c := range cInstances {
		a.cInstances[aws.StringValue(c.ContainerInstanceArn)] = c
	}
}

func (a *awsCache) flushContainerInstances() {
	a.cInstancesMutex.Lock()
	defer a.cInstancesMutex.Unlock()
	a.cInstances = map[string]*ecs.ContainerInstance{}
}

func (a *awsCache) getTask(taskARN string) (task *ecs.Task, ok bool) {
	a.tasksMutex.Lock()
	defer a.tasksMutex.Unlock()
	task, ok = a.tasks[taskARN]
	return
}

func (a *awsCache) setTasks(tasks ...*ecs.Task) {
	a.tasksMutex.Lock()
	defer a.tasksMutex.Unlock()
	for _, t := range tasks {
		a.tasks[aws.StringValue(t.TaskArn)] = t
	}
}

func (a *awsCache) flushTasks() {
	a.tasksMutex.Lock()
	defer a.tasksMutex.Unlock()
	a.tasks = map[string]*ecs.Task{}
}

func (a *awsCache) getIntance(instanceID string) (instance *ec2.Instance, ok bool) {
	a.instancesMutex.Lock()
	defer a.instancesMutex.Unlock()
	instance, ok = a.instances[instanceID]
	return
}

func (a *awsCache) setInstances(instances ...*ec2.Instance) {
	a.instancesMutex.Lock()
	defer a.instancesMutex.Unlock()
	for _, i := range instances {
		a.instances[aws.StringValue(i.InstanceId)] = i
	}
}

func (a *awsCache) flushInstances() {
	a.instancesMutex.Lock()
	defer a.instancesMutex.Unlock()
	a.instances = map[string]*ec2.Instance{}
}

func (a *awsCache) getService(taskDefinitionARN string) (service *ecs.Service, ok bool) {
	a.servicesMutex.Lock()
	defer a.servicesMutex.Unlock()
	service, ok = a.services[taskDefinitionARN]
	return
}

func (a *awsCache) setServices(services ...*ecs.Service) {
	a.servicesMutex.Lock()
	defer a.servicesMutex.Unlock()
	for _, service := range services {
		// Insert one entry per running deployment task indexed by task defintion, the task defintion ARN is
		// the glue to reference a service from a task
		for _, d := range service.Deployments {
			if aws.StringValue(d.Status) != serviceTaskStatusInactive {
				a.services[aws.StringValue(d.TaskDefinition)] = service
			}
		}
	}
}

func (a *awsCache) flushServices() (service *ecs.Service, ok bool) {
	a.servicesMutex.Lock()
	defer a.servicesMutex.Unlock()
	a.services = map[string]*ecs.Service{}
	return
}

func (a *awsCache) getTaskDefinition(taskDefinitionARN string) (taskDef *ecs.TaskDefinition, ok bool) {
	a.taskDefsMutex.Lock()
	defer a.taskDefsMutex.Unlock()
	taskDef, ok = a.taskDefs[taskDefinitionARN]
	return
}

func (a *awsCache) setTaskDefinition(taskDefs ...*ecs.TaskDefinition) {
	a.taskDefsMutex.Lock()
	defer a.taskDefsMutex.Unlock()
	for _, td := range taskDefs {
		a.taskDefs[aws.StringValue(td.TaskDefinitionArn)] = td
	}
}

func (a *awsCache) flushTaskDefinitions() {
	a.taskDefsMutex.Lock()
	defer a.taskDefsMutex.Unlock()
	a.taskDefs = map[string]*ecs.TaskDefinition{}
}
