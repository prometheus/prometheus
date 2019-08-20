package location

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func TestDescribeServices(t *testing.T) {
	client := NewTestClientForDebug()

	args := &DescribeServicesArgs{
		RegionId: common.Beijing,
	}

	services, err := client.DescribeServices(args)
	if err != nil {
		t.Fatalf("Failed to describe services %++v", err)
	}

	t.Logf("services is %++v", services)
}
