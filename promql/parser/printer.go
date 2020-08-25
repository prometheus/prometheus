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
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/promlog"
	"strings"
)

var logger = promlog.New(&promlog.Config{})

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
	prettyExpr, err := PromqlPrettier(node.ExprString()).Prettify()
	if err != nil {
		level.Error(logger).Log(errors.Wrap(err, "pretty-print"))
		return node.ExprString()
	}
	return prettyExpr
}

func (node *BinaryExpr) String() string {
	prettyExpr, err := PromqlPrettier(node.ExprString()).Prettify()
	if err != nil {
		level.Error(logger).Log(errors.Wrap(err, "pretty-print"))
		return node.ExprString()
	}
	return prettyExpr
}

func (node *Call) String() string {
	prettyExpr, err := PromqlPrettier(node.ExprString()).Prettify()
	if err != nil {
		level.Error(logger).Log(errors.Wrap(err, "pretty-print"))
		return node.ExprString()
	}
	return prettyExpr
}

func (node *MatrixSelector) String() string {
	prettyExpr, err := PromqlPrettier(node.ExprString()).Prettify()
	if err != nil {
		level.Error(logger).Log(errors.Wrap(err, "pretty-print"))
		return node.ExprString()
	}
	return prettyExpr
}

func (node *SubqueryExpr) String() string {
	prettyExpr, err := PromqlPrettier(node.ExprString()).Prettify()
	if err != nil {
		level.Error(logger).Log(errors.Wrap(err, "pretty-print"))
		return node.ExprString()
	}
	return prettyExpr
}

func (node *NumberLiteral) String() string {
	prettyExpr, err := PromqlPrettier(node.ExprString()).Prettify()
	if err != nil {
		level.Error(logger).Log(errors.Wrap(err, "pretty-print"))
		return node.ExprString()
	}
	return prettyExpr
}

func (node *ParenExpr) String() string {
	prettyExpr, err := PromqlPrettier(node.ExprString()).Prettify()
	if err != nil {
		level.Error(logger).Log(errors.Wrap(err, "pretty-print"))
		return node.ExprString()
	}
	return prettyExpr
}

func (node *StringLiteral) String() string {
	prettyExpr, err := PromqlPrettier(node.ExprString()).Prettify()
	if err != nil {
		level.Error(logger).Log(errors.Wrap(err, "pretty-print"))
		return node.ExprString()
	}
	return prettyExpr
}

func (node *UnaryExpr) String() string {
	prettyExpr, err := PromqlPrettier(node.ExprString()).Prettify()
	if err != nil {
		level.Error(logger).Log(errors.Wrap(err, "pretty-print"))
		return node.ExprString()
	}
	return prettyExpr
}

func (node *VectorSelector) String() string {
	prettyExpr, err := PromqlPrettier(node.ExprString()).Prettify()
	if err != nil {
		level.Error(logger).Log(errors.Wrap(err, "pretty-print"))
		return node.ExprString()
	}
	return prettyExpr
}
