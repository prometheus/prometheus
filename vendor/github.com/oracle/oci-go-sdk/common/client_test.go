// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package common

import (
	"bytes"
	"context"
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"time"
)

type customConfig struct {
	Reg Region
}

func (c customConfig) Region() (string, error) {
	return string(c.Reg), nil
}

func (c customConfig) KeyFingerprint() (string, error) {
	return "a/a/a", nil
}
func (c customConfig) UserOCID() (string, error) {
	return "ocid", nil
}
func (c customConfig) TenancyOCID() (string, error) {
	return "ocid1", nil
}
func (c customConfig) PrivateRSAKey() (*rsa.PrivateKey, error) {
	key, _ := PrivateKeyFromBytes([]byte(testPrivateKeyConf), nil)
	return key, nil
}
func (c customConfig) KeyID() (string, error) {
	return "b/b/b", nil
}

func testClientWithRegion(r Region) BaseClient {
	p := customConfig{Reg: r}
	c, _ := NewClientWithConfig(p)
	return c
}

func TestClient_prepareRequestDefScheme(t *testing.T) {
	host := "somehost:9000"
	basePath := "basePath"
	restPath := "somepath"

	c := BaseClient{UserAgent: "asdf"}
	c.Host = host
	c.BasePath = basePath

	request := http.Request{}
	request.URL = &url.URL{Path: restPath}
	c.prepareRequest(&request)
	assert.Equal(t, "https", request.URL.Scheme)
	assert.Equal(t, host, request.URL.Host)
}

func TestClient_prepareRequestCanBeCalledMultipleTimes(t *testing.T) {
	host := "somehost:9000"
	basePath := "basePath"
	restPath := "somepath"

	c := BaseClient{UserAgent: "asdf"}
	c.Host = host
	c.BasePath = basePath

	request := http.Request{}
	request.URL = &url.URL{Path: restPath}
	c.prepareRequest(&request)
	assert.Equal(t, "https", request.URL.Scheme)
	assert.Equal(t, "/basePath/somepath", request.URL.Path)
	assert.Equal(t, host, request.URL.Host)
	c.prepareRequest(&request)
	assert.Equal(t, "https", request.URL.Scheme)
	assert.Equal(t, "/basePath/somepath", request.URL.Path)
}

func TestClient_prepareRequestUpdatesDateHeader(t *testing.T) {
	host := "somehost:9000"
	basePath := "basePath"
	restPath := "somepath"

	c := BaseClient{UserAgent: "asdf"}
	c.Host = host
	c.BasePath = basePath

	request := http.Request{}
	request.URL = &url.URL{Path: restPath}
	c.prepareRequest(&request)
	d1 := request.Header.Get(requestHeaderDate)
	// make sure we wait some time to see that d1 and d2 have different times (set at second-level granularity)
	time.Sleep(2 * time.Second)
	c.prepareRequest(&request)
	d2 := request.Header.Get(requestHeaderDate)
	assert.NotEqual(t, d1, d2)
}

func TestClient_prepareRequestOnlySetsRetryTokenOnce(t *testing.T) {
	host := "somehost:9000"
	basePath := "basePath"
	restPath := "somepath"

	c := BaseClient{UserAgent: "asdf"}
	c.Host = host
	c.BasePath = basePath

	request := http.Request{}
	request.URL = &url.URL{Path: restPath}
	c.prepareRequest(&request)
	token1 := request.Header.Get(requestHeaderOpcRetryToken)
	assert.NotEmpty(t, token1)
	c.prepareRequest(&request)
	token2 := request.Header.Get(requestHeaderOpcRetryToken)
	assert.NotEmpty(t, token2)
	assert.Equal(t, token1, token2)
}

func TestDefaultHTTPDispatcher_transportNotSet(t *testing.T) {
	client := defaultHTTPDispatcher()

	if client.Transport != nil {
		t.Errorf("Expecting default http transport to be nil")
	}
}

func TestClient_prepareRequestSetScheme(t *testing.T) {
	host := "http://somehost:9000"
	basePath := "basePath"
	restPath := "somepath"

	c := BaseClient{UserAgent: "asdf"}
	c.Host = host
	c.BasePath = basePath

	request := http.Request{}
	request.URL = &url.URL{Path: restPath}
	c.prepareRequest(&request)
	assert.Equal(t, "http", request.URL.Scheme)
	assert.Equal(t, "somehost:9000", request.URL.Host)
}

func TestClient_containsUserAgent(t *testing.T) {
	host := "http://somehost:9000"
	basePath := "basePath"
	restPath := "somepath"
	userAgent := "myuserAgent"

	c := BaseClient{}
	c.Host = host
	c.BasePath = basePath
	c.UserAgent = userAgent

	request := http.Request{}
	request.URL = &url.URL{Path: restPath}
	c.prepareRequest(&request)
	assert.Equal(t, userAgent, request.UserAgent())
}

