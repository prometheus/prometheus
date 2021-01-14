// Copyright 2019 The Prometheus Authors
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

package exemplar

import (
	"github.com/prometheus/prometheus/pkg/labels"
)

// Exemplar is additional information associated with a time series.
type Exemplar struct {
	Labels labels.Labels `json:"labels"`
	Value  float64       `json:"value"`
	Ts     int64         `json:"timestamp"`
	HasTs  bool          `json:"hasTimestamp"`
}

// Equals compares if the exemplar e is the same as e2.
func (e Exemplar) Equals(e2 Exemplar) bool {
	if e.Labels.String() != e2.Labels.String() {
		return false
	}

	if e.Value != e2.Value {
		return false
	}

	return true
}
