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
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus-community/promql-langserver/prometheus"
	"github.com/prometheus-community/promql-langserver/rest"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
)

func newLangServerAPI(q storage.Queryable, tr func(context.Context) TargetRetriever, logger log.Logger) (*rest.API, error) {
	// Create the custom prometheus client dedicated to the langserver.
	langserverClient := &prometheusLangServerClient{
		lookbackInterval: 12 * time.Hour,
		metadataProvider: &metadataProvider{
			queryable:       q,
			targetRetriever: tr,
		},
	}
	return rest.NewLangServerAPI(context.Background(), langserverClient, logger, false)
}

type prometheusLangServerClient struct {
	prometheus.MetadataService
	metadataProvider *metadataProvider
	lookbackInterval time.Duration
}

// MetricMetadata returns the first occurrence of metadata about metrics currently scraped by the metric name.
func (c *prometheusLangServerClient) MetricMetadata(ctx context.Context, metric string) (v1.Metadata, error) {
	metadata := c.metadataProvider.metricMetadata(ctx, metric, 1)
	if len(metadata) == 0 {
		return v1.Metadata{}, nil
	}
	return v1.Metadata{
		Type: v1.MetricType(metadata[metric][0].Type),
		Help: metadata[metric][0].Help,
		Unit: metadata[metric][0].Unit,
	}, nil
}

// AllMetricMetadata returns metadata about metrics currently scraped for all existing metrics.
func (c *prometheusLangServerClient) AllMetricMetadata(ctx context.Context) (map[string][]v1.Metadata, error) {
	metadata := c.metadataProvider.metricMetadata(ctx, "", -1)
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

// LabelNames returns all the unique label names present in the interval given by the attribute lookbackInterval in sorted order.
// If a metric is provided, then it will return all unique label names linked to the metric during the same interval of time.
func (c *prometheusLangServerClient) LabelNames(ctx context.Context, metricName string) ([]string, error) {
	var q storage.Querier
	var err error
	defer func() {
		if q != nil {
			q.Close()
		}
	}()

	if len(metricName) == 0 {
		var names []string
		q, names, _, err = c.metadataProvider.labelNames(ctx, time.Now().Add(-1*c.lookbackInterval), time.Now())
		if err != nil {
			return nil, err
		}
		// Here we have to copy the result before closing the query. Otherwise the result will disappear
		result := make([]string, 0, len(names))
		for _, name := range names {
			newName := "" + name
			result = append(result, newName)
		}
		return result, nil
	}

	var labelNames []labels.Labels
	q, labelNames, _, err = c.metadataProvider.series(ctx,
		[][]*labels.Matcher{
			{
				&labels.Matcher{
					Type:  labels.MatchEqual,
					Name:  "__name__",
					Value: metricName,
				},
			},
		}, time.Now().Add(-1*c.lookbackInterval), time.Now())
	if err != nil {
		return nil, err
	}

	// subResult is used as a set of label. Like that we are sure we don't have any duplication.
	subResult := make(map[string]bool)
	for _, ln := range labelNames {
		for _, l := range ln {
			subResult[l.Name] = true
		}
	}
	result := make([]string, 0, len(subResult))
	for l := range subResult {
		// Create a complete new string. It will avoid to return an empty because the query has been closed too earlier.
		newLabelName := "" + l
		result = append(result, newLabelName)
	}
	return result, nil
}

// LabelValues performs a query for the values of the given label.
func (c *prometheusLangServerClient) LabelValues(ctx context.Context, label string) ([]model.LabelValue, error) {
	q, values, _, err := c.metadataProvider.labelValues(ctx, label, time.Now().Add(-1*c.lookbackInterval), time.Now())
	defer func() {
		if q != nil {
			q.Close()
		}
	}()

	if err != nil {
		return nil, err
	}
	labelValues := make([]model.LabelValue, 0, len(values))
	for _, value := range values {
		// Create a complete new string. It will avoid to return an empty because the query has been closed too earlier.
		newLabelValue := model.LabelValue("" + value)
		labelValues = append(labelValues, newLabelValue)
	}
	return labelValues, nil
}

// ChangeDataSource is not used in this context because we are inside prometheus.
// So datasource/url doesn't mean anything and cannot be changed.
func (c *prometheusLangServerClient) ChangeDataSource(_ string) error {
	return nil
}

// GetURL is returning the url used to contact the prometheus server
// Since it is all call is internal, the URL is not needed.
func (c *prometheusLangServerClient) GetURL() string {
	return ""
}

// SetLookbackInterval is a method to use to change the interval that then will be used to retrieve data such as label and metrics from prometheus.
func (c *prometheusLangServerClient) SetLookbackInterval(interval time.Duration) {
	c.lookbackInterval = interval
}
