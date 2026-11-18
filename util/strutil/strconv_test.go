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

package strutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type linkTest struct {
	expression        string
	expectedGraphLink string
	expectedTableLink string
}

var linkTests = []linkTest{
	{
		"sum(incoming_http_requests_total) by (system)",
		"/graph?g0.expr=sum%28incoming_http_requests_total%29+by+%28system%29&g0.tab=0",
		"/graph?g0.expr=sum%28incoming_http_requests_total%29+by+%28system%29&g0.tab=1",
	},
	{
		"sum(incoming_http_requests_total{system=\"trackmetadata\"})",
		"/graph?g0.expr=sum%28incoming_http_requests_total%7Bsystem%3D%22trackmetadata%22%7D%29&g0.tab=0",
		"/graph?g0.expr=sum%28incoming_http_requests_total%7Bsystem%3D%22trackmetadata%22%7D%29&g0.tab=1",
	},
	{
		"up",
		"/graph?g0.expr=up&g0.tab=0",
		"/graph?g0.expr=up&g0.tab=1",
	},
	{
		"rate(http_requests_total[5m])",
		"/graph?g0.expr=rate%28http_requests_total%5B5m%5D%29&g0.tab=0",
		"/graph?g0.expr=rate%28http_requests_total%5B5m%5D%29&g0.tab=1",
	},
	{
		"",
		"/graph?g0.expr=&g0.tab=0",
		"/graph?g0.expr=&g0.tab=1",
	},
	{
		"metric_name{label=\"value with spaces\"}",
		"/graph?g0.expr=metric_name%7Blabel%3D%22value+with+spaces%22%7D&g0.tab=0",
		"/graph?g0.expr=metric_name%7Blabel%3D%22value+with+spaces%22%7D&g0.tab=1",
	},
}

func TestLink(t *testing.T) {
	for _, tt := range linkTests {
		graphLink := GraphLinkForExpression(tt.expression)
		require.Equal(t, tt.expectedGraphLink, graphLink,
			"GraphLinkForExpression failed for expression (%#q)", tt.expression)

		tableLink := TableLinkForExpression(tt.expression)
		require.Equal(t, tt.expectedTableLink, tableLink,
			"TableLinkForExpression failed for expression (%#q)", tt.expression)
	}
}

func TestSanitizeLabelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid label name",
			input:    "fooClientLABEL",
			expected: "fooClientLABEL",
		},
		{
			name:     "label with special characters",
			input:    "barClient.LABEL$$##",
			expected: "barClient_LABEL____",
		},
		{
			name:     "label starting with digit",
			input:    "123label",
			expected: "123label",
		},
		{
			name:     "label with dashes",
			input:    "my-label-name",
			expected: "my_label_name",
		},
		{
			name:     "label with spaces",
			input:    "my label name",
			expected: "my_label_name",
		},
		{
			name:     "label with mixed case and numbers",
			input:    "Test123Label456",
			expected: "Test123Label456",
		},
		{
			name:     "label with unicode characters",
			input:    "test-ñ-ü-label",
			expected: "test_____label",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only underscores",
			input:    "___",
			expected: "___",
		},
		{
			name:     "label with colons",
			input:    "namespace:metric_name",
			expected: "namespace_metric_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := SanitizeLabelName(tt.input)
			require.Equal(t, tt.expected, actual, "SanitizeLabelName(%q) = %q, want %q", tt.input, actual, tt.expected)
		})
	}
}

func TestSanitizeFullLabelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid label name",
			input:    "fooClientLABEL",
			expected: "fooClientLABEL",
		},
		{
			name:     "label with special characters",
			input:    "barClient.LABEL$$##",
			expected: "barClient_LABEL____",
		},
		{
			name:     "label starting with digit",
			input:    "0zerothClient1LABEL",
			expected: "_zerothClient1LABEL",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "_",
		},
		{
			name:     "label starting with multiple digits",
			input:    "123abc",
			expected: "_23abc",
		},
		{
			name:     "label with dashes",
			input:    "my-label-name",
			expected: "my_label_name",
		},
		{
			name:     "label with spaces",
			input:    "my label name",
			expected: "my_label_name",
		},
		{
			name:     "label with numbers in middle",
			input:    "Test123Label456",
			expected: "Test123Label456",
		},
		{
			name:     "single underscore",
			input:    "_",
			expected: "_",
		},
		{
			name:     "label starting with underscore",
			input:    "_validLabel",
			expected: "_validLabel",
		},
		{
			name:     "label with colons",
			input:    "namespace:metric_name",
			expected: "namespace_metric_name",
		},
		{
			name:     "label with unicode characters",
			input:    "test-ñ-ü-label",
			expected: "test_____label",
		},
		{
			name:     "only digits",
			input:    "12345",
			expected: "_2345",
		},
		{
			name:     "label with mixed invalid characters at start",
			input:    "!@#test",
			expected: "___test",
		},
		{
			name:     "label with consecutive digits at start",
			input:    "0123test",
			expected: "_123test",
		},
		{
			name:     "invalid character at last position",
			input:    "valid_name-",
			expected: "valid_name_",
		},
		{
			name:     "single invalid character",
			input:    "-",
			expected: "_",
		},
		{
			name:     "single digit",
			input:    "7",
			expected: "_",
		},
		{
			name:     "invalid first character followed by digits",
			input:    "-1a2",
			expected: "_1a2",
		},
		{
			name:     "digits after an invalid character are kept",
			input:    "a-0b9",
			expected: "a_0b9",
		},
		{
			name:     "two byte rune",
			input:    "aéb",
			expected: "a_b",
		},
		{
			name:     "three byte rune",
			input:    "a€b",
			expected: "a_b",
		},
		{
			name:     "four byte rune",
			input:    "a😀b",
			expected: "a_b",
		},
		{
			name:     "multi byte rune at start",
			input:    "éabc",
			expected: "_abc",
		},
		{
			name:     "multi byte rune at end",
			input:    "abc😀",
			expected: "abc_",
		},
		{
			name:     "only multi byte runes",
			input:    "日本語",
			expected: "___",
		},
		{
			name:     "invalid UTF-8 byte in middle",
			input:    "a\xffb",
			expected: "a_b",
		},
		{
			name:     "invalid UTF-8 byte at start",
			input:    "\xffabc",
			expected: "_abc",
		},
		{
			name:     "invalid UTF-8 byte at end",
			input:    "abc\x80",
			expected: "abc_",
		},
		{
			name:     "truncated multi byte sequence at end",
			input:    "ab\xe2\x82",
			expected: "ab__",
		},
		{
			name:     "invalid UTF-8 followed by valid multi byte rune",
			input:    "a\xff€b",
			expected: "a__b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := SanitizeFullLabelName(tt.input)
			require.Equal(t, tt.expected, actual, "SanitizeFullLabelName(%q) = %q, want %q", tt.input, actual, tt.expected)
		})
	}
}

func BenchmarkSanitizeFullLabelName(b *testing.B) {
	cases := []struct {
		name  string
		input string
	}{
		{"valid_short", "http_requests_total"},
		{"valid_long", "container_cpu_usage_seconds_total_with_extended_dimensions_and_namespaces"},
		{"needs_sanitizing_short", "123-invalid.metric.name"},
		{"needs_sanitizing_long", "http-service.cluster.local:8080/metrics@prod-us-west-2/system-metric"},
	}

	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_ = SanitizeFullLabelName(bc.input)
			}
		})
	}
}
