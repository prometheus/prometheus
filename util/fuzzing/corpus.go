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

// ChunkFuzzSeed is a seed corpus entry for FuzzXORChunk.
type ChunkFuzzSeed struct {
	// Seed is the RNG seed used to generate sample timestamps and values.
	Seed int64
	// N drives the sample count: count = int(N)%120 + 1.
	N uint8
	// NaNMask forces StaleNaN on specific samples: bit i set means sample i
	// uses StaleNaN instead of a random value.
	NaNMask uint64
}

// XOR2ChunkFuzzSeed is a seed corpus entry for FuzzXOR2Chunk.
type XOR2ChunkFuzzSeed struct {
	// Seed is the RNG seed used to generate sample timestamps and values.
	Seed int64
	// N drives the sample count: count = int(N)%120 + 1.
	N uint8
	// NaNMask forces StaleNaN on specific samples: bit i set means sample i
	// uses StaleNaN instead of a random value.
	NaNMask uint64
	// STMode selects the start-timestamp pattern used by the fuzzer.
	STMode uint8
}

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

// GetCorpusForFuzzXORChunk returns the seed corpus for FuzzXORChunk.
func GetCorpusForFuzzXORChunk() []ChunkFuzzSeed {
	return []ChunkFuzzSeed{
		// Basic cases: no StaleNaN.
		{Seed: 0, N: 0, NaNMask: 0},
		{Seed: 42, N: 2, NaNMask: 0},
		{Seed: 1234567890, N: 119, NaNMask: 0},
		// Single StaleNaN at first sample.
		{Seed: 0, N: 0, NaNMask: 0b1},
		// StaleNaN in the middle of a run.
		{Seed: 42, N: 4, NaNMask: 0b00100},
		// Alternating StaleNaN.
		{Seed: 1, N: 9, NaNMask: 0b0101010101},
		// All StaleNaN.
		{Seed: 7, N: 9, NaNMask: ^uint64(0)},
	}
}

// GetDictForFuzzParseExpr returns the libFuzzer dictionary tokens for
// FuzzParseExpr. Tokens are derived from the exported PromQL keyword list,
// function names, and operator symbols so that the dictionary stays in sync
// with the grammar automatically.
func GetDictForFuzzParseExpr() []string {
	seen := make(map[string]struct{})

	// All PromQL keywords (aggregators, modifiers, histogram descriptors, etc.).
	for _, kw := range parser.Keywords() {
		seen[kw] = struct{}{}
	}

	// All built-in function names.
	for name := range parser.Functions {
		seen[name] = struct{}{}
	}

	// Operator and syntax tokens from ItemTypeStr. The SPACE entry is a
	// display-only placeholder ("<space>"), not an actual token, so remove it.
	for _, s := range parser.ItemTypeStr {
		seen[s] = struct{}{}
	}
	delete(seen, parser.ItemTypeStr[parser.SPACE])

	// Special numeric literals not covered by the keyword map.
	for _, s := range []string{"+Inf", "-Inf", "NaN"} {
		seen[s] = struct{}{}
	}

	result := make([]string, 0, len(seen))
	for s := range seen {
		result = append(result, s)
	}
	return result
}

// GetCorpusForFuzzXOR2Chunk returns the seed corpus for FuzzXOR2Chunk.
func GetCorpusForFuzzXOR2Chunk() []XOR2ChunkFuzzSeed {
	return []XOR2ChunkFuzzSeed{
		// No ST at all.
		{Seed: 0, N: 0, NaNMask: 0, STMode: 0},
		{Seed: 1234567890, N: 119, NaNMask: 0, STMode: 0},
		// ST known from sample 0 and then constant.
		{Seed: 42, N: 2, NaNMask: 0, STMode: 1},
		// First ST change happens after sample 1.
		{Seed: 42, N: 4, NaNMask: 0b00100, STMode: 2},
		// Active ST with small deltas to hit compact encodings.
		{Seed: 1, N: 9, NaNMask: 0b0101010101, STMode: 3},
		// Active ST with large deltas to hit varbit fallback.
		{Seed: 7, N: 9, NaNMask: ^uint64(0), STMode: 4},
	}
}

const (
	apiQueryEndpoint uint8 = iota
	apiQueryRangeEndpoint
	apiQueryExemplarsEndpoint
	apiEndpointCount
)

