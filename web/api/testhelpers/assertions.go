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

// This file provides assertion helpers for validating API responses in tests.
package testhelpers

import (
	"fmt"
	"slices"
	"strings"

	"github.com/stretchr/testify/require"
)

// RequireSuccess asserts that the response has status "success" and returns the response for chaining.
func (r *Response) RequireSuccess() *Response {
	r.t.Helper()
	require.NotNil(r.t, r.JSON, "response body is not JSON")
	require.Equal(r.t, "success", r.JSON["status"], "expected status to be 'success'")
	return r
}

// RequireError asserts that the response has status "error" and returns the response for chaining.
func (r *Response) RequireError() *Response {
	r.t.Helper()
	require.NotNil(r.t, r.JSON, "response body is not JSON")
	require.Equal(r.t, "error", r.JSON["status"], "expected status to be 'error'")
	return r
}

// RequireStatusCode asserts that the response has the given HTTP status code and returns the response for chaining.
func (r *Response) RequireStatusCode(expectedCode int) *Response {
	r.t.Helper()
	require.Equal(r.t, expectedCode, r.StatusCode, "unexpected HTTP status code")
	return r
}

// RequireJSONPathExists asserts that a JSON path exists and returns the response for chaining.
func (r *Response) RequireJSONPathExists(path string) *Response {
	r.t.Helper()
	require.NotNil(r.t, r.JSON, "response body is not JSON")

	value := getJSONPath(r.JSON, path)
	require.NotNil(r.t, value, "JSON path %q does not exist", path)
	return r
}

// RequireEquals asserts that a JSON path equals the expected value and returns the response for chaining.
func (r *Response) RequireEquals(path string, expected any) *Response {
	r.t.Helper()
	require.NotNil(r.t, r.JSON, "response body is not JSON")

	value := getJSONPath(r.JSON, path)
	require.NotNil(r.t, value, "JSON path %q does not exist", path)
	require.Equal(r.t, expected, value, "JSON path %q has unexpected value", path)
	return r
}

// RequireJSONArray asserts that a JSON path contains an array and returns the response for chaining.
func (r *Response) RequireJSONArray(path string) *Response {
	r.t.Helper()
	require.NotNil(r.t, r.JSON, "response body is not JSON")

	value := getJSONPath(r.JSON, path)
	require.NotNil(r.t, value, "JSON path %q does not exist", path)
	_, ok := value.([]any)
	require.True(r.t, ok, "JSON path %q is not an array", path)
	return r
}

// RequireLenAtLeast asserts that a JSON path contains an array with at least minLen elements and returns the response for chaining.
func (r *Response) RequireLenAtLeast(path string, minLen int) *Response {
	r.t.Helper()
	require.NotNil(r.t, r.JSON, "response body is not JSON")

	value := getJSONPath(r.JSON, path)
	require.NotNil(r.t, value, "JSON path %q does not exist", path)
	arr, ok := value.([]any)
	require.True(r.t, ok, "JSON path %q is not an array", path)
	require.GreaterOrEqual(r.t, len(arr), minLen, "JSON path %q has fewer than %d elements", path, minLen)
	return r
}

// RequireArrayContains asserts that a JSON path contains an array with the expected element and returns the response for chaining.
func (r *Response) RequireArrayContains(path string, expected any) *Response {
	r.t.Helper()
	require.NotNil(r.t, r.JSON, "response body is not JSON")

	value := getJSONPath(r.JSON, path)
	require.NotNil(r.t, value, "JSON path %q does not exist", path)
	arr, ok := value.([]any)
	require.True(r.t, ok, "JSON path %q is not an array", path)

	found := slices.Contains(arr, expected)
	require.True(r.t, found, "JSON path %q does not contain expected value %v", path, expected)
	return r
}

// RequireSome asserts that at least one element in an array satisfies the predicate and returns the response for chaining.
func (r *Response) RequireSome(path string, predicate func(any) bool) *Response {
	r.t.Helper()
	require.NotNil(r.t, r.JSON, "response body is not JSON")

	value := getJSONPath(r.JSON, path)
	require.NotNil(r.t, value, "JSON path %q does not exist", path)
	arr, ok := value.([]any)
	require.True(r.t, ok, "JSON path %q is not an array", path)

	found := slices.ContainsFunc(arr, predicate)
	require.True(r.t, found, "no element in JSON path %q satisfies the predicate", path)
	return r
}

