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

// Untyped is a Metric that represents a single numerical value that can
// arbitrarily go up and down.
//
// An Untyped metric works the same as a Gauge. The only difference is that to
// no type information is implied.
//
// To create Untyped instances, use NewUntyped.
//
// Deprecated: The Untyped type is deprecated because it doesn't make sense in
// direct instrumentation. If you need to mirror an external metric of unknown
// type (usually while writing exporters), Use MustNewConstMetric to create an
// untyped metric instance on the fly.
type Untyped interface {
	Metric
	Collector

	// Set sets the Untyped metric to an arbitrary value.
	Set(float64)
	// Inc increments the Untyped metric by 1.
	Inc()
	// Dec decrements the Untyped metric by 1.
	Dec()
	// Add adds the given value to the Untyped metric. (The value can be
	// negative, resulting in a decrease.)
	Add(float64)
	// Sub subtracts the given value from the Untyped metric. (The value can
	// be negative, resulting in an increase.)
	Sub(float64)
}

// UntypedOpts is an alias for Opts. See there for doc comments.
type UntypedOpts Opts

// NewUntyped creates a new Untyped metric from the provided UntypedOpts.
func NewUntyped(opts UntypedOpts) Untyped {
	return newValue(NewDesc(
		BuildFQName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		nil,
		opts.ConstLabels,
	), UntypedValue, 0)
}

// UntypedVec is a Collector that bundles a set of Untyped metrics that all
// share the same Desc, but have different values for their variable
// labels. This is used if you want to count the same thing partitioned by
// various dimensions. Create instances with NewUntypedVec.
//
// Deprecated: UntypedVec is deprecated for the same reasons as Untyped.
type UntypedVec struct {
	*metricVec
}

// NewUntypedVec creates a new UntypedVec based on the provided UntypedOpts and
// partitioned by the given label names.
func NewUntypedVec(opts UntypedOpts, labelNames []string) *UntypedVec {
	desc := NewDesc(
		BuildFQName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		labelNames,
		opts.ConstLabels,
	)
	return &UntypedVec{
		metricVec: newMetricVec(desc, func(lvs ...string) Metric {
			return newValue(desc, UntypedValue, 0, lvs...)
		}),
	}
}

// GetMetricWithLabelValues returns the Untyped for the given slice of label
// values (same order as the VariableLabels in Desc). If that combination of
// label values is accessed for the first time, a new Untyped is created.
//
// It is possible to call this method without using the returned Untyped to only
// create the new Untyped but leave it at its starting value 0. See also the
// SummaryVec example.
//
// Keeping the Untyped for later use is possible (and should be considered if
// performance is critical), but keep in mind that Reset, DeleteLabelValues and
// Delete can be used to delete the Untyped from the UntypedVec. In that case, the
// Untyped will still exist, but it will not be exported anymore, even if a
// Untyped with the same label values is created later. See also the CounterVec
// example.
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
func (m *UntypedVec) GetMetricWithLabelValues(lvs ...string) (Untyped, error) {
	metric, err := m.metricVec.getMetricWithLabelValues(lvs...)
	if metric != nil {
		return metric.(Untyped), err
	}
	return nil, err
}

// GetMetricWith returns the Untyped for the given Labels map (the label names
// must match those of the VariableLabels in Desc). If that label map is
// accessed for the first time, a new Untyped is created. Implications of
// creating a Untyped without using it and keeping the Untyped for later use are
// the same as for GetMetricWithLabelValues.
//
// An error is returned if the number and names of the Labels are inconsistent
// with those of the VariableLabels in Desc.
//
// This method is used for the same purpose as
// GetMetricWithLabelValues(...string). See there for pros and cons of the two
// methods.
func (m *UntypedVec) GetMetricWith(labels Labels) (Untyped, error) {
	metric, err := m.metricVec.getMetricWith(labels)
	if metric != nil {
		return metric.(Untyped), err
	}
	return nil, err
}

// WithLabelValues works as GetMetricWithLabelValues, but panics where
// GetMetricWithLabelValues would have returned an error. By not returning an
// error, WithLabelValues allows shortcuts like
//     myVec.WithLabelValues("404", "GET").Add(42)
func (m *UntypedVec) WithLabelValues(lvs ...string) Untyped {
	return m.metricVec.withLabelValues(lvs...).(Untyped)
}

// With works as GetMetricWith, but panics where GetMetricWithLabels would have
// returned an error. By not returning an error, With allows shortcuts like
//     myVec.With(Labels{"code": "404", "method": "GET"}).Add(42)
func (m *UntypedVec) With(labels Labels) Untyped {
	return m.metricVec.with(labels).(Untyped)
}

// UntypedFunc is an Untyped whose value is determined at collect time by
// calling a provided function.
//
// To create UntypedFunc instances, use NewUntypedFunc.
type UntypedFunc interface {
	Metric
	Collector
}

// NewUntypedFunc creates a new UntypedFunc based on the provided
// UntypedOpts. The value reported is determined by calling the given function
// from within the Write method. Take into account that metric collection may
// happen concurrently. If that results in concurrent calls to Write, like in
// the case where an UntypedFunc is directly registered with Prometheus, the
// provided function must be concurrency-safe.
func NewUntypedFunc(opts UntypedOpts, function func() float64) UntypedFunc {
	return newValueFunc(NewDesc(
		BuildFQName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		nil,
		opts.ConstLabels,
	), UntypedValue, function)
}
