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

//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
)

// TestRulesUnitTestRuleFilesPipe reproduces the scenario from
// https://github.com/prometheus/prometheus/issues/6222: a unit test file
// whose rule_files entry can only be read once, such as a named pipe,
// loaded by more than one test group. Before the rules were cached in a
// readOnceGroupLoader, every test group re-read rule_files from scratch,
// so only the first test group ever saw the rules; every later one found
// the pipe already drained and got no rules at all.
func TestRulesUnitTestRuleFilesPipe(t *testing.T) {
	dir := t.TempDir()

	pipe := filepath.Join(dir, "rules.pipe")
	require.NoError(t, syscall.Mkfifo(pipe, 0o666))

	const ruleFileContent = `
groups:
  - name: g
    rules:
      - record: always_one
        expr: 1
`
	go func() {
		f, err := os.OpenFile(pipe, os.O_WRONLY, os.ModeNamedPipe)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(ruleFileContent)
	}()

	unitTestContent := `
rule_files:
  - ` + pipe + `

evaluation_interval: 1m

tests:
  - promql_expr_test:
      - expr: always_one
        eval_time: 0
        exp_samples:
          - value: 1
            labels: always_one

  - promql_expr_test:
      - expr: always_one
        eval_time: 0
        exp_samples:
          - value: 1
            labels: always_one
`
	unitTestFile := filepath.Join(dir, "unittest.yml")
	require.NoError(t, os.WriteFile(unitTestFile, []byte(unitTestContent), 0o644))

	got := RulesUnitTest(promqltest.LazyLoaderOpts{}, parser.NewParser(parser.Options{}), nil, false, false, false, unitTestFile)
	require.Equal(t, 0, got, "both test groups should see the rules loaded from the pipe, not just the first one")
}
