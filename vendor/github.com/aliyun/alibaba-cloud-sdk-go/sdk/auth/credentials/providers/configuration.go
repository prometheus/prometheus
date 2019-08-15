package providers

import (
	"errors"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
)

var (
	ErrNoValidCredentialsFound = errors.New("no valid credentials were found")
)

type Configuration struct {
	AccessKeyID           string
	AccessKeySecret       string
	AccessKeyStsToken     string
	RoleArn               string
	RoleSessionName       string
	RoleSessionExpiration *int
	PrivateKey            string
	PublicKeyID           string
	SessionExpiration     *int
	RoleName              string
}

func NewConfigurationCredentialProvider(configuration *Configuration) Provider {
	return &ConfigurationProvider{
		Configuration: configuration,
	}
}

type ConfigurationProvider struct {
	Configuration *Configuration
}

// Retrieve walks through all currently supported credential types and attempts to build them
// using the given configuration.
func (p *ConfigurationProvider) Retrieve() (auth.Credential, error) {

	if p.Configuration.AccessKeyID != "" && p.Configuration.AccessKeySecret != "" {

		if p.Configuration.RoleArn != "" && p.Configuration.RoleSessionName != "" && p.Configuration.RoleSessionExpiration != nil {
			return credentials.NewRamRoleArnCredential(p.Configuration.AccessKeyID, p.Configuration.AccessKeySecret, p.Configuration.RoleArn, p.Configuration.RoleSessionName, *p.Configuration.RoleSessionExpiration), nil
		}

		if p.Configuration.AccessKeyStsToken != "" {
			return credentials.NewStsTokenCredential(p.Configuration.AccessKeyID, p.Configuration.AccessKeySecret, p.Configuration.AccessKeyStsToken), nil
		}

		return credentials.NewAccessKeyCredential(p.Configuration.AccessKeyID, p.Configuration.AccessKeySecret), nil
	}

	if p.Configuration.RoleName != "" {
		return credentials.NewEcsRamRoleCredential(p.Configuration.RoleName), nil
	}

	if p.Configuration.PrivateKey != "" && p.Configuration.PublicKeyID != "" && p.Configuration.SessionExpiration != nil {
		return credentials.NewRsaKeyPairCredential(p.Configuration.PrivateKey, p.Configuration.PublicKeyID, *p.Configuration.SessionExpiration), nil
	}
	return nil, ErrNoValidCredentialsFound
}