func TestClient_userAgentBlank(t *testing.T) {
	host := "http://somehost:9000"
	basePath := "basePath"
	restPath := "somepath"
	userAgent := ""

	c := BaseClient{}
	c.Host = host
	c.BasePath = basePath
	c.UserAgent = userAgent

	request := http.Request{}
	request.URL = &url.URL{Path: restPath}
	e := c.prepareRequest(&request)
	assert.Error(t, e)
}

func TestClient_clientForRegion(t *testing.T) {
	region := RegionPHX
	c := testClientWithRegion(region)
	assert.Equal(t, defaultUserAgent(), c.UserAgent)
	assert.NotNil(t, c.HTTPClient)
	assert.Nil(t, c.Interceptor)
	assert.NotNil(t, c.Signer)

}

func TestClient_customClientForRegion(t *testing.T) {
	host := "http://somehost:9000"
	basePath := "basePath"
	restPath := "somepath"
	userAgent := "suseragent"

	region := RegionPHX
	c := testClientWithRegion(region)
	c.Host = host
	c.UserAgent = userAgent
	c.BasePath = basePath

	request := http.Request{}
	request.URL = &url.URL{Path: restPath}
	c.prepareRequest(&request)

	assert.Equal(t, userAgent, c.UserAgent)
	assert.NotNil(t, c.HTTPClient)
	assert.Nil(t, c.Interceptor)
	assert.NotNil(t, c.Signer)
	assert.Equal(t, "http", request.URL.Scheme)
	assert.Equal(t, "somehost:9000", request.URL.Host)
}

type fakeCaller struct {
	CustomResponse *http.Response
	Customcall     func(r *http.Request) (*http.Response, error)
}

func (f fakeCaller) Do(req *http.Request) (*http.Response, error) {
	if f.CustomResponse != nil {
		return f.CustomResponse, nil
	}
	return f.Customcall(req)
}

func TestBaseClient_Call(t *testing.T) {
	response := http.Response{
		Header:     http.Header{},
		StatusCode: 200,
	}
	body := `{"key" : "RegionFRA","name" : "eu-frankfurt-1"}`
	c := testClientWithRegion(RegionIAD)
	host := "http://somehost:9000"
	basePath := "basePath/"
	restPath := "/somepath"
	caller := fakeCaller{
		Customcall: func(r *http.Request) (*http.Response, error) {
			assert.Equal(t, "somehost:9000", r.URL.Host)
			assert.Equal(t, defaultUserAgent(), r.UserAgent())
			assert.Contains(t, r.Header.Get(requestHeaderAuthorization), "signature")
			assert.Contains(t, r.URL.Path, "basePath/somepath")
			bodyBuffer := bytes.NewBufferString(body)
			response.Body = ioutil.NopCloser(bodyBuffer)
			return &response, nil
		},
	}

	c.Host = host
	c.BasePath = basePath
	c.HTTPClient = caller

	request := http.Request{}
	request.URL = &url.URL{Path: restPath}
	retRes, err := c.Call(context.Background(), &request)
	assert.Equal(t, &response, retRes)
	assert.NoError(t, err)

}

func TestBaseClient_CallWithInterceptor(t *testing.T) {
	response := http.Response{
		Header:     http.Header{},
		StatusCode: 200,
	}
	body := `{"key" : "RegionFRA","name" : "eu-frankfurt-1"}`
	c := testClientWithRegion(RegionIAD)
	c.Interceptor = func(request *http.Request) error {
		request.Header.Set("Custom-Header", "CustomValue")
		return nil
	}
	host := "http://somehost:9000"
	basePath := "basePath/"
	restPath := "/somepath"
	caller := fakeCaller{
		Customcall: func(r *http.Request) (*http.Response, error) {
			assert.Equal(t, "somehost:9000", r.URL.Host)
			assert.Equal(t, defaultUserAgent(), r.UserAgent())
			assert.Contains(t, r.Header.Get(requestHeaderAuthorization), "signature")
			assert.Contains(t, r.URL.Path, "basePath/somepath")
			assert.Equal(t, "CustomValue", r.Header.Get("Custom-Header"))
			bodyBuffer := bytes.NewBufferString(body)
			response.Body = ioutil.NopCloser(bodyBuffer)
			return &response, nil
		},
	}

	c.Host = host
	c.BasePath = basePath
	c.HTTPClient = caller

	request := http.Request{}
	request.URL = &url.URL{Path: restPath}
	retRes, err := c.Call(context.Background(), &request)
	assert.Equal(t, &response, retRes)
	assert.NoError(t, err)

}

