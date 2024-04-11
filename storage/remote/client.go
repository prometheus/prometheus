// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package remote

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/sigv4"
	"github.com/prometheus/common/version"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote/azuread"
)

const maxErrMsgLen = 1024

var UserAgent = fmt.Sprintf("Prometheus/%s", version.Version)

// If we send a Remote Write 2.0 request to a Remote Write endpoint that only understands
// Remote Write 1.0 it will respond with an error. We need to handle these errors
// accordingly. Any 5xx errors will just need to be retried as they are considered
// transient/recoverable errors. A 4xx error will need to be passed back to the queue
// manager in order to be re-encoded in a suitable format.

// A Remote Write 2.0 request sent to, for example, a Prometheus 2.50 receiver (which does
// not understand Remote Write 2.0) will result in an HTTP 400 status code from the receiver.

// A Remote Write 2.0 request sent to a remote write receiver may (depending on receiver version)
// result in an HTTP 406 status code to indicate that it does not accept the protocol or
// encoding of that request and that the sender should retry with a more suitable protocol
// version or encoding.

// We bundle any error we want to return to the queue manager to trigger a renegotiation into
// this custom error.
type ErrRenegotiate struct {
	FirstLine  string
	StatusCode int
}

func (r *ErrRenegotiate) Error() string {
	return fmt.Sprintf("HTTP %d: msg: %s", r.StatusCode, r.FirstLine)
}

var (
	remoteReadQueriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "read_queries_total",
			Help:      "The total number of remote read queries.",
		},
		[]string{remoteName, endpoint, "code"},
	)
	remoteReadQueries = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "remote_read_queries",
			Help:      "The number of in-flight remote read queries.",
		},
		[]string{remoteName, endpoint},
	)
	remoteReadQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "read_request_duration_seconds",
			Help:      "Histogram of the latency for remote read requests.",
			Buckets:   append(prometheus.DefBuckets, 25, 60),
		},
		[]string{remoteName, endpoint},
	)
)

func init() {
	prometheus.MustRegister(remoteReadQueriesTotal, remoteReadQueries, remoteReadQueryDuration)
}

// Client allows reading and writing from/to a remote HTTP endpoint.
type Client struct {
	remoteName   string // Used to differentiate clients in metrics.
	urlString    string // url.String()
	lastRWHeader string
	Client       *http.Client
	timeout      time.Duration

	retryOnRateLimit bool

	readQueries         prometheus.Gauge
	readQueriesTotal    *prometheus.CounterVec
	readQueriesDuration prometheus.Observer
}

// ClientConfig configures a client.
type ClientConfig struct {
	URL               *config_util.URL
	RemoteWriteFormat config.RemoteWriteFormat
	Timeout           model.Duration
	HTTPClientConfig  config_util.HTTPClientConfig
	SigV4Config       *sigv4.SigV4Config
	AzureADConfig     *azuread.AzureADConfig
	Headers           map[string]string
	RetryOnRateLimit  bool
}

// ReadClient uses the SAMPLES method of remote read to read series samples from remote server.
// TODO(bwplotka): Add streamed chunked remote read method as well (https://github.com/prometheus/prometheus/issues/5926).
type ReadClient interface {
	Read(ctx context.Context, query *prompb.Query) (*prompb.QueryResult, error)
}

// NewReadClient creates a new client for remote read.
func NewReadClient(name string, conf *ClientConfig) (ReadClient, error) {
	httpClient, err := config_util.NewClientFromConfig(conf.HTTPClientConfig, "remote_storage_read_client")
	if err != nil {
		return nil, err
	}

	t := httpClient.Transport
	if len(conf.Headers) > 0 {
		t = newInjectHeadersRoundTripper(conf.Headers, t)
	}
	httpClient.Transport = otelhttp.NewTransport(t)

	return &Client{
		remoteName:          name,
		urlString:           conf.URL.String(),
		Client:              httpClient,
		timeout:             time.Duration(conf.Timeout),
		readQueries:         remoteReadQueries.WithLabelValues(name, conf.URL.String()),
		readQueriesTotal:    remoteReadQueriesTotal.MustCurryWith(prometheus.Labels{remoteName: name, endpoint: conf.URL.String()}),
		readQueriesDuration: remoteReadQueryDuration.WithLabelValues(name, conf.URL.String()),
	}, nil
}

