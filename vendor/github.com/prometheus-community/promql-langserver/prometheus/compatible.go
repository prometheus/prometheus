// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.  // You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prometheus

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// compatibleHTTPClient must be used to contact a distant prometheus with a version >= v2.15.
type compatibleHTTPClient struct {
	MetadataService
	prometheusClient v1.API
	lookbackInterval time.Duration
}

func (c *compatibleHTTPClient) MetricMetadata(ctx context.Context, metric string) (v1.Metadata, error) {
	metadata, err := c.prometheusClient.Metadata(ctx, metric, "1")
	if err != nil {
		return v1.Metadata{}, err
	}
	if len(metadata) == 0 {
		return v1.Metadata{}, nil
	}
	return v1.Metadata{
		Type: metadata[metric][0].Type,
		Help: metadata[metric][0].Help,
		Unit: metadata[metric][0].Unit,
	}, nil
}

func (c *compatibleHTTPClient) AllMetricMetadata(ctx context.Context) (map[string][]v1.Metadata, error) {
	return c.prometheusClient.Metadata(ctx, "", "")
}

func (c *compatibleHTTPClient) LabelNames(ctx context.Context, name string) ([]string, error) {
	if len(name) == 0 {
		names, _, err := c.prometheusClient.LabelNames(ctx, time.Now().Add(-1*c.lookbackInterval), time.Now())
		return names, err
	}
	labelNames, _, err := c.prometheusClient.Series(ctx, []string{name}, time.Now().Add(-1*c.lookbackInterval), time.Now())
	if err != nil {
		return nil, err
	}
	// subResult is used as a set of label. Like that we are sure we don't have any duplication
	subResult := make(map[string]bool)
	for _, ln := range labelNames {
		for l := range ln {
			subResult[string(l)] = true
		}
	}
	result := make([]string, 0, len(subResult))
	for l := range subResult {
		result = append(result, l)
	}
	return result, nil
}

func (c *compatibleHTTPClient) LabelValues(ctx context.Context, label string) ([]model.LabelValue, error) {
	values, _, err := c.prometheusClient.LabelValues(ctx, label, time.Now().Add(-1*c.lookbackInterval), time.Now())
	return values, err
}

func (c *compatibleHTTPClient) ChangeDataSource(_ string) error {
	return fmt.Errorf("method not supported")
}

func (c *compatibleHTTPClient) SetLookbackInterval(interval time.Duration) {
	c.lookbackInterval = interval
}

func (c *compatibleHTTPClient) GetURL() string {
	return ""
}
