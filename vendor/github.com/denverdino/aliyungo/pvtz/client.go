package pvtz

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
	PVTZDefaultEndpoint = "https://pvtz.aliyuncs.com"
	PVTZServiceCode     = "pvtz"
	PVTZAPIVersion      = "2018-01-01"
)

// NewClient creates a new instance of ECS client
func NewClient(accessKeyId, accessKeySecret string) *Client {
	endpoint := os.Getenv("PVTZ_ENDPOINT")
	if endpoint == "" {
		endpoint = PVTZDefaultEndpoint
	}
	return NewClientWithEndpoint(endpoint, accessKeyId, accessKeySecret)
}

func NewClientWithRegion(endpoint string, accessKeyId string, accessKeySecret string, regionID common.Region) *Client {
	client := &Client{}
	client.NewInit(endpoint, PVTZAPIVersion, accessKeyId, accessKeySecret, PVTZServiceCode, regionID)
	return client
}

func NewClientWithEndpoint(endpoint string, accessKeyId string, accessKeySecret string) *Client {
	client := &Client{}
	client.Init(endpoint, PVTZAPIVersion, accessKeyId, accessKeySecret)
	return client
}

// ---------------------------------------
// NewPVTZClient creates a new instance of PVTZ client
// ---------------------------------------
func NewPVTZClient(accessKeyId, accessKeySecret string, regionID common.Region) *Client {
	return NewPVTZClientWithSecurityToken(accessKeyId, accessKeySecret, "", regionID)
}

func NewPVTZClientWithSecurityToken(accessKeyId string, accessKeySecret string, securityToken string, regionID common.Region) *Client {
	endpoint := os.Getenv("PVTZ_ENDPOINT")
	if endpoint == "" {
		endpoint = PVTZDefaultEndpoint
	}

	return NewPVTZClientWithEndpointAndSecurityToken(endpoint, accessKeyId, accessKeySecret, securityToken, regionID)
}

func NewPVTZClientWithEndpoint(endpoint string, accessKeyId string, accessKeySecret string, regionID common.Region) *Client {
	return NewPVTZClientWithEndpointAndSecurityToken(endpoint, accessKeyId, accessKeySecret, "", regionID)
}

func NewPVTZClientWithEndpointAndSecurityToken(endpoint string, accessKeyId string, accessKeySecret string, securityToken string, regionID common.Region) *Client {
	client := &Client{}
	client.WithEndpoint(endpoint).
		WithVersion(PVTZAPIVersion).
		WithAccessKeyId(accessKeyId).
		WithAccessKeySecret(accessKeySecret).
		WithSecurityToken(securityToken).
		WithServiceCode(PVTZServiceCode).
		WithRegionID(regionID).
		InitClient()
	return client
}
