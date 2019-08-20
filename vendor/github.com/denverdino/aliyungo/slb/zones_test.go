package slb

import (
	"github.com/denverdino/aliyungo/common"
	"testing"
)

func TestDescribeZones(t *testing.T) {

	client := NewTestNewSLBClientForDebug()

	zones, err := client.DescribeZones(common.Hangzhou)

	if err == nil {
		t.Logf("regions: %v", zones)
	} else {
		t.Errorf("Failed to DescribeZones: %v", err)
	}

}
