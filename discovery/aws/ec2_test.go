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
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// "Helper" functions.
const chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandIpv4() string {
	return fmt.Sprintf("%d.%d.%d.%d", RandNumber(1, 255), RandNumber(0, 255), RandNumber(0, 255), RandNumber(0, 255))
}

func RandNumber(minimum int, maximum int) int {
	return rand.Intn(maximum-minimum) + minimum
}

func RandString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}

	return string(b)
}

// Struct for test data.
type EC2Data struct {
	region string

	azNames  []string
	azIDs    []string
	azToAZID map[string]string

	ownerId string

	instances []*ec2.Instance
}

var ec2Data EC2Data

// Generate test data.
func GenerateTestAzData() {
	// tabula rasa
	ec2Data = EC2Data{}

	// region information
	ec2Data.region = fmt.Sprintf("region-%s", RandString(4))

	// availability zone information
	azCount := RandNumber(2, 5)
	ec2Data.azNames = make([]string, azCount)
	ec2Data.azIDs = make([]string, azCount)
	ec2Data.azToAZID = make(map[string]string, azCount)

	for i := 0; i < azCount; i++ {
		ec2Data.azNames[i] = fmt.Sprintf("azname-%s", RandString(5))
		ec2Data.azIDs[i] = fmt.Sprintf("azid-%s", RandString(5))
		ec2Data.azToAZID[ec2Data.azNames[i]] = ec2Data.azIDs[i]
	}

	// ownerId for the reservation
	ec2Data.ownerId = fmt.Sprintf("ownerid-%s", RandString(5))
}

func GenerateTestInstanceData() int {
	// without VPC configuration, just the "basics"
	azIdx := RandNumber(0, len(ec2Data.azNames))

	placement := ec2.Placement{}
	placement.SetAvailabilityZone(ec2Data.azNames[azIdx])

	// FIXME: this could be a "global" variable
	state := ec2.InstanceState{}
	state.SetName("running")

	instance := ec2.Instance{}
	instance.SetArchitecture(fmt.Sprintf("architecture-%s", RandString(8)))
	instance.SetImageId(fmt.Sprintf("ami-%s", RandString(8)))
	instance.SetInstanceId(fmt.Sprintf("instance-id-%s", RandString(8)))
	instance.SetInstanceLifecycle(fmt.Sprintf("instance-lifecycle-%s", RandString(8)))
	instance.SetInstanceType(fmt.Sprintf("instance-type-%s", RandString(8)))
	instance.SetPlacement(&placement)
	instance.SetPlatform(fmt.Sprintf("platform-%s", RandString(8)))
	instance.SetPrivateDnsName(fmt.Sprintf("private-dns-%s", RandString(8)))
	instance.SetPrivateIpAddress(RandIpv4())
	instance.SetPublicDnsName(fmt.Sprintf("public-ip-%s", RandString(8)))
	instance.SetPublicIpAddress(RandIpv4())
	instance.SetState(&state)

	tags := make([]*ec2.Tag, RandNumber(2, 5))
	for i := 0; i < len(tags); i++ {
		tags[i] = &ec2.Tag{}
		tags[i].SetKey(fmt.Sprintf("tag-%d-key-%s", i, RandString(8)))
		tags[i].SetValue(fmt.Sprintf("tag-%d-value-%s", i, RandString(8)))
	}

	instance.SetTags(tags)

	ec2Data.instances = append(ec2Data.instances, &instance)

	return azIdx
}

// The tests itself.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRefreshAZIDs(t *testing.T) {
	ctx := context.Background()
	client := &mockEC2Client{}

	GenerateTestAzData()

	d := &EC2Discovery{
		ec2: client,
	}

	err := d.refreshAZIDs(ctx)
	require.Nil(t, err)
	require.Equal(t, ec2Data.azToAZID, d.azToAZID)
}

func TestRefreshAZIDsHandleError(t *testing.T) {
	ctx := context.Background()
	client := &mockEC2Client{}

	ec2Data = EC2Data{}

	d := &EC2Discovery{
		ec2: client,
	}

	err := d.refreshAZIDs(ctx)
	require.Equal(t, "No AZs found", err.Error())
}

func TestRefreshNoPrivateIp(t *testing.T) {
	ctx := context.Background()
	client := &mockEC2Client{}

	GenerateTestAzData()

	instance := ec2.Instance{}
	instance.SetInstanceId(fmt.Sprintf("instance-id-%s", RandString(8)))

	ec2Data.instances = append(ec2Data.instances, &instance)

	d := &EC2Discovery{
		ec2: client,
		cfg: &EC2SDConfig{
			Region: ec2Data.region,
		},
	}

	expected := make([]*targetgroup.Group, 1)
	expected[0] = &targetgroup.Group{
		Source: ec2Data.region,
	}

	g, err := d.refresh(ctx)
	require.Equal(t, expected, g)
	require.Nil(t, err)
}

