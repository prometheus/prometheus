// Copyright 2013 The Prometheus Authors
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

package rules

import (
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
)

// Grouploader is an interface for loading and parsing rules from arbitrary sources.
type GroupLoader interface {
	// Load resolves a list of parsed rules from a set of identifiers.
	Load(identifiers ...string) ([]*ParsedGroup, []error)
}

// ParsedGroup binds a set of ParsedRules with an evaluation interval.
// 0 indicates that the global default interval should be used.
type ParsedGroup struct {
	File, Name string
	Interval   time.Duration
	Rules      []*ParsedRule
}

// ParsedRule is a rule which has undergone validation.
type ParsedRule struct {
	Name                string
	Expr                parser.Expr
	Alert               bool
	Period              time.Duration
	Labels, Annotations labels.Labels
}

// FileLoader is the default GroupLoader which reads files.
type FileLoader struct{}

// Load implements GroupLoader by reading local files
func (l FileLoader) Load(filenames ...string) ([]*ParsedGroup, []error) {
	var groups []*ParsedGroup

	for _, fn := range filenames {
		rgs, errs := rulefmt.ParseFile(fn)
		if errs != nil {
			return nil, errs
		}
		for _, rg := range rgs.Groups {
			grp := &ParsedGroup{
				File:     fn,
				Name:     rg.Name,
				Interval: time.Duration(rg.Interval),
			}
			for _, r := range rg.Rules {
				expr, err := parser.ParseExpr(r.Expr.Value)
				if err != nil {
					return nil, []error{errors.Wrap(err, fn)}
				}
				parsed := &ParsedRule{
					Expr:   expr,
					Period: time.Duration(r.For),
				}
				if len(r.Labels) > 0 {
					parsed.Labels = labels.FromMap(r.Labels)
				}
				if len(r.Annotations) > 0 {
					parsed.Annotations = labels.FromMap(r.Annotations)
				}

				if r.Alert.Value != "" {
					parsed.Name = r.Alert.Value
					parsed.Alert = true
				} else {
					parsed.Name = r.Record.Value
				}
				grp.Rules = append(grp.Rules, parsed)
			}
			groups = append(groups, grp)
		}
	}
	return groups, nil
}
