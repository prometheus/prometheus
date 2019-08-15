package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSTSCredential(t *testing.T) {
	c := NewStsTokenCredential("accessKeyId", "accessKeySecret", "token")
	assert.Equal(t, "accessKeyId", c.AccessKeyId)
	assert.Equal(t, "accessKeySecret", c.AccessKeySecret)
	assert.Equal(t, "token", c.AccessKeyStsToken)
}
