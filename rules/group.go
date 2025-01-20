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
	"context"
	"errors"
	"log/slog"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/promql/parser"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// Group is a set of rules that have a logical relation.
type Group struct {
	name                  string
	file                  string
	interval              time.Duration
	queryOffset           *time.Duration
	limit                 int
	rules                 []Rule
	seriesInPreviousEval  []map[string]labels.Labels // One per Rule.
	staleSeries           []labels.Labels
	opts                  *ManagerOptions
	mtx                   sync.Mutex
	evaluationTime        time.Duration // Time it took to evaluate the group.
	evaluationRuleTimeSum time.Duration // Sum of time it took to evaluate each rule in the group.
	lastEvaluation        time.Time     // Wall-clock time of most recent evaluation.
	lastEvalTimestamp     time.Time     // Time slot used for most recent evaluation.

	shouldRestore bool

	markStale   bool
	done        chan struct{}
	terminated  chan struct{}
	managerDone chan struct{}

	logger *slog.Logger

	metrics *Metrics

	// Rule group evaluation iteration function,
	// defaults to DefaultEvalIterationFunc.
	evalIterationFunc GroupEvalIterationFunc

	appOpts *storage.AppendOptions
}

// GroupEvalIterationFunc is used to implement and extend rule group
// evaluation iteration logic. It is configured in Group.evalIterationFunc,
// and periodically invoked at each group evaluation interval to
// evaluate the rules in the group at that point in time.
// DefaultEvalIterationFunc is the default implementation.
type GroupEvalIterationFunc func(ctx context.Context, g *Group, evalTimestamp time.Time)

type GroupOptions struct {
	Name, File        string
	Interval          time.Duration
	Limit             int
	Rules             []Rule
	ShouldRestore     bool
	Opts              *ManagerOptions
	QueryOffset       *time.Duration
	done              chan struct{}
	EvalIterationFunc GroupEvalIterationFunc
}

// NewGroup makes a new Group with the given name, options, and rules.
func NewGroup(o GroupOptions) *Group {
	opts := o.Opts
	if opts == nil {
		opts = &ManagerOptions{}
	}
	metrics := opts.Metrics
	if metrics == nil {
		metrics = NewGroupMetrics(opts.Registerer)
	}

	key := GroupKey(o.File, o.Name)
	metrics.IterationsMissed.WithLabelValues(key)
	metrics.IterationsScheduled.WithLabelValues(key)
	metrics.EvalTotal.WithLabelValues(key)
	metrics.EvalFailures.WithLabelValues(key)
	metrics.GroupLastEvalTime.WithLabelValues(key)
	metrics.GroupLastDuration.WithLabelValues(key)
	metrics.GroupLastRuleDurationSum.WithLabelValues(key)
	metrics.GroupRules.WithLabelValues(key).Set(float64(len(o.Rules)))
	metrics.GroupSamples.WithLabelValues(key)
	metrics.GroupInterval.WithLabelValues(key).Set(o.Interval.Seconds())

	evalIterationFunc := o.EvalIterationFunc
	if evalIterationFunc == nil {
		evalIterationFunc = DefaultEvalIterationFunc
	}

	if opts.Logger == nil {
		opts.Logger = promslog.NewNopLogger()
	}

	return &Group{
		name:                 o.Name,
		file:                 o.File,
		interval:             o.Interval,
		queryOffset:          o.QueryOffset,
		limit:                o.Limit,
		rules:                o.Rules,
		shouldRestore:        o.ShouldRestore,
		opts:                 opts,
		seriesInPreviousEval: make([]map[string]labels.Labels, len(o.Rules)),
		done:                 make(chan struct{}),
		managerDone:          o.done,
		terminated:           make(chan struct{}),
		logger:               opts.Logger.With("file", o.File, "group", o.Name),
		metrics:              metrics,
		evalIterationFunc:    evalIterationFunc,
		appOpts:              &storage.AppendOptions{DiscardOutOfOrder: true},
	}
}

// Name returns the group name.
func (g *Group) Name() string { return g.name }

// File returns the group's file.
func (g *Group) File() string { return g.file }

// Rules returns the group's rules.
func (g *Group) Rules(matcherSets ...[]*labels.Matcher) []Rule {
	if len(matcherSets) == 0 {
		return g.rules
	}
	var rules []Rule
	for _, rule := range g.rules {
		if matchesMatcherSets(matcherSets, rule.Labels()) {
			rules = append(rules, rule)
		}
	}
	return rules
}

func matches(lbls labels.Labels, matchers ...*labels.Matcher) bool {
	for _, m := range matchers {
		if v := lbls.Get(m.Name); !m.Matches(v) {
			return false
		}
	}
	return true
}

