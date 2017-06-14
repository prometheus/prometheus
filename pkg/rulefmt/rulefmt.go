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
	"io/ioutil"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql"
)

// Error represents semantical errors on parsing rule groups.
type Error struct {
	Group string
	Rule  int
	Err   error
}

func (err *Error) Error() string {
	return errors.Wrapf(err.Err, "group %q, rule %d", err.Group, err.Rule).Error()
}

// RuleGroups is a set of rule groups that are typically exposed in a file.
type RuleGroups struct {
	Version int         `json:"version"`
	Groups  []RuleGroup `json:"groups"`
}

// Validate validates all rules in the rule groups.
func (g *RuleGroups) Validate() (errs []error) {
	if g.Version != 1 {
		errs = append(errs, errors.Errorf("invalid rule group version %d", g.Version))
	}
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
				errs = append(errs, &Error{
					Group: g.Name,
					Rule:  i,
					Err:   err,
				})
			}
		}
	}
	return errs
}

// RuleGroup is a list of sequentially evaluated recording and alerting rules.
type RuleGroup struct {
	Name     string `json:"name"`
	Interval string `json:"interval,omitempty"`
	Rules    []Rule `json:"rules"`
}

// Rule describes an alerting or recording rule.
type Rule struct {
	Record      string            `json:"record,omitempty"`
	Alert       string            `json:"alert,omitempty"`
	Expr        string            `json:"expr"`
	For         string            `json:"for,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
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
		if r.For != "" {
			errs = append(errs, errors.Errorf("invalid field 'for' in recording rule"))
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

	return errs
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
