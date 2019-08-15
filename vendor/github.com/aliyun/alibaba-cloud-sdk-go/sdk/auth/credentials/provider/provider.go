package provider

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth"
)

//Environmental virables that may be used by the provider
const (
	ENVAccessKeyID     = "ALIBABA_CLOUD_ACCESS_KEY_ID"
	ENVAccessKeySecret = "ALIBABA_CLOUD_ACCESS_KEY_SECRET"
	ENVCredentialFile  = "ALIBABA_CLOUD_CREDENTIALS_FILE"
	ENVEcsMetadata     = "ALIBABA_CLOUD_ECS_METADATA"
	PATHCredentialFile = "~/.alibabacloud/credentials"
)

// When you want to customize the provider, you only need to implement the method of the interface.
type Provider interface {
	Resolve() (auth.Credential, error)
}
