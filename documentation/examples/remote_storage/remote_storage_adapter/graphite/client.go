// Copyright 2015 The Prometheus Authors
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

package graphite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

// Client allows sending batches of Prometheus samples to Graphite.
type Client struct {
	logger log.Logger

	carbonAddress   string
	graphiteAddress string
	transport       string
	timeout         time.Duration
	prefix          string
	enableTags      bool
}

// TargetResponse allows for easy unmarshalling of graphite responses
type TargetResponse struct {
	DataPoints [][2]json.Number  `json:"datapoints"`
	Tags       map[string]string `json:"tags"`
}

// NewClient creates a new Client.
func NewClient(logger log.Logger, carbonAddress string, graphiteAddress string, transport string, timeout time.Duration, prefix string, enableTags bool) *Client {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &Client{
		logger:          logger,
		carbonAddress:   carbonAddress,
		graphiteAddress: graphiteAddress,
		transport:       transport,
		timeout:         timeout,
		prefix:          prefix,
		enableTags:      enableTags,
	}
}

func pathFromMetric(m model.Metric, prefix string, enableTags bool) string {
	var buffer bytes.Buffer

	buffer.WriteString(prefix)
	buffer.WriteString(escape(m[model.MetricNameLabel]))

	// We want to sort the labels.
	labels := make(model.LabelNames, 0, len(m))
	for l := range m {
		labels = append(labels, l)
	}
	sort.Sort(labels)

	// For each label, in order, add ".<label>.<value>".
	for _, l := range labels {
		v := m[l]

		if l == model.MetricNameLabel || len(l) == 0 {
			continue
		}

		if enableTags {
			// Write the label using the updated carbon tag format
			// https://graphite.readthedocs.io/en/latest/tags.html
			buffer.WriteString(fmt.Sprintf(
				";%s=%s", string(l), escape(v)))
			continue
		}

		// Since we use '.' instead of '=' to separate label and values
		// it means that we can't have an '.' in the metric name. Fortunately
		// this is prohibited in prometheus metrics.
		buffer.WriteString(fmt.Sprintf(
			".%s.%s", string(l), escape(v)))
	}

	return buffer.String()
}

// Write sends a batch of samples to Graphite.
func (c Client) Write(samples model.Samples) error {
	conn, err := net.DialTimeout(c.transport, c.carbonAddress, c.timeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	var buf bytes.Buffer
	for _, s := range samples {
		k := pathFromMetric(s.Metric, c.prefix, c.enableTags)
		t := float64(s.Timestamp.UnixNano()) / 1e9
		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			level.Debug(c.logger).Log("msg", "cannot send value to Graphite, skipping sample", "value", v, "sample", s)
			continue
		}
		fmt.Fprintf(&buf, "%s %f %f\n", k, v, t)
	}

	_, err = conn.Write(buf.Bytes())
	return err
}

func (c Client) Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	PromSeries := []*prompb.TimeSeries{}
	for _, q := range req.Queries {
		formData, err := c.buildForm(q)
		if err != nil {
			return nil, err
		}

		req, err := c.createRequest(formData)
		if err != nil {
			return nil, err
		}

		http.DefaultClient.Do(req)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		series, err := c.parseResponse(resp)
		if err != nil {
			return nil, err
		}

		for _, s := range series {
			ps, err := buildPromSeries(s)
			if err != nil {
				level.Debug(c.logger).Log("msg", "Unable to parse graphite series into valid prometheus series", "reason", err)
				continue
			}
			PromSeries = append(
				PromSeries,
				ps,
			)
		}
	}

	queryResponse := prompb.ReadResponse{
		Results: []*prompb.QueryResult{
			{Timeseries: make([]*prompb.TimeSeries, 0, len(PromSeries))},
		},
	}
	for _, ts := range PromSeries {
		queryResponse.Results[0].Timeseries = append(queryResponse.Results[0].Timeseries, ts)
	}
	return &queryResponse, nil
}

func (c Client) createRequest(data url.Values) (*http.Request, error) {
	u, _ := url.Parse(c.graphiteAddress)
	u.Path = path.Join(u.Path, "render")
	u.RawQuery = data.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request. error: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return req, err
}

func (c Client) buildForm(q *prompb.Query) (url.Values, error) {
	tagSet := []string{}

	for _, m := range q.Matchers {
		var name string
		if m.Name == model.MetricNameLabel {
			name = "name"
		} else {
			name = m.Name
		}

		switch m.Type {
		case prompb.LabelMatcher_EQ:
			tagSet = append(tagSet, "\""+name+"="+m.Value+"\"")
		case prompb.LabelMatcher_NEQ:
			tagSet = append(tagSet, "\""+name+"!="+m.Value+"\"")
		case prompb.LabelMatcher_RE:
			tagSet = append(tagSet, "\""+name+"=~^("+m.Value+")$\"")
		case prompb.LabelMatcher_NRE:
			tagSet = append(tagSet, "\""+name+"!=~^("+m.Value+")$\"")
		default:
			return nil, fmt.Errorf("unknown match type %v", m.Type)
		}
	}

	formData := url.Values{
		"from":   []string{strconv.Itoa(int(q.GetStartTimestampMs()) / 1000)},
		"until":  []string{strconv.Itoa(int(q.GetEndTimestampMs()) / 1000)},
		"format": []string{"json"},
		"target": []string{"seriesByTag(" + strings.Join(tagSet, ",") + ")"},
	}

	return formData, nil
}

func (c *Client) parseResponse(res *http.Response) ([]TargetResponse, error) {
	body, err := ioutil.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		return nil, err
	}

	if res.StatusCode/100 != 2 {
		return nil, fmt.Errorf("request failed status: %v", res.Status)
	}

	var data []TargetResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func buildPromSeries(s TargetResponse) (*prompb.TimeSeries, error) {
	labels := []*prompb.Label{}
	for k, v := range s.Tags {
		if k == "name" {
			k = model.MetricNameLabel
			// Ensure the metric name is valid in case a regex query returns a
			// invalid metric
			if !model.IsValidMetricName(model.LabelValue(v)) {
				return nil, fmt.Errorf("metric name %v is not a valid prometheus metric", v)
			}
		}
		labels = append(labels, &prompb.Label{Name: k, Value: v})
	}

	samples := []*prompb.Sample{}
	for _, pair := range s.DataPoints {
		// Check to see if if the graphite values are not Null
		data, err := pair[0].Float64()
		if err != nil {
			continue
		}
		timestamp, err := pair[1].Float64()
		if err != nil {
			continue
		}
		samples = append(samples, &prompb.Sample{Value: data, Timestamp: int64(1000 * timestamp)})
	}

	return &prompb.TimeSeries{
		Labels:  labels,
		Samples: samples,
	}, nil
}

// Name identifies the client as a Graphite client.
func (c Client) Name() string {
	return "graphite"
}
