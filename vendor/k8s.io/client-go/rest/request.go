/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rest

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	utilclock "k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/watch"
	restclientwatch "k8s.io/client-go/rest/watch"
	"k8s.io/client-go/tools/metrics"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog/v2"
)

var (
	// longThrottleLatency defines threshold for logging requests. All requests being
	// throttled (via the provided rateLimiter) for more than longThrottleLatency will
	// be logged.
	longThrottleLatency = 50 * time.Millisecond

	// extraLongThrottleLatency defines the threshold for logging requests at log level 2.
	extraLongThrottleLatency = 1 * time.Second
)

// HTTPClient is an interface for testing a request object.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// ResponseWrapper is an interface for getting a response.
// The response may be either accessed as a raw data (the whole output is put into memory) or as a stream.
type ResponseWrapper interface {
	DoRaw(context.Context) ([]byte, error)
	Stream(context.Context) (io.ReadCloser, error)
}

// RequestConstructionError is returned when there's an error assembling a request.
type RequestConstructionError struct {
	Err error
}

// Error returns a textual description of 'r'.
func (r *RequestConstructionError) Error() string {
	return fmt.Sprintf("request construction error: '%v'", r.Err)
}

var noBackoff = &NoBackoff{}

// Request allows for building up a request to a server in a chained fashion.
// Any errors are stored until the end of your call, so you only have to
// check once.
type Request struct {
	c *RESTClient

	warningHandler WarningHandler

	rateLimiter flowcontrol.RateLimiter
	backoff     BackoffManager
	timeout     time.Duration
	maxRetries  int

	// generic components accessible via method setters
	verb       string
	pathPrefix string
	subpath    string
	params     url.Values
	headers    http.Header

	// structural elements of the request that are part of the Kubernetes API conventions
	namespace    string
	namespaceSet bool
	resource     string
	resourceName string
	subresource  string

	// output
	err  error
	body io.Reader
}

// NewRequest creates a new request helper object for accessing runtime.Objects on a server.
func NewRequest(c *RESTClient) *Request {
	var backoff BackoffManager
	if c.createBackoffMgr != nil {
		backoff = c.createBackoffMgr()
	}
	if backoff == nil {
		backoff = noBackoff
	}

	var pathPrefix string
	if c.base != nil {
		pathPrefix = path.Join("/", c.base.Path, c.versionedAPIPath)
	} else {
		pathPrefix = path.Join("/", c.versionedAPIPath)
	}

	var timeout time.Duration
	if c.Client != nil {
		timeout = c.Client.Timeout
	}

	r := &Request{
		c:              c,
		rateLimiter:    c.rateLimiter,
		backoff:        backoff,
		timeout:        timeout,
		pathPrefix:     pathPrefix,
		maxRetries:     10,
		warningHandler: c.warningHandler,
	}

	switch {
	case len(c.content.AcceptContentTypes) > 0:
		r.SetHeader("Accept", c.content.AcceptContentTypes)
	case len(c.content.ContentType) > 0:
		r.SetHeader("Accept", c.content.ContentType+", */*")
	}
	return r
}

// NewRequestWithClient creates a Request with an embedded RESTClient for use in test scenarios.
func NewRequestWithClient(base *url.URL, versionedAPIPath string, content ClientContentConfig, client *http.Client) *Request {
	return NewRequest(&RESTClient{
		base:             base,
		versionedAPIPath: versionedAPIPath,
		content:          content,
		Client:           client,
	})
}

// Verb sets the verb this request will use.
func (r *Request) Verb(verb string) *Request {
	r.verb = verb
	return r
}

// Prefix adds segments to the relative beginning to the request path. These
// items will be placed before the optional Namespace, Resource, or Name sections.
// Setting AbsPath will clear any previously set Prefix segments
func (r *Request) Prefix(segments ...string) *Request {
	if r.err != nil {
		return r
	}
	r.pathPrefix = path.Join(r.pathPrefix, path.Join(segments...))
	return r
}

// Suffix appends segments to the end of the path. These items will be placed after the prefix and optional
// Namespace, Resource, or Name sections.
func (r *Request) Suffix(segments ...string) *Request {
	if r.err != nil {
		return r
	}
	r.subpath = path.Join(r.subpath, path.Join(segments...))
	return r
}

// Resource sets the resource to access (<resource>/[ns/<namespace>/]<name>)
func (r *Request) Resource(resource string) *Request {
	if r.err != nil {
		return r
	}
	if len(r.resource) != 0 {
		r.err = fmt.Errorf("resource already set to %q, cannot change to %q", r.resource, resource)
		return r
	}
	if msgs := IsValidPathSegmentName(resource); len(msgs) != 0 {
		r.err = fmt.Errorf("invalid resource %q: %v", resource, msgs)
		return r
	}
	r.resource = resource
	return r
}

// BackOff sets the request's backoff manager to the one specified,
// or defaults to the stub implementation if nil is provided
func (r *Request) BackOff(manager BackoffManager) *Request {
	if manager == nil {
		r.backoff = &NoBackoff{}
		return r
	}

	r.backoff = manager
	return r
}

// WarningHandler sets the handler this client uses when warning headers are encountered.
// If set to nil, this client will use the default warning handler (see SetDefaultWarningHandler).
func (r *Request) WarningHandler(handler WarningHandler) *Request {
	r.warningHandler = handler
	return r
}

