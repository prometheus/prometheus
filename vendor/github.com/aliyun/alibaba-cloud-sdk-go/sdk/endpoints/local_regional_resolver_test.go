package endpoints

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalRegionalResolver_GetName(t *testing.T) {
	resolver := &LocalRegionalResolver{}
	assert.Equal(t, "local regional resolver", resolver.GetName())
}

func TestLocalRegionalResolver_TryResolve(t *testing.T) {
	resolver := &LocalRegionalResolver{}
	resolveParam := &ResolveParam{
		Product:  "push",
		RegionId: "cn-beijing",
	}
	endpoint, support, err := resolver.TryResolve(resolveParam)
	assert.Nil(t, err)
	assert.Equal(t, "", endpoint)
	assert.Equal(t, false, support)

	resolveParam = &ResolveParam{
		Product:  "arms",
		RegionId: "cn-beijing",
	}
	endpoint, support, err = resolver.TryResolve(resolveParam)
	assert.Nil(t, err)
	assert.Equal(t, "arms.cn-beijing.aliyuncs.com", endpoint)
	assert.Equal(t, true, support)
}
