package rules

import (
	"context"

	"github.com/prometheus/prometheus/model/labels"
)

type ruleOrigin struct{}

// RuleDetail contains information about the rule that is being evaluated.
type RuleDetail struct {
	Name   string
	Query  string
	Labels labels.Labels
	Kind   string
}

const (
	KindAlerting  = "alerting"
	KindRecording = "recording"
)

// NewRuleDetail creates a RuleDetail from a given Rule.
func NewRuleDetail(r Rule) RuleDetail {
	var kind string
	switch r.(type) {
	case *AlertingRule:
		kind = KindAlerting
	case *RecordingRule:
		kind = KindRecording
	default:
		kind = "unknown"
	}

	return RuleDetail{
		Name:   r.Name(),
		Query:  r.Query().String(),
		Labels: r.Labels(),
		Kind:   kind,
	}
}

// NewOriginContext returns a new context with data about the origin attached.
func NewOriginContext(ctx context.Context, rule RuleDetail) context.Context {
	return context.WithValue(ctx, ruleOrigin{}, rule)
}

// FromOriginContext returns the RuleDetail origin data from the context.
func FromOriginContext(ctx context.Context) RuleDetail {
	if rule, ok := ctx.Value(ruleOrigin{}).(RuleDetail); ok {
		return rule
	}
	return RuleDetail{}
}
