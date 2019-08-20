package dns

import (
	"os"

	"github.com/denverdino/aliyungo/common"
)

type Client struct {
	common.Client
}

const (
	// DNSDefaultEndpoint is the default API endpoint of DNS services
	DNSDefaultEndpoint = "http://dns.aliyuncs.com"
	DNSAPIVersion      = "2015-01-09"

	DNSDefaultEndpointNew = "http://alidns.aliyuncs.com"
)

// NewClient creates a new instance of DNS client
func NewClient(accessKeyId, accessKeySecret string) *Client {
	endpoint := os.Getenv("DNS_ENDPOINT")
	if endpoint == "" {
		endpoint = DNSDefaultEndpoint
	}
	return NewClientWithEndpoint(endpoint, accessKeyId, accessKeySecret)
}

// NewClientNew creates a new instance of DNS client, with http://alidns.aliyuncs.com as default endpoint
func NewClientNew(accessKeyId, accessKeySecret string) *Client {
	endpoint := os.Getenv("DNS_ENDPOINT")
	if endpoint == "" {
		endpoint = DNSDefaultEndpointNew
	}
	return NewClientWithEndpoint(endpoint, accessKeyId, accessKeySecret)
}

// NewCustomClient creates a new instance of ECS client with customized API endpoint
func NewCustomClient(accessKeyId, accessKeySecret string, endpoint string) *Client {
	client := &Client{}
	client.Init(endpoint, DNSAPIVersion, accessKeyId, accessKeySecret)
	return client
}

func NewClientWithEndpoint(endpoint string, accessKeyId, accessKeySecret string) *Client {
	client := &Client{}
	client.Init(endpoint, DNSAPIVersion, accessKeyId, accessKeySecret)
	return client
}
