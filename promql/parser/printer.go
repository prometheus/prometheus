// Copyright 2015 The Prometheus Authors
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

package parser

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
)

// Tree returns a string of the tree structure of the given node.
func Tree(node Node) string {
	return tree(node, "")
}

func tree(node Node, level string) string {
	if node == nil {
		return fmt.Sprintf("%s |---- %T\n", level, node)
	}
	typs := strings.Split(fmt.Sprintf("%T", node), ".")[1]

	t := fmt.Sprintf("%s |---- %s :: %s\n", level, typs, node)

	level += " · · ·"

	for _, e := range Children(node) {
		t += tree(e, level)
	}

	return t
}

func (node *EvalStmt) String() string {
	return "EVAL " + node.Expr.String()
}

func (es Expressions) String() (s string) {
	if len(es) == 0 {
		return ""
	}
	for _, e := range es {
		s += e.String()
		s += ", "
	}
	return s[:len(s)-2]
}

func (node *AggregateExpr) String() string {
	aggrString := node.getAggOpStr()
	aggrString += "("
	if node.Op.IsAggregatorWithParam() {
		aggrString += fmt.Sprintf("%s, ", node.Param)
	}
	aggrString += fmt.Sprintf("%s)", node.Expr)

	return aggrString
}

func (node *AggregateExpr) getAggOpStr() string {
	aggrString := node.Op.String()

	switch {
	case node.Without:
		aggrString += fmt.Sprintf(" without (%s) ", strings.Join(node.Grouping, ", "))
	case len(node.Grouping) > 0:
		aggrString += fmt.Sprintf(" by (%s) ", strings.Join(node.Grouping, ", "))
	}

	return aggrString
}

func (node *BinaryExpr) String() string {
	returnBool := ""
	if node.ReturnBool {
		returnBool = " bool"
	}

	matching := node.getMatchingStr()
	return fmt.Sprintf("%s %s%s%s %s", node.LHS, node.Op, returnBool, matching, node.RHS)
}

func (node *BinaryExpr) getMatchingStr() string {
	matching := ""
	vm := node.VectorMatching
	if vm != nil && (len(vm.MatchingLabels) > 0 || vm.On) {
		vmTag := "ignoring"
		if vm.On {
			vmTag = "on"
		}
		matching = fmt.Sprintf(" %s (%s)", vmTag, strings.Join(vm.MatchingLabels, ", "))

		if vm.Card == CardManyToOne || vm.Card == CardOneToMany {
			vmCard := "right"
			if vm.Card == CardManyToOne {
				vmCard = "left"
			}
			matching += fmt.Sprintf(" group_%s (%s)", vmCard, strings.Join(vm.Include, ", "))
		}
	}
	return matching
}

func (node *Call) String() string {
	return fmt.Sprintf("%s(%s)", node.Func.Name, node.Args)
}

func (node *MatrixSelector) String() string {
	// Copy the Vector selector before changing the offset
	vecSelector := *node.VectorSelector.(*VectorSelector)
	offset := ""
	switch {
	case vecSelector.OriginalOffset > time.Duration(0):
		offset = fmt.Sprintf(" offset %s", model.Duration(vecSelector.OriginalOffset))
	case vecSelector.OriginalOffset < time.Duration(0):
		offset = fmt.Sprintf(" offset -%s", model.Duration(-vecSelector.OriginalOffset))
	}
	at := ""
	switch {
	case vecSelector.Timestamp != nil:
		at = fmt.Sprintf(" @ %.3f", float64(*vecSelector.Timestamp)/1000.0)
	case vecSelector.StartOrEnd == START:
		at = " @ start()"
	case vecSelector.StartOrEnd == END:
		at = " @ end()"
	}

	// Do not print the @ and offset twice.
	offsetVal, atVal, preproc := vecSelector.OriginalOffset, vecSelector.Timestamp, vecSelector.StartOrEnd
	vecSelector.OriginalOffset = 0
	vecSelector.Timestamp = nil
	vecSelector.StartOrEnd = 0

	str := fmt.Sprintf("%s[%s]%s%s", vecSelector.String(), model.Duration(node.Range), at, offset)

	vecSelector.OriginalOffset, vecSelector.Timestamp, vecSelector.StartOrEnd = offsetVal, atVal, preproc

	return str
}

func (node *SubqueryExpr) String() string {
	return fmt.Sprintf("%s%s", node.Expr.String(), node.getSubqueryTimeSuffix())
}

// getSubqueryTimeSuffix returns the '[<range>:<step>] @ <timestamp> offset <offset>' suffix of the subquery.
func (node *SubqueryExpr) getSubqueryTimeSuffix() string {
	step := ""
	if node.Step != 0 {
		step = model.Duration(node.Step).String()
	}
	offset := ""
	switch {
	case node.OriginalOffset > time.Duration(0):
		offset = fmt.Sprintf(" offset %s", model.Duration(node.OriginalOffset))
	case node.OriginalOffset < time.Duration(0):
		offset = fmt.Sprintf(" offset -%s", model.Duration(-node.OriginalOffset))
	}
	at := ""
	switch {
	case node.Timestamp != nil:
		at = fmt.Sprintf(" @ %.3f", float64(*node.Timestamp)/1000.0)
	case node.StartOrEnd == START:
		at = " @ start()"
	case node.StartOrEnd == END:
		at = " @ end()"
	}
	return fmt.Sprintf("[%s:%s]%s%s", model.Duration(node.Range), step, at, offset)
}

func (node *NumberLiteral) String() string {
	return fmt.Sprint(node.Val)
}

func (node *ParenExpr) String() string {
	return fmt.Sprintf("(%s)", node.Expr)
}

func (node *StringLiteral) String() string {
	return fmt.Sprintf("%q", node.Val)
}

func (node *UnaryExpr) String() string {
	return fmt.Sprintf("%s%s", node.Op, node.Expr)
}

func (node *VectorSelector) String() string {
	var labelStrings []string
	if len(node.LabelMatchers) > 1 {
		labelStrings = make([]string, 0, len(node.LabelMatchers)-1)
	}
	for _, matcher := range node.LabelMatchers {
		// Only include the __name__ label if its equality matching and matches the name.
		if matcher.Name == labels.MetricName && matcher.Type == labels.MatchEqual && matcher.Value == node.Name {
			continue
		}
		labelStrings = append(labelStrings, matcher.String())
	}
	offset := ""
	switch {
	case node.OriginalOffset > time.Duration(0):
		offset = fmt.Sprintf(" offset %s", model.Duration(node.OriginalOffset))
	case node.OriginalOffset < time.Duration(0):
		offset = fmt.Sprintf(" offset -%s", model.Duration(-node.OriginalOffset))
	}
	at := ""
	switch {
	case node.Timestamp != nil:
		at = fmt.Sprintf(" @ %.3f", float64(*node.Timestamp)/1000.0)
	case node.StartOrEnd == START:
		at = " @ start()"
	case node.StartOrEnd == END:
		at = " @ end()"
	}

	if len(labelStrings) == 0 {
		return fmt.Sprintf("%s%s%s", node.Name, at, offset)
	}
	sort.Strings(labelStrings)
	return fmt.Sprintf("%s{%s}%s%s", node.Name, strings.Join(labelStrings, ","), at, offset)
}
