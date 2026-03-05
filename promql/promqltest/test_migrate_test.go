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

package promqltest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeTestFile(t *testing.T, dir, content string) string {
	t.Helper()
	testFile := filepath.Join(dir, "testcase.test")
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0o644))
	return testFile
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	output, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(output)
}

func assertMigration(t *testing.T, mode, input, expected string) {
	dir := t.TempDir()
	testFile := writeTestFile(t, dir, input)

	err := MigrateTestData(mode, dir)
	require.NoError(t, err)

	output := readTestFile(t, testFile)
	require.Equal(t, expected, output)
}

func TestMigrateTestData_BasicMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Basic mode with fail",
			input: `
				eval_fail instant at 1m sum(foo)
				  expected_fail_message something went wrong
				  {src="a"} 1
			`,
			expected: `
				eval instant at 1m sum(foo)
				  expect fail msg:something went wrong
				  {src="a"} 1
			`,
		},
		{
			name: "Basic mode with warn",
			input: `
				eval_warn instant at 2m avg(bar)
				  {src="a"} 1
			`,
			expected: `
				eval instant at 2m avg(bar)
				  expect warn
				  {src="a"} 1
			`,
		},
		{
			name: "Basic mode with info",
			input: `
				eval_info instant at 3m min(baz)
				  {src="a"} 1
			`,
			expected: `
				eval instant at 3m min(baz)
				  expect info
				  {src="a"} 1
			`,
		},
		{
			name: "Basic mode with ordered",
			input: `
				eval_ordered instant at 4m max(qux)
				  {src="a"} 1
			`,
			expected: `
				eval instant at 4m max(qux)
				  expect ordered
				  {src="a"} 1
			`,
		},
		{
			name: "Basic mode with multiple eval blocks",
			input: `
				eval_fail instant at 1m sum(foo)
				  expected_fail_message something else went wrong
				  {src="a"} 1

				eval_warn instant at 2m avg(bar)
				  {src="a"} 1
			`,
			expected: `
				eval instant at 1m sum(foo)
				  expect fail msg:something else went wrong
				  {src="a"} 1

				eval instant at 2m avg(bar)
				  expect warn
				  {src="a"} 1
			`,
		},
		{
			name: "Basic mode with already migrated syntax (no changes)",
			input: `
				eval instant at 1m sum(foo)
				  expect fail msg:something went wrong
				  {src="a"} 1
			`,
			expected: `
				eval instant at 1m sum(foo)
				  expect fail msg:something went wrong
				  {src="a"} 1
			`,
		},
		{
			name: "Basic mode with only comments and whitespace",
			input: `
			# This is a comment
			
			`,
			expected: `
			# This is a comment
			
			`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertMigration(t, "basic", tc.input, tc.expected)
		})
	}
}

func TestMigrateTestData_StrictMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Strict mode with fail",
			input: `
				eval_fail instant at 1m sum(foo)
					expected_fail_message something went wrong
					{src="a"} 1
`,
			expected: `
				eval instant at 1m sum(foo)
					expect fail msg:something went wrong
					expect no_warn
					expect no_info
					{src="a"} 1
`,
		},
		{
			name: "Strict mode with warn",
			input: `
				eval_warn instant at 2m avg(bar)
					{src="a"} 1
`,
			expected: `
				eval instant at 2m avg(bar)
					expect warn
					expect no_info
					{src="a"} 1
`,
		},
		{
			name: "Strict mode with info",
			input: `
				eval_info instant at 3m min(baz)
					{src="a"} 1
`,
			expected: `
				eval instant at 3m min(baz)
					expect info
					expect no_warn
					{src="a"} 1
`,
		},
		{
			name: "Strict mode with ordered",
			input: `
				eval_ordered instant at 4m max(qux)
					{src="a"} 1
`,
			expected: `
				eval instant at 4m max(qux)
					expect ordered
					expect no_warn
					expect no_info
					{src="a"} 1
`,
		},
		{
			name: "Strict mode with multiple eval blocks",
			input: `
				eval_fail instant at 1m sum(foo)
					expected_fail_message something else went wrong
					{src="a"} 1

				eval_warn instant at 2m avg(bar)
					{src="a"} 1
`,
			expected: `
				eval instant at 1m sum(foo)
					expect fail msg:something else went wrong
					expect no_warn
					expect no_info
					{src="a"} 1

				eval instant at 2m avg(bar)
					expect warn
					expect no_info
					{src="a"} 1
`,
		},
		{
			name: "Strict mode with already migrated syntax (no changes)",
			input: `
				eval instant at 1m sum(foo)
					expect fail msg:something went wrong
					expect no_warn
					expect no_info
					{src="a"} 1
`,
			expected: `
				eval instant at 1m sum(foo)
					expect fail msg:something went wrong
					expect no_warn
					expect no_info
					{src="a"} 1
`,
		},
		{
			name: "Strict mode with only comments and whitespace",
			input: `
			# This is a comment

`,
			expected: `
			# This is a comment

`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertMigration(t, "strict", tc.input, tc.expected)
		})
	}
}

func TestMigrateTestData_TolerantMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Tolerant mode with fail",
			input: `
				eval_fail instant at 1m sum(foo)
					expected_fail_message something went wrong
					{src="a"} 1
`,
			expected: `
				eval instant at 1m sum(foo)
					expect fail msg:something went wrong
					{src="a"} 1
`,
		},
		{
			name: "Tolerant mode with warn",
			input: `
				eval_warn instant at 2m avg(bar)
					{src="a"} 1
`,
			expected: `
				eval instant at 2m avg(bar)
					{src="a"} 1
`,
		},
		{
			name: "Tolerant mode with info",
			input: `
				eval_info instant at 3m min(baz)
					{src="a"} 1
`,
			expected: `
				eval instant at 3m min(baz)
					{src="a"} 1
`,
		},
		{
			name: "Tolerant mode with ordered",
			input: `
				eval_ordered instant at 4m max(qux)
					{src="a"} 1
`,
			expected: `
				eval instant at 4m max(qux)
					expect ordered
					{src="a"} 1
`,
		},
		{
			name: "Tolerant mode with multiple eval blocks",
			input: `
				eval_fail instant at 1m sum(foo)
					expected_fail_message something else went wrong
					{src="a"} 1

				eval_warn instant at 2m avg(bar)
					{src="a"} 1
`,
			expected: `
				eval instant at 1m sum(foo)
					expect fail msg:something else went wrong
					{src="a"} 1

				eval instant at 2m avg(bar)
					{src="a"} 1
`,
		},
		{
			name: "Tolerant mode with already migrated syntax (no changes)",
			input: `
				eval instant at 1m sum(foo)
					expect fail msg:something went wrong
					{src="a"} 1
`,
			expected: `
				eval instant at 1m sum(foo)
					expect fail msg:something went wrong
					{src="a"} 1
`,
		},
		{
			name: "Tolerant mode with only comments and whitespace",
			input: `
			# This is a comment

`,
			expected: `
			# This is a comment

`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertMigration(t, "tolerant", tc.input, tc.expected)
		})
	}
}
