package providers

import (
	"os"
	"strconv"
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
)

func TestEnvProvider_Retrieve_NewRamRoleArnCredential(t *testing.T) {

	expectedAccesKeyId := "access-key-id"
	expectedAccessKeySecret := "access-key-secret"
	expectedRoleArn := "role-arn"
	expectedRoleSessionName := "role-session-name"
	expectedRoleSessionExpiration := 1

	if err := os.Setenv(EnvVarAccessKeyID, expectedAccesKeyId); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarAccessKeyID)

	if err := os.Setenv(EnvVarAccessKeySecret, expectedAccessKeySecret); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarAccessKeySecret)

	if err := os.Setenv(EnvVarRoleArn, expectedRoleArn); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarRoleArn)

	if err := os.Setenv(EnvVarRoleSessionName, expectedRoleSessionName); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarRoleSessionName)

	if err := os.Setenv(EnvVarRoleSessionExpiration, strconv.Itoa(expectedRoleSessionExpiration)); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarRoleSessionExpiration)

	credential, err := NewEnvCredentialProvider().Retrieve()
	if err != nil {
		t.Fatal(err)
	}
	ramRoleArnCredential, ok := credential.(*credentials.RamRoleArnCredential)
	if !ok {
		t.Fatal("expected AccessKeyCredential")
	}

	if ramRoleArnCredential.AccessKeyId != expectedAccesKeyId {
		t.Fatalf("expected AccessKeyId %s but received %s", expectedAccesKeyId, ramRoleArnCredential.AccessKeyId)
	}
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
		t.Fatalf("expected RoleSessionExpiration %d but received %d", expectedRoleSessionExpiration, ramRoleArnCredential.RoleSessionExpiration)
	}
}

func TestEnvProvider_Retrieve_NewStsTokenCredential(t *testing.T) {

	expectedAccesKeyId := "access-key-id"
	expectedAccessKeySecret := "access-key-secret"
	expectedAccessKeyStsToken := "access-key-sts-token"

	if err := os.Setenv(EnvVarAccessKeyID, expectedAccesKeyId); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarAccessKeyID)

	if err := os.Setenv(EnvVarAccessKeySecret, expectedAccessKeySecret); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarAccessKeySecret)

	if err := os.Setenv(EnvVarAccessKeyStsToken, expectedAccessKeyStsToken); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarAccessKeyStsToken)

	credential, err := NewEnvCredentialProvider().Retrieve()
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

func TestEnvProvider_Retrieve_NewAccessKeyCredential(t *testing.T) {

	expectedAccesKeyId := "access-key-id"
	expectedAccessKeySecret := "access-key-secret"

	if err := os.Setenv(EnvVarAccessKeyID, expectedAccesKeyId); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarAccessKeyID)

	if err := os.Setenv(EnvVarAccessKeySecret, expectedAccessKeySecret); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarAccessKeySecret)

	credential, err := NewEnvCredentialProvider().Retrieve()
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

func TestEnvProvider_Retrieve_NewEcsRamRoleCredential(t *testing.T) {

	expectedRoleName := "role-name"

	if err := os.Setenv(EnvVarRoleName, expectedRoleName); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarRoleName)

	credential, err := NewEnvCredentialProvider().Retrieve()
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

func TestEnvProvider_Retrieve_NewRsaKeyPairCredential(t *testing.T) {

	expectedPrivateKey := "private-key"
	expectedPublicKeyId := "public-key-id"
	expectedSessionExpiration := 1

	if err := os.Setenv(EnvVarPrivateKey, expectedPrivateKey); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarPrivateKey)

	if err := os.Setenv(EnvVarPublicKeyID, expectedPublicKeyId); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarPublicKeyID)

	if err := os.Setenv(EnvVarSessionExpiration, strconv.Itoa(expectedSessionExpiration)); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(EnvVarSessionExpiration)

	credential, err := NewEnvCredentialProvider().Retrieve()
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

func TestEnvProvider_Retrieve_ErrNoValidCredentialsFound(t *testing.T) {
	_, err := NewEnvCredentialProvider().Retrieve()
	if err == nil {
		t.Fatal("expected ErrNoValidCredentialsFound for empty configuration")
	}
	if err != ErrNoValidCredentialsFound {
		t.Fatal("expected ErrNoValidCredentialsFound for empty configuration")
	}
}
