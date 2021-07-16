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
	"math"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	p_value "github.com/prometheus/prometheus/pkg/value"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
)

const maxSamplesInMemory = 5000

type queryRangeAPI interface {
	QueryRange(ctx context.Context, query string, r v1.Range) (model.Value, v1.Warnings, error)
}

type ruleImporter struct {
	logger log.Logger
	config ruleImporterConfig

	apiClient queryRangeAPI

	groups      map[string]*rules.Group
	ruleManager *rules.Manager
}

type ruleImporterConfig struct {
	outputDir    string
	start        time.Time
	end          time.Time
	evalInterval time.Duration
}

// newRuleImporter creates a new rule importer that can be used to parse and evaluate recording rule files and create new series
// written to disk in blocks.
func newRuleImporter(logger log.Logger, config ruleImporterConfig, apiClient queryRangeAPI) *ruleImporter {
	level.Info(logger).Log("backfiller", "new rule importer from start", config.start.Format(time.RFC822), " to end", config.end.Format(time.RFC822))
	return &ruleImporter{
		logger:      logger,
		config:      config,
		apiClient:   apiClient,
		ruleManager: rules.NewManager(&rules.ManagerOptions{}),
	}
}

// loadGroups parses groups from a list of recording rule files.
func (importer *ruleImporter) loadGroups(ctx context.Context, filenames []string) (errs []error) {
	groups, errs := importer.ruleManager.LoadGroups(importer.config.evalInterval, labels.Labels{}, "", filenames...)
	if errs != nil {
		return errs
	}
	importer.groups = groups
	return nil
}