// matchesMatcherSets ensures all matches in each matcher set are ANDed and the set of those is ORed.
func matchesMatcherSets(matcherSets [][]*labels.Matcher, lbls labels.Labels) bool {
	if len(matcherSets) == 0 {
		return true
	}

	var ok bool
	for _, matchers := range matcherSets {
		if matches(lbls, matchers...) {
			ok = true
		}
	}
	return ok
}

// Queryable returns the group's queryable.
func (g *Group) Queryable() storage.Queryable { return g.opts.Queryable }

// Context returns the group's context.
func (g *Group) Context() context.Context { return g.opts.Context }

// Interval returns the group's interval.
func (g *Group) Interval() time.Duration { return g.interval }

// Limit returns the group's limit.
func (g *Group) Limit() int { return g.limit }

func (g *Group) Logger() *slog.Logger { return g.logger }

func (g *Group) run(ctx context.Context) {
	defer close(g.terminated)

	// Wait an initial amount to have consistently slotted intervals.
	evalTimestamp := g.EvalTimestamp(time.Now().UnixNano()).Add(g.interval)
	select {
	case <-time.After(time.Until(evalTimestamp)):
	case <-g.done:
		return
	}

	ctx = promql.NewOriginContext(ctx, map[string]interface{}{
		"ruleGroup": map[string]string{
			"file": g.File(),
			"name": g.Name(),
		},
	})

	// The assumption here is that since the ticker was started after having
	// waited for `evalTimestamp` to pass, the ticks will trigger soon
	// after each `evalTimestamp + N * g.interval` occurrence.
	tick := time.NewTicker(g.interval)
	defer tick.Stop()

	defer func() {
		if !g.markStale {
			return
		}
		go func(now time.Time) {
			for _, rule := range g.seriesInPreviousEval {
				for _, r := range rule {
					g.staleSeries = append(g.staleSeries, r)
				}
			}
			// That can be garbage collected at this point.
			g.seriesInPreviousEval = nil
			// Wait for 2 intervals to give the opportunity to renamed rules
			// to insert new series in the tsdb. At this point if there is a
			// renamed rule, it should already be started.
			select {
			case <-g.managerDone:
			case <-time.After(2 * g.interval):
				g.cleanupStaleSeries(ctx, now)
			}
		}(time.Now())
	}()

	g.evalIterationFunc(ctx, g, evalTimestamp)
	if g.shouldRestore {
		// If we have to restore, we wait for another Eval to finish.
		// The reason behind this is, during first eval (or before it)
		// we might not have enough data scraped, and recording rules would not
		// have updated the latest values, on which some alerts might depend.
		select {
		case <-g.done:
			return
		case <-tick.C:
			missed := (time.Since(evalTimestamp) / g.interval) - 1
			if missed > 0 {
				g.metrics.IterationsMissed.WithLabelValues(GroupKey(g.file, g.name)).Add(float64(missed))
				g.metrics.IterationsScheduled.WithLabelValues(GroupKey(g.file, g.name)).Add(float64(missed))
			}
			evalTimestamp = evalTimestamp.Add((missed + 1) * g.interval)
			g.evalIterationFunc(ctx, g, evalTimestamp)
		}

		restoreStartTime := time.Now()
		g.RestoreForState(restoreStartTime)
		totalRestoreTimeSeconds := time.Since(restoreStartTime).Seconds()
		g.metrics.GroupLastRestoreDuration.WithLabelValues(GroupKey(g.file, g.name)).Set(totalRestoreTimeSeconds)
		g.logger.Debug("'for' state restoration completed", "duration_seconds", totalRestoreTimeSeconds)
		g.shouldRestore = false
	}

	for {
		select {
		case <-g.done:
			return
		default:
			select {
			case <-g.done:
				return
			case <-tick.C:
				missed := (time.Since(evalTimestamp) / g.interval) - 1
				if missed > 0 {
					g.metrics.IterationsMissed.WithLabelValues(GroupKey(g.file, g.name)).Add(float64(missed))
					g.metrics.IterationsScheduled.WithLabelValues(GroupKey(g.file, g.name)).Add(float64(missed))
				}
				evalTimestamp = evalTimestamp.Add((missed + 1) * g.interval)

				g.evalIterationFunc(ctx, g, evalTimestamp)
			}
		}
	}
}

func (g *Group) stopAsync() {
	close(g.done)
}

func (g *Group) waitStopped() {
	<-g.terminated
}

func (g *Group) stop() {
	g.stopAsync()
	g.waitStopped()
}

