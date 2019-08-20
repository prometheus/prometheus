package kms

import (
	"os"

	"github.com/denverdino/aliyungo/common"
)

const (
	// KMSDefaultEndpoint is the default API endpoint of KMS services
	KMSDefaultEndpoint = "https://kms.cn-hangzhou.aliyuncs.com"
	KMSAPIVersion      = "2016-01-20"
	KMSServiceCode     = "kms"
)

type Client struct {
	common.Client
}

// NewClient creates a new instance of KMS client
func NewClient(accessKeyId, accessKeySecret string) *Client {
	endpoint := os.Getenv("KMS_ENDPOINT")
	if endpoint == "" {
		endpoint = KMSDefaultEndpoint
	}
	return NewClientWithEndpoint(endpoint, accessKeyId, accessKeySecret)
}

func NewClientWithRegion(accessKeyId string, accessKeySecret string, regionID common.Region) *Client {
	endpoint := os.Getenv("KMS_ENDPOINT")
	if endpoint == "" {
		endpoint = KMSDefaultEndpoint
	}
	client := &Client{}
	client.NewInit(endpoint, KMSAPIVersion, accessKeyId, accessKeySecret, KMSServiceCode, regionID)
	return client
}

func NewClientWithEndpoint(endpoint string, accessKeyId string, accessKeySecret string) *Client {
	client := &Client{}
	client.Init(endpoint, KMSAPIVersion, accessKeyId, accessKeySecret)
	return client
}

func NewECSClientWithSecurityToken(accessKeyId string, accessKeySecret string, securityToken string, regionID common.Region) *Client {
	endpoint := os.Getenv("KMS_ENDPOINT")
	if endpoint == "" {
		endpoint = KMSDefaultEndpoint
	}

	return NewKMSClientWithEndpointAndSecurityToken(endpoint, accessKeyId, accessKeySecret, securityToken, regionID)
}

func NewKMSClientWithEndpointAndSecurityToken(endpoint string, accessKeyId string, accessKeySecret string, securityToken string, regionID common.Region) *Client {
	client := &Client{}
	client.WithEndpoint(endpoint).
		WithVersion(KMSAPIVersion).
		WithAccessKeyId(accessKeyId).
		WithAccessKeySecret(accessKeySecret).
		WithSecurityToken(securityToken).
		WithServiceCode(KMSServiceCode).
		WithRegionID(regionID).
		InitClient()
	return client
}
