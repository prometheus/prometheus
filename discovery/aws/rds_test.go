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
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

// Mock RDS client for testing.
type mockRDSClient struct {
	clusters  map[string]types.DBCluster
	instances map[string][]types.DBInstance

	// onDescribeDBInstances, when set, runs at the start of every
	// DescribeDBInstances call.
	onDescribeDBInstances func()
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
	if m.onDescribeDBInstances != nil {
		m.onDescribeDBInstances()
	}

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

	for _, filter := range input.Filters {
		if filter.Name == nil || *filter.Name != "engine" {
			continue
		}

		var filtered []types.DBInstance
		for _, inst := range instances {
			if inst.Engine == nil {
				continue
			}
			if slices.Contains(filter.Values, *inst.Engine) {
				filtered = append(filtered, inst)
			}
		}
		instances = filtered
	}

	return &rds.DescribeDBInstancesOutput{
		DBInstances: instances,
	}, nil
}

func TestRDSDiscoveryRefresh(t *testing.T) {
	t.Parallel()
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		clusters       map[string]types.DBCluster
		instances      map[string][]types.DBInstance
		filters        []*Filter
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
					model.AddressLabel:                                  model.LabelValue("test-instance-1.xyz.us-east-1.rds.amazonaws.com:9187"),
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
					model.AddressLabel:                   model.LabelValue("prod-instance-1.xyz.us-west-2.rds.amazonaws.com:9187"),
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
					model.AddressLabel:                   model.LabelValue("prod-instance-2.xyz.us-west-2.rds.amazonaws.com:9187"),
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
		{
			name: "NoInstancesInCluster",
			clusters: map[string]types.DBCluster{
				"arn:aws:rds:us-west-2:123456789012:cluster:prod-cluster": {
					DBClusterArn:        aws.String("arn:aws:rds:us-west-2:123456789012:cluster:prod-cluster"),
					DBClusterIdentifier: aws.String("prod-cluster"),
					Engine:              aws.String("aurora-mysql"),
					EngineVersion:       aws.String("8.0.mysql_aurora.3.04.0"),
					Status:              aws.String("available"),
					DBClusterMembers:    []types.DBClusterMember{},
				},
			},
			instances:      map[string][]types.DBInstance{},
			expectedLabels: []model.LabelSet{},
		},
		{
			name: "FiltersMatchSingleInstance",
			clusters: map[string]types.DBCluster{
				"arn:aws:rds:us-east-1:123456789012:cluster:filter-cluster": {
					DBClusterArn:        aws.String("arn:aws:rds:us-east-1:123456789012:cluster:filter-cluster"),
					DBClusterIdentifier: aws.String("filter-cluster"),
					Engine:              aws.String("aurora-postgresql"),
					Status:              aws.String("available"),
					DBClusterMembers: []types.DBClusterMember{
						{DBInstanceIdentifier: aws.String("filter-instance-1"), IsClusterWriter: aws.Bool(true)},
						{DBInstanceIdentifier: aws.String("filter-instance-2"), IsClusterWriter: aws.Bool(false)},
					},
				},
			},
			instances: map[string][]types.DBInstance{
				"arn:aws:rds:us-east-1:123456789012:cluster:filter-cluster": {
					{
						DBInstanceArn:        aws.String("arn:aws:rds:us-east-1:123456789012:db:filter-instance-1"),
						DBInstanceIdentifier: aws.String("filter-instance-1"),
						DBInstanceClass:      aws.String("db.r6g.large"),
						DBInstanceStatus:     aws.String("available"),
						Engine:               aws.String("aurora-postgresql"),
						Endpoint:             &types.Endpoint{Address: aws.String("filter-instance-1.rds.amazonaws.com"), Port: aws.Int32(5432)},
					},
					{
						DBInstanceArn:        aws.String("arn:aws:rds:us-east-1:123456789012:db:filter-instance-2"),
						DBInstanceIdentifier: aws.String("filter-instance-2"),
						DBInstanceClass:      aws.String("db.r6g.large"),
						DBInstanceStatus:     aws.String("available"),
						Engine:               aws.String("mysql"),
						Endpoint:             &types.Endpoint{Address: aws.String("filter-instance-2.rds.amazonaws.com"), Port: aws.Int32(3306)},
					},
				},
			},
			filters: []*Filter{{Name: "engine", Values: []string{"aurora-postgresql"}}},
			expectedLabels: []model.LabelSet{
				{
					model.AddressLabel:                   model.LabelValue("filter-instance-1.rds.amazonaws.com:9187"),
					rdsLabelClusterDBClusterArn:          model.LabelValue("arn:aws:rds:us-east-1:123456789012:cluster:filter-cluster"),
					rdsLabelClusterDBClusterIdentifier:   model.LabelValue("filter-cluster"),
					rdsLabelClusterEngine:                model.LabelValue("aurora-postgresql"),
					rdsLabelClusterStatus:                model.LabelValue("available"),
					rdsLabelInstanceDBInstanceArn:        model.LabelValue("arn:aws:rds:us-east-1:123456789012:db:filter-instance-1"),
					rdsLabelInstanceDBInstanceIdentifier: model.LabelValue("filter-instance-1"),
					rdsLabelInstanceIsClusterWriter:      model.LabelValue("true"),
					rdsLabelInstanceDBInstanceClass:      model.LabelValue("db.r6g.large"),
					rdsLabelInstanceDBInstanceStatus:     model.LabelValue("available"),
					rdsLabelInstanceEngine:               model.LabelValue("aurora-postgresql"),
					rdsLabelInstanceEndpointAddress:      model.LabelValue("filter-instance-1.rds.amazonaws.com"),
					rdsLabelInstanceEndpointPort:         model.LabelValue("5432"),
				},
			},
		},
		{
			name: "FiltersMatchNoInstances",
			clusters: map[string]types.DBCluster{
				"arn:aws:rds:us-east-1:123456789012:cluster:filter-cluster": {
					DBClusterArn:        aws.String("arn:aws:rds:us-east-1:123456789012:cluster:filter-cluster"),
					DBClusterIdentifier: aws.String("filter-cluster"),
					Engine:              aws.String("aurora-postgresql"),
					Status:              aws.String("available"),
				},
			},
			instances: map[string][]types.DBInstance{
				"arn:aws:rds:us-east-1:123456789012:cluster:filter-cluster": {
					{DBInstanceIdentifier: aws.String("filter-instance-1"), Engine: aws.String("aurora-postgresql")},
					{DBInstanceIdentifier: aws.String("filter-instance-2"), Engine: aws.String("mysql")},
				},
			},
			filters:        []*Filter{{Name: "engine", Values: []string{"sqlserver-ee"}}},
			expectedLabels: []model.LabelSet{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockRDSClient{
				clusters:  tt.clusters,
				instances: tt.instances,
			}

			d := &RDSDiscovery{
				logger: promslog.NewNopLogger(),
				rds:    mockClient,
				cfg: &RDSSDConfig{
					Region:             "us-east-1",
					Port:               9187,
					RequestConcurrency: 10,
					Filters:            tt.filters,
				},
			}

			tgs, err := d.refresh(context.Background())
			require.NoError(t, err)
			require.Len(t, tgs, 1)
			tg := tgs[0]

			require.Len(t, tg.Targets, len(tt.expectedLabels))

			// Every expected target must be present with exactly the expected labels.
			targetsByAddress := make(map[model.LabelValue]model.LabelSet, len(tg.Targets))
			for _, target := range tg.Targets {
				targetsByAddress[target[model.AddressLabel]] = target
			}
			for _, expectedLabels := range tt.expectedLabels {
				address := expectedLabels[model.AddressLabel]
				target, found := targetsByAddress[address]
				require.True(t, found, "Expected target with address %s not found", address)
				require.Equal(t, expectedLabels, target)
			}
		})
	}
}

