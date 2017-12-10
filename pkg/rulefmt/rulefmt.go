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
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql"
	yaml "gopkg.in/yaml.v2"
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

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
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

		if err := checkOverflow(g.XXX, "rule_group"); err != nil {
			errs = append(errs, errors.Wrapf(err, "Group: %s", g.Name))
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

	if err := checkOverflow(g.XXX, "config_file"); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// RuleGroup is a list of sequentially evaluated recording and alerting rules.
type RuleGroup struct {
	Name     string         `yaml:"name"`
	Interval model.Duration `yaml:"interval,omitempty"`
	Rules    []Rule         `yaml:"rules"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// Rule describes an alerting or recording rule.
type Rule struct {
	Record      string            `yaml:"record,omitempty"`
	Alert       string            `yaml:"alert,omitempty"`
	Expr        string            `yaml:"expr"`
	For         model.Duration    `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
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
		errs = append(errs, errors.Errorf("could not parse expression: %s", err))
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

	if err := checkOverflow(r.XXX, "rule"); err != nil {
		errs = append(errs, err)
	}

	return errs
}

func checkOverflow(m map[string]interface{}, ctx string) error {
	if len(m) > 0 {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		return fmt.Errorf("unknown fields in %s: %s", ctx, strings.Join(keys, ", "))
	}
	return nil
}

// ParseFile parses the rule file and validates it.
func ParseFile(file string) (*RuleGroups, []error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, []error{err}
	}
	var groups RuleGroups
	if err := yaml.Unmarshal(b, &groups); err != nil {
		return nil, []error{err}
	}
	return &groups, groups.Validate()
}
