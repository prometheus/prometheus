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
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
)

type queryRangeAPI interface {
	QueryRange(ctx context.Context, query string, r v1.Range) (model.Value, v1.Warnings, error)
}

type ruleImporter struct {
	logger log.Logger
	config ruleImporterConfig

	apiClient queryRangeAPI

	appender *multipleAppender

	groups      map[string]*rules.Group
	ruleManager *rules.Manager
}

type ruleImporterConfig struct {
	Start        time.Time
	End          time.Time
	EvalInterval time.Duration
}

// newRuleImporter creates a new rule importer that can be used to parse and evaluate recording rule files and create new series
// written to disk in blocks.
func newRuleImporter(logger log.Logger, config ruleImporterConfig, apiClient queryRangeAPI, appender *multipleAppender) *ruleImporter {
	return &ruleImporter{
		logger:      logger,
		config:      config,
		apiClient:   apiClient,
		appender:    appender,
		ruleManager: rules.NewManager(&rules.ManagerOptions{}),
	}
}

// loadGroups parses groups from a list of recording rule files.
func (importer *ruleImporter) loadGroups(ctx context.Context, filenames []string) (errs []error) {
	groups, errs := importer.ruleManager.LoadGroups(importer.config.EvalInterval, labels.Labels{}, filenames...)
	if errs != nil {
		return errs
	}
	importer.groups = groups
	return nil
}

// importAll evaluates all the recording rules and creates new time series and writes them to disk in blocks.
func (importer *ruleImporter) importAll(ctx context.Context) (errs []error) {
	for name, group := range importer.groups {
		level.Info(importer.logger).Log("backfiller", fmt.Sprintf("processing group, name: %s", name))

		stimeWithAlignment := group.EvalTimestamp(importer.config.Start.UnixNano())
		for i, r := range group.Rules() {
			level.Info(importer.logger).Log("backfiller", fmt.Sprintf("processing rule %d, name: %s", i+1, r.Name()))
			if err := importer.importRule(ctx, r.Query().String(), r.Name(), r.Labels(), stimeWithAlignment, importer.config.End, group.Interval()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if err := importer.appender.flushAndCommit(ctx); err != nil {
		errs = append(errs, err)
	}
	return errs
}

// importRule queries a prometheus API to evaluate rules at times in the past.
func (importer *ruleImporter) importRule(ctx context.Context, ruleExpr, ruleName string, ruleLabels labels.Labels, start, end time.Time, interval time.Duration) error {

	// This loop breaks up the calls to the QueryRange API into 2 hr chunks so that we
	// don't ever request too much data or take to long to process to avoid timeout.
	for start.Before(end) {
		currentBlockEnd := start.Add(time.Duration(tsdb.DefaultBlockDuration) * time.Millisecond)
		if currentBlockEnd.After(end) {
			currentBlockEnd = end
		}

		val, warnings, err := importer.apiClient.QueryRange(ctx,
			ruleExpr,
			v1.Range{
				Start: start,
				End:   end,
				Step:  interval,
			},
		)
		if err != nil {
			return err
		}
		if warnings != nil {
			level.Warn(importer.logger).Log("backfiller", fmt.Sprintf("warnings QueryRange api: %v", warnings))
		}

		var matrix model.Matrix
		switch val.Type() {
		case model.ValMatrix:
			matrix = val.(model.Matrix)
			for _, sample := range matrix {
				currentLabels := make(labels.Labels, 0, len(sample.Metric)+len(ruleLabels)+1)
				currentLabels = append(currentLabels, labels.Label{
					Name:  labels.MetricName,
					Value: ruleName,
				})

				currentLabels = append(currentLabels, ruleLabels...)

				for name, value := range sample.Metric {
					currentLabels = append(currentLabels, labels.Label{
						Name:  string(name),
						Value: string(value),
					})
				}
				for _, value := range sample.Values {
					if err := importer.appender.add(ctx, currentLabels, timestamp.FromTime(value.Timestamp.Time()), float64(value.Value)); err != nil {
						return err
					}
				}
			}
		default:
			return errors.New(fmt.Sprintf("rule result is wrong type %s", val.Type().String()))
		}
		start = currentBlockEnd
	}
	return nil
}

// multipleAppender keeps track of how many series have been added to the current appender.
// If the max samples have been added, then all series are flushed to disk and commited and a new
// appender is created.
type multipleAppender struct {
	maxSamplesInMemory int
	currentSampleCount int
	writer             *tsdb.BlockWriter
	appender           storage.Appender
}

func newMultipleAppender(ctx context.Context, maxSamplesInMemory int, blockWriter *tsdb.BlockWriter) *multipleAppender {
	return &multipleAppender{
		maxSamplesInMemory: maxSamplesInMemory,
		writer:             blockWriter,
		appender:           blockWriter.Appender(ctx),
	}
}

func (m *multipleAppender) add(ctx context.Context, l labels.Labels, t int64, v float64) error {
	if _, err := m.appender.Add(l, t, v); err != nil {
		return err
	}
	m.currentSampleCount++
	if m.currentSampleCount >= m.maxSamplesInMemory {
		return m.flushAndCommit(ctx)
	}
	return nil
}

func (m *multipleAppender) flushAndCommit(ctx context.Context) error {
	if m.currentSampleCount == 0 {
		return nil
	}
	if err := m.appender.Commit(); err != nil {
		return err
	}
	if _, err := m.writer.Flush(ctx); err != nil {
		return err
	}
	m.appender = m.writer.Appender(ctx)
	m.currentSampleCount = 0
	return nil
}
