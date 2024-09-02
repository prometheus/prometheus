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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote/azuread"
	"github.com/prometheus/prometheus/storage/remote/googleiam"
)

const (
	maxErrMsgLen = 1024

	RemoteWriteVersionHeader        = "X-Prometheus-Remote-Write-Version"
	RemoteWriteVersion1HeaderValue  = "0.1.0"
	RemoteWriteVersion20HeaderValue = "2.0.0"
	appProtoContentType             = "application/x-protobuf"
)

// Compression represents the encoding. Currently remote storage supports only
// one, but we experiment with more, thus leaving the compression scaffolding
// for now.
// NOTE(bwplotka): Keeping it public, as a non-stable help for importers to use.
type Compression string

const (
	// SnappyBlockCompression represents https://github.com/google/snappy/blob/2c94e11145f0b7b184b831577c93e5a41c4c0346/format_description.txt
	SnappyBlockCompression Compression = "snappy"
)

var (
	// UserAgent represents Prometheus version to use for user agent header.
	UserAgent = fmt.Sprintf("Prometheus/%s", version.Version)

	remoteWriteContentTypeHeaders = map[config.RemoteWriteProtoMsg]string{
		config.RemoteWriteProtoMsgV1: appProtoContentType, // Also application/x-protobuf;proto=prometheus.WriteRequest but simplified for compatibility with 1.x spec.
		config.RemoteWriteProtoMsgV2: appProtoContentType + ";proto=io.prometheus.write.v2.Request",
	}

	AcceptedResponseTypes = []prompb.ReadRequest_ResponseType{
		prompb.ReadRequest_STREAMED_XOR_CHUNKS,
		prompb.ReadRequest_SAMPLES,
	}

	remoteReadQueriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "read_queries_total",
			Help:      "The total number of remote read queries.",
		},
		[]string{remoteName, endpoint, "response_type", "code"},
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
			Namespace:                       namespace,
			Subsystem:                       subsystem,
			Name:                            "read_request_duration_seconds",
			Help:                            "Histogram of the latency for remote read requests. Note that for streamed responses this is only the duration of the initial call and does not include the processing of the stream.",
			Buckets:                         append(prometheus.DefBuckets, 25, 60),
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
		[]string{remoteName, endpoint, "response_type"},
	)
)

func init() {
	prometheus.MustRegister(remoteReadQueriesTotal, remoteReadQueries, remoteReadQueryDuration)
}

// Client allows reading and writing from/to a remote HTTP endpoint.
type Client struct {
	remoteName string // Used to differentiate clients in metrics.
	urlString  string // url.String()
	Client     *http.Client
	timeout    time.Duration

	retryOnRateLimit bool
	chunkedReadLimit uint64

	readQueries         prometheus.Gauge
	readQueriesTotal    *prometheus.CounterVec
	readQueriesDuration prometheus.ObserverVec

	writeProtoMsg    config.RemoteWriteProtoMsg
	writeCompression Compression // Not exposed by ClientConfig for now.
}

// ClientConfig configures a client.
type ClientConfig struct {
	URL              *config_util.URL
	Timeout          model.Duration
	HTTPClientConfig config_util.HTTPClientConfig
	SigV4Config      *sigv4.SigV4Config
	AzureADConfig    *azuread.AzureADConfig
	GoogleIAMConfig  *googleiam.Config
	Headers          map[string]string
	RetryOnRateLimit bool
	WriteProtoMsg    config.RemoteWriteProtoMsg
	ChunkedReadLimit uint64
}

