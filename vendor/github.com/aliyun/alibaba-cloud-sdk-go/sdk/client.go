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
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials/provider"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/endpoints"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/utils"
)

var debug utils.Debug

func init() {
	debug = utils.Init("sdk")
}

// Version this value will be replaced while build: -ldflags="-X sdk.version=x.x.x"
var Version = "0.0.1"
var defaultConnectTimeout = 5 * time.Second
var defaultReadTimeout = 10 * time.Second

var DefaultUserAgent = fmt.Sprintf("AlibabaCloud (%s; %s) Golang/%s Core/%s", runtime.GOOS, runtime.GOARCH, strings.Trim(runtime.Version(), "go"), Version)

var hookDo = func(fn func(req *http.Request) (*http.Response, error)) func(req *http.Request) (*http.Response, error) {
	return fn
}

// Client the type Client
type Client struct {
	isInsecure     bool
	regionId       string
	config         *Config
	httpProxy      string
	httpsProxy     string
	noProxy        string
	logger         *Logger
	userAgent      map[string]string
	signer         auth.Signer
	httpClient     *http.Client
	asyncTaskQueue chan func()
	readTimeout    time.Duration
	connectTimeout time.Duration
	EndpointMap    map[string]string
	EndpointType   string
	Network        string

	debug     bool
	isRunning bool
	// void "panic(write to close channel)" cause of addAsync() after Shutdown()
	asyncChanLock *sync.RWMutex
}

func (client *Client) Init() (err error) {
	panic("not support yet")
}

func (client *Client) SetEndpointRules(endpointMap map[string]string, endpointType string, netWork string) {
	client.EndpointMap = endpointMap
	client.Network = netWork
	client.EndpointType = endpointType
}

func (client *Client) SetHTTPSInsecure(isInsecure bool) {
	client.isInsecure = isInsecure
}

func (client *Client) GetHTTPSInsecure() bool {
	return client.isInsecure
}

func (client *Client) SetHttpsProxy(httpsProxy string) {
	client.httpsProxy = httpsProxy
}

func (client *Client) GetHttpsProxy() string {
	return client.httpsProxy
}

func (client *Client) SetHttpProxy(httpProxy string) {
	client.httpProxy = httpProxy
}

func (client *Client) GetHttpProxy() string {
	return client.httpProxy
}

func (client *Client) SetNoProxy(noProxy string) {
	client.noProxy = noProxy
}

func (client *Client) GetNoProxy() string {
	return client.noProxy
}

// InitWithProviderChain will get credential from the providerChain,
// the RsaKeyPairCredential Only applicable to regionID `ap-northeast-1`,
// if your providerChain may return a credential type with RsaKeyPairCredential,
// please ensure your regionID is `ap-northeast-1`.
func (client *Client) InitWithProviderChain(regionId string, provider provider.Provider) (err error) {
	config := client.InitClientConfig()
	credential, err := provider.Resolve()
	if err != nil {
		return
	}
	return client.InitWithOptions(regionId, config, credential)
}

func (client *Client) InitWithOptions(regionId string, config *Config, credential auth.Credential) (err error) {
	client.isRunning = true
	client.asyncChanLock = new(sync.RWMutex)
	client.regionId = regionId
	client.config = config
	client.httpClient = &http.Client{}

	if config.HttpTransport != nil {
		client.httpClient.Transport = config.HttpTransport
	}

	if config.Timeout > 0 {
		client.httpClient.Timeout = config.Timeout
	}

	if config.EnableAsync {
		client.EnableAsync(config.GoRoutinePoolSize, config.MaxTaskQueueSize)
	}

	client.signer, err = auth.NewSignerWithCredential(credential, client.ProcessCommonRequestWithSigner)

	return
}

func (client *Client) SetReadTimeout(readTimeout time.Duration) {
	client.readTimeout = readTimeout
}

func (client *Client) SetConnectTimeout(connectTimeout time.Duration) {
	client.connectTimeout = connectTimeout
}

func (client *Client) GetReadTimeout() time.Duration {
	return client.readTimeout
}

func (client *Client) GetConnectTimeout() time.Duration {
	return client.connectTimeout
}