// NewWriteClient creates a new client for remote write.
func NewWriteClient(name string, conf *ClientConfig) (WriteClient, error) {
	httpClient, err := config_util.NewClientFromConfig(conf.HTTPClientConfig, "remote_storage_write_client")
	if err != nil {
		return nil, err
	}
	t := httpClient.Transport

	if len(conf.Headers) > 0 {
		t = newInjectHeadersRoundTripper(conf.Headers, t)
	}

	if conf.SigV4Config != nil {
		t, err = sigv4.NewSigV4RoundTripper(conf.SigV4Config, t)
		if err != nil {
			return nil, err
		}
	}

	if conf.AzureADConfig != nil {
		t, err = azuread.NewAzureADRoundTripper(conf.AzureADConfig, t)
		if err != nil {
			return nil, err
		}
	}

	httpClient.Transport = otelhttp.NewTransport(t)

	return &Client{
		remoteName:       name,
		urlString:        conf.URL.String(),
		Client:           httpClient,
		retryOnRateLimit: conf.RetryOnRateLimit,
		timeout:          time.Duration(conf.Timeout),
	}, nil
}

func newInjectHeadersRoundTripper(h map[string]string, underlyingRT http.RoundTripper) *injectHeadersRoundTripper {
	return &injectHeadersRoundTripper{headers: h, RoundTripper: underlyingRT}
}

type injectHeadersRoundTripper struct {
	headers map[string]string
	http.RoundTripper
}

func (t *injectHeadersRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}
	return t.RoundTripper.RoundTrip(req)
}

const defaultBackoff = 0

type RecoverableError struct {
	error
	retryAfter model.Duration
}

// Attempt a HEAD request against a remote write endpoint to see what it supports.
func (c *Client) probeRemoteVersions(ctx context.Context) error {
	// We assume we are in Version2 mode otherwise we shouldn't be calling this.

	httpReq, err := http.NewRequest("HEAD", c.urlString, nil)
	if err != nil {
		// Errors from NewRequest are from unparsable URLs, so are not
		// recoverable.
		return err
	}

	// Set the version header to be nice.
	httpReq.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion20HeaderValue)
	httpReq.Header.Set("User-Agent", UserAgent)

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	httpResp, err := c.Client.Do(httpReq.WithContext(ctx))
	if err != nil {
		// We don't attempt a retry here.
		return err
	}

	// See if we got a header anyway.
	promHeader := httpResp.Header.Get(RemoteWriteVersionHeader)

	// Only update lastRWHeader if the X-Prometheus-Remote-Write header is not blank.
	if promHeader != "" {
		c.lastRWHeader = promHeader
	}

	// Check for an error.
	if httpResp.StatusCode != 200 {
		if httpResp.StatusCode == 405 {
			// If we get a 405 (MethodNotAllowed) error then it means the endpoint doesn't
			// understand Remote Write 2.0, so we allow the lastRWHeader to be overwritten
			// even if it is blank.
			// This will make subsequent sends use RemoteWrite 1.0 until the endpoint gives
			// a response that confirms it can speak 2.0.
			c.lastRWHeader = promHeader
		}
		return fmt.Errorf(httpResp.Status)
	}

	// All ok, return no error.
	return nil
}

