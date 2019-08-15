package providers

import (
	"fmt"
	"os"
	"strconv"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth"
)

const (
	EnvVarAccessKeyID           = "ALICLOUD_ACCESS_KEY"
	EnvVarAccessKeySecret       = "ALICLOUD_SECRET_KEY"
	EnvVarAccessKeyStsToken     = "ALICLOUD_ACCESS_KEY_STS_TOKEN"
	EnvVarRoleArn               = "ALICLOUD_ROLE_ARN"
	EnvVarRoleSessionName       = "ALICLOUD_ROLE_SESSION_NAME"
	EnvVarRoleSessionExpiration = "ALICLOUD_ROLE_SESSION_EXPIRATION"
	EnvVarPrivateKey            = "ALICLOUD_PRIVATE_KEY"
	EnvVarPublicKeyID           = "ALICLOUD_PUBLIC_KEY_ID"
	EnvVarSessionExpiration     = "ALICLOUD_SESSION_EXPIRATION"
	EnvVarRoleName              = "ALICLOUD_ROLE_NAME"
)

func NewEnvCredentialProvider() Provider {
	return &EnvProvider{}
}

type EnvProvider struct{}

func (p *EnvProvider) Retrieve() (auth.Credential, error) {
	roleSessionExpiration, err := envVarToInt(EnvVarRoleSessionExpiration)
	if err != nil {
		return nil, err
	}
	sessionExpiration, err := envVarToInt(EnvVarSessionExpiration)
	if err != nil {
		return nil, err
	}
	c := &Configuration{
		AccessKeyID:           os.Getenv(EnvVarAccessKeyID),
		AccessKeySecret:       os.Getenv(EnvVarAccessKeySecret),
		AccessKeyStsToken:     os.Getenv(EnvVarAccessKeyStsToken),
		RoleArn:               os.Getenv(EnvVarRoleArn),
		RoleSessionName:       os.Getenv(EnvVarRoleSessionName),
		RoleSessionExpiration: &roleSessionExpiration,
		PrivateKey:            os.Getenv(EnvVarPrivateKey),
		PublicKeyID:           os.Getenv(EnvVarPublicKeyID),
		SessionExpiration:     &sessionExpiration,
		RoleName:              os.Getenv(EnvVarRoleName),
	}
	return NewConfigurationCredentialProvider(c).Retrieve()
}

func envVarToInt(envVar string) (int, error) {
	asInt := 0
	asStr := os.Getenv(envVar)
	if asStr != "" {
		if i, err := strconv.Atoi(asStr); err != nil {
			return 0, fmt.Errorf("error parsing %s: %s", envVar, err)
		} else {
			asInt = i
		}
	}
	return asInt, nil
}
