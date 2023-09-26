// Copyright 2018 The Prometheus Authors
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

package kubernetes

import (
	"context"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/tools/metrics"
	"k8s.io/client-go/util/workqueue"
)

const workqueueMetricsNamespace = metricsNamespace + "_workqueue"

var (
	// Metrics for client-go's HTTP requests.
	clientGoRequestResultMetricVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "http_request_total",
			Help:      "Total number of HTTP requests to the Kubernetes API by status code.",
		},
		[]string{"status_code"},
	)
	clientGoRequestLatencyMetricVec = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  metricsNamespace,
			Name:       "http_request_duration_seconds",
			Help:       "Summary of latencies for HTTP requests to the Kubernetes API by endpoint.",
			Objectives: map[float64]float64{},
		},
		[]string{"endpoint"},
	)

	// Definition of metrics for client-go workflow metrics provider
	clientGoWorkqueueDepthMetricVec = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: workqueueMetricsNamespace,
			Name:      "depth",
			Help:      "Current depth of the work queue.",
		},
		[]string{"queue_name"},
	)
	clientGoWorkqueueAddsMetricVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: workqueueMetricsNamespace,
			Name:      "items_total",
			Help:      "Total number of items added to the work queue.",
		},
		[]string{"queue_name"},
	)
	clientGoWorkqueueLatencyMetricVec = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  workqueueMetricsNamespace,
			Name:       "latency_seconds",
			Help:       "How long an item stays in the work queue.",
			Objectives: map[float64]float64{},
		},
		[]string{"queue_name"},
	)
	clientGoWorkqueueUnfinishedWorkSecondsMetricVec = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: workqueueMetricsNamespace,
			Name:      "unfinished_work_seconds",
			Help:      "How long an item has remained unfinished in the work queue.",
		},
		[]string{"queue_name"},
	)
	clientGoWorkqueueLongestRunningProcessorMetricVec = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: workqueueMetricsNamespace,
			Name:      "longest_running_processor_seconds",
			Help:      "Duration of the longest running processor in the work queue.",
		},
		[]string{"queue_name"},
	)
	clientGoWorkqueueWorkDurationMetricVec = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  workqueueMetricsNamespace,
			Name:       "work_duration_seconds",
			Help:       "How long processing an item from the work queue takes.",
			Objectives: map[float64]float64{},
		},
		[]string{"queue_name"},
	)
)

// Definition of dummy metric used as a placeholder if we don't want to observe some data.
type noopMetric struct{}

func (noopMetric) Inc()            {}
func (noopMetric) Dec()            {}
func (noopMetric) Observe(float64) {}
func (noopMetric) Set(float64)     {}

// Definition of client-go metrics adapters for HTTP requests observation
type clientGoRequestMetricAdapter struct{}

func (f *clientGoRequestMetricAdapter) Register(registerer prometheus.Registerer) {
	metrics.Register(
		metrics.RegisterOpts{
			RequestLatency: f,
			RequestResult:  f,
		},
	)
	registerer.MustRegister(
		clientGoRequestResultMetricVec,
		clientGoRequestLatencyMetricVec,
	)
}

func (clientGoRequestMetricAdapter) Increment(_ context.Context, code, _, _ string) {
	clientGoRequestResultMetricVec.WithLabelValues(code).Inc()
}

func (clientGoRequestMetricAdapter) Observe(_ context.Context, _ string, u url.URL, latency time.Duration) {
	clientGoRequestLatencyMetricVec.WithLabelValues(u.EscapedPath()).Observe(latency.Seconds())
}

// Definition of client-go workqueue metrics provider definition
type clientGoWorkqueueMetricsProvider struct{}

func (f *clientGoWorkqueueMetricsProvider) Register(registerer prometheus.Registerer) {
	workqueue.SetProvider(f)
	registerer.MustRegister(
		clientGoWorkqueueDepthMetricVec,
		clientGoWorkqueueAddsMetricVec,
		clientGoWorkqueueLatencyMetricVec,
		clientGoWorkqueueWorkDurationMetricVec,
		clientGoWorkqueueUnfinishedWorkSecondsMetricVec,
		clientGoWorkqueueLongestRunningProcessorMetricVec,
	)
}

func (f *clientGoWorkqueueMetricsProvider) NewDepthMetric(name string) workqueue.GaugeMetric {
	return clientGoWorkqueueDepthMetricVec.WithLabelValues(name)
}

func (f *clientGoWorkqueueMetricsProvider) NewAddsMetric(name string) workqueue.CounterMetric {
	return clientGoWorkqueueAddsMetricVec.WithLabelValues(name)
}

func (f *clientGoWorkqueueMetricsProvider) NewLatencyMetric(name string) workqueue.HistogramMetric {
	return clientGoWorkqueueLatencyMetricVec.WithLabelValues(name)
}

func (f *clientGoWorkqueueMetricsProvider) NewWorkDurationMetric(name string) workqueue.HistogramMetric {
	return clientGoWorkqueueWorkDurationMetricVec.WithLabelValues(name)
}

func (f *clientGoWorkqueueMetricsProvider) NewUnfinishedWorkSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return clientGoWorkqueueUnfinishedWorkSecondsMetricVec.WithLabelValues(name)
}

func (f *clientGoWorkqueueMetricsProvider) NewLongestRunningProcessorSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return clientGoWorkqueueLongestRunningProcessorMetricVec.WithLabelValues(name)
}

func (clientGoWorkqueueMetricsProvider) NewRetriesMetric(string) workqueue.CounterMetric {
	// Retries are not used so the metric is omitted.
	return noopMetric{}
}
