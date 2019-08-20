package kms

import (
	"os"

	"github.com/denverdino/aliyungo/common"
)

var (
	TestAccessKeyId     = os.Getenv("AccessKeyId")
	TestAccessKeySecret = os.Getenv("AccessKeySecret")
	TestSecurityToken   = os.Getenv("SecurityToken")
	TestRegionID        = common.Region(os.Getenv("RegionId"))
	TestKeyId           = os.Getenv("KeyId")

	encryptionContext = map[string]string{
		"Key1": "Value1",
		"Key2": "Value2",
	}

	debugClient = NetTestLocationClientForDebug()
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

var testLocationClient *Client

func NetTestLocationClientForDebug() *Client {
	if testLocationClient == nil {
		testLocationClient = NewClientWithRegion(TestAccessKeyId, TestAccessKeySecret, TestRegionID)
		testLocationClient.SetDebug(true)
	}

	return testLocationClient
}
