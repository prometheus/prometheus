package endpoints

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalGlobalResolver_GetName(t *testing.T) {
	resolver := &LocalGlobalResolver{}
	assert.Equal(t, "local global resolver", resolver.GetName())
}

func TestLocalGlobalResolver_TryResolve(t *testing.T) {
	resolver := &LocalGlobalResolver{}
	resolveParam := &ResolveParam{
		Product: "ecs",
	}
	endpoint, support, err := resolver.TryResolve(resolveParam)
	assert.Nil(t, err)
	assert.Equal(t, "ecs-cn-hangzhou.aliyuncs.com", endpoint)
	assert.Equal(t, true, support)

	resolveParam = &ResolveParam{
		Product: "drds",
	}
	endpoint, support, err = resolver.TryResolve(resolveParam)
	assert.Nil(t, err)
	assert.Equal(t, "drds.aliyuncs.com", endpoint)
	assert.Equal(t, true, support)

	resolveParam = &ResolveParam{
		Product: "inexist",
	}
	endpoint, support, err = resolver.TryResolve(resolveParam)
	assert.Nil(t, err)
	assert.Equal(t, "", endpoint)
	assert.Equal(t, false, support)
}