func (client *Client) getHttpProxy(scheme string) (proxy *url.URL, err error) {
	if scheme == "https" {
		if client.GetHttpsProxy() != "" {
			proxy, err = url.Parse(client.httpsProxy)
		} else if rawurl := os.Getenv("HTTPS_PROXY"); rawurl != "" {
			proxy, err = url.Parse(rawurl)
		} else if rawurl := os.Getenv("https_proxy"); rawurl != "" {
			proxy, err = url.Parse(rawurl)
		}
	} else {
		if client.GetHttpProxy() != "" {
			proxy, err = url.Parse(client.httpProxy)
		} else if rawurl := os.Getenv("HTTP_PROXY"); rawurl != "" {
			proxy, err = url.Parse(rawurl)
		} else if rawurl := os.Getenv("http_proxy"); rawurl != "" {
			proxy, err = url.Parse(rawurl)
		}
	}

	return proxy, err
}

func (client *Client) getNoProxy(scheme string) []string {
	var urls []string
	if client.GetNoProxy() != "" {
		urls = strings.Split(client.noProxy, ",")
	} else if rawurl := os.Getenv("NO_PROXY"); rawurl != "" {
		urls = strings.Split(rawurl, ",")
	} else if rawurl := os.Getenv("no_proxy"); rawurl != "" {
		urls = strings.Split(rawurl, ",")
	}

	return urls
}

// EnableAsync enable the async task queue
func (client *Client) EnableAsync(routinePoolSize, maxTaskQueueSize int) {
	client.asyncTaskQueue = make(chan func(), maxTaskQueueSize)
	for i := 0; i < routinePoolSize; i++ {
		go func() {
			for client.isRunning {
				select {
				case task, notClosed := <-client.asyncTaskQueue:
					if notClosed {
						task()
					}
				}
			}
		}()
	}
}

func (client *Client) InitWithAccessKey(regionId, accessKeyId, accessKeySecret string) (err error) {
	config := client.InitClientConfig()
	credential := &credentials.BaseCredential{
		AccessKeyId:     accessKeyId,
		AccessKeySecret: accessKeySecret,
	}
	return client.InitWithOptions(regionId, config, credential)
}

func (client *Client) InitWithStsToken(regionId, accessKeyId, accessKeySecret, securityToken string) (err error) {
	config := client.InitClientConfig()
	credential := &credentials.StsTokenCredential{
		AccessKeyId:       accessKeyId,
		AccessKeySecret:   accessKeySecret,
		AccessKeyStsToken: securityToken,
	}
	return client.InitWithOptions(regionId, config, credential)
}

func (client *Client) InitWithRamRoleArn(regionId, accessKeyId, accessKeySecret, roleArn, roleSessionName string) (err error) {
	config := client.InitClientConfig()
	credential := &credentials.RamRoleArnCredential{
		AccessKeyId:     accessKeyId,
		AccessKeySecret: accessKeySecret,
		RoleArn:         roleArn,
		RoleSessionName: roleSessionName,
	}
	return client.InitWithOptions(regionId, config, credential)
}

func (client *Client) InitWithRamRoleArnAndPolicy(regionId, accessKeyId, accessKeySecret, roleArn, roleSessionName, policy string) (err error) {
	config := client.InitClientConfig()
	credential := &credentials.RamRoleArnCredential{
		AccessKeyId:     accessKeyId,
		AccessKeySecret: accessKeySecret,
		RoleArn:         roleArn,
		RoleSessionName: roleSessionName,
		Policy:          policy,
	}
	return client.InitWithOptions(regionId, config, credential)
}

func (client *Client) InitWithRsaKeyPair(regionId, publicKeyId, privateKey string, sessionExpiration int) (err error) {
	config := client.InitClientConfig()
	credential := &credentials.RsaKeyPairCredential{
		PrivateKey:        privateKey,
		PublicKeyId:       publicKeyId,
		SessionExpiration: sessionExpiration,
	}
	return client.InitWithOptions(regionId, config, credential)
}

func (client *Client) InitWithEcsRamRole(regionId, roleName string) (err error) {
	config := client.InitClientConfig()
	credential := &credentials.EcsRamRoleCredential{
		RoleName: roleName,
	}
	return client.InitWithOptions(regionId, config, credential)
}