func TestDescribeAllDBClusters(t *testing.T) {
	t.Parallel()
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

// rdsFixture builds clusters populated the way the RDS API populates a
// provisioned Aurora PostgreSQL cluster, each with instanceCount instances.
func rdsFixture(clusterCount, instanceCount int) (map[string]types.DBCluster, map[string][]types.DBInstance) {
	createTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clusters := make(map[string]types.DBCluster, clusterCount)
	instances := make(map[string][]types.DBInstance, clusterCount)

	for c := range clusterCount {
		clusterID := "aurora-cluster-" + strconv.Itoa(c)
		clusterARN := "arn:aws:rds:us-east-1:123456789012:cluster:" + clusterID

		members := make([]types.DBClusterMember, 0, instanceCount)
		clusterInstances := make([]types.DBInstance, 0, instanceCount)
		for i := range instanceCount {
			instanceID := clusterID + "-instance-" + strconv.Itoa(i)
			members = append(members, types.DBClusterMember{
				DBInstanceIdentifier: aws.String(instanceID),
				IsClusterWriter:      aws.Bool(i == 0),
				PromotionTier:        aws.Int32(int32(i)),
			})
			clusterInstances = append(clusterInstances, types.DBInstance{
				AutoMinorVersionUpgrade:    aws.Bool(true),
				AvailabilityZone:           aws.String("us-east-1a"),
				BackupRetentionPeriod:      aws.Int32(7),
				CopyTagsToSnapshot:         aws.Bool(true),
				DBInstanceArn:              aws.String("arn:aws:rds:us-east-1:123456789012:db:" + instanceID),
				DBInstanceClass:            aws.String("db.r6g.xlarge"),
				DBInstanceIdentifier:       aws.String(instanceID),
				DBInstanceStatus:           aws.String("available"),
				DBClusterIdentifier:        aws.String(clusterID),
				DbiResourceId:              aws.String("db-ABCDEFGHIJKLMNOPQRSTUVWXYZ"),
				DeletionProtection:         aws.Bool(false),
				Endpoint:                   &types.Endpoint{Address: aws.String(instanceID + ".xyz.us-east-1.rds.amazonaws.com"), Port: aws.Int32(5432), HostedZoneId: aws.String("Z2R2ITUGPM61AM")},
				Engine:                     aws.String("aurora-postgresql"),
				EngineVersion:              aws.String("15.4"),
				InstanceCreateTime:         aws.Time(createTime),
				KmsKeyId:                   aws.String("arn:aws:kms:us-east-1:123456789012:key/abcd1234"),
				MonitoringInterval:         aws.Int32(60),
				MonitoringRoleArn:          aws.String("arn:aws:iam::123456789012:role/rds-monitoring-role"),
				MultiAZ:                    aws.Bool(false),
				NetworkType:                aws.String("IPV4"),
				PerformanceInsightsEnabled: aws.Bool(true),
				PreferredBackupWindow:      aws.String("07:00-07:30"),
				PreferredMaintenanceWindow: aws.String("sun:09:00-sun:09:30"),
				PromotionTier:              aws.Int32(int32(i)),
				PubliclyAccessible:         aws.Bool(false),
				StorageEncrypted:           aws.Bool(true),
				StorageType:                aws.String("aurora"),
				TagList: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String(instanceID)},
					{Key: aws.String("Environment"), Value: aws.String("production")},
				},
			})
		}

		clusters[clusterARN] = types.DBCluster{
			ActivityStreamStatus:               types.ActivityStreamStatusStopped,
			AllocatedStorage:                   aws.Int32(1),
			AutoMinorVersionUpgrade:            aws.Bool(true),
			BackupRetentionPeriod:              aws.Int32(7),
			ClusterCreateTime:                  aws.Time(createTime),
			CopyTagsToSnapshot:                 aws.Bool(true),
			CrossAccountClone:                  aws.Bool(false),
			DatabaseName:                       aws.String("appdb"),
			DBClusterArn:                       aws.String(clusterARN),
			DBClusterIdentifier:                aws.String(clusterID),
			DBClusterMembers:                   members,
			DBClusterParameterGroup:            aws.String("default.aurora-postgresql15"),
			DbClusterResourceId:                aws.String("cluster-ABCDEFGHIJKLMNOPQRSTUVWXYZ"),
			DBSubnetGroup:                      aws.String("default-vpc-0123456789abcdef0"),
			DeletionProtection:                 aws.Bool(false),
			EarliestRestorableTime:             aws.Time(createTime),
			Endpoint:                           aws.String(clusterID + ".cluster-xyz.us-east-1.rds.amazonaws.com"),
			Engine:                             aws.String("aurora-postgresql"),
			EngineLifecycleSupport:             aws.String("open-source-rds-extended-support"),
			EngineMode:                         aws.String("provisioned"),
			EngineVersion:                      aws.String("15.4"),
			HostedZoneId:                       aws.String("Z2R2ITUGPM61AM"),
			HttpEndpointEnabled:                aws.Bool(false),
			IAMDatabaseAuthenticationEnabled:   aws.Bool(false),
			KmsKeyId:                           aws.String("arn:aws:kms:us-east-1:123456789012:key/abcd1234"),
			LatestRestorableTime:               aws.Time(createTime),
			LocalWriteForwardingStatus:         types.LocalWriteForwardingStatusDisabled,
			MasterUsername:                     aws.String("postgres"),
			MonitoringInterval:                 aws.Int32(60),
			MonitoringRoleArn:                  aws.String("arn:aws:iam::123456789012:role/rds-monitoring-role"),
			MultiAZ:                            aws.Bool(true),
			NetworkType:                        aws.String("IPV4"),
			PercentProgress:                    aws.String("100"),
			PerformanceInsightsEnabled:         aws.Bool(true),
			PerformanceInsightsRetentionPeriod: aws.Int32(7),
			Port:                               aws.Int32(5432),
			PreferredBackupWindow:              aws.String("07:00-07:30"),
			PreferredMaintenanceWindow:         aws.String("sun:09:00-sun:09:30"),
			PubliclyAccessible:                 aws.Bool(false),
			ReaderEndpoint:                     aws.String(clusterID + ".cluster-ro-xyz.us-east-1.rds.amazonaws.com"),
			Status:                             aws.String("available"),
			StorageEncrypted:                   aws.Bool(true),
			StorageType:                        aws.String("aurora"),
			TagList: []types.Tag{
				{Key: aws.String("Name"), Value: aws.String(clusterID)},
				{Key: aws.String("Environment"), Value: aws.String("production")},
				{Key: aws.String("Team"), Value: aws.String("platform")},
			},
		}
		instances[clusterARN] = clusterInstances
	}

	return clusters, instances
}

