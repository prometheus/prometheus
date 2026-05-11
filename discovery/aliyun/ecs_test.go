// Copyright 2024 The Prometheus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aliyun

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"

	ecs "github.com/alibabacloud-go/ecs-20140526/v7/client"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/discovery/targetgroup"

	gomock "go.uber.org/mock/gomock"
	"gopkg.in/yaml.v2"
)

const UpperLimit = 1000 // upper limit of the number of instances

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// newClient create new ecsClient with mockClient.
func newClient(t *testing.T) *ecsClient {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := NewMockclient(ctrl)
	cli := &ecsClient{
		regionID: "cn-beijing",
		limit:    100,
		client:   mockClient,
		logger:   promslog.NewNopLogger(),
	}

	mockClient.EXPECT().
		ListTagResourcesWithOptions(gomock.Any(), gomock.Any()).
		DoAndReturn(func(request *ecs.ListTagResourcesRequest, runtime *dara.RuntimeOptions) (*ecs.ListTagResourcesResponse, error) {
			// construct response
			// [NextToken] indicates the location where the next query starts
			nextToken := dara.StringValue(request.NextToken)
			if nextToken == "" {
				nextToken = "0"
			}
			start, err := strconv.Atoi(nextToken)
			if err != nil {
				return &ecs.ListTagResourcesResponse{
					Body: &ecs.ListTagResourcesResponseBody{},
				}, nil
			}
			if start >= UpperLimit { // up to 1000 instances
				return &ecs.ListTagResourcesResponse{
					Body: &ecs.ListTagResourcesResponseBody{},
				}, nil
			}

			end := min(UpperLimit, start+MaxPageLimit)
			tagResources := make([]*ecs.ListTagResourcesResponseBodyTagResourcesTagResource, end-start)
			for i := start; i < end; i++ {
				tagResource := &ecs.ListTagResourcesResponseBodyTagResourcesTagResource{
					ResourceType: dara.String("instance"),
					ResourceId:   dara.String(strconv.Itoa(i)),
					TagKey:       dara.String("name"),
					TagValue:     dara.String("ecs-test"),
				}
				tagResources[i-start] = tagResource
			}
			responseNextToken := strconv.Itoa(start + MaxPageLimit)
			listTagResponse := &ecs.ListTagResourcesResponse{
				Body: &ecs.ListTagResourcesResponseBody{
					TagResources: &ecs.ListTagResourcesResponseBodyTagResources{
						TagResource: tagResources,
					},
					NextToken: dara.String(responseNextToken),
				},
			}
			return listTagResponse, nil
		}).AnyTimes()

	mockClient.EXPECT().
		DescribeInstancesWithOptions(gomock.Any(), gomock.Any()).
		DoAndReturn(func(request *ecs.DescribeInstancesRequest, runtime *dara.RuntimeOptions) (*ecs.DescribeInstancesResponse, error) {
			// construct data
			totalCount := UpperLimit
			allInstances := make([]*ecs.DescribeInstancesResponseBodyInstancesInstance, totalCount)
			for i := 0; i < totalCount; i++ {
				instance := &ecs.DescribeInstancesResponseBodyInstancesInstance{
					InstanceId: dara.String(strconv.Itoa(i)),
					Tags: &ecs.DescribeInstancesResponseBodyInstancesInstanceTags{
						Tag: []*ecs.DescribeInstancesResponseBodyInstancesInstanceTagsTag{
							{TagKey: dara.String("name"), TagValue: dara.String("ecs-test")},
						},
					},
				}
				allInstances[i] = instance
			}

			instanceIds := dara.StringValue(request.InstanceIds)
			if instanceIds == "" {
				pageNumber := int(dara.Int32Value(request.PageNumber))
				pageSize := int(dara.Int32Value(request.PageSize))
				start, end := (pageNumber-1)*pageSize, pageNumber*pageSize
				describeResponse := &ecs.DescribeInstancesResponse{
					Body: &ecs.DescribeInstancesResponseBody{
						TotalCount: dara.Int32(int32(totalCount)),
						Instances: &ecs.DescribeInstancesResponseBodyInstances{
							Instance: allInstances[start:end],
						},
					},
				}
				return describeResponse, nil
			}

			// construct response
			ids := make([]string, 0)
			err := json.Unmarshal([]byte(instanceIds), &ids)
			if err != nil {
				return nil, fmt.Errorf("unmarshal instance ids, err: %w", err)
			}

			retInstances := make([]*ecs.DescribeInstancesResponseBodyInstancesInstance, 0)
			for _, instance := range allInstances {
				if contains(ids, dara.StringValue(instance.InstanceId)) {
					retInstances = append(retInstances, instance)
				}
			}
			describeResponse := &ecs.DescribeInstancesResponse{
				Body: &ecs.DescribeInstancesResponseBody{
					TotalCount: dara.Int32(int32(totalCount)),
					Instances: &ecs.DescribeInstancesResponseBodyInstances{
						Instance: retInstances,
					},
				},
			}
			return describeResponse, nil
		}).AnyTimes()
	return cli
}

