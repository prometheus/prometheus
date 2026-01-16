// Code generated from semantic convention specification. DO NOT EDIT.

// Package metrics provides Prometheus instrumentation types for metrics
// defined in this semantic convention registry.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Attribute is an interface for metric label attributes.
type Attribute interface {
	ID() string
	Value() string
}
type RuleGroupAttr string

func (a RuleGroupAttr) ID() string {
	return "rule_group"
}

func (a RuleGroupAttr) Value() string {
	return string(a)
}

// PrometheusRuleEvaluationDurationHistogramSeconds records the duration of rule evaluations as a histogram.
type PrometheusRuleEvaluationDurationHistogramSeconds struct {
	prometheus.Histogram
}

// NewPrometheusRuleEvaluationDurationHistogramSeconds returns a new PrometheusRuleEvaluationDurationHistogramSeconds instrument.
func NewPrometheusRuleEvaluationDurationHistogramSeconds() PrometheusRuleEvaluationDurationHistogramSeconds {
	return PrometheusRuleEvaluationDurationHistogramSeconds{
		Histogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "prometheus_rule_evaluation_duration_histogram_seconds",
			Help: "The duration of rule evaluations as a histogram.",
		}),
	}
}

// PrometheusRuleEvaluationDurationSeconds records the duration of rule group evaluations.
type PrometheusRuleEvaluationDurationSeconds struct {
	prometheus.Histogram
}

// NewPrometheusRuleEvaluationDurationSeconds returns a new PrometheusRuleEvaluationDurationSeconds instrument.
func NewPrometheusRuleEvaluationDurationSeconds() PrometheusRuleEvaluationDurationSeconds {
	return PrometheusRuleEvaluationDurationSeconds{
		Histogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "prometheus_rule_evaluation_duration_seconds",
			Help: "The duration of rule group evaluations.",
		}),
	}
}

// PrometheusRuleEvaluationFailuresTotal records the total number of rule evaluation failures.
type PrometheusRuleEvaluationFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusRuleEvaluationFailuresTotal returns a new PrometheusRuleEvaluationFailuresTotal instrument.
func NewPrometheusRuleEvaluationFailuresTotal() PrometheusRuleEvaluationFailuresTotal {
	labels := []string{
		"rule_group",
	}
	return PrometheusRuleEvaluationFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_rule_evaluation_failures_total",
			Help: "The total number of rule evaluation failures.",
		}, labels),
	}
}

type PrometheusRuleEvaluationFailuresTotalAttr interface {
	Attribute
	implPrometheusRuleEvaluationFailuresTotal()
}

func (a RuleGroupAttr) implPrometheusRuleEvaluationFailuresTotal() {}

func (m PrometheusRuleEvaluationFailuresTotal) With(
	extra ...PrometheusRuleEvaluationFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"rule_group": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusRuleEvaluationsTotal records the total number of rule evaluations.
type PrometheusRuleEvaluationsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusRuleEvaluationsTotal returns a new PrometheusRuleEvaluationsTotal instrument.
func NewPrometheusRuleEvaluationsTotal() PrometheusRuleEvaluationsTotal {
	labels := []string{
		"rule_group",
	}
	return PrometheusRuleEvaluationsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_rule_evaluations_total",
			Help: "The total number of rule evaluations.",
		}, labels),
	}
}

type PrometheusRuleEvaluationsTotalAttr interface {
	Attribute
	implPrometheusRuleEvaluationsTotal()
}

func (a RuleGroupAttr) implPrometheusRuleEvaluationsTotal() {}

func (m PrometheusRuleEvaluationsTotal) With(
	extra ...PrometheusRuleEvaluationsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"rule_group": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusRuleGroupDurationHistogramSeconds records the duration of rule group evaluations as a histogram.
type PrometheusRuleGroupDurationHistogramSeconds struct {
	prometheus.Histogram
}

// NewPrometheusRuleGroupDurationHistogramSeconds returns a new PrometheusRuleGroupDurationHistogramSeconds instrument.
func NewPrometheusRuleGroupDurationHistogramSeconds() PrometheusRuleGroupDurationHistogramSeconds {
	return PrometheusRuleGroupDurationHistogramSeconds{
		Histogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "prometheus_rule_group_duration_histogram_seconds",
			Help: "The duration of rule group evaluations as a histogram.",
		}),
	}
}

// PrometheusRuleGroupDurationSeconds records the duration of rule group evaluations.
type PrometheusRuleGroupDurationSeconds struct {
	prometheus.Histogram
}

// NewPrometheusRuleGroupDurationSeconds returns a new PrometheusRuleGroupDurationSeconds instrument.
func NewPrometheusRuleGroupDurationSeconds() PrometheusRuleGroupDurationSeconds {
	return PrometheusRuleGroupDurationSeconds{
		Histogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "prometheus_rule_group_duration_seconds",
			Help: "The duration of rule group evaluations.",
		}),
	}
}

