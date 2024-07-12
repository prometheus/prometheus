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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v3"

	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/template"
)

// Error represents semantic errors on parsing rule groups.
type Error struct {
	Group    string
	Rule     int
	RuleName string
	Err      WrappedError
}

// Error prints the error message in a formatted string.
func (err *Error) Error() string {
	if err.Err.err == nil {
		return ""
	}
	if err.Err.nodeAlt != nil {
		return fmt.Sprintf("%d:%d: %d:%d: group %q, rule %d, %q: %v", err.Err.node.Line, err.Err.node.Column, err.Err.nodeAlt.Line, err.Err.nodeAlt.Column, err.Group, err.Rule, err.RuleName, err.Err.err)
	}
	if err.Err.node != nil {
		return fmt.Sprintf("%d:%d: group %q, rule %d, %q: %v", err.Err.node.Line, err.Err.node.Column, err.Group, err.Rule, err.RuleName, err.Err.err)
	}
	return fmt.Sprintf("group %q, rule %d, %q: %v", err.Group, err.Rule, err.RuleName, err.Err.err)
}

// Unwrap unpacks wrapped error for use in errors.Is & errors.As.
func (err *Error) Unwrap() error {
	return &err.Err
}

// WrappedError wraps error with the yaml node which can be used to represent
// the line and column numbers of the error.
type WrappedError struct {
	err     error
	node    *yaml.Node
	nodeAlt *yaml.Node
}

// Error prints the error message in a formatted string.
func (we *WrappedError) Error() string {
	if we.err == nil {
		return ""
	}
	if we.nodeAlt != nil {
		return fmt.Sprintf("%d:%d: %d:%d: %v", we.node.Line, we.node.Column, we.nodeAlt.Line, we.nodeAlt.Column, we.err)
	}
	if we.node != nil {
		return fmt.Sprintf("%d:%d: %v", we.node.Line, we.node.Column, we.err)
	}
	return we.err.Error()
}

// Unwrap unpacks wrapped error for use in errors.Is & errors.As.
func (we *WrappedError) Unwrap() error {
	return we.err
}

// RuleGroups is a set of rule groups that are typically exposed in a file.
type RuleGroups struct {
	Groups []RuleGroup `yaml:"groups"`
}

type ruleGroups struct {
	Groups []yaml.Node `yaml:"groups"`
}

// Validate validates all rules in the rule groups.
func (g *RuleGroups) Validate(node ruleGroups) (errs []error) {
	set := map[string]struct{}{}

	for j, g := range g.Groups {
		if g.Name == "" {
			errs = append(errs, fmt.Errorf("%d:%d: Groupname must not be empty", node.Groups[j].Line, node.Groups[j].Column))
		}

		if _, ok := set[g.Name]; ok {
			errs = append(
				errs,
				fmt.Errorf("%d:%d: groupname: \"%s\" is repeated in the same file", node.Groups[j].Line, node.Groups[j].Column, g.Name),
			)
		}

		set[g.Name] = struct{}{}

		for i, r := range g.Rules {
			for _, node := range g.Rules[i].Validate() {
				var ruleName yaml.Node
				if r.Alert.Value != "" {
					ruleName = r.Alert
				} else {
					ruleName = r.Record
				}
				errs = append(errs, &Error{
					Group:    g.Name,
					Rule:     i + 1,
					RuleName: ruleName.Value,
					Err:      node,
				})
			}
		}
	}

	return errs
}

// RuleGroup is a list of sequentially evaluated recording and alerting rules.
type RuleGroup struct {
	Name        string          `yaml:"name"`
	Interval    model.Duration  `yaml:"interval,omitempty"`
	QueryOffset *model.Duration `yaml:"query_offset,omitempty"`
	Limit       int             `yaml:"limit,omitempty"`
	Rules       []RuleNode      `yaml:"rules"`
}

// Rule describes an alerting or recording rule.
type Rule struct {
	Record        string            `yaml:"record,omitempty"`
	Alert         string            `yaml:"alert,omitempty"`
	Expr          string            `yaml:"expr"`
	For           model.Duration    `yaml:"for,omitempty"`
	KeepFiringFor model.Duration    `yaml:"keep_firing_for,omitempty"`
	Labels        map[string]string `yaml:"labels,omitempty"`
	Annotations   map[string]string `yaml:"annotations,omitempty"`
}

// RuleNode adds yaml.v3 layer to support line and column outputs for invalid rules.
type RuleNode struct {
	Record        yaml.Node         `yaml:"record,omitempty"`
	Alert         yaml.Node         `yaml:"alert,omitempty"`
	Expr          yaml.Node         `yaml:"expr"`
	For           model.Duration    `yaml:"for,omitempty"`
	KeepFiringFor model.Duration    `yaml:"keep_firing_for,omitempty"`
	Labels        map[string]string `yaml:"labels,omitempty"`
	Annotations   map[string]string `yaml:"annotations,omitempty"`
}

