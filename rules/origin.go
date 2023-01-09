package rules

import "context"

type ruleOrigin struct{}

type RuleDetail struct {
	Name  string
	Kind  string
	Query string
}

const KindAlerting = "alerting"
const KindRecording = "recording"

func NewRuleDetail(name, query, kind string) RuleDetail {
	return RuleDetail{
		Name:  name,
		Query: query,
		Kind:  kind,
	}
}

// NewOriginContext returns a new context with data about the origin attached.
func NewOriginContext(ctx context.Context, rule RuleDetail) context.Context {
	return context.WithValue(ctx, ruleOrigin{}, rule)
}

func FromOriginContext(ctx context.Context) RuleDetail {
	if rule, ok := ctx.Value(ruleOrigin{}).(RuleDetail); ok {
		return rule
	}
	return RuleDetail{}
}
