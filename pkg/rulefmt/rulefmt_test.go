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
	"strings"
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestParseFileSuccess(t *testing.T) {
	if _, errs := ParseFile("testdata/test.yaml"); len(errs) > 0 {
		t.Errorf("unexpected errors parsing file")
		for _, err := range errs {
			t.Error(err)
		}
	}
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
	}

	for _, c := range table {
		_, errs := ParseFile(filepath.Join("testdata", c.filename))
		if errs == nil {
			t.Errorf("Expected error parsing %s but got none", c.filename)
			continue
		}
		if !strings.Contains(errs[0].Error(), c.errMsg) {
			t.Errorf("Expected error for %s to contain %q but got: %s", c.filename, c.errMsg, errs)
		}
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
		testutil.Assert(t, rgs != nil, "Rule parsing, rule=\n"+tst.ruleString)
		passed := (tst.shouldPass && len(errs) == 0) || (!tst.shouldPass && len(errs) > 0)
		testutil.Assert(t, passed, "Rule validation failed, rule=\n"+tst.ruleString)
	}

}
