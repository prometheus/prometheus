package endpoints

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMappingResolver_GetName(t *testing.T) {
	resolver := &MappingResolver{}
	assert.Equal(t, "mapping resolver", resolver.GetName())
}

func TestMappingResolver_TryResolve(t *testing.T) {
	resolver := &MappingResolver{}
	resolveParam := &ResolveParam{
		RegionId: "cn-hangzhou",
		Product:  "ecs",
	}
	endpoint, support, err := resolver.TryResolve(resolveParam)
	assert.Nil(t, err)
	assert.Equal(t, "", endpoint)
	assert.Equal(t, false, support)

	AddEndpointMapping("cn-hangzhou", "Ecs", "unreachable.aliyuncs.com")

	endpoint, support, err = resolver.TryResolve(resolveParam)
	assert.Nil(t, err)
	assert.Equal(t, "unreachable.aliyuncs.com", endpoint)
	assert.Equal(t, true, support)
}
