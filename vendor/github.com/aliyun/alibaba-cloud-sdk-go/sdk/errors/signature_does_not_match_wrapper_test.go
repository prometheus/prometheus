package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrap(t *testing.T) {
	wrapper := &SignatureDostNotMatchWrapper{}
	m := make(map[string]string)
	err := NewServerError(400, "content", "comment")
	se, ok := err.(*ServerError)
	assert.True(t, ok)
	wrapped := wrapper.tryWrap(se, m)
	assert.False(t, wrapped)
}

func TestWrapNotMatch(t *testing.T) {
	wrapper := &SignatureDostNotMatchWrapper{}
	err := NewServerError(400, `{"Code":"SignatureDoesNotMatch","Message":"Specified signature is not matched with our calculation. server string to sign is:hehe"}`, "comment")
	se, ok := err.(*ServerError)
	assert.True(t, ok)
	assert.Equal(t, "SignatureDoesNotMatch", se.ErrorCode())
	m := make(map[string]string)
	m["StringToSign"] = "not match"
	wrapped := wrapper.tryWrap(se, m)
	assert.True(t, wrapped)
	assert.Equal(t, "This may be a bug with the SDK and we hope you can submit this question in the github issue(https://github.com/aliyun/alibaba-cloud-sdk-go/issues), thanks very much", se.Recommend())
}

func TestWrapMatch(t *testing.T) {
	wrapper := &SignatureDostNotMatchWrapper{}
	err := NewServerError(400, `{"Code":"SignatureDoesNotMatch","Message":"Specified signature is not matched with our calculation. server string to sign is:match"}`, "comment")
	se, ok := err.(*ServerError)
	assert.True(t, ok)
	assert.Equal(t, "SignatureDoesNotMatch", se.ErrorCode())
	m := make(map[string]string)
	m["StringToSign"] = "match"
	wrapped := wrapper.tryWrap(se, m)
	assert.True(t, wrapped)
	assert.Equal(t, "InvalidAccessKeySecret: Please check you AccessKeySecret", se.Recommend())
}

func TestWrapMatchWhenCodeIsIncompleteSignature(t *testing.T) {
	wrapper := &SignatureDostNotMatchWrapper{}
	err := NewServerError(400, `{"Code":"IncompleteSignature","Message":"Specified signature is not matched with our calculation. server string to sign is:match"}`, "comment")
	se, ok := err.(*ServerError)
	assert.True(t, ok)
	assert.Equal(t, "IncompleteSignature", se.ErrorCode())
	m := make(map[string]string)
	m["StringToSign"] = "match"
	wrapped := wrapper.tryWrap(se, m)
	assert.True(t, wrapped)
	assert.Equal(t, "InvalidAccessKeySecret: Please check you AccessKeySecret", se.Recommend())
}
