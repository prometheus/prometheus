package cms

import (
	"net/http"

	"os"

	"github.com/denverdino/aliyungo/common"
)

type Client struct {
	endpoint        string
	accessKeyId     string //Access Key Id
	accessKeySecret string //Access Key Secret

	debug      bool
	httpClient *http.Client
	version    string
	internal   bool
	//region     common.Region
	securityToken string
}

type CMSClient struct {
	common.Client
}

func (client *Client) SetSecurityToken(securityToken string) {
	client.securityToken = securityToken
}

func (client *Client) SetDebug(debug bool) {
	client.debug = debug
}

const (
	//TODO 旧的API，暂时保留
	DefaultEndpoint = "http://alert.aliyuncs.com"
	APIVersion      = "2015-08-15"
	METHOD_GET      = "GET"
	METHOD_POST     = "POST"
	METHOD_PUT      = "PUT"
	METHOD_DELETE   = "DELETE"

	CMSDefaultEndpoint = "http://metrics.cn-hangzhou.aliyuncs.com"
	CMSAPIVersion      = "2017-03-01"
	CMSServiceCode     = "cms"
)

//TODO 旧的API
// NewClient creates a new instance of ECS client
func NewClient(accessKeyId, accessKeySecret string) *Client {
	return &Client{
		accessKeyId:     accessKeyId,
		accessKeySecret: accessKeySecret,
		internal:        false,
		//region:          region,
		version:    APIVersion,
		endpoint:   DefaultEndpoint,
		httpClient: &http.Client{},
	}
}

func (client *Client) GetApiUri() string {
	return client.endpoint
}

func (client *Client) GetAccessKey() string {
	return client.accessKeyId
}

func (client *Client) GetAccessSecret() string {
	return client.accessKeySecret
}

// NewClient creates a new instance of CMS client
func NewCMSClient(accessKeyId, accessKeySecret string) *CMSClient {
	endpoint := os.Getenv("CMS_ENDPOINT")
	if endpoint == "" {
		endpoint = CMSDefaultEndpoint
	}
	return NewClientWithEndpoint(endpoint, accessKeyId, accessKeySecret)
}

func NewClientWithEndpoint(endpoint string, accessKeyId, accessKeySecret string) *CMSClient {
	client := &CMSClient{}
	client.Init(endpoint, CMSAPIVersion, accessKeyId, accessKeySecret)
	return client
}

func NewCMSRegionClient(accessKeyId, accessKeySecret string, regionID common.Region) *CMSClient {
	endpoint := os.Getenv("CMS_ENDPOINT")
	if endpoint == "" {
		endpoint = CMSDefaultEndpoint
	}

	return NewClientWithRegion(endpoint, accessKeyId, accessKeySecret, regionID)
}

func NewClientWithRegion(endpoint string, accessKeyId, accessKeySecret string, regionID common.Region) *CMSClient {
	client := &CMSClient{}
	client.NewInit(endpoint, CMSAPIVersion, accessKeyId, accessKeySecret, CMSServiceCode, regionID)
	return client
}
