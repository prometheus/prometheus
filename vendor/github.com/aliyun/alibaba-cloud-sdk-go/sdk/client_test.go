/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sdk

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials/provider"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"

	"github.com/stretchr/testify/assert"
)

type signertest struct {
	name string
}

func (s *signertest) GetName() string {
	return ""
}

func (s *signertest) GetType() string {
	return ""
}

func (s *signertest) GetVersion() string {
	return ""
}

func (s *signertest) GetAccessKeyId() (string, error) {
	return "", nil
}

func (s *signertest) GetExtraParam() map[string]string {
	return nil
}

func (s *signertest) Sign(stringToSign, secretSuffix string) string {
	return ""
}

func Test_Client(t *testing.T) {
	defer func() {
		err := recover()
		assert.NotNil(t, err)
		assert.Equal(t, "not support yet", err)
	}()
	NewClient()
}

func Test_NewClientWithOptions(t *testing.T) {
	c := NewConfig()
	c.HttpTransport = &http.Transport{
		IdleConnTimeout: time.Duration(10 * time.Second),
	}
	c.EnableAsync = true
	c.GoRoutinePoolSize = 1
	c.MaxTaskQueueSize = 1
	c.Timeout = 10 * time.Second
	credential := credentials.NewAccessKeyCredential("acesskeyid", "accesskeysecret")
	client, err := NewClientWithOptions("regionid", c, credential)
	assert.Nil(t, err)
	assert.NotNil(t, client)
}

func Test_NewClientWithPolicy(t *testing.T) {
	client, err := NewClientWithRamRoleArnAndPolicy("regionid", "acesskeyid", "accesskeysecret", "roleArn", "sessionName", "policy")
	assert.Nil(t, err)
	assert.NotNil(t, client)
}

func Test_NewClientWithAccessKey(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
}

func Test_NewClientWithStsToken(t *testing.T) {
	client, err := NewClientWithStsToken("regionid", "acesskeyid", "accesskeysecret", "token")
	assert.Nil(t, err)
	assert.NotNil(t, client)
}

func Test_NewClientWithRamRoleArn(t *testing.T) {
	client, err := NewClientWithRamRoleArn("regionid", "acesskeyid", "accesskeysecret", "roleArn", "roleSessionName")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	config := client.InitClientConfig()
	assert.NotNil(t, config)
}

func Test_NewClientWithEcsRamRole(t *testing.T) {
	client, err := NewClientWithEcsRamRole("regionid", "roleName")
	assert.Nil(t, err)
	assert.NotNil(t, client)
}

func Test_NewClientWithRsaKeyPair(t *testing.T) {
	client, err := NewClientWithRsaKeyPair("regionid", "publicKey", "privateKey", 3600)
	assert.Nil(t, err)
	assert.NotNil(t, client)
}

func mockResponse(statusCode int, content string, mockerr error) (res *http.Response, err error) {
	status := strconv.Itoa(statusCode)
	res = &http.Response{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		Header:     make(http.Header),
		StatusCode: statusCode,
		Status:     status + " " + http.StatusText(statusCode),
	}
	res.Body = ioutil.NopCloser(bytes.NewReader([]byte(content)))
	err = mockerr
	return
}

