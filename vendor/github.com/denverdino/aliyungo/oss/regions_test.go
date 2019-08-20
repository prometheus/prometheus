package oss_test

import (
	"github.com/denverdino/aliyungo/oss"

	"testing"
)

func TestRegionEndpoint(t *testing.T) {
	t.Log(oss.Beijing.GetInternalEndpoint("test", false))
	t.Log(oss.Beijing.GetVPCInternalEndpoint("test", true))
	t.Log(oss.USEast1.GetVPCInternalEndpoint("test", true))
}
