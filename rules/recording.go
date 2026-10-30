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

package rules

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"go.uber.org/atomic"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/schema"
)

var (
	_ Rule             = (*RecordingRule)(nil)
	_ ruleWithMetadata = (*RecordingRule)(nil)
)

// ErrDuplicateRecordingLabelSet is returned when a recording rule evaluation produces
// metrics with identical labelsets after applying rule labels.
var ErrDuplicateRecordingLabelSet = errors.New("vector contains metrics with the same labelset after applying rule labels")

// A RecordingRule records its vector expression into new timeseries.
type RecordingRule struct {
	name   string
	vector parser.Expr
	labels labels.Labels
	meta   metadata.Metadata
	// The health of the recording rule.
	health *atomic.String
	// Timestamp of last evaluation of the recording rule.
	evaluationTimestamp *atomic.Time
	// The last error seen by the recording rule.
	lastError *atomic.Error
	// Duration of how long it took to evaluate the recording rule.
	evaluationDuration *atomic.Duration

	dependenciesMutex sync.RWMutex
	dependentRules    []Rule
	dependencyRules   []Rule
}

// NewRecordingRule returns a new recording rule.
func NewRecordingRule(name string, vector parser.Expr, lset labels.Labels) *RecordingRule {
	return NewRecordingRuleWithMetadata(name, vector, lset, metadata.Metadata{})
}

// NewRecordingRuleWithMetadata returns a new recording rule with configured metric metadata.
func NewRecordingRuleWithMetadata(name string, vector parser.Expr, lset labels.Labels, meta metadata.Metadata) *RecordingRule {
	return &RecordingRule{
		name:                name,
		vector:              vector,
		labels:              lset,
		meta:                meta,
		health:              atomic.NewString(string(HealthUnknown)),
		evaluationTimestamp: atomic.NewTime(time.Time{}),
		evaluationDuration:  atomic.NewDuration(0),
		lastError:           atomic.NewError(nil),
	}
}

// Name returns the rule name.
func (rule *RecordingRule) Name() string {
	return rule.name
}

// Query returns the rule query expression.
func (rule *RecordingRule) Query() parser.Expr {
	return rule.vector
}

// Labels returns the rule labels.
func (rule *RecordingRule) Labels() labels.Labels {
	return rule.labels
}

// Metadata returns the configured metric metadata for the recording rule.
func (rule *RecordingRule) Metadata() metadata.Metadata {
	return rule.meta
}

// Eval evaluates the rule and then overrides the metric names and labels accordingly.
func (rule *RecordingRule) Eval(ctx context.Context, queryOffset time.Duration, ts time.Time, query QueryFunc, externalURL *url.URL, limit int) (promql.Vector, error) {
	vec, _, err := rule.evalWithMetadata(ctx, queryOffset, ts, query, externalURL, limit, false, false)
	return vec, err
}

func (rule *RecordingRule) evalWithMetadata(
	ctx context.Context,
	queryOffset time.Duration,
	ts time.Time,
	query QueryFunc,
	_ *url.URL,
	limit int,
	appendMetadata bool,
	enableTypeAndUnitLabels bool,
) (promql.Vector, []metadata.Metadata, error) {
	ctx = NewOriginContext(ctx, NewRuleDetail(rule))
	vector, err := query(ctx, rule.vector.String(), ts.Add(-queryOffset))
	if err != nil {
		return nil, nil, err
	}

	var metas []metadata.Metadata
	if appendMetadata && len(vector) > 0 {
		metas = make([]metadata.Metadata, len(vector))
	}

	// Override the metric name, rule labels, and schema metadata labels.
	lb := labels.NewBuilder(labels.EmptyLabels())

	for i := range vector {
		sample := &vector[i]

		var meta metadata.Metadata
		if appendMetadata || enableTypeAndUnitLabels {
			meta = rule.deduceSampleMetadata(sample, enableTypeAndUnitLabels)
		}

		lb.Reset(sample.Metric)
		if !enableTypeAndUnitLabels {
			// Strip any reserved __type__ and __unit__ labels and set only __name__.
			schema.Metadata{Name: rule.name}.SetToLabels(lb)
			rule.labels.Range(func(l labels.Label) {
				lb.Set(l.Name, l.Value)
			})
		} else {
			rule.labels.Range(func(l labels.Label) {
				lb.Set(l.Name, l.Value)
			})
			// Set __name__, __type__, and __unit__ before duplicate labelset detection
			// and staleness tracking so schema labels stay authoritative.
			schema.Metadata{
				Name: rule.name,
				Type: meta.Type,
				Unit: meta.Unit,
			}.SetToLabels(lb)
		}

		sample.Metric = lb.Labels()
		if appendMetadata {
			metas[i] = meta
		}
	}

	// Check that the rule does not produce identical metrics after applying
	// labels.
	if vector.ContainsSameLabelset() {
		return nil, nil, ErrDuplicateRecordingLabelSet
	}

	numSeries := len(vector)
	if limit > 0 && numSeries > limit {
		return nil, nil, fmt.Errorf("exceeded limit of %d with %d series", limit, numSeries)
	}

	rule.SetHealth(HealthGood)
	rule.SetLastError(err)
	return vector, metas, nil
}

