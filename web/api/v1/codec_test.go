// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"testing"

	"github.com/munnerz/goautoneg"
	"github.com/stretchr/testify/require"
)

func TestMIMEType_String(t *testing.T) {
	m := MIMEType{Type: "application", SubType: "json"}

	require.Equal(t, "application/json", m.String())
}

func TestMIMEType_Satisfies(t *testing.T) {
	m := MIMEType{Type: "application", SubType: "json"}

	scenarios := map[string]struct {
		accept   goautoneg.Accept
		expected bool
	}{
		"exact match": {
			accept:   goautoneg.Accept{Type: "application", SubType: "json"},
			expected: true,
		},
		"sub-type wildcard match": {
			accept:   goautoneg.Accept{Type: "application", SubType: "*"},
			expected: true,
		},
		"full wildcard match": {
			accept:   goautoneg.Accept{Type: "*", SubType: "*"},
			expected: true,
		},
		"inverted": {
			accept:   goautoneg.Accept{Type: "json", SubType: "application"},
			expected: false,
		},
		"inverted sub-type wildcard": {
			accept:   goautoneg.Accept{Type: "json", SubType: "*"},
			expected: false,
		},
		"complete mismatch": {
			accept:   goautoneg.Accept{Type: "text", SubType: "plain"},
			expected: false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			actual := m.Satisfies(scenario.accept)
			require.Equal(t, scenario.expected, actual)
		})
	}
}
