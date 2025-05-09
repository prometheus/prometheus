// Copyright 2017 The Prometheus Authors
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

package labels

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"unsafe"

	"github.com/prometheus/common/model"
)

const (
	// MetricName is a special label name and selector for MetricDescriptor.Name.
	MetricName = "__name__"

	// metricType is a special label name and selector for MetricDescriptor.Type.
	// It's currently private to ensure __name__, __type__ and __unit__ are used
	// together and remain extensible in Prometheus. See Labels.GetMetricDescriptor,
	// Builder.SetMetricDescriptor and ScratchBuilder.AddMetricDescriptor for access.
	metricType = "__type__"
	// MetricUnit is a special label name and selector for MetricDescriptor.Unit.
	// It's currently private to ensure __name__, __type__ and __unit__ are used
	// together and remain extensible in Prometheus. See Labels.GetMetricDescriptor,
	//	// Builder.SetMetricDescriptor and ScratchBuilder.AddMetricDescriptor for access.
	metricUnit = "__unit__"

	AlertName    = "alertname"
	BucketLabel  = "le"
	InstanceName = "instance"

	labelSep = '\xfe' // Used at beginning of `Bytes` return.
	sep      = '\xff' // Used between labels in `Bytes` and `Hash`.
)

// IsMetricDescriptorLabel returns true if the given label name is a special
// metric descriptor label.
func IsMetricDescriptorLabel(name string) bool {
	return name == MetricName || name == metricType || name == metricUnit
}

// MetricDescriptor represents the basic metric schema information like metric
// name, type and unit that contribute to the general metric/timeseries identity.
//
// Historically, similar information was encoded in the MetricName and in the separate
// metadata structures. However with the type-and-unit-label feature, this information
// can be now stored directly in the special metric descriptor labels, which offers
// better reliability (e.g. atomicity), compatibility and, in many cases, efficiency.
//
// NOTE: MetricDescriptor in the current form is generally similar to the MetricFamily
// definition in OpenMetrics (https://prometheus.io/docs/specs/om/open_metrics_spec/#metricfamily).
// However, there is a small and important distinction around the MetricName semantics
// for the "classic" representation of complex metrics like histograms: the
// MetricDescriptor.Name follows the __name__ semantics. See Name for details.
type MetricDescriptor struct {
	// Name represents the final metric name for a Prometheus series.
	// NOTE(bwplotka): Prometheus scrape formats (e.g. OpenMetrics) define
	// the "metric family name". The MetricDescriptor.Name (so __name__ label) is not
	// always the same as the MetricFamily.Name e.g.:
	// * OpenMetrics metric family name on scrape: "acme_http_router_request_seconds"
	// * Resulting Prometheus metric name: "acme_http_router_request_seconds_sum"
	//
	// Empty string means nameless metric (e.g. result of the PromQL function).
	Name string
	// Type represents metric type. Empty value ("") is equivalent to
	// model.UnknownMetricType.
	Type model.MetricType
	// Unit represents the metric unit. Empty string means an unitless metric (e.g.
	// result of the PromQL function).
	//
	// NOTE: Currently unit value is not strictly defined other than OpenMetrics
	// recommendations: https://prometheus.io/docs/specs/om/open_metrics_spec/#units-and-base-units
	// TODO(bwplotka): Consider a stricter validation and rules e.g. lowercase only or UCUM standard.
	// Read more in https://github.com/prometheus/proposals/blob/main/proposals/2024-09-25_metadata-labels.md#more-strict-unit-and-type-value-definition
	Unit string
}

var seps = []byte{sep} // Used with Hash, which has no WriteByte method.

// Label is a key/value a pair of strings.
type Label struct {
	Name, Value string
}

// GetMetricDescriptor returns the metric descriptor information from the labels.
func (ls Labels) GetMetricDescriptor() MetricDescriptor {
	typ := model.MetricTypeUnknown
	if got := ls.Get(metricType); got != "" {
		typ = model.MetricType(got)
	}
	return MetricDescriptor{
		Name: ls.Get(MetricName),
		Type: typ,
		Unit: ls.Get(metricUnit),
	}
}

// SetMetricDescriptor injects the metric descriptor information into labels.
// It follows the Set semantics, so empty MetricDescriptor fields (or
// unknown metric type) will cause removal of the existing part labels.
func (b *Builder) SetMetricDescriptor(md MetricDescriptor) *Builder {
	b.Set(MetricName, md.Name)
	if md.Type == model.MetricTypeUnknown {
		// Unknown equals empty semantically, so remove the label on unknown too as per
		// method signature comment.
		md.Type = ""
	}
	b.Set(metricType, string(md.Type))
	b.Set(metricUnit, md.Unit)
	return b
}

// IgnoreMetricDescriptorLabelsScratchBuilder is a wrapper over scratch builder
// that ignores subsequent additions of special metric descriptor labels.
type IgnoreMetricDescriptorLabelsScratchBuilder struct {
	*ScratchBuilder
}

// Add a name/value pair, unless it's a special metric descriptor label e.g. __name__, __type__, __unit__.
// Note if you Add the same name twice you will get a duplicate label, which is invalid.
func (b IgnoreMetricDescriptorLabelsScratchBuilder) Add(name, value string) {
	if IsMetricDescriptorLabel(name) {
		return
	}
	b.ScratchBuilder.Add(name, value)
}

