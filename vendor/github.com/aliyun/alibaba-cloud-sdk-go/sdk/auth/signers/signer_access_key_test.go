package signers

import (
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/stretchr/testify/assert"
)

func TestSignerAccessKey(t *testing.T) {
	c := credentials.NewAccessKeyCredential("accessKeyId", "accessKeySecret")
	assert.NotNil(t, c)
	s := NewAccessKeySigner(c)
	assert.Nil(t, s.GetExtraParam())
	assert.Equal(t, "HMAC-SHA1", s.GetName())
	assert.Equal(t, "", s.GetType())
	assert.Equal(t, "1.0", s.GetVersion())
	accessKeyId, err := s.GetAccessKeyId()
	assert.Nil(t, err)
	assert.Equal(t, "accessKeyId", accessKeyId)
	assert.Equal(t, "Dqy7QZhP4TyQUDa3SBSFXopJaIo=", s.Sign("string to sign", "suffix"))
}
