package signers

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
)

type BearerTokenSigner struct {
	credential *credentials.BearerTokenCredential
}

func NewBearerTokenSigner(credential *credentials.BearerTokenCredential) *BearerTokenSigner {
	return &BearerTokenSigner{
		credential: credential,
	}
}

func (signer *BearerTokenSigner) GetExtraParam() map[string]string {
	return map[string]string{"BearerToken": signer.credential.BearerToken}
}

func (*BearerTokenSigner) GetName() string {
	return ""
}
func (*BearerTokenSigner) GetType() string {
	return "BEARERTOKEN"
}
func (*BearerTokenSigner) GetVersion() string {
	return "1.0"
}
func (signer *BearerTokenSigner) GetAccessKeyId() (accessKeyId string, err error) {
	return "", nil
}
func (signer *BearerTokenSigner) Sign(stringToSign, secretSuffix string) string {
	return ""
}
