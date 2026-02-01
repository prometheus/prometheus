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

func (m *mockEC2Client) DescribeInstances(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	r := ec2Types.Reservation{}
	r.Instances = append(r.Instances, m.ec2Data.instances...)
	r.OwnerId = aws.String(m.ec2Data.ownerID)

	o := ec2.DescribeInstancesOutput{}
	o.Reservations = []ec2Types.Reservation{r}

	return &o, nil
}