// importAll evaluates all the recording rules and creates new time series and writes them to disk in blocks.
func (importer *ruleImporter) importAll(ctx context.Context) (errs []error) {
	for name, group := range importer.groups {
		level.Info(importer.logger).Log("backfiller", "processing group", "name", name)

		stimeWithAlignment := group.EvalTimestamp(importer.config.start.UnixNano())
		for i, r := range group.Rules() {
			level.Info(importer.logger).Log("backfiller", "processing rule", "id", i, "name", r.Name())
			if err := importer.importRule(ctx, r.Query().String(), r.Name(), r.Labels(), stimeWithAlignment, importer.config.end, group); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

type cacheEntry struct {
	ts   time.Time
	lset labels.Labels
}

// stalenessCache tracks staleness of series between blocks of data.
type stalenessCache struct {
	step time.Duration
	// seriesCur store the labels of series that were seen previous block.
	seriesPrev map[uint64]*cacheEntry
}

func newStalenessCache(step time.Duration) *stalenessCache {
	return &stalenessCache{
		step:       step,
		seriesPrev: map[uint64]*cacheEntry{},
	}
}

func (c *stalenessCache) trackStaleness(ts time.Time, lset labels.Labels) {
	c.seriesPrev[lset.Hash()] = &cacheEntry{
		ts:   ts,
		lset: lset,
	}
}

func (c *stalenessCache) isStale(lset labels.Labels, currTs time.Time) bool {
	previous, ok := c.seriesPrev[lset.Hash()]
	if !ok {
		c.trackStaleness(currTs, lset)
		return false
	}
	return previous.ts.Add(c.step).Before(currTs)
}
func (c *stalenessCache) remove(lset labels.Labels) {
	delete(c.seriesPrev, lset.Hash())
}

// importRule queries a prometheus API to evaluate rules at times in the past.
func (importer *ruleImporter) importRule(ctx context.Context, ruleExpr, ruleName string, ruleLabels labels.Labels, ruleBackfillStart, ruleBackfillEnd time.Time, grp *rules.Group) (err error) {
	blockDuration := tsdb.DefaultBlockDuration
	ruleBackfillStartMs := ruleBackfillStart.Unix() * int64(time.Second/time.Millisecond)
	ruleBackfillEndMs := ruleBackfillEnd.Unix() * int64(time.Second/time.Millisecond)

	// use cache for track staleness of series between blocks
	c := newStalenessCache(grp.Interval())

	var currTs, blockEndWithAlignment time.Time
	var app *multipleAppender
	var currentLabels labels.Labels
	var closed bool
	var w *tsdb.BlockWriter
	stale := math.Float64frombits(p_value.StaleNaN)
	for startOfBlock := blockDuration * (ruleBackfillStartMs / blockDuration); startOfBlock <= ruleBackfillEndMs; startOfBlock = startOfBlock + blockDuration {
		endOfBlock := startOfBlock + blockDuration - 1

		currStart := max(startOfBlock/int64(time.Second/time.Millisecond), ruleBackfillStart.Unix())
		blockStartWithAlignment := grp.EvalTimestamp(time.Unix(currStart, 0).UTC().UnixNano())
		blockEndWithAlignment = time.Unix(min(endOfBlock/int64(time.Second/time.Millisecond), ruleBackfillEnd.Unix()), 0).UTC()
		val, warnings, err := importer.apiClient.QueryRange(ctx,
			ruleExpr,
			v1.Range{
				Start: blockStartWithAlignment,
				End:   blockEndWithAlignment,
				Step:  grp.Interval(),
			},
		)
		if err != nil {
			return errors.Wrap(err, "query range")
		}
		if warnings != nil {
			level.Warn(importer.logger).Log("msg", "Range query returned warnings.", "warnings", warnings)
		}

		// To prevent races with compaction, a block writer only allows appending samples
		// that are at most half a block size older than the most recent sample appended so far.
		// However, in the way we use the block writer here, compaction doesn't happen, while we
		// also need to append samples throughout the whole block range. To allow that, we
		// pretend that the block is twice as large here, but only really add sample in the
		// original interval later.
		w, err = tsdb.NewBlockWriter(log.NewNopLogger(), importer.config.outputDir, 2*tsdb.DefaultBlockDuration)
		if err != nil {
			return errors.Wrap(err, "new block writer")
		}
		defer func() {
			if !closed {
				err = tsdb_errors.NewMulti(err, w.Close()).Err()
			}
		}()

		app = newMultipleAppender(ctx, w)
		var matrix model.Matrix
		switch val.Type() {
		case model.ValMatrix:
			matrix = val.(model.Matrix)

			for _, sample := range matrix {
				currentLabels = make(labels.Labels, 0, len(sample.Metric)+len(ruleLabels)+1)
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

				// track timestamps between samples in a block to check for staleness
				var previousTs time.Time
				for _, value := range sample.Values {
					currTs = value.Timestamp.Time()
					v := float64(value.Value)

					// Add a stale markers when:
					// 1) thers is a gap in data greater than the eval interval
					// 2) the last sample occurs before the "end"

					// when previousTs is zero it means we are at the beginning of a block
					if previousTs.IsZero() {
						// check the staleness cache to confirm the current sample is not stale from the previous block
						if c.isStale(currentLabels, currTs) {
							if err := app.add(ctx, currentLabels, timestamp.FromTime(previousTs.Add(grp.Interval())), stale); err != nil {
								return errors.Wrap(err, "add")
							}
							// remove from the stale marker cache once a sample is created for it
							c.remove(currentLabels)
						}
					}

					// insert staleNaN if there is a gap in the data greater than the eval interval
					if previousTs.Add(grp.Interval()).Before(currTs) && !previousTs.IsZero() {
						if err := app.add(ctx, currentLabels, timestamp.FromTime(previousTs.Add(grp.Interval())), stale); err != nil {
							return errors.Wrap(err, "add")
						}
					}

					if err := app.add(ctx, currentLabels, timestamp.FromTime(currTs), v); err != nil {
						return errors.Wrap(err, "add")
					}

					previousTs = currTs
				}

				// add each sample to stale cache to track staleness between blocks
				c.trackStaleness(previousTs, currentLabels)
			}
		default:
			return errors.New(fmt.Sprintf("rule result is wrong type %s", val.Type().String()))
		}

		if err := app.flushAndCommit(ctx); err != nil {
			return errors.Wrap(err, "flush and commit")
		}
	}

	// insert staleNan at the end of all the blocks if its before the backfill end time
	if !currTs.IsZero() {
		nextTs := currTs.Add(grp.Interval())
		if nextTs.Before(ruleBackfillEnd) {
			if err := app.add(ctx, currentLabels, timestamp.FromTime(nextTs), stale); err != nil {
				return errors.Wrap(err, "add")
			}
		}
	}
	if err := app.flushAndCommit(ctx); err != nil {
		return errors.Wrap(err, "flush and commit")
	}
	err = tsdb_errors.NewMulti(err, w.Close()).Err()
	closed = true

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
		return errors.Wrap(err, "multiappender append")
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
		return errors.Wrap(err, "multiappender commit")
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
		return errors.Wrap(err, "multiappender flush")
	}
	return nil
}

func max(x, y int64) int64 {
	if x > y {
		return x
	}
	return y
}

func min(x, y int64) int64 {
	if x < y {
		return x
	}
	return y
}