func (g *Group) hash() uint64 {
	l := labels.New(
		labels.Label{Name: "name", Value: g.name},
		labels.Label{Name: "file", Value: g.file},
	)
	return l.Hash()
}

// AlertingRules returns the list of the group's alerting rules.
func (g *Group) AlertingRules() []*AlertingRule {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	var alerts []*AlertingRule
	for _, rule := range g.rules {
		if alertingRule, ok := rule.(*AlertingRule); ok {
			alerts = append(alerts, alertingRule)
		}
	}
	slices.SortFunc(alerts, func(a, b *AlertingRule) int {
		if a.State() == b.State() {
			return strings.Compare(a.Name(), b.Name())
		}
		return int(b.State() - a.State())
	})
	return alerts
}

// HasAlertingRules returns true if the group contains at least one AlertingRule.
func (g *Group) HasAlertingRules() bool {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	for _, rule := range g.rules {
		if _, ok := rule.(*AlertingRule); ok {
			return true
		}
	}
	return false
}

// GetEvaluationTime returns the time in seconds it took to evaluate the rule group.
func (g *Group) GetEvaluationTime() time.Duration {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	return g.evaluationTime
}

// setEvaluationTime sets the time in seconds the last evaluation took.
func (g *Group) setEvaluationTime(dur time.Duration) {
	g.metrics.GroupLastDuration.WithLabelValues(GroupKey(g.file, g.name)).Set(dur.Seconds())

	g.mtx.Lock()
	defer g.mtx.Unlock()
	g.evaluationTime = dur
}

// GetRuleEvaluationTimeSum returns the sum of the time it took to evaluate each rule in the group irrespective of concurrency.
func (g *Group) GetRuleEvaluationTimeSum() time.Duration {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	return g.evaluationRuleTimeSum
}

// updateRuleEvaluationTimeSum updates evaluationRuleTimeSum which is the sum of the time it took to evaluate each rule in the group irrespective of concurrency.
// It collects the times from the rules themselves.
func (g *Group) updateRuleEvaluationTimeSum() {
	var sum time.Duration
	for _, rule := range g.rules {
		sum += rule.GetEvaluationDuration()
	}

	g.metrics.GroupLastRuleDurationSum.WithLabelValues(GroupKey(g.file, g.name)).Set(sum.Seconds())

	g.mtx.Lock()
	defer g.mtx.Unlock()
	g.evaluationRuleTimeSum = sum
}

// GetLastEvaluation returns the time the last evaluation of the rule group took place.
func (g *Group) GetLastEvaluation() time.Time {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	return g.lastEvaluation
}

// setLastEvaluation updates evaluationTimestamp to the timestamp of when the rule group was last evaluated.
func (g *Group) setLastEvaluation(ts time.Time) {
	g.metrics.GroupLastEvalTime.WithLabelValues(GroupKey(g.file, g.name)).Set(float64(ts.UnixNano()) / 1e9)

	g.mtx.Lock()
	defer g.mtx.Unlock()
	g.lastEvaluation = ts
}

// GetLastEvalTimestamp returns the timestamp of the last evaluation.
func (g *Group) GetLastEvalTimestamp() time.Time {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	return g.lastEvalTimestamp
}

// setLastEvalTimestamp updates lastEvalTimestamp to the timestamp of the last evaluation.
func (g *Group) setLastEvalTimestamp(ts time.Time) {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	g.lastEvalTimestamp = ts
}

// EvalTimestamp returns the immediately preceding consistently slotted evaluation time.
func (g *Group) EvalTimestamp(startTime int64) time.Time {
	var (
		offset = int64(g.hash() % uint64(g.interval))

		// This group's evaluation times differ from the perfect time intervals by `offset` nanoseconds.
		// But we can only use `% interval` to align with the interval. And `% interval` will always
		// align with the perfect time intervals, instead of this group's. Because of this we add
		// `offset` _after_ aligning with the perfect time interval.
		//
		// There can be cases where adding `offset` to the perfect evaluation time can yield a
		// timestamp in the future, which is not what EvalTimestamp should do.
		// So we subtract one `offset` to make sure that `now - (now % interval) + offset` gives an
		// evaluation time in the past.
		adjNow = startTime - offset

		// Adjust to perfect evaluation intervals.
		base = adjNow - (adjNow % int64(g.interval))

		// Add one offset to randomize the evaluation times of this group.
		next = base + offset
	)

	return time.Unix(0, next).UTC()
}

func nameAndLabels(rule Rule) string {
	return rule.Name() + rule.Labels().String()
}

