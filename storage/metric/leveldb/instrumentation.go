// Copyright 2012 Prometheus Team
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

package leveldb

import (
	"github.com/matttproud/golang_instrumentation"
	"github.com/matttproud/golang_instrumentation/maths"
	"github.com/matttproud/golang_instrumentation/metrics"
)

var (
	diskLatencyHistogram = &metrics.HistogramSpecification{
		Starts:                metrics.LogarithmicSizedBucketsFor(0, 5000),
		BucketMaker:           metrics.AccumulatingBucketBuilder(metrics.EvictAndReplaceWith(10, maths.Average), 100),
		ReportablePercentiles: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
	}

	targetsHealthy = &metrics.CounterMetric{}

	appendLatency = metrics.CreateHistogram(diskLatencyHistogram)
)

func init() {
	registry.Register("sample_append_disk_latency_microseconds", appendLatency)
}
