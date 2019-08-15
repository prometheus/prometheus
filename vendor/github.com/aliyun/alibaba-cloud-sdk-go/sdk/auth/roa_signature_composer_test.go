package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/signers"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
)

func mockDate(fn func() string) string {
	return "mock date"
}

func TestRoaSignatureComposer_buildRoaStringToSign(t *testing.T) {
	request := requests.NewCommonRequest()
	request.PathPattern = "/users/:user"
	request.TransToAcsRequest()
	stringToSign := buildRoaStringToSign(request)
	assert.Equal(t, "GET\nx-acs-version:\n/users/:user", stringToSign)

	request.Headers["Accept"] = "application/json;charset=utf8"
	stringToSign = buildRoaStringToSign(request)
	assert.Equal(t, "GET\napplication/json;charset=utf8\nx-acs-version:\n/users/:user", stringToSign)
	request.SetContentType("application/json")
	request.Headers["x-acs-custom-header"] = "value"
	stringToSign = buildRoaStringToSign(request)
	assert.Equal(t, "GET\napplication/json;charset=utf8\napplication/json\nx-acs-custom-header:value\nx-acs-version:\n/users/:user", stringToSign)
	request.QueryParams["q"] = "value"
	stringToSign = buildRoaStringToSign(request)
	assert.Equal(t, "GET\napplication/json;charset=utf8\napplication/json\nx-acs-custom-header:value\nx-acs-version:\n/users/:user?q=value", stringToSign)
	request.QueryParams["q"] = "http://domain/?q=value&q2=value2"
	stringToSign = buildRoaStringToSign(request)
	assert.Equal(t, "GET\napplication/json;charset=utf8\napplication/json\nx-acs-custom-header:value\nx-acs-version:\n/users/:user?q=http://domain/?q=value&q2=value2", stringToSign)
}

func TestRoaSignatureComposer(t *testing.T) {
	request := requests.NewCommonRequest()
	request.PathPattern = "/users/:user"
	request.TransToAcsRequest()
	c := credentials.NewAccessKeyCredential("accessKeyId", "accessKeySecret")
	signer := signers.NewAccessKeySigner(c)

	origTestHookGetDate := hookGetDate
	defer func() { hookGetDate = origTestHookGetDate }()
	hookGetDate = mockDate
	signRoaRequest(request, signer, "regionId")
	assert.Equal(t, "mock date", request.GetHeaders()["Date"])
	assert.Equal(t, "acs accessKeyId:degLHXLEN6rMojj+bOlK74U9iic=", request.GetHeaders()["Authorization"])
}

func TestRoaSignatureComposer2(t *testing.T) {
	request := requests.NewCommonRequest()
	request.PathPattern = "/users/:user"
	request.FormParams["key"] = "value"
	request.AcceptFormat = "XML"
	request.TransToAcsRequest()
	c := credentials.NewAccessKeyCredential("accessKeyId", "accessKeySecret")
	signer := signers.NewAccessKeySigner(c)

	origTestHookLookupIP := hookGetDate
	defer func() { hookGetDate = origTestHookLookupIP }()
	hookGetDate = mockDate
	signRoaRequest(request, signer, "regionId")
	assert.Equal(t, "application/x-www-form-urlencoded", request.GetHeaders()["Content-Type"])
	assert.Equal(t, "mock date", request.GetHeaders()["Date"])
	assert.Equal(t, "application/xml", request.GetHeaders()["Accept"])
	assert.Equal(t, "acs accessKeyId:U9uA3ftRZKixHPB08Z7Z4GOlpTY=", request.GetHeaders()["Authorization"])
}

func TestRoaSignatureComposer3(t *testing.T) {
	request := requests.NewCommonRequest()
	request.PathPattern = "/users/:user"
	request.AcceptFormat = "RAW"
	request.TransToAcsRequest()
	c := credentials.NewAccessKeyCredential("accessKeyId", "accessKeySecret")
	signer := signers.NewAccessKeySigner(c)

	origTestHookGetDate := hookGetDate
	defer func() { hookGetDate = origTestHookGetDate }()
	hookGetDate = mockDate
	signRoaRequest(request, signer, "regionId")
	assert.Equal(t, "mock date", request.GetHeaders()["Date"])
}
func TestCompleteROASignParams(t *testing.T) {
	req := requests.NewCommonRequest()
	req.TransToAcsRequest()
	sign := signers.NewBearerTokenSigner(credentials.NewBearerTokenCredential("Bearer.Token"))
	completeROASignParams(req, sign, "cn-hangzhou")
	head := req.GetHeaders()
	assert.Equal(t, "Bearer.Token", head["x-acs-bearer-token"])
}