// CopyState copies the alerting rule and staleness related state from the given group.
//
// Rules are matched based on their name and labels. If there are duplicates, the
// first is matched with the first, second with the second etc.
func (g *Group) CopyState(from *Group) {
	g.evaluationTime = from.evaluationTime
	g.lastEvaluation = from.lastEvaluation

	ruleMap := make(map[string][]int, len(from.rules))

	for fi, fromRule := range from.rules {
		nameAndLabels := nameAndLabels(fromRule)
		l := ruleMap[nameAndLabels]
		ruleMap[nameAndLabels] = append(l, fi)
	}

	for i, rule := range g.rules {
		nameAndLabels := nameAndLabels(rule)
		indexes := ruleMap[nameAndLabels]
		if len(indexes) == 0 {
			continue
		}
		fi := indexes[0]
		g.seriesInPreviousEval[i] = from.seriesInPreviousEval[fi]
		ruleMap[nameAndLabels] = indexes[1:]

		ar, ok := rule.(*AlertingRule)
		if !ok {
			continue
		}
		far, ok := from.rules[fi].(*AlertingRule)
		if !ok {
			continue
		}

		for fp, a := range far.active {
			ar.active[fp] = a
		}
	}

	// Handle deleted and unmatched duplicate rules.
	g.staleSeries = from.staleSeries
	for fi, fromRule := range from.rules {
		nameAndLabels := nameAndLabels(fromRule)
		l := ruleMap[nameAndLabels]
		if len(l) != 0 {
			for _, series := range from.seriesInPreviousEval[fi] {
				g.staleSeries = append(g.staleSeries, series)
			}
		}
	}
}

