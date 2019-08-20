package ram

import (
	"os"

	"github.com/denverdino/aliyungo/common"
)

const (
	// RAMDefaultEndpoint is the default API endpoint of RAM services
	RAMDefaultEndpoint = "https://ram.aliyuncs.com"
	RAMAPIVersion      = "2015-05-01"
)

type RamClient struct {
	common.Client
}

func NewClient(accessKeyId string, accessKeySecret string) RamClientInterface {
	return NewClientWithSecurityToken(accessKeyId, accessKeySecret, "")
}

func NewClientWithSecurityToken(accessKeyId string, accessKeySecret string, securityToken string) RamClientInterface {
	endpoint := os.Getenv("RAM_ENDPOINT")
	if endpoint == "" {
		endpoint = RAMDefaultEndpoint
	}

	return NewClientWithEndpointAndSecurityToken(endpoint, accessKeyId, accessKeySecret, securityToken)
}

func NewClientWithEndpoint(endpoint string, accessKeyId string, accessKeySecret string) RamClientInterface {
	return NewClientWithEndpointAndSecurityToken(endpoint, accessKeyId, accessKeySecret, "")
}

func NewClientWithEndpointAndSecurityToken(endpoint string, accessKeyId string, accessKeySecret string, securityToken string) RamClientInterface {
	client := &RamClient{}
	client.WithEndpoint(endpoint).
		WithVersion(RAMAPIVersion).
		WithAccessKeyId(accessKeyId).
		WithAccessKeySecret(accessKeySecret).
		WithSecurityToken(securityToken).
		InitClient()
	return client
}
