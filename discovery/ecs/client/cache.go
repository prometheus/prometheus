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

		clustersMutex:   sync.Mutex{},
		cInstancesMutex: sync.Mutex{},
		tasksMutex:      sync.Mutex{},
		instancesMutex:  sync.Mutex{},
		servicesMutex:   sync.Mutex{},
		taskDefsMutex:   sync.Mutex{},
	}

	return a
}

func (a *awsCache) getCluster(clusterARN string) (cluster *ecs.Cluster, ok bool) {
	cluster, ok = a.clusters[clusterARN]
	return
}

func (a *awsCache) SetCluster(cluster *ecs.Cluster) {
	a.clustersMutex.Lock()
	defer a.clustersMutex.Unlock()
	a.clusters[aws.StringValue(cluster.ClusterArn)] = cluster
}

func (a *awsCache) getContainerInstance(cIARN string) (cInstance *ecs.ContainerInstance, ok bool) {
	cInstance, ok = a.cInstances[cIARN]
	return
}

func (a *awsCache) setContainerInstance(cInstance *ecs.ContainerInstance) {
	a.cInstancesMutex.Lock()
	defer a.cInstancesMutex.Unlock()
	a.cInstances[aws.StringValue(cInstance.ContainerInstanceArn)] = cInstance
}

func (a *awsCache) getTask(taskARN string) (task *ecs.Task, ok bool) {
	task, ok = a.tasks[taskARN]
	return
}

func (a *awsCache) setTask(task *ecs.Task) {
	a.tasksMutex.Lock()
	defer a.tasksMutex.Unlock()
	a.tasks[aws.StringValue(task.TaskArn)] = task
}

func (a *awsCache) getIntance(instanceID string) (instance *ec2.Instance, ok bool) {
	instance, ok = a.instances[instanceID]
	return
}

func (a *awsCache) setInstance(instance *ec2.Instance) {
	a.instancesMutex.Lock()
	defer a.instancesMutex.Unlock()
	a.instances[aws.StringValue(instance.InstanceId)] = instance
}

func (a *awsCache) getService(taskDefinitionARN string) (service *ecs.Service, ok bool) {
	service, ok = a.services[taskDefinitionARN]
	return
}

func (a *awsCache) setService(service *ecs.Service) {
	a.servicesMutex.Lock()
	defer a.servicesMutex.Unlock()
	// Insert one entry per running deployment task indexed by task defintion, the task defintion ARN is
	// the glue to reference a service from a task
	for _, d := range service.Deployments {
		if aws.StringValue(d.Status) != serviceTaskStatusInactive {
			a.services[aws.StringValue(d.TaskDefinition)] = service
		}
	}
}

func (a *awsCache) getTaskDefinition(taskDefinitionARN string) (taskDef *ecs.TaskDefinition, ok bool) {
	taskDef, ok = a.taskDefs[taskDefinitionARN]
	return
}

func (a *awsCache) setTaskDefinition(taskDef *ecs.TaskDefinition) {
	a.taskDefsMutex.Lock()
	defer a.taskDefsMutex.Unlock()
	a.taskDefs[aws.StringValue(taskDef.TaskDefinitionArn)] = taskDef
}
