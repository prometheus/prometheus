package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAccessKeyCredential(t *testing.T) {
	c := NewAccessKeyCredential("accesskeyid", "accesskeysecret")
	assert.Equal(t, "accesskeyid", c.AccessKeyId)
	assert.Equal(t, "accesskeysecret", c.AccessKeySecret)
	b:= NewBaseCredential("accesskeyid", "accesskeysecret")
	assert.Equal(t, "accesskeyid", b.AccessKeyId)
	assert.Equal(t, "accesskeysecret", b.AccessKeySecret)
	a := b.ToAccessKeyCredential()
	assert.Equal(t, "accesskeyid", a.AccessKeyId)
	assert.Equal(t, "accesskeysecret", a.AccessKeySecret)
}