func (client *Client) InitWithBearerToken(regionId, bearerToken string) (err error) {
	config := client.InitClientConfig()
	credential := &credentials.BearerTokenCredential{
		BearerToken: bearerToken,
	}
	return client.InitWithOptions(regionId, config, credential)
}

func (client *Client) InitClientConfig() (config *Config) {
	if client.config != nil {
		return client.config
	} else {
		return NewConfig()
	}
}

func (client *Client) DoAction(request requests.AcsRequest, response responses.AcsResponse) (err error) {
	return client.DoActionWithSigner(request, response, nil)
}

func (client *Client) GetEndpointRules(regionId string, product string) (endpointRaw string) {
	if client.EndpointType == "regional" {
		endpointRaw = strings.Replace("<product><network>.<region_id>.aliyuncs.com", "<region_id>", regionId, 1)
	} else {
		endpointRaw = "<product><network>.aliyuncs.com"
	}
	endpointRaw = strings.Replace(endpointRaw, "<product>", strings.ToLower(product), 1)
	if client.Network == "" || client.Network == "public" {
		endpointRaw = strings.Replace(endpointRaw, "<network>", "", 1)
	} else {
		endpointRaw = strings.Replace(endpointRaw, "<network>", "-"+client.Network, 1)
	}
	return endpointRaw
}

func (client *Client) buildRequestWithSigner(request requests.AcsRequest, signer auth.Signer) (httpRequest *http.Request, err error) {
	// add clientVersion
	request.GetHeaders()["x-sdk-core-version"] = Version

	regionId := client.regionId
	if len(request.GetRegionId()) > 0 {
		regionId = request.GetRegionId()
	}

	// resolve endpoint
	endpoint := request.GetDomain()
	if endpoint == "" && client.EndpointType != "" && request.GetProduct() != "Sts" {
		if client.EndpointMap != nil && client.Network == "" || client.Network == "public" {
			endpoint = client.EndpointMap[regionId]
		}

		if endpoint == "" {
			endpoint = client.GetEndpointRules(regionId, request.GetProduct())
		}
	}

	if endpoint == "" {
		resolveParam := &endpoints.ResolveParam{
			Domain:               request.GetDomain(),
			Product:              request.GetProduct(),
			RegionId:             regionId,
			LocationProduct:      request.GetLocationServiceCode(),
			LocationEndpointType: request.GetLocationEndpointType(),
			CommonApi:            client.ProcessCommonRequest,
		}
		endpoint, err = endpoints.Resolve(resolveParam)
		if err != nil {
			return
		}
	}

	request.SetDomain(endpoint)
	if request.GetScheme() == "" {
		request.SetScheme(client.config.Scheme)
	}
	// init request params
	err = requests.InitParams(request)
	if err != nil {
		return
	}

	// signature
	var finalSigner auth.Signer
	if signer != nil {
		finalSigner = signer
	} else {
		finalSigner = client.signer
	}
	httpRequest, err = buildHttpRequest(request, finalSigner, regionId)
	if err == nil {
		userAgent := DefaultUserAgent + getSendUserAgent(client.config.UserAgent, client.userAgent, request.GetUserAgent())
		httpRequest.Header.Set("User-Agent", userAgent)
	}

	return
}

func getSendUserAgent(configUserAgent string, clientUserAgent, requestUserAgent map[string]string) string {
	realUserAgent := ""
	for key1, value1 := range clientUserAgent {
		for key2, _ := range requestUserAgent {
			if key1 == key2 {
				key1 = ""
			}
		}
		if key1 != "" {
			realUserAgent += fmt.Sprintf(" %s/%s", key1, value1)

		}
	}
	for key, value := range requestUserAgent {
		realUserAgent += fmt.Sprintf(" %s/%s", key, value)
	}
	if configUserAgent != "" {
		return realUserAgent + fmt.Sprintf(" Extra/%s", configUserAgent)
	}
	return realUserAgent
}

func (client *Client) AppendUserAgent(key, value string) {
	newkey := true

	if client.userAgent == nil {
		client.userAgent = make(map[string]string)
	}
	if strings.ToLower(key) != "core" && strings.ToLower(key) != "go" {
		for tag, _ := range client.userAgent {
			if tag == key {
				client.userAgent[tag] = value
				newkey = false
			}
		}
		if newkey {
			client.userAgent[key] = value
		}
	}
}

