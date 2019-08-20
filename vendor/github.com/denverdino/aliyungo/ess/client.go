package ess

import (
	"github.com/denverdino/aliyungo/common"

	"os"
)

type Client struct {
	common.Client
}

const (
	// ESSDefaultEndpoint is the default API endpoint of ESS services
	ESSDefaultEndpoint = "https://ess.aliyuncs.com"
	ESSAPIVersion      = "2014-08-28"
	ESSServiceCode     = "ess"
)

// NewClient creates a new instance of RDS client
func NewClient(accessKeyId, accessKeySecret string) *Client {
	endpoint := os.Getenv("ESS_ENDPOINT")
	if endpoint == "" {
		endpoint = ESSDefaultEndpoint
	}
	return NewClientWithEndpoint(endpoint, accessKeyId, accessKeySecret)
}

func NewClientWithEndpoint(endpoint string, accessKeyId, accessKeySecret string) *Client {
	client := &Client{}
	client.Init(endpoint, ESSAPIVersion, accessKeyId, accessKeySecret)
	return client
}

func NewESSClient(accessKeyId, accessKeySecret string, regionID common.Region) *Client {
	endpoint := os.Getenv("ESS_ENDPOINT")
	if endpoint == "" {
		endpoint = ESSDefaultEndpoint
	}

	return NewClientWithRegion(endpoint, accessKeyId, accessKeySecret, regionID)
}

func NewClientWithRegion(endpoint string, accessKeyId, accessKeySecret string, regionID common.Region) *Client {
	client := &Client{}
	client.NewInit(endpoint, ESSAPIVersion, accessKeyId, accessKeySecret, ESSServiceCode, regionID)
	return client
}
