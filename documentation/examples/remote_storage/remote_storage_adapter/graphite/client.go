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

package graphite

import (
	"bytes"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sort"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
)

// Client allows sending batches of Prometheus samples to Graphite.
type Client struct {
	logger *slog.Logger

	address   string
	transport string
	timeout   time.Duration
	prefix    string
}

// NewClient creates a new Client.
func NewClient(logger *slog.Logger, address, transport string, timeout time.Duration, prefix string) *Client {
	if logger == nil {
		logger = promslog.NewNopLogger()
	}
	return &Client{
		logger:    logger,
		address:   address,
		transport: transport,
		timeout:   timeout,
		prefix:    prefix,
	}
}

func pathFromMetric(m model.Metric, prefix string) string {
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
		// Since we use '.' instead of '=' to separate label and values
		// it means that we can't have an '.' in the metric name. Fortunately
		// this is prohibited in prometheus metrics.
		buffer.WriteString(fmt.Sprintf(
			".%s.%s", string(l), escape(v)))
	}
	return buffer.String()
}

// Write sends a batch of samples to Graphite.
func (c *Client) Write(samples model.Samples) error {
	conn, err := net.DialTimeout(c.transport, c.address, c.timeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	var buf bytes.Buffer
	for _, s := range samples {
		k := pathFromMetric(s.Metric, c.prefix)
		t := float64(s.Timestamp.UnixNano()) / 1e9
		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			c.logger.Debug("Cannot send value to Graphite, skipping sample", "value", v, "sample", s)
			continue
		}
		fmt.Fprintf(&buf, "%s %f %f\n", k, v, t)
	}

	_, err = conn.Write(buf.Bytes())
	return err
}

// Name identifies the client as a Graphite client.
func (Client) Name() string {
	return "graphite"
}
