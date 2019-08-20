package location

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func TestDescribeEndpoint(t *testing.T) {
	client := NewTestClientForDebug()

	args := &DescribeEndpointArgs{
		Id:          common.Hangzhou,
		ServiceCode: "slb",
		Type:        "openAPI",
	}

	endpoint, err := client.DescribeEndpoint(args)
	if err != nil {
		t.Fatalf("Failed to describe endpoint %++v", err)
	}

	t.Logf("endpoint is %++v", endpoint)
}

func TestDescribeEndpoints(t *testing.T) {
	client := NewTestClientForDebug()

	args := &DescribeEndpointsArgs{
		Id:          common.Hangzhou,
		ServiceCode: "slb",
		Type:        "openAPI",
	}

	endpoints, err := client.DescribeEndpoints(args)
	if err != nil {
		t.Fatalf("Failed to describe endpoints %++v", err)
	}

	t.Logf("endpoints is %++v", endpoints)
}
