package sts

import (
	"os"

	"github.com/denverdino/aliyungo/common"
)

const (
	// STSDefaultEndpoint is the default API endpoint of STS services
	STSDefaultEndpoint = "https://sts.aliyuncs.com"
	STSAPIVersion      = "2015-04-01"
)

type STSClient struct {
	common.Client
}

func NewClient(accessKeyId string, accessKeySecret string) *STSClient {
	return NewClientWithSecurityToken(accessKeyId, accessKeySecret, "")
}

func NewClientWithSecurityToken(accessKeyId string, accessKeySecret string, securityToken string) *STSClient {
	endpoint := os.Getenv("STS_ENDPOINT")
	if endpoint == "" {
		endpoint = STSDefaultEndpoint
	}

	return NewClientWithEndpointAndSecurityToken(endpoint, accessKeyId, accessKeySecret, securityToken)
}

func NewClientWithEndpoint(endpoint string, accessKeyId string, accessKeySecret string) *STSClient {
	return NewClientWithEndpointAndSecurityToken(endpoint, accessKeyId, accessKeySecret, "")
}

func NewClientWithEndpointAndSecurityToken(endpoint string, accessKeyId string, accessKeySecret string, securityToken string) *STSClient {
	client := &STSClient{}
	client.WithEndpoint(endpoint).
		WithVersion(STSAPIVersion).
		WithAccessKeyId(accessKeyId).
		WithAccessKeySecret(accessKeySecret).
		WithSecurityToken(securityToken).
		InitClient()
	return client
}