// Throttle receives a rate-limiter and sets or replaces an existing request limiter
func (r *Request) Throttle(limiter flowcontrol.RateLimiter) *Request {
	r.rateLimiter = limiter
	return r
}

// SubResource sets a sub-resource path which can be multiple segments after the resource
// name but before the suffix.
func (r *Request) SubResource(subresources ...string) *Request {
	if r.err != nil {
		return r
	}
	subresource := path.Join(subresources...)
	if len(r.subresource) != 0 {
		r.err = fmt.Errorf("subresource already set to %q, cannot change to %q", r.resource, subresource)
		return r
	}
	for _, s := range subresources {
		if msgs := IsValidPathSegmentName(s); len(msgs) != 0 {
			r.err = fmt.Errorf("invalid subresource %q: %v", s, msgs)
			return r
		}
	}
	r.subresource = subresource
	return r
}

// Name sets the name of a resource to access (<resource>/[ns/<namespace>/]<name>)
func (r *Request) Name(resourceName string) *Request {
	if r.err != nil {
		return r
	}
	if len(resourceName) == 0 {
		r.err = fmt.Errorf("resource name may not be empty")
		return r
	}
	if len(r.resourceName) != 0 {
		r.err = fmt.Errorf("resource name already set to %q, cannot change to %q", r.resourceName, resourceName)
		return r
	}
	if msgs := IsValidPathSegmentName(resourceName); len(msgs) != 0 {
		r.err = fmt.Errorf("invalid resource name %q: %v", resourceName, msgs)
		return r
	}
	r.resourceName = resourceName
	return r
}

// Namespace applies the namespace scope to a request (<resource>/[ns/<namespace>/]<name>)
func (r *Request) Namespace(namespace string) *Request {
	if r.err != nil {
		return r
	}
	if r.namespaceSet {
		r.err = fmt.Errorf("namespace already set to %q, cannot change to %q", r.namespace, namespace)
		return r
	}
	if msgs := IsValidPathSegmentName(namespace); len(msgs) != 0 {
		r.err = fmt.Errorf("invalid namespace %q: %v", namespace, msgs)
		return r
	}
	r.namespaceSet = true
	r.namespace = namespace
	return r
}

// NamespaceIfScoped is a convenience function to set a namespace if scoped is true
func (r *Request) NamespaceIfScoped(namespace string, scoped bool) *Request {
	if scoped {
		return r.Namespace(namespace)
	}
	return r
}

// AbsPath overwrites an existing path with the segments provided. Trailing slashes are preserved
// when a single segment is passed.
func (r *Request) AbsPath(segments ...string) *Request {
	if r.err != nil {
		return r
	}
	r.pathPrefix = path.Join(r.c.base.Path, path.Join(segments...))
	if len(segments) == 1 && (len(r.c.base.Path) > 1 || len(segments[0]) > 1) && strings.HasSuffix(segments[0], "/") {
		// preserve any trailing slashes for legacy behavior
		r.pathPrefix += "/"
	}
	return r
}

// RequestURI overwrites existing path and parameters with the value of the provided server relative
// URI.
func (r *Request) RequestURI(uri string) *Request {
	if r.err != nil {
		return r
	}
	locator, err := url.Parse(uri)
	if err != nil {
		r.err = err
		return r
	}
	r.pathPrefix = locator.Path
	if len(locator.Query()) > 0 {
		if r.params == nil {
			r.params = make(url.Values)
		}
		for k, v := range locator.Query() {
			r.params[k] = v
		}
	}
	return r
}

// Param creates a query parameter with the given string value.
func (r *Request) Param(paramName, s string) *Request {
	if r.err != nil {
		return r
	}
	return r.setParam(paramName, s)
}

// VersionedParams will take the provided object, serialize it to a map[string][]string using the
// implicit RESTClient API version and the default parameter codec, and then add those as parameters
// to the request. Use this to provide versioned query parameters from client libraries.
// VersionedParams will not write query parameters that have omitempty set and are empty. If a
// parameter has already been set it is appended to (Params and VersionedParams are additive).
func (r *Request) VersionedParams(obj runtime.Object, codec runtime.ParameterCodec) *Request {
	return r.SpecificallyVersionedParams(obj, codec, r.c.content.GroupVersion)
}

func (r *Request) SpecificallyVersionedParams(obj runtime.Object, codec runtime.ParameterCodec, version schema.GroupVersion) *Request {
	if r.err != nil {
		return r
	}
	params, err := codec.EncodeParameters(obj, version)
	if err != nil {
		r.err = err
		return r
	}
	for k, v := range params {
		if r.params == nil {
			r.params = make(url.Values)
		}
		r.params[k] = append(r.params[k], v...)
	}
	return r
}

func (r *Request) setParam(paramName, value string) *Request {
	if r.params == nil {
		r.params = make(url.Values)
	}
	r.params[paramName] = append(r.params[paramName], value)
	return r
}

func (r *Request) SetHeader(key string, values ...string) *Request {
	if r.headers == nil {
		r.headers = http.Header{}
	}
	r.headers.Del(key)
	for _, value := range values {
		r.headers.Add(key, value)
	}
	return r
}

// Timeout makes the request use the given duration as an overall timeout for the
// request. Additionally, if set passes the value as "timeout" parameter in URL.
func (r *Request) Timeout(d time.Duration) *Request {
	if r.err != nil {
		return r
	}
	r.timeout = d
	return r
}

