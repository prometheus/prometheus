package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServerError(t *testing.T) {
	e := NewServerError(400, "content", "comment")
	assert.NotNil(t, e)
	serverError, ok := e.(*ServerError)
	assert.True(t, ok)
	assert.Nil(t, serverError.OriginError())
	assert.Equal(t, 400, serverError.HttpStatus())
	assert.Equal(t, "", serverError.RequestId())
	assert.Equal(t, "", serverError.ErrorCode())
	assert.Equal(t, "", serverError.Recommend())
	assert.Equal(t, "", serverError.HostId())
	assert.Equal(t, "comment", serverError.Comment())
	assert.Equal(t, "content", serverError.Message())
	assert.Equal(t, "SDK.ServerError\nErrorCode: \nRecommend: comment\nRequestId: \nMessage: content", serverError.Error())
}

func TestServerErrorWithContent(t *testing.T) {
	e := NewServerError(400, `{"RequestId":"request id","HostId":"host id","Code":"InvalidAK","Recommend":"recommend","Message":"message"}`, "comment")
	assert.NotNil(t, e)
	serverError, ok := e.(*ServerError)
	assert.True(t, ok)
	assert.Nil(t, serverError.OriginError())
	assert.Equal(t, 400, serverError.HttpStatus())
	assert.Equal(t, "request id", serverError.RequestId())
	assert.Equal(t, "host id", serverError.HostId())
	assert.Equal(t, "InvalidAK", serverError.ErrorCode())
	assert.Equal(t, "recommend", serverError.Recommend())
	assert.Equal(t, "comment", serverError.Comment())
	assert.Equal(t, "message", serverError.Message())
	assert.Equal(t, "SDK.ServerError\nErrorCode: InvalidAK\nRecommend: commentrecommend\nRequestId: request id\nMessage: message", serverError.Error())
}

func TestWrapServerError(t *testing.T) {
	err := NewServerError(400, `{"Code":"SignatureDoesNotMatch","Message":"Specified signature is not matched with our calculation. server string to sign is:hehe"}`, "comment")
	se, ok := err.(*ServerError)
	assert.True(t, ok)
	assert.Equal(t, "SignatureDoesNotMatch", se.ErrorCode())
	m := make(map[string]string)
	m["StringToSign"] = "not match"
	WrapServerError(se, m)
	assert.Equal(t, "This may be a bug with the SDK and we hope you can submit this question in the github issue(https://github.com/aliyun/alibaba-cloud-sdk-go/issues), thanks very much", se.Recommend())

	err = NewServerError(400, `{"Code":"SignatureDoesNotMatch","Message":"Specified signature is not matched with our calculation. server string to sign is:match"}`, "comment")
	se, ok = err.(*ServerError)
	assert.True(t, ok)
	assert.Equal(t, "SignatureDoesNotMatch", se.ErrorCode())
	m = make(map[string]string)
	m["StringToSign"] = "match"
	WrapServerError(se, m)
	assert.Equal(t, "InvalidAccessKeySecret: Please check you AccessKeySecret", se.Recommend())

	err = NewServerError(400, `{"Code":"Other"}`, "comment")
	se, ok = err.(*ServerError)
	assert.True(t, ok)
	assert.Equal(t, "Other", se.ErrorCode())
	m = make(map[string]string)
	WrapServerError(se, m)
	assert.Equal(t, "", se.Recommend())
}
