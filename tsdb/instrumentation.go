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

package tsdb

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	result  = "result"
	success = "success"
	failure = "failure"
	dropped = "dropped"

	facet     = "facet"
	occupancy = "occupancy"
	capacity  = "capacity"
)

var (
	samplesCount = prometheus.NewCounter()
	sendLatency  = prometheus.NewDefaultHistogram()
	queueSize    = prometheus.NewGauge()
)

func recordOutcome(duration time.Duration, sampleCount int, err error) {
	labels := map[string]string{result: success}
	if err != nil {
		labels[result] = failure
	}

	samplesCount.IncrementBy(labels, float64(sampleCount))
	ms := float64(duration / time.Millisecond)
	sendLatency.Add(labels, ms)
}

func init() {
	prometheus.Register("prometheus_tsdb_sent_samples_total", "Total number of samples processed to be sent to TSDB.", prometheus.NilLabels, samplesCount)

	prometheus.Register("prometheus_tsdb_latency_ms", "Latency quantiles for sending samples to the TSDB in milliseconds.", prometheus.NilLabels, sendLatency)
	prometheus.Register("prometheus_tsdb_queue_size_total", "The size and capacity of the queue of samples to be sent to the TSDB.", prometheus.NilLabels, queueSize)
}