func BenchmarkRDSRefresh(b *testing.B) {
	benchmarks := []struct {
		name      string
		clusters  int
		instances int
	}{
		{"1Cluster/1Instance", 1, 1},
		{"1Cluster/16Instances", 1, 16},
		{"20Clusters/4Instances", 20, 4},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			clusters, instances := rdsFixture(bm.clusters, bm.instances)
			d := &RDSDiscovery{
				logger: promslog.NewNopLogger(),
				rds:    &mockRDSClient{clusters: clusters, instances: instances},
				cfg: &RDSSDConfig{
					Region:             "us-east-1",
					Port:               9187,
					RequestConcurrency: 10,
				},
			}

			b.ReportAllocs()
			for b.Loop() {
				tgs, err := d.refresh(context.Background())
				if err != nil {
					b.Fatal(err)
				}
				if len(tgs[0].Targets) != bm.clusters*bm.instances {
					b.Fatalf("got %d targets, want %d", len(tgs[0].Targets), bm.clusters*bm.instances)
				}
			}
		})
	}
}

// BenchmarkRDSRefreshAPILatency models the DescribeDBInstances round trip, which
// dominates a refresh over many clusters and which BenchmarkRDSRefresh cannot
// show because its client answers instantly. The absolute latency is arbitrary;
// the ratio between the two benchmarks is what the number says.
func BenchmarkRDSRefreshAPILatency(b *testing.B) {
	const (
		clusterCount = 20
		roundTrip    = time.Millisecond
	)
	clusters, instances := rdsFixture(clusterCount, 2)

	d := &RDSDiscovery{
		logger: promslog.NewNopLogger(),
		rds: &mockRDSClient{
			clusters:              clusters,
			instances:             instances,
			onDescribeDBInstances: func() { time.Sleep(roundTrip) },
		},
		cfg: &RDSSDConfig{
			Region:             "us-east-1",
			Port:               9187,
			RequestConcurrency: 10,
		},
	}

	b.ReportAllocs()
	for b.Loop() {
		tgs, err := d.refresh(context.Background())
		if err != nil {
			b.Fatal(err)
		}
		if len(tgs[0].Targets) != clusterCount*2 {
			b.Fatalf("got %d targets, want %d", len(tgs[0].Targets), clusterCount*2)
		}
	}
}

