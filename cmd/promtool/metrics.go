// Copyright 2023 The Prometheus Authors
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

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/golang/snappy"
	dto "github.com/prometheus/client_model/go"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
)

// Push metrics to a prometheus remote write.
func PushMetrics(url *url.URL, roundTripper http.RoundTripper, headers map[string]string, timeout time.Duration, jobLabel string, files ...string) int {
	// remote write should respect specification: https://prometheus.io/docs/concepts/remote_write_spec/
	failed := false

	addressURL, err := url.Parse(url.String())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failureExitCode
	}

	// build remote write client
	writeClient, err := remote.NewWriteClient("remote-write", &remote.ClientConfig{
		URL:     &config_util.URL{URL: addressURL},
		Timeout: model.Duration(timeout),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failureExitCode
	}

	// set custom tls config from httpConfigFilePath
	// set custom headers to every request
	client, ok := writeClient.(*remote.Client)
	if !ok {
		fmt.Fprintln(os.Stderr, fmt.Errorf("unexpected type %T", writeClient))
		return failureExitCode
	}
	client.Client.Transport = &setHeadersTransport{
		RoundTripper: roundTripper,
		headers:      headers,
	}

	for _, f := range files {
		var data []byte
		var err error
		data, err = os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			continue
		}

		fmt.Printf("Parsing metric file %s\n", f)
		metricsData, err := parseMetricsTextAndFormat(bytes.NewReader(data), jobLabel)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			continue
		}

		raw, err := metricsData.Marshal()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			continue
		}

		// Encode the request body into snappy encoding.
		compressed := snappy.Encode(nil, raw)
		err = client.Store(context.Background(), compressed)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			continue
		}
		fmt.Printf("Successfully pushed metric file %s\n", f)
	}

	if failed {
		return failureExitCode
	}

	return successExitCode
}

type setHeadersTransport struct {
	http.RoundTripper
	headers map[string]string
}

func (s *setHeadersTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, value := range s.headers {
		req.Header.Set(key, value)
	}
	return s.RoundTripper.RoundTrip(req)
}

var MetricMetadataTypeValue = map[string]int32{
	"UNKNOWN":        0,
	"COUNTER":        1,
	"GAUGE":          2,
	"HISTOGRAM":      3,
	"GAUGEHISTOGRAM": 4,
	"SUMMARY":        5,
	"INFO":           6,
	"STATESET":       7,
}

// formatMetrics convert metric family to a writerequest
func formatMetrics(mf map[string]*dto.MetricFamily, jobLabel string) (*prompb.WriteRequest, error) {
	wr := &prompb.WriteRequest{}

	// build metric list
	sortedMetricNames := make([]string, 0, len(mf))
	for metric := range mf {
		sortedMetricNames = append(sortedMetricNames, metric)
	}
	// sort metrics name in lexicographical order
	sort.Strings(sortedMetricNames)

	for _, metricName := range sortedMetricNames {
		// Set metadata writerequest
		mtype := MetricMetadataTypeValue[mf[metricName].Type.String()]
		metadata := prompb.MetricMetadata{
			MetricFamilyName: mf[metricName].GetName(),
			Type:             prompb.MetricMetadata_MetricType(mtype),
			Help:             mf[metricName].GetHelp(),
		}
		wr.Metadata = append(wr.Metadata, metadata)

		for _, metric := range mf[metricName].Metric {
			var timeserie prompb.TimeSeries

			// build labels map
			labels := make(map[string]string, len(metric.Label)+2)
			labels[model.MetricNameLabel] = metricName
			labels[model.JobLabel] = jobLabel

			for _, label := range metric.Label {
				labelname := label.GetName()
				if labelname == model.JobLabel {
					labelname = fmt.Sprintf("%s%s", model.ExportedLabelPrefix, labelname)
				}
				labels[labelname] = label.GetValue()
			}

			// build labels name list
			sortedLabelNames := make([]string, 0, len(labels))
			for label := range labels {
				sortedLabelNames = append(sortedLabelNames, label)
			}
			// sort labels name in lexicographical order
			sort.Strings(sortedLabelNames)

			for _, label := range sortedLabelNames {
				timeserie.Labels = append(timeserie.Labels, prompb.Label{
					Name:  label,
					Value: labels[label],
				})
			}

			timeserie.Samples = []prompb.Sample{
				{
					Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
					Value:     getMetricsValue(metric),
				},
			}

			wr.Timeseries = append(wr.Timeseries, timeserie)
		}
	}
	return wr, nil
}

// parseMetricsTextReader consumes an io.Reader and returns the MetricFamily
func parseMetricsTextReader(input io.Reader) (map[string]*dto.MetricFamily, error) {
	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(input)
	if err != nil {
		return nil, err
	}
	return mf, nil
}

// getMetricsValue return the value of a timeserie without the need to give value type
func getMetricsValue(m *dto.Metric) float64 {
	switch {
	case m.Gauge != nil:
		return m.GetGauge().GetValue()
	case m.Counter != nil:
		return m.GetCounter().GetValue()
	case m.Untyped != nil:
		return m.GetUntyped().GetValue()
	default:
		return 0.
	}
}

// parseMetricsTextAndFormat return the data in the expected prometheus metrics write request format
func parseMetricsTextAndFormat(input io.Reader, jobLabel string) (*prompb.WriteRequest, error) {
	mf, err := parseMetricsTextReader(input)
	if err != nil {
		return nil, err
	}

	return formatMetrics(mf, jobLabel)
}
