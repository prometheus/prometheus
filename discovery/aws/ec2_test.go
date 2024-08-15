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

package aws

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
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

// "Helper" functions.
const (
	chars     = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	ipv6chars = "0123456789abcdef"
)

func randIpv4() string {
	return fmt.Sprintf("%d.%d.%d.%d", randNumber(1, 255), randNumber(0, 255), randNumber(0, 255), randNumber(0, 255))
}

func randIpv6() string {
	address := "2000"

	for i := 0; i < 7; i++ {
		e := make([]byte, 4)
		for j := range e {
			e[j] = ipv6chars[rand.Intn(len(ipv6chars))]
		}
		address += ":" + string(e)
	}

	return address
}

func randNumber(minimum, maximum int) int {
	return rand.Intn(maximum-minimum) + minimum
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}

	return string(b)
}

// Struct for test data.
type ec2DataStore struct {
	region string

	azNames  []string
	azIDs    []string
	azToAZID map[string]string

	ownerID string

	instances []*ec2.Instance
}

// Generate test data.
func generateTestAzData(ec2Data *ec2DataStore) {
	// region information
	ec2Data.region = fmt.Sprintf("region-%s", randString(4))

	// availability zone information
	azCount := randNumber(3, 5)
	ec2Data.azNames = make([]string, azCount)
	ec2Data.azIDs = make([]string, azCount)
	ec2Data.azToAZID = make(map[string]string, azCount)

	for i := 0; i < azCount; i++ {
		ec2Data.azNames[i] = fmt.Sprintf("azname-%s", randString(5))
		ec2Data.azIDs[i] = fmt.Sprintf("azid-%s", randString(5))
		ec2Data.azToAZID[ec2Data.azNames[i]] = ec2Data.azIDs[i]
	}

	// ownerId for the reservation
	ec2Data.ownerID = fmt.Sprintf("ownerid-%s", randString(5))
}

func generateTestInstanceData(ec2Data *ec2DataStore) int {
	// without VPC configuration, just the "basics"
	azIdx := randNumber(0, len(ec2Data.azNames))

	placement := ec2.Placement{}
	placement.SetAvailabilityZone(ec2Data.azNames[azIdx])

	// FIXME: this could be a "global" variable
	state := ec2.InstanceState{}
	state.SetName("running")

	instance := ec2.Instance{}
	instance.SetArchitecture(fmt.Sprintf("architecture-%s", randString(8)))
	instance.SetImageId(fmt.Sprintf("ami-%s", randString(8)))
	instance.SetInstanceId(fmt.Sprintf("instance-id-%s", randString(8)))
	instance.SetInstanceLifecycle(fmt.Sprintf("instance-lifecycle-%s", randString(8)))
	instance.SetInstanceType(fmt.Sprintf("instance-type-%s", randString(8)))
	instance.SetPlacement(&placement)
	instance.SetPlatform(fmt.Sprintf("platform-%s", randString(8)))
	instance.SetPrivateDnsName(fmt.Sprintf("private-dns-%s", randString(8)))
	instance.SetPrivateIpAddress(randIpv4())
	instance.SetPublicDnsName(fmt.Sprintf("public-ip-%s", randString(8)))
	instance.SetPublicIpAddress(randIpv4())
	instance.SetState(&state)

	tags := make([]*ec2.Tag, randNumber(2, 5))
	for i := 0; i < len(tags); i++ {
		tags[i] = &ec2.Tag{}
		tags[i].SetKey(fmt.Sprintf("tag-%d-key-%s", i, randString(8)))
		tags[i].SetValue(fmt.Sprintf("tag-%d-value-%s", i, randString(8)))
	}

	instance.SetTags(tags)

	ec2Data.instances = append(ec2Data.instances, &instance)

	return azIdx
}

