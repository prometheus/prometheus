// Copyright The Prometheus Authors
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
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Struct for test data.
type ecsDataStore struct {
	region string

	clusters []ecsTypes.Cluster
	services []ecsTypes.Service
	tasks    []ecsTypes.Task
}

func TestECSDiscoveryListClusterARNs(t *testing.T) {
	ctx := context.Background()

	// iterate through the test cases
	for _, tt := range []struct {
		name     string
		ecsData  *ecsDataStore
		expected []string
	}{
		{
			name: "MultipleClusters",
			ecsData: &ecsDataStore{
				region: "us-west-2",
				clusters: []ecsTypes.Cluster{
					{
						ClusterName: strptr("test-cluster"),
						ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:      strptr("ACTIVE"),
					},
					{
						ClusterName: strptr("prod-cluster"),
						ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/prod-cluster"),
						Status:      strptr("ACTIVE"),
					},
				},
			},
			expected: []string{
				"arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster",
				"arn:aws:ecs:us-west-2:123456789012:cluster/prod-cluster",
			},
		},
		{
			name: "SingleCluster",
			ecsData: &ecsDataStore{
				region: "us-east-1",
				clusters: []ecsTypes.Cluster{
					{
						ClusterName: strptr("single-cluster"),
						ClusterArn:  strptr("arn:aws:ecs:us-east-1:123456789012:cluster/single-cluster"),
						Status:      strptr("ACTIVE"),
					},
				},
			},
			expected: []string{
				"arn:aws:ecs:us-east-1:123456789012:cluster/single-cluster",
			},
		},
		{
			name: "NoClusters",
			ecsData: &ecsDataStore{
				region:   "us-east-1",
				clusters: []ecsTypes.Cluster{},
			},
			expected: nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockECSClient(tt.ecsData)

			d := &ECSDiscovery{
				ecs: client,
				cfg: &ECSSDConfig{
					Region:             tt.ecsData.region,
					RequestConcurrency: 10,
				},
			}

			clusters, err := d.listClusterARNs(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.expected, clusters)
		})
	}
}

