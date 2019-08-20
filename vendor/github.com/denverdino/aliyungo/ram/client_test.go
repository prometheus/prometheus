package ram

import (
	"os"
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

func NewTestClient() RamClientInterface {
	client := NewClient(AccessKeyId, AccessKeySecret)
	return client
}
