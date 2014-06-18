// Copyright 2013 Prometheus Team
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

package tiered

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	operation = "operation"
	success   = "success"
	failure   = "failure"
	result    = "result"

	appendSample                    = "append_sample"
	appendSamples                   = "append_samples"
	flushMemory                     = "flush_memory"
	getLabelValuesForLabelName      = "get_label_values_for_label_name"
	getFingerprintsForLabelMatchers = "get_fingerprints_for_label_matchers"
	getMetricForFingerprint         = "get_metric_for_fingerprint"
	hasIndexMetric                  = "has_index_metric"
	refreshHighWatermarks           = "refresh_high_watermarks"
	renderView                      = "render_view"

	cutOff        = "recency_threshold"
	processorName = "processor"
)

var (
	curationDurations = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_curation_durations_ms",
			Help:       "Histogram of time spent in curation (ms).",
			Objectives: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
		},
		[]string{cutOff, processorName, result},
	)
	curationFilterOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_curation_filter_operations_total",
			Help: "The number of curation filter operations completed.",
		},
		[]string{cutOff, processorName, result},
	)
	storageLatency = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_metric_disk_latency_microseconds",
			Help:       "Latency for metric disk operations in microseconds.",
			Objectives: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
		},
		[]string{operation, result},
	)
	storedSamplesCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_stored_samples_total",
		Help: "The number of samples that have been stored.",
	})
)

func recordOutcome(duration time.Duration, err error, success, failure prometheus.Labels) {
	labels := success
	if err != nil {
		labels = failure
	}

	storageLatency.With(labels).Observe(float64(duration / time.Microsecond))
}

func init() {
	prometheus.MustRegister(curationDurations)
	prometheus.MustRegister(curationFilterOperations)
	prometheus.MustRegister(storageLatency)
	prometheus.MustRegister(storedSamplesCount)
}