func Test_DoActionWithProxy(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)
	request := requests.NewCommonRequest()
	request.Domain = "ecs.aliyuncs.com"
	request.Version = "2014-05-26"
	request.ApiName = "DescribeInstanceStatus"

	request.QueryParams["PageNumber"] = "1"
	request.QueryParams["PageSize"] = "30"
	request.TransToAcsRequest()
	response := responses.NewCommonResponse()
	origTestHookDo := hookDo
	defer func() { hookDo = origTestHookDo }()
	hookDo = func(fn func(req *http.Request) (*http.Response, error)) func(req *http.Request) (*http.Response, error) {
		return func(req *http.Request) (*http.Response, error) {
			return mockResponse(200, "", nil)
		}
	}
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	assert.Equal(t, 200, response.GetHttpStatus())
	assert.Equal(t, "", response.GetHttpContentString())

	// Test when scheme is http, only http proxy is valid.
	envHttpsProxy := os.Getenv("https_proxy")
	os.Setenv("https_proxy", "https://127.0.0.1:9000")
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans, _ := client.httpClient.Transport.(*http.Transport)
	assert.Nil(t, trans.Proxy)

	// Test when host is in no_proxy, proxy is invalid
	envNoProxy := os.Getenv("no_proxy")
	os.Setenv("no_proxy", "ecs.aliyuncs.com")
	envHttpProxy := os.Getenv("http_proxy")
	os.Setenv("http_proxy", "http://127.0.0.1:8888")
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans, _ = client.httpClient.Transport.(*http.Transport)
	assert.Nil(t, trans.Proxy)

	client.SetNoProxy("ecs.testaliyuncs.com")
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans, _ = client.httpClient.Transport.(*http.Transport)
	url, _ := trans.Proxy(nil)
	assert.Equal(t, url.Scheme, "http")
	assert.Equal(t, url.Host, "127.0.0.1:8888")

	// Test when setting http proxy, client has a high priority than environment variable
	client.SetHttpProxy("http://127.0.0.1:8080")
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans, _ = client.httpClient.Transport.(*http.Transport)
	url, _ = trans.Proxy(nil)
	assert.Equal(t, url.Scheme, "http")
	assert.Equal(t, url.Host, "127.0.0.1:8080")

	// Test when scheme is https, only https proxy is valid
	request.Scheme = "https"
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans, _ = client.httpClient.Transport.(*http.Transport)
	url, _ = trans.Proxy(nil)
	assert.Equal(t, url.Scheme, "https")
	assert.Equal(t, url.Host, "127.0.0.1:9000")

	// Test when setting https proxy, client has a high priority than environment variable
	client.SetHttpsProxy("https://username:password@127.0.0.1:6666")
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans, _ = client.httpClient.Transport.(*http.Transport)
	url, _ = trans.Proxy(nil)
	assert.Equal(t, url.Scheme, "https")
	assert.Equal(t, url.Host, "127.0.0.1:6666")
	assert.Equal(t, url.User.Username(), "username")

	client.Shutdown()
	os.Setenv("https_proxy", envHttpsProxy)
	os.Setenv("http_proxy", envHttpProxy)
	os.Setenv("no_proxy", envNoProxy)
	assert.Equal(t, false, client.isRunning)
}

