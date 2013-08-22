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

	appendFingerprints          = "append_fingerprints"
	appendLabelNameFingerprint  = "append_label_name_fingerprint"
	appendLabelPairFingerprint  = "append_label_pair_fingerprint"
	appendSample                = "append_sample"
	appendSamples               = "append_samples"
	findUnindexedMetrics        = "find_unindexed_metrics"
	flushMemory                 = "flush_memory"
	getBoundaryValues           = "get_boundary_values"
	getFingerprintsForLabelName = "get_fingerprints_for_label_name"
	getFingerprintsForLabelSet  = "get_fingerprints_for_labelset"
	getLabelNameFingerprints    = "get_label_name_fingerprints"
	getMetricForFingerprint     = "get_metric_for_fingerprint"
	getRangeValues              = "get_range_values"
	getValueAtTime              = "get_value_at_time"
	hasIndexMetric              = "has_index_metric"
	hasLabelName                = "has_label_name"
	hasLabelPair                = "has_label_pair"
	indexFingerprints           = "index_fingerprints"
	indexLabelNames             = "index_label_names"
	indexLabelPairs             = "index_label_pairs"
	indexMetric                 = "index_metric"
	indexMetrics                = "index_metrics"
	rebuildDiskFrontier         = "rebuild_disk_frontier"
	refreshHighWatermarks       = "refresh_high_watermarks"
	refreshHighBlockWatermarks  = "refresh_high_block_watermarks"
	renderView                  = "render_view"
	setLabelNameFingerprints    = "set_label_name_fingerprints"
	setLabelPairFingerprints    = "set_label_pair_fingerprints"
	writeMemory                 = "write_memory"

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
	prometheus.Register("curation_filter_operations_total", "The number of curation filter operations completed.", prometheus.NilLabels, curationFilterOperations)
	prometheus.Register("curation_duration_ms_total", "The total time spent in curation (ms).", prometheus.NilLabels, curationDuration)
	prometheus.Register("curation_durations_ms", "Histogram of time spent in curation (ms).", prometheus.NilLabels, curationDurations)
}
