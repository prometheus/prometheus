// Copyright The Prometheus Authors
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

package opentsdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/prometheus/common/model"
)

const (
	putEndpoint     = "/api/put"
	contentTypeJSON = "application/json"
)

// Client allows sending batches of Prometheus samples to OpenTSDB.
type Client struct {
	logger *slog.Logger

	url     string
	timeout time.Duration
}

// NewClient creates a new Client.
func NewClient(logger *slog.Logger, url string, timeout time.Duration) *Client {
	return &Client{
		logger:  logger,
		url:     url,
		timeout: timeout,
	}
}

// StoreSamplesRequest is used for building a JSON request for storing samples
// via the OpenTSDB.
type StoreSamplesRequest struct {
	Metric    TagValue            `json:"metric"`
	Timestamp int64               `json:"timestamp"`
	Value     float64             `json:"value"`
	Tags      map[string]TagValue `json:"tags"`
}

// tagsFromMetric translates Prometheus metric into OpenTSDB tags.
func tagsFromMetric(m model.Metric) map[string]TagValue {
	tags := make(map[string]TagValue, len(m)-1)
	for l, v := range m {
		if l == model.MetricNameLabel {
			continue
		}
		tags[string(l)] = TagValue(v)
	}
	return tags
}

// Write sends a batch of samples to OpenTSDB via its HTTP API.
func (c *Client) Write(samples model.Samples) error {
	reqs := make([]StoreSamplesRequest, 0, len(samples))
	for _, s := range samples {
		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			c.logger.Debug("Cannot send value to OpenTSDB, skipping sample", "value", v, "sample", s)
			continue
		}
		metric := TagValue(s.Metric[model.MetricNameLabel])
		reqs = append(reqs, StoreSamplesRequest{
			Metric:    metric,
			Timestamp: s.Timestamp.Unix(),
			Value:     v,
			Tags:      tagsFromMetric(s.Metric),
		})
	}

	u, err := url.Parse(c.url)
	if err != nil {
		return err
	}

	u.Path = putEndpoint

	buf, err := json.Marshal(reqs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewBuffer(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	// API returns status code 204 for successful writes.
	// http://opentsdb.net/docs/build/html/api_http/put.html
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	// API returns status code 400 on error, encoding error details in the
	// response content in JSON.
	buf, err = io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var r map[string]int
	if err := json.Unmarshal(buf, &r); err != nil {
		return err
	}
	return fmt.Errorf("failed to write %d samples to OpenTSDB, %d succeeded", r["failed"], r["success"])
}

// Name identifies the client as an OpenTSDB client.
func (Client) Name() string {
	return "opentsdb"
}
