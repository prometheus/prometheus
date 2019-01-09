/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workqueue

import (
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/clock"
)

// This file provides abstractions for setting the provider (e.g., prometheus)
// of metrics.

type queueMetrics interface {
	add(item t)
	get(item t)
	done(item t)
	updateUnfinishedWork()
}

// GaugeMetric represents a single numerical value that can arbitrarily go up
// and down.
type GaugeMetric interface {
	Inc()
	Dec()
}

// SettableGaugeMetric represents a single numerical value that can arbitrarily go up
// and down. (Separate from GaugeMetric to preserve backwards compatibility.)
type SettableGaugeMetric interface {
	Set(float64)
}

// CounterMetric represents a single numerical value that only ever
// goes up.
type CounterMetric interface {
	Inc()
}

// SummaryMetric captures individual observations.
type SummaryMetric interface {
	Observe(float64)
}

type noopMetric struct{}

func (noopMetric) Inc()            {}
func (noopMetric) Dec()            {}
func (noopMetric) Set(float64)     {}
func (noopMetric) Observe(float64) {}

// defaultQueueMetrics expects the caller to lock before setting any metrics.
type defaultQueueMetrics struct {
	clock clock.Clock

	// current depth of a workqueue
	depth GaugeMetric
	// total number of adds handled by a workqueue
	adds CounterMetric
	// how long an item stays in a workqueue
	latency SummaryMetric
	// how long processing an item from a workqueue takes
	workDuration         SummaryMetric
	addTimes             map[t]time.Time
	processingStartTimes map[t]time.Time

	// how long have current threads been working?
	unfinishedWorkSeconds   SettableGaugeMetric
	longestRunningProcessor SettableGaugeMetric
}

func (m *defaultQueueMetrics) add(item t) {
	if m == nil {
		return
	}

	m.adds.Inc()
	m.depth.Inc()
	if _, exists := m.addTimes[item]; !exists {
		m.addTimes[item] = m.clock.Now()
	}
}

func (m *defaultQueueMetrics) get(item t) {
	if m == nil {
		return
	}

	m.depth.Dec()
	m.processingStartTimes[item] = m.clock.Now()
	if startTime, exists := m.addTimes[item]; exists {
		m.latency.Observe(m.sinceInMicroseconds(startTime))
		delete(m.addTimes, item)
	}
}

func (m *defaultQueueMetrics) done(item t) {
	if m == nil {
		return
	}

	if startTime, exists := m.processingStartTimes[item]; exists {
		m.workDuration.Observe(m.sinceInMicroseconds(startTime))
		delete(m.processingStartTimes, item)
	}
}

func (m *defaultQueueMetrics) updateUnfinishedWork() {
	// Note that a summary metric would be better for this, but prometheus
	// doesn't seem to have non-hacky ways to reset the summary metrics.
	var total float64
	var oldest float64
	for _, t := range m.processingStartTimes {
		age := m.sinceInMicroseconds(t)
		total += age
		if age > oldest {
			oldest = age
		}
	}
	// Convert to seconds; microseconds is unhelpfully granular for this.
	total /= 1000000
	m.unfinishedWorkSeconds.Set(total)
	m.longestRunningProcessor.Set(oldest) // in microseconds.
}

type noMetrics struct{}

func (noMetrics) add(item t)            {}
func (noMetrics) get(item t)            {}
func (noMetrics) done(item t)           {}
func (noMetrics) updateUnfinishedWork() {}

// Gets the time since the specified start in microseconds.
func (m *defaultQueueMetrics) sinceInMicroseconds(start time.Time) float64 {
	return float64(m.clock.Since(start).Nanoseconds() / time.Microsecond.Nanoseconds())
}

type retryMetrics interface {
	retry()
}

type defaultRetryMetrics struct {
	retries CounterMetric
}

func (m *defaultRetryMetrics) retry() {
	if m == nil {
		return
	}

	m.retries.Inc()
}

// MetricsProvider generates various metrics used by the queue.
type MetricsProvider interface {
	NewDepthMetric(name string) GaugeMetric
	NewAddsMetric(name string) CounterMetric
	NewLatencyMetric(name string) SummaryMetric
	NewWorkDurationMetric(name string) SummaryMetric
	NewUnfinishedWorkSecondsMetric(name string) SettableGaugeMetric
	NewLongestRunningProcessorMicrosecondsMetric(name string) SettableGaugeMetric
	NewRetriesMetric(name string) CounterMetric
}

type noopMetricsProvider struct{}

func (_ noopMetricsProvider) NewDepthMetric(name string) GaugeMetric {
	return noopMetric{}
}

func (_ noopMetricsProvider) NewAddsMetric(name string) CounterMetric {
	return noopMetric{}
}

func (_ noopMetricsProvider) NewLatencyMetric(name string) SummaryMetric {
	return noopMetric{}
}

func (_ noopMetricsProvider) NewWorkDurationMetric(name string) SummaryMetric {
	return noopMetric{}
}

func (_ noopMetricsProvider) NewUnfinishedWorkSecondsMetric(name string) SettableGaugeMetric {
	return noopMetric{}
}

func (_ noopMetricsProvider) NewLongestRunningProcessorMicrosecondsMetric(name string) SettableGaugeMetric {
	return noopMetric{}
}

func (_ noopMetricsProvider) NewRetriesMetric(name string) CounterMetric {
	return noopMetric{}
}

var globalMetricsFactory = queueMetricsFactory{
	metricsProvider: noopMetricsProvider{},
}

type queueMetricsFactory struct {
	metricsProvider MetricsProvider

	onlyOnce sync.Once
}

func (f *queueMetricsFactory) setProvider(mp MetricsProvider) {
	f.onlyOnce.Do(func() {
		f.metricsProvider = mp
	})
}

func (f *queueMetricsFactory) newQueueMetrics(name string, clock clock.Clock) queueMetrics {
	mp := f.metricsProvider
	if len(name) == 0 || mp == (noopMetricsProvider{}) {
		return noMetrics{}
	}
	return &defaultQueueMetrics{
		clock:                   clock,
		depth:                   mp.NewDepthMetric(name),
		adds:                    mp.NewAddsMetric(name),
		latency:                 mp.NewLatencyMetric(name),
		workDuration:            mp.NewWorkDurationMetric(name),
		unfinishedWorkSeconds:   mp.NewUnfinishedWorkSecondsMetric(name),
		longestRunningProcessor: mp.NewLongestRunningProcessorMicrosecondsMetric(name),
		addTimes:                map[t]time.Time{},
		processingStartTimes:    map[t]time.Time{},
	}
}

func newRetryMetrics(name string) retryMetrics {
	var ret *defaultRetryMetrics
	if len(name) == 0 {
		return ret
	}
	return &defaultRetryMetrics{
		retries: globalMetricsFactory.metricsProvider.NewRetriesMetric(name),
	}
}

// SetProvider sets the metrics provider for all subsequently created work
// queues. Only the first call has an effect.
func SetProvider(metricsProvider MetricsProvider) {
	globalMetricsFactory.setProvider(metricsProvider)
}