func TestRefreshNoVpc(t *testing.T) {
	ctx := context.Background()
	client := &mockEC2Client{}

	GenerateTestAzData()
	azIdx := GenerateTestInstanceData()

	d := &EC2Discovery{
		ec2: client,
		cfg: &EC2SDConfig{
			Port:   RandNumber(1024, 65535),
			Region: ec2Data.region,
		},
	}

	labels := model.LabelSet{
		"__address__":                     model.LabelValue(fmt.Sprintf("%s:%d", *ec2Data.instances[0].PrivateIpAddress, d.cfg.Port)),
		"__meta_ec2_ami":                  model.LabelValue(*ec2Data.instances[0].ImageId),
		"__meta_ec2_architecture":         model.LabelValue(*ec2Data.instances[0].Architecture),
		"__meta_ec2_availability_zone":    model.LabelValue(ec2Data.azNames[azIdx]),
		"__meta_ec2_availability_zone_id": model.LabelValue(ec2Data.azIDs[azIdx]),
		"__meta_ec2_instance_id":          model.LabelValue(*ec2Data.instances[0].InstanceId),
		"__meta_ec2_instance_lifecycle":   model.LabelValue(*ec2Data.instances[0].InstanceLifecycle),
		"__meta_ec2_instance_state":       model.LabelValue(*ec2Data.instances[0].State.Name),
		"__meta_ec2_instance_type":        model.LabelValue(*ec2Data.instances[0].InstanceType),
		"__meta_ec2_owner_id":             model.LabelValue(ec2Data.ownerId),
		"__meta_ec2_platform":             model.LabelValue(*ec2Data.instances[0].Platform),
		"__meta_ec2_private_dns_name":     model.LabelValue(*ec2Data.instances[0].PrivateDnsName),
		"__meta_ec2_private_ip":           model.LabelValue(*ec2Data.instances[0].PrivateIpAddress),
		"__meta_ec2_public_dns_name":      model.LabelValue(*ec2Data.instances[0].PublicDnsName),
		"__meta_ec2_public_ip":            model.LabelValue(*ec2Data.instances[0].PublicIpAddress),
		"__meta_ec2_region":               model.LabelValue(ec2Data.region),
	}

	for i := 0; i < len(ec2Data.instances[0].Tags); i++ {
		key := strings.Replace(fmt.Sprintf("__meta_ec2_tag_%s", *ec2Data.instances[0].Tags[i].Key), "-", "_", -1)
		labels[model.LabelName(key)] = model.LabelValue(*ec2Data.instances[0].Tags[i].Value)
	}

	expected := make([]*targetgroup.Group, 1)
	expected[0] = &targetgroup.Group{
		Source: ec2Data.region,
	}
	expected[0].Targets = append(expected[0].Targets, labels)

	g, err := d.refresh(ctx)
	require.Equal(t, expected, g)
	require.Nil(t, err)
}

// EC2 client mock.
type mockEC2Client struct {
	ec2iface.EC2API
}

func (m *mockEC2Client) DescribeAvailabilityZonesWithContext(ctx aws.Context, input *ec2.DescribeAvailabilityZonesInput, opts ...request.Option) (*ec2.DescribeAvailabilityZonesOutput, error) {
	if len(ec2Data.azNames) > 0 {
		azs := make([]*ec2.AvailabilityZone, len(ec2Data.azNames))

		for i := range ec2Data.azNames {
			azs[i] = &ec2.AvailabilityZone{
				ZoneName: &ec2Data.azNames[i],
				ZoneId:   &ec2Data.azIDs[i],
			}
		}

		return &ec2.DescribeAvailabilityZonesOutput{
			AvailabilityZones: azs,
		}, nil
	} else {
		return nil, errors.New("No AZs found")
	}
}

func (m *mockEC2Client) DescribeInstancesPagesWithContext(ctx aws.Context, input *ec2.DescribeInstancesInput, fn func(*ec2.DescribeInstancesOutput, bool) bool, opts ...request.Option) error {
	r := ec2.Reservation{}
	r.SetInstances(ec2Data.instances)
	r.SetOwnerId(ec2Data.ownerId)

	o := ec2.DescribeInstancesOutput{}
	o.SetReservations([]*ec2.Reservation{&r})

	_ = fn(&o, true)

	return nil
}
