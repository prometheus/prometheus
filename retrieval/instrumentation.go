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
	"github.com/matttproud/golang_instrumentation"
	"github.com/matttproud/golang_instrumentation/maths"
	"github.com/matttproud/golang_instrumentation/metrics"
)

var (
	networkLatencyHistogram = &metrics.HistogramSpecification{
		Starts:                metrics.LogarithmicSizedBucketsFor(0, 1000),
		BucketMaker:           metrics.AccumulatingBucketBuilder(metrics.EvictAndReplaceWith(10, maths.Average), 100),
		ReportablePercentiles: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
	}

	targetsHealthy   = &metrics.CounterMetric{}
	targetsUnhealthy = &metrics.CounterMetric{}

	scrapeLatencyHealthy   = metrics.CreateHistogram(networkLatencyHistogram)
	scrapeLatencyUnhealthy = metrics.CreateHistogram(networkLatencyHistogram)
)

func init() {
	registry.Register("targets_healthy_total", targetsHealthy)
	registry.Register("targets_unhealthy_total", targetsUnhealthy)
	registry.Register("targets_healthy_scrape_latency_ms", scrapeLatencyHealthy)
	registry.Register("targets_unhealthy_scrape_latency_ms", scrapeLatencyUnhealthy)
}