func Test_DoAction_HTTPSInsecure(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)

	client.SetHTTPSInsecure(true)
	request := requests.NewCommonRequest()
	request.Product = "Ram"
	request.Version = "2015-05-01"
	request.ApiName = "CreateRole"
	request.Domain = "ecs.aliyuncs.com"
	request.QueryParams["RegionId"] = os.Getenv("REGION_ID")
	request.TransToAcsRequest()
	response := responses.NewCommonResponse()
	origTestHookDo := hookDo
	defer func() { hookDo = origTestHookDo }()
	hookDo = func(fn func(req *http.Request) (*http.Response, error)) func(req *http.Request) (*http.Response, error) {
		return func(req *http.Request) (*http.Response, error) {
			return mockResponse(200, "", nil)
		}
	}
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	assert.Equal(t, 200, response.GetHttpStatus())
	assert.Equal(t, "", response.GetHttpContentString())
	trans := client.httpClient.Transport.(*http.Transport)
	assert.Equal(t, true, trans.TLSClientConfig.InsecureSkipVerify)

	request.SetHTTPSInsecure(false)
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans = client.httpClient.Transport.(*http.Transport)
	assert.Equal(t, false, trans.TLSClientConfig.InsecureSkipVerify)

	// Test when scheme is http, only http proxy is valid.
	envHttpsProxy := os.Getenv("HTTPS_PROXY")
	os.Setenv("HTTPS_PROXY", "https://127.0.0.1:9000")
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans, _ = client.httpClient.Transport.(*http.Transport)
	assert.Nil(t, trans.Proxy)

	// Test when host is in no_proxy, proxy is invalid
	envNoProxy := os.Getenv("NO_PROXY")
	os.Setenv("NO_PROXY", "ecs.aliyuncs.com")
	envHttpProxy := os.Getenv("HTTP_PROXY")
	os.Setenv("HTTP_PROXY", "http://127.0.0.1:8888")
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans, _ = client.httpClient.Transport.(*http.Transport)
	assert.Nil(t, trans.Proxy)

	client.SetNoProxy("ecs.testaliyuncs.com")
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans, _ = client.httpClient.Transport.(*http.Transport)
	url, _ := trans.Proxy(nil)
	assert.Equal(t, url.Scheme, "http")
	assert.Equal(t, url.Host, "127.0.0.1:8888")

	// Test when setting http proxy, client has a high priority than environment variable
	client.SetHttpProxy("http://127.0.0.1:8080")
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans, _ = client.httpClient.Transport.(*http.Transport)
	url, _ = trans.Proxy(nil)
	assert.Equal(t, url.Scheme, "http")
	assert.Equal(t, url.Host, "127.0.0.1:8080")

	// Test when scheme is https, only https proxy is valid
	request.Scheme = "https"
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans, _ = client.httpClient.Transport.(*http.Transport)
	url, _ = trans.Proxy(nil)
	assert.Equal(t, url.Scheme, "https")
	assert.Equal(t, url.Host, "127.0.0.1:9000")

	// Test when setting https proxy, client has a high priority than environment variable
	client.SetHttpsProxy("https://127.0.0.1:6666")
	err = client.DoAction(request, response)
	assert.Nil(t, err)
	trans, _ = client.httpClient.Transport.(*http.Transport)
	url, _ = trans.Proxy(nil)
	assert.Equal(t, url.Scheme, "https")
	assert.Equal(t, url.Host, "127.0.0.1:6666")

	client.Shutdown()
	os.Setenv("HTTPS_PROXY", envHttpsProxy)
	os.Setenv("HTTP_PROXY", envHttpProxy)
	os.Setenv("NO_PROXY", envNoProxy)
	assert.Equal(t, false, client.isRunning)
}

func Test_DoAction_Timeout(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)
	request := requests.NewCommonRequest()
	request.Domain = "ecs.aliyuncs.com"
	request.Version = "2014-05-26"
	request.Product = "ecs"
	request.ApiName = "DescribeInstanceStatus"

	request.QueryParams["PageNumber"] = "1"
	request.QueryParams["PageSize"] = "30"
	request.TransToAcsRequest()
	response := responses.NewCommonResponse()
	origTestHookDo := hookDo
	defer func() { hookDo = origTestHookDo }()
	hookDo = func(fn func(req *http.Request) (*http.Response, error)) func(req *http.Request) (*http.Response, error) {
		return func(req *http.Request) (*http.Response, error) {
			return mockResponse(400, "Server Internel Error", fmt.Errorf("read tcp"))
		}
	}
	err = client.DoAction(request, response)
	assert.NotNil(t, err)
	assert.Equal(t, 0, response.GetHttpStatus())
	assert.Equal(t, "", response.GetHttpContentString())

	client.httpClient.Timeout = 15 * time.Second
	err = client.DoAction(request, response)
	assert.NotNil(t, err)
	assert.Equal(t, 0, response.GetHttpStatus())
	assert.Equal(t, "", response.GetHttpContentString())

	// Test set client timeout
	client.SetReadTimeout(1 * time.Millisecond)
	client.SetConnectTimeout(1 * time.Millisecond)
	assert.Equal(t, 1*time.Millisecond, client.GetConnectTimeout())
	assert.Equal(t, 1*time.Millisecond, client.GetReadTimeout())
	client.config.AutoRetry = false
	err = client.DoAction(request, response)
	assert.NotNil(t, err)
	assert.Equal(t, 0, response.GetHttpStatus())
	assert.Equal(t, "", response.GetHttpContentString())

	// Test set request timeout
	request.SetReadTimeout(1 * time.Millisecond)
	request.SetConnectTimeout(1 * time.Millisecond)
	err = client.DoAction(request, response)
	assert.NotNil(t, err)
	assert.Equal(t, 0, response.GetHttpStatus())
	assert.Equal(t, "", response.GetHttpContentString())

	client.Shutdown()
	assert.Equal(t, false, client.isRunning)
}

