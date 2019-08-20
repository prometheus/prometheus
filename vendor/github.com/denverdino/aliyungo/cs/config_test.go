package cs

import (
	"os"

	"github.com/denverdino/aliyungo/common"
)

//Modify with your Access Key Id and Access Key Secret

var (
	TestAccessKeyId     = os.Getenv("AccessKeyId")
	TestAccessKeySecret = os.Getenv("AccessKeySecret")
	TestSecurityToken   = os.Getenv("SecurityToken")
	TestRegionID        = common.Region(os.Getenv("RegionId"))
)

var testClient *Client

func NewTestClient() *Client {
	if testClient == nil {
		testClient = NewClient(TestAccessKeyId, TestAccessKeySecret)
	}
	return testClient
}

var testDebugClient *Client

func NewTestClientForDebug() *Client {
	if testDebugClient == nil {
		testDebugClient = NewClient(TestAccessKeyId, TestAccessKeySecret)
		testDebugClient.SetDebug(true)
	}
	return testDebugClient
}

var testDebugAussumeRoleClient *Client

func NewTestDebugAussumeRoleClient() *Client {
	if testDebugAussumeRoleClient == nil {
		testDebugAussumeRoleClient = NewClientForAussumeRole(TestAccessKeyId, TestAccessKeySecret, TestSecurityToken)
		testDebugAussumeRoleClient.SetDebug(true)
	}
	return testDebugAussumeRoleClient
}
