package sts

import (
	"os"

	"github.com/denverdino/aliyungo/ram"
)

/*
	Set your AccessKeyId and AccessKeySecret in env
	simply use the command below
	AccessKeyId=YourAccessKeyId AccessKeySecret=YourAccessKeySecret go test
*/
var (
	AccessKeyId     = os.Getenv("AccessKeyId")
	AccessKeySecret = os.Getenv("AccessKeySecret")
)

func NewRAMTestClient() ram.RamClientInterface {
	client := ram.NewClient(AccessKeyId, AccessKeySecret)
	return client
}

func NewTestClient() *STSClient {
	client := NewClient(AccessKeyId, AccessKeySecret)
	client.SetDebug(true)
	return client
}
