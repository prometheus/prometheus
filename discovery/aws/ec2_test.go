// Copyright 2024 The Prometheus Authors
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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Struct for test data.
type ec2DataStore struct {
	region string

	azNames  []string
	azIDs    []string
	azToAZID map[string]string

	ownerID string

	instances []*ec2.Instance
}

// The tests itself.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRefreshAZIDs(t *testing.T) {
	ctx := context.Background()
	client := newMockEC2Client(
		&ec2DataStore{
			azNames: []string{"azname-a", "azname-b", "azname-c"},
			azIDs:   []string{"azid-1", "azid-2", "azid-3"},
			azToAZID: map[string]string{
				"azname-a": "azid-1",
				"azname-b": "azid-2",
				"azname-c": "azid-3",
			},
		},
	)

	d := &EC2Discovery{
		ec2: client,
	}

	err := d.refreshAZIDs(ctx)
	require.NoError(t, err)
	require.Equal(t, client.ec2Data.azToAZID, d.azToAZID)
}

func TestRefreshAZIDsHandleError(t *testing.T) {
	ctx := context.Background()
	client := newMockEC2Client(
		&ec2DataStore{},
	)

	d := &EC2Discovery{
		ec2: client,
	}

	err := d.refreshAZIDs(ctx)
	require.Equal(t, "No AZs found", err.Error())
}

