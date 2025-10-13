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
	"math"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/sigv4"
	"go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote/azuread"
	"github.com/prometheus/prometheus/storage/remote/googleiam"
	"github.com/prometheus/prometheus/util/compression"
)

const (
	maxErrMsgLen = 1024

	RemoteWriteVersionHeader        = "X-Prometheus-Remote-Write-Version"
	RemoteWriteVersion1HeaderValue  = "0.1.0"
	RemoteWriteVersion20HeaderValue = "2.0.0"
	appProtoContentType             = "application/x-protobuf"
)

var (
	// UserAgent represents Prometheus version to use for user agent header.
	UserAgent = version.PrometheusUserAgent()

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
			Subsystem: "remote_read_client",
			Name:      "queries_total",
			Help:      "The total number of remote read queries.",
		},
		[]string{remoteName, endpoint, "response_type", "code"},
	)
	remoteReadQueries = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "remote_read_client",
			Name:      "queries",
			Help:      "The number of in-flight remote read queries.",
		},
		[]string{remoteName, endpoint},
	)
	remoteReadQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:                       namespace,
			Subsystem:                       "remote_read_client",
			Name:                            "request_duration_seconds",
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

	retryOnRateLimit      bool
	chunkedReadLimit      uint64
	acceptedResponseTypes []prompb.ReadRequest_ResponseType

	readQueries         prometheus.Gauge
	readQueriesTotal    *prometheus.CounterVec
	readQueriesDuration prometheus.ObserverVec

	writeProtoMsg    config.RemoteWriteProtoMsg
	writeCompression compression.Type // Not exposed by ClientConfig for now.
}

// ClientConfig configures a client.
type ClientConfig struct {
	URL                   *config_util.URL
	Timeout               model.Duration
	HTTPClientConfig      config_util.HTTPClientConfig
	SigV4Config           *sigv4.SigV4Config
	AzureADConfig         *azuread.AzureADConfig
	GoogleIAMConfig       *googleiam.Config
	Headers               map[string]string
	RetryOnRateLimit      bool
	WriteProtoMsg         config.RemoteWriteProtoMsg
	ChunkedReadLimit      uint64
	RoundRobinDNS         bool
	AcceptedResponseTypes []prompb.ReadRequest_ResponseType
}

// ReadClient will request the STREAMED_XOR_CHUNKS method of remote read but can
// also fall back to the SAMPLES method if necessary.
type ReadClient interface {
	Read(ctx context.Context, query *prompb.Query, sortSeries bool) (storage.SeriesSet, error)
	ReadMultiple(ctx context.Context, queries []*prompb.Query, sortSeries bool) (storage.SeriesSet, error)
}

// NewReadClient creates a new client for remote read.
func NewReadClient(name string, conf *ClientConfig, optFuncs ...config_util.HTTPClientOption) (ReadClient, error) {
	httpClient, err := config_util.NewClientFromConfig(conf.HTTPClientConfig, "remote_storage_read_client", optFuncs...)
	if err != nil {
		return nil, err
	}

	t := httpClient.Transport
	if len(conf.Headers) > 0 {
		t = newInjectHeadersRoundTripper(conf.Headers, t)
	}
	httpClient.Transport = otelhttp.NewTransport(t)

	// Set accepted response types, default to existing behavior if not specified.
	acceptedResponseTypes := conf.AcceptedResponseTypes
	if len(acceptedResponseTypes) == 0 {
		acceptedResponseTypes = AcceptedResponseTypes
	}

	return &Client{
		remoteName:            name,
		urlString:             conf.URL.String(),
		Client:                httpClient,
		timeout:               time.Duration(conf.Timeout),
		chunkedReadLimit:      conf.ChunkedReadLimit,
		acceptedResponseTypes: acceptedResponseTypes,
		readQueries:           remoteReadQueries.WithLabelValues(name, conf.URL.String()),
		readQueriesTotal:      remoteReadQueriesTotal.MustCurryWith(prometheus.Labels{remoteName: name, endpoint: conf.URL.String()}),
		readQueriesDuration:   remoteReadQueryDuration.MustCurryWith(prometheus.Labels{remoteName: name, endpoint: conf.URL.String()}),
	}, nil
}

