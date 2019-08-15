package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
)

func TestConfigurationProvider_Retrieve_NewRamRoleArnCredential(t *testing.T) {

	expectedAccesKeyId := "access-key-id"
	expectedAccessKeySecret := "access-key-secret"
	expectedRoleArn := "role-arn"
	expectedRoleSessionName := "role-session-name"
	expectedRoleSessionExpiration := 1

	configuration := &Configuration{
		AccessKeyID:           expectedAccesKeyId,
		AccessKeySecret:       expectedAccessKeySecret,
		RoleArn:               expectedRoleArn,
		RoleSessionName:       expectedRoleSessionName,
		RoleSessionExpiration: &expectedRoleSessionExpiration,
	}
	credential, err := NewConfigurationCredentialProvider(configuration).Retrieve()
	if err != nil {
		t.Fatal(err)
	}
	ramRoleArnCredential, ok := credential.(*credentials.RamRoleArnCredential)
	assert.True(t, ok, "expected AccessKeyCredential")
	assert.Equal(t, expectedAccesKeyId, ramRoleArnCredential.AccessKeyId, "expected AccessKeyId %s but received %s", expectedAccesKeyId, ramRoleArnCredential.AccessKeyId)

	if ramRoleArnCredential.AccessKeySecret != expectedAccessKeySecret {
		t.Fatalf("expected AccessKeySecret %s but received %s", expectedAccessKeySecret, ramRoleArnCredential.AccessKeySecret)
	}
	if ramRoleArnCredential.RoleArn != expectedRoleArn {
		t.Fatalf("expected RoleArn %s but received %s", expectedRoleArn, ramRoleArnCredential.RoleArn)
	}
	if ramRoleArnCredential.RoleSessionName != expectedRoleSessionName {
		t.Fatalf("expected RoleSessionName %s but received %s", expectedRoleSessionName, ramRoleArnCredential.RoleSessionName)
	}
	if ramRoleArnCredential.RoleSessionExpiration != expectedRoleSessionExpiration {
		t.Fatalf("expected expectedRoleSessionExpiration %d but received %d", expectedRoleSessionExpiration, ramRoleArnCredential.RoleSessionExpiration)
	}
}

func TestConfigurationProvider_Retrieve_NewStsTokenCredential(t *testing.T) {

	expectedAccesKeyId := "access-key-id"
	expectedAccessKeySecret := "access-key-secret"
	expectedAccessKeyStsToken := "access-key-sts-token"

	configuration := &Configuration{
		AccessKeyID:       expectedAccesKeyId,
		AccessKeySecret:   expectedAccessKeySecret,
		AccessKeyStsToken: expectedAccessKeyStsToken,
	}
	credential, err := NewConfigurationCredentialProvider(configuration).Retrieve()
	if err != nil {
		t.Fatal(err)
	}
	stsTokenCredential, ok := credential.(*credentials.StsTokenCredential)
	if !ok {
		t.Fatal("expected AccessKeyCredential")
	}

	if stsTokenCredential.AccessKeyId != expectedAccesKeyId {
		t.Fatalf("expected AccessKeyId %s but received %s", expectedAccesKeyId, stsTokenCredential.AccessKeyId)
	}
	if stsTokenCredential.AccessKeySecret != expectedAccessKeySecret {
		t.Fatalf("expected AccessKeySecret %s but received %s", expectedAccessKeySecret, stsTokenCredential.AccessKeySecret)
	}
	if stsTokenCredential.AccessKeyStsToken != expectedAccessKeyStsToken {
		t.Fatalf("expected AccessKeyStsToken %s but received %s", expectedAccessKeyStsToken, stsTokenCredential.AccessKeyStsToken)
	}
}

func TestConfigurationProvider_Retrieve_NewAccessKeyCredential(t *testing.T) {

	expectedAccesKeyId := "access-key-id"
	expectedAccessKeySecret := "access-key-secret"

	configuration := &Configuration{
		AccessKeyID:     expectedAccesKeyId,
		AccessKeySecret: expectedAccessKeySecret,
	}
	credential, err := NewConfigurationCredentialProvider(configuration).Retrieve()
	if err != nil {
		t.Fatal(err)
	}
	accessKeyCredential, ok := credential.(*credentials.AccessKeyCredential)
	if !ok {
		t.Fatal("expected AccessKeyCredential")
	}

	if accessKeyCredential.AccessKeyId != expectedAccesKeyId {
		t.Fatalf("expected AccessKeyId %s but received %s", expectedAccesKeyId, accessKeyCredential.AccessKeyId)
	}
	if accessKeyCredential.AccessKeySecret != expectedAccessKeySecret {
		t.Fatalf("expected AccessKeySecret %s but received %s", expectedAccessKeySecret, accessKeyCredential.AccessKeySecret)
	}
}

func TestConfigurationProvider_Retrieve_NewEcsRamRoleCredential(t *testing.T) {

	expectedRoleName := "role-name"

	configuration := &Configuration{
		RoleName: expectedRoleName,
	}
	credential, err := NewConfigurationCredentialProvider(configuration).Retrieve()
	if err != nil {
		t.Fatal(err)
	}
	ecsRamRoleCredential, ok := credential.(*credentials.EcsRamRoleCredential)
	if !ok {
		t.Fatal("expected AccessKeyCredential")
	}

	if ecsRamRoleCredential.RoleName != expectedRoleName {
		t.Fatalf("expected RoleName %s but received %s", expectedRoleName, ecsRamRoleCredential.RoleName)
	}
}

func TestConfigurationProvider_Retrieve_NewRsaKeyPairCredential(t *testing.T) {

	expectedPrivateKey := "private-key"
	expectedPublicKeyId := "public-key-id"
	expectedSessionExpiration := 1

	configuration := &Configuration{
		PrivateKey:        expectedPrivateKey,
		PublicKeyID:       expectedPublicKeyId,
		SessionExpiration: &expectedSessionExpiration,
	}
	credential, err := NewConfigurationCredentialProvider(configuration).Retrieve()
	if err != nil {
		t.Fatal(err)
	}
	rsaKeyPairCredential, ok := credential.(*credentials.RsaKeyPairCredential)
	if !ok {
		t.Fatal("expected AccessKeyCredential")
	}

	if rsaKeyPairCredential.PrivateKey != expectedPrivateKey {
		t.Fatalf("expected PrivateKey %s but received %s", expectedPrivateKey, rsaKeyPairCredential.PrivateKey)
	}
	if rsaKeyPairCredential.PublicKeyId != expectedPublicKeyId {
		t.Fatalf("expected PublicKeyId %s but received %s", expectedPublicKeyId, rsaKeyPairCredential.PublicKeyId)
	}
	if rsaKeyPairCredential.SessionExpiration != expectedSessionExpiration {
		t.Fatalf("expected SessionExpiration %d but received %d", expectedSessionExpiration, rsaKeyPairCredential.SessionExpiration)
	}
}

func TestConfigurationProvider_Retrieve_ErrNoValidCredentialsFound(t *testing.T) {
	_, err := NewConfigurationCredentialProvider(&Configuration{}).Retrieve()
	if err == nil {
		t.Fatal("expected ErrNoValidCredentialsFound for empty configuration")
	}
	if err != ErrNoValidCredentialsFound {
		t.Fatal("expected ErrNoValidCredentialsFound for empty configuration")
	}
}