// APIGETFuzzSeed is a seed corpus entry for an HTTP GET API fuzzer.
type APIGETFuzzSeed struct {
	// Endpoint selects the HTTP API endpoint exercised by the seed.
	Endpoint uint8
	// RawQuery contains the URL-encoded query string.
	RawQuery string
}

// APIFormPOSTFuzzSeed is a seed corpus entry for a form-encoded HTTP POST API fuzzer.
type APIFormPOSTFuzzSeed struct {
	// Endpoint selects the HTTP API endpoint exercised by the seed.
	Endpoint uint8
	// RawQuery contains the URL-encoded query string.
	RawQuery string
	// FormBody contains the URL-encoded request body.
	FormBody string
}

// GetCorpusForFuzzAPIQueryGET returns the seed corpus for FuzzAPIQueryGET.
func GetCorpusForFuzzAPIQueryGET() []APIGETFuzzSeed {
	return []APIGETFuzzSeed{
		// Valid instant query with all supported optional parameters.
		{Endpoint: apiQueryEndpoint, RawQuery: "query=up&time=0&timeout=1m&limit=1&lookback_delta=5m"},
		// Valid nested PromQL expression: sum(rate(up[5m])).
		{Endpoint: apiQueryEndpoint, RawQuery: "query=sum%28rate%28up%5B5m%5D%29"},
		// Invalid evaluation timestamp.
		{Endpoint: apiQueryEndpoint, RawQuery: "query=up&time=NaN"},
		// Limit overflows an integer.
		{Endpoint: apiQueryEndpoint, RawQuery: "query=up&limit=999999999999999999999"},
		// Valid range query with all supported optional parameters.
		{Endpoint: apiQueryRangeEndpoint, RawQuery: "query=up&start=0&end=100&step=10&timeout=1m&limit=1"},
		// Invalid range with end before start.
		{Endpoint: apiQueryRangeEndpoint, RawQuery: "query=up&start=100&end=0&step=10"},
		// Invalid range with a zero step.
		{Endpoint: apiQueryRangeEndpoint, RawQuery: "query=up&start=0&end=100&step=0"},
		// Valid exemplar query.
		{Endpoint: apiQueryExemplarsEndpoint, RawQuery: "query=up&start=0&end=100"},
		// Invalid exemplar range with end before start.
		{Endpoint: apiQueryExemplarsEndpoint, RawQuery: "query=up&start=100&end=0"},
	}
}

// GetCorpusForFuzzAPIQueryPOST returns the seed corpus for FuzzAPIQueryPOST.
func GetCorpusForFuzzAPIQueryPOST() []APIFormPOSTFuzzSeed {
	return []APIFormPOSTFuzzSeed{
		// Valid matcher with a multi-byte UTF-8 label value: up{region="東京"}.
		{Endpoint: apiQueryEndpoint, FormBody: "query=up%7Bregion%3D%22%E6%9D%B1%E4%BA%AC%22%7D"},
		// Valid request with time in the URL and query in the form body.
		{Endpoint: apiQueryEndpoint, RawQuery: "time=0", FormBody: "query=up"},
		// Invalid query timeout.
		{Endpoint: apiQueryEndpoint, FormBody: "query=up&timeout=NaN"},
		// Malformed percent-encoding in the form body.
		{Endpoint: apiQueryEndpoint, FormBody: "query=%"},
		// Valid range query with the multi-byte UTF-8 label value up{region="東京"}.
		{Endpoint: apiQueryRangeEndpoint, FormBody: "query=up%7Bregion%3D%22%E6%9D%B1%E4%BA%AC%22%7D&start=0&end=100&step=10"},
		// Valid range query using sum(rate(up[5m])).
		{Endpoint: apiQueryRangeEndpoint, FormBody: "query=sum%28rate%28up%5B5m%5D%29&start=0&end=100&step=10"},
		// Valid exemplar query with the multi-byte UTF-8 label value up{region="東京"}.
		{Endpoint: apiQueryExemplarsEndpoint, FormBody: "query=up%7Bregion%3D%22%E6%9D%B1%E4%BA%AC%22%7D&start=0&end=100"},
		// Invalid exemplar start timestamp.
		{Endpoint: apiQueryExemplarsEndpoint, FormBody: "query=up&start=NaN&end=100"},
		// Invalid exemplar end timestamp.
		{Endpoint: apiQueryExemplarsEndpoint, FormBody: "query=up&start=0&end=NaN"},
		// Invalid PromQL expression: {.
		{Endpoint: apiQueryExemplarsEndpoint, FormBody: "query=%7B&start=0&end=100"},
	}
}
