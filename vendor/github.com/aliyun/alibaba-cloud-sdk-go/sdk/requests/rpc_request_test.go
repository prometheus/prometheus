package requests

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_RpcRequest(t *testing.T) {
	r := &RpcRequest{}
	r.InitWithApiInfo("product", "version", "action", "serviceCode", "endpointType")
	assert.NotNil(t, r)

	assert.Equal(t, "POST", r.GetMethod())
	assert.Equal(t, "RPC", r.GetStyle())
	assert.Equal(t, "product", r.GetProduct())
	assert.Equal(t, "version", r.GetVersion())
	assert.Equal(t, "action", r.GetActionName())
	assert.Equal(t, "serviceCode", r.GetLocationServiceCode())
	assert.Equal(t, "endpointType", r.GetLocationEndpointType())
}

func Test_RpcRequest_BuildQueries(t *testing.T) {
	// url
	r := &RpcRequest{}
	r.InitWithApiInfo("product", "version", "action", "serviceCode", "endpointType")
	assert.Equal(t, "/?", r.BuildQueries())
	r.addQueryParam("key", "value")
	assert.Equal(t, "/?key=value", r.BuildQueries())
	r.addQueryParam("key", "https://domain/?q=v")
	assert.Equal(t, "/?key=https%3A%2F%2Fdomain%2F%3Fq%3Dv", r.BuildQueries())
}

func Test_RpcRequest_BuildUrl(t *testing.T) {
	r := &RpcRequest{}
	r.InitWithApiInfo("product", "version", "action", "serviceCode", "endpointType")
	r.Domain = "domain.com"
	r.Scheme = "http"
	r.Port = "80"
	assert.Equal(t, "http://domain.com:80/?", r.BuildUrl())
	r.addQueryParam("key", "value")
	assert.Equal(t, "http://domain.com:80/?key=value", r.BuildUrl())
	r.addQueryParam("key", "https://domain/?q=v")
	assert.Equal(t, "http://domain.com:80/?key=https%3A%2F%2Fdomain%2F%3Fq%3Dv", r.BuildUrl())
}

func Test_RpcRequest_GetBodyReader(t *testing.T) {
	r := &RpcRequest{}
	r.InitWithApiInfo("product", "version", "action", "serviceCode", "endpointType")

	reader := r.GetBodyReader()
	b, _ := ioutil.ReadAll(reader)
	assert.Equal(t, "", string(b))
	r.addFormParam("key", "value")
	reader = r.GetBodyReader()
	b, _ = ioutil.ReadAll(reader)
	assert.Equal(t, "key=value", string(b))
}

func Test_RpcRequest_addPathParam(t *testing.T) {
	defer func() { //进行异常捕捉
		err := recover()
		assert.NotNil(t, err)
		assert.Equal(t, "not support", err)
	}()
	r := &RpcRequest{}
	r.InitWithApiInfo("product", "version", "action", "serviceCode", "endpointType")
	r.addPathParam("key", "value")
}
