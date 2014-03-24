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

package metric

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	operation = "operation"
	success   = "success"
	failure   = "failure"
	result    = "result"

	appendSample               = "append_sample"
	appendSamples              = "append_samples"
	flushMemory                = "flush_memory"
	getLabelValuesForLabelName = "get_label_values_for_label_name"
	getFingerprintsForLabelSet = "get_fingerprints_for_labelset"
	getMetricForFingerprint    = "get_metric_for_fingerprint"
	hasIndexMetric             = "has_index_metric"
	hasLabelName               = "has_label_name"
	hasLabelPair               = "has_label_pair"
	refreshHighWatermarks      = "refresh_high_watermarks"
	renderView                 = "render_view"

	cutOff        = "recency_threshold"
	processorName = "processor"
)

var (
	diskLatencyHistogram = &prometheus.HistogramSpecification{
		Starts:                prometheus.LogarithmicSizedBucketsFor(0, 5000),
		BucketBuilder:         prometheus.AccumulatingBucketBuilder(prometheus.EvictAndReplaceWith(10, prometheus.AverageReducer), 100),
		ReportablePercentiles: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
	}

	curationDuration          = prometheus.NewCounter()
	curationDurations         = prometheus.NewHistogram(diskLatencyHistogram)
	curationFilterOperations  = prometheus.NewCounter()
	storageOperations         = prometheus.NewCounter()
	storageOperationDurations = prometheus.NewCounter()
	storageLatency            = prometheus.NewHistogram(diskLatencyHistogram)
	queueSizes                = prometheus.NewGauge()
	storedSamplesCount        = prometheus.NewCounter()
)

func recordOutcome(duration time.Duration, err error, success, failure map[string]string) {
	labels := success
	if err != nil {
		labels = failure
	}

	storageOperations.Increment(labels)
	asFloat := float64(duration / time.Microsecond)
	storageLatency.Add(labels, asFloat)
	storageOperationDurations.IncrementBy(labels, asFloat)
}

func init() {
	prometheus.Register("prometheus_metric_disk_operations_total", "Total number of metric-related disk operations.", prometheus.NilLabels, storageOperations)
	prometheus.Register("prometheus_metric_disk_latency_microseconds", "Latency for metric disk operations in microseconds.", prometheus.NilLabels, storageLatency)
	prometheus.Register("prometheus_storage_operation_time_total_microseconds", "The total time spent performing a given storage operation.", prometheus.NilLabels, storageOperationDurations)
	prometheus.Register("prometheus_storage_queue_sizes_total", "The various sizes and capacities of the storage queues.", prometheus.NilLabels, queueSizes)
	prometheus.Register("prometheus_curation_filter_operations_total", "The number of curation filter operations completed.", prometheus.NilLabels, curationFilterOperations)
	prometheus.Register("prometheus_curation_duration_ms_total", "The total time spent in curation (ms).", prometheus.NilLabels, curationDuration)
	prometheus.Register("prometheus_curation_durations_ms", "Histogram of time spent in curation (ms).", prometheus.NilLabels, curationDurations)
	prometheus.Register("prometheus_stored_samples_total", "The number of samples that have been stored.", prometheus.NilLabels, storedSamplesCount)
}
