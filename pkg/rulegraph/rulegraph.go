// Copyright 2017 The Prometheus Authors
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

package rulegraph

import (
	"fmt"
	"io"
	"strings"

	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
)

type ruleType uint8

const (
	recorded ruleType = 0
	alert    ruleType = 1
)

type metricFinder struct {
	names  []string
	visits int
}

func (mf *metricFinder) Visit(node parser.Node, path []parser.Node) (parser.Visitor, error) {
	if node == nil && path == nil {
		return nil, nil
	}

	mf.visits++

	if vs, ok := node.(*parser.VectorSelector); ok {
		mf.names = append(mf.names, vs.Name)
		return nil, nil
	}

	return mf, nil
}

// Return all dependency edges from a single rule, this is basically
// "record/alerd name -> <each metric used in the expr>" with all
// label filters stripped out.
func diagramEdges(r rulefmt.RuleNode) []string {
	var acc []string

	name := ruleName(r)
	parsed, err := parser.ParseExpr(r.Expr.Value)
	if err != nil {
		// This should already have been verified
		return acc
	}

	var mf metricFinder
	err = parser.Walk(&mf, parsed, nil)
	if err != nil {
		return acc
	}

	for _, next := range mf.names {
		acc = append(acc, fmt.Sprintf("%s -> %s", name, next))
	}

	return acc
}

func getType(r rulefmt.RuleNode) ruleType {
	if r.Record.Value == "" {
		return alert
	}

	return recorded
}

func ruleName(r rulefmt.RuleNode) string {
	name := r.Record.Value
	if name == "" {
		name = r.Alert.Value
	}

	if i := strings.Index(name, "{"); i >= 0 {
		name = name[0:i]
	}

	return name
}

// Build a diagram of the interdependency of all rule files passed
// in. We expect that these have already been checked for errors and
// passed that check.
//
// When we have processed these, simply serialise the graph as a DOT
// graph to w.
func BuildRuleDiagram(groups []rulefmt.RuleGroup, w io.Writer) {
	nodes := make(map[string]ruleType)
	edges := make(map[string]bool)

	for _, group := range groups {
		for _, rule := range group.Rules {
			nodes[ruleName(rule)] = getType(rule)
			for _, edge := range diagramEdges(rule) {
				edges[edge] = true
			}
		}
	}

	fmt.Fprintf(w, "digraph {\n")
	for name, t := range nodes {
		switch {
		case t == recorded:
			fmt.Fprintf(w, "  %s [shape=oval]\n", name)
		case t == alert:
			fmt.Fprintf(w, "  %s [shape=doubleoctagon]", name)
		default:
			fmt.Fprintf(w, "  /* Unknown node type %v for %s */\n", t, name)
		}
	}

	fmt.Fprintf(w, "\n")

	for edge, _ := range edges {
		fmt.Fprintf(w, "  %s\n", edge)
	}

	fmt.Fprintf(w, "}\n")
}
