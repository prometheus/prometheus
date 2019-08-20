package cs

import (
	"net/http"
)

const (
	CRDefaultEndpoint = "https://cr.cn-hangzhou.aliyuncs.com"
	CRAPIVersion      = "2016-06-07"
)

func NewCRClientWithSecurityToken(region, accessKeyId, accessKeySecret, securityToken string) *Client {
	return &Client{
		AccessKeyId:     accessKeyId,
		AccessKeySecret: accessKeySecret,
		SecurityToken:   securityToken,
		endpoint:        "https://cr." + region + ".aliyuncs.com",
		Version:         CRAPIVersion,
		httpClient:      &http.Client{},
	}
}

type CRAuthorizationToken struct {
	Data struct {
		AuthorizationToken string `json:"authorizationToken"`
		TempUserName       string `json:"tempUserName"`
		ExpireDate         int64  `json:"expireDate"`
	} `json:"data"`
	RequestId string `json:"requestId"`
}

func (client *Client) GetCRAuthorizationToken() (crtoken CRAuthorizationToken, err error) {
	err = client.Invoke("", http.MethodGet, "/tokens", nil, nil, &crtoken)
	return
}

func (client *Client) GetCRRepoInfo(repoNamespace, repoName string) (str string, err error) {
	err = client.Invoke("", http.MethodGet, "/repos/"+repoNamespace+"/"+repoName, nil, nil, &str)
	return
}