// getJSONPath extracts a value from a JSON object using a simple path notation.
// Supports paths like "$.data", "$.data.groups", "$.data.groups[0]".
func getJSONPath(data map[string]any, path string) any {
	// Remove leading "$." if present.
	path = strings.TrimPrefix(path, "$.")

	if path == "" {
		return data
	}

	parts := strings.Split(path, ".")
	current := any(data)

	for _, part := range parts {
		// Handle array indexing (e.g., "groups[0]").
		if strings.Contains(part, "[") {
			// Not implementing array indexing for simplicity.
			// Tests should use direct field access or RequireSome.
			return nil
		}

		// Navigate to the next level.
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[part]
	}

	return current
}

// RequireVectorResult is a convenience helper for checking vector query results.
func (r *Response) RequireVectorResult() *Response {
	r.t.Helper()
	return r.RequireSuccess().RequireEquals("$.data.resultType", "vector")
}

// RequireMatrixResult is a convenience helper for checking matrix query results.
func (r *Response) RequireMatrixResult() *Response {
	r.t.Helper()
	return r.RequireSuccess().RequireEquals("$.data.resultType", "matrix")
}

// RequireScalarResult is a convenience helper for checking scalar query results.
func (r *Response) RequireScalarResult() *Response {
	r.t.Helper()
	return r.RequireSuccess().RequireEquals("$.data.resultType", "scalar")
}

// RequireRulesGroupNamed asserts that a rules response contains a group with the given name.
func (r *Response) RequireRulesGroupNamed(name string) *Response {
	r.t.Helper()
	return r.RequireSuccess().RequireSome("$.data.groups", func(group any) bool {
		if g, ok := group.(map[string]any); ok {
			return g["name"] == name
		}
		return false
	})
}

// RequireTargetCount asserts that a targets response contains at least n targets.
func (r *Response) RequireTargetCount(minCount int) *Response {
	r.t.Helper()
	r.RequireSuccess()

	// The targets endpoint returns activeTargets as an array of targets.
	value := getJSONPath(r.JSON, "$.data.activeTargets")
	require.NotNil(r.t, value, "JSON path $.data.activeTargets does not exist")

	arr, ok := value.([]any)
	require.True(r.t, ok, "$.data.activeTargets is not an array")
	require.GreaterOrEqual(r.t, len(arr), minCount, "expected at least %d targets, got %d", minCount, len(arr))
	return r
}

// DebugJSON is a helper for debugging JSON responses in tests.
func (r *Response) DebugJSON() *Response {
	r.t.Helper()
	r.t.Logf("Response status code: %d", r.StatusCode)
	r.t.Logf("Response body: %s", r.Body)
	if r.JSON != nil {
		r.t.Logf("Response JSON: %+v", r.JSON)
	}
	return r
}

// RequireContainsSubstring asserts that the response body contains the given substring.
func (r *Response) RequireContainsSubstring(substring string) *Response {
	r.t.Helper()
	require.Contains(r.t, r.Body, substring, "response body does not contain expected substring")
	return r
}

// RequireField asserts that a field exists at the given path and returns its value.
// Note: This method cannot be chained further since it returns the field value, not the Response.
func (r *Response) RequireField(path string) any {
	r.t.Helper()
	require.NotNil(r.t, r.JSON, "response body is not JSON")

	value := getJSONPath(r.JSON, path)
	require.NotNil(r.t, value, "JSON path %q does not exist", path)
	return value
}

// RequireFieldType asserts that a field exists and has the expected type.
func (r *Response) RequireFieldType(path, expectedType string) *Response {
	r.t.Helper()
	value := r.RequireField(path)

	var actualType string
	switch value.(type) {
	case string:
		actualType = "string"
	case float64:
		actualType = "number"
	case bool:
		actualType = "bool"
	case []any:
		actualType = "array"
	case map[string]any:
		actualType = "object"
	default:
		actualType = fmt.Sprintf("%T", value)
	}

	require.Equal(r.t, expectedType, actualType, "JSON path %q has unexpected type", path)
	return r
}
