package prettier

import (
	"github.com/prometheus/prometheus/promql/parser"
	"reflect"
)

type nodeInfo struct {
	head parser.Expr
	// Node details.
	columnLimit int
	history     []reflect.Type
	item        parser.Item
}

func newNodeInfo(head parser.Expr, columnLimit int, item parser.Item) *nodeInfo {
	return &nodeInfo{
		head:        head,
		columnLimit: columnLimit,
		item:        item,
	}
}

func (p *nodeInfo) violatesColumnLimit() bool {
	return len(p.head.String()) > p.columnLimit
}

// getNode returns the node corresponding to the given position range in the AST.
func (p *nodeInfo) getNode(root parser.Expr, item parser.Item) parser.Expr {
	history, node, _ := p.nodeHistory(root, item.PositionRange(), []reflect.Type{})
	p.history = history
	return node
}

func (p *nodeInfo) baseIndent(item parser.Item) int {
	history, _, _ := p.nodeHistory(p.head, item.PositionRange(), []reflect.Type{})
	history = reduceContinuous(history, "*parser.BinaryExpr")
	return len(history)
}

// contains verifies whether the current node contains a particular entity.
func (p *nodeInfo) contains(element string) bool {
	switch element {
	case "grouping-modifier":
		switch n := p.head.(type) {
		case *parser.BinaryExpr:
			return len(n.VectorMatching.MatchingLabels) > 0 || n.ReturnBool
		}
	}
	return false
}

// parentNode returns the parent node of the given node/item position range.
func (p *nodeInfo) parentNode(head parser.Expr, rnge parser.PositionRange) reflect.Type {
	ancestors, _, found := p.nodeHistory(head, rnge, []reflect.Type{})
	if !found {
		return nil
	}
	if len(ancestors) < 2 {
		return nil
	}
	return ancestors[len(ancestors)-2]
}

// nodeHistory returns the ancestors of the node the item position range is passed of,
// along with the node in which the item is present. This is done with the help of AST.
// posRange can also be called as itemPosRange since carries the position range of the lexical item.
func (p *nodeInfo) nodeHistory(head parser.Expr, posRange parser.PositionRange, stack []reflect.Type) ([]reflect.Type, parser.Expr, bool) {
	var nodeMatch bool
	switch n := head.(type) {
	case *parser.ParenExpr:
		nodeMatch = true
		stack = append(stack, reflect.TypeOf(n))
		if n.PositionRange().Start <= posRange.Start && n.PositionRange().End >= posRange.End {
			return p.nodeHistory(n.Expr, posRange, stack)
		}
	case *parser.VectorSelector:
		if n.PositionRange().Start <= posRange.Start && n.PositionRange().End >= posRange.End {
			nodeMatch = true
			stack = append(stack, reflect.TypeOf(n))
		}
	case *parser.BinaryExpr:
		stack = append(stack, reflect.TypeOf(n))
		stmp, node, found := p.nodeHistory(n.LHS, posRange, stack)
		if found {
			return stmp, node, found
		}
		stmp, node, found = p.nodeHistory(n.RHS, posRange, stack)
		if found {
			return stmp, node, found
		}
		// Since the item exists in both the child. This means that it is in binary expr range,
		// but not satisfied by a single child. This is possible only for Op and grouping
		// modifiers.
		if n.PositionRange().Start <= posRange.Start && n.PositionRange().End >= posRange.End {
			return stack, head, true
		}
		return stack, head, false
	case *parser.AggregateExpr:
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			nodeMatch = true
			stack = append(stack, reflect.TypeOf(n))
			p.nodeHistory(n.Expr, posRange, stack)
		}
	case *parser.Call:
		stack = append(stack, reflect.TypeOf(n))
		for _, exprs := range n.Args {
			nodeMatch = true
			if exprs.PositionRange().Start <= posRange.Start && exprs.PositionRange().End >= posRange.End {
				stmp, _head, found := p.nodeHistory(exprs, posRange, stack)
				if found {
					return stmp, _head, true
				}
			}
		}
	case *parser.MatrixSelector:
		stack = append(stack, reflect.TypeOf(n))
		if n.VectorSelector.PositionRange().Start <= posRange.Start && n.VectorSelector.PositionRange().End >= posRange.End {
			nodeMatch = true
			p.nodeHistory(n.VectorSelector, posRange, stack)
		}
	case *parser.UnaryExpr:
		stack = append(stack, reflect.TypeOf(n))
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			nodeMatch = true
			p.nodeHistory(n.Expr, posRange, stack)
		}
	case *parser.SubqueryExpr:
		stack = append(stack, reflect.TypeOf(n))
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			nodeMatch = true
			p.nodeHistory(n.Expr, posRange, stack)
		}
	case *parser.NumberLiteral, *parser.StringLiteral:
		nodeMatch = true
		stack = append(stack, reflect.TypeOf(n))
	}
	return stack, head, nodeMatch
}

// reduceContinuous reduces from end, the continuous
// occurrence of a type to its single representation.
func reduceContinuous(history []reflect.Type, typ string) []reflect.Type {
	var temp []reflect.Type
	for i := 0; i < len(history)-1; i++ {
		if history[i].String() == typ && history[i].String() != history[i+1].String() {
			temp = append(temp, history[i])
		} else if history[i].String() != typ {
			temp = append(temp, history[i])
		}
	}
	if history[len(history)-1].String() != typ {
		temp = append(temp, history[len(history)-1])
	}
	return temp
}
