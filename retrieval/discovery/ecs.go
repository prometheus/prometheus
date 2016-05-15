// Copyright 2015 The Prometheus Authors
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

package discovery

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"golang.org/x/net/context"
	"time"
)

const (
	ecsServiceNameLabel = model.MetaLabelPrefix + "ecs_service_name"
	ecsClusterNameLabel = model.MetaLabelPrefix + "ecs_cluster_name"
)

// ECSDiscovery periodically performs ECS-SD requests. It implements
// the TargetProvider interface.
type ECSDiscovery struct {
	aws      *aws.Config
	interval time.Duration
	port     int64
}

// NewECSDiscovery returns a new ECSDiscovery which periodically refreshes its targets.
func NewECSDiscovery(conf *config.ECSSDConfig) *ECSDiscovery {
	awsConfig := defaults.Get().Config

	// override the default region
	if conf.Region != "" {
		awsConfig.Region = &conf.Region
	}
	if conf.AccessKey != "" && conf.SecretKey != "" {
		creds := credentials.NewStaticCredentials(conf.AccessKey, conf.SecretKey, "")
		awsConfig.Credentials = creds
	}

	return &ECSDiscovery{
		aws:      awsConfig,
		interval: time.Duration(conf.RefreshInterval),
		port:     int64(conf.Port),
	}
}

// Run implements the TargetProvider interface.
func (ed *ECSDiscovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	defer close(ch)

	ticker := time.NewTicker(ed.interval)
	defer ticker.Stop()

	// Get an initial set right away.
	tg, err := ed.refresh()
	if err != nil {
		log.Error(err)
	} else {
		ch <- []*config.TargetGroup{tg}
	}

	for {
		select {
		case <-ticker.C:
			tg, err := ed.refresh()
			if err != nil {
				log.Error(err)
			} else {
				ch <- []*config.TargetGroup{tg}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (ed *ECSDiscovery) refresh() (*config.TargetGroup, error) {
	tg := &config.TargetGroup{
		Source: *ed.aws.Region,
	}
	ecsService := ecs.New(session.New(), ed.aws)
	ec2s := ec2.New(session.New(), ed.aws)

	listClustersResponse, err := ecsService.ListClusters(&ecs.ListClustersInput{})
	if err != nil {
		return nil, fmt.Errorf("error listing ecs clusters: %s", err)
	}

	describeClustersResponse, err := ecsService.DescribeClusters(&ecs.DescribeClustersInput{Clusters: listClustersResponse.ClusterArns})
	if err != nil {
		return nil, fmt.Errorf("error describing ecs clusters: %s", err)
	}

	for _, cluster := range describeClustersResponse.Clusters {

		listServicesResponse, err := ecsService.ListServices(&ecs.ListServicesInput{
			Cluster:    aws.String(*cluster.ClusterArn),
			MaxResults: aws.Int64(100),
		})
		if err != nil {
			return nil, fmt.Errorf("error listing ecs services: %s", err)
		}

		services, err := ecsService.DescribeServices(&ecs.DescribeServicesInput{
			Cluster:  aws.String(*cluster.ClusterArn),
			Services: listServicesResponse.ServiceArns,
		})
		if err != nil {
			return nil, fmt.Errorf("error listing describing services: %s", err)
		}

		taskDefitionCache := make(map[string]*ecs.DescribeTaskDefinitionOutput)

		for _, service := range services.Services {

			// In case we encounter more than one service using the same service definition, no need to look it up again
			taskDefinitionResponse, cached := taskDefitionCache[*service.TaskDefinition]
			if !cached {
				taskDefinitionResponse, err = ecsService.DescribeTaskDefinition(&ecs.DescribeTaskDefinitionInput{
					TaskDefinition: service.TaskDefinition,
				})
				if err != nil {
					return nil, fmt.Errorf("error listing describing task description: %s", err)
				}
				taskDefitionCache[*service.TaskDefinition] = taskDefinitionResponse
			}

			monitored := false
			monitoringPort := 0

			var dockerLabels map[string]*string

			// See if any container is exposing the specified port and use the host post which it is mapped to
			for _, containerDesc := range taskDefinitionResponse.TaskDefinition.ContainerDefinitions {
				for _, portMapping := range containerDesc.PortMappings {
					if *portMapping.ContainerPort == ed.port {
						monitored = true
						monitoringPort = int(*portMapping.HostPort)
						dockerLabels = containerDesc.DockerLabels
						break
					}
				}
				if monitored {
					break
				}
			}

			if !monitored {
				continue
			}

			tasksArns, err := ecsService.ListTasks(&ecs.ListTasksInput{
				ServiceName: aws.String(*service.ServiceName),
			})
			if err != nil {
				return nil, fmt.Errorf("error listing listing tasks: %s", err)
			}

			taskDescriptionsResponse, err := ecsService.DescribeTasks(&ecs.DescribeTasksInput{
				Tasks: tasksArns.TaskArns,
			})
			if err != nil {
				return nil, fmt.Errorf("error describing tasks: %s", err)
			}

			describeContainerInstancesRequest := &ecs.DescribeContainerInstancesInput{
				ContainerInstances: []*string{},
			}

			// First we populate the cache keys
			for _, task := range taskDescriptionsResponse.Tasks {
				describeContainerInstancesRequest.ContainerInstances = append(describeContainerInstancesRequest.ContainerInstances, task.ContainerInstanceArn)
			}
			containerInstanceResponse, err := ecsService.DescribeContainerInstances(describeContainerInstancesRequest)
			if err != nil {
				return nil, fmt.Errorf("error describing container instance: %s", err)
			}

			containerInstances := make(map[string]*ecs.ContainerInstance)

			ec2InstancesRequest := &ec2.DescribeInstancesInput{
				InstanceIds: []*string{},
			}

			for _, containerInstance := range containerInstanceResponse.ContainerInstances {
				containerInstances[*containerInstance.ContainerInstanceArn] = containerInstance
				ec2InstancesRequest.InstanceIds = append(ec2InstancesRequest.InstanceIds, containerInstance.Ec2InstanceId)
			}

			ec2Response, err := ec2s.DescribeInstances(ec2InstancesRequest)
			if err != nil {
				return nil, fmt.Errorf("error describing container ec2 instance: %s", err)
			}

			ec2Instances := make(map[string]*ec2.Instance)
			for _, reservation := range ec2Response.Reservations {
				for _, ec2 := range reservation.Instances {
					ec2Instances[*ec2.InstanceId] = ec2
				}
			}

			for _, task := range taskDescriptionsResponse.Tasks {
				containerInstance, found := containerInstances[*task.ContainerInstanceArn]
				if !found {
					continue
				}
				inst, found := ec2Instances[*containerInstance.Ec2InstanceId]
				if !found {
					continue
				}

				labels := model.LabelSet{
					ecsServiceNameLabel: model.LabelValue(*service.ServiceName),
					ecsClusterNameLabel: model.LabelValue(*cluster.ClusterName),
				}
				addr := fmt.Sprintf("%s:%d", *inst.PrivateIpAddress, monitoringPort)
				labels[model.AddressLabel] = model.LabelValue(addr)

				// We also want to pipe the docker labels to the series
				for dockerLabelName, dockerLabelValue := range dockerLabels {
					labels[model.LabelName(model.MetaLabelPrefix+dockerLabelName)] = model.LabelValue(*dockerLabelValue)
				}

				tg.Targets = append(tg.Targets, labels)

			}
		}
	}
	return tg, nil
}
