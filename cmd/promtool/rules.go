// Copyright The Prometheus Authors
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
	"log/slog"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
)

const maxSamplesInMemory = 5000

type queryRangeAPI interface {
	QueryRange(ctx context.Context, query string, r v1.Range, opts ...v1.Option) (model.Value, v1.Warnings, error)
}

type ruleImporter struct {
	logger *slog.Logger
	config ruleImporterConfig

	apiClient queryRangeAPI

	groups      map[string]*rules.Group
	ruleManager *rules.Manager
}

type ruleImporterConfig struct {
	outputDir            string
	start                time.Time
	end                  time.Time
	evalInterval         time.Duration
	maxBlockDuration     time.Duration
	nameValidationScheme model.ValidationScheme
}

// newRuleImporter creates a new rule importer that can be used to parse and evaluate recording rule files and create new series
// written to disk in blocks.
func newRuleImporter(logger *slog.Logger, config ruleImporterConfig, apiClient queryRangeAPI) *ruleImporter {
	logger.Info("new rule importer", "component", "backfiller", "start", config.start.Format(time.RFC822), "end", config.end.Format(time.RFC822))
	return &ruleImporter{
		logger:    logger,
		config:    config,
		apiClient: apiClient,
		ruleManager: rules.NewManager(&rules.ManagerOptions{
			NameValidationScheme: config.nameValidationScheme,
		}),
	}
}

// loadGroups parses groups from a list of recording rule files.
func (importer *ruleImporter) loadGroups(_ context.Context, filenames []string) (errs []error) {
	groups, errs := importer.ruleManager.LoadGroups(importer.config.evalInterval, labels.Labels{}, "", nil, false, filenames...)
	if errs != nil {
		return errs
	}
	importer.groups = groups
	return nil
}

// importAll evaluates all the recording rules and creates new time series and writes them to disk in blocks.
func (importer *ruleImporter) importAll(ctx context.Context) (errs []error) {
	for name, group := range importer.groups {
		importer.logger.Info("processing group", "component", "backfiller", "name", name)

		for i, r := range group.Rules() {
			importer.logger.Info("processing rule", "component", "backfiller", "id", i, "name", r.Name())
			if err := importer.importRule(ctx, r.Query().String(), r.Name(), r.Labels(), importer.config.start, importer.config.end, int64(importer.config.maxBlockDuration/time.Millisecond), group); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

// importRule queries a prometheus API to evaluate rules at times in the past.
func (importer *ruleImporter) importRule(ctx context.Context, ruleExpr, ruleName string, ruleLabels labels.Labels, start, end time.Time,
	maxBlockDuration int64, grp *rules.Group,
) (err error) {
	blockDuration := getCompatibleBlockDuration(maxBlockDuration)
	startInMs := start.Unix() * int64(time.Second/time.Millisecond)
	endInMs := end.Unix() * int64(time.Second/time.Millisecond)

	for startOfBlock := blockDuration * (startInMs / blockDuration); startOfBlock <= endInMs; startOfBlock += blockDuration {
		endOfBlock := startOfBlock + blockDuration - 1

		currStart := max(startOfBlock/int64(time.Second/time.Millisecond), start.Unix())
		startWithAlignment := grp.EvalTimestamp(time.Unix(currStart, 0).UTC().UnixNano())
		for startWithAlignment.Unix() < currStart {
			startWithAlignment = startWithAlignment.Add(grp.Interval())
		}
		end := time.Unix(min(endOfBlock/int64(time.Second/time.Millisecond), end.Unix()), 0).UTC()
		if end.Before(startWithAlignment) {
			break
		}
		val, warnings, err := importer.apiClient.QueryRange(ctx,
			ruleExpr,
			v1.Range{
				Start: startWithAlignment,
				End:   end,
				Step:  grp.Interval(),
			},
		)
		if err != nil {
			return fmt.Errorf("query range: %w", err)
		}
		if warnings != nil {
			importer.logger.Warn("Range query returned warnings.", "warnings", warnings)
		}

		// To prevent races with compaction, a block writer only allows appending samples
		// that are at most half a block size older than the most recent sample appended so far.
		// However, in the way we use the block writer here, compaction doesn't happen, while we
		// also need to append samples throughout the whole block range. To allow that, we
		// pretend that the block is twice as large here, but only really add sample in the
		// original interval later.
		w, err := tsdb.NewBlockWriter(promslog.NewNopLogger(), importer.config.outputDir, 2*blockDuration)
		if err != nil {
			return fmt.Errorf("new block writer: %w", err)
		}
		var closed bool
		defer func() {
			if !closed {
				err = tsdb_errors.NewMulti(err, w.Close()).Err()
			}
		}()
		app := newMultipleAppender(ctx, w)
		var matrix model.Matrix
		switch val.Type() {
		case model.ValMatrix:
			matrix = val.(model.Matrix)

			for _, sample := range matrix {
				lb := labels.NewBuilder(labels.Labels{})

				for name, value := range sample.Metric {
					lb.Set(string(name), string(value))
				}

				// Setting the rule labels after the output of the query,
				// so they can override query output.
				ruleLabels.Range(func(l labels.Label) {
					lb.Set(l.Name, l.Value)
				})

				lb.Set(labels.MetricName, ruleName)
				lbls := lb.Labels()

				for _, value := range sample.Values {
					if err := app.add(ctx, lbls, timestamp.FromTime(value.Timestamp.Time()), float64(value.Value)); err != nil {
						return fmt.Errorf("add: %w", err)
					}
				}
			}
		default:
			return fmt.Errorf("rule result is wrong type %s", val.Type().String())
		}

		if err := app.flushAndCommit(ctx); err != nil {
			return fmt.Errorf("flush and commit: %w", err)
		}
		err = tsdb_errors.NewMulti(err, w.Close()).Err()
		closed = true
	}

	return err
}

func newMultipleAppender(ctx context.Context, blockWriter *tsdb.BlockWriter) *multipleAppender {
	return &multipleAppender{
		maxSamplesInMemory: maxSamplesInMemory,
		writer:             blockWriter,
		appender:           blockWriter.Appender(ctx),
	}
}

// multipleAppender keeps track of how many series have been added to the current appender.
// If the max samples have been added, then all series are committed and a new appender is created.
type multipleAppender struct {
	maxSamplesInMemory int
	currentSampleCount int
	writer             *tsdb.BlockWriter
	appender           storage.Appender
}

func (m *multipleAppender) add(ctx context.Context, l labels.Labels, t int64, v float64) error {
	if _, err := m.appender.Append(0, l, t, v); err != nil {
		return fmt.Errorf("multiappender append: %w", err)
	}
	m.currentSampleCount++
	if m.currentSampleCount >= m.maxSamplesInMemory {
		return m.commit(ctx)
	}
	return nil
}

func (m *multipleAppender) commit(ctx context.Context) error {
	if m.currentSampleCount == 0 {
		return nil
	}
	if err := m.appender.Commit(); err != nil {
		return fmt.Errorf("multiappender commit: %w", err)
	}
	m.appender = m.writer.Appender(ctx)
	m.currentSampleCount = 0
	return nil
}

func (m *multipleAppender) flushAndCommit(ctx context.Context) error {
	if err := m.commit(ctx); err != nil {
		return err
	}
	if _, err := m.writer.Flush(ctx); err != nil {
		return fmt.Errorf("multiappender flush: %w", err)
	}
	return nil
}
