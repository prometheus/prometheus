package provider

import (
	"errors"
	"os"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth"
)

type EnvProvider struct{}

var ProviderEnv = new(EnvProvider)

func NewEnvProvider() Provider {
	return &EnvProvider{}
}

func (p *EnvProvider) Resolve() (auth.Credential, error) {
	accessKeyID, ok1 := os.LookupEnv(ENVAccessKeyID)
	accessKeySecret, ok2 := os.LookupEnv(ENVAccessKeySecret)
	if !ok1 || !ok2 {
		return nil, nil
	}
	if accessKeyID == "" || accessKeySecret == "" {
		return nil, errors.New("Environmental variable (ALIBABACLOUD_ACCESS_KEY_ID or ALIBABACLOUD_ACCESS_KEY_SECRET) is empty")
	}
	return credentials.NewAccessKeyCredential(accessKeyID, accessKeySecret), nil
}
