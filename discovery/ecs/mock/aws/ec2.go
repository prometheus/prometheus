package aws

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/prometheus/common/log"
	"github.com/prometheus/prometheus/discovery/ecs/mock/aws/sdk"
)

// MockEC2DescribeInstances mocks the description of instances EC2 API call
func MockEC2DescribeInstances(t *testing.T, mockMatcher *sdk.MockEC2API, wantError bool, instances ...*ec2.Instance) {
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
	mockMatcher.EXPECT().DescribeInstances(gomock.Any()).Do(func(input interface{}) {
		i := input.(*ec2.DescribeInstancesInput)
		if len(i.InstanceIds) == 0 {
			t.Errorf("Wrong api call, needs at least 1 instance ID")
		}
	}).AnyTimes().Return(result, err)
}
