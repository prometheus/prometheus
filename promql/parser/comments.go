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

package parser

import (
	"bytes"
	"sort"
	"strings"

	"github.com/prometheus/prometheus/promql/parser/posrange"
)

func (e *CommentedExpr) Pretty(int) string {
	return e.String()
}

func renderWithComments(expr Expr, comments []Comment) string {
	if len(comments) == 0 {
		return expr.String()
	}
	r := commentRenderer{
		comments: append([]Comment(nil), comments...),
	}
	sort.SliceStable(r.comments, func(i, j int) bool {
		return r.comments[i].PosRange.Start < r.comments[j].PosRange.Start
	})
	r.renderExpr(expr)
	r.flushRemaining()
	return strings.TrimRight(r.b.String(), "\n")
}

type commentRenderer struct {
	comments []Comment
	next     int
	b        strings.Builder
}

func (r *commentRenderer) renderExpr(expr Expr) {
	if expr == nil {
		return
	}
	pos := expr.PositionRange()
	r.flushBefore(pos.Start)
	if !r.hasCommentBefore(pos.End) {
		r.b.WriteString(expr.String())
		return
	}

	switch e := expr.(type) {
	case *CommentedExpr:
		r.renderExpr(e.Expr)
	case *AggregateExpr:
		var b bytes.Buffer
		e.writeAggOpStr(&b)
		r.b.Write(b.Bytes())
		r.b.WriteByte('(')
		if e.Op.IsAggregatorWithParam() {
			r.renderExpr(e.Param)
			r.b.WriteString(", ")
		}
		r.renderExpr(e.Expr)
		r.flushBefore(pos.End)
		r.b.WriteByte(')')
	case *BinaryExpr:
		r.renderExpr(e.LHS)
		r.b.WriteByte(' ')
		r.b.WriteString(e.Op.String())
		r.b.WriteString(e.returnBool())
		r.b.WriteString(e.getMatchingStr())
		r.b.WriteByte(' ')
		r.renderExpr(e.RHS)
	case *Call:
		r.b.WriteString(e.Func.Name)
		r.b.WriteByte('(')
		r.renderExpressions(e.Args)
		r.flushBefore(pos.End)
		r.b.WriteByte(')')
	case *ParenExpr:
		r.b.WriteByte('(')
		r.renderExpr(e.Expr)
		r.flushBefore(pos.End)
		r.b.WriteByte(')')
	case *UnaryExpr:
		r.b.WriteString(e.Op.String())
		r.renderExpr(e.Expr)
	default:
		r.flushBefore(pos.End)
		r.b.WriteString(expr.String())
	}
}

func (r *commentRenderer) renderExpressions(exprs Expressions) {
	for i, expr := range exprs {
		if i > 0 {
			r.b.WriteString(", ")
		}
		r.renderExpr(expr)
	}
}

func (r *commentRenderer) hasCommentBefore(end posrange.Pos) bool {
	return r.next < len(r.comments) && r.comments[r.next].PosRange.Start < end
}

func (r *commentRenderer) flushBefore(pos posrange.Pos) {
	for r.next < len(r.comments) && r.comments[r.next].PosRange.Start < pos {
		r.writeComment(r.comments[r.next])
		r.next++
	}
}

func (r *commentRenderer) flushRemaining() {
	for r.next < len(r.comments) {
		r.writeComment(r.comments[r.next])
		r.next++
	}
}

func (r *commentRenderer) writeComment(comment Comment) {
	if r.b.Len() != 0 && !strings.HasSuffix(r.b.String(), "\n") {
		r.b.WriteByte(' ')
	}
	r.b.WriteString(comment.Text)
	r.b.WriteByte('\n')
}