func Test_ProcessCommonRequest(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)

	request := requests.NewCommonRequest()
	request.Domain = "ecs.aliyuncs.com"
	request.Version = "2014-05-26"
	request.ApiName = "DescribeInstanceStatus"

	request.QueryParams["PageNumber"] = "1"
	request.QueryParams["PageSize"] = "30"

	origTestHookDo := hookDo
	defer func() { hookDo = origTestHookDo }()
	hookDo = func(fn func(req *http.Request) (*http.Response, error)) func(req *http.Request) (*http.Response, error) {
		return func(req *http.Request) (*http.Response, error) {
			return mockResponse(400, "", fmt.Errorf("test error"))
		}
	}
	resp, err := client.ProcessCommonRequest(request)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "test error")
	assert.Equal(t, 0, resp.GetHttpStatus())
	assert.Equal(t, "", resp.GetHttpContentString())
}

func mockServer(status int, json string) (server *httptest.Server) {
	// Start a test server locally.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(json))
		return
	}))
	return ts
}

func Test_DoAction_With500(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)
	request := requests.NewCommonRequest()
	request.Version = "2014-05-26"
	request.ApiName = "DescribeInstanceStatus"

	request.QueryParams["PageNumber"] = "1"
	request.QueryParams["PageSize"] = "30"
	request.TransToAcsRequest()
	response := responses.NewCommonResponse()
	ts := mockServer(500, "Server Internel Error")
	defer ts.Close()
	domain := strings.Replace(ts.URL, "http://", "", 1)
	request.Domain = domain
	err = client.DoAction(request, response)
	assert.NotNil(t, err)
	assert.Equal(t, 500, response.GetHttpStatus())
	assert.Equal(t, "Server Internel Error", response.GetHttpContentString())
}

func Test_DoAction_WithLogger(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)
	request := requests.NewCommonRequest()
	request.Version = "2014-05-26"
	request.ApiName = "DescribeInstanceStatus"
	request.TransToAcsRequest()
	response := responses.NewCommonResponse()
	ts := mockServer(500, "Server Internel Error")
	defer ts.Close()
	domain := strings.Replace(ts.URL, "http://", "", 1)
	request.Domain = domain
	f1, err := os.Create("test.txt")
	defer os.Remove("test.txt")
	assert.Nil(t, err)

	// Test when set logger, it will create a new client logger.
	client.SetLogger("error", "Alibaba", f1, "")
	err = client.DoAction(request, response)
	assert.NotNil(t, err)
	log := client.GetLogger()
	assert.Contains(t, client.GetLoggerMsg(), "Alibaba: \"GET /?AccessKeyId=acesskeyid&Action=DescribeInstanceStatus&Format=JSON&RegionId=regionid")
	assert.Equal(t, 500, response.GetHttpStatus())
	assert.Equal(t, true, log.isOpen)
	assert.Equal(t, "Server Internel Error", response.GetHttpContentString())

	// Test when close logger, it will not print log.
	client.CloseLogger()
	err = client.DoAction(request, response)
	assert.NotNil(t, err)
	log = client.GetLogger()
	assert.Equal(t, 500, response.GetHttpStatus())
	assert.Equal(t, false, log.isOpen)
	assert.Equal(t, "{time} {channel}: \"{method} {uri} HTTP/{version}\" {code} {cost} {hostname}", client.GetTemplate())
	assert.Contains(t, client.GetLoggerMsg(), `GET /?AccessKeyId=acesskeyid&Action=DescribeInstanceStatus&Format=JSON&RegionId=regionid`)
	assert.Equal(t, "Server Internel Error", response.GetHttpContentString())

	// Test when open logger, it will print log.
	client.OpenLogger()
	template := "{channel}: \"{method} {code} {res_body}"
	client.SetTemplate(template)
	err = client.DoAction(request, response)
	assert.NotNil(t, err)
	log = client.GetLogger()
	assert.Equal(t, 500, response.GetHttpStatus())
	assert.Equal(t, true, log.isOpen)
	assert.Equal(t, "{channel}: \"{method} {code} {res_body}", client.GetTemplate())
	assert.Equal(t, client.GetLoggerMsg(), `Alibaba: "GET 500 Server Internel Error`)
}