func TestMergeHashInstances(t *testing.T) {
	testCases := []struct {
		instances1 []*ecs.DescribeInstancesResponseBodyInstancesInstance
		instances2 []*ecs.DescribeInstancesResponseBodyInstancesInstance
		expected   []*ecs.DescribeInstancesResponseBodyInstancesInstance
	}{
		{
			instances1: []*ecs.DescribeInstancesResponseBodyInstancesInstance{},
			instances2: []*ecs.DescribeInstancesResponseBodyInstancesInstance{},
			expected:   []*ecs.DescribeInstancesResponseBodyInstancesInstance{},
		},
		{
			instances1: []*ecs.DescribeInstancesResponseBodyInstancesInstance{},
			instances2: []*ecs.DescribeInstancesResponseBodyInstancesInstance{
				{InstanceId: dara.String("1")},
				{InstanceId: dara.String("2")},
				{InstanceId: dara.String("3")},
			},
			expected: []*ecs.DescribeInstancesResponseBodyInstancesInstance{
				{InstanceId: dara.String("1")},
				{InstanceId: dara.String("2")},
				{InstanceId: dara.String("3")},
			},
		},
		{
			instances1: []*ecs.DescribeInstancesResponseBodyInstancesInstance{
				{InstanceId: dara.String("1")},
				{InstanceId: dara.String("2")},
			},
			instances2: []*ecs.DescribeInstancesResponseBodyInstancesInstance{
				{InstanceId: dara.String("2")},
				{InstanceId: dara.String("3")},
			},
			expected: []*ecs.DescribeInstancesResponseBodyInstancesInstance{
				{InstanceId: dara.String("1")},
				{InstanceId: dara.String("2")},
				{InstanceId: dara.String("3")},
			},
		},
		{
			instances1: []*ecs.DescribeInstancesResponseBodyInstancesInstance{
				{InstanceId: dara.String("1")},
				{InstanceId: dara.String("2")},
			},
			instances2: []*ecs.DescribeInstancesResponseBodyInstancesInstance{
				{InstanceId: dara.String("3")},
				{InstanceId: dara.String("4")},
			},
			expected: []*ecs.DescribeInstancesResponseBodyInstancesInstance{
				{InstanceId: dara.String("1")},
				{InstanceId: dara.String("2")},
				{InstanceId: dara.String("3")},
				{InstanceId: dara.String("4")},
			},
		},
	}
	for _, tc := range testCases {
		t.Run("test MergeHashInstances", func(t *testing.T) {
			actual := mergeHashInstances(tc.instances1, tc.instances2)
			require.True(t, instancesEqual(tc.expected, actual))
		})
	}
}