func generateExpectedLabels(ec2Data *ec2DataStore, azIdx, port int) model.LabelSet {
	labels := model.LabelSet{
		"__address__":                     model.LabelValue(fmt.Sprintf("%s:%d", *ec2Data.instances[0].PrivateIpAddress, port)),
		"__meta_ec2_ami":                  model.LabelValue(*ec2Data.instances[0].ImageId),
		"__meta_ec2_architecture":         model.LabelValue(*ec2Data.instances[0].Architecture),
		"__meta_ec2_availability_zone":    model.LabelValue(ec2Data.azNames[azIdx]),
		"__meta_ec2_availability_zone_id": model.LabelValue(ec2Data.azIDs[azIdx]),
		"__meta_ec2_instance_id":          model.LabelValue(*ec2Data.instances[0].InstanceId),
		"__meta_ec2_instance_lifecycle":   model.LabelValue(*ec2Data.instances[0].InstanceLifecycle),
		"__meta_ec2_instance_state":       model.LabelValue(*ec2Data.instances[0].State.Name),
		"__meta_ec2_instance_type":        model.LabelValue(*ec2Data.instances[0].InstanceType),
		"__meta_ec2_owner_id":             model.LabelValue(ec2Data.ownerID),
		"__meta_ec2_platform":             model.LabelValue(*ec2Data.instances[0].Platform),
		"__meta_ec2_private_dns_name":     model.LabelValue(*ec2Data.instances[0].PrivateDnsName),
		"__meta_ec2_private_ip":           model.LabelValue(*ec2Data.instances[0].PrivateIpAddress),
		"__meta_ec2_public_dns_name":      model.LabelValue(*ec2Data.instances[0].PublicDnsName),
		"__meta_ec2_public_ip":            model.LabelValue(*ec2Data.instances[0].PublicIpAddress),
		"__meta_ec2_region":               model.LabelValue(ec2Data.region),
	}

	for i := 0; i < len(ec2Data.instances[0].Tags); i++ {
		key := strings.ReplaceAll(fmt.Sprintf("__meta_ec2_tag_%s", *ec2Data.instances[0].Tags[i].Key), "-", "_")
		labels[model.LabelName(key)] = model.LabelValue(*ec2Data.instances[0].Tags[i].Value)
	}

	return labels
}

// The tests itself.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRefreshAZIDs(t *testing.T) {
	ctx := context.Background()
	client := newMockEC2Client()
	ec2Data := &client.ec2Data

	generateTestAzData(ec2Data)

	d := &EC2Discovery{
		ec2: client,
	}

	err := d.refreshAZIDs(ctx)
	require.NoError(t, err)
	require.Equal(t, ec2Data.azToAZID, d.azToAZID)
}

func TestRefreshAZIDsHandleError(t *testing.T) {
	ctx := context.Background()
	client := newMockEC2Client()

	d := &EC2Discovery{
		ec2: client,
	}

	err := d.refreshAZIDs(ctx)
	require.Equal(t, "No AZs found", err.Error())
}

// Tests for the refresh function.
//
//	name - the name of the test
//	generator - a generator function for the input and expected output
type generatorFunction func(*EC2Discovery) []*targetgroup.Group

var refreshTests = []struct {
	name      string
	generator generatorFunction
}{
	{
		name:      "NoPrivateIp",
		generator: generateRefreshNoPrivateIP,
	},
	{
		name:      "NoVpc",
		generator: generateRefreshNoVpc,
	},
	{
		name:      "Ipv4",
		generator: generateRefreshIpv4,
	},
	{
		name:      "Ipv6",
		generator: generateRefreshIpv6,
	},
}

func generateRefreshNoPrivateIP(d *EC2Discovery) []*targetgroup.Group {
	ec2Data := &d.ec2.(*mockEC2Client).ec2Data

	instance := ec2.Instance{}
	instance.SetInstanceId(fmt.Sprintf("instance-id-%s", randString(8)))

	ec2Data.instances = append(ec2Data.instances, &instance)

	expected := make([]*targetgroup.Group, 1)
	expected[0] = &targetgroup.Group{
		Source: ec2Data.region,
	}

	return expected
}

func generateRefreshNoVpc(d *EC2Discovery) []*targetgroup.Group {
	ec2Data := &d.ec2.(*mockEC2Client).ec2Data

	azIdx := generateTestInstanceData(ec2Data)

	expected := make([]*targetgroup.Group, 1)
	expected[0] = &targetgroup.Group{
		Source: ec2Data.region,
	}
	expected[0].Targets = append(expected[0].Targets, generateExpectedLabels(ec2Data, azIdx, d.cfg.Port))

	return expected
}