// PrometheusRuleGroupIntervalSeconds records the interval of a rule group.
type PrometheusRuleGroupIntervalSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusRuleGroupIntervalSeconds returns a new PrometheusRuleGroupIntervalSeconds instrument.
func NewPrometheusRuleGroupIntervalSeconds() PrometheusRuleGroupIntervalSeconds {
	labels := []string{
		"rule_group",
	}
	return PrometheusRuleGroupIntervalSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_rule_group_interval_seconds",
			Help: "The interval of a rule group.",
		}, labels),
	}
}

type PrometheusRuleGroupIntervalSecondsAttr interface {
	Attribute
	implPrometheusRuleGroupIntervalSeconds()
}

func (a RuleGroupAttr) implPrometheusRuleGroupIntervalSeconds() {}

func (m PrometheusRuleGroupIntervalSeconds) With(
	extra ...PrometheusRuleGroupIntervalSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"rule_group": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusRuleGroupIterationsMissedTotal records the total number of rule group evaluations missed due to slow rule group evaluation.
type PrometheusRuleGroupIterationsMissedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusRuleGroupIterationsMissedTotal returns a new PrometheusRuleGroupIterationsMissedTotal instrument.
func NewPrometheusRuleGroupIterationsMissedTotal() PrometheusRuleGroupIterationsMissedTotal {
	labels := []string{
		"rule_group",
	}
	return PrometheusRuleGroupIterationsMissedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_rule_group_iterations_missed_total",
			Help: "The total number of rule group evaluations missed due to slow rule group evaluation.",
		}, labels),
	}
}

type PrometheusRuleGroupIterationsMissedTotalAttr interface {
	Attribute
	implPrometheusRuleGroupIterationsMissedTotal()
}

func (a RuleGroupAttr) implPrometheusRuleGroupIterationsMissedTotal() {}

func (m PrometheusRuleGroupIterationsMissedTotal) With(
	extra ...PrometheusRuleGroupIterationsMissedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"rule_group": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusRuleGroupIterationsTotal records the total number of scheduled rule group evaluations.
type PrometheusRuleGroupIterationsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusRuleGroupIterationsTotal returns a new PrometheusRuleGroupIterationsTotal instrument.
func NewPrometheusRuleGroupIterationsTotal() PrometheusRuleGroupIterationsTotal {
	labels := []string{
		"rule_group",
	}
	return PrometheusRuleGroupIterationsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_rule_group_iterations_total",
			Help: "The total number of scheduled rule group evaluations.",
		}, labels),
	}
}

type PrometheusRuleGroupIterationsTotalAttr interface {
	Attribute
	implPrometheusRuleGroupIterationsTotal()
}

func (a RuleGroupAttr) implPrometheusRuleGroupIterationsTotal() {}

func (m PrometheusRuleGroupIterationsTotal) With(
	extra ...PrometheusRuleGroupIterationsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"rule_group": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusRuleGroupLastDurationSeconds records the duration of the last rule group evaluation.
type PrometheusRuleGroupLastDurationSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusRuleGroupLastDurationSeconds returns a new PrometheusRuleGroupLastDurationSeconds instrument.
func NewPrometheusRuleGroupLastDurationSeconds() PrometheusRuleGroupLastDurationSeconds {
	labels := []string{
		"rule_group",
	}
	return PrometheusRuleGroupLastDurationSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_rule_group_last_duration_seconds",
			Help: "The duration of the last rule group evaluation.",
		}, labels),
	}
}

type PrometheusRuleGroupLastDurationSecondsAttr interface {
	Attribute
	implPrometheusRuleGroupLastDurationSeconds()
}

func (a RuleGroupAttr) implPrometheusRuleGroupLastDurationSeconds() {}

func (m PrometheusRuleGroupLastDurationSeconds) With(
	extra ...PrometheusRuleGroupLastDurationSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"rule_group": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusRuleGroupLastEvaluationSamples records the number of samples returned during the last rule group evaluation.
type PrometheusRuleGroupLastEvaluationSamples struct {
	*prometheus.GaugeVec
}

// NewPrometheusRuleGroupLastEvaluationSamples returns a new PrometheusRuleGroupLastEvaluationSamples instrument.
func NewPrometheusRuleGroupLastEvaluationSamples() PrometheusRuleGroupLastEvaluationSamples {
	labels := []string{
		"rule_group",
	}
	return PrometheusRuleGroupLastEvaluationSamples{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_rule_group_last_evaluation_samples",
			Help: "The number of samples returned during the last rule group evaluation.",
		}, labels),
	}
}

