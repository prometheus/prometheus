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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestMetadata_IsEmpty(t *testing.T) {
	for _, tt := range []struct {
		name     string
		m        Metadata
		expected bool
	}{
		{
			name: "empty struct", expected: true,
		},
		{
			name: "unknown type with empty fields", expected: true,
			m: Metadata{Type: model.MetricTypeUnknown},
		},
		{
			name: "type", expected: false,
			m: Metadata{Type: model.MetricTypeCounter},
		},
		{
			name: "unit", expected: false,
			m: Metadata{Unit: "seconds"},
		},
		{
			name: "help", expected: false,
			m: Metadata{Help: "help text"},
		},
		{
			name: "unknown type with help", expected: false,
			m: Metadata{Type: model.MetricTypeUnknown, Help: "help text"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.m.IsEmpty())
		})
	}
}

func TestMetadata_Equals(t *testing.T) {
	for _, tt := range []struct {
		name     string
		m        Metadata
		other    Metadata
		expected bool
	}{
		{
			name: "same empty", expected: true,
		},
		{
			name: "same", expected: true,
			m:     Metadata{Type: model.MetricTypeCounter, Unit: "s", Help: "doc"},
			other: Metadata{Type: model.MetricTypeCounter, Unit: "s", Help: "doc"},
		},
		{
			name: "same unknown type", expected: true,
			m:     Metadata{Type: model.MetricTypeUnknown, Unit: "s", Help: "doc"},
			other: Metadata{Type: model.MetricTypeUnknown, Unit: "s", Help: "doc"},
		},
		{
			name: "same mixed unknown type", expected: true,
			m:     Metadata{Type: "", Unit: "s", Help: "doc"},
			other: Metadata{Type: model.MetricTypeUnknown, Unit: "s", Help: "doc"},
		},
		{
			name: "different unit", expected: false,
			m:     Metadata{Type: model.MetricTypeCounter, Unit: "s", Help: "doc"},
			other: Metadata{Type: model.MetricTypeCounter, Unit: "bytes", Help: "doc"},
		},
		{
			name: "different help", expected: false,
			m:     Metadata{Type: model.MetricTypeCounter, Unit: "s", Help: "doc"},
			other: Metadata{Type: model.MetricTypeCounter, Unit: "s", Help: "other doc"},
		},
		{
			name: "different type", expected: false,
			m:     Metadata{Type: model.MetricTypeCounter, Unit: "s", Help: "doc"},
			other: Metadata{Type: model.MetricTypeGauge, Unit: "s", Help: "doc"},
		},
		{
			name: "different type with unknown", expected: false,
			m:     Metadata{Type: model.MetricTypeUnknown, Unit: "s", Help: "doc"},
			other: Metadata{Type: model.MetricTypeCounter, Unit: "s", Help: "doc"},
		},
		{
			name: "different type with empty", expected: false,
			m:     Metadata{Type: "", Unit: "s", Help: "doc"},
			other: Metadata{Type: model.MetricTypeCounter, Unit: "s", Help: "doc"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.Equals(tt.other); got != tt.expected {
				t.Errorf("Metadata.Equals() = %v, expected %v", got, tt.expected)
			}
		})
	}
}