// ReadClient will request the STREAMED_XOR_CHUNKS method of remote read but can
// also fall back to the SAMPLES method if necessary.
type ReadClient interface {
	Read(ctx context.Context, query *prompb.Query, sortSeries bool) (storage.SeriesSet, error)
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
		chunkedReadLimit:    conf.ChunkedReadLimit,
		readQueries:         remoteReadQueries.WithLabelValues(name, conf.URL.String()),
		readQueriesTotal:    remoteReadQueriesTotal.MustCurryWith(prometheus.Labels{remoteName: name, endpoint: conf.URL.String()}),
		readQueriesDuration: remoteReadQueryDuration.MustCurryWith(prometheus.Labels{remoteName: name, endpoint: conf.URL.String()}),
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

	if conf.GoogleIAMConfig != nil {
		t, err = googleiam.NewRoundTripper(conf.GoogleIAMConfig, t)
		if err != nil {
			return nil, err
		}
	}

	writeProtoMsg := config.RemoteWriteProtoMsgV1
	if conf.WriteProtoMsg != "" {
		writeProtoMsg = conf.WriteProtoMsg
	}

	httpClient.Transport = otelhttp.NewTransport(t)
	return &Client{
		remoteName:       name,
		urlString:        conf.URL.String(),
		Client:           httpClient,
		retryOnRateLimit: conf.RetryOnRateLimit,
		timeout:          time.Duration(conf.Timeout),
		writeProtoMsg:    writeProtoMsg,
		writeCompression: SnappyBlockCompression,
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

// Store sends a batch of samples to the HTTP endpoint, the request is the proto marshalled
// and encoded bytes from codec.go.
func (c *Client) Store(ctx context.Context, req []byte, attempt int) (WriteResponseStats, error) {
	httpReq, err := http.NewRequest(http.MethodPost, c.urlString, bytes.NewReader(req))
	if err != nil {
		// Errors from NewRequest are from unparsable URLs, so are not
		// recoverable.
		return WriteResponseStats{}, err
	}

	httpReq.Header.Add("Content-Encoding", string(c.writeCompression))
	httpReq.Header.Set("Content-Type", remoteWriteContentTypeHeaders[c.writeProtoMsg])
	httpReq.Header.Set("User-Agent", UserAgent)
	if c.writeProtoMsg == config.RemoteWriteProtoMsgV1 {
		// Compatibility mode for 1.0.
		httpReq.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion1HeaderValue)
	} else {
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
		return WriteResponseStats{}, RecoverableError{err, defaultBackoff}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, httpResp.Body)
		_ = httpResp.Body.Close()
	}()

	// TODO(bwplotka): Pass logger and emit debug on error?
	// Parsing error means there were some response header values we can't parse,
	// we can continue handling.
	rs, _ := ParseWriteResponseStats(httpResp)

	if httpResp.StatusCode/100 == 2 {
		return rs, nil
	}

	// Handling errors e.g. read potential error in the body.
	// TODO(bwplotka): Pass logger and emit debug on error?
	body, _ := io.ReadAll(io.LimitReader(httpResp.Body, maxErrMsgLen))
	err = fmt.Errorf("server returned HTTP status %s: %s", httpResp.Status, body)

	if httpResp.StatusCode/100 == 5 ||
		(c.retryOnRateLimit && httpResp.StatusCode == http.StatusTooManyRequests) {
		return rs, RecoverableError{err, retryAfterDuration(httpResp.Header.Get("Retry-After"))}
	}
	return rs, err
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
func (c *Client) Name() string {
	return c.remoteName
}

// Endpoint is the remote read or write endpoint.
func (c *Client) Endpoint() string {
	return c.urlString
}

// Read reads from a remote endpoint. The sortSeries parameter is only respected in the case of a sampled response;
// chunked responses arrive already sorted by the server.
func (c *Client) Read(ctx context.Context, query *prompb.Query, sortSeries bool) (storage.SeriesSet, error) {
	c.readQueries.Inc()
	defer c.readQueries.Dec()

	req := &prompb.ReadRequest{
		// TODO: Support batching multiple queries into one read request,
		// as the protobuf interface allows for it.
		Queries:               []*prompb.Query{query},
		AcceptedResponseTypes: AcceptedResponseTypes,
	}
	data, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal read request: %w", err)
	}

	compressed := snappy.Encode(nil, data)
	httpReq, err := http.NewRequest(http.MethodPost, c.urlString, bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}
	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Add("Accept-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", UserAgent)
	httpReq.Header.Set("X-Prometheus-Remote-Read-Version", "0.1.0")

	ctx, cancel := context.WithTimeout(ctx, c.timeout)

	ctx, span := otel.Tracer("").Start(ctx, "Remote Read", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	start := time.Now()
	httpResp, err := c.Client.Do(httpReq.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	if httpResp.StatusCode/100 != 2 {
		// Make an attempt at getting an error message.
		body, _ := io.ReadAll(httpResp.Body)
		_ = httpResp.Body.Close()

		cancel()
		return nil, fmt.Errorf("remote server %s returned http status %s: %s", c.urlString, httpResp.Status, string(body))
	}

	contentType := httpResp.Header.Get("Content-Type")

	switch {
	case strings.HasPrefix(contentType, "application/x-protobuf"):
		c.readQueriesDuration.WithLabelValues("sampled").Observe(time.Since(start).Seconds())
		c.readQueriesTotal.WithLabelValues("sampled", strconv.Itoa(httpResp.StatusCode)).Inc()
		ss, err := c.handleSampledResponse(req, httpResp, sortSeries)
		cancel()
		return ss, err
	case strings.HasPrefix(contentType, "application/x-streamed-protobuf; proto=prometheus.ChunkedReadResponse"):
		c.readQueriesDuration.WithLabelValues("chunked").Observe(time.Since(start).Seconds())

		s := NewChunkedReader(httpResp.Body, c.chunkedReadLimit, nil)
		return NewChunkedSeriesSet(s, httpResp.Body, query.StartTimestampMs, query.EndTimestampMs, func(err error) {
			code := strconv.Itoa(httpResp.StatusCode)
			if !errors.Is(err, io.EOF) {
				code = "aborted_stream"
			}
			c.readQueriesTotal.WithLabelValues("chunked", code).Inc()
			cancel()
		}), nil
	default:
		c.readQueriesDuration.WithLabelValues("unsupported").Observe(time.Since(start).Seconds())
		c.readQueriesTotal.WithLabelValues("unsupported", strconv.Itoa(httpResp.StatusCode)).Inc()
		cancel()
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}
}

func (c *Client) handleSampledResponse(req *prompb.ReadRequest, httpResp *http.Response, sortSeries bool) (storage.SeriesSet, error) {
	compressed, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response. HTTP status code: %s: %w", httpResp.Status, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, httpResp.Body)
		_ = httpResp.Body.Close()
	}()

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

	// This client does not batch queries so there's always only 1 result.
	res := resp.Results[0]

	return FromQueryResult(sortSeries, res), nil
}