func TestECSConfigUnmarshalYAML(t *testing.T) {
	marshal := func(c ECSConfig) []byte {
		d, err := yaml.Marshal(c)
		if err != nil {
			panic(err)
		}
		return d
	}

	unmarshal := func(d []byte) func(interface{}) error {
		return func(o interface{}) error {
			return yaml.Unmarshal(d, o)
		}
	}

	testCases := []struct {
		name          string
		input         ECSConfig
		expectedError error
	}{
		{
			name:          "WithoutRegionId",
			input:         ECSConfig{},
			expectedError: errors.New("ECS SD configuration need RegionId"),
		},
		{
			name: "WithoutTagFilterValue",
			input: ECSConfig{
				RegionID:   "cn-beijing",
				TagFilters: []*TagFilter{{Key: "test", Values: []string{}}},
			},
			expectedError: errors.New("ECS SD configuration filter values cannot be empty"),
		},
		{
			name: "ValidECSConfig",
			input: ECSConfig{
				RegionID:   "cn-beijing",
				TagFilters: []*TagFilter{{Key: "test", Values: []string{"test"}}},
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var config ECSConfig
			d := marshal(tc.input)
			err := config.UnmarshalYAML(unmarshal(d))
			if tc.expectedError != nil {
				require.EqualError(t, err, tc.expectedError.Error())
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAddLabel(t *testing.T) {
	testCases := []struct {
		name           string
		userID         string
		port           int
		instance       *ecs.DescribeInstancesResponseBodyInstancesInstance
		expectedLabels model.LabelSet
		expectedError  error
	}{
		{
			name:   "ClassicNetwork",
			userID: "testUserId",
			port:   8888,
			instance: &ecs.DescribeInstancesResponseBodyInstancesInstance{
				InstanceId:          dara.String("1"),
				RegionId:            dara.String("cn-beijing"),
				Status:              dara.String("Running"),
				ZoneId:              dara.String("cn-beijing"),
				InstanceNetworkType: dara.String("classic"),
				PublicIpAddress: &ecs.DescribeInstancesResponseBodyInstancesInstancePublicIpAddress{
					IpAddress: []*string{dara.String("1.2.3.4")},
				},
				InnerIpAddress: &ecs.DescribeInstancesResponseBodyInstancesInstanceInnerIpAddress{
					IpAddress: []*string{dara.String("10.0.0.1")},
				},
				Tags: &ecs.DescribeInstancesResponseBodyInstancesInstanceTags{
					Tag: []*ecs.DescribeInstancesResponseBodyInstancesInstanceTagsTag{
						{TagKey: dara.String("app"), TagValue: dara.String("k8s")},
					},
				},
			},
			expectedLabels: model.LabelSet{
				ecsLabelInstanceID:  "1",
				ecsLabelRegionID:    "cn-beijing",
				ecsLabelStatus:      "Running",
				ecsLabelZoneID:      "cn-beijing",
				ecsLabelNetworkType: "classic",
				ecsLabelUserID:      "testUserId",
				ecsLabelPublicIP:    "1.2.3.4",
				ecsLabelInnerIP:     "10.0.0.1",
				model.AddressLabel:  "10.0.0.1:8888",
				ecsLabelTag + "app": "k8s",
			},
			expectedError: nil,
		},
		{
			name:   "VPCNetwork",
			userID: "testUserId",
			port:   8888,
			instance: &ecs.DescribeInstancesResponseBodyInstancesInstance{
				InstanceId:          dara.String("2"),
				RegionId:            dara.String("cn-beijing"),
				Status:              dara.String("Running"),
				ZoneId:              dara.String("cn-beijing"),
				InstanceNetworkType: dara.String("vpc"),
				EipAddress: &ecs.DescribeInstancesResponseBodyInstancesInstanceEipAddress{
					IpAddress: dara.String("1.2.3.4"),
				},
				VpcAttributes: &ecs.DescribeInstancesResponseBodyInstancesInstanceVpcAttributes{
					PrivateIpAddress: &ecs.DescribeInstancesResponseBodyInstancesInstanceVpcAttributesPrivateIpAddress{
						IpAddress: []*string{dara.String("10.0.0.1")},
					},
				},
				Tags: &ecs.DescribeInstancesResponseBodyInstancesInstanceTags{
					Tag: []*ecs.DescribeInstancesResponseBodyInstancesInstanceTagsTag{
						{TagKey: dara.String("app"), TagValue: dara.String("k8s")},
					},
				},
			},
			expectedLabels: model.LabelSet{
				ecsLabelInstanceID:  "2",
				ecsLabelRegionID:    "cn-beijing",
				ecsLabelStatus:      "Running",
				ecsLabelZoneID:      "cn-beijing",
				ecsLabelNetworkType: "vpc",
				ecsLabelUserID:      "testUserId",
				ecsLabelEip:         "1.2.3.4",
				ecsLabelPrivateIP:   "10.0.0.1",
				model.AddressLabel:  "10.0.0.1:8888",
				ecsLabelTag + "app": "k8s",
			},
			expectedError: nil,
		},
		{
			name:   "NoAddressLabel",
			userID: "testUserId",
			port:   8888,
			instance: &ecs.DescribeInstancesResponseBodyInstancesInstance{
				InstanceId: dara.String("3"),
				RegionId:   dara.String("cn-beijing"),
				Status:     dara.String("Running"),
				ZoneId:     dara.String("cn-beijing"),
				Tags: &ecs.DescribeInstancesResponseBodyInstancesInstanceTags{
					Tag: []*ecs.DescribeInstancesResponseBodyInstancesInstanceTagsTag{
						{TagKey: dara.String("app"), TagValue: dara.String("k8s")},
					},
				},
			},
			expectedLabels: nil,
			expectedError:  fmt.Errorf("instance %s dont have AddressLabel", "3"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			labels, err := addLabel(tc.userID, tc.port, tc.instance)
			if tc.expectedError != nil {
				require.EqualError(t, err, tc.expectedError.Error())
				return
			}
			require.NoError(t, err)
			require.True(t, labels.Equal(tc.expectedLabels))
		})
	}
}

func TestQueryInstances(t *testing.T) {
	cli := newClient(t)

	tagFilters := []*TagFilter{{Key: "name", Values: []string{"ecs-test"}}}

	totalCount := 200
	allLabelSets := make([]model.LabelSet, totalCount)
	allInstances := make([]*ecs.DescribeInstancesResponseBodyInstancesInstance, totalCount)
	for i := 0; i < totalCount; i++ {
		labelSet := model.LabelSet{
			ecsLabelInstanceID: model.LabelValue(strconv.Itoa(i)),
		}
		allLabelSets[i] = labelSet

		instance := &ecs.DescribeInstancesResponseBodyInstancesInstance{
			InstanceId: dara.String(strconv.Itoa(i)),
		}
		allInstances[i] = instance
	}

	testCases := []struct {
		name              string
		tagFilters        []*TagFilter
		labelSets         []model.LabelSet
		expectedInstances []*ecs.DescribeInstancesResponseBodyInstancesInstance
	}{
		{
			name:              "EmptyTagFiltersAndLabelSets",
			tagFilters:        nil,
			labelSets:         nil,
			expectedInstances: allInstances[:cli.limit],
		},
		{
			name:              "EmptyTagFilters",
			tagFilters:        nil,
			labelSets:         allLabelSets[:100],
			expectedInstances: append(allInstances[:cli.limit], allInstances[:100]...),
		},
		{
			name:              "EmptyLabelSets",
			tagFilters:        tagFilters,
			labelSets:         nil,
			expectedInstances: append(allInstances[:cli.limit], allInstances[:100]...),
		},
		{
			name:              "TagFiltersAndLabelSets",
			tagFilters:        tagFilters,
			labelSets:         allLabelSets[100:200],
			expectedInstances: allInstances[:cli.limit],
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cache := &targetgroup.Group{
				Targets: tc.labelSets,
			}
			instances, err := cli.QueryInstances(tc.tagFilters, cache)
			require.NoError(t, err)
			require.True(t, instancesEqual(tc.expectedInstances, instances))
		})
	}
}

func TestQueryFromListTagResources(t *testing.T) {
	cli := newClient(t)
	tagFilters := []*TagFilter{{Key: "name", Values: []string{"ecs-test"}}}

	testCases := []struct {
		name                   string
		limit                  int
		expectedInstancesCount int
	}{
		{
			name:                   "DefaultLimit",
			limit:                  100, // default value
			expectedInstancesCount: 100,
		},
		{
			name:                   "Limit:20",
			limit:                  20, // less than [MaxPageLimit]
			expectedInstancesCount: 20,
		},
		{
			name:                   "Limit:50",
			limit:                  50, // equal to [MaxPageLimit]
			expectedInstancesCount: 50,
		},
		{
			name:                   "Limit:70",
			limit:                  70, // more than [MaxPageLimit]
			expectedInstancesCount: 70,
		},
		{
			name:                   "Limit:-1",
			limit:                  -1, // less than zero
			expectedInstancesCount: UpperLimit,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cli.limit = tc.limit
			instances, err := cli.queryFromListTagResources(tagFilters)
			require.NoError(t, err)
			require.Len(t, instances, tc.expectedInstancesCount)
		})
	}
}

func TestQueryFromDescribeInstances(t *testing.T) {
	cli := newClient(t)

	testCases := []struct {
		name                   string
		limit                  int
		expectedInstancesCount int
	}{
		{
			name:                   "Limit:-1",
			limit:                  -1, // less than zero
			expectedInstancesCount: UpperLimit,
		},
		{
			name:                   "Limit:0",
			limit:                  0, // equal to zero
			expectedInstancesCount: 0,
		},
		{
			name:                   "Limit:20",
			limit:                  20, // less than [MaxPageLimit]
			expectedInstancesCount: 20,
		},
		{
			name:                   "Limit:50",
			limit:                  50, // equal to [MaxPageLimit]
			expectedInstancesCount: 50,
		},
		{
			name:                   "Limit:70",
			limit:                  70, // more than [MaxPageLimit]
			expectedInstancesCount: 70,
		},
		{
			name:                   "DefaultLimit",
			limit:                  100, // default value
			expectedInstancesCount: 100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cli.limit = tc.limit
			instances, err := cli.queryFromDescribeInstances()
			require.NoError(t, err)
			require.Len(t, instances, tc.expectedInstancesCount)
		})
	}
}

func TestGetCacheReCheckInstances(t *testing.T) {
	cli := newClient(t)
	totalCount := 100
	allLabelSets := make([]model.LabelSet, totalCount)
	allInstances := make([]*ecs.DescribeInstancesResponseBodyInstancesInstance, totalCount)

	for i := 0; i < totalCount; i++ {
		labelSet := model.LabelSet{
			ecsLabelInstanceID: model.LabelValue(strconv.Itoa(i)),
		}
		allLabelSets[i] = labelSet

		instance := &ecs.DescribeInstancesResponseBodyInstancesInstance{
			InstanceId: dara.String(strconv.Itoa(i)),
		}
		allInstances[i] = instance
	}

	testCases := []struct {
		name              string
		labelSets         []model.LabelSet
		expectedInstances []*ecs.DescribeInstancesResponseBodyInstancesInstance
	}{
		{
			name:              "LabelSets0:0",
			labelSets:         []model.LabelSet{},
			expectedInstances: []*ecs.DescribeInstancesResponseBodyInstancesInstance{},
		},
		{
			name:              "LabelSets0:50",
			labelSets:         allLabelSets[0:50],
			expectedInstances: allInstances[0:50],
		},
		{
			name:              "LabelSets0:75",
			labelSets:         allLabelSets[0:75],
			expectedInstances: allInstances[0:75],
		},
		{
			name:              "LabelSets25:75",
			labelSets:         allLabelSets[25:75],
			expectedInstances: allInstances[25:75],
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cache := &targetgroup.Group{
				Targets: tc.labelSets,
			}
			instances := cli.getCacheReCheckInstances(cache)
			require.True(t, instancesEqual(instances, tc.expectedInstances))
		})
	}
}

func TestDescribeInstances(t *testing.T) {
	cli := newClient(t)

	testCases := []struct {
		name              string
		ids               []string
		expectedInstances []*ecs.DescribeInstancesResponseBodyInstancesInstance
		expectedError     error
	}{
		{
			name:              "NilIds",
			ids:               nil,
			expectedInstances: []*ecs.DescribeInstancesResponseBodyInstancesInstance{},
			expectedError:     nil,
		},
		{
			name:              "EmptyIds",
			ids:               []string{},
			expectedInstances: []*ecs.DescribeInstancesResponseBodyInstancesInstance{},
			expectedError:     nil,
		},
		{
			name: "ThreeIds",
			ids:  []string{"1", "2", "3"},
			expectedInstances: []*ecs.DescribeInstancesResponseBodyInstancesInstance{
				{InstanceId: dara.String("1")},
				{InstanceId: dara.String("2")},
				{InstanceId: dara.String("3")},
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			instances, err := cli.describeInstances(tc.ids)
			if tc.expectedError != nil {
				require.EqualError(t, err, tc.expectedError.Error())
				return
			}
			require.NoError(t, err)
			require.True(t, instancesEqual(instances, tc.expectedInstances))
		})
	}
}

func TestListTagInstanceIds(t *testing.T) {
	cli := newClient(t)

	tagFilters := []*TagFilter{{Key: "name", Values: []string{"ecs-test"}}}

	testCases := []struct {
		name                   string
		token                  string
		tagFilters             []*TagFilter
		expectedInstancesCount int
		expectedError          error
	}{
		{
			name:                   "TestTokenFirst",
			token:                  "FIRST",
			tagFilters:             tagFilters,
			expectedInstancesCount: MaxPageLimit,
			expectedError:          nil,
		},
		{
			name:                   "TestTokenICM=",
			token:                  "ICM=",
			tagFilters:             tagFilters,
			expectedInstancesCount: 0,
			expectedError:          errors.New("token is empty, but not first request"),
		},
		{
			name:                   "TestTokenEmpty",
			token:                  "",
			tagFilters:             tagFilters,
			expectedInstancesCount: 0,
			expectedError:          errors.New("token is empty, but not first request"),
		},
		{
			name:                   "TestTokenValid",
			token:                  "50",
			tagFilters:             tagFilters,
			expectedInstancesCount: MaxPageLimit,
			expectedError:          nil,
		},
		{
			name:                   "TestTokenInvalid",
			token:                  "invalid",
			tagFilters:             tagFilters,
			expectedInstancesCount: 0,
			expectedError:          nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ids, _, err := cli.listTagInstanceIDs(tc.token, tc.tagFilters)
			if tc.expectedError != nil {
				require.EqualError(t, err, tc.expectedError.Error())
				return
			}
			require.NoError(t, err)
			require.Len(t, ids, tc.expectedInstancesCount)
		})
	}
}

