package rules

import (
	"fmt"
	"github.com/matttproud/prometheus/model"
	"github.com/matttproud/prometheus/rules/ast"
	"time"
)

// A recorded rule.
type Rule struct {
	name      string
	vector    ast.VectorNode
	labels    model.LabelSet
	permanent bool
}

func (rule *Rule) Name() string { return rule.name }

func (rule *Rule) EvalRaw(timestamp *time.Time) ast.Vector {
	return rule.vector.Eval(timestamp)
}

func (rule *Rule) Eval(timestamp *time.Time) ast.Vector {
	// Get the raw value of the rule expression.
	vector := rule.EvalRaw(timestamp)

	// Override the metric name and labels.
	for _, sample := range vector {
		sample.Metric["name"] = model.LabelValue(rule.name)
		for label, value := range rule.labels {
			if value == "" {
				delete(sample.Metric, label)
			} else {
				sample.Metric[label] = value
			}
		}
	}
	return vector
}

func (rule *Rule) RuleToDotGraph() string {
	graph := "digraph \"Rules\" {\n"
	graph += fmt.Sprintf("%#p[shape=\"box\",label=\"%v = \"];\n", rule, rule.name)
	graph += fmt.Sprintf("%#p -> %#p;\n", rule, rule.vector)
	graph += rule.vector.NodeTreeToDotGraph()
	graph += "}\n"
	return graph
}

func NewRule(name string, labels model.LabelSet, vector ast.VectorNode, permanent bool) *Rule {
	return &Rule{
		name:      name,
		labels:    labels,
		vector:    vector,
		permanent: permanent,
	}
}