// Eval runs a single evaluation cycle in which all rules are evaluated sequentially.
// Rules can be evaluated concurrently if the `concurrent-rule-eval` feature flag is enabled.
func (g *Group) Eval(ctx context.Context, ts time.Time) {
	var (
		samplesTotal    atomic.Float64
		ruleQueryOffset = g.QueryOffset()
	)
	eval := func(i int, rule Rule, cleanup func()) {
		if cleanup != nil {
			defer cleanup()
		}

		logger := g.logger.With("name", rule.Name(), "index", i)
		ctx, sp := otel.Tracer("").Start(ctx, "rule")
		sp.SetAttributes(attribute.String("name", rule.Name()))
		defer func(t time.Time) {
			sp.End()

			since := time.Since(t)
			g.metrics.EvalDuration.Observe(since.Seconds())
			rule.SetEvaluationDuration(since)
			rule.SetEvaluationTimestamp(t)
		}(time.Now())

		if sp.SpanContext().IsSampled() && sp.SpanContext().HasTraceID() {
			logger = logger.With("trace_id", sp.SpanContext().TraceID())
		}

		g.metrics.EvalTotal.WithLabelValues(GroupKey(g.File(), g.Name())).Inc()

		vector, err := rule.Eval(ctx, ruleQueryOffset, ts, g.opts.QueryFunc, g.opts.ExternalURL, g.Limit())
		if err != nil {
			rule.SetHealth(HealthBad)
			rule.SetLastError(err)
			sp.SetStatus(codes.Error, err.Error())
			g.metrics.EvalFailures.WithLabelValues(GroupKey(g.File(), g.Name())).Inc()

			// Canceled queries are intentional termination of queries. This normally
			// happens on shutdown and thus we skip logging of any errors here.
			var eqc promql.ErrQueryCanceled
			if !errors.As(err, &eqc) {
				logger.Warn("Evaluating rule failed", "rule", rule, "err", err)
			}
			return
		}
		rule.SetHealth(HealthGood)
		rule.SetLastError(nil)
		samplesTotal.Add(float64(len(vector)))

		if ar, ok := rule.(*AlertingRule); ok {
			ar.sendAlerts(ctx, ts, g.opts.ResendDelay, g.interval, g.opts.NotifyFunc)
		}
		var (
			numOutOfOrder = 0
			numTooOld     = 0
			numDuplicates = 0
		)

		app := g.opts.Appendable.Appender(ctx)
		seriesReturned := make(map[string]labels.Labels, len(g.seriesInPreviousEval[i]))
		defer func() {
			if err := app.Commit(); err != nil {
				rule.SetHealth(HealthBad)
				rule.SetLastError(err)
				sp.SetStatus(codes.Error, err.Error())
				g.metrics.EvalFailures.WithLabelValues(GroupKey(g.File(), g.Name())).Inc()

				logger.Warn("Rule sample appending failed", "err", err)
				return
			}
			g.seriesInPreviousEval[i] = seriesReturned
		}()

		for _, s := range vector {
			if s.H != nil {
				_, err = app.AppendHistogram(0, s.Metric, s.T, nil, s.H)
			} else {
				app.SetOptions(g.appOpts)
				_, err = app.Append(0, s.Metric, s.T, s.F)
			}

			if err != nil {
				rule.SetHealth(HealthBad)
				rule.SetLastError(err)
				sp.SetStatus(codes.Error, err.Error())
				unwrappedErr := errors.Unwrap(err)
				if unwrappedErr == nil {
					unwrappedErr = err
				}
				switch {
				case errors.Is(unwrappedErr, storage.ErrOutOfOrderSample):
					numOutOfOrder++
					logger.Debug("Rule evaluation result discarded", "err", err, "sample", s)
				case errors.Is(unwrappedErr, storage.ErrTooOldSample):
					numTooOld++
					logger.Debug("Rule evaluation result discarded", "err", err, "sample", s)
				case errors.Is(unwrappedErr, storage.ErrDuplicateSampleForTimestamp):
					numDuplicates++
					logger.Debug("Rule evaluation result discarded", "err", err, "sample", s)
				default:
					logger.Warn("Rule evaluation result discarded", "err", err, "sample", s)
				}
			} else {
				buf := [1024]byte{}
				seriesReturned[string(s.Metric.Bytes(buf[:]))] = s.Metric
			}
		}
		if numOutOfOrder > 0 {
			logger.Warn("Error on ingesting out-of-order result from rule evaluation", "num_dropped", numOutOfOrder)
		}
		if numTooOld > 0 {
			logger.Warn("Error on ingesting too old result from rule evaluation", "num_dropped", numTooOld)
		}
		if numDuplicates > 0 {
			logger.Warn("Error on ingesting results from rule evaluation with different value but same timestamp", "num_dropped", numDuplicates)
		}

		for metric, lset := range g.seriesInPreviousEval[i] {
			if _, ok := seriesReturned[metric]; !ok {
				// Series no longer exposed, mark it stale.
				_, err = app.Append(0, lset, timestamp.FromTime(ts.Add(-ruleQueryOffset)), math.Float64frombits(value.StaleNaN))
				unwrappedErr := errors.Unwrap(err)
				if unwrappedErr == nil {
					unwrappedErr = err
				}
				switch {
				case unwrappedErr == nil:
				case errors.Is(unwrappedErr, storage.ErrOutOfOrderSample),
					errors.Is(unwrappedErr, storage.ErrTooOldSample),
					errors.Is(unwrappedErr, storage.ErrDuplicateSampleForTimestamp):
					// Do not count these in logging, as this is expected if series
					// is exposed from a different rule.
				default:
					logger.Warn("Adding stale sample failed", "sample", lset.String(), "err", err)
				}
			}
		}
	}

	var wg sync.WaitGroup
	ctrl := g.opts.RuleConcurrencyController
	if ctrl == nil {
		ctrl = sequentialRuleEvalController{}
	}

	batches := ctrl.SplitGroupIntoBatches(ctx, g)
	if len(batches) == 0 {
		// Sequential evaluation when batches aren't set.
		// This is the behaviour without a defined RuleConcurrencyController
		for i, rule := range g.rules {
			// Check if the group has been stopped.
			select {
			case <-g.done:
				return
			default:
			}
			eval(i, rule, nil)
		}
	} else {
		// Concurrent evaluation.
		for _, batch := range batches {
			for _, ruleIndex := range batch {
				// Check if the group has been stopped.
				select {
				case <-g.done:
					wg.Wait()
					return
				default:
				}
				rule := g.rules[ruleIndex]
				if len(batch) > 1 && ctrl.Allow(ctx, g, rule) {
					wg.Add(1)

					go eval(ruleIndex, rule, func() {
						wg.Done()
						ctrl.Done(ctx)
					})
				} else {
					eval(ruleIndex, rule, nil)
				}
			}
			// It is important that we finish processing any rules in this current batch - before we move into the next one.
			wg.Wait()
		}
	}

	g.metrics.GroupSamples.WithLabelValues(GroupKey(g.File(), g.Name())).Set(samplesTotal.Load())
	g.cleanupStaleSeries(ctx, ts)
}

func (g *Group) QueryOffset() time.Duration {
	if g.queryOffset != nil {
		return *g.queryOffset
	}

	if g.opts.DefaultRuleQueryOffset != nil {
		return g.opts.DefaultRuleQueryOffset()
	}

	return time.Duration(0)
}

