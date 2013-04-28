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

package retrieval

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	address     = "instance"
	alive       = "alive"
	failure     = "failure"
	outcome     = "outcome"
	state       = "state"
	success     = "success"
	unreachable = "unreachable"
)

var (
	networkLatencyHistogram = &prometheus.HistogramSpecification{
		Starts:                prometheus.LogarithmicSizedBucketsFor(0, 1000),
		BucketBuilder:         prometheus.AccumulatingBucketBuilder(prometheus.EvictAndReplaceWith(10, prometheus.AverageReducer), 100),
		ReportablePercentiles: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
	}

	targetOperationLatencies = prometheus.NewHistogram(networkLatencyHistogram)

	retrievalDurations = prometheus.NewHistogram(&prometheus.HistogramSpecification{
		Starts:                prometheus.LogarithmicSizedBucketsFor(0, 10000),
		BucketBuilder:         prometheus.AccumulatingBucketBuilder(prometheus.EvictAndReplaceWith(10, prometheus.AverageReducer), 100),
		ReportablePercentiles: []float64{0.01, 0.05, 0.5, 0.90, 0.99}})

	targetOperations = prometheus.NewCounter()
)

func init() {
	prometheus.Register("prometheus_target_operations_total", "The total numbers of operations of the various targets that are being monitored.", prometheus.NilLabels, targetOperations)
	prometheus.Register("prometheus_target_operation_latency_ms", "The latencies for various target operations.", prometheus.NilLabels, targetOperationLatencies)
	prometheus.Register("prometheus_targetpool_duration_ms", "The durations for each TargetPool to retrieve state from all included entities.", prometheus.NilLabels, retrievalDurations)
}