func TestClient_BuildRequestWithSigner(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)
	request := requests.NewCommonRequest()
	request.Domain = "ecs.aliyuncs.com"
	request.Version = "2014-05-26"
	request.ApiName = "DescribeInstanceStatus"

	request.QueryParams["PageNumber"] = "1"
	request.QueryParams["PageSize"] = "30"
	request.RegionId = "regionid"
	request.TransToAcsRequest()
	client.config.UserAgent = "user_agent"
	err = client.BuildRequestWithSigner(request, nil)
	assert.Nil(t, err)
}

func TestClient_BuildRequestWithSigner1(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)
	request := requests.NewCommonRequest()
	request.Version = "2014-05-26"
	request.ApiName = "DescribeInstanceStatus"

	request.QueryParams["PageNumber"] = "1"
	request.QueryParams["PageSize"] = "30"
	request.RegionId = "regionid"
	request.TransToAcsRequest()
	signer := &signertest{
		name: "signer",
	}
	err = client.BuildRequestWithSigner(request, signer)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "SDK.CanNotResolveEndpoint] Can not resolve endpoint")
}

func TestClient_BuildRequestWithSigner2(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)
	request := requests.NewCommonRequest()
	request.Version = "2014-05-26"
	request.ApiName = "DescribeInstanceStatus"

	request.QueryParams["PageNumber"] = "1"
	request.QueryParams["PageSize"] = "30"
	request.RegionId = "regionid"
	request.Product = "Ecs"
	request.TransToAcsRequest()
	signer := &signertest{
		name: "signer",
	}

	//Test: regional rule
	client.EndpointType = "regional"
	httprequest, err := client.buildRequestWithSigner(request, signer)
	assert.Nil(t, err)
	assert.Equal(t, "ecs.regionid.aliyuncs.com", httprequest.URL.Host)

	//Test: exceptional rule
	request.Domain = ""
	client.EndpointMap = map[string]string{
		"regionid": "ecs.test.com",
	}
	httprequest, err = client.buildRequestWithSigner(request, signer)
	assert.Nil(t, err)
	assert.Equal(t, "ecs.test.com", httprequest.URL.Host)

	//Test: no valid exceptional rule
	request.Domain = ""
	client.EndpointMap = map[string]string{
		"regiontest": "ecs.test.com",
	}
	httprequest, err = client.buildRequestWithSigner(request, signer)
	assert.Nil(t, err)
	assert.Equal(t, "ecs.regionid.aliyuncs.com", httprequest.URL.Host)

	//Test: center rule
	request.Domain = ""
	client.EndpointType = "centeral"
	client.Network = "share"
	httprequest, err = client.buildRequestWithSigner(request, signer)
	assert.Nil(t, err)
	assert.Equal(t, "ecs-share.aliyuncs.com", httprequest.URL.Host)

	client.SetEndpointRules(client.EndpointMap, "regional", "vpc")
	assert.Equal(t, "regional", client.EndpointType)
	assert.Equal(t, "vpc", client.Network)
	assert.Equal(t, map[string]string{"regiontest": "ecs.test.com"}, client.EndpointMap)
}

