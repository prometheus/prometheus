package prettier

import (
	"github.com/prometheus/prometheus/promql/parser"
	"reflect"
)

type nodeInfo struct {
	node parser.Expr
	// node information details.
	// information details should be used only after calling the fetch.
	shouldSplit bool
	columnLimit int
}

func newNodeInfo(node parser.Expr, columnLimit int) *nodeInfo {
	return &nodeInfo{
		node:        node,
		columnLimit: columnLimit,
	}
}

// fetch fetches the node information.
func (p *nodeInfo) fetch() {
	p.shouldSplit = p.violatesColumnLimit()
}

func (p *nodeInfo) violatesColumnLimit() bool {
	return len(p.node.String()) > p.columnLimit
}

// getNode returns the node corresponding to the given position range in the AST.
func (p *nodeInfo) getNode(headAST parser.Expr, rnge parser.PositionRange) parser.Expr {
	_, node, _ := p.getCurrentNodeHistory(headAST, rnge, []reflect.Type{})
	return node
}

// getParentNode returns the parent node of the given node/item position range.
func (p *nodeInfo) getParentNode(headAST parser.Expr, rnge parser.PositionRange) reflect.Type {
	// TODO: var found is of no use. Hence, remove it and the bool return from the getNodeAncestorsStack
	// since found condition can be handled by ancestors alone(confirmation needed).
	ancestors, _, found := p.getCurrentNodeHistory(headAST, rnge, []reflect.Type{})
	if !found {
		return nil
	}
	if len(ancestors) < 2 {
		return nil
	}
	return ancestors[len(ancestors)-2]
}

// getCurrentNodeHistory returns the ancestors along with the node in which the item is present, using the help of AST.
// posRange can also be called as itemPosRange since carries the position range of the lexical item.
func (p *nodeInfo) getCurrentNodeHistory(head parser.Expr, posRange parser.PositionRange, stack []reflect.Type) ([]reflect.Type, parser.Expr, bool) {
	switch n := head.(type) {
	case *parser.ParenExpr:
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			stack = append(stack, reflect.TypeOf(n))
			p.getCurrentNodeHistory(n.Expr, posRange, stack)
		}
	case *parser.VectorSelector:
		stack = append(stack, reflect.TypeOf(n))
	case *parser.BinaryExpr:
		stack = append(stack, reflect.TypeOf(n))
		stmp, _head, found := p.getCurrentNodeHistory(n.LHS, posRange, stack)
		if found {
			return stmp, _head, true
		}
		stmp, _head, found = p.getCurrentNodeHistory(n.RHS, posRange, stack)
		if found {
			return stmp, _head, true
		}
		// if none of the above are true, that implies, the position range is of a symbol or a grouping modifier.
	case *parser.AggregateExpr:
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			stack = append(stack, reflect.TypeOf(n))
			p.getCurrentNodeHistory(n.Expr, posRange, stack)
		}
	case *parser.Call:
		stack = append(stack, reflect.TypeOf(n))
		for _, exprs := range n.Args {
			if exprs.PositionRange().Start <= posRange.Start && exprs.PositionRange().End >= posRange.End {
				stmp, _head, found := p.getCurrentNodeHistory(exprs, posRange, stack)
				if found {
					return stmp, _head, true
				}
			}
		}
	case *parser.MatrixSelector:
		stack = append(stack, reflect.TypeOf(n))
		if n.VectorSelector.PositionRange().Start <= posRange.Start && n.VectorSelector.PositionRange().End >= posRange.End {
			p.getCurrentNodeHistory(n.VectorSelector, posRange, stack)
		}
	case *parser.UnaryExpr:
		stack = append(stack, reflect.TypeOf(n))
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			p.getCurrentNodeHistory(n.Expr, posRange, stack)
		}
	case *parser.SubqueryExpr:
		stack = append(stack, reflect.TypeOf(n))
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			p.getCurrentNodeHistory(n.Expr, posRange, stack)
		}
	case *parser.NumberLiteral, *parser.StringLiteral:
		stack = append(stack, reflect.TypeOf(n))
	}
	return stack, head, true
}
