package provider_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials/provider"
)

func TestProviderChain(t *testing.T) {
	env := provider.NewEnvProvider()
	pp := provider.NewProfileProvider()
	instanceP := provider.NewInstanceCredentialsProvider()

	pc := provider.NewProviderChain([]provider.Provider{env, pp, instanceP})

	c, err := pc.Resolve()
	assert.Equal(t, &credentials.AccessKeyCredential{AccessKeyId: "AccessKeyId", AccessKeySecret: "AccessKeySecret"}, c)
	assert.Nil(t, err)

	os.Setenv(provider.ENVAccessKeyID, "")
	os.Setenv(provider.ENVAccessKeySecret, "")
	c, err = pc.Resolve()
	assert.Nil(t, c)
	assert.EqualError(t, err, "Environmental variable (ALIBABACLOUD_ACCESS_KEY_ID or ALIBABACLOUD_ACCESS_KEY_SECRET) is empty")

	os.Unsetenv(provider.ENVAccessKeyID)
	os.Unsetenv(provider.ENVCredentialFile)
	os.Unsetenv(provider.ENVEcsMetadata)

	c, err = pc.Resolve()
	assert.Nil(t, c)
	assert.EqualError(t, err, "No credential found")
}
