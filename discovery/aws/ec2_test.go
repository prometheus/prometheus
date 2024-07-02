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
	"math/rand"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// "Helper" functions.
const chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}

	return string(b)
}

// Struct for test data.
type EC2Data struct {
	azNames  []string
	azIDs    []string
	azToAZID map[string]string
}

var ec2Data EC2Data

// Prepare test data.
func init() {
	// availability zone information
	azCount := rand.Intn(3) + 2
	ec2Data.azNames = make([]string, azCount)
	ec2Data.azIDs = make([]string, azCount)
	ec2Data.azToAZID = make(map[string]string, azCount)

	for i := 0; i < azCount; i++ {
		ec2Data.azNames[i] = RandString(5)
		ec2Data.azIDs[i] = RandString(6)
		ec2Data.azToAZID[ec2Data.azNames[i]] = ec2Data.azIDs[i]
	}
}

// The tests itself.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRefreshAZIDs(t *testing.T) {
	ctx := context.Background()
	client := &mockEC2Client{}

	d := &EC2Discovery{
		ec2: client,
	}

	err := d.refreshAZIDs(ctx)
	if err != nil {
		t.Fatalf("refreshAZIDs returned with an error [%v]\n", err)
	}
	require.Equal(t, ec2Data.azToAZID, d.azToAZID)
}

// EC2 client mock.
type mockEC2Client struct {
	ec2iface.EC2API
}

func (m *mockEC2Client) DescribeAvailabilityZonesWithContext(ctx aws.Context, input *ec2.DescribeAvailabilityZonesInput, opts ...request.Option) (*ec2.DescribeAvailabilityZonesOutput, error) {
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
}
