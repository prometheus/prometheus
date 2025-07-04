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
	"bytes"
	"fmt"
	"sort"
	"strconv"
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

func (node *AggregateExpr) ShortString() string {
	aggrString := node.getAggOpStr()
	return aggrString
}

func (node *AggregateExpr) getAggOpStr() string {
	aggrString := node.Op.String()

	switch {
	case node.Without:
		aggrString += fmt.Sprintf(" without (%s) ", joinLabels(node.Grouping))
	case len(node.Grouping) > 0:
		aggrString += fmt.Sprintf(" by (%s) ", joinLabels(node.Grouping))
	}

	return aggrString
}

func joinLabels(ss []string) string {
	var bytea [1024]byte // On stack to avoid memory allocation while building the output.
	b := bytes.NewBuffer(bytea[:0])

	for i, s := range ss {
		if i > 0 {
			b.WriteString(", ")
		}
		if !model.IsValidLegacyMetricName(string(model.LabelValue(s))) {
			b.Write(strconv.AppendQuote(b.AvailableBuffer(), s))
		} else {
			b.WriteString(s)
		}
	}
	return b.String()
}

func (node *BinaryExpr) returnBool() string {
	if node.ReturnBool {
		return " bool"
	}
	return ""
}

func (node *BinaryExpr) String() string {
	matching := node.getMatchingStr()
	return fmt.Sprintf("%s %s%s%s %s", node.LHS, node.Op, node.returnBool(), matching, node.RHS)
}

func (node *BinaryExpr) ShortString() string {
	return fmt.Sprintf("%s%s%s", node.Op, node.returnBool(), node.getMatchingStr())
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

func (node *DurationExpr) String() string {
	var expr string
	switch {
	case node.Op == STEP:
		expr = "step()"
	case node.Op == MIN:
		expr = fmt.Sprintf("min(%s, %s)", node.LHS, node.RHS)
	case node.Op == MAX:
		expr = fmt.Sprintf("max(%s, %s)", node.LHS, node.RHS)
	case node.LHS == nil:
		// This is a unary duration expression.
		switch node.Op {
		case SUB:
			expr = fmt.Sprintf("%s%s", node.Op, node.RHS)
		case ADD:
			expr = node.RHS.String()
		default:
			// This should never happen.
			panic(fmt.Sprintf("unexpected unary duration expression: %s", node.Op))
		}
	default:
		expr = fmt.Sprintf("%s %s %s", node.LHS, node.Op, node.RHS)
	}
	if node.Wrapped {
		return fmt.Sprintf("(%s)", expr)
	}
	return expr
}

func (node *DurationExpr) ShortString() string {
	return node.Op.String()
}

func (node *Call) String() string {
	return fmt.Sprintf("%s(%s)", node.Func.Name, node.Args)
}

func (node *Call) ShortString() string {
	return node.Func.Name
}

func (node *MatrixSelector) atOffset() (string, string) {
	// Copy the Vector selector before changing the offset
	vecSelector := node.VectorSelector.(*VectorSelector)
	offset := ""
	switch {
	case vecSelector.OriginalOffsetExpr != nil:
		offset = fmt.Sprintf(" offset %s", vecSelector.OriginalOffsetExpr)
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
	return at, offset
}

func (node *MatrixSelector) String() string {
	at, offset := node.atOffset()
	// Copy the Vector selector before changing the offset
	vecSelector := *node.VectorSelector.(*VectorSelector)
	// Do not print the @ and offset twice.
	offsetVal, offsetExprVal, atVal, preproc := vecSelector.OriginalOffset, vecSelector.OriginalOffsetExpr, vecSelector.Timestamp, vecSelector.StartOrEnd
	vecSelector.OriginalOffset = 0
	vecSelector.OriginalOffsetExpr = nil
	vecSelector.Timestamp = nil
	vecSelector.StartOrEnd = 0

	rangeStr := model.Duration(node.Range).String()
	if node.RangeExpr != nil {
		rangeStr = node.RangeExpr.String()
	}
	str := fmt.Sprintf("%s[%s]%s%s", vecSelector.String(), rangeStr, at, offset)

	vecSelector.OriginalOffset, vecSelector.OriginalOffsetExpr, vecSelector.Timestamp, vecSelector.StartOrEnd = offsetVal, offsetExprVal, atVal, preproc

	return str
}

func (node *MatrixSelector) ShortString() string {
	at, offset := node.atOffset()
	rangeStr := model.Duration(node.Range).String()
	if node.RangeExpr != nil {
		rangeStr = node.RangeExpr.String()
	}
	return fmt.Sprintf("[%s]%s%s", rangeStr, at, offset)
}

func (node *SubqueryExpr) String() string {
	return fmt.Sprintf("%s%s", node.Expr.String(), node.getSubqueryTimeSuffix())
}

func (node *SubqueryExpr) ShortString() string {
	return node.getSubqueryTimeSuffix()
}

// getSubqueryTimeSuffix returns the '[<range>:<step>] @ <timestamp> offset <offset>' suffix of the subquery.
func (node *SubqueryExpr) getSubqueryTimeSuffix() string {
	step := ""
	if node.Step != 0 {
		step = model.Duration(node.Step).String()
	} else if node.StepExpr != nil {
		step = node.StepExpr.String()
	}
	offset := ""
	switch {
	case node.OriginalOffsetExpr != nil:
		offset = fmt.Sprintf(" offset %s", node.OriginalOffsetExpr)
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
	rangeStr := model.Duration(node.Range).String()
	if node.RangeExpr != nil {
		rangeStr = node.RangeExpr.String()
	}
	return fmt.Sprintf("[%s:%s]%s%s", rangeStr, step, at, offset)
}

func (node *NumberLiteral) String() string {
	if node.Duration {
		if node.Val < 0 {
			return fmt.Sprintf("-%s", model.Duration(-node.Val*1e9).String())
		}
		return model.Duration(node.Val * 1e9).String()
	}
	return strconv.FormatFloat(node.Val, 'f', -1, 64)
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

func (node *UnaryExpr) ShortString() string {
	return node.Op.String()
}

func (node *VectorSelector) String() string {
	var labelStrings []string
	if len(node.LabelMatchers) > 1 {
		labelStrings = make([]string, 0, len(node.LabelMatchers)-1)
	}
	for _, matcher := range node.LabelMatchers {
		// Only include the __name__ label if its equality matching and matches the name, but don't skip if it's an explicit empty name matcher.
		if matcher.Name == labels.MetricName && matcher.Type == labels.MatchEqual && matcher.Value == node.Name && matcher.Value != "" {
			continue
		}
		labelStrings = append(labelStrings, matcher.String())
	}
	offset := ""
	switch {
	case node.OriginalOffsetExpr != nil:
		offset = fmt.Sprintf(" offset %s", node.OriginalOffsetExpr)
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