// AddMetricDescriptor adds metric descriptor information into labels.
// Empty fields of the given MetricDescriptor (or unknown metric type), will be ignored.
//
//nolint:revive // unexported type
func (b *ScratchBuilder) AddMetricDescriptor(md MetricDescriptor) {
	if md.Name != "" {
		b.Add(MetricName, md.Name)
	}
	if md.Type != "" && md.Type != model.MetricTypeUnknown {
		b.Add(metricType, string(md.Type))
	}
	if md.Unit != "" {
		b.Add(metricUnit, md.Unit)
	}
}

func (ls Labels) String() string {
	var bytea [1024]byte // On stack to avoid memory allocation while building the output.
	b := bytes.NewBuffer(bytea[:0])

	b.WriteByte('{')
	i := 0
	ls.Range(func(l Label) {
		if i > 0 {
			b.WriteByte(',')
			b.WriteByte(' ')
		}
		if !model.LabelName(l.Name).IsValidLegacy() {
			b.Write(strconv.AppendQuote(b.AvailableBuffer(), l.Name))
		} else {
			b.WriteString(l.Name)
		}
		b.WriteByte('=')
		b.Write(strconv.AppendQuote(b.AvailableBuffer(), l.Value))
		i++
	})
	b.WriteByte('}')
	return b.String()
}

// MarshalJSON implements json.Marshaler.
func (ls Labels) MarshalJSON() ([]byte, error) {
	return json.Marshal(ls.Map())
}

// UnmarshalJSON implements json.Unmarshaler.
func (ls *Labels) UnmarshalJSON(b []byte) error {
	var m map[string]string

	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	*ls = FromMap(m)
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (ls Labels) MarshalYAML() (interface{}, error) {
	return ls.Map(), nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (ls *Labels) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var m map[string]string

	if err := unmarshal(&m); err != nil {
		return err
	}

	*ls = FromMap(m)
	return nil
}

// IsValid checks if the metric name or label names are valid.
func (ls Labels) IsValid(validationScheme model.ValidationScheme) bool {
	err := ls.Validate(func(l Label) error {
		if l.Name == model.MetricNameLabel {
			// If the default validation scheme has been overridden with legacy mode,
			// we need to call the special legacy validation checker.
			if validationScheme == model.LegacyValidation && !model.IsValidLegacyMetricName(string(model.LabelValue(l.Value))) {
				return strconv.ErrSyntax
			}
			if !model.IsValidMetricName(model.LabelValue(l.Value)) {
				return strconv.ErrSyntax
			}
		}
		if validationScheme == model.LegacyValidation {
			if !model.LabelName(l.Name).IsValidLegacy() || !model.LabelValue(l.Value).IsValid() {
				return strconv.ErrSyntax
			}
		} else if !model.LabelName(l.Name).IsValid() || !model.LabelValue(l.Value).IsValid() {
			return strconv.ErrSyntax
		}
		return nil
	})
	return err == nil
}

// Map returns a string map of the labels.
func (ls Labels) Map() map[string]string {
	m := make(map[string]string)
	ls.Range(func(l Label) {
		m[l.Name] = l.Value
	})
	return m
}

// FromMap returns new sorted Labels from the given map.
func FromMap(m map[string]string) Labels {
	l := make([]Label, 0, len(m))
	for k, v := range m {
		l = append(l, Label{Name: k, Value: v})
	}
	return New(l...)
}

// NewBuilder returns a new LabelsBuilder.
func NewBuilder(base Labels) *Builder {
	b := &Builder{
		del: make([]string, 0, 5),
		add: make([]Label, 0, 5),
	}
	b.Reset(base)
	return b
}

// Del deletes the label of the given name.
func (b *Builder) Del(ns ...string) *Builder {
	for _, n := range ns {
		for i, a := range b.add {
			if a.Name == n {
				b.add = append(b.add[:i], b.add[i+1:]...)
			}
		}
		b.del = append(b.del, n)
	}
	return b
}

// Keep removes all labels from the base except those with the given names.
func (b *Builder) Keep(ns ...string) *Builder {
	b.base.Range(func(l Label) {
		if slices.Contains(ns, l.Name) {
			return
		}
		b.del = append(b.del, l.Name)
	})
	return b
}

// Set the name/value pair as a label. A value of "" means delete that label.
func (b *Builder) Set(n, v string) *Builder {
	if v == "" {
		// Empty labels are the same as missing labels.
		return b.Del(n)
	}
	for i, a := range b.add {
		if a.Name == n {
			b.add[i].Value = v
			return b
		}
	}
	b.add = append(b.add, Label{Name: n, Value: v})

	return b
}

func (b *Builder) Get(n string) string {
	// Del() removes entries from .add but Set() does not remove from .del, so check .add first.
	for _, a := range b.add {
		if a.Name == n {
			return a.Value
		}
	}
	if slices.Contains(b.del, n) {
		return ""
	}
	return b.base.Get(n)
}

// Range calls f on each label in the Builder.
func (b *Builder) Range(f func(l Label)) {
	// Stack-based arrays to avoid heap allocation in most cases.
	var addStack [128]Label
	var delStack [128]string
	// Take a copy of add and del, so they are unaffected by calls to Set() or Del().
	origAdd, origDel := append(addStack[:0], b.add...), append(delStack[:0], b.del...)
	b.base.Range(func(l Label) {
		if !slices.Contains(origDel, l.Name) && !contains(origAdd, l.Name) {
			f(l)
		}
	})
	for _, a := range origAdd {
		f(a)
	}
}

func contains(s []Label, n string) bool {
	for _, a := range s {
		if a.Name == n {
			return true
		}
	}
	return false
}

func yoloString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}
