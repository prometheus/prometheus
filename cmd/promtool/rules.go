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
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
)

// ruleImporter is the importer to backfill rules.
type ruleImporter struct {
	logger log.Logger
	config ruleImporterConfig

	groups      map[string]*rules.Group
	groupLoader rules.GroupLoader

	apiClient v1.API

	writer *tsdb.BlockWriter
}

// ruleImporterConfig is the config for the rule importer.
type ruleImporterConfig struct {
	Start        time.Time
	End          time.Time
	OutputDir    string
	EvalInterval time.Duration
	URL          string
}

// newRuleImporter creates a new rule importer that can be used to backfill rules.
func newRuleImporter(logger log.Logger, config ruleImporterConfig) *ruleImporter {
	return &ruleImporter{
		logger:      logger,
		config:      config,
		groupLoader: rules.FileLoader{},
	}
}

// init initializes the rule importer which creates a new block writer
// and creates an Prometheus API client.
func (importer *ruleImporter) init() error {
	w, err := tsdb.NewBlockWriter(importer.logger,
		importer.config.OutputDir,
		tsdb.DefaultBlockDuration,
	)
	if err != nil {
		return err
	}
	importer.writer = w

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

// close cleans up any open resources.
func (importer *ruleImporter) close() error {
	return importer.writer.Close()
}

// loadGroups reads groups from a list of rule files.
func (importer *ruleImporter) loadGroups(ctx context.Context, filenames []string) (errs []error) {
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
				Opts:     &rules.ManagerOptions{},
			})
		}
	}

	importer.groups = groups
	return nil
}

// importAll evaluates all the groups and rules and creates new time series
// and stores them in new blocks.
func (importer *ruleImporter) importAll(ctx context.Context) []error {
	var errs = []error{}
	var currentBlockEnd time.Time
	var appender storage.Appender

	for name, group := range importer.groups {
		level.Info(importer.logger).Log("backfiller", fmt.Sprintf("processing group, name: %s", name))
		stimeWithAlignment := group.EvalTimestamp(importer.config.Start.UnixNano())

		ts := stimeWithAlignment
		// a 2-hr block that contains all the data for each rule
		for ts.Before(importer.config.End) {
			currentBlockEnd = ts.Add(time.Duration(tsdb.DefaultBlockDuration) * time.Millisecond)
			if currentBlockEnd.After(importer.config.End) {
				currentBlockEnd = importer.config.End
			}
			// should we be creating a new appender for each block?
			appender = importer.writer.Appender(ctx)

			for i, r := range group.Rules() {
				level.Info(importer.logger).Log("backfiller", fmt.Sprintf("processing rule %d, name: %s", i+1, r.Name()))
				err := importer.importRule(ctx, r.Query().String(), r.Name(), r.Labels(), ts, currentBlockEnd, appender)
				if err != nil {
					errs = append(errs, err)
				}
			}

			ts = currentBlockEnd
			_, err := importer.writer.Flush(ctx)
			if err != nil {
				errs = append(errs, err)
			}

			err = appender.Commit()
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

// importRule imports the historical data for a single rule.
func (importer *ruleImporter) importRule(ctx context.Context, ruleExpr, ruleName string, ruleLabels labels.Labels, start, end time.Time, appender storage.Appender) error {
	val, warnings, err := importer.apiClient.QueryRange(ctx,
		ruleExpr,
		v1.Range{
			Start: start,
			End:   end,
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
			currentLabels = append(currentLabels, labels.Label{
				Name:  labels.MetricName,
				Value: ruleName,
			})
			for _, ruleLabel := range ruleLabels {
				currentLabels = append(currentLabels, ruleLabel)
			}
			for k, v := range sample.Metric {
				currentLabels = append(currentLabels, labels.Label{
					Name:  string(k),
					Value: string(v),
				})
			}
			for _, value := range sample.Values {
				_, err := appender.Add(currentLabels, value.Timestamp.Unix(), float64(value.Value))
				if err != nil {
					return err
				}
			}
		}
	default:
		return errors.New("rule result is wrong type")
	}
	return nil
}