type PrometheusRuleGroupLastEvaluationSamplesAttr interface {
	Attribute
	implPrometheusRuleGroupLastEvaluationSamples()
}

func (a RuleGroupAttr) implPrometheusRuleGroupLastEvaluationSamples() {}

func (m PrometheusRuleGroupLastEvaluationSamples) With(
	extra ...PrometheusRuleGroupLastEvaluationSamplesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"rule_group": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusRuleGroupLastEvaluationTimestampSeconds records the timestamp of the last rule group evaluation.
type PrometheusRuleGroupLastEvaluationTimestampSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusRuleGroupLastEvaluationTimestampSeconds returns a new PrometheusRuleGroupLastEvaluationTimestampSeconds instrument.
func NewPrometheusRuleGroupLastEvaluationTimestampSeconds() PrometheusRuleGroupLastEvaluationTimestampSeconds {
	labels := []string{
		"rule_group",
	}
	return PrometheusRuleGroupLastEvaluationTimestampSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_rule_group_last_evaluation_timestamp_seconds",
			Help: "The timestamp of the last rule group evaluation.",
		}, labels),
	}
}

type PrometheusRuleGroupLastEvaluationTimestampSecondsAttr interface {
	Attribute
	implPrometheusRuleGroupLastEvaluationTimestampSeconds()
}

func (a RuleGroupAttr) implPrometheusRuleGroupLastEvaluationTimestampSeconds() {}

func (m PrometheusRuleGroupLastEvaluationTimestampSeconds) With(
	extra ...PrometheusRuleGroupLastEvaluationTimestampSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"rule_group": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusRuleGroupLastRestoreDurationSeconds records the duration of the last alert restoration from the ALERTS_FOR_STATE series.
type PrometheusRuleGroupLastRestoreDurationSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusRuleGroupLastRestoreDurationSeconds returns a new PrometheusRuleGroupLastRestoreDurationSeconds instrument.
func NewPrometheusRuleGroupLastRestoreDurationSeconds() PrometheusRuleGroupLastRestoreDurationSeconds {
	labels := []string{
		"rule_group",
	}
	return PrometheusRuleGroupLastRestoreDurationSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_rule_group_last_restore_duration_seconds",
			Help: "The duration of the last alert restoration from the ALERTS_FOR_STATE series.",
		}, labels),
	}
}

type PrometheusRuleGroupLastRestoreDurationSecondsAttr interface {
	Attribute
	implPrometheusRuleGroupLastRestoreDurationSeconds()
}

func (a RuleGroupAttr) implPrometheusRuleGroupLastRestoreDurationSeconds() {}

func (m PrometheusRuleGroupLastRestoreDurationSeconds) With(
	extra ...PrometheusRuleGroupLastRestoreDurationSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"rule_group": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusRuleGroupLastRuleDurationSumSeconds records the sum of the durations of all rules in the last rule group evaluation.
type PrometheusRuleGroupLastRuleDurationSumSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusRuleGroupLastRuleDurationSumSeconds returns a new PrometheusRuleGroupLastRuleDurationSumSeconds instrument.
func NewPrometheusRuleGroupLastRuleDurationSumSeconds() PrometheusRuleGroupLastRuleDurationSumSeconds {
	labels := []string{
		"rule_group",
	}
	return PrometheusRuleGroupLastRuleDurationSumSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_rule_group_last_rule_duration_sum_seconds",
			Help: "The sum of the durations of all rules in the last rule group evaluation.",
		}, labels),
	}
}

type PrometheusRuleGroupLastRuleDurationSumSecondsAttr interface {
	Attribute
	implPrometheusRuleGroupLastRuleDurationSumSeconds()
}

func (a RuleGroupAttr) implPrometheusRuleGroupLastRuleDurationSumSeconds() {}

func (m PrometheusRuleGroupLastRuleDurationSumSeconds) With(
	extra ...PrometheusRuleGroupLastRuleDurationSumSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"rule_group": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusRuleGroupRules records the number of rules in a rule group.
type PrometheusRuleGroupRules struct {
	*prometheus.GaugeVec
}

// NewPrometheusRuleGroupRules returns a new PrometheusRuleGroupRules instrument.
func NewPrometheusRuleGroupRules() PrometheusRuleGroupRules {
	labels := []string{
		"rule_group",
	}
	return PrometheusRuleGroupRules{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_rule_group_rules",
			Help: "The number of rules in a rule group.",
		}, labels),
	}
}

type PrometheusRuleGroupRulesAttr interface {
	Attribute
	implPrometheusRuleGroupRules()
}

func (a RuleGroupAttr) implPrometheusRuleGroupRules() {}

func (m PrometheusRuleGroupRules) With(
	extra ...PrometheusRuleGroupRulesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"rule_group": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}