type genericOCIResponse struct {
	RawResponse *http.Response
}

type retryableOCIRequest struct {
	retryPolicy *RetryPolicy
}

func (request retryableOCIRequest) HTTPRequest(method, path string) (http.Request, error) {
	r := http.Request{}
	r.Method = method
	r.URL = &url.URL{Path: path}
	return r, nil
}

func (request retryableOCIRequest) RetryPolicy() *RetryPolicy {
	return request.retryPolicy
}

// HTTPResponse implements the OCIResponse interface
func (response genericOCIResponse) HTTPResponse() *http.Response {
	return response.RawResponse
}

func TestRetry_NeverGetSuccessfulResponse(t *testing.T) {
	errorResponse := genericOCIResponse{
		RawResponse: &http.Response{
			Header:     http.Header{},
			StatusCode: 400,
		},
	}
	totalNumberAttempts := uint(5)
	numberOfTimesWeEnterShouldRetry := uint(0)
	numberOfTimesWeEnterGetNextDuration := uint(0)
	shouldRetryOperation := func(operationResponse OCIOperationResponse) bool {
		numberOfTimesWeEnterShouldRetry = numberOfTimesWeEnterShouldRetry + 1
		return true
	}
	getNextDuration := func(operationResponse OCIOperationResponse) time.Duration {
		numberOfTimesWeEnterGetNextDuration = numberOfTimesWeEnterGetNextDuration + 1
		return 0
	}
	retryPolicy := NewRetryPolicy(totalNumberAttempts, shouldRetryOperation, getNextDuration)
	retryableRequest := retryableOCIRequest{
		retryPolicy: &retryPolicy,
	}
	// type OCIOperation func(context.Context, OCIRequest) (OCIResponse, error)
	fakeOperation := func(context.Context, OCIRequest) (OCIResponse, error) {
		return errorResponse, nil
	}

	response, err := Retry(context.Background(), retryableRequest, fakeOperation, retryPolicy)
	assert.Equal(t, totalNumberAttempts, numberOfTimesWeEnterShouldRetry)
	assert.Equal(t, totalNumberAttempts-1, numberOfTimesWeEnterGetNextDuration)
	assert.Nil(t, response)
	assert.Equal(t, err.Error(), "maximum number of attempts exceeded (5)")
}

func TestRetry_ImmediatelyGetsSuccessfulResponse(t *testing.T) {
	successResponse := genericOCIResponse{
		RawResponse: &http.Response{
			Header:     http.Header{},
			StatusCode: 200,
		},
	}
	totalNumberAttempts := uint(5)
	numberOfTimesWeEnterShouldRetry := uint(0)
	numberOfTimesWeEnterGetNextDuration := uint(0)
	shouldRetryOperation := func(operationResponse OCIOperationResponse) bool {
		numberOfTimesWeEnterShouldRetry = numberOfTimesWeEnterShouldRetry + 1
		return false
	}
	getNextDuration := func(operationResponse OCIOperationResponse) time.Duration {
		numberOfTimesWeEnterGetNextDuration = numberOfTimesWeEnterGetNextDuration + 1
		return 0
	}
	retryPolicy := NewRetryPolicy(totalNumberAttempts, shouldRetryOperation, getNextDuration)
	retryableRequest := retryableOCIRequest{
		retryPolicy: &retryPolicy,
	}
	// type OCIOperation func(context.Context, OCIRequest) (OCIResponse, error)
	fakeOperation := func(context.Context, OCIRequest) (OCIResponse, error) {
		return successResponse, nil
	}

	response, err := Retry(context.Background(), retryableRequest, fakeOperation, retryPolicy)
	assert.Equal(t, uint(1), numberOfTimesWeEnterShouldRetry)
	assert.Equal(t, uint(0), numberOfTimesWeEnterGetNextDuration)
	assert.Equal(t, response, successResponse)
	assert.Nil(t, err)
}

