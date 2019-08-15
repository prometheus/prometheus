package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
)

var securityCredURL = "http://100.100.100.200/latest/meta-data/ram/security-credentials/"

func NewInstanceMetadataProvider() Provider {
	return &InstanceMetadataProvider{}
}

type InstanceMetadataProvider struct {
	RoleName string
}

func get(url string) (status int, content string, err error) {
	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	return resp.StatusCode, string(bodyBytes), err
}

func (p *InstanceMetadataProvider) GetRoleName() (roleName string, err error) {
	// Instances can have only one role name that never changes,
	// so attempt to populate it.
	// If this call is executed in an environment that doesn't support instance metadata,
	// it will time out after 30 seconds and return an err.
	status, roleName, err := get(securityCredURL)

	if err != nil {
		return
	}

	if status != 200 {
		err = fmt.Errorf("received %d getting role name: %s", status, roleName)
		return
	}

	if roleName == "" {
		err = errors.New("unable to retrieve role name, it may be unset")
	}

	return
}

func (p *InstanceMetadataProvider) Retrieve() (auth.Credential, error) {
	if p.RoleName == "" {
		roleName, err := p.GetRoleName()
		if err != nil {
			return nil, err
		}
		p.RoleName = roleName
	}

	status, content, err := get(securityCredURL + p.RoleName)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("received %d getting security credentials for %s", status, p.RoleName)
	}

	body := make(map[string]interface{})

	if err := json.Unmarshal([]byte(content), &body); err != nil {
		return nil, err
	}

	accessKeyID, err := extractString(body, "AccessKeyId")
	if err != nil {
		return nil, err
	}
	accessKeySecret, err := extractString(body, "AccessKeySecret")
	if err != nil {
		return nil, err
	}
	securityToken, err := extractString(body, "SecurityToken")
	if err != nil {
		return nil, err
	}
	return credentials.NewStsTokenCredential(accessKeyID, accessKeySecret, securityToken), nil
}

func extractString(m map[string]interface{}, key string) (string, error) {
	raw, ok := m[key]
	if !ok {
		return "", fmt.Errorf("%s not in map", key)
	}
	str, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s is not a string in map", key)
	}
	return str, nil
}