func (client *Client) BuildRequestWithSigner(request requests.AcsRequest, signer auth.Signer) (err error) {
	_, err = client.buildRequestWithSigner(request, signer)
	return
}

func (client *Client) getTimeout(request requests.AcsRequest) (time.Duration, time.Duration) {
	readTimeout := defaultReadTimeout
	connectTimeout := defaultConnectTimeout

	reqReadTimeout := request.GetReadTimeout()
	reqConnectTimeout := request.GetConnectTimeout()
	if reqReadTimeout != 0*time.Millisecond {
		readTimeout = reqReadTimeout
	} else if client.readTimeout != 0*time.Millisecond {
		readTimeout = client.readTimeout
	} else if client.httpClient.Timeout != 0 {
		readTimeout = client.httpClient.Timeout
	} else if timeout, ok := getAPIMaxTimeout(request.GetProduct(), request.GetActionName()); ok {
		readTimeout = timeout
	}

	if reqConnectTimeout != 0*time.Millisecond {
		connectTimeout = reqConnectTimeout
	} else if client.connectTimeout != 0*time.Millisecond {
		connectTimeout = client.connectTimeout
	}
	return readTimeout, connectTimeout
}

func Timeout(connectTimeout time.Duration) func(cxt context.Context, net, addr string) (c net.Conn, err error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{
			Timeout:   connectTimeout,
			DualStack: true,
		}).DialContext(ctx, network, address)
	}
}

func (client *Client) setTimeout(request requests.AcsRequest) {
	readTimeout, connectTimeout := client.getTimeout(request)
	client.httpClient.Timeout = readTimeout
	if trans, ok := client.httpClient.Transport.(*http.Transport); ok && trans != nil {
		trans.DialContext = Timeout(connectTimeout)
		client.httpClient.Transport = trans
	} else {
		client.httpClient.Transport = &http.Transport{
			DialContext: Timeout(connectTimeout),
		}
	}
}

func (client *Client) getHTTPSInsecure(request requests.AcsRequest) (insecure bool) {
	if request.GetHTTPSInsecure() != nil {
		insecure = *request.GetHTTPSInsecure()
	} else {
		insecure = client.GetHTTPSInsecure()
	}
	return insecure
}