func TestClient_ProcessCommonRequestWithSigner(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)
	request := requests.NewCommonRequest()
	request.Domain = "ecs.aliyuncs.com"
	request.Version = "2014-05-26"
	request.ApiName = "DescribeInstanceStatus"

	request.QueryParams["PageNumber"] = "1"
	request.QueryParams["PageSize"] = "30"
	request.RegionId = "regionid"
	signer := &signertest{
		name: "signer",
	}
	origTestHookDo := hookDo
	defer func() { hookDo = origTestHookDo }()
	hookDo = func(fn func(req *http.Request) (*http.Response, error)) func(req *http.Request) (*http.Response, error) {
		return func(req *http.Request) (*http.Response, error) {
			return mockResponse(500, "Server Internel Error", fmt.Errorf("test error"))
		}
	}
	resp, err := client.ProcessCommonRequestWithSigner(request, signer)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "test error")
	assert.Equal(t, resp.GetHttpContentString(), "")
}

func TestClient_AppendUserAgent(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)
	request := requests.NewCommonRequest()
	request.Domain = "ecs.aliyuncs.com"
	request.Version = "2014-05-26"
	request.ApiName = "DescribeInstanceStatus"

	request.RegionId = "regionid"
	signer := &signertest{
		name: "signer",
	}
	request.TransToAcsRequest()
	httpRequest, err := client.buildRequestWithSigner(request, signer)
	assert.Nil(t, err)
	assert.Equal(t, DefaultUserAgent, httpRequest.Header.Get("User-Agent"))

	// Test set client useragent.
	client.AppendUserAgent("test", "1.01")
	httpRequest, err = client.buildRequestWithSigner(request, signer)
	assert.Equal(t, DefaultUserAgent+" test/1.01", httpRequest.Header.Get("User-Agent"))

	// Test set request useragent. And request useragent has a higner priority than client's.
	request.AppendUserAgent("test", "2.01")
	httpRequest, err = client.buildRequestWithSigner(request, signer)
	assert.Equal(t, DefaultUserAgent+" test/2.01", httpRequest.Header.Get("User-Agent"))

	client.AppendUserAgent("test", "2.02")
	httpRequest, err = client.buildRequestWithSigner(request, signer)
	assert.Equal(t, DefaultUserAgent+" test/2.01", httpRequest.Header.Get("User-Agent"))

	// Test update request useragent.
	request.AppendUserAgent("test", "2.02")
	httpRequest, err = client.buildRequestWithSigner(request, signer)
	assert.Equal(t, DefaultUserAgent+" test/2.02", httpRequest.Header.Get("User-Agent"))

	// Test client can't modify DefaultUserAgent.
	client.AppendUserAgent("core", "1.01")
	httpRequest, err = client.buildRequestWithSigner(request, signer)
	assert.Equal(t, DefaultUserAgent+" test/2.02", httpRequest.Header.Get("User-Agent"))

	// Test request can't modify DefaultUserAgent.
	request.AppendUserAgent("core", "1.01")
	httpRequest, err = client.buildRequestWithSigner(request, signer)
	assert.Equal(t, DefaultUserAgent+" test/2.02", httpRequest.Header.Get("User-Agent"))

	request1 := requests.NewCommonRequest()
	request1.Domain = "ecs.aliyuncs.com"
	request1.Version = "2014-05-26"
	request1.ApiName = "DescribeRegions"
	request1.RegionId = "regionid"
	request1.AppendUserAgent("sys", "1.01")
	request1.TransToAcsRequest()
	httpRequest, err = client.buildRequestWithSigner(request1, signer)
	assert.Nil(t, err)
	assert.Equal(t, DefaultUserAgent+" test/2.02 sys/1.01", httpRequest.Header.Get("User-Agent"))
}