// MaxRetries makes the request use the given integer as a ceiling of retrying upon receiving
// "Retry-After" headers and 429 status-code in the response. The default is 10 unless this
// function is specifically called with a different value.
// A zero maxRetries prevent it from doing retires and return an error immediately.
func (r *Request) MaxRetries(maxRetries int) *Request {
	if maxRetries < 0 {
		maxRetries = 0
	}
	r.maxRetries = maxRetries
	return r
}

// Body makes the request use obj as the body. Optional.
// If obj is a string, try to read a file of that name.
// If obj is a []byte, send it directly.
// If obj is an io.Reader, use it directly.
// If obj is a runtime.Object, marshal it correctly, and set Content-Type header.
// If obj is a runtime.Object and nil, do nothing.
// Otherwise, set an error.
func (r *Request) Body(obj interface{}) *Request {
	if r.err != nil {
		return r
	}
	switch t := obj.(type) {
	case string:
		data, err := ioutil.ReadFile(t)
		if err != nil {
			r.err = err
			return r
		}
		glogBody("Request Body", data)
		r.body = bytes.NewReader(data)
	case []byte:
		glogBody("Request Body", t)
		r.body = bytes.NewReader(t)
	case io.Reader:
		r.body = t
	case runtime.Object:
		// callers may pass typed interface pointers, therefore we must check nil with reflection
		if reflect.ValueOf(t).IsNil() {
			return r
		}
		encoder, err := r.c.content.Negotiator.Encoder(r.c.content.ContentType, nil)
		if err != nil {
			r.err = err
			return r
		}
		data, err := runtime.Encode(encoder, t)
		if err != nil {
			r.err = err
			return r
		}
		glogBody("Request Body", data)
		r.body = bytes.NewReader(data)
		r.SetHeader("Content-Type", r.c.content.ContentType)
	default:
		r.err = fmt.Errorf("unknown type used for body: %+v", obj)
	}
	return r
}

// URL returns the current working URL.
func (r *Request) URL() *url.URL {
	p := r.pathPrefix
	if r.namespaceSet && len(r.namespace) > 0 {
		p = path.Join(p, "namespaces", r.namespace)
	}
	if len(r.resource) != 0 {
		p = path.Join(p, strings.ToLower(r.resource))
	}
	// Join trims trailing slashes, so preserve r.pathPrefix's trailing slash for backwards compatibility if nothing was changed
	if len(r.resourceName) != 0 || len(r.subpath) != 0 || len(r.subresource) != 0 {
		p = path.Join(p, r.resourceName, r.subresource, r.subpath)
	}

	finalURL := &url.URL{}
	if r.c.base != nil {
		*finalURL = *r.c.base
	}
	finalURL.Path = p

	query := url.Values{}
	for key, values := range r.params {
		for _, value := range values {
			query.Add(key, value)
		}
	}

	// timeout is handled specially here.
	if r.timeout != 0 {
		query.Set("timeout", r.timeout.String())
	}
	finalURL.RawQuery = query.Encode()
	return finalURL
}

// finalURLTemplate is similar to URL(), but will make all specific parameter values equal
// - instead of name or namespace, "{name}" and "{namespace}" will be used, and all query
// parameters will be reset. This creates a copy of the url so as not to change the
// underlying object.
func (r Request) finalURLTemplate() url.URL {
	newParams := url.Values{}
	v := []string{"{value}"}
	for k := range r.params {
		newParams[k] = v
	}
	r.params = newParams
	url := r.URL()
	segments := strings.Split(r.URL().Path, "/")
	groupIndex := 0
	index := 0
	if r.URL() != nil && r.c.base != nil && strings.Contains(r.URL().Path, r.c.base.Path) {
		groupIndex += len(strings.Split(r.c.base.Path, "/"))
	}
	if groupIndex >= len(segments) {
		return *url
	}

	const CoreGroupPrefix = "api"
	const NamedGroupPrefix = "apis"
	isCoreGroup := segments[groupIndex] == CoreGroupPrefix
	isNamedGroup := segments[groupIndex] == NamedGroupPrefix
	if isCoreGroup {
		// checking the case of core group with /api/v1/... format
		index = groupIndex + 2
	} else if isNamedGroup {
		// checking the case of named group with /apis/apps/v1/... format
		index = groupIndex + 3
	} else {
		// this should not happen that the only two possibilities are /api... and /apis..., just want to put an
		// outlet here in case more API groups are added in future if ever possible:
		// https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-groups
		// if a wrong API groups name is encountered, return the {prefix} for url.Path
		url.Path = "/{prefix}"
		url.RawQuery = ""
		return *url
	}
	//switch segLength := len(segments) - index; segLength {
	switch {
	// case len(segments) - index == 1:
	// resource (with no name) do nothing
	case len(segments)-index == 2:
		// /$RESOURCE/$NAME: replace $NAME with {name}
		segments[index+1] = "{name}"
	case len(segments)-index == 3:
		if segments[index+2] == "finalize" || segments[index+2] == "status" {
			// /$RESOURCE/$NAME/$SUBRESOURCE: replace $NAME with {name}
			segments[index+1] = "{name}"
		} else {
			// /namespace/$NAMESPACE/$RESOURCE: replace $NAMESPACE with {namespace}
			segments[index+1] = "{namespace}"
		}
	case len(segments)-index >= 4:
		segments[index+1] = "{namespace}"
		// /namespace/$NAMESPACE/$RESOURCE/$NAME: replace $NAMESPACE with {namespace},  $NAME with {name}
		if segments[index+3] != "finalize" && segments[index+3] != "status" {
			// /$RESOURCE/$NAME/$SUBRESOURCE: replace $NAME with {name}
			segments[index+3] = "{name}"
		}
	}
	url.Path = path.Join(segments...)
	return *url
}