// Validate the rule and return a list of encountered errors.
func (r *RuleNode) Validate() (nodes []WrappedError) {
	if r.Record.Value != "" && r.Alert.Value != "" {
		nodes = append(nodes, WrappedError{
			err:     fmt.Errorf("only one of 'record' and 'alert' must be set"),
			node:    &r.Record,
			nodeAlt: &r.Alert,
		})
	}
	if r.Record.Value == "" && r.Alert.Value == "" {
		nodes = append(nodes, WrappedError{
			err:     fmt.Errorf("one of 'record' or 'alert' must be set"),
			node:    &r.Record,
			nodeAlt: &r.Alert,
		})
	}

	if r.Expr.Value == "" {
		nodes = append(nodes, WrappedError{
			err:  fmt.Errorf("field 'expr' must be set in rule"),
			node: &r.Expr,
		})
	} else if _, err := parser.ParseExpr(r.Expr.Value); err != nil {
		nodes = append(nodes, WrappedError{
			err:  fmt.Errorf("could not parse expression: %w", err),
			node: &r.Expr,
		})
	}
	if r.Record.Value != "" {
		if len(r.Annotations) > 0 {
			nodes = append(nodes, WrappedError{
				err:  fmt.Errorf("invalid field 'annotations' in recording rule"),
				node: &r.Record,
			})
		}
		if r.For != 0 {
			nodes = append(nodes, WrappedError{
				err:  fmt.Errorf("invalid field 'for' in recording rule"),
				node: &r.Record,
			})
		}
		if r.KeepFiringFor != 0 {
			nodes = append(nodes, WrappedError{
				err:  fmt.Errorf("invalid field 'keep_firing_for' in recording rule"),
				node: &r.Record,
			})
		}
		if !model.IsValidMetricName(model.LabelValue(r.Record.Value)) {
			nodes = append(nodes, WrappedError{
				err:  fmt.Errorf("invalid recording rule name: %s", r.Record.Value),
				node: &r.Record,
			})
		}
	}

	for k, v := range r.Labels {
		if !model.LabelName(k).IsValid() || k == model.MetricNameLabel {
			nodes = append(nodes, WrappedError{
				err: fmt.Errorf("invalid label name: %s", k),
			})
		}

		if !model.LabelValue(v).IsValid() {
			nodes = append(nodes, WrappedError{
				err: fmt.Errorf("invalid label value: %s", v),
			})
		}
	}

	for k := range r.Annotations {
		if !model.LabelName(k).IsValid() {
			nodes = append(nodes, WrappedError{
				err: fmt.Errorf("invalid annotation name: %s", k),
			})
		}
	}

	for _, err := range testTemplateParsing(r) {
		nodes = append(nodes, WrappedError{err: err})
	}

	return
}

// testTemplateParsing checks if the templates used in labels and annotations
// of the alerting rules are parsed correctly.
func testTemplateParsing(rl *RuleNode) (errs []error) {
	if rl.Alert.Value == "" {
		// Not an alerting rule.
		return errs
	}

	// Trying to parse templates.
	tmplData := template.AlertTemplateData(map[string]string{}, map[string]string{}, "", promql.Sample{})
	defs := []string{
		"{{$labels := .Labels}}",
		"{{$externalLabels := .ExternalLabels}}",
		"{{$externalURL := .ExternalURL}}",
		"{{$value := .Value}}",
	}
	parseTest := func(text string) error {
		tmpl := template.NewTemplateExpander(
			context.TODO(),
			strings.Join(append(defs, text), ""),
			"__alert_"+rl.Alert.Value,
			tmplData,
			model.Time(timestamp.FromTime(time.Now())),
			nil,
			nil,
			nil,
		)
		return tmpl.ParseTest()
	}

	// Parsing Labels.
	for k, val := range rl.Labels {
		err := parseTest(val)
		if err != nil {
			errs = append(errs, fmt.Errorf("label %q: %w", k, err))
		}
	}

	// Parsing Annotations.
	for k, val := range rl.Annotations {
		err := parseTest(val)
		if err != nil {
			errs = append(errs, fmt.Errorf("annotation %q: %w", k, err))
		}
	}

	return errs
}

// Parse parses and validates a set of rules.
func Parse(content []byte) (*RuleGroups, []error) {
	var (
		groups RuleGroups
		node   ruleGroups
		errs   []error
	)

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	err := decoder.Decode(&groups)
	// Ignore io.EOF which happens with empty input.
	if err != nil && !errors.Is(err, io.EOF) {
		errs = append(errs, err)
	}
	err = yaml.Unmarshal(content, &node)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return nil, errs
	}

	return &groups, groups.Validate(node)
}

// ParseFile reads and parses rules from a file.
func ParseFile(file string) (*RuleGroups, []error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, []error{fmt.Errorf("%s: %w", file, err)}
	}
	rgs, errs := Parse(b)
	for i := range errs {
		errs[i] = fmt.Errorf("%s: %w", file, errs[i])
	}
	return rgs, errs
}
