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

package metric

import "github.com/prometheus/common/model"

// Metric wraps a model.Metric and copies it upon modification if Copied is false.
type Metric struct {
	Copied bool
	Metric model.Metric
}

// Set sets a label name in the wrapped Metric to a given value and copies the
// Metric initially, if it is not already a copy.
func (m *Metric) Set(ln model.LabelName, lv model.LabelValue) {
	m.Copy()
	m.Metric[ln] = lv
}

// Del deletes a given label name from the wrapped Metric and copies the
// Metric initially, if it is not already a copy.
func (m *Metric) Del(ln model.LabelName) {
	m.Copy()
	delete(m.Metric, ln)
}

// Get the value for the given label name. An empty value is returned
// if the label does not exist in the metric.
func (m *Metric) Get(ln model.LabelName) model.LabelValue {
	return m.Metric[ln]
}

// Gets behaves as Get but the returned boolean is false iff the label
// does not exist.
func (m *Metric) Gets(ln model.LabelName) (model.LabelValue, bool) {
	lv, ok := m.Metric[ln]
	return lv, ok
}

// Copy the underlying Metric if it is not already a copy.
func (m *Metric) Copy() *Metric {
	if !m.Copied {
		m.Metric = m.Metric.Clone()
		m.Copied = true
	}
	return m
}

// String implements fmt.Stringer.
func (m Metric) String() string {
	return m.Metric.String()
}
