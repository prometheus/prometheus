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
	"fmt"
	"io/ioutil"
	"regexp"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/template"
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
	Filters     map[string][]*FilterSection `yaml:"filters,omitempty"`
}

type FilterSection struct {
	MatchMap           map[string]string  `yaml:"label,omitempty"`
	MatchREMap         map[string]Regexp  `yaml:"label_re,omitempty"`
	Value              float64            `yaml:"v,omitempty"`
	RelationalOperator string             `yaml:"op,omitempty"`
	ValueRange         map[string]float64 `yaml:"v_range,omitempty"`
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

	matchKeyMap := map[string]bool{"match": true, "match_re": true}
	relationalOpMap := map[string]bool{"eq": true, "ne": true, "gt": true, "ge": true, "lt": true, "le": true,
		"=": true, "!=": true, ">": true, ">=": true, "<": true, "<=": true}
	valueRangeMap := map[string]bool{"min": true, "max": true}
	for key, matches := range r.Filters {
		if _, ok := matchKeyMap[key]; !ok {
			errs = append(errs, errors.Errorf("invalid filter map key name: %s", key))
		}
		for _, match := range matches {
			if len(match.MatchMap) > 0 {
				for k, _ := range match.MatchMap {
					if !model.LabelName(k).IsValid() {
						errs = append(errs, errors.Errorf("invalid filter match condition label name: %s", k))
					}
				}
			}
			if len(match.MatchREMap) > 0 {
				for k, _ := range match.MatchREMap {
					if !model.LabelName(k).IsValid() {
						errs = append(errs, errors.Errorf("invalid filter match_re condition label name: %s", k))
					}
				}
			}
			if len(match.ValueRange) > 0 {
				fbool := true
				for k, _ := range match.ValueRange {
					if _, ok := valueRangeMap[k]; !ok {
						errs = append(errs, errors.Errorf("invalid filter condition value range name: %s", k))
						fbool = false
					}
				}

				if fbool {
					if match.ValueRange["min"] > match.ValueRange["max"] {
						errs = append(errs, errors.Errorf("invalid filter v_range: min is greater than max"))
					}
				}

				if len(match.RelationalOperator) > 0 {
					errs = append(errs, errors.Errorf("invalid filter: mutual exclusion between op and v_range"))
				}
			}else{
				if _, ok := relationalOpMap[match.RelationalOperator]; !ok {
					errs = append(errs, errors.Errorf("invalid filter match operator name: %s", match.RelationalOperator))
				}
			}
		}
	}

	errs = append(errs, testTemplateParsing(r)...)
	return errs
}

// testTemplateParsing checks if the templates used in labels and annotations
// of the alerting rules are parsed correctly.
func testTemplateParsing(rl *Rule) (errs []error) {
	if rl.Alert == "" {
		// Not an alerting rule.
		return errs
	}

	// Trying to parse templates.
	tmplData := template.AlertTemplateData(make(map[string]string), 0)
	defs := "{{$labels := .Labels}}{{$value := .Value}}"
	parseTest := func(text string) error {
		tmpl := template.NewTemplateExpander(
			context.TODO(),
			defs+text,
			"__alert_"+rl.Alert,
			tmplData,
			model.Time(timestamp.FromTime(time.Now())),
			nil,
			nil,
		)
		return tmpl.ParseTest()
	}

	// Parsing Labels.
	for _, val := range rl.Labels {
		err := parseTest(val)
		if err != nil {
			errs = append(errs, fmt.Errorf("msg=%s", err.Error()))
		}
	}

	// Parsing Annotations.
	for _, val := range rl.Annotations {
		err := parseTest(val)
		if err != nil {
			errs = append(errs, fmt.Errorf("msg=%s", err.Error()))
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
		return nil, []error{err}
	}
	return Parse(b)
}


// Regexp encapsulates a regexp.Regexp and makes it YAML marshallable.
type Regexp struct {
	*regexp.Regexp
	original string
}

// NewRegexp creates a new anchored Regexp and returns an error if the
// passed-in regular expression does not compile.
func NewRegexp(s string) (Regexp, error) {
	regex, err := regexp.Compile("^(?:" + s + ")$")
	return Regexp{
		Regexp:   regex,
		original: s,
	}, err
}

// MustNewRegexp works like NewRegexp, but panics if the regular expression does not compile.
func MustNewRegexp(s string) Regexp {
	re, err := NewRegexp(s)
	if err != nil {
		panic(err)
	}
	return re
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (re *Regexp) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	r, err := NewRegexp(s)
	if err != nil {
		return err
	}
	*re = r
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (re Regexp) MarshalYAML() (interface{}, error) {
	if re.original != "" {
		return re.original, nil
	}
	return nil, nil
}
