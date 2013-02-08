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
	"github.com/prometheus/client_golang"
	"github.com/prometheus/client_golang/maths"
	"github.com/prometheus/client_golang/metrics"
	"time"
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
	indexMetric                 = "index_metric"
	setLabelNameFingerprints    = "set_label_name_fingerprints"
	setLabelPairFingerprints    = "set_label_pair_fingerprints"
)

var (
	diskLatencyHistogram = &metrics.HistogramSpecification{
		Starts:                metrics.LogarithmicSizedBucketsFor(0, 5000),
		BucketBuilder:         metrics.AccumulatingBucketBuilder(metrics.EvictAndReplaceWith(10, maths.Average), 100),
		ReportablePercentiles: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
	}

	storageOperations = metrics.NewCounter()
	storageLatency    = metrics.NewHistogram(diskLatencyHistogram)
)

func recordOutcome(counter metrics.Counter, latency metrics.Histogram, duration time.Duration, err error, success, failure map[string]string) {
	labels := success
	if err != nil {
		labels = failure
	}

	counter.Increment(labels)
	latency.Add(labels, float64(duration/time.Microsecond))
}

func init() {
	registry.Register("prometheus_metric_disk_operations_total", "Total number of metric-related disk operations.", registry.NilLabels, storageOperations)
	registry.Register("prometheus_metric_disk_latency_microseconds", "Latency for metric disk operations in microseconds.", registry.NilLabels, storageLatency)
}
