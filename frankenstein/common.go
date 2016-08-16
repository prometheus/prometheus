// Copyright 2016 The Prometheus Authors
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

package frankenstein

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func errorCode(err error) string {
	if err == nil {
		return "200"
	}
	return "500"
}

// timeRequest runs 'f' and records how long it took in the given Prometheus
// metric. If 'f' returns successfully, record a "200". Otherwise, record
// "500".
//
// If you want more complicated logic for translating errors into statuses,
// use 'timeRequestStatus'.
func timeRequest(method string, metric *prometheus.SummaryVec, f func() error) error {
	return timeRequestMethodStatus(method, metric, errorCode, f)
}

// timeRequestStatus runs 'f' and records how long it took in the given
// Prometheus metric.
//
// toStatusCode is a function that translates errors returned by 'f' into
// HTTP-like status codes.
func timeRequestMethodStatus(method string, metric *prometheus.SummaryVec, toStatusCode func(error) string, f func() error) error {
	if toStatusCode == nil {
		toStatusCode = errorCode
	}
	startTime := time.Now()
	err := f()
	duration := time.Now().Sub(startTime)
	metric.WithLabelValues(method, toStatusCode(err)).Observe(float64(duration.Seconds()))
	return err
}

// TimeRequestHistogramMethodStatus runs 'f' and records how long it took in the given
// Prometheus histogram metric.
//
// toStatusCode is a function that translates errors returned by 'f' into
// HTTP-like status codes.
func timeRequestHistogramMethodStatus(method string, metric *prometheus.HistogramVec, toStatusCode func(error) string, f func() error) error {
	if toStatusCode == nil {
		toStatusCode = errorCode
	}
	startTime := time.Now()
	err := f()
	duration := time.Now().Sub(startTime)
	metric.WithLabelValues(method, toStatusCode(err)).Observe(duration.Seconds())
	return err
}

// TimeRequestHistogramStatus runs 'f' and records how long it took in the given
// Prometheus histogram metric.
//
// toStatusCode is a function that translates errors returned by 'f' into
// HTTP-like status codes.
func timeRequestHistogramStatus(metric *prometheus.HistogramVec, toStatusCode func(error) string, f func() error) error {
	if toStatusCode == nil {
		toStatusCode = errorCode
	}
	startTime := time.Now()
	err := f()
	duration := time.Now().Sub(startTime)
	metric.WithLabelValues(toStatusCode(err)).Observe(duration.Seconds())
	return err
}
