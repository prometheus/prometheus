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
	"context"
	"io/ioutil"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	yaml "gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/template"
)

// Error represents semantical errors on parsing rule groups.
type Error struct {
	Group    string
	Rule     int
	RuleName string
	Err      error
}

func (err *Error) Error() string {
	return errors.Wrapf(err.Err, "group %q, rule %d, %q", err.Group, err.Rule, err.RuleName).Error()
}

// RuleGroups is a set of rule groups that are typically exposed in a file.
type RuleGroups struct {
	Groups []RuleGroup `yaml:"groups"`
}

// Validate validates all rules in the rule groups.
func (g *RuleGroups) Validate() (errs []error) {
	set := map[string]struct{}{}

	for _, g := range g.Groups {
		if g.Name == "" {
			errs = append(errs, errors.Errorf("Groupname should not be empty"))
		}

		if _, ok := set[g.Name]; ok {
			errs = append(
				errs,
				errors.Errorf("groupname: \"%s\" is repeated in the same file", g.Name),
			)
		}

		set[g.Name] = struct{}{}

		for i, r := range g.Rules {
			for _, err := range r.Validate() {
				var ruleName string
				if r.Alert != "" {
					ruleName = r.Alert
				} else {
					ruleName = r.Record
				}
				errs = append(errs, &Error{
					Group:    g.Name,
					Rule:     i,
					RuleName: ruleName,
					Err:      err,
				})
			}
		}
	}

	return errs
}

// RuleGroup is a list of sequentially evaluated recording and alerting rules.
type RuleGroup struct {
	Name     string         `yaml:"name"`
	Interval model.Duration `yaml:"interval,omitempty"`
	Rules    []Rule         `yaml:"rules"`
}

// Rule describes an alerting or recording rule.
type Rule struct {
	Record      string            `yaml:"record,omitempty"`
	Alert       string            `yaml:"alert,omitempty"`
	Expr        string            `yaml:"expr"`
	For         model.Duration    `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

// Validate the rule and return a list of encountered errors.
func (r *Rule) Validate() (errs []error) {
	if r.Record != "" && r.Alert != "" {
		errs = append(errs, errors.Errorf("only one of 'record' and 'alert' must be set"))
	}
	if r.Record == "" && r.Alert == "" {
		errs = append(errs, errors.Errorf("one of 'record' or 'alert' must be set"))
	}

	if r.Expr == "" {
		errs = append(errs, errors.Errorf("field 'expr' must be set in rule"))
	} else if _, err := promql.ParseExpr(r.Expr); err != nil {
		errs = append(errs, errors.Wrap(err, "could not parse expression"))
	}
	if r.Record != "" {
		if len(r.Annotations) > 0 {
			errs = append(errs, errors.Errorf("invalid field 'annotations' in recording rule"))
		}
		if r.For != 0 {
			errs = append(errs, errors.Errorf("invalid field 'for' in recording rule"))
		}
		if !model.IsValidMetricName(model.LabelValue(r.Record)) {
			errs = append(errs, errors.Errorf("invalid recording rule name: %s", r.Record))
		}
	}

	for k, v := range r.Labels {
		if !model.LabelName(k).IsValid() {
			errs = append(errs, errors.Errorf("invalid label name: %s", k))
		}

		if !model.LabelValue(v).IsValid() {
			errs = append(errs, errors.Errorf("invalid label value: %s", v))
		}
	}

	for k := range r.Annotations {
		if !model.LabelName(k).IsValid() {
			errs = append(errs, errors.Errorf("invalid annotation name: %s", k))
		}
	}

	return append(errs, testTemplateParsing(r)...)
}

// testTemplateParsing checks if the templates used in labels and annotations
// of the alerting rules are parsed correctly.
func testTemplateParsing(rl *Rule) (errs []error) {
	if rl.Alert == "" {
		// Not an alerting rule.
		return errs
	}

	// Trying to parse templates.
	tmplData := template.AlertTemplateData(map[string]string{}, map[string]string{}, 0)
	defs := []string{
		"{{$labels := .Labels}}",
		"{{$externalLabels := .ExternalLabels}}",
		"{{$value := .Value}}",
	}
	parseTest := func(text string) error {
		tmpl := template.NewTemplateExpander(
			context.TODO(),
			strings.Join(append(defs, text), ""),
			"__alert_"+rl.Alert,
			tmplData,
			model.Time(timestamp.FromTime(time.Now())),
			nil,
			nil,
		)
		return tmpl.ParseTest()
	}

	// Parsing Labels.
	for k, val := range rl.Labels {
		err := parseTest(val)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "label %q", k))
		}
	}

	// Parsing Annotations.
	for k, val := range rl.Annotations {
		err := parseTest(val)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "annotation %q", k))
		}
	}

	return errs
}

// Parse parses and validates a set of rules.
func Parse(content []byte) (*RuleGroups, []error) {
	var groups RuleGroups
	if err := yaml.UnmarshalStrict(content, &groups); err != nil {
		return nil, []error{err}
	}
	return &groups, groups.Validate()
}

// ParseFile reads and parses rules from a file.
func ParseFile(file string) (*RuleGroups, []error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, []error{errors.Wrap(err, file)}
	}
	rgs, errs := Parse(b)
	for i := range errs {
		errs[i] = errors.Wrap(errs[i], file)
	}
	return rgs, errs
}