// Store sends a batch of samples to the HTTP endpoint, the request is the proto marshalled
// and encoded bytes from codec.go.
func (c *Client) Store(ctx context.Context, req []byte, attempt int, rwFormat config.RemoteWriteFormat, compression string) error {
	httpReq, err := http.NewRequest("POST", c.urlString, bytes.NewReader(req))
	if err != nil {
		// Errors from NewRequest are from unparsable URLs, so are not
		// recoverable.
		return err
	}

	httpReq.Header.Add("Content-Encoding", compression)
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", UserAgent)

	if rwFormat == Version1 {
		httpReq.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion1HeaderValue)
	} else {
		// Set the right header if we're using v2.0 remote write protocol.
		httpReq.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion20HeaderValue)
	}

	if attempt > 0 {
		httpReq.Header.Set("Retry-Attempt", strconv.Itoa(attempt))
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ctx, span := otel.Tracer("").Start(ctx, "Remote Store", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	httpResp, err := c.Client.Do(httpReq.WithContext(ctx))
	if err != nil {
		// Errors from Client.Do are from (for example) network errors, so are
		// recoverable.
		return RecoverableError{err, defaultBackoff}
	}
	defer func() {
		io.Copy(io.Discard, httpResp.Body)
		httpResp.Body.Close()
	}()

	// See if we got a X-Prometheus-Remote-Write header in the response.
	if promHeader := httpResp.Header.Get(RemoteWriteVersionHeader); promHeader != "" {
		// Only update lastRWHeader if the X-Prometheus-Remote-Write header is not blank.
		// (It's blank if it wasn't present, we don't care about that distinction.)
		c.lastRWHeader = promHeader
	}

	if httpResp.StatusCode/100 != 2 {
		scanner := bufio.NewScanner(io.LimitReader(httpResp.Body, maxErrMsgLen))
		line := ""
		if scanner.Scan() {
			line = scanner.Text()
		}
		switch httpResp.StatusCode {
		case 400:
			// Return an unrecoverable error to indicate the 400.
			// This then gets passed up the chain so we can react to it properly.
			return &ErrRenegotiate{line, httpResp.StatusCode}
		case 406:
			// Return an unrecoverable error to indicate the 406.
			// This then gets passed up the chain so we can react to it properly.
			return &ErrRenegotiate{line, httpResp.StatusCode}
		default:
			// We want to end up returning a non-specific error.
			err = fmt.Errorf("server returned HTTP status %s: %s", httpResp.Status, line)
		}
	}
	if httpResp.StatusCode/100 == 5 ||
		(c.retryOnRateLimit && httpResp.StatusCode == http.StatusTooManyRequests) {
		return RecoverableError{err, retryAfterDuration(httpResp.Header.Get("Retry-After"))}
	}
	return err
}

// retryAfterDuration returns the duration for the Retry-After header. In case of any errors, it
// returns the defaultBackoff as if the header was never supplied.
func retryAfterDuration(t string) model.Duration {
	parsedDuration, err := time.Parse(http.TimeFormat, t)
	if err == nil {
		s := time.Until(parsedDuration).Seconds()
		return model.Duration(s) * model.Duration(time.Second)
	}
	// The duration can be in seconds.
	d, err := strconv.Atoi(t)
	if err != nil {
		return defaultBackoff
	}
	return model.Duration(d) * model.Duration(time.Second)
}

// Name uniquely identifies the client.
func (c Client) Name() string {
	return c.remoteName
}

// Endpoint is the remote read or write endpoint.
func (c Client) Endpoint() string {
	return c.urlString
}

func (c *Client) GetLastRWHeader() string {
	return c.lastRWHeader
}

// Read reads from a remote endpoint.
func (c *Client) Read(ctx context.Context, query *prompb.Query) (*prompb.QueryResult, error) {
	c.readQueries.Inc()
	defer c.readQueries.Dec()

	req := &prompb.ReadRequest{
		// TODO: Support batching multiple queries into one read request,
		// as the protobuf interface allows for it.
		Queries: []*prompb.Query{
			query,
		},
	}
	data, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal read request: %w", err)
	}

	compressed := snappy.Encode(nil, data)
	httpReq, err := http.NewRequest("POST", c.urlString, bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}
	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Add("Accept-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", UserAgent)
	httpReq.Header.Set("X-Prometheus-Remote-Read-Version", "0.1.0")

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ctx, span := otel.Tracer("").Start(ctx, "Remote Read", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	start := time.Now()
	httpResp, err := c.Client.Do(httpReq.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, httpResp.Body)
		httpResp.Body.Close()
	}()
	c.readQueriesDuration.Observe(time.Since(start).Seconds())
	c.readQueriesTotal.WithLabelValues(strconv.Itoa(httpResp.StatusCode)).Inc()

	compressed, err = io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response. HTTP status code: %s: %w", httpResp.Status, err)
	}

	if httpResp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("remote server %s returned HTTP status %s: %s", c.urlString, httpResp.Status, strings.TrimSpace(string(compressed)))
	}

	uncompressed, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	var resp prompb.ReadResponse
	err = proto.Unmarshal(uncompressed, &resp)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal response body: %w", err)
	}

	if len(resp.Results) != len(req.Queries) {
		return nil, fmt.Errorf("responses: want %d, got %d", len(req.Queries), len(resp.Results))
	}

	return resp.Results[0], nil
}

type TestClient struct {
	name string
	url  string
}

func NewTestClient(name, url string) WriteClient {
	return &TestClient{name: name, url: url}
}

func (c *TestClient) probeRemoteVersions(_ context.Context) error {
	return nil
}

func (c *TestClient) Store(_ context.Context, req []byte, _ int, _ config.RemoteWriteFormat, _ string) error {
	r := rand.Intn(200-100) + 100
	time.Sleep(time.Duration(r) * time.Millisecond)
	return nil
}

func (c *TestClient) Name() string {
	return c.name
}

func (c *TestClient) Endpoint() string {
	return c.url
}

func (c *TestClient) GetLastRWHeader() string {
	return "2.0;snappy,0.1.0"
}
