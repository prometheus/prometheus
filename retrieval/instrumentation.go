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
	"github.com/prometheus/client_golang"
	"github.com/prometheus/client_golang/maths"
	"github.com/prometheus/client_golang/metrics"
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
	networkLatencyHistogram = &metrics.HistogramSpecification{
		Starts:                metrics.LogarithmicSizedBucketsFor(0, 1000),
		BucketBuilder:         metrics.AccumulatingBucketBuilder(metrics.EvictAndReplaceWith(10, maths.Average), 100),
		ReportablePercentiles: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
	}

	targetOperationLatencies = metrics.NewHistogram(networkLatencyHistogram)

	retrievalDurations = metrics.NewHistogram(&metrics.HistogramSpecification{
		Starts:                metrics.LogarithmicSizedBucketsFor(0, 10000),
		BucketBuilder:         metrics.AccumulatingBucketBuilder(metrics.EvictAndReplaceWith(10, maths.Average), 100),
		ReportablePercentiles: []float64{0.01, 0.05, 0.5, 0.90, 0.99}})

	targetOperations = metrics.NewCounter()
)

func init() {
	registry.Register("prometheus_target_operations_total", "The total numbers of operations of the various targets that are being monitored.", registry.NilLabels, targetOperations)
	registry.Register("prometheus_target_operation_latency_ms", "The latencies for various target operations.", registry.NilLabels, targetOperationLatencies)
	registry.Register("prometheus_targetpool_duration_ms", "The durations for each TargetPool to retrieve state from all included entities.", registry.NilLabels, retrievalDurations)
}
