// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

// Package common provides supporting functions and structs used by service packages
package common

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/user"
	"path"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// DefaultHostURLTemplate The default url template for service hosts
	DefaultHostURLTemplate = "%s.%s.oraclecloud.com"

	// requestHeaderAccept The key for passing a header to indicate Accept
	requestHeaderAccept = "Accept"

	// requestHeaderAuthorization The key for passing a header to indicate Authorization
	requestHeaderAuthorization = "Authorization"

	// requestHeaderContentLength The key for passing a header to indicate Content Length
	requestHeaderContentLength = "Content-Length"

	// requestHeaderContentType The key for passing a header to indicate Content Type
	requestHeaderContentType = "Content-Type"

	// requestHeaderDate The key for passing a header to indicate Date
	requestHeaderDate = "Date"

	// requestHeaderIfMatch The key for passing a header to indicate If Match
	requestHeaderIfMatch = "if-match"

	// requestHeaderOpcClientInfo The key for passing a header to indicate OPC Client Info
	requestHeaderOpcClientInfo = "opc-client-info"

	// requestHeaderOpcRetryToken The key for passing a header to indicate OPC Retry Token
	requestHeaderOpcRetryToken = "opc-retry-token"

	// requestHeaderOpcRequestID The key for unique Oracle-assigned identifier for the request.
	requestHeaderOpcRequestID = "opc-request-id"

	// requestHeaderOpcClientRequestID The key for unique Oracle-assigned identifier for the request.
	requestHeaderOpcClientRequestID = "opc-client-request-id"

	// requestHeaderUserAgent The key for passing a header to indicate User Agent
	requestHeaderUserAgent = "User-Agent"

	// requestHeaderXContentSHA256 The key for passing a header to indicate SHA256 hash
	requestHeaderXContentSHA256 = "X-Content-SHA256"

	// private constants
	defaultScheme            = "https"
	defaultSDKMarker         = "Oracle-GoSDK"
	defaultUserAgentTemplate = "%s/%s (%s/%s; go/%s)" //SDK/SDKVersion (OS/OSVersion; Lang/LangVersion)
	defaultTimeout           = 60 * time.Second
	defaultConfigFileName    = "config"
	defaultConfigDirName     = ".oci"
	secondaryConfigDirName   = ".oraclebmc"
	maxBodyLenForDebug       = 1024 * 1000
)

// RequestInterceptor function used to customize the request before calling the underlying service
type RequestInterceptor func(*http.Request) error

// HTTPRequestDispatcher wraps the execution of a http request, it is generally implemented by
// http.Client.Do, but can be customized for testing
type HTTPRequestDispatcher interface {
	Do(req *http.Request) (*http.Response, error)
}

// BaseClient struct implements all basic operations to call oci web services.
type BaseClient struct {
	//HTTPClient performs the http network operations
	HTTPClient HTTPRequestDispatcher

	//Signer performs auth operation
	Signer HTTPRequestSigner

	//A request interceptor can be used to customize the request before signing and dispatching
	Interceptor RequestInterceptor

	//The host of the service
	Host string

	//The user agent
	UserAgent string

	//Base path for all operations of this client
	BasePath string
}

func defaultUserAgent() string {
	userAgent := fmt.Sprintf(defaultUserAgentTemplate, defaultSDKMarker, Version(), runtime.GOOS, runtime.GOARCH, runtime.Version())
	return userAgent
}

var clientCounter int64

func getNextSeed() int64 {
	newCounterValue := atomic.AddInt64(&clientCounter, 1)
	return newCounterValue + time.Now().UnixNano()
}

func newBaseClient(signer HTTPRequestSigner, dispatcher HTTPRequestDispatcher) BaseClient {
	rand.Seed(getNextSeed())
	return BaseClient{
		UserAgent:   defaultUserAgent(),
		Interceptor: nil,
		Signer:      signer,
		HTTPClient:  dispatcher,
	}
}

func defaultHTTPDispatcher() http.Client {
	httpClient := http.Client{
		Timeout: defaultTimeout,
	}
	return httpClient
}

func defaultBaseClient(provider KeyProvider) BaseClient {
	dispatcher := defaultHTTPDispatcher()
	signer := DefaultRequestSigner(provider)
	return newBaseClient(signer, &dispatcher)
}

//DefaultBaseClientWithSigner creates a default base client with a given signer
func DefaultBaseClientWithSigner(signer HTTPRequestSigner) BaseClient {
	dispatcher := defaultHTTPDispatcher()
	return newBaseClient(signer, &dispatcher)
}

// NewClientWithConfig Create a new client with a configuration provider, the configuration provider
// will be used for the default signer as well as reading the region
// This function does not check for valid regions to implement forward compatibility
func NewClientWithConfig(configProvider ConfigurationProvider) (client BaseClient, err error) {
	var ok bool
	if ok, err = IsConfigurationProviderValid(configProvider); !ok {
		err = fmt.Errorf("can not create client, bad configuration: %s", err.Error())
		return
	}

	client = defaultBaseClient(configProvider)
	return
}

func getHomeFolder() string {
	current, e := user.Current()
	if e != nil {
		//Give up and try to return something sensible
		home := os.Getenv("HOME")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home
	}
	return current.HomeDir
}