func TestRDSDiscoveryDescribesInstancesConcurrently(t *testing.T) {
	t.Parallel()

	const (
		clusterCount = 8
		concurrency  = 4
	)
	clusters, instances := rdsFixture(clusterCount, 1)

	var (
		mu          sync.Mutex
		once        sync.Once
		inFlight    int
		maxInFlight int
	)
	allInFlight := make(chan struct{})

	mockClient := &mockRDSClient{clusters: clusters, instances: instances}
	mockClient.onDescribeDBInstances = func() {
		mu.Lock()
		inFlight++
		maxInFlight = max(maxInFlight, inFlight)
		if inFlight == concurrency {
			once.Do(func() { close(allInFlight) })
		}
		mu.Unlock()

		// Hold every call open until RequestConcurrency of them are in flight.
		// A serial implementation never gets past the first one and falls back
		// on the timeout, leaving maxInFlight at 1.
		select {
		case <-allInFlight:
		case <-time.After(2 * time.Second):
		}

		mu.Lock()
		inFlight--
		mu.Unlock()
	}

	d := &RDSDiscovery{
		logger: promslog.NewNopLogger(),
		rds:    mockClient,
		cfg: &RDSSDConfig{
			Region:             "us-east-1",
			Port:               9187,
			RequestConcurrency: concurrency,
		},
	}

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	require.Len(t, tgs[0].Targets, clusterCount)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, concurrency, maxInFlight, "DescribeDBInstances was not called concurrently")
}
