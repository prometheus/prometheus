package provider_test

import (
	"os"
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"

	"github.com/stretchr/testify/assert"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials/provider"
)

func TestEnvResolve(t *testing.T) {
	p := provider.NewEnvProvider()
	assert.Equal(t, &provider.EnvProvider{}, p)
	c, err := p.Resolve()
	assert.Nil(t, c)
	assert.Nil(t, err)
	os.Setenv(provider.ENVAccessKeyID, "")
	os.Setenv(provider.ENVAccessKeySecret, "")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.EqualError(t, err, "Environmental variable (ALIBABACLOUD_ACCESS_KEY_ID or ALIBABACLOUD_ACCESS_KEY_SECRET) is empty")
	os.Setenv(provider.ENVAccessKeyID, "AccessKeyId")
	os.Setenv(provider.ENVAccessKeySecret, "AccessKeySecret")
	c, err = p.Resolve()
	assert.Nil(t, err)
	assert.Equal(t, &credentials.AccessKeyCredential{"AccessKeyId", "AccessKeySecret"}, c)
}