func TestECSDiscoveryDescribeClusters(t *testing.T) {
	ctx := context.Background()

	// iterate through the test cases
	for _, tt := range []struct {
		name        string
		ecsData     *ecsDataStore
		clusterARNs []string
		expected    map[string]ecsTypes.Cluster
	}{
		{
			name: "SingleClusterWithTags",
			ecsData: &ecsDataStore{
				region: "us-west-2",
				clusters: []ecsTypes.Cluster{
					{
						ClusterName: strptr("test-cluster"),
						ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:      strptr("ACTIVE"),
						Tags: []ecsTypes.Tag{
							{Key: strptr("Environment"), Value: strptr("test")},
							{Key: strptr("Team"), Value: strptr("backend")},
						},
					},
				},
			},
			clusterARNs: []string{"arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"},
			expected: map[string]ecsTypes.Cluster{
				"arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster": {
					ClusterName: strptr("test-cluster"),
					ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
					Status:      strptr("ACTIVE"),
					Tags: []ecsTypes.Tag{
						{Key: strptr("Environment"), Value: strptr("test")},
						{Key: strptr("Team"), Value: strptr("backend")},
					},
				},
			},
		},
		{
			name: "MultipleClusters",
			ecsData: &ecsDataStore{
				region: "us-east-1",
				clusters: []ecsTypes.Cluster{
					{
						ClusterName: strptr("cluster-1"),
						ClusterArn:  strptr("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-1"),
						Status:      strptr("ACTIVE"),
					},
					{
						ClusterName: strptr("cluster-2"),
						ClusterArn:  strptr("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-2"),
						Status:      strptr("DRAINING"),
						Tags: []ecsTypes.Tag{
							{Key: strptr("Stage"), Value: strptr("prod")},
						},
					},
				},
			},
			clusterARNs: []string{
				"arn:aws:ecs:us-east-1:123456789012:cluster/cluster-1",
				"arn:aws:ecs:us-east-1:123456789012:cluster/cluster-2",
			},
			expected: map[string]ecsTypes.Cluster{
				"arn:aws:ecs:us-east-1:123456789012:cluster/cluster-1": {
					ClusterName: strptr("cluster-1"),
					ClusterArn:  strptr("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-1"),
					Status:      strptr("ACTIVE"),
				},
				"arn:aws:ecs:us-east-1:123456789012:cluster/cluster-2": {
					ClusterName: strptr("cluster-2"),
					ClusterArn:  strptr("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-2"),
					Status:      strptr("DRAINING"),
					Tags: []ecsTypes.Tag{
						{Key: strptr("Stage"), Value: strptr("prod")},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockECSClient(tt.ecsData)

			d := &ECSDiscovery{
				ecs: client,
				cfg: &ECSSDConfig{
					Region:             tt.ecsData.region,
					RequestConcurrency: 10,
				},
			}

			clusterMap, err := d.describeClusters(ctx, tt.clusterARNs)
			require.NoError(t, err)
			require.Equal(t, tt.expected, clusterMap)
		})
	}
}

func TestECSDiscoveryListServiceARNs(t *testing.T) {
	ctx := context.Background()

	// iterate through the test cases
	for _, tt := range []struct {
		name        string
		ecsData     *ecsDataStore
		clusterARNs []string
		expected    map[string][]string
	}{
		{
			name: "SingleClusterWithServices",
			ecsData: &ecsDataStore{
				region: "us-west-2",
				clusters: []ecsTypes.Cluster{
					{
						ClusterName: strptr("test-cluster"),
						ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:      strptr("ACTIVE"),
					},
				},
				services: []ecsTypes.Service{
					{
						ServiceName: strptr("web-service"),
						ServiceArn:  strptr("arn:aws:ecs:us-west-2:123456789012:service/test-cluster/web-service"),
						ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:      strptr("RUNNING"),
					},
					{
						ServiceName: strptr("api-service"),
						ServiceArn:  strptr("arn:aws:ecs:us-west-2:123456789012:service/test-cluster/api-service"),
						ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:      strptr("RUNNING"),
					},
					{
						// this is to test the old arn format without the cluster name in the service arn
						// https://docs.aws.amazon.com/AmazonECS/latest/developerguide/service-arn-migration.html
						ServiceName: strptr("old-api-service"),
						ServiceArn:  strptr("arn:aws:ecs:us-west-2:123456789012:service/old-api-service"),
						ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:      strptr("RUNNING"),
					},
				},
			},
			clusterARNs: []string{"arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"},
			expected: map[string][]string{
				"arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster": {
					"arn:aws:ecs:us-west-2:123456789012:service/test-cluster/web-service",
					"arn:aws:ecs:us-west-2:123456789012:service/test-cluster/api-service",
					"arn:aws:ecs:us-west-2:123456789012:service/old-api-service",
				},
			},
		},
		{
			name: "MultipleClustesWithServices",
			ecsData: &ecsDataStore{
				region: "us-east-1",
				clusters: []ecsTypes.Cluster{
					{
						ClusterName: strptr("cluster-1"),
						ClusterArn:  strptr("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-1"),
						Status:      strptr("ACTIVE"),
					},
					{
						ClusterName: strptr("cluster-2"),
						ClusterArn:  strptr("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-2"),
						Status:      strptr("ACTIVE"),
					},
				},
				services: []ecsTypes.Service{
					{
						ServiceName: strptr("service-1"),
						ServiceArn:  strptr("arn:aws:ecs:us-east-1:123456789012:service/cluster-1/service-1"),
						ClusterArn:  strptr("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-1"),
						Status:      strptr("RUNNING"),
					},
					{
						ServiceName: strptr("service-2"),
						ServiceArn:  strptr("arn:aws:ecs:us-east-1:123456789012:service/cluster-2/service-2"),
						ClusterArn:  strptr("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-2"),
						Status:      strptr("RUNNING"),
					},
				},
			},
			clusterARNs: []string{
				"arn:aws:ecs:us-east-1:123456789012:cluster/cluster-1",
				"arn:aws:ecs:us-east-1:123456789012:cluster/cluster-2",
			},
			expected: map[string][]string{
				"arn:aws:ecs:us-east-1:123456789012:cluster/cluster-1": {
					"arn:aws:ecs:us-east-1:123456789012:service/cluster-1/service-1",
				},
				"arn:aws:ecs:us-east-1:123456789012:cluster/cluster-2": {
					"arn:aws:ecs:us-east-1:123456789012:service/cluster-2/service-2",
				},
			},
		},
		{
			name: "ClusterWithNoServices",
			ecsData: &ecsDataStore{
				region: "us-west-2",
				clusters: []ecsTypes.Cluster{
					{
						ClusterName: strptr("empty-cluster"),
						ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/empty-cluster"),
						Status:      strptr("ACTIVE"),
					},
				},
				services: []ecsTypes.Service{},
			},
			clusterARNs: []string{"arn:aws:ecs:us-west-2:123456789012:cluster/empty-cluster"},
			expected: map[string][]string{
				"arn:aws:ecs:us-west-2:123456789012:cluster/empty-cluster": nil,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockECSClient(tt.ecsData)

			d := &ECSDiscovery{
				ecs: client,
				cfg: &ECSSDConfig{
					Region:             tt.ecsData.region,
					RequestConcurrency: 1,
				},
			}

			serviceMap, err := d.listServiceARNs(ctx, tt.clusterARNs)
			require.NoError(t, err)
			require.Equal(t, tt.expected, serviceMap)
		})
	}
}

func TestECSDiscoveryDescribeServices(t *testing.T) {
	ctx := context.Background()

	// iterate through the test cases
	for _, tt := range []struct {
		name                  string
		ecsData               *ecsDataStore
		clusterServiceARNsMap map[string][]string
		expected              map[string][]ecsTypes.Service
	}{
		{
			name: "SingleClusterServices",
			ecsData: &ecsDataStore{
				region: "us-west-2",
				services: []ecsTypes.Service{
					{
						ServiceName:    strptr("web-service"),
						ServiceArn:     strptr("arn:aws:ecs:us-west-2:123456789012:service/test-cluster/web-service"),
						ClusterArn:     strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:         strptr("RUNNING"),
						TaskDefinition: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/web-task:1"),
						Tags: []ecsTypes.Tag{
							{Key: strptr("Environment"), Value: strptr("production")},
						},
					},
					{
						ServiceName:    strptr("api-service"),
						ServiceArn:     strptr("arn:aws:ecs:us-west-2:123456789012:service/test-cluster/api-service"),
						ClusterArn:     strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:         strptr("RUNNING"),
						TaskDefinition: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/api-task:2"),
					},
				},
			},
			clusterServiceARNsMap: map[string][]string{
				"arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster": {
					"arn:aws:ecs:us-west-2:123456789012:service/test-cluster/web-service",
					"arn:aws:ecs:us-west-2:123456789012:service/test-cluster/api-service",
				},
			},
			expected: map[string][]ecsTypes.Service{
				"arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster": {
					{
						ServiceName:    strptr("web-service"),
						ServiceArn:     strptr("arn:aws:ecs:us-west-2:123456789012:service/test-cluster/web-service"),
						ClusterArn:     strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:         strptr("RUNNING"),
						TaskDefinition: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/web-task:1"),
						Tags: []ecsTypes.Tag{
							{Key: strptr("Environment"), Value: strptr("production")},
						},
					},
					{
						ServiceName:    strptr("api-service"),
						ServiceArn:     strptr("arn:aws:ecs:us-west-2:123456789012:service/test-cluster/api-service"),
						ClusterArn:     strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:         strptr("RUNNING"),
						TaskDefinition: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/api-task:2"),
					},
				},
			},
		},
		{
			name: "MultipleClustersServices",
			ecsData: &ecsDataStore{
				region: "us-east-1",
				services: []ecsTypes.Service{
					{
						ServiceName:    strptr("service-1"),
						ServiceArn:     strptr("arn:aws:ecs:us-east-1:123456789012:service/cluster-1/service-1"),
						ClusterArn:     strptr("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-1"),
						Status:         strptr("RUNNING"),
						TaskDefinition: strptr("arn:aws:ecs:us-east-1:123456789012:task-definition/task-1:1"),
					},
					{
						ServiceName:    strptr("service-2"),
						ServiceArn:     strptr("arn:aws:ecs:us-east-1:123456789012:service/cluster-2/service-2"),
						ClusterArn:     strptr("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-2"),
						Status:         strptr("DRAINING"),
						TaskDefinition: strptr("arn:aws:ecs:us-east-1:123456789012:task-definition/task-2:1"),
					},
				},
			},
			clusterServiceARNsMap: map[string][]string{
				"arn:aws:ecs:us-east-1:123456789012:cluster/cluster-1": {
					"arn:aws:ecs:us-east-1:123456789012:service/cluster-1/service-1",
				},
				"arn:aws:ecs:us-east-1:123456789012:cluster/cluster-2": {
					"arn:aws:ecs:us-east-1:123456789012:service/cluster-2/service-2",
				},
			},
			expected: map[string][]ecsTypes.Service{
				"arn:aws:ecs:us-east-1:123456789012:cluster/cluster-1": {
					{
						ServiceName:    strptr("service-1"),
						ServiceArn:     strptr("arn:aws:ecs:us-east-1:123456789012:service/cluster-1/service-1"),
						ClusterArn:     strptr("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-1"),
						Status:         strptr("RUNNING"),
						TaskDefinition: strptr("arn:aws:ecs:us-east-1:123456789012:task-definition/task-1:1"),
					},
				},
				"arn:aws:ecs:us-east-1:123456789012:cluster/cluster-2": {
					{
						ServiceName:    strptr("service-2"),
						ServiceArn:     strptr("arn:aws:ecs:us-east-1:123456789012:service/cluster-2/service-2"),
						ClusterArn:     strptr("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-2"),
						Status:         strptr("DRAINING"),
						TaskDefinition: strptr("arn:aws:ecs:us-east-1:123456789012:task-definition/task-2:1"),
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockECSClient(tt.ecsData)

			d := &ECSDiscovery{
				ecs: client,
				cfg: &ECSSDConfig{
					Region:             tt.ecsData.region,
					RequestConcurrency: 1,
				},
			}

			serviceMap, err := d.describeServices(ctx, tt.clusterServiceARNsMap)
			require.NoError(t, err)
			require.Equal(t, tt.expected, serviceMap)
		})
	}
}

func TestECSDiscoveryListTaskARNs(t *testing.T) {
	ctx := context.Background()

	// iterate through the test cases
	for _, tt := range []struct {
		name     string
		ecsData  *ecsDataStore
		services []ecsTypes.Service
		expected map[string][]string
	}{
		{
			name: "ServicesWithTasks",
			ecsData: &ecsDataStore{
				region: "us-west-2",
				tasks: []ecsTypes.Task{
					{
						TaskArn:           strptr("arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-1"),
						ClusterArn:        strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Group:             strptr("service:web-service"),
						TaskDefinitionArn: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/web-task:1"),
					},
					{
						TaskArn:           strptr("arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-2"),
						ClusterArn:        strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Group:             strptr("service:web-service"),
						TaskDefinitionArn: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/web-task:1"),
					},
					{
						TaskArn:           strptr("arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-3"),
						ClusterArn:        strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Group:             strptr("service:api-service"),
						TaskDefinitionArn: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/api-task:2"),
					},
				},
			},
			services: []ecsTypes.Service{
				{
					ServiceName: strptr("web-service"),
					ServiceArn:  strptr("arn:aws:ecs:us-west-2:123456789012:service/test-cluster/web-service"),
					ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
					Status:      strptr("RUNNING"),
				},
				{
					ServiceName: strptr("api-service"),
					ServiceArn:  strptr("arn:aws:ecs:us-west-2:123456789012:service/test-cluster/api-service"),
					ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
					Status:      strptr("RUNNING"),
				},
			},
			expected: map[string][]string{
				"arn:aws:ecs:us-west-2:123456789012:service/test-cluster/web-service": {
					"arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-1",
					"arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-2",
				},
				"arn:aws:ecs:us-west-2:123456789012:service/test-cluster/api-service": {
					"arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-3",
				},
			},
		},
		{
			name: "ServiceWithNoTasks",
			ecsData: &ecsDataStore{
				region: "us-west-2",
				tasks:  []ecsTypes.Task{},
			},
			services: []ecsTypes.Service{
				{
					ServiceName: strptr("empty-service"),
					ServiceArn:  strptr("arn:aws:ecs:us-west-2:123456789012:service/test-cluster/empty-service"),
					ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
					Status:      strptr("RUNNING"),
				},
			},
			expected: map[string][]string{
				"arn:aws:ecs:us-west-2:123456789012:service/test-cluster/empty-service": nil,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockECSClient(tt.ecsData)

			d := &ECSDiscovery{
				ecs: client,
				cfg: &ECSSDConfig{
					Region:             tt.ecsData.region,
					RequestConcurrency: 1,
				},
			}

			taskMap, err := d.listTaskARNs(ctx, tt.services)
			require.NoError(t, err)
			require.Equal(t, tt.expected, taskMap)
		})
	}
}

func TestECSDiscoveryDescribeTasks(t *testing.T) {
	ctx := context.Background()

	// iterate through the test cases
	for _, tt := range []struct {
		name        string
		ecsData     *ecsDataStore
		clusterARN  string
		taskARNsMap map[string][]string
		expected    map[string][]ecsTypes.Task
	}{
		{
			name: "TasksInCluster",
			ecsData: &ecsDataStore{
				region: "us-west-2",
				tasks: []ecsTypes.Task{
					{
						TaskArn:           strptr("arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-1"),
						ClusterArn:        strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Group:             strptr("service:web-service"),
						TaskDefinitionArn: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/web-task:1"),
						LastStatus:        strptr("RUNNING"),
						Tags: []ecsTypes.Tag{
							{Key: strptr("Environment"), Value: strptr("production")},
						},
					},
					{
						TaskArn:           strptr("arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-2"),
						ClusterArn:        strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Group:             strptr("service:api-service"),
						TaskDefinitionArn: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/api-task:2"),
						LastStatus:        strptr("RUNNING"),
					},
				},
			},
			clusterARN: "arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster",
			taskARNsMap: map[string][]string{
				"arn:aws:ecs:us-west-2:123456789012:service/test-cluster/web-service": {
					"arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-1",
				},
				"arn:aws:ecs:us-west-2:123456789012:service/test-cluster/api-service": {
					"arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-2",
				},
			},
			expected: map[string][]ecsTypes.Task{
				"arn:aws:ecs:us-west-2:123456789012:service/test-cluster/web-service": {
					{
						TaskArn:           strptr("arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-1"),
						ClusterArn:        strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Group:             strptr("service:web-service"),
						TaskDefinitionArn: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/web-task:1"),
						LastStatus:        strptr("RUNNING"),
						Tags: []ecsTypes.Tag{
							{Key: strptr("Environment"), Value: strptr("production")},
						},
					},
				},
				"arn:aws:ecs:us-west-2:123456789012:service/test-cluster/api-service": {
					{
						TaskArn:           strptr("arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-2"),
						ClusterArn:        strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Group:             strptr("service:api-service"),
						TaskDefinitionArn: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/api-task:2"),
						LastStatus:        strptr("RUNNING"),
					},
				},
			},
		},
		{
			name: "EmptyTaskARNsMap",
			ecsData: &ecsDataStore{
				region: "us-west-2",
				tasks:  []ecsTypes.Task{},
			},
			clusterARN:  "arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster",
			taskARNsMap: map[string][]string{},
			expected:    map[string][]ecsTypes.Task{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockECSClient(tt.ecsData)

			d := &ECSDiscovery{
				ecs: client,
				cfg: &ECSSDConfig{
					Region:             tt.ecsData.region,
					RequestConcurrency: 1,
				},
			}

			taskMap, err := d.describeTasks(ctx, tt.clusterARN, tt.taskARNsMap)
			require.NoError(t, err)
			require.Equal(t, tt.expected, taskMap)
		})
	}
}

func TestECSDiscoveryRefresh(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		ecsData  *ecsDataStore
		expected []*targetgroup.Group
	}{
		{
			name: "SingleClusterWithTasks",
			ecsData: &ecsDataStore{
				region: "us-west-2",
				clusters: []ecsTypes.Cluster{
					{
						ClusterName: strptr("test-cluster"),
						ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:      strptr("ACTIVE"),
						Tags: []ecsTypes.Tag{
							{Key: strptr("Environment"), Value: strptr("test")},
						},
					},
				},
				services: []ecsTypes.Service{
					{
						ServiceName: strptr("web-service"),
						ServiceArn:  strptr("arn:aws:ecs:us-west-2:123456789012:service/test-cluster/web-service"),
						ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:      strptr("ACTIVE"),
						Tags: []ecsTypes.Tag{
							{Key: strptr("App"), Value: strptr("web")},
						},
					},
				},
				tasks: []ecsTypes.Task{
					{
						TaskArn:           strptr("arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-1"),
						ClusterArn:        strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						TaskDefinitionArn: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/web-task:1"),
						Group:             strptr("service:web-service"),
						LaunchType:        ecsTypes.LaunchTypeFargate,
						LastStatus:        strptr("RUNNING"),
						DesiredStatus:     strptr("RUNNING"),
						HealthStatus:      ecsTypes.HealthStatusHealthy,
						AvailabilityZone:  strptr("us-west-2a"),
						PlatformFamily:    strptr("Linux"),
						PlatformVersion:   strptr("1.4.0"),
						Attachments: []ecsTypes.Attachment{
							{
								Type: strptr("ElasticNetworkInterface"),
								Details: []ecsTypes.KeyValuePair{
									{Name: strptr("subnetId"), Value: strptr("subnet-12345")},
									{Name: strptr("privateIPv4Address"), Value: strptr("10.0.1.100")},
								},
							},
						},
						Tags: []ecsTypes.Tag{
							{Key: strptr("Version"), Value: strptr("v1.0")},
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "us-west-2",
					Targets: []model.LabelSet{
						{
							model.AddressLabel:                   model.LabelValue("10.0.1.100:80"),
							"__meta_ecs_cluster":                 model.LabelValue("test-cluster"),
							"__meta_ecs_cluster_arn":             model.LabelValue("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
							"__meta_ecs_service":                 model.LabelValue("web-service"),
							"__meta_ecs_service_arn":             model.LabelValue("arn:aws:ecs:us-west-2:123456789012:service/test-cluster/web-service"),
							"__meta_ecs_service_status":          model.LabelValue("ACTIVE"),
							"__meta_ecs_task_group":              model.LabelValue("service:web-service"),
							"__meta_ecs_task_arn":                model.LabelValue("arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-1"),
							"__meta_ecs_task_definition":         model.LabelValue("arn:aws:ecs:us-west-2:123456789012:task-definition/web-task:1"),
							"__meta_ecs_region":                  model.LabelValue("us-west-2"),
							"__meta_ecs_availability_zone":       model.LabelValue("us-west-2a"),
							"__meta_ecs_subnet_id":               model.LabelValue("subnet-12345"),
							"__meta_ecs_ip_address":              model.LabelValue("10.0.1.100"),
							"__meta_ecs_launch_type":             model.LabelValue("FARGATE"),
							"__meta_ecs_desired_status":          model.LabelValue("RUNNING"),
							"__meta_ecs_last_status":             model.LabelValue("RUNNING"),
							"__meta_ecs_health_status":           model.LabelValue("HEALTHY"),
							"__meta_ecs_platform_family":         model.LabelValue("Linux"),
							"__meta_ecs_platform_version":        model.LabelValue("1.4.0"),
							"__meta_ecs_tag_cluster_Environment": model.LabelValue("test"),
							"__meta_ecs_tag_service_App":         model.LabelValue("web"),
							"__meta_ecs_tag_task_Version":        model.LabelValue("v1.0"),
						},
					},
				},
			},
		},
		{
			name: "NoTasks",
			ecsData: &ecsDataStore{
				region: "us-east-1",
				clusters: []ecsTypes.Cluster{
					{
						ClusterName: strptr("empty-cluster"),
						ClusterArn:  strptr("arn:aws:ecs:us-east-1:123456789012:cluster/empty-cluster"),
						Status:      strptr("ACTIVE"),
					},
				},
				services: []ecsTypes.Service{
					{
						ServiceName: strptr("empty-service"),
						ServiceArn:  strptr("arn:aws:ecs:us-east-1:123456789012:service/empty-cluster/empty-service"),
						ClusterArn:  strptr("arn:aws:ecs:us-east-1:123456789012:cluster/empty-cluster"),
						Status:      strptr("ACTIVE"),
					},
				},
				tasks: []ecsTypes.Task{},
			},
			expected: []*targetgroup.Group{
				{
					Source: "us-east-1",
				},
			},
		},
		{
			name: "TaskWithoutENI",
			ecsData: &ecsDataStore{
				region: "us-west-2",
				clusters: []ecsTypes.Cluster{
					{
						ClusterName: strptr("test-cluster"),
						ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:      strptr("ACTIVE"),
					},
				},
				services: []ecsTypes.Service{
					{
						ServiceName: strptr("service-1"),
						ServiceArn:  strptr("arn:aws:ecs:us-west-2:123456789012:service/test-cluster/service-1"),
						ClusterArn:  strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						Status:      strptr("ACTIVE"),
					},
				},
				tasks: []ecsTypes.Task{
					{
						TaskArn:           strptr("arn:aws:ecs:us-west-2:123456789012:task/test-cluster/task-1"),
						ClusterArn:        strptr("arn:aws:ecs:us-west-2:123456789012:cluster/test-cluster"),
						TaskDefinitionArn: strptr("arn:aws:ecs:us-west-2:123456789012:task-definition/task-def:1"),
						Group:             strptr("service:service-1"),
						LaunchType:        ecsTypes.LaunchTypeEc2,
						LastStatus:        strptr("RUNNING"),
						DesiredStatus:     strptr("RUNNING"),
						HealthStatus:      ecsTypes.HealthStatusHealthy,
						AvailabilityZone:  strptr("us-west-2a"),
						// No attachments - should be skipped
						Attachments: []ecsTypes.Attachment{},
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "us-west-2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockECSClient(tt.ecsData)

			d := &ECSDiscovery{
				ecs: client,
				cfg: &ECSSDConfig{
					Region:             tt.ecsData.region,
					Port:               80,
					RequestConcurrency: 1,
				},
			}

			groups, err := d.refresh(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.expected, groups)
		})
	}
}

// ECS client mock.
type mockECSClient struct {
	ecsData ecsDataStore
}

func newMockECSClient(ecsData *ecsDataStore) *mockECSClient {
	client := mockECSClient{
		ecsData: *ecsData,
	}
	return &client
}

func (m *mockECSClient) ListClusters(_ context.Context, _ *ecs.ListClustersInput, _ ...func(*ecs.Options)) (*ecs.ListClustersOutput, error) {
	clusterArns := make([]string, 0, len(m.ecsData.clusters))
	for _, cluster := range m.ecsData.clusters {
		clusterArns = append(clusterArns, *cluster.ClusterArn)
	}

	return &ecs.ListClustersOutput{
		ClusterArns: clusterArns,
	}, nil
}

func (m *mockECSClient) DescribeClusters(_ context.Context, input *ecs.DescribeClustersInput, _ ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
	var clusters []ecsTypes.Cluster
	for _, clusterArn := range input.Clusters {
		for _, cluster := range m.ecsData.clusters {
			if *cluster.ClusterArn == clusterArn {
				clusters = append(clusters, cluster)
				break
			}
		}
	}

	return &ecs.DescribeClustersOutput{
		Clusters: clusters,
	}, nil
}

func (m *mockECSClient) ListServices(_ context.Context, input *ecs.ListServicesInput, _ ...func(*ecs.Options)) (*ecs.ListServicesOutput, error) {
	var serviceArns []string
	for _, service := range m.ecsData.services {
		if *service.ClusterArn == *input.Cluster {
			serviceArns = append(serviceArns, *service.ServiceArn)
		}
	}

	return &ecs.ListServicesOutput{
		ServiceArns: serviceArns,
	}, nil
}

func (m *mockECSClient) DescribeServices(_ context.Context, input *ecs.DescribeServicesInput, _ ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	var services []ecsTypes.Service
	for _, serviceArn := range input.Services {
		for _, service := range m.ecsData.services {
			if *service.ServiceArn == serviceArn && *service.ClusterArn == *input.Cluster {
				services = append(services, service)
				break
			}
		}
	}

	return &ecs.DescribeServicesOutput{
		Services: services,
	}, nil
}

func (m *mockECSClient) ListTasks(_ context.Context, input *ecs.ListTasksInput, _ ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	var taskArns []string
	for _, task := range m.ecsData.tasks {
		if *task.ClusterArn == *input.Cluster {
			// If ServiceName is specified, filter by service
			if input.ServiceName != nil {
				expectedGroup := "service:" + *input.ServiceName
				if task.Group != nil && *task.Group == expectedGroup {
					taskArns = append(taskArns, *task.TaskArn)
				}
			} else {
				taskArns = append(taskArns, *task.TaskArn)
			}
		}
	}

	return &ecs.ListTasksOutput{
		TaskArns: taskArns,
	}, nil
}

func (m *mockECSClient) DescribeTasks(_ context.Context, input *ecs.DescribeTasksInput, _ ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	var tasks []ecsTypes.Task
	for _, taskArn := range input.Tasks {
		for _, task := range m.ecsData.tasks {
			if *task.TaskArn == taskArn && *task.ClusterArn == *input.Cluster {
				tasks = append(tasks, task)
				break
			}
		}
	}

	return &ecs.DescribeTasksOutput{
		Tasks: tasks,
	}, nil
}