// Tests for the refresh function.
func TestEC2DiscoveryRefresh(t *testing.T) {
	// we need to get addresses of untyped string, int64 and boolean(!) constants as the AWS SDK expects
	addrB := func(b bool) *bool { return &b }
	addrI := func(i int64) *int64 { return &i }
	addrS := func(s string) *string { return &s }

	ctx := context.Background()

	// iterate through the test cases
	for _, tt := range []struct {
		name     string
		ec2Data  *ec2DataStore
		expected []*targetgroup.Group
	}{
		{
			name: "NoPrivateIp",
			ec2Data: &ec2DataStore{
				region:  "region-noprivateip",
				azNames: []string{"azname-a", "azname-b", "azname-c"},
				azIDs:   []string{"azid-1", "azid-2", "azid-3"},
				azToAZID: map[string]string{
					"azname-a": "azid-1",
					"azname-b": "azid-2",
					"azname-c": "azid-3",
				},
				instances: []*ec2.Instance{
					&ec2.Instance{
						InstanceId: addrS("instance-id-noprivateip"),
					},
				},
			},
			expected: []*targetgroup.Group{
				&targetgroup.Group{
					Source: "region-noprivateip",
				},
			},
		},
		{
			name: "NoVpc",
			ec2Data: &ec2DataStore{
				region:  "region-novpc",
				azNames: []string{"azname-a", "azname-b", "azname-c"},
				azIDs:   []string{"azid-1", "azid-2", "azid-3"},
				azToAZID: map[string]string{
					"azname-a": "azid-1",
					"azname-b": "azid-2",
					"azname-c": "azid-3",
				},
				ownerID: "owner-id-novpc",
				instances: []*ec2.Instance{
					&ec2.Instance{
						// set every possible options and test them here
						Architecture:      addrS("architecture-novpc"),
						ImageId:           addrS("ami-novpc"),
						InstanceId:        addrS("instance-id-novpc"),
						InstanceLifecycle: addrS("instance-lifecycle-novpc"),
						InstanceType:      addrS("instance-type-novpc"),
						Placement:         &ec2.Placement{AvailabilityZone: addrS("azname-b")},
						Platform:          addrS("platform-novpc"),
						PrivateDnsName:    addrS("private-dns-novpc"),
						PrivateIpAddress:  addrS("1.2.3.4"),
						PublicDnsName:     addrS("public-dns-novpc"),
						PublicIpAddress:   addrS("42.42.42.2"),
						State:             &ec2.InstanceState{Name: addrS("running")},
						// test tags once and for all
						Tags: []*ec2.Tag{
							&ec2.Tag{Key: addrS("tag-1-key"), Value: addrS("tag-1-value")},
							&ec2.Tag{Key: addrS("tag-2-key"), Value: addrS("tag-2-value")},
							nil,
							&ec2.Tag{Value: addrS("tag-4-value")},
							&ec2.Tag{Key: addrS("tag-5-key")},
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				&targetgroup.Group{
					Source: "region-novpc",
					Targets: []model.LabelSet{
						model.LabelSet{
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
				region:  "region-ipv4",
				azNames: []string{"azname-a", "azname-b", "azname-c"},
				azIDs:   []string{"azid-1", "azid-2", "azid-3"},
				azToAZID: map[string]string{
					"azname-a": "azid-1",
					"azname-b": "azid-2",
					"azname-c": "azid-3",
				},
				instances: []*ec2.Instance{
					&ec2.Instance{
						// just the minimum needed for the refresh work
						ImageId:          addrS("ami-ipv4"),
						InstanceId:       addrS("instance-id-ipv4"),
						InstanceType:     addrS("instance-type-ipv4"),
						Placement:        &ec2.Placement{AvailabilityZone: addrS("azname-c")},
						PrivateIpAddress: addrS("5.6.7.8"),
						State:            &ec2.InstanceState{Name: addrS("running")},
						SubnetId:         addrS("azid-3"),
						VpcId:            addrS("vpc-ipv4"),
						// network intefaces
						NetworkInterfaces: []*ec2.InstanceNetworkInterface{
							// interface without subnet -> should be ignored
							&ec2.InstanceNetworkInterface{
								Ipv6Addresses: []*ec2.InstanceIpv6Address{
									&ec2.InstanceIpv6Address{
										Ipv6Address:   addrS("2001:db8:1::1"),
										IsPrimaryIpv6: addrB(true),
									},
								},
							},
							// interface with subnet, no IPv6
							&ec2.InstanceNetworkInterface{
								Ipv6Addresses: []*ec2.InstanceIpv6Address{},
								SubnetId:      addrS("azid-3"),
							},
							// interface with another subnet, no IPv6
							&ec2.InstanceNetworkInterface{
								Ipv6Addresses: []*ec2.InstanceIpv6Address{},
								SubnetId:      addrS("azid-1"),
							},
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				&targetgroup.Group{
					Source: "region-ipv4",
					Targets: []model.LabelSet{
						model.LabelSet{
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
				region:  "region-ipv6",
				azNames: []string{"azname-a", "azname-b", "azname-c"},
				azIDs:   []string{"azid-1", "azid-2", "azid-3"},
				azToAZID: map[string]string{
					"azname-a": "azid-1",
					"azname-b": "azid-2",
					"azname-c": "azid-3",
				},
				instances: []*ec2.Instance{
					&ec2.Instance{
						// just the minimum needed for the refresh work
						ImageId:          addrS("ami-ipv6"),
						InstanceId:       addrS("instance-id-ipv6"),
						InstanceType:     addrS("instance-type-ipv6"),
						Placement:        &ec2.Placement{AvailabilityZone: addrS("azname-b")},
						PrivateIpAddress: addrS("9.10.11.12"),
						State:            &ec2.InstanceState{Name: addrS("running")},
						SubnetId:         addrS("azid-2"),
						VpcId:            addrS("vpc-ipv6"),
						// network intefaces
						NetworkInterfaces: []*ec2.InstanceNetworkInterface{
							// interface without primary IPv6, index 2
							&ec2.InstanceNetworkInterface{
								Attachment: &ec2.InstanceNetworkInterfaceAttachment{
									DeviceIndex: addrI(3),
								},
								Ipv6Addresses: []*ec2.InstanceIpv6Address{
									&ec2.InstanceIpv6Address{
										Ipv6Address:   addrS("2001:db8:2::1:1"),
										IsPrimaryIpv6: addrB(false),
									},
								},
								SubnetId: addrS("azid-2"),
							},
							// interface with primary IPv6, index 1
							&ec2.InstanceNetworkInterface{
								Attachment: &ec2.InstanceNetworkInterfaceAttachment{
									DeviceIndex: addrI(1),
								},
								Ipv6Addresses: []*ec2.InstanceIpv6Address{
									&ec2.InstanceIpv6Address{
										Ipv6Address:   addrS("2001:db8:2::2:1"),
										IsPrimaryIpv6: addrB(false),
									},
									&ec2.InstanceIpv6Address{
										Ipv6Address:   addrS("2001:db8:2::2:2"),
										IsPrimaryIpv6: addrB(true),
									},
								},
								SubnetId: addrS("azid-2"),
							},
							// interface with primary IPv6, index 3
							&ec2.InstanceNetworkInterface{
								Attachment: &ec2.InstanceNetworkInterfaceAttachment{
									DeviceIndex: addrI(3),
								},
								Ipv6Addresses: []*ec2.InstanceIpv6Address{
									&ec2.InstanceIpv6Address{
										Ipv6Address:   addrS("2001:db8:2::3:1"),
										IsPrimaryIpv6: addrB(true),
									},
								},
								SubnetId: addrS("azid-1"),
							},
							// interface without primary IPv6, index 0
							&ec2.InstanceNetworkInterface{
								Attachment: &ec2.InstanceNetworkInterfaceAttachment{
									DeviceIndex: addrI(0),
								},
								Ipv6Addresses: []*ec2.InstanceIpv6Address{},
								SubnetId:      addrS("azid-3"),
							},
						},
					},
				},
			},
			expected: []*targetgroup.Group{
				&targetgroup.Group{
					Source: "region-ipv6",
					Targets: []model.LabelSet{
						model.LabelSet{
							"__address__":                       model.LabelValue("9.10.11.12:4242"),
							"__meta_ec2_ami":                    model.LabelValue("ami-ipv6"),
							"__meta_ec2_availability_zone":      model.LabelValue("azname-b"),
							"__meta_ec2_availability_zone_id":   model.LabelValue("azid-2"),
							"__meta_ec2_instance_id":            model.LabelValue("instance-id-ipv6"),
							"__meta_ec2_instance_state":         model.LabelValue("running"),
							"__meta_ec2_instance_type":          model.LabelValue("instance-type-ipv6"),
							"__meta_ec2_ipv6_addresses":         model.LabelValue(",2001:db8:2::1:1,2001:db8:2::2:1,2001:db8:2::2:2,2001:db8:2::3:1,"),
							"__meta_ec2_owner_id":               model.LabelValue(""),
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
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockEC2Client(tt.ec2Data)

			d := &EC2Discovery{
				ec2: client,
				cfg: &EC2SDConfig{
					Port:   4242,
					Region: client.ec2Data.region,
				},
			}

			g, err := d.refresh(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.expected, g)
		})
	}
}

// EC2 client mock.
type mockEC2Client struct {
	ec2iface.EC2API
	ec2Data ec2DataStore
}

func newMockEC2Client(ec2Data *ec2DataStore) *mockEC2Client {
	client := mockEC2Client{
		ec2Data: *ec2Data,
	}
	return &client
}

func (m *mockEC2Client) DescribeAvailabilityZonesWithContext(ctx aws.Context, input *ec2.DescribeAvailabilityZonesInput, opts ...request.Option) (*ec2.DescribeAvailabilityZonesOutput, error) {
	if len(m.ec2Data.azNames) == 0 {
		return nil, errors.New("No AZs found")
	}

	azs := make([]*ec2.AvailabilityZone, len(m.ec2Data.azNames))

	for i := range m.ec2Data.azNames {
		azs[i] = &ec2.AvailabilityZone{
			ZoneName: &m.ec2Data.azNames[i],
			ZoneId:   &m.ec2Data.azIDs[i],
		}
	}

	return &ec2.DescribeAvailabilityZonesOutput{
		AvailabilityZones: azs,
	}, nil
}

func (m *mockEC2Client) DescribeInstancesPagesWithContext(ctx aws.Context, input *ec2.DescribeInstancesInput, fn func(*ec2.DescribeInstancesOutput, bool) bool, opts ...request.Option) error {
	r := ec2.Reservation{}
	r.SetInstances(m.ec2Data.instances)
	r.SetOwnerId(m.ec2Data.ownerID)

	o := ec2.DescribeInstancesOutput{}
	o.SetReservations([]*ec2.Reservation{&r})

	_ = fn(&o, true)

	return nil
}
