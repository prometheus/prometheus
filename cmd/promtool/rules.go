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

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/tsdb/importer/blocks"
)

const blockSize = 2 // in hours

// RuleImporter is the importer to backfill rules.
type RuleImporter struct {
	logger log.Logger
	config RuleImporterConfig

	groups      map[string]*rules.Group
	groupLoader rules.GroupLoader

	apiClient v1.API

	writer *blocks.MultiWriter
}

// RuleImporterConfig is the config for the rule importer.
type RuleImporterConfig struct {
	Start        time.Time
	End          time.Time
	EvalInterval time.Duration
	URL          string
}

// NewRuleImporter creates a new rule importer that can be used to backfill rules.
func NewRuleImporter(logger log.Logger, config RuleImporterConfig) *RuleImporter {
	return &RuleImporter{
		config:      config,
		groupLoader: rules.FileLoader{},
	}
}

// Init initializes the rule importer which creates a new block writer
// and creates an Prometheus API client.
func (importer *RuleImporter) Init() error {
	// todo: clean up dir
	newBlockDir, err := ioutil.TempDir("", "rule_blocks")
	if err != nil {
		return err
	}
	importer.writer = blocks.NewMultiWriter(importer.logger, newBlockDir, importer.config.EvalInterval.Nanoseconds())

	config := api.Config{
		Address: importer.config.URL,
	}
	c, err := api.NewClient(config)
	if err != nil {
		return err
	}
	importer.apiClient = v1.NewAPI(c)
	return nil
}

// Close cleans up any open resources.
func (importer *RuleImporter) Close() error {
	// todo: clean up any dirs that were created
	return importer.writer.Close()
}

// LoadGroups reads groups from a list of rule files.
func (importer *RuleImporter) LoadGroups(ctx context.Context, filenames []string) (errs []error) {
	groups := make(map[string]*rules.Group)

	for _, filename := range filenames {
		rgs, errs := importer.groupLoader.Load(filename)
		if errs != nil {
			return errs
		}

		for _, ruleGroup := range rgs.Groups {

			itv := importer.config.EvalInterval
			if ruleGroup.Interval != 0 {
				itv = time.Duration(ruleGroup.Interval)
			}

			rgRules := make([]rules.Rule, 0, len(ruleGroup.Rules))

			for _, r := range ruleGroup.Rules {

				expr, err := importer.groupLoader.Parse(r.Expr.Value)
				if err != nil {
					return []error{errors.Wrap(err, filename)}
				}

				rgRules = append(rgRules, rules.NewRecordingRule(
					r.Record.Value,
					expr,
					labels.FromMap(r.Labels),
				))
			}

			groups[rules.GroupKey(filename, ruleGroup.Name)] = rules.NewGroup(rules.GroupOptions{
				Name:     ruleGroup.Name,
				File:     filename,
				Interval: itv,
				Rules:    rgRules,
			})
		}
	}

	importer.groups = groups
	return nil
}

// ImportAll evaluates all the groups and rules and creates new time series
// and stores them in new blocks.
func (importer *RuleImporter) ImportAll(ctx context.Context) []error {
	var errs = []error{}
	for _, group := range importer.groups {
		stimeWithAlignment := group.EvalTimestamp(importer.config.Start.UnixNano())

		for _, r := range group.Rules() {
			err := importer.ImportRule(ctx, r.Query().String(), stimeWithAlignment, group.Interval())
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	_, err := importer.writer.Flush()
	if err != nil {
		errs = append(errs, err)
	}
	return errs
}

// ImportRule imports the historical data for a single rule.
func (importer *RuleImporter) ImportRule(ctx context.Context, ruleExpr string, stimeWithAlignment time.Time, internval time.Duration) error {
	ts := stimeWithAlignment

	appender := importer.writer.Appender()

	for ts.Before(importer.config.End) {
		currentBlockEnd := ts.Add(blockSize * time.Hour)
		if currentBlockEnd.After(importer.config.End) {
			currentBlockEnd = importer.config.End
		}

		val, warnings, err := importer.apiClient.QueryRange(ctx,
			ruleExpr,
			v1.Range{
				Start: ts,
				End:   currentBlockEnd,
				Step:  importer.config.EvalInterval,
			},
		)
		if err != nil {
			return err
		}
		if warnings != nil {
			fmt.Fprint(os.Stderr, "warning api.QueryRange:", warnings)
		}

		var matrix model.Matrix
		switch val.Type() {
		case model.ValMatrix:
			matrix = val.(model.Matrix)
			for _, sample := range matrix {
				currentLabels := make(labels.Labels, 0, len(sample.Metric))
				for k, v := range sample.Metric {
					currentLabels = append(currentLabels, labels.Label{
						Name:  string(k),
						Value: string(v),
					})
				}
				for _, value := range sample.Values {
					_, err := appender.Add(currentLabels, value.Timestamp.Unix(), float64(value.Value))
					if err != nil {
						// todo: handle other errors, i.e. ErrOutOfOrderSample and ErrDuplicateSampleForTimestamp
						return err
					}
				}
			}
		default:
			return errors.New("rule result is wrong type")
		}

		ts = currentBlockEnd
	}
	_, err := importer.writer.Flush()
	if err != nil {
		return err
	}
	return appender.Commit()
}
