// SPDX-License-Identifier: AGPL-3.0-only

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