// deduceSampleMetadata infers metric metadata (type, unit, help) for an evaluated
// recording rule sample using YAML overrides, preserved schema labels, and sample value kind.
func (rule *RecordingRule) deduceSampleMetadata(sample *promql.Sample, enableTypeAndUnitLabels bool) metadata.Metadata {
	isHist := sample.H != nil
	var preserved schema.Metadata
	if origName := sample.Metric.Get(labels.MetricName); origName != "" {
		if enableTypeAndUnitLabels {
			preserved = schema.NewMetadataFromLabels(sample.Metric)
		} else {
			preserved.Name = origName
			// NOTE(bwplotka): Until native TSDB metadata lookup lands (prometheus#19563),
			// fall back to standard Prometheus naming suffixes when __name__ survived
			// PromQL evaluation unchanged (DropName == false).
			switch {
			case strings.HasSuffix(origName, "_total"):
				preserved.Type = model.MetricTypeCounter
			case strings.HasSuffix(origName, "_info"):
				preserved.Type = model.MetricTypeInfo
			}
		}
	}

	var mType model.MetricType
	switch {
	case rule.meta.Type != "" && isSampleTypeCompatible(rule.meta.Type, isHist):
		mType = rule.meta.Type
	case !preserved.IsTypeEmpty() && isSampleTypeCompatible(preserved.Type, isHist):
		mType = preserved.Type
	case isHist:
		if sample.H.CounterResetHint == histogram.GaugeType {
			mType = model.MetricTypeGaugeHistogram
		} else {
			mType = model.MetricTypeHistogram
		}
	default:
		mType = model.MetricTypeGauge
	}

	unit := rule.meta.Unit
	if unit == "" {
		unit = preserved.Unit
	}

	return metadata.Metadata{
		Type: mType,
		Unit: unit,
		Help: rule.meta.Help,
	}
}

// isSampleTypeCompatible reports whether t is valid for the runtime sample value kind.
// Native histogram samples require histogram or gaugehistogram metadata, whereas
// float samples cannot use native histogram metadata types.
func isSampleTypeCompatible(t model.MetricType, isHistogram bool) bool {
	if isHistogram {
		return t == model.MetricTypeHistogram || t == model.MetricTypeGaugeHistogram
	}
	return t != model.MetricTypeHistogram && t != model.MetricTypeGaugeHistogram
}

// LogValue returns the rule's representation for logging.
func (rule *RecordingRule) LogValue() slog.Value {
	if rule == nil {
		return slog.AnyValue(nil)
	}
	return slog.StringValue(rule.String())
}

func (rule *RecordingRule) String() string {
	r := rulefmt.Rule{
		Record: rule.name,
		Expr:   rule.vector.String(),
		Labels: rule.labels.Map(),
		Help:   rule.meta.Help,
		Type:   rule.meta.Type,
		Unit:   rule.meta.Unit,
	}

	byt, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Sprintf("error marshaling recording rule: %q", err.Error())
	}

	return string(byt)
}

// SetEvaluationDuration updates evaluationDuration to the time in seconds it took to evaluate the rule on its last evaluation.
func (rule *RecordingRule) SetEvaluationDuration(dur time.Duration) {
	rule.evaluationDuration.Store(dur)
}

// SetLastError sets the current error seen by the recording rule.
func (rule *RecordingRule) SetLastError(err error) {
	rule.lastError.Store(err)
}

// LastError returns the last error seen by the recording rule.
func (rule *RecordingRule) LastError() error {
	return rule.lastError.Load()
}

// SetHealth sets the current health of the recording rule.
func (rule *RecordingRule) SetHealth(health RuleHealth) {
	rule.health.Store(string(health))
}

// Health returns the current health of the recording rule.
func (rule *RecordingRule) Health() RuleHealth {
	return RuleHealth(rule.health.Load())
}

// GetEvaluationDuration returns the time in seconds it took to evaluate the recording rule.
func (rule *RecordingRule) GetEvaluationDuration() time.Duration {
	return rule.evaluationDuration.Load()
}

// SetEvaluationTimestamp updates evaluationTimestamp to the timestamp of when the rule was last evaluated.
func (rule *RecordingRule) SetEvaluationTimestamp(ts time.Time) {
	rule.evaluationTimestamp.Store(ts)
}

// GetEvaluationTimestamp returns the time the evaluation took place.
func (rule *RecordingRule) GetEvaluationTimestamp() time.Time {
	return rule.evaluationTimestamp.Load()
}

func (rule *RecordingRule) SetDependentRules(dependents []Rule) {
	rule.dependenciesMutex.Lock()
	defer rule.dependenciesMutex.Unlock()

	rule.dependentRules = make([]Rule, len(dependents))
	copy(rule.dependentRules, dependents)
}

func (rule *RecordingRule) NoDependentRules() bool {
	rule.dependenciesMutex.RLock()
	defer rule.dependenciesMutex.RUnlock()

	if rule.dependentRules == nil {
		return false // We don't know if there are dependent rules.
	}

	return len(rule.dependentRules) == 0
}

func (rule *RecordingRule) DependentRules() []Rule {
	rule.dependenciesMutex.RLock()
	defer rule.dependenciesMutex.RUnlock()
	return rule.dependentRules
}

func (rule *RecordingRule) SetDependencyRules(dependencies []Rule) {
	rule.dependenciesMutex.Lock()
	defer rule.dependenciesMutex.Unlock()

	rule.dependencyRules = make([]Rule, len(dependencies))
	copy(rule.dependencyRules, dependencies)
}

func (rule *RecordingRule) NoDependencyRules() bool {
	rule.dependenciesMutex.RLock()
	defer rule.dependenciesMutex.RUnlock()

	if rule.dependencyRules == nil {
		return false // We don't know if there are dependency rules.
	}

	return len(rule.dependencyRules) == 0
}

func (rule *RecordingRule) DependencyRules() []Rule {
	rule.dependenciesMutex.RLock()
	defer rule.dependenciesMutex.RUnlock()
	return rule.dependencyRules
}