// NewWriteClient creates a new client for remote write.
func NewWriteClient(name string, conf *ClientConfig) (WriteClient, error) {
	var httpOpts []config_util.HTTPClientOption
	if conf.RoundRobinDNS {
		httpOpts = []config_util.HTTPClientOption{config_util.WithDialContextFunc(newDialContextWithRoundRobinDNS().dialContextFn())}
	}
	httpClient, err := config_util.NewClientFromConfig(conf.HTTPClientConfig, "remote_storage_write_client", httpOpts...)
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
	httpClient.Transport = otelhttp.NewTransport(
		t,
		otelhttp.WithClientTrace(func(ctx context.Context) *httptrace.ClientTrace {
			return otelhttptrace.NewClientTrace(ctx, otelhttptrace.WithoutSubSpans())
		}))
	return &Client{
		remoteName:       name,
		urlString:        conf.URL.String(),
		Client:           httpClient,
		retryOnRateLimit: conf.RetryOnRateLimit,
		timeout:          time.Duration(conf.Timeout),
		writeProtoMsg:    writeProtoMsg,
		writeCompression: compression.Snappy,
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

	httpReq.Header.Add("Content-Encoding", c.writeCompression)
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

// Read reads from a remote endpoint. The sortSeries parameter is only respected in the case of a samples response;
// chunked responses arrive already sorted by the server.
func (c *Client) Read(ctx context.Context, query *prompb.Query, sortSeries bool) (storage.SeriesSet, error) {
	return c.ReadMultiple(ctx, []*prompb.Query{query}, sortSeries)
}

// ReadMultiple reads from a remote endpoint using multiple queries in a single request.
// The sortSeries parameter is only respected in the case of a samples response;
// chunked responses arrive already sorted by the server.
// Returns a single SeriesSet with interleaved series from all queries.
func (c *Client) ReadMultiple(ctx context.Context, queries []*prompb.Query, sortSeries bool) (storage.SeriesSet, error) {
	c.readQueries.Inc()
	defer c.readQueries.Dec()

	req := &prompb.ReadRequest{
		Queries:               queries,
		AcceptedResponseTypes: c.acceptedResponseTypes,
	}

	httpResp, cancel, start, err := c.executeReadRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	return c.handleReadResponse(httpResp, req, queries, sortSeries, start, cancel)
}

// executeReadRequest creates and executes an HTTP request for reading data.
func (c *Client) executeReadRequest(ctx context.Context, req *prompb.ReadRequest) (*http.Response, context.CancelFunc, time.Time, error) {
	data, err := proto.Marshal(req)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("unable to marshal read request: %w", err)
	}

	compressed := snappy.Encode(nil, data)
	httpReq, err := http.NewRequest(http.MethodPost, c.urlString, bytes.NewReader(compressed))
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("unable to create request: %w", err)
	}
	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Add("Accept-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", UserAgent)
	httpReq.Header.Set("X-Prometheus-Remote-Read-Version", "0.1.0")

	errTimeout := fmt.Errorf("%w: request timed out after %s", context.DeadlineExceeded, c.timeout)
	ctx, cancel := context.WithTimeoutCause(ctx, c.timeout, errTimeout)

	ctx, span := otel.Tracer("").Start(ctx, "Remote Read", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	start := time.Now()
	httpResp, err := c.Client.Do(httpReq.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, nil, time.Time{}, fmt.Errorf("error sending request: %w", err)
	}

	return httpResp, cancel, start, nil
}

// handleReadResponse processes the HTTP response and returns a SeriesSet.
func (c *Client) handleReadResponse(httpResp *http.Response, req *prompb.ReadRequest, queries []*prompb.Query, sortSeries bool, start time.Time, cancel context.CancelFunc) (storage.SeriesSet, error) {
	if httpResp.StatusCode/100 != 2 {
		// Make an attempt at getting an error message.
		body, _ := io.ReadAll(httpResp.Body)
		_ = httpResp.Body.Close()

		cancel()
		errStr := strings.Trim(string(body), "\n")
		err := errors.New(errStr)
		return nil, fmt.Errorf("remote server %s returned http status %s: %w", c.urlString, httpResp.Status, err)
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
		return c.handleChunkedResponseImpl(s, httpResp, queries, func(err error) {
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

func (*Client) handleSampledResponse(req *prompb.ReadRequest, httpResp *http.Response, sortSeries bool) (storage.SeriesSet, error) {
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

	return combineQueryResults(resp.Results, sortSeries)
}

// combineQueryResults combines multiple query results into a single SeriesSet,
// handling both sorted and unsorted cases appropriately.
func combineQueryResults(results []*prompb.QueryResult, sortSeries bool) (storage.SeriesSet, error) {
	if len(results) == 0 {
		return &concreteSeriesSet{series: nil, cur: 0}, nil
	}

	if len(results) == 1 {
		return FromQueryResult(sortSeries, results[0]), nil
	}

	// Multiple queries case - combine all results
	if sortSeries {
		// When sorting is requested, use MergeSeriesSet which can efficiently merge sorted inputs
		var allSeriesSets []storage.SeriesSet
		for _, result := range results {
			seriesSet := FromQueryResult(sortSeries, result)
			if err := seriesSet.Err(); err != nil {
				return nil, fmt.Errorf("error reading series from query result: %w", err)
			}
			allSeriesSets = append(allSeriesSets, seriesSet)
		}
		return storage.NewMergeSeriesSet(allSeriesSets, 0, storage.ChainedSeriesMerge), nil
	}

	// When sorting is not requested, just concatenate all series without using MergeSeriesSet
	// since MergeSeriesSet requires sorted inputs
	var allSeries []storage.Series
	for _, result := range results {
		seriesSet := FromQueryResult(sortSeries, result)
		for seriesSet.Next() {
			allSeries = append(allSeries, seriesSet.At())
		}
		if err := seriesSet.Err(); err != nil {
			return nil, fmt.Errorf("error reading series from query result: %w", err)
		}
	}

	return &concreteSeriesSet{series: allSeries, cur: 0}, nil
}

// handleChunkedResponseImpl handles chunked responses for both single and multiple queries.
func (*Client) handleChunkedResponseImpl(s *ChunkedReader, httpResp *http.Response, queries []*prompb.Query, onClose func(error)) storage.SeriesSet {
	// For multiple queries in chunked response, we'll still use the existing infrastructure
	// but we need to provide the timestamp range that covers all queries
	var minStartTs, maxEndTs int64 = math.MaxInt64, math.MinInt64

	for _, query := range queries {
		minStartTs = min(minStartTs, query.StartTimestampMs)
		maxEndTs = max(maxEndTs, query.EndTimestampMs)
	}

	return NewChunkedSeriesSet(s, httpResp.Body, minStartTs, maxEndTs, onClose)
}