func generateRefreshIpv4(d *EC2Discovery) []*targetgroup.Group {
	ec2Data := &d.ec2.(*mockEC2Client).ec2Data

	azIdx := generateTestInstanceData(ec2Data)

	ec2Data.instances[0].SetSubnetId(ec2Data.azIDs[azIdx])
	ec2Data.instances[0].SetVpcId(fmt.Sprintf("vpc-%s", randString(8)))

	enis := make([]*ec2.InstanceNetworkInterface, 4)

	// interface in the primary subnet
	enis[0] = &ec2.InstanceNetworkInterface{}
	enis[0].SetIpv6Addresses([]*ec2.InstanceIpv6Address{})
	enis[0].SetSubnetId(*ec2Data.instances[0].SubnetId)

	// interface without any subnet
	enis[1] = &ec2.InstanceNetworkInterface{}
	enis[1].SetIpv6Addresses([]*ec2.InstanceIpv6Address{})
	// azIdx + 1 modulo #azs
	secondAzID := (azIdx + 1) % len(ec2Data.azIDs)
	enis[1].SetSubnetId(ec2Data.azIDs[secondAzID])

	// interface in another subnet
	enis[2] = &ec2.InstanceNetworkInterface{}
	enis[2].SetIpv6Addresses([]*ec2.InstanceIpv6Address{})

	// inteface in the primary subnet
	enis[3] = &ec2.InstanceNetworkInterface{}
	enis[3].SetIpv6Addresses([]*ec2.InstanceIpv6Address{})
	enis[3].SetSubnetId(*ec2Data.instances[0].SubnetId)

	ec2Data.instances[0].SetNetworkInterfaces(enis)

	labels := generateExpectedLabels(ec2Data, azIdx, d.cfg.Port)

	labels["__meta_ec2_primary_subnet_id"] = model.LabelValue(*ec2Data.instances[0].SubnetId)
	labels["__meta_ec2_subnet_id"] = model.LabelValue(fmt.Sprintf(",%s,%s,", ec2Data.azIDs[azIdx], ec2Data.azIDs[secondAzID]))
	labels["__meta_ec2_vpc_id"] = model.LabelValue(*ec2Data.instances[0].VpcId)

	expected := make([]*targetgroup.Group, 1)
	expected[0] = &targetgroup.Group{
		Source: ec2Data.region,
	}
	expected[0].Targets = append(expected[0].Targets, labels)

	return expected
}