func (client *Client) DoActionWithSigner(request requests.AcsRequest, response responses.AcsResponse, signer auth.Signer) (err error) {

	fieldMap := make(map[string]string)
	initLogMsg(fieldMap)
	defer func() {
		client.printLog(fieldMap, err)
	}()
	httpRequest, err := client.buildRequestWithSigner(request, signer)
	if err != nil {
		return
	}

	client.setTimeout(request)
	proxy, err := client.getHttpProxy(httpRequest.URL.Scheme)
	if err != nil {
		return err
	}

	noProxy := client.getNoProxy(httpRequest.URL.Scheme)

	var flag bool
	for _, value := range noProxy {
		if value == httpRequest.Host {
			flag = true
			break
		}
	}

	// Set whether to ignore certificate validation.
	// Default InsecureSkipVerify is false.
	if trans, ok := client.httpClient.Transport.(*http.Transport); ok && trans != nil {
		trans.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: client.getHTTPSInsecure(request),
		}
		if proxy != nil && !flag {
			trans.Proxy = http.ProxyURL(proxy)
		}
		client.httpClient.Transport = trans
	}

	var httpResponse *http.Response
	for retryTimes := 0; retryTimes <= client.config.MaxRetryTime; retryTimes++ {
		if proxy != nil && proxy.User != nil {
			if password, passwordSet := proxy.User.Password(); passwordSet {
				httpRequest.SetBasicAuth(proxy.User.Username(), password)
			}
		}
		if retryTimes > 0 {
			client.printLog(fieldMap, err)
			initLogMsg(fieldMap)
		}
		putMsgToMap(fieldMap, httpRequest)
		debug("> %s %s %s", httpRequest.Method, httpRequest.URL.RequestURI(), httpRequest.Proto)
		debug("> Host: %s", httpRequest.Host)
		for key, value := range httpRequest.Header {
			debug("> %s: %v", key, strings.Join(value, ""))
		}
		debug(">")
		debug(" Retry Times: %d.", retryTimes)

		startTime := time.Now()
		fieldMap["{start_time}"] = startTime.Format("2006-01-02 15:04:05")
		httpResponse, err = hookDo(client.httpClient.Do)(httpRequest)
		fieldMap["{cost}"] = time.Now().Sub(startTime).String()
		if err == nil {
			fieldMap["{code}"] = strconv.Itoa(httpResponse.StatusCode)
			fieldMap["{res_headers}"] = TransToString(httpResponse.Header)
			debug("< %s %s", httpResponse.Proto, httpResponse.Status)
			for key, value := range httpResponse.Header {
				debug("< %s: %v", key, strings.Join(value, ""))
			}
		}
		debug("<")
		// receive error
		if err != nil {
			debug(" Error: %s.", err.Error())
			if !client.config.AutoRetry {
				return
			} else if retryTimes >= client.config.MaxRetryTime {
				// timeout but reached the max retry times, return
				times := strconv.Itoa(retryTimes + 1)
				timeoutErrorMsg := fmt.Sprintf(errors.TimeoutErrorMessage, times, times)
				if strings.Contains(err.Error(), "Client.Timeout") {
					timeoutErrorMsg += " Read timeout. Please set a valid ReadTimeout."
				} else {
					timeoutErrorMsg += " Connect timeout. Please set a valid ConnectTimeout."
				}
				err = errors.NewClientError(errors.TimeoutErrorCode, timeoutErrorMsg, err)
				return
			}
		}
		//  if status code >= 500 or timeout, will trigger retry
		if client.config.AutoRetry && (err != nil || isServerError(httpResponse)) {
			client.setTimeout(request)
			// rewrite signatureNonce and signature
			httpRequest, err = client.buildRequestWithSigner(request, signer)
			// buildHttpRequest(request, finalSigner, regionId)
			if err != nil {
				return
			}
			continue
		}
		break
	}

	err = responses.Unmarshal(response, httpResponse, request.GetAcceptFormat())
	fieldMap["{res_body}"] = response.GetHttpContentString()
	debug("%s", response.GetHttpContentString())
	// wrap server errors
	if serverErr, ok := err.(*errors.ServerError); ok {
		var wrapInfo = map[string]string{}
		wrapInfo["StringToSign"] = request.GetStringToSign()
		err = errors.WrapServerError(serverErr, wrapInfo)
	}
	return
}

func putMsgToMap(fieldMap map[string]string, request *http.Request) {
	fieldMap["{host}"] = request.Host
	fieldMap["{method}"] = request.Method
	fieldMap["{uri}"] = request.URL.RequestURI()
	fieldMap["{pid}"] = strconv.Itoa(os.Getpid())
	fieldMap["{version}"] = strings.Split(request.Proto, "/")[1]
	hostname, _ := os.Hostname()
	fieldMap["{hostname}"] = hostname
	fieldMap["{req_headers}"] = TransToString(request.Header)
	fieldMap["{target}"] = request.URL.Path + request.URL.RawQuery
}

func buildHttpRequest(request requests.AcsRequest, singer auth.Signer, regionId string) (httpRequest *http.Request, err error) {
	err = auth.Sign(request, singer, regionId)
	if err != nil {
		return
	}
	requestMethod := request.GetMethod()
	requestUrl := request.BuildUrl()
	body := request.GetBodyReader()
	httpRequest, err = http.NewRequest(requestMethod, requestUrl, body)
	if err != nil {
		return
	}
	for key, value := range request.GetHeaders() {
		httpRequest.Header[key] = []string{value}
	}
	// host is a special case
	if host, containsHost := request.GetHeaders()["Host"]; containsHost {
		httpRequest.Host = host
	}
	return
}

func isServerError(httpResponse *http.Response) bool {
	return httpResponse.StatusCode >= http.StatusInternalServerError
}

/**
only block when any one of the following occurs:
1. the asyncTaskQueue is full, increase the queue size to avoid this
2. Shutdown() in progressing, the client is being closed
**/
func (client *Client) AddAsyncTask(task func()) (err error) {
	if client.asyncTaskQueue != nil {
		client.asyncChanLock.RLock()
		defer client.asyncChanLock.RUnlock()
		if client.isRunning {
			client.asyncTaskQueue <- task
		}
	} else {
		err = errors.NewClientError(errors.AsyncFunctionNotEnabledCode, errors.AsyncFunctionNotEnabledMessage, nil)
	}
	return
}

