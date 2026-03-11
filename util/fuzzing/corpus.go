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

package fuzzing

import (
	"github.com/prometheus/prometheus/promql/promqltest"
)

// GetCorpusForFuzzParseMetricText returns the seed corpus for FuzzParseMetricText.
func GetCorpusForFuzzParseMetricText() [][]byte {
	return [][]byte{
		[]byte(""),
		[]byte("metric_name 1.0"),
		[]byte("# HELP metric_name help text\n# TYPE metric_name counter\nmetric_name 1.0"),
		[]byte("o { quantile = \"1.0\", a = \"b\" } 8.3835e-05"),
		[]byte("# HELP api_http_request_count The total number of HTTP requests.\n# TYPE api_http_request_count counter\nhttp_request_count{method=\"post\",code=\"200\"} 1027 1395066363000"),
		[]byte("msdos_file_access_time_ms{path=\"C:\\\\DIR\\\\FILE.TXT\",error=\"Cannot find file:\\n\\\"FILE.TXT\\\"\"} 1.234e3"),
		[]byte("metric_without_timestamp_and_labels 12.47"),
		[]byte("something_weird{problem=\"division by zero\"} +Inf -3982045"),
		[]byte("http_request_duration_seconds_bucket{le=\"+Inf\"} 144320"),
		[]byte("go_gc_duration_seconds{ quantile=\"0.9\", a=\"b\"} 8.3835e-05"),
		[]byte("go_gc_duration_seconds{ quantile=\"1.0\", a=\"b\" } 8.3835e-05"),
		[]byte("go_gc_duration_seconds{ quantile = \"1.0\", a = \"b\" } 8.3835e-05"),
	}
}

// GetCorpusForFuzzParseOpenMetric returns the seed corpus for FuzzParseOpenMetric.
func GetCorpusForFuzzParseOpenMetric() [][]byte {
	return [][]byte{
		[]byte(""),
		[]byte("# TYPE metric_name counter\nmetric_name_total 1.0"),
		[]byte("# HELP metric_name help text\n# TYPE metric_name counter\nmetric_name_total 1.0\n# EOF"),
	}
}

// GetCorpusForFuzzParseMetricSelector returns the seed corpus for FuzzParseMetricSelector.
func GetCorpusForFuzzParseMetricSelector() []string {
	return []string{
		"",
		"metric_name",
		`metric_name{label="value"}`,
		`{label="value"}`,
		`metric_name{label=~"val.*"}`,
	}
}

// GetCorpusForFuzzParseExpr returns the seed corpus for FuzzParseExpr.
func GetCorpusForFuzzParseExpr() ([]string, error) {
	// Get built-in test expressions.
	builtInExprs, err := promqltest.GetBuiltInExprs()
	if err != nil {
		return nil, err
	}

	// Add additional seed corpus.
	additionalExprs := []string{
		"",
		"1",
		"metric_name",
		`"str"`,
		// Numeric literals
		".5",
		"5.",
		"123.4567",
		"5e3",
		"5e-3",
		"+5.5e-3",
		"0xc",
		"0755",
		"-0755",
		"+Inf",
		"-Inf",
		// Basic binary operations
		"1 + 1",
		"1 - 1",
		"1 * 1",
		"1 / 1",
		"1 % 1",
		// Comparison operators
		"1 == 1",
		"1 != 1",
		"1 > 1",
		"1 >= 1",
		"1 < 1",
		"1 <= 1",
		// Operations with identifiers
		"foo == 1",
		"foo * bar",
		"2.5 / bar",
		"foo and bar",
		"foo or bar",
		// Complex expressions
		"+1 + -2 * 1",
		"1 + 2/(3*1)",
		// Comment
		"#comment",
	}

	return append(builtInExprs, additionalExprs...), nil
}
