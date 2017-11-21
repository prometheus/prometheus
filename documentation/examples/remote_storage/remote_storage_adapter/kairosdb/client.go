// Copyright 2013 The Prometheus Authors
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

package kairosdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"time"
	//"reflect"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context/ctxhttp"
)

const (
	putEndpoint     = "/api/v1/datapoints"
	contentTypeJSON = "application/json"
)
// Client allows sending batches of Prometheus samples to KairosDB.
type Client struct {
	logger log.Logger

	url     string
	timeout time.Duration
}

// NewClient creates a new Client.
func NewClient(logger log.Logger, url string, timeout time.Duration) *Client {
	return &Client{
		logger:  logger,
		url:     url,
		timeout: timeout,
	}
}

// StoreSamplesRequest is used for building a JSON request for storing samples
// via the KairosDB.
//https://kairosdb.github.io/docs/build/html/restapi/AddDataPoints.html
//everything with caps for easier debugging of the jsons
type StoreSamplesRequest struct {
	Name      TagValue             `json:"name"`
	Timestamp int64                `json:"timestamp"`
	Value     float64               `json:"value"`
	Type      string               `json:"type"`
	Tags      map[string]TagValue  `json:"tags"`
}

// tagsFromMetric translates Prometheus metric into KairosDB tags.
func tagsFromMetric(m model.Metric) map[string]TagValue {
        tags := make(map[string]TagValue, len(m)-1)
        for l, v := range m {
                tags[string(l)] = TagValue(v)
        }
        return tags
}

// Write sends a batch of samples to KairosDB via its HTTP API.
func (c *Client) Write(samples model.Samples) error {
	reqs := make([]StoreSamplesRequest, 0, len(samples))
	for _, s := range samples {

		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			level.Debug(c.logger).Log("msg", "cannot send value to KairosDB, skipping sample", "value", v, "sample", s)
			continue
		}
		metric := TagValue(s.Metric[model.MetricNameLabel])
		tags_parse := tagsFromMetric(s.Metric)
		exit := 0
		for key,value := range tags_parse {
			if len(value) == 0 {
				exit = 1
				level.Debug(c.logger).Log("msg", "cannot send value to KairosDB, empty tag, skipping sample", "tag", key, "value", v, "sample", s)
				break
			}
		}
		if exit == 1 {
			continue
		}
		// All types will be double even if they are int, maybe could be fixed
		reqs = append(reqs, StoreSamplesRequest{
			Name:      metric,
			Timestamp: int64(s.Timestamp),
			Value:     v,
			Tags:      tags_parse,
			Type:      "double",
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
	resp, err := ctxhttp.Post(ctx, http.DefaultClient, u.String(), contentTypeJSON, bytes.NewBuffer(buf))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// API returns status code 204 for successful writes.
	//https://kairosdb.github.io/docs/build/html/restapi/AddDataPoints.html
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	// API returns status code 400 on error, encoding error details in the
	// response content in JSON.
	buf, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
        var r map[string]int
        if err := json.Unmarshal(buf, &r); err != nil {
                return err
        }
        return fmt.Errorf("failed to write %d samples to KairosDB, %d succeeded", r["failed"], r["success"])

}

// Name identifies the client as an KairosDB client.
func (c Client) Name() string {
	return "kairosdb"
}
