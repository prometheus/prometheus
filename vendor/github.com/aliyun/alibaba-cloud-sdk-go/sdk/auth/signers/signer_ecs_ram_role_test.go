package signers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/stretchr/testify/assert"
)

func Test_ECSRamRole(t *testing.T) {
	c := credentials.NewEcsRamRoleCredential("roleName")
	singer := NewEcsRamRoleSigner(c, nil)
	assert.NotNil(t, singer)
	assert.Equal(t, "HMAC-SHA1", singer.GetName())
	assert.Equal(t, "", singer.GetType())
	assert.Equal(t, "1.0", singer.GetVersion())
}

func Test_EcsRamRoleSigner_buildCommonRequest(t *testing.T) {
	c := credentials.NewEcsRamRoleCredential("roleName")
	s := NewEcsRamRoleSigner(c, nil)
	request, err := s.buildCommonRequest()
	assert.Nil(t, err)
	assert.Nil(t, request)
}

func Test_EcsRamRoleSigner_GetAccessKeyId(t *testing.T) {
	c := credentials.NewEcsRamRoleCredential("roleName")
	s := NewEcsRamRoleSigner(c, nil)
	assert.NotNil(t, s)
	// Update our securityCredURL to point at our local test server.
	originalSecurityCredURL := securityCredURL
	securityCredURL = strings.Replace(securityCredURL, "http://100.100.100.200", "http://invalid-domain-xxx", -1)
	defer func() {
		securityCredURL = originalSecurityCredURL
	}()

	accessKeyId, err := s.GetAccessKeyId()
	assert.True(t, strings.HasSuffix(err.Error(), "no such host"))
	assert.Equal(t, "", accessKeyId)
}

func mockServer(status int, json string) (server *httptest.Server) {
	// Start a test server locally.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/meta-data/ram/security-credentials/roleName":
			w.WriteHeader(status)
			w.Write([]byte(json))
		}
	}))
	return ts
}

func Test_EcsRamRoleSigner_GetAccessKeyId2(t *testing.T) {
	c := credentials.NewEcsRamRoleCredential("roleName")
	s := NewEcsRamRoleSigner(c, nil)
	assert.NotNil(t, s)
	// Start a test server locally.
	ts := mockServer(400, "{}")
	defer ts.Close()
	originalSecurityCredURL := securityCredURL
	securityCredURL = strings.Replace(securityCredURL, "http://100.100.100.200", ts.URL, -1)
	defer func() {
		securityCredURL = originalSecurityCredURL
	}()
	accessKeyId, err := s.GetAccessKeyId()
	assert.Equal(t, "SDK.ServerError\nErrorCode: \nRecommend: \nRequestId: \nMessage: {}", err.Error())
	assert.Equal(t, "", accessKeyId)
}

func Test_EcsRamRoleSigner_GetAccessKeyId3(t *testing.T) {
	c := credentials.NewEcsRamRoleCredential("roleName")
	s := NewEcsRamRoleSigner(c, nil)
	assert.NotNil(t, s)
	// Start a test server locally.
	ts := mockServer(200, "invalid json")
	defer ts.Close()
	originalSecurityCredURL := securityCredURL
	securityCredURL = strings.Replace(securityCredURL, "http://100.100.100.200", ts.URL, -1)
	defer func() {
		securityCredURL = originalSecurityCredURL
	}()
	accessKeyId, err := s.GetAccessKeyId()
	assert.Equal(t, "refresh Ecs sts token err, json.Unmarshal fail: invalid character 'i' looking for beginning of value", err.Error())
	assert.Equal(t, "", accessKeyId)
}