func TestClient_ProcessCommonRequestWithSigner_Error(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)
	request := requests.NewCommonRequest()
	request.Domain = "ecs.aliyuncs.com"
	request.Version = "2014-05-26"
	request.ApiName = "DescribeInstanceStatus"

	request.QueryParams["PageNumber"] = "1"
	request.QueryParams["PageSize"] = "30"
	request.RegionId = "regionid"
	origTestHookDo := hookDo
	defer func() {
		hookDo = origTestHookDo
		err := recover()
		assert.NotNil(t, err)
	}()
	hookDo = func(fn func(req *http.Request) (*http.Response, error)) func(req *http.Request) (*http.Response, error) {
		return func(req *http.Request) (*http.Response, error) {
			return mockResponse(500, "Server Internel Error", fmt.Errorf("test error"))
		}
	}
	resp, err := client.ProcessCommonRequestWithSigner(request, nil)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "test error")
	assert.Equal(t, resp.GetHttpContentString(), "Server Internel Error")
}

func TestClient_NewClientWithStsRoleNameOnEcs(t *testing.T) {
	client, err := NewClientWithStsRoleNameOnEcs("regionid", "rolename")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)
	config := client.GetConfig()
	assert.NotNil(t, config)
	err = client.AddAsyncTask(nil)
	assert.NotNil(t, err)
}

func TestClient_NewClientWithStsRoleArn(t *testing.T) {
	client, err := NewClientWithStsRoleArn("regionid", "acesskeyid", "accesskeysecret", "rolearn", "rolesessionname")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)
	task := func() {}
	client.asyncTaskQueue = make(chan func(), 1)
	err = client.AddAsyncTask(task)
	assert.Nil(t, err)
	client.Shutdown()
	assert.Equal(t, false, client.isRunning)
}

func TestInitWithProviderChain(t *testing.T) {

	//testcase1: No any environment variable
	c, err := NewClientWithProvider("cn-hangzhou")
	assert.Empty(t, c)
	assert.EqualError(t, err, "No credential found")

	//testcase2: AK
	os.Setenv(provider.ENVAccessKeyID, "AccessKeyId")
	os.Setenv(provider.ENVAccessKeySecret, "AccessKeySecret")

	c, err = NewClientWithProvider("cn-hangzhou")
	assert.Nil(t, err)
	expC, err := NewClientWithAccessKey("cn-hangzhou", "AccessKeyId", "AccessKeySecret")
	assert.Nil(t, err)
	assert.Equal(t, expC, c)

	//testcase3:AK value is ""
	os.Setenv(provider.ENVAccessKeyID, "")
	os.Setenv(provider.ENVAccessKeySecret, "bbbb")
	c, err = NewClientWithProvider("cn-hangzhou")
	assert.EqualError(t, err, "Environmental variable (ALIBABACLOUD_ACCESS_KEY_ID or ALIBABACLOUD_ACCESS_KEY_SECRET) is empty")
	assert.Empty(t, c)

	//testcase4: Profile value is ""
	os.Unsetenv(provider.ENVAccessKeyID)
	os.Unsetenv(provider.ENVAccessKeySecret)
	os.Setenv(provider.ENVCredentialFile, "")
	c, err = NewClientWithProvider("cn-hangzhou")
	assert.Empty(t, c)
	assert.EqualError(t, err, "Environment variable 'ALIBABA_CLOUD_CREDENTIALS_FILE' cannot be empty")

	//testcase5: Profile
	os.Setenv(provider.ENVCredentialFile, "./profile")
	c, err = NewClientWithProvider("cn-hangzhou")
	assert.Empty(t, c)
	assert.NotNil(t, err)
	//testcase6:Instances
	os.Unsetenv(provider.ENVCredentialFile)
	os.Setenv(provider.ENVEcsMetadata, "")
	c, err = NewClientWithProvider("cn-hangzhou")
	assert.Empty(t, c)
	assert.EqualError(t, err, "Environmental variable 'ALIBABA_CLOUD_ECS_METADATA' are empty")

	//testcase7: Custom Providers
	c, err = NewClientWithProvider("cn-hangzhou", provider.ProviderProfile, provider.ProviderEnv)
	assert.Empty(t, c)
	assert.EqualError(t, err, "No credential found")

}

func TestNewClientWithBearerToken(t *testing.T) {
	client, err := NewClientWithBearerToken("cn-hangzhou", "Bearer.Token")
	assert.Nil(t, err)
	assert.NotNil(t, client)
}
