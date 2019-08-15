package auth

import (
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/signers"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/stretchr/testify/assert"
)

func TestSigner_AccessKeySigner(t *testing.T) {
	c := credentials.NewAccessKeyCredential("accessKeyId", "accessKeySecret")
	signer, err := NewSignerWithCredential(c, nil)
	assert.Nil(t, err)
	_, ok := signer.(*signers.AccessKeySigner)
	assert.True(t, ok)
}

func TestSigner_BaseSigner(t *testing.T) {
	c := credentials.NewBaseCredential("accessKeyId", "accessKeySecret")
	signer, err := NewSignerWithCredential(c, nil)
	assert.Nil(t, err)
	_, ok := signer.(*signers.AccessKeySigner)
	assert.True(t, ok)
}

func TestSigner_StsRoleArnSigner(t *testing.T) {
	c := credentials.NewStsRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", 3600)
	signer, err := NewSignerWithCredential(c, nil)
	assert.Nil(t, err)
	_, ok := signer.(*signers.RamRoleArnSigner)
	assert.True(t, ok)
}

func TestSigner_StsRoleNameOnEcsSigner(t *testing.T) {
	c := credentials.NewStsRoleNameOnEcsCredential("roleName")
	signer, err := NewSignerWithCredential(c, nil)
	assert.Nil(t, err)
	_, ok := signer.(*signers.EcsRamRoleSigner)
	assert.True(t, ok)
}

func TestSigner_StsTokenSigner(t *testing.T) {
	c := credentials.NewStsTokenCredential("accessKeyId", "accessKeySecret", "token")
	signer, err := NewSignerWithCredential(c, nil)
	assert.Nil(t, err)
	_, ok := signer.(*signers.StsTokenSigner)
	assert.True(t, ok)
}

func TestSigner_RamRoleArnSigner(t *testing.T) {
	c := credentials.NewRamRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", 3600)
	signer, err := NewSignerWithCredential(c, nil)
	assert.Nil(t, err)
	_, ok := signer.(*signers.RamRoleArnSigner)
	assert.True(t, ok)
}

func TestSigner_NewSignerKeyPair(t *testing.T) {
	c := credentials.NewRsaKeyPairCredential("publicKeyId", "privateKeyId", 3600)
	signer, err := NewSignerWithCredential(c, nil)
	assert.Nil(t, err)
	_, ok := signer.(*signers.SignerKeyPair)
	assert.True(t, ok)
}

func TestSigner_EcsRamRoleSigner(t *testing.T) {
	c := credentials.NewEcsRamRoleCredential("roleName")
	signer, err := NewSignerWithCredential(c, nil)
	assert.Nil(t, err)
	_, ok := signer.(*signers.EcsRamRoleSigner)
	assert.True(t, ok)
}

func TestSigner_BearerTokenSigner(t *testing.T) {
	c := credentials.NewBearerTokenCredential("Bearer.Token")
	signer, err := NewSignerWithCredential(c, nil)
	assert.Nil(t, err)
	_, ok := signer.(*signers.BearerTokenSigner)
	assert.True(t, ok)
}

type OtherCredential struct {
}

func TestSigner_OtherSigner(t *testing.T) {
	c := &OtherCredential{}
	_, err := NewSignerWithCredential(c, nil)
	assert.NotNil(t, err)
	assert.Equal(t, "[SDK.UnsupportedCredential] Specified credential (type = *auth.OtherCredential) is not supported, please check", err.Error())
}

func Test_Sign_ROA(t *testing.T) {
	request := requests.NewCommonRequest()
	request.PathPattern = "/users/:user"
	request.TransToAcsRequest()
	c := credentials.NewAccessKeyCredential("accessKeyId", "accessKeySecret")
	signer := signers.NewAccessKeySigner(c)

	err := Sign(request, signer, "regionId")
	assert.Nil(t, err)
}

func Test_Sign_RPC(t *testing.T) {
	request := requests.NewCommonRequest()
	request.TransToAcsRequest()
	c := credentials.NewAccessKeyCredential("accessKeyId", "accessKeySecret")
	signer := signers.NewAccessKeySigner(c)

	err := Sign(request, signer, "regionId")
	assert.Nil(t, err)
}