func (r *Request) tryThrottle(ctx context.Context) error {
	if r.rateLimiter == nil {
		return nil
	}

	now := time.Now()

	err := r.rateLimiter.Wait(ctx)

	latency := time.Since(now)
	if latency > longThrottleLatency {
		klog.V(3).Infof("Throttling request took %v, request: %s:%s", latency, r.verb, r.URL().String())
	}
	if latency > extraLongThrottleLatency {
		// If the rate limiter latency is very high, the log message should be printed at a higher log level,
		// but we use a throttled logger to prevent spamming.
		globalThrottledLogger.Infof("Throttling request took %v, request: %s:%s", latency, r.verb, r.URL().String())
	}
	metrics.RateLimiterLatency.Observe(r.verb, r.finalURLTemplate(), latency)

	return err
}

type throttleSettings struct {
	logLevel       klog.Level
	minLogInterval time.Duration

	lastLogTime time.Time
	lock        sync.RWMutex
}

type throttledLogger struct {
	clock    utilclock.PassiveClock
	settings []*throttleSettings
}

var globalThrottledLogger = &throttledLogger{
	clock: utilclock.RealClock{},
	settings: []*throttleSettings{
		{
			logLevel:       2,
			minLogInterval: 1 * time.Second,
		}, {
			logLevel:       0,
			minLogInterval: 10 * time.Second,
		},
	},
}

func (b *throttledLogger) attemptToLog() (klog.Level, bool) {
	for _, setting := range b.settings {
		if bool(klog.V(setting.logLevel).Enabled()) {
			// Return early without write locking if possible.
			if func() bool {
				setting.lock.RLock()
				defer setting.lock.RUnlock()
				return b.clock.Since(setting.lastLogTime) >= setting.minLogInterval
			}() {
				setting.lock.Lock()
				defer setting.lock.Unlock()
				if b.clock.Since(setting.lastLogTime) >= setting.minLogInterval {
					setting.lastLogTime = b.clock.Now()
					return setting.logLevel, true
				}
			}
			return -1, false
		}
	}
	return -1, false
}

// Infof will write a log message at each logLevel specified by the reciever's throttleSettings
// as long as it hasn't written a log message more recently than minLogInterval.
func (b *throttledLogger) Infof(message string, args ...interface{}) {
	if logLevel, ok := b.attemptToLog(); ok {
		klog.V(logLevel).Infof(message, args...)
	}
}

