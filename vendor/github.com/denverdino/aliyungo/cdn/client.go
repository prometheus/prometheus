package cdn

import (
	"os"

	"github.com/denverdino/aliyungo/common"
)

const (
	// CDNDefaultEndpoint is the default API endpoint of CDN services
	CDNDefaultEndpoint = "https://cdn.aliyuncs.com"
	CDNAPIVersion      = "2014-11-11"
)

type CdnClient struct {
	common.Client
}

func NewClient(accessKeyId string, accessKeySecret string) *CdnClient {
	endpoint := os.Getenv("CDN_ENDPOINT")
	if endpoint == "" {
		endpoint = CDNDefaultEndpoint
	}
	return NewClientWithEndpoint(endpoint, accessKeyId, accessKeySecret)
}

func NewClientWithEndpoint(endpoint string, accessKeyId string, accessKeySecret string) *CdnClient {
	client := &CdnClient{}
	client.Init(endpoint, CDNAPIVersion, accessKeyId, accessKeySecret)
	return client
}
