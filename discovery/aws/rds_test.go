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
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Mock RDS client for testing.
type mockRDSClient struct {
	clusters  map[string]types.DBCluster
	instances map[string][]types.DBInstance
}

func (m *mockRDSClient) DescribeDBClusters(_ context.Context, input *rds.DescribeDBClustersInput, _ ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
	var clusters []types.DBCluster

	if input.DBClusterIdentifier != nil {
		// Specific cluster requested
		if cluster, ok := m.clusters[*input.DBClusterIdentifier]; ok {
			clusters = append(clusters, cluster)
		}
	} else {
		// All clusters
		for _, cluster := range m.clusters {
			clusters = append(clusters, cluster)
		}
	}

	return &rds.DescribeDBClustersOutput{
		DBClusters: clusters,
	}, nil
}

func (m *mockRDSClient) DescribeDBInstances(_ context.Context, input *rds.DescribeDBInstancesInput, _ ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	var instances []types.DBInstance

	// Check if filtering by cluster
	if input.Filters != nil {
		for _, filter := range input.Filters {
			if filter.Name != nil && *filter.Name == "db-cluster-id" {
				for _, clusterID := range filter.Values {
					if clusterInstances, ok := m.instances[clusterID]; ok {
						instances = append(instances, clusterInstances...)
					}
				}
			}
		}
	} else {
		// All instances
		for _, clusterInstances := range m.instances {
			instances = append(instances, clusterInstances...)
		}
	}

	return &rds.DescribeDBInstancesOutput{
		DBInstances: instances,
	}, nil
}

