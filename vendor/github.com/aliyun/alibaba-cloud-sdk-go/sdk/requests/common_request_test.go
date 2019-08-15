package requests

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NewCommonRequest(t *testing.T) {
	r := NewCommonRequest()
	assert.NotNil(t, r)

	assert.Equal(t, "common", r.GetHeaders()["x-sdk-invoke-type"])
	assert.Equal(t, 0, len(r.PathParams))

	r.addPathParam("name", "value")
	assert.Equal(t, "value", r.PathParams["name"])
}

func Test_CommonRequest_TransToAcsRequest(t *testing.T) {
	r := NewCommonRequest()
	assert.NotNil(t, r)
	r.TransToAcsRequest()

	assert.Equal(t, "RPC", r.GetStyle())

	r2 := NewCommonRequest()
	assert.NotNil(t, r2)
	r2.PathPattern = "/users/[user]"
	r2.TransToAcsRequest()

	assert.Equal(t, "ROA", r2.GetStyle())
}

func Test_CommonRequest_String(t *testing.T) {
	r := NewCommonRequest()
	assert.NotNil(t, r)
	r.SetDomain("domain")

	expected := `GET /? /1.1
Host: domain
Accept-Encoding: identity
x-sdk-client: golang/1.0.0
x-sdk-invoke-type: common

`
	assert.Equal(t, expected, r.String())

	r.SetContent([]byte("content"))

	expected = `GET /? /1.1
Host: domain
Accept-Encoding: identity
x-sdk-client: golang/1.0.0
x-sdk-invoke-type: common

content
`
	assert.Equal(t, expected, r.String())
}

func Test_CommonRequest_BuildUrl(t *testing.T) {
	r := NewCommonRequest()
	assert.NotNil(t, r)
	r.SetDomain("host")
	r.SetScheme("http")

	r.TransToAcsRequest()

	assert.Equal(t, "http://host/?", r.BuildUrl())
	r.Port = "8080"
	assert.Equal(t, "http://host:8080/?", r.BuildUrl())
}

func Test_CommonRequest_GetBodyReader(t *testing.T) {
	r := NewCommonRequest()
	r.TransToAcsRequest()
	reader := r.GetBodyReader()
	b, _ := ioutil.ReadAll(reader)
	assert.Equal(t, "", string(b))
}
