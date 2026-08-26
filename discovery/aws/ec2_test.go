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
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Helper function to get pointers on literals.
// NOTE: this is common between a few tests. In the future it might worth to move this out into a separate package.
func strptr(str string) *string {
	return &str
}

func boolptr(b bool) *bool {
	return &b
}

// Struct for test data.
type ec2DataStore struct {
	region string

	azToAZID map[string]string

	ownerID string

	instances []ec2Types.Instance
}

// The tests itself.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestEC2DiscoveryRefreshAZIDs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// iterate through the test cases
	for _, tt := range []struct {
		name       string
		shouldFail bool
		ec2Data    *ec2DataStore
	}{
		{
			name:       "Normal",
			shouldFail: false,
			ec2Data: &ec2DataStore{
				azToAZID: map[string]string{
					"azname-a": "azid-1",
					"azname-b": "azid-2",
					"azname-c": "azid-3",
				},
			},
		},
		{
			name:       "HandleError",
			shouldFail: true,
			ec2Data:    &ec2DataStore{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockEC2Client(tt.ec2Data)

			d := &EC2Discovery{
				ec2: client,
			}

			err := d.refreshAZIDs(ctx)
			if tt.shouldFail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, client.ec2Data.azToAZID, d.azToAZID)
			}
		})
	}
}

