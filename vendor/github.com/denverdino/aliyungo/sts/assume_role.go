package sts

import "github.com/denverdino/aliyungo/common"

type AssumeRoleRequest struct {
	RoleArn         string
	RoleSessionName string
	DurationSeconds int
	Policy          string
}

type AssumedRoleUser struct {
	AssumedRoleId string
	Arn           string
}

type AssumedRoleUserCredentials struct {
	AccessKeySecret string
	AccessKeyId     string
	Expiration      string
	SecurityToken   string
}

type AssumeRoleResponse struct {
	common.Response
	AssumedRoleUser AssumedRoleUser
	Credentials     AssumedRoleUserCredentials
}

func (c *STSClient) AssumeRole(r AssumeRoleRequest) (AssumeRoleResponse, error) {
	resp := AssumeRoleResponse{}
	e := c.Invoke("AssumeRole", r, &resp)
	if e != nil {
		return AssumeRoleResponse{}, e
	}
	return resp, nil
}