// DefaultConfigProvider returns the default config provider. The default config provider
// will look for configurations in 3 places: file in $HOME/.oci/config, HOME/.obmcs/config and
// variables names starting with the string TF_VAR. If the same configuration is found in multiple
// places the provider will prefer the first one.
func DefaultConfigProvider() ConfigurationProvider {
	homeFolder := getHomeFolder()
	defaultConfigFile := path.Join(homeFolder, defaultConfigDirName, defaultConfigFileName)
	secondaryConfigFile := path.Join(homeFolder, secondaryConfigDirName, defaultConfigFileName)

	defaultFileProvider, _ := ConfigurationProviderFromFile(defaultConfigFile, "")
	secondaryFileProvider, _ := ConfigurationProviderFromFile(secondaryConfigFile, "")
	environmentProvider := environmentConfigurationProvider{EnvironmentVariablePrefix: "TF_VAR"}

	provider, _ := ComposingConfigurationProvider([]ConfigurationProvider{defaultFileProvider, secondaryFileProvider, environmentProvider})
	Debugf("Configuration provided by: %s", provider)
	return provider
}

func (client *BaseClient) prepareRequest(request *http.Request) (err error) {
	if client.UserAgent == "" {
		return fmt.Errorf("user agent can not be blank")
	}

	if request.Header == nil {
		request.Header = http.Header{}
	}
	request.Header.Set(requestHeaderUserAgent, client.UserAgent)
	request.Header.Set(requestHeaderDate, time.Now().UTC().Format(http.TimeFormat))

	if request.Header.Get(requestHeaderOpcRetryToken) == "" {
		request.Header.Set(requestHeaderOpcRetryToken, generateRetryToken())
	}

	if !strings.Contains(client.Host, "http") &&
		!strings.Contains(client.Host, "https") {
		client.Host = fmt.Sprintf("%s://%s", defaultScheme, client.Host)
	}

	clientURL, err := url.Parse(client.Host)
	if err != nil {
		return fmt.Errorf("host is invalid. %s", err.Error())
	}
	request.URL.Host = clientURL.Host
	request.URL.Scheme = clientURL.Scheme
	currentPath := request.URL.Path
	if !strings.Contains(currentPath, fmt.Sprintf("/%s", client.BasePath)) {
		request.URL.Path = path.Clean(fmt.Sprintf("/%s/%s", client.BasePath, currentPath))
	}
	return
}

func (client BaseClient) intercept(request *http.Request) (err error) {
	if client.Interceptor != nil {
		err = client.Interceptor(request)
	}
	return
}

func checkForSuccessfulResponse(res *http.Response) error {
	familyStatusCode := res.StatusCode / 100
	if familyStatusCode == 4 || familyStatusCode == 5 {
		return newServiceFailureFromResponse(res)
	}
	return nil
}

// OCIRequest is any request made to an OCI service.
type OCIRequest interface {
	// HTTPRequest assembles an HTTP request.
	HTTPRequest(method, path string) (http.Request, error)
}

// RequestMetadata is metadata about an OCIRequest. This structure represents the behavior exhibited by the SDK when
// issuing (or reissuing) a request.
type RequestMetadata struct {
	// RetryPolicy is the policy for reissuing the request. If no retry policy is set on the request,
	// then the request will be issued exactly once.
	RetryPolicy *RetryPolicy
}

// OCIResponse is the response from issuing a request to an OCI service.
type OCIResponse interface {
	// HTTPResponse returns the raw HTTP response.
	HTTPResponse() *http.Response
}

// OCIOperation is the generalization of a request-response cycle undergone by an OCI service.
type OCIOperation func(context.Context, OCIRequest) (OCIResponse, error)

// Call executes the http request with the given context
func (client BaseClient) Call(ctx context.Context, request *http.Request) (response *http.Response, err error) {
	Debugln("Atempting to call downstream service")
	request = request.WithContext(ctx)

	err = client.prepareRequest(request)
	if err != nil {
		return
	}

	//Intercept
	err = client.intercept(request)
	if err != nil {
		return
	}

	//Sign the request
	err = client.Signer.Sign(request)
	if err != nil {
		return
	}

	IfDebug(func() {
		dumpBody := true
		if request.ContentLength > maxBodyLenForDebug {
			Logln("not dumping body too big")
			dumpBody = false
		}
		if dump, e := httputil.DumpRequest(request, dumpBody); e == nil {
			Logf("Dump Request %v", string(dump))
		} else {
			Debugln(e)
		}
	})

	//Execute the http request
	response, err = client.HTTPClient.Do(request)

	IfDebug(func() {
		if err != nil {
			Logln(err)
			return
		}

		dumpBody := true
		if response.ContentLength > maxBodyLenForDebug {
			Logln("not dumping body too big")
			dumpBody = false
		}

		if dump, e := httputil.DumpResponse(response, dumpBody); e == nil {
			Logf("Dump Response %v", string(dump))
		} else {
			Debugln(e)
		}
	})

	if err != nil {
		return
	}

	err = checkForSuccessfulResponse(response)
	return
}

//CloseBodyIfValid closes the body of an http response if the response and the body are valid
func CloseBodyIfValid(httpResponse *http.Response) {
	if httpResponse != nil && httpResponse.Body != nil {
		httpResponse.Body.Close()
	}
}