func TestRetry_RaisesDeadlineExceededException(t *testing.T) {
	errorResponse := genericOCIResponse{
		RawResponse: &http.Response{
			Header:     http.Header{},
			StatusCode: 400,
		},
	}
	totalNumberAttempts := uint(5)
	numberOfTimesWeEnterShouldRetry := uint(0)
	numberOfTimesWeEnterGetNextDuration := uint(0)
	shouldRetryOperation := func(operationResponse OCIOperationResponse) bool {
		numberOfTimesWeEnterShouldRetry = numberOfTimesWeEnterShouldRetry + 1
		return true
	}
	getNextDuration := func(operationResponse OCIOperationResponse) time.Duration {
		numberOfTimesWeEnterGetNextDuration = numberOfTimesWeEnterGetNextDuration + 1
		return 10 * time.Second
	}
	retryPolicy := NewRetryPolicy(totalNumberAttempts, shouldRetryOperation, getNextDuration)
	retryableRequest := retryableOCIRequest{
		retryPolicy: &retryPolicy,
	}
	// type OCIOperation func(context.Context, OCIRequest) (OCIResponse, error)
	fakeOperation := func(context.Context, OCIRequest) (OCIResponse, error) {
		return errorResponse, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := Retry(ctx, retryableRequest, fakeOperation, retryPolicy)
	assert.Equal(t, uint(1), numberOfTimesWeEnterShouldRetry)
	assert.Equal(t, uint(1), numberOfTimesWeEnterGetNextDuration)
	assert.Equal(t, response, errorResponse)
	assert.Equal(t, err, DeadlineExceededByBackoff)
}

func TestRetry_GetsSuccessfulResponseAfterMultipleAttempts(t *testing.T) {
	errorResponse := genericOCIResponse{
		RawResponse: &http.Response{
			Header:     http.Header{},
			StatusCode: 400,
		},
	}
	successResponse := genericOCIResponse{
		RawResponse: &http.Response{
			Header:     http.Header{},
			StatusCode: 200,
		},
	}
	totalNumberAttempts := uint(10)
	numberOfTimesWeEnterShouldRetry := uint(0)
	numberOfTimesWeEnterGetNextDuration := uint(0)
	shouldRetryOperation := func(operationResponse OCIOperationResponse) bool {
		numberOfTimesWeEnterShouldRetry = numberOfTimesWeEnterShouldRetry + 1
		return operationResponse.Response.HTTPResponse().StatusCode == 400
	}
	getNextDuration := func(operationResponse OCIOperationResponse) time.Duration {
		numberOfTimesWeEnterGetNextDuration = numberOfTimesWeEnterGetNextDuration + 1
		return 0 * time.Second
	}
	retryPolicy := NewRetryPolicy(totalNumberAttempts, shouldRetryOperation, getNextDuration)
	retryableRequest := retryableOCIRequest{
		retryPolicy: &retryPolicy,
	}
	// type OCIOperation func(context.Context, OCIRequest) (OCIResponse, error)
	fakeOperation := func(context.Context, OCIRequest) (OCIResponse, error) {
		if numberOfTimesWeEnterShouldRetry < 7 {
			return errorResponse, nil
		}
		return successResponse, nil
	}

	response, err := Retry(context.Background(), retryableRequest, fakeOperation, retryPolicy)
	assert.Equal(t, uint(8), numberOfTimesWeEnterShouldRetry)
	assert.Equal(t, uint(7), numberOfTimesWeEnterGetNextDuration)
	assert.Equal(t, response, successResponse)
	assert.Nil(t, err)
}

func TestRetryToken_GenerateMultipleTimes(t *testing.T) {
	token1 := generateRetryToken()
	token2 := generateRetryToken()
	assert.NotEqual(t, token1, token2)
}

func TestBaseClient_CreateWithInvalidConfig(t *testing.T) {
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
region=us-ashburn-1
`

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	configurationProvider, _ := ConfigurationProviderFromFile(tmpConfFile, "")

	_, err := NewClientWithConfig(configurationProvider)
	assert.Error(t, err)
}

func TestBaseClient_CreateWithConfig(t *testing.T) {
	dataTpl := `[DEFAULT]
tenancy=sometenancy
user=someuser
fingerprint=somefingerprint
key_file=%s
region=us-ashburn-1
`

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	configurationProvider, errConf := ConfigurationProviderFromFile(tmpConfFile, "")
	assert.NoError(t, errConf)

	client, err := NewClientWithConfig(configurationProvider)
	assert.NotNil(t, client)
	assert.NoError(t, err)
}

func TestBaseClient_CreateWithBadRegion(t *testing.T) {
	dataTpl := `[DEFAULT]
tenancy=sometenancy
user=someuser
fingerprint=somefingerprint
key_file=%s
region=noregion
`

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	configurationProvider, errConf := ConfigurationProviderFromFile(tmpConfFile, "")
	assert.NoError(t, errConf)

	_, err := NewClientWithConfig(configurationProvider)
	assert.NoError(t, err)
}

func TestBaseClient_CreateWithoutRegion(t *testing.T) {
	dataTpl := `[DEFAULT]
tenancy=sometenancy
user=someuser
fingerprint=somefingerprint
key_file=%s
`

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	configurationProvider, errConf := ConfigurationProviderFromFile(tmpConfFile, "")
	assert.NoError(t, errConf)

	_, err := NewClientWithConfig(configurationProvider)
	assert.Error(t, err)
}

func TestHomeDir(t *testing.T) {
	h := getHomeFolder()
	_, e := os.Stat(h)
	assert.NoError(t, e)
}
