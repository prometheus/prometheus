// Copyright 2016 The Prometheus Authors
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
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/prometheus/common/log"
	"github.com/prometheus/prometheus/discovery/ecs/mock/aws/sdk"
	"github.com/stretchr/testify/mock"
)

// MockEC2DescribeInstances mocks the description of instances EC2 API call
func MockEC2DescribeInstances(t *testing.T, m *sdk.EC2API, wantError bool, instances ...*ec2.Instance) {
	log.Warnf("Mocking AWS iface: DescribeInstances")
	var err error
	if wantError {
		err = errors.New("DescribeInstances wrong!")
	}
	result := &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			&ec2.Reservation{
				Instances: instances,
			},
		},
	}
	m.On("DescribeInstances", mock.AnythingOfType("*ec2.DescribeInstancesInput")).Run(func(args mock.Arguments) {
		i := args.Get(0).(*ec2.DescribeInstancesInput)
		if len(i.InstanceIds) == 0 {
			t.Errorf("Wrong api call, needs at least 1 instance ID")
		}
	}).Return(result, err)
}
