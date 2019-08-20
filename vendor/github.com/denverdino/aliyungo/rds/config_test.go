package rds

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

	ZoneId                 = "MY_TEST_ZONE_ID"
	VPCId                  = "MY_TEST_VPC_ID"
	VSwitchId              = "MY_TEST_VSWITCH_ID"
	DBInstanceId           = "MY_TEST_INSTANCE_ID"
	DBName                 = "MY_TEST_DB_NAME"
	AccountPassword        = "MY_TEST_ACCOUNT_PWD"
	AccountName            = "MY_TEST_ACCOUNT_NAME"
	EngineVersion          = "MY_TEST_ENGINE_VERSION"
	DBInstanceClass        = "MY_TEST_DB_CLASS"
	DBInstanceUpgradeId    = "MY_TEST_INSTANCE_TO_UPGRADE"
	DBInstanceUpgradeClass = "MY_TEST_DB_CLASS_TO_UPGRADE"
	TestIAmRich            = false
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
