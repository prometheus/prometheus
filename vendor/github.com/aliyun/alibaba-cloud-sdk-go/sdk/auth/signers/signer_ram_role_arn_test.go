package signers

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
	"github.com/stretchr/testify/assert"
)

func Test_NewRamRoleArnSigner(t *testing.T) {
	c := credentials.NewRamRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", 3500)
	signer, err := NewRamRoleArnSigner(c, nil)
	assert.Nil(t, err)
	assert.Equal(t, "roleSessionName", signer.roleSessionName)
	assert.Equal(t, 3500, signer.credentialExpiration)

	assert.Equal(t, "HMAC-SHA1", signer.GetName())
	assert.Equal(t, "", signer.GetType())
	assert.Equal(t, "1.0", signer.GetVersion())

	c = credentials.NewRamRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "", 0)
	signer, err = NewRamRoleArnSigner(c, nil)
	assert.Nil(t, err)
	assert.True(t, strings.HasPrefix(signer.roleSessionName, "aliyun-go-sdk-"))
	assert.Equal(t, 3600, signer.credentialExpiration)

	c = credentials.NewRamRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "", 100)
	signer, err = NewRamRoleArnSigner(c, nil)
	assert.NotNil(t, err)
	assert.Equal(t, "[SDK.InvalidParam] Assume Role session duration should be in the range of 15min - 1Hr", err.Error())
}

func Test_RamRoleArn_buildCommonRequest(t *testing.T) {
	c := credentials.NewRamRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", 3600)
	s, err := NewRamRoleArnSigner(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
		return nil, fmt.Errorf("common api fails")
	})
	assert.Nil(t, err)
	request, err := s.buildCommonRequest()
	assert.Nil(t, err)
	assert.NotNil(t, request)
	assert.Equal(t, "Sts", request.Product)
	assert.Equal(t, "2015-04-01", request.Version)
	assert.Equal(t, "AssumeRole", request.ApiName)
	assert.Equal(t, "HTTPS", request.Scheme)
	assert.Equal(t, "roleArn", request.QueryParams["RoleArn"])
	assert.Equal(t, "roleSessionName", request.QueryParams["RoleSessionName"])
	assert.Equal(t, "3600", request.QueryParams["DurationSeconds"])
	assert.Nil(t, s.GetSessionCredential())
}

func Test_RamRoleArn_GetAccessKeyId(t *testing.T) {
	c := credentials.NewRamRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", 3600)
	s, err := NewRamRoleArnSigner(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
		return nil, fmt.Errorf("common api fails")
	})
	assert.Nil(t, err)
	assert.NotNil(t, s)
	accessKeyId, err := s.GetAccessKeyId()
	assert.Equal(t, "common api fails", err.Error())
	assert.Equal(t, "", accessKeyId)
}

func Test_RamRoleArn_GetAccessKeyId2(t *testing.T) {
	// default response is not OK
	c := credentials.NewRamRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", 3600)
	s, err := NewRamRoleArnSigner(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
		return responses.NewCommonResponse(), nil
	})
	assert.Nil(t, err)
	assert.NotNil(t, s)
	// s.lastUpdateTimestamp = time.Now().Unix() - 1000
	accessKeyId, err := s.GetAccessKeyId()
	assert.Equal(t, "SDK.ServerError\nErrorCode: \nRecommend: refresh session token failed\nRequestId: \nMessage: ", err.Error())
	assert.Equal(t, "", accessKeyId)
}

func Test_RamRoleArn_GetAccessKeyId3(t *testing.T) {
	c := credentials.NewRamRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", 3600)
	// Mock the 200 response and invalid json
	s, err := NewRamRoleArnSigner(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
		res := responses.NewCommonResponse()
		statusCode := 200
		status := strconv.Itoa(statusCode)
		httpresp := &http.Response{
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			Header:     make(http.Header),
			StatusCode: statusCode,
			Status:     status + " " + http.StatusText(statusCode),
		}
		httpresp.Body = ioutil.NopCloser(bytes.NewReader([]byte("invalid json")))
		responses.Unmarshal(res, httpresp, "JSON")
		return res, nil
	})
	assert.Nil(t, err)
	assert.NotNil(t, s)
	// s.lastUpdateTimestamp = time.Now().Unix() - 1000
	accessKeyId, err := s.GetAccessKeyId()
	assert.NotNil(t, err)
	assert.Equal(t, "refresh RoleArn sts token err, json.Unmarshal fail: invalid character 'i' looking for beginning of value", err.Error())
	assert.Equal(t, "", accessKeyId)
}

