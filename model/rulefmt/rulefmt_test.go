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
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

func TestUniqueErrorNodes(t *testing.T) {
	group := `
groups:
- name: example
  rules:
  - alert: InstanceDown
    expr: up ===== 0
    for: 5m
    labels:
      severity: "page"
    annotations:
      summary: "Instance {{ $labels.instance }} down"
  - alert: InstanceUp
    expr: up ===== 1
    for: 5m
    labels:
      severity: "page"
    annotations:
      summary: "Instance {{ $labels.instance }} up"
`
	_, errs := Parse([]byte(group))
	require.Len(t, errs, 2, "Expected two errors")
	err0 := errs[0].(*Error).Err.node
	err1 := errs[1].(*Error).Err.node
	require.NotEqual(t, err0, err1, "Error nodes should not be the same")
}

func TestError(t *testing.T) {
	tests := []struct {
		name  string
		error *Error
		want  string
	}{
		{
			name: "with alternative node provided in WrappedError",
			error: &Error{
				Group:    "some group",
				Rule:     1,
				RuleName: "some rule name",
				Err: WrappedError{
					err: errors.New("some error"),
					node: &yaml.Node{
						Line:   10,
						Column: 20,
					},
					nodeAlt: &yaml.Node{
						Line:   11,
						Column: 21,
					},
				},
			},
			want: `10:20: 11:21: group "some group", rule 1, "some rule name": some error`,
		},
		{
			name: "with node provided in WrappedError",
			error: &Error{
				Group:    "some group",
				Rule:     1,
				RuleName: "some rule name",
				Err: WrappedError{
					err: errors.New("some error"),
					node: &yaml.Node{
						Line:   10,
						Column: 20,
					},
				},
			},
			want: `10:20: group "some group", rule 1, "some rule name": some error`,
		},
		{
			name: "with only err provided in WrappedError",
			error: &Error{
				Group:    "some group",
				Rule:     1,
				RuleName: "some rule name",
				Err: WrappedError{
					err: errors.New("some error"),
				},
			},
			want: `group "some group", rule 1, "some rule name": some error`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.error.Error()
			require.Equal(t, tt.want, got)
		})
	}
}

func TestWrappedError(t *testing.T) {
	tests := []struct {
		name         string
		wrappedError *WrappedError
		want         string
	}{
		{
			name: "with alternative node provided",
			wrappedError: &WrappedError{
				err: errors.New("some error"),
				node: &yaml.Node{
					Line:   10,
					Column: 20,
				},
				nodeAlt: &yaml.Node{
					Line:   11,
					Column: 21,
				},
			},
			want: `10:20: 11:21: some error`,
		},
		{
			name: "with node provided",
			wrappedError: &WrappedError{
				err: errors.New("some error"),
				node: &yaml.Node{
					Line:   10,
					Column: 20,
				},
			},
			want: `10:20: some error`,
		},
		{
			name: "with only err provided",
			wrappedError: &WrappedError{
				err: errors.New("some error"),
			},
			want: `some error`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.wrappedError.Error()
			require.Equal(t, tt.want, got)
		})
	}
}
