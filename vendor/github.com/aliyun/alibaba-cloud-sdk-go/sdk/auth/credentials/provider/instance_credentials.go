package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
)

var securityCredURL = "http://100.100.100.200/latest/meta-data/ram/security-credentials/"

type InstanceCredentialsProvider struct{}

var ProviderInstance = new(InstanceCredentialsProvider)

var HookGet = func(fn func(string) (int, []byte, error)) func(string) (int, []byte, error) {
	return fn
}

func NewInstanceCredentialsProvider() Provider {
	return &InstanceCredentialsProvider{}
}

func (p *InstanceCredentialsProvider) Resolve() (auth.Credential, error) {
	roleName, ok := os.LookupEnv(ENVEcsMetadata)
	if !ok {
		return nil, nil
	}
	if roleName == "" {
		return nil, errors.New("Environmental variable 'ALIBABA_CLOUD_ECS_METADATA' are empty")
	}
	status, content, err := HookGet(get)(securityCredURL + roleName)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		if status == 404 {
			return nil, fmt.Errorf("The role was not found in the instance")
		}
		return nil, fmt.Errorf("Received %d when getting security credentials for %s", status, roleName)
	}
	body := make(map[string]interface{})

	if err := json.Unmarshal(content, &body); err != nil {
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

func get(url string) (status int, content []byte, err error) {
	httpClient := http.DefaultClient
	httpClient.Timeout = time.Second * 1
	resp, err := httpClient.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	content, err = ioutil.ReadAll(resp.Body)
	return resp.StatusCode, content, err
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
