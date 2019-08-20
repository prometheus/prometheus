package slb

import (
	"os"

	"github.com/denverdino/aliyungo/common"
)

type Client struct {
	common.Client
}

const (
	// SLBDefaultEndpoint is the default API endpoint of SLB services
	SLBDefaultEndpoint = "https://slb.aliyuncs.com"
	SLBAPIVersion      = "2014-05-15"

	SLBServiceCode = "slb"
)

// NewClient creates a new instance of ECS client
func NewClient(accessKeyId, accessKeySecret string) *Client {
	endpoint := os.Getenv("SLB_ENDPOINT")
	if endpoint == "" {
		endpoint = SLBDefaultEndpoint
	}
	return NewClientWithEndpoint(endpoint, accessKeyId, accessKeySecret)
}

func NewClientWithEndpoint(endpoint string, accessKeyId, accessKeySecret string) *Client {
	client := &Client{}
	client.Init(endpoint, SLBAPIVersion, accessKeyId, accessKeySecret)
	return client
}

func NewSLBClient(accessKeyId, accessKeySecret string, regionID common.Region) *Client {
	endpoint := os.Getenv("SLB_ENDPOINT")
	if endpoint == "" {
		endpoint = SLBDefaultEndpoint
	}

	return NewClientWithRegion(endpoint, accessKeyId, accessKeySecret, regionID)
}

func NewClientWithRegion(endpoint string, accessKeyId, accessKeySecret string, regionID common.Region) *Client {
	client := &Client{}
	client.NewInit(endpoint, SLBAPIVersion, accessKeyId, accessKeySecret, SLBServiceCode, regionID)
	return client
}

func NewSLBClientWithSecurityToken(accessKeyId string, accessKeySecret string, securityToken string, regionID common.Region) *Client {
	endpoint := os.Getenv("SLB_ENDPOINT")
	if endpoint == "" {
		endpoint = SLBDefaultEndpoint
	}

	return NewSLBClientWithEndpointAndSecurityToken(endpoint, accessKeyId, accessKeySecret, securityToken, regionID)
}

func NewSLBClientWithEndpointAndSecurityToken(endpoint string, accessKeyId string, accessKeySecret string, securityToken string, regionID common.Region) *Client {
	client := &Client{}
	client.WithEndpoint(endpoint).
		WithVersion(SLBAPIVersion).
		WithAccessKeyId(accessKeyId).
		WithAccessKeySecret(accessKeySecret).
		WithSecurityToken(securityToken).
		WithServiceCode(SLBServiceCode).
		WithRegionID(regionID).
		InitClient()
	return client
}