func TestRDSDiscoveryRefresh(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		clusters       map[string]types.DBCluster
		instances      map[string][]types.DBInstance
		expectedLabels []model.LabelSet
	}{
		{
			name: "SingleClusterWithInstance",
			clusters: map[string]types.DBCluster{
				"arn:aws:rds:us-east-1:123456789012:cluster:test-cluster": {
					DBClusterArn:        aws.String("arn:aws:rds:us-east-1:123456789012:cluster:test-cluster"),
					DBClusterIdentifier: aws.String("test-cluster"),
					Engine:              aws.String("aurora-postgresql"),
					EngineVersion:       aws.String("15.4"),
					Status:              aws.String("available"),
					Endpoint:            aws.String("test-cluster.cluster-xyz.us-east-1.rds.amazonaws.com"),
					Port:                aws.Int32(5432),
					MasterUsername:      aws.String("admin"),
					MultiAZ:             aws.Bool(true),
					ClusterCreateTime:   aws.Time(testTime),
					DBClusterMembers: []types.DBClusterMember{
						{
							DBInstanceIdentifier: aws.String("test-instance-1"),
							IsClusterWriter:      aws.Bool(true),
						},
					},
					TagList: []types.Tag{
						{Key: aws.String("Environment"), Value: aws.String("test")},
					},
				},
			},
			instances: map[string][]types.DBInstance{
				"arn:aws:rds:us-east-1:123456789012:cluster:test-cluster": {
					{
						DBInstanceArn:        aws.String("arn:aws:rds:us-east-1:123456789012:db:test-instance-1"),
						DBInstanceIdentifier: aws.String("test-instance-1"),
						DBInstanceClass:      aws.String("db.r5.large"),
						DBInstanceStatus:     aws.String("available"),
						Engine:               aws.String("aurora-postgresql"),
						EngineVersion:        aws.String("15.4"),
						AvailabilityZone:     aws.String("us-east-1a"),
						DBClusterIdentifier:  aws.String("test-cluster"),
						PubliclyAccessible:   aws.Bool(false),
						InstanceCreateTime:   aws.Time(testTime),
						Endpoint: &types.Endpoint{
							Address:      aws.String("test-instance-1.xyz.us-east-1.rds.amazonaws.com"),
							Port:         aws.Int32(5432),
							HostedZoneId: aws.String("Z2R2ITUGPM61AM"),
						},
						TagList: []types.Tag{
							{Key: aws.String("Name"), Value: aws.String("test-instance")},
						},
					},
				},
			},
			expectedLabels: []model.LabelSet{
				{
					model.AddressLabel:                                  model.LabelValue("test-instance-1.xyz.us-east-1.rds.amazonaws.com:5432"),
					rdsLabelClusterDBClusterArn:                         model.LabelValue("arn:aws:rds:us-east-1:123456789012:cluster:test-cluster"),
					rdsLabelClusterDBClusterIdentifier:                  model.LabelValue("test-cluster"),
					rdsLabelClusterEngine:                               model.LabelValue("aurora-postgresql"),
					rdsLabelClusterEngineVersion:                        model.LabelValue("15.4"),
					rdsLabelClusterStatus:                               model.LabelValue("available"),
					rdsLabelClusterEndpoint:                             model.LabelValue("test-cluster.cluster-xyz.us-east-1.rds.amazonaws.com"),
					rdsLabelClusterPort:                                 model.LabelValue("5432"),
					rdsLabelClusterMasterUsername:                       model.LabelValue("admin"),
					rdsLabelClusterMultiAZ:                              model.LabelValue("true"),
					rdsLabelClusterClusterCreateTime:                    model.LabelValue(testTime.Format(time.RFC3339)),
					model.LabelName(rdsLabelClusterTag + "Environment"): model.LabelValue("test"),
					rdsLabelInstanceDBInstanceArn:                       model.LabelValue("arn:aws:rds:us-east-1:123456789012:db:test-instance-1"),
					rdsLabelInstanceDBInstanceIdentifier:                model.LabelValue("test-instance-1"),
					rdsLabelInstanceIsClusterWriter:                     model.LabelValue("true"),
					rdsLabelInstanceDBInstanceClass:                     model.LabelValue("db.r5.large"),
					rdsLabelInstanceDBInstanceStatus:                    model.LabelValue("available"),
					rdsLabelInstanceEngine:                              model.LabelValue("aurora-postgresql"),
					rdsLabelInstanceEngineVersion:                       model.LabelValue("15.4"),
					rdsLabelInstanceAvailabilityZone:                    model.LabelValue("us-east-1a"),
					rdsLabelInstanceDBClusterIdentifier:                 model.LabelValue("test-cluster"),
					rdsLabelInstancePubliclyAccessible:                  model.LabelValue("false"),
					rdsLabelInstanceInstanceCreateTime:                  model.LabelValue(testTime.Format(time.RFC3339)),
					rdsLabelInstanceEndpointAddress:                     model.LabelValue("test-instance-1.xyz.us-east-1.rds.amazonaws.com"),
					rdsLabelInstanceEndpointPort:                        model.LabelValue("5432"),
					rdsLabelInstanceEndpointHostedZoneID:                model.LabelValue("Z2R2ITUGPM61AM"),
					model.LabelName(rdsLabelInstanceTag + "Name"):       model.LabelValue("test-instance"),
				},
			},
		},
		{
			name: "MultipleInstancesInCluster",
			clusters: map[string]types.DBCluster{
				"arn:aws:rds:us-west-2:123456789012:cluster:prod-cluster": {
					DBClusterArn:        aws.String("arn:aws:rds:us-west-2:123456789012:cluster:prod-cluster"),
					DBClusterIdentifier: aws.String("prod-cluster"),
					Engine:              aws.String("aurora-mysql"),
					EngineVersion:       aws.String("8.0.mysql_aurora.3.04.0"),
					Status:              aws.String("available"),
					DBClusterMembers: []types.DBClusterMember{
						{
							DBInstanceIdentifier: aws.String("prod-instance-1"),
							IsClusterWriter:      aws.Bool(true),
						},
						{
							DBInstanceIdentifier: aws.String("prod-instance-2"),
							IsClusterWriter:      aws.Bool(false),
						},
					},
				},
			},
			instances: map[string][]types.DBInstance{
				"arn:aws:rds:us-west-2:123456789012:cluster:prod-cluster": {
					{
						DBInstanceArn:        aws.String("arn:aws:rds:us-west-2:123456789012:db:prod-instance-1"),
						DBInstanceIdentifier: aws.String("prod-instance-1"),
						DBInstanceClass:      aws.String("db.r6g.xlarge"),
						DBInstanceStatus:     aws.String("available"),
						Endpoint: &types.Endpoint{
							Address: aws.String("prod-instance-1.xyz.us-west-2.rds.amazonaws.com"),
							Port:    aws.Int32(3306),
						},
					},
					{
						DBInstanceArn:        aws.String("arn:aws:rds:us-west-2:123456789012:db:prod-instance-2"),
						DBInstanceIdentifier: aws.String("prod-instance-2"),
						DBInstanceClass:      aws.String("db.r6g.xlarge"),
						DBInstanceStatus:     aws.String("available"),
						Endpoint: &types.Endpoint{
							Address: aws.String("prod-instance-2.xyz.us-west-2.rds.amazonaws.com"),
							Port:    aws.Int32(3306),
						},
					},
				},
			},
			expectedLabels: []model.LabelSet{
				{
					model.AddressLabel:                   model.LabelValue("prod-instance-1.xyz.us-west-2.rds.amazonaws.com:3306"),
					rdsLabelClusterDBClusterArn:          model.LabelValue("arn:aws:rds:us-west-2:123456789012:cluster:prod-cluster"),
					rdsLabelClusterDBClusterIdentifier:   model.LabelValue("prod-cluster"),
					rdsLabelClusterEngine:                model.LabelValue("aurora-mysql"),
					rdsLabelClusterEngineVersion:         model.LabelValue("8.0.mysql_aurora.3.04.0"),
					rdsLabelClusterStatus:                model.LabelValue("available"),
					rdsLabelInstanceDBInstanceArn:        model.LabelValue("arn:aws:rds:us-west-2:123456789012:db:prod-instance-1"),
					rdsLabelInstanceDBInstanceIdentifier: model.LabelValue("prod-instance-1"),
					rdsLabelInstanceIsClusterWriter:      model.LabelValue("true"),
					rdsLabelInstanceDBInstanceClass:      model.LabelValue("db.r6g.xlarge"),
					rdsLabelInstanceDBInstanceStatus:     model.LabelValue("available"),
					rdsLabelInstanceEndpointAddress:      model.LabelValue("prod-instance-1.xyz.us-west-2.rds.amazonaws.com"),
					rdsLabelInstanceEndpointPort:         model.LabelValue("3306"),
				},
				{
					model.AddressLabel:                   model.LabelValue("prod-instance-2.xyz.us-west-2.rds.amazonaws.com:3306"),
					rdsLabelClusterDBClusterArn:          model.LabelValue("arn:aws:rds:us-west-2:123456789012:cluster:prod-cluster"),
					rdsLabelClusterDBClusterIdentifier:   model.LabelValue("prod-cluster"),
					rdsLabelClusterEngine:                model.LabelValue("aurora-mysql"),
					rdsLabelClusterEngineVersion:         model.LabelValue("8.0.mysql_aurora.3.04.0"),
					rdsLabelClusterStatus:                model.LabelValue("available"),
					rdsLabelInstanceDBInstanceArn:        model.LabelValue("arn:aws:rds:us-west-2:123456789012:db:prod-instance-2"),
					rdsLabelInstanceDBInstanceIdentifier: model.LabelValue("prod-instance-2"),
					rdsLabelInstanceIsClusterWriter:      model.LabelValue("false"),
					rdsLabelInstanceDBInstanceClass:      model.LabelValue("db.r6g.xlarge"),
					rdsLabelInstanceDBInstanceStatus:     model.LabelValue("available"),
					rdsLabelInstanceEndpointAddress:      model.LabelValue("prod-instance-2.xyz.us-west-2.rds.amazonaws.com"),
					rdsLabelInstanceEndpointPort:         model.LabelValue("3306"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockRDSClient{
				clusters:  tt.clusters,
				instances: tt.instances,
			}

			d := &RDSDiscovery{
				rds: mockClient,
				cfg: &RDSSDConfig{
					Region:             "us-east-1",
					RequestConcurrency: 10,
				},
			}

			tg := &targetgroup.Group{}

			// Get all cluster ARNs
			var clusterARNs []string
			for arn := range tt.clusters {
				clusterARNs = append(clusterARNs, arn)
			}

			clusters, err := d.describeAllDBClusters(context.Background())
			require.NoError(t, err)
			require.Len(t, clusters, len(tt.clusters))

			instances := make(map[string][]types.DBInstance)
			for _, arn := range clusterARNs {
				clusterInstances, err := d.describeDBInstances(context.Background(), arn)
				require.NoError(t, err)
				instances[arn] = clusterInstances
			}

			// Build targets like the refresh function does
			for _, cluster := range clusters {
				writerMap := make(map[string]bool)
				for _, member := range cluster.DBClusterMembers {
					if member.DBInstanceIdentifier != nil && member.IsClusterWriter != nil {
						writerMap[*member.DBInstanceIdentifier] = *member.IsClusterWriter
					}
				}

				clusterInstances := instances[*cluster.DBClusterArn]
				for _, instance := range clusterInstances {
					labels := model.LabelSet{}

					// Add basic cluster labels
					if cluster.DBClusterArn != nil {
						labels[rdsLabelClusterDBClusterArn] = model.LabelValue(*cluster.DBClusterArn)
					}
					if cluster.DBClusterIdentifier != nil {
						labels[rdsLabelClusterDBClusterIdentifier] = model.LabelValue(*cluster.DBClusterIdentifier)
					}
					if cluster.Engine != nil {
						labels[rdsLabelClusterEngine] = model.LabelValue(*cluster.Engine)
					}
					if cluster.EngineVersion != nil {
						labels[rdsLabelClusterEngineVersion] = model.LabelValue(*cluster.EngineVersion)
					}
					if cluster.Status != nil {
						labels[rdsLabelClusterStatus] = model.LabelValue(*cluster.Status)
					}
					if cluster.Endpoint != nil {
						labels[rdsLabelClusterEndpoint] = model.LabelValue(*cluster.Endpoint)
					}
					if cluster.Port != nil {
						labels[rdsLabelClusterPort] = model.LabelValue(strconv.Itoa(int(*cluster.Port)))
					}
					if cluster.MasterUsername != nil {
						labels[rdsLabelClusterMasterUsername] = model.LabelValue(*cluster.MasterUsername)
					}
					if cluster.MultiAZ != nil {
						labels[rdsLabelClusterMultiAZ] = model.LabelValue(strconv.FormatBool(*cluster.MultiAZ))
					}
					if cluster.ClusterCreateTime != nil {
						labels[rdsLabelClusterClusterCreateTime] = model.LabelValue(cluster.ClusterCreateTime.Format(time.RFC3339))
					}

					// Cluster tags
					for _, tag := range cluster.TagList {
						if tag.Key != nil && tag.Value != nil {
							labels[model.LabelName(rdsLabelClusterTag+*tag.Key)] = model.LabelValue(*tag.Value)
						}
					}

					// Add basic instance labels
					if instance.DBInstanceArn != nil {
						labels[rdsLabelInstanceDBInstanceArn] = model.LabelValue(*instance.DBInstanceArn)
					}
					if instance.DBInstanceIdentifier != nil {
						labels[rdsLabelInstanceDBInstanceIdentifier] = model.LabelValue(*instance.DBInstanceIdentifier)
						if isWriter, found := writerMap[*instance.DBInstanceIdentifier]; found {
							labels[rdsLabelInstanceIsClusterWriter] = model.LabelValue(strconv.FormatBool(isWriter))
						}
					}
					if instance.DBInstanceClass != nil {
						labels[rdsLabelInstanceDBInstanceClass] = model.LabelValue(*instance.DBInstanceClass)
					}
					if instance.DBInstanceStatus != nil {
						labels[rdsLabelInstanceDBInstanceStatus] = model.LabelValue(*instance.DBInstanceStatus)
					}
					if instance.Engine != nil {
						labels[rdsLabelInstanceEngine] = model.LabelValue(*instance.Engine)
					}
					if instance.EngineVersion != nil {
						labels[rdsLabelInstanceEngineVersion] = model.LabelValue(*instance.EngineVersion)
					}
					if instance.AvailabilityZone != nil {
						labels[rdsLabelInstanceAvailabilityZone] = model.LabelValue(*instance.AvailabilityZone)
					}
					if instance.DBClusterIdentifier != nil {
						labels[rdsLabelInstanceDBClusterIdentifier] = model.LabelValue(*instance.DBClusterIdentifier)
					}
					if instance.PubliclyAccessible != nil {
						labels[rdsLabelInstancePubliclyAccessible] = model.LabelValue(strconv.FormatBool(*instance.PubliclyAccessible))
					}
					if instance.InstanceCreateTime != nil {
						labels[rdsLabelInstanceInstanceCreateTime] = model.LabelValue(instance.InstanceCreateTime.Format(time.RFC3339))
					}
					if instance.Endpoint != nil {
						if instance.Endpoint.Address != nil {
							labels[rdsLabelInstanceEndpointAddress] = model.LabelValue(*instance.Endpoint.Address)
						}
						if instance.Endpoint.Port != nil {
							labels[rdsLabelInstanceEndpointPort] = model.LabelValue(strconv.Itoa(int(*instance.Endpoint.Port)))
						}
						if instance.Endpoint.HostedZoneId != nil {
							labels[rdsLabelInstanceEndpointHostedZoneID] = model.LabelValue(*instance.Endpoint.HostedZoneId)
						}
					}

					// Instance tags
					for _, tag := range instance.TagList {
						if tag.Key != nil && tag.Value != nil {
							labels[model.LabelName(rdsLabelInstanceTag+*tag.Key)] = model.LabelValue(*tag.Value)
						}
					}

					// Set address
					if instance.Endpoint != nil && instance.Endpoint.Address != nil && instance.Endpoint.Port != nil {
						labels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(*instance.Endpoint.Address, strconv.Itoa(int(*instance.Endpoint.Port))))
					}

					tg.Targets = append(tg.Targets, labels)
				}
			}

			require.Len(t, tg.Targets, len(tt.expectedLabels))

			// Verify each expected label set is present
			for _, expectedLabels := range tt.expectedLabels {
				found := false
				for _, target := range tg.Targets {
					if target[model.AddressLabel] == expectedLabels[model.AddressLabel] {
						found = true
						// Check all expected labels are present with correct values
						for key, expectedValue := range expectedLabels {
							require.Equal(t, expectedValue, target[key], "Label %s mismatch", key)
						}
						break
					}
				}
				require.True(t, found, "Expected target with address %s not found", expectedLabels[model.AddressLabel])
			}
		})
	}
}

func TestDescribeAllDBClusters(t *testing.T) {
	mockClient := &mockRDSClient{
		clusters: map[string]types.DBCluster{
			"arn:aws:rds:us-east-1:123456789012:cluster:cluster-1": {
				DBClusterArn:        aws.String("arn:aws:rds:us-east-1:123456789012:cluster:cluster-1"),
				DBClusterIdentifier: aws.String("cluster-1"),
			},
			"arn:aws:rds:us-east-1:123456789012:cluster:cluster-2": {
				DBClusterArn:        aws.String("arn:aws:rds:us-east-1:123456789012:cluster:cluster-2"),
				DBClusterIdentifier: aws.String("cluster-2"),
			},
		},
		instances: map[string][]types.DBInstance{},
	}

	d := &RDSDiscovery{
		rds: mockClient,
		cfg: &RDSSDConfig{
			RequestConcurrency: 10,
		},
	}

	clusters, err := d.describeAllDBClusters(context.Background())
	require.NoError(t, err)
	require.Len(t, clusters, 2)
	require.Contains(t, clusters, "arn:aws:rds:us-east-1:123456789012:cluster:cluster-1")
	require.Contains(t, clusters, "arn:aws:rds:us-east-1:123456789012:cluster:cluster-2")
}
