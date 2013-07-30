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

package notification

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
	notificationsCount     = prometheus.NewCounter()
	notificationLatency    = prometheus.NewDefaultHistogram()
	notificationsQueueSize = prometheus.NewGauge()
)

func recordOutcome(duration time.Duration, err error) {
	labels := map[string]string{result: success}
	if err != nil {
		labels[result] = failure
	}

	notificationsCount.Increment(labels)
	ms := float64(duration / time.Millisecond)
	notificationLatency.Add(labels, ms)
}

func init() {
	prometheus.Register("prometheus_notifications_total", "Total number of processed alert notifications.", prometheus.NilLabels, notificationsCount)
	prometheus.Register("prometheus_notifications_latency_ms", "Latency quantiles for sending alert notifications in milliseconds.", prometheus.NilLabels, notificationLatency)
	prometheus.Register("prometheus_notifications_queue_size_total", "The size and capacity of the alert notification queue.", prometheus.NilLabels, notificationsQueueSize)
}