func (g *Group) cleanupStaleSeries(ctx context.Context, ts time.Time) {
	if len(g.staleSeries) == 0 {
		return
	}
	app := g.opts.Appendable.Appender(ctx)
	app.SetOptions(g.appOpts)
	queryOffset := g.QueryOffset()
	for _, s := range g.staleSeries {
		// Rule that produced series no longer configured, mark it stale.
		_, err := app.Append(0, s, timestamp.FromTime(ts.Add(-queryOffset)), math.Float64frombits(value.StaleNaN))
		unwrappedErr := errors.Unwrap(err)
		if unwrappedErr == nil {
			unwrappedErr = err
		}
		switch {
		case unwrappedErr == nil:
		case errors.Is(unwrappedErr, storage.ErrOutOfOrderSample),
			errors.Is(unwrappedErr, storage.ErrTooOldSample),
			errors.Is(unwrappedErr, storage.ErrDuplicateSampleForTimestamp):
			// Do not count these in logging, as this is expected if series
			// is exposed from a different rule.
		default:
			g.logger.Warn("Adding stale sample for previous configuration failed", "sample", s, "err", err)
		}
	}
	if err := app.Commit(); err != nil {
		g.logger.Warn("Stale sample appending for previous configuration failed", "err", err)
	} else {
		g.staleSeries = nil
	}
}

// RestoreForState restores the 'for' state of the alerts
// by looking up last ActiveAt from storage.
func (g *Group) RestoreForState(ts time.Time) {
	maxtMS := int64(model.TimeFromUnixNano(ts.UnixNano()))
	// We allow restoration only if alerts were active before after certain time.
	mint := ts.Add(-g.opts.OutageTolerance)
	mintMS := int64(model.TimeFromUnixNano(mint.UnixNano()))
	q, err := g.opts.Queryable.Querier(mintMS, maxtMS)
	if err != nil {
		g.logger.Error("Failed to get Querier", "err", err)
		return
	}
	defer func() {
		if err := q.Close(); err != nil {
			g.logger.Error("Failed to close Querier", "err", err)
		}
	}()

	for _, rule := range g.Rules() {
		alertRule, ok := rule.(*AlertingRule)
		if !ok {
			continue
		}

		alertHoldDuration := alertRule.HoldDuration()
		if alertHoldDuration < g.opts.ForGracePeriod {
			// If alertHoldDuration is already less than grace period, we would not
			// like to make it wait for `g.opts.ForGracePeriod` time before firing.
			// Hence we skip restoration, which will make it wait for alertHoldDuration.
			alertRule.SetRestored(true)
			continue
		}

		sset, err := alertRule.QueryForStateSeries(g.opts.Context, q)
		if err != nil {
			g.logger.Error(
				"Failed to restore 'for' state",
				labels.AlertName, alertRule.Name(),
				"stage", "Select",
				"err", err,
			)
			// Even if we failed to query the `ALERT_FOR_STATE` series, we currently have no way to retry the restore process.
			// So the best we can do is mark the rule as restored and let it eventually fire.
			alertRule.SetRestored(true)
			continue
		}

		// While not technically the same number of series we expect, it's as good of an approximation as any.
		seriesByLabels := make(map[string]storage.Series, alertRule.ActiveAlertsCount())
		for sset.Next() {
			seriesByLabels[sset.At().Labels().DropMetricName().String()] = sset.At()
		}

		// No results for this alert rule.
		if len(seriesByLabels) == 0 {
			g.logger.Debug("No series found to restore the 'for' state of the alert rule", labels.AlertName, alertRule.Name())
			alertRule.SetRestored(true)
			continue
		}

		alertRule.ForEachActiveAlert(func(a *Alert) {
			var s storage.Series

			s, ok := seriesByLabels[a.Labels.String()]
			if !ok {
				return
			}
			// Series found for the 'for' state.
			var t int64
			var v float64
			it := s.Iterator(nil)
			for it.Next() == chunkenc.ValFloat {
				t, v = it.At()
			}
			if it.Err() != nil {
				g.logger.Error("Failed to restore 'for' state",
					labels.AlertName, alertRule.Name(), "stage", "Iterator", "err", it.Err())
				return
			}
			if value.IsStaleNaN(v) { // Alert was not active.
				return
			}

			downAt := time.Unix(t/1000, 0).UTC()
			restoredActiveAt := time.Unix(int64(v), 0).UTC()
			timeSpentPending := downAt.Sub(restoredActiveAt)
			timeRemainingPending := alertHoldDuration - timeSpentPending

			switch {
			case timeRemainingPending <= 0:
				// It means that alert was firing when prometheus went down.
				// In the next Eval, the state of this alert will be set back to
				// firing again if it's still firing in that Eval.
				// Nothing to be done in this case.
			case timeRemainingPending < g.opts.ForGracePeriod:
				// (new) restoredActiveAt = (ts + m.opts.ForGracePeriod) - alertHoldDuration
				//                            /* new firing time */      /* moving back by hold duration */
				//
				// Proof of correctness:
				// firingTime = restoredActiveAt.Add(alertHoldDuration)
				//            = ts + m.opts.ForGracePeriod - alertHoldDuration + alertHoldDuration
				//            = ts + m.opts.ForGracePeriod
				//
				// Time remaining to fire = firingTime.Sub(ts)
				//                        = (ts + m.opts.ForGracePeriod) - ts
				//                        = m.opts.ForGracePeriod
				restoredActiveAt = ts.Add(g.opts.ForGracePeriod).Add(-alertHoldDuration)
			default:
				// By shifting ActiveAt to the future (ActiveAt + some_duration),
				// the total pending time from the original ActiveAt
				// would be `alertHoldDuration + some_duration`.
				// Here, some_duration = downDuration.
				downDuration := ts.Sub(downAt)
				restoredActiveAt = restoredActiveAt.Add(downDuration)
			}

			a.ActiveAt = restoredActiveAt
			g.logger.Debug("'for' state restored",
				labels.AlertName, alertRule.Name(), "restored_time", a.ActiveAt.Format(time.RFC850),
				"labels", a.Labels.String())
		})

		alertRule.SetRestored(true)
	}
}