func TestEC2DiscoveryRefresh(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// iterate through the test cases
	for _, tt := range []struct {
		name     string
		ec2Data  *ec2DataStore
		filters  []*Filter
		expected []*targetgroup.Group
	}{
		{
			name: "NoPrivateIpOrIpv6",
			ec2Data: &ec2DataStore{
				region: "region-noprivateip",
				azToAZID: map[string]string{
					"azname-a": "azid-1",
					"azname-b": "azid-2",
					"azname-c": "azid-3",
				},
				instances: []ec2Types.Instance{
					{
						InstanceId: strptr("instance-id-noprivateip"),
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "region-noprivateip",
				},
			},
		},
		{
			name: "NoVpc",
			ec2Data: &ec2DataStore{
				region: "region-novpc",
				azToAZID: map[string]string{
					"azname-a": "azid-1",
					"azname-b": "azid-2",
					"azname-c": "azid-3",
				},
				ownerID: "owner-id-novpc",
				instances: []ec2Types.Instance{
					{
						// set every possible options and test them here
						Architecture:      "architecture-novpc",
						ImageId:           strptr("ami-novpc"),
						InstanceId:        strptr("instance-id-novpc"),
						InstanceLifecycle: "instance-lifecycle-novpc",
						InstanceType:      "instance-type-novpc",
						Placement:         &ec2Types.Placement{AvailabilityZone: strptr("azname-b")},
						Platform:          "platform-novpc",
						PrivateDnsName:    strptr("private-dns-novpc"),
						PrivateIpAddress:  strptr("1.2.3.4"),
						PublicDnsName:     strptr("public-dns-novpc"),
						PublicIpAddress:   strptr("42.42.42.2"),
						State:             &ec2Types.InstanceState{Name: "running"},
						// test tags once and for all
						Tags: []ec2Types.Tag{
							{Key: strptr("tag-1-key"), Value: strptr("tag-1-value")},
							{Key: strptr("tag-2-key"), Value: strptr("tag-2-value")},
							{},
							{Value: strptr("tag-4-value")},
							{Key: strptr("tag-5-key")},
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "region-novpc",
					Targets: []model.LabelSet{
						{
							"__address__":                     model.LabelValue("1.2.3.4:4242"),
							"__meta_ec2_ami":                  model.LabelValue("ami-novpc"),
							"__meta_ec2_architecture":         model.LabelValue("architecture-novpc"),
							"__meta_ec2_availability_zone":    model.LabelValue("azname-b"),
							"__meta_ec2_availability_zone_id": model.LabelValue("azid-2"),
							"__meta_ec2_instance_id":          model.LabelValue("instance-id-novpc"),
							"__meta_ec2_instance_lifecycle":   model.LabelValue("instance-lifecycle-novpc"),
							"__meta_ec2_instance_type":        model.LabelValue("instance-type-novpc"),
							"__meta_ec2_instance_state":       model.LabelValue("running"),
							"__meta_ec2_owner_id":             model.LabelValue("owner-id-novpc"),
							"__meta_ec2_platform":             model.LabelValue("platform-novpc"),
							"__meta_ec2_private_dns_name":     model.LabelValue("private-dns-novpc"),
							"__meta_ec2_private_ip":           model.LabelValue("1.2.3.4"),
							"__meta_ec2_public_dns_name":      model.LabelValue("public-dns-novpc"),
							"__meta_ec2_public_ip":            model.LabelValue("42.42.42.2"),
							"__meta_ec2_region":               model.LabelValue("region-novpc"),
							"__meta_ec2_tag_tag_1_key":        model.LabelValue("tag-1-value"),
							"__meta_ec2_tag_tag_2_key":        model.LabelValue("tag-2-value"),
						},
					},
				},
			},
		},
		{
			name: "Ipv4",
			ec2Data: &ec2DataStore{
				region: "region-ipv4",
				azToAZID: map[string]string{
					"azname-a": "azid-1",
					"azname-b": "azid-2",
					"azname-c": "azid-3",
				},
				instances: []ec2Types.Instance{
					{
						// just the minimum needed for the refresh work
						ImageId:          strptr("ami-ipv4"),
						InstanceId:       strptr("instance-id-ipv4"),
						InstanceType:     "instance-type-ipv4",
						Placement:        &ec2Types.Placement{AvailabilityZone: strptr("azname-c")},
						PrivateIpAddress: strptr("5.6.7.8"),
						State:            &ec2Types.InstanceState{Name: "running"},
						SubnetId:         strptr("azid-3"),
						VpcId:            strptr("vpc-ipv4"),
						// network interfaces
						NetworkInterfaces: []ec2Types.InstanceNetworkInterface{
							// interface without subnet -> should be ignored
							{
								Ipv6Addresses: []ec2Types.InstanceIpv6Address{
									{
										Ipv6Address:   strptr("2001:db8:1::1"),
										IsPrimaryIpv6: boolptr(true),
									},
								},
							},
							// interface with subnet, no IPv6
							{
								Ipv6Addresses: []ec2Types.InstanceIpv6Address{},
								SubnetId:      strptr("azid-3"),
							},
							// interface with another subnet, no IPv6
							{
								Ipv6Addresses: []ec2Types.InstanceIpv6Address{},
								SubnetId:      strptr("azid-1"),
							},
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "region-ipv4",
					Targets: []model.LabelSet{
						{
							"__address__":                     model.LabelValue("5.6.7.8:4242"),
							"__meta_ec2_ami":                  model.LabelValue("ami-ipv4"),
							"__meta_ec2_availability_zone":    model.LabelValue("azname-c"),
							"__meta_ec2_availability_zone_id": model.LabelValue("azid-3"),
							"__meta_ec2_instance_id":          model.LabelValue("instance-id-ipv4"),
							"__meta_ec2_instance_state":       model.LabelValue("running"),
							"__meta_ec2_instance_type":        model.LabelValue("instance-type-ipv4"),
							"__meta_ec2_owner_id":             model.LabelValue(""),
							"__meta_ec2_primary_subnet_id":    model.LabelValue("azid-3"),
							"__meta_ec2_private_ip":           model.LabelValue("5.6.7.8"),
							"__meta_ec2_region":               model.LabelValue("region-ipv4"),
							"__meta_ec2_subnet_id":            model.LabelValue(",azid-3,azid-1,"),
							"__meta_ec2_vpc_id":               model.LabelValue("vpc-ipv4"),
						},
					},
				},
			},
		},
		{
			name: "Ipv6",
			ec2Data: &ec2DataStore{
				region: "region-ipv6",
				azToAZID: map[string]string{
					"azname-a": "azid-1",
					"azname-b": "azid-2",
					"azname-c": "azid-3",
				},
				instances: []ec2Types.Instance{
					{
						// just the minimum needed for the refresh work
						ImageId:          strptr("ami-ipv6"),
						InstanceId:       strptr("instance-id-ipv6"),
						InstanceType:     "instance-type-ipv6",
						Placement:        &ec2Types.Placement{AvailabilityZone: strptr("azname-b")},
						PrivateIpAddress: strptr("9.10.11.12"),
						State:            &ec2Types.InstanceState{Name: "running"},
						SubnetId:         strptr("azid-2"),
						VpcId:            strptr("vpc-ipv6"),
						// network interfaces
						NetworkInterfaces: []ec2Types.InstanceNetworkInterface{
							// interface without primary IPv6, index 2
							{
								Attachment: &ec2Types.InstanceNetworkInterfaceAttachment{
									DeviceIndex: aws.Int32(3),
								},
								Ipv6Addresses: []ec2Types.InstanceIpv6Address{
									{
										Ipv6Address:   strptr("2001:db8:2::1:1"),
										IsPrimaryIpv6: boolptr(false),
									},
								},
								SubnetId: strptr("azid-2"),
							},
							// interface with primary IPv6, index 1
							{
								Attachment: &ec2Types.InstanceNetworkInterfaceAttachment{
									DeviceIndex: aws.Int32(1),
								},
								Ipv6Addresses: []ec2Types.InstanceIpv6Address{
									{
										Ipv6Address:   strptr("2001:db8:2::2:1"),
										IsPrimaryIpv6: boolptr(false),
									},
									{
										Ipv6Address:   strptr("2001:db8:2::2:2"),
										IsPrimaryIpv6: boolptr(true),
									},
								},
								SubnetId: strptr("azid-2"),
							},
							// interface with primary IPv6, index 3
							{
								Attachment: &ec2Types.InstanceNetworkInterfaceAttachment{
									DeviceIndex: aws.Int32(3),
								},
								Ipv6Addresses: []ec2Types.InstanceIpv6Address{
									{
										Ipv6Address:   strptr("2001:db8:2::3:1"),
										IsPrimaryIpv6: boolptr(true),
									},
								},
								SubnetId: strptr("azid-1"),
							},
							// interface without primary IPv6, index 0
							{
								Attachment: &ec2Types.InstanceNetworkInterfaceAttachment{
									DeviceIndex: aws.Int32(0),
								},
								Ipv6Addresses: []ec2Types.InstanceIpv6Address{},
								SubnetId:      strptr("azid-3"),
							},
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "region-ipv6",
					Targets: []model.LabelSet{
						{
							"__address__":                       model.LabelValue("9.10.11.12:4242"),
							"__meta_ec2_ami":                    model.LabelValue("ami-ipv6"),
							"__meta_ec2_availability_zone":      model.LabelValue("azname-b"),
							"__meta_ec2_availability_zone_id":   model.LabelValue("azid-2"),
							"__meta_ec2_instance_id":            model.LabelValue("instance-id-ipv6"),
							"__meta_ec2_instance_state":         model.LabelValue("running"),
							"__meta_ec2_instance_type":          model.LabelValue("instance-type-ipv6"),
							"__meta_ec2_ipv6_addresses":         model.LabelValue(",2001:db8:2::1:1,2001:db8:2::2:1,2001:db8:2::2:2,2001:db8:2::3:1,"),
							"__meta_ec2_owner_id":               model.LabelValue(""),
							"__meta_ec2_default_ipv6_address":   model.LabelValue("2001:db8:2::2:2"),
							"__meta_ec2_primary_ipv6_addresses": model.LabelValue(",,2001:db8:2::2:2,,2001:db8:2::3:1,"),
							"__meta_ec2_primary_subnet_id":      model.LabelValue("azid-2"),
							"__meta_ec2_private_ip":             model.LabelValue("9.10.11.12"),
							"__meta_ec2_region":                 model.LabelValue("region-ipv6"),
							"__meta_ec2_subnet_id":              model.LabelValue(",azid-2,azid-1,azid-3,"),
							"__meta_ec2_vpc_id":                 model.LabelValue("vpc-ipv6"),
						},
					},
				},
			},
		},
		{
			name: "Ipv6-Only",
			ec2Data: &ec2DataStore{
				region: "region-ipv6-only",
				azToAZID: map[string]string{
					"azname-a": "azid-1",
					"azname-b": "azid-2",
					"azname-c": "azid-3",
				},

				instances: []ec2Types.Instance{
					{
						// just the minimum needed for the refresh work
						ImageId:      strptr("ami-ipv6-only"),
						InstanceId:   strptr("instance-id-ipv6-only"),
						InstanceType: "instance-type-ipv6-only",
						Placement:    &ec2Types.Placement{AvailabilityZone: strptr("azname-b")},
						State:        &ec2Types.InstanceState{Name: "running"},
						SubnetId:     strptr("azid-2"),
						VpcId:        strptr("vpc-ipv6-only"),
						// network interfaces
						NetworkInterfaces: []ec2Types.InstanceNetworkInterface{
							// interface without primary IPv6, index 0
							{
								Attachment: &ec2Types.InstanceNetworkInterfaceAttachment{
									DeviceIndex: aws.Int32(0),
								},
								Ipv6Addresses: []ec2Types.InstanceIpv6Address{
									{
										Ipv6Address:   strptr("2001:db8:2::1:1"),
										IsPrimaryIpv6: boolptr(false),
									},
								},
								SubnetId: strptr("azid-2"),
							},
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				{
					Source: "region-ipv6-only",
					Targets: []model.LabelSet{
						{
							"__address__":                     model.LabelValue("[2001:db8:2::1:1]:4242"),
							"__meta_ec2_ami":                  model.LabelValue("ami-ipv6-only"),
							"__meta_ec2_availability_zone":    model.LabelValue("azname-b"),
							"__meta_ec2_availability_zone_id": model.LabelValue("azid-2"),
							"__meta_ec2_instance_id":          model.LabelValue("instance-id-ipv6-only"),
							"__meta_ec2_instance_state":       model.LabelValue("running"),
							"__meta_ec2_instance_type":        model.LabelValue("instance-type-ipv6-only"),
							"__meta_ec2_ipv6_addresses":       model.LabelValue(",2001:db8:2::1:1,"),
							"__meta_ec2_owner_id":             model.LabelValue(""),
							"__meta_ec2_default_ipv6_address": model.LabelValue("2001:db8:2::1:1"),
							"__meta_ec2_primary_subnet_id":    model.LabelValue("azid-2"),
							"__meta_ec2_region":               model.LabelValue("region-ipv6-only"),
							"__meta_ec2_subnet_id":            model.LabelValue(",azid-2,"),
							"__meta_ec2_vpc_id":               model.LabelValue("vpc-ipv6-only"),
						},
					},
				},
			},
		},
		{
			name: "FiltersMatchSingleInstance",
			ec2Data: &ec2DataStore{
				region: "region-filter-match",
				azToAZID: map[string]string{
					"azname-a": "azid-1",
				},
				instances: []ec2Types.Instance{
					{
						ImageId:          strptr("ami-filter-1"),
						InstanceId:       strptr("instance-filter-1"),
						InstanceType:     "instance-type-filter",
						Placement:        &ec2Types.Placement{AvailabilityZone: strptr("azname-a")},
						PrivateIpAddress: strptr("10.0.0.1"),
						State:            &ec2Types.InstanceState{Name: "running"},
					},
					{
						ImageId:          strptr("ami-filter-2"),
						InstanceId:       strptr("instance-filter-2"),
						InstanceType:     "instance-type-filter",
						Placement:        &ec2Types.Placement{AvailabilityZone: strptr("azname-a")},
						PrivateIpAddress: strptr("10.0.0.2"),
						State:            &ec2Types.InstanceState{Name: "stopped"},
					},
				},
			},
			filters: []*Filter{{Name: "instance-state-name", Values: []string{"running"}}},
			expected: []*targetgroup.Group{
				{
					Source: "region-filter-match",
					Targets: []model.LabelSet{
						{
							"__address__":                     model.LabelValue("10.0.0.1:4242"),
							"__meta_ec2_ami":                  model.LabelValue("ami-filter-1"),
							"__meta_ec2_availability_zone":    model.LabelValue("azname-a"),
							"__meta_ec2_availability_zone_id": model.LabelValue("azid-1"),
							"__meta_ec2_instance_id":          model.LabelValue("instance-filter-1"),
							"__meta_ec2_instance_state":       model.LabelValue("running"),
							"__meta_ec2_instance_type":        model.LabelValue("instance-type-filter"),
							"__meta_ec2_owner_id":             model.LabelValue(""),
							"__meta_ec2_private_ip":           model.LabelValue("10.0.0.1"),
							"__meta_ec2_region":               model.LabelValue("region-filter-match"),
						},
					},
				},
			},
		},
		{
			name: "FiltersMatchNoInstances",
			ec2Data: &ec2DataStore{
				region: "region-filter-none",
				azToAZID: map[string]string{
					"azname-a": "azid-1",
				},
				instances: []ec2Types.Instance{
					{
						ImageId:          strptr("ami-filter-1"),
						InstanceId:       strptr("instance-filter-1"),
						InstanceType:     "instance-type-filter",
						Placement:        &ec2Types.Placement{AvailabilityZone: strptr("azname-a")},
						PrivateIpAddress: strptr("10.0.1.1"),
						State:            &ec2Types.InstanceState{Name: "running"},
					},
					{
						ImageId:          strptr("ami-filter-2"),
						InstanceId:       strptr("instance-filter-2"),
						InstanceType:     "instance-type-filter",
						Placement:        &ec2Types.Placement{AvailabilityZone: strptr("azname-a")},
						PrivateIpAddress: strptr("10.0.1.2"),
						State:            &ec2Types.InstanceState{Name: "stopped"},
					},
				},
			},
			filters:  []*Filter{{Name: "instance-state-name", Values: []string{"terminated"}}},
			expected: []*targetgroup.Group{{Source: "region-filter-none"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockEC2Client(tt.ec2Data)

			d := &EC2Discovery{
				ec2: client,
				cfg: &EC2SDConfig{
					Port:    4242,
					Region:  client.ec2Data.region,
					Filters: tt.filters,
				},
				region: client.ec2Data.region,
			}

			g, err := d.refresh(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.expected, g)
		})
	}
}

// EC2 client mock.
type mockEC2Client struct {
	ec2Data ec2DataStore
}

func newMockEC2Client(ec2Data *ec2DataStore) *mockEC2Client {
	client := mockEC2Client{
		ec2Data: *ec2Data,
	}
	return &client
}

func (m *mockEC2Client) DescribeAvailabilityZones(context.Context, *ec2.DescribeAvailabilityZonesInput, ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
	if len(m.ec2Data.azToAZID) == 0 {
		return nil, errors.New("No AZs found")
	}

	azs := make([]ec2Types.AvailabilityZone, len(m.ec2Data.azToAZID))

	i := 0
	for k, v := range m.ec2Data.azToAZID {
		azs[i] = ec2Types.AvailabilityZone{
			ZoneName: strptr(k),
			ZoneId:   strptr(v),
		}
		i++
	}

	return &ec2.DescribeAvailabilityZonesOutput{
		AvailabilityZones: azs,
	}, nil
}

// ec2TestDiscovery returns a discovery backed by the mock client, so refresh()
// can be exercised without reaching AWS. ec2Client returns early when ec2 is
// already set, which also leaves region unset, so it is populated here.
func ec2TestDiscovery(data *ec2DataStore) *EC2Discovery {
	return &EC2Discovery{
		logger: promslog.NewNopLogger(),
		ec2:    newMockEC2Client(data),
		cfg: &EC2SDConfig{
			Port:   4242,
			Region: data.region,
		},
		region: data.region,
	}
}

// fullyPopulatedEC2Instance is the shape the AWS API returns when every
// optional field happens to be set. Each subtest below blanks exactly one of
// them.
func fullyPopulatedEC2Instance() ec2Types.Instance {
	return ec2Types.Instance{
		Architecture:      "x86_64",
		ImageId:           strptr("ami-full"),
		InstanceId:        strptr("i-full"),
		InstanceLifecycle: "spot",
		InstanceType:      "t3.micro",
		Placement:         &ec2Types.Placement{AvailabilityZone: strptr("azname-a")},
		Platform:          "windows",
		PrivateDnsName:    strptr("ip-10-0-0-1.ec2.internal"),
		PrivateIpAddress:  strptr("10.0.0.1"),
		PublicDnsName:     strptr("ec2-42-42-42-42.compute-1.amazonaws.com"),
		PublicIpAddress:   strptr("42.42.42.42"),
		State:             &ec2Types.InstanceState{Name: "running"},
		SubnetId:          strptr("subnet-full"),
		VpcId:             strptr("vpc-full"),
		NetworkInterfaces: []ec2Types.InstanceNetworkInterface{
			{
				Attachment: &ec2Types.InstanceNetworkInterfaceAttachment{
					DeviceIndex: aws.Int32(0),
				},
				Ipv6Addresses: []ec2Types.InstanceIpv6Address{
					{
						Ipv6Address:   strptr("2001:db8::1"),
						IsPrimaryIpv6: boolptr(true),
					},
				},
				SubnetId: strptr("subnet-full"),
			},
		},
	}
}

// fullyPopulatedEC2DataStore backs the instance above with the region, AZ map
// and owner ID the mock client needs.
func fullyPopulatedEC2DataStore() *ec2DataStore {
	return &ec2DataStore{
		region:    "region-full",
		azToAZID:  map[string]string{"azname-a": "azid-1"},
		ownerID:   "owner-id-full",
		instances: []ec2Types.Instance{fullyPopulatedEC2Instance()},
	}
}

// TestEC2DiscoveryRefreshFullyPopulated pins the happy path so the nil handling
// added for the cases below cannot silently drop labels that used to be set.
func TestEC2DiscoveryRefreshFullyPopulated(t *testing.T) {
	t.Parallel()

	tgs, err := ec2TestDiscovery(fullyPopulatedEC2DataStore()).refresh(context.Background())
	require.NoError(t, err)
	require.Equal(t, []*targetgroup.Group{
		{
			Source: "region-full",
			Targets: []model.LabelSet{
				{
					"__address__":                       model.LabelValue("10.0.0.1:4242"),
					"__meta_ec2_ami":                    model.LabelValue("ami-full"),
					"__meta_ec2_architecture":           model.LabelValue("x86_64"),
					"__meta_ec2_availability_zone":      model.LabelValue("azname-a"),
					"__meta_ec2_availability_zone_id":   model.LabelValue("azid-1"),
					"__meta_ec2_default_ipv6_address":   model.LabelValue("2001:db8::1"),
					"__meta_ec2_instance_id":            model.LabelValue("i-full"),
					"__meta_ec2_instance_lifecycle":     model.LabelValue("spot"),
					"__meta_ec2_instance_state":         model.LabelValue("running"),
					"__meta_ec2_instance_type":          model.LabelValue("t3.micro"),
					"__meta_ec2_ipv6_addresses":         model.LabelValue(",2001:db8::1,"),
					"__meta_ec2_owner_id":               model.LabelValue("owner-id-full"),
					"__meta_ec2_platform":               model.LabelValue("windows"),
					"__meta_ec2_primary_ipv6_addresses": model.LabelValue(",2001:db8::1,"),
					"__meta_ec2_primary_subnet_id":      model.LabelValue("subnet-full"),
					"__meta_ec2_private_dns_name":       model.LabelValue("ip-10-0-0-1.ec2.internal"),
					"__meta_ec2_private_ip":             model.LabelValue("10.0.0.1"),
					"__meta_ec2_public_dns_name":        model.LabelValue("ec2-42-42-42-42.compute-1.amazonaws.com"),
					"__meta_ec2_public_ip":              model.LabelValue("42.42.42.42"),
					"__meta_ec2_region":                 model.LabelValue("region-full"),
					"__meta_ec2_subnet_id":              model.LabelValue(",subnet-full,"),
					"__meta_ec2_vpc_id":                 model.LabelValue("vpc-full"),
				},
			},
		},
	}, tgs)
}

// TestEC2DiscoveryRefreshInstanceNilOptionalFields covers instances where the
// AWS API omitted an optional field. None of these members are marked required
// by the SDK, yet refresh() dereferenced them without a nil check, so a single
// missing field panicked the whole Prometheus process during service discovery
// rather than degrading the target.
func TestEC2DiscoveryRefreshInstanceNilOptionalFields(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		mutate func(*ec2Types.Instance)
		absent []model.LabelName
	}{
		{
			name:   "NilImageId",
			mutate: func(i *ec2Types.Instance) { i.ImageId = nil },
			absent: []model.LabelName{ec2LabelAMI},
		},
		{
			name:   "NilInstanceId",
			mutate: func(i *ec2Types.Instance) { i.InstanceId = nil },
			absent: []model.LabelName{ec2LabelInstanceID},
		},
		{
			name:   "NilPlacement",
			mutate: func(i *ec2Types.Instance) { i.Placement = nil },
			absent: []model.LabelName{ec2LabelAZ, ec2LabelAZID},
		},
		{
			name:   "NilAvailabilityZone",
			mutate: func(i *ec2Types.Instance) { i.Placement.AvailabilityZone = nil },
			absent: []model.LabelName{ec2LabelAZ, ec2LabelAZID},
		},
		{
			name:   "NilState",
			mutate: func(i *ec2Types.Instance) { i.State = nil },
			absent: []model.LabelName{ec2LabelInstanceState},
		},
		{
			name:   "NilSubnetId",
			mutate: func(i *ec2Types.Instance) { i.SubnetId = nil },
			absent: []model.LabelName{ec2LabelPrimarySubnetID},
		},
		{
			name:   "NilPublicDnsName",
			mutate: func(i *ec2Types.Instance) { i.PublicDnsName = nil },
			absent: []model.LabelName{ec2LabelPublicDNS},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data := fullyPopulatedEC2DataStore()
			tc.mutate(&data.instances[0])

			tgs, err := ec2TestDiscovery(data).refresh(context.Background())
			require.NoError(t, err)
			require.Len(t, tgs, 1)
			require.Len(t, tgs[0].Targets, 1,
				"instance must still be discovered when an optional field is absent")

			target := tgs[0].Targets[0]
			for _, label := range tc.absent {
				require.NotContains(t, target, label,
					"label sourced from the absent field should be omitted")
			}
			// The instance is still usable: the address is what makes it a target.
			require.Equal(t, model.LabelValue("10.0.0.1:4242"), target[model.AddressLabel])
		})
	}
}

// TestEC2DiscoveryRefreshIPv6NilOptionalFields covers the IPv6 fields
// getInstanceIPv6Addresses dereferenced without a nil check. IsPrimaryIpv6 is
// only populated once a primary IPv6 address has been enabled on the interface,
// and the attachment device index is optional too, so an instance holding a
// plain IPv6 address was enough to panic the process.
func TestEC2DiscoveryRefreshIPv6NilOptionalFields(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// mutate operates on the single fully populated network interface.
		mutate func(*ec2Types.InstanceNetworkInterface)
		// expected holds the IPv6 labels left once the absent field is handled;
		// a label missing from the map must be absent from the target.
		expected model.LabelSet
	}{
		{
			name: "NilIpv6Address",
			mutate: func(eni *ec2Types.InstanceNetworkInterface) {
				eni.Ipv6Addresses[0].Ipv6Address = nil
			},
			// Nothing identifies the address, so it drops out entirely.
			expected: model.LabelSet{},
		},
		{
			name: "NilIsPrimaryIpv6",
			mutate: func(eni *ec2Types.InstanceNetworkInterface) {
				eni.Ipv6Addresses[0].IsPrimaryIpv6 = nil
			},
			// An absent flag means the address is not primary.
			expected: model.LabelSet{
				ec2LabelIPv6Addresses:      model.LabelValue(",2001:db8::1,"),
				ec2LabelDefaultIPv6Address: model.LabelValue("2001:db8::1"),
			},
		},
		{
			name: "NilAttachment",
			mutate: func(eni *ec2Types.InstanceNetworkInterface) {
				eni.Attachment = nil
			},
			// Without an attachment there is no device index to record the
			// primary address at, but the address itself is still discovered.
			expected: model.LabelSet{
				ec2LabelIPv6Addresses:      model.LabelValue(",2001:db8::1,"),
				ec2LabelDefaultIPv6Address: model.LabelValue("2001:db8::1"),
			},
		},
		{
			name: "NilDeviceIndex",
			mutate: func(eni *ec2Types.InstanceNetworkInterface) {
				eni.Attachment.DeviceIndex = nil
			},
			expected: model.LabelSet{
				ec2LabelIPv6Addresses:      model.LabelValue(",2001:db8::1,"),
				ec2LabelDefaultIPv6Address: model.LabelValue("2001:db8::1"),
			},
		},
		{
			name: "NegativeDeviceIndex",
			mutate: func(eni *ec2Types.InstanceNetworkInterface) {
				eni.Attachment.DeviceIndex = aws.Int32(-1)
			},
			// A negative index has no slot in the positional list.
			expected: model.LabelSet{
				ec2LabelIPv6Addresses:      model.LabelValue(",2001:db8::1,"),
				ec2LabelDefaultIPv6Address: model.LabelValue("2001:db8::1"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data := fullyPopulatedEC2DataStore()
			tc.mutate(&data.instances[0].NetworkInterfaces[0])

			tgs, err := ec2TestDiscovery(data).refresh(context.Background())
			require.NoError(t, err)
			require.Len(t, tgs, 1)
			require.Len(t, tgs[0].Targets, 1,
				"instance must still be discovered when an optional field is absent")

			target := tgs[0].Targets[0]
			for _, label := range []model.LabelName{ec2LabelIPv6Addresses, ec2LabelPrimaryIPv6Addresses, ec2LabelDefaultIPv6Address} {
				if want, ok := tc.expected[label]; ok {
					require.Equal(t, want, target[label])
					continue
				}
				require.NotContains(t, target, label,
					"label sourced from the absent field should be omitted")
			}
			// The instance keeps its IPv4 address either way.
			require.Equal(t, model.LabelValue("10.0.0.1:4242"), target[model.AddressLabel])
		})
	}
}

func (m *mockEC2Client) DescribeInstances(_ context.Context, input *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	allowedStates := map[string]struct{}{}
	hasStateFilter := false
	for _, f := range input.Filters {
		if f.Name == nil || *f.Name != "instance-state-name" {
			continue
		}
		hasStateFilter = true
		for _, v := range f.Values {
			allowedStates[v] = struct{}{}
		}
	}

	r := ec2Types.Reservation{}
	for _, inst := range m.ec2Data.instances {
		if hasStateFilter {
			if inst.State == nil {
				continue
			}
			if _, ok := allowedStates[string(inst.State.Name)]; !ok {
				continue
			}
		}
		r.Instances = append(r.Instances, inst)
	}
	r.OwnerId = aws.String(m.ec2Data.ownerID)

	o := ec2.DescribeInstancesOutput{}
	o.Reservations = []ec2Types.Reservation{r}

	return &o, nil
}
