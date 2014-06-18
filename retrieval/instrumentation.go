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
	address = "instance"
	alive   = "alive"
	failure = "failure"

	outcome     = "outcome"
	state       = "state"
	success     = "success"
	unreachable = "unreachable"

	intervalKey = "interval"
)

var (
	targetOperationLatencies = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_target_operation_latency_ms",
			Help:       "The latencies for various target operations.",
			Objectives: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
		},
		[]string{address, outcome},
	)

	retrievalDurations = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_targetpool_duration_ms",
			Help:       "The durations for each TargetPool to retrieve state from all included entities.",
			Objectives: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
		},
		[]string{intervalKey},
	)

	dnsSDLookupsCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_dns_sd_lookups_total",
			Help: "The number of DNS-SD lookup successes/failures per pool.",
		},
		[]string{outcome},
	)
)

func recordOutcome(err error) {
	message := success
	if err != nil {
		message = failure
	}
	dnsSDLookupsCount.WithLabelValues(message).Inc()
}

func init() {
	prometheus.MustRegister(targetOperationLatencies)
	prometheus.MustRegister(retrievalDurations)
	prometheus.MustRegister(dnsSDLookupsCount)
}