// Equals return if two groups are the same.
func (g *Group) Equals(ng *Group) bool {
	if g.name != ng.name {
		return false
	}

	if g.file != ng.file {
		return false
	}

	if g.interval != ng.interval {
		return false
	}

	if g.limit != ng.limit {
		return false
	}

	if ((g.queryOffset == nil) != (ng.queryOffset == nil)) || (g.queryOffset != nil && ng.queryOffset != nil && *g.queryOffset != *ng.queryOffset) {
		return false
	}

	if len(g.rules) != len(ng.rules) {
		return false
	}

	for i, gr := range g.rules {
		if gr.String() != ng.rules[i].String() {
			return false
		}
	}

	return true
}

// GroupKey group names need not be unique across filenames.
func GroupKey(file, name string) string {
	return file + ";" + name
}

// Constants for instrumentation.
const namespace = "prometheus"

// Metrics for rule evaluation.
type Metrics struct {
	EvalDuration             prometheus.Summary
	IterationDuration        prometheus.Summary
	IterationsMissed         *prometheus.CounterVec
	IterationsScheduled      *prometheus.CounterVec
	EvalTotal                *prometheus.CounterVec
	EvalFailures             *prometheus.CounterVec
	GroupInterval            *prometheus.GaugeVec
	GroupLastEvalTime        *prometheus.GaugeVec
	GroupLastDuration        *prometheus.GaugeVec
	GroupLastRuleDurationSum *prometheus.GaugeVec
	GroupLastRestoreDuration *prometheus.GaugeVec
	GroupRules               *prometheus.GaugeVec
	GroupSamples             *prometheus.GaugeVec
}

// NewGroupMetrics creates a new instance of Metrics and registers it with the provided registerer,
// if not nil.
func NewGroupMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		EvalDuration: prometheus.NewSummary(
			prometheus.SummaryOpts{
				Namespace:  namespace,
				Name:       "rule_evaluation_duration_seconds",
				Help:       "The duration for a rule to execute.",
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			}),
		IterationDuration: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       "rule_group_duration_seconds",
			Help:       "The duration of rule group evaluations.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		}),
		IterationsMissed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rule_group_iterations_missed_total",
				Help:      "The total number of rule group evaluations missed due to slow rule group evaluation.",
			},
			[]string{"rule_group"},
		),
		IterationsScheduled: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rule_group_iterations_total",
				Help:      "The total number of scheduled rule group evaluations, whether executed or missed.",
			},
			[]string{"rule_group"},
		),
		EvalTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rule_evaluations_total",
				Help:      "The total number of rule evaluations.",
			},
			[]string{"rule_group"},
		),
		EvalFailures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rule_evaluation_failures_total",
				Help:      "The total number of rule evaluation failures.",
			},
			[]string{"rule_group"},
		),
		GroupInterval: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rule_group_interval_seconds",
				Help:      "The interval of a rule group.",
			},
			[]string{"rule_group"},
		),
		GroupLastEvalTime: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rule_group_last_evaluation_timestamp_seconds",
				Help:      "The timestamp of the last rule group evaluation in seconds.",
			},
			[]string{"rule_group"},
		),
		GroupLastDuration: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rule_group_last_duration_seconds",
				Help:      "The duration of the last rule group evaluation.",
			},
			[]string{"rule_group"},
		),
		GroupLastRuleDurationSum: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rule_group_last_rule_duration_sum_seconds",
				Help:      "The sum of time in seconds it took to evaluate each rule in the group regardless of concurrency. This should be higher than the group duration if rules are evaluated concurrently.",
			},
			[]string{"rule_group"},
		),
		GroupLastRestoreDuration: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rule_group_last_restore_duration_seconds",
				Help:      "The duration of the last alert rules alerts restoration using the `ALERTS_FOR_STATE` series.",
			},
			[]string{"rule_group"},
		),
		GroupRules: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rule_group_rules",
				Help:      "The number of rules.",
			},
			[]string{"rule_group"},
		),
		GroupSamples: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rule_group_last_evaluation_samples",
				Help:      "The number of samples returned during the last rule group evaluation.",
			},
			[]string{"rule_group"},
		),
	}

	if reg != nil {
		reg.MustRegister(
			m.EvalDuration,
			m.IterationDuration,
			m.IterationsMissed,
			m.IterationsScheduled,
			m.EvalTotal,
			m.EvalFailures,
			m.GroupInterval,
			m.GroupLastEvalTime,
			m.GroupLastDuration,
			m.GroupLastRuleDurationSum,
			m.GroupLastRestoreDuration,
			m.GroupRules,
			m.GroupSamples,
		)
	}

	return m
}

