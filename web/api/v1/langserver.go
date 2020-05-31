// Copyright 2020 The Prometheus Authors
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

package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus-community/promql-langserver/prometheus"
	"github.com/prometheus-community/promql-langserver/rest"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
)

func NewLangServer(externalURL string, q storage.Queryable,
	tr func(context.Context) TargetRetriever, logger log.Logger) (http.Handler, error) {
	// create the custom prometheus client dedicated to the langserver
	langserverClient := newPrometheusLangServerClient(externalURL, newWebkit(q, tr))
	// TODO use the method CreateInstHandler once it is taking the interface prometheus.Registerer in parameter
	// instead of the struct Registry
	return rest.CreateHandler(context.Background(), langserverClient, logger)
}

type prometheusLangServerClient struct {
	prometheus.Client
	externalURL string
	webkit      *webkit
}

func newPrometheusLangServerClient(externalURL string, webkit *webkit) prometheus.Client {
	return &prometheusLangServerClient{
		externalURL: externalURL,
		webkit:      webkit,
	}
}

// Metadata returns the first occurrence of metadata about metrics currently scraped by the metric name.
func (c *prometheusLangServerClient) Metadata(ctx context.Context, metric string) (v1.Metadata, error) {
	metadata := c.webkit.metadata(ctx, metric, 1)
	if len(metadata) == 0 {
		return v1.Metadata{}, nil
	}
	return v1.Metadata{
		Type: v1.MetricType(metadata[metric][0].Type),
		Help: metadata[metric][0].Help,
		Unit: metadata[metric][0].Unit,
	}, nil
}

// AllMetadata returns metadata about metrics currently scraped for all existing metrics.
func (c *prometheusLangServerClient) AllMetadata(ctx context.Context) (map[string][]v1.Metadata, error) {
	metadata := c.webkit.metadata(ctx, "", -1)
	result := make(map[string][]v1.Metadata)
	for metric, infos := range metadata {
		result[metric] = make([]v1.Metadata, 0, len(infos))
		for i, info := range infos {
			result[metric][i] = v1.Metadata{
				Type: v1.MetricType(info.Type),
				Help: info.Help,
				Unit: info.Unit,
			}
		}
	}
	return result, nil
}

// LabelNames returns all the unique label names present in the block in sorted order.
// If a metric is provided, then it will return all unique label names linked to the metric during a predefined period of time
func (c *prometheusLangServerClient) LabelNames(ctx context.Context, metricName string) ([]string, error) {
	if len(metricName) == 0 {
		names, _, err := c.webkit.labelNames(ctx, minTime, maxTime)
		return names, err
	}
	labelNames, _, err := c.webkit.series(ctx,
		[][]*labels.Matcher{
			{
				&labels.Matcher{
					Type:  labels.MatchEqual,
					Name:  "__name__",
					Value: metricName,
				},
			},
		}, time.Now().Add(-100*time.Hour), time.Now())
	if err != nil {
		return nil, err
	}
	var result []string
	for _, ln := range labelNames {
		for l := range ln {
			result = append(result, string(l))
		}
	}
	return result, nil
}

// LabelValues performs a query for the values of the given label.
func (c *prometheusLangServerClient) LabelValues(ctx context.Context, label string) ([]model.LabelValue, error) {
	values, _, err := c.webkit.labelValues(ctx, label, minTime, maxTime)
	if err != nil {
		return nil, err
	}
	var labelValues []model.LabelValue
	for _, value := range values {
		labelValues = append(labelValues, model.LabelValue(value))
	}
	return labelValues, nil
}

// ChangeDataSource is not used in this context because we are inside prometheus.
// So datasource/url doesn't mean anything and cannot be changed
func (c *prometheusLangServerClient) ChangeDataSource(_ string) error {
	return nil
}

// GetURL is returning the url used to contact the prometheus server
// In case the instance is used directly in Prometheus, it should be the externalURL
func (c *prometheusLangServerClient) GetURL() string {
	return c.externalURL
}
