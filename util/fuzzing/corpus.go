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
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
)

// GetCorpusForFuzzParseMetricText returns the seed corpus for FuzzParseMetricText.
func GetCorpusForFuzzParseMetricText() [][]byte {
	return [][]byte{
		[]byte(""),
		[]byte("metric_name 1.0"),
		[]byte("# HELP metric_name help text\n# TYPE metric_name counter\nmetric_name 1.0"),
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
	// Enable experimental features to parse all test expressions.
	parser.EnableExperimentalFunctions = true
	parser.ExperimentalDurationExpr = true
	parser.EnableExtendedRangeSelectors = true
	defer func() {
		parser.EnableExperimentalFunctions = false
		parser.ExperimentalDurationExpr = false
		parser.EnableExtendedRangeSelectors = false
	}()

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
	}

	return append(builtInExprs, additionalExprs...), nil
}