// dependencyMap is a data-structure which contains the relationships between rules within a group.
// It is used to describe the dependency associations between rules in a group whereby one rule uses the
// output metric produced by another rule in its expression (i.e. as its "input").
type dependencyMap map[Rule][]Rule

// dependents returns the rules which use the output of the given rule as one of their inputs.
func (m dependencyMap) dependents(r Rule) []Rule {
	return m[r]
}

// dependencies returns the rules on which the given rule is dependent for input.
func (m dependencyMap) dependencies(r Rule) []Rule {
	if len(m) == 0 {
		return []Rule{}
	}

	var dependencies []Rule
	for rule, dependents := range m {
		if slices.Contains(dependents, r) {
			dependencies = append(dependencies, rule)
		}
	}

	return dependencies
}

// isIndependent determines whether the given rule is not dependent on another rule for its input, nor is any other rule
// dependent on its output.
func (m dependencyMap) isIndependent(r Rule) bool {
	if m == nil {
		return false
	}

	return len(m.dependents(r)) == 0 && len(m.dependencies(r)) == 0
}

// buildDependencyMap builds a data-structure which contains the relationships between rules within a group.
//
// Alert rules, by definition, cannot have any dependents - but they can have dependencies. Any recording rule on whose
// output an Alert rule depends will not be able to run concurrently.
//
// There is a class of rule expressions which are considered "indeterminate", because either relationships cannot be
// inferred, or concurrent evaluation of rules depending on these series would produce undefined/unexpected behaviour:
//   - wildcard queriers like {cluster="prod1"} which would match every series with that label selector
//   - any "meta" series (series produced by Prometheus itself) like ALERTS, ALERTS_FOR_STATE
//
// Rules which are independent can run concurrently with no side-effects.
func buildDependencyMap(rules []Rule) dependencyMap {
	dependencies := make(dependencyMap)

	if len(rules) <= 1 {
		// No relationships if group has 1 or fewer rules.
		return dependencies
	}

	inputs := make(map[string][]Rule, len(rules))
	outputs := make(map[string][]Rule, len(rules))

	var indeterminate bool

	for _, rule := range rules {
		if indeterminate {
			break
		}

		name := rule.Name()
		outputs[name] = append(outputs[name], rule)

		parser.Inspect(rule.Query(), func(node parser.Node, path []parser.Node) error {
			if n, ok := node.(*parser.VectorSelector); ok {
				// A wildcard metric expression means we cannot reliably determine if this rule depends on any other,
				// which means we cannot safely run any rules concurrently.
				if n.Name == "" && len(n.LabelMatchers) > 0 {
					indeterminate = true
					return nil
				}

				// Rules which depend on "meta-metrics" like ALERTS and ALERTS_FOR_STATE will have undefined behaviour
				// if they run concurrently.
				if n.Name == alertMetricName || n.Name == alertForStateMetricName {
					indeterminate = true
					return nil
				}

				inputs[n.Name] = append(inputs[n.Name], rule)
			}
			return nil
		})
	}

	if indeterminate {
		return nil
	}

	for output, outRules := range outputs {
		for _, outRule := range outRules {
			if inRules, found := inputs[output]; found && len(inRules) > 0 {
				dependencies[outRule] = append(dependencies[outRule], inRules...)
			}
		}
	}

	return dependencies
}
