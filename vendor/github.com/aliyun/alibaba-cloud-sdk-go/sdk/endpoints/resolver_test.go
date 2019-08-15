package endpoints

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolver_ResolveParam_String(t *testing.T) {
	param := &ResolveParam{}
	assert.Equal(t, "{\"Domain\":\"\",\"Product\":\"\",\"RegionId\":\"\",\"LocationProduct\":\"\",\"LocationEndpointType\":\"\"}", param.String())
}

func TestResolve(t *testing.T) {
	param := &ResolveParam{
		Product: "Ecs",
		RegionId: "cn-hangzhou",
	}
	AddEndpointMapping("cn-hangzhou", "Ecs", "unreachable.aliyuncs.com")
	endpoint, err := Resolve(param)

	assert.Nil(t, err)
	assert.Equal(t, "unreachable.aliyuncs.com", endpoint)
}

func TestResolve_WithInvalidProduct(t *testing.T) {
	param := &ResolveParam{
		Product: "Invalid",
		RegionId: "cn-hangzhou",
	}
	endpoint, err := Resolve(param)

	assert.NotNil(t, err)
	assert.Equal(t, "", endpoint)
}
