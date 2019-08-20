package cms

import (
	"os"

	"github.com/denverdino/aliyungo/common"
)

var (
	TestAccessKeyId     = os.Getenv("AccessKeyId")
	TestAccessKeySecret = os.Getenv("AccessKeySecret")
	TestSecurityToken   = os.Getenv("SecurityToken")
	TestRegionID        = common.Region(os.Getenv("RegionId"))
)

var testClient *CMSClient

func NewTestClient() *CMSClient {
	if testClient == nil {
		testClient = NewCMSClient(TestAccessKeyId, TestAccessKeySecret)
	}
	return testClient
}

var testDebugClient *CMSClient

func NewTestClientForDebug() *CMSClient {
	if testDebugClient == nil {
		testDebugClient = NewCMSClient(TestAccessKeyId, TestAccessKeySecret)
		testDebugClient.SetDebug(true)
	}
	return testDebugClient
}