func generateRefreshIpv6(d *EC2Discovery) []*targetgroup.Group {
	ec2Data := &d.ec2.(*mockEC2Client).ec2Data

	azIdx := generateTestInstanceData(ec2Data)

	ec2Data.instances[0].SetSubnetId(ec2Data.azIDs[azIdx])
	ec2Data.instances[0].SetVpcId(fmt.Sprintf("vpc-%s", randString(8)))

	attachments := make([]*ec2.InstanceNetworkInterfaceAttachment, 4)
	enis := make([]*ec2.InstanceNetworkInterface, 4)
	ips := make([][]*ec2.InstanceIpv6Address, 4)

	// interface without primary IPv6
	attachments[0] = &ec2.InstanceNetworkInterfaceAttachment{}
	attachments[0].SetDeviceIndex(3)

	ips[0] = make([]*ec2.InstanceIpv6Address, 1)
	ips[0][0] = &ec2.InstanceIpv6Address{}
	ips[0][0].SetIpv6Address(randIpv6())
	ips[0][0].SetIsPrimaryIpv6(false)

	enis[0] = &ec2.InstanceNetworkInterface{}
	enis[0].SetAttachment(attachments[0])
	enis[0].SetIpv6Addresses(ips[0])
	enis[0].SetSubnetId(*ec2Data.instances[0].SubnetId)

	// interface with primary IPv6
	attachments[1] = &ec2.InstanceNetworkInterfaceAttachment{}
	attachments[1].SetDeviceIndex(2)

	ips[1] = make([]*ec2.InstanceIpv6Address, 2)
	ips[1][0] = &ec2.InstanceIpv6Address{}
	ips[1][0].SetIpv6Address(randIpv6())
	ips[1][0].SetIsPrimaryIpv6(false)
	ips[1][1] = &ec2.InstanceIpv6Address{}
	ips[1][1].SetIpv6Address(randIpv6())
	ips[1][1].SetIsPrimaryIpv6(true)

	enis[1] = &ec2.InstanceNetworkInterface{}
	enis[1].SetAttachment(attachments[1])
	enis[1].SetIpv6Addresses(ips[1])
	enis[1].SetSubnetId(*ec2Data.instances[0].SubnetId)

	// interface with primary IPv6
	attachments[2] = &ec2.InstanceNetworkInterfaceAttachment{}
	attachments[2].SetDeviceIndex(1)

	ips[2] = make([]*ec2.InstanceIpv6Address, 1)
	ips[2][0] = &ec2.InstanceIpv6Address{}
	ips[2][0].SetIpv6Address(randIpv6())
	ips[2][0].SetIsPrimaryIpv6(true)

	enis[2] = &ec2.InstanceNetworkInterface{}
	enis[2].SetAttachment(attachments[2])
	enis[2].SetIpv6Addresses(ips[2])
	enis[2].SetSubnetId(*ec2Data.instances[0].SubnetId)

	// interface without primary IPv6
	attachments[3] = &ec2.InstanceNetworkInterfaceAttachment{}
	attachments[3].SetDeviceIndex(0)

	enis[3] = &ec2.InstanceNetworkInterface{}
	enis[3].SetAttachment(attachments[3])
	enis[3].SetIpv6Addresses([]*ec2.InstanceIpv6Address{})
	enis[3].SetSubnetId(*ec2Data.instances[0].SubnetId)

	ec2Data.instances[0].SetNetworkInterfaces(enis)

	labels := generateExpectedLabels(ec2Data, azIdx, d.cfg.Port)

	labels["__meta_ec2_ipv6_addresses"] = model.LabelValue(
		fmt.Sprintf(",%s,%s,%s,%s,",
			*ips[0][0].Ipv6Address,
			*ips[1][0].Ipv6Address,
			*ips[1][1].Ipv6Address,
			*ips[2][0].Ipv6Address,
		),
	)
	labels["__meta_ec2_primary_ipv6_addresses"] = model.LabelValue(
		fmt.Sprintf(",,%s,%s,", *ips[2][0].Ipv6Address, *ips[1][1].Ipv6Address),
	)
	labels["__meta_ec2_primary_subnet_id"] = model.LabelValue(*ec2Data.instances[0].SubnetId)
	labels["__meta_ec2_subnet_id"] = model.LabelValue(fmt.Sprintf(",%s,", ec2Data.azIDs[azIdx]))
	labels["__meta_ec2_vpc_id"] = model.LabelValue(*ec2Data.instances[0].VpcId)

	expected := make([]*targetgroup.Group, 1)
	expected[0] = &targetgroup.Group{
		Source: ec2Data.region,
	}
	expected[0].Targets = append(expected[0].Targets, labels)

	return expected
}

func TestRefresh(t *testing.T) {
	ctx := context.Background()
	client := newMockEC2Client()

	// iterate through the refreshTests array
	for _, tt := range refreshTests {
		t.Run(tt.name, func(t *testing.T) {
			// generate basic availability zone test data
			client.ec2Data = ec2DataStore{}
			generateTestAzData(&client.ec2Data)

			d := &EC2Discovery{
				ec2: client,
				cfg: &EC2SDConfig{
					Port:   randNumber(1024, 65535),
					Region: client.ec2Data.region,
				},
			}

			expected := tt.generator(d)

			g, err := d.refresh(ctx)
			require.NoError(t, err)
			require.Equal(t, expected, g)
		})
	}
}

// EC2 client mock.
type mockEC2Client struct {
	ec2iface.EC2API
	ec2Data ec2DataStore
}

func newMockEC2Client() *mockEC2Client {
	client := mockEC2Client{
		ec2Data: ec2DataStore{},
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
