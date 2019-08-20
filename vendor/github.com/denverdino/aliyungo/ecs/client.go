package ecs

import (
	"os"

	"github.com/denverdino/aliyungo/common"
)

// Interval for checking status in WaitForXXX method
const DefaultWaitForInterval = 5

// Default timeout value for WaitForXXX method
const DefaultTimeout = 60

type Client struct {
	common.Client
}

const (
	// ECSDefaultEndpoint is the default API endpoint of ECS services
	ECSDefaultEndpoint = "https://ecs-cn-hangzhou.aliyuncs.com"
	ECSAPIVersion      = "2014-05-26"
	ECSServiceCode     = "ecs"

	VPCDefaultEndpoint = "https://vpc.aliyuncs.com"
	VPCAPIVersion      = "2016-04-28"
	VPCServiceCode     = "vpc"
)

// NewClient creates a new instance of ECS client
func NewClient(accessKeyId, accessKeySecret string) *Client {
	endpoint := os.Getenv("ECS_ENDPOINT")
	if endpoint == "" {
		endpoint = ECSDefaultEndpoint
	}
	return NewClientWithEndpoint(endpoint, accessKeyId, accessKeySecret)
}

func NewClientWithRegion(endpoint string, accessKeyId string, accessKeySecret string, regionID common.Region) *Client {
	client := &Client{}
	client.NewInit(endpoint, ECSAPIVersion, accessKeyId, accessKeySecret, ECSServiceCode, regionID)
	return client
}

func NewClientWithEndpoint(endpoint string, accessKeyId string, accessKeySecret string) *Client {
	client := &Client{}
	client.Init(endpoint, ECSAPIVersion, accessKeyId, accessKeySecret)
	return client
}

// ---------------------------------------
// NewECSClient creates a new instance of ECS client
// ---------------------------------------
func NewECSClient(accessKeyId, accessKeySecret string, regionID common.Region) *Client {
	return NewECSClientWithSecurityToken(accessKeyId, accessKeySecret, "", regionID)
}

func NewECSClientWithSecurityToken(accessKeyId string, accessKeySecret string, securityToken string, regionID common.Region) *Client {
	endpoint := os.Getenv("ECS_ENDPOINT")
	if endpoint == "" {
		endpoint = ECSDefaultEndpoint
	}

	return NewECSClientWithEndpointAndSecurityToken(endpoint, accessKeyId, accessKeySecret, securityToken, regionID)
}

func NewECSClientWithEndpoint(endpoint string, accessKeyId string, accessKeySecret string, regionID common.Region) *Client {
	return NewECSClientWithEndpointAndSecurityToken(endpoint, accessKeyId, accessKeySecret, "", regionID)
}

func NewECSClientWithEndpointAndSecurityToken(endpoint string, accessKeyId string, accessKeySecret string, securityToken string, regionID common.Region) *Client {
	client := &Client{}
	client.WithEndpoint(endpoint).
		WithVersion(ECSAPIVersion).
		WithAccessKeyId(accessKeyId).
		WithAccessKeySecret(accessKeySecret).
		WithSecurityToken(securityToken).
		WithServiceCode(ECSServiceCode).
		WithRegionID(regionID).
		InitClient()
	return client
}

// ---------------------------------------
// NewVPCClient creates a new instance of VPC client
// ---------------------------------------
func NewVPCClient(accessKeyId string, accessKeySecret string, regionID common.Region) *Client {
	return NewVPCClientWithSecurityToken(accessKeyId, accessKeySecret, "", regionID)
}

func NewVPCClientWithSecurityToken(accessKeyId string, accessKeySecret string, securityToken string, regionID common.Region) *Client {
	endpoint := os.Getenv("VPC_ENDPOINT")
	if endpoint == "" {
		endpoint = VPCDefaultEndpoint
	}

	return NewVPCClientWithEndpointAndSecurityToken(endpoint, accessKeyId, accessKeySecret, securityToken, regionID)
}

func NewVPCClientWithEndpoint(endpoint string, accessKeyId string, accessKeySecret string, regionID common.Region) *Client {
	return NewVPCClientWithEndpointAndSecurityToken(endpoint, accessKeyId, accessKeySecret, "", regionID)
}

func NewVPCClientWithEndpointAndSecurityToken(endpoint string, accessKeyId string, accessKeySecret string, securityToken string, regionID common.Region) *Client {
	client := &Client{}
	client.WithEndpoint(endpoint).
		WithVersion(VPCAPIVersion).
		WithAccessKeyId(accessKeyId).
		WithAccessKeySecret(accessKeySecret).
		WithSecurityToken(securityToken).
		WithServiceCode(VPCServiceCode).
		WithRegionID(regionID).
		InitClient()
	return client
}

// ---------------------------------------
// NewVPCClientWithRegion creates a new instance of VPC client automatically get endpoint
// ---------------------------------------
func NewVPCClientWithRegion(endpoint string, accessKeyId string, accessKeySecret string, regionID common.Region) *Client {
	client := &Client{}
	client.NewInit(endpoint, VPCAPIVersion, accessKeyId, accessKeySecret, VPCServiceCode, regionID)
	return client
}
