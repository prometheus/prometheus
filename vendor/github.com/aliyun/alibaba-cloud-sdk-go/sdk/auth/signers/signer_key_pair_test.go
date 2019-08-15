package signers

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
)

func TestKeyPairError(t *testing.T) {
	c := credentials.NewRsaKeyPairCredential("privateKey", "publicKey", 1)
	_, err := NewSignerKeyPair(c, nil)
	assert.NotNil(t, err)
	assert.Equal(t, "[SDK.InvalidParam] Key Pair session duration should be in the range of 15min - 1Hr", err.Error())
}

func TestKeyPairOk(t *testing.T) {
	c := credentials.NewRsaKeyPairCredential("privateKey", "publicKey", 0)
	s, err := NewSignerKeyPair(c, nil)
	assert.Nil(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, 3600, s.credentialExpiration)
	c = credentials.NewRsaKeyPairCredential("privateKey", "publicKey", 3500)
	s, err = NewSignerKeyPair(c, nil)
	assert.Nil(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, 3500, s.credentialExpiration)
	assert.Equal(t, "HMAC-SHA1", s.GetName())
	assert.Equal(t, "1.0", s.GetVersion())
	assert.Equal(t, "", s.GetType())
	assert.Len(t, s.GetExtraParam(), 0)
}

func Test_buildCommonRequest(t *testing.T) {
	c := credentials.NewRsaKeyPairCredential("privateKey", "publicKey", 0)
	s, err := NewSignerKeyPair(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
		return nil, fmt.Errorf("common api fails")
	})
	assert.Nil(t, err)
	request, err := s.buildCommonRequest()
	assert.Nil(t, err)
	assert.NotNil(t, request)
	assert.Equal(t, "Sts", request.Product)
	assert.Equal(t, "2015-04-01", request.Version)
	assert.Equal(t, "GenerateSessionAccessKey", request.ApiName)
	assert.Equal(t, "HTTPS", request.Scheme)
	assert.Equal(t, "publicKey", request.QueryParams["PublicKeyId"])
	assert.Equal(t, "3600", request.QueryParams["DurationSeconds"])
}

func TestGetAccessKeyId(t *testing.T) {
	c := credentials.NewRsaKeyPairCredential("privateKey", "publicKey", 0)
	s, err := NewSignerKeyPair(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
		return nil, fmt.Errorf("common api fails")
	})
	assert.Nil(t, err)
	assert.NotNil(t, s)
	accessKeyId, err := s.GetAccessKeyId()
	assert.Equal(t, "common api fails", err.Error())
	assert.Equal(t, "", accessKeyId)
}

func TestGetAccessKeyId2(t *testing.T) {
	// default response is not OK
	c := credentials.NewRsaKeyPairCredential("privateKey", "publicKey", 0)
	s, err := NewSignerKeyPair(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
		return responses.NewCommonResponse(), nil
	})
	assert.Nil(t, err)
	assert.NotNil(t, s)
	// s.lastUpdateTimestamp = time.Now().Unix() - 1000
	accessKeyId, err := s.GetAccessKeyId()
	assert.Equal(t, "SDK.ServerError\nErrorCode: \nRecommend: refresh session AccessKey failed\nRequestId: \nMessage: ", err.Error())
	assert.Equal(t, "", accessKeyId)
}

func TestGetAccessKeyId3(t *testing.T) {
	c := credentials.NewRsaKeyPairCredential("privateKey", "publicKey", 0)
	// Mock the 200 response and invalid json
	s, err := NewSignerKeyPair(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
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
		httpresp.Body = ioutil.NopCloser(bytes.NewReader([]byte("invalid json")))
		responses.Unmarshal(res, httpresp, "JSON")
		return res, nil
	})
	assert.Nil(t, err)
	assert.NotNil(t, s)
	// s.lastUpdateTimestamp = time.Now().Unix() - 1000
	accessKeyId, err := s.GetAccessKeyId()
	assert.NotNil(t, err)
	assert.Equal(t, "refresh KeyPair err, json.Unmarshal fail: invalid character 'i' looking for beginning of value", err.Error())
	assert.Equal(t, "", accessKeyId)
}

func TestGetAccessKeyId4(t *testing.T) {
	c := credentials.NewRsaKeyPairCredential("privateKey", "publicKey", 0)
	// mock 200 response and valid json, but no data
	s, err := NewSignerKeyPair(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
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

func TestGetAccessKeyIdAndSign(t *testing.T) {
	c := credentials.NewRsaKeyPairCredential("privateKey", "publicKey", 0)
	// mock 200 response and valid json and valid result
	s, err := NewSignerKeyPair(c, func(*requests.CommonRequest, interface{}) (response *responses.CommonResponse, err error) {
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
		json := `{"SessionAccessKey":{"SessionAccessKeyId":"session access key id","SessionAccessKeySecret": "session access key secret"}}`
		httpresp.Body = ioutil.NopCloser(bytes.NewReader([]byte(json)))
		responses.Unmarshal(res, httpresp, "JSON")
		return res, nil
	})
	assert.Nil(t, err)
	assert.NotNil(t, s)
	// s.lastUpdateTimestamp = time.Now().Unix() - 1000
	accessKeyId, err := s.GetAccessKeyId()
	assert.Nil(t, err)
	assert.Equal(t, "session access key id", accessKeyId)
	// no need update
	err = s.ensureCredential()
	assert.Nil(t, err)
	signature := s.Sign("string to sign", "/")
	assert.Equal(t, "cgoenM6nl61t2wFdlaHVySuGAgY=", signature)
}
