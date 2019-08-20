package oss_test

import (
	"os"

	"github.com/denverdino/aliyungo/oss"
)

//
//There are two ways to run unit tests here:
// 1. Set your AccessKeyId and AccessKeySecret in env
//		simply use the command below:
//			"AccessKeyId=YourAccessKeyId AccessKeySecret=YourAccessKeySecret go test"
//
// 2. Replace "MY_ACCESS_KEY_ID" & "MY_ACCESS_KEY_SECRET" with your own access key & secret and run "go test"
//

var (
	TestAccessKeyId     = os.Getenv("AccessKeyId")
	TestAccessKeySecret = os.Getenv("AccessKeySecret")
	TestSecurityToken   = os.Getenv("SecurityToken")
	TestRegion          = oss.Region(os.Getenv("RegionId"))
)
