package slb

import (
	"os"

	"github.com/denverdino/aliyungo/common"
)

//Modify with your Access Key Id and Access Key Secret

var (
	TestAccessKeyId     = os.Getenv("AccessKeyId")
	TestAccessKeySecret = os.Getenv("AccessKeySecret")
	TestSecurityToken   = os.Getenv("SecurityToken")
	TestLoadBlancerID   = "MY_LOADBALANCEID"
	TestVServerGroupID  = "MY_VSERVER_GROUPID"
	TestListenerPort    = 9000
	TestInstanceId      = "MY_INSTANCE_ID"
	TestRegionID        = common.Region(os.Getenv("RegionId"))
	TestIAmRich         = false
	TestQuick           = false
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

var testDebugNewSLBClient *Client

func NewTestNewSLBClientForDebug() *Client {
	if testDebugNewSLBClient == nil {
		testDebugNewSLBClient = NewSLBClientWithSecurityToken(TestAccessKeyId, TestAccessKeySecret, TestSecurityToken, TestRegionID)
		testDebugNewSLBClient.SetDebug(true)
	}
	return testDebugNewSLBClient
}
