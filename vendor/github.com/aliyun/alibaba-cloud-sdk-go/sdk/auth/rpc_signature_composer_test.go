package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/signers"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
)

func mockRpcDate(fn func() string) string {
	return "mock date"
}

func mockRpcGetNonce(fn func() string) string {
	return "MOCK_UUID"
}

func TestRpcSignatureComposer_buildRpcStringToSign(t *testing.T) {
	request := requests.NewCommonRequest()
	request.TransToAcsRequest()
	stringToSign := buildRpcStringToSign(request)
	assert.Equal(t, "GET&%2F&", stringToSign)
	request.FormParams["key"] = "value"
	stringToSign = buildRpcStringToSign(request)
	assert.Equal(t, "GET&%2F&key%3Dvalue", stringToSign)
	request.QueryParams["q"] = "value"
	stringToSign = buildRpcStringToSign(request)
	assert.Equal(t, "GET&%2F&key%3Dvalue%26q%3Dvalue", stringToSign)
	request.QueryParams["q"] = "http://domain/?q=value&q2=value2"
	stringToSign = buildRpcStringToSign(request)
	assert.Equal(t, "GET&%2F&key%3Dvalue%26q%3Dhttp%253A%252F%252Fdomain%252F%253Fq%253Dvalue%2526q2%253Dvalue2", stringToSign)
}

func TestRpcSignatureComposer(t *testing.T) {
	request := requests.NewCommonRequest()
	request.TransToAcsRequest()
	c := credentials.NewAccessKeyCredential("accessKeyId", "accessKeySecret")
	signer := signers.NewAccessKeySigner(c)

	origTestHookGetDate := hookGetDate
	defer func() { hookGetDate = origTestHookGetDate }()
	hookGetDate = mockRpcDate
	origTestHookGetNonce := hookGetNonce
	defer func() { hookGetNonce = origTestHookGetNonce }()
	hookGetNonce = mockRpcGetNonce
	signRpcRequest(request, signer, "regionId")
	assert.Equal(t, "mock date", request.GetQueryParams()["Timestamp"])
	assert.Equal(t, "MOCK_UUID", request.GetQueryParams()["SignatureNonce"])
	assert.Equal(t, "7loPmFjvDnzOVnQeQNj85S6nFGY=", request.GetQueryParams()["Signature"])
	signRpcRequest(request, signer, "regionId")
	assert.Equal(t, "mock date", request.GetQueryParams()["Timestamp"])
	assert.Equal(t, "MOCK_UUID", request.GetQueryParams()["SignatureNonce"])
	assert.Equal(t, "7loPmFjvDnzOVnQeQNj85S6nFGY=", request.GetQueryParams()["Signature"])
}

// func TestRpcSignatureComposer2(t *testing.T) {
// 	request := requests.NewCommonRequest()
// 	request.PathPattern = "/users/:user"
// 	request.FormParams["key"] = "value"
// 	request.AcceptFormat = "XML"
// 	request.TransToAcsRequest()
// 	c := credentials.NewAccessKeyCredential("accessKeyId", "accessKeySecret")
// 	signer := signers.NewAccessKeySigner(c)

// 	origTestHookLookupIP := hookGetDate
// 	defer func() { hookGetDate = origTestHookLookupIP }()
// 	hookGetDate = mockDate
// 	signRpcRequest(request, signer, "regionId")
// 	assert.Equal(t, "application/x-www-form-urlencoded", request.GetHeaders()["Content-Type"])
// 	assert.Equal(t, "mock date", request.GetHeaders()["Date"])
// 	assert.Equal(t, "application/xml", request.GetHeaders()["Accept"])
// 	assert.Equal(t, "acs accessKeyId:U9uA3ftRZKixHPB08Z7Z4GOlpTY=", request.GetQueryParams()["Signature"])
// }
