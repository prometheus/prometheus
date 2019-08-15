package requests

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_RoaRequest(t *testing.T) {
	r := &RoaRequest{}
	r.InitWithApiInfo("product", "version", "action", "/", "serviceCode", "endpointType")
	assert.NotNil(t, r)

	assert.Equal(t, "GET", r.GetMethod())
	assert.Equal(t, "ROA", r.GetStyle())
	// assert.Equal(t, "version", r.GetVersion())
	// assert.Equal(t, "action", r.GetActionName())
	assert.Equal(t, "serviceCode", r.GetLocationServiceCode())
	assert.Equal(t, "endpointType", r.GetLocationEndpointType())
}

func Test_RoaRequest_initWithCommonRequest(t *testing.T) {
	r := &RoaRequest{}
	common := NewCommonRequest()
	r.initWithCommonRequest(common)
	assert.NotNil(t, r)

	assert.Equal(t, "GET", r.GetMethod())
	assert.Equal(t, "ROA", r.GetStyle())
	assert.Equal(t, "common", r.Headers["x-sdk-invoke-type"])
	// assert.Equal(t, "version", r.GetVersion())
	// assert.Equal(t, "action", r.GetActionName())
}

func Test_RoaRequest_BuildQueries(t *testing.T) {
	// url
	r := &RoaRequest{}
	r.InitWithApiInfo("product", "version", "action", "/", "serviceCode", "endpointType")
	assert.Equal(t, "/", r.BuildQueries())
	r.addQueryParam("key", "value")
	assert.Equal(t, "/?key=value", r.BuildQueries())
	r.addQueryParam("key2", "value2")
	assert.Equal(t, "/?key=value&key2=value2", r.BuildQueries())
	// assert.Equal(t, "/?key=https%3A%2F%2Fdomain%2F%3Fq%3Dv", r.BuildQueries())
}

func Test_RoaRequest_BuildUrl(t *testing.T) {
	r := &RoaRequest{}
	r.InitWithApiInfo("product", "version", "action", "/", "serviceCode", "endpointType")
	r.Domain = "domain.com"
	r.Scheme = "http"
	r.Port = "80"
	assert.Equal(t, "http://domain.com:80/", r.BuildUrl())
	r.addQueryParam("key", "value")
	assert.Equal(t, "http://domain.com:80/?key=value", r.BuildUrl())
	r.addQueryParam("key", "https://domain/?q=v")
	assert.Equal(t, "http://domain.com:80/?key=https%3A%2F%2Fdomain%2F%3Fq%3Dv", r.BuildUrl())
	r.addQueryParam("url", "https://domain/?q1=v1&q2=v2")
	assert.Equal(t, "http://domain.com:80/?key=https%3A%2F%2Fdomain%2F%3Fq%3Dv&url=https%3A%2F%2Fdomain%2F%3Fq1%3Dv1%26q2%3Dv2", r.BuildUrl())
}

func Test_RoaRequest_BuildUrl2(t *testing.T) {
	r := &RoaRequest{}
	r.InitWithApiInfo("product", "version", "action", "/", "serviceCode", "endpointType")
	r.Domain = "domain.com"
	r.Scheme = "http"
	r.Port = "80"
	assert.Equal(t, "http://domain.com:80/", r.BuildUrl())
	r.addPathParam("key", "value")
	assert.Equal(t, "http://domain.com:80/", r.BuildUrl())

	r = &RoaRequest{}
	r.InitWithApiInfo("product", "version", "action", "/users/[user]", "serviceCode", "endpointType")
	r.Domain = "domain.com"
	r.Scheme = "http"
	r.Port = "80"
	r.addPathParam("user", "name")
	assert.Equal(t, "http://domain.com:80/users/name", r.BuildUrl())
	r.addQueryParam("key", "value")
	assert.Equal(t, "http://domain.com:80/users/name?key=value", r.BuildUrl())
}

func Test_RoaRequest_GetBodyReader_Nil(t *testing.T) {
	r := &RoaRequest{}
	r.InitWithApiInfo("product", "version", "action", "/", "serviceCode", "endpointType")

	reader := r.GetBodyReader()
	assert.Nil(t, reader)
}

func Test_RoaRequest_GetBodyReader_Form(t *testing.T) {
	r := &RoaRequest{}
	r.InitWithApiInfo("product", "version", "action", "/", "serviceCode", "endpointType")

	r.addFormParam("key", "value")
	reader := r.GetBodyReader()
	b, _ := ioutil.ReadAll(reader)
	assert.Equal(t, "key=value", string(b))
}

func Test_RoaRequest_GetBodyReader_Content(t *testing.T) {
	r := &RoaRequest{}
	r.InitWithApiInfo("product", "version", "action", "/", "serviceCode", "endpointType")

	r.SetContent([]byte("Hello world"))
	reader := r.GetBodyReader()
	b, _ := ioutil.ReadAll(reader)
	assert.Equal(t, "Hello world", string(b))
}

// func Test_RoaRequest_addPathParam(t *testing.T) {
// 	r := &RoaRequest{}
// 	r.InitWithApiInfo("product", "version", "action", "/", "serviceCode", "endpointType")
// 	r.addPathParam("key", "value")
// }
