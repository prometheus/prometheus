// Copyright The Prometheus Authors
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

package metadata

import (
	"strings"

	"github.com/prometheus/common/model"
)

// Metadata stores a series' metadata information.
type Metadata struct {
	Type model.MetricType `json:"type"`
	Unit string           `json:"unit"`
	Help string           `json:"help"`
}

// IsEmpty returns true if metadata structure is empty, including unknown type case.
func (m Metadata) IsEmpty() bool {
	return (m.Type == "" || m.Type == model.MetricTypeUnknown) && m.Unit == "" && m.Help == ""
}

// Equals returns true if m is semantically the same as other metadata.
func (m Metadata) Equals(other Metadata) bool {
	if strings.Compare(m.Unit, other.Unit) != 0 || strings.Compare(m.Help, other.Help) != 0 {
		return false
	}

	// Unknown means the same as empty string.
	if m.Type == "" || m.Type == model.MetricTypeUnknown {
		return other.Type == "" || other.Type == model.MetricTypeUnknown
	}
	return m.Type == other.Type
}
