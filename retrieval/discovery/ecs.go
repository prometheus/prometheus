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
	ecs "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"golang.org/x/net/context"
	"time"
)

// ECSDiscovery periodically performs ECS-SD requests. It implements
// the TargetProvider interface.
type ECSDiscovery struct {
	aws         *aws.Config
	interval    time.Duration
	portFrom    int64
	portTo      int64
	clusterName string
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
		aws:         awsConfig,
		interval:    time.Duration(conf.RefreshInterval),
		portFrom:    int64(conf.PortFrom),
		portTo:      int64(conf.PortTo),
		clusterName: conf.ClusterName,
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

	listServicesResponse, err := ecsService.ListServices(&ecs.ListServicesInput{
		Cluster:    aws.String(ed.clusterName),
		MaxResults: aws.Int64(100),
	})
	if err != nil {
		return nil, fmt.Errorf("error listing ecs services: %s", err)
	}

	services, err := ecsService.DescribeServices(&ecs.DescribeServicesInput{
		Cluster:  aws.String("default"),
		Services: listServicesResponse.ServiceArns,
	})
	if err != nil {
		return nil, fmt.Errorf("error listing describing services: %s", err)
	}

	for _, service := range services.Services {
		// Possibly cache theses, since ARNS are immutable, afaik
		taskDefinitionResponse, err := ecsService.DescribeTaskDefinition(&ecs.DescribeTaskDefinitionInput{
			TaskDefinition: service.TaskDefinition,
		})
		if err != nil {
			return nil, fmt.Errorf("error listing describing task description: %s", err)
		}

		monitored := false
		monitoringPort := 0

		var dockerLabels map[string]*string

		// See if any container is exposing the specified ports
		for _, containerDesc := range taskDefinitionResponse.TaskDefinition.ContainerDefinitions {
			for _, portMapping := range containerDesc.PortMappings {
				if *portMapping.HostPort >= ed.portFrom && *portMapping.HostPort <= ed.portTo {
					monitored = true
					monitoringPort = int(*portMapping.HostPort)
					dockerLabels = containerDesc.DockerLabels
					break
				}
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

		for _, task := range taskDescriptionsResponse.Tasks {
			// Describe the container instance to get the IP
			containerInstanceResponse, err := ecsService.DescribeContainerInstances(&ecs.DescribeContainerInstancesInput{
				ContainerInstances: []*string{task.ContainerInstanceArn},
			})
			if err != nil {
				return nil, fmt.Errorf("error describing container instance: %s", err)
			}

			containerInstance := containerInstanceResponse.ContainerInstances[0]
			ec2Response, err := ec2s.DescribeInstances(&ec2.DescribeInstancesInput{
				InstanceIds: []*string{containerInstance.Ec2InstanceId},
			})
			if err != nil {
				return nil, fmt.Errorf("error describing container ec2 instance: %s", err)
			}

			inst := ec2Response.Reservations[0].Instances[0]
			labels := model.LabelSet{
				"ecs_service_name": model.LabelValue(*service.ServiceName),
			}
			addr := fmt.Sprintf("%s:%d", *inst.PrivateIpAddress, monitoringPort)
			labels[model.AddressLabel] = model.LabelValue(addr)

			// We also want to pipe the docker labels to the series
			for dockerLabelName, dockerLabelValue := range dockerLabels {
				labels[model.LabelName(dockerLabelName)] = model.LabelValue(*dockerLabelValue)
			}

			tg.Targets = append(tg.Targets, labels)

		}
	}
	return tg, nil
}
