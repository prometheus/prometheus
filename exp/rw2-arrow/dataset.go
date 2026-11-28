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

package rw2arrow

import (
	"fmt"

	"github.com/prometheus/prometheus/model/labels"
)

// SamplePoint represents a single timestamped float64 sample.
type SamplePoint struct {
	Timestamp int64
	Value     float64
}

// SeriesData represents a Prometheus time series with its labels and samples.
type SeriesData struct {
	Labels  labels.Labels
	Samples []SamplePoint
}

// GenerateBatch creates a realistic synthetic batch of numSeries time series,
// each with samplesPerSeries samples, mimicking Prometheus scrape targets.
func GenerateBatch(numSeries, samplesPerSeries int) []SeriesData {
	metricNames := []string{
		"http_requests_total",
		"http_request_duration_seconds_count",
		"http_request_duration_seconds_sum",
		"grpc_server_handled_total",
		"process_cpu_seconds_total",
	}
	methods := []string{"GET", "POST", "PUT", "DELETE"}
	statuses := []string{"200", "201", "400", "404", "500"}
	handlers := []string{"/api/v1/query", "/api/v1/query_range", "/api/v1/write", "/metrics", "/healthz"}

	baseTS := int64(1700000000000)
	out := make([]SeriesData, numSeries)

	var sb labels.ScratchBuilder
	for i := range numSeries {
		sb.Reset()
		// Sorted label names as required by Prometheus labels.Labels.
		sb.Add("__name__", metricNames[i%len(metricNames)])
		sb.Add("handler", handlers[(i/5)%len(handlers)])
		sb.Add("instance", fmt.Sprintf("10.0.%d.%d:8080", (i/250)%255, i%250))
		sb.Add("job", fmt.Sprintf("service-%d", i%10))
		sb.Add("method", methods[i%len(methods)])
		sb.Add("status", statuses[(i/4)%len(statuses)])
		sb.Sort()

		samples := make([]SamplePoint, samplesPerSeries)
		for j := range samplesPerSeries {
			samples[j] = SamplePoint{
				Timestamp: baseTS + int64(j*15000),
				Value:     float64(i)*1.5 + float64(j)*0.25,
			}
		}

		out[i] = SeriesData{
			Labels:  sb.Labels(),
			Samples: samples,
		}
	}
	return out
}
