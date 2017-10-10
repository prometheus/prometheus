// Copyright 2014 The Prometheus Authors
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

package prometheus

import (
	"errors"
)

// Counter is a Metric that represents a single numerical value that only ever
// goes up. That implies that it cannot be used to count items whose number can
// also go down, e.g. the number of currently running goroutines. Those
// "counters" are represented by Gauges.
//
// A Counter is typically used to count requests served, tasks completed, errors
// occurred, etc.
//
// To create Counter instances, use NewCounter.
type Counter interface {
	Metric
	Collector

	// Inc increments the counter by 1. Use Add to increment it by arbitrary
	// non-negative values.
	Inc()
	// Add adds the given value to the counter. It panics if the value is <
	// 0.
	Add(float64)
}

// CounterOpts is an alias for Opts. See there for doc comments.
type CounterOpts Opts

// NewCounter creates a new Counter based on the provided CounterOpts.
func NewCounter(opts CounterOpts) Counter {
	desc := NewDesc(
		BuildFQName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		nil,
		opts.ConstLabels,
	)
	result := &counter{value: value{desc: desc, valType: CounterValue, labelPairs: desc.constLabelPairs}}
	result.init(result) // Init self-collection.
	return result
}

type counter struct {
	value
}

func (c *counter) Add(v float64) {
	if v < 0 {
		panic(errors.New("counter cannot decrease in value"))
	}
	c.value.Add(v)
}

// CounterVec is a Collector that bundles a set of Counters that all share the
// same Desc, but have different values for their variable labels. This is used
// if you want to count the same thing partitioned by various dimensions
// (e.g. number of HTTP requests, partitioned by response code and
// method). Create instances with NewCounterVec.
type CounterVec struct {
	*metricVec
}

// NewCounterVec creates a new CounterVec based on the provided CounterOpts and
// partitioned by the given label names.
func NewCounterVec(opts CounterOpts, labelNames []string) *CounterVec {
	desc := NewDesc(
		BuildFQName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		labelNames,
		opts.ConstLabels,
	)
	return &CounterVec{
		metricVec: newMetricVec(desc, func(lvs ...string) Metric {
			result := &counter{value: value{
				desc:       desc,
				valType:    CounterValue,
				labelPairs: makeLabelPairs(desc, lvs),
			}}
			result.init(result) // Init self-collection.
			return result
		}),
	}
}

// GetMetricWithLabelValues returns the Counter for the given slice of label
// values (same order as the VariableLabels in Desc). If that combination of
// label values is accessed for the first time, a new Counter is created.
//
// It is possible to call this method without using the returned Counter to only
// create the new Counter but leave it at its starting value 0. See also the
// SummaryVec example.
//
// Keeping the Counter for later use is possible (and should be considered if
// performance is critical), but keep in mind that Reset, DeleteLabelValues and
// Delete can be used to delete the Counter from the CounterVec. In that case,
// the Counter will still exist, but it will not be exported anymore, even if a
// Counter with the same label values is created later.
//
// An error is returned if the number of label values is not the same as the
// number of VariableLabels in Desc.
//
// Note that for more than one label value, this method is prone to mistakes
// caused by an incorrect order of arguments. Consider GetMetricWith(Labels) as
// an alternative to avoid that type of mistake. For higher label numbers, the
// latter has a much more readable (albeit more verbose) syntax, but it comes
// with a performance overhead (for creating and processing the Labels map).
// See also the GaugeVec example.
func (m *CounterVec) GetMetricWithLabelValues(lvs ...string) (Counter, error) {
	metric, err := m.metricVec.getMetricWithLabelValues(lvs...)
	if metric != nil {
		return metric.(Counter), err
	}
	return nil, err
}

// GetMetricWith returns the Counter for the given Labels map (the label names
// must match those of the VariableLabels in Desc). If that label map is
// accessed for the first time, a new Counter is created. Implications of
// creating a Counter without using it and keeping the Counter for later use are
// the same as for GetMetricWithLabelValues.
//
// An error is returned if the number and names of the Labels are inconsistent
// with those of the VariableLabels in Desc.
//
// This method is used for the same purpose as
// GetMetricWithLabelValues(...string). See there for pros and cons of the two
// methods.
func (m *CounterVec) GetMetricWith(labels Labels) (Counter, error) {
	metric, err := m.metricVec.getMetricWith(labels)
	if metric != nil {
		return metric.(Counter), err
	}
	return nil, err
}

// WithLabelValues works as GetMetricWithLabelValues, but panics where
// GetMetricWithLabelValues would have returned an error. By not returning an
// error, WithLabelValues allows shortcuts like
//     myVec.WithLabelValues("404", "GET").Add(42)
func (m *CounterVec) WithLabelValues(lvs ...string) Counter {
	return m.metricVec.withLabelValues(lvs...).(Counter)
}

// With works as GetMetricWith, but panics where GetMetricWithLabels would have
// returned an error. By not returning an error, With allows shortcuts like
//     myVec.With(Labels{"code": "404", "method": "GET"}).Add(42)
func (m *CounterVec) With(labels Labels) Counter {
	return m.metricVec.with(labels).(Counter)
}

// CounterFunc is a Counter whose value is determined at collect time by calling a
// provided function.
//
// To create CounterFunc instances, use NewCounterFunc.
type CounterFunc interface {
	Metric
	Collector
}

// NewCounterFunc creates a new CounterFunc based on the provided
// CounterOpts. The value reported is determined by calling the given function
// from within the Write method. Take into account that metric collection may
// happen concurrently. If that results in concurrent calls to Write, like in
// the case where a CounterFunc is directly registered with Prometheus, the
// provided function must be concurrency-safe. The function should also honor
// the contract for a Counter (values only go up, not down), but compliance will
// not be checked.
func NewCounterFunc(opts CounterOpts, function func() float64) CounterFunc {
	return newValueFunc(NewDesc(
		BuildFQName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		nil,
		opts.ConstLabels,
	), CounterValue, function)
}
