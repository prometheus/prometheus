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
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/prometheus/prometheus/prompb"
)

const maxErrMsgLen = 256

var userAgent = fmt.Sprintf("Prometheus/%s", version.Version)

var remoteReadQueriesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "read_queries_total",
		Help:      "The total number of remote read queries.",
	},
	[]string{remoteName, endpoint, "code"},
)

// Client allows reading and writing from/to a remote HTTP endpoint.
type Client struct {
	remoteName string // Used to differentiate clients in metrics.
	url        *config_util.URL
	client     *http.Client
	timeout    time.Duration
}

// ClientConfig configures a Client.
type ClientConfig struct {
	URL              *config_util.URL
	Timeout          model.Duration
	HTTPClientConfig config_util.HTTPClientConfig
}

func init() {
	prometheus.MustRegister(remoteReadQueriesTotal)
}

// NewClient creates a new Client.
func NewClient(remoteName string, conf *ClientConfig) (*Client, error) {
	httpClient, err := config_util.NewClientFromConfig(conf.HTTPClientConfig, "remote_storage", false)
	if err != nil {
		return nil, err
	}

	t := httpClient.Transport
	httpClient.Transport = &nethttp.Transport{
		RoundTripper: t,
	}

	return &Client{
		remoteName: remoteName,
		url:        conf.URL,
		client:     httpClient,
		timeout:    time.Duration(conf.Timeout),
	}, nil
}

type recoverableError struct {
	error
}

// Store sends a batch of samples to the HTTP endpoint, the request is the proto marshalled
// and encoded bytes from codec.go.
func (c *Client) Store(ctx context.Context, req []byte) error {
	httpReq, err := http.NewRequest("POST", c.url.String(), bytes.NewReader(req))
	if err != nil {
		// Errors from NewRequest are from unparsable URLs, so are not
		// recoverable.
		return err
	}
	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", userAgent)
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	httpReq = httpReq.WithContext(ctx)

	if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
		var ht *nethttp.Tracer
		httpReq, ht = nethttp.TraceRequest(
			parentSpan.Tracer(),
			httpReq,
			nethttp.OperationName("Remote Store"),
			nethttp.ClientTrace(false),
		)
		defer ht.Finish()
	}

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		// Errors from client.Do are from (for example) network errors, so are
		// recoverable.
		return recoverableError{err}
	}
	defer func() {
		io.Copy(ioutil.Discard, httpResp.Body)
		httpResp.Body.Close()
	}()

	if httpResp.StatusCode/100 != 2 {
		scanner := bufio.NewScanner(io.LimitReader(httpResp.Body, maxErrMsgLen))
		line := ""
		if scanner.Scan() {
			line = scanner.Text()
		}
		err = errors.Errorf("server returned HTTP status %s: %s", httpResp.Status, line)
	}
	if httpResp.StatusCode/100 == 5 {
		return recoverableError{err}
	}
	return err
}

// Name uniquely identifies the client.
func (c Client) Name() string {
	return c.remoteName
}

// Endpoint is the remote read or write endpoint.
func (c Client) Endpoint() string {
	return c.url.String()
}

// Read reads from a remote endpoint.
func (c *Client) Read(ctx context.Context, query *prompb.Query) (*prompb.QueryResult, error) {
	req := &prompb.ReadRequest{
		// TODO: Support batching multiple queries into one read request,
		// as the protobuf interface allows for it.
		Queries: []*prompb.Query{
			query,
		},
	}
	data, err := proto.Marshal(req)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to marshal read request")
	}

	compressed := snappy.Encode(nil, data)
	httpReq, err := http.NewRequest("POST", c.url.String(), bytes.NewReader(compressed))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create request")
	}
	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Add("Accept-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", userAgent)
	httpReq.Header.Set("X-Prometheus-Remote-Read-Version", "0.1.0")

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	httpReq = httpReq.WithContext(ctx)

	if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
		var ht *nethttp.Tracer
		httpReq, ht = nethttp.TraceRequest(
			parentSpan.Tracer(),
			httpReq,
			nethttp.OperationName("Remote Read"),
			nethttp.ClientTrace(false),
		)
		defer ht.Finish()
	}

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "error sending request")
	}
	defer func() {
		io.Copy(ioutil.Discard, httpResp.Body)
		httpResp.Body.Close()
	}()

	remoteReadTotalCounter := remoteReadQueriesTotal.WithLabelValues(c.remoteName, c.url.String(), strconv.Itoa(httpResp.StatusCode))
	remoteReadTotalCounter.Inc()

	compressed, err = ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error reading response. HTTP status code: %s", httpResp.Status))
	}

	if httpResp.StatusCode/100 != 2 {
		return nil, errors.Errorf("remote server %s returned HTTP status %s: %s", c.url.String(), httpResp.Status, strings.TrimSpace(string(compressed)))
	}

	uncompressed, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, errors.Wrap(err, "error reading response")
	}

	var resp prompb.ReadResponse
	err = proto.Unmarshal(uncompressed, &resp)
	if err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal response body")
	}

	if len(resp.Results) != len(req.Queries) {
		return nil, errors.Errorf("responses: want %d, got %d", len(req.Queries), len(resp.Results))
	}

	return resp.Results[0], nil
}
