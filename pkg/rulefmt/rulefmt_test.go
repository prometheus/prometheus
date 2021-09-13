// Copyright 2017 The Prometheus Authors
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

package rulefmt

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFileSuccess(t *testing.T) {
	_, errs := ParseFile("testdata/test.yaml")
	require.Empty(t, errs, "unexpected errors parsing file")
}

func TestParseFileFailure(t *testing.T) {
	table := []struct {
		filename string
		errMsg   string
	}{
		{
			filename: "duplicate_grp.bad.yaml",
			errMsg:   "groupname: \"yolo\" is repeated in the same file",
		},
		{
			filename: "bad_expr.bad.yaml",
			errMsg:   "parse error",
		},
		{
			filename: "record_and_alert.bad.yaml",
			errMsg:   "only one of 'record' and 'alert' must be set",
		},
		{
			filename: "no_rec_alert.bad.yaml",
			errMsg:   "one of 'record' or 'alert' must be set",
		},
		{
			filename: "noexpr.bad.yaml",
			errMsg:   "field 'expr' must be set in rule",
		},
		{
			filename: "bad_lname.bad.yaml",
			errMsg:   "invalid label name",
		},
		{
			filename: "bad_annotation.bad.yaml",
			errMsg:   "invalid annotation name",
		},
		{
			filename: "invalid_record_name.bad.yaml",
			errMsg:   "invalid recording rule name",
		},
		{
			filename: "bad_field.bad.yaml",
			errMsg:   "field annotation not found",
		},
		{
			filename: "invalid_label_name.bad.yaml",
			errMsg:   "invalid label name",
		},
	}

	for _, c := range table {
		_, errs := ParseFile(filepath.Join("testdata", c.filename))
		require.NotNil(t, errs, "Expected error parsing %s but got none", c.filename)
		require.Error(t, errs[0], c.errMsg, "Expected error for %s.", c.filename)
	}
}

func TestTemplateParsing(t *testing.T) {
	tests := []struct {
		ruleString string
		shouldPass bool
	}{
		{
			ruleString: `
groups:
- name: example
  rules:
  - alert: InstanceDown
    expr: up == 0
    for: 5m
    labels:
      severity: "page"
    annotations:
      summary: "Instance {{ $labels.instance }} down"
`,
			shouldPass: true,
		},
		{
			// `$label` instead of `$labels`.
			ruleString: `
groups:
- name: example
  rules:
  - alert: InstanceDown
    expr: up == 0
    for: 5m
    labels:
      severity: "page"
    annotations:
      summary: "Instance {{ $label.instance }} down"
`,
			shouldPass: false,
		},
		{
			// `$this_is_wrong`.
			ruleString: `
groups:
- name: example
  rules:
  - alert: InstanceDown
    expr: up == 0
    for: 5m
    labels:
      severity: "{{$this_is_wrong}}"
    annotations:
      summary: "Instance {{ $labels.instance }} down"
`,
			shouldPass: false,
		},
		{
			// `$labels.quantile * 100`.
			ruleString: `
groups:
- name: example
  rules:
  - alert: InstanceDown
    expr: up == 0
    for: 5m
    labels:
      severity: "page"
    annotations:
      summary: "Instance {{ $labels.instance }} down"
      description: "{{$labels.quantile * 100}}"
`,
			shouldPass: false,
		},
	}

	for _, tst := range tests {
		rgs, errs := Parse([]byte(tst.ruleString))
		require.NotNil(t, rgs, "Rule parsing, rule=\n"+tst.ruleString)
		passed := (tst.shouldPass && len(errs) == 0) || (!tst.shouldPass && len(errs) > 0)
		require.True(t, passed, "Rule validation failed, rule=\n"+tst.ruleString)
	}

}