func TestListTagInstances(t *testing.T) {
	cli := newClient(t)
	tagFilters := []*TagFilter{{Key: "name", Values: []string{"ecs-test"}}}

	testCases := []struct {
		name                   string
		token                  string
		currentTotalCount      int
		limit                  int
		tagFilters             []*TagFilter
		expectedToken          string
		expectedInstancesCount int
		expectedError          error
	}{
		{
			name:                   "Test0/-1",
			token:                  "0",
			currentTotalCount:      0,
			limit:                  -1,
			tagFilters:             tagFilters,
			expectedToken:          "50",
			expectedInstancesCount: 50,
			expectedError:          nil,
		},
		{
			name:                   "Test50/50",
			token:                  "50",
			currentTotalCount:      50,
			limit:                  50,
			tagFilters:             tagFilters,
			expectedToken:          "100",
			expectedInstancesCount: 0,
			expectedError:          nil,
		},
		{
			name:                   "Test50/75",
			token:                  "50",
			currentTotalCount:      50,
			limit:                  75,
			tagFilters:             tagFilters,
			expectedToken:          "100",
			expectedInstancesCount: 25,
			expectedError:          nil,
		},
		{
			name:                   "Test50/100",
			token:                  "50",
			currentTotalCount:      50,
			limit:                  100,
			tagFilters:             tagFilters,
			expectedToken:          "100",
			expectedInstancesCount: 50,
			expectedError:          nil,
		},
		{
			name:                   "Test1000/1000",
			token:                  strconv.Itoa(UpperLimit),
			currentTotalCount:      UpperLimit,
			limit:                  -1,
			tagFilters:             tagFilters,
			expectedToken:          "",
			expectedInstancesCount: 0,
			expectedError:          nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cli.limit = tc.limit
			token, instances, err := cli.listTagInstances(tc.token, tc.currentTotalCount, tc.tagFilters)
			require.NoError(t, err)
			require.Equal(t, tc.expectedToken, token)
			require.Len(t, instances, tc.expectedInstancesCount)
		})
	}
}

var _ client = &Mockclient{}

// contains reports whether v is present in s.
func contains(s []string, v string) bool {
	for _, value := range s {
		if value == v {
			return true
		}
	}
	return false
}

// instancesEqual determine whether two instance lists are the same based on id.
func instancesEqual(instances1, instances2 []*ecs.DescribeInstancesResponseBodyInstancesInstance) bool {
	// remove duplicate elements
	ids1, ids2 := make(map[string]struct{}, 0), make(map[string]struct{}, 0)
	for _, in1 := range instances1 {
		ids1[dara.StringValue(in1.InstanceId)] = struct{}{}
	}
	for _, in2 := range instances2 {
		ids2[dara.StringValue(in2.InstanceId)] = struct{}{}
	}
	if len(ids1) != len(ids2) {
		return false
	}
	for id1 := range ids1 {
		_, ok := ids2[id1]
		if !ok {
			return false
		}
	}
	return true
}
