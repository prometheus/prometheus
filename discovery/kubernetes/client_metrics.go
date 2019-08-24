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
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/metrics"
	"k8s.io/client-go/util/workqueue"
)

const (
	cacheMetricsNamespace     = metricsNamespace + "_cache"
	workqueueMetricsNamespace = metricsNamespace + "_workqueue"
)

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

	// Definition of metrics for client-go cache metrics provider.
	clientGoCacheListTotalMetric = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: cacheMetricsNamespace,
			Name:      "list_total",
			Help:      "Total number of list operations.",
		},
	)
	clientGoCacheListDurationMetric = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace:  cacheMetricsNamespace,
			Name:       "list_duration_seconds",
			Help:       "Duration of a Kubernetes API call in seconds.",
			Objectives: map[float64]float64{},
		},
	)
	clientGoCacheItemsInListCountMetric = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace:  cacheMetricsNamespace,
			Name:       "list_items",
			Help:       "Count of items in a list from the Kubernetes API.",
			Objectives: map[float64]float64{},
		},
	)
	clientGoCacheWatchesCountMetric = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: cacheMetricsNamespace,
			Name:      "watches_total",
			Help:      "Total number of watch operations.",
		},
	)
	clientGoCacheShortWatchesCountMetric = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: cacheMetricsNamespace,
			Name:      "short_watches_total",
			Help:      "Total number of short watch operations.",
		},
	)
	clientGoCacheWatchesDurationMetric = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace:  cacheMetricsNamespace,
			Name:       "watch_duration_seconds",
			Help:       "Duration of watches on the Kubernetes API.",
			Objectives: map[float64]float64{},
		},
	)
	clientGoCacheItemsInWatchesCountMetric = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace:  cacheMetricsNamespace,
			Name:       "watch_events",
			Help:       "Number of items in watches on the Kubernetes API.",
			Objectives: map[float64]float64{},
		},
	)
	clientGoCacheLastResourceVersionMetric = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: cacheMetricsNamespace,
			Name:      "last_resource_version",
			Help:      "Last resource version from the Kubernetes API.",
		},
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
	metrics.Register(f, f)
	registerer.MustRegister(
		clientGoRequestResultMetricVec,
		clientGoRequestLatencyMetricVec,
	)
}
func (clientGoRequestMetricAdapter) Increment(code string, method string, host string) {
	clientGoRequestResultMetricVec.WithLabelValues(code).Inc()
}
func (clientGoRequestMetricAdapter) Observe(verb string, u url.URL, latency time.Duration) {
	clientGoRequestLatencyMetricVec.WithLabelValues(u.EscapedPath()).Observe(latency.Seconds())
}

// Definition of client-go cache metrics provider definition
type clientGoCacheMetricsProvider struct{}

func (f *clientGoCacheMetricsProvider) Register(registerer prometheus.Registerer) {
	cache.SetReflectorMetricsProvider(f)
	registerer.MustRegister(
		clientGoCacheWatchesDurationMetric,
		clientGoCacheWatchesCountMetric,
		clientGoCacheListDurationMetric,
		clientGoCacheListTotalMetric,
		clientGoCacheLastResourceVersionMetric,
		clientGoCacheShortWatchesCountMetric,
		clientGoCacheItemsInWatchesCountMetric,
		clientGoCacheItemsInListCountMetric,
	)
}
func (clientGoCacheMetricsProvider) NewListsMetric(name string) cache.CounterMetric {
	return clientGoCacheListTotalMetric
}
func (clientGoCacheMetricsProvider) NewListDurationMetric(name string) cache.SummaryMetric {
	return clientGoCacheListDurationMetric
}
func (clientGoCacheMetricsProvider) NewItemsInListMetric(name string) cache.SummaryMetric {
	return clientGoCacheItemsInListCountMetric
}
func (clientGoCacheMetricsProvider) NewWatchesMetric(name string) cache.CounterMetric {
	return clientGoCacheWatchesCountMetric
}
func (clientGoCacheMetricsProvider) NewShortWatchesMetric(name string) cache.CounterMetric {
	return clientGoCacheShortWatchesCountMetric
}
func (clientGoCacheMetricsProvider) NewWatchDurationMetric(name string) cache.SummaryMetric {
	return clientGoCacheWatchesDurationMetric
}
func (clientGoCacheMetricsProvider) NewItemsInWatchMetric(name string) cache.SummaryMetric {
	return clientGoCacheItemsInWatchesCountMetric
}
func (clientGoCacheMetricsProvider) NewLastResourceVersionMetric(name string) cache.GaugeMetric {
	return clientGoCacheLastResourceVersionMetric
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
	metric := clientGoWorkqueueLatencyMetricVec.WithLabelValues(name)
	// Convert microseconds to seconds for consistency across metrics.
	return prometheus.ObserverFunc(func(v float64) {
		metric.Observe(v / 1e6)
	})
}
func (f *clientGoWorkqueueMetricsProvider) NewWorkDurationMetric(name string) workqueue.HistogramMetric {
	metric := clientGoWorkqueueWorkDurationMetricVec.WithLabelValues(name)
	// Convert microseconds to seconds for consistency across metrics.
	return prometheus.ObserverFunc(func(v float64) {
		metric.Observe(v / 1e6)
	})
}
func (f *clientGoWorkqueueMetricsProvider) NewUnfinishedWorkSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return clientGoWorkqueueUnfinishedWorkSecondsMetricVec.WithLabelValues(name)
}
func (f *clientGoWorkqueueMetricsProvider) NewLongestRunningProcessorSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return clientGoWorkqueueLongestRunningProcessorMetricVec.WithLabelValues(name)
}
func (clientGoWorkqueueMetricsProvider) NewRetriesMetric(name string) workqueue.CounterMetric {
	// Retries are not used so the metric is omitted.
	return noopMetric{}
}
func (clientGoWorkqueueMetricsProvider) NewDeprecatedDepthMetric(name string) workqueue.GaugeMetric {
	return noopMetric{}
}
func (clientGoWorkqueueMetricsProvider) NewDeprecatedAddsMetric(name string) workqueue.CounterMetric {
	return noopMetric{}
}
func (clientGoWorkqueueMetricsProvider) NewDeprecatedLatencyMetric(name string) workqueue.SummaryMetric {
	return noopMetric{}
}
func (f *clientGoWorkqueueMetricsProvider) NewDeprecatedWorkDurationMetric(name string) workqueue.SummaryMetric {
	return noopMetric{}
}
func (f *clientGoWorkqueueMetricsProvider) NewDeprecatedUnfinishedWorkSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return noopMetric{}
}
func (f *clientGoWorkqueueMetricsProvider) NewDeprecatedLongestRunningProcessorMicrosecondsMetric(name string) workqueue.SettableGaugeMetric {
	return noopMetric{}
}
func (clientGoWorkqueueMetricsProvider) NewDeprecatedRetriesMetric(name string) workqueue.CounterMetric {
	return noopMetric{}
}