func Test_EcsRamRoleSigner_GetAccessKeyId4(t *testing.T) {
	c := credentials.NewEcsRamRoleCredential("roleName")
	s := NewEcsRamRoleSigner(c, nil)
	assert.NotNil(t, s)
	// Start a test server locally.
	ts := mockServer(200, `{"Code":"Fails"}`)
	defer ts.Close()
	originalSecurityCredURL := securityCredURL
	securityCredURL = strings.Replace(securityCredURL, "http://100.100.100.200", ts.URL, -1)
	defer func() {
		securityCredURL = originalSecurityCredURL
	}()
	accessKeyId, err := s.GetAccessKeyId()
	assert.Equal(t, "refresh Ecs sts token err, Code is not Success", err.Error())
	assert.Equal(t, "", accessKeyId)
}

func Test_EcsRamRoleSigner_GetAccessKeyId5(t *testing.T) {
	c := credentials.NewEcsRamRoleCredential("roleName")
	s := NewEcsRamRoleSigner(c, nil)
	assert.NotNil(t, s)
	// Start a test server locally.
	ts := mockServer(200, `{"Code":"Success"}`)
	defer ts.Close()
	originalSecurityCredURL := securityCredURL
	securityCredURL = strings.Replace(securityCredURL, "http://100.100.100.200", ts.URL, -1)
	defer func() {
		securityCredURL = originalSecurityCredURL
	}()
	accessKeyId, err := s.GetAccessKeyId()
	assert.Nil(t, err)
	assert.Equal(t, "", accessKeyId)
}

func Test_EcsRamRoleSigner_GetAccessKeyId6(t *testing.T) {
	c := credentials.NewEcsRamRoleCredential("roleName")
	s := NewEcsRamRoleSigner(c, nil)
	assert.NotNil(t, s)
	// Start a test server locally.
	ts := mockServer(201, `{"Code":"Success"}`)
	defer ts.Close()
	originalSecurityCredURL := securityCredURL
	securityCredURL = strings.Replace(securityCredURL, "http://100.100.100.200", ts.URL, -1)
	defer func() {
		securityCredURL = originalSecurityCredURL
	}()
	accessKeyId, err := s.GetAccessKeyId()
	assert.Equal(t, "refresh Ecs sts token err, httpStatus: 201, message = {\"Code\":\"Success\"}", err.Error())
	assert.Equal(t, "", accessKeyId)
}

func Test_EcsRamRoleSigner_GetAccessKeyId_Success(t *testing.T) {
	c := credentials.NewEcsRamRoleCredential("roleName")
	s := NewEcsRamRoleSigner(c, nil)
	assert.NotNil(t, s)
	// Start a test server locally.
	nextDay := time.Now().AddDate(0, 0, 1)
	ts := mockServer(200, fmt.Sprintf(`{
		"Code": "Success",
		"AccessKeyId":"access key id",
		"AccessKeySecret":"access key secret",
		"SecurityToken":"security token",
		"Expiration": "%s"
	}`, nextDay.Format("2006-01-02T15:04:05Z")))
	defer ts.Close()
	originalSecurityCredURL := securityCredURL
	securityCredURL = strings.Replace(securityCredURL, "http://100.100.100.200", ts.URL, -1)
	defer func() {
		securityCredURL = originalSecurityCredURL
	}()
	// sessionCredential should be nil
	assert.Len(t, s.GetExtraParam(), 0)
	assert.Nil(t, s.GetSessionCredential())
	accessKeyId, err := s.GetAccessKeyId()
	assert.Nil(t, err)
	assert.Equal(t, "access key id", accessKeyId)
	expiration := s.credentialExpiration
	accessKeyId, err = s.GetAccessKeyId()
	assert.NotNil(t, s.GetSessionCredential())
	assert.Nil(t, err)
	assert.Equal(t, "access key id", accessKeyId)
	assert.Len(t, s.GetExtraParam(), 1)
	assert.Equal(t, "security token", s.GetExtraParam()["SecurityToken"])
	// the expiration should not changed. hit cache
	assert.Equal(t, expiration, s.credentialExpiration)

	assert.Equal(t, "dcM4bWGEoD5QUp9xhLW3SfcWfgs=", s.Sign("string to sign", "/"))
	s.sessionCredential.StsToken = ""
	assert.Len(t, s.GetExtraParam(), 0)
}
