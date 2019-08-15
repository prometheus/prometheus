package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBearerTokenCredential(t *testing.T) {
	bc := NewBearerTokenCredential("Bearer.Token")
	assert.Equal(t, &BearerTokenCredential{"Bearer.Token"}, bc)
}
