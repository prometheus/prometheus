package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRSAKeyPairCredential(t *testing.T) {
	c := NewRsaKeyPairCredential("privateKey", "publicKey", 3600)
	assert.Equal(t, "privateKey", c.PrivateKey)
	assert.Equal(t, "publicKey", c.PublicKeyId)
	assert.Equal(t, 3600, c.SessionExpiration)
}
