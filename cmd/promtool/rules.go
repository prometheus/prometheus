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
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
)

const maxSamplesInMemory = 5000

var staleNaN = math.Float64frombits(value.StaleNaN)

type queryRangeAPI interface {
	QueryRange(ctx context.Context, query string, r v1.Range) (model.Value, v1.Warnings, error)
}

type ruleImporter struct {
	logger log.Logger
	config ruleImporterConfig

	apiClient queryRangeAPI

	groups      map[string]*rules.Group
	ruleManager *rules.Manager

	startMs            int64
	endMs              int64
	maxBlockDurationMs int64
}

type ruleImporterConfig struct {
	outputDir        string
	start            time.Time
	end              time.Time
	evalInterval     time.Duration
	maxBlockDuration time.Duration
}

// newRuleImporter creates a new rule importer that can be used to parse and evaluate recording rule files and create new series
// written to disk in blocks.
func newRuleImporter(logger log.Logger, config ruleImporterConfig, apiClient queryRangeAPI) *ruleImporter {
	level.Info(logger).Log("backfiller", "new rule importer", "start", config.start.Format(time.RFC822), "end", config.end.Format(time.RFC822))
	return &ruleImporter{
		logger:             logger,
		config:             config,
		apiClient:          apiClient,
		ruleManager:        rules.NewManager(&rules.ManagerOptions{}),
		startMs:            config.start.Unix() * int64(time.Second/time.Millisecond),
		endMs:              config.end.Unix() * int64(time.Second/time.Millisecond),
		maxBlockDurationMs: int64(config.maxBlockDuration / time.Millisecond),
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

		for i, r := range group.Rules() {
			level.Info(importer.logger).Log("backfiller", "processing rule", "id", i, "name", r.Name())
			if err := importer.importRule(ctx, r.Query().String(), r.Name(), r.Labels(), group); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

// importRule queries a prometheus API to evaluate rules at times in the past.
func (importer *ruleImporter) importRule(ctx context.Context, ruleExpr, ruleName string, ruleLabels labels.Labels, grp *rules.Group) (err error) {
	blockDuration := getCompatibleBlockDuration(importer.maxBlockDurationMs)

	interval := grp.Interval()
	// use cache for track staleness of series between blocks
	staleCache := newStalenessCache(interval)

	var (
		appender *ruleBackfillAppender
		closed   bool
		w        *tsdb.BlockWriter
	)
	currentLabels := labels.Labels{}

	for startOfBlock := blockDuration * (importer.startMs / blockDuration); startOfBlock <= importer.endMs; startOfBlock += blockDuration {

		endOfBlock := startOfBlock + blockDuration - 1

		currStart := max(startOfBlock/int64(time.Second/time.Millisecond), importer.config.start.Unix())

		blockStartWithAlignment := grp.EvalTimestamp(time.Unix(currStart, 0).UTC().UnixNano())
		for blockStartWithAlignment.Unix() < currStart {
			blockStartWithAlignment = blockStartWithAlignment.Add(interval)
		}

		blockEndWithAlignment := time.Unix(min(endOfBlock/int64(time.Second/time.Millisecond), importer.config.end.Unix()), 0).UTC()
		if blockStartWithAlignment.After(blockEndWithAlignment) {
			break
		}

		val, warnings, err := importer.apiClient.QueryRange(ctx,
			ruleExpr,
			v1.Range{
				Start: blockStartWithAlignment,
				End:   blockEndWithAlignment,
				Step:  interval,
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
		w, err := tsdb.NewBlockWriter(log.NewNopLogger(), importer.config.outputDir, 2*blockDuration)
		if err != nil {
			return errors.Wrap(err, "new block writer")
		}
		defer func() {
			if !closed {
				err = tsdb_errors.NewMulti(err, w.Close()).Err()
			}
		}()

		appender = newAppenderWithStaleMarkers(
			newMultipleAppender(ctx, w),
			staleCache,
			interval,
		)

		var matrix model.Matrix
		switch val.Type() {
		case model.ValMatrix:
			matrix = val.(model.Matrix)
			for _, sample := range matrix {
				currentLabels = buildLabels(sample.Metric, ruleLabels, ruleName)
				if err := appender.add(ctx, sample.Values, currentLabels); err != nil {
					return errors.Wrap(err, "app.add")
				}
			}
		default:
			return fmt.Errorf("rule result is wrong type %s", val.Type().String())
		}

		if err := appender.flushAndCommit(ctx); err != nil {
			return errors.Wrap(err, "flush and commit")
		}
	}

	// if appender is nil it means we never processed any data so just return
	if appender == nil {
		return err
	}
	// insert a staleNan at the end of all the blocks if the last sample is before the backfill end time.
	if err := appender.insertEndStaleMarker(ctx, importer.config.end, currentLabels); err != nil {
		return errors.Wrap(err, "insert end stale")
	}
	if err := appender.flushAndCommit(ctx); err != nil {
		return errors.Wrap(err, "flush and commit")
	}

	if w == nil {
		return err
	}
	err = tsdb_errors.NewMulti(err, w.Close()).Err()
	closed = true

	return err
}

// buildLabels creates a label set with the labels from the metrics returned from the Query API
// and the labels from the recording rule. Any conflicting rule labels overwrites the metrics
// from the Query API. The rule name overrides the Query API metric name.
func buildLabels(queryMetric model.Metric, ruleLabels labels.Labels, ruleName string) labels.Labels {
	lb := labels.NewBuilder(labels.Labels{})

	for name, value := range queryMetric {
		lb.Set(string(name), string(value))
	}

	// rule labels should override query metric labels so
	// we set them here after the query labels are set
	for _, l := range ruleLabels {
		lb.Set(l.Name, l.Value)
	}

	// set the metric name last so that the rule name
	// overrides the query metric name
	lb.Set(labels.MetricName, ruleName)
	return lb.Labels()
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

type cacheEntry struct {
	ts   time.Time
	lset labels.Labels
}

// stalenessCache tracks staleness of series between blocks of data.
type stalenessCache struct {
	step time.Duration
	// seriesPrev stores the labels of series that were seen previous block.
	seriesPrev map[uint64]*cacheEntry
}

func newStalenessCache(step time.Duration) *stalenessCache {
	return &stalenessCache{
		step:       step,
		seriesPrev: map[uint64]*cacheEntry{},
	}
}

func (c *stalenessCache) trackStaleness(ts time.Time, lset labels.Labels, lsetHash uint64) {
	c.seriesPrev[lsetHash] = &cacheEntry{
		ts:   ts,
		lset: lset,
	}
}

func (c *stalenessCache) isStale(ts time.Time, lset labels.Labels, lsetHash uint64) bool {
	previous, ok := c.seriesPrev[uint64(lsetHash)]
	if !ok {
		c.trackStaleness(ts, lset, lsetHash)
		return false
	}
	return previous.ts.Add(c.step).Before(ts)
}

func (c *stalenessCache) remove(lsetHash uint64) {
	delete(c.seriesPrev, lsetHash)
}

// ruleBackfillAppender adds samples and stale markers where needed.
// stale markers are added when:
// 1) there is a gap in data greater than the eval interval
// 2) the last sample occurs before the desired backfill end time
type ruleBackfillAppender struct {
	app          *multipleAppender
	staleCache   *stalenessCache
	previousTs   time.Time
	evalInterval time.Duration
}

func newAppenderWithStaleMarkers(multiAppender *multipleAppender, cache *stalenessCache, interval time.Duration) *ruleBackfillAppender {
	return &ruleBackfillAppender{
		app:          multiAppender,
		staleCache:   cache,
		evalInterval: interval,
	}
}

func (a *ruleBackfillAppender) add(ctx context.Context, values []model.SamplePair, currentLabels labels.Labels) error {
	labelsHash := currentLabels.Hash()

	// track timestamps between samples in a block to check for staleness
	for _, value := range values {
		currTs := value.Timestamp.Time()
		v := float64(value.Value)

		// when previousTs is zero it means we are at the beginning of a block
		if a.previousTs.IsZero() {
			// check the staleness cache to confirm the current sample is not stale from the previous block
			if a.staleCache.isStale(currTs, currentLabels, labelsHash) {
				if err := a.app.add(ctx, currentLabels, timestamp.FromTime(a.previousTs.Add(a.evalInterval)), staleNaN); err != nil {
					return errors.Wrap(err, "add")
				}
				// remove the sample from the stale marker cache once a stale marker is created for it
				a.staleCache.remove(labelsHash)
			}
		}

		// insert a stale marker if there is a gap in the data greater than the eval interval
		if a.previousTs.Add(a.evalInterval).Before(currTs) && !a.previousTs.IsZero() {
			if err := a.app.add(ctx, currentLabels, timestamp.FromTime(a.previousTs.Add(a.evalInterval)), staleNaN); err != nil {
				return errors.Wrap(err, "add")
			}
		}

		if err := a.app.add(ctx, currentLabels, timestamp.FromTime(currTs), v); err != nil {
			return errors.Wrap(err, "add")
		}

		a.previousTs = currTs
	}

	// add the final sample to the stale cache to track staleness between blocks
	a.staleCache.trackStaleness(a.previousTs, currentLabels, labelsHash)
	return nil
}

func (a *ruleBackfillAppender) flushAndCommit(ctx context.Context) error {
	return a.app.flushAndCommit(ctx)
}

// insertEndStaleMarker inserts a stale marker if the last sample added is before the end of the backfill.
func (a *ruleBackfillAppender) insertEndStaleMarker(ctx context.Context, backfillEnd time.Time, currentLabels labels.Labels) error {
	nextTs := a.previousTs.Add(a.evalInterval)
	if nextTs.Before(backfillEnd) && len(currentLabels) != 0 {
		if err := a.app.add(ctx, currentLabels, timestamp.FromTime(nextTs), staleNaN); err != nil {
			return errors.Wrap(err, "add")
		}
	}
	return nil
}
