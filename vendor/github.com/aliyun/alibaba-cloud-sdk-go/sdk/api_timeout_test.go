package sdk

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_getAPIMaxTimeout(t *testing.T) {
	timeout, ok := getAPIMaxTimeout("ecs", "UnassociateEipAddress")
	assert.True(t, ok)
	assert.Equal(t, 16*time.Second, timeout)

	timeout, ok = getAPIMaxTimeout("Ecs", "UnassociateEipAddress")
	assert.True(t, ok)
	assert.Equal(t, 16*time.Second, timeout)

	timeout, ok = getAPIMaxTimeout("Acs", "UnassociateEipAddress")
	assert.False(t, ok)
	assert.Equal(t, 0*time.Second, timeout)
}
