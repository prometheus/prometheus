package endpoints

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleHostResolver_GetName(t *testing.T) {
	resolver := &SimpleHostResolver{}
	assert.Equal(t, "simple host resolver", resolver.GetName())
}

func TestSimpleHostResolver_TryResolve(t *testing.T) {
	resolver := &SimpleHostResolver{}
	resolveParam := &ResolveParam{}
	endpoint, support, err := resolver.TryResolve(resolveParam)
	assert.Nil(t, err)
	assert.Equal(t, "", endpoint)
	assert.Equal(t, false, support)

	resolveParam = &ResolveParam{
		Domain: "unreachable.aliyuncs.com",
	}
	endpoint, support, err = resolver.TryResolve(resolveParam)
	assert.Nil(t, err)
	assert.Equal(t, "unreachable.aliyuncs.com", endpoint)
	assert.Equal(t, true, support)

	fmt.Println("finished")
}
