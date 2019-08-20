package common

import "os"

var (
	TestAccessKeyId     = os.Getenv("AccessKeyId")
	TestAccessKeySecret = os.Getenv("AccessKeySecret")
	TestSecurityToken   = os.Getenv("SecurityToken")
	TestRegionId        = os.Getenv("RegionId")
	TestServiceCode     = os.Getenv("ServiceCode")
)
var testDebugClient *LocationClient

func NewTestClientForDebug() *LocationClient {
	if testDebugClient == nil {
		testDebugClient = NewLocationClient(TestAccessKeyId, TestAccessKeySecret, TestSecurityToken)
		testDebugClient.SetDebug(true)
	}
	return testDebugClient
}
