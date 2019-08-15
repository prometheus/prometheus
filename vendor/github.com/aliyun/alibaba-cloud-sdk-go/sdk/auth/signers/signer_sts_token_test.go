package signers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
)

func TestSignerSTSToken(t *testing.T) {
	c := credentials.NewStsTokenCredential("accessKeyId", "accessKeySecret", "token")
	assert.NotNil(t, c)
	s := NewStsTokenSigner(c)
	assert.NotNil(t, s)
	assert.Equal(t, "HMAC-SHA1", s.GetName())
	assert.Equal(t, "", s.GetType())
	assert.Equal(t, "1.0", s.GetVersion())
	accessKeyId, err := s.GetAccessKeyId()
	assert.Nil(t, err)
	assert.Equal(t, "accessKeyId", accessKeyId)
	params := s.GetExtraParam()
	assert.Len(t, params, 1)
	assert.Equal(t, "token", params["SecurityToken"])
	assert.Equal(t, "Dqy7QZhP4TyQUDa3SBSFXopJaIo=", s.Sign("string to sign", "suffix"))
}