func (client *Client) GetConfig() *Config {
	return client.config
}

func NewClient() (client *Client, err error) {
	client = &Client{}
	err = client.Init()
	return
}

func NewClientWithProvider(regionId string, providers ...provider.Provider) (client *Client, err error) {
	client = &Client{}
	var pc provider.Provider
	if len(providers) == 0 {
		pc = provider.DefaultChain
	} else {
		pc = provider.NewProviderChain(providers)
	}
	err = client.InitWithProviderChain(regionId, pc)
	return
}

func NewClientWithOptions(regionId string, config *Config, credential auth.Credential) (client *Client, err error) {
	client = &Client{}
	err = client.InitWithOptions(regionId, config, credential)
	return
}

func NewClientWithAccessKey(regionId, accessKeyId, accessKeySecret string) (client *Client, err error) {
	client = &Client{}
	err = client.InitWithAccessKey(regionId, accessKeyId, accessKeySecret)
	return
}

func NewClientWithStsToken(regionId, stsAccessKeyId, stsAccessKeySecret, stsToken string) (client *Client, err error) {
	client = &Client{}
	err = client.InitWithStsToken(regionId, stsAccessKeyId, stsAccessKeySecret, stsToken)
	return
}

func NewClientWithRamRoleArn(regionId string, accessKeyId, accessKeySecret, roleArn, roleSessionName string) (client *Client, err error) {
	client = &Client{}
	err = client.InitWithRamRoleArn(regionId, accessKeyId, accessKeySecret, roleArn, roleSessionName)
	return
}

func NewClientWithRamRoleArnAndPolicy(regionId string, accessKeyId, accessKeySecret, roleArn, roleSessionName, policy string) (client *Client, err error) {
	client = &Client{}
	err = client.InitWithRamRoleArnAndPolicy(regionId, accessKeyId, accessKeySecret, roleArn, roleSessionName, policy)
	return
}

func NewClientWithEcsRamRole(regionId string, roleName string) (client *Client, err error) {
	client = &Client{}
	err = client.InitWithEcsRamRole(regionId, roleName)
	return
}

func NewClientWithRsaKeyPair(regionId string, publicKeyId, privateKey string, sessionExpiration int) (client *Client, err error) {
	client = &Client{}
	err = client.InitWithRsaKeyPair(regionId, publicKeyId, privateKey, sessionExpiration)
	return
}

func NewClientWithBearerToken(regionId, bearerToken string) (client *Client, err error) {
	client = &Client{}
	err = client.InitWithBearerToken(regionId, bearerToken)
	return
}

func (client *Client) ProcessCommonRequest(request *requests.CommonRequest) (response *responses.CommonResponse, err error) {
	request.TransToAcsRequest()
	response = responses.NewCommonResponse()
	err = client.DoAction(request, response)
	return
}

func (client *Client) ProcessCommonRequestWithSigner(request *requests.CommonRequest, signerInterface interface{}) (response *responses.CommonResponse, err error) {
	if signer, isSigner := signerInterface.(auth.Signer); isSigner {
		request.TransToAcsRequest()
		response = responses.NewCommonResponse()
		err = client.DoActionWithSigner(request, response, signer)
		return
	}
	panic("should not be here")
}

func (client *Client) Shutdown() {
	// lock the addAsync()
	client.asyncChanLock.Lock()
	defer client.asyncChanLock.Unlock()
	if client.asyncTaskQueue != nil {
		close(client.asyncTaskQueue)
	}
	client.isRunning = false
}

// Deprecated: Use NewClientWithRamRoleArn in this package instead.
func NewClientWithStsRoleArn(regionId string, accessKeyId, accessKeySecret, roleArn, roleSessionName string) (client *Client, err error) {
	return NewClientWithRamRoleArn(regionId, accessKeyId, accessKeySecret, roleArn, roleSessionName)
}

// Deprecated: Use NewClientWithEcsRamRole in this package instead.
func NewClientWithStsRoleNameOnEcs(regionId string, roleName string) (client *Client, err error) {
	return NewClientWithEcsRamRole(regionId, roleName)
}
