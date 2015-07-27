// Copyright 2013 The Prometheus Authors
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

package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

var separator = []byte{0}

// A Metric is similar to a LabelSet, but the key difference is that a Metric is
// a singleton and refers to one and only one stream of samples.
type Metric map[LabelName]LabelValue

// Equal compares the metrics.
func (m Metric) Equal(o Metric) bool {
	if len(m) != len(o) {
		return false
	}
	for ln, lv := range m {
		olv, ok := o[ln]
		if !ok {
			return false
		}
		if olv != lv {
			return false
		}
	}
	return true
}

// Before compares the metrics, using the following criteria:
//
// If m has fewer labels than o, it is before o. If it has more, it is not.
//
// If the number of labels is the same, the superset of all label names is
// sorted alphanumerically. The first differing label pair found in that order
// determines the outcome: If the label does not exist at all in m, then m is
// before o, and vice versa. Otherwise the label value is compared
// alphanumerically.
//
// If m and o are equal, the method returns false.
func (m Metric) Before(o Metric) bool {
	if len(m) < len(o) {
		return true
	}
	if len(m) > len(o) {
		return false
	}

	lns := make(LabelNames, 0, len(m)+len(o))
	for ln := range m {
		lns = append(lns, ln)
	}
	for ln := range o {
		lns = append(lns, ln)
	}
	// It's probably not worth it to de-dup lns.
	sort.Sort(lns)
	for _, ln := range lns {
		mlv, ok := m[ln]
		if !ok {
			return true
		}
		olv, ok := o[ln]
		if !ok {
			return false
		}
		if mlv < olv {
			return true
		}
		if mlv > olv {
			return false
		}
	}
	return false
}

// String implements Stringer.
func (m Metric) String() string {
	metricName, hasName := m[MetricNameLabel]
	numLabels := len(m) - 1
	if !hasName {
		numLabels = len(m)
	}
	labelStrings := make([]string, 0, numLabels)
	for label, value := range m {
		if label != MetricNameLabel {
			labelStrings = append(labelStrings, fmt.Sprintf("%s=%q", label, value))
		}
	}

	switch numLabels {
	case 0:
		if hasName {
			return string(metricName)
		}
		return "{}"
	default:
		sort.Strings(labelStrings)
		return fmt.Sprintf("%s{%s}", metricName, strings.Join(labelStrings, ", "))
	}
}

// Fingerprint returns a Metric's Fingerprint.
func (m Metric) Fingerprint() Fingerprint {
	return metricToFingerprint(m)
}

// FastFingerprint returns a Metric's Fingerprint calculated by a faster hashing
// algorithm, which is, however, more susceptible to hash collisions.
func (m Metric) FastFingerprint() Fingerprint {
	return metricToFastFingerprint(m)
}

// Clone returns a copy of the Metric.
func (m Metric) Clone() Metric {
	clone := Metric{}
	for k, v := range m {
		clone[k] = v
	}
	return clone
}

// MergeFromLabelSet merges a label set into this Metric, prefixing a collision
// prefix to the label names merged from the label set where required.
func (m Metric) MergeFromLabelSet(labels LabelSet, collisionPrefix LabelName) {
	for k, v := range labels {
		if collisionPrefix != "" {
			for {
				if _, exists := m[k]; !exists {
					break
				}
				k = collisionPrefix + k
			}
		}

		m[k] = v
	}
}

// COWMetric wraps a Metric to enable copy-on-write access patterns.
type COWMetric struct {
	Copied bool
	Metric Metric
}

// Set sets a label name in the wrapped Metric to a given value and copies the
// Metric initially, if it is not already a copy.
func (m *COWMetric) Set(ln LabelName, lv LabelValue) {
	m.doCOW()
	m.Metric[ln] = lv
}

// Delete deletes a given label name from the wrapped Metric and copies the
// Metric initially, if it is not already a copy.
func (m *COWMetric) Delete(ln LabelName) {
	m.doCOW()
	delete(m.Metric, ln)
}

// doCOW copies the underlying Metric if it is not already a copy.
func (m *COWMetric) doCOW() {
	if !m.Copied {
		m.Metric = m.Metric.Clone()
		m.Copied = true
	}
}

// String implements fmt.Stringer.
func (m COWMetric) String() string {
	return m.Metric.String()
}

// MarshalJSON implements json.Marshaler.
func (m COWMetric) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Metric)
}
