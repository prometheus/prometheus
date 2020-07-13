// Copyright 2020 The Prometheus Authors
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

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
)

// Grouploader is an interface for loading and parsing rules from arbitrary sources.
type GroupLoader interface {
	// Load resolves a list of parsed rules from a set of identifiers.
	Load(
		opts *ManagerOptions,
		done chan struct{},
		defaultInterval time.Duration,
		externalLabels labels.Labels,
		shouldRestore bool,
		logger log.Logger,
		identifiers ...string,
	) (map[string]*Group, []error)
}

// FileLoader is the default GroupLoader which reads files.
type FileLoader struct{}

// Load implements GroupLoader by reading local files
func (FileLoader) Load(
	opts *ManagerOptions,
	done chan struct{},
	defaultInterval time.Duration,
	externalLabels labels.Labels,
	shouldRestore bool,
	logger log.Logger,
	filenames ...string,
) (map[string]*Group, []error) {
	groups := make(map[string]*Group)

	for _, fn := range filenames {
		rgs, errs := rulefmt.ParseFile(fn)
		if errs != nil {
			return nil, errs
		}

		for _, rg := range rgs.Groups {
			itv := defaultInterval
			if rg.Interval != 0 {
				itv = time.Duration(rg.Interval)
			}

			rules := make([]Rule, 0, len(rg.Rules))
			for _, r := range rg.Rules {
				expr, err := parser.ParseExpr(r.Expr.Value)
				if err != nil {
					return nil, []error{errors.Wrap(err, fn)}
				}

				if r.Alert.Value != "" {
					rules = append(rules, NewAlertingRule(
						r.Alert.Value,
						expr,
						time.Duration(r.For),
						labels.FromMap(r.Labels),
						labels.FromMap(r.Annotations),
						externalLabels,
						shouldRestore,
						log.With(logger, "alert", r.Alert),
					))
					continue
				}
				rules = append(rules, NewRecordingRule(
					r.Record.Value,
					expr,
					labels.FromMap(r.Labels),
				))
			}

			groups[groupKey(fn, rg.Name)] = NewGroup(GroupOptions{
				Name:          rg.Name,
				File:          fn,
				Interval:      itv,
				Rules:         rules,
				ShouldRestore: shouldRestore,
				Opts:          opts,
				done:          done,
			})
		}
	}

	return groups, nil
}