// Watch attempts to begin watching the requested location.
// Returns a watch.Interface, or an error.
func (r *Request) Watch(ctx context.Context) (watch.Interface, error) {
	// We specifically don't want to rate limit watches, so we
	// don't use r.rateLimiter here.
	if r.err != nil {
		return nil, r.err
	}

	url := r.URL().String()
	req, err := http.NewRequest(r.verb, url, r.body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.Header = r.headers
	client := r.c.Client
	if client == nil {
		client = http.DefaultClient
	}
	r.backoff.Sleep(r.backoff.CalculateBackoff(r.URL()))
	resp, err := client.Do(req)
	updateURLMetrics(r, resp, err)
	if r.c.base != nil {
		if err != nil {
			r.backoff.UpdateBackoff(r.c.base, err, 0)
		} else {
			r.backoff.UpdateBackoff(r.c.base, err, resp.StatusCode)
		}
	}
	if err != nil {
		// The watch stream mechanism handles many common partial data errors, so closed
		// connections can be retried in many cases.
		if net.IsProbableEOF(err) || net.IsTimeout(err) {
			return watch.NewEmptyWatch(), nil
		}
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		if result := r.transformResponse(resp, req); result.err != nil {
			return nil, result.err
		}
		return nil, fmt.Errorf("for request %s, got status: %v", url, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		klog.V(4).Infof("Unexpected content type from the server: %q: %v", contentType, err)
	}
	objectDecoder, streamingSerializer, framer, err := r.c.content.Negotiator.StreamDecoder(mediaType, params)
	if err != nil {
		return nil, err
	}

	handleWarnings(resp.Header, r.warningHandler)

	frameReader := framer.NewFrameReader(resp.Body)
	watchEventDecoder := streaming.NewDecoder(frameReader, streamingSerializer)

	return watch.NewStreamWatcher(
		restclientwatch.NewDecoder(watchEventDecoder, objectDecoder),
		// use 500 to indicate that the cause of the error is unknown - other error codes
		// are more specific to HTTP interactions, and set a reason
		errors.NewClientErrorReporter(http.StatusInternalServerError, r.verb, "ClientWatchDecoding"),
	), nil
}

// updateURLMetrics is a convenience function for pushing metrics.
// It also handles corner cases for incomplete/invalid request data.
func updateURLMetrics(req *Request, resp *http.Response, err error) {
	url := "none"
	if req.c.base != nil {
		url = req.c.base.Host
	}

	// Errors can be arbitrary strings. Unbound label cardinality is not suitable for a metric
	// system so we just report them as `<error>`.
	if err != nil {
		metrics.RequestResult.Increment("<error>", req.verb, url)
	} else {
		//Metrics for failure codes
		metrics.RequestResult.Increment(strconv.Itoa(resp.StatusCode), req.verb, url)
	}
}

// Stream formats and executes the request, and offers streaming of the response.
// Returns io.ReadCloser which could be used for streaming of the response, or an error
// Any non-2xx http status code causes an error.  If we get a non-2xx code, we try to convert the body into an APIStatus object.
// If we can, we return that as an error.  Otherwise, we create an error that lists the http status and the content of the response.
func (r *Request) Stream(ctx context.Context) (io.ReadCloser, error) {
	if r.err != nil {
		return nil, r.err
	}

	if err := r.tryThrottle(ctx); err != nil {
		return nil, err
	}

	url := r.URL().String()
	req, err := http.NewRequest(r.verb, url, nil)
	if err != nil {
		return nil, err
	}
	if r.body != nil {
		req.Body = ioutil.NopCloser(r.body)
	}
	req = req.WithContext(ctx)
	req.Header = r.headers
	client := r.c.Client
	if client == nil {
		client = http.DefaultClient
	}
	r.backoff.Sleep(r.backoff.CalculateBackoff(r.URL()))
	resp, err := client.Do(req)
	updateURLMetrics(r, resp, err)
	if r.c.base != nil {
		if err != nil {
			r.backoff.UpdateBackoff(r.URL(), err, 0)
		} else {
			r.backoff.UpdateBackoff(r.URL(), err, resp.StatusCode)
		}
	}
	if err != nil {
		return nil, err
	}

	switch {
	case (resp.StatusCode >= 200) && (resp.StatusCode < 300):
		handleWarnings(resp.Header, r.warningHandler)
		return resp.Body, nil

	default:
		// ensure we close the body before returning the error
		defer resp.Body.Close()

		result := r.transformResponse(resp, req)
		err := result.Error()
		if err == nil {
			err = fmt.Errorf("%d while accessing %v: %s", result.statusCode, url, string(result.body))
		}
		return nil, err
	}
}

// requestPreflightCheck looks for common programmer errors on Request.
//
// We tackle here two programmer mistakes. The first one is to try to create
// something(POST) using an empty string as namespace with namespaceSet as
// true. If namespaceSet is true then namespace should also be defined. The
// second mistake is, when under the same circumstances, the programmer tries
// to GET, PUT or DELETE a named resource(resourceName != ""), again, if
// namespaceSet is true then namespace must not be empty.
func (r *Request) requestPreflightCheck() error {
	if !r.namespaceSet {
		return nil
	}
	if len(r.namespace) > 0 {
		return nil
	}

	switch r.verb {
	case "POST":
		return fmt.Errorf("an empty namespace may not be set during creation")
	case "GET", "PUT", "DELETE":
		if len(r.resourceName) > 0 {
			return fmt.Errorf("an empty namespace may not be set when a resource name is provided")
		}
	}
	return nil
}

// request connects to the server and invokes the provided function when a server response is
// received. It handles retry behavior and up front validation of requests. It will invoke
// fn at most once. It will return an error if a problem occurred prior to connecting to the
// server - the provided function is responsible for handling server errors.
func (r *Request) request(ctx context.Context, fn func(*http.Request, *http.Response)) error {
	//Metrics for total request latency
	start := time.Now()
	defer func() {
		metrics.RequestLatency.Observe(r.verb, r.finalURLTemplate(), time.Since(start))
	}()

	if r.err != nil {
		klog.V(4).Infof("Error in request: %v", r.err)
		return r.err
	}

	if err := r.requestPreflightCheck(); err != nil {
		return err
	}

	client := r.c.Client
	if client == nil {
		client = http.DefaultClient
	}

	// Throttle the first try before setting up the timeout configured on the
	// client. We don't want a throttled client to return timeouts to callers
	// before it makes a single request.
	if err := r.tryThrottle(ctx); err != nil {
		return err
	}

	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	// Right now we make about ten retry attempts if we get a Retry-After response.
	retries := 0
	for {

		url := r.URL().String()
		req, err := http.NewRequest(r.verb, url, r.body)
		if err != nil {
			return err
		}
		req = req.WithContext(ctx)
		req.Header = r.headers

		r.backoff.Sleep(r.backoff.CalculateBackoff(r.URL()))
		if retries > 0 {
			// We are retrying the request that we already send to apiserver
			// at least once before.
			// This request should also be throttled with the client-internal rate limiter.
			if err := r.tryThrottle(ctx); err != nil {
				return err
			}
		}
		resp, err := client.Do(req)
		updateURLMetrics(r, resp, err)
		if err != nil {
			r.backoff.UpdateBackoff(r.URL(), err, 0)
		} else {
			r.backoff.UpdateBackoff(r.URL(), err, resp.StatusCode)
		}
		if err != nil {
			// "Connection reset by peer" or "apiserver is shutting down" are usually a transient errors.
			// Thus in case of "GET" operations, we simply retry it.
			// We are not automatically retrying "write" operations, as
			// they are not idempotent.
			if r.verb != "GET" {
				return err
			}
			// For connection errors and apiserver shutdown errors retry.
			if net.IsConnectionReset(err) || net.IsProbableEOF(err) {
				// For the purpose of retry, we set the artificial "retry-after" response.
				// TODO: Should we clean the original response if it exists?
				resp = &http.Response{
					StatusCode: http.StatusInternalServerError,
					Header:     http.Header{"Retry-After": []string{"1"}},
					Body:       ioutil.NopCloser(bytes.NewReader([]byte{})),
				}
			} else {
				return err
			}
		}

		done := func() bool {
			// Ensure the response body is fully read and closed
			// before we reconnect, so that we reuse the same TCP
			// connection.
			defer func() {
				const maxBodySlurpSize = 2 << 10
				if resp.ContentLength <= maxBodySlurpSize {
					io.Copy(ioutil.Discard, &io.LimitedReader{R: resp.Body, N: maxBodySlurpSize})
				}
				resp.Body.Close()
			}()

			retries++
			if seconds, wait := checkWait(resp); wait && retries <= r.maxRetries {
				if seeker, ok := r.body.(io.Seeker); ok && r.body != nil {
					_, err := seeker.Seek(0, 0)
					if err != nil {
						klog.V(4).Infof("Could not retry request, can't Seek() back to beginning of body for %T", r.body)
						fn(req, resp)
						return true
					}
				}

				klog.V(4).Infof("Got a Retry-After %ds response for attempt %d to %v", seconds, retries, url)
				r.backoff.Sleep(time.Duration(seconds) * time.Second)
				return false
			}
			fn(req, resp)
			return true
		}()
		if done {
			return nil
		}
	}
}

// Do formats and executes the request. Returns a Result object for easy response
// processing.
//
// Error type:
//  * If the server responds with a status: *errors.StatusError or *errors.UnexpectedObjectError
//  * http.Client.Do errors are returned directly.
func (r *Request) Do(ctx context.Context) Result {
	var result Result
	err := r.request(ctx, func(req *http.Request, resp *http.Response) {
		result = r.transformResponse(resp, req)
	})
	if err != nil {
		return Result{err: err}
	}
	return result
}

// DoRaw executes the request but does not process the response body.
func (r *Request) DoRaw(ctx context.Context) ([]byte, error) {
	var result Result
	err := r.request(ctx, func(req *http.Request, resp *http.Response) {
		result.body, result.err = ioutil.ReadAll(resp.Body)
		glogBody("Response Body", result.body)
		if resp.StatusCode < http.StatusOK || resp.StatusCode > http.StatusPartialContent {
			result.err = r.transformUnstructuredResponseError(resp, req, result.body)
		}
	})
	if err != nil {
		return nil, err
	}
	return result.body, result.err
}

// transformResponse converts an API response into a structured API object
func (r *Request) transformResponse(resp *http.Response, req *http.Request) Result {
	var body []byte
	if resp.Body != nil {
		data, err := ioutil.ReadAll(resp.Body)
		switch err.(type) {
		case nil:
			body = data
		case http2.StreamError:
			// This is trying to catch the scenario that the server may close the connection when sending the
			// response body. This can be caused by server timeout due to a slow network connection.
			// TODO: Add test for this. Steps may be:
			// 1. client-go (or kubectl) sends a GET request.
			// 2. Apiserver sends back the headers and then part of the body
			// 3. Apiserver closes connection.
			// 4. client-go should catch this and return an error.
			klog.V(2).Infof("Stream error %#v when reading response body, may be caused by closed connection.", err)
			streamErr := fmt.Errorf("stream error when reading response body, may be caused by closed connection. Please retry. Original error: %v", err)
			return Result{
				err: streamErr,
			}
		default:
			klog.Errorf("Unexpected error when reading response body: %v", err)
			unexpectedErr := fmt.Errorf("unexpected error when reading response body. Please retry. Original error: %v", err)
			return Result{
				err: unexpectedErr,
			}
		}
	}

	glogBody("Response Body", body)

	// verify the content type is accurate
	var decoder runtime.Decoder
	contentType := resp.Header.Get("Content-Type")
	if len(contentType) == 0 {
		contentType = r.c.content.ContentType
	}
	if len(contentType) > 0 {
		var err error
		mediaType, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return Result{err: errors.NewInternalError(err)}
		}
		decoder, err = r.c.content.Negotiator.Decoder(mediaType, params)
		if err != nil {
			// if we fail to negotiate a decoder, treat this as an unstructured error
			switch {
			case resp.StatusCode == http.StatusSwitchingProtocols:
				// no-op, we've been upgraded
			case resp.StatusCode < http.StatusOK || resp.StatusCode > http.StatusPartialContent:
				return Result{err: r.transformUnstructuredResponseError(resp, req, body)}
			}
			return Result{
				body:        body,
				contentType: contentType,
				statusCode:  resp.StatusCode,
				warnings:    handleWarnings(resp.Header, r.warningHandler),
			}
		}
	}

	switch {
	case resp.StatusCode == http.StatusSwitchingProtocols:
		// no-op, we've been upgraded
	case resp.StatusCode < http.StatusOK || resp.StatusCode > http.StatusPartialContent:
		// calculate an unstructured error from the response which the Result object may use if the caller
		// did not return a structured error.
		retryAfter, _ := retryAfterSeconds(resp)
		err := r.newUnstructuredResponseError(body, isTextResponse(resp), resp.StatusCode, req.Method, retryAfter)
		return Result{
			body:        body,
			contentType: contentType,
			statusCode:  resp.StatusCode,
			decoder:     decoder,
			err:         err,
			warnings:    handleWarnings(resp.Header, r.warningHandler),
		}
	}

	return Result{
		body:        body,
		contentType: contentType,
		statusCode:  resp.StatusCode,
		decoder:     decoder,
		warnings:    handleWarnings(resp.Header, r.warningHandler),
	}
}

// truncateBody decides if the body should be truncated, based on the glog Verbosity.
func truncateBody(body string) string {
	max := 0
	switch {
	case bool(klog.V(10).Enabled()):
		return body
	case bool(klog.V(9).Enabled()):
		max = 10240
	case bool(klog.V(8).Enabled()):
		max = 1024
	}

	if len(body) <= max {
		return body
	}

	return body[:max] + fmt.Sprintf(" [truncated %d chars]", len(body)-max)
}

// glogBody logs a body output that could be either JSON or protobuf. It explicitly guards against
// allocating a new string for the body output unless necessary. Uses a simple heuristic to determine
// whether the body is printable.
func glogBody(prefix string, body []byte) {
	if klog.V(8).Enabled() {
		if bytes.IndexFunc(body, func(r rune) bool {
			return r < 0x0a
		}) != -1 {
			klog.Infof("%s:\n%s", prefix, truncateBody(hex.Dump(body)))
		} else {
			klog.Infof("%s: %s", prefix, truncateBody(string(body)))
		}
	}
}

// maxUnstructuredResponseTextBytes is an upper bound on how much output to include in the unstructured error.
const maxUnstructuredResponseTextBytes = 2048

// transformUnstructuredResponseError handles an error from the server that is not in a structured form.
// It is expected to transform any response that is not recognizable as a clear server sent error from the
// K8S API using the information provided with the request. In practice, HTTP proxies and client libraries
// introduce a level of uncertainty to the responses returned by servers that in common use result in
// unexpected responses. The rough structure is:
//
// 1. Assume the server sends you something sane - JSON + well defined error objects + proper codes
//    - this is the happy path
//    - when you get this output, trust what the server sends
// 2. Guard against empty fields / bodies in received JSON and attempt to cull sufficient info from them to
//    generate a reasonable facsimile of the original failure.
//    - Be sure to use a distinct error type or flag that allows a client to distinguish between this and error 1 above
// 3. Handle true disconnect failures / completely malformed data by moving up to a more generic client error
// 4. Distinguish between various connection failures like SSL certificates, timeouts, proxy errors, unexpected
//    initial contact, the presence of mismatched body contents from posted content types
//    - Give these a separate distinct error type and capture as much as possible of the original message
//
// TODO: introduce transformation of generic http.Client.Do() errors that separates 4.
func (r *Request) transformUnstructuredResponseError(resp *http.Response, req *http.Request, body []byte) error {
	if body == nil && resp.Body != nil {
		if data, err := ioutil.ReadAll(&io.LimitedReader{R: resp.Body, N: maxUnstructuredResponseTextBytes}); err == nil {
			body = data
		}
	}
	retryAfter, _ := retryAfterSeconds(resp)
	return r.newUnstructuredResponseError(body, isTextResponse(resp), resp.StatusCode, req.Method, retryAfter)
}

// newUnstructuredResponseError instantiates the appropriate generic error for the provided input. It also logs the body.
func (r *Request) newUnstructuredResponseError(body []byte, isTextResponse bool, statusCode int, method string, retryAfter int) error {
	// cap the amount of output we create
	if len(body) > maxUnstructuredResponseTextBytes {
		body = body[:maxUnstructuredResponseTextBytes]
	}

	message := "unknown"
	if isTextResponse {
		message = strings.TrimSpace(string(body))
	}
	var groupResource schema.GroupResource
	if len(r.resource) > 0 {
		groupResource.Group = r.c.content.GroupVersion.Group
		groupResource.Resource = r.resource
	}
	return errors.NewGenericServerResponse(
		statusCode,
		method,
		groupResource,
		r.resourceName,
		message,
		retryAfter,
		true,
	)
}

// isTextResponse returns true if the response appears to be a textual media type.
func isTextResponse(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	if len(contentType) == 0 {
		return true
	}
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return strings.HasPrefix(media, "text/")
}

// checkWait returns true along with a number of seconds if the server instructed us to wait
// before retrying.
func checkWait(resp *http.Response) (int, bool) {
	switch r := resp.StatusCode; {
	// any 500 error code and 429 can trigger a wait
	case r == http.StatusTooManyRequests, r >= 500:
	default:
		return 0, false
	}
	i, ok := retryAfterSeconds(resp)
	return i, ok
}

// retryAfterSeconds returns the value of the Retry-After header and true, or 0 and false if
// the header was missing or not a valid number.
func retryAfterSeconds(resp *http.Response) (int, bool) {
	if h := resp.Header.Get("Retry-After"); len(h) > 0 {
		if i, err := strconv.Atoi(h); err == nil {
			return i, true
		}
	}
	return 0, false
}

// Result contains the result of calling Request.Do().
type Result struct {
	body        []byte
	warnings    []net.WarningHeader
	contentType string
	err         error
	statusCode  int

	decoder runtime.Decoder
}

// Raw returns the raw result.
func (r Result) Raw() ([]byte, error) {
	return r.body, r.err
}

// Get returns the result as an object, which means it passes through the decoder.
// If the returned object is of type Status and has .Status != StatusSuccess, the
// additional information in Status will be used to enrich the error.
func (r Result) Get() (runtime.Object, error) {
	if r.err != nil {
		// Check whether the result has a Status object in the body and prefer that.
		return nil, r.Error()
	}
	if r.decoder == nil {
		return nil, fmt.Errorf("serializer for %s doesn't exist", r.contentType)
	}

	// decode, but if the result is Status return that as an error instead.
	out, _, err := r.decoder.Decode(r.body, nil, nil)
	if err != nil {
		return nil, err
	}
	switch t := out.(type) {
	case *metav1.Status:
		// any status besides StatusSuccess is considered an error.
		if t.Status != metav1.StatusSuccess {
			return nil, errors.FromObject(t)
		}
	}
	return out, nil
}

// StatusCode returns the HTTP status code of the request. (Only valid if no
// error was returned.)
func (r Result) StatusCode(statusCode *int) Result {
	*statusCode = r.statusCode
	return r
}

// Into stores the result into obj, if possible. If obj is nil it is ignored.
// If the returned object is of type Status and has .Status != StatusSuccess, the
// additional information in Status will be used to enrich the error.
func (r Result) Into(obj runtime.Object) error {
	if r.err != nil {
		// Check whether the result has a Status object in the body and prefer that.
		return r.Error()
	}
	if r.decoder == nil {
		return fmt.Errorf("serializer for %s doesn't exist", r.contentType)
	}
	if len(r.body) == 0 {
		return fmt.Errorf("0-length response with status code: %d and content type: %s",
			r.statusCode, r.contentType)
	}

	out, _, err := r.decoder.Decode(r.body, nil, obj)
	if err != nil || out == obj {
		return err
	}
	// if a different object is returned, see if it is Status and avoid double decoding
	// the object.
	switch t := out.(type) {
	case *metav1.Status:
		// any status besides StatusSuccess is considered an error.
		if t.Status != metav1.StatusSuccess {
			return errors.FromObject(t)
		}
	}
	return nil
}

// WasCreated updates the provided bool pointer to whether the server returned
// 201 created or a different response.
func (r Result) WasCreated(wasCreated *bool) Result {
	*wasCreated = r.statusCode == http.StatusCreated
	return r
}

// Error returns the error executing the request, nil if no error occurred.
// If the returned object is of type Status and has Status != StatusSuccess, the
// additional information in Status will be used to enrich the error.
// See the Request.Do() comment for what errors you might get.
func (r Result) Error() error {
	// if we have received an unexpected server error, and we have a body and decoder, we can try to extract
	// a Status object.
	if r.err == nil || !errors.IsUnexpectedServerError(r.err) || len(r.body) == 0 || r.decoder == nil {
		return r.err
	}

	// attempt to convert the body into a Status object
	// to be backwards compatible with old servers that do not return a version, default to "v1"
	out, _, err := r.decoder.Decode(r.body, &schema.GroupVersionKind{Version: "v1"}, nil)
	if err != nil {
		klog.V(5).Infof("body was not decodable (unable to check for Status): %v", err)
		return r.err
	}
	switch t := out.(type) {
	case *metav1.Status:
		// because we default the kind, we *must* check for StatusFailure
		if t.Status == metav1.StatusFailure {
			return errors.FromObject(t)
		}
	}
	return r.err
}

// Warnings returns any warning headers received in the response
func (r Result) Warnings() []net.WarningHeader {
	return r.warnings
}

// NameMayNotBe specifies strings that cannot be used as names specified as path segments (like the REST API or etcd store)
var NameMayNotBe = []string{".", ".."}

// NameMayNotContain specifies substrings that cannot be used in names specified as path segments (like the REST API or etcd store)
var NameMayNotContain = []string{"/", "%"}

// IsValidPathSegmentName validates the name can be safely encoded as a path segment
func IsValidPathSegmentName(name string) []string {
	for _, illegalName := range NameMayNotBe {
		if name == illegalName {
			return []string{fmt.Sprintf(`may not be '%s'`, illegalName)}
		}
	}

	var errors []string
	for _, illegalContent := range NameMayNotContain {
		if strings.Contains(name, illegalContent) {
			errors = append(errors, fmt.Sprintf(`may not contain '%s'`, illegalContent))
		}
	}

	return errors
}

// IsValidPathSegmentPrefix validates the name can be used as a prefix for a name which will be encoded as a path segment
// It does not check for exact matches with disallowed names, since an arbitrary suffix might make the name valid
func IsValidPathSegmentPrefix(name string) []string {
	var errors []string
	for _, illegalContent := range NameMayNotContain {
		if strings.Contains(name, illegalContent) {
			errors = append(errors, fmt.Sprintf(`may not contain '%s'`, illegalContent))
		}
	}

	return errors
}

// ValidatePathSegmentName validates the name can be safely encoded as a path segment
func ValidatePathSegmentName(name string, prefix bool) []string {
	if prefix {
		return IsValidPathSegmentPrefix(name)
	}
	return IsValidPathSegmentName(name)
}
