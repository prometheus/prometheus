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
	"strings"
	"unsafe"

	"github.com/prometheus/common/model"
)

const (
	// MetricName is a special label name and selector for MetricIdentity.Name.
	MetricName = "__name__"

	// metricType is a special label name and selector for MetricIdentity.Type.
	// Private to ensure __name__, __type__ and __unit__ are used together
	// and remain extensible in Prometheus. See Labels.MetricIdentity,
	// Builder.SetMetricIdentity and ScratchBuilder.AddMetricIdentity for access.
	metricType = "__type__"
	// MetricUnit is a special label name and selector for MetricIdentity.Unit,
	// which in the past used to be stored in metadata.
	// Private to ensure __name__, __type__ and __unit__ are used together
	// and remain extensible in Prometheus. See Labels.MetricIdentity,
	// Builder.SetMetricIdentity and ScratchBuilder.AddMetricIdentity for access.
	metricUnit = "__unit__"

	AlertName    = "alertname"
	BucketLabel  = "le"
	InstanceName = "instance"

	labelSep = '\xfe' // Used at beginning of `Bytes` return.
	sep      = '\xff' // Used between labels in `Bytes` and `Hash`.
)

// IsMetricIdentityLabel returns true if the given label name is a special
// metric identity label.
func IsMetricIdentityLabel(name string) bool {
	return name == MetricName || name == metricType || name == metricUnit
}

// MetricIdentity represents extended metric identity parts beyond the metric name.
// Each "time series" is identifiable by MetricIdentity and other labels e.g. job.
type MetricIdentity struct {
	// Name represents metric name (not always the same as metric family, until we
	// have native, structured metric representation for all types).
	// Empty means nameless metric (e.g. result of the PromQL function).
	Name string
	// Type, empty ("") is equivalent to model.UnknownMetricType.
	// In the past Prometheus used to be stored it n metadata.
	Type model.MetricType
	// Unit of the metric, regardless if encoded in the metric name. Empty means
	// unitless metric (e.g. result of the PromQL function).
	// In the past Prometheus used to be stored it n metadata.
	Unit string
}

func (m MetricIdentity) String() string {
	b := strings.Builder{}
	b.WriteString(m.Name)
	if m.Unit != "" {
		b.WriteString("~")
		b.WriteString(m.Unit)
	}
	if m.Type != "" && m.Type != model.MetricTypeUnknown {
		b.WriteString(".")
		b.WriteString(string(m.Type))
	}
	return b.String()
}

var seps = []byte{sep} // Used with Hash, which has no WriteByte method.

// Label is a key/value pair of strings.
type Label struct {
	Name, Value string
}

// MetricIdentity returns the metric identity parts.
func (ls Labels) MetricIdentity() MetricIdentity {
	typ := model.MetricTypeUnknown
	if got := ls.Get(metricType); got != "" {
		typ = model.MetricType(got)
	}
	return MetricIdentity{
		Name: ls.Get(MetricName),
		Type: typ,
		Unit: ls.Get(metricUnit),
	}
}

// SetMetricIdentity injects metric identity parts into labels.
// Empty fields of the given MetricIdentity (or unknown metric type), will
// cause removal of the existing part labels.
func (b *Builder) SetMetricIdentity(mid MetricIdentity) *Builder {
	b.Set(MetricName, mid.Name)
	if mid.Type == model.MetricTypeUnknown {
		// Unknown equals empty semantically, so remove the label on unknown too as per
		// method signature comment.
		mid.Type = ""
	}
	b.Set(metricType, string(mid.Type))
	b.Set(metricUnit, mid.Unit)
	return b
}

// IgnoreIdentityLabelsScratchBuilder is a wrapper over scratch builder
// that ignores subsequent additions of special metric identity labels.
type IgnoreIdentityLabelsScratchBuilder struct {
	*ScratchBuilder
}

// Add a name/value pair, unless it's a special metric identity label e.g. __name__, __type__, __unit__.
// Note if you Add the same name twice you will get a duplicate label, which is invalid.
func (b IgnoreIdentityLabelsScratchBuilder) Add(name, value string) {
	if IsMetricIdentityLabel(name) {
		return
	}
	b.ScratchBuilder.Add(name, value)
}

// AddMetricIdentity adds metric identity parts into labels.
// Empty fields of the given MetricIdentity (or unknown metric type), will be ignored.
//
//nolint:revive // unexported type
func (b *ScratchBuilder) AddMetricIdentity(mid MetricIdentity) {
	if mid.Name != "" {
		b.Add(MetricName, mid.Name)
	}
	if mid.Type != "" && mid.Type != model.MetricTypeUnknown {
		b.Add(metricType, string(mid.Type))
	}
	if mid.Unit != "" {
		b.Add(metricUnit, mid.Unit)
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
		for _, n := range ns {
			if l.Name == n {
				return
			}
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
