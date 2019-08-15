package signers_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/signers"
)

func TestBearerTokenSigner(t *testing.T) {
	c := credentials.NewBearerTokenCredential("Bearer.Token")
	sign := signers.NewBearerTokenSigner(c)
	assert.NotNil(t, sign)
	exparam := sign.GetExtraParam()
	assert.True(t, reflect.DeepEqual(exparam, map[string]string{"BearerToken": "Bearer.Token"}))

	assert.Empty(t, sign.GetName())

	assert.Equal(t, "BEARERTOKEN", sign.GetType())

	assert.Equal(t, "1.0", sign.GetVersion())

	accessKeyID, err := sign.GetAccessKeyId()
	assert.Empty(t, accessKeyID)
	assert.Nil(t, err)

	assert.Empty(t, sign.Sign("stringToSign", "&"))
}
