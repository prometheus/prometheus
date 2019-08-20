package mns

import (
	"net/http"
)

const (
	MNSAPIVersion = "2015-06-06"
)

type Client struct {
	AccessKeyId     string
	AccessKeySecret string
	SecurityToken   string
	Endpoint        string
	Version         string
	httpClient      *http.Client
	debug           bool
}

func (client *Client) SetDebug(debug bool) {
	client.debug = debug
}

func NewClient(accessKeyId, accessKeySecret, endpoint string) (client *Client) {
	client = &Client{
		AccessKeyId:     accessKeyId,
		AccessKeySecret: accessKeySecret,
		Endpoint:        endpoint,
		Version:         MNSAPIVersion,
		httpClient:      &http.Client{},
	}
	return client
}

func NewClientForAssumeRole(accessKeyId, accessKeySecret, securityToken, endpoint string) (client *Client) {
	client = &Client{
		AccessKeyId:     accessKeyId,
		AccessKeySecret: accessKeySecret,
		SecurityToken:   securityToken,
		Endpoint:        endpoint,
		Version:         MNSAPIVersion,
		httpClient:      &http.Client{},
	}
	return client
}
