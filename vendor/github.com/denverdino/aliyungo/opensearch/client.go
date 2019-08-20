package opensearch

import "github.com/denverdino/aliyungo/common"

const (
	Internet   = ""
	Intranet   = "intranet."
	VPC        = "vpc."
	APIVersion = "v2"
)

type Client struct {
	common.Client
}

//OpenSearch的API比较奇怪，action不在公共参数里面
type OpenSearchArgs struct {
	Action string `ArgName:"action"`
}

func NewClient(networkType string, region common.Region, accessKeyId, accessKeySecret string) *Client {
	client := new(Client)
	client.Init("http://"+networkType+"opensearch-"+string(region)+".aliyuncs.com", APIVersion, accessKeyId, accessKeySecret)
	return client
}