func Test_RamRoleArn_GetAccessKeyId4(t *testing.T) {
	c := credentials.NewRamRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", 3600)
	// Mock the 200 response and invalid json
	s, err := NewRamRoleArnSigner(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
		res := responses.NewCommonResponse()
		statusCode := 200
		header := make(http.Header)
		status := strconv.Itoa(statusCode)
		httpresp := &http.Response{
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			Header:     header,
			StatusCode: statusCode,
			Status:     status + " " + http.StatusText(statusCode),
		}
		httpresp.Header = make(http.Header)
		httpresp.Body = ioutil.NopCloser(bytes.NewReader([]byte("{}")))
		responses.Unmarshal(res, httpresp, "JSON")
		return res, nil
	})
	assert.Nil(t, err)
	assert.NotNil(t, s)
	// s.lastUpdateTimestamp = time.Now().Unix() - 1000
	accessKeyId, err := s.GetAccessKeyId()
	assert.Nil(t, err)
	assert.Equal(t, "", accessKeyId)
}

func Test_RamRoleArn_GetAccessKeyIdAndSign(t *testing.T) {
	c := credentials.NewRamRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", 3600)
	// mock 200 response and valid json and valid result
	s, err := NewRamRoleArnSigner(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
		res := responses.NewCommonResponse()
		statusCode := 200
		header := make(http.Header)
		status := strconv.Itoa(statusCode)
		httpresp := &http.Response{
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			Header:     header,
			StatusCode: statusCode,
			Status:     status + " " + http.StatusText(statusCode),
		}

		json := `{"Credentials":{"AccessKeyId":"access key id","AccessKeySecret": "access key secret","SecurityToken":"security token"}}`
		httpresp.Body = ioutil.NopCloser(bytes.NewReader([]byte(json)))
		responses.Unmarshal(res, httpresp, "JSON")
		return res, nil
	})
	assert.Nil(t, err)
	assert.NotNil(t, s)
	// s.lastUpdateTimestamp = time.Now().Unix() - 1000
	accessKeyId, err := s.GetAccessKeyId()
	assert.Nil(t, err)
	assert.Equal(t, "access key id", accessKeyId)

	params := s.GetExtraParam()
	assert.NotNil(t, params)
	assert.Len(t, params, 1)
	assert.Equal(t, "security token", params["SecurityToken"])
	// assert.Nil(t, err)
	signature := s.Sign("string to sign", "/")
	assert.Equal(t, "dcM4bWGEoD5QUp9xhLW3SfcWfgs=", signature)
}

func Test_RamRoleArn_GetExtraParam_Fail(t *testing.T) {
	c := credentials.NewRamRoleArnWithPolicyCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", "policy", 3600)
	// mock 200 response and valid json and valid result
	s, err := NewRamRoleArnSigner(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
		res := responses.NewCommonResponse()
		statusCode := 200
		header := make(http.Header)
		status := strconv.Itoa(statusCode)
		httpresp := &http.Response{
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			Header:     header,
			StatusCode: statusCode,
			Status:     status + " " + http.StatusText(statusCode),
		}

		json := `{"Credentials":{"AccessKeyId":"access key id","AccessKeySecret": "access key secret","SecurityToken":""}}`
		httpresp.Body = ioutil.NopCloser(bytes.NewReader([]byte(json)))
		responses.Unmarshal(res, httpresp, "JSON")
		return res, nil
	})
	assert.Nil(t, err)
	assert.NotNil(t, s)

	params := s.GetExtraParam()
	assert.Len(t, params, 0)
}
