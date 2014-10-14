// Copyright 2014 Prometheus Team
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

package local

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
	numSeries = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_stored_series_count",
		Help: "The number of currently stored series.",
	})
	numSamples = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_stored_samples_total",
		Help: "The total number of stored samples.",
	})
	evictionDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_memory_eviction_duration_milliseconds",
		Help: "The duration of the last memory eviction iteration in milliseconds.",
	})
	purgeDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_storage_purge_duration_milliseconds",
		Help: "The duration of the last storage purge iteration in milliseconds.",
	})

	numTranscodes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_chunk_transcodes_total",
		Help: "The total number of chunk transcodes.",
	})
	numPinnedChunks = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_pinned_chunks_count",
		Help: "The current number of pinned chunks.",
	})

	persistLatencies = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "prometheus_persist_latency_milliseconds",
		Help: "A summary of latencies for persisting each chunk.",
	}, []string{outcome})
	persistQueueLength = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_persist_queue_length",
		Help: "The current number of chunks waiting in the persist queue.",
	})
	persistQueueCapacity = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_persist_queue_capacity",
		Help: "The total capacity of the persist queue.",
	})
)

func init() {
	prometheus.MustRegister(numSeries)
	prometheus.MustRegister(numSamples)
	prometheus.MustRegister(evictionDuration)
	prometheus.MustRegister(purgeDuration)
	prometheus.MustRegister(numTranscodes)
	prometheus.MustRegister(numPinnedChunks)
	prometheus.MustRegister(persistLatencies)
	prometheus.MustRegister(persistQueueLength)
	prometheus.MustRegister(persistQueueCapacity)

	persistQueueCapacity.Set(float64(persistQueueCap))
}
